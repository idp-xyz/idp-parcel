package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// CustomerServiceRuleLayerView 是客户服务规则的层次读口（ADR-0176 决定二）：一次取回闭包已采纳的那一版客户服务
// 规则正文（合同版）与同范围产品底座版的正文，连同各自在场标志。合同版没写的行由消费方取底座版那一行，拼接在
// 消费侧，本口不拼。
//
// 底座在本口内按 CommercialRegistry.CustomerServiceRuleProductBase 选，消费方不自己挑：闭包合同层零候选时回落的
// 就是那一层，「哪一版算产品底座」只有一种答法。底座不随闭包冻结，按传入的锚点读时选；已接受委托的底座只靠锚点
// 固定与版本不回溯生效保持不变。
//
// contractRule 是闭包采纳的那一版，即决定二读口键里的「合同版本」，本口不重选它：受理时冻结的引用不因后续发布
// 改口。declaredProduct 是冻结闭包键上声明的服务产品，可缺席，收窄规则与闭包回落层同一；不传它，同范围两个
// 产品各有一版时底座答`适用冲突`，而闭包在同一处答唯一。
//
// 合同版只有在场与未登记两格。底座先看选法的答案（ProductBaseOutcome），`唯一解析`时再看那一版正文在不在；多候选
// 答`适用冲突`、零候选答`无适用依据`，都不是 error。锚点立不住与权威读不到同答`解析未决`，不带原因。读正文失败与
// 坏数据走 error，判据同 CustomerServiceRuleContentView。
//
// 本口不核底座正文挂的是不是服务产品：选法只看壳，壳上什么都没指名而正文挂合同的那一版也会被选作底座。交回的
// 每份正文都带着自己的适用声明，核在消费侧（ADR-0176 决定四）。
//
// 租户显式入参，同本包其余端口（ADR-0003）。显式租户必须与合同版同一身份，范围必须是合同版自己的范围：别的范围的
// 产品版对这一户不可见，拿它补行就是跨范围套条款。
type CustomerServiceRuleLayerView interface {
	LoadCustomerServiceRuleLayers(
		ctx context.Context,
		tenant domain.TenantID,
		scope domain.CommercialScopeReference,
		contractRule domain.CommercialVersion,
		anchor domain.SelectionAnchor,
		declaredProduct domain.CommercialObjectID,
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
