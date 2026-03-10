package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"os"
	"time"

	"github.com/go-kratos/kratos/v2"
	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/ratelimit"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/middleware/validate"
	"github.com/go-kratos/kratos/v2/transport/grpc"
	"github.com/go-kratos/kratos/v2/transport/http"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/jaeger"
	"go.opentelemetry.io/otel/sdk/resource"
	tracesdk "go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.4.0"
)

func initTracer(url string) (*tracesdk.TracerProvider, error) {
	exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
	if err != nil {
		return nil, err
	}

	tp := tracesdk.NewTracerProvider(
		tracesdk.WithBatcher(exporter),
		tracesdk.WithResource(resource.NewWithAttributes(
			semconv.SchemaURL,
			semconv.ServiceNameKey.String("service-name"),
		)),
	)

	otel.SetTracerProvider(tp)
	return tp, nil
}

func RequestIDMiddleware() middleware.Middleware {
	return func(next middleware.Handler) middleware.Handler {
		return func(ctx context.Context, req any) (any, error) {
			requestID := generateRequestID()
			ctx = context.WithValue(ctx, "request_id", requestID)

			reply, err := next(ctx, req)
			log.Context(ctx).Infow("request_id", requestID, "reply", reply)
			return reply, err
		}
	}
}

func wrapLogger(logger log.Logger) log.Logger {
	return log.With(
		logger,
		"ts", log.DefaultTimestamp,
		"caller", log.DefaultCaller,
		"service.id", "service-id",
		"service.name", "service-name",
		"service.version", "v1.0.0",
	)
}

func createHTTPServer(logger log.Logger) *http.Server {
	return http.NewServer(
		http.Address(":8000"),
		http.Timeout(30*time.Second),
		http.Middleware(
			recovery.Server(),      // Wrap later middleware and the handler.
			ratelimit.Server(),     // Reject early.
			tracing.Server(),       // Capture end-to-end latency.
			logging.Server(logger), // Log the downstream chain.
			metadata.Server(),
			RequestIDMiddleware(),
			validate.Validator(),
		),
	)
}

func createGRPCServer(logger log.Logger) *grpc.Server {
	return grpc.NewServer(
		grpc.Address(":9000"),
		grpc.Timeout(30*time.Second),
		grpc.Middleware(
			recovery.Server(),
			tracing.Server(),
			logging.Server(logger),
			metadata.Server(),
			validate.Validator(),
		),
	)
}

func newApp(logger log.Logger, hs *http.Server, gs *grpc.Server) *kratos.App {
	return kratos.New(
		kratos.Name("service-name"),
		kratos.Version("v1.0.0"),
		kratos.Metadata(map[string]string{}),
		kratos.Logger(logger),
		kratos.Server(hs, gs),
	)
}

func main() {
	logger := wrapLogger(log.NewStdLogger(os.Stdout))

	tp, err := initTracer("http://jaeger:14268/api/traces")
	if err != nil {
		log.Fatalf("failed to initialize tracer: %v", err)
	}
	defer tp.Shutdown(context.Background())

	hs := createHTTPServer(logger)
	gs := createGRPCServer(logger)

	app := newApp(logger, hs, gs)
	if err := app.Run(); err != nil {
		log.Fatalf("failed to run app: %v", err)
	}
}

func generateRequestID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return fmt.Sprintf("%x", b)
}
