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

// AsOfPolicyDeclaration 取某个接单规则包为各类下游判断声明的时点锚。
//
// 它与 CommercialAuthorityView 分开，因为两阶段机制的全部意义就在这个分界上：第一阶段用独立
// 于候选规则包的选择锚点选出规则包，第二阶段才由已选出的那个包声明各判断的 `asOf`。合成一个
// 端口，规则包就有机会参与决定它自己被选中的时间。
//
// 租户是显式入参：时点政策按租户登记为 `PAR-COM-14`，而按 ADR-0003 跨租户必须在签名上看得见。
//
// 没有租户时它必然交回空声明，编排据以停在`未配置`。这正是首发唯一走得到的真实分支——那组政策
// 属实例半边，本上下文不内置任何默认，尤其不拿系统当前时间顶替。
type AsOfPolicyDeclaration interface {
	LoadAsOfPolicies(
		ctx context.Context,
		tenant domain.TenantID,
		rulePackage domain.CommercialVersion,
	) ([]domain.AsOfPolicy, error)
}

// CommercialResolutionStore 按解析标识取回一次已固定的解析。
//
// 用例步骤 5 要求本上下文「固定解析标识、判断时间、锚点、版本、有效区间和当前修订」并「返回
// 不可覆盖解析结果」，`AT-PC-024` 又要求相同输入与修订「返回原解析语义」——两处都要求本上下文
// 对一次解析负有超出单次调用的责任。本端口就是那份责任的接口（[ADR-0027](../../../docs/adr/0027-multi-step-cross-context-protocol-state-held-by-the-provider.md)）。
//
// 它的存在是为了让后续阶段只回指标识：调用方带着整个闭包回来，键就可以被替换，一次「校验」
// 便能拿另一个范围的视图去证明这份解析仍然成立。
//
// 租户是显式入参，与本包另外两个端口同理：按 ADR-0003 跨越租户必须在签名上看得见。取回后
// 调用方身份仍要与解析键比对——解析标识不是一张能力凭证。
type CommercialResolutionStore interface {
	LoadResolution(
		ctx context.Context,
		tenant domain.TenantID,
		resolution domain.ResolutionID,
	) (domain.CommercialClosure, bool, error)
}

type Clock interface {
	Now() time.Time
}
