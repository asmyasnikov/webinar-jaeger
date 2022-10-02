package main

import (
	"context"
	"fmt"
	"sync"

	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/asmyasnikov/webinar-jaeger/server/pb"
)

type storage struct {
	pb.UnimplementedStorageServer

	tr   trace.Tracer
	urls sync.Map
}

func (s *storage) Put(ctx context.Context, request *pb.PutRequest) (response *pb.PutResponse, err error) {
	ctx, span := s.tr.Start(ctx, "get", trace.WithAttributes(
		attribute.String("url", request.GetUrl()),
		attribute.String("hash", request.GetHash()),
	))
	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		} else {
			span.AddEvent("put done")
		}
		span.End()
	}()
	s.urls.Store(request.GetHash(), request.GetUrl())
	return &pb.PutResponse{}, nil
}

func (s *storage) Get(ctx context.Context, request *pb.GetRequest) (response *pb.GetResponse, err error) {
	ctx, span := s.tr.Start(ctx, "Get", trace.WithAttributes(
		attribute.String("hash", request.GetHash()),
	))
	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		} else {
			span.AddEvent("get done", trace.WithAttributes(
				attribute.String("url", response.GetUrl()),
			))
		}
		span.End()
	}()
	if v, ok := s.urls.Load(request.GetHash()); ok {
		return &pb.GetResponse{
			Url: v.(string),
		}, nil
	}
	return nil, fmt.Errorf("url for hash '%s' not found", request.GetHash())
}

func newStorage(ctx context.Context, tr trace.Tracer) (_ *storage, err error) {
	ctx, span := tr.Start(ctx, "newStorage")
	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		}
		span.End()
	}()

	return &storage{
		tr: tr,
	}, nil
}
