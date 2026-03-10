# Project Structure Guide

## Standard Project Layout

```
project-root/
├── api/                    # API & Error Proto definitions
│   └── helloworld/
│       └── v1/
│           ├── helloworld.proto
│           ├── helloworld.pb.go
│           ├── helloworld_grpc.pb.go
│           ├── helloworld_http.pb.go
│           ├── helloworld_errors.pb.go
│           └── helloworld_validate.pb.go
│
├── cmd/                    # Application entrypoints
│   └── server/
│       ├── main.go         # Program entrypoint
│       ├── wire.go         # Wire dependency injection
│       └── wire_gen.go     # Wire generated code
│
├── configs/                # Configuration files
│   └── config.yaml
│
├── internal/               # Internal business code
│   ├── conf/               # Configuration struct definitions
│   │   ├── conf.proto
│   │   └── conf.pb.go
│   ├── data/               # Data Access Layer (Repository)
│   │   ├── data.go
│   │   ├── redis.go
│   │   └── user.go
│   ├── biz/                # Business Logic Layer (Domain)
│   │   ├── biz.go
│   │   └── user.go
│   ├── service/            # Service Layer (Application)
│   │   ├── service.go
│   │   └── user.go
│   └── server/             # Server initialization
│       ├── http.go
│       └── grpc.go
│
├── pkg/                    # Public libraries (importable by external projects)
│   ├── middleware/
│   └── utils/
│
├── third_party/            # Third-party proto files
│   ├── google/
│   ├── validate/
│   └── errors/
│
├── go.mod
├── Makefile
└── README.md
```

## Directory Details

### api/ - API Definitions

Stores all API-related protobuf definitions and generated code.

```
api/
└── helloworld/
    └── v1/
        ├── helloworld.proto          # API definitions
        ├── helloworld.pb.go          # protoc-gen-go generated
        ├── helloworld_grpc.pb.go     # protoc-gen-go-grpc generated
        ├── helloworld_http.pb.go     # protoc-gen-go-http generated
        ├── helloworld_errors.pb.go   # protoc-gen-go-errors generated
        └── helloworld_validate.pb.go # protoc-gen-validate generated
```

**Conventions**:
- Organize directories by service
- Use versioned subdirectories (v1, v2)
- Include complete generated code

### cmd/ - Application Entrypoints

Entrypoint for each executable.

```
cmd/
├── server/              # HTTP/gRPC service
│   ├── main.go
│   └── wire.go
├── job/                 # Message queue consumer
│   ├── main.go
│   └── wire.go
└── task/                # Scheduled tasks
    ├── main.go
    └── wire.go
```

**main.go template**:

```go
package main

import (
    "flag"
    "os"
    "github.com/go-kratos/kratos/v2"
    "github.com/go-kratos/kratos/v2/config"
    "github.com/go-kratos/kratos/v2/config/file"
    "github.com/go-kratos/kratos/v2/log"
)

var flagconf string

func init() {
    flag.StringVar(&flagconf, "conf", "../../configs", "config path")
}

func main() {
    flag.Parse()
    logger := log.With(log.NewStdLogger(os.Stdout),
        "ts", log.DefaultTimestamp,
        "caller", log.DefaultCaller,
        "service.id", id,
        "service.name", Name,
        "service.version", Version,
    )

    c := config.New(
        config.WithSource(
            file.NewSource(flagconf),
        ),
    )
    defer c.Close()

    if err := c.Load(); err != nil {
        panic(err)
    }

    var bc conf.Bootstrap
    if err := c.Scan(&bc); err != nil {
        panic(err)
    }

    app, cleanup, err := wireApp(bc.Server, bc.Data, logger)
    if err != nil {
        panic(err)
    }
    defer cleanup()

    if err := app.Run(); err != nil {
        panic(err)
    }
}
```

### internal/ - Internal Code

Project-private code, not importable by external packages.

#### internal/conf/ - Configuration Structs

```go
// conf.proto
syntax = "proto3";
package internal.conf;

option go_package = "github.com/example/app/internal/conf;conf";

message Bootstrap {
    Server server = 1;
    Data data = 2;
}

message Server {
    message HTTP {
        string network = 1;
        string addr = 2;
        string timeout = 3;
    }
    message GRPC {
        string network = 1;
        string addr = 2;
        string timeout = 3;
    }
    HTTP http = 1;
    GRPC grpc = 2;
}

message Data {
    message Database {
        string driver = 1;
        string source = 2;
    }
    message Redis {
        string network = 1;
        string addr = 2;
        string password = 3;
        int32 db = 4;
    }
    Database database = 1;
    Redis redis = 2;
}
```

#### internal/data/ - Data Access Layer

Implements the Repository pattern, responsible for data access.

> **ORM Selection**: Kratos does not bind to a specific ORM. **Ent** (`entgo.io/ent`) is the recommended choice for new projects — it generates type-safe code from schema definitions and integrates cleanly with Kratos' code generation philosophy. **GORM** (`gorm.io/gorm`) is an acceptable alternative, especially for teams already familiar with it. Check `go.mod` first to confirm which ORM the project uses.

```go
// data.go (ent)
package data

import (
    "github.com/go-kratos/kratos/v2/log"
    "github.com/google/wire"
    "github.com/example/app/ent"
)

var ProviderSet = wire.NewSet(NewData, NewUserRepo)

type Data struct {
    DB *ent.Client
    // Redis *redis.Client
}

func NewData(client *ent.Client, logger log.Logger) (*Data, func(), error) {
    cleanup := func() {
        log.NewHelper(logger).Info("closing the data resources")
        client.Close()
    }
    return &Data{DB: client}, cleanup, nil
}
```

```go
// data.go (GORM)
package data

import (
    "github.com/go-kratos/kratos/v2/log"
    "github.com/google/wire"
    "gorm.io/gorm"
)

var ProviderSet = wire.NewSet(NewData, NewUserRepo)

type Data struct {
    DB    *gorm.DB
    // Redis *redis.Client
}

func NewData(db *gorm.DB, logger log.Logger) (*Data, func(), error) {
    cleanup := func() {
        log.NewHelper(logger).Info("closing the data resources")
    }
    return &Data{DB: db}, cleanup, nil
}
```

UserRepo struct and constructor (ORM-agnostic):

```go
// user.go
package data

import (
    "context"
    "github.com/example/app/internal/biz"
    "github.com/go-kratos/kratos/v2/log"
)

type UserRepo struct {
    data *Data
    log  *log.Helper
}

func NewUserRepo(data *Data, logger log.Logger) biz.UserRepo {
    return &UserRepo{
        data: data,
        log:  log.NewHelper(logger),
    }
}
```

Data query methods — note that ORM errors are translated to domain errors in the data layer:

```go
// user.go (ent)
package data

import (
    "context"
    "fmt"
    "github.com/example/app/ent"
    "github.com/example/app/internal/biz"
    "github.com/go-kratos/kratos/v2/log"
)

func (r *UserRepo) GetUser(ctx context.Context, id int64) (*biz.User, error) {
    user, err := r.data.DB.User.Get(ctx, id)
    if err != nil {
        if ent.IsNotFound(err) {
            return nil, biz.ErrUserNotFound  // ORM error → domain error
        }
        return nil, fmt.Errorf("query user: %w", err)
    }
    return toBizUser(user), nil
}
```

```go
// user.go (GORM)
package data

import (
    "context"
    "errors"
    "fmt"
    "github.com/example/app/internal/biz"
    "github.com/go-kratos/kratos/v2/log"
    "gorm.io/gorm"
)

func (r *UserRepo) GetUser(ctx context.Context, id int64) (*biz.User, error) {
    var user User
    if err := r.data.DB.WithContext(ctx).First(&user, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, biz.ErrUserNotFound  // ORM error → domain error
        }
        return nil, fmt.Errorf("query user: %w", err)
    }
    return toBizUser(&user), nil
}
```

#### internal/biz/ - Business Logic Layer

Contains core business rules.

```go
// biz/biz.go
package biz

var ProviderSet = wire.NewSet(NewUserUsecase)
```

```go
// user.go
package biz

import (
    "context"

    v1 "github.com/example/app/api/user/v1"
    "github.com/go-kratos/kratos/v2/errors"
    "github.com/go-kratos/kratos/v2/log"
)

// Domain errors use Kratos errors with proto ErrorReason, following kratos-layout.
// This ensures errors carry correct HTTP/gRPC codes and reason strings.
var (
    ErrUserNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")
    ErrUserExists   = errors.Conflict(v1.ErrorReason_USER_ALREADY_EXISTS.String(), "user already exists")
)

type User struct {
    ID    int64
    Name  string
    Email string
}

type UserRepo interface {
    GetUser(ctx context.Context, id int64) (*User, error)
    CreateUser(ctx context.Context, user *User) (*User, error)
}

type UserUsecase struct {
    repo UserRepo
    log  *log.Helper
}

func NewUserUsecase(repo UserRepo, logger log.Logger) *UserUsecase {
    return &UserUsecase{
        repo: repo,
        log:  log.NewHelper(logger),
    }
}

func (uc *UserUsecase) GetUser(ctx context.Context, id int64) (*User, error) {
    user, err := uc.repo.GetUser(ctx, id)
    if err != nil {
        // Known domain error (translated by data layer) — pass through.
        if ErrUserNotFound.Is(err) {
            return nil, ErrUserNotFound
        }
        // Unknown data error — log and return a clean domain error.
        uc.log.WithContext(ctx).Errorf("get user %d: %v", id, err)
        return nil, errors.InternalServer("INTERNAL_ERROR", "internal server error")
    }
    return user, nil
}

func (uc *UserUsecase) CreateUser(ctx context.Context, user *User) (*User, error) {
    if user.Name == "" {
        return nil, errors.BadRequest(v1.ErrorReason_USER_NAME_EMPTY.String(), "user name is required")
    }
    result, err := uc.repo.CreateUser(ctx, user)
    if err != nil {
        if ErrUserExists.Is(err) {
            return nil, ErrUserExists
        }
        uc.log.WithContext(ctx).Errorf("create user: %v", err)
        return nil, errors.InternalServer("INTERNAL_ERROR", "create user failed")
    }
    return result, nil
}
```

#### internal/service/ - Service Layer

Handles API request/response conversion.

```go
// service/service.go
package service

var ProviderSet = wire.NewSet(NewUserService)
```

```go
// user.go
package service

import (
    "context"

    pb "github.com/example/app/api/helloworld/v1"
    "github.com/example/app/internal/biz"
)

type UserService struct {
    pb.UnimplementedUserServiceServer
    uc *biz.UserUsecase
}

func NewUserService(uc *biz.UserUsecase) *UserService {
    return &UserService{uc: uc}
}

func (s *UserService) GetUser(ctx context.Context, req *pb.GetUserRequest) (*pb.GetUserReply, error) {
    user, err := s.uc.GetUser(ctx, req.Id)
    if err != nil {
        // Biz already returns proper Kratos errors — pass through directly.
        // Optionally remap for richer client messages:
        // if biz.ErrUserNotFound.Is(err) {
        //     return nil, pb.ErrorUserNotFound("user %d not found", req.Id)
        // }
        return nil, err
    }
    return &pb.GetUserReply{
        User: &pb.User{
            Id:    user.ID,
            Name:  user.Name,
            Email: user.Email,
        },
    }, nil
}
```

#### internal/server/ - Server Initialization

```go
// http.go
package server

import (
    "github.com/example/app/internal/conf"
    "github.com/example/app/internal/service"
    "github.com/go-kratos/kratos/v2/log"
    "github.com/go-kratos/kratos/v2/middleware/recovery"
    "github.com/go-kratos/kratos/v2/transport/http"
    pb "github.com/example/app/api/helloworld/v1"
)

func NewHTTPServer(c *conf.Server, user *service.UserService, logger log.Logger) *http.Server {
    srv := http.NewServer(
        http.Address(c.Http.Addr),
        http.Middleware(recovery.Server()),
    )
    pb.RegisterUserServiceHTTPServer(srv, user)
    return srv
}
```

### pkg/ - Public Libraries

Public code importable by external projects.

```
pkg/
├── middleware/          # Custom middleware
│   ├── auth/
│   └── logging/
├── utils/               # Utility functions
│   ├── time/
│   └── crypto/
└── errors/              # Common errors
```

## Dependency Injection (Wire)

### wire.go

```go
//go:build wireinject
// +build wireinject

package main

import (
    "github.com/example/app/internal/biz"
    "github.com/example/app/internal/conf"
    "github.com/example/app/internal/data"
    "github.com/example/app/internal/server"
    "github.com/example/app/internal/service"
    "github.com/go-kratos/kratos/v2"
    "github.com/go-kratos/kratos/v2/log"
    "github.com/google/wire"
)

func wireApp(*conf.Server, *conf.Data, log.Logger) (*kratos.App, func(), error) {
    panic(wire.Build(
        server.ProviderSet,
        data.ProviderSet,
        biz.ProviderSet,
        service.ProviderSet,
        newApp,
    ))
}

func newApp(logger log.Logger, hs *http.Server, gs *grpc.Server) *kratos.App {
    return kratos.New(
        kratos.Name("service"),
        kratos.Version("v1.0.0"),
        kratos.Logger(logger),
        kratos.Server(hs, gs),
    )
}
```

### Codegen

```bash
cd cmd/server
wire
```

## Naming Conventions

### Directory Naming

- ✅ Use singular form: `data/` not `datas/`
- ✅ Use lowercase: `internal/` not `Internal/`
- ❌ Do not use a `src/` directory
- ❌ Do not use a `model/` directory

### File Naming

- ✅ Use snake_case: `user_repo.go`
- ✅ Test files: `xxx_test.go`
- ✅ Wire files: `wire.go`, `wire_gen.go`

### Package Naming

- ✅ Use singular form: `package data` not `package datas`
- ✅ Package name matches directory name

### Interface Naming

- ✅ End with `er`: `Reader`, `Writer`, `UserRepo`

## DDD Layering Principles

```
┌─────────────────────────────────────┐
│         API Layer (pb.go)           │  ← Protocol layer
├─────────────────────────────────────┤
│       Service (Application)         │  ← Application layer: request/response conversion
├─────────────────────────────────────┤
│         Biz (Domain)                │  ← Domain layer: core business logic
├─────────────────────────────────────┤
│         Data (Repository)           │  ← Data access layer
├─────────────────────────────────────┤
│         Infrastructure              │  ← Infrastructure layer
└─────────────────────────────────────┘
```

### Cross-Repository Transactions

When a biz usecase needs to atomically write across multiple repos (e.g., create an order + create order items + deduct stock), Kratos projects use a **context-based transaction pattern** that keeps the biz layer ORM-free.

#### Step 1: Define Transaction interface in biz

```go
// internal/biz/biz.go
package biz

// Transaction defines the interface for executing a function within a database transaction.
// The data layer implements this; biz consumes it without knowing about GORM/ent.
type Transaction interface {
    ExecTx(context.Context, func(ctx context.Context) error) error
}
```

#### Step 2: Data layer implements Transaction with context-based tx propagation

```go
// internal/data/data.go
package data

import (
    "context"
    "github.com/example/app/internal/biz"
    "gorm.io/gorm"
)

type contextTxKey struct{}

// ExecTx executes fn within a GORM transaction.
// The *gorm.DB tx is stored in context so all repos called within fn
// automatically participate in the same transaction.
func (d *Data) ExecTx(ctx context.Context, fn func(ctx context.Context) error) error {
    return d.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
        ctx = context.WithValue(ctx, contextTxKey{}, tx)
        return fn(ctx)
    })
}

// DB returns the active transaction from context, or the base DB if none.
// All repo methods MUST use d.DB(ctx) instead of d.db directly.
func (d *Data) DB(ctx context.Context) *gorm.DB {
    if tx, ok := ctx.Value(contextTxKey{}).(*gorm.DB); ok {
        return tx
    }
    return d.db.WithContext(ctx)
}
```

#### Step 3: Repos use DB(ctx) to automatically join transactions

```go
// internal/data/order.go
func (r *OrderRepo) Create(ctx context.Context, order *biz.Order) (*biz.Order, error) {
    po := toOrderPO(order)
    if err := r.data.DB(ctx).Create(&po).Error; err != nil {
        return nil, fmt.Errorf("create order: %w", err)
    }
    return toBizOrder(&po), nil
}

// internal/data/product.go
func (r *ProductRepo) DeductStock(ctx context.Context, productID int64, qty int32) error {
    result := r.data.DB(ctx).Model(&Product{}).
        Where("id = ? AND stock >= ?", productID, qty).
        Update("stock", gorm.Expr("stock - ?", qty))
    if result.Error != nil {
        return fmt.Errorf("deduct stock: %w", result.Error)
    }
    if result.RowsAffected == 0 {
        return biz.ErrInsufficientStock
    }
    return nil
}
```

#### Step 4: Biz usecase wraps operations in ExecTx

```go
// internal/biz/order.go
package biz

// OrderUsecase receives Transaction via constructor (Wire injection).
type OrderUsecase struct {
    orderRepo   OrderRepo
    itemRepo    OrderItemRepo
    productRepo ProductRepo
    tx          Transaction  // injected, NOT created here
    log         *log.Helper
}

func (uc *OrderUsecase) CreateOrder(ctx context.Context, order *Order, items []*OrderItem) (*Order, error) {
    var created *Order
    err := uc.tx.ExecTx(ctx, func(ctx context.Context) error {
        // All repo calls inside this closure share the same DB transaction
        var err error
        created, err = uc.orderRepo.Create(ctx, order)
        if err != nil {
            return err
        }
        for _, item := range items {
            item.OrderID = created.ID
            if err := uc.itemRepo.Create(ctx, item); err != nil {
                return err
            }
            if err := uc.productRepo.DeductStock(ctx, item.ProductID, item.Quantity); err != nil {
                return err  // triggers rollback
            }
        }
        return nil
    })
    if err != nil {
        return nil, err
    }
    return created, nil
}
```

#### Wire setup

```go
// internal/data/data.go
var ProviderSet = wire.NewSet(
    NewData,
    NewOrderRepo,
    NewOrderItemRepo,
    NewProductRepo,
    // Data implements biz.Transaction, bind the interface
    wire.Bind(new(biz.Transaction), new(*Data)),
)
```

**Key rules**:
- Biz layer defines the `Transaction` interface — it never imports GORM/ent
- Data layer implements it and propagates the tx via `context.WithValue`
- All repos use `d.DB(ctx)` (not `d.db`) so they automatically join the active transaction
- Transaction is injected via Wire's `wire.Bind`, not created in biz

> **Ent transactions**: For Ent, use `client.Tx(ctx)` to start a transaction, which returns an `*ent.Tx`. Pass the `*ent.Tx` client through context in the same pattern shown above — repos extract it via `ctx.Value` and use the transactional client instead of the base `*ent.Client`.

## Test Organization

### Test File Placement

```
project-root/
├── internal/
│   ├── biz/
│   │   ├── user.go
│   │   └── user_test.go        # Unit tests (adjacent to source file)
│   ├── data/
│   │   └── user_test.go        # Data layer unit tests
│   └── service/
│       └── user_test.go        # Service layer unit tests
└── test/                        # Integration tests & E2E tests
    ├── user_test.go
    └── testdata/
```

### Biz Layer Mock Testing

The biz layer depends on Repo interfaces, making it naturally suited for mock testing:

```go
// internal/biz/user_test.go
package biz

type mockUserRepo struct {
    users map[int64]*User
}

func (m *mockUserRepo) GetUser(_ context.Context, id int64) (*User, error) {
    if u, ok := m.users[id]; ok {
        return u, nil
    }
    return nil, ErrUserNotFound
}

func (m *mockUserRepo) CreateUser(_ context.Context, user *User) (*User, error) {
    user.ID = int64(len(m.users) + 1)
    m.users[user.ID] = user
    return user, nil
}

func TestUserUsecase_CreateUser(t *testing.T) {
    repo := &mockUserRepo{users: make(map[int64]*User)}
    uc := NewUserUsecase(repo, log.DefaultLogger)

    // Normal creation
    user, err := uc.CreateUser(context.Background(), &User{Name: "test"})
    if err != nil {
        t.Fatalf("unexpected error: %v", err)
    }
    if user.ID == 0 {
        t.Fatal("expected non-zero ID")
    }

    // Empty name should return an error
    _, err = uc.CreateUser(context.Background(), &User{})
    if err == nil {
        t.Fatal("expected error for empty name")
    }

    // User not found should return domain error
    _, err = uc.GetUser(context.Background(), 999)
    if !ErrUserNotFound.Is(err) {
        t.Fatalf("expected ErrUserNotFound, got %v", err)
    }
}
```

### Running Tests

```bash
go test ./internal/biz/...          # Biz layer unit tests only
go test ./test/... -tags=integration # Integration tests
go test ./...                        # All tests
```

## Multi-Service Project (Monorepo)

Recommended structure when multiple microservices coexist in the same repository:

```
app/
├── api/                           # Proto definitions for all services
│   ├── user/v1/
│   │   ├── user.proto
│   │   └── user_error.proto
│   └── order/v1/
│       ├── order.proto
│       └── order_error.proto
├── app/                           # Independent directory per service
│   ├── user/
│   │   ├── cmd/
│   │   │   └── server/
│   │   │       ├── main.go
│   │   │       └── wire.go
│   │   ├── internal/
│   │   │   ├── biz/
│   │   │   ├── data/
│   │   │   ├── service/
│   │   │   └── server/
│   │   └── configs/
│   └── order/
│       ├── cmd/
│       │   └── server/
│       ├── internal/
│       └── configs/
├── pkg/                           # Shared code across services
│   ├── middleware/
│   └── util/
├── third_party/
├── go.mod                         # Single go.mod (recommended)
└── Makefile
```

**Key principles**:
- Each service's `internal/` remains independent; cross-service imports are prohibited
- Shared code goes in `pkg/`, decoupled via interfaces
- API Proto files are managed centrally for easy cross-service referencing
- Each service is deployed independently with separate `cmd/` entrypoints
