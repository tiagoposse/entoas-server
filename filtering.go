package entoasserver

// FilterFieldsAnnotation controls which fields are filterable on list endpoints.
type FilterFieldsAnnotation struct {
	// Include lists field names to make filterable (whitelist mode).
	// If empty, all non-sensitive fields are filterable (when global filtering is enabled).
	Include []string `json:"include,omitempty"`
	// Exclude lists field names to exclude from filtering.
	Exclude []string `json:"exclude,omitempty"`
}

func (FilterFieldsAnnotation) Name() string { return "EntAPIFilterFields" }

// FilterFields creates an annotation that whitelists specific fields for filtering.
func FilterFields(fields ...string) *FilterFieldsAnnotation {
	return &FilterFieldsAnnotation{Include: fields}
}

// FilterFieldsExclude creates an annotation that blacklists specific fields from filtering.
func FilterFieldsExclude(fields ...string) *FilterFieldsAnnotation {
	return &FilterFieldsAnnotation{Exclude: fields}
}

// NoFilterAnnotation marks a single field as non-filterable.
// Use as a field-level annotation in Ent schema.
type NoFilterAnnotation struct{}

func (NoFilterAnnotation) Name() string { return "EntAPINoFilter" }
func NoFilter() *NoFilterAnnotation     { return &NoFilterAnnotation{} }

// AllowFilterAnnotation marks a sensitive field as filterable (override).
type AllowFilterAnnotation struct{}

func (AllowFilterAnnotation) Name() string   { return "EntAPIAllowFilter" }
func AllowFilter() *AllowFilterAnnotation { return &AllowFilterAnnotation{} }
