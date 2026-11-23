package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// 发布门要用的事实。发出记录与答复在本上下文，计价只拿到裁过的一格。

type QuotationSendRef struct {
	ID     string
	Digest string
}

type QuotationPublicationView interface {
	LoadQuotationPublicationRequirement(ctx context.Context, tenant domain.TenantID) (requires bool, found bool, err error)
	SaveQuotationPublicationRequirement(ctx context.Context, requirement domain.QuotationPublicationRequirement) (same bool, saved bool, err error)
	ListQuotationSendsForPlan(ctx context.Context, tenant domain.TenantID, planID, planVersion string) ([]QuotationSendRef, error)
	AcceptedQuotationReply(ctx context.Context, tenant domain.TenantID, quotationID string) (digest string, accepted bool, found bool, err error)
	ContractNamesPricingScope(ctx context.Context, tenant domain.TenantID, partyID, scope string) (bool, error)
}
