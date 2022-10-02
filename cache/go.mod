module github.com/asmyasnikov/webinar-jaeger/server

go 1.18

require (
	github.com/ydb-platform/ydb-go-sdk/v3 v3.38.2
	go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc v0.36.1
	go.opentelemetry.io/contrib/propagators/jaeger v1.10.0
	go.opentelemetry.io/otel v1.10.0
	go.opentelemetry.io/otel/exporters/jaeger v1.10.0
	go.opentelemetry.io/otel/sdk v1.10.0
	go.opentelemetry.io/otel/trace v1.10.0
	google.golang.org/grpc v1.49.0
	google.golang.org/protobuf v1.28.1
)

require (
	github.com/go-logr/logr v1.2.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/golang-jwt/jwt/v4 v4.4.1 // indirect
	github.com/golang/protobuf v1.5.2 // indirect
	github.com/jonboulle/clockwork v0.2.2 // indirect
	github.com/ydb-platform/ydb-go-genproto v0.0.0-20220922065549-66df47a830ba // indirect
	golang.org/x/net v0.0.0-20210405180319-a5a99cb37ef4 // indirect
	golang.org/x/sync v0.0.0-20220601150217-0de741cfad7f // indirect
	golang.org/x/sys v0.0.0-20210510120138-977fb7262007 // indirect
	golang.org/x/text v0.3.3 // indirect
	google.golang.org/genproto v0.0.0-20200526211855-cb27e3aa2013 // indirect
)
