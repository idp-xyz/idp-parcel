package ports

import (
	"context"
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// EvaluationRequestUnformedCause 是计价对一份评价请求答不出评价的那一格，封闭集。三格都不随重投变好：没有在用价卡要
// 租户登一张，多于一版同时适用要人裁，造不出计价输入快照要提供方那一侧补正事实或接上读口（ADR-0229 决定三）。
type EvaluationRequestUnformedCause uint8

const (
	EvaluationRequestUnformedCauseInvalid EvaluationRequestUnformedCause = iota
	UnformedPriceCardNotConfigured
	UnformedPriceCardApplicabilityConflict
	UnformedPricingInputUnavailable
)

func (cause EvaluationRequestUnformedCause) String() string {
	switch cause {
	case UnformedPriceCardNotConfigured:
		return "PRICE_CARD_NOT_CONFIGURED"
	case UnformedPriceCardApplicabilityConflict:
		return "PRICE_CARD_APPLICABILITY_CONFLICT"
	case UnformedPricingInputUnavailable:
		return "PRICING_INPUT_UNAVAILABLE"
	default:
		return ""
	}
}

// EvaluationRequestUnformedIntent 是一次「评价未形成」：哪个租户、哪份请求（连同它的价格方向，结算按方向找请求册）、
// 停在哪一格，以及给人读的说明（冲突的候选、缺的读口或事实）。评价不存在，所以没有评价标识。
type EvaluationRequestUnformedIntent struct {
	Tenant    domain.TenantID
	Request   domain.EvaluationRequestReference
	Direction domain.PricingDirection
	Cause     EvaluationRequestUnformedCause
	Detail    []string
}

// ErrEvaluationRequestUnformedHandoffRejected 表示「评价未形成」信封被框架的确定性校验拒收。同一份重交永远同一个结果。
var ErrEvaluationRequestUnformedHandoffRejected = errors.New("parcel pricing: evaluation request unformed envelope rejected by envelope validation")

// EvaluationRequestUnformedHandoff 把「评价未形成」写入 Outbox，与消费门的入账同一事务：信封没入队，入账就跟着回滚，
// 请求信封重投再来一次——不会出现计价入账了、结算却永远不知道这份请求停在哪的那一格。
type EvaluationRequestUnformedHandoff interface {
	HandOffEvaluationRequestUnformed(ctx context.Context, intent EvaluationRequestUnformedIntent) error
}

// PricingParameterKind 是计价交出的「参数已登记」种类，封闭集：价卡登记（新的一版在册）、序列或目录的一版通过复核
// （自复核时刻起在用）。三类都可能让一份停下的评价请求再问一次得到不同的答案。
type PricingParameterKind uint8

const (
	PricingParameterKindInvalid PricingParameterKind = iota
	PricingParameterPriceCard
	PricingParameterReferenceSeries
	PricingParameterReferenceCatalogue
)

func (kind PricingParameterKind) String() string {
	switch kind {
	case PricingParameterPriceCard:
		return "PRICE_CARD"
	case PricingParameterReferenceSeries:
		return "REFERENCE_SERIES"
	case PricingParameterReferenceCatalogue:
		return "REFERENCE_CATALOGUE"
	default:
		return ""
	}
}

// PricingParameterRegisteredIntent 是一次计价参数落库在一个计价范围上的那一面：租户、种类、哪一版（进信封标识，认领同一次
// 登记的重放）、受影响的计价范围与方向（进载荷，结算按它只续办问这个范围、这一路的请求）。价卡一版一面；序列与目录
// 一版按绑了它的卡各一面——没有卡绑它，就没有等着它的请求，一面都不交。方向只有 BUY 与 SELL：结算的评价请求只有这两路。
//
// Closure 非零说明这一面出自一次价卡收尾（ADR-0234），Registration 是被收尾的那一版。
type PricingParameterRegisteredIntent struct {
	Tenant       domain.TenantID
	Kind         PricingParameterKind
	Registration domain.VersionReference
	Scope        domain.PricingScopeID
	Direction    domain.PricingDirection
	Closure      PriceCardClosureOccasion
}

// PriceCardClosureKind 是价卡收尾的两种，封闭集。
type PriceCardClosureKind uint8

const (
	PriceCardClosureKindInvalid PriceCardClosureKind = iota
	PriceCardCutClosure
	PriceCardRetirementClosure
)

func (kind PriceCardClosureKind) String() string {
	switch kind {
	case PriceCardCutClosure:
		return "CUT"
	case PriceCardRetirementClosure:
		return "RETIREMENT"
	default:
		return ""
	}
}

// PriceCardClosureOccasion 是一次价卡收尾的身份：哪一种、让位给哪一版、凭什么。零值即这一面不是收尾。
//
// 它只进信封标识，不进载荷：结算按种类、范围与方向续办，不问是哪一种价卡登记。进标识是因为收尾那一封必须与旧版、后继
// 各自登记时交的那两封分开——标识撞上会被当成重发吞掉，而后继登记那一封续办出来的请求在两版同时适用时只会停在适用冲突上，
// 能续办出后继的只有收尾这一封。
type PriceCardClosureOccasion struct {
	Kind      PriceCardClosureKind
	Successor string
	Reference string
}

// Present 答这一面是不是出自收尾。
func (occasion PriceCardClosureOccasion) Present() bool {
	return occasion.Kind != PriceCardClosureKindInvalid
}

// PricedScope 是一张卡问的计价范围与方向。
type PricedScope struct {
	Scope     domain.PricingScopeID
	Direction domain.PricingDirection
}

// PricingParameterBindingView 答一条序列或一份目录被哪些卡绑着，交回这些卡各自的计价范围与方向，去重、定序。不按卡的
// 版本与适用期筛：停着的请求的计价基准时点可能落在任何一版的适用期里，按适用期筛就是替它选卡。kind 只收序列与目录，
// id 是序列或目录在租户内的标识（与卡上绑定的那一格同一取值）。
type PricingParameterBindingView interface {
	FindPricedScopesBinding(
		ctx context.Context,
		tenant domain.TenantID,
		kind PricingParameterKind,
		id string,
	) ([]PricedScope, error)
}

// ErrPricingParameterHandoffRejected 表示「参数已登记」信封被框架的确定性校验拒收。登记口据此整笔回滚、交出硬失败。
var ErrPricingParameterHandoffRejected = errors.New("parcel pricing: pricing parameter registered envelope rejected by envelope validation")

// PricingParameterRegisteredHandoff 把「参数已登记」写入 Outbox，与登记或复核行同一事务（ADR-0094 决定四：续办的触发与
// 登记同笔落地）。
type PricingParameterRegisteredHandoff interface {
	HandOffPricingParameterRegistered(ctx context.Context, intent PricingParameterRegisteredIntent) error
}
