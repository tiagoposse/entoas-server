package entoasserver

// StripFieldsAnnotation is an Ent schema annotation that removes specified fields
// from API responses on read and list endpoints.
//
// Fields are set to nil (for pointer types) or zero value (for non-pointer types)
// in the generated API response. This happens after the Ent-to-API conversion.
//
// The optional SelfCheck allows skipping the stripping when the authenticated user
// is viewing their own data (viewer ID == entity ID).
type StripFieldsAnnotation struct {
	Fields     []string `json:"fields"`               // JSON field names to strip
	Operations []string `json:"operations,omitempty"`  // "read", "list" — default: both
	SelfCheck  bool     `json:"self_check,omitempty"`  // if true, skip stripping when viewer == entity owner
}

// Name implements the ent Annotation interface.
func (StripFieldsAnnotation) Name() string { return "EntAPIStripFields" }

// StripFields creates a strip-fields annotation for the given JSON field names.
func StripFields(fields ...string) *StripFieldsAnnotation {
	return &StripFieldsAnnotation{Fields: fields}
}

// OnOperations restricts field stripping to specific operations (default: read + list).
func (a *StripFieldsAnnotation) OnOperations(ops ...string) *StripFieldsAnnotation {
	a.Operations = ops
	return a
}

// WithSelfCheck enables skipping the strip when the viewer's ID matches the entity's ID.
// The generated handler calls `auth.OptionalAuth(ctx)` to get the viewer ID and compares
// it to `entity.ID`. Requires the entity to have a UUID primary key.
func (a *StripFieldsAnnotation) WithSelfCheck() *StripFieldsAnnotation {
	a.SelfCheck = true
	return a
}
