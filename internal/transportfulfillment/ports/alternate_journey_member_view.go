package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// AlternateJourneyMemberView 按成员读替代 / 退运旅程册：实际履约发生项自动形成时，要知道对象在参与入场那一刻走的是不是
// 一趟新旅程（ADR-0218）。只读、只按成员问；哪一趟算数由领域 ActualFulfillmentJourney 判，本口全交回、不挑。
//
// 另立文件、不并进 ports.go 的 AlternateJourneyStore：那个口带写，依赖它等于声明发生项形成可能开新旅程；新口放新文件，
// 别处写的替身也不必跟着加方法（SegmentCommissionAdoptionStore 文件头同一条理由）。AlternateJourneys 适配器天然满足它。
type AlternateJourneyMemberView interface {
	FindByMember(ctx context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]AlternateJourneyRecord, error)
}
