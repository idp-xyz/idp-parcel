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

// TransportCommissionSpec 是提出一份运输委托所需的全部输入。
type TransportCommissionSpec struct {
	TenantID       TenantID
	Commission     TransportCommissionReference
	Provider       ServiceProviderReference
	Agreement      AgreementSnapshotReference
	Conditions     ConditionsSnapshotReference
	Role           RoleSnapshotReference
	Responsibility ResponsibilitySnapshotReference
	Members        []CarriedObjectReference
	SubmittedAt    time.Time
}

// TransportCommission 是向外部运输服务提供方提出的明确运输范围与条件请求（CONTEXT
// 「运输委托」：引用供应商商业协议和履约条件快照，但不等于订舱、承运接受、实际履约段
// 或供应商账单）。协议、条件、角色与责任依据都是**快照引用**——版本生命周期属
// party-commercial，本类型持有的只是引用，结构上改不了商业版本（CONTEXT 243）。
type TransportCommission struct {
	tenantID       TenantID
	commission     TransportCommissionReference
	provider       ServiceProviderReference
	agreement      AgreementSnapshotReference
	conditions     ConditionsSnapshotReference
	role           RoleSnapshotReference
	responsibility ResponsibilitySnapshotReference
	members        []CarriedObjectReference
	submittedAt    time.Time
	startedAt      time.Time
	startedBasis   ParticipationBasisReference
	cancelledAt    time.Time
}

func SubmitTransportCommission(spec TransportCommissionSpec) (TransportCommission, error) {
	if !spec.TenantID.valid() ||
		!spec.Commission.valid() ||
		!spec.Provider.valid() ||
		!spec.Agreement.valid() ||
		!spec.Conditions.valid() ||
		!spec.Role.valid() ||
		!spec.Responsibility.valid() ||
		len(spec.Members) == 0 ||
		spec.SubmittedAt.IsZero() {
		return TransportCommission{}, ErrInvalidTransportCommission
	}
	seen := make(map[CarriedObjectReference]struct{}, len(spec.Members))
	for _, member := range spec.Members {
		if !member.valid() {
			return TransportCommission{}, ErrInvalidTransportCommission
		}
		if _, exists := seen[member]; exists {
			return TransportCommission{}, ErrInvalidTransportCommission
		}
		seen[member] = struct{}{}
	}
	return TransportCommission{
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
	if !basis.valid() || at.IsZero() || at.Before(commission.submittedAt) {
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
	if at.IsZero() || at.Before(commission.submittedAt) {
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
type BookingRequest struct {
	tenantID    TenantID
	booking     BookingReference
	commission  TransportCommissionReference
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
	Quantity    int64
	Unit        QuantityUnitReference
	RequestedAt time.Time
}

func SubmitBookingRequest(spec BookingRequestSpec) (BookingRequest, error) {
	if !spec.TenantID.valid() ||
		!spec.Booking.valid() ||
		!spec.Commission.valid() ||
		spec.Quantity <= 0 ||
		!spec.Unit.valid() ||
		spec.RequestedAt.IsZero() {
		return BookingRequest{}, ErrInvalidBookingRequest
	}
	return BookingRequest{
		tenantID:    spec.TenantID,
		booking:     spec.Booking,
		commission:  spec.Commission,
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
	if at.IsZero() || at.Before(booking.requestedAt) {
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

// AcceptanceBasisReference 指名拒绝、失效或撤回的原因来源。
type AcceptanceBasisReference struct{ requiredValue }

func NewAcceptanceBasisReference(value string) (AcceptanceBasisReference, error) {
	required, err := newRequiredValue("acceptance basis reference", value)
	return AcceptanceBasisReference{required}, err
}

// CarrierAcceptanceOutcome 是承运方对订舱作答的封闭四值：接受成约；拒绝、失效与撤回
// 分格（CONTEXT 194：每次结果保留数量和来源，后续替代不覆盖原申请）。
type CarrierAcceptanceOutcome uint8

const (
	CarrierAcceptanceOutcomeInvalid CarrierAcceptanceOutcome = iota
	BookingAccepted
	BookingRefused
	BookingExpired
	BookingWithdrawn
)

func (outcome CarrierAcceptanceOutcome) valid() bool {
	return outcome >= BookingAccepted && outcome <= BookingWithdrawn
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
// （部分接受合法，CONTEXT 193）；拒绝/失效/撤回必须带原因来源且没有接受量。
func FormCarrierAcceptance(
	booking BookingRequest,
	acceptance CarrierAcceptanceReference,
	outcome CarrierAcceptanceOutcome,
	quantity int64,
	basis AcceptanceBasisReference,
	decidedAt time.Time,
) (CarrierAcceptance, error) {
	if !booking.booking.valid() || !acceptance.valid() || !outcome.valid() || decidedAt.IsZero() {
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

// Basis 在拒绝、失效或撤回时交回原因来源；接受没有它。
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
