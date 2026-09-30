package registrationjson

import (
	"fmt"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

type allocationFormDocument struct {
	TenantID    string `json:"tenantId"`
	RuleVersion string `json:"ruleVersion"`
	Form        string `json:"form"`
}

// AllocationFormFromJSON 译装一条分法选用。不收权重，也不把来源金额展开成份额。
func AllocationFormFromJSON(raw []byte) (domain.TenantID, domain.AllocationFormRegistration, error) {
	var document allocationFormDocument
	if err := decodeStrict(raw, &document); err != nil {
		return domain.TenantID{}, domain.AllocationFormRegistration{}, fmt.Errorf("分摊分法登记输入不是本入口的形状：%w", err)
	}
	tenant, err := domain.NewTenantID(document.TenantID)
	if err != nil {
		return domain.TenantID{}, domain.AllocationFormRegistration{}, fmt.Errorf("tenantId：%w", err)
	}
	rule, err := domain.NewAllocationRuleVersionReference(document.RuleVersion)
	if err != nil {
		return domain.TenantID{}, domain.AllocationFormRegistration{}, fmt.Errorf("ruleVersion：%w", err)
	}
	form, err := domain.AllocationFormFromName(document.Form)
	if err != nil {
		return domain.TenantID{}, domain.AllocationFormRegistration{}, fmt.Errorf("form：%w", err)
	}
	registration, err := domain.NewAllocationFormRegistration(rule, form)
	if err != nil {
		return domain.TenantID{}, domain.AllocationFormRegistration{}, err
	}
	return tenant, registration, nil
}
