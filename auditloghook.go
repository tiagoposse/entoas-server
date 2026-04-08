package entoasserver

import (
	"encoding/json"
	"fmt"
	"strings"
)

// AuditLogAnnotation declares which CRUD operations should be audited on a schema.
// Annotate schemas with AuditLog().OnCreate().OnUpdate().OnDelete() etc.
type AuditLogAnnotation struct {
	Create bool `json:"create,omitempty"`
	Read   bool `json:"read,omitempty"`
	Update bool `json:"update,omitempty"`
	Delete bool `json:"delete,omitempty"`
	List   bool `json:"list,omitempty"`
}

// Name implements the ent Annotation interface.
func (AuditLogAnnotation) Name() string { return "AuditLog" }

// AuditLog creates a new empty AuditLogAnnotation builder.
func AuditLog() *AuditLogAnnotation { return &AuditLogAnnotation{} }

func (a *AuditLogAnnotation) OnCreate() *AuditLogAnnotation { a.Create = true; return a }
func (a *AuditLogAnnotation) OnRead() *AuditLogAnnotation   { a.Read = true; return a }
func (a *AuditLogAnnotation) OnUpdate() *AuditLogAnnotation { a.Update = true; return a }
func (a *AuditLogAnnotation) OnDelete() *AuditLogAnnotation { a.Delete = true; return a }
func (a *AuditLogAnnotation) OnList() *AuditLogAnnotation   { a.List = true; return a }

// AuditLogHook creates an AfterHandlerHook that reads AuditLog annotations from schemas
// and injects audit logging code into generated handlers.
//
// The injected code calls a function with the signature:
//
//	func(ctx context.Context, client *ent.Client, action, entityType string, entityID *uuid.UUID, metadata map[string]interface{})
//
// Parameters:
//   - auditFuncName: the function name to call, e.g. "createAuditLog".
//     Must be defined in the same package as the generated handlers.
func AuditLogHook(auditFuncName string) AfterHandlerHook {
	return func(entityName string, operation string, annotations map[string]interface{}) (string, []string) {
		raw, ok := annotations["AuditLog"]
		if !ok || raw == nil {
			return "", nil
		}

		data, err := json.Marshal(raw)
		if err != nil {
			return "", nil
		}

		var ann AuditLogAnnotation
		if json.Unmarshal(data, &ann) != nil {
			return "", nil
		}

		shouldAudit := false
		switch operation {
		case "create":
			shouldAudit = ann.Create
		case "read":
			shouldAudit = ann.Read
		case "update":
			shouldAudit = ann.Update
		case "delete":
			shouldAudit = ann.Delete
		case "list":
			shouldAudit = ann.List
		}

		if !shouldAudit {
			return "", nil
		}

		entityTypeLower := strings.ToLower(entityName)

		switch operation {
		case "create":
			return fmt.Sprintf("\t%s(ctx, h.client, %q, %q, &entity.ID, nil)\n",
				auditFuncName, operation, entityTypeLower), nil
		case "update":
			return fmt.Sprintf("\t%s(ctx, h.client, %q, %q, &entity.ID, nil)\n",
				auditFuncName, operation, entityTypeLower), nil
		case "delete":
			return fmt.Sprintf("\t%s(ctx, h.client, %q, %q, &request.Id, nil)\n",
				auditFuncName, operation, entityTypeLower), nil
		}

		return "", nil
	}
}
