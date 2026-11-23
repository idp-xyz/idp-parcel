package ports

import (
	"context"
	"time"

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
	// ContractNamesPricingScope 答这位相对方名下、此刻仍适用的合同版本里，有没有一份销售价格来源指名这个计价范围。
	// at 是读的时刻。账户已停用、合同版本已退役、已被后继截止，或选用区间不含 at，都不算。
	ContractNamesPricingScope(ctx context.Context, tenant domain.TenantID, partyID, scope string, at time.Time) (bool, error)
}
