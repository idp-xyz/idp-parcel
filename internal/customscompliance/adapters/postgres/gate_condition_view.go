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

// GateConditionView 实现 ports.GateConditionView：按（范围+动作+边界）盘出门禁前置
// 条件逐项判断。
//
// 与义务盘点同一形状、同一理由：目录行给 configured，明细行给清单。这里两格的分界
// 尤其要紧——目录未登记是未决（没有清单的门禁判断无从复核），登记了却空清单是「此
// 动作在此边界本就不受门禁」的如实答案，领域会把它折成`不适用`。合成一格就等于用
// 「查不到」冒充「不受管」，那是直接把门禁放开。
type GateConditionView struct {
	db *bentopg.DB
}

func NewGateConditionView(db *bentopg.DB) (*GateConditionView, error) {
	if db == nil {
		return nil, fmt.Errorf("customs compliance postgres: db is nil")
	}
	return &GateConditionView{db: db}, nil
}

var _ ports.GateConditionView = (*GateConditionView)(nil)

func (view *GateConditionView) LoadPreconditionFindings(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.DecisionScopeReference,
	action domain.GuardedAction,
	boundary domain.CustomsProcedureReference,
) ([]domain.PreconditionFinding, bool, error) {
	actionText := action.String()
	if actionText == "" {
		return nil, false, fmt.Errorf("load precondition findings: unknown guarded action %d", action)
	}

	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("load precondition findings: %w", err)
	}

	var registered bool
	err = querier.QueryRow(ctx,
		`SELECT true
		   FROM customs_compliance.gate_condition_catalog
		  WHERE tenant_id = $1 AND scope_ref = $2 AND action = $3 AND boundary_ref = $4`,
		tenant.String(), scope.String(), actionText, boundary.String(),
	).Scan(&registered)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("load precondition findings: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT precondition_ref, finding_state
		   FROM customs_compliance.gate_condition_finding
		  WHERE tenant_id = $1 AND scope_ref = $2 AND action = $3 AND boundary_ref = $4
		  ORDER BY precondition_ref`,
		tenant.String(), scope.String(), actionText, boundary.String(),
	)
	if err != nil {
		return nil, false, fmt.Errorf("load precondition findings: %w", err)
	}
	defer rows.Close()

	findings := make([]domain.PreconditionFinding, 0)
	for rows.Next() {
		var preconditionRef, stateText string
		if err := rows.Scan(&preconditionRef, &stateText); err != nil {
			return nil, false, fmt.Errorf("load precondition findings: %w", err)
		}
		precondition, err := domain.NewPreconditionReference(preconditionRef)
		if err != nil {
			return nil, false, fmt.Errorf("load precondition findings: %w", err)
		}
		state, err := preconditionStateOf(stateText)
		if err != nil {
			return nil, false, fmt.Errorf("load precondition findings: %w", err)
		}
		findings = append(findings, domain.PreconditionFinding{Precondition: precondition, State: state})
	}
	if err := rows.Err(); err != nil {
		return nil, false, fmt.Errorf("load precondition findings: %w", err)
	}
	return findings, true, nil
}

// preconditionStateOf 把库里的封闭三值译回领域取值。没有「未知」格可落：判断不出来
// 的前置条件不该进折叠，硬塞一格会让证据装配问题伪装成门禁语义。
func preconditionStateOf(raw string) (domain.PreconditionState, error) {
	switch raw {
	case "MET":
		return domain.PreconditionMet, nil
	case "UNMET":
		return domain.PreconditionUnmet, nil
	case "CONFLICTING":
		return domain.PreconditionConflicting, nil
	default:
		return domain.PreconditionStateInvalid, fmt.Errorf("unknown precondition state %q", raw)
	}
}
