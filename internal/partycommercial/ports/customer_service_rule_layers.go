package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CustomerServiceRuleLayerView 是客户服务规则的层次读口（ADR-0176 决定二）：按一份已固定的闭包，一次取回它采纳的
// 那一版客户服务规则正文（合同版）与继承底座的正文，连同各自在场标志。合同版没写的行由消费方取底座那一行，拼接在
// 消费侧，本口不拼。
//
// 收整份闭包而不只收锚点与声明产品，因为底座有两种来路要分辨：闭包上固定了底座（ADR-0268 决定一）就按那一版点读，
// 不回登记册现选，那一版此后被截止、退役或有效性更正都不改口；闭包快照没有底座一格（ADR-0268 之前形成，决定三，
// 只出现在演示库与测试夹具里）才按闭包的锚点与键上声明的服务产品，经 CommercialRegistry.CustomerServiceRuleProductBase
// 此刻现选。现选与固定读的是同一层，「哪一版算产品底座」只有一种答法，消费方不自己挑。
//
// 合同版就是闭包采纳的那一版，本口不重选它。闭包要是唯一已解析、属显式租户、采纳了一版壳上指名客户合同的客户服务
// 规则，且那一版在闭包键的范围里；任一条立不住是调用方违约，走 error——采纳产品层那一版的闭包没有继承底座，单版点读
// 归 CustomerServiceRuleContentView。
//
// 合同版只有在场与未登记两格。底座先看 ProductBaseOutcome，`唯一解析`时再看那一版正文在不在。固定了一版答`唯一解析`，
// 固定了没有答`无适用依据`；`适用冲突`与`解析未决`只可能出自现选那一路，后者今天只由权威读不到造成，不带原因。读正文失败与坏数据走 error，判据同 CustomerServiceRuleContentView。
//
// 本口不核底座正文挂的是不是服务产品：选法与固定都只看壳，壳上什么都没指名而正文挂合同的那一版也会成为底座。交回的
// 每份正文都带着自己的适用声明，核在消费侧（ADR-0176 决定四，比较对象照 ADR-0268 决定二）。
//
// 租户显式入参，同本包其余端口（ADR-0003）：闭包属别的租户即 error，不按闭包上的租户代答。
type CustomerServiceRuleLayerView interface {
	LoadCustomerServiceRuleLayers(
		ctx context.Context,
		tenant domain.TenantID,
		closure domain.CommercialClosure,
	) (CustomerServiceRuleLayers, error)
}

// CustomerServiceRuleLayers 是层次读口的一次答复。两层各用显式布尔标在场，不拿正文零值兼作「未登记」，判据同
// CustomerServiceRuleRow.HasContent。
type CustomerServiceRuleLayers struct {
	ContractTier    domain.CustomerServiceRuleVersion
	HasContractTier bool
	// ProductBaseOutcome 是底座选法的答案；ProductBase 只在它为`唯一解析`且那一版正文已登记时在场。
	ProductBaseOutcome domain.ResolutionOutcome
	ProductBase        domain.CustomerServiceRuleVersion
	HasProductBase     bool
}
