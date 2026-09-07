package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidLabelValidity 是面单有效期声明立不住：起算时刻种类不在集内，或时长不为正。零时长与
	// 负时长都不是「多久失效」的答案；「没有有效期」不走这里，那由 NewFinalRuleContent 表达。
	ErrInvalidLabelValidity = errors.New("party commercial: invalid label validity declaration")
)

// ValidityAnchorKind 是面单有效期的起算时刻种类封闭集（ADR-0119 Decision 二）。首发只有一格
// 「渠道结果业务时间」，对应 parcel-shipment 面单交易上渠道结果的观察时刻（MCP-1 代裁 Q1）；面单
// 签发时刻、委托接受时刻不预开——加格是新一版声明的事，类型在，加格只改这里的 valid() 与库上 CHECK。
type ValidityAnchorKind uint8

const (
	ValidityAnchorKindInvalid ValidityAnchorKind = iota
	ChannelResultObservedAnchor
)

func (kind ValidityAnchorKind) valid() bool {
	return kind == ChannelResultObservedAnchor
}

func (kind ValidityAnchorKind) String() string {
	switch kind {
	case ChannelResultObservedAnchor:
		return "CHANNEL_RESULT_OBSERVED"
	default:
		return ""
	}
}

// LabelValidityDeclaration 是终局规则版本声明的一条面单有效期：自起算时刻种类所指的那一刻起，经过
// duration 后该包裹的成功面单结果不可逆失效（CONTEXT「接受时固定规则下的不可逆失效」）。
//
// 时长按时刻精度登、不预设日粒度：锚是带时刻精度的业务时间，「N 天」是租户的选择不是产品的假设。
// 本上下文只登声明，不判失效——「asOf 是否已过锚加时长」是 parcel-shipment 读时算的事。
type LabelValidityDeclaration struct {
	anchor   ValidityAnchorKind
	duration time.Duration
}

// NewLabelValidityDeclaration 要求种类在集内、时长严格为正。
func NewLabelValidityDeclaration(anchor ValidityAnchorKind, duration time.Duration) (LabelValidityDeclaration, error) {
	if !anchor.valid() || duration <= 0 {
		return LabelValidityDeclaration{}, ErrInvalidLabelValidity
	}
	return LabelValidityDeclaration{anchor: anchor, duration: duration}, nil
}

func (declaration LabelValidityDeclaration) Anchor() ValidityAnchorKind {
	return declaration.anchor
}

func (declaration LabelValidityDeclaration) Duration() time.Duration {
	return declaration.duration
}

func (declaration LabelValidityDeclaration) valid() bool {
	return declaration.anchor.valid() && declaration.duration > 0
}

// NewFinalRuleContentWithValidity 组装带面单有效期的终局规则声明。它与 NewFinalRuleContent 分立而不是
// 给后者加一个可选入参：零值 time.Duration 是 0，而「零时长」与「没声明」要人做的事相反——前者是坏
// 声明该拒，后者是合法缺席——一个入参装不下这两件（判据同 ADR-0116 Decision 二「恰一由两个构造器
// 分立」）。终局规则行的既有判据（拥有者、至少一行、不两行）原样经 NewFinalRuleContent 守，这里只多
// 一道：有效期必须立得住。
func NewFinalRuleContentWithValidity(
	owner CommercialVersion,
	declarations []FinalizationDeclaration,
	validity LabelValidityDeclaration,
) (FinalRuleContent, error) {
	content, err := NewFinalRuleContent(owner, declarations)
	if err != nil {
		return FinalRuleContent{}, err
	}
	if !validity.valid() {
		return FinalRuleContent{}, ErrInvalidLabelValidity
	}
	content.validity = validity
	content.hasValidity = true
	return content, nil
}
