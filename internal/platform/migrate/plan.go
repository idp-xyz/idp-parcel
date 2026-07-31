// Package migrate owns the Parcel migration plan. It combines the immutable
// framework migration assets with the Parcel business migrations into one
// ordered history and records what was actually applied. It never edits a
// framework template and never runs at application startup.
package migrate

import (
	"embed"
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strconv"
	"strings"

	"go.idp.xyz/idp-bento-go/postgres"
)

// Schema names Parcel creates ahead of the migration run. The framework
// technical tables stay in their own schema so a business migration can never
// reshape them.
const (
	SchemaBento          = "bento"
	SchemaParcelShipment = "parcel_shipment"
	SchemaHistory        = "parcel_migration"
)

// FrameworkVersion is the exact framework candidate whose migration assets this
// plan applies.
const FrameworkVersion = "v0.1.0-rc.1"

// VersionTable is the tern version table. It lives in the Parcel history
// schema, never in a framework or business schema.
const VersionTable = SchemaHistory + ".schema_version"

//go:embed all:sql
var businessMigrations embed.FS

// Origin separates migrations owned by the framework from migrations owned by
// Parcel.
type Origin string

const (
	OriginFramework Origin = "FRAMEWORK"
	OriginParcel    Origin = "PARCEL"
)

// Step is one ordered migration in the Parcel history.
type Step struct {
	ID       string
	Origin   Origin
	Schema   string
	Checksum string
	// FrameworkVersion is set for framework steps only.
	FrameworkVersion string
	UpSQL            string
	DownSQL          string
}

// Plan is the ordered migration history Parcel applies.
func Plan() ([]Step, error) {
	steps, err := frameworkSteps()
	if err != nil {
		return nil, err
	}
	business, err := stepsForModule("parcel_shipment", SchemaParcelShipment)
	if err != nil {
		return nil, err
	}
	return append(steps, business...), nil
}

// Schemas returns the schemas the migration job creates before applying the
// plan. Production API and publisher roles never hold the privileges to do it.
func Schemas() []string {
	return []string{SchemaHistory, SchemaBento, SchemaParcelShipment}
}

func frameworkSteps() ([]Step, error) {
	assets := postgres.Migrations()
	steps := make([]Step, 0, len(assets))

	for _, asset := range assets {
		rendered, err := postgres.RenderMigration(asset.ID, SchemaBento)
		if err != nil {
			return nil, fmt.Errorf("render framework migration %s: %w", asset.ID, err)
		}
		steps = append(steps, Step{
			ID:               "framework/" + asset.ID,
			Origin:           OriginFramework,
			Schema:           SchemaBento,
			Checksum:         asset.Checksum,
			FrameworkVersion: FrameworkVersion,
			UpSQL:            rendered,
		})
	}
	return steps, nil
}

func stepsForModule(module, schema string) ([]Step, error) {
	directory := path.Join("sql", module)

	entries, err := fs.ReadDir(businessMigrations, directory)
	if err != nil {
		return nil, fmt.Errorf("read migrations for %s: %w", module, err)
	}

	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}
		if err := checkSequence(entry.Name()); err != nil {
			return nil, fmt.Errorf("migration %s/%s: %w", module, entry.Name(), err)
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)

	steps := make([]Step, 0, len(names))
	for _, name := range names {
		content, err := fs.ReadFile(businessMigrations, path.Join(directory, name))
		if err != nil {
			return nil, fmt.Errorf("read migration %s/%s: %w", module, name, err)
		}
		up, down := splitDirections(string(content))
		if strings.TrimSpace(up) == "" {
			return nil, fmt.Errorf("migration %s/%s has no forward SQL", module, name)
		}
		steps = append(steps, Step{
			ID:       module + "/" + strings.TrimSuffix(name, ".sql"),
			Origin:   OriginParcel,
			Schema:   schema,
			Checksum: checksumOf(content),
			UpSQL:    up,
			DownSQL:  down,
		})
	}
	return steps, nil
}

// directionSeparator is the tern convention for splitting forward and reverse
// SQL inside one migration file.
const directionSeparator = "---- create above / drop below ----"

func splitDirections(content string) (string, string) {
	up, down, found := strings.Cut(content, directionSeparator)
	if !found {
		return content, ""
	}
	return up, down
}

func checkSequence(name string) error {
	prefix, _, found := strings.Cut(name, "_")
	if !found {
		return fmt.Errorf("name must start with a zero padded sequence")
	}
	sequence, err := strconv.Atoi(prefix)
	if err != nil || sequence <= 0 {
		return fmt.Errorf("name must start with a zero padded sequence")
	}
	return nil
}
