package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// operatingBasisAdoptionDocument 的两个布尔用指针：缺键与 JSON null 都是 nil，不能靠 bool 的零值当成「不采用」。
type operatingBasisAdoptionDocument struct {
	TenantID  string `json:"tenantId"`
	Estimated *bool  `json:"estimated"`
	Confirmed *bool  `json:"confirmed"`
}

// OperatingBasisAdoptionFromJSON 译装一户采用哪些内置经营口径。两个布尔都必须出现，缺一个就拒，不把缺席当成 false。
func OperatingBasisAdoptionFromJSON(raw []byte) (domain.TenantID, domain.OperatingBasisAdoption, error) {
	var document operatingBasisAdoptionDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.OperatingBasisAdoption{}, fmt.Errorf("经营口径采用输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.OperatingBasisAdoption{}, fmt.Errorf("tenantId：%w", err)
	}
	if document.Estimated == nil || document.Confirmed == nil {
		return domain.TenantID{}, domain.OperatingBasisAdoption{}, fmt.Errorf("estimated 与 confirmed 都必须出现，缺省不能当成不采用")
	}
	adoption, err := domain.NewOperatingBasisAdoption(*document.Estimated, *document.Confirmed)
	if err != nil {
		return domain.TenantID{}, domain.OperatingBasisAdoption{}, err
	}
	return tenant, adoption, nil
}
