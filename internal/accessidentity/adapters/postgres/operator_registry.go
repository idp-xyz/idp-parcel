// Package postgres 是 accessidentity 登记册在 PostgreSQL 上的适配器，表在 access_identity schema。
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

// OperatorRegistry 实现操作者册的登记口与装载口（access_identity 0001、ADR-0100 决定二第三条）。
// 撞键从不覆盖：同内容重放答原结果，异内容答冲突，册上已有的那一行原样不动。
type OperatorRegistry struct {
	db *bentopg.DB
}

func NewOperatorRegistry(db *bentopg.DB) (*OperatorRegistry, error) {
	if db == nil {
		return nil, fmt.Errorf("access identity postgres: db is nil")
	}
	return &OperatorRegistry{db: db}, nil
}

var (
	_ accessidentity.OperatorRegistry  = (*OperatorRegistry)(nil)
	_ accessidentity.OperatorRegistrar = (*OperatorRegistry)(nil)
)

func (registry *OperatorRegistry) RegisterOperator(
	ctx context.Context,
	binding accessidentity.OperatorBinding,
) (accessidentity.OperatorRegistrationOutcome, error) {
	const operation = "register operator"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	subject := binding.Subject()
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.operator (issuer, subject, tenant_id, basis_ref)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		subject.Issuer(), subject.Subject(), binding.TenantID(), binding.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.OperatorRegistrationRecorded, nil
	}

	var tenant, basis string
	err = executor.QueryRow(ctx,
		`SELECT tenant_id, basis_ref
		   FROM access_identity.operator
		  WHERE issuer = $1 AND subject = $2`,
		subject.Issuer(), subject.Subject(),
	).Scan(&tenant, &basis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	switch {
	case tenant != binding.TenantID():
		return accessidentity.OperatorSubjectBoundToAnotherTenant, nil
	case basis == binding.Basis():
		return accessidentity.OperatorRegistrationAlreadyRegistered, nil
	default:
		return accessidentity.OperatorRegistrationContentConflict, nil
	}
}

// RegisterGrant 先查本租户册上有没有这个主体再写：外键同样挡得住，但让它报错会把整个事务打成
// 失败态，调用方就没法在同一事务里拿到一个「未登记」的答复。只增的册里绑定不会消失，先查后写
// 之间没有可钻的窗口。
func (registry *OperatorRegistry) RegisterGrant(
	ctx context.Context,
	grant accessidentity.OperatorGrant,
) (accessidentity.OperatorRegistrationOutcome, error) {
	const operation = "register operator grant"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	subject := grant.Subject()
	var bound bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.operator
			 WHERE issuer = $1 AND subject = $2 AND tenant_id = $3)`,
		subject.Issuer(), subject.Subject(), grant.TenantID(),
	).Scan(&bound); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !bound {
		return accessidentity.OperatorNotRegistered, nil
	}

	startsAt := grant.Interval().StartsAt()
	var endsAt *time.Time
	if end, bounded := grant.Interval().EndsAt(); bounded {
		endsAt = &end
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.operator_grant
			(tenant_id, grant_id, issuer, subject, capability_face,
			 effective_starts_at, effective_ends_at, basis_ref, decision_kind)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)
		 ON CONFLICT DO NOTHING`,
		grant.TenantID(), grant.GrantID(), subject.Issuer(), subject.Subject(), grant.Face().String(),
		startsAt, endsAt, grant.Basis(), optionalDecisionKind(grant.DecisionKind()),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.OperatorRegistrationRecorded, nil
	}

	var (
		storedIssuer, storedSubject, storedFace, storedBasis string
		storedStartsAt                                       time.Time
		storedEndsAt                                         *time.Time
		storedKind                                           *string
	)
	err = executor.QueryRow(ctx,
		`SELECT issuer, subject, capability_face, effective_starts_at, effective_ends_at, basis_ref, decision_kind
		   FROM access_identity.operator_grant
		  WHERE tenant_id = $1 AND grant_id = $2`,
		grant.TenantID(), grant.GrantID(),
	).Scan(&storedIssuer, &storedSubject, &storedFace, &storedStartsAt, &storedEndsAt, &storedBasis, &storedKind)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	same := storedIssuer == subject.Issuer() &&
		storedSubject == subject.Subject() &&
		storedFace == grant.Face().String() &&
		sameOptionalText(storedKind, optionalDecisionKind(grant.DecisionKind())) &&
		sameInstant(storedStartsAt, startsAt) &&
		sameOptionalInstant(storedEndsAt, endsAt) &&
		storedBasis == grant.Basis()
	if same {
		return accessidentity.OperatorRegistrationAlreadyRegistered, nil
	}
	return accessidentity.OperatorRegistrationContentConflict, nil
}

// RegisterRevocation 先查本租户册上有没有这笔授予，理由同 RegisterGrant。按（租户、授予标识）查，
// 别的租户拿同一个授予标识撤不动这一笔。
func (registry *OperatorRegistry) RegisterRevocation(
	ctx context.Context,
	revocation accessidentity.GrantRevocation,
) (accessidentity.OperatorRegistrationOutcome, error) {
	const operation = "register operator grant revocation"
	executor, err := registry.db.RequireExecutor(ctx)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	var granted bool
	if err := executor.QueryRow(ctx,
		`SELECT EXISTS (
			SELECT 1 FROM access_identity.operator_grant
			 WHERE tenant_id = $1 AND grant_id = $2)`,
		revocation.TenantID(), revocation.GrantID(),
	).Scan(&granted); err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if !granted {
		return accessidentity.OperatorGrantNotRegistered, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO access_identity.operator_grant_revocation (tenant_id, grant_id, revoked_at, basis_ref)
		 VALUES ($1, $2, $3, $4)
		 ON CONFLICT DO NOTHING`,
		revocation.TenantID(), revocation.GrantID(), revocation.RevokedAt(), revocation.Basis(),
	)
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if tag.RowsAffected() > 0 {
		return accessidentity.OperatorRegistrationRecorded, nil
	}

	var (
		storedRevokedAt time.Time
		storedBasis     string
	)
	err = executor.QueryRow(ctx,
		`SELECT revoked_at, basis_ref
		   FROM access_identity.operator_grant_revocation
		  WHERE tenant_id = $1 AND grant_id = $2`,
		revocation.TenantID(), revocation.GrantID(),
	).Scan(&storedRevokedAt, &storedBasis)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", fmt.Errorf("%s: 撞键后读不回既有行", operation)
	}
	if err != nil {
		return "", fmt.Errorf("%s: %w", operation, err)
	}
	if sameInstant(storedRevokedAt, revocation.RevokedAt()) && storedBasis == revocation.Basis() {
		return accessidentity.OperatorRegistrationAlreadyRegistered, nil
	}
	return accessidentity.OperatorRegistrationContentConflict, nil
}

// FindOperator 用一条查询取绑定与名下全部授予（连同撤销）：分两次查会让绑定与授予取自两个时点。
//
// 此刻生不生效不在这里判——那是一份会过期的推导，固化进读口就等于替调用方答了；调用方拿
// OperatorStanding 对自己的时点判。
func (registry *OperatorRegistry) FindOperator(
	ctx context.Context,
	subject accessidentity.OperatorSubject,
) (accessidentity.OperatorStanding, bool, error) {
	const operation = "find operator"
	querier, err := registry.db.ReadExecutor(ctx)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	rows, err := querier.Query(ctx,
		`SELECT o.tenant_id, o.basis_ref,
		        g.grant_id, g.capability_face, g.effective_starts_at, g.effective_ends_at, g.basis_ref,
		        g.decision_kind, r.revoked_at, r.basis_ref
		   FROM access_identity.operator o
		   LEFT JOIN access_identity.operator_grant g
		     ON g.issuer = o.issuer AND g.subject = o.subject AND g.tenant_id = o.tenant_id
		   LEFT JOIN access_identity.operator_grant_revocation r
		     ON r.tenant_id = g.tenant_id AND r.grant_id = g.grant_id
		  WHERE o.issuer = $1 AND o.subject = $2
		  ORDER BY g.effective_starts_at, g.grant_id`,
		subject.Issuer(), subject.Subject(),
	)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	defer rows.Close()

	var (
		found   bool
		binding accessidentity.OperatorBinding
		grants  []accessidentity.RecordedGrant
	)
	for rows.Next() {
		var (
			tenant, basis               string
			grantID, face, grantBasis   *string
			startsAt, endsAt, revokedAt *time.Time
			decisionKind                *string
			revocationBasis             *string
		)
		if err := rows.Scan(&tenant, &basis,
			&grantID, &face, &startsAt, &endsAt, &grantBasis,
			&decisionKind, &revokedAt, &revocationBasis,
		); err != nil {
			return accessidentity.OperatorStanding{}, false, fmt.Errorf("%s: %w", operation, err)
		}
		if !found {
			binding, err = accessidentity.NewOperatorBinding(subject, tenant, basis)
			if err != nil {
				return accessidentity.OperatorStanding{}, false, fmt.Errorf("%s: %w", operation, err)
			}
			found = true
		}
		if grantID == nil {
			continue
		}
		recorded, err := recordedGrantFromRow(tenant, subject,
			*grantID, *face, decisionKind, *startsAt, endsAt, *grantBasis, revokedAt, revocationBasis)
		if err != nil {
			return accessidentity.OperatorStanding{}, false, fmt.Errorf("%s: 授予 %s：%w", operation, *grantID, err)
		}
		grants = append(grants, recorded)
	}
	if err := rows.Err(); err != nil {
		return accessidentity.OperatorStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	if !found {
		return accessidentity.OperatorStanding{}, false, nil
	}
	standing, err := accessidentity.NewOperatorStanding(binding, grants)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, fmt.Errorf("%s: %w", operation, err)
	}
	return standing, true, nil
}

// recordedGrantFromRow 经领域构造门重建一笔授予：库里的行照样要过门，门变严之后读得出哪一行已经
// 不合今天的规则，而不是把它当合格的授予交出去。
func recordedGrantFromRow(
	tenant string,
	subject accessidentity.OperatorSubject,
	grantID, rawFace string,
	rawDecisionKind *string,
	startsAt time.Time,
	endsAt *time.Time,
	basis string,
	revokedAt *time.Time,
	revocationBasis *string,
) (accessidentity.RecordedGrant, error) {
	face, err := accessidentity.ParseCapabilityFace(rawFace)
	if err != nil {
		return accessidentity.RecordedGrant{}, err
	}
	end := time.Time{}
	if endsAt != nil {
		end = *endsAt
	}
	interval, err := accessidentity.NewEffectiveInterval(startsAt, end)
	if err != nil {
		return accessidentity.RecordedGrant{}, err
	}
	var grant accessidentity.OperatorGrant
	if face == accessidentity.CapabilityOperationDecision {
		kind := ""
		if rawDecisionKind != nil {
			kind = *rawDecisionKind
		}
		decision, parseErr := accessidentity.ParseDecisionKind(kind)
		if parseErr != nil {
			return accessidentity.RecordedGrant{}, parseErr
		}
		grant, err = accessidentity.NewOperationDecisionGrant(tenant, grantID, subject, decision, interval, basis)
	} else {
		grant, err = accessidentity.NewOperatorGrant(tenant, grantID, subject, face, interval, basis)
	}
	if err != nil {
		return accessidentity.RecordedGrant{}, err
	}
	if revokedAt == nil {
		return accessidentity.NewRecordedGrant(grant, nil)
	}
	reference := ""
	if revocationBasis != nil {
		reference = *revocationBasis
	}
	revocation, err := accessidentity.NewGrantRevocation(tenant, grantID, *revokedAt, reference)
	if err != nil {
		return accessidentity.RecordedGrant{}, err
	}
	return accessidentity.NewRecordedGrant(grant, &revocation)
}

// sameInstant 按库的精度比对：timestamptz 只到微秒，驱动写入时截去微秒以下，同一份输入重放后
// 读回来会差那几百纳秒。按纳秒比，重放就被判成冲突。
func sameInstant(stored, given time.Time) bool {
	return stored.Equal(given.Truncate(time.Microsecond))
}

func sameOptionalInstant(stored, given *time.Time) bool {
	if stored == nil || given == nil {
		return stored == nil && given == nil
	}
	return sameInstant(*stored, *given)
}

// optionalDecisionKind 把「无决定种类」写成 NULL：列上的 CHECK 以 NULL 表示别的能力面。
func optionalDecisionKind(kind accessidentity.DecisionKind) *string {
	if kind == "" {
		return nil
	}
	value := kind.String()
	return &value
}

func sameOptionalText(stored, wanted *string) bool {
	if stored == nil || wanted == nil {
		return stored == nil && wanted == nil
	}
	return *stored == *wanted
}
