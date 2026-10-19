package partycommercial

import (
	"context"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SupplierAgreementScope 实现 saports.SupplierAgreementScopeView：把发生项上固定的那一版协议的指称交给 party-commercial，
// 把协议的商业范围译成 BUY 请求的主要范围（ADR-0208 决定四）。
//
// 主要范围等同协议的商业范围，是本路的产品策略：计价按主要范围选在用价卡，演示的成本卡与干线协议同在一个范围里。
// 一份协议覆盖多条线、各线另有成本卡的时候它不够用，那是 ADR-0208 的越权风险点，不在这里补。
type SupplierAgreementScope struct {
	agreements pcports.SupplierAgreementScopeView
}

// NewSupplierAgreementScope 装配读口。不许为 nil：缺读口而静默答「没有这份协议」会把一次装配疏漏报成两侧矛盾。
func NewSupplierAgreementScope(agreements pcports.SupplierAgreementScopeView) (*SupplierAgreementScope, error) {
	if agreements == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: supplier agreement scope view is nil")
	}
	return &SupplierAgreementScope{agreements: agreements}, nil
}

var _ saports.SupplierAgreementScopeView = (*SupplierAgreementScope)(nil)

func (adapter *SupplierAgreementScope) LoadSupplierAgreementScope(
	ctx context.Context,
	tenant sadomain.TenantID,
	agreement sadomain.SupplierAgreementReference,
) (sadomain.PrimaryScopeReference, bool, error) {
	none := sadomain.PrimaryScopeReference{}
	commercialTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	label, err := pcdomain.NewCommercialVersionLabel(agreement.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: supplier agreement: %v", ErrUntranslatableAnswer, err)
	}
	scope, found, err := adapter.agreements.LoadSupplierAgreementScope(ctx, commercialTenant, label)
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement scope: %w", err)
	}
	if !found {
		return none, false, nil
	}
	primary, err := sadomain.NewPrimaryScopeReference(scope.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: supplier agreement scope: %v", ErrUntranslatableAnswer, err)
	}
	return primary, true, nil
}
