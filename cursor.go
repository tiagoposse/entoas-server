package entoasserver

// CursorPaginationAnnotation is an Ent schema annotation that enables cursor-based
// pagination for list endpoints instead of offset-based pagination.
// Uses `?cursor=<base64>&limit=N` and returns `next_cursor` in the response.
type CursorPaginationAnnotation struct {
	Enabled bool `json:"enabled"`
}

// Name implements the ent Annotation interface.
func (CursorPaginationAnnotation) Name() string { return "EntAPICursorPagination" }

// CursorPagination creates a cursor pagination annotation for Ent schemas.
func CursorPagination() *CursorPaginationAnnotation {
	return &CursorPaginationAnnotation{Enabled: true}
}
