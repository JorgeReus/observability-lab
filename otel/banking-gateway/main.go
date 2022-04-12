package main

import (
	"log"

	"banking-gateway/application/controllers"
	msgbroker "banking-gateway/application/msg-broker"
	"banking-gateway/application/tracing"

	_ "banking-gateway/application/docs"

	"banking-gateway/core/usecases"

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

	// Init the consumer
	go usecases.BankingInstitutionReqConsumer(msgbroker.New())
	log.Fatal(app.Listen(":8080"))
}
