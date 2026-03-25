package entoasserver

// PatchSemanticsAnnotation is an Ent schema annotation that enables JSON merge patch
// semantics for update handlers. When enabled, the handler distinguishes between
// "field absent" (don't change) and "field explicitly null" (clear the value).
type PatchSemanticsAnnotation struct {
	Enabled bool `json:"enabled"`
}

// Name implements the ent Annotation interface.
func (PatchSemanticsAnnotation) Name() string { return "EntAPIPatchSemantics" }

// PatchSemantics creates a patch semantics annotation for Ent schemas.
func PatchSemantics() *PatchSemanticsAnnotation {
	return &PatchSemanticsAnnotation{Enabled: true}
}
