module github.com/asmyasnikov/webinar-jaeger/server

go 1.18

require (
	github.com/ydb-platform/ydb-go-sdk-otel v0.1.1
	github.com/ydb-platform/ydb-go-sdk/v3 v3.40.1
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.37.0
	go.opentelemetry.io/contrib/propagators/jaeger v1.12.0
	go.opentelemetry.io/otel v1.11.2
	go.opentelemetry.io/otel/exporters/jaeger v1.11.2
	go.opentelemetry.io/otel/sdk v1.11.2
	go.opentelemetry.io/otel/trace v1.11.2
	google.golang.org/grpc v1.51.0
	google.golang.org/protobuf v1.28.1
)

require (
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.4.3 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/google/uuid v1.3.0 // indirect
	github.com/jonboulle/clockwork v0.3.0 // indirect
	github.com/ydb-platform/ydb-go-genproto v0.0.0-20220922065549-66df47a830ba // indirect
	go.opentelemetry.io/otel/metric v0.34.0 // indirect
	golang.org/x/net v0.3.0 // indirect
	golang.org/x/sync v0.1.0 // indirect
	golang.org/x/sys v0.3.0 // indirect
	golang.org/x/text v0.5.0 // indirect
	google.golang.org/genproto v0.0.0-20221205194025-8222ab48f5fc // indirect
)
