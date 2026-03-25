//go:build ignore

// Example: with-auth
//
// Integration with authguard for JWT authentication and guard annotations.
// Shows entity-level ownership guards and role-based access control.
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
			spec.Info.SetTitle("Authenticated API")
			spec.Info.SetVersion("1.0.0")

			// Add Bearer auth security scheme.
			spec.Security = []ogen.SecurityRequirement{
				{"bearerAuth": []string{}},
			}
			if spec.Components == nil {
				spec.Components = &ogen.Components{}
			}
			if spec.Components.SecuritySchemes == nil {
				spec.Components.SecuritySchemes = make(map[string]*ogen.SecurityScheme)
			}
			spec.Components.SecuritySchemes["bearerAuth"] = &ogen.SecurityScheme{
				Type:         "http",
				Scheme:       "bearer",
				BearerFormat: "JWT",
			}
			return nil
		}),
	)
	if err != nil {
		log.Fatalf("creating entoas extension: %v", err)
	}

	apiExt, err := entoasserver.NewExtension(spec,
		entoasserver.WithOutputDir("internal/api"),
		entoasserver.WithPackageName("api"),
		entoasserver.WithPagination(30),
		entoasserver.WithFieldFiltering(true),

		// Auth guard hook: reads Guards() annotations from schemas and
		// injects guard checks into generated handlers.
		entoasserver.WithBeforeHandlerHook(entoasserver.AuthGuardHook(
			// Import alias for the auth package in generated code.
			`auth "github.com/tiagoposse/authguard"`,

			// Entity-level guards: these fetch the entity from the DB
			// and check a field against the authenticated user.
			map[string]entoasserver.EntityGuardTemplate{
				"requiresOwner": {
					FieldName: "OwnerID",    // entity field to check
					FuncName:  "requiresOwner", // function you provide
				},
				"requiresOrganizer": {
					FieldName: "OrganizerID",
					FuncName:  "requiresOrganizer",
				},
			},
		)),
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
