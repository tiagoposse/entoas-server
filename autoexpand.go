package entoasserver

// AutoExpandAnnotation is an Ent schema annotation that automatically expands
// specified edges on read and list endpoints without requiring ?expand= query params.
//
// Optional field selection restricts which fields are loaded on the expanded edge,
// reducing response payload size.
type AutoExpandAnnotation struct {
	Edges      []AutoExpandEdge `json:"edges"`
	Operations []string         `json:"operations,omitempty"` // "read", "list" — default: both
}

// AutoExpandEdge defines a single edge to auto-expand with optional field selection.
type AutoExpandEdge struct {
	Name   string   `json:"name"`             // edge name (e.g. "user")
	Fields []string `json:"fields,omitempty"` // field selection (e.g. ["id", "display_name"])
}

// Name implements the ent Annotation interface.
func (AutoExpandAnnotation) Name() string { return "EntAPIAutoExpand" }

// AutoExpand creates an auto-expand annotation for the given edges (no field selection).
func AutoExpand(edges ...string) *AutoExpandAnnotation {
	ae := make([]AutoExpandEdge, len(edges))
	for i, e := range edges {
		ae[i] = AutoExpandEdge{Name: e}
	}
	return &AutoExpandAnnotation{Edges: ae}
}

// AutoExpandWithFields creates an auto-expand annotation for a single edge with field selection.
func AutoExpandWithFields(edge string, fields ...string) *AutoExpandAnnotation {
	return &AutoExpandAnnotation{
		Edges: []AutoExpandEdge{{Name: edge, Fields: fields}},
	}
}

// AddEdge adds another edge to the auto-expand list.
func (a *AutoExpandAnnotation) AddEdge(edge string, fields ...string) *AutoExpandAnnotation {
	a.Edges = append(a.Edges, AutoExpandEdge{Name: edge, Fields: fields})
	return a
}

// OnOperations restricts auto-expand to specific operations (default: read + list).
func (a *AutoExpandAnnotation) OnOperations(ops ...string) *AutoExpandAnnotation {
	a.Operations = ops
	return a
}
