package entoasserver

// SoftDeleteAnnotation is an Ent schema annotation that enables soft-delete behavior
// for generated delete handlers. Instead of removing the row, the handler sets `deleted_at`.
// List and read handlers auto-filter out rows where `deleted_at IS NOT NULL`.
//
// The entity schema must define a `deleted_at` field:
//
//	field.Time("deleted_at").Optional().Nillable()
type SoftDeleteAnnotation struct {
	Enabled bool `json:"enabled"`
}

// Name implements the ent Annotation interface.
func (SoftDeleteAnnotation) Name() string { return "EntAPISoftDelete" }

// SoftDelete creates a soft-delete annotation for Ent schemas.
func SoftDelete() *SoftDeleteAnnotation {
	return &SoftDeleteAnnotation{Enabled: true}
}
