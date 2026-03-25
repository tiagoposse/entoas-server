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

// Product demonstrates all entoas-server features on a single entity.
type Product struct {
	ent.Schema
}

func (Product) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(uuid.New).
			Immutable().
			Annotations(entoas.ReadOnly(true)),
		field.String("name").
			NotEmpty(),
		field.Text("description").
			Optional(),
		field.Float("price").
			Positive(),
		field.Int("stock").
			Default(0),
		field.Enum("category").
			Values("electronics", "clothing", "food", "other").
			Default("other"),
		field.Enum("status").
			Values("active", "discontinued", "out_of_stock").
			Default("active"),
		field.String("sku").
			Unique().
			NotEmpty(),
		field.String("internal_notes").
			Optional().
			Annotations(entoasserver.NoFilter()), // exclude from filtering
		field.UUID("category_id", uuid.UUID{}).
			Optional().
			Nillable(),
		field.Time("deleted_at"). // required for soft delete
						Optional().
						Nillable(),
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

func (Product) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("category", Category.Type).
			Ref("products").
			Field("category_id").
			Unique(),
	}
}

func (Product) Annotations() []schema.Annotation {
	return []schema.Annotation{
		entoas.CreateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ReadOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.UpdateOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.DeleteOperation(entoas.OperationPolicy(entoas.PolicyExpose)),
		entoas.ListOperation(entoas.OperationPolicy(entoas.PolicyExpose)),

		// Override global defaults for this entity.
		entoasserver.Paginate(20),
		entoasserver.SortFields("name", "price", "stock", "created_at").
			WithDefaultSort("created_at:desc"),
		entoasserver.FilterFieldsExclude("internal_notes"),
		entoasserver.FieldSelection(),
		entoasserver.SoftDelete(),
		entoasserver.PatchSemantics(),
	}
}

// Example queries:
//
//   GET /products?category=electronics&price_gte=10&price_lte=100
//   GET /products?name_contains=phone&sort=price:asc
//   GET /products?fields=id,name,price,status&page=1&items_per_page=10
//   GET /products?status=active&sort=stock:desc,name:asc
//
//   PATCH /products/{id}  {"description": null}  — clears description (patch semantics)
//   DELETE /products/{id}                         — sets deleted_at (soft delete)
