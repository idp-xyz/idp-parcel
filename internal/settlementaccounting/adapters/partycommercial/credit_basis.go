package partycommercial

import (
	"context"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// CreditBasis 实现 saports.CreditBasisView：凭调用方回指的商业解析取已固定的闭包，从闭包里取出
// 已采用的信用政策版本随身带的额度（ADR-0127）。它与 PreAcceptanceControlPolicy 并列——同一份
// 回指、同一份闭包、各取各的成员；不合成一只，是因为两口的三格含义不同（控制策略的`未配置`是
// 声明或正文没登，本口的`未配置`是闭包没要信用政策），合成一只就得让一格答两件事。
//
// 为什么从闭包取而不点读 0020：额度是闭包解析在同一范围的多份信用政策之间**选**出来的
// （resolveCreditPolicyBasis），点读只能按版本壳取正文、答不了「该按哪一版」；且闭包快照整份留存
// 了当初授权的额度（ADR-0028），正文改一次已固定的解析不改口。回指是 ADR-0027 / ADR-0062 钦定的
// 「消费方只回指标识」形状，理由与 PreAcceptanceControlPolicy 一字不差。
//
// 三格：
//
//   - 闭包没采用信用政策 → found=false。租户登记的解析键没要求这一项、或该范围没有生效的信用
//     政策正文，恢复动作是补解析键与正文，不是重试。
//   - 采用了信用政策且带额度 → found=true，金额或比例原样译过来，出处是采用的那一版。
//   - 回指译不动、闭包读不回或查无、闭包不是唯一已解析、采用了信用政策却没带额度（本记录之前
//     固定的快照）→ error。都不是「没人登记过」，答成 found=false 会把租户支去补一份其实已经存在
//     的东西。
type CreditBasis struct {
	closures pcports.CommercialResolutionView
}

// NewCreditBasis 装配闭包读口。不许为 nil：本适配器一旦被装上就是要真去问商业侧的；缺读口而
// 静默答「未配置」会让一次装配疏漏与租户没登记长得一样。
func NewCreditBasis(closures pcports.CommercialResolutionView) (*CreditBasis, error) {
	if closures == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: commercial resolution view is nil")
	}
	return &CreditBasis{closures: closures}, nil
}

var _ saports.CreditBasisView = (*CreditBasis)(nil)

// LoadCreditBasis 分两段：回指换闭包 → 闭包取信用依据。
//
// scope 收下但不参与提问，理由与 PreAcceptanceControlPolicy.LoadControlPolicy 同一条：它是资金维，
// 商业侧的额度不按它键入；拿它去过滤就是用一个装不下答案的键去找答案。
func (adapter *CreditBasis) LoadCreditBasis(
	ctx context.Context,
	tenant sadomain.TenantID,
	_ sadomain.SettlementScope,
	resolution sadomain.CommercialResolutionReference,
) (sadomain.CreditBasis, bool, error) {
	commercialTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return sadomain.CreditBasis{}, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	resolutionID, err := pcdomain.NewResolutionID(resolution.String())
	if err != nil {
		return sadomain.CreditBasis{}, false,
			fmt.Errorf("%w: commercial resolution reference: %v", ErrUntranslatableAnswer, err)
	}

	closure, loaded, err := adapter.closures.LoadResolution(ctx, commercialTenant, resolutionID)
	if err != nil {
		return sadomain.CreditBasis{}, false, fmt.Errorf("load commercial closure: %w", err)
	}
	if !loaded {
		// 查无是坏回指，不是「未登记」：调用方刚刚才由这次解析派生出作用域，闭包却不在。
		return sadomain.CreditBasis{}, false,
			fmt.Errorf("%w: commercial closure %q was not found", ErrUntranslatableAnswer, resolutionID)
	}
	if closure.Outcome() != pcdomain.UniquelyResolved {
		return sadomain.CreditBasis{}, false,
			fmt.Errorf("%w: fixed closure is %q, not uniquely resolved", ErrUntranslatableAnswer, closure.Outcome())
	}

	adopted, present := closure.AdoptedFor(pcdomain.CreditPolicyObject)
	if !present {
		// 这一格是本口的「未配置」：闭包没要信用政策，额度无从谈起，等租户补解析键与正文。
		return sadomain.CreditBasis{}, false, nil
	}
	basis, carried := adopted.CreditBasis()
	if !carried || !basis.Applicable() {
		// 采用了信用政策版本却没带额度：ADR-0127 之前固定的快照，或提供方阶段契约被打破。答成
		// found=false 会把租户支去登一份其实已经在的政策；真正要做的是重解一次闭包。
		return sadomain.CreditBasis{}, false, fmt.Errorf(
			"%w: closure %q adopted credit policy %s without a credit basis",
			ErrUntranslatableAnswer, resolutionID, versionReference(adopted.Version()))
	}
	return creditBasisFrom(basis)
}

// creditBasisFrom 是两侧两格封闭额度之间的全函数（ADR-0025）：金额译金额、比例译比例连基数，两格都不在场
// 走 error 不吸收——那是 CreditLimit 构造门不会放出的形状。没有基数的比例（ADR-0129 之前固定的存量快照）
// 译成本侧的「未声明」形，如实带过去让编排停在 CREDIT_RATIO_BASE_UNDECIDED；这里不替它补一个。
func creditBasisFrom(basis pcdomain.CreditBasis) (sadomain.CreditBasis, bool, error) {
	policy, err := sadomain.NewCreditPolicyReference(versionReference(basis.PolicyVersion()))
	if err != nil {
		return sadomain.CreditBasis{}, false, fmt.Errorf("%w: credit policy reference: %v", ErrUntranslatableAnswer, err)
	}
	if minor, ok := basis.AuthorizedLimit().AmountMinor(); ok {
		translated, err := sadomain.NewCreditAmountBasis(policy, minor)
		if err != nil {
			return sadomain.CreditBasis{}, false, fmt.Errorf("%w: credit amount basis: %v", ErrUntranslatableAnswer, err)
		}
		return translated, true, nil
	}
	if bps, ok := basis.AuthorizedLimit().RatioBasisPoints(); ok {
		pcBase, declared := basis.AuthorizedLimit().RatioBase()
		if !declared {
			translated, err := sadomain.NewUndeclaredCreditRatioBasis(policy, bps)
			if err != nil {
				return sadomain.CreditBasis{}, false, fmt.Errorf("%w: credit ratio basis: %v", ErrUntranslatableAnswer, err)
			}
			return translated, true, nil
		}
		base, err := creditRatioBaseFrom(pcBase)
		if err != nil {
			return sadomain.CreditBasis{}, false, err
		}
		translated, err := sadomain.NewCreditRatioBasis(policy, bps, base)
		if err != nil {
			return sadomain.CreditBasis{}, false, fmt.Errorf("%w: credit ratio basis: %v", ErrUntranslatableAnswer, err)
		}
		return translated, true, nil
	}
	return sadomain.CreditBasis{}, false, fmt.Errorf(
		"%w: credit limit of %s is neither an amount nor a ratio", ErrUntranslatableAnswer, policy)
}

// creditRatioBaseFrom 是两侧封闭集之间的全函数：逐格译，集外走 error 不吸收——提供方加一格成员而本侧没接时，
// 要在这里响亮，而不是让一份新基数悄悄折成某个已有的分母。
func creditRatioBaseFrom(base pcdomain.CreditRatioBase) (sadomain.CreditRatioBase, error) {
	switch base {
	case pcdomain.PostedBalanceBase:
		return sadomain.PostedBalanceBase, nil
	case pcdomain.PriorPeriodConfirmedChargesBase:
		return sadomain.PriorPeriodConfirmedChargesBase, nil
	default:
		return sadomain.CreditRatioBaseUndeclared, fmt.Errorf(
			"%w: credit ratio base %q is outside the set this side translates", ErrUntranslatableAnswer, base)
	}
}
