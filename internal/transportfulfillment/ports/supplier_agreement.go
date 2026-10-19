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
}

// SupplierAgreementSource 按（租户、协议版本、服务提供方、决定时点）核一版供应商协议。
//
// error 只有两种来历：party-commercial 读不动，或它交回的东西不在词汇表里。两者都不得折成任何一格否定结论——
// 把读不到答成「未在册」，一次故障就会让登记方去 party-commercial 补一份其实在册的协议。调用方据 error 停在未决。
type SupplierAgreementSource interface {
	CheckSupplierAgreement(
		ctx context.Context,
		tenant domain.TenantID,
		agreement domain.SupplierAgreementVersion,
		provider domain.ServiceProviderReference,
		at time.Time,
	) (SupplierAgreementCheck, error)
}
