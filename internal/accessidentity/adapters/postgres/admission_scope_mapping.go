package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
)

// AdmissionScopeMappings 实现准入范围对照册的登记口与读口（access_identity 0005、ADR-0180 决定一、二）。撞键从不覆盖：
// 同内容重放答原结果，异内容答冲突；同键生效期重叠答 MAPPING_OVERLAPS。三格都不动册上已有的行。
type AdmissionScopeMappings struct {
	db *bentopg.DB
}

func NewAdmissionScopeMappings(db *bentopg.DB) (*AdmissionScopeMappings, error) {
	if db == nil {
		return nil, fmt.Errorf("access identity postgres: db is nil")
	}
	return &AdmissionScopeMappings{db: db}, nil
}

var (
	_ accessidentity.AdmissionScopeMappingReader    = (*AdmissionScopeMappings)(nil)
	_ accessidentity.AdmissionScopeMappingRegistrar = (*AdmissionScopeMappings)(nil)
)

// mappingQuerier 是取同键各行要的那一个方法：登记时在调用方的事务里取，铸信封前在读执行器上取，两处走同一段查询。
type mappingQuerier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

// lockMappingKey 把同一个键上的重叠判定与写入串起来（先例：可见性目录登记的事务级咨询锁）。没有它，两笔各自查得
// 「不重叠」再各自插入，重叠就这样进了库。锁不管绕过本登记口的直插；那种数据由读侧的
// ErrConflictingAdmissionScopeMappings 兜住，不任选一行。
func lockMappingKey(ctx context.Context, executor bentopg.Executor, tenantID string, requirement accessidentity.AdmissionRequirement) error {
	_, err := executor.Exec(ctx,
		`SELECT pg_advisory_xact_lock(hashtext('access_identity.admission_scope_mapping'), hashtext($1 || chr(31) || $2 || chr(31) || $3))`,
		tenantID, requirement.Capability, requirement.FactKind,
	)
	return err
}

func (mappings *AdmissionScopeMappings) RegisterAdmissionScopeMapping(
	ctx context.Context,
	mapping accessidentity.AdmissionScopeMapping,
) (accessidentity.AdmissionScopeMappingOutcome, error) {
	const operation = "register admission scope mapping"
	executor, err := mappings.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	requirement := mapping.Requirement()
	if err := lockMappingKey(ctx, executor, mapping.TenantID(), requirement); err != nil {
		return "", fmt.Errorf("%s: 取键锁：%w", operation, err)
	}

	startsAt := mapping.Interval().StartsAt()
	var endsAt *time.Time
	if end, bounded := mapping.Interval().EndsAt(); bounded {
		endsAt = &end
	}
	// 同标识的行先比：它已在册，重放与否、冲突与否由它答，轮不到重叠判定。
	if outcome, stored, err := compareStoredMapping(ctx, executor, mapping, startsAt, endsAt); err != nil || stored {
		if err != nil {
			return "", fmt.Errorf("%s: %w", operation, err)
		}
		return outcome, nil
	}

	existing, err := findMappings(ctx, executor, mapping.TenantID(), requirement)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	for _, recorded := range existing {
		if recorded.OverlapsWith(mapping) {
			return accessidentity.AdmissionScopeMappingOverlaps, nil
		}
	}

	// 键锁管不到「同一个标识登在另一个键上」的并发那一笔：撞上主键时不报错，回头按同标识的行答。
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.admission_scope_mapping
			(tenant_id, mapping_id, capability, fact_kind, object_scope, effective_starts_at, effective_ends_at, basis_ref)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		 ON CONFLICT DO NOTHING`,
		mapping.TenantID(), mapping.MappingID(), requirement.Capability, requirement.FactKind, mapping.ObjectScope(),
		startsAt, endsAt, mapping.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.AdmissionScopeMappingRecorded, nil
	}
	outcome, stored, err := compareStoredMapping(ctx, executor, mapping, startsAt, endsAt)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !stored {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	return outcome, nil
}

// compareStoredMapping 按（租户、对照标识）读册上的那一行与待登的比：同内容答重放，异内容答冲突；那一行不在册答 stored=false。
func compareStoredMapping(
	ctx context.Context,
	executor bentopg.Executor,
	mapping accessidentity.AdmissionScopeMapping,
	startsAt time.Time,
	endsAt *time.Time,
) (accessidentity.AdmissionScopeMappingOutcome, bool, error) {
	var (
		storedCapability, storedFact, storedScope, storedBasis string
		storedStartsAt                                         time.Time
		storedEndsAt                                           *time.Time
	)
	err := executor.QueryRow(ctx,
		`SELECT capability, fact_kind, object_scope, effective_starts_at, effective_ends_at, basis_ref
		   FROM access_identity.admission_scope_mapping
		  WHERE tenant_id = $1 AND mapping_id = $2`,
		mapping.TenantID(), mapping.MappingID(),
	).Scan(&storedCapability, &storedFact, &storedScope, &storedStartsAt, &storedEndsAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	requirement := mapping.Requirement()
	same := storedCapability == requirement.Capability &&
		storedFact == requirement.FactKind &&
		storedScope == mapping.ObjectScope() &&
		sameInstant(storedStartsAt, startsAt) &&
		sameOptionalInstant(storedEndsAt, endsAt) &&
		storedBasis == mapping.Basis()
	if same {
		return accessidentity.AdmissionScopeMappingAlreadyRegistered, true, nil
	}
	return accessidentity.AdmissionScopeMappingContentConflict, true, nil
}

// RegisterAdmissionScopeMappingEnd 先查本租户册上有没有这一行再写，理由同 OperatorRegistry.RegisterGrant：让外键报错会把
// 事务打成失败态，调用方就拿不到「未登记」这一格答复。按（租户、对照标识）查，别的租户拿同一个标识结束不了这一行。
// 结束也取同一把键锁：它与同键的登记交错时，登记那一笔的重叠判定要么看得见这笔结束，要么排在它前面。
func (mappings *AdmissionScopeMappings) RegisterAdmissionScopeMappingEnd(
	ctx context.Context,
	end accessidentity.AdmissionScopeMappingEnd,
) (accessidentity.AdmissionScopeMappingOutcome, error) {
	const operation = "register admission scope mapping end"
	executor, err := mappings.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var requirement accessidentity.AdmissionRequirement
	err = executor.QueryRow(ctx,
		`SELECT capability, fact_kind FROM access_identity.admission_scope_mapping
		  WHERE tenant_id = $1 AND mapping_id = $2`,
		end.TenantID(), end.MappingID(),
	).Scan(&requirement.Capability, &requirement.FactKind)
	if errors.Is(err, pgx.ErrNoRows) {
		return accessidentity.AdmissionScopeMappingNotRegistered, nil
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if err := lockMappingKey(ctx, executor, end.TenantID(), requirement); err != nil {
		return "", fmt.Errorf("%s: 取键锁：%w", operation, err)
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.admission_scope_mapping_end (tenant_id, mapping_id, ended_at, basis_ref)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		end.TenantID(), end.MappingID(), end.EndedAt(), end.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.AdmissionScopeMappingRecorded, nil
	}

	var (
		storedEndedAt time.Time
		storedBasis   string
	)
	err = executor.QueryRow(ctx,
		`SELECT ended_at, basis_ref FROM access_identity.admission_scope_mapping_end
		  WHERE tenant_id = $1 AND mapping_id = $2`,
		end.TenantID(), end.MappingID(),
	).Scan(&storedEndedAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if sameInstant(storedEndedAt, end.EndedAt()) && storedBasis == end.Basis() {
		return accessidentity.AdmissionScopeMappingAlreadyRegistered, nil
	}
	return accessidentity.AdmissionScopeMappingContentConflict, nil
}

// FindAdmissionScopeMappings 用一条查询取这个键的全部行与各自的结束，理由同 FindOperator：分两次查会让对照与结束取自两个
// 时点。此刻哪一行生效不在这里判。
func (mappings *AdmissionScopeMappings) FindAdmissionScopeMappings(
	ctx context.Context,
	tenantID string,
	requirement accessidentity.AdmissionRequirement,
) ([]accessidentity.RecordedAdmissionScopeMapping, error) {
	const operation = "find admission scope mappings"
	querier, err := mappings.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	found, err := findMappings(ctx, querier, tenantID, requirement)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", operation, err)
	}
	return found, nil
}

func findMappings(
	ctx context.Context,
	querier mappingQuerier,
	tenantID string,
	requirement accessidentity.AdmissionRequirement,
) ([]accessidentity.RecordedAdmissionScopeMapping, error) {
	rows, err := querier.Query(ctx,
		`SELECT m.mapping_id, m.object_scope, m.effective_starts_at, m.effective_ends_at, m.basis_ref,
		        e.ended_at, e.basis_ref
		   FROM access_identity.admission_scope_mapping m
		   LEFT JOIN access_identity.admission_scope_mapping_end e
		     ON e.tenant_id = m.tenant_id AND e.mapping_id = m.mapping_id
		  WHERE m.tenant_id = $1 AND m.capability = $2 AND m.fact_kind = $3
		  ORDER BY m.effective_starts_at, m.mapping_id`,
		tenantID, requirement.Capability, requirement.FactKind,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var found []accessidentity.RecordedAdmissionScopeMapping
	for rows.Next() {
		var (
			mappingID, scope, basis string
			startsAt                time.Time
			endsAt, endedAt         *time.Time
			endBasis                *string
		)
		if err := rows.Scan(&mappingID, &scope, &startsAt, &endsAt, &basis, &endedAt, &endBasis); err != nil {
			return nil, err
		}
		recorded, err := recordedMappingFromRow(tenantID, requirement, mappingID, scope, startsAt, endsAt, basis, endedAt, endBasis)
		if err != nil {
			return nil, fmt.Errorf("对照 %s：%w", mappingID, err)
		}
		found = append(found, recorded)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return found, nil
}

// recordedMappingFromRow 经领域构造门重建，理由同 recordedGrantFromRow：门变严之后读得出哪一行已经不合今天的规则。
func recordedMappingFromRow(
	tenantID string,
	requirement accessidentity.AdmissionRequirement,
	mappingID, scope string,
	startsAt time.Time,
	endsAt *time.Time,
	basis string,
	endedAt *time.Time,
	endBasis *string,
) (accessidentity.RecordedAdmissionScopeMapping, error) {
	upper := time.Time{}
	if endsAt != nil {
		upper = *endsAt
	}
	interval, err := accessidentity.NewEffectiveInterval(startsAt, upper)
	if err != nil {
		return accessidentity.RecordedAdmissionScopeMapping{}, err
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping(tenantID, mappingID, requirement, scope, interval, basis)
	if err != nil {
		return accessidentity.RecordedAdmissionScopeMapping{}, err
	}
	if endedAt == nil {
		return accessidentity.NewRecordedAdmissionScopeMapping(mapping, nil)
	}
	reference := ""
	if endBasis != nil {
		reference = *endBasis
	}
	end, err := accessidentity.NewAdmissionScopeMappingEnd(tenantID, mappingID, *endedAt, reference)
	if err != nil {
		return accessidentity.RecordedAdmissionScopeMapping{}, err
	}
	return accessidentity.NewRecordedAdmissionScopeMapping(mapping, &end)
}
