package partycommercial

import (
	"context"
	"errors"
	"fmt"

	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// SupplierAgreementSupplier 实现 saports.SupplierAgreementSupplierView：把预期成本所依那一版协议的指称交给 party-commercial，
// 把协议正文上的供应商（往来单位标识）译成结算账单上同一种供应商引用（ADR-0358 决定一）。两侧说的是同一个往来单位，
// 这里只换类型，不换值——换了值，匹配门比对的就不是同一个东西。
//
// 译不过去、指称歧义与 PC 库里存的供应商立不成标识都包 saports.ErrSupplierAgreementSupplierUntranslatable：三种都是确定性的，
// 编排凭它与读不动分开——形成上抛不重投，补记把那一版留空列出、不为它挡住其余各版（ADR-0358 决定三）。
type SupplierAgreementSupplier struct {
	agreements pcports.SupplierAgreementSupplierView
}

// NewSupplierAgreementSupplier 装配读口。不许为 nil：缺读口而静默答「没有这份协议」会把一次装配疏漏报成两侧矛盾。
func NewSupplierAgreementSupplier(agreements pcports.SupplierAgreementSupplierView) (*SupplierAgreementSupplier, error) {
	if agreements == nil {
		return nil, fmt.Errorf("settlement accounting partycommercial adapter: supplier agreement supplier view is nil")
	}
	return &SupplierAgreementSupplier{agreements: agreements}, nil
}

var _ saports.SupplierAgreementSupplierView = (*SupplierAgreementSupplier)(nil)

func (adapter *SupplierAgreementSupplier) LoadSupplierAgreementSupplier(
	ctx context.Context,
	tenant sadomain.TenantID,
	agreement sadomain.SupplierAgreementReference,
) (sadomain.SupplierPartyReference, bool, error) {
	none := sadomain.SupplierPartyReference{}
	commercialTenant, err := pcdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: %w: tenant: %v", saports.ErrSupplierAgreementSupplierUntranslatable, ErrUntranslatableAnswer, err)
	}
	label, err := pcdomain.NewCommercialVersionLabel(agreement.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: %w: supplier agreement: %v", saports.ErrSupplierAgreementSupplierUntranslatable, ErrUntranslatableAnswer, err)
	}
	party, found, err := adapter.agreements.LoadSupplierAgreementSupplier(ctx, commercialTenant, label)
	if errors.Is(err, pcports.ErrSupplierAgreementLabelAmbiguous) || errors.Is(err, pcports.ErrSupplierAgreementSupplierMalformed) {
		return none, false, fmt.Errorf("load supplier agreement supplier: %w: %w", saports.ErrSupplierAgreementSupplierUntranslatable, err)
	}
	if err != nil {
		return none, false, fmt.Errorf("load supplier agreement supplier: %w", err)
	}
	if !found {
		return none, false, nil
	}
	supplier, err := sadomain.NewSupplierPartyReference(party.String())
	if err != nil {
		return none, false, fmt.Errorf("%w: %w: supplier: %v", saports.ErrSupplierAgreementSupplierUntranslatable, ErrUntranslatableAnswer, err)
	}
	return supplier, true, nil
}
