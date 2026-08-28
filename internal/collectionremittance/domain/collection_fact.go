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
	// RecipientPayment 是履约侧代收证据：尾程执行方记录的收件人付款主张。
	RecipientPayment
	// ChannelCollectionReport 是渠道报告已代收，钱仍在渠道手上。
	ChannelCollectionReport
	// ChannelRemittanceNotice 是渠道通知已回款，通知本身不是到账。
	ChannelRemittanceNotice
	// OperatorBankCredit 是运营企业真实到账。只有它支撑本金推进到`应付客户`。
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
type CollectionFactSpec struct {
	ID          CollectionFactID
	Instruction CollectionInstructionID
	Layer       CollectionSourceLayer
	Evidence    EvidenceReference
	Amount      Money
	OccurredAt  time.Time
}

// CollectionFact 是一次已被本上下文接受的收款主张或资金事实。它追加式登记且不可
// 覆盖：来源侧更正或撤销时追加新事实并关联原事实，原事实继续留在册上。
type CollectionFact struct {
	id          CollectionFactID
	instruction CollectionInstructionID
	layer       CollectionSourceLayer
	evidence    EvidenceReference
	amount      Money
	occurredAt  time.Time
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
	return CollectionFact{
		id:          spec.ID,
		instruction: spec.Instruction,
		layer:       spec.Layer,
		evidence:    spec.Evidence,
		amount:      spec.Amount,
		occurredAt:  spec.OccurredAt.UTC(),
	}, nil
}

func (fact CollectionFact) ID() CollectionFactID { return fact.id }

func (fact CollectionFact) Instruction() CollectionInstructionID { return fact.instruction }

func (fact CollectionFact) Layer() CollectionSourceLayer { return fact.layer }

func (fact CollectionFact) Evidence() EvidenceReference { return fact.evidence }

func (fact CollectionFact) Amount() Money { return fact.amount }

func (fact CollectionFact) OccurredAt() time.Time { return fact.occurredAt }

// IntakePosition 是这条事实入账时本金应停的位置：真实到账停`待清分`（归属还没落到
// 具体客户应付），其余三层只停`渠道在途`。
//
// 这条映射是「未实际收到的代收款不得进入应付客户」那句硬规则的落点。把它做进词表
// 而不是留给记账方选位置，是因为选错的那一次在库里看不出来——一笔停在`应付客户`的
// 钱，无论它凭哪一层进来，行上长得一模一样。
func (fact CollectionFact) IntakePosition() FundPosition {
	if fact.layer.SettlesToOperator() {
		return PositionAwaitingAllocation
	}
	return PositionInTransitAtChannel
}
