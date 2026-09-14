package parcelpricing

import (
	"context"
	"errors"
	"fmt"

	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

var (
	// ErrUntranslatableReference 表示信封带来的引用在本上下文词汇里构造不出来（空白租户 / 评价标识）。那是引用
	// 坏了，不是等谁——消费者译码已经拒过两维缺席，这里只可能是两侧词汇表出了分歧。
	ErrUntranslatableReference = errors.New(
		"settlement accounting parcelpricing adapter: untranslatable buy evaluation reference")
	// ErrEvaluationNotVisible 表示按信封引用在提供方还读不回那份评价。评价与信封在提供方同一事务落库，读不回
	// 只剩可见性滞后一种成因：续办，重投会改变结果，不当毒丸拒收、也不当「指名了不存在的评价」那种提交矛盾。
	// 读口自己答不出（库不可用）同格——等的都是提供方那一侧。生产装配把它登进 WithUndecidedSentinels。
	ErrEvaluationNotVisible = errors.New(
		"settlement accounting parcelpricing adapter: buy evaluation is not yet visible")
	// ErrSourceReferencesUnrecorded 是裁决 (c) 那一格：形成预期成本的命令除评价外还要发生项、费用项目与供应商
	// 协议的引用（FormSupplierExpectedCostCommand 头注），它们在 SA 请求评价那一步记进评价请求登记册
	// （票 sa-cc/08），而评价回指评价请求标识是 PP 侧的事（票 11）；两者接上之前，本处理方对「命令齐不齐」
	// 只能答未决并指名等评价请求记录——不为缺引用发明来源，不反查 TF 登记册（裁决对 (b) 的否决）。
	// 生产装配同样把它登进 WithUndecidedSentinels：它等的是后继票，不是传输。
	ErrSourceReferencesUnrecorded = errors.New(
		"settlement accounting parcelpricing adapter: source references for the expected cost command are not recorded yet")
)

// FormOnBuyEvaluationRecordedAdapter 是 sainbox.BuyEvaluationRecordedConsumer 的真实处理方：按信封引用向提供方
// 取评价，分辨是不是本消费者的评价，再看形成命令凑不凑得齐。
//
// 只译不判（票 sa-cc/01 红线）：金额、币种、换算步骤与规则版本整组出自评价，这里一个数字都不碰（ADR-0107）；
// 评价的每一种结果（saports.BuyEvaluationOutcome）怎么分格是 FormSupplierExpectedCostHandler 的事——命令凑不齐时
// 到不了它，所以今天这里没有那只编排的依赖：接一个永不被调用的口，与接一个「永远答不在」的来源替身，都是在
// 生产装配里说假话。08 的登记册读口与 11 的回指接上之后，后继票在这里补「按回指取来源引用 → 命令 → 形成」那一段，
// 消费者与本适配器的错误分格不变。
type FormOnBuyEvaluationRecordedAdapter struct {
	evaluations saports.BuyEvaluationView
}

func NewFormOnBuyEvaluationRecordedAdapter(
	evaluations saports.BuyEvaluationView,
) (*FormOnBuyEvaluationRecordedAdapter, error) {
	if evaluations == nil {
		return nil, fmt.Errorf("settlement accounting parcelpricing adapter: buy evaluation view is nil")
	}
	return &FormOnBuyEvaluationRecordedAdapter{evaluations: evaluations}, nil
}

var _ sainbox.RecordedBuyEvaluationHandler = (*FormOnBuyEvaluationRecordedAdapter)(nil)

// HandleRecordedBuyEvaluation 译引用 → 取评价 → 分辨方向 → 核命令。
//
// 读口的三种拒绝各走各的格：ErrNotABuyEvaluation 是「不是本消费者的信封」，答 nil 让消费门入账不重投——提供方
// 对 BUY 与 SELL 发同一种信封，SELL 那一半只能在这里安静地走掉；ErrUntranslatableAnswer 与
// ErrAmountPrecisionUndeclared 重投不会变好（提供方形状变了 / 价卡没声明取整策略），原样上抛、不译成未决，
// 装配把它们留在名单之外让失败码落 publish_failed；其余错误是读口答不出，与读不回同归可见性滞后。
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

	_, found, err := adapter.evaluations.LoadBuyEvaluation(ctx, tenant, reference)
	switch {
	case errors.Is(err, ErrNotABuyEvaluation):
		return nil
	case errors.Is(err, ErrUntranslatableAnswer), errors.Is(err, ErrAmountPrecisionUndeclared):
		return err
	case err != nil:
		return fmt.Errorf("%w: %v", ErrEvaluationNotVisible, err)
	case !found:
		return fmt.Errorf("%w: evaluation %s", ErrEvaluationNotVisible, reference)
	}

	return fmt.Errorf("%w: evaluation %s", ErrSourceReferencesUnrecorded, reference)
}
