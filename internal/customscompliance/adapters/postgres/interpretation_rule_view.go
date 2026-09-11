package postgres

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// InterpretationRuleView 实现 ports.InterpretationRuleView：按（租户，结果层，适用
// 辖区）在评估时点上解析适用的解释规则版本。半开区间与义务盘点同形（applies_until
// 等于评估时点的版本已不再适用）；区间不重叠由迁移的排他约束守着，至多一行。
//
// found=false 即该辖区该层在该时点无已登记版本——实例半边未提供时解释停在未决。这里
// 没有任何兜底口径：一个「默认按成功解释」的分支会让未知监管语义被读成放行，而 CONTEXT
// 要求每条外部结果都保存「实际采用的解释规则」，默认口径根本填不进那一栏；
// 「按当前指针解释」同样是兜底——CONTEXT「不能统一替代规则的法定适用时点」禁的正是拿到达时点顶替法定适用时点。
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
	jurisdiction domain.RegulatoryJurisdictionReference,
	evaluatedAt time.Time,
) (domain.InterpretationRuleReference, bool, error) {
	// 封闭六层之外的取值取不出规则，也不该悄悄查成「未配置」：那是调用方的编程错误，
	// 与「实例还没登记」是两回事，续办动作也不同。空辖区与零评估时点同理——辖区与
	// 评估时点的来源由编排负责（回指案件、取业务发生时间），走到这里还缺就是编排坏了。
	layerText := layer.String()
	if layerText == "" {
		return domain.InterpretationRuleReference{}, false,
			fmt.Errorf("load interpretation rule: unknown result layer %d", layer)
	}
	if strings.TrimSpace(jurisdiction.String()) == "" {
		return domain.InterpretationRuleReference{}, false,
			fmt.Errorf("load interpretation rule: the jurisdiction is blank")
	}
	if evaluatedAt.IsZero() {
		return domain.InterpretationRuleReference{}, false,
			fmt.Errorf("load interpretation rule: the evaluation instant is zero")
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return domain.InterpretationRuleReference{}, false, fmt.Errorf("load interpretation rule: %w", err)
	}

	var ruleRef string
	err = querier.QueryRow(ctx,
		`SELECT rule_ref
		   FROM customs_compliance.interpretation_rule
		  WHERE tenant_id = $1 AND result_layer = $2 AND jurisdiction_ref = $3
		    AND applies_from <= $4
		    AND (applies_until IS NULL OR applies_until > $4)`,
		tenant.String(), layerText, jurisdiction.String(), evaluatedAt.UTC(),
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
