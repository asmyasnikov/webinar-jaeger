package main

import (
	"context"
	"fmt"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/asmyasnikov/webinar-jaeger/server/pb"
)

type Storage interface {
	Close() error
	Get(ctx context.Context, hash string) (url string, err error)
	Put(ctx context.Context, url, hash string) (err error)
}

type coalesceStorage []*storage

func initStorages(ctx context.Context, tr trace.Tracer, addrs ...string) (Storage, error) {
	if len(addrs) == 1 {
		return newStorage(ctx, tr, addrs[0])
	}
	ss := make([]*storage, 0, len(addrs))
	for _, addr := range addrs {
		s, err := newStorage(ctx, tr, addr)
		if err != nil {
			return nil, err
		}
		ss = append(ss, s)
	}
	return coalesceStorage(ss), nil
}

func (ss coalesceStorage) Close() error {
	errs := make([]error, 0, len(ss))
	for _, s := range ss {
		err := s.Close()
		if err != nil {
			errs = append(errs, err)
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("get failed: %v", errs)
	}
	return nil
}

func (ss coalesceStorage) Get(ctx context.Context, hash string) (url string, err error) {
	errs := make([]error, 0, len(ss))
	for _, s := range ss {
		url, err = s.Get(ctx, hash)
		if err == nil {
			return url, err
		}
		errs = append(errs, err)
	}
	return "", fmt.Errorf("get failed: %v", errs)
}

func (ss coalesceStorage) Put(ctx context.Context, url, hash string) (err error) {
	errs := make([]error, 0, len(ss))
	for _, s := range ss {
		err = s.Put(ctx, url, hash)
		if err == nil {
			return nil
		}
		errs = append(errs, err)
	}
	return fmt.Errorf("get failed: %v", errs)
}

type storage struct {
	tr     trace.Tracer
	addr   string
	conn   *grpc.ClientConn
	client pb.StorageClient
}

func newStorage(ctx context.Context, tr trace.Tracer, addr string) (*storage, error) {
	_, span := tr.Start(ctx, "newStorage")
	defer span.End()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
	)
	if err != nil {
		return nil, err
	}

	return &storage{
		tr:     tr,
		addr:   addr,
		conn:   conn,
		client: pb.NewStorageClient(conn),
	}, nil
}

func (a *storage) Close() error {
	return a.conn.Close()
}

func (a *storage) Get(ctx context.Context, hash string) (url string, err error) {
	ctx, span := a.tr.Start(ctx, "get", trace.WithAttributes(
		attribute.String("address", a.addr),
	))
	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		} else {
			span.AddEvent("get successful", trace.WithAttributes(
				attribute.String("url", url),
			))
		}
		span.End()
	}()

	response, err := a.client.Get(ctx, &pb.GetRequest{
		Hash: hash,
	})
	if err != nil {
		return url, err
	}

	return response.GetUrl(), nil
}

func (a *storage) Put(ctx context.Context, url, hash string) (err error) {
	ctx, span := a.tr.Start(ctx, "get", trace.WithAttributes(
		attribute.String("address", a.addr),
	))
	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		} else {
			span.AddEvent("get successful", trace.WithAttributes(
				attribute.String("url", url),
			))
		}
		span.End()
	}()

	_, err = a.client.Put(ctx, &pb.PutRequest{
		Url:  url,
		Hash: hash,
	})

	return err
}
