package entoasserver

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AuthGuardAnnotation mirrors the entauth.GuardAnnotation structure for reading annotations.
type AuthGuardAnnotation struct {
	Create []string `json:"create,omitempty"`
	Read   []string `json:"read,omitempty"`
	Update []string `json:"update,omitempty"`
	Delete []string `json:"delete,omitempty"`
	List   []string `json:"list,omitempty"`
}

// EntityGuardTemplate defines how to generate inline guard code for entity-specific guards.
// The template receives the entity name and produces Go code that fetches the entity
// and calls a guard function.
type EntityGuardTemplate struct {
	// FieldName is the entity field to check (e.g., "OrganizerID")
	FieldName string
	// FuncName is the guard function to call (e.g., "requiresOrganizer")
	FuncName string
}

// AuthGuardHook creates a BeforeHandlerHook that reads AuthGuard annotations from
// schemas and injects guard code into generated handlers.
//
// authImport is the full import string including alias, e.g. `auth "github.com/tiagoposse/authguard"`.
// entityGuards maps guard names to their entity-specific templates. Guards not in this
// map are resolved via `auth.Resolve("guardName")` at runtime.
func AuthGuardHook(authImport string, entityGuards map[string]EntityGuardTemplate) BeforeHandlerHook {
	return func(entityName string, operation string, annotations map[string]interface{}) (string, []string) {
		raw, ok := annotations["AuthGuard"]
		if !ok || raw == nil {
			return "", nil
		}

		data, err := json.Marshal(raw)
		if err != nil {
			return "", nil
		}

		var ann AuthGuardAnnotation
		if json.Unmarshal(data, &ann) != nil {
			return "", nil
		}

		var guards []string
		switch operation {
		case "create":
			guards = ann.Create
		case "read":
			guards = ann.Read
		case "update":
			guards = ann.Update
		case "delete":
			guards = ann.Delete
		case "list":
			guards = ann.List
		}

		if len(guards) == 0 {
			return "", nil
		}

		var code strings.Builder
		var imports []string
		imports = append(imports, authImport)

		for _, guardName := range guards {
			if eg, ok := entityGuards[guardName]; ok {
				// Entity-specific guard: inline template that fetches entity and calls guard func
				code.WriteString(fmt.Sprintf(`	{
		_entity, _err := h.client.%s.Get(ctx, request.Id)
		if _err != nil { return nil, _err }
		if _err = %s(ctx, _entity.%s); _err != nil { return nil, _err }
	}
`, entityName, eg.FuncName, eg.FieldName))
			} else {
				// Simple guard: resolved at runtime via auth.Resolve
				code.WriteString(fmt.Sprintf("\tif err := auth.Resolve(%q)(ctx, h.client); err != nil {\n\t\treturn nil, err\n\t}\n", guardName))
			}
		}

		return code.String(), imports
	}
}
