package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path"

	"github.com/ydb-platform/ydb-go-sdk/v3"
	"github.com/ydb-platform/ydb-go-sdk/v3/retry"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"

	pb "github.com/asmyasnikov/webinar-jaeger/server/pb"
)

type storage struct {
	pb.UnimplementedStorageServer

	tr     trace.Tracer
	db     *sql.DB
	prefix string
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
	err = retry.DoTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) (err error) {
		_, err = tx.ExecContext(ctx, fmt.Sprintf(`
			PRAGMA TablePathPrefix("%s");

			DECLARE $hash AS Text;
			DECLARE $url AS Text;

			UPSERT INTO urls (hash, url) VALUES ($hash, $url); 
		`, s.prefix), sql.Named("hash", request.GetHash()), sql.Named("url", request.GetUrl()))
		return err
	}, retry.WithDoTxRetryOptions(retry.WithIdempotent(true)))
	if err != nil {
		return nil, err
	}
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
	err = retry.DoTx(ctx, s.db, func(ctx context.Context, tx *sql.Tx) error {
		row := tx.QueryRowContext(ctx, fmt.Sprintf(`
			PRAGMA TablePathPrefix("%s");

			DECLARE $hash AS Text;

			SELECT url FROM urls WHERE hash = $hash; 
		`, s.prefix), sql.Named("hash", request.GetHash()))
		var url sql.NullString
		if err := row.Scan(&url); err != nil {
			return err
		}
		if !url.Valid {
			// non-retryable error
			return fmt.Errorf("url for hash '%s' not found", request.GetHash())
		}
		response = &pb.GetResponse{
			Url: url.String,
		}
		return row.Err()
	}, retry.WithDoTxRetryOptions(retry.WithIdempotent(true)))
	return response, err
}

func initSchema(ctx context.Context, tr trace.Tracer, db *sql.DB, prefix string) (err error) {
	ctx, span := tr.Start(ctx, "initSchema")
	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		} else {
			span.AddEvent("schema prepared")
		}
		span.End()
	}()
	return retry.Do(ctx, db, func(ctx context.Context, cc *sql.Conn) error {
		_, err = cc.ExecContext(
			ydb.WithQueryMode(ctx, ydb.SchemeQueryMode),
			fmt.Sprintf("DROP TABLE `%s`", path.Join(prefix, "urls")),
		)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stdout, "warn: drop series table failed: %v", err)
		}
		_, err = cc.ExecContext(
			ydb.WithQueryMode(ctx, ydb.SchemeQueryMode),
			fmt.Sprintf(`
				PRAGMA TablePathPrefix("%s");

				CREATE TABLE urls (
					hash Text,
					url Text,
					PRIMARY KEY (
						hash
					)
				) WITH (
					AUTO_PARTITIONING_BY_LOAD = ENABLED
				);
			`, prefix),
		)
		if err != nil {
			_, _ = fmt.Fprintf(os.Stderr, "create urls table failed: %v", err)
			return err
		}
		return nil
	}, retry.WithDoRetryOptions(retry.WithIdempotent(true)))
}

func newStorage(ctx context.Context, tr trace.Tracer, db *sql.DB, prefix string) (_ *storage, err error) {
	ctx, span := tr.Start(ctx, "newStorage")
	defer func() {
		if err != nil {
			span.SetAttributes(attribute.Bool("error", true))
			span.RecordError(err)
		}
		span.End()
	}()

	if err = initSchema(ctx, tr, db, prefix); err != nil {
		return nil, err
	}

	return &storage{
		tr:     tr,
		db:     db,
		prefix: prefix,
	}, nil
}
