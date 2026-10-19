package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// ErrSellChargeFormationUndecided 是 SELL 形成那一路唯一的未决哨兵：评价读不回，或请求册、映射册、费用库某一个答不出。
// 形成与否未知，整笔回滚等重投；停在哪个依赖上、依赖原样答了什么，都写在错误正文里。生产装配把它登进这一路的
// WithUndecidedSentinels。
//
// 评价与信封同事务落库，读不回本该只剩可见性滞后；但 SellEvaluationAdapter.LoadSellEvaluation 把引用译不成计价侧标识、
// 评价不属本租户也折成读不回，这两种重投不自愈，会耗尽失败预算落 ABANDONED。
var ErrSellChargeFormationUndecided = errors.New(
	"settlement accounting parcelpricing adapter: estimated customer charge formation is undecided")

// EstimatedCustomerChargeFormation 是本消费门转交的形成编排外层，生产装配接 saapplication.FormEstimatedCustomerChargeHandler。
type EstimatedCustomerChargeFormation interface {
	Handle(
		ctx context.Context,
		command saapplication.FormEstimatedCustomerChargeCommand,
	) (saapplication.EstimatedCustomerChargeResult, error)
}

// FormOnSellEvaluationRecordedAdapter 是 SELL 消费门（ADR-0169 决定一、ADR-0190 决定五）：按信封引用读回评价，不是
// SELL·CUSTOMER_CHARGE 的答 nil，让 BUY 那一路去处理；是的，把采用快照交给形成编排外层。BUY 评价请求不经这里。
type FormOnSellEvaluationRecordedAdapter struct {
	evaluations saports.SellEvaluationView
	formation   EstimatedCustomerChargeFormation
}

func NewFormOnSellEvaluationRecordedAdapter(
	evaluations saports.SellEvaluationView,
	formation EstimatedCustomerChargeFormation,
) (*FormOnSellEvaluationRecordedAdapter, error) {
	if evaluations == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: sell evaluation view is nil")
	}
	if formation == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: estimated customer charge formation is nil")
	}
	return &FormOnSellEvaluationRecordedAdapter{evaluations: evaluations, formation: formation}, nil
}

var _ sainbox.RecordedSellEvaluationHandler = (*FormOnSellEvaluationRecordedAdapter)(nil)

// HandleRecordedSellEvaluation 译引用 → 取评价 → 分辨方向 → 交形成编排 → 折结果。
//
// 入账（答 nil）的各格：不是 SELL·CUSTOMER_CHARGE、评价没有回指、请求记下了结论或早有结论、评价失败而请求停在
// `已提交`——都是这一封信的定局，重投同一封不会变。ErrUntranslatableAnswer 与编排上抛的错误原样交回，落
// publish_failed：提供方形状变了，或两侧说法矛盾（回指的请求不在册上、结算币种对不上），重投不自愈。
func (adapter *FormOnSellEvaluationRecordedAdapter) HandleRecordedSellEvaluation(
	ctx context.Context,
	recorded sainbox.RecordedSellEvaluation,
) error {
	tenant, err := sadomain.NewTenantID(recorded.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	reference, err := sadomain.NewSellEvaluationReference(recorded.EvaluationID)
	if err != nil {
		return fmt.Errorf("%w: evaluation: %v", ErrUntranslatableReference, err)
	}
	adoption, found, err := adapter.evaluations.LoadSellEvaluation(ctx, tenant, reference)
	switch {
	case errors.Is(err, saports.ErrNotASellEvaluation):
		return nil
	case errors.Is(err, ErrUntranslatableAnswer):
		return err
	case err != nil:
		return fmt.Errorf("%w: sell evaluation view: %v", ErrSellChargeFormationUndecided, err)
	case !found:
		return fmt.Errorf("%w: evaluation %s is not yet visible", ErrSellChargeFormationUndecided, reference)
	}

	result, err := adapter.formation.Handle(ctx, saapplication.FormEstimatedCustomerChargeCommand{
		TenantID:   tenant,
		Evaluation: adoption,
	})
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case saapplication.EstimatedCustomerChargeConcluded,
		saapplication.EstimatedCustomerChargeAlreadyConcluded,
		saapplication.EstimatedCustomerChargeNoSellRequest,
		saapplication.EstimatedCustomerChargeEvaluationNotFormed:
		return nil
	case saapplication.EstimatedCustomerChargeUndecided:
		return fmt.Errorf("%w: evaluation %s: %s: %v", ErrSellChargeFormationUndecided, reference, result.UndecidedReason(), result.UndecidedCause())
	default:
		return fmt.Errorf("settlement accounting parcelpricing adapter: unexpected estimated customer charge outcome %d", result.Outcome())
	}
}
