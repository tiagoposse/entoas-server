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
	entoasserver "github.com/tiagoposse/entoas-server"
)

// User with role-based guards and sensitive field handling.
type User struct {
	ent.Schema
}

func (User) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
		field.String("email").
			Unique().
			NotEmpty().
			Annotations(entoasserver.AllowFilter()), // allow filtering despite sensitivity
		field.String("username").
			Unique().
			NotEmpty(),
		field.String("display_name").
			Optional(),
		field.String("password_hash").
			Sensitive(), // excluded from API responses and filtering
		field.Enum("role").
			Values("user", "editor", "admin").
			Default("user"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
	}
}

func (User) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("documents", Document.Type),
	}
}

func (User) Annotations() []schema.Annotation {
	return []schema.Annotation{
		// Exclude create — use a custom /auth/register endpoint instead.
		entoas.CreateOperation(entoas.OperationPolicy(entoas.PolicyExclude)),
		entoas.ReadOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.UpdateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.DeleteOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ListOperation(entoas.OperationPolicy(entoas.PolicyExpose)),

		auth.Guards().
			OnRead("requiresAuth").
			OnUpdate("requiresAuth").
			OnDelete("requiresRole:admin").
			OnList("requiresAuth"),

		entoasserver.FilterFieldsExclude("password_hash"),
	}
}
