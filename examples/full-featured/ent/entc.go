//go:build ignore

// Example: full-featured
//
// All features enabled: pagination, filtering, sorting, field selection,
// soft delete, patch semantics, and cursor pagination.
package main

import (
	"log"

	"entgo.io/contrib/entoas"
	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
	"github.com/ogen-go/ogen"
	entoasserver "github.com/tiagoposse/entoas-server"
)

func main() {
	spec := new(ogen.Spec)

	oasExt, err := entoas.NewExtension(
		entoas.Spec(spec),
		entoas.SimpleModels(),
		entoas.Mutations(func(_ *gen.Graph, spec *ogen.Spec) error {
			spec.Info.SetTitle("Full-Featured API")
			spec.Info.SetVersion("1.0.0")
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("creating entoas extension: %v", err)
	}

	apiExt, err := entoasserver.NewExtension(spec,
		entoasserver.WithOutputDir("internal/api"),
		entoasserver.WithPackageName("api"),

		// Pagination & query features (global defaults).
		entoasserver.WithPagination(30),
		entoasserver.WithFieldFiltering(true),
		entoasserver.WithSorting("created_at:desc"),
		entoasserver.WithFieldSelection(),

		// Write behavior.
		entoasserver.WithSoftDelete(),
		entoasserver.WithPatchSemantics(),

		// Testing support.
		entoasserver.WithResponseValidation("openapi.json"),
	)
	if err != nil {
		log.Fatalf("creating entoas-server extension: %v", err)
	}

	err = entc.Generate("./schema", &gen.Config{
		Features: []gen.Feature{gen.FeatureUpsert},
	}, entc.Extensions(oasExt, apiExt))
	if err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
