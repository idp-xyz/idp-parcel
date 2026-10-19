package domain

import (
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
)

// 送站交接单（NO CONTEXT「送站交接单」；ADR-0212 决定三）：节点在一次送站上当场开立的记录——交付方线索、声称的客户账户、开立
// 时刻，以及经收寄或归入落在它名下的作业实物。对每个声称的客户账户的绑定由客户渠道或运营决定作结论；声称的全部已确认，它才构成
// 可验证交付范围。开立、成员、关闭、结论各是一份只增的事实，范围成立是派生，不另存。

// ErrInvalidDeliveryHandoverNote 说明一份交接单事实立不起来：开立缺格或声称为空、重复，成员缺形成方式，结论的途径与结论、证据对
// 不上。重发同样的内容不会变好。
var ErrInvalidDeliveryHandoverNote = errors.New("node operations: invalid delivery handover note")

// DeliveryHandoverNoteID 是设备签发的交接单标识（ADR-0023）：收寄时作为交付范围依据引用上报，归入、关闭、确认都按它寻址。
type DeliveryHandoverNoteID struct{ requiredValue }

func NewDeliveryHandoverNoteID(value string) (DeliveryHandoverNoteID, error) {
	required, err := newRequiredValue("delivery handover note ID", value)
	return DeliveryHandoverNoteID{required}, err
}

// DeliveryHandoverNoteSpec 是开立一张交接单所需的全部输入（ADR-0212 决定三「开立固定节点、交付方线索、声称的客户账户（至少
// 一个）与开立时刻」）。交付方线索必给：开立就在交付方面前，它与收寄的 deliveredBy 同类，只是线索。
type DeliveryHandoverNoteSpec struct {
	TenantID        TenantID
	ID              DeliveryHandoverNoteID
	Node            NodeReference
	DeliveringParty DeliveringPartyReference
	ClaimedAccounts []CustomerAccountReference
	OpenedAt        time.Time
}

// DeliveryHandoverNote 是一张已开立的交接单。声称按标识定序：同一组声称落成同一个样子，范围成立时交出的客户账户也按这个序。
type DeliveryHandoverNote struct {
	tenantID        TenantID
	id              DeliveryHandoverNoteID
	node            NodeReference
	deliveringParty DeliveringPartyReference
	claimed         []CustomerAccountReference
	openedAt        time.Time
}

// OpenDeliveryHandoverNote 立一张交接单。声称至少一个、逐字不重复。
func OpenDeliveryHandoverNote(spec DeliveryHandoverNoteSpec) (DeliveryHandoverNote, error) {
	if !spec.TenantID.valid() ||
		!spec.ID.valid() ||
		!spec.Node.valid() ||
		!spec.DeliveringParty.valid() ||
		spec.OpenedAt.IsZero() ||
		len(spec.ClaimedAccounts) == 0 {
		return DeliveryHandoverNote{}, ErrInvalidDeliveryHandoverNote
	}
	claimed := slices.Clone(spec.ClaimedAccounts)
	slices.SortFunc(claimed, func(left, right CustomerAccountReference) int { return strings.Compare(left.value, right.value) })
	for index, account := range claimed {
		if !account.valid() || (index > 0 && claimed[index-1] == account) {
			return DeliveryHandoverNote{}, ErrInvalidDeliveryHandoverNote
		}
	}
	return DeliveryHandoverNote{
		tenantID:        spec.TenantID,
		id:              spec.ID,
		node:            spec.Node,
		deliveringParty: spec.DeliveringParty,
		claimed:         claimed,
		openedAt:        registerInstant(spec.OpenedAt),
	}, nil
}

func (note DeliveryHandoverNote) TenantID() TenantID         { return note.tenantID }
func (note DeliveryHandoverNote) ID() DeliveryHandoverNoteID { return note.id }
func (note DeliveryHandoverNote) Node() NodeReference        { return note.node }
func (note DeliveryHandoverNote) DeliveringParty() DeliveringPartyReference {
	return note.deliveringParty
}
func (note DeliveryHandoverNote) OpenedAt() time.Time { return note.openedAt }

// ClaimedAccounts 交回声称的客户账户的副本，按标识定序。
func (note DeliveryHandoverNote) ClaimedAccounts() []CustomerAccountReference {
	return slices.Clone(note.claimed)
}

// Claims 报告这个客户账户在声称之列。客户渠道只能对声称了自己的交接单作结论（ADR-0212 决定三）。
func (note DeliveryHandoverNote) Claims(account CustomerAccountReference) bool {
	return slices.Contains(note.claimed, account)
}

func (note DeliveryHandoverNote) valid() bool {
	return note.tenantID.valid() && note.id.valid()
}

// HandoverMembershipKind 是一件作业实物成为交接单成员的方式（ADR-0212 决定三「成员」）。
type HandoverMembershipKind uint8

const (
	HandoverMembershipKindInvalid HandoverMembershipKind = iota
	// MembershipByReception：收寄来源带着这张交接单的标识。
	MembershipByReception
	// MembershipByAssignment：开放期间把同一节点上待识别的作业实物归入。一件实物的依据取它最后归入的交接单。
	MembershipByAssignment
)

func (kind HandoverMembershipKind) String() string {
	switch kind {
	case MembershipByReception:
		return "RECEPTION"
	case MembershipByAssignment:
		return "ASSIGNMENT"
	default:
		return ""
	}
}

// ParseHandoverMembershipKind 按线上名认形成方式，大小写不折叠。
func ParseHandoverMembershipKind(raw string) (HandoverMembershipKind, error) {
	for _, kind := range []HandoverMembershipKind{MembershipByReception, MembershipByAssignment} {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return HandoverMembershipKindInvalid, fmt.Errorf("%w: membership kind %q", ErrInvalidDeliveryHandoverNote, raw)
}

// DeliveryHandoverMemberSpec 是一件作业实物落在交接单名下的事实。FactID 是这份事实的来源身份，由设备签发（ADR-0023）：收寄带来的
// 成员取那次收寄的来源身份，归入取归入事实自己的身份，重放按它认。FormedAt 是业务时间。
type DeliveryHandoverMemberSpec struct {
	TenantID TenantID
	Note     DeliveryHandoverNoteID
	Unit     HandlingUnitID
	Kind     HandoverMembershipKind
	FactID   string
	FormedAt time.Time
}

// DeliveryHandoverMember 是交接单的一个成员。成员只增：归入是追加的事实，不改写来源，也不从别的交接单里把实物「移走」——一件实物
// 可以先后落在几张交接单名下，依据取最后归入的那一张。
type DeliveryHandoverMember struct {
	tenantID TenantID
	note     DeliveryHandoverNoteID
	unit     HandlingUnitID
	kind     HandoverMembershipKind
	factID   string
	formedAt time.Time
}

func FormDeliveryHandoverMember(spec DeliveryHandoverMemberSpec) (DeliveryHandoverMember, error) {
	if !spec.TenantID.valid() ||
		!spec.Note.valid() ||
		!spec.Unit.valid() ||
		spec.Kind.String() == "" ||
		strings.TrimSpace(spec.FactID) == "" ||
		spec.FormedAt.IsZero() {
		return DeliveryHandoverMember{}, ErrInvalidDeliveryHandoverNote
	}
	return DeliveryHandoverMember{
		tenantID: spec.TenantID,
		note:     spec.Note,
		unit:     spec.Unit,
		kind:     spec.Kind,
		factID:   spec.FactID,
		formedAt: registerInstant(spec.FormedAt),
	}, nil
}

func (member DeliveryHandoverMember) TenantID() TenantID           { return member.tenantID }
func (member DeliveryHandoverMember) Note() DeliveryHandoverNoteID { return member.note }
func (member DeliveryHandoverMember) Unit() HandlingUnitID         { return member.unit }
func (member DeliveryHandoverMember) Kind() HandoverMembershipKind { return member.kind }
func (member DeliveryHandoverMember) FactID() string               { return member.factID }
func (member DeliveryHandoverMember) FormedAt() time.Time          { return member.formedAt }

// DeliveryHandoverClosureSpec 是关闭一张交接单的事实。FactID 由设备签发，重放按它认。
type DeliveryHandoverClosureSpec struct {
	TenantID TenantID
	Note     DeliveryHandoverNoteID
	FactID   string
	ClosedAt time.Time
}

// DeliveryHandoverClosure 是交接单的关闭。关闭冻结成员（NO CONTEXT 生命周期「送站交接单」）；一张交接单至多关闭一次。
type DeliveryHandoverClosure struct {
	tenantID TenantID
	note     DeliveryHandoverNoteID
	factID   string
	closedAt time.Time
}

func FormDeliveryHandoverClosure(spec DeliveryHandoverClosureSpec) (DeliveryHandoverClosure, error) {
	if !spec.TenantID.valid() || !spec.Note.valid() || strings.TrimSpace(spec.FactID) == "" || spec.ClosedAt.IsZero() {
		return DeliveryHandoverClosure{}, ErrInvalidDeliveryHandoverNote
	}
	return DeliveryHandoverClosure{
		tenantID: spec.TenantID,
		note:     spec.Note,
		factID:   spec.FactID,
		closedAt: registerInstant(spec.ClosedAt),
	}, nil
}

func (closure DeliveryHandoverClosure) TenantID() TenantID           { return closure.tenantID }
func (closure DeliveryHandoverClosure) Note() DeliveryHandoverNoteID { return closure.note }
func (closure DeliveryHandoverClosure) FactID() string               { return closure.factID }
func (closure DeliveryHandoverClosure) ClosedAt() time.Time          { return closure.closedAt }

func (closure DeliveryHandoverClosure) valid() bool {
	return closure.tenantID.valid() && closure.note.valid()
}

// HandoverBindingConclusion 是交接单对一个声称的客户账户的绑定结论。两种都是终态（ADR-0212 决定三）。
type HandoverBindingConclusion uint8

const (
	HandoverBindingConclusionInvalid HandoverBindingConclusion = iota
	BindingConfirmed
	BindingDenied
)

func (conclusion HandoverBindingConclusion) String() string {
	switch conclusion {
	case BindingConfirmed:
		return "CONFIRMED"
	case BindingDenied:
		return "DENIED"
	default:
		return ""
	}
}

// ParseHandoverBindingConclusion 按线上名认结论，大小写不折叠。
func ParseHandoverBindingConclusion(raw string) (HandoverBindingConclusion, error) {
	for _, conclusion := range []HandoverBindingConclusion{BindingConfirmed, BindingDenied} {
		if conclusion.String() == raw {
			return conclusion, nil
		}
	}
	return HandoverBindingConclusionInvalid, fmt.Errorf("%w: binding conclusion %q", ErrInvalidDeliveryHandoverNote, raw)
}

// HandoverBindingRoute 是结论经哪一路作出（ADR-0212 决定三「绑定确认有两路」）。
type HandoverBindingRoute uint8

const (
	HandoverBindingRouteInvalid HandoverBindingRoute = iota
	// ConcludedViaCustomerChannel：客户账户经首方客户渠道确认或否认。
	ConcludedViaCustomerChannel
	// ConcludedViaOperationDecision：持「送站交接确认」授予的人依授权交付关系确认。只能确认——否认是客户账户自己的话。
	ConcludedViaOperationDecision
)

func (route HandoverBindingRoute) String() string {
	switch route {
	case ConcludedViaCustomerChannel:
		return "CUSTOMER_CHANNEL"
	case ConcludedViaOperationDecision:
		return "OPERATION_DECISION"
	default:
		return ""
	}
}

// ParseHandoverBindingRoute 按线上名认途径，大小写不折叠。
func ParseHandoverBindingRoute(raw string) (HandoverBindingRoute, error) {
	for _, route := range []HandoverBindingRoute{ConcludedViaCustomerChannel, ConcludedViaOperationDecision} {
		if route.String() == raw {
			return route, nil
		}
	}
	return HandoverBindingRouteInvalid, fmt.Errorf("%w: binding route %q", ErrInvalidDeliveryHandoverNote, raw)
}

// DeliveryHandoverBindingSpec 是一份绑定结论的输入。Concluder 是作结论的一方：客户渠道写客户端的来源格，运营决定写提交操作者
// （与 ADR-0187 操作证据同一写法）。DeliveringParticipant 是 party-commercial 的参与方标识、Basis 是依据引用，两格只在运营决定时
// 在场，逐字保存、不判真伪。
type DeliveryHandoverBindingSpec struct {
	TenantID              TenantID
	Note                  DeliveryHandoverNoteID
	Account               CustomerAccountReference
	Conclusion            HandoverBindingConclusion
	Route                 HandoverBindingRoute
	Concluder             string
	DeliveringParticipant string
	Basis                 string
	ConcludedAt           time.Time
}

// DeliveryHandoverBinding 是交接单对一个客户账户的绑定结论。（租户，交接单，客户账户）至多一个结论，确认错了不回滚已形成的关联
// （ADR-0212 决定三，同 ADR-0196 决定五）。
type DeliveryHandoverBinding struct {
	tenantID              TenantID
	note                  DeliveryHandoverNoteID
	account               CustomerAccountReference
	conclusion            HandoverBindingConclusion
	route                 HandoverBindingRoute
	concluder             string
	deliveringParticipant string
	basis                 string
	concludedAt           time.Time
}

// FormDeliveryHandoverBinding 立一份结论并验途径与结论、证据的搭配：运营决定只能确认，且必带交付方参与方与依据引用；客户渠道两种
// 结论都可以，不带那两格。账户在不在声称之列不在这里判——那要读交接单，由作结论的编排与册各守一道。
func FormDeliveryHandoverBinding(spec DeliveryHandoverBindingSpec) (DeliveryHandoverBinding, error) {
	participant := strings.TrimSpace(spec.DeliveringParticipant)
	basis := strings.TrimSpace(spec.Basis)
	concluder := strings.TrimSpace(spec.Concluder)
	if !spec.TenantID.valid() ||
		!spec.Note.valid() ||
		!spec.Account.valid() ||
		spec.Conclusion.String() == "" ||
		concluder == "" ||
		spec.ConcludedAt.IsZero() {
		return DeliveryHandoverBinding{}, ErrInvalidDeliveryHandoverNote
	}
	switch spec.Route {
	case ConcludedViaOperationDecision:
		if spec.Conclusion != BindingConfirmed || participant == "" || basis == "" {
			return DeliveryHandoverBinding{}, ErrInvalidDeliveryHandoverNote
		}
	case ConcludedViaCustomerChannel:
		if participant != "" || basis != "" {
			return DeliveryHandoverBinding{}, ErrInvalidDeliveryHandoverNote
		}
	default:
		return DeliveryHandoverBinding{}, ErrInvalidDeliveryHandoverNote
	}
	return DeliveryHandoverBinding{
		tenantID:              spec.TenantID,
		note:                  spec.Note,
		account:               spec.Account,
		conclusion:            spec.Conclusion,
		route:                 spec.Route,
		concluder:             concluder,
		deliveringParticipant: participant,
		basis:                 basis,
		concludedAt:           registerInstant(spec.ConcludedAt),
	}, nil
}

func (binding DeliveryHandoverBinding) TenantID() TenantID                { return binding.tenantID }
func (binding DeliveryHandoverBinding) Note() DeliveryHandoverNoteID      { return binding.note }
func (binding DeliveryHandoverBinding) Account() CustomerAccountReference { return binding.account }
func (binding DeliveryHandoverBinding) Conclusion() HandoverBindingConclusion {
	return binding.conclusion
}
func (binding DeliveryHandoverBinding) Route() HandoverBindingRoute { return binding.route }
func (binding DeliveryHandoverBinding) Concluder() string           { return binding.concluder }
func (binding DeliveryHandoverBinding) ConcludedAt() time.Time      { return binding.concludedAt }

// OperationDecisionEvidence 交回运营决定确认时记下的交付方参与方与依据引用；客户渠道的结论两格都是空。
func (binding DeliveryHandoverBinding) OperationDecisionEvidence() (deliveringParticipant, basis string) {
	return binding.deliveringParticipant, binding.basis
}

// DeliveryHandoverNoteState 是一张交接单此刻的证据状态：交接单本身、它的关闭（零值即开放）与各客户账户的结论。零值即「不在册」。
type DeliveryHandoverNoteState struct {
	Note     DeliveryHandoverNote
	Closure  DeliveryHandoverClosure
	Bindings []DeliveryHandoverBinding
}

// Registered 报告这张交接单在册。
func (state DeliveryHandoverNoteState) Registered() bool {
	return state.Note.valid()
}

// Closed 报告这张交接单已关闭。
func (state DeliveryHandoverNoteState) Closed() bool {
	return state.Closure.valid()
}

// BindingFor 交回这个客户账户的结论及其是否在场。
func (state DeliveryHandoverNoteState) BindingFor(account CustomerAccountReference) (DeliveryHandoverBinding, bool) {
	for _, binding := range state.Bindings {
		if binding.account == account && binding.note == state.Note.id {
			return binding, true
		}
	}
	return DeliveryHandoverBinding{}, false
}

// ScopeEstablished 报告这张交接单此刻成立范围：声称的客户账户全部已确认（ADR-0212 决定三「成立」）。不认部分确认——声称了 A、B
// 而只确认 A 时以｛A｝作范围，会把 B 的件里凡在 A 编号空间同号的都消歧到 A 那一件；任一否认，它就不再成立。不在声称之列的结论不
// 算数。
func (state DeliveryHandoverNoteState) ScopeEstablished() bool {
	if !state.Registered() {
		return false
	}
	for _, account := range state.Note.claimed {
		binding, present := state.BindingFor(account)
		if !present || binding.conclusion != BindingConfirmed {
			return false
		}
	}
	return true
}
