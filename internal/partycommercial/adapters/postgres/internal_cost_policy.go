package postgres

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// LoadInternalCostPolicy 按版本身份点读价格政策正文（ports.InternalCostPolicyView）。整册装载是
// 闭包解析的取回方式（ADR-0057）；本口与 CreditPolicyContentView 同一条「点读口另立」的理由——
// 消费方持显式引用，为它装下整册不合比例。
//
// 快照走版本行同一份 versionDocument 重建，与整册装载共用一条重建门；政策列按保存时
// 同键关联读回。方向照登记原样交回，由消费侧桥拒译。
func (repository *CommercialPublications) LoadInternalCostPolicy(
	ctx context.Context,
	tenant domain.TenantID,
	objectID domain.CommercialObjectID,
	versionLabel domain.CommercialVersionLabel,
) (domain.CommercialPricePolicy, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}

	var raw []byte
	row := scannedPricePolicy{}
	err = querier.QueryRow(ctx,
		`SELECT version.snapshot,
		        price.direction, price.plan_ref, price.plan_direction, price.binding_conversion,
		        price.policy_scope_ref, price.effective_starts_at, price.effective_ends_at
		   FROM party_commercial.commercial_version AS version
		   LEFT JOIN party_commercial.commercial_price_policy AS price
		          ON price.tenant_id     = version.tenant_id
		         AND price.object_kind   = version.object_kind
		         AND price.object_id     = version.object_id
		         AND price.version_label = version.version_label
		  WHERE version.tenant_id = $1
		    AND version.object_kind = $2
		    AND version.object_id = $3
		    AND version.version_label = $4`,
		tenant.String(), uint8(domain.PriceRuleObject), objectID.String(), versionLabel.String(),
	).Scan(&raw,
		&row.direction, &row.planRef, &row.planDirection, &row.conversion,
		&row.scope, &row.startsAt, &row.endsAt)
	if errors.Is(err, pgx.ErrNoRows) {
		// 版本行本身没有：这版政策从未发布过。
		return domain.CommercialPricePolicy{}, false, nil
	}
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	if !row.present() {
		// 版本在而没登价格正文：政策版本可以只有规则没有正文（保存门只管正文自洽）。
		return domain.CommercialPricePolicy{}, false, nil
	}
	if !row.complete() {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: price policy row is incomplete")
	}

	var document versionDocument
	if err := json.Unmarshal(raw, &document); err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: 快照不是本适配器写下的形状：%w", err)
	}
	version, err := document.version()
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	if version.Status() != domain.CommercialVersionEffective {
		// 非生效版本带不出正文（正文保存门只放行生效版本），落到这里是引用侧拿了个没生效的
		// 版本身：如实当「没这份正文」答，续办与本口语义表一致。
		return domain.CommercialPricePolicy{}, false, nil
	}
	direction, err := priceDirectionFrom(*row.direction)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	planDirection, err := priceDirectionFrom(*row.planDirection)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	conversion, err := planBindingConversionFrom(*row.conversion)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	plan, err := domain.NewPricingPlanReference(*row.planRef)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	scope, err := domain.NewCommercialScopeReference(*row.scope)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	endsAt := time.Time{}
	if row.endsAt != nil {
		endsAt = *row.endsAt
	}
	interval, err := domain.NewEffectiveInterval(*row.startsAt, endsAt)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	policy, err := domain.NewCommercialPricePolicy(
		version, direction, plan, planDirection, conversion, scope, interval)
	if err != nil {
		return domain.CommercialPricePolicy{}, false, fmt.Errorf("load internal cost policy: %w", err)
	}
	return policy, true, nil
}
