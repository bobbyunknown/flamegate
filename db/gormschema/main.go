// Command gormschema is used by Atlas to generate migration diffs from GORM
// schema models. It is NOT invoked at runtime — only by:
//
//	atlas migrate diff --env gorm init
package main

import (
	"io"
	"os"

	"ariga.io/atlas-provider-gorm/gormschema"

	"github.com/bobbyunknown/flamegate/internal/infrastructure/persistence/schema"
)

func main() {
	stmts, err := gormschema.New("sqlite").Load(schema.AllModels()...)
	if err != nil {
		_, _ = io.WriteString(os.Stderr, "gormschema: "+err.Error()+"\n")
		os.Exit(1)
	}
	_, _ = io.WriteString(os.Stdout, stmts)
}
