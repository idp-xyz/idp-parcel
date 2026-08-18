package postgres

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// CommercialAuthority 实现 ports.CommercialAuthorityView：交回一个范围下的当前权威
// 商业视图。
//
// 它不自带 SQL，而是从发布登记册派生——「当前权威视图」与「该范围已发布的版本册」
// 在本上下文里是同一份事实（PublicationRegistry.LoadForScope 的存在理由就写着「整册
// 按（租户+范围）取回供解析选用」）。另写一条同义的查询会立第二处口径，而两处口径
// 分歧的那一刻，解析会按其中一处走且没人说得清是哪一处。
//
// 两个端口仍分开，不是同一个类型挂两个方法：写侧（登记已发布版本）与读侧（解析取权威
// 视图）由不同调用方依赖，合并会让只需读的解析持有 SaveVersion。
//
// **已知的收窄**：登记册的完整形状还含价格/结算政策（ADR-0034/0044），这两册今天
// 还没有持久化面——本仓也没有任何生产代码调用 RegisterPricePolicy /
// RegisterSettlementPolicy。区间更正册（ADR-0038）已随本口的 LoadForScope 装入。
// 本口现在交回的就是全部已落库的权威事实，不是「省略了几册」。其余册子拿到持久化面
// 时，装配点在这里而不在解析侧：把它们漏在外面会让 ViewRevision 按不完整的内容派生，
// 从而在视图其实已经变了的时候答「还是同一个视图」。
type CommercialAuthority struct {
	registry ports.PublicationRegistry
}

func NewCommercialAuthority(registry ports.PublicationRegistry) (*CommercialAuthority, error) {
	if registry == nil {
		return nil, fmt.Errorf("party commercial postgres: publication registry is nil")
	}
	return &CommercialAuthority{registry: registry}, nil
}

var _ ports.CommercialAuthorityView = (*CommercialAuthority)(nil)

// LoadScope 按范围取回权威视图。读取失败照原样上抛：解析编排把 error 与空视图分开
// 处理（前者停在`权威不可读`等重试，后者才是「这个范围没有适用依据」），在这里把
// 失败折成空登记册会让一次超时被下游读成这个客户没有合同。
func (authority *CommercialAuthority) LoadScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	registry, err := authority.registry.LoadForScope(ctx, tenant, scope)
	if err != nil {
		return nil, fmt.Errorf("load commercial authority view: %w", err)
	}
	return registry, nil
}
