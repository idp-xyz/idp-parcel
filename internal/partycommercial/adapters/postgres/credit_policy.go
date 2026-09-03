package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 信用政策册的持久化面（票 party-commercial-context-gaps/03，0020 迁移）。写口挂在
// CommercialPublications 上与其余正文册同笔登记；读口是独立的 CreditPolicyContents——正文
// 不进整册装载（LoadForScope），理由见 ports.CreditPolicyContentView。

// SaveCreditPolicy 登记一份信用政策版本的正文。撞键不覆盖：同内容是重放，异内容（含额度换格）
// 是需要商业责任方修正的冲突。
func (repository *CommercialPublications) SaveCreditPolicy(
	ctx context.Context,
	policy domain.CreditPolicy,
) (ports.CreditPolicySaveOutcome, error) {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return ports.CreditPolicySaveOutcomeInvalid, fmt.Errorf("save credit policy: %w", err)
	}

	version := policy.Version()
	limitMinor, limitBps := creditLimitColumns(policy.AuthorizedLimit())
	if limitMinor == nil && limitBps == nil {
		// 零值 CreditLimit 过不了 NewCreditPolicy，走到这里只可能是绕开构造门的零值政策。
		// 拦在 INSERT 前，否则 CHECK 会以一条技术错误报出一件领域上早该拒绝的事。
		return ports.CreditPolicySaveOutcomeInvalid, fmt.Errorf("save credit policy: %w", domain.ErrInvalidCreditPolicy)
	}
	startsAt, endsAt := intervalColumns(policy.Effective())

	tag, err := executor.Exec(ctx,
		`INSERT INTO party_commercial.credit_policy
			(tenant_id, object_kind, object_id, version_label,
			 legal_entity_ref, authority_level_ref, charge_type_ref,
			 limit_minor, limit_ratio_bps, effective_starts_at, effective_ends_at)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		 ON CONFLICT DO NOTHING`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
		policy.LegalEntity().String(),
		policy.Level().String(),
		policy.ChargeType().String(),
		limitMinor,
		limitBps,
		startsAt,
		endsAt,
	)
	if err != nil {
		return ports.CreditPolicySaveOutcomeInvalid, fmt.Errorf("save credit policy: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return ports.CreditPolicySaved, nil
	}

	var existing scannedCreditPolicy
	err = executor.QueryRow(ctx,
		`SELECT legal_entity_ref, authority_level_ref, charge_type_ref,
		        limit_minor, limit_ratio_bps, effective_starts_at, effective_ends_at
		   FROM party_commercial.credit_policy
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		version.Tenant().String(),
		uint8(version.Kind()),
		version.ObjectID().String(),
		version.Version().String(),
	).Scan(&existing.legalEntity, &existing.level, &existing.chargeType,
		&existing.limitMinor, &existing.limitBps, &existing.startsAt, &existing.endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return ports.CreditPolicySaveOutcomeInvalid, fmt.Errorf("save credit policy: 撞键后读不回既有行")
	}
	if err != nil {
		return ports.CreditPolicySaveOutcomeInvalid, fmt.Errorf("save credit policy: %w", err)
	}
	if existing.legalEntity == policy.LegalEntity().String() &&
		existing.level == policy.Level().String() &&
		existing.chargeType == policy.ChargeType().String() &&
		sameOptionalInt64(existing.limitMinor, limitMinor) &&
		sameOptionalInt64(existing.limitBps, limitBps) &&
		existing.startsAt.Equal(startsAt) &&
		sameOptionalTime(existing.endsAt, endsAt) {
		return ports.CreditPolicyAlreadyRegistered, nil
	}
	return ports.CreditPolicyContentConflict, nil
}

type scannedCreditPolicy struct {
	legalEntity string
	level       string
	chargeType  string
	limitMinor  *int64
	limitBps    *int64
	startsAt    time.Time
	endsAt      *time.Time
}

// creditLimitColumns 把两格封闭的额度摊成两列，恰一非空——列上 CHECK 是这一条的镜像。
func creditLimitColumns(limit domain.CreditLimit) (*int64, *int64) {
	if minor, ok := limit.AmountMinor(); ok {
		return &minor, nil
	}
	if bps, ok := limit.RatioBasisPoints(); ok {
		return nil, &bps
	}
	return nil, nil
}

// creditLimitFrom 把两列折回领域构造门。CHECK 保证恰一列在场，走到两空/两满说明库与领域
// 已经分叉，报错不吸收。
func creditLimitFrom(limitMinor, limitBps *int64) (domain.CreditLimit, error) {
	switch {
	case limitMinor != nil && limitBps == nil:
		return domain.NewCreditAmountLimit(*limitMinor)
	case limitMinor == nil && limitBps != nil:
		return domain.NewCreditRatioLimit(*limitBps)
	default:
		return domain.CreditLimit{}, fmt.Errorf("credit limit row is neither an amount nor a ratio")
	}
}

func intervalColumns(interval domain.EffectiveInterval) (time.Time, *time.Time) {
	var endsAt *time.Time
	if end, bounded := interval.EndsAt(); bounded {
		utc := end.UTC()
		endsAt = &utc
	}
	return interval.StartsAt().UTC(), endsAt
}

func intervalFrom(startsAt time.Time, endsAt *time.Time) (domain.EffectiveInterval, error) {
	end := time.Time{}
	if endsAt != nil {
		end = *endsAt
	}
	return domain.NewEffectiveInterval(startsAt, end)
}

func sameOptionalInt64(left, right *int64) bool {
	return (left == nil && right == nil) ||
		(left != nil && right != nil && *left == *right)
}

// CreditPolicyContents 实现 ports.CreditPolicyContentView：按已唯一选出的信用政策版本取回正文。
//
// 只读。正文属实例半边，本适配器不提供写口（写口在 CommercialPublications 上随发布同笔），
// 也不在读不到时代拟任何额度——缺政策既不是无限信用也不是零额度。
type CreditPolicyContents struct {
	db *bentopg.DB
}

func NewCreditPolicyContents(db *bentopg.DB) (*CreditPolicyContents, error) {
	if db == nil {
		return nil, fmt.Errorf("party commercial postgres: db is nil")
	}
	return &CreditPolicyContents{db: db}, nil
}

var _ ports.CreditPolicyContentView = (*CreditPolicyContents)(nil)

// LoadCreditPolicy 取回信用政策正文。
//
// found=false = 正文未登记（无行）。显式租户与版本必须同一身份，否则 error 且不交内容
// （ADR-0003/0040，租户是身份不是过滤器）。读回的每一行都过 NewCreditPolicy 重建，不按列
// 直接拼结构体——构造门是「什么算合法」的单一权威，绕过它库里一行坏数据就会变成一份合法额度。
func (repository *CreditPolicyContents) LoadCreditPolicy(
	ctx context.Context,
	tenant domain.TenantID,
	policy domain.CommercialVersion,
) (domain.CreditPolicy, bool, error) {
	none := domain.CreditPolicy{}
	if tenant.String() == "" ||
		policy.ObjectID().String() == "" || policy.Version().String() == "" {
		return none, false, fmt.Errorf("load credit policy: tenant and policy identity are required")
	}
	if tenant != policy.Tenant() {
		return none, false, fmt.Errorf("load credit policy: tenant does not own this policy")
	}
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return none, false, fmt.Errorf("load credit policy: %w", err)
	}

	var row scannedCreditPolicy
	err = querier.QueryRow(ctx,
		`SELECT legal_entity_ref, authority_level_ref, charge_type_ref,
		        limit_minor, limit_ratio_bps, effective_starts_at, effective_ends_at
		   FROM party_commercial.credit_policy
		  WHERE tenant_id = $1 AND object_kind = $2 AND object_id = $3 AND version_label = $4`,
		tenant.String(),
		uint8(domain.CreditPolicyObject),
		policy.ObjectID().String(),
		policy.Version().String(),
	).Scan(&row.legalEntity, &row.level, &row.chargeType,
		&row.limitMinor, &row.limitBps, &row.startsAt, &row.endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}
	if err != nil {
		return none, false, fmt.Errorf("load credit policy: %w", err)
	}

	content, err := creditPolicyFrom(policy, row)
	if err != nil {
		return none, false, fmt.Errorf("load credit policy: %w", err)
	}
	return content, true, nil
}

func creditPolicyFrom(version domain.CommercialVersion, row scannedCreditPolicy) (domain.CreditPolicy, error) {
	legalEntity, err := domain.NewLegalEntityReference(row.legalEntity)
	if err != nil {
		return domain.CreditPolicy{}, err
	}
	level, err := domain.NewAuthorityLevel(row.level)
	if err != nil {
		return domain.CreditPolicy{}, err
	}
	chargeType, err := domain.NewChargeTypeReference(row.chargeType)
	if err != nil {
		return domain.CreditPolicy{}, err
	}
	limit, err := creditLimitFrom(row.limitMinor, row.limitBps)
	if err != nil {
		return domain.CreditPolicy{}, err
	}
	interval, err := intervalFrom(row.startsAt, row.endsAt)
	if err != nil {
		return domain.CreditPolicy{}, err
	}
	return domain.NewCreditPolicy(version, legalEntity, level, chargeType, limit, interval)
}

// ListCreditPolicies 上列信用政策册。额度两列照列转写，HasAmount 说明哪一列在场；两空/两满
// 是库与领域分叉的坏数据，上抛不吸收。
func (catalogue *OperationsCatalogue) ListCreditPolicies(
	ctx context.Context,
	tenant domain.TenantID,
	limit int,
) ([]ports.CreditPolicyRow, error) {
	if err := requirePositiveLimit("list credit policies", limit); err != nil {
		return nil, err
	}
	querier, err := catalogue.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("list credit policies: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT object_id, version_label, legal_entity_ref, authority_level_ref, charge_type_ref,
		        limit_minor, limit_ratio_bps,
		        effective_starts_at, effective_ends_at, registered_at
		   FROM party_commercial.credit_policy
		  WHERE tenant_id = $1
		  ORDER BY registered_at DESC, object_id, version_label
		  LIMIT $2`,
		tenant.String(),
		limit,
	)
	if err != nil {
		return nil, fmt.Errorf("list credit policies: %w", err)
	}
	defer rows.Close()

	policyRows := make([]ports.CreditPolicyRow, 0, limit)
	for rows.Next() {
		var row ports.CreditPolicyRow
		var limitMinor, limitBps *int64
		var endsAt *time.Time
		if err := rows.Scan(
			&row.ObjectID, &row.VersionLabel, &row.LegalEntity, &row.AuthorityLevel, &row.ChargeType,
			&limitMinor, &limitBps,
			&row.EffectiveStartsAt, &endsAt, &row.RegisteredAt,
		); err != nil {
			return nil, fmt.Errorf("list credit policies: %w", err)
		}
		switch {
		case limitMinor != nil && limitBps == nil:
			row.HasAmount = true
			row.LimitMinor = *limitMinor
		case limitMinor == nil && limitBps != nil:
			row.LimitRatioBasisPoints = *limitBps
		default:
			return nil, fmt.Errorf("list credit policies: %s/%s 的额度既不是金额也不是比例",
				row.ObjectID, row.VersionLabel)
		}
		if endsAt != nil {
			row.EffectiveEndsAt = *endsAt
			row.HasEffectiveEnd = true
		}
		policyRows = append(policyRows, row)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("list credit policies: %w", err)
	}
	return policyRows, nil
}
