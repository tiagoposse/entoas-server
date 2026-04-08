# CLAUDE.md — entoas-server

## What This Is

A Go code generation extension for [Ent ORM](https://entgo.io) that generates REST API handlers from Ent schemas. It plugs into the Ent codegen pipeline alongside `entoas` (which generates the OpenAPI spec) and `oapi-codegen` (which generates the Go server interface). This extension fills in the handler implementations with Ent-backed CRUD logic.

**Module path:** `github.com/tiagoposse/entoas-server`

## Code Generation Pipeline

```
Ent schemas (user-written)
    → entoas generates OpenAPI spec (openapi.json)
    → This extension reads the spec + Ent graph and generates:
        → server.gen.go   (via oapi-codegen — types, interfaces, routing)
        → handlers.gen.go (CRUD handler implementations)
        → converters.gen.go (Ent entity ↔ OpenAPI model converters)
```

The entry point is `ent/entc.go` in the consuming project, which calls `entc.Generate()` with both `entoas` and `entoas-server` as extensions.

## File Structure

```
extension.go          (1836 lines) — Main extension: code generation orchestration, feature resolvers, template execution
templates/
  handlers/
    list.tmpl         — List handler: filtering, sorting, pagination, edge expansion, auto-expand, strip-fields
    read.tmpl         — Read handler: field selection, edge expansion, auto-expand, strip-fields
    create.tmpl       — Create handler: field assignment, JSON custom types
    update.tmpl       — Update handler: patch semantics, optional field handling
    delete.tmpl       — Delete handler: soft-delete support
  converters.tmpl     — Reflection-based Ent entity → OpenAPI model conversion
```

### Annotation Files (one per feature)

Each file defines an annotation struct implementing `ent.Annotation` (with a `Name() string` method) and builder functions:

| File | Annotation Name | Purpose |
|------|----------------|---------|
| `pagination.go` | `EntAPIPagination` | Offset pagination with configurable page size |
| `cursor.go` | `EntAPICursorPagination` | Cursor-based pagination |
| `filtering.go` | `EntAPIFilterFields` / `EntAPINoFilter` / `EntAPIAllowFilter` | Field filtering control |
| `sorting.go` | `EntAPISortFields` | Sortable fields with default sort |
| `fieldselection.go` | `EntAPIFieldSelection` | `?fields=` query param |
| `edgeexpansion.go` | `EntAPIEdgeExpansion` | `?expand=` query param for eager loading |
| `softdelete.go` | `EntAPISoftDelete` | Soft delete via `deleted_at` field |
| `patchsemantics.go` | `EntAPIPatchSemantics` | JSON merge patch for updates |
| `autoexpand.go` | `EntAPIAutoExpand` | Auto-expand edges without `?expand=` param |
| `stripfields.go` | `EntAPIStripFields` | Remove fields from API responses |

### Hook Files

| File | Purpose |
|------|---------|
| `guardhook.go` | `AuthGuardHook` — injects auth guard checks from `authguard` annotations |
| `auditloghook.go` | `AuditLogHook` — injects audit logging calls |

## Architecture: How Features Are Added

Every feature follows the same pattern:

### 1. Annotation definition (e.g. `myfeature.go`)

```go
type MyFeatureAnnotation struct {
    Enabled bool   `json:"enabled"`
    Config  string `json:"config,omitempty"`
}

func (MyFeatureAnnotation) Name() string { return "EntAPIMyFeature" }

func MyFeature() *MyFeatureAnnotation {
    return &MyFeatureAnnotation{Enabled: true}
}
```

Users add this to their Ent schema's `Annotations()` method.

### 2. Global option (in `extension.go`)

```go
// Field on Extension struct
type Extension struct {
    ...
    myFeatureAll bool
}

// Option function
func WithMyFeature() Option {
    return func(e *Extension) { e.myFeatureAll = true }
}
```

### 3. Resolver (in `extension.go`)

Reads per-entity annotation first, falls back to global setting:

```go
func (e *Extension) resolveMyFeature(node *gen.Type) bool {
    if raw, ok := node.Annotations["EntAPIMyFeature"]; ok && raw != nil {
        data, _ := json.Marshal(raw)
        var ann MyFeatureAnnotation
        if json.Unmarshal(data, &ann) == nil && ann.Enabled {
            return true
        }
    }
    return e.myFeatureAll
}
```

### 4. Template data (in `extension.go`, `generateHandlers` method ~line 770)

Add field to the anonymous struct passed to `tmpl.ExecuteTemplate`:

```go
if err := tmpl.ExecuteTemplate(&buf, op, struct {
    ...
    MyFeature bool
}{
    ...
    MyFeature: e.resolveMyFeature(node),
}); err != nil { ... }
```

### 5. Template code (in `templates/handlers/*.tmpl`)

```go
{{- if .MyFeature }}
    // My feature logic here
{{- end }}
```

### 6. Imports (in `extension.go`, import resolution ~line 530)

If your feature needs additional imports (e.g. `"strings"`, `"time"`, entity packages), add checks in the import section of `generateHandlers`.

## Key Sections in extension.go

| Lines (approx) | Section |
|----------------|---------|
| 1-55 | Extension struct, types |
| 57-135 | Option functions (global config) |
| 137-175 | NewExtension constructor |
| 180-470 | generateCode — main orchestrator (spec mutation → oapi-codegen → handlers → converters) |
| 470-620 | generateHandlers — import resolution, header generation |
| 620-810 | generateHandlers — per-entity template execution loop |
| 810-860 | generateRemainingOperationStubs — stubs for custom endpoints |
| 860-1040 | generateConverters — entity ↔ API model conversion |
| 1040-1080 | templateFuncs — template helper functions |
| 1080-1200 | Field type detection helpers (isTimeField, isUUIDField, isEnumField, etc.) |
| 1200-1400 | Feature resolvers (resolvePagination, resolveSoftDelete, etc.) |
| 1400-1520 | Edge expansion (edgeInfo, getExpandableEdges) |
| 1520-1700 | Auto-expand and strip-fields resolvers |
| 1700-1836 | OpenAPI spec mutation (addFilterQueryParams, addSortQueryParams, etc.) |

## Template System

Templates use Go's `text/template`. Available functions (registered in `templateFuncs`):

| Function | Purpose |
|----------|---------|
| `lower` | `strings.ToLower` |
| `plural` | Simple English pluralization |
| `snakeToPascal` | `snake_case` → `PascalCase` |
| `toPascalCase` | Generic PascalCase conversion |
| `toCamelCase` | PascalCase with lowercase first letter |
| `oapiFieldName` | Handles acronyms (ID→Id, URL→Url) for oapi-codegen naming |
| `isOptional` | Whether a field generates a pointer type in oapi-codegen |
| `isSensitive` | Whether a field is marked sensitive |
| `isReadOnly` | Whether a field is read-only |
| `isSlice` | Whether a field is a slice type |
| `isEnum` | Whether a field is an enum |
| `isJSONCustomType` | Whether a field uses a custom JSON type |
| `split` | `strings.Split` |

Template context struct (passed to each handler template):

```go
struct {
    Node            *gen.Type          // Ent entity type with all fields, edges, annotations
    Package         string             // Output package name
    GuardCode       string             // Injected auth guard code (from BeforeHandlerHooks)
    AfterHookCode   string             // Injected after-hook code (from AfterHandlerHooks)
    Paginated       bool               // Offset pagination enabled
    DefaultPageSize int
    FilterFields    []filterableField  // Fields with auto-generated filter params
    SortFields      []sortableField    // Sortable fields
    DefaultSort     string             // e.g. "created_at:desc"
    SoftDelete      bool
    FieldSelection  bool
    CursorPag       bool               // Cursor pagination enabled
    PatchSemantics  bool
    EdgeExpansion   bool               // ?expand= param enabled
    Edges           []edgeInfo         // Expandable edges with nested edge info
    AutoExpand      []autoExpandInfo   // Edges to auto-expand (no ?expand= needed)
    StripFields     *stripFieldsConfig // Fields to nil out in responses
}
```

## Handler Override System

For each entity+operation, the extension generates a `var` that can be set to override the generated handler:

```go
var customUserRead func(context.Context, *ent.Client, ReadUserRequestObject) (ReadUserResponseObject, error)
var beforeUserRead func(context.Context, *ent.Client, **ent.UserQuery) error
var afterUserRead func(context.Context, *ent.Client, *ent.User) error
```

If `customUserRead` is non-nil, the generated handler short-circuits and delegates entirely.
`beforeUserRead` modifies the query before execution (add Where clauses, eager loading).
`afterUserRead` post-processes the entity after fetch (but before API conversion — cannot modify API response fields).

These are set in the consuming project's `server.go` (hand-written, not generated).

## Converter System (converters.gen.go)

Uses Go reflection to map Ent entity fields to OpenAPI model fields by JSON tag:

- Matches field by `json:"field_name"` tag
- Handles type conversions: `uuid.UUID` → `openapi_types.UUID`, enums, JSON custom types
- For pointer fields: wraps non-zero values in pointers
- **Important caveat**: Non-pointer base types (string, int, bool) always produce values even when zero. If an edge is loaded with `Select()` (partial fields), unselected fields appear as zero values (`""`, `0`, `false`) in the response. This is why the `StripFields` and `AutoExpand` features work at the API response level — the converter can't distinguish "not loaded" from "actually zero".

Edge conversion: recursively calls `entTo{TargetEntity}` for loaded edges. Unloaded edges are nil.

## Feature Details: AutoExpand

Automatically calls `.With{Edge}()` on queries without requiring `?expand=` query param. Supports field selection on the expanded edge.

**Annotation:**
```go
entapi.AutoExpandWithFields("user", "id", "display_name", "username").
    AddEdge("friend", "id", "display_name", "username").
    OnOperations("list") // optional: restrict to specific operations
```

**Generated code** (in list/read handlers, before the `?expand=` block):
```go
q = q.WithUser(func(sq *ent.UserQuery) {
    sq.Select(user.FieldID, user.FieldDisplayName, user.FieldUsername)
})
```

**Resolver:** Maps annotation edge names to Ent edge metadata, resolves field names to Ent field constants (handles `id` → `FieldID` correctly).

## Feature Details: StripFields

Nils out specified pointer fields on the API response struct after conversion. Supports a self-check that skips stripping when the authenticated user is viewing their own data.

**Annotation:**
```go
entapi.StripFields("email", "home_lat", "role", "banned").WithSelfCheck()
```

**Generated code** (after `entTo{Entity}` conversion):
```go
// Read handler:
if _viewerID := auth.OptionalAuth(ctx); _viewerID != uuid.UUID(result.Id) {
    stripUserFields(&result)
}

// List handler:
for i := range result {
    if _viewerID := auth.OptionalAuth(ctx); _viewerID != uuid.UUID(result[i].Id) {
        stripUserFields(&result[i])
    }
}
```

**Strip function** (generated once per entity at end of handlers.gen.go):
```go
func stripUserFields(e *User) {
    e.Email = nil
    e.HomeLat = nil
    e.Role = nil
    e.Banned = nil
}
```

**Resolver:** Maps JSON field names from the annotation to Go struct field names using `oapiFieldName`. Only works for pointer fields (all optional Ent fields generate pointer types in oapi-codegen).

**Self-check:** Requires `auth.OptionalAuth(ctx)` to be available — this comes from the `authguard` library, imported via the `AuthGuardHook`. If not using auth guards, the auth import must be provided via a custom `BeforeHandlerHook`.

## Build & Test

```bash
go build ./...    # Verify the extension compiles
go test ./...     # Run tests (if any)
```

To test changes end-to-end, regenerate the consuming project:
```bash
cd /path/to/consuming-project
make backend/generate   # or: cd ent && go generate ./...
go build ./...          # verify generated code compiles
```

## Common Pitfalls

1. **Template variable scoping**: Inside `{{ range }}`, `.` changes to the current element. Use `$` to access the root context, or capture values with `{{ $var := .Field }}` before entering inner ranges.

2. **Ent field constants**: Ent uses `FieldID` (all caps) not `FieldId`. The `snakeToPascal` function produces `Id` — use `StructField()` from the Ent field metadata instead when generating field constants.

3. **Duplicate function generation**: If both `read.tmpl` and `list.tmpl` need a helper function (like `stripFields`), generate it once in `extension.go` after the template loop, not inside individual templates.

4. **Import deduplication**: When adding imports for target entity packages (e.g. auto-expand), check if the package is already imported for the entity's own operations.

5. **Zero values in converters**: The reflection-based converter wraps all non-zero and non-time-zero values into pointers. For `string("")`, `int(0)`, `bool(false)` — these are treated as valid data and wrapped. This means `Select()` on a partial set of fields still produces all fields in the API response with zero values. Use `StripFields` or custom handlers to control this.
