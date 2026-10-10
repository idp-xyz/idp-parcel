package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// PriceCardApprovalDutyRules 实现 ports.PriceCardApprovalDutyRuleRegistry：一租户一条价卡发布审批职责规则（0012；
// ADR-0101 决定六）。行属实例半边，写口今天只给装配与测试用。
type PriceCardApprovalDutyRules struct {
	db *bentopg.DB
}

var _ ports.PriceCardApprovalDutyRuleRegistry = (*PriceCardApprovalDutyRules)(nil)

func NewPriceCardApprovalDutyRules(db *bentopg.DB) (*PriceCardApprovalDutyRules, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &PriceCardApprovalDutyRules{db: db}, nil
}

// LoadPriceCardApprovalDutyRule 读该租户的规则；没有行即 found=false，由批准门答`未配置`。
func (rules *PriceCardApprovalDutyRules) LoadPriceCardApprovalDutyRule(
	ctx context.Context,
	tenant domain.TenantID,
) (domain.PriceCardApprovalDutyRule, bool, error) {
	querier, err := rules.db.ReadExecutor(ctx)
	if err != nil {
		return domain.PriceCardApprovalDutyRule{}, false, fmt.Errorf("load price card approval duty rule: %w", err)
	}
	var distinct bool
	var requiredGrant *string
	err = querier.QueryRow(ctx,
		`SELECT requires_distinct_subjects, required_grant
		   FROM parcel_pricing.price_card_approval_duty_rule
		  WHERE tenant_id = $1`,
		tenant.String(),
	).Scan(&distinct, &requiredGrant)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.PriceCardApprovalDutyRule{}, false, nil
	}
	if err != nil {
		return domain.PriceCardApprovalDutyRule{}, false, fmt.Errorf("load price card approval duty rule: %w", err)
	}
	rule, err := approvalDutyRuleOf(tenant, distinct, requiredGrant)
	if err != nil {
		return domain.PriceCardApprovalDutyRule{}, false, fmt.Errorf("load price card approval duty rule: %w", err)
	}
	return rule, true, nil
}

// SavePriceCardApprovalDutyRule 登记一条规则：同租户首登入册；再登同内容是重放，异内容答冲突、原行不被顶替。
func (rules *PriceCardApprovalDutyRules) SavePriceCardApprovalDutyRule(
	ctx context.Context,
	rule domain.PriceCardApprovalDutyRule,
) (ports.PriceCardApprovalDutyRuleSaveOutcome, error) {
	executor, err := rules.db.RequireExecutor(ctx)
	if err != nil {
		return ports.PriceCardApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save price card approval duty rule: %w", err)
	}
	if rule.Tenant().String() == "" {
		return ports.PriceCardApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save price card approval duty rule: %w", domain.ErrInvalidPriceCardApprovalDutyRule)
	}
	var requiredGrant *string
	if grant, required := rule.RequiredGrant(); required {
		name := grant.String()
		requiredGrant = &name
	}
	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.price_card_approval_duty_rule (tenant_id, requires_distinct_subjects, required_grant)
		 VALUES ($1, $2, $3)
		 ON CONFLICT DO NOTHING`,
		rule.Tenant().String(), rule.RequiresDistinctSubjects(), requiredGrant,
	)
	if err != nil {
		return ports.PriceCardApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save price card approval duty rule: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.PriceCardApprovalDutyRuleSaved, nil
	}

	var distinct bool
	var recordedGrant *string
	if err := executor.QueryRow(ctx,
		`SELECT requires_distinct_subjects, required_grant
		   FROM parcel_pricing.price_card_approval_duty_rule
		  WHERE tenant_id = $1`,
		rule.Tenant().String(),
	).Scan(&distinct, &recordedGrant); err != nil {
		return ports.PriceCardApprovalDutyRuleSaveOutcomeInvalid, fmt.Errorf("save price card approval duty rule: 撞键后读册上那一行：%w", err)
	}
	if distinct == rule.RequiresDistinctSubjects() && sameOptional(recordedGrant, requiredGrant) {
		return ports.PriceCardApprovalDutyRuleAlreadyRegistered, nil
	}
	return ports.PriceCardApprovalDutyRuleContentConflict, nil
}

func approvalDutyRuleOf(tenant domain.TenantID, distinct bool, requiredGrant *string) (domain.PriceCardApprovalDutyRule, error) {
	var grant domain.OperatorGrant
	if requiredGrant != nil {
		var err error
		if grant, err = domain.NewOperatorGrant(*requiredGrant); err != nil {
			return domain.PriceCardApprovalDutyRule{}, err
		}
	}
	return domain.NewPriceCardApprovalDutyRule(tenant, distinct, grant)
}
