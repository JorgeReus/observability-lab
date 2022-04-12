package main

import (
	"log"

	"credit-score-service/application/controllers"
	"credit-score-service/application/msg-broker/banking_gateway_sqs"
	"credit-score-service/application/msg-broker/client_score_sqs"
	"credit-score-service/application/tracing"

	_ "credit-score-service/application/docs"

	"credit-score-service/core/usecases"

	swagger "github.com/arsmn/fiber-swagger/v2"
	"github.com/gofiber/adaptor/v2"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/recover"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	_ "go.uber.org/automaxprocs"
)

func main() {
	tp := tracing.NewProvider()
	defer tp.ShuwDownTracer()

	app := fiber.New(fiber.Config{})
	app.Use(recover.New())

	app.Get("/swagger/*", swagger.HandlerDefault)
	app.Get("/healthz", controllers.Healthz)
	app.Get("/readiness", controllers.ReadinessProbe)
	app.Get("/metrics", adaptor.HTTPHandler(promhttp.Handler()))
	app.Get("/score", controllers.GetUserBankingScore)

	// Init the consumer
	clientScoreClient, err := client_score_sqs.New()
	if err != nil {
		log.Fatal(err)
	}

	bankingClient, err := banking_gateway_sqs.New()
	if err != nil {
		log.Fatal(err)
	}
	usecases.CalculateScoreHandler(clientScoreClient, bankingClient)
	log.Fatal(app.Listen(":8080"))
}
