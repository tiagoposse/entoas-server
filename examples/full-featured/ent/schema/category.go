//go:build ignore

package schema

import (
	"time"

	"entgo.io/contrib/entoas"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
	entoasserver "github.com/tiagoposse/entoas-server"
)

// Category uses cursor pagination instead of offset pagination.
type Category struct {
	ent.Schema
}

func (Category) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
		field.String("name").
			Unique().
			NotEmpty(),
		field.Text("description").
			Optional(),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
	}
}

func (Category) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("products", Product.Type),
	}
}

func (Category) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entoas.CreateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ReadOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.UpdateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.DeleteOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ListOperation(entoas.OperationPolicy(entoas.PolicyExpose)),

		// Cursor pagination instead of offset.
		entoasserver.CursorPagination(),
		entoasserver.FilterFields("name"),
	}
}

// Example queries:
//
//   GET /categories?limit=10                          — first page
//   GET /categories?cursor=dXVpZC1oZXJl&limit=10     — next page
//   GET /categories?name_contains=elec&limit=5
