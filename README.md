# entoas-server

A code generation extension for [Ent](https://entgo.io) that produces fully functional OpenAPI REST API handlers from your Ent schemas. Works with [entoas](https://entgo.io/docs/generating-openapi-spec) to generate the OpenAPI spec and [oapi-codegen](https://github.com/oapi-codegen/oapi-codegen) to generate the server interface, then fills in the handler implementations with Ent-backed CRUD logic.

## Features

- **CRUD Handler Generation** — Create, Read, Update, Delete, and List handlers for every exposed entity
- **Filtering** — Auto-generated query parameters for string, numeric, boolean, enum, time, and UUID fields
- **Sorting** — Multi-field sorting with configurable defaults
- **Offset Pagination** — Page-based pagination with total count
- **Cursor Pagination** — Efficient cursor-based pagination for large datasets
- **Field Selection** — `?fields=id,name,email` to return only requested fields
- **Soft Delete** — `deleted_at` timestamp instead of hard deletes
- **Patch Semantics** — JSON merge patch that distinguishes absent fields from explicit nulls
- **Auth Guards** — Inject authentication/authorization checks via schema annotations
- **Before/After Hooks** — Inject custom logic at any point in the handler lifecycle
- **Response Validation** — Optional test helper that validates responses against the OpenAPI spec
- **Entity Converters** — Reflection-based Ent entity to OpenAPI model converters with edge support

All features can be enabled globally or per-entity via annotations.

## Examples

Reference examples are in the [`examples/`](examples/) directory:

| Example | Description |
|---------|-------------|
| [basic](examples/basic) | Minimal setup with pagination, filtering, and sorting |
| [full-featured](examples/full-featured) | All features: soft delete, patch semantics, cursor pagination, field selection |
| [with-auth](examples/with-auth) | authguard integration with ownership guards and role-based access |

## Installation

```bash
go get github.com/tiagoposse/entoas-server
```

## Quick Start

### 1. Configure code generation

In your `ent/entc.go`:

```go
//go:build ignore

package main

import (
    "log"

    "entgo.io/contrib/entoas"
    "entgo.io/ent/entc"
    "entgo.io/ent/entc/gen"
    "github.com/ogen-go/ogen"
    entoasserver "github.com/tiagoposse/entoas-server"
)

func main() {
    spec := new(ogen.Spec)

    oasExt, err := entoas.NewExtension(
        entoas.Spec(spec),
        entoas.SimpleModels(),
    )
    if err != nil {
        log.Fatalf("creating entoas extension: %v", err)
    }

    apiExt, err := entoasserver.NewExtension(spec,
        entoasserver.WithOutputDir("internal/api"),
        entoasserver.WithPackageName("api"),
        entoasserver.WithPagination(30),
        entoasserver.WithFieldFiltering(true),
        entoasserver.WithSorting("created_at:desc"),
    )
    if err != nil {
        log.Fatalf("creating entoas-server extension: %v", err)
    }

    err = entc.Generate("./schema", &gen.Config{},
        entc.Extensions(oasExt, apiExt),
    )
    if err != nil {
        log.Fatalf("running ent codegen: %v", err)
    }
}
```

### 2. Define your schema

```go
package schema

import (
    "time"

    "entgo.io/contrib/entoas"
    "entgo.io/ent"
    "entgo.io/ent/schema"
    "entgo.io/ent/schema/field"
    "github.com/google/uuid"
)

type Article struct {
    ent.Schema
}

func (Article) Fields() []ent.Field {
    return []ent.Field{
        field.UUID("id", uuid.UUID{}).Default(uuid.New).Immutable(),
        field.String("title").NotEmpty(),
        field.Text("body"),
        field.Enum("status").Values("draft", "published", "archived").Default("draft"),
        field.Time("created_at").Default(time.Now).Immutable(),
        field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now),
    }
}

func (Article) Annotations() []schema.Annotation {
    return []schema.Annotation{
        entoas.CreateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
        entoas.ReadOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
        entoas.UpdateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
        entoas.DeleteOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
        entoas.ListOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
    }
}
```

### 3. Run code generation

```bash
go generate ./ent
```

### 4. Wire the server

```go
handler := api.NewEntHandler(entClient)
strictHandler := api.NewStrictHandler(handler, nil)
http.ListenAndServe(":8080", api.Handler(strictHandler))
```

## Generated Output

The extension generates the following files in your output directory:

| File | Description |
|------|-------------|
| `openapi.json` | OpenAPI 3.0 spec with all endpoints and query parameters |
| `server.gen.go` | oapi-codegen server interface, request/response types, HTTP routing |
| `handlers.gen.go` | Ent-backed CRUD handler implementations with hooks and guards |
| `converters.gen.go` | Ent entity to OpenAPI model converters |
| `validation_helper_test.go` | Response validation against OpenAPI spec (optional) |

## Configuration Options

```go
entoasserver.NewExtension(spec,
    // Output
    entoasserver.WithOutputDir("internal/api"),
    entoasserver.WithPackageName("api"),
    entoasserver.WithModulePath("github.com/org/app/ent"), // auto-detected if omitted

    // Pagination (pick one)
    entoasserver.WithPagination(30),       // offset-based, 30 items/page
    entoasserver.WithCursorPagination(),   // cursor-based

    // Query features
    entoasserver.WithFieldFiltering(true),          // auto-generate filter params
    entoasserver.WithSorting("created_at:desc"),    // sortable fields + default
    entoasserver.WithFieldSelection(),              // ?fields=id,name

    // Write behavior
    entoasserver.WithSoftDelete(),         // soft-delete via deleted_at
    entoasserver.WithPatchSemantics(),     // JSON merge patch for updates

    // Hooks
    entoasserver.WithBeforeHandlerHook(hook),
    entoasserver.WithAfterHandlerHook(hook),

    // Testing
    entoasserver.WithResponseValidation("openapi.json"),
)
```

## Filtering

When enabled, the extension auto-generates query parameters based on field types:

| Field Type | Parameters | Example |
|------------|-----------|---------|
| `string` | `?field=`, `?field_contains=` | `?name=alice`, `?name_contains=ali` |
| `int`, `float` | `?field=`, `?field_gte=`, `?field_lte=` | `?age_gte=18`, `?price_lte=100` |
| `bool` | `?field=` | `?active=true` |
| `enum` | `?field=` | `?status=published` |
| `time` | `?field_gte=`, `?field_lte=` | `?created_at_gte=2024-01-01T00:00:00Z` |
| `uuid` | `?field=` | `?owner_id=550e8400-...` |

String `_contains` filters are case-insensitive.

### Per-field control

```go
// Exclude a field from filtering
field.String("internal_notes").Annotations(entoasserver.NoFilter())

// Allow filtering on a sensitive field
field.String("email").Sensitive().Annotations(entoasserver.AllowFilter())
```

### Per-entity control

```go
// Whitelist: only these fields are filterable
entoasserver.FilterFields("name", "status", "created_at")

// Blacklist: all fields except these
entoasserver.FilterFieldsExclude("internal_notes", "metadata")
```

## Sorting

```
GET /articles?sort=created_at:desc,title:asc
```

The `id`, `created_at`, and `updated_at` fields are always sortable. Other fields follow the same include/exclude pattern as filtering.

```go
// Per-entity sort config with default
entoasserver.SortFields("title", "created_at", "status").WithDefaultSort("created_at:desc")
```

## Pagination

### Offset-based (default)

```
GET /articles?page=2&items_per_page=20
```

Response:
```json
{
    "items": [...],
    "total": 150,
    "page": 2,
    "items_per_page": 20
}
```

### Cursor-based

```
GET /articles?limit=20
GET /articles?cursor=dXVpZC1oZXJl&limit=20
```

Response:
```json
{
    "items": [...],
    "next_cursor": "bmV4dC11dWlk"
}
```

Per-entity override:

```go
entoasserver.Paginate(50)          // offset, 50 items/page
entoasserver.CursorPagination()    // cursor-based
```

## Field Selection

```
GET /articles?fields=id,title,status
```

Only returns the requested fields. Sensitive fields are never returned even if requested.

## Soft Delete

Requires a `deleted_at` field on the schema:

```go
field.Time("deleted_at").Optional().Nillable()
```

- **DELETE** sets `deleted_at = NOW()` instead of removing the row
- **LIST** and **READ** exclude rows where `deleted_at IS NOT NULL`

## Patch Semantics

Distinguishes between absent fields and explicit nulls in update requests:

```json
{"name": "New Name"}           // updates name only
{"name": "New Name", "bio": null}  // updates name, clears bio
{}                              // no changes
```

## Handler Hooks

Every generated handler has before and after hook variables that you can set to inject custom logic:

```go
// Before hooks — modify the query or builder before execution
var before{Entity}Create func(ctx context.Context, client *ent.Client, builder *ent.{Entity}Create) error
var before{Entity}Read   func(ctx context.Context, client *ent.Client, q **ent.{Entity}Query) error
var before{Entity}List   func(ctx context.Context, client *ent.Client, q **ent.{Entity}Query) error
var before{Entity}Update func(ctx context.Context, client *ent.Client, builder *ent.{Entity}UpdateOne) error
var before{Entity}Delete func(ctx context.Context, client *ent.Client) error

// After hooks — run after the operation succeeds
var after{Entity}Create func(ctx context.Context, client *ent.Client, entity *ent.{Entity}) error
var after{Entity}Read   func(ctx context.Context, client *ent.Client, entity *ent.{Entity}) error
var after{Entity}List   func(ctx context.Context, client *ent.Client, entities []*ent.{Entity}) ([]*ent.{Entity}, error)
var after{Entity}Update func(ctx context.Context, client *ent.Client, entity *ent.{Entity}) error
var after{Entity}Delete func(ctx context.Context, client *ent.Client, id uuid.UUID) error

// Custom handler override — bypass generated logic entirely
var custom{Entity}Create func(ctx context.Context, client *ent.Client, request Create{Entity}RequestObject) (Create{Entity}ResponseObject, error)
```

Example:

```go
// Eager-load edges on read
beforeArticleRead = func(ctx context.Context, _ *ent.Client, q **ent.ArticleQuery) error {
    *q = (*q).WithAuthor().WithComments()
    return nil
}

// Send notification after creation
afterArticleCreate = func(ctx context.Context, client *ent.Client, entity *ent.Article) error {
    return notifySubscribers(ctx, entity)
}
```

## Auth Guards

Integrate with [authguard](https://github.com/tiagoposse/authguard) (or any auth library) via the `AuthGuardHook`:

```go
apiExt, _ := entoasserver.NewExtension(spec,
    entoasserver.WithBeforeHandlerHook(entoasserver.AuthGuardHook(
        `auth "github.com/tiagoposse/authguard"`,
        map[string]entoasserver.EntityGuardTemplate{
            "requiresOwner": {
                FieldName: "OwnerID",
                FuncName:  "requiresOwner",
            },
        },
    )),
)
```

Then annotate schemas:

```go
auth.Guards().
    OnCreate("requiresAuth", "requiresScope:write").
    OnUpdate("requiresAuth", "requiresOwner").
    OnDelete("requiresAuth", "requiresOwner").
    OnList("requiresAuth")
```

**Simple guards** are resolved at runtime:
```go
if err := auth.Resolve("requiresAuth")(ctx, h.client); err != nil {
    return nil, err
}
```

**Entity guards** fetch the entity and check a field:
```go
_entity, _err := h.client.Article.Get(ctx, request.Id)
if _err != nil { return nil, _err }
if _err = requiresOwner(ctx, _entity.OwnerID); _err != nil { return nil, _err }
```

## Per-Entity Annotations

All global options can be overridden per-entity:

```go
func (Article) Annotations() []schema.Annotation {
    return []schema.Annotation{
        // entoas CRUD exposure
        entoas.ListOperation(entoas.OperationPolicy(entoas.PolicyExpose)),

        // entoas-server features
        entoasserver.Paginate(50),
        entoasserver.SortFields("title", "created_at").WithDefaultSort("created_at:desc"),
        entoasserver.FilterFields("title", "status"),
        entoasserver.FieldSelection(),
        entoasserver.SoftDelete(),
        entoasserver.PatchSemantics(),
        entoasserver.CursorPagination(),
    }
}
```

## License

See [LICENSE](LICENSE) for details.
