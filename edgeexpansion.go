package entoasserver

// EdgeExpansionAnnotation is an Ent schema annotation that enables dynamic edge
// expansion via `?expand=teams.players,standings` on read and list endpoints.
//
// When enabled, the handler parses the comma-separated expand parameter and calls
// the corresponding With* methods on the query. Dot notation (e.g. "teams.players")
// triggers nested eager loading (one level deep).
//
// Only edges listed in AllowedEdges are expandable. If AllowedEdges is empty,
// all exposed edges are expandable.
type EdgeExpansionAnnotation struct {
	Enabled      bool     `json:"enabled"`
	AllowedEdges []string `json:"allowed_edges,omitempty"`
}

// Name implements the ent Annotation interface.
func (EdgeExpansionAnnotation) Name() string { return "EntAPIEdgeExpansion" }

// EdgeExpansion creates an edge expansion annotation that allows all exposed edges.
func EdgeExpansion() *EdgeExpansionAnnotation {
	return &EdgeExpansionAnnotation{Enabled: true}
}

// WithAllowedEdges restricts which edges can be expanded.
// Supports dot notation for nested edges (e.g. "teams.players").
func (a *EdgeExpansionAnnotation) WithAllowedEdges(edges ...string) *EdgeExpansionAnnotation {
	a.AllowedEdges = edges
	return a
}

// nestedEdgeInfo holds metadata about an edge's sub-edges for nested expansion.
type nestedEdgeInfo struct {
	Name        string // Edge name (e.g. "players")
	StructField string // Go struct field (e.g. "Players")
	TypeName    string // Target entity type (e.g. "User")
}
