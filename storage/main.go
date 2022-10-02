package main

import (
	"context"
	"database/sql"
	"fmt"
	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/balancers"
	"github.com/ydb-platform/ydb-go-sdk/v3/trace"
	"google.golang.org/grpc"
	"log"
	"net"
	"os"
	"os/signal"
	"time"

	ydbTracing "github.com/ydb-platform/ydb-go-sdk-opentracing"
	_ "github.com/ydb-platform/ydb-go-sdk/v3"
	jaegerPropogator "go.opentelemetry.io/contrib/propagators/jaeger"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.12.0"

	pb "github.com/asmyasnikov/webinar-jaeger/server/pb"
)

const (
	applicationID = "storage"
	port          = 5300
)

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

	db, err := ydb.Open(ctx, "grpc://localhost:2136/local",
		ydb.WithBalancer(balancers.SingleConn()),
		ydbTracing.WithTraces(trace.DetailsAll),
	)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}
	defer db.Close(ctx)

	connector, err := ydb.Connector(db)
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}
	defer connector.Close()

	s, err := newStorage(ctx, tr, sql.OpenDB(connector), db.Name())
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		span.SetAttributes(attribute.Bool("error", true))
		span.RecordError(err)
		return
	}

	grpcServer := grpc.NewServer()

	pb.RegisterStorageServer(grpcServer, s)
	span.AddEvent("storage server registered")

	ch := make(chan os.Signal, 1)
	signal.Notify(ch, os.Interrupt)

	go func() {
		if err := grpcServer.Serve(listener); err != nil {
			close(ch)
		}
	}()

	fmt.Printf("Start starege service on port %d...\n", port)

	for range ch {
		fmt.Println("shutdown...")
		span.AddEvent("received interrupt signal")
		grpcServer.GracefulStop()
	}
}
