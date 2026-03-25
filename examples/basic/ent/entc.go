//go:build ignore

// Example: basic
//
// Minimal entoas-server setup with offset pagination and filtering.
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

	// 1. entoas generates the OpenAPI spec from Ent schemas.
	oasExt, err := entoas.NewExtension(
		entoas.Spec(spec),
		entoas.SimpleModels(),
		entoas.Mutations(func(_ *gen.Graph, spec *ogen.Spec) error {
			spec.Info.SetTitle("Blog API")
			spec.Info.SetVersion("1.0.0")
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("creating entoas extension: %v", err)
	}

	// 2. entoas-server generates handler implementations.
	apiExt, err := entoasserver.NewExtension(spec,
		entoasserver.WithOutputDir("internal/api"),
		entoasserver.WithPackageName("api"),
		entoasserver.WithPagination(30),          // 30 items per page
		entoasserver.WithFieldFiltering(true),    // auto-generate filter query params
		entoasserver.WithSorting("created_at:desc"),
	)
	if err != nil {
		log.Fatalf("creating entoas-server extension: %v", err)
	}

	// 3. Run code generation.
	err = entc.Generate("./schema", &gen.Config{
		Features: []gen.Feature{gen.FeatureUpsert},
	}, entc.Extensions(oasExt, apiExt))
	if err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
