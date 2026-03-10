# Middleware Usage Patterns

## Core Rule

Kratos chains middleware in registration order:

```go
func Chain(m ...Middleware) Middleware {
    return func(next Handler) Handler {
        for i := len(m) - 1; i >= 0; i-- {
            next = m[i](next)
        }
        return next
    }
}
```

That means:

- Requests enter middleware in the order you register them.
- Responses and errors unwind in reverse order.
- `recovery.Server()` should be registered early if it must wrap later middleware and the handler.

## Ordering Heuristics

Use intent, not superstition:

- `recovery`: early or outer when it must catch downstream panics
- `ratelimit` or `circuitbreaker`: early to reject cheap
- `tracing`: outside the work you want measured
- `logging`: outside the work you want logged
- `metadata`: before code that reads or writes transport metadata
- `auth`: wherever authentication should happen, usually before business code
- `validate`: close to the handler, after transport metadata is available

Example global chain:

```go
http.Middleware(
    recovery.Server(),
    ratelimit.Server(),
    tracing.Server(),
    logging.Server(logger),
    metadata.Server(),
    validate.Validator(),
)
```

### Complete Middleware Setup (compilable)

When generating middleware code, always include complete import blocks. Missing imports are compilation errors. Here is a full, self-contained example:

```go
package server

import (
	"context"

	"github.com/example/myproject/internal/conf" // project-specific config

	"github.com/go-kratos/kratos/v2/log"
	"github.com/go-kratos/kratos/v2/middleware"
	"github.com/go-kratos/kratos/v2/middleware/logging"
	"github.com/go-kratos/kratos/v2/middleware/metadata"
	"github.com/go-kratos/kratos/v2/middleware/ratelimit"
	"github.com/go-kratos/kratos/v2/middleware/recovery"
	"github.com/go-kratos/kratos/v2/middleware/selector"
	"github.com/go-kratos/kratos/v2/middleware/tracing"
	"github.com/go-kratos/kratos/v2/middleware/validate"
	"github.com/go-kratos/kratos/v2/transport"
	kratosHttp "github.com/go-kratos/kratos/v2/transport/http"

	jwtv5 "github.com/golang-jwt/jwt/v5"
	kratosJwt "github.com/go-kratos/kratos/v2/middleware/auth/jwt"
)

func NewHTTPServer(c *conf.Server, logger log.Logger) *kratosHttp.Server {
	middlewares := []middleware.Middleware{
		recovery.Server(),
		ratelimit.Server(),
		tracing.Server(),
		logging.Server(logger),
		metadata.Server(),
		selector.Server(
			kratosJwt.Server(func(token *jwtv5.Token) (interface{}, error) {
				return []byte("secret"), nil
			}),
		).Match(func(ctx context.Context, operation string) bool {
			// Skip auth for health and metrics endpoints
			if tr, ok := transport.FromServerContext(ctx); ok {
				if ht, ok := tr.(*kratosHttp.Transport); ok {
					path := ht.Request().URL.Path
					if path == "/health" || path == "/metrics" {
						return false
					}
				}
			}
			return true
		}).Build(),
		validate.Validator(),
	}

	opts := []kratosHttp.ServerOption{
		kratosHttp.Middleware(middlewares...),
	}
	if c.Http.Addr != "" {
		opts = append(opts, kratosHttp.Address(c.Http.Addr))
	}
	return kratosHttp.NewServer(opts...)
}
```

## Auth and Selector

For Kratos operations, scope auth by operation instead of raw HTTP path matching:

```go
grpc.Middleware(
    selector.Server(
        jwt.Server(keyFunc),
    ).Match(func(ctx context.Context, operation string) bool {
        switch operation {
        case "/api.user.v1.User/Login", "/api.user.v1.User/Register":
            return false
        default:
            return true
        }
    }).Build(),
)
```

Use the same pattern for HTTP routes generated from proto.

> **Important: non-proto endpoints like `/metrics` and `/health`.** These are typically served by handlers registered directly on the HTTP mux (e.g., `promhttp.Handler()` for `/metrics`), not through proto-generated routing. They have no proto operation name, so an operation-based selector won't match them. You have two options:
> 1. **Separate mux registration**: register them on the underlying HTTP mux before Kratos middleware applies — they bypass the chain entirely and don't need a selector skip.
> 2. **Path-based selector skip**: if they go through Kratos middleware (e.g., you still want logging/tracing), use `transport.FromServerContext(ctx)` to extract the HTTP path and skip auth for those paths.
>
> Always mention this distinction when the user's requirements include non-proto endpoints like `/metrics`.

## Transport Context

Use the right context helper:

| Helper | Where | Purpose |
|--------|-------|---------|
| `transport.FromServerContext(ctx)` | server middleware or handler | read inbound operation and headers |
| `transport.FromClientContext(ctx)` | client middleware | set outbound headers or metadata |

```go
func ServerMiddleware() middleware.Middleware {
    return func(next middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req any) (any, error) {
            if tr, ok := transport.FromServerContext(ctx); ok {
                _ = tr.Operation()
            }
            return next(ctx, req)
        }
    }
}

func ClientMiddleware() middleware.Middleware {
    return func(next middleware.Handler) middleware.Handler {
        return func(ctx context.Context, req any) (any, error) {
            if tr, ok := transport.FromClientContext(ctx); ok {
                tr.RequestHeader().Set("x-request-id", "...")
            }
            return next(ctx, req)
        }
    }
}
```

## Cross-Service Client Middleware

When one Kratos service calls another, add client middleware explicitly:

```go
conn, err := grpc.DialInsecure(
    context.Background(),
    grpc.WithEndpoint("discovery:///user-service"),
    grpc.WithDiscovery(r),
    grpc.WithMiddleware(
        tracing.Client(),
        metadata.Client(),
    ),
)
```

This keeps trace propagation and request metadata consistent across services.

## Recovery

Default recovery is fine for many services:

```go
recovery.Server()
```

Customize it when you need panic logging or custom error mapping:

```go
recovery.Server(
    recovery.WithHandler(func(ctx context.Context, req, err any) error {
        log.Errorf("panic recovered: %v", err)
        return errors.InternalServer("INTERNAL_PANIC", "internal server error")
    }),
)
```

## Common Mistakes

- Placing `recovery` last, then expecting it to catch panics from earlier middleware
- Using path matching for JWT policy when the request is a normal Kratos operation
- Reading server transport context from client middleware
- Forgetting client middleware on outbound gRPC calls
