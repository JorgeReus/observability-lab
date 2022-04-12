package msgbroker

import (
	"banking-gateway/application/tracing"

	msg_broker_iface "banking-gateway/core/msg_broker"
	"context"
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
		Name: "total_requests_processed",
		Help: "The total number of processed events for banking calls",
	})
	IsInitialized = false
	IsHealthy     = false
)

type SQSClient struct {
	requestsQueueURL  *string
	responsesQueueURL *string
	api               *sqs.Client
}

func New() msg_broker_iface.Client {
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
		cfg, err = config.LoadDefaultConfig(context.TODO(), config.WithEndpointResolverWithOptions(customResolver))
	} else {
		cfg, err = config.LoadDefaultConfig(context.TODO())
	}

	if err != nil {
		panic("configuration error, " + err.Error())
	}

	client := sqs.NewFromConfig(cfg)

	requestsQueueName := os.Getenv("BANKING_REQUESTS_QUEUE_NAME")
	reqQueueUrlOutput, err := client.GetQueueUrl(context.TODO(), &sqs.GetQueueUrlInput{
		QueueName: &requestsQueueName,
	})

	responsesQueueName := os.Getenv("BANKING_RESPONSES_QUEUE_NAME")
	respQueueUrlOutput, err := client.GetQueueUrl(context.TODO(), &sqs.GetQueueUrlInput{
		QueueName: &responsesQueueName,
	})
	if err != nil {
		panic(err)
	}
	IsInitialized = true
	IsHealthy = true
	return &SQSClient{
		requestsQueueURL:  reqQueueUrlOutput.QueueUrl,
		responsesQueueURL: respQueueUrlOutput.QueueUrl,
		api:               client,
	}
}

func (c *SQSClient) Send(resp *msg_broker_iface.BankingDataResponse) error {
	tp := tracing.NewProvider()
	tracingInfo := resp.Data["tracingInformation"].(map[string]interface{})
	traceparent := tracingInfo["traceparent"].(string)
	prop := propagation.TraceContext{}
	ctxCall := prop.Extract(context.Background(), propagation.MapCarrier{
		"traceparent": traceparent,
	})

	_, span := tp.GetTracer().Start(ctxCall, "sentResponse", oteltrace.WithAttributes(
		attribute.String("req.userId", resp.Data["userId"].(string)),
		attribute.String("req.bankingInstitutionId", resp.Data["bankingInstitutionId"].(string)),
	))
	ctx, cancel := context.WithTimeout(ctxCall, time.Second*15)
	defer cancel()

	data, err := json.Marshal(resp)
	if err != nil {
		return errors.New(fmt.Sprintf("Cannot marshall message data: %v", err))
	}

	log.Println(fmt.Printf("Sent banking data response from banking %s and user %s", resp.Data["bankingInstitutionId"], resp.Data["userId"]))
	stringData := string(data)
	_, err = c.api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    c.responsesQueueURL,
		MessageBody: &stringData,
	})

	if err != nil {
		return errors.New(fmt.Sprintf("Cannot send sqs message: %v", err))
	}

	span.End()
	return nil
}

func (c *SQSClient) processMsg(handlerFunc func(msg *msg_broker_iface.BankingDataRequest) error, req *msg_broker_iface.BankingDataRequest, receiptHandle *string, tp *tracing.TracingProvider) error {
	if req != nil {
		// Instrument SQS recv
		log.Println(fmt.Printf("Recieved banking data request %s, from bank %s and user %s", *receiptHandle, req.BankingInstitutionId, req.UserId))

		// Apply the function and then continue the process
		if err := handlerFunc(req); err == nil {
			ctxDelete, cancel := context.WithTimeout(context.Background(), time.Second*5)
			defer cancel()
			_, err = c.api.DeleteMessage(ctxDelete, &sqs.DeleteMessageInput{
				QueueUrl:      c.requestsQueueURL,
				ReceiptHandle: receiptHandle,
			})

			// Idempotent operation dead letter queues not needed
			if err != nil {
				// span.RecordError(errors.New(fmt.Sprintf("Couldn't delete message : %v", receiptHandle)))
				fmt.Println(fmt.Sprintf("Couldn't delete message : %v", receiptHandle))
			}

			opsProcessed.Inc()
		} else {
			newErr := errors.New(fmt.Sprintf("Couldn't process message %s :%v", *receiptHandle, err))
			fmt.Println(newErr)
			// span.RecordError(newErr)
		}
	}
	return nil
}

func (c *SQSClient) Recv(handlerFunc func(msg *msg_broker_iface.BankingDataRequest) error) error {
	ctx := context.Background()
	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping polling because a context kill signal was sent")
			return nil
		default:
			tp := tracing.NewProvider()
			req, receiptHandle, err := c.recv()
			if req != nil {
				traceparent := req.TracingInformation["traceparent"].(string)
				prop := propagation.TraceContext{}
				ctxCall := prop.Extract(context.Background(), propagation.MapCarrier{
					"traceparent": traceparent,
				})

				fmt.Println(traceparent)

				_, span := tp.GetTracer().Start(ctxCall, "processBankingMsg")
				if err != nil {
					// Record error in a new span
					IsHealthy = false
					log.Printf(fmt.Sprintf("Couldn't recieve message : %v", err))
					span.RecordError(errors.New(fmt.Sprintf("Couldn't recieve message : %v", err)))
					span.End()
					return err
				}
				// Handle process in a goroutine

				IsHealthy = true
				go c.processMsg(handlerFunc, req, receiptHandle, tp)
				span.End()
			}
		}
	}
}

func (c *SQSClient) recv() (*msg_broker_iface.BankingDataRequest, *string, error) {
	recvMsgInput := &sqs.ReceiveMessageInput{
		MessageAttributeNames: []string{
			string(types.QueueAttributeNameAll),
		},
		WaitTimeSeconds:   1,
		QueueUrl:          c.requestsQueueURL,
		VisibilityTimeout: VISIBILITY_TIMEOUT,
	}

	msgOutput, err := c.api.ReceiveMessage(context.TODO(), recvMsgInput)
	if err != nil {
		return nil, nil, err
	}

	if len(msgOutput.Messages) > 0 {
		msg := msgOutput.Messages[0]
		var req msg_broker_iface.BankingDataRequest
		if err := json.Unmarshal([]byte(*msg.Body), &req); err != nil || reflect.DeepEqual(req, msg_broker_iface.BankingDataRequest{}) {
			return nil, nil, errors.New(fmt.Sprintf("Unknown message format, cannot parse json: %v", *msg.Body))
		}
		return &req, msg.ReceiptHandle, nil
	} else {
		return nil, nil, nil
	}
}
