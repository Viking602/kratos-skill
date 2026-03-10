# Production Best Practices

## Configuration Management

### Config Loading

> Configuration structure definitions (`conf.proto`) are in `references/project-structure.md` and are not repeated here.

```go
c := config.New(
    config.WithSource(file.NewSource(flagconf)),
)
defer c.Close()

if err := c.Load(); err != nil {
    panic(err)
}
var bc conf.Bootstrap
if err := c.Scan(&bc); err != nil {
    panic(err)
}
```

### Config Hot Reload (Watch)

```go
c.Watch("data.database.source", func(key string, value config.Value) {
    dsn, _ := value.String()
    log.Infof("config changed: %s = %s", key, dsn)
    // Rebuild database connection or update connection pool
})

c.Watch("data.redis.addr", func(key string, value config.Value) {
    addr, _ := value.String()
    // Rebuild Redis client
})
```

### Sensitive Configuration

```go
dbPassword := os.Getenv("DB_PASSWORD")
jwtSecret := os.Getenv("JWT_SECRET")
```

## Logging

```go
// Initialization
logger := log.With(log.NewStdLogger(os.Stdout),
    "ts", log.DefaultTimestamp,
    "caller", log.DefaultCaller,
    "service.id", id,
    "service.name", Name,
    "service.version", Version,
    "trace.id", tracing.TraceID(),
)
h := log.NewHelper(logger)

// Structured logging
h.Infow("key1", "value1", "key2", "value2")

// With context (automatically carries trace_id)
h.WithContext(ctx).Infof("Processing order %d", orderID)

// Error logs with sufficient context
h.Errorf("failed to create user: name=%s err=%v", name, err)
```

## Distributed Tracing

### Initialize TracerProvider

```go
func initTracer(url string) (*tracesdk.TracerProvider, error) {
    exporter, err := jaeger.New(jaeger.WithCollectorEndpoint(jaeger.WithEndpoint(url)))
    if err != nil {
        return nil, err
    }
    tp := tracesdk.NewTracerProvider(
        tracesdk.WithBatcher(exporter),
        tracesdk.WithResource(resource.NewWithAttributes(
            semconv.SchemaURL,
            semconv.ServiceNameKey.String("serviceName"),
        )),
    )
    otel.SetTracerProvider(tp)
    return tp, nil
}
```

### Register Middleware

```go
// Server side
httpSrv := http.NewServer(http.Middleware(tracing.Server()))

// Client side
conn, err := grpc.DialInsecure(ctx, grpc.WithMiddleware(tracing.Client()))
```

## Service Registration & Discovery

### Etcd

```go
etcdClient, err := clientv3.New(clientv3.Config{
    Endpoints: []string{"127.0.0.1:2379"},
})
if err != nil {
    panic(err)
}
reg := etcd.New(etcdClient)

app := kratos.New(
    kratos.Name("serviceName"),
    kratos.Registry(reg),
    kratos.Server(httpSrv, grpcSrv),
)
```

### Consul

```go
consulClient, err := consulAPI.NewClient(consulAPI.DefaultConfig())
if err != nil {
    panic(err)
}
reg := consul.New(consulClient)

app := kratos.New(
    kratos.Name("serviceName"),
    kratos.Registry(reg),
    kratos.Server(httpSrv, grpcSrv),
)
```

### Service Discovery (Client Side)

```go
discovery := etcd.New(etcdClient)
conn, err := grpc.DialInsecure(ctx,
    grpc.WithEndpoint("discovery:///serviceName"),
    grpc.WithDiscovery(discovery),
)
```

## Health Checks

```go
type healthChecker struct {
    db    *ent.Client
    redis *redis.Client
}

func (h *healthChecker) Check(ctx context.Context) error {
    if err := h.db.QueryRowContext(ctx, "SELECT 1").Scan(new(int)); err != nil {
        return err
    }
    if _, err := h.redis.Ping(ctx).Result(); err != nil {
        return err
    }
    return nil
}

app := kratos.New(
    kratos.Name("serviceName"),
    kratos.HealthCheck(checker),
)
```

## Graceful Shutdown

```go
app := kratos.New(
    kratos.Name("serviceName"),
    kratos.Server(httpSrv, grpcSrv),
    kratos.StopTimeout(30 * time.Second),
)
if err := app.Run(); err != nil {
    panic(err)
}

// Custom shutdown logic
app := kratos.New(
    kratos.BeforeStop(func(ctx context.Context) error {
        return drainConnections(ctx)
    }),
    kratos.AfterStop(func(ctx context.Context) error {
        return nil
    }),
)
```

## Testing Patterns

Kratos' layered architecture is naturally suited for mock testing — the `biz` layer defines `Repo` interfaces, and the `service` layer depends on `Usecase` interfaces.

### Generate Mocks

```bash
go install go.uber.org/mock/mockgen@latest

# Add go:generate directive in the interface file
#go:generate mockgen -source=./internal/biz/user.go -destination=./internal/biz/mock_user.go -package=biz
```

### Test the Biz Layer (Mock Repo)

```go
func TestUserUsecase_GetUser(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockRepo := NewMockUserRepo(ctrl)
    mockRepo.EXPECT().
        GetUser(gomock.Any(), int64(1)).
        Return(&User{ID: 1, Name: "alice"}, nil)

    uc := NewUserUsecase(mockRepo, log.DefaultLogger)
    user, err := uc.GetUser(context.Background(), 1)
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if user.Name != "alice" {
        t.Fatalf("expected alice, got %s", user.Name)
    }
}
```

### Test the Service Layer (Mock Usecase)

```go
func TestUserService_GetUser(t *testing.T) {
    ctrl := gomock.NewController(t)
    defer ctrl.Finish()

    mockUC := biz.NewMockUserUsecase(ctrl)
    mockUC.EXPECT().
        GetUser(gomock.Any(), int64(1)).
        Return(&biz.User{ID: 1, Name: "alice"}, nil)

    svc := NewUserService(mockUC)
    resp, err := svc.GetUser(context.Background(), &v1.GetUserRequest{Id: 1})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if resp.Name != "alice" {
        t.Fatalf("expected alice, got %s", resp.Name)
    }
}
```

## Deployment

### Dockerfile (Multi-Stage Build)

```dockerfile
FROM golang:1.21 AS builder
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /app ./cmd/server

FROM debian:bookworm-slim
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates && rm -rf /var/lib/apt/lists/*
COPY --from=builder /app /app
COPY configs/ /data/conf/
EXPOSE 8000 9000
ENTRYPOINT ["/app"]
CMD ["-conf", "/data/conf"]
```

```bash
docker build -t myservice:latest .
docker run -p 8000:8000 -p 9000:9000 \
    -e DB_PASSWORD=secret -e JWT_SECRET=secret \
    myservice:latest
```

## Monitoring & Alerting

### Prometheus Metrics Collection

```go
var requestDuration = prometheus.NewHistogramVec(
    prometheus.HistogramOpts{
        Name:    "http_request_duration_seconds",
        Help:    "HTTP request duration",
        Buckets: prometheus.DefBuckets,
    },
    []string{"method", "endpoint", "status"},
)

func init() { prometheus.MustRegister(requestDuration) }

httpSrv := http.NewServer(
    http.Middleware(metrics.Server(metrics.WithSeconds(requestDuration))),
)
```

### Key Metrics & Alert Thresholds

| Metric | Alert Threshold | Description |
|------|----------|------|
| Error rate | > 1% | HTTP 5xx error ratio |
| P99 latency | > 1s | 99th percentile request latency |
| CPU usage | > 80% | Sustained for 5 minutes |
| Memory usage | > 85% | Sustained for 5 minutes |
| DB connections | > 90% | Connection pool utilization |
| Redis connections | > 90% | Connection pool utilization |

### Prometheus Alert Rules

```yaml
groups:
  - name: kratos-service
    rules:
      - alert: HighErrorRate
        expr: rate(http_requests_total{status=~"5.."}[5m]) > 0.01
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "{{ $labels.service }} error rate too high"

      - alert: HighP99Latency
        expr: histogram_quantile(0.99, rate(http_request_duration_seconds_bucket[5m])) > 1
        for: 5m
        labels:
          severity: warning
        annotations:
          summary: "{{ $labels.service }} P99 latency too high"

      - alert: HighCPUUsage
        expr: process_cpu_seconds_total > 0.8
        for: 5m
        labels:
          severity: critical
        annotations:
          summary: "{{ $labels.service }} CPU usage too high"
```

## Performance Optimization

### Connection Pool Configuration

```go
// Database
db.SetMaxOpenConns(100)
db.SetMaxIdleConns(10)
db.SetConnMaxLifetime(time.Hour)

// Redis
redisClient := redis.NewClient(&redis.Options{
    PoolSize:     100,
    MinIdleConns: 10,
    MaxConnAge:   time.Hour,
})
```

### Object Pool

```go
var bufferPool = sync.Pool{
    New: func() interface{} { return make([]byte, 4096) },
}

func processData() {
    buf := bufferPool.Get().([]byte)
    defer bufferPool.Put(buf)
    // use buf
}
```
