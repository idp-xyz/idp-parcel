package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 供应商协议核对口（票 demo-chain-gaps/22）。运输委托提交与运输收费发生项登记都要先核登记方指名的那一版协议，
// 两处共用这一口；消费侧适配器落在 internal/transportfulfillment/adapters/partycommercial/（ADR-0025）。
//
// 另开一份文件不并进 ports.go，按关注点分文件，理由同 fulfillment_segment.go 的头注。

// SupplierAgreementStanding 是一版协议对一次采购决定的核对结论。
//
// 三项检查按序：这一版在册（壳与正文都登了）；在决定时点支持新的采购决定；协议的供应商就是本次的服务提供方。
// 先不过的那一项定结论，后面的不再看——一版不在有效期的协议，它的供应商对不对上已经无关紧要。四个否定格是
// 业务答案，不是错误：续办各不相同（去 party-commercial 发布或补正文、换一版、取得新协议、改服务方），
// 折成一格调用方就只能叫人去猜（ADR-0029）。
type SupplierAgreementStanding uint8

const (
	SupplierAgreementStandingInvalid SupplierAgreementStanding = iota
	// SupplierAgreementRegistered：在册，且三项都过。只有这一格带采购责任法人与协议快照引用。
	SupplierAgreementRegistered
	// SupplierAgreementNotRegistered：没有这一版，或壳在而正文没登——供应商与责任法人都无从得知。
	SupplierAgreementNotRegistered
	// SupplierAgreementNotEffective：这一版在决定时点不支持新的采购决定（尚未生效、区间未开或已过、已到期、
	// 已被替代）。
	SupplierAgreementNotEffective
	// SupplierAgreementTerminated：这一版已被终止或退役，自那一刻起不再支持新的采购决定。
	SupplierAgreementTerminated
	// SupplierAgreementProviderMismatch：协议在有效期内，但约定的供应商不是本次的服务提供方。
	SupplierAgreementProviderMismatch
)

func (standing SupplierAgreementStanding) String() string {
	switch standing {
	case SupplierAgreementRegistered:
		return "AGREEMENT_REGISTERED"
	case SupplierAgreementNotRegistered:
		return "AGREEMENT_NOT_REGISTERED"
	case SupplierAgreementNotEffective:
		return "AGREEMENT_NOT_EFFECTIVE"
	case SupplierAgreementTerminated:
		return "AGREEMENT_TERMINATED"
	case SupplierAgreementProviderMismatch:
		return "PROVIDER_MISMATCH"
	default:
		return ""
	}
}

// SupplierAgreementCheck 是核对口的答复。
//
// Snapshot 与 LegalEntity 只在在册格有值：采购责任法人取自协议版本，不由调用方给；快照引用是 party-commercial
// 一处拼出的「对象/版本」指称，委托与发生项照原样存它，本上下文不拆不拼。Supplier 在在册与服务方不符两格有值，
// 后者用它告诉登记方这一版协议约定的是谁。
type SupplierAgreementCheck struct {
	Standing    SupplierAgreementStanding
	Snapshot    domain.AgreementSnapshotReference
	LegalEntity domain.ProcurementLegalEntityReference
	Supplier    domain.ServiceProviderReference
	// ChargeConditions 只在在册格有值：这一版协议声明的收费发生条件（ADR-0209），零值即整节没声明。运输委托提交不看
	// 它；发生项登记按它判计不计费、数出数量与单位。
	ChargeConditions domain.AgreementChargeConditions
}

// SupplierAgreementSource 按（租户、协议版本、服务提供方、决定时点）核一版供应商协议。
//
// 否定结论只出自 SupplierAgreementStanding 的格；error 不是其中任何一格，也不得被折成任何一格——把读不到答成
// 「未在册」，一次故障就会让登记方去 party-commercial 补一份其实在册的协议。error 的来历如下，恢复动作各不相同：
//
//   - party-commercial 那一侧失败：读不动（连接、超时之类），或点读时发现它自己的数据写坏了（正文挂在已计划生效
//     的壳上、快照与所问的键不符、正文行不完整）。PC 对这两者交的都是不带哨兵的 error，适配器原样包裹上抛，所以
//     **调用方在值上分不开它们**：前者重试会好；后者要等 PC 的数据修好，在那之前重试只会原样再撞。
//   - 适配器包上 ErrUntranslatableAnswer（adapters/partycommercial）：本上下文的入参在 PC 的词汇表里立不起来，或
//     PC 交回了词汇表之外的东西（所答非所问、正文挂错版本、未知的生命周期位置）。确定性的，重试无益，要修的是
//     PC 读口或适配器。
//   - 零决定时点：调用方违约，不带哨兵。SupportsProcurementAt 对零时点恒答否，照问会被译成一格看似真实的未生效，
//     所以在问 PC 之前就拒。确定性的，要修的是调用方；调用方传自己的提交或登记时刻，就撞不上。
//
// 命令面把任一 error 答成未决即可：登记方能做的只有稍后再交，分不分得开不改变它的续办。若有走派发的消费者用本口
// 落未决哨兵，能用 errors.Is 认出的只有 ErrUntranslatableAnswer，它与零时点都不进名单；PC 那一侧失败进名单时，
// PC 坏数据会随读不动一起被当成未决反复重试，直到数据修好——要分开，得先由 PC 给坏数据一个哨兵。
type SupplierAgreementSource interface {
	CheckSupplierAgreement(
		ctx context.Context,
		tenant domain.TenantID,
		agreement domain.SupplierAgreementVersion,
		provider domain.ServiceProviderReference,
		at time.Time,
	) (SupplierAgreementCheck, error)
}
