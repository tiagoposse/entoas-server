package entoasserver

import (
	"bytes"
	"embed"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"entgo.io/contrib/entoas"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/codegen"
	"github.com/oapi-codegen/oapi-codegen/v2/pkg/util"
	"github.com/ogen-go/ogen"
)

// BeforeHandlerHook is a function that returns Go code to inject before a handler's
// main logic. It receives the entity name and operation (create/read/update/delete/list).
// Return empty string to inject nothing for that entity+operation.
type BeforeHandlerHook func(entityName string, operation string, annotations map[string]interface{}) (code string, imports []string)

// AfterHandlerHook is a function that returns Go code to inject after a handler's
// main logic succeeds. It receives the entity name and operation.
// The injected code has access to `ctx`, `h.client`, and `entity` (or `request.Id` for delete).
// Return empty string to inject nothing for that entity+operation.
type AfterHandlerHook func(entityName string, operation string, annotations map[string]interface{}) (code string, imports []string)

//go:embed templates/*
var templateFS embed.FS

// Extension is an entc extension that generates oapi-codegen server code
// with Ent-backed CRUD handlers.
type Extension struct {
	spec               *ogen.Spec
	modulePath         string // e.g. "github.com/posse/padelmatch/ent" — auto-detected if empty
	outputDir          string
	pkgName            string
	beforeHandlerHooks []BeforeHandlerHook
	afterHandlerHooks  []AfterHandlerHook
	paginateAll        bool
	defaultPageSize    int
	fieldFiltering     bool
	sortingAll         bool
	defaultSort        string
	softDeleteAll      bool
	fieldSelectionAll  bool
	cursorPagAll       bool
	patchSemanticsAll  bool
	edgeExpansionAll   bool
	autoExpandAll      []AutoExpandEdge // global auto-expand edges
	stripFieldsAll     []string         // global fields to strip from responses
	validationSpecPath string           // if set, generate response validation test helper
}

// Option configures the Extension.
type Option func(*Extension)

// WithModulePath sets the Go module path for the Ent package (e.g. "github.com/myorg/myapp/ent").
// If not set, auto-detected from the Ent graph config.
func WithModulePath(path string) Option {
	return func(e *Extension) { e.modulePath = path }
}

// WithOutputDir sets the output directory for generated code.
func WithOutputDir(dir string) Option {
	return func(e *Extension) { e.outputDir = dir }
}

// WithPackageName sets the package name for generated code.
func WithPackageName(name string) Option {
	return func(e *Extension) { e.pkgName = name }
}

// WithBeforeHandlerHook registers a hook that injects code before handler logic.
func WithBeforeHandlerHook(hook BeforeHandlerHook) Option {
	return func(e *Extension) { e.beforeHandlerHooks = append(e.beforeHandlerHooks, hook) }
}

// WithAfterHandlerHook registers a hook that injects code after handler logic succeeds.
func WithAfterHandlerHook(hook AfterHandlerHook) Option {
	return func(e *Extension) { e.afterHandlerHooks = append(e.afterHandlerHooks, hook) }
}

// WithPagination enables offset-based paginated list responses globally.
func WithPagination(defaultPageSize int) Option {
	return func(e *Extension) { e.paginateAll = true; e.defaultPageSize = defaultPageSize }
}

// WithFieldFiltering enables auto-generated field filtering on list endpoints.
func WithFieldFiltering(enabled bool) Option {
	return func(e *Extension) { e.fieldFiltering = enabled }
}

// WithSorting enables `?sort=field:direction` on list endpoints globally.
func WithSorting(defaultSort string) Option {
	return func(e *Extension) { e.sortingAll = true; e.defaultSort = defaultSort }
}

// WithSoftDelete enables soft-delete globally. Entities must have a `deleted_at` field.
func WithSoftDelete() Option {
	return func(e *Extension) { e.softDeleteAll = true }
}

// WithFieldSelection enables `?fields=id,name` on list and read endpoints globally.
func WithFieldSelection() Option {
	return func(e *Extension) { e.fieldSelectionAll = true }
}

// WithCursorPagination enables cursor-based pagination globally instead of offset-based.
func WithCursorPagination() Option {
	return func(e *Extension) { e.cursorPagAll = true }
}

// WithPatchSemantics enables JSON merge patch behavior for update handlers globally.
func WithPatchSemantics() Option {
	return func(e *Extension) { e.patchSemanticsAll = true }
}

// WithEdgeExpansion enables `?expand=edge1,edge2.nested` on read and list endpoints globally.
func WithEdgeExpansion() Option {
	return func(e *Extension) { e.edgeExpansionAll = true }
}

// WithAutoExpand configures edges to be automatically expanded on read and list endpoints globally.
func WithAutoExpand(edges ...AutoExpandEdge) Option {
	return func(e *Extension) { e.autoExpandAll = edges }
}

// WithStripFields configures fields to be stripped from responses on read and list endpoints globally.
func WithStripFields(fields ...string) Option {
	return func(e *Extension) { e.stripFieldsAll = fields }
}

// WithResponseValidation enables generation of a test helper that validates responses
// against the OpenAPI spec. specPath is the runtime path to the spec file.
func WithResponseValidation(specPath string) Option {
	return func(e *Extension) { e.validationSpecPath = specPath }
}

// NewExtension creates a new entapi extension.
func NewExtension(spec *ogen.Spec, opts ...Option) (*Extension, error) {
	ext := &Extension{
		spec:      spec,
		outputDir: "internal/api",
		pkgName:   "api",
	}

	for _, opt := range opts {
		opt(ext)
	}

	return ext, nil
}

// Hooks returns entc hooks.
func (e *Extension) Hooks() []gen.Hook {
	return []gen.Hook{
		e.generateCode,
	}
}

// Annotations returns entc annotations.
func (e *Extension) Annotations() []entc.Annotation {
	return nil
}

// Templates returns templates to extend.
func (e *Extension) Templates() []*gen.Template {
	return nil
}

// Options returns entc options.
func (e *Extension) Options() []entc.Option {
	return nil
}

// filterableField describes a field that can be filtered on in a list endpoint.
type filterableField struct {
	Name        string // Ent field name (snake_case)
	FieldType   string // "string", "int", "float", "bool", "enum", "time", "uuid"
	StructField string // Go struct field name (PascalCase)
}

// getFilterableFields returns the list of fields that should be filterable for a given entity.
func (e *Extension) getFilterableFields(node *gen.Type) []filterableField {
	// Check per-entity annotation
	var ann *FilterFieldsAnnotation
	if raw, ok := node.Annotations["EntAPIFilterFields"]; ok && raw != nil {
		data, err := json.Marshal(raw)
		if err == nil {
			var a FilterFieldsAnnotation
			if json.Unmarshal(data, &a) == nil {
				ann = &a
			}
		}
	}

	// If no annotation and global filtering disabled, return empty
	if ann == nil && !e.fieldFiltering {
		return nil
	}

	includeSet := make(map[string]bool)
	if ann != nil {
		for _, name := range ann.Include {
			includeSet[name] = true
		}
	}

	excludeSet := make(map[string]bool)
	if ann != nil {
		for _, name := range ann.Exclude {
			excludeSet[name] = true
		}
	}

	var result []filterableField
	for _, f := range node.Fields {
		name := strings.ToLower(f.Name)

		// Skip read-only/system fields
		if name == "id" || name == "created_at" || name == "updated_at" {
			continue
		}

		// Check field-level NoFilter annotation
		if _, ok := f.Annotations["EntAPINoFilter"]; ok {
			continue
		}

		// Check if sensitive (skip unless AllowFilter)
		_, hasAllowFilter := f.Annotations["EntAPIAllowFilter"]
		if f.Sensitive() && !hasAllowFilter {
			continue
		}

		// Heuristic: skip fields with sensitive-sounding names unless AllowFilter
		lowerName := strings.ToLower(f.Name)
		if !hasAllowFilter {
			isSensitiveName := false
			for _, keyword := range []string{"password", "secret", "token", "hash"} {
				if strings.Contains(lowerName, keyword) {
					isSensitiveName = true
					break
				}
			}
			if isSensitiveName {
				continue
			}
		}

		// If Include list is set, only include whitelisted fields
		if len(includeSet) > 0 && !includeSet[f.Name] {
			continue
		}

		// If in Exclude list, skip
		if excludeSet[f.Name] {
			continue
		}

		// Skip slice/JSON fields (not easily filterable)
		if isSliceField(f) {
			continue
		}

		// Determine field type
		var fieldType string
		switch {
		case f.IsEnum():
			fieldType = "enum"
		case isTimeField(f):
			fieldType = "time"
		case isUUIDField(f):
			fieldType = "uuid"
		case isStringField(f):
			fieldType = "string"
		case f.Type.Type.String() == "bool":
			fieldType = "bool"
		case f.Type.Type.Float():
			fieldType = "float"
		case f.Type.Type.String() == "int64" || f.Type.Type.String() == "uint64":
			fieldType = "int64"
		case f.Type.Type.Integer():
			fieldType = "int"
		default:
			continue
		}

		result = append(result, filterableField{
			Name:        f.Name,
			FieldType:   fieldType,
			StructField: f.StructField(),
		})
	}

	return result
}

// addFilterQueryParams modifies the OpenAPI spec to add filter query params to list operations.
func (e *Extension) addFilterQueryParams(g *gen.Graph) {
	for _, node := range g.Nodes {
		fields := e.getFilterableFields(node)
		if len(fields) == 0 {
			continue
		}

		listOpID := "list" + node.Name

		for _, pathItem := range e.spec.Paths {
			if pathItem.Get == nil || pathItem.Get.OperationID != listOpID {
				continue
			}

			for _, f := range fields {
				switch f.FieldType {
				case "string":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name, ogen.String(), "Filter by exact "+f.Name),
						newQueryParam(f.Name+"_contains", ogen.String(), "Filter by "+f.Name+" (case-insensitive contains)"),
					)
				case "int64":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name, ogen.Int(), "Filter by exact "+f.Name),
						newQueryParam(f.Name+"_gte", ogen.Int(), "Filter by "+f.Name+" (>=)"),
						newQueryParam(f.Name+"_lte", ogen.Int(), "Filter by "+f.Name+" (<=)"),
					)
				case "int":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name, ogen.Int(), "Filter by exact "+f.Name),
						newQueryParam(f.Name+"_gte", ogen.Int(), "Filter by "+f.Name+" (>=)"),
						newQueryParam(f.Name+"_lte", ogen.Int(), "Filter by "+f.Name+" (<=)"),
					)
				case "float":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name, ogen.Double(), "Filter by exact "+f.Name),
						newQueryParam(f.Name+"_gte", ogen.Double(), "Filter by "+f.Name+" (>=)"),
						newQueryParam(f.Name+"_lte", ogen.Double(), "Filter by "+f.Name+" (<=)"),
					)
				case "bool":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name, ogen.Bool(), "Filter by "+f.Name),
					)
				case "enum":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name, ogen.String(), "Filter by "+f.Name),
					)
				case "time":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name+"_gte", ogen.String().SetFormat("date-time"), "Filter by "+f.Name+" (>=), RFC3339 format"),
						newQueryParam(f.Name+"_lte", ogen.String().SetFormat("date-time"), "Filter by "+f.Name+" (<=), RFC3339 format"),
					)
				case "uuid":
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam(f.Name, ogen.String().SetFormat("uuid"), "Filter by "+f.Name),
					)
				}
			}
		}
	}
}

func newQueryParam(name string, schema *ogen.Schema, desc string) *ogen.Parameter {
	return ogen.NewParameter().
		SetName(name).
		SetIn("query").
		SetDescription(desc).
		SetSchema(schema)
}

// hasFilterableFields returns true if the entity has any filterable fields.
func (e *Extension) hasFilterableFields(node *gen.Type) bool {
	return len(e.getFilterableFields(node)) > 0
}

// generateCode is the main hook that runs after Ent code generation.
func (e *Extension) generateCode(next gen.Generator) gen.Generator {
	return gen.GenerateFunc(func(g *gen.Graph) error {
		if err := next.Generate(g); err != nil {
			return err
		}

		// Auto-detect module path from graph config if not set
		if e.modulePath == "" {
			e.modulePath = g.Config.Package
		}

		// Mutate OpenAPI spec: add query params for filtering, sorting, field selection
		e.addFilterQueryParams(g)
		e.addSortQueryParams(g)
		e.addFieldSelectionParams(g)
		e.addCursorPaginationParams(g)
		e.addEdgeExpansionParams(g)

		specPath := filepath.Join(g.Config.Target, "openapi.json")
		if err := e.writeOpenAPISpec(specPath); err != nil {
			return fmt.Errorf("writing openapi spec: %w", err)
		}

		if err := e.generateOAPICodegen(g, specPath); err != nil {
			return fmt.Errorf("generating oapi-codegen code: %w", err)
		}

		if err := e.generateHandlers(g); err != nil {
			return fmt.Errorf("generating handlers: %w", err)
		}

		if err := e.generateConverters(g); err != nil {
			return fmt.Errorf("generating converters: %w", err)
		}

		if e.validationSpecPath != "" {
			if err := e.generateValidationHelper(g); err != nil {
				return fmt.Errorf("generating validation helper: %w", err)
			}
		}

		return nil
	})
}

func (e *Extension) writeOpenAPISpec(path string) error {
	data, err := json.MarshalIndent(e.spec, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

func (e *Extension) generateOAPICodegen(g *gen.Graph, specPath string) error {
	outputPath := filepath.Join(g.Config.Target, "..", e.outputDir, "server.gen.go")

	if err := os.MkdirAll(filepath.Dir(outputPath), 0755); err != nil {
		return err
	}

	swagger, err := util.LoadSwaggerWithOverlay(specPath, util.LoadSwaggerWithOverlayOpts{})
	if err != nil {
		return fmt.Errorf("loading OpenAPI spec: %w", err)
	}

	opts := codegen.Configuration{
		PackageName: e.pkgName,
		Generate: codegen.GenerateOptions{
			Models:        true,
			GorillaServer: true,
			Strict:        true,
		},
	}

	code, err := codegen.Generate(swagger, opts)
	if err != nil {
		return fmt.Errorf("generating code: %w", err)
	}

	if err := os.WriteFile(outputPath, []byte(code), 0644); err != nil {
		return fmt.Errorf("writing generated code: %w", err)
	}

	return nil
}

func (e *Extension) generateHandlers(g *gen.Graph) error {
	operations := []string{"list", "create", "read", "update", "delete"}

	entityOps := e.getEntityOperations(g)

	templates := make(map[string]*template.Template)
	for _, op := range operations {
		tmplPath := fmt.Sprintf("templates/handlers/%s.tmpl", op)
		data, err := templateFS.ReadFile(tmplPath)
		if err != nil {
			return fmt.Errorf("reading %s template: %w", op, err)
		}

		tmpl, err := template.New(op).Funcs(e.templateFuncs(g)).Parse(string(data))
		if err != nil {
			return fmt.Errorf("parsing %s template: %w", op, err)
		}
		templates[op] = tmpl
	}

	// Collect extra imports from before-handler hooks.
	extraImports := make(map[string]bool)
	autoExpandImports := make(map[string]string) // alias -> full path

	var buf bytes.Buffer
	buf.WriteString(fmt.Sprintf("// Code generated by entoas-server. DO NOT EDIT.\n\npackage %s\n\n", e.pkgName)) //nolint
	buf.WriteString("import (\n")
	buf.WriteString("\t\"context\"\n")
	buf.WriteString("\t\"encoding/json\"\n")
	buf.WriteString("\t\"fmt\"\n")
	buf.WriteString("\t\"net/http\"\n")
	buf.WriteString(fmt.Sprintf("\t%q\n", e.modulePath))
	buf.WriteString("\t\"github.com/google/uuid\"\n")
	// Pre-scan: run hooks on all entities to collect needed imports.
	for _, node := range g.Nodes {
		for _, op := range operations {
			nodeOps := entityOps[node.Name]
			if nodeOps == nil || !nodeOps[op] {
				continue
			}
			for _, hook := range e.beforeHandlerHooks {
				_, imports := hook(node.Name, op, node.Annotations)
				for _, imp := range imports {
					extraImports[imp] = true
				}
			}
			for _, hook := range e.afterHandlerHooks {
				_, imports := hook(node.Name, op, node.Annotations)
				for _, imp := range imports {
					extraImports[imp] = true
				}
			}
		}
	}
	for imp := range extraImports {
		if strings.Contains(imp, " ") {
			// Aliased import (e.g. `auth "github.com/tiagoposse/entauth"`)
			buf.WriteString(fmt.Sprintf("\t%s\n", imp))
		} else {
			buf.WriteString(fmt.Sprintf("\t%q\n", imp))
		}
	}
	// Conditional imports based on active features
	needsStrings := false
	needsBase64 := false
	needsTime := false
	for _, node := range g.Nodes {
		nodeOps := entityOps[node.Name]
		if nodeOps == nil {
			continue
		}
		if nodeOps["list"] && len(e.getSortableFields(node)) > 0 {
			needsStrings = true
		}
		if e.resolveFieldSelection(node) {
			needsStrings = true
		}
		if len(e.getExpandableEdges(node)) > 0 {
			needsStrings = true
		}
		if e.resolveCursorPagination(node) && nodeOps["list"] {
			needsBase64 = true
		}
		if e.resolveSoftDelete(node) {
			needsTime = true
		}
		if e.resolvePatchSemantics(node) {
			needsStrings = true
		}
		// Check if strip-fields with self-check needs auth import
		for _, op := range operations {
			nodeOps2 := entityOps[node.Name]
			if nodeOps2 != nil && nodeOps2[op] {
				if sf := e.resolveStripFields(node, op); sf != nil && sf.SelfCheck {
					// The auth import is expected to come from the BeforeHandlerHook (auth guard).
					// If not using auth guards, the user must provide the auth import via a hook.
					break
				}
			}
		}
	}
	if needsStrings {
		buf.WriteString("\t\"strings\"\n")
	}
	if needsBase64 {
		buf.WriteString("\t\"encoding/base64\"\n")
	}
	if needsTime {
		buf.WriteString("\t\"time\"\n")
	}
	// Check if any node has JSON custom type fields that need schema import
	needsSchema := false
	for _, node := range g.Nodes {
		nodeOps := entityOps[node.Name]
		if nodeOps == nil || (!nodeOps["create"] && !nodeOps["update"]) {
			continue
		}
		for _, f := range node.Fields {
			if isJSONCustomType(f) && !isReadOnlyField(f) {
				needsSchema = true
				break
			}
		}
		if needsSchema {
			break
		}
	}
	if needsSchema {
		schemaPath := e.modulePath + "/schema"
		buf.WriteString(fmt.Sprintf("\tentschema %q\n", schemaPath))
	}
	for _, node := range g.Nodes {
		nodeOps := entityOps[node.Name]
		if len(nodeOps) == 0 {
			continue
		}
		needsImport := false
		// Import for enum fields on create/update
		if hasEnumFields(node) && (nodeOps["create"] || nodeOps["update"]) {
			needsImport = true
		}
		// Import for filter fields on list
		if nodeOps["list"] && e.hasFilterableFields(node) {
			needsImport = true
		}
		// Import for read query predicate
		if nodeOps["read"] {
			needsImport = true
		}
		// Import for soft-delete, sorting, field selection, cursor pagination
		if e.resolveSoftDelete(node) || len(e.getSortableFields(node)) > 0 || e.resolveCursorPagination(node) {
			needsImport = true
		}
		if needsImport {
			buf.WriteString(fmt.Sprintf("\t%s %q\n", strings.ToLower(node.Name), e.modulePath+"/"+strings.ToLower(node.Name)))
		}

		// Import target entity packages for auto-expand edges with field selection
		for _, op := range operations {
			if nodeOps == nil || !nodeOps[op] {
				continue
			}
			for _, ae := range e.resolveAutoExpand(node, op) {
				if len(ae.Fields) > 0 {
					pkg := strings.ToLower(ae.TypeName)
					autoExpandImports[pkg] = e.modulePath + "/" + pkg
				}
			}
		}
	}
	for alias, path := range autoExpandImports {
		// Skip if already imported above
		alreadyImported := false
		for _, node := range g.Nodes {
			if strings.ToLower(node.Name) == alias && len(entityOps[node.Name]) > 0 {
				alreadyImported = true
				break
			}
		}
		if !alreadyImported {
			buf.WriteString(fmt.Sprintf("\t%s %q\n", alias, path))
		}
	}
	buf.WriteString(")\n\n")

	buf.WriteString("// EntHandler implements the StrictServerInterface with Ent-backed operations.\n")
	buf.WriteString("type EntHandler struct {\n")
	buf.WriteString("\tclient *ent.Client\n")
	buf.WriteString("}\n\n")

	buf.WriteString("// NewEntHandler creates a new Ent-backed handler.\n")
	buf.WriteString("func NewEntHandler(client *ent.Client) *EntHandler {\n")
	buf.WriteString("\treturn &EntHandler{client: client}\n")
	buf.WriteString("}\n\n")

	// Generate override function variables
	for _, node := range g.Nodes {
		nodeOps, exists := entityOps[node.Name]
		if !exists {
			continue
		}

		for _, op := range operations {
			if !nodeOps[op] {
				continue
			}

			funcName := fmt.Sprintf("custom%s%s", node.Name, capitalize(op))
			switch op {
			case "list":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, List%sRequestObject) (List%sResponseObject, error)\n", funcName, node.Name, node.Name))
			case "create":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, Create%sRequestObject) (Create%sResponseObject, error)\n", funcName, node.Name, node.Name))
			case "read":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, Read%sRequestObject) (Read%sResponseObject, error)\n", funcName, node.Name, node.Name))
			case "update":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, Update%sRequestObject) (Update%sResponseObject, error)\n", funcName, node.Name, node.Name))
			case "delete":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, Delete%sRequestObject) (Delete%sResponseObject, error)\n", funcName, node.Name, node.Name))
			}

			beforeFunc := fmt.Sprintf("before%s%s", node.Name, capitalize(op))
			afterFunc := fmt.Sprintf("after%s%s", node.Name, capitalize(op))

			switch op {
			case "list":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, **ent.%sQuery) error\n", beforeFunc, node.Name))
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, []*ent.%s) ([]*ent.%s, error)\n", afterFunc, node.Name, node.Name))
			case "create":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, *ent.%sCreate) error\n", beforeFunc, node.Name))
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, *ent.%s) error\n", afterFunc, node.Name))
			case "read":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, **ent.%sQuery) error\n", beforeFunc, node.Name))
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, *ent.%s) error\n", afterFunc, node.Name))
			case "update":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, *ent.%sUpdateOne) error\n", beforeFunc, node.Name))
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, *ent.%s) error\n", afterFunc, node.Name))
			case "delete":
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, uuid.UUID) error\n", beforeFunc))
				buf.WriteString(fmt.Sprintf("var %s func(context.Context, *ent.Client, uuid.UUID) error\n", afterFunc))
			}
		}
	}
	buf.WriteString("\n")

	// Generate handlers
	for _, node := range g.Nodes {
		nodeOps, exists := entityOps[node.Name]
		if !exists {
			continue
		}

		for _, op := range operations {
			if !nodeOps[op] {
				continue
			}

			// Run before-handler hooks to collect injected code.
			var beforeCode strings.Builder
			for _, hook := range e.beforeHandlerHooks {
				code, _ := hook(node.Name, op, node.Annotations)
				if code != "" {
					beforeCode.WriteString(code)
					beforeCode.WriteString("\n")
				}
			}
			guardCode := beforeCode.String()

			// Run after-handler hooks to collect injected code.
			var afterCode strings.Builder
			for _, hook := range e.afterHandlerHooks {
				code, _ := hook(node.Name, op, node.Annotations)
				if code != "" {
					afterCode.WriteString(code)
					afterCode.WriteString("\n")
				}
			}
			afterHookCode := afterCode.String()

			overridePath := fmt.Sprintf("templates/handlers/%s_%s.tmpl", strings.ToLower(node.Name), op)
			customData, err := templateFS.ReadFile(overridePath)

			var tmpl *template.Template
			if err == nil {
				tmpl, err = template.New(op).Funcs(e.templateFuncs(g)).Parse(string(customData))
				if err != nil {
					return fmt.Errorf("parsing custom template %s: %w", overridePath, err)
				}
			} else {
				tmpl = templates[op]
			}

			// Resolve per-entity feature flags
			paginated := false
			pageSize := 30
			var filterFields []filterableField
			var sortFields []sortableField
			cursorPag := false
			if op == "list" {
				paginated, pageSize = e.resolvePagination(node)
				filterFields = e.getFilterableFields(node)
				sortFields = e.getSortableFields(node)
				cursorPag = e.resolveCursorPagination(node)
			}
			softDelete := e.resolveSoftDelete(node)
			fieldSelection := e.resolveFieldSelection(node)
			patchSemantics := e.resolvePatchSemantics(node)
			defaultSort := e.resolveDefaultSort(node)
			expandableEdges := e.getExpandableEdges(node)
			autoExpand := e.resolveAutoExpand(node, op)
			stripFields := e.resolveStripFields(node, op)

			if err := tmpl.ExecuteTemplate(&buf, op, struct {
				Node            *gen.Type
				Package         string
				GuardCode       string
				AfterHookCode   string
				Paginated       bool
				DefaultPageSize int
				FilterFields    []filterableField
				SortFields      []sortableField
				DefaultSort     string
				SoftDelete      bool
				FieldSelection  bool
				CursorPag       bool
				PatchSemantics  bool
				EdgeExpansion   bool
				Edges           []edgeInfo
				AutoExpand      []autoExpandInfo
				StripFields     *stripFieldsConfig
			}{
				Node:            node,
				Package:         e.pkgName,
				GuardCode:       guardCode,
				AfterHookCode:   afterHookCode,
				Paginated:       paginated,
				DefaultPageSize: pageSize,
				FilterFields:    filterFields,
				SortFields:      sortFields,
				DefaultSort:     defaultSort,
				SoftDelete:      softDelete,
				FieldSelection:  fieldSelection,
				CursorPag:       cursorPag,
				PatchSemantics:  patchSemantics,
				EdgeExpansion:   len(expandableEdges) > 0,
				Edges:           expandableEdges,
				AutoExpand:      autoExpand,
				StripFields:     stripFields,
			}); err != nil {
				return fmt.Errorf("executing %s template for %s: %w", op, node.Name, err)
			}
			buf.WriteString("\n")
		}
	}

	buf.WriteString(e.generateRemainingOperationStubs(g, entityOps))

	// Generate strip-fields helper functions (once per entity)
	generatedStrip := make(map[string]bool)
	for _, node := range g.Nodes {
		if generatedStrip[node.Name] {
			continue
		}
		// Check if any operation uses strip-fields
		for _, op := range operations {
			nodeOps := entityOps[node.Name]
			if nodeOps == nil || !nodeOps[op] {
				continue
			}
			sf := e.resolveStripFields(node, op)
			if sf == nil {
				continue
			}
			generatedStrip[node.Name] = true
			buf.WriteString(fmt.Sprintf("\nfunc strip%sFields(e *%s) {\n", node.Name, node.Name))
			for _, f := range sf.Fields {
				buf.WriteString(fmt.Sprintf("\te.%s = nil\n", f.StructField))
			}
			buf.WriteString("}\n")
			break
		}
	}

	outputPath := filepath.Join(g.Config.Target, "..", e.outputDir, "handlers.gen.go")
	if err := os.WriteFile(outputPath, buf.Bytes(), 0644); err != nil {
		return fmt.Errorf("writing handlers file: %w", err)
	}

	return nil
}

func (e *Extension) generateRemainingOperationStubs(g *gen.Graph, entityOps map[string]map[string]bool) string {
	var buf bytes.Buffer

	allOps := make(map[string]bool)
	for _, pathItem := range e.spec.Paths {
		ops := []*ogen.Operation{
			pathItem.Get,
			pathItem.Post,
			pathItem.Put,
			pathItem.Patch,
			pathItem.Delete,
		}

		for _, op := range ops {
			if op != nil && op.OperationID != "" {
				allOps[op.OperationID] = true
			}
		}
	}

	for _, node := range g.Nodes {
		nodeOps, exists := entityOps[node.Name]
		if !exists {
			continue
		}

		for op := range nodeOps {
			opID := ""
			switch op {
			case "list":
				opID = fmt.Sprintf("list%s", node.Name)
			case "create":
				opID = fmt.Sprintf("create%s", node.Name)
			case "read":
				opID = fmt.Sprintf("read%s", node.Name)
			case "update":
				opID = fmt.Sprintf("update%s", node.Name)
			case "delete":
				opID = fmt.Sprintf("delete%s", node.Name)
			}
			delete(allOps, opID)
		}
	}

	if len(allOps) == 0 {
		return ""
	}

	buf.WriteString("// Stub handlers for custom operations.\n\n")

	for opID := range allOps {
		pascalOpID := toPascalCase(opID)
		buf.WriteString(fmt.Sprintf("var custom%s func(context.Context, *ent.Client, %sRequestObject) (%sResponseObject, error)\n", pascalOpID, pascalOpID, pascalOpID))
	}
	buf.WriteString("\n")

	for opID := range allOps {
		pascalOpID := toPascalCase(opID)
		buf.WriteString(fmt.Sprintf("func (h *EntHandler) %s(ctx context.Context, request %sRequestObject) (%sResponseObject, error) {\n", pascalOpID, pascalOpID, pascalOpID))
		buf.WriteString(fmt.Sprintf("\tif custom%s != nil {\n", pascalOpID))
		buf.WriteString(fmt.Sprintf("\t\treturn custom%s(ctx, h.client, request)\n", pascalOpID))
		buf.WriteString("\t}\n")
		buf.WriteString(fmt.Sprintf("\treturn nil, fmt.Errorf(\"%s not implemented\")\n", opID))
		buf.WriteString("}\n\n")
	}

	return buf.String()
}

func (e *Extension) getEntityOperations(g *gen.Graph) map[string]map[string]bool {
	result := make(map[string]map[string]bool)

	for path, pathItem := range e.spec.Paths {
		entityName, isEdgePath := e.entityNameFromPath(path)
		if entityName == "" || isEdgePath {
			continue
		}

		var matchedNode *gen.Type
		for _, node := range g.Nodes {
			if strings.EqualFold(plural(node.Name), entityName) {
				matchedNode = node
				break
			}
		}
		if matchedNode == nil {
			continue
		}

		if result[matchedNode.Name] == nil {
			result[matchedNode.Name] = make(map[string]bool)
		}

		// Only map operations whose operation IDs follow the CRUD naming convention.
		// This prevents custom endpoints (e.g. ListMyNotifications at /notifications)
		// from being confused with entity CRUD operations (listNotification).
		if pathItem.Get != nil {
			if strings.Contains(path, "{id}") {
				if pathItem.Get.OperationID == "read"+matchedNode.Name {
					result[matchedNode.Name]["read"] = true
				}
			} else {
				if pathItem.Get.OperationID == "list"+matchedNode.Name {
					result[matchedNode.Name]["list"] = true
				}
			}
		}
		if pathItem.Post != nil {
			if pathItem.Post.OperationID == "create"+matchedNode.Name {
				result[matchedNode.Name]["create"] = true
			}
		}
		if pathItem.Put != nil && pathItem.Put.OperationID == "update"+matchedNode.Name {
			result[matchedNode.Name]["update"] = true
		}
		if pathItem.Patch != nil && pathItem.Patch.OperationID == "update"+matchedNode.Name {
			result[matchedNode.Name]["update"] = true
		}
		if pathItem.Delete != nil {
			if pathItem.Delete.OperationID == "delete"+matchedNode.Name {
				result[matchedNode.Name]["delete"] = true
			}
		}
	}

	return result
}

func (e *Extension) entityNameFromPath(path string) (string, bool) {
	path = strings.TrimPrefix(path, "/")
	parts := strings.Split(path, "/")
	if len(parts) == 0 {
		return "", false
	}

	if len(parts) > 2 {
		return "", true
	}

	// Two-part paths are only entity CRUD if the second part is {id}
	if len(parts) == 2 && parts[1] != "{id}" {
		return "", true
	}

	entityName := parts[0]
	entityName = strings.ReplaceAll(entityName, "-", "")

	return entityName, false
}

func capitalize(s string) string {
	if s == "" {
		return ""
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

func (e *Extension) generateConverters(g *gen.Graph) error {
	data, err := templateFS.ReadFile("templates/converters.tmpl")
	if err != nil {
		return fmt.Errorf("reading converters template: %w", err)
	}

	tmpl, err := template.New("converters").
		Funcs(e.templateFuncs(g)).
		Parse(string(data))
	if err != nil {
		return fmt.Errorf("parsing converters template: %w", err)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, map[string]interface{}{
		"Graph":   g,
		"Package": e.pkgName,
	}); err != nil {
		return fmt.Errorf("executing converters template: %w", err)
	}

	outputPath := filepath.Join(g.Config.Target, "..", e.outputDir, "converters.gen.go")
	return os.WriteFile(outputPath, buf.Bytes(), 0644)
}

func (e *Extension) templateFuncs(g *gen.Graph) template.FuncMap {
	return template.FuncMap{
		"pascal":        toPascalCase,
		"camel":         toCamelCase,
		"lower":         strings.ToLower,
		"upper":         strings.ToUpper,
		"plural":        plural,
		"hasOptional":   hasOptionalFields,
		"hasEnum":       hasEnumFields,
		"hasUUIDFields": hasUUIDFieldsInGraph,
		"isTime":        isTimeField,
		"isUUID":        isUUIDField,
		"isEnum":        isEnumField,
		"isString":      isStringField,
		"isSlice":       isSliceField,
		"isJSONCustom":  isJSONCustomType,
		"jsonGoType":    jsonFieldGoType,
		"isReadOnly":    isReadOnlyField,
		"isSensitive":   isSensitiveField,
		"isOptionalAPI": isOptionalAPIField,
		"oapiFieldName": oapiFieldName,
		"snakeToPascal": snakeToPascal,
		"split":         strings.Split,
		"index": func(arr []string, i int) string {
			if i < len(arr) {
				return arr[i]
			}
			return ""
		},
	}
}

func toPascalCase(s string) string {
	return strings.ToUpper(s[:1]) + s[1:]
}

func toCamelCase(s string) string {
	return strings.ToLower(s[:1]) + s[1:]
}

func plural(s string) string {
	lower := strings.ToLower(s)
	// Handle irregular plurals
	irregulars := map[string]string{
		"match":     "matches",
		"blacklist": "blacklists",
	}
	if p, ok := irregulars[lower]; ok {
		// Preserve original casing of first char
		if s[0] >= 'A' && s[0] <= 'Z' {
			return strings.ToUpper(p[:1]) + p[1:]
		}
		return p
	}

	if strings.HasSuffix(s, "s") || strings.HasSuffix(s, "sh") || strings.HasSuffix(s, "ch") || strings.HasSuffix(s, "x") || strings.HasSuffix(s, "z") {
		return s + "es"
	}
	if strings.HasSuffix(s, "y") && !strings.HasSuffix(s, "ey") && !strings.HasSuffix(s, "ay") && !strings.HasSuffix(s, "oy") && !strings.HasSuffix(s, "uy") {
		return s[:len(s)-1] + "ies"
	}
	return s + "s"
}

func hasOptionalFields(t *gen.Type) bool {
	for _, f := range t.Fields {
		if f.Optional {
			return true
		}
	}
	return false
}

func hasEnumFields(t *gen.Type) bool {
	for _, f := range t.Fields {
		if f.IsEnum() {
			return true
		}
	}
	return false
}

func hasUUIDFieldsInGraph(g *gen.Graph) bool {
	for _, node := range g.Nodes {
		for _, f := range node.Fields {
			if isUUIDField(f) {
				return true
			}
		}
	}
	return false
}

func isTimeField(f *gen.Field) bool {
	return f.Type.Type.String() == "time.Time"
}

func isUUIDField(f *gen.Field) bool {
	s := f.Type.Type.String()
	return s == "uuid.UUID" || s == "[16]byte"
}

func isEnumField(f *gen.Field) bool {
	return f.IsEnum()
}

func isStringField(f *gen.Field) bool {
	return f.Type.Type.String() == "string"
}

func isSliceField(f *gen.Field) bool {
	typeStr := f.Type.Type.String()
	if strings.HasPrefix(typeStr, "[]") {
		return true
	}
	if f.Type.RType != nil && f.Type.RType.Kind == 23 {
		return true
	}
	return false
}

// isJSONCustomType returns true for JSON fields with custom struct element types
// (e.g. []schema.GroupLocation or schema.PrivacySettings) that can't be directly assigned from generated request types.
func isJSONCustomType(f *gen.Field) bool {
	typeStr := f.Type.Type.String()
	// Check if it's a JSON field type
	if typeStr == "JSON" || (f.Type.RType != nil && f.Type.RType.Kind == 25) {
		// Kind 25 = struct
		return true
	}
	if !isSliceField(f) {
		return false
	}
	// Primitive slices are fine — direct assignment works
	switch typeStr {
	case "[]string", "[]int", "[]int64", "[]float64", "[]bool":
		return false
	}
	return true
}

// jsonFieldGoType returns the Go type string for a JSON field, using the "entschema" import alias.
// e.g. "[]schema.GroupLocation" -> "[]entschema.GroupLocation"
func jsonFieldGoType(f *gen.Field) string {
	typeStr := f.Type.Type.String()
	if f.Type.RType != nil {
		typeStr = f.Type.RType.String()
	}
	// Replace the package prefix with the entschema alias
	// RType.String() returns e.g. "[]schema.GroupLocation"
	if strings.Contains(typeStr, ".") {
		// Extract "[]" prefix if present
		prefix := ""
		inner := typeStr
		if strings.HasPrefix(typeStr, "[]") {
			prefix = "[]"
			inner = typeStr[2:]
		}
		// Replace package name with entschema alias
		if dotIdx := strings.LastIndex(inner, "."); dotIdx >= 0 {
			typeName := inner[dotIdx+1:]
			return prefix + "entschema." + typeName
		}
	}
	return typeStr
}

func isReadOnlyField(f *gen.Field) bool {
	ant, err := entoas.FieldAnnotation(f)
	if err != nil {
		return false
	}
	return ant.ReadOnly
}

func isSensitiveField(f *gen.Field) bool {
	return f.Sensitive()
}

// isOptionalAPIField returns true if the oapi-codegen generated field will be a pointer.
// Required non-nillable non-UUID fields produce non-pointer types in oapi-codegen output.
func isOptionalAPIField(f *gen.Field) bool {
	return f.Optional || f.Nillable
}

// snakeToPascal converts a snake_case string to PascalCase, matching oapi-codegen's ToCamelCase behavior.
func snakeToPascal(s string) string {
	parts := strings.Split(s, "_")
	var result strings.Builder
	for _, part := range parts {
		if part == "" {
			continue
		}
		result.WriteString(strings.ToUpper(part[:1]) + part[1:])
	}
	return result.String()
}

func oapiFieldName(entFieldName string) string {
	acronyms := map[string]string{
		"ID":   "Id",
		"IP":   "Ip",
		"HTTP": "Http",
		"URL":  "Url",
		"API":  "Api",
		"JSON": "Json",
		"XML":  "Xml",
		"HTML": "Html",
		"SQL":  "Sql",
		"SSH":  "Ssh",
		"TLS":  "Tls",
		"TCP":  "Tcp",
		"UDP":  "Udp",
	}

	result := entFieldName
	for acronym, replacement := range acronyms {
		if strings.HasSuffix(result, acronym) {
			result = result[:len(result)-len(acronym)] + replacement
		} else {
			result = strings.ReplaceAll(result, acronym, replacement)
		}
	}
	return result
}

// resolvePagination determines if a node's list handler should return paginated responses
// and what the default page size should be. Per-entity annotations override global config.
func (e *Extension) resolvePagination(node *gen.Type) (enabled bool, pageSize int) {
	pageSize = 30
	if e.defaultPageSize > 0 {
		pageSize = e.defaultPageSize
	}

	// Check per-entity annotation
	if raw, ok := node.Annotations["EntAPIPagination"]; ok && raw != nil {
		data, err := json.Marshal(raw)
		if err == nil {
			var ann PaginationAnnotation
			if json.Unmarshal(data, &ann) == nil {
				if ann.DefaultPageSize > 0 {
					pageSize = ann.DefaultPageSize
				}
				return ann.Enabled, pageSize
			}
		}
	}

	// Fall back to global setting
	return e.paginateAll, pageSize
}

// --- Sortable fields ---

type sortableField struct {
	Name        string // snake_case field name
	StructField string // PascalCase Go struct field name
}

func (e *Extension) getSortableFields(node *gen.Type) []sortableField {
	var ann *SortFieldsAnnotation
	if raw, ok := node.Annotations["EntAPISortFields"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		var a SortFieldsAnnotation
		if json.Unmarshal(data, &a) == nil {
			ann = &a
		}
	}
	if ann == nil && !e.sortingAll {
		return nil
	}

	includeSet := make(map[string]bool)
	if ann != nil {
		for _, name := range ann.Include {
			includeSet[name] = true
		}
	}
	excludeSet := make(map[string]bool)
	if ann != nil {
		for _, name := range ann.Exclude {
			excludeSet[name] = true
		}
	}

	var result []sortableField
	// Build a map of field names to their StructField for quick lookup
	fieldMap := make(map[string]string)
	for _, f := range node.Fields {
		fieldMap[f.Name] = f.StructField()
	}
	// Include id, created_at, updated_at only if they exist on the entity
	for _, name := range []string{"id", "created_at", "updated_at"} {
		if sf, ok := fieldMap[name]; ok && !excludeSet[name] {
			result = append(result, sortableField{Name: name, StructField: sf})
		}
	}
	// id is always the ID field from Ent (special case - not in Fields)
	if !excludeSet["id"] {
		found := false
		for _, r := range result {
			if r.Name == "id" {
				found = true
				break
			}
		}
		if !found {
			result = append(result, sortableField{Name: "id", StructField: "ID"})
		}
	}
	for _, f := range node.Fields {
		name := f.Name
		if name == "id" || name == "created_at" || name == "updated_at" {
			continue
		}
		if f.Sensitive() {
			continue
		}
		if len(includeSet) > 0 && !includeSet[name] {
			continue
		}
		if excludeSet[name] {
			continue
		}
		if isSliceField(f) {
			continue
		}
		result = append(result, sortableField{Name: name, StructField: f.StructField()})
	}
	return result
}

func (e *Extension) resolveDefaultSort(node *gen.Type) string {
	if raw, ok := node.Annotations["EntAPISortFields"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		var ann SortFieldsAnnotation
		if json.Unmarshal(data, &ann) == nil && ann.DefaultSort != "" {
			return ann.DefaultSort
		}
	}
	return e.defaultSort
}

func (e *Extension) addSortQueryParams(g *gen.Graph) {
	for _, node := range g.Nodes {
		fields := e.getSortableFields(node)
		if len(fields) == 0 {
			continue
		}
		listOpID := "list" + node.Name
		for _, pathItem := range e.spec.Paths {
			if pathItem.Get != nil && pathItem.Get.OperationID == listOpID {
				names := make([]string, len(fields))
				for i, f := range fields {
					names[i] = f.Name
				}
				pathItem.Get.Parameters = append(pathItem.Get.Parameters,
					newQueryParam("sort", ogen.String(), "Sort by fields: "+strings.Join(names, ",")+". Format: field:asc or field:desc, comma-separated."),
				)
			}
		}
	}
}

// --- Soft delete resolution ---

func (e *Extension) resolveSoftDelete(node *gen.Type) bool {
	if raw, ok := node.Annotations["EntAPISoftDelete"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		var ann SoftDeleteAnnotation
		if json.Unmarshal(data, &ann) == nil {
			return ann.Enabled
		}
	}
	return e.softDeleteAll
}

// --- Field selection resolution ---

func (e *Extension) resolveFieldSelection(node *gen.Type) bool {
	if raw, ok := node.Annotations["EntAPIFieldSelection"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		var ann FieldSelectionAnnotation
		if json.Unmarshal(data, &ann) == nil {
			return ann.Enabled
		}
	}
	return e.fieldSelectionAll
}

func (e *Extension) addFieldSelectionParams(g *gen.Graph) {
	for _, node := range g.Nodes {
		if !e.resolveFieldSelection(node) {
			continue
		}
		for _, opID := range []string{"list" + node.Name, "read" + node.Name} {
			for _, pathItem := range e.spec.Paths {
				if pathItem.Get != nil && pathItem.Get.OperationID == opID {
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam("fields", ogen.String(), "Comma-separated list of fields to include in response (e.g. id,name,status)"),
					)
				}
			}
		}
	}
}

// --- Cursor pagination resolution ---

func (e *Extension) resolveCursorPagination(node *gen.Type) bool {
	if raw, ok := node.Annotations["EntAPICursorPagination"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		var ann CursorPaginationAnnotation
		if json.Unmarshal(data, &ann) == nil {
			return ann.Enabled
		}
	}
	return e.cursorPagAll
}

func (e *Extension) addCursorPaginationParams(g *gen.Graph) {
	for _, node := range g.Nodes {
		if !e.resolveCursorPagination(node) {
			continue
		}
		listOpID := "list" + node.Name
		for _, pathItem := range e.spec.Paths {
			if pathItem.Get != nil && pathItem.Get.OperationID == listOpID {
				pathItem.Get.Parameters = append(pathItem.Get.Parameters,
					newQueryParam("cursor", ogen.String(), "Cursor for pagination (base64-encoded last item ID)"),
					newQueryParam("limit", ogen.Int(), "Maximum number of items to return"),
				)
			}
		}
	}
}

// --- Patch semantics resolution ---

func (e *Extension) resolvePatchSemantics(node *gen.Type) bool {
	if raw, ok := node.Annotations["EntAPIPatchSemantics"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		var ann PatchSemanticsAnnotation
		if json.Unmarshal(data, &ann) == nil {
			return ann.Enabled
		}
	}
	return e.patchSemanticsAll
}

// --- Edge expansion resolution ---

// edgeInfo holds metadata about an entity's edge for use in templates.
type edgeInfo struct {
	Name        string           // Edge name as defined in schema (e.g. "teams")
	StructField string           // Go struct field name (e.g. "Teams")
	TypeName    string           // Target entity type name (e.g. "TournamentTeam")
	Unique      bool             // Whether this is a unique (belongs-to) edge
	NestedEdges []nestedEdgeInfo // Sub-edges of the target type (for dot notation expansion)
}

func (e *Extension) resolveEdgeExpansion(node *gen.Type) (bool, []string) {
	if raw, ok := node.Annotations["EntAPIEdgeExpansion"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		var ann EdgeExpansionAnnotation
		if json.Unmarshal(data, &ann) == nil && ann.Enabled {
			return true, ann.AllowedEdges
		}
	}
	if e.edgeExpansionAll {
		return true, nil
	}
	return false, nil
}

func (e *Extension) getExpandableEdges(node *gen.Type) []edgeInfo {
	enabled, allowedEdges := e.resolveEdgeExpansion(node)
	if !enabled {
		return nil
	}

	allowedSet := make(map[string]bool)
	for _, edge := range allowedEdges {
		// For "teams.players", only the top-level "teams" is an edge of this entity
		parts := strings.SplitN(edge, ".", 2)
		allowedSet[parts[0]] = true
	}

	// Build set of allowed nested edges (e.g. "teams" -> ["players"])
	allowedNested := make(map[string]map[string]bool)
	for _, ae := range allowedEdges {
		parts := strings.SplitN(ae, ".", 2)
		if len(parts) == 2 {
			if allowedNested[parts[0]] == nil {
				allowedNested[parts[0]] = make(map[string]bool)
			}
			allowedNested[parts[0]][parts[1]] = true
		}
	}

	var edges []edgeInfo
	for _, edge := range node.Edges {
		// Skip edges that are excluded from the API
		if ann, ok := edge.Annotations["EntOAS"]; ok {
			data, _ := json.Marshal(ann)
			var oasAnn struct {
				Skip bool `json:"Skip"`
			}
			if json.Unmarshal(data, &oasAnn) == nil && oasAnn.Skip {
				continue
			}
		}

		if len(allowedSet) > 0 && !allowedSet[edge.Name] {
			continue
		}

		// Collect nested edges (sub-edges of the target type)
		var nested []nestedEdgeInfo
		for _, subEdge := range edge.Type.Edges {
			// If there's an allowlist for this edge's nested, filter by it
			if nestedAllowed, ok := allowedNested[edge.Name]; ok && len(nestedAllowed) > 0 {
				if !nestedAllowed[subEdge.Name] {
					continue
				}
			}
			nested = append(nested, nestedEdgeInfo{
				Name:        subEdge.Name,
				StructField: subEdge.StructField(),
				TypeName:    subEdge.Type.Name,
			})
		}

		edges = append(edges, edgeInfo{
			Name:        edge.Name,
			StructField: edge.StructField(),
			TypeName:    edge.Type.Name,
			Unique:      edge.Unique,
			NestedEdges: nested,
		})
	}
	return edges
}

// --- Auto-expand resolution ---

// autoExpandInfo holds resolved auto-expand config for a single edge.
type autoExpandInfo struct {
	Name        string              // Edge name
	StructField string              // Go struct field (e.g. "User")
	TypeName    string              // Target entity type (e.g. "User")
	Fields      []autoExpandField   // Field selection (empty = all fields)
}

// autoExpandField holds a resolved field constant for auto-expand.
type autoExpandField struct {
	EntConst string // Ent field constant (e.g. "FieldID", "FieldDisplayName")
}

func (e *Extension) resolveAutoExpand(node *gen.Type, operation string) []autoExpandInfo {
	var ann AutoExpandAnnotation

	if raw, ok := node.Annotations["EntAPIAutoExpand"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		json.Unmarshal(data, &ann)
	} else if len(e.autoExpandAll) > 0 {
		ann.Edges = e.autoExpandAll
	} else {
		return nil
	}

	// Check operation filter
	if len(ann.Operations) > 0 {
		found := false
		for _, op := range ann.Operations {
			if op == operation {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	// Map edge names to actual edge metadata
	edgeMap := make(map[string]*gen.Edge)
	for _, edge := range node.Edges {
		edgeMap[edge.Name] = edge
	}

	var result []autoExpandInfo
	for _, ae := range ann.Edges {
		edge, ok := edgeMap[ae.Name]
		if !ok {
			continue
		}

		// Resolve field names to Ent field constants
		var fields []autoExpandField
		if len(ae.Fields) > 0 {
			// Build field name -> StructField map for the target entity
			targetFieldMap := make(map[string]string)
			targetFieldMap["id"] = "ID" // ID is always available
			for _, f := range edge.Type.Fields {
				targetFieldMap[f.Name] = f.StructField()
			}

			for _, fname := range ae.Fields {
				sf, ok := targetFieldMap[fname]
				if !ok {
					continue
				}
				fields = append(fields, autoExpandField{EntConst: "Field" + sf})
			}
		}

		result = append(result, autoExpandInfo{
			Name:        ae.Name,
			StructField: edge.StructField(),
			TypeName:    edge.Type.Name,
			Fields:      fields,
		})
	}
	return result
}

// --- Strip-fields resolution ---

// stripFieldInfo holds resolved config for a field to strip from responses.
type stripFieldInfo struct {
	JSONName    string // JSON field name (e.g. "email")
	StructField string // Go struct field name (e.g. "Email")
}

// stripFieldsConfig holds the full strip-fields config for a handler.
type stripFieldsConfig struct {
	Fields    []stripFieldInfo
	SelfCheck bool
}

func (e *Extension) resolveStripFields(node *gen.Type, operation string) *stripFieldsConfig {
	var ann StripFieldsAnnotation

	if raw, ok := node.Annotations["EntAPIStripFields"]; ok && raw != nil {
		data, _ := json.Marshal(raw)
		json.Unmarshal(data, &ann)
	} else if len(e.stripFieldsAll) > 0 {
		ann.Fields = e.stripFieldsAll
	} else {
		return nil
	}

	if len(ann.Fields) == 0 {
		return nil
	}

	// Check operation filter
	if len(ann.Operations) > 0 {
		found := false
		for _, op := range ann.Operations {
			if op == operation {
				found = true
				break
			}
		}
		if !found {
			return nil
		}
	}

	// Map JSON field names to struct field names
	fieldMap := make(map[string]string)
	for _, f := range node.Fields {
		fieldMap[f.Name] = oapiFieldName(f.StructField())
	}

	var fields []stripFieldInfo
	for _, name := range ann.Fields {
		sf, ok := fieldMap[name]
		if !ok {
			continue
		}
		fields = append(fields, stripFieldInfo{
			JSONName:    name,
			StructField: sf,
		})
	}

	if len(fields) == 0 {
		return nil
	}

	return &stripFieldsConfig{
		Fields:    fields,
		SelfCheck: ann.SelfCheck,
	}
}

func (e *Extension) addEdgeExpansionParams(g *gen.Graph) {
	for _, node := range g.Nodes {
		edges := e.getExpandableEdges(node)
		if len(edges) == 0 {
			continue
		}
		edgeNames := make([]string, len(edges))
		for i, edge := range edges {
			edgeNames[i] = edge.Name
		}
		desc := fmt.Sprintf("Comma-separated edges to expand (e.g. %s). Supports dot notation for nested edges.", strings.Join(edgeNames, ","))

		for _, opID := range []string{"list" + node.Name, "read" + node.Name} {
			for _, pathItem := range e.spec.Paths {
				if pathItem.Get != nil && pathItem.Get.OperationID == opID {
					pathItem.Get.Parameters = append(pathItem.Get.Parameters,
						newQueryParam("expand", ogen.String(), desc),
					)
				}
			}
		}
	}
}

// --- Validation helper generation ---

func (e *Extension) generateValidationHelper(g *gen.Graph) error {
	outputPath := filepath.Join(g.Config.Target, "..", e.outputDir, "validation_helper_test.go")

	code := fmt.Sprintf(`// Code generated by entapi. DO NOT EDIT.

package %s

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/getkin/kin-openapi/openapi3"
	"github.com/getkin/kin-openapi/openapi3filter"
	"github.com/getkin/kin-openapi/routers"
	gorillarouter "github.com/getkin/kin-openapi/routers/gorillamux"
)

// ResponseValidator validates HTTP responses against the OpenAPI spec.
type ResponseValidator struct {
	router routers.Router
	t      *testing.T
}

// NewResponseValidator loads the OpenAPI spec and creates a validator for use in tests.
func NewResponseValidator(t *testing.T) *ResponseValidator {
	t.Helper()
	loader := openapi3.NewLoader()
	doc, err := loader.LoadFromFile(%q)
	if err != nil {
		t.Fatalf("loading OpenAPI spec: %%v", err)
	}
	if err := doc.Validate(loader.Context); err != nil {
		t.Fatalf("validating OpenAPI spec: %%v", err)
	}
	router, err := gorillarouter.NewRouter(doc)
	if err != nil {
		t.Fatalf("creating router: %%v", err)
	}
	return &ResponseValidator{router: router, t: t}
}

// Validate checks that the recorded HTTP response matches the OpenAPI spec
// for the given request's operation.
func (v *ResponseValidator) Validate(req *http.Request, resp *httptest.ResponseRecorder) {
	v.t.Helper()
	route, pathParams, err := v.router.FindRoute(req)
	if err != nil {
		v.t.Errorf("finding route for %%s %%s: %%v", req.Method, req.URL.Path, err)
		return
	}
	input := &openapi3filter.ResponseValidationInput{
		RequestValidationInput: &openapi3filter.RequestValidationInput{
			Request:    req,
			PathParams: pathParams,
			Route:      route,
		},
		Status: resp.Code,
		Header: resp.Header(),
		Body:   io.NopCloser(resp.Body),
	}
	if err := openapi3filter.ValidateResponse(context.Background(), input); err != nil {
		v.t.Errorf("response validation failed for %%s %%s (%%d): %%v", req.Method, req.URL.Path, resp.Code, err)
	}
}
`, e.pkgName, e.validationSpecPath)

	return os.WriteFile(outputPath, []byte(code), 0644)
}

