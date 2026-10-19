package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidTransportCommission = errors.New("transport fulfillment: invalid transport commission")
	ErrInvalidBookingRequest      = errors.New("transport fulfillment: invalid booking request")
	ErrInvalidCarrierAcceptance   = errors.New("transport fulfillment: invalid carrier acceptance")
	// ErrTransportAlreadyStarted：实际运输开始前可以取消或释放运输意图；开始后只能依据
	// 事实形成中断、改降、折返或其他实际结果（CONTEXT 班次节）。已成立履约段的不可取消
	// 由 ActualFulfillmentSegment 钉后半，这里钉前半。
	ErrTransportAlreadyStarted = errors.New("transport fulfillment: actual transport has already started")
)

// TransportCommissionReference 指名一份运输委托。
type TransportCommissionReference struct{ requiredValue }

func NewTransportCommissionReference(value string) (TransportCommissionReference, error) {
	required, err := newRequiredValue("transport commission reference", value)
	return TransportCommissionReference{required}, err
}

// ConditionsSnapshotReference 指名实际采用的履约条件快照。
type ConditionsSnapshotReference struct{ requiredValue }

func NewConditionsSnapshotReference(value string) (ConditionsSnapshotReference, error) {
	required, err := newRequiredValue("conditions snapshot reference", value)
	return ConditionsSnapshotReference{required}, err
}

// RoleSnapshotReference 指名实际采用的角色快照（谁以什么角色承担本次运输）。
type RoleSnapshotReference struct{ requiredValue }

func NewRoleSnapshotReference(value string) (RoleSnapshotReference, error) {
	required, err := newRequiredValue("role snapshot reference", value)
	return RoleSnapshotReference{required}, err
}

// ResponsibilitySnapshotReference 指名实际采用的责任依据快照。
type ResponsibilitySnapshotReference struct{ requiredValue }

func NewResponsibilitySnapshotReference(value string) (ResponsibilitySnapshotReference, error) {
	required, err := newRequiredValue("responsibility snapshot reference", value)
	return ResponsibilitySnapshotReference{required}, err
}

// TransportCommission 是向外部运输服务提供方提出的明确运输范围与条件请求（CONTEXT
// 「运输委托」：引用供应商商业协议和履约条件快照，但不等于订舱、承运接受、实际履约段
// 或供应商账单）。协议、条件、角色与责任依据都是**快照引用**——版本生命周期属
// party-commercial，本类型持有的只是引用，结构上改不了商业版本（CONTEXT「Boundaries and ownership」：「不修改商业版本」）。
type TransportCommission struct {
	tenantID       TenantID
	commission     TransportCommissionReference
	provider       ServiceProviderReference
	agreement      AgreementSnapshotReference
	legalEntity    ProcurementLegalEntityReference
	conditions     ConditionsSnapshotReference
	role           RoleSnapshotReference
	responsibility ResponsibilitySnapshotReference
	members        []CarriedObjectReference
	submittedAt    time.Time
	startedAt      time.Time
	startedBasis   ParticipationBasisReference
	cancelledAt    time.Time
	// agreementVersion 是登记方指名、提交时核过的那一版协议（对象与版本标签）。快照 agreement 是 PC 一处拼出的指称，本上下文
	// 不拆它（SupplierAgreementVersion 头注）；要按这份委托回去问 PC 那一版声明的收费发生条件，就得留下原来那两格。
	agreementVersion SupplierAgreementVersion
}

// declaredCommissionValid 是登记方声明那一半的构造门，只在草稿处判一次：协议快照与法人之外，提交不比草稿多判任何一格，
// 否则一份过了草稿门的声明会在核过协议之后才被拒，而那时已经白问了一次 party-commercial。
func declaredCommissionValid(
	tenant TenantID,
	commission TransportCommissionReference,
	provider ServiceProviderReference,
	conditions ConditionsSnapshotReference,
	role RoleSnapshotReference,
	responsibility ResponsibilitySnapshotReference,
	members []CarriedObjectReference,
	submittedAt time.Time,
) bool {
	if !tenant.valid() ||
		!commission.valid() ||
		!provider.valid() ||
		!conditions.valid() ||
		!role.valid() ||
		!responsibility.valid() ||
		len(members) == 0 ||
		AtRegistryPrecision(submittedAt).IsZero() {
		return false
	}
	seen := make(map[CarriedObjectReference]struct{}, len(members))
	for _, member := range members {
		if !member.valid() {
			return false
		}
		if _, exists := seen[member]; exists {
			return false
		}
		seen[member] = struct{}{}
	}
	return true
}

// TransportCommissionDraftSpec 是登记方声明的一份委托：指名一版供应商协议（对象与版本标签），其余各格与提交时相同。
type TransportCommissionDraftSpec struct {
	TenantID       TenantID
	Commission     TransportCommissionReference
	Provider       ServiceProviderReference
	Agreement      SupplierAgreementVersion
	Conditions     ConditionsSnapshotReference
	Role           RoleSnapshotReference
	Responsibility ResponsibilitySnapshotReference
	Members        []CarriedObjectReference
	SubmittedAt    time.Time
}

// TransportCommissionDraft 是运输委托草稿（CONTEXT 生命周期「运输委托草稿 → 已提交」）：登记方声明的各格都已过构造门，
// 但指名的那一版供应商协议还没核过，也就还没有协议快照与采购责任法人。草稿不落库——委托只在提交时成立。
//
// 草稿是成立一份委托的唯一入口（读回走 RehydrateTransportCommission，那是另一道门）：把「先判声明、再问协议、最后固定
// 快照」这个次序钉在类型上——声明不合法的请求不该花一次跨上下文读，而快照两格只能由 Submit 从核对结果里来，没有一条
// 不经核对就能填上它们的构造路径。
type TransportCommissionDraft struct {
	tenantID       TenantID
	commission     TransportCommissionReference
	provider       ServiceProviderReference
	agreement      SupplierAgreementVersion
	conditions     ConditionsSnapshotReference
	role           RoleSnapshotReference
	responsibility ResponsibilitySnapshotReference
	members        []CarriedObjectReference
	submittedAt    time.Time
}

func DraftTransportCommission(spec TransportCommissionDraftSpec) (TransportCommissionDraft, error) {
	if !spec.Agreement.valid() || !declaredCommissionValid(
		spec.TenantID, spec.Commission, spec.Provider, spec.Conditions, spec.Role, spec.Responsibility, spec.Members, spec.SubmittedAt,
	) {
		return TransportCommissionDraft{}, ErrInvalidTransportCommission
	}
	return TransportCommissionDraft{
		tenantID:       spec.TenantID,
		commission:     spec.Commission,
		provider:       spec.Provider,
		agreement:      spec.Agreement,
		conditions:     spec.Conditions,
		role:           spec.Role,
		responsibility: spec.Responsibility,
		members:        append([]CarriedObjectReference(nil), spec.Members...),
		submittedAt:    spec.SubmittedAt.UTC(),
	}, nil
}

func (draft TransportCommissionDraft) TenantID() TenantID {
	return draft.tenantID
}

func (draft TransportCommissionDraft) Commission() TransportCommissionReference {
	return draft.commission
}

func (draft TransportCommissionDraft) Provider() ServiceProviderReference {
	return draft.provider
}

// Agreement 是登记方指名、待核的那一版供应商协议。
func (draft TransportCommissionDraft) Agreement() SupplierAgreementVersion {
	return draft.agreement
}

// SubmittedAt 是登记方给的提交时刻，也是核协议「在提交时点支持采购」所用的那一刻。
func (draft TransportCommissionDraft) SubmittedAt() time.Time {
	return draft.submittedAt
}

// Submit 以核过的协议快照与采购责任法人把草稿提交成委托。两格都取自核对结果，不由登记方给（ADR-0202 决定三）；草稿本身
// 不变，同一份草稿可以再交一次。
func (draft TransportCommissionDraft) Submit(
	agreement AgreementSnapshotReference,
	legalEntity ProcurementLegalEntityReference,
) (TransportCommission, error) {
	if !agreement.valid() || !legalEntity.valid() || !draft.commission.valid() {
		return TransportCommission{}, ErrInvalidTransportCommission
	}
	return TransportCommission{
		tenantID:         draft.tenantID,
		commission:       draft.commission,
		provider:         draft.provider,
		agreement:        agreement,
		legalEntity:      legalEntity,
		conditions:       draft.conditions,
		role:             draft.role,
		responsibility:   draft.responsibility,
		members:          append([]CarriedObjectReference(nil), draft.members...),
		submittedAt:      draft.submittedAt,
		agreementVersion: draft.agreement,
	}, nil
}

func (commission TransportCommission) TenantID() TenantID {
	return commission.tenantID
}

func (commission TransportCommission) Commission() TransportCommissionReference {
	return commission.commission
}

func (commission TransportCommission) Provider() ServiceProviderReference {
	return commission.provider
}

func (commission TransportCommission) Agreement() AgreementSnapshotReference {
	return commission.agreement
}

// AgreementVersion 交回提交时登记方指名、核过的那一版协议。运输收费发生项从委托取采购上下文时按它回问 PC 那一版声明的
// 收费发生条件（ADR-0207 文末补记）。迁移 TF 0038 之前提交的委托没有记下它，答 false：那时只存了快照，快照不拆。
func (commission TransportCommission) AgreementVersion() (SupplierAgreementVersion, bool) {
	return commission.agreementVersion, commission.agreementVersion.valid()
}

// LegalEntity 是承担这次采购责任的运营法人，提交时从核过的那一版供应商协议快照下来，不由登记方给（ADR-0202 决定三）。
func (commission TransportCommission) LegalEntity() ProcurementLegalEntityReference {
	return commission.legalEntity
}

func (commission TransportCommission) Conditions() ConditionsSnapshotReference {
	return commission.conditions
}

func (commission TransportCommission) Role() RoleSnapshotReference {
	return commission.role
}

func (commission TransportCommission) Responsibility() ResponsibilitySnapshotReference {
	return commission.responsibility
}

func (commission TransportCommission) Members() []CarriedObjectReference {
	return append([]CarriedObjectReference(nil), commission.members...)
}

func (commission TransportCommission) SubmittedAt() time.Time {
	return commission.submittedAt
}

// TransportStarted 报告实际运输是否已依据控制事实开始。
func (commission TransportCommission) TransportStarted() (ParticipationBasisReference, time.Time, bool) {
	if commission.startedAt.IsZero() {
		return ParticipationBasisReference{}, time.Time{}, false
	}
	return commission.startedBasis, commission.startedAt, true
}

func (commission TransportCommission) Cancelled() (time.Time, bool) {
	if commission.cancelledAt.IsZero() {
		return time.Time{}, false
	}
	return commission.cancelledAt, true
}

// MarkTransportStarted 以首个控制事实（有效收寄或权威交接）登记实际运输已开始。开始
// 只发生一次；已取消的委托没有可开始的意图。
func (commission TransportCommission) MarkTransportStarted(
	basis ParticipationBasisReference,
	at time.Time,
) (TransportCommission, error) {
	if !basis.valid() || AtRegistryPrecision(at).IsZero() || at.Before(commission.submittedAt) {
		return TransportCommission{}, ErrInvalidTransportCommission
	}
	if _, cancelled := commission.Cancelled(); cancelled {
		return TransportCommission{}, ErrInvalidTransportCommission
	}
	if _, _, started := commission.TransportStarted(); started {
		return TransportCommission{}, ErrTransportAlreadyStarted
	}
	marked := commission
	marked.members = append([]CarriedObjectReference(nil), commission.members...)
	marked.startedBasis = basis
	marked.startedAt = at.UTC()
	return marked, nil
}

// Cancel 取消尚未开始的运输意图。已经形成的承运接受、容量预占、费用责任和履约事实
// 继续按各自规则保留和处置（CONTEXT 195）——本方法不触碰任何那些对象，它们也不在
// 本类型上；实际运输已开始后不可取消，只能按事实形成中断或其他实际结果。
func (commission TransportCommission) Cancel(at time.Time) (TransportCommission, error) {
	if AtRegistryPrecision(at).IsZero() || at.Before(commission.submittedAt) {
		return TransportCommission{}, ErrInvalidTransportCommission
	}
	if _, _, started := commission.TransportStarted(); started {
		return TransportCommission{}, ErrTransportAlreadyStarted
	}
	if _, cancelled := commission.Cancelled(); cancelled {
		return TransportCommission{}, ErrInvalidTransportCommission
	}
	cancelled := commission
	cancelled.members = append([]CarriedObjectReference(nil), commission.members...)
	cancelled.cancelledAt = at.UTC()
	return cancelled, nil
}

// BookingReference 指名一次订舱申请。
type BookingReference struct{ requiredValue }

func NewBookingReference(value string) (BookingReference, error) {
	required, err := newRequiredValue("booking reference", value)
	return BookingReference{required}, err
}

// BookingRequest 是针对具体班次或外部运输服务的舱位/容量申请（CONTEXT「订舱」）。
// 订舱申请本身不占用容量、不等于承运接受——它与委托、接受、预占分别拥有身份与生命
// 周期（CONTEXT 119/122）。
//
// 订舱必须指名一个具体班次（ADR-0202 决定五）：周期安排不是班次，不能被订舱直接引用（CONTEXT「周期安排」），而不指名
// 班次的订舱说不清申请的是哪一班的舱位。班次在不在册由编排在提交时核，这里只钉「有这一格」。
type BookingRequest struct {
	tenantID    TenantID
	booking     BookingReference
	commission  TransportCommissionReference
	schedule    ScheduleReference
	quantity    int64
	unit        QuantityUnitReference
	requestedAt time.Time
	cancelledAt time.Time
}

// BookingRequestSpec 是提出一次订舱申请所需的全部输入。
type BookingRequestSpec struct {
	TenantID    TenantID
	Booking     BookingReference
	Commission  TransportCommissionReference
	Schedule    ScheduleReference
	Quantity    int64
	Unit        QuantityUnitReference
	RequestedAt time.Time
}

func SubmitBookingRequest(spec BookingRequestSpec) (BookingRequest, error) {
	if !spec.TenantID.valid() ||
		!spec.Booking.valid() ||
		!spec.Commission.valid() ||
		!spec.Schedule.valid() ||
		spec.Quantity <= 0 ||
		!spec.Unit.valid() ||
		AtRegistryPrecision(spec.RequestedAt).IsZero() {
		return BookingRequest{}, ErrInvalidBookingRequest
	}
	return BookingRequest{
		tenantID:    spec.TenantID,
		booking:     spec.Booking,
		commission:  spec.Commission,
		schedule:    spec.Schedule,
		quantity:    spec.Quantity,
		unit:        spec.Unit,
		requestedAt: spec.RequestedAt.UTC(),
	}, nil
}

func (booking BookingRequest) TenantID() TenantID {
	return booking.tenantID
}

func (booking BookingRequest) Booking() BookingReference {
	return booking.booking
}

func (booking BookingRequest) Commission() TransportCommissionReference {
	return booking.commission
}

// Schedule 是这次订舱申请舱位的那一个具体班次。
func (booking BookingRequest) Schedule() ScheduleReference {
	return booking.schedule
}

func (booking BookingRequest) Quantity() (int64, QuantityUnitReference) {
	return booking.quantity, booking.unit
}

func (booking BookingRequest) RequestedAt() time.Time {
	return booking.requestedAt
}

func (booking BookingRequest) Cancelled() (time.Time, bool) {
	if booking.cancelledAt.IsZero() {
		return time.Time{}, false
	}
	return booking.cancelledAt, true
}

// Cancel 取消尚未消耗的订舱范围。已经形成的承运接受继续按自己的规则保留——接受是
// 独立对象，本方法碰不到它。
func (booking BookingRequest) Cancel(at time.Time) (BookingRequest, error) {
	if AtRegistryPrecision(at).IsZero() || at.Before(booking.requestedAt) {
		return BookingRequest{}, ErrInvalidBookingRequest
	}
	if _, cancelled := booking.Cancelled(); cancelled {
		return BookingRequest{}, ErrInvalidBookingRequest
	}
	cancelled := booking
	cancelled.cancelledAt = at.UTC()
	return cancelled, nil
}

// CarrierAcceptanceReference 指名一次承运接受结果。
type CarrierAcceptanceReference struct{ requiredValue }

func NewCarrierAcceptanceReference(value string) (CarrierAcceptanceReference, error) {
	required, err := newRequiredValue("carrier acceptance reference", value)
	return CarrierAcceptanceReference{required}, err
}

// AcceptanceBasisReference 指名拒绝、失效、撤回或候补的原因来源。
type AcceptanceBasisReference struct{ requiredValue }

func NewAcceptanceBasisReference(value string) (AcceptanceBasisReference, error) {
	required, err := newRequiredValue("acceptance basis reference", value)
	return AcceptanceBasisReference{required}, err
}

// CarrierAcceptanceOutcome 是承运方对订舱作答的封闭五值：接受成约；拒绝、失效与撤回
// 分格（CONTEXT 生命周期「订舱申请 → 待确认、候补、部分确认、已确认、拒绝或失效：每次结果保留数量和来源，后续替代不覆盖
// 原申请」）。
//
// 候补（ADR-0202 决定五）是承运方答「此刻排进候补、尚未确认」：不成约、没有接受量，带候补的依据。它是五值里唯一不定局的
// 一值——同一订舱在候补之后还可以有一个定局答复（见 AdmitsSuccessor），其余四值一经答出就是这次订舱的结局。`待确认`
// 与`待补充`不在这里：两者是读法，不是承运方答出的值，口径以 ADR-0202 决定五为准。
type CarrierAcceptanceOutcome uint8

const (
	CarrierAcceptanceOutcomeInvalid CarrierAcceptanceOutcome = iota
	BookingAccepted
	BookingRefused
	BookingExpired
	BookingWithdrawn
	BookingWaitlisted
)

func (outcome CarrierAcceptanceOutcome) valid() bool {
	return outcome >= BookingAccepted && outcome <= BookingWaitlisted
}

func (outcome CarrierAcceptanceOutcome) String() string {
	switch outcome {
	case BookingAccepted:
		return "ACCEPTED"
	case BookingRefused:
		return "REFUSED"
	case BookingExpired:
		return "EXPIRED"
	case BookingWithdrawn:
		return "WITHDRAWN"
	case BookingWaitlisted:
		return "WAITLISTED"
	default:
		return ""
	}
}

// CarrierAcceptance 是运输服务提供方针对订舱作出的结果（CONTEXT「承运接受」）。只有
// 接受成约；接受也**不等于**容量已预占、对象已分配或实际承运商已收寄——面单生成、
// 渠道受理、预报成功、订舱接受、舱单建立、电子数据接收和车辆到场均不构成实际承运商
// 首次有效收寄（CONTEXT 105），所以本类型上没有任何收寄或控制字段可以冒充那道边界。
type CarrierAcceptance struct {
	tenantID   TenantID
	acceptance CarrierAcceptanceReference
	booking    BookingReference
	outcome    CarrierAcceptanceOutcome
	quantity   int64
	basis      AcceptanceBasisReference
	decidedAt  time.Time
}

// FormCarrierAcceptance 依附订舱申请形成承运结果。接受必须带不超过申请量的接受量
// （部分接受合法，CONTEXT「承运接受可以部分或附条件成立」）；拒绝/失效/撤回/候补必须带原因来源且没有接受量。
func FormCarrierAcceptance(
	booking BookingRequest,
	acceptance CarrierAcceptanceReference,
	outcome CarrierAcceptanceOutcome,
	quantity int64,
	basis AcceptanceBasisReference,
	decidedAt time.Time,
) (CarrierAcceptance, error) {
	if !booking.booking.valid() || !acceptance.valid() || !outcome.valid() || AtRegistryPrecision(decidedAt).IsZero() {
		return CarrierAcceptance{}, ErrInvalidCarrierAcceptance
	}
	if decidedAt.Before(booking.requestedAt) {
		return CarrierAcceptance{}, ErrInvalidCarrierAcceptance
	}
	if outcome == BookingAccepted {
		if quantity <= 0 || quantity > booking.quantity {
			return CarrierAcceptance{}, ErrInvalidCarrierAcceptance
		}
		if basis.valid() {
			// 接受的依据是协议与申请本身；这里塞「原因」只会与拒绝家族混格。
			return CarrierAcceptance{}, ErrInvalidCarrierAcceptance
		}
	} else {
		if quantity != 0 {
			return CarrierAcceptance{}, ErrInvalidCarrierAcceptance
		}
		if !basis.valid() {
			return CarrierAcceptance{}, ErrInvalidCarrierAcceptance
		}
	}
	return CarrierAcceptance{
		tenantID:   booking.tenantID,
		acceptance: acceptance,
		booking:    booking.booking,
		outcome:    outcome,
		quantity:   quantity,
		basis:      basis,
		decidedAt:  decidedAt.UTC(),
	}, nil
}

func (acceptance CarrierAcceptance) TenantID() TenantID {
	return acceptance.tenantID
}

func (acceptance CarrierAcceptance) Acceptance() CarrierAcceptanceReference {
	return acceptance.acceptance
}

func (acceptance CarrierAcceptance) Booking() BookingReference {
	return acceptance.booking
}

func (acceptance CarrierAcceptance) Outcome() CarrierAcceptanceOutcome {
	return acceptance.outcome
}

// AcceptedQuantity 只在接受时非零——部分接受保留数量。
func (acceptance CarrierAcceptance) AcceptedQuantity() int64 {
	return acceptance.quantity
}

// Basis 在拒绝、失效、撤回或候补时交回原因来源；接受没有它。
func (acceptance CarrierAcceptance) Basis() (AcceptanceBasisReference, bool) {
	if !acceptance.basis.valid() {
		return AcceptanceBasisReference{}, false
	}
	return acceptance.basis, true
}

func (acceptance CarrierAcceptance) DecidedAt() time.Time {
	return acceptance.decidedAt
}

// Binds 只在接受时为真：订舱不是接受，接受才成约（CONTEXT「承运接受」）。
func (acceptance CarrierAcceptance) Binds() bool {
	return acceptance.outcome == BookingAccepted
}

// Settles 答这个答复是不是这次订舱的结局：候补不是，其余四值都是。
func (acceptance CarrierAcceptance) Settles() bool {
	return acceptance.outcome != BookingWaitlisted
}

// AdmitsSuccessor 答 next 能不能接在本答复之后，成为同一订舱的下一份答复：只有候补可以被后继；后继必须是同一租户同一
// 订舱上另一份答复，是定局（候补之后再候补，只是把同一件事又说一遍，改的应是原答复），且不早于候补。
//
// 后继不覆盖候补：两份答复各自保留，候补那一份仍是那段时间里承运方确实说过的话（CONTEXT「后续替代不覆盖原申请」）。定局
// 之后不再有后继——已接受的订舱不能再被拒绝，改约走撤回与新订舱。
func (acceptance CarrierAcceptance) AdmitsSuccessor(next CarrierAcceptance) bool {
	return acceptance.outcome == BookingWaitlisted &&
		next.Settles() &&
		next.outcome.valid() &&
		next.tenantID == acceptance.tenantID &&
		next.booking == acceptance.booking &&
		next.acceptance != acceptance.acceptance &&
		!next.decidedAt.Before(acceptance.decidedAt)
}
