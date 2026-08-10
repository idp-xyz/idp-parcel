// Package ports 声明 party-commercial 自有的语义边界。这些只是接口：它们的 PostgreSQL
// 适配器仍阻断在 Bento 持久化闸门之后（ADR-0017），今天唯一的实现是测试用替身。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CommercialAuthorityView 取一个范围下的当前权威商业视图。
//
// 按范围取而不是按对象取，是因为解析要回答的是「这个范围里有几个适用候选」。逐对象取
// 看不见新增的同范围候选，而那恰恰是提交前失效要抓的东西——已采用对象自身一字未动，
// 解析却已经不再唯一。
//
// 租户是显式入参，不从 context 里补：按 ADR-0003 运营集团租户是最高数据隔离边界，跨越
// 它必须在签名上看得见。
type CommercialAuthorityView interface {
	LoadScope(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.CommercialScopeReference,
	) (*domain.CommercialRegistry, error)
}

type Clock interface {
	Now() time.Time
}
