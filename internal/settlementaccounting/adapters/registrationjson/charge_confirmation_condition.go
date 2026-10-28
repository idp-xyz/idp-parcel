package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type chargeConfirmationConditionDocument struct {
	TenantID  string `json:"tenantId"`
	FeeItem   string `json:"feeItem"`
	BasisKind string `json:"basisKind"`
}

// ChargeConfirmationConditionFromJSON 译装一条确认条件：哪个费用项目要哪种确认依据。依据种类只收产品认得的那几种，
// 写错的种类登上去只会让这类费用永远等不到依据，不如当场拒。
func ChargeConfirmationConditionFromJSON(raw []byte) (domain.TenantID, domain.FeeItemReference, domain.ConfirmationBasisKind, error) {
	var document chargeConfirmationConditionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.FeeItemReference{}, domain.ConfirmationBasisKind{},
			fmt.Errorf("确认条件登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemReference{}, domain.ConfirmationBasisKind{}, fmt.Errorf("tenantId：%w", err)
	}
	feeItem, err := domain.NewFeeItemReference(document.FeeItem)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemReference{}, domain.ConfirmationBasisKind{}, fmt.Errorf("feeItem：%w", err)
	}
	kind, err := domain.ConfirmationBasisKindFromName(document.BasisKind)
	if err != nil {
		return domain.TenantID{}, domain.FeeItemReference{}, domain.ConfirmationBasisKind{}, fmt.Errorf("basisKind：%w", err)
	}
	return tenant, feeItem, kind, nil
}
