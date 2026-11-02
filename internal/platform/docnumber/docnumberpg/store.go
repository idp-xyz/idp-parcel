// Package docnumberpg 是单据编号规则册与平台序列的 PostgreSQL 适配器（ADR-0355 决定四、六）。
//
// 规则册在本仓自己的 schema（migrate.SchemaDocumentNumbering）；序列表的资产归平台库，本包只用平台的
// `numbering/postgres` 在 migrate.SchemaPlatform 上构造序列存储，不复制它的 SQL。
package docnumberpg

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-platform-go/numbering"
	numberingpg "go.idp.xyz/idp-platform-go/numbering/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// Store 是规则册的存储，同时答取号编排「是否在开立事务里」。
//
// 读走 ReadExecutor：在事务里读事务内的快照，在事务外读连接池——读口不必为了读册开一笔事务。写走 RequireExecutor，
// 无事务即拒，同框架合同对写参与者的要求。
type Store struct {
	db *bentopg.DB
}

var (
	_ docnumber.RuleBook         = (*Store)(nil)
	_ docnumber.TransactionScope = (*Store)(nil)
)

func NewStore(db *bentopg.DB) (*Store, error) {
	if db == nil {
		return nil, errors.New("document numbering postgres: db is nil")
	}
	return &Store{db: db}, nil
}

// NewSequences 在平台 schema 上构造平台的序列存储。它与调用方的开立事务必须接在同一个 *bentopg.DB 上：框架按 DB
// 认事务归属，换一个 DB 构造的序列存储在调用方事务里会被拒。
func NewSequences(db *bentopg.DB) (numbering.SequenceAllocator, error) {
	store, err := numberingpg.NewSequenceStore(db, numberingpg.WithSchema(migrate.SchemaPlatform))
	if err != nil {
		return nil, fmt.Errorf("document numbering postgres: sequence store: %w", err)
	}
	return store, nil
}

// RequireTransaction 答 ctx 是否带着本 DB 的事务。
func (store *Store) RequireTransaction(ctx context.Context) error {
	if _, err := store.db.RequireExecutor(ctx); err != nil {
		return err
	}
	return nil
}

const ruleColumns = `tenant_id, type_code, template, effective_from, registered_by, registered_at`

// AppendRuleVersion 追加一版；同（租户，类型，生效时刻）已在册时不落，交回在册那一版。并发两笔同键追加时，后到者的
// INSERT 等前者提交后走 DO NOTHING，随后的 SELECT 在读已提交下看得见前者那一行。
func (store *Store) AppendRuleVersion(ctx context.Context, version docnumber.RuleVersion) (docnumber.RuleVersion, bool, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return docnumber.RuleVersion{}, false, fmt.Errorf("append rule version: %w", err)
	}
	stored, err := scanRule(executor.QueryRow(ctx, `
		INSERT INTO document_numbering.rule_version (`+ruleColumns+`)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (tenant_id, type_code, effective_from) DO NOTHING
		RETURNING `+ruleColumns,
		version.Tenant, string(version.Type), version.Template, version.EffectiveFrom, version.RegisteredBy, version.RegisteredAt))
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return docnumber.RuleVersion{}, false, fmt.Errorf("append rule version: %w", err)
	}
	existing, err := scanRule(executor.QueryRow(ctx, `
		SELECT `+ruleColumns+` FROM document_numbering.rule_version
		WHERE tenant_id = $1 AND type_code = $2 AND effective_from = $3`,
		version.Tenant, string(version.Type), version.EffectiveFrom))
	if err != nil {
		return docnumber.RuleVersion{}, false, fmt.Errorf("read the rule version holding that moment: %w", err)
	}
	return existing, false, nil
}

func (store *Store) EffectiveRuleVersion(ctx context.Context, tenant string, code docnumber.TypeCode, at time.Time) (docnumber.RuleVersion, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return docnumber.RuleVersion{}, false, fmt.Errorf("read effective rule version: %w", err)
	}
	version, err := scanRule(querier.QueryRow(ctx, `
		SELECT `+ruleColumns+` FROM document_numbering.rule_version
		WHERE tenant_id = $1 AND type_code = $2 AND effective_from <= $3
		ORDER BY effective_from DESC
		LIMIT 1`,
		tenant, string(code), at))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return docnumber.RuleVersion{}, false, nil
	case err != nil:
		return docnumber.RuleVersion{}, false, fmt.Errorf("read effective rule version: %w", err)
	}
	return version, true, nil
}

func (store *Store) RuleVersions(ctx context.Context, tenant string) ([]docnumber.RuleVersion, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("read rule versions: %w", err)
	}
	rows, err := querier.Query(ctx, `
		SELECT `+ruleColumns+` FROM document_numbering.rule_version
		WHERE tenant_id = $1
		ORDER BY type_code, effective_from`, tenant)
	if err != nil {
		return nil, fmt.Errorf("read rule versions: %w", err)
	}
	defer rows.Close()
	var versions []docnumber.RuleVersion
	for rows.Next() {
		version, err := scanRule(rows)
		if err != nil {
			return nil, fmt.Errorf("read rule versions: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read rule versions: %w", err)
	}
	return versions, nil
}

const timeZoneColumns = `tenant_id, time_zone, effective_from, registered_by, registered_at`

// AppendTimeZoneVersion 追加一版单据编号时区，同键语义同 AppendRuleVersion。
func (store *Store) AppendTimeZoneVersion(ctx context.Context, version docnumber.TimeZoneVersion) (docnumber.TimeZoneVersion, bool, error) {
	executor, err := store.db.RequireExecutor(ctx)
	if err != nil {
		return docnumber.TimeZoneVersion{}, false, fmt.Errorf("append numbering time zone version: %w", err)
	}
	stored, err := scanTimeZone(executor.QueryRow(ctx, `
		INSERT INTO document_numbering.numbering_time_zone_version (`+timeZoneColumns+`)
		VALUES ($1, $2, $3, $4, $5)
		ON CONFLICT (tenant_id, effective_from) DO NOTHING
		RETURNING `+timeZoneColumns,
		version.Tenant, version.TimeZone, version.EffectiveFrom, version.RegisteredBy, version.RegisteredAt))
	if err == nil {
		return stored, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return docnumber.TimeZoneVersion{}, false, fmt.Errorf("append numbering time zone version: %w", err)
	}
	existing, err := scanTimeZone(executor.QueryRow(ctx, `
		SELECT `+timeZoneColumns+` FROM document_numbering.numbering_time_zone_version
		WHERE tenant_id = $1 AND effective_from = $2`,
		version.Tenant, version.EffectiveFrom))
	if err != nil {
		return docnumber.TimeZoneVersion{}, false, fmt.Errorf("read the numbering time zone version holding that moment: %w", err)
	}
	return existing, false, nil
}

func (store *Store) EffectiveTimeZoneVersion(ctx context.Context, tenant string, at time.Time) (docnumber.TimeZoneVersion, bool, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return docnumber.TimeZoneVersion{}, false, fmt.Errorf("read effective numbering time zone: %w", err)
	}
	version, err := scanTimeZone(querier.QueryRow(ctx, `
		SELECT `+timeZoneColumns+` FROM document_numbering.numbering_time_zone_version
		WHERE tenant_id = $1 AND effective_from <= $2
		ORDER BY effective_from DESC
		LIMIT 1`,
		tenant, at))
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return docnumber.TimeZoneVersion{}, false, nil
	case err != nil:
		return docnumber.TimeZoneVersion{}, false, fmt.Errorf("read effective numbering time zone: %w", err)
	}
	return version, true, nil
}

func (store *Store) TimeZoneVersions(ctx context.Context, tenant string) ([]docnumber.TimeZoneVersion, error) {
	querier, err := store.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("read numbering time zone versions: %w", err)
	}
	rows, err := querier.Query(ctx, `
		SELECT `+timeZoneColumns+` FROM document_numbering.numbering_time_zone_version
		WHERE tenant_id = $1
		ORDER BY effective_from`, tenant)
	if err != nil {
		return nil, fmt.Errorf("read numbering time zone versions: %w", err)
	}
	defer rows.Close()
	var versions []docnumber.TimeZoneVersion
	for rows.Next() {
		version, err := scanTimeZone(rows)
		if err != nil {
			return nil, fmt.Errorf("read numbering time zone versions: %w", err)
		}
		versions = append(versions, version)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("read numbering time zone versions: %w", err)
	}
	return versions, nil
}

func scanRule(row pgx.Row) (docnumber.RuleVersion, error) {
	var version docnumber.RuleVersion
	var code string
	if err := row.Scan(&version.Tenant, &code, &version.Template, &version.EffectiveFrom, &version.RegisteredBy, &version.RegisteredAt); err != nil {
		return docnumber.RuleVersion{}, err
	}
	version.Type = docnumber.TypeCode(code)
	version.EffectiveFrom, version.RegisteredAt = version.EffectiveFrom.UTC(), version.RegisteredAt.UTC()
	return version, nil
}

func scanTimeZone(row pgx.Row) (docnumber.TimeZoneVersion, error) {
	var version docnumber.TimeZoneVersion
	if err := row.Scan(&version.Tenant, &version.TimeZone, &version.EffectiveFrom, &version.RegisteredBy, &version.RegisteredAt); err != nil {
		return docnumber.TimeZoneVersion{}, err
	}
	version.EffectiveFrom, version.RegisteredAt = version.EffectiveFrom.UTC(), version.RegisteredAt.UTC()
	return version, nil
}
