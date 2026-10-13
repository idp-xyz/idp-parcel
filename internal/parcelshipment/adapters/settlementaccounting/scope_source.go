package settlementaccounting

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SettlementAccountDirectory 把采用结算政策的（法人/相对方/币种/政策对象）换成结算账户。
// 生产装配接的是结算账户登记册。第二个返回值为 false 即这一组应收账户没登记——停下，
// 不代拟一个账户去冻别人的钱。
type SettlementAccountDirectory interface {
	FindSettlementAccount(
		ctx context.Context,
		identity psdomain.SourceIdentity,
		terms psdomain.AdoptedSettlementTerms,
	) (sadomain.SettlementAccountID, bool, error)
}

// PolicyBackedControlScopeSource 从已解析的结算政策推导资金作用域（ADR-0044/0047 接通
// 的那条缝）：解析回显给出法人与币种，账户目录补上结算账户；同一次解析的标识一并交回，
// 作为 SA 向商业侧提问控制策略的键。
//
// 它走 CommercialBasisResolver 而不是另开取数口：解析幂等（同一查询交回同一采用），施加
// 与释放两条路径因此拿到同一个作用域，不需要在请求上多背一份回显。
type PolicyBackedControlScopeSource struct {
	commercial psports.CommercialBasisResolver
	directory  SettlementAccountDirectory
}

// NewPolicyBackedControlScopeSource 装配两半。directory 为 nil 时作用域停在账户没登记那一格，
// 留给还没接目录的测试；生产装配传入登记册，空册与没登记的应收行都答未找到。
func NewPolicyBackedControlScopeSource(
	commercial psports.CommercialBasisResolver,
	directory SettlementAccountDirectory,
) *PolicyBackedControlScopeSource {
	return &PolicyBackedControlScopeSource{commercial: commercial, directory: directory}
}

var _ ControlScopeSource = (*PolicyBackedControlScopeSource)(nil)

// FormControlScope 分三段：解析取回显 → 目录换账户 → 拼作用域。两种「没有」都不是错误，但各报
// 各的缺口：解析没带回结算政策是一格，目录查无（或没接）是另一格——两格的恢复动作不同，见
// ControlScopeGap。
//
// 解析标识与作用域同出这一次解析，一并交回。译不动它是编程错误不是未形成：解析既然成了，
// 标识就在快照的构造期受过护（NewCommercialBasisSnapshot 拒无标识），此处译不过去只可能是
// 两侧词汇表出了分歧。
func (source *PolicyBackedControlScopeSource) FormControlScope(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	shipmentRequestID psdomain.ShipmentRequestID,
	submissionVersion psdomain.SubmissionVersionID,
) (ControlScope, ControlScopeGap, error) {
	if source.commercial == nil {
		return ControlScope{}, ControlScopeSettlementPolicyNotAdopted, nil
	}
	if source.directory == nil {
		return ControlScope{}, ControlScopeAccountNotRegistered, nil
	}

	resolution, err := source.commercial.ResolveCommercialBasis(ctx, psports.CommercialBasisQuery{
		Identity:          identity,
		ShipmentRequestID: shipmentRequestID,
		SubmissionVersion: submissionVersion,
	})
	if err != nil {
		return ControlScope{}, ControlScopeGapInvalid, fmt.Errorf("resolve commercial basis for control scope: %w", err)
	}
	terms, present := resolution.Snapshot.SettlementTerms()
	if !present {
		// 解析没成或成了但不含结算政策：两者下都推不出作用域。走到控制这一步时商业解析已经成了，
		// 剩下的只有登记的解析键没要求结算政策这一种。
		return ControlScope{}, ControlScopeSettlementPolicyNotAdopted, nil
	}

	account, found, err := source.directory.FindSettlementAccount(ctx, identity, terms)
	if err != nil {
		return ControlScope{}, ControlScopeGapInvalid, fmt.Errorf("find settlement account: %w", err)
	}
	if !found {
		return ControlScope{}, ControlScopeAccountNotRegistered, nil
	}

	legalEntity, err := sadomain.NewLegalEntityReference(terms.LegalEntity().String())
	if err != nil {
		return ControlScope{}, ControlScopeGapInvalid, fmt.Errorf("%w: legal entity: %v", ErrUntranslatableAnswer, err)
	}
	currency, err := sadomain.NewCurrencyCode(terms.Currency().String())
	if err != nil {
		return ControlScope{}, ControlScopeGapInvalid, fmt.Errorf("%w: currency: %v", ErrUntranslatableAnswer, err)
	}
	scope, err := sadomain.NewSettlementScope(legalEntity, account, currency)
	if err != nil {
		return ControlScope{}, ControlScopeGapInvalid, fmt.Errorf("%w: settlement scope: %v", ErrUntranslatableAnswer, err)
	}
	reference, err := sadomain.NewCommercialResolutionReference(resolution.Snapshot.ResolutionID().String())
	if err != nil {
		return ControlScope{}, ControlScopeGapInvalid, fmt.Errorf("%w: commercial resolution reference: %v",
			ErrUntranslatableAnswer, err)
	}
	return ControlScope{Settlement: scope, Resolution: reference}, ControlScopeFormed, nil
}
