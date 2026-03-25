package entoasserver

// SortFieldsAnnotation is an Ent schema annotation that controls which fields
// are sortable on list endpoints via the `?sort=field:direction` query param.
type SortFieldsAnnotation struct {
	Include     []string `json:"include,omitempty"`
	Exclude     []string `json:"exclude,omitempty"`
	DefaultSort string   `json:"default_sort,omitempty"` // e.g. "created_at:desc"
}

// Name implements the ent Annotation interface.
func (SortFieldsAnnotation) Name() string { return "EntAPISortFields" }

// SortFields creates a sort fields annotation with an explicit include list.
func SortFields(fields ...string) *SortFieldsAnnotation {
	return &SortFieldsAnnotation{Include: fields}
}

// SortFieldsExclude creates a sort fields annotation with an exclude list.
func SortFieldsExclude(fields ...string) *SortFieldsAnnotation {
	return &SortFieldsAnnotation{Exclude: fields}
}

// WithDefaultSort sets the default sort order.
func (s *SortFieldsAnnotation) WithDefaultSort(sort string) *SortFieldsAnnotation {
	s.DefaultSort = sort
	return s
}
