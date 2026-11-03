package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// expectedCostSupplierBackfillDocument 是补记口的输入（ADR-0358 决定三）：只点名租户。补记的对象由库里仍空着的那几格定，
// 供应商由各自的协议定，没有一格由人填——人填就是在协议之外给供应商另立一处出处。
type expectedCostSupplierBackfillDocument struct {
	TenantID string `json:"tenantId"`
}

// ExpectedCostSupplierBackfillFromJSON 译装一次补记。
func ExpectedCostSupplierBackfillFromJSON(raw []byte) (application.BackfillExpectedCostSupplierCommand, error) {
	none := application.BackfillExpectedCostSupplierCommand{}
	var document expectedCostSupplierBackfillDocument
	if err := decodeStrict(raw, &document); err != nil {
		return none, fmt.Errorf("预期成本供应商补记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return none, fmt.Errorf("tenantId：%w", err)
	}
	return application.BackfillExpectedCostSupplierCommand{TenantID: tenant}, nil
}
