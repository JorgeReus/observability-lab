package tracing

import (
	"context"
	"credit-score-service/core/constants"
	"log"
	"os"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/resource"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
	"go.opentelemetry.io/otel/trace"

	"go.opentelemetry.io/otel/exporters/jaeger"
	stdout "go.opentelemetry.io/otel/exporters/stdout/stdouttrace"
	sdktrace "go.opentelemetry.io/otel/sdk/trace"
)

type TracingProvider struct {
	provider *sdktrace.TracerProvider
	tracer   trace.Tracer
}

var (
	globalProvider *TracingProvider
	jaegerEnpoint  = os.Getenv("JAEGER_ENDPOINT")
)

func NewProvider() *TracingProvider {
	if globalProvider == nil {
		tracer := otel.Tracer(constants.APP_NAME)
		var exporter sdktrace.SpanExporter
		var err error
		if jaegerEnpoint == "" {
			exporter, err = stdout.New(stdout.WithPrettyPrint())
		} else {
			exporter, err = jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(jaegerEnpoint)))
		}
		if err != nil {
			log.Fatal(err)
		}
		tp := sdktrace.NewTracerProvider(
			sdktrace.WithSampler(sdktrace.AlwaysSample()),
			sdktrace.WithBatcher(exporter),
			sdktrace.WithResource(
				resource.NewWithAttributes(
					semconv.SchemaURL,
					semconv.ServiceNameKey.String(constants.APP_NAME),
				)),
		)
		otel.SetTracerProvider(tp)
		otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(propagation.TraceContext{}, propagation.Baggage{}))
		globalProvider = &TracingProvider{
			provider: tp,
			tracer:   tracer,
		}
	}

	return globalProvider
}

func (tp *TracingProvider) GetTracer() trace.Tracer {
	return tp.tracer
}

func (tp *TracingProvider) ShuwDownTracer() {
	if err := tp.provider.Shutdown(context.Background()); err != nil {
		log.Printf("Error shutting down tracer provider: %v", err)
	}
}
