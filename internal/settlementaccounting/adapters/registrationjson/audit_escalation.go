package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type auditEscalationCeilingDocument struct {
	TenantID      string `json:"tenantId"`
	SupplierID    string `json:"supplierId"`
	LegalEntityID string `json:"legalEntityId"`
	Currency      string `json:"currency"`
	CeilingMinor  int64  `json:"ceilingMinor"`
}

// AuditEscalationCeilingFromJSON 译装一条金额上限。不判断这笔审核越不越权。
func AuditEscalationCeilingFromJSON(raw []byte) (domain.TenantID, domain.AuditEscalationCeilingRegistration, error) {
	var document auditEscalationCeilingDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.AuditEscalationCeilingRegistration{}, fmt.Errorf("越权升级上限登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.AuditEscalationCeilingRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	ceiling, err := domain.NewAuditEscalationCeiling(document.CeilingMinor)
	if err != nil {
		return domain.TenantID{}, domain.AuditEscalationCeilingRegistration{}, err
	}
	registration, err := domain.NewAuditEscalationCeilingRegistration(
		catalogueRef(domain.NewSupplierPartyReference, document.SupplierID),
		catalogueRef(domain.NewLegalEntityReference, document.LegalEntityID),
		catalogueRef(domain.NewCurrencyCode, document.Currency),
		ceiling,
	)
	if err != nil {
		return domain.TenantID{}, domain.AuditEscalationCeilingRegistration{}, err
	}
	return tenant, registration, nil
}
