package domain

import (
	"errors"
	"time"
)

// ErrInvalidAlternateJourney 是新旅程立不起来：缺关联、缺决定依据或身份与原旅程重合。
var ErrInvalidAlternateJourney = errors.New("transport fulfillment: invalid alternate journey")

// JourneyPurpose 是新旅程服务目的的封闭二值：备用切换/改送（ALTERNATE）或退运（RETURN）。
type JourneyPurpose uint8

const (
	JourneyPurposeInvalid JourneyPurpose = iota
	AlternateJourneyPurpose
	ReturnJourneyPurpose
)

func (purpose JourneyPurpose) valid() bool {
	return purpose == AlternateJourneyPurpose || purpose == ReturnJourneyPurpose
}

func (purpose JourneyPurpose) String() string {
	switch purpose {
	case AlternateJourneyPurpose:
		return "ALTERNATE"
	case ReturnJourneyPurpose:
		return "RETURN"
	default:
		return ""
	}
}

// DispositionBasisKind 把新旅程的决定来源分格：普通/商业处置决定（UC-PS-006 一类）走
// 常规运输主链，监管处置决定由 UC-TF-001 按监管协作范围承接。监管退运不得用普通退运
// 冒充——冒充首先要抹掉这一格，而它是必填的封闭集合。
type DispositionBasisKind uint8

const (
	DispositionBasisKindInvalid DispositionBasisKind = iota
	ServiceDispositionDecision
	RegulatoryDispositionDecision
)

func (kind DispositionBasisKind) valid() bool {
	return kind == ServiceDispositionDecision || kind == RegulatoryDispositionDecision
}

func (kind DispositionBasisKind) String() string {
	switch kind {
	case ServiceDispositionDecision:
		return "SERVICE_DISPOSITION"
	case RegulatoryDispositionDecision:
		return "REGULATORY_DISPOSITION"
	default:
		return ""
	}
}

// DispositionBasisReference 指名启动新旅程的处置决定。拒收、失败尝试或异常案件都不是
// 决定——只有事实没有决定时新旅程立不起来（AT-TF-078）。
type DispositionBasisReference struct{ requiredValue }

func NewDispositionBasisReference(value string) (DispositionBasisReference, error) {
	required, err := newRequiredValue("disposition basis reference", value)
	return DispositionBasisReference{required}, err
}

// AlternateJourneySpec 是建立新旅程所需的全部输入。
type AlternateJourneySpec struct {
	TenantID        TenantID
	Journey         JourneyReference
	Purpose         JourneyPurpose
	OriginalJourney JourneyReference
	BasisKind       DispositionBasisKind
	Basis           DispositionBasisReference
	Members         []CarriedObjectReference
	StartedAt       time.Time
}

// AlternateJourney 是围绕新的服务目的形成的**关联但独立**的旅程（CONTEXT：「退运围绕
// 新的服务目的形成关联但独立的旅程、路由计划和实际履约过程；不得通过倒退原实际履约段、
// 原路由或原交付状态表达」）。
//
// 独立是结构性的：本类型只持有对原旅程的引用，不持有原段、原交付或原交接的本体，方法集
// 也没有任何触碰原旅程的入口——原旅程的中断、提前终止或继续执行由原对象按事实自行形成
// （AT-TF-077），新旅程建不建立都改不了它们。
type AlternateJourney struct {
	tenantID        TenantID
	journey         JourneyReference
	purpose         JourneyPurpose
	originalJourney JourneyReference
	basisKind       DispositionBasisKind
	basis           DispositionBasisReference
	members         []CarriedObjectReference
	startedAt       time.Time
}

// FormAlternateJourney 建立新旅程。关联原旅程不可缺，且新身份必须不同于原旅程——同一个
// 引用不是「关联」，是把原旅程自己当成新旅程重开；处置决定依据必备，拒收或失败事实顶替
// 不了它。
func FormAlternateJourney(spec AlternateJourneySpec) (AlternateJourney, error) {
	if !spec.TenantID.valid() ||
		!spec.Journey.valid() ||
		!spec.Purpose.valid() ||
		!spec.OriginalJourney.valid() ||
		!spec.BasisKind.valid() ||
		!spec.Basis.valid() ||
		len(spec.Members) == 0 ||
		spec.StartedAt.IsZero() {
		return AlternateJourney{}, ErrInvalidAlternateJourney
	}
	if spec.Journey == spec.OriginalJourney {
		return AlternateJourney{}, ErrInvalidAlternateJourney
	}
	seen := make(map[CarriedObjectReference]struct{}, len(spec.Members))
	for _, member := range spec.Members {
		if !member.valid() {
			return AlternateJourney{}, ErrInvalidAlternateJourney
		}
		if _, exists := seen[member]; exists {
			return AlternateJourney{}, ErrInvalidAlternateJourney
		}
		seen[member] = struct{}{}
	}
	return AlternateJourney{
		tenantID:        spec.TenantID,
		journey:         spec.Journey,
		purpose:         spec.Purpose,
		originalJourney: spec.OriginalJourney,
		basisKind:       spec.BasisKind,
		basis:           spec.Basis,
		members:         append([]CarriedObjectReference(nil), spec.Members...),
		startedAt:       spec.StartedAt.UTC(),
	}, nil
}

func (journey AlternateJourney) TenantID() TenantID {
	return journey.tenantID
}

func (journey AlternateJourney) Journey() JourneyReference {
	return journey.journey
}

func (journey AlternateJourney) Purpose() JourneyPurpose {
	return journey.purpose
}

// OriginalJourney 交回被关联的原旅程引用。只是引用：原旅程的发生项、段与结果继续归属
// 原旅程，新旅程不得搬移或覆盖（CONTEXT 成本来源节）。
func (journey AlternateJourney) OriginalJourney() JourneyReference {
	return journey.originalJourney
}

func (journey AlternateJourney) BasisKind() DispositionBasisKind {
	return journey.basisKind
}

func (journey AlternateJourney) Basis() DispositionBasisReference {
	return journey.basis
}

func (journey AlternateJourney) Members() []CarriedObjectReference {
	return append([]CarriedObjectReference(nil), journey.members...)
}

func (journey AlternateJourney) StartedAt() time.Time {
	return journey.startedAt
}

// RegulatoryOrigin 报告新旅程是否由监管处置承接（UC-TF-001）而来。消费方据此分流：
// 监管范围的续办、限制与核对不走普通退运的路（AT-TF-080）。
func (journey AlternateJourney) RegulatoryOrigin() bool {
	return journey.basisKind == RegulatoryDispositionDecision
}
