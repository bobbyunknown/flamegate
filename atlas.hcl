// atlas.hcl — Atlas configuration for FlameGate schema migrations.
//
// Usage:
//   atlas migrate diff --env gorm init    # generate initial migration
//   atlas migrate apply --env gorm --url "file:./data/flamegate.db"

env "gorm" {
  src = "file://db/gormschema"
  dev = "sqlite://file?mode=memory"

  migration {
    dir = "file://db/migrations"
  }

  format {
    migrate {
      diff = "{{ sql . \"  \" }}"
    }
  }
}
