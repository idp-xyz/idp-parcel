package domain

import (
	"fmt"
	"time"
)

// CollectionSourceLayer 是代收事实的来源层级，封闭四值。
//
// 词表取 GLOSSARY「代收货款」的硬句：收件人付款、渠道报告已代收、渠道通知已回款和
// 运营企业真实到账是**四层不同事实，不得互相覆盖或直接推导**。它不是租户实例词表，
// 因此写成封闭枚举不算替租户拟默认值——恰恰相反，缺了这一格就没有任何东西拦得住
// 「按 POD 推定已收款」「按渠道报告推定钱已回来」那两类。
type CollectionSourceLayer uint8

const (
	SourceLayerUnknown CollectionSourceLayer = iota
	// RecipientPayment 是履约侧代收证据：尾程执行方记录的收件人付款主张。它是旁证——
	// 登记在册、不支撑任何记账：它说的是尾程收了钱，不是谁对运营企业欠这笔钱。
	RecipientPayment
	// ChannelCollectionReport 是渠道报告已代收，钱仍在渠道手上。本金只凭它进账。
	ChannelCollectionReport
	// ChannelRemittanceNotice 是渠道通知已回款，通知本身不是到账。它也是旁证。
	ChannelRemittanceNotice
	// OperatorBankCredit 是运营企业真实到账。只有它撑得起`渠道在途` → `待清分`那一跳，
	// 而它自己不进账：钱早已凭渠道报告在账上，到账只是把它从渠道手上移回来。
	OperatorBankCredit
)

func (layer CollectionSourceLayer) String() string {
	switch layer {
	case RecipientPayment:
		return "RECIPIENT_PAYMENT"
	case ChannelCollectionReport:
		return "CHANNEL_COLLECTION_REPORT"
	case ChannelRemittanceNotice:
		return "CHANNEL_REMITTANCE_NOTICE"
	case OperatorBankCredit:
		return "OPERATOR_BANK_CREDIT"
	default:
		return ""
	}
}

func (layer CollectionSourceLayer) valid() bool { return layer.String() != "" }

// SettlesToOperator 区分「钱已经到运营企业手上」与「只是有人这么报」。只有真实到账
// 那一层为真；判断放在词表上而不是散在调用点，是因为这一格错一次就会把未实际收到的
// 代收款推进成可付客户余额，而那正是 CONTEXT 明确不给的权力。
func (layer CollectionSourceLayer) SettlesToOperator() bool {
	return layer == OperatorBankCredit
}

func ParseCollectionSourceLayer(text string) (CollectionSourceLayer, bool) {
	for _, candidate := range []CollectionSourceLayer{
		RecipientPayment, ChannelCollectionReport, ChannelRemittanceNotice, OperatorBankCredit,
	} {
		if candidate.String() == text {
			return candidate, true
		}
	}
	return SourceLayerUnknown, false
}

// CollectionFactSpec 是接受一条代收事实所需的全部输入。
//
// AdoptedFundsVersion 只属于运营企业真实到账：那一层的 Evidence 就是 settlement-accounting 已采用事实的引用
// （ADR-0274、ADR-0330），版本是登记时按引用读到的链头版本，由编排读回结算之后钉上（PinnedToAdoptedFunds），
// 接受时可以还没有；别的三层带它不成立。
type CollectionFactSpec struct {
	ID                  CollectionFactID
	Instruction         CollectionInstructionID
	Layer               CollectionSourceLayer
	Evidence            EvidenceReference
	Amount              Money
	OccurredAt          time.Time
	AdoptedFundsVersion AdoptedFundsVersion
}

// CollectionFact 是一次已被本上下文接受的收款主张或资金事实。它追加式登记且不可
// 覆盖：来源侧更正或撤销时追加新事实并关联原事实，原事实继续留在册上。
type CollectionFact struct {
	id           CollectionFactID
	instruction  CollectionInstructionID
	layer        CollectionSourceLayer
	evidence     EvidenceReference
	amount       Money
	occurredAt   time.Time
	fundsVersion AdoptedFundsVersion
}

// AcceptCollectionFact 接受一条代收事实。来源层级与来源证据引用都必填——不声明层级
// 的事实在账上分不出「渠道说收到了」和「钱真的到了」，而这两件事的下一步动作相反。
func AcceptCollectionFact(spec CollectionFactSpec) (CollectionFact, error) {
	if !spec.ID.valid() {
		return CollectionFact{}, fmt.Errorf("%w: fact ID", ErrInvalidCollectionFact)
	}
	if !spec.Instruction.valid() {
		return CollectionFact{}, fmt.Errorf("%w: instruction ID", ErrInvalidCollectionFact)
	}
	if !spec.Layer.valid() {
		return CollectionFact{}, fmt.Errorf(
			"%w: unknown source layer %d", ErrInvalidCollectionFact, spec.Layer)
	}
	if !spec.Evidence.valid() {
		return CollectionFact{}, fmt.Errorf("%w: evidence reference", ErrInvalidCollectionFact)
	}
	if !spec.Amount.valid() {
		return CollectionFact{}, fmt.Errorf("%w: amount", ErrInvalidCollectionFact)
	}
	if spec.OccurredAt.IsZero() {
		return CollectionFact{}, fmt.Errorf("%w: occurred at", ErrInvalidCollectionFact)
	}
	if spec.AdoptedFundsVersion.valid() && !spec.Layer.SettlesToOperator() {
		return CollectionFact{}, fmt.Errorf(
			"%w: only an operator bank credit pins an adopted funds version", ErrInvalidCollectionFact)
	}
	return CollectionFact{
		id:           spec.ID,
		instruction:  spec.Instruction,
		layer:        spec.Layer,
		evidence:     spec.Evidence,
		amount:       spec.Amount,
		occurredAt:   spec.OccurredAt.UTC(),
		fundsVersion: spec.AdoptedFundsVersion,
	}, nil
}

// AdoptedFundsReference 交回这条事实引用的 settlement-accounting 已采用外部资金事实；第二值为 false 即这一层不引用
// 结算的事实。只有运营企业真实到账引用：它是外部资金事实，只经结算的采用进入本产品，证据引用就是那条事实的引用。
func (fact CollectionFact) AdoptedFundsReference() (AdoptedFundsReference, bool) {
	if !fact.layer.SettlesToOperator() {
		return AdoptedFundsReference{}, false
	}
	return AdoptedFundsReference{requiredValue{value: fact.evidence.String()}}, true
}

// AdoptedFundsVersion 交回登记时读到的链头版本；第二值为 false 即还没按引用读过结算（接受门刚构造出来的、或库里按旧口径
// 登记过的存量）。没有版本的真实到账撑不起任何记账（PostSubledger 的采用事实门）。
func (fact CollectionFact) AdoptedFundsVersion() (AdoptedFundsVersion, bool) {
	return fact.fundsVersion, fact.fundsVersion.valid()
}

// PinnedToAdoptedFunds 交回钉上登记时所读链头版本的同一条事实（值语义，原值不动）。只有真实到账能钉：别的层没有
// 结算的事实可钉。
func (fact CollectionFact) PinnedToAdoptedFunds(version AdoptedFundsVersion) (CollectionFact, error) {
	if !fact.layer.SettlesToOperator() || !version.valid() {
		return CollectionFact{}, fmt.Errorf(
			"%w: only an operator bank credit pins a non-blank adopted funds version", ErrInvalidCollectionFact)
	}
	pinned := fact
	pinned.fundsVersion = version
	return pinned, nil
}

func (fact CollectionFact) ID() CollectionFactID { return fact.id }

func (fact CollectionFact) Instruction() CollectionInstructionID { return fact.instruction }

func (fact CollectionFact) Layer() CollectionSourceLayer { return fact.layer }

func (fact CollectionFact) Evidence() EvidenceReference { return fact.evidence }

func (fact CollectionFact) Amount() Money { return fact.amount }

func (fact CollectionFact) OccurredAt() time.Time { return fact.occurredAt }

// IntakePosition 是这条事实自外部来源入账时本金应停的位置。只有渠道代收报告有位置
// ——`渠道在途`；其余三层交回 FundPositionUnknown，即凭它们入不了账（ADR-0271 决定一）。
//
// 四层各自都能入账时，同一件包裹的同一笔钱经三层各记一次，账上就是两三倍；而事后
// 分不出哪一笔是重的。只留一层入账，是因为渠道报告正是`渠道在途`那一格的意思：渠道
// 陈述这笔钱在它手上、欠着运营企业，账上的每一分钱从此都有一个对它负责的渠道。位置
// 做进词表而不留给记账方挑，是因为挑错的那一次在库里看不出来。
func (fact CollectionFact) IntakePosition() FundPosition {
	if fact.layer == ChannelCollectionReport {
		return PositionInTransitAtChannel
	}
	return FundPositionUnknown
}
