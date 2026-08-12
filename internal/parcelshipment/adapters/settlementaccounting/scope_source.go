package settlementaccounting

import (
	"context"
	"fmt"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// SettlementAccountDirectory 把采用结算政策的（法人/相对方/币种）换成结算账户。这是
// 作用域缝的实例半边：账户映射是租户的配置（SA CONTEXT「一个结算账户固定一个责任法人、
// 结算相对方、收付方向和结算币种」），没有租户就没有目录。第二个返回值为 false 即
// 「显式未配置」——停下，不代拟一个账户去冻别人的钱。
type SettlementAccountDirectory interface {
	FindSettlementAccount(
		ctx context.Context,
		identity psdomain.SourceIdentity,
		terms psdomain.AdoptedSettlementTerms,
	) (sadomain.SettlementAccountID, bool, error)
}

// PolicyBackedControlScopeSource 从已解析的结算政策推导资金作用域（ADR-0044/0047 接通
// 的那条缝）：解析回显给出法人与币种，账户目录补上结算账户。
//
// 它走 CommercialBasisResolver 而不是另开取数口：解析幂等（同一查询交回同一采用），施加
// 与释放两条路径因此拿到同一个作用域，不需要在请求上多背一份回显。
type PolicyBackedControlScopeSource struct {
	commercial psports.CommercialBasisResolver
	directory  SettlementAccountDirectory
}

// NewPolicyBackedControlScopeSource 装配两半。directory 允许为 nil：账户目录属实例半边，
// nil 是「显式未配置」的诚实表达，届时作用域停在未形成——正是首发要停的地方。
func NewPolicyBackedControlScopeSource(
	commercial psports.CommercialBasisResolver,
	directory SettlementAccountDirectory,
) *PolicyBackedControlScopeSource {
	return &PolicyBackedControlScopeSource{commercial: commercial, directory: directory}
}

var _ ControlScopeSource = (*PolicyBackedControlScopeSource)(nil)

// FormControlScope 分三段：解析取回显 → 目录换账户 → 拼作用域。任何一段答「没有」都是
// 未形成而不是错误——解析未含结算政策、目录未配置、映射查无，三者都要停在
// `CONTROL_SCOPE_NOT_CONFIGURED` 等配置或商业依据补齐，而不是让编排把它当故障重试。
func (source *PolicyBackedControlScopeSource) FormControlScope(
	ctx context.Context,
	identity psdomain.SourceIdentity,
	shipmentRequestID psdomain.ShipmentRequestID,
	submissionVersion psdomain.SubmissionVersionID,
) (sadomain.SettlementScope, bool, error) {
	if source.commercial == nil || source.directory == nil {
		return sadomain.SettlementScope{}, false, nil
	}

	resolution, err := source.commercial.ResolveCommercialBasis(ctx, psports.CommercialBasisQuery{
		Identity:          identity,
		ShipmentRequestID: shipmentRequestID,
		SubmissionVersion: submissionVersion,
	})
	if err != nil {
		return sadomain.SettlementScope{}, false, fmt.Errorf("resolve commercial basis for control scope: %w", err)
	}
	terms, present := resolution.Snapshot.SettlementTerms()
	if !present {
		// 解析没成或成了但不含结算政策：两者下都推不出作用域。不在这里区分——该由谁
		// 补什么，判断编排在商业解析那一步早已答过。
		return sadomain.SettlementScope{}, false, nil
	}

	account, found, err := source.directory.FindSettlementAccount(ctx, identity, terms)
	if err != nil {
		return sadomain.SettlementScope{}, false, fmt.Errorf("find settlement account: %w", err)
	}
	if !found {
		return sadomain.SettlementScope{}, false, nil
	}

	legalEntity, err := sadomain.NewLegalEntityReference(terms.LegalEntity().String())
	if err != nil {
		return sadomain.SettlementScope{}, false, fmt.Errorf("%w: legal entity: %v", ErrUntranslatableAnswer, err)
	}
	currency, err := sadomain.NewCurrencyCode(terms.Currency().String())
	if err != nil {
		return sadomain.SettlementScope{}, false, fmt.Errorf("%w: currency: %v", ErrUntranslatableAnswer, err)
	}
	scope, err := sadomain.NewSettlementScope(legalEntity, account, currency)
	if err != nil {
		return sadomain.SettlementScope{}, false, fmt.Errorf("%w: settlement scope: %v", ErrUntranslatableAnswer, err)
	}
	return scope, true, nil
}
