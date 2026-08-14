package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// InterpretationRuleView 实现 ports.InterpretationRuleView：取某结果层的解释规则。
//
// found=false 即该层规则未配置——实例半边未提供时解释停在未决。这里没有任何兜底口径：
// 一个「默认按成功解释」的分支会让未知监管语义被读成放行，而 CONTEXT 硬句 186 要求
// 每条外部结果都保存**实际采用**的解释规则，默认口径根本填不进那一栏。
type InterpretationRuleView struct {
	db *bentopg.DB
}

func NewInterpretationRuleView(db *bentopg.DB) (*InterpretationRuleView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &InterpretationRuleView{db: db}, nil
}

var _ ports.InterpretationRuleView = (*InterpretationRuleView)(nil)

func (view *InterpretationRuleView) LoadInterpretationRule(
	ctx context.Context,
	tenant domain.TenantID,
	layer domain.ResultLayer,
) (domain.InterpretationRuleReference, bool, error) {
	// 封闭六层之外的取值取不出规则，也不该悄悄查成「未配置」：那是调用方的编程错误，
	// 与「实例还没登记」是两回事，续办动作也不同。
	layerText := layer.String()
	if layerText == "" {
		return domain.InterpretationRuleReference{}, false,
			fmt.Errorf("load interpretation rule: unknown result layer %d", layer)
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.InterpretationRuleReference{}, false, fmt.Errorf("load interpretation rule: %w", err)
	}

	var ruleRef string
	err = querier.QueryRow(ctx,
		`SELECT rule_ref
		   FROM customs_compliance.interpretation_rule
		  WHERE tenant_id = $1 AND result_layer = $2`,
		tenant.String(), layerText,
	).Scan(&ruleRef)
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.InterpretationRuleReference{}, false, nil
	}
	if err != nil {
		return domain.InterpretationRuleReference{}, false, fmt.Errorf("load interpretation rule: %w", err)
	}

	rule, err := domain.NewInterpretationRuleReference(ruleRef)
	if err != nil {
		return domain.InterpretationRuleReference{}, false, fmt.Errorf("rebuild interpretation rule: %w", err)
	}
	return rule, true, nil
}
