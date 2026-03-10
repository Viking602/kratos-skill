# Error Handling Patterns

## Core Principles

1. Define public API errors in `errors.proto`.
2. Generate helpers with `make errors`.
3. Keep layer boundaries clear.
4. Use one consistent error strategy per service.

## Proto as the Source of Truth

Define reasons and codes in `api/<service>/v1/errors.proto`:

```protobuf
syntax = "proto3";

package api.user.v1;

import "errors/errors.proto";

enum ErrorReason {
  option (errors.default_code) = 500;

  USER_NOT_FOUND = 0 [(errors.code) = 404];
  USER_ALREADY_EXISTS = 1 [(errors.code) = 409];
  USER_NAME_EMPTY = 2 [(errors.code) = 400];
}
```
Generate helpers:

```bash
make errors
```
Generated code gives you:

- `ErrorUserNotFound(...)`
- `IsUserNotFound(err)`

Prefer those helpers for client-facing API errors because they keep code and reason aligned with proto.

## Error Handling Pattern (kratos-layout Style)


This follows the [official kratos-layout](https://github.com/go-kratos/kratos-layout). Biz defines Kratos errors using proto `ErrorReason` constants directly.

**Biz error definitions:**

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
    ErrUserNameEmpty = errors.BadRequest(v1.ErrorReason_USER_NAME_EMPTY.String(), "user name is empty")
)
```
**Data layer** translates ORM errors to biz Kratos errors (see [Data Layer Responsibilities](#data-layer-responsibilities)).

**Biz error translation** — biz MUST NOT blindly return repo errors (see [Biz Layer Error Translation](#biz-layer-error-translation)).

**Service** can pass biz errors through directly since they are already proper Kratos errors:

```go
// internal/service/user.go
func (s *UserService) GetUser(ctx context.Context, req *v1.GetUserRequest) (*v1.GetUserReply, error) {
    user, err := s.uc.GetUser(ctx, req.Id)
    if err != nil {
        // Biz errors are already Kratos errors with correct codes.
        // Pass through directly:
        return nil, err
        // Or remap for richer client messages:
        // if biz.ErrUserNotFound.Is(err) {
        //     return nil, v1.ErrorUserNotFound("user %d not found", req.Id)
        // }
        // return nil, err
    }
    return &v1.GetUserReply{User: toProtoUser(user)}, nil
}
```

> **Defense-in-depth:** Biz errors are already Kratos errors, so pass-through is safe. But if a raw (non-Kratos) error somehow leaks from biz, service should catch it rather than exposing internals to the client:
>
> ```go
> if err != nil {
>     if se := new(kratosErrors.Error); !errors.As(err, &se) {
>         // Not a Kratos error — wrap it so clients never see raw details.
>         return nil, kratosErrors.InternalServer("INTERNAL_ERROR", "internal server error")
>     }
>     return nil, err
> }
> ```
`biz.ErrUserNotFound` is a complete Kratos error with HTTP code and reason from proto — it can travel all the way to the client without further mapping. The tradeoff is that biz imports `v1` for `ErrorReason` constants, which creates a compile-time dependency. In practice this is just the error reason enum, not request/response types, so the coupling is minimal.

## Biz Layer Error Translation

Biz has two error responsibilities: business validation and data error translation. Never blindly `return uc.repo.Method(ctx, ...)`.

```go
func (uc *UserUsecase) CreateUser(ctx context.Context, user *User) (*User, error) {
    // 1. Business validation — domain rules checked before touching the repo.
    if user.Name == "" {
        return nil, ErrUserNameEmpty
    }

    // 2. Data error translation — pass through known domain errors, wrap unknown ones.
    result, err := uc.repo.Save(ctx, user)
    if err != nil {
        if ErrUserExists.Is(err) {
            return nil, ErrUserExists
        }
        // Unknown error: log the real cause, return a clean domain error.
        uc.log.WithContext(ctx).Errorf("create user: %v", err)
        return nil, errors.InternalServer("INTERNAL_ERROR", "create user failed")
    }
    return result, nil
}

func (uc *UserUsecase) GetUser(ctx context.Context, id int64) (*User, error) {
    user, err := uc.repo.GetByID(ctx, id)
    if err != nil {
        if ErrUserNotFound.Is(err) {
            return nil, ErrUserNotFound
        }
        uc.log.WithContext(ctx).Errorf("get user %d: %v", id, err)
        return nil, errors.InternalServer("INTERNAL_ERROR", "internal server error")
    }
    return user, nil
}
```
**Why this matters.** Raw data layer errors can leak sensitive infrastructure details to API clients:

- GORM: `Error 1045 (28000): Access denied for user 'root'@'localhost'`
- Ent: `ent: user not found` with schema/table details
- SQL driver: connection strings, hostnames, internal IPs

These should never reach callers. The biz layer is the firewall: known domain errors pass through, everything else gets logged and replaced with a clean Kratos error.

## Data Layer Responsibilities

Data owns driver, ORM, and remote-client details. Translate infrastructure errors into biz-level errors:

```go
// Ent example
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
```

```go
// GORM example
func (r *userRepo) GetByID(ctx context.Context, id int64) (*biz.User, error) {
    var u User
    if err := r.db.WithContext(ctx).First(&u, id).Error; err != nil {
        if errors.Is(err, gorm.ErrRecordNotFound) {
            return nil, biz.ErrUserNotFound
        }
        return nil, fmt.Errorf("query user %d: %w", id, err)
    }
    return toBizUser(&u), nil
}
```

> **Anti-pattern: blanket catch.** Mapping all errors to a single domain error (`if err != nil { return nil, biz.ErrXxx }`) destroys error specificity. A connection timeout, a constraint violation, and a not-found are all different situations. For query operations, catch specific ORM errors (e.g., `ent.IsNotFound`, `gorm.ErrRecordNotFound`). For write operations, catch constraint errors when the domain has a meaningful response. Always wrap unknown errors with `fmt.Errorf("...: %w", err)` to preserve the error chain.

Rules:

- Data must not import `api/v1`.
- Biz may import `v1` for `ErrorReason` only; biz must not import ORM packages.
- Data must catch specific ORM errors for queries (`ent.IsNotFound`, `gorm.ErrRecordNotFound`) — do not map all errors to a single domain error. Wrap unknown errors with `%w`.
- Unknown errors should be wrapped with context using `%w`.

## Manual Constructors

Kratos allows direct constructors such as:

```go
errors.New(404, "USER_NOT_FOUND", "user not found")
```

That is valid, but do not spread it everywhere. Prefer generated helpers for public API errors, and prefer a centralized strategy over one-off reason strings that drift from proto.

## Cross-Service Error Propagation

Remote Kratos errors come back through gRPC status details. Unwrap them before deciding what your own service should return:

```go
resp, err := c.GetUser(ctx, req)
if err != nil {
    if se, ok := status.FromError(err); ok {
        switch se.Code() {
        case codes.DeadlineExceeded:
            return nil, v1.ErrorDependencyTimeout("user service timeout")
        case codes.Unavailable:
            return nil, v1.ErrorDependencyUnavailable("user service unavailable")
        }
    }
    if e := errors.FromError(err); e != nil && e.Reason == remotev1.ErrorReason_USER_NOT_FOUND.String() {
        return nil, v1.ErrorOrderUserNotFound("user %d not found", req.UserId)
    }
    return nil, v1.ErrorInternalError("user service request failed")
}
```

Do not pass remote service reasons and codes straight through to your own callers unless the two services intentionally share one public API contract.

## Common Mistakes

- Data layer returns `v1.ErrorXxx(...)` — data must not import `api/v1`
- Biz imports `gorm` or `ent` — biz must not import ORM packages
- Service leaks raw database or remote-client errors to the client
- Ad-hoc `errors.New(code, reason, msg)` scattered across files
- One service mixes incompatible error strategies without a clear boundary
- Biz blindly returns `uc.repo.Method(ctx, ...)` without error translation
- GORM/Ent errors leaking through biz to service/client, exposing SQL details
