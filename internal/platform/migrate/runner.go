package migrate

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/tern/v2/migrate"
)

// historyDDL records what was actually applied. tern tracks a single version
// number; Parcel additionally records the migration id, the canonical checksum,
// the framework module version and the real schema so a deployed database can
// be traced back to immutable artifacts.
const historyDDL = `
CREATE TABLE IF NOT EXISTS ` + SchemaHistory + `.applied_migration (
    migration_id       text        NOT NULL PRIMARY KEY,
    sequence           integer     NOT NULL,
    origin             text        NOT NULL,
    canonical_checksum text        NOT NULL,
    framework_version  text,
    target_schema      text        NOT NULL,
    applied_at         timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT applied_migration_origin_known
        CHECK (origin IN ('FRAMEWORK', 'PARCEL')),
    CONSTRAINT applied_migration_framework_version_present
        CHECK (origin <> 'FRAMEWORK' OR framework_version IS NOT NULL)
)`

// ErrChecksumDrift reports that an already applied migration no longer matches
// the artifact this build carries. It blocks the run rather than reapplying or
// silently accepting a rewritten template.
var ErrChecksumDrift = errors.New("migrate: applied migration checksum differs from the current artifact")

// Run applies the full Parcel migration plan against one connection. The caller
// owns connection lifetime, credentials and the deployment window; nothing here
// is reachable from an application process.
func Run(ctx context.Context, conn *pgx.Conn, logger *slog.Logger) error {
	plan, err := Plan()
	if err != nil {
		return err
	}
	if err := prepare(ctx, conn); err != nil {
		return err
	}
	if err := verifyNoDrift(ctx, conn, plan); err != nil {
		return err
	}

	migrator, err := migrate.NewMigrator(ctx, conn, VersionTable)
	if err != nil {
		return fmt.Errorf("migrate: build migrator: %w", err)
	}
	for _, step := range plan {
		migrator.AppendMigration(step.ID, step.UpSQL, step.DownSQL)
	}

	applied := make(map[int32]Step, len(plan))
	for index, step := range plan {
		applied[int32(index+1)] = step
	}
	migrator.OnStart = func(sequence int32, name, direction, _ string) {
		logger.Info("applying migration",
			"sequence", sequence, "migration", name, "direction", direction)
	}

	before, err := migrator.GetCurrentVersion(ctx)
	if err != nil {
		return fmt.Errorf("migrate: read current version: %w", err)
	}
	if err := migrator.Migrate(ctx); err != nil {
		return fmt.Errorf("migrate: apply plan: %w", err)
	}

	for sequence := before + 1; int(sequence) <= len(plan); sequence++ {
		if err := recordApplied(ctx, conn, sequence, applied[sequence]); err != nil {
			return err
		}
	}
	logger.Info("migration plan applied",
		"from_version", before, "to_version", len(plan), "framework_version", FrameworkVersion)
	return nil
}

func prepare(ctx context.Context, conn *pgx.Conn) error {
	for _, schema := range Schemas() {
		// Schema names come from this package's constants, never from input.
		if _, err := conn.Exec(ctx, `CREATE SCHEMA IF NOT EXISTS `+schema); err != nil {
			return fmt.Errorf("migrate: create schema %s: %w", schema, err)
		}
	}
	if _, err := conn.Exec(ctx, historyDDL); err != nil {
		return fmt.Errorf("migrate: create migration history: %w", err)
	}
	return nil
}

// verifyNoDrift compares every already recorded migration against the artifact
// in this build. A framework template that was copied and edited, or a business
// migration that was rewritten after being applied, fails here.
func verifyNoDrift(ctx context.Context, conn *pgx.Conn, plan []Step) error {
	rows, err := conn.Query(ctx,
		`SELECT migration_id, canonical_checksum FROM `+SchemaHistory+`.applied_migration`)
	if err != nil {
		return fmt.Errorf("migrate: read migration history: %w", err)
	}
	defer rows.Close()

	recorded := make(map[string]string)
	for rows.Next() {
		var id, checksum string
		if err := rows.Scan(&id, &checksum); err != nil {
			return fmt.Errorf("migrate: scan migration history: %w", err)
		}
		recorded[id] = checksum
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("migrate: read migration history: %w", err)
	}

	for _, step := range plan {
		checksum, present := recorded[step.ID]
		if present && checksum != step.Checksum {
			return fmt.Errorf("%w: %s recorded %s, artifact %s",
				ErrChecksumDrift, step.ID, checksum, step.Checksum)
		}
	}
	return nil
}

func recordApplied(ctx context.Context, conn *pgx.Conn, sequence int32, step Step) error {
	var frameworkVersion *string
	if step.Origin == OriginFramework {
		version := step.FrameworkVersion
		frameworkVersion = &version
	}

	_, err := conn.Exec(ctx,
		`INSERT INTO `+SchemaHistory+`.applied_migration
			(migration_id, sequence, origin, canonical_checksum, framework_version, target_schema)
		 VALUES ($1, $2, $3, $4, $5, $6)
		 ON CONFLICT (migration_id) DO NOTHING`,
		step.ID, sequence, string(step.Origin), step.Checksum, frameworkVersion, step.Schema)
	if err != nil {
		return fmt.Errorf("migrate: record %s: %w", step.ID, err)
	}
	return nil
}

func checksumOf(content []byte) string {
	sum := sha256.Sum256(content)
	return "sha256:" + hex.EncodeToString(sum[:])
}
