package main

import (
	"context"
	"fmt"
	"log"
	"time"

	jaegerPropogator "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"
	"go.opentelemetry.io/otel/trace"
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

	token, err := a.Login(ctx, "user1", "user1")
	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(fmt.Errorf("unexpected successful result"), trace.WithAttributes(attribute.String("token", token)))
	}

	token, err = a.Login(ctx, "user", "user1")
	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(fmt.Errorf("unexpected successful result"), trace.WithAttributes(attribute.String("token", token)))
	}

	token, err = a.Login(ctx, "user", "user")
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
	}

	err = a.Validate(ctx, "azaza")
	if err != nil {
		span.RecordError(err)
	} else {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(fmt.Errorf("unexpected successful result"))
	}

	err = a.Validate(ctx, token)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
	}
}
