# Troubleshooting Guide
## Quick Index

| Symptom | Possible Cause | Jump To |
|------|----------|------|
| protoc fails | Missing plugin / incorrect path | [#1](#1-proto-codegen-failure) |
| wire error | Incomplete ProviderSet | [#2](#2-wire-dependency-injection-failure) |
| validate not working | Middleware not registered | [#3](#3-validation-rules-not-applied) |
| Wrong error code | Not using generated functions | [#4](#4-error-code-mismatch) |
| Uncaught panic | Wrong recovery order | [#5](#5-middleware-order-error) |
| Registration failure | Etcd connection / name conflict | [#6](#6-service-registration-failure) |
| Config load failure | Path / format / struct mismatch | [#7](#7-config-load-failure) |
| Trace data missing | TracerProvider not initialized | [#8](#8-distributed-tracing-data-missing) |
| Cross-origin blocked | CORS not configured | [#9](#9-cross-origin-issues-cors) |
| Request timeout | Short timeout / insufficient pool | [#10](#10-request-timeout) |

## Common Issues

### 1. Proto Codegen Failure

**Problem**: `protoc` command execution fails

**Cause**:
- Missing protoc plugins
- Incorrect proto file import paths

**Solution**:
```bash
go install github.com/go-kratos/kratos/cmd/kratos/v2@latest
go install github.com/go-kratos/kratos/cmd/protoc-gen-go-http/v2@latest
go install github.com/go-kratos/kratos/cmd/protoc-gen-go-errors/v2@latest
go install google.golang.org/protobuf/cmd/protoc-gen-go@latest
go install google.golang.org/grpc/cmd/protoc-gen-go-grpc@latest
go install github.com/envoyproxy/protoc-gen-validate@latest

mkdir -p third_party
```

### 2. Wire Dependency Injection Failure

**Problem**: `wire: err: no inject function found`

**Cause**:
- ProviderSet not correctly defined
- Circular dependency
- Missing provider

**Solution**:
```go
// Each layer must have a ProviderSet
var ProviderSet = wire.NewSet(NewData, NewUserRepo)       // data/
var ProviderSet = wire.NewSet(NewUserUsecase)              // biz/
var ProviderSet = wire.NewSet(NewUserService)              // service/
var ProviderSet = wire.NewSet(NewHTTPServer, NewGRPCServer) // server/

// wire.go
func wireApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
    panic(wire.Build(
        server.ProviderSet, data.ProviderSet,
        biz.ProviderSet, service.ProviderSet, newApp,
    ))
}
```

### 3. Validation Rules Not Applied

**Problem**: Request parameter validation not executed

**Cause**:
- Validate middleware not registered
- Validate code not generated from proto file

**Solution**:
```go
httpSrv := http.NewServer(
    http.Middleware(validate.Validator()),
)
```
```protobuf
import "validate/validate.proto";
```
```bash
make validate
```

### 4. Error Code Mismatch

**Problem**: Returned error code does not match the definition

**Cause**:
- Generated helpers not used consistently
- Error definition missing `errors.code` annotation

**Solution**:
```protobuf
enum ErrorReason {
    option (errors.default_code) = 500;
    USER_NOT_FOUND = 0 [(errors.code) = 404];
    USER_NAME_EMPTY = 1 [(errors.code) = 400];
}
```
```go
// Recommended: use the generated helper for public API errors
return nil, v1.ErrorUserNotFound("user %d not found", id)
// Allowed but easy to drift if scattered everywhere
return nil, errors.New(404, "USER_NOT_FOUND", "user not found")
```

### 5. Middleware Order Error

**Problem**: Panic not caught or logs not recorded

**Cause**: Recovery middleware does not wrap the middleware or handler that panic

**Solution**:
```go
http.Middleware(
    recovery.Server(),      // 1. Wrap downstream middleware and the handler
    ratelimit.Server(),     // 2. Rate limiting
    tracing.Server(),       // 3. Distributed tracing
    logging.Server(logger), // 4. Logging
    metadata.Server(),      // 5. Metadata
    validate.Validator(),   // 6. Parameter validation
)
```

### 6. Service Registration Failure

**Problem**: Service cannot register with Etcd

**Cause**:
- Incorrect Etcd connection configuration
- Duplicate service name
- Network unreachable

**Solution**:
```go
etcdClient, err := clientv3.New(clientv3.Config{
    Endpoints:   []string{"127.0.0.1:2379"},
    DialTimeout: 5 * time.Second,
})
if err != nil {
    panic(fmt.Sprintf("etcd connect failed: %v", err))
}

app := kratos.New(
    kratos.Name("unique-service-name"),
    kratos.Registry(reg),
)
```

### 7. Config Load Failure

**Problem**: Configuration file cannot be loaded

**Cause**:
- Incorrect config file path
- Format error (YAML/JSON)
- Struct mismatch

**Solution**:
```go
var flagconf string
flag.StringVar(&flagconf, "conf", "../../configs", "config path")
flag.Parse()

if _, err := os.Stat(flagconf); os.IsNotExist(err) {
    panic(fmt.Sprintf("config path not found: %s", flagconf))
}

var bc conf.Bootstrap
if err := c.Scan(&bc); err != nil {
    panic(fmt.Sprintf("config scan failed: %v", err))
}
```

### 8. Distributed Tracing Data Missing

**Problem**: Trace data not visible in Jaeger/Zipkin

**Cause**:
- TracerProvider not initialized
- Tracing middleware not registered
- Sample rate is 0

**Solution**:
```go
tp, err := initTracer("http://jaeger:14268/api/traces")
if err != nil {
    panic(err)
}
defer tp.Shutdown(context.Background())

otel.SetTracerProvider(tp)

httpSrv := http.NewServer(
    http.Middleware(
        tracing.Server(tracing.WithSampler(tracing.AlwaysSample())),
    ),
)
```

### 9. Cross-Origin Issues (CORS)

**Problem**: Frontend requests blocked by browser

**Solution**:
```go
import "github.com/go-kratos/kratos/v2/middleware/cors"

httpSrv := http.NewServer(
    http.Middleware(
        cors.Server(
            cors.WithAllowedOrigins([]string{"*"}),
            cors.WithAllowedMethods([]string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}),
            cors.WithAllowedHeaders([]string{"Content-Type", "Authorization"}),
        ),
    ),
)
```

### 10. Request Timeout

**Problem**: Requests frequently time out

**Cause**:
- Timeout value set too short
- Insufficient database/Redis connection pool
- Slow queries

**Solution**:
```go
httpSrv := http.NewServer(
    http.Timeout(30 * time.Second),
)

db.SetMaxOpenConns(100)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)

redisClient := redis.NewClient(&redis.Options{
    PoolSize:     100,
    MinIdleConns: 10,
})
```

## Debugging Tips

### pprof Performance Profiling

```go
import _ "net/http/pprof"
httpSrv := http.NewServer(
    http.Middleware(profile.Server()),
)
// Visit http://localhost:8000/debug/pprof
```

### delve Debugging

```bash
go install github.com/go-delve/delve/cmd/dlv@latest
dlv debug ./cmd/server -- --conf ./configs
# break main.main  | continue | next | print <var> | goroutines
```

### Health Check

```bash
curl http://localhost:8000/healthz
curl http://localhost:8000/ready
```

## Performance Profiling

### CPU Profiling

```bash
go tool pprof http://localhost:8000/debug/pprof/profile?seconds=30
# (pprof) top
# (pprof) list FunctionName
```

### Memory Profiling

```bash
go tool pprof http://localhost:8000/debug/pprof/heap
# (pprof) top
# (pprof) alloc_space
```

### Slow Query Detection

```go
db, err := ent.Open("mysql", dsn,
    ent.Log(func(a ...interface{}) {
        log.Infof("SQL: %v", a)
    }),
)
```
