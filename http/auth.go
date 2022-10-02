package main

import (
	"context"
	"time"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	pb "github.com/asmyasnikov/webinar-jaeger/server/pb"
)

type auth struct {
	tr     trace.Tracer
	conn   *grpc.ClientConn
	client pb.AuthClient
}

func newAuth(ctx context.Context, tr trace.Tracer, addr string) (*auth, error) {
	_, span := tr.Start(ctx, "newAuth")
	defer span.End()

	conn, err := grpc.DialContext(ctx, addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(otelgrpc.UnaryClientInterceptor()),
	)
	if err != nil {
		return nil, err
	}

	return &auth{
		tr:     tr,
		conn:   conn,
		client: pb.NewAuthClient(conn),
	}, nil
}

func (a *auth) Close() error {
	return a.conn.Close()
}

func (a *auth) Login(ctx context.Context, user, password string) (token string, expireAt time.Time, err error) {
	ctx, span := a.tr.Start(ctx, "login")
	defer span.End()

	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		} else {
			span.AddEvent("login successful", trace.WithAttributes(
				attribute.String("token", token),
			))
		}
	}()
	response, err := a.client.Login(ctx, &pb.LoginRequest{
		User:     user,
		Password: password,
	})
	if err != nil {
		return token, expireAt, err
	}

	return response.GetToken(), response.GetExpireAt().AsTime(), nil
}

func (a *auth) Validate(ctx context.Context, token string) (err error) {
	ctx, span := a.tr.Start(ctx, "validate")
	defer span.End()

	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		} else {
			span.AddEvent("validate successful")
		}
	}()
	_, err = a.client.Validate(ctx, &pb.ValidateRequest{
		Token: token,
	})
	return err
}
