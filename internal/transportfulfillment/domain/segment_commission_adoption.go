package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidCommissionAdoption = errors.New("transport fulfillment: invalid segment commission adoption")
	// ErrCommissionNotAdoptable：委托在这一段开始之前就已取消。取消停的正是尚未消耗的运输意图，再把一个段挂上去等于把停掉的
	// 意图续上，而已取消的委托也没有可开始的意图（MarkTransportStarted 同一条）。
	ErrCommissionNotAdoptable = errors.New("transport fulfillment: the commission is cancelled and cannot be adopted")
	// ErrCommissionCancelledAfterStart：委托已取消，但取消时刻不早于这一段的首个参与。按业务时间，取消到达时这一段下的运输
	// 已经开始，取消本该只停尚可停止的范围（AT-TF-092）；它之所以成立，是因为采用登记晚于实际开始，取消那一刻本上下文还不知道
	// 这一段属于这份委托。两份事实在业务时间上相抵，采用不替任何一方改：不撤已经答过成立的取消，也不把段挂到已取消的委托上
	// （ADR-0204 决定三「采用与取消的先后」）。与 ErrCommissionNotAdoptable 分开，是因为续办不同——那一格是另起委托，这一格
	// 是先更正取消或更正控制事实。
	ErrCommissionCancelledAfterStart = errors.New("transport fulfillment: the commission was cancelled after the segment had started")
	// ErrAdoptionProviderMismatch：声明的服务方不是委托的服务提供方。采用声明的是「这一段由这家按这份委托履约」，两头
	// 对不上说明指错了委托或指错了服务方，不能挑一个当真。
	ErrAdoptionProviderMismatch = errors.New("transport fulfillment: the declared service provider is not the commission's")
	// ErrSegmentWithoutEffectiveParticipation：段在册，但每一条参与都已被替代链改判失效，没有一条当前参与可作开始依据。
	ErrSegmentWithoutEffectiveParticipation = errors.New("transport fulfillment: the segment has no effective participation to start from")
	// ErrSegmentStartedBeforeCommission：段的首个参与早于委托的提交时刻。委托下的运输不可能先于委托开始，两份事实里
	// 至少一份的时刻填错了，本处不替任何一方改。
	ErrSegmentStartedBeforeCommission = errors.New("transport fulfillment: the segment started before the commission was submitted")
)

// SegmentCommissionAdoption 是「履约段采用运输委托」这一声明（ADR-0204）：登记方声明一个在册的实际履约段在一份已提交的
// 运输委托下履约，并随之以该段首个参与作为这份委托下运输已开始的依据。
//
// 连线是声明，不是推导（ADR-0096 同判据）：本上下文不从计划段、承运商判断、交接接收方或协议有无推出哪一段该挂哪份委托，
// 没有连线的段也不读作自营——自营与外包不由委托有无推断（CONTEXT「履约主体与承运证据」）。一段至多采用一份委托，采用之后
// 不改、不解除（ADR-0204 越权风险点「没有反向解除采用的面」）；同一份委托可以被多段采用。
//
// 开始依据与开始时刻随连线一起保存：委托上的开始是已采用各段里最早的那一次，而每一段在这份委托下何时、凭哪条控制事实开始，
// 是这一段自己的事实，不能借别的段的时刻。
type SegmentCommissionAdoption struct {
	tenantID   TenantID
	segment    FulfillmentSegmentReference
	commission TransportCommissionReference
	provider   ServiceProviderReference
	startBasis ParticipationBasisReference
	startedAt  time.Time
}

// CommissionStartChange 说一次采用要对委托的开始做什么。委托的开始是已采用各段首个参与里最早的那一条：先被采用的段未必先
// 开始，后到的段可能把开始往前带。只前移、不后移——后移就是让一段已经开始过的运输「还没开始」，那是 UC-TF-004「一致性、
// 幂等与并发」的「已发生事实不回退」挡的。
type CommissionStartChange uint8

const (
	CommissionStartChangeInvalid CommissionStartChange = iota
	// CommissionStartKept：委托已开始，且开始不晚于这一段的首个参与，原样不动。
	CommissionStartKept
	// CommissionStartMarked：委托尚未开始，以这一段的首个参与标为已开始。
	CommissionStartMarked
	// CommissionStartAdvanced：委托已开始，但这一段的首个参与更早，开始前移到这一条。
	CommissionStartAdvanced
)

func (change CommissionStartChange) String() string {
	switch change {
	case CommissionStartKept:
		return "KEPT"
	case CommissionStartMarked:
		return "MARKED"
	case CommissionStartAdvanced:
		return "ADVANCED"
	default:
		return ""
	}
}

// AdoptCommissionForSegment 判一次采用声明，交回连线、委托此后的样子，以及委托的开始要怎么变（CommissionStartChange）。
//
// 判序：声明不成形 → 服务方不符 → 委托已取消（取消不早于段的首个参与另成一格）→ 段没有当前参与 → 段早于委托提交就已开始。
// 服务方排在取消之前：服务方对不上多半是指错了委托，告诉登记方「这份已取消」会让他去处理一份本不相干的委托。
//
// 已取消的委托按业务时间分两格（UC-TF-004「以业务时间和有效边界裁决」）：段在委托提交之后开始、取消又不早于那一刻的，答
// ErrCommissionCancelledAfterStart；其余——取消早于段的开始，或段没有当前参与、或段早于委托提交——答 ErrCommissionNotAdoptable：
// 前者委托确实是在运输开始之前停掉的，后两者没有一个落在委托下的开始可以拿来和取消比。
func AdoptCommissionForSegment(
	segment ActualFulfillmentSegment,
	commission TransportCommission,
	provider ServiceProviderReference,
) (adoption SegmentCommissionAdoption, adopted TransportCommission, change CommissionStartChange, err error) {
	if !segment.tenantID.valid() ||
		!segment.segment.valid() ||
		!segment.Established() ||
		!commission.commission.valid() ||
		commission.tenantID != segment.tenantID ||
		!provider.valid() {
		return SegmentCommissionAdoption{}, TransportCommission{}, CommissionStartChangeInvalid, ErrInvalidCommissionAdoption
	}
	if provider != commission.provider {
		return SegmentCommissionAdoption{}, TransportCommission{}, CommissionStartChangeInvalid, ErrAdoptionProviderMismatch
	}
	first, present := segment.FirstEffectiveParticipation()
	if cancelledAt, cancelled := commission.Cancelled(); cancelled {
		if present && !first.enteredAt.Before(commission.submittedAt) && !cancelledAt.Before(first.enteredAt) {
			return SegmentCommissionAdoption{}, TransportCommission{}, CommissionStartChangeInvalid, ErrCommissionCancelledAfterStart
		}
		return SegmentCommissionAdoption{}, TransportCommission{}, CommissionStartChangeInvalid, ErrCommissionNotAdoptable
	}
	if !present {
		return SegmentCommissionAdoption{}, TransportCommission{}, CommissionStartChangeInvalid, ErrSegmentWithoutEffectiveParticipation
	}
	if first.enteredAt.Before(commission.submittedAt) {
		return SegmentCommissionAdoption{}, TransportCommission{}, CommissionStartChangeInvalid, ErrSegmentStartedBeforeCommission
	}
	adoption = SegmentCommissionAdoption{
		tenantID:   segment.tenantID,
		segment:    segment.segment,
		commission: commission.commission,
		provider:   provider,
		startBasis: first.entryBasis,
		startedAt:  first.enteredAt.UTC(),
	}
	_, startedAt, started := commission.TransportStarted()
	switch {
	case !started:
		change = CommissionStartMarked
		adopted, err = commission.MarkTransportStarted(first.entryBasis, first.enteredAt)
	case first.enteredAt.Before(startedAt):
		change = CommissionStartAdvanced
		adopted, err = commission.advanceTransportStart(first.entryBasis, first.enteredAt)
	default:
		return adoption, commission, CommissionStartKept, nil
	}
	if err != nil {
		return SegmentCommissionAdoption{}, TransportCommission{}, CommissionStartChangeInvalid, err
	}
	return adoption, adopted, change, nil
}

// advanceTransportStart 把已开始委托的开始前移到更早的一条控制事实。只给采用用：开始依据只能是某一段的首个参与，别的来源
// 没有「更早的一条控制事实」可给。与「开始取首个参与、不取登记时刻」同一个理由：取消订舱按申请时刻对开始时刻判（CancelBooking），
// 开始偏晚，运输开始之后为剩余范围提出的订舱就被读作开始之前提出、取消不了；前移之后，申请落在新旧两个开始之间的订舱回到
// 可取消。
func (commission TransportCommission) advanceTransportStart(basis ParticipationBasisReference, at time.Time) (TransportCommission, error) {
	_, startedAt, started := commission.TransportStarted()
	if !started || !basis.valid() || AtRegistryPrecision(at).IsZero() || at.Before(commission.submittedAt) || !at.Before(startedAt) {
		return TransportCommission{}, ErrInvalidTransportCommission
	}
	advanced := commission
	advanced.members = append([]CarriedObjectReference(nil), commission.members...)
	advanced.startedBasis = basis
	advanced.startedAt = at.UTC()
	return advanced, nil
}

// FirstEffectiveParticipation 交该段入场最早的那一条当前参与：被替代的版本与失效版本不算——它们记的入场已被更正，不再是
// 控制的起点；已离场的照算，开始过就是开始过。同一时刻入场的按入场依据的字面序取第一条，同一份段读几次都交同一条。
func (segment ActualFulfillmentSegment) FirstEffectiveParticipation() (FulfillmentParticipation, bool) {
	var first FulfillmentParticipation
	found := false
	for _, participation := range segment.participations {
		if participation.superseded || participation.voided {
			continue
		}
		if !found ||
			participation.enteredAt.Before(first.enteredAt) ||
			(participation.enteredAt.Equal(first.enteredAt) && participation.entryBasis.String() < first.entryBasis.String()) {
			first = participation
			found = true
		}
	}
	return first, found
}

func (adoption SegmentCommissionAdoption) TenantID() TenantID {
	return adoption.tenantID
}

func (adoption SegmentCommissionAdoption) Segment() FulfillmentSegmentReference {
	return adoption.segment
}

func (adoption SegmentCommissionAdoption) Commission() TransportCommissionReference {
	return adoption.commission
}

// Provider 是登记方声明、并已核过等于委托服务提供方的那一家。
func (adoption SegmentCommissionAdoption) Provider() ServiceProviderReference {
	return adoption.provider
}

// Started 交这一段在这份委托下开始的依据与时刻：采用时段的首个参与。
func (adoption SegmentCommissionAdoption) Started() (ParticipationBasisReference, time.Time) {
	return adoption.startBasis, adoption.startedAt
}

// RehydrateSegmentCommissionAdoptionSpec 是采用行在库里的样子。
type RehydrateSegmentCommissionAdoptionSpec struct {
	TenantID   TenantID
	Segment    FulfillmentSegmentReference
	Commission TransportCommissionReference
	Provider   ServiceProviderReference
	StartBasis ParticipationBasisReference
	StartedAt  time.Time
}

// RehydrateSegmentCommissionAdoption 只验各格成形，不重判服务方与开始时刻：那是采用当时对着段与委托判过的，行里没有段与
// 委托可以再对一遍，AdoptCommissionForSegment 不能兼职重建。
func RehydrateSegmentCommissionAdoption(spec RehydrateSegmentCommissionAdoptionSpec) (SegmentCommissionAdoption, error) {
	if !spec.TenantID.valid() ||
		!spec.Segment.valid() ||
		!spec.Commission.valid() ||
		!spec.Provider.valid() ||
		!spec.StartBasis.valid() ||
		spec.StartedAt.IsZero() {
		return SegmentCommissionAdoption{}, ErrInvalidCommissionAdoption
	}
	return SegmentCommissionAdoption{
		tenantID:   spec.TenantID,
		segment:    spec.Segment,
		commission: spec.Commission,
		provider:   spec.Provider,
		startBasis: spec.StartBasis,
		startedAt:  spec.StartedAt.UTC(),
	}, nil
}
