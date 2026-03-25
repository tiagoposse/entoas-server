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
	auth "github.com/tiagoposse/authguard"
)

// Document demonstrates guard annotations for auth-protected CRUD.
type Document struct {
	ent.Schema
}

func (Document) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
		field.UUID("owner_id", uuid.UUID{}).
			Immutable(),
		field.String("title").
			NotEmpty(),
		field.Text("content").
			Optional(),
		field.Enum("visibility").
			Values("private", "shared", "public").
			Default("private"),
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

func (Document) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("owner", User.Type).
			Ref("documents").
			Field("owner_id").
			Required().
			Unique().
			Immutable(),
	}
}

func (Document) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entoas.CreateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ReadOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.UpdateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.DeleteOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ListOperation(entoas.OperationPolicy(entoas.PolicyExpose)),

		// Guard annotations:
		// - Create requires auth + write scope
		// - Read requires auth
		// - Update/Delete require auth + ownership check
		// - List requires auth
		auth.Guards().
			OnCreate("requiresAuth", "requiresScope:write").
			OnRead("requiresAuth").
			OnUpdate("requiresAuth", "requiresOwner").
			OnDelete("requiresAuth", "requiresOwner").
			OnList("requiresAuth"),
	}
}

// Generated handler for UpdateDocument will include:
//
//   // Simple guard — resolved at runtime
//   if err := auth.Resolve("requiresAuth")(ctx, h.client); err != nil {
//       return nil, err
//   }
//
//   // Entity guard — fetches entity and checks OwnerID
//   {
//       _entity, _err := h.client.Document.Get(ctx, request.Id)
//       if _err != nil { return nil, _err }
//       if _err = requiresOwner(ctx, _entity.OwnerID); _err != nil { return nil, _err }
//   }
