package partycommercial

import (
	"context"
	"fmt"
	"time"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// DeclaredLabelValidityRule 实现 psports.LabelValidityRuleView：「接受时固定的有效期规则」是接单规则包版本下
// 终局规则声明上的一格（ADR-0119 Decision 一），本适配器只消费——包裹 → 当前已接受委托 → 接受时固定的
// 规则包版本 → 终局规则声明 → 有效期这一格 → 以起算时刻种类所指的那一刻为锚，判 asOf 是否已到锚加时长。
//
// 「接受时固定」不在 PS 另存：回指走 FinalContentSource（生产装配是 DeclaredStageContent 经 AdoptedStageOwner
// 回指接受时固定的规则包版本，ADR-0062），读的就是接受那一刻钉住的那版，规则包换版不追溯。
//
// 三格的分派：
//
//   - 包裹不属任何当前已接受委托 / 委托尚未固定采用哪版规则包 / 终局规则无父行 / 声明在场但没有有效期这一格
//     → configured=false。都是实例半边没到：没有规则就没有失效，消费方让那笔成功照常阻止终局。**不得因为
//     终局规则其它行在场就把有效期当已配置**（ADR-0119 Decision 六）——分辨这一格的是 Validity() 的第二个
//     返回值，不是 LoadFinalRule 的 found。
//   - 声明在场 → configured=true；asOf ≥ 锚 + 时长即 lapsed=true，否则 false。边界取「≥」（票面裁决①）。
//   - 读口出错、锚种类不在本适配器认识的集内、锚种类所指的那一刻在这笔交易上还不存在 → error。这些都不是
//     「没人登记过」，答成未配置会把一次故障或一处装配缺陷与租户没登记混成一格。
//
// 不拿墙钟：asOf 由调用方给（消费编排从它的 Clock 取），本适配器不知道现在几点。
type DeclaredLabelValidityRule struct {
	targets psports.CurrentAcceptedParcelTargetView
	final   FinalContentSource
}

// NewDeclaredLabelValidityRule 装配两个只读半边。两者都不许为 nil：本适配器一旦被装上，就是要真去问商业侧的；
// 「有效期规则未配置」由消费编排的 Validity 依赖留空表达，不由这里静默扮演。
func NewDeclaredLabelValidityRule(
	targets psports.CurrentAcceptedParcelTargetView,
	final FinalContentSource,
) (*DeclaredLabelValidityRule, error) {
	if targets == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: parcel target view is nil")
	}
	if final == nil {
		return nil, fmt.Errorf("parcel shipment partycommercial adapter: final content source is nil")
	}
	return &DeclaredLabelValidityRule{targets: targets, final: final}, nil
}

var _ psports.LabelValidityRuleView = (*DeclaredLabelValidityRule)(nil)

// JudgeLabelLapsed 分三段：包裹反查当前已接受委托 → 按其来源身份取接受时固定那版的终局规则声明 → 取有效期
// 这一格，按锚种类译出这笔交易上的锚，比 asOf。
func (rule *DeclaredLabelValidityRule) JudgeLabelLapsed(
	ctx context.Context,
	tenant psdomain.TenantID,
	transaction psdomain.LabelTransaction,
	parcel psdomain.DeclaredParcelID,
	asOf time.Time,
) (bool, bool, error) {
	target, found, err := rule.targets.FindCurrentAcceptedByParcel(ctx, tenant, parcel)
	if err != nil {
		return false, false, fmt.Errorf("label validity rule: find current accepted parcel target: %w", err)
	}
	if !found {
		return false, false, nil
	}
	content, found, err := rule.final.FinalContentFor(ctx, target.Identity())
	if err != nil {
		return false, false, fmt.Errorf("label validity rule: final rule content: %w", err)
	}
	if !found {
		return false, false, nil
	}
	validity, declared := content.Validity()
	if !declared {
		return false, false, nil
	}
	anchor, err := validityAnchorOf(validity.Anchor(), transaction)
	if err != nil {
		return false, false, err
	}
	return !asOf.Before(anchor.Add(validity.Duration())), true, nil
}

// validityAnchorOf 把起算时刻种类译成这笔交易上的那一刻，按格分路；本适配器今天只接「渠道结果业务时间」
// （ADR-0119 Decision 二），提供方日后加格在这里加分支。default 报错不吸收（ADR-0025）：提供方开了本适配器
// 不认识的起算时刻而这里没接分支时必须炸出来，静默按结果时间算等于替租户改了规则。
func validityAnchorOf(kind pcdomain.ValidityAnchorKind, transaction psdomain.LabelTransaction) (time.Time, error) {
	switch kind {
	case pcdomain.ChannelResultObservedAnchor:
		observed := transaction.ResultObservedAt()
		if observed.IsZero() {
			// 结果未回的交易没有「渠道结果业务时间」这一刻。零值当锚会让任何 asOf 都算已过期，而消费编排只对
			// 已受理的结果问本口——走到这里是调用契约被打破，不是一种失效。
			return time.Time{}, fmt.Errorf("%w: label transaction %q has no channel result to anchor validity on",
				ErrUntranslatableAnswer, transaction.ID())
		}
		return observed, nil
	default:
		return time.Time{}, fmt.Errorf("%w: validity anchor kind %d", ErrUntranslatableAnswer, uint8(kind))
	}
}
