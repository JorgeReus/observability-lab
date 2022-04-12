package banking_gateway_sqs

import (
	"context"
	"credit-score-service/application/tracing"
	banking_gateway "credit-score-service/core/baking_gateway"
	"credit-score-service/core/credit_score"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"reflect"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"

	oteltrace "go.opentelemetry.io/otel/trace"
)

const (
	VISIBILITY_TIMEOUT = int32(15)
)

var (
	opsProcessed = promauto.NewCounter(prometheus.CounterOpts{
		Name: "total_banking_requests_processed",
		Help: "The total number of processed events for banking_requests",
	})
	IsInitialized = false
	IsHealthy     = false
)

type BankingGatewaySQSClient struct {
	requestsQueueURL  *string
	responsesQueueURL *string
	api               *sqs.Client
}

func New() (banking_gateway.Client, error) {
	endpointUrl := os.Getenv("ENDPOINT_URL")
	var cfg aws.Config
	var err error
	if endpointUrl != "" {
		customResolver := aws.EndpointResolverWithOptionsFunc(func(service, region string, options ...interface{}) (aws.Endpoint, error) {
			return aws.Endpoint{
				URL:           endpointUrl,
				SigningRegion: region,
			}, nil
		})
		cfg, err = config.LoadDefaultConfig(context.Background(), config.WithEndpointResolverWithOptions(customResolver))
	} else {
		cfg, err = config.LoadDefaultConfig(context.Background())
	}

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Couldn't create credit score client: %v", err))
	}

	client := sqs.NewFromConfig(cfg)

	requestsQueueName := os.Getenv("BANKING_REQUESTS_QUEUE_NAME")
	reqQueueUrlOutput, err := client.GetQueueUrl(context.Background(), &sqs.GetQueueUrlInput{
		QueueName: &requestsQueueName,
	})

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Couldn't get url for queue %s: %v", requestsQueueName, err))
	}

	responsesQueueName := os.Getenv("BANKING_RESPONSES_QUEUE_NAME")
	respQueueUrlOutput, err := client.GetQueueUrl(context.Background(), &sqs.GetQueueUrlInput{
		QueueName: &responsesQueueName,
	})
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Couldn't get url for queue %s: %v", responsesQueueName, err))
	}
	IsInitialized = true
	IsHealthy = true
	return &BankingGatewaySQSClient{
		requestsQueueURL:  reqQueueUrlOutput.QueueUrl,
		responsesQueueURL: respQueueUrlOutput.QueueUrl,
		api:               client,
	}, nil
}

func (c *BankingGatewaySQSClient) Send(resp *banking_gateway.BankingGatewayRequest) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	data, err := json.Marshal(resp)
	if err != nil {
		return errors.New(fmt.Sprintf("Cannot marshall message data: %v", err))
	}
	stringData := string(data)
	_, err = c.api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    c.requestsQueueURL,
		MessageBody: &stringData,
	})

	if err != nil {
		return errors.New(fmt.Sprintf("Cannot send sqs message: %v", err))
	}
	return nil
}

func (c *BankingGatewaySQSClient) processMsg(handlerFunc func(msg *banking_gateway.BankingGatesWayResponse) error, req *banking_gateway.BankingGatesWayResponse, receiptHandle *string, tp *tracing.TracingProvider) error {
	if req != nil && len(req.Data) > 0 {
		// Instrument SQS recv
		traceContext := req.Data["tracingInformation"].(map[string]interface{})
		prop := propagation.TraceContext{}
		ctxCall := prop.Extract(context.Background(), propagation.MapCarrier{
			"traceparent": traceContext["traceparent"].(string),
		})
		_, span := tp.GetTracer().Start(ctxCall, "processBankingResponse", oteltrace.WithAttributes(
			attribute.String("req.userId", req.Data["userId"].(string)),
			attribute.String("req.bankingInstitutionId", req.Data["bankingInstitutionId"].(string)),
		))

		defer span.End()

		// Apply the function and then continue the process
		if err := handlerFunc(req); err == nil {
			ctxDelete, cancel := context.WithTimeout(ctxCall, time.Second*5)
			defer cancel()
			_, err = c.api.DeleteMessage(ctxDelete, &sqs.DeleteMessageInput{
				QueueUrl:      c.responsesQueueURL,
				ReceiptHandle: receiptHandle,
			})

			// Idempotent operation dead letter queues not needed
			if err != nil {
				span.RecordError(errors.New(fmt.Sprintf("Couldn't delete message %s: %v", *receiptHandle, err)))
				fmt.Println(fmt.Sprintf("Couldn't delete message %s : %v", *receiptHandle, err))
			}

			opsProcessed.Inc()
		} else {
			span.RecordError(errors.New(fmt.Sprintf("Couldn't process message : %v", receiptHandle)))
			fmt.Println(fmt.Sprintf("Couldn't process message %s : %v", *receiptHandle, err))
		}
		fmt.Println("finished recv")
	}
	return nil
}

// func (c *BankingGatewaySQSClient) Recv(handlerFunc func(msg *banking_gateway.BankingGatesWayResponse) error) error {
//   ctx := context.Background()
//   log.Println(fmt.Sprintf("Listening queue: %v", *c.requestsQueueURL))
//   for {
//     select {
//     case <-ctx.Done():
//       log.Println("Stopping polling because a context kill signal was sent")
//       return nil
//     default:
//       tp := tracing.NewProvider()
//       req, receiptHandle, err := c.recv()
//       if err != nil && len(req.Data) > 0 {
//         // Record error in a new span
//         IsHealthy = false
//         traceparent := req.Data["tracingInformation"].(string)
//         prop := propagation.TraceContext{}
//         ctxCall := prop.Extract(context.Background(), propagation.MapCarrier{
//           "traceparent": traceparent,
//         })
//         _, span := tp.GetTracer().Start(ctxCall, "recvCalculateScore")
//         log.Printf(fmt.Sprintf("Couldn't recieve message : %v", err))
//         span.RecordError(errors.New(fmt.Sprintf("Couldn't recieve message : %v", err)))
//       }
//       // Handle process in a goroutine
//       IsHealthy = true
//       go c.processMsg(handlerFunc, req, receiptHandle, tp)
//     }
//   }
// }

func (c *BankingGatewaySQSClient) recv() (*banking_gateway.BankingGatesWayResponse, *string, error) {
	recvMsgInput := &sqs.ReceiveMessageInput{
		MessageAttributeNames: []string{
			string(types.QueueAttributeNameAll),
		},
		WaitTimeSeconds:   1,
		QueueUrl:          c.responsesQueueURL,
		VisibilityTimeout: VISIBILITY_TIMEOUT,
	}

	msgOutput, err := c.api.ReceiveMessage(context.TODO(), recvMsgInput)
	if err != nil {
		return nil, nil, err
	}

	if len(msgOutput.Messages) > 0 {
		msg := msgOutput.Messages[0]
		var req banking_gateway.BankingGatesWayResponse
		if err := json.Unmarshal([]byte(*msg.Body), &req); err != nil || reflect.DeepEqual(req, credit_score.CreditScoreRequest{}) {
			return nil, nil, errors.New(fmt.Sprintf("Unknown message format, cannot parse json: %v", *msg.Body))
		}
		return &req, msg.ReceiptHandle, nil
	} else {
		return nil, nil, nil
	}
}

func (c *BankingGatewaySQSClient) Recv() (float64, error) {
	log.Println(fmt.Sprintf("Listening queue: %v", *c.requestsQueueURL))
	tp := tracing.NewProvider()
	var req *banking_gateway.BankingGatesWayResponse
	var err error
	for req == nil {
		req, _, err = c.recv()
	}

	tracingInfo := req.Data["tracingInformation"].(map[string]interface{})
	traceparent := tracingInfo["traceparent"].(string)
	prop := propagation.TraceContext{}
	ctxCall := prop.Extract(context.Background(), propagation.MapCarrier{
		"traceparent": traceparent,
	})

	_, span := tp.GetTracer().Start(ctxCall, "recvCalculateScore")
	defer span.End()
	if err != nil && len(req.Data) > 0 {
		// Record error in a new span
		IsHealthy = false
		log.Printf(fmt.Sprintf("Couldn't recieve message : %v", err))
		span.RecordError(errors.New(fmt.Sprintf("Couldn't recieve message : %v", err)))
	}
	// Handle process in a goroutine
	IsHealthy = true

	scores := req.Data["scores"].(map[string]interface{})
	var generalScore float64 = 0
	for _, v := range scores {
		monthlyScore := v.(map[string]interface{})
		for _, score := range monthlyScore {
			generalScore += score.(float64)
		}
	}
	return generalScore, nil
}
