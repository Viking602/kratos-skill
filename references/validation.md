# Parameter Validation Patterns

## Core Concepts

Kratos uses `protoc-gen-validate` (PGV) to define validation rules in Protocol Buffers.

**Advantages**:
- Declarative validation: define validation rules in proto files
- Codegen: automatically generate validation code
- Type safety: compile-time validation rule checking
- Unified standard: consistent validation specification across services

> **PGV vs protovalidate**: The current Kratos ecosystem standard uses `protoc-gen-validate` (PGV). The Buf team has released [`protovalidate`](https://github.com/bufbuild/protovalidate) as the next-generation alternative (based on CEL expressions, supports cross-field validation), but Kratos middleware has not yet natively integrated protovalidate. New projects may want to follow its development.

## Middleware Configuration

```go
httpSrv := http.NewServer(
    http.Address(":8000"),
    http.Middleware(validate.Validator()),
)
```

## Common Validation Rules

### Numeric Types

```protobuf
message IntegerValidation {
    int32 age = 1 [(validate.rules).int32 = {gt: 0, lte: 150}];
    int32 status = 2 [(validate.rules).int32.const = 1];
    uint32 code = 3 [(validate.rules).uint32 = {in: [200, 400, 500]}];
    uint64 exclude_id = 4 [(validate.rules).uint64 = {not_in: [0]}];
    float score = 5 [(validate.rules).float = {gte: 0, lte: 100}];
}
```

| Rule | Description | Example |
|------|-------------|---------|
| `const` | Must equal the specified value | `const: 42` |
| `lt` / `lte` | Must be less than / less than or equal to | `lt: 100` |
| `gt` / `gte` | Must be greater than / greater than or equal to | `gt: 0` |
| `in` | Must be in the list | `in: [1, 2, 3]` |
| `not_in` | Must not be in the list | `not_in: [0, -1]` |

### String Types

```protobuf
message StringValidation {
    string phone = 1 [(validate.rules).string.len = 11];
    string name = 2 [(validate.rules).string = {min_len: 1, max_len: 100}];
    string email = 3 [(validate.rules).string.email = true];
    string uuid = 4 [(validate.rules).string.uuid = true];
    string uri = 5 [(validate.rules).string.uri = true];
    string phone_regex = 6 [(validate.rules).string.pattern = "^1[3-9]\\d{9}$"];
    string path = 7 [(validate.rules).string = {prefix: "/api/"}];
    string file = 8 [(validate.rules).string = {suffix: ".txt"}];
}
```

| Rule | Description | Example |
|------|-------------|---------|
| `const` | Must equal the specified value | `const: "hello"` |
| `len` | Length must equal | `len: 11` |
| `min_len` / `max_len` | Minimum / maximum length | `min_len: 1` |
| `pattern` | Regex pattern match | `pattern: "^[a-z]+$"` |
| `email` / `uuid` / `uri` | Format validation | `email: true` |
| `prefix` / `suffix` | Prefix / suffix match | `prefix: "/api/"` |

### Boolean Types

```protobuf
message BoolValidation {
    bool active = 1 [(validate.rules).bool.const = true];
    bool deleted = 2 [(validate.rules).bool.const = false];
}
```

### Enum Types

```protobuf
enum Status {
    STATUS_UNSPECIFIED = 0;
    STATUS_ACTIVE = 1;
    STATUS_INACTIVE = 2;
}

message EnumValidation {
    Status status = 1 [(validate.rules).enum.defined_only = true];
}
```

### Nested Messages

```protobuf
message Address {
    string street = 1 [(validate.rules).string.min_len = 1];
    string city = 2 [(validate.rules).string.min_len = 1];
    string zip = 3 [(validate.rules).string.pattern = "^\\d{6}$"];
}

message NestedValidation {
    // Nested message must exist and be valid
    Address address = 1 [(validate.rules).message.required = true];
    // Optional nested message (must be valid if present)
    Address optional_address = 2;
}
```

### Arrays (Repeated)

```protobuf
message RepeatedValidation {
    repeated string tags = 1 [(validate.rules).repeated = {
        min_items: 1,
        max_items: 10,
        unique: true
    }];
    // Array element validation
    repeated string emails = 2 [(validate.rules).repeated = {
        min_items: 1,
        items: {string: {email: true}}
    }];
}
```

| Rule | Description | Example |
|------|-------------|---------|
| `min_items` / `max_items` | Minimum / maximum number of elements | `min_items: 1` |
| `unique` | Elements must be unique | `unique: true` |
| `items` | Element validation rules | `items: {string: {email: true}}` |

### Map Validation

```protobuf
message MapValidation {
    map<string, string> metadata = 1 [(validate.rules).map = {
        min_pairs: 0,
        max_pairs: 100
    }];
    // Key and Value validation
    map<string, string> config = 2 [(validate.rules).map = {
        keys: {string: {min_len: 1, max_len: 64}},
        values: {string: {max_len: 1024}}
    }];
}
```

### Oneof Validation

```protobuf
message OneofValidation {
    oneof identifier {
        option (validate.required) = true;  // Must select one
        string username = 1 [(validate.rules).string = {min_len: 3, max_len: 32}];
        string email = 2 [(validate.rules).string.email = true];
        int64 user_id = 3 [(validate.rules).int64 = {gt: 0}];
    }
}
```

## Practical Examples & Best Practices

### User Registration Request

Comprehensive use of multiple validation rules: multi-layer validation (length + format), business range constraints, array element validation.

```protobuf
message CreateUserRequest {
    // Username: 4-32 characters, must start with a letter
    string username = 1 [(validate.rules).string = {
        pattern: "^[a-zA-Z][a-zA-Z0-9_]{3,31}$"
    }];
    // Email: length + format dual validation (RFC 5321 max 254)
    string email = 2 [(validate.rules).string = {
        min_len: 5, max_len: 254, email: true
    }];
    // Password: 8-64 characters, must contain uppercase, lowercase, and digits
    string password = 3 [(validate.rules).string = {
        min_len: 8, max_len: 64,
        pattern: "^(?=.*[a-z])(?=.*[A-Z])(?=.*\\d).+$"
    }];
    // Phone number: 11-digit number
    string phone = 4 [(validate.rules).string = {
        pattern: "^1[3-9]\\d{9}$"
    }];
    // Age: reasonable business range, not the full int32 range
    int32 age = 5 [(validate.rules).int32 = {gt: 0, lte: 150}];
    // Tags: limit count + element length + uniqueness
    repeated string tags = 6 [(validate.rules).repeated = {
        max_items: 10,
        items: {string: {max_len: 20}},
        unique: true
    }];
    // Nested message: must be provided
    Address address = 7 [(validate.rules).message.required = true];
}
```

### Paginated Query Request

```protobuf
message ListUsersRequest {
    int32 page = 1 [(validate.rules).int32 = {gte: 1}];
    int32 page_size = 2 [(validate.rules).int32 = {gte: 1, lte: 100}];
    string order_by = 3 [(validate.rules).string = {
        in: ["created_at", "updated_at", "name"]
    }];
    string sort_order = 4 [(validate.rules).string = {
        in: ["asc", "desc"]
    }];
    string keyword = 5 [(validate.rules).string = {max_len: 100}];
}
```

### Cross-Field Validation

PGV only supports single-field validation and cannot express inter-field dependencies (e.g., `start_date` must be before `end_date`). Such validation should be handled in the biz layer:

```go
func (uc *OrderUsecase) CreateOrder(ctx context.Context, req *CreateOrderReq) error {
    // PGV has validated individual field formats; perform cross-field business validation here
    if req.StartDate.AsTime().After(req.EndDate.AsTime()) {
        return v1.ErrorInvalidDateRange("start must be before end")
    }
    if req.MinPrice > req.MaxPrice {
        return v1.ErrorInvalidPriceRange("min_price must <= max_price")
    }
    // ...business logic
}
```

**Principle**: Use PGV at the proto layer for single-field format/range validation; use the biz layer for cross-field business rule validation. The two layers together provide complete validation coverage.

## Validation Error Handling

### Default Error Response

```json
{
  "code": 400,
  "reason": "VALIDATOR",
  "message": "validation error: ..."
}
```

### Custom Error Handling

```go
httpSrv := http.NewServer(
    http.Middleware(
        validate.Validator(
            validate.WithErrorFunc(func(field string, err error) error {
                return errors.BadRequest(
                    "VALIDATION_ERROR",
                    fmt.Sprintf("field %s validation failed: %v", field, err),
                )
            }),
        ),
    ),
)
```
