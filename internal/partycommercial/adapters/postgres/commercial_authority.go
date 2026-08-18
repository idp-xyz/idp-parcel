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
// 在本上下文里是同一份事实（CommercialPublicationView.LoadForScope 的存在理由就写着
// 「整册按（租户+范围）取回供解析选用」）。另写一条同义的查询会立第二处口径，而两处
// 口径分歧的那一刻，解析会按其中一处走且没人说得清是哪一处。
//
// 本适配器只依赖只读口。写侧（SaveVersion 及各通道具名 Save）留在 PublicationRegistry
// 上，由登记调用方持有——合并会让只需读的解析持有写能力，而那正是两端口分开的理由。
//
// **已知的收窄**：族 A 四册（形态、更正、价格、结算）已随 LoadForScope 装入。登记册
// 上其余内容属族 B 点读（合同正文、规则包正文、阶段声明），本来就不进 ViewRevision。
// 本口现在交回的就是全部已落库的权威视图事实。把族 A 漏在外面会让 ViewRevision 按不
// 完整的内容派生，从而在视图其实已经变了的时候答「还是同一个视图」。
type CommercialAuthority struct {
	publications ports.CommercialPublicationView
}

func NewCommercialAuthority(publications ports.CommercialPublicationView) (*CommercialAuthority, error) {
	if publications == nil {
		return nil, fmt.Errorf("party commercial postgres: commercial publication view is nil")
	}
	return &CommercialAuthority{publications: publications}, nil
}

var _ ports.CommercialAuthorityView = (*CommercialAuthority)(nil)

// 写侧登记册内嵌只读口，因此仍可交给本构造器；只实现 LoadForScope 的类型同样可以。
var _ ports.CommercialPublicationView = ports.PublicationRegistry(nil)

// LoadScope 按范围取回权威视图。读取失败照原样上抛：解析编排把 error 与空视图分开
// 处理（前者停在`权威不可读`等重试，后者才是「这个范围没有适用依据」），在这里把
// 失败折成空登记册会让一次超时被下游读成这个客户没有合同。
func (authority *CommercialAuthority) LoadScope(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.CommercialScopeReference,
) (*domain.CommercialRegistry, error) {
	registry, err := authority.publications.LoadForScope(ctx, tenant, scope)
	if err != nil {
		return nil, fmt.Errorf("load commercial authority view: %w", err)
	}
	return registry, nil
}
