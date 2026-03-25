package entoasserver

// FieldSelectionAnnotation is an Ent schema annotation that enables field selection
// via `?fields=id,name,status` on list and read endpoints.
type FieldSelectionAnnotation struct {
	Enabled bool `json:"enabled"`
}

// Name implements the ent Annotation interface.
func (FieldSelectionAnnotation) Name() string { return "EntAPIFieldSelection" }

// FieldSelection creates a field selection annotation for Ent schemas.
func FieldSelection() *FieldSelectionAnnotation {
	return &FieldSelectionAnnotation{Enabled: true}
}
