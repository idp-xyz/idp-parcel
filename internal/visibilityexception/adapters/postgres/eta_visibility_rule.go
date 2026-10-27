package postgres

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// ETAVisibilityRules 实现 ports.ETAVisibilityRuleView：这个租户的这个货主客户账户在判断时点适用哪一条
// ETA 对客规则（0031）。
//
// 内容属租户取值（`PAR-VIS-03` 与 `PAR-COM-07` 待提供），实现不属：没有适用行如实交回「未配置」，同一时点
// 适用两行如实交回「冲突」——都是确定的业务答案，不是错误，不挑一条顶上，也没有默认值可发明。依赖调不通、
// 行读回后过不了领域不变量才作为 error 返回，由应用层形成未决。适用按判断时点选，不按查询当下：
// 规则换版只作用于生效后的新判断。
type ETAVisibilityRules struct {
	db *bentopg.DB
}

func NewETAVisibilityRules(db *bentopg.DB) (*ETAVisibilityRules, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &ETAVisibilityRules{db: db}, nil
}

var _ ports.ETAVisibilityRuleView = (*ETAVisibilityRules)(nil)

func (view *ETAVisibilityRules) AssessETAVisibilityRule(
	ctx context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	at time.Time,
) (domain.ETAVisibilityRuleBasis, error) {
	if tenant.String() == "" || customer.String() == "" || at.IsZero() {
		return domain.UnconfiguredETARule(), nil
	}
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	rows, err := querier.Query(ctx,
		`SELECT service_rule_version, gate_version, target_milestone_ref,
		        qualifying_confidences, expression, stop_boundary_ref
		   FROM visibility_exception.eta_visibility_rule
		  WHERE tenant_id = $1
		    AND customer_account_ref = $2
		    AND effective_from <= $3
		    AND (effective_to IS NULL OR effective_to > $3)
		  LIMIT 2`,
		tenant.String(), customer.String(), at.UTC(),
	)
	if err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	type ruleRow struct {
		service, gate, milestone, expression, boundary string
		qualifying                                     []string
	}
	found, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (ruleRow, error) {
		var collected ruleRow
		err := row.Scan(&collected.service, &collected.gate, &collected.milestone,
			&collected.qualifying, &collected.expression, &collected.boundary)
		return collected, err
	})
	if err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	switch len(found) {
	case 0:
		return domain.UnconfiguredETARule(), nil
	case 1:
	default:
		return domain.ConflictingETARules(), nil
	}

	row := found[0]
	spec := domain.ETAVisibilityRuleSpec{}
	if spec.ServiceRule, err = domain.NewCustomerServiceRuleVersion(row.service); err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	if spec.GateVersion, err = domain.NewETAQualityGateVersion(row.gate); err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	if spec.TargetMilestone, err = domain.NewMilestoneReference(row.milestone); err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	for _, raw := range row.qualifying {
		confidence, err := domain.NewConfidenceReference(raw)
		if err != nil {
			return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
		}
		spec.Qualifying = append(spec.Qualifying, confidence)
	}
	if spec.Expression, err = etaExpressionFrom(row.expression); err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	if spec.StopBoundary, err = domain.NewETAStopShowingBoundary(row.boundary); err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	rule, err := domain.NewETAVisibilityRule(spec)
	if err != nil {
		return domain.ETAVisibilityRuleBasis{}, fmt.Errorf("assess eta visibility rule: %w", err)
	}
	return domain.ConfiguredETARule(rule), nil
}
