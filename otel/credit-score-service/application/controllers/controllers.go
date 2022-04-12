package controllers

import (
	"credit-score-service/application/msg-broker/banking_gateway_sqs"
	"credit-score-service/application/msg-broker/client_score_sqs"
	"credit-score-service/application/tracing"
	banking_gateway "credit-score-service/core/baking_gateway"
	"fmt"

	"github.com/gofiber/fiber/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/propagation"
	oteltrace "go.opentelemetry.io/otel/trace"
)

var (
	bankingClient banking_gateway.Client
	tp            *tracing.TracingProvider
)

func init() {
	tp = tracing.NewProvider()
	var err error
	bankingClient, err = banking_gateway_sqs.New()
	if err != nil {
		panic(err)
	}
}

// Ready godoc
// @Summary Readiness probe
// @Description Readiness of the jwk service
// @ID readiness
// @Success 200
// @Router /readiness [get]
func ReadinessProbe(c *fiber.Ctx) error {
	if client_score_sqs.IsInitialized && banking_gateway_sqs.IsInitialized {
		return c.SendStatus(200)
	}
	return c.SendStatus(500)
}

// Healthy godoc
// @Summary Healhiness probe
// @Description Healthiness of the jwk service
// @ID healthz
// @Success 200
// @Router /healthz [get]
func Healthz(c *fiber.Ctx) error {
	if client_score_sqs.IsHealthy && banking_gateway_sqs.IsHealthy {
		return c.SendStatus(200)
	}
	return c.SendStatus(500)
}

// GetUserBankingScore godoc
// @Summary User Banking Score
// @Description Returns the score of a user
// @ID GetUserBankingScore
// @Success 200
// @Router /score [get]
func GetUserBankingScore(c *fiber.Ctx) error {
	userId := "reus"
	bankingInstitutionId := "userId"
	ctx, span := tp.GetTracer().Start(c.Context(), "calculateScore", oteltrace.WithAttributes(
		attribute.String("req.userId", userId),
		attribute.String("req.bankingInstitutionId", bankingInstitutionId),
	))

	prop := propagation.TraceContext{}
	carrier := propagation.MapCarrier{}
	prop.Inject(ctx, carrier)
	tracingInformation := map[string]interface{}{
		"traceparent": carrier["traceparent"],
	}

	err := bankingClient.Send(&banking_gateway.BankingGatewayRequest{
		UserId:               userId,
		BankingInstitutionId: bankingInstitutionId,
		BankingCredentials:   make(map[string]string),
		TracingInformation:   tracingInformation,
	})
	if err != nil {
		span.RecordError(err)
	}

	res, _ := bankingClient.Recv()
	fmt.Println(res)
	span.End()
	return c.SendStatus(200)
}
