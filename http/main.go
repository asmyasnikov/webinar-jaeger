package main

import (
	"context"
	"log"
	"time"

	jaegerPropogator "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
)

const applicationID = "http"

func tracerProvider(url string) (*tracesdk.TracerProvider, error) {
	exp, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}

	otel.SetTextMapPropagator(jaegerPropogator.Jaeger{})

	tp := tracesdk.NewTracerProvider(
		// Always be sure to batch in production.
		tracesdk.WithBatcher(exp),
		// Record information about this application in a Resource.
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String(applicationID),
		)),
	)

	return tp, nil
}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tp, err := tracerProvider("http://localhost:14268/api/traces")
	if err != nil {
		panic(err)
	}
	defer func(ctx context.Context) {
		// Do not make the application hang when it is shutdown.
		ctx, cancel = context.WithTimeout(ctx, time.Second*5)
		defer cancel()
		if err := tp.Shutdown(ctx); err != nil {
			log.Fatal(err)
		}
	}(ctx)

	tr := tp.Tracer(applicationID)

	ctx, span := tr.Start(ctx, "main")
	defer span.End()

	a, err := newAuth(ctx, tr, "127.0.0.1:50051")
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		panic(err)
	}
	defer a.Close()

	span.AddEvent("auth client initialized")

	s, err := initStorages(ctx, tr)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		panic(err)
	}
	defer s.Close()

	h, err := newHandlers(ctx, tr, a, s)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		panic(err)
	}

	h.run(ctx, 8080)
}
