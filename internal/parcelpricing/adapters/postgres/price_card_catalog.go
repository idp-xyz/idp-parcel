package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// ErrAmbiguousPriceCard 说明同一方案身份在同一计价基准时点有两个适用版本。
//
// 它是错误而不是业务答案（先例：NR 目录的 ErrAmbiguousNetworkCatalog）：多份**不同**
// 方案同时适用是正当的多候选形态，但同一方案的两个版本同时适用意味着发布责任方没有
// 完成替代关系，挑任何一版都是替它作决定。数据要修登记册，不能靠挑一版把它藏起来。
var ErrAmbiguousPriceCard = errors.New(
	"parcel pricing postgres: 同一方案身份在该时点有多个适用版本")

// PriceCards 实现 ports.PriceCardCatalog（票 07 件①②）。它拥有价卡版本行的登记与
// 装载，不做评价也不择优——列面只承担键、方向隔离过滤与比对，权威内容在领域折装的
// 登记快照里，读回经领域整图重验。
type PriceCards struct {
	db *bentopg.DB
}

func NewPriceCards(db *bentopg.DB) (*PriceCards, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &PriceCards{db: db}, nil
}

// Register 登记一份已批准发布的价卡版本。行只增不改：同键第二份由主键拦住，再按
// （规范化版本 + 内容摘要）比对译成结果代数——同摘要是幂等重放，同规范化不同摘要是
// 版本内容冲突（CONTEXT：不得静默替换），规范化版本不同则摘要不可比（ADR-0014），
// 三种都不顶替原行。
//
// 走 RequireExecutor：登记与将来同一步的治理记录必须同生共死。
func (catalog *PriceCards) Register(
	ctx context.Context,
	registration domain.PriceCardRegistration,
) (ports.PriceCardRegistrationOutcome, error) {
	executor, err := catalog.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PriceCardRegistrationOutcomeInvalid, fmt.Errorf("register price card: %w", err)
	}

	snapshot, err := domain.MarshalPriceCardRegistration(registration)
	if err != nil {
		return ports.PriceCardRegistrationOutcomeInvalid, fmt.Errorf("register price card: %w", err)
	}

	plan := registration.Plan()
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.price_card_version
			(tenant_id, plan_id, plan_version, direction, purpose, scope,
			 rate_table_id, rate_table_version, effective_from, effective_to,
			 canonicalization, content_digest, source_file_name, source_file_sha256,
			 authorization_id, authorization_version, publication_approver, snapshot)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13, $14, $15, $16, $17, $18)
		 ON CONFLICT DO NOTHING`,
		registration.Tenant().String(),
		plan.Reference().ID(),
		plan.Reference().Version(),
		plan.Direction().String(),
		plan.Purpose().String(),
		plan.Scope().String(),
		plan.RateTable().Reference().ID(),
		plan.RateTable().Reference().Version(),
		plan.EffectivePeriod().StartsAt().UTC(),
		optionalPeriodEnd(plan.EffectivePeriod()),
		plan.CanonicalizationVersion(),
		plan.ContentDigest(),
		registration.SourceFile().Name(),
		registration.SourceFile().SHA256(),
		registration.DirectionAuthorization().ID(),
		registration.DirectionAuthorization().Version(),
		registration.PublicationApprover(),
		snapshot,
	)
	if err != nil {
		return ports.PriceCardRegistrationOutcomeInvalid, fmt.Errorf("register price card: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PriceCardRegistered, nil
	}

	var canonicalization, digest string
	err = executor.QueryRow(ctx,
		`SELECT canonicalization, content_digest
		   FROM parcel_pricing.price_card_version
		  WHERE tenant_id = $1 AND plan_id = $2 AND plan_version = $3`,
		registration.Tenant().String(), plan.Reference().ID(), plan.Reference().Version(),
	).Scan(&canonicalization, &digest)
	if err != nil {
		return ports.PriceCardRegistrationOutcomeInvalid, fmt.Errorf("register price card: 比对在册行：%w", err)
	}
	if canonicalization != plan.CanonicalizationVersion() {
		return ports.PriceCardCanonicalizationDiffers, nil
	}
	if digest != plan.ContentDigest() {
		return ports.PriceCardContentConflict, nil
	}
	return ports.PriceCardAlreadyRegistered, nil
}

// LoadApplicable 取（方向 + 适用范围 + 计价基准时点）下的全部适用价卡版本。方向
// 隔离在查询谓词上（BUY 的册面不进 SELL 的答案）；选版按 [effective_from,
// effective_to) 含 asOf。读回逐行经领域整图重验（含按规范化版本重算内容摘要自校），
// 再与比对列交叉核——列与快照分岔说明行被改过。
func (catalog *PriceCards) LoadApplicable(
	ctx context.Context,
	tenant domain.TenantID,
	direction domain.PricingDirection,
	scope domain.PricingScopeID,
	asOf time.Time,
) ([]domain.PricingPlanVersion, error) {
	if tenant.String() == "" || scope.String() == "" || asOf.IsZero() {
		return nil, fmt.Errorf("load price cards: tenant, scope and asOf are required")
	}
	querier, err := catalog.db.ReadExecutor(ctx)
	if err != nil {
		return nil, fmt.Errorf("load price cards: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT plan_id, plan_version, purpose, rate_table_id, rate_table_version,
		        effective_from, effective_to, canonicalization, content_digest,
		        source_file_sha256, snapshot
		   FROM parcel_pricing.price_card_version
		  WHERE tenant_id = $1 AND direction = $2 AND scope = $3
		    AND effective_from <= $4
		    AND (effective_to IS NULL OR effective_to > $4)
		  ORDER BY plan_id, plan_version`,
		tenant.String(), direction.String(), scope.String(), asOf.UTC(),
	)
	if err != nil {
		return nil, fmt.Errorf("load price cards: %w", err)
	}
	defer rows.Close()

	var plans []domain.PricingPlanVersion
	previousPlanID := ""
	for rows.Next() {
		var planID, planVersion, purpose, tableID, tableVersion string
		var canonicalization, digest, sourceSHA string
		var effectiveFrom time.Time
		var effectiveTo *time.Time
		var snapshot []byte
		if err := rows.Scan(&planID, &planVersion, &purpose, &tableID, &tableVersion,
			&effectiveFrom, &effectiveTo, &canonicalization, &digest, &sourceSHA, &snapshot); err != nil {
			return nil, fmt.Errorf("load price cards: %w", err)
		}
		// 同一方案身份两版同时适用：交回错误，不挑一版（行按 plan_id 有序，相邻
		// 比较即可）。
		if planID == previousPlanID {
			return nil, fmt.Errorf("%w：%s", ErrAmbiguousPriceCard, planID)
		}
		previousPlanID = planID

		registration, err := domain.RehydratePriceCardRegistration(snapshot)
		if err != nil {
			return nil, fmt.Errorf("load price cards: %s/%s：%w", planID, planVersion, err)
		}
		plan := registration.Plan()
		if registration.Tenant() != tenant ||
			plan.Reference().ID() != planID ||
			plan.Reference().Version() != planVersion ||
			plan.Direction() != direction ||
			plan.Purpose().String() != purpose ||
			plan.Scope() != scope ||
			plan.RateTable().Reference().ID() != tableID ||
			plan.RateTable().Reference().Version() != tableVersion ||
			plan.CanonicalizationVersion() != canonicalization ||
			plan.ContentDigest() != digest ||
			registration.SourceFile().SHA256() != sourceSHA ||
			!plan.EffectivePeriod().StartsAt().Equal(effectiveFrom) ||
			!periodEndMatches(plan.EffectivePeriod(), effectiveTo) {
			return nil, fmt.Errorf(
				"load price cards: comparison columns disagree with the snapshot for %s/%s", planID, planVersion)
		}
		plans = append(plans, plan)
	}
	if err := rows.Err(); err != nil {
		if errors.Is(err, pgx.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("load price cards: %w", err)
	}
	return plans, nil
}

// optionalPeriodEnd 把无上界适用期（endsAt 零值）折成 NULL 列。
func optionalPeriodEnd(period domain.EffectivePeriod) *time.Time {
	if period.EndsAt().IsZero() {
		return nil
	}
	end := period.EndsAt().UTC()
	return &end
}

func periodEndMatches(period domain.EffectivePeriod, column *time.Time) bool {
	if period.EndsAt().IsZero() {
		return column == nil
	}
	return column != nil && period.EndsAt().Equal(*column)
}
