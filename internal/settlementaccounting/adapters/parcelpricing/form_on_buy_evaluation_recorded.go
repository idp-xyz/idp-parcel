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

var (
	// ErrUntranslatableReference 表示信封带来的引用在本上下文词汇里构造不出来（空白租户 / 评价标识）。那是引用
	// 坏了，不是等谁——消费者译码已经拒过两维缺席，这里只可能是两侧词汇表出了分歧。
	ErrUntranslatableReference = errors.New(
		"settlement accounting parcelpricing adapter: untranslatable buy evaluation reference")
	// ErrEvaluationNotVisible 表示按信封引用在提供方还读不回那份评价。评价与信封在提供方同一事务落库，读不回
	// 本该只剩可见性滞后：续办，重投会改变结果，不当毒丸拒收、也不当「指名了不存在的评价」那种提交矛盾。读口
	// 自己答不出（库不可用）同格——等的都是提供方那一侧；依赖的原错误一并包着。生产装配把它登进
	// WithUndecidedSentinels。BuyEvaluationAdapter.LoadBuyEvaluation 把引用译不成计价侧标识、评价不属本租户也折成
	// 读不回，这两种重投不自愈，会耗尽失败预算——与 SELL 那一路同一处已知的不精确。
	ErrEvaluationNotVisible = errors.New(
		"settlement accounting parcelpricing adapter: buy evaluation is not yet visible")
	// ErrSupplierCostFormationUndecided 表示形成编排停在自己的依赖上（BUY 请求册、评价读口、预期成本库答不出）：形成
	// 与否未知，整笔回滚等重投。停在哪个依赖写在错误正文里，依赖的原错误一并包着。生产装配同样把它登进
	// WithUndecidedSentinels。
	ErrSupplierCostFormationUndecided = errors.New(
		"settlement accounting parcelpricing adapter: supplier expected cost formation is undecided")
)

// RequestedSupplierExpectedCostFormation 是本消费门转交的形成编排外层，生产装配接
// saapplication.FormRequestedSupplierExpectedCostHandler。
type RequestedSupplierExpectedCostFormation interface {
	Handle(
		ctx context.Context,
		command saapplication.FormRequestedSupplierExpectedCostCommand,
	) (saapplication.RequestedSupplierExpectedCostResult, error)
}

// FormOnBuyEvaluationRecordedAdapter 是 sainbox.BuyEvaluationRecordedConsumer 的真实处理方：按信封引用向提供方
// 取评价，分辨是不是本消费者的评价，是的就把采用快照交给形成编排外层，再把它的答案折成入账、未决或错误。
//
// 只译不判（票 sa-cc/01 红线）：金额、币种、换算步骤与规则版本整组出自评价，这里一个数字都不碰（ADR-0107）；
// 评价的每一种结果怎么分格是形成编排的事，这里只按编排的答案决定消费门入不入账。
type FormOnBuyEvaluationRecordedAdapter struct {
	evaluations saports.BuyEvaluationView
	formation   RequestedSupplierExpectedCostFormation
}

func NewFormOnBuyEvaluationRecordedAdapter(
	evaluations saports.BuyEvaluationView,
	formation RequestedSupplierExpectedCostFormation,
) (*FormOnBuyEvaluationRecordedAdapter, error) {
	if evaluations == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: buy evaluation view is nil")
	}
	if formation == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: supplier expected cost formation is nil")
	}
	return &FormOnBuyEvaluationRecordedAdapter{evaluations: evaluations, formation: formation}, nil
}

var _ sainbox.RecordedBuyEvaluationHandler = (*FormOnBuyEvaluationRecordedAdapter)(nil)

// HandleRecordedBuyEvaluation 译引用 → 取评价 → 分辨方向 → 交形成编排 → 折结果。
//
// 入账（答 nil）的各格都是这一封信的定局，重投同一封不会变：不是 BUY·供应商成本的评价（提供方对 BUY 与 SELL 发
// 同一种信封，SELL 那一半由扇出的另一路接）、评价没有回指、预期成本已采用或同三维已有首版、评价采用不了。
// ErrUntranslatableAnswer 与编排上抛的错误原样交回，落 publish_failed：提供方形状变了，或两侧说法矛盾，重投不自愈。
func (adapter *FormOnBuyEvaluationRecordedAdapter) HandleRecordedBuyEvaluation(
	ctx context.Context,
	recorded sainbox.RecordedBuyEvaluation,
) error {
	tenant, err := sadomain.NewTenantID(recorded.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableReference, err)
	}
	reference, err := sadomain.NewBuyEvaluationReference(recorded.EvaluationID)
	if err != nil {
		return fmt.Errorf("%w: evaluation: %v", ErrUntranslatableReference, err)
	}

	adoption, found, err := adapter.evaluations.LoadBuyEvaluation(ctx, tenant, reference)
	switch {
	case errors.Is(err, ErrNotABuyEvaluation):
		return nil
	case errors.Is(err, ErrUntranslatableAnswer):
		return err
	case err != nil:
		return fmt.Errorf("%w: evaluation %s: %w", ErrEvaluationNotVisible, reference, err)
	case !found:
		return fmt.Errorf("%w: evaluation %s", ErrEvaluationNotVisible, reference)
	}

	result, err := adapter.formation.Handle(ctx, saapplication.FormRequestedSupplierExpectedCostCommand{
		TenantID:   tenant,
		Evaluation: adoption,
	})
	if err != nil {
		return err
	}
	switch result.Outcome() {
	case saapplication.RequestedSupplierExpectedCostAdopted,
		saapplication.RequestedSupplierExpectedCostFirstVersionExists,
		saapplication.RequestedSupplierExpectedCostEvaluationNotAdopted,
		saapplication.RequestedSupplierExpectedCostNoBuyRequest:
		return nil
	case saapplication.RequestedSupplierExpectedCostUndecided:
		undecided := fmt.Errorf("%w: evaluation %s: %s", ErrSupplierCostFormationUndecided, reference, result.UndecidedReason())
		if cause := result.UndecidedCause(); cause != nil {
			return fmt.Errorf("%w: %w", undecided, cause)
		}
		return undecided
	default:
		return fmt.Errorf("settlement accounting parcelpricing adapter: unexpected supplier expected cost outcome %d", result.Outcome())
	}
}
