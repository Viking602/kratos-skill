# API Definition Patterns (Protobuf)

## Table of Contents

- [Core Specifications](#core-specifications) — File header, naming rules
- [Service Definition](#service-definition) — RESTful mapping, HTTP method mapping
- [Custom Actions](#custom-actions) — Non-CRUD POST actions
- [Message Definition](#message-definition) — Request/response naming, common patterns
- [Code Generation](#code-generation) — Kratos CLI and Makefile
- [Backward Compatibility](#backward-compatibility) — Safe changes and breaking changes
- [API Versioning](#api-versioning) — v1/v2 coexistence strategy
- [Best Practices](#best-practices) — FieldMask, Oneof, Empty

## Core Specifications

### File Header

```protobuf
syntax = "proto3";
package helloworld.v1;

import "google/api/annotations.proto";
import "openapi/v3/annotations.proto";
import "validate/validate.proto";
import "errors/errors.proto";

option go_package = "github.com/example/helloworld/api/helloworld/v1;v1";
```

### Naming Conventions

| Type | Naming Rule | Example |
|------|----------|------|
| File name | snake_case | `user_service.proto` |
| Package name | lowercase | `helloworld.v1` |
| Message name | PascalCase | `GetUserRequest` |
| Field name | snake_case | `user_name` |
| Enum name | PascalCase | `UserStatus` |
| Enum value | UPPER_SNAKE_CASE | `STATUS_ACTIVE` |
| Service name | PascalCase | `UserService` |
| RPC method name | PascalCase | `GetUser` |

## Service Definition

### RESTful Mapping with OpenAPI v3 Annotations

Every RPC should carry three annotations: (1) a leading `//` comment for the method, (2) `google.api.http` for REST routing, and (3) `openapi.v3.operation` for API documentation. The leading comment is also picked up by protoc-gen-openapiv3 and appears as the operation's description in the generated `openapi.yaml`, so it should be concise and meaningful.

```protobuf
service UserService {
  // Get user by ID
  rpc GetUser(GetUserRequest) returns (GetUserReply) {
    option (google.api.http) = {
      get: "/api/user/v1/users/{id}"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "Get user"
      description: "Retrieve user details by ID, including status and creation time"
    };
  }

  // List users
  rpc ListUsers(ListUsersRequest) returns (ListUsersReply) {
    option (google.api.http) = {
      get: "/api/user/v1/users"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "List users"
      description: "Paginated user listing with keyword search and sorting"
    };
  }

  // Create user
  rpc CreateUser(CreateUserRequest) returns (CreateUserReply) {
    option (google.api.http) = {
      post: "/api/user/v1/users"
      body: "*"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "Create user"
      description: "Create a new user; name and email are required"
    };
  }

  // Update user
  rpc UpdateUser(UpdateUserRequest) returns (UpdateUserReply) {
    option (google.api.http) = {
      put: "/api/user/v1/users/{id}"
      body: "user"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "Update user"
      description: "Partially update user fields specified by update_mask"
    };
  }

  // Delete user
  rpc DeleteUser(DeleteUserRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      delete: "/api/user/v1/users/{id}"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "Delete user"
      description: "Delete a user by ID; this action is irreversible"
    };
  }
}
```

### HTTP Method Mapping

| Operation | HTTP Method | URL Pattern |
|------|-----------|----------|
| Get a single resource | GET | `/v1/{resource}/{id}` |
| List resources | GET | `/v1/{resource}` |
| Create a resource | POST | `/v1/{resource}` |
| Full update | PUT | `/v1/{resource}/{id}` |
| Partial update | PATCH | `/v1/{resource}/{id}` |
| Delete a resource | DELETE | `/v1/{resource}/{id}` |
| Custom action | POST | `/v1/{resource}/{id}:{action}` |

## Custom Actions

When standard CRUD cannot cover the business semantics, use the `POST /{resource}/{id}:{action}` pattern:

```protobuf
service UserService {
  // Disable user account
  rpc DisableUser(DisableUserRequest) returns (DisableUserReply) {
    option (google.api.http) = {
      post: "/api/user/v1/users/{id}:disable"
      body: "*"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "Disable user account"
      description: "Disable the specified user account; the user will be unable to log in"
    };
  }

  // Reset user password
  rpc ResetPassword(ResetPasswordRequest) returns (google.protobuf.Empty) {
    option (google.api.http) = {
      post: "/api/user/v1/users/{id}:resetPassword"
      body: "*"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "Reset user password"
      description: "Reset the login password for the specified user; requires admin privileges"
    };
  }

  // Batch create users
  rpc BatchCreateUsers(BatchCreateUsersRequest) returns (BatchCreateUsersReply) {
    option (google.api.http) = {
      post: "/api/user/v1/users:batchCreate"
      body: "*"
    };
    option (openapi.v3.operation) = {
      tags: "User Management"
      summary: "Batch create users"
      description: "Create multiple users in one request; maximum 100 per batch"
    };
  }
}
```

**Naming guidelines:** Action names use camelCase (`resetPassword`); collection-level operations omit `{id}`.

## Message Definition

### Request/Response Naming

```protobuf
// ✅ Correct: use Request/Reply suffix
rpc GetUser(GetUserRequest) returns (GetUserReply);

// ❌ Wrong: non-standard naming
rpc GetUser(GetUserReq) returns (GetUserResp);  // Do not use abbreviations
rpc GetUser(Request) returns (Response);        // Too generic
```

### Common Patterns

#### Single Resource Operation

```protobuf
// Get user request
message GetUserRequest {
  // User ID
  int64 id = 1 [(validate.rules).int64 = {gt: 0}];
}


// Get user response
message GetUserReply {
  // User details
  User user = 1;
}

// Create user request
message CreateUserRequest {
  // User name
  string name = 1 [(validate.rules).string = {min_len: 1, max_len: 100}];
  // User email address
  string email = 2 [(validate.rules).string.email = true];
}

// Create user response
message CreateUserReply {
}
```

#### List Query

```protobuf
message ListUsersRequest {
  int32 page = 1 [(validate.rules).int32 = {gte: 1}];
  int32 page_size = 2 [(validate.rules).int32 = {gte: 1, lte: 100}];
  string order_by = 3;
  string keyword = 4 [(validate.rules).string = {max_len: 100}];
}

message ListUsersReply {
  repeated User users = 1;
  int32 total = 2;  // Total record count
}
```

#### Resource Message

```protobuf
message User {
  int64 id = 1;
  string name = 2;
  string email = 3;
  UserStatus status = 4;
  google.protobuf.Timestamp created_at = 5;
  google.protobuf.Timestamp updated_at = 6;
}

enum UserStatus {
  USER_STATUS_UNSPECIFIED = 0;  // Must include UNSPECIFIED = 0
  USER_STATUS_ACTIVE = 1;
  USER_STATUS_INACTIVE = 2;
  USER_STATUS_DELETED = 3;
}
```

## Code Generation

### Using Kratos CLI

```bash
# Generate client code
kratos proto client api/helloworld/helloworld.proto

# Generate server template
kratos proto server api/helloworld/helloworld.proto -t internal/service
```

### Using Makefile

```makefile
API_PROTO_FILES=$(shell find api -name *.proto)

.PHONY: api
api:
	protoc --proto_path=. \
	       --proto_path=./third_party \
	       --go_out=paths=source_relative:. \
	       --go-grpc_out=paths=source_relative:. \
	       --go-http_out=paths=source_relative:. \
	       $(API_PROTO_FILES)
```

## Backward Compatibility

### Safe Changes ✅

```protobuf
// ✅ Add new field (use a new field number)
message User {
  int64 id = 1;
  string name = 2;
  string phone = 3;  // Added
}

// ✅ Add new RPC method
service UserService {
  rpc GetUser(GetUserRequest) returns (GetUserReply);
  rpc GetUserProfile(GetUserRequest) returns (GetUserProfileReply);  // Added
}

// ✅ Add new enum value
enum UserStatus {
  USER_STATUS_UNSPECIFIED = 0;
  USER_STATUS_ACTIVE = 1;
  USER_STATUS_PENDING = 2;  // Added
}
```

### Breaking Changes ❌

```protobuf
// ❌ Deleting a field (use reserved instead)
message User {
  int64 id = 1;
  // string name = 2;  // Do not delete directly
}

// ✅ Correct: use reserved
message User {
  reserved 2;
  reserved "name";
  int64 id = 1;
  string email = 3;  // New fields can be added
}

// ❌ Do not change field type or field number
message User {
  // int32 id = 1;   // Do not change the type
  int64 id = 2;     // Do not change the field number
}
```

## API Versioning

### When to Create v2

Only create a new version when **breaking changes are unavoidable**:

- Deleting or renaming fields/RPC methods
- Changing field type or semantics (e.g., `id` from `int64` to `string`)
- Redesigning the resource model structure

**Cases that do not require a new version:** Adding fields, adding RPCs, adding enum values — these are all backward compatible.

### v1 and v2 Coexistence

Directory structure is isolated by version; the server registers both versions simultaneously:

```
api/
  user/
    v1/
      user.proto          # package user.v1
    v2/
      user.proto          # package user.v2
```

```go
// Register both versions in main.go or server initialization
import (
    v1 "example/api/user/v1"
    v2 "example/api/user/v2"
)

// HTTP server
v1.RegisterUserServiceHTTPServer(httpSrv, userSvcV1)
v2.RegisterUserServiceHTTPServer(httpSrv, userSvcV2)

// gRPC server
v1.RegisterUserServiceServer(grpcSrv, userSvcV1)
v2.RegisterUserServiceServer(grpcSrv, userSvcV2)
```

**Migration strategy:** After marking v1 as deprecated, keep it running for at least one full release cycle. Only decommission after monitoring confirms zero traffic.

## Best Practices

### 1. Use FieldMask for Partial Updates

FieldMask allows clients to specify exactly which fields to update, avoiding the risk of full overwrites:

```protobuf
import "google/protobuf/field_mask.proto";

message UpdateUserRequest {
  int64 id = 1 [(validate.rules).int64 = {gt: 0}];
  // Client provides the field paths to update, e.g., ["name", "email"]
  google.protobuf.FieldMask update_mask = 2;
  User user = 3;
}
```

Server-side handling logic:

```go
func (s *UserService) UpdateUser(ctx context.Context, req *v1.UpdateUserRequest) (*v1.UpdateUserReply, error) {
    // Only update fields specified in update_mask
    for _, path := range req.UpdateMask.GetPaths() {
        switch path {
        case "name":
            user.Name = req.User.Name
        case "email":
            user.Email = req.User.Email
        }
    }
    // ...
}
```

### 2. Use Oneof for Mutually Exclusive Fields

```protobuf
message SearchRequest {
  oneof search_by {
    string username = 1;
    string email = 2;
    int64 user_id = 3;
  }
}
```

### 3. Use Empty for No Return Value

```protobuf
import "google/protobuf/empty.proto";

rpc DeleteUser(DeleteUserRequest) returns (google.protobuf.Empty);
```
