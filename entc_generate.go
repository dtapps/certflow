//go:build entc

package main

import (
	"log"

	"entgo.io/ent/entc"
	"entgo.io/ent/entc/gen"
)

func main() {
	err := entc.Generate("./internal/ent/schema", &gen.Config{
		Features: []gen.Feature{
			gen.FeatureUpsert,
		},
	})
	if err != nil {
		log.Fatalf("running ent codegen: %v", err)
	}
}
