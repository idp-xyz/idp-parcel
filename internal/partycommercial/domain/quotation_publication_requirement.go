package domain

// 有发出记录的价卡草稿，发布要不要以一条有效接受为前提。没登记不设门。

type QuotationPublicationRequirement struct {
	tenant   TenantID
	requires bool
}

func NewQuotationPublicationRequirement(tenant TenantID, requires bool) (QuotationPublicationRequirement, error) {
	if !tenant.valid() {
		return QuotationPublicationRequirement{}, ErrInvalidQuotationReply
	}
	return QuotationPublicationRequirement{tenant: tenant, requires: requires}, nil
}

func (requirement QuotationPublicationRequirement) Tenant() TenantID { return requirement.tenant }
func (requirement QuotationPublicationRequirement) Requires() bool   { return requirement.requires }

func (requirement QuotationPublicationRequirement) valid() bool { return requirement.tenant.valid() }
