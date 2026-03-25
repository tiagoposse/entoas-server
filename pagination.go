package entoasserver

// PaginationAnnotation is an Ent schema annotation that controls pagination
// for generated list handlers.
type PaginationAnnotation struct {
	// Enabled controls whether the list handler returns paginated responses
	// with total count and page metadata. If false, returns a bare array.
	Enabled bool `json:"enabled"`
	// DefaultPageSize is the default number of items per page.
	// If 0, the global default (or 30) is used.
	DefaultPageSize int `json:"default_page_size,omitempty"`
}

// Name implements the ent Annotation interface.
func (PaginationAnnotation) Name() string { return "EntAPIPagination" }

// Paginate creates a pagination annotation for Ent schemas.
// Use in schema Annotations() to enable paginated list responses.
//
//	func (Match) Annotations() []schema.Annotation {
//	    return []schema.Annotation{
//	        entapi.Paginate(30), // paginated with 30 items per page
//	    }
//	}
func Paginate(defaultPageSize int) *PaginationAnnotation {
	return &PaginationAnnotation{
		Enabled:         true,
		DefaultPageSize: defaultPageSize,
	}
}
