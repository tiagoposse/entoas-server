//go:build ignore

package schema

import (
	"time"

	"entgo.io/contrib/entoas"
	"entgo.io/ent"
	"entgo.io/ent/schema"
	"entgo.io/ent/schema/field"
	"github.com/google/uuid"
)

// Post is a simple blog post entity.
type Post struct {
	ent.Schema
}

func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
		field.String("title").
			NotEmpty(),
		field.Text("body"),
		field.Enum("status").
			Values("draft", "published", "archived").
			Default("draft"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			Annotations(entoas.ReadOnly(true)),
	}
}

func (Post) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entoas.CreateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ReadOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.UpdateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.DeleteOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ListOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
	}
}

// Generated endpoints:
//
//   POST   /posts               — create a post
//   GET    /posts               — list posts (paginated, filterable, sortable)
//   GET    /posts/{id}          — read a post
//   PATCH  /posts/{id}          — update a post
//   DELETE /posts/{id}          — delete a post
//
// Auto-generated query parameters:
//
//   GET /posts?title=hello                   — exact match
//   GET /posts?title_contains=hel            — case-insensitive contains
//   GET /posts?status=published              — enum filter
//   GET /posts?sort=created_at:desc          — sorting
//   GET /posts?page=2&items_per_page=10      — pagination
