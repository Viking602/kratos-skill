---
name: kratos-skill
description: Practical guidance for designing, implementing, and troubleshooting Go-Kratos services. Use when working on Kratos-based Go services or Kratos-style layouts, especially `api/**/*.proto`, `errors.proto`, validate rules, `make api` or `make errors` or `make validate`, Wire setup, `internal/{biz,data,service,server}`, middleware or auth or selector configuration, service discovery, or cross-service gRPC calls.
---

# Kratos Skill

## Core Objective

Implement and review Kratos code in a way that matches current Kratos docs, official templates, and widely used repository patterns.

## Layer Boundaries (Critical)

Kratos follows a strict three-layer architecture. Violating these boundaries creates tight coupling and makes testing difficult.

```
┌─────────────────────────────────────────────────────────────┐
│  Service Layer (internal/service/)                         │
│  - Imports: api/v1, biz, context                           │
│  - Biz returns Kratos errors; service can pass through     │
│    or remap via v1.ErrorXxx() for richer client messages   │
│  - MUST NOT: contain domain logic                           │
└─────────────────────────────────────────────────────────────┘
                              ↓ calls
┌─────────────────────────────────────────────────────────────┐
│  Business Layer (internal/biz/)                            │
│  - Imports: context, v1 (ErrorReason only),                │
│             kratos/v2/errors, log                          │
│  - Defines: ErrUserNotFound = errors.NotFound(             │
│      v1.ErrorReason_USER_NOT_FOUND.String(), "...")        │
│  - Translates data errors → domain Kratos errors           │
│  - MUST NOT: import gorm, ent, or v1 request/response types│
└─────────────────────────────────────────────────────────────┘
                              ↓ calls
┌─────────────────────────────────────────────────────────────┐
│  Data Layer (internal/data/)                               │
│  - Imports: biz, ent/gorm, remote API clients              │
│  - Translates: ent.IsNotFound / gorm.ErrRecordNotFound → biz.ErrNotFound │
│  - MUST NOT: import local api/v1 or call v1.ErrorXxx()     │
└─────────────────────────────────────────────────────────────┘
```

**The most common mistakes:** (1) Data layer importing `api/v1` and calling `v1.ErrorUserNotFound()` — this couples storage logic to transport concerns. Data should return `biz.ErrUserNotFound`. (2) Biz layer blindly passing through data errors via `return uc.repo.Method(ctx, ...)` — this leaks raw ORM/driver errors to callers. Biz must translate unknown data errors into domain-level Kratos errors.

## Error Handling Strategy

### kratos-layout Style

Define errors in proto, use Kratos errors in biz with proto ErrorReason, translate at each layer boundary.

**Step 1: Define in errors.proto**
```protobuf
enum ErrorReason {
  USER_NOT_FOUND = 0 [(errors.code) = 404];
  USER_ALREADY_EXISTS = 1 [(errors.code) = 409];
}
```

**Step 2: Biz layer defines Kratos errors with proto ErrorReason**
```go
// internal/biz/user.go
package biz

import (
    "context"

    v1 "github.com/example/app/api/user/v1"

    "github.com/go-kratos/kratos/v2/errors"
    "github.com/go-kratos/kratos/v2/log"
)

var (
    ErrUserNotFound = errors.NotFound(v1.ErrorReason_USER_NOT_FOUND.String(), "user not found")
    ErrUserExists   = errors.Conflict(v1.ErrorReason_USER_ALREADY_EXISTS.String(), "user already exists")
)

func (uc *UserUsecase) GetUser(ctx context.Context, id int64) (*User, error) {
    user, err := uc.repo.GetByID(ctx, id)
    if err != nil {
        // Data layer already translated known ORM errors to biz Kratos errors.
        // Check for them and pass through.
        if ErrUserNotFound.Is(err) {
            return nil, ErrUserNotFound
        }
        // Unknown error: don't leak raw data layer details (GORM, Ent, SQL driver).
        // Log the real error for debugging, return a clean domain error to the caller.
        uc.log.WithContext(ctx).Errorf("get user %d: %v", id, err)
        return nil, errors.InternalServer("INTERNAL_ERROR", "internal server error")
    }
    return user, nil
}
```

**Step 3: Data layer translates ORM errors to biz Kratos errors**
```go
// internal/data/user.go (Ent example — recommended for new projects)
func (r *userRepo) GetByID(ctx context.Context, id int64) (*biz.User, error) {
    row, err := r.data.db.User.Get(ctx, id)
    if err != nil {
        if ent.IsNotFound(err) {
            return nil, biz.ErrUserNotFound
        }
        return nil, fmt.Errorf("query user %d: %w", id, err)
    }
    return toBizUser(row), nil
}

// internal/data/user.go (GORM example)
func (r *userRepo) GetByID(ctx context.Context, id int64) (*biz.User, error) {
    var u User
    if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, biz.ErrUserNotFound  // NOT v1.ErrorUserNotFound!
        }
        return nil, fmt.Errorf("query user %d: %w", id, err)
    }
    return toBizUser(&u), nil
}
```

> **Anti-pattern: blanket catch.** Do NOT map all errors to the same domain error:
> ```go
> // BAD — connection failures, timeouts, constraint violations all become "create failed"
> if err != nil { return nil, biz.ErrOrderCreateFailed }
> ```
> For query operations, catch specific ORM errors (`ent.IsNotFound`, `gorm.ErrRecordNotFound`). For write operations, catch constraint errors (`ent.IsConstraintError`, `gorm.ErrDuplicatedKey`) when the domain has a meaningful response. Always wrap unknown errors with `fmt.Errorf("...: %w", err)` to preserve the error chain — this is correct, not a blanket catch.

**Step 4: Service passes through or remaps biz errors**
```go
// internal/service/user.go
func (s *UserService) GetUser(ctx context.Context, req *v1.GetUserRequest) (*v1.GetUserReply, error) {
    user, err := s.uc.GetUser(ctx, req.Id)
    if err != nil {
        return nil, err  // biz errors are already Kratos errors
    }
    return &v1.GetUserReply{User: toProtoUser(user)}, nil
}
```

Biz already translates all errors into Kratos errors, so simple pass-through is safe. For defense-in-depth, service can add a guard:

```go
if err != nil {
    if se := new(errors.Error); !stderrors.As(err, &se) {
        // Raw non-Kratos error leaked — shouldn't happen, but guard it.
        return nil, errors.InternalServer("INTERNAL_ERROR", "internal server error")
    }
    return nil, err
}
```

## Workflow

1. Detect project context before proposing code:
   - Existing service or greenfield?
   - ORM in use (`gorm`, `ent`, or something else)?
   - HTTP, gRPC, or both?
   - Existing error style: does biz already use Kratos errors with proto ErrorReason (kratos-layout style)?

2. Work proto first:
   - Confirm `api/**/*.proto`, `errors.proto`, and validation rules before touching implementation.
   - Every RPC must have a leading `//` comment, `google.api.http` routing, and `openapi.v3.operation` (tags + summary + description). See `references/api-patterns.md` for the full pattern.
   - Every message and field must have a leading `//` comment so `protoc-gen-openapiv3` carries it into the generated `openapi.yaml`.
   - Preserve backward compatibility; reserve removed fields instead of reusing tags.

3. Run code generation before hand-written implementation when proto changes:
   - `make api`
   - `make errors`
   - `make validate`

4. Keep layer boundaries explicit:
   - `service`: transport concerns, request/response mapping, API-facing error mapping
   - `biz`: business rules and domain semantics
   - `data`: storage, remote clients, ORM or driver details

5. Finish with executable verification:
   - Prefer `make api`, `make errors`, `wire ./cmd/server`, `go test ./...`, or a focused curl/grpcurl command

## Conventions

### Project Triage

- If the task is data-layer work, inspect `go.mod` and `internal/data/` first to determine the ORM or storage stack.
- If the project is new and no ORM is implied, recommend Ent as the default ORM. Mention GORM as an alternative if the team prefers it.
- Match examples to the detected stack. For new projects, default to Ent examples.

### Cross-Service Calls

- Prefer service discovery over hardcoded addresses: `grpc.WithEndpoint("discovery:///service-name")` plus `grpc.WithDiscovery(...)`.
- Add client middleware such as `tracing.Client()` and `metadata.Client()` when services call other Kratos services.
- Unwrap remote Kratos errors with `errors.FromError(err)` or the remote service's generated `IsXxx(...)`.
- Handle framework-level gRPC errors such as `codes.DeadlineExceeded` and `codes.Unavailable` separately.

### Middleware

- Kratos middleware runs in registration order and unwinds in reverse.
- Put `recovery.Server()` early in the chain if it is expected to catch panics from later middleware and the handler.
- Put cheap rejection middleware such as rate limiting or circuit breaking before expensive instrumentation or business work.
- Put tracing and logging outside the parts of the chain you want measured and logged.
- Use `selector.Server(...).Match(func(ctx, operation string) bool { ... }).Build()` to scope auth middleware by operation.
- Use `transport.FromServerContext(ctx)` in server middleware and handlers; use `transport.FromClientContext(ctx)` in client middleware.
- Always include complete import blocks in middleware code — `selector`, `middleware`, `transport`, `kratosHttp` are commonly forgotten. See the compilable example in `references/middleware.md`.

## Common Commands

```bash
kratos new demo
make api
make errors
make validate
kratos proto server api/<svc>/v1/<name>.proto -t internal/service
wire ./cmd/server
go test ./...
```

## Documentation Navigation

Read only the file that matches the task:

| Task | File |
|------|------|
| Proto and HTTP or gRPC API design | `references/api-patterns.md` |
| Error design and layering | `references/error-handling.md` |
| Middleware ordering, selector, and client middleware | `references/middleware.md` |
| Layout, Wire, and transaction boundaries | `references/project-structure.md` |
| Validation rules | `references/validation.md` |
| Troubleshooting common failures | `troubleshooting.md` |
| Production operations and deployment concerns | `best-practices.md` |

## Pre-Delivery Checklist

- [ ] Proto contract matches implementation
- [ ] Every RPC has leading `//` comment + `google.api.http` + `openapi.v3.operation` (tags, summary, description)
- [ ] Every message and field has a leading `//` comment for openapi.yaml generation
- [ ] `import "openapi/v3/annotations.proto"` present in proto files that define services
- [ ] Codegen rerun after proto changes
- [ ] Data layer does not import `api/v1` (CRITICAL)
- [ ] Data layer does not call `v1.ErrorXxx()` (CRITICAL)
- [ ] Biz layer does not import ORM packages (CRITICAL)
- [ ] Biz layer defines domain errors using `kratos/v2/errors` with proto ErrorReason (per kratos-layout)
- [ ] Biz layer translates data errors — no blind return of repo errors
- [ ] Biz layer may import `v1` for ErrorReason only — not request/response types
- [ ] Service layer handles biz errors correctly (pass-through or remap)
- [ ] Error handling follows one consistent strategy for the service
- [ ] Middleware order matches the intended execution model
- [ ] All generated Go files include complete import blocks (no missing packages)
- [ ] Cross-service calls use discovery and unwrap remote errors explicitly
- [ ] At least one verification command and result is provided