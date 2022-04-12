package client_score_sqs

import (
	"context"
	"credit-score-service/application/tracing"
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
		Name: "total_credit_score_requests_processed",
		Help: "The total number of processed events for credit scores",
	})
	IsInitialized = false
	IsHealthy     = false
)

type CreditScoreSQSClient struct {
	requestsQueueURL  *string
	responsesQueueURL *string
	api               *sqs.Client
}

func New() (credit_score.Client, error) {
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

	requestsQueueName := os.Getenv("CREDIT_SCORE_REQUESTS_QUEUE_NAME")
	reqQueueUrlOutput, err := client.GetQueueUrl(context.Background(), &sqs.GetQueueUrlInput{
		QueueName: &requestsQueueName,
	})

	if err != nil {
		return nil, errors.New(fmt.Sprintf("Couldn't get url for queue %s: %v", requestsQueueName, err))
	}

	responsesQueueName := os.Getenv("CREDIT_SCORE_RESPONSES_QUEUE_NAME")
	respQueueUrlOutput, err := client.GetQueueUrl(context.Background(), &sqs.GetQueueUrlInput{
		QueueName: &responsesQueueName,
	})
	if err != nil {
		return nil, errors.New(fmt.Sprintf("Couldn't get url for queue %s: %v", responsesQueueName, err))
	}
	IsInitialized = true
	IsHealthy = true
	return &CreditScoreSQSClient{
		requestsQueueURL:  reqQueueUrlOutput.QueueUrl,
		responsesQueueURL: respQueueUrlOutput.QueueUrl,
		api:               client,
	}, nil
}

func (c *CreditScoreSQSClient) Send(resp *credit_score.CreditScoreResponse) error {
	ctx, cancel := context.WithTimeout(context.Background(), time.Second*15)
	defer cancel()

	data, err := json.Marshal(resp)
	if err != nil {
		return errors.New(fmt.Sprintf("Cannot marshall message data: %v", err))
	}
	stringData := string(data)
	_, err = c.api.SendMessage(ctx, &sqs.SendMessageInput{
		QueueUrl:    c.responsesQueueURL,
		MessageBody: &stringData,
	})

	if err != nil {
		return errors.New(fmt.Sprintf("Cannot send sqs message: %v", err))
	}
	return nil
}

func (c *CreditScoreSQSClient) processMsg(handlerFunc func(msg *credit_score.CreditScoreRequest) error, req *credit_score.CreditScoreRequest, receiptHandle *string, tp *tracing.TracingProvider) error {
	if req != nil {
		// Instrument SQS recv
		traceparent := req.TracingInformation["traceparent"].(string)
		prop := propagation.TraceContext{}
		ctxCall := prop.Extract(context.Background(), propagation.MapCarrier{
			"traceparent": traceparent,
		})
		_, span := tp.GetTracer().Start(ctxCall, "calulateScore", oteltrace.WithAttributes(
			attribute.String("userId", req.UserId),
			attribute.String("bankingInstitutionId", req.BankingInstitutionId),
		))

		// Apply the function and then continue the process
		if err := handlerFunc(req); err == nil {
			ctxDelete, cancel := context.WithTimeout(ctxCall, time.Second*5)
			defer cancel()
			_, err = c.api.DeleteMessage(ctxDelete, &sqs.DeleteMessageInput{
				QueueUrl:      c.requestsQueueURL,
				ReceiptHandle: receiptHandle,
			})

			// Idempotent operation dead letter queues not needed
			if err != nil {
				span.RecordError(errors.New(fmt.Sprintf("Couldn't delete message : %v", receiptHandle)))
				fmt.Println(fmt.Sprintf("Couldn't delete message : %v", receiptHandle))
			}

			opsProcessed.Inc()
			span.End()
		} else {
			span.RecordError(errors.New(fmt.Sprintf("Couldn't process message : %v", receiptHandle)))
			fmt.Println(fmt.Sprintf("Couldn't process message : %v", receiptHandle))
			span.End()
		}
	}
	return nil
}

func (c *CreditScoreSQSClient) Recv(handlerFunc func(msg *credit_score.CreditScoreRequest) error) error {
	ctx := context.Background()
	log.Println(fmt.Sprintf("Listening queue: %v", *c.requestsQueueURL))
	for {
		select {
		case <-ctx.Done():
			log.Println("Stopping polling because a context kill signal was sent")
			return nil
		default:
			tp := tracing.NewProvider()
			req, receiptHandle, err := c.recv()
			if err != nil {
				// Record error in a new span
				IsHealthy = false
				_, span := tp.GetTracer().Start(context.Background(), "calculateScore")
				log.Printf(fmt.Sprintf("Couldn't recieve message : %v", err))
				span.RecordError(errors.New(fmt.Sprintf("Couldn't recieve message : %v", err)))
			}
			// Handle process in a goroutine
			IsHealthy = true
			go c.processMsg(handlerFunc, req, receiptHandle, tp)
		}
	}
}

func (c *CreditScoreSQSClient) recv() (*credit_score.CreditScoreRequest, *string, error) {
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
		var req credit_score.CreditScoreRequest
		if err := json.Unmarshal([]byte(*msg.Body), &req); err != nil || reflect.DeepEqual(req, credit_score.CreditScoreRequest{}) {
			return nil, nil, errors.New(fmt.Sprintf("Unknown message format, cannot parse json: %v", *msg.Body))
		}
		return &req, msg.ReceiptHandle, nil
	} else {
		return nil, nil, nil
	}
}
