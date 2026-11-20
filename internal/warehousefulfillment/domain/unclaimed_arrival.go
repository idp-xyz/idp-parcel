package domain

import (
	"slices"
	"time"
)

// UnclaimedArrivalID 是待认领到货的标识，由本上下文签发。前缀不在这个类型里：签发器单独打。
type UnclaimedArrivalID struct{ requiredValue }

func NewUnclaimedArrivalID(value string) (UnclaimedArrivalID, error) {
	required, err := newRequiredValue("unclaimed arrival ID", value)
	return UnclaimedArrivalID{required}, err
}

// UnclaimedArrivalFactID 是作业事实登记待认领到货时由设备签发的事实身份（ADR-0023）。
type UnclaimedArrivalFactID struct{ requiredValue }

func NewUnclaimedArrivalFactID(value string) (UnclaimedArrivalFactID, error) {
	required, err := newRequiredValue("unclaimed arrival fact ID", value)
	return UnclaimedArrivalFactID{required}, err
}

// UnclaimedArrivalObservation 是仓内作业对外箱单号、跟踪号、发件人、件数与外观的逐字观察。
// 严禁明文进日志或指标（UC-WF-005「权限、隔离与审计」）。
type UnclaimedArrivalObservation struct{ requiredValue }

func NewUnclaimedArrivalObservation(value string) (UnclaimedArrivalObservation, error) {
	required, err := newRequiredValue("unclaimed arrival observation", value)
	return UnclaimedArrivalObservation{required}, err
}

// UnclaimedArrivalStorageLocation 是仓内便于找货的逐字存放处描述，不是库存账上的库位。
type UnclaimedArrivalStorageLocation struct{ requiredValue }

func NewUnclaimedArrivalStorageLocation(value string) (UnclaimedArrivalStorageLocation, error) {
	required, err := newRequiredValue("unclaimed arrival storage location", value)
	return UnclaimedArrivalStorageLocation{required}, err
}

// UnclaimedArrivalNote 是撤销原因或处置说明，可选、逐字保存。
type UnclaimedArrivalNote struct{ requiredValue }

func NewUnclaimedArrivalNote(value string) (UnclaimedArrivalNote, error) {
	required, err := newRequiredValue("unclaimed arrival note", value)
	return UnclaimedArrivalNote{required}, err
}

// UnclaimedDisposalMethod 是租户自己定义的处置方式词，逐字保存、逐字比对，产品不内置任何词。
type UnclaimedDisposalMethod struct{ requiredValue }

func NewUnclaimedDisposalMethod(value string) (UnclaimedDisposalMethod, error) {
	required, err := newRequiredValue("unclaimed disposal method", value)
	return UnclaimedDisposalMethod{required}, err
}

// UnclaimedArrivalOrigin 区分待认领到货是一线作业直接登记的，还是由不能收货的收货事实转来的。
type UnclaimedArrivalOrigin uint8

const (
	UnclaimedArrivalOriginInvalid UnclaimedArrivalOrigin = iota
	UnclaimedArrivalOriginWorkFact
	UnclaimedArrivalOriginDivertedReceipt
)

func (origin UnclaimedArrivalOrigin) String() string {
	switch origin {
	case UnclaimedArrivalOriginWorkFact:
		return "WORK_FACT"
	case UnclaimedArrivalOriginDivertedReceipt:
		return "DIVERTED_RECEIPT"
	default:
		return ""
	}
}

func NewUnclaimedArrivalOrigin(word string) (UnclaimedArrivalOrigin, error) {
	switch word {
	case UnclaimedArrivalOriginWorkFact.String():
		return UnclaimedArrivalOriginWorkFact, nil
	case UnclaimedArrivalOriginDivertedReceipt.String():
		return UnclaimedArrivalOriginDivertedReceipt, nil
	default:
		return UnclaimedArrivalOriginInvalid, ErrInvalidUnclaimedArrival
	}
}

// UnclaimedArrivalKind 是待认领到货的封闭两种：入库到货或退件（UC-WF-005）。
type UnclaimedArrivalKind uint8

const (
	UnclaimedArrivalKindInvalid UnclaimedArrivalKind = iota
	UnclaimedArrivalKindInbound
	UnclaimedArrivalKindReturn
)

func (kind UnclaimedArrivalKind) String() string {
	switch kind {
	case UnclaimedArrivalKindInbound:
		return "INBOUND"
	case UnclaimedArrivalKindReturn:
		return "RETURN"
	default:
		return ""
	}
}

func NewUnclaimedArrivalKind(word string) (UnclaimedArrivalKind, error) {
	switch word {
	case UnclaimedArrivalKindInbound.String():
		return UnclaimedArrivalKindInbound, nil
	case UnclaimedArrivalKindReturn.String():
		return UnclaimedArrivalKindReturn, nil
	default:
		return UnclaimedArrivalKindInvalid, ErrInvalidUnclaimedArrival
	}
}

// UnclaimedArrivalStatus 是待认领到货的生命周期状态：待认领、已认领、已处置。
type UnclaimedArrivalStatus uint8

const (
	UnclaimedArrivalStatusInvalid UnclaimedArrivalStatus = iota
	UnclaimedArrivalStatusUnclaimed
	UnclaimedArrivalStatusClaimed
	UnclaimedArrivalStatusDisposed
)

func (status UnclaimedArrivalStatus) String() string {
	switch status {
	case UnclaimedArrivalStatusUnclaimed:
		return "UNCLAIMED"
	case UnclaimedArrivalStatusClaimed:
		return "CLAIMED"
	case UnclaimedArrivalStatusDisposed:
		return "DISPOSED"
	default:
		return ""
	}
}

func NewUnclaimedArrivalStatus(word string) (UnclaimedArrivalStatus, error) {
	switch word {
	case UnclaimedArrivalStatusUnclaimed.String():
		return UnclaimedArrivalStatusUnclaimed, nil
	case UnclaimedArrivalStatusClaimed.String():
		return UnclaimedArrivalStatusClaimed, nil
	case UnclaimedArrivalStatusDisposed.String():
		return UnclaimedArrivalStatusDisposed, nil
	default:
		return UnclaimedArrivalStatusInvalid, ErrInvalidUnclaimedArrival
	}
}

// UnclaimedArrival 是收货事实到达时预报已不能收货、原事实保全下来的那一件。
// Receipt 上是原事实的全部内容。货主取自预报，只作线索，不是认定这批货归它。
type UnclaimedArrival struct {
	id      UnclaimedArrivalID
	owner   OwnerAccountID
	receipt RecordedReceipt
}

func NewUnclaimedArrival(id UnclaimedArrivalID, owner OwnerAccountID, receipt RecordedReceipt) (UnclaimedArrival, error) {
	arrival := UnclaimedArrival{id: id, owner: owner, receipt: receipt}
	if !arrival.id.valid() || !arrival.owner.valid() || arrival.receipt.Outcome() != RecordedReceiptDivertedToUnclaimed ||
		!arrival.receipt.Tenant().valid() || !arrival.receipt.FactID().valid() || !arrival.receipt.PreAdvice().valid() {
		return UnclaimedArrival{}, ErrInvalidUnclaimedArrival
	}
	return arrival, nil
}

func (arrival UnclaimedArrival) ID() UnclaimedArrivalID { return arrival.id }
func (arrival UnclaimedArrival) Owner() OwnerAccountID  { return arrival.owner }
func (arrival UnclaimedArrival) Tenant() TenantID       { return arrival.receipt.Tenant() }
func (arrival UnclaimedArrival) PreAdvice() PreAdviceID { return arrival.receipt.PreAdvice() }
func (arrival UnclaimedArrival) Receipt() RecordedReceipt {
	return arrival.receipt
}

// DirectUnclaimedArrivalSubmission 是作业事实直接登记待认领到货时拿来比重放与内容冲突的内容，不含接收时刻。
type DirectUnclaimedArrivalSubmission struct {
	Device          ReceiptDeviceID
	Operator        ReceiptOperatorID
	OccurredAt      time.Time
	DeviceSequence  int64
	Warehouse       WarehouseCode
	Kind            UnclaimedArrivalKind
	Observation     UnclaimedArrivalObservation
	OwnerClue       OwnerAccountID
	PreAdviceClue   PreAdviceID
	StorageLocation UnclaimedArrivalStorageLocation
}

// RecordedUnclaimedArrivalSpec 构造一件在册待认领到货（作业事实直接登记或收货事实转来）。
type RecordedUnclaimedArrivalSpec struct {
	Tenant          TenantID
	ID              UnclaimedArrivalID
	FactID          UnclaimedArrivalFactID
	Origin          UnclaimedArrivalOrigin
	Kind            UnclaimedArrivalKind
	Status          UnclaimedArrivalStatus
	Warehouse       WarehouseCode
	Device          ReceiptDeviceID
	Operator        ReceiptOperatorID
	OccurredAt      time.Time
	DeviceSequence  int64
	ReceivedAt      time.Time
	Observation     UnclaimedArrivalObservation
	OwnerClue       OwnerAccountID
	PreAdviceClue   PreAdviceID
	StorageLocation UnclaimedArrivalStorageLocation
	DivertedReceipt *RecordedReceipt
}

// RecordedUnclaimedArrival 是一件已落册待认领到货的完整头部与原事实线索。
type RecordedUnclaimedArrival struct {
	tenant          TenantID
	id              UnclaimedArrivalID
	factID          UnclaimedArrivalFactID
	origin          UnclaimedArrivalOrigin
	kind            UnclaimedArrivalKind
	status          UnclaimedArrivalStatus
	warehouse       WarehouseCode
	device          ReceiptDeviceID
	operator        ReceiptOperatorID
	occurredAt      time.Time
	deviceSequence  int64
	receivedAt      time.Time
	observation     UnclaimedArrivalObservation
	ownerClue       OwnerAccountID
	preAdviceClue   PreAdviceID
	storageLocation UnclaimedArrivalStorageLocation
	divertedReceipt *RecordedReceipt
}

func NewRecordedUnclaimedArrival(spec RecordedUnclaimedArrivalSpec) (RecordedUnclaimedArrival, error) {
	item := RecordedUnclaimedArrival{
		tenant:          spec.Tenant,
		id:              spec.ID,
		factID:          spec.FactID,
		origin:          spec.Origin,
		kind:            spec.Kind,
		status:          spec.Status,
		warehouse:       spec.Warehouse,
		device:          spec.Device,
		operator:        spec.Operator,
		occurredAt:      normalizedInstant(spec.OccurredAt),
		deviceSequence:  spec.DeviceSequence,
		receivedAt:      normalizedInstant(spec.ReceivedAt),
		observation:     spec.Observation,
		ownerClue:       spec.OwnerClue,
		preAdviceClue:   spec.PreAdviceClue,
		storageLocation: spec.StorageLocation,
	}
	if spec.DivertedReceipt != nil {
		copied := *spec.DivertedReceipt
		item.divertedReceipt = &copied
	}
	if !item.tenant.valid() || !item.id.valid() || !item.factID.valid() ||
		item.origin.String() == "" || item.kind.String() == "" || item.status.String() == "" ||
		!item.warehouse.valid() || !item.device.valid() || !item.operator.valid() ||
		item.occurredAt.IsZero() || item.receivedAt.IsZero() {
		return RecordedUnclaimedArrival{}, ErrInvalidUnclaimedArrival
	}
	switch item.origin {
	case UnclaimedArrivalOriginWorkFact:
		if item.deviceSequence < 1 || !item.observation.valid() || item.divertedReceipt != nil {
			return RecordedUnclaimedArrival{}, ErrInvalidUnclaimedArrival
		}
	case UnclaimedArrivalOriginDivertedReceipt:
		if item.kind != UnclaimedArrivalKindInbound || !item.ownerClue.valid() || !item.preAdviceClue.valid() || item.deviceSequence != 0 {
			return RecordedUnclaimedArrival{}, ErrInvalidUnclaimedArrival
		}
	default:
		return RecordedUnclaimedArrival{}, ErrInvalidUnclaimedArrival
	}
	return item, nil
}

func (arrival RecordedUnclaimedArrival) Tenant() TenantID               { return arrival.tenant }
func (arrival RecordedUnclaimedArrival) ID() UnclaimedArrivalID         { return arrival.id }
func (arrival RecordedUnclaimedArrival) FactID() UnclaimedArrivalFactID { return arrival.factID }
func (arrival RecordedUnclaimedArrival) Origin() UnclaimedArrivalOrigin { return arrival.origin }
func (arrival RecordedUnclaimedArrival) Kind() UnclaimedArrivalKind     { return arrival.kind }
func (arrival RecordedUnclaimedArrival) Status() UnclaimedArrivalStatus { return arrival.status }
func (arrival RecordedUnclaimedArrival) Warehouse() WarehouseCode       { return arrival.warehouse }
func (arrival RecordedUnclaimedArrival) Device() ReceiptDeviceID        { return arrival.device }
func (arrival RecordedUnclaimedArrival) Operator() ReceiptOperatorID    { return arrival.operator }
func (arrival RecordedUnclaimedArrival) OccurredAt() time.Time          { return arrival.occurredAt }
func (arrival RecordedUnclaimedArrival) DeviceSequence() int64          { return arrival.deviceSequence }
func (arrival RecordedUnclaimedArrival) ReceivedAt() time.Time          { return arrival.receivedAt }
func (arrival RecordedUnclaimedArrival) Observation() (UnclaimedArrivalObservation, bool) {
	return arrival.observation, arrival.observation.valid()
}
func (arrival RecordedUnclaimedArrival) OwnerClue() (OwnerAccountID, bool) {
	return arrival.ownerClue, arrival.ownerClue.valid()
}
func (arrival RecordedUnclaimedArrival) PreAdviceClue() (PreAdviceID, bool) {
	return arrival.preAdviceClue, arrival.preAdviceClue.valid()
}
func (arrival RecordedUnclaimedArrival) StorageLocation() (UnclaimedArrivalStorageLocation, bool) {
	return arrival.storageLocation, arrival.storageLocation.valid()
}
func (arrival RecordedUnclaimedArrival) DivertedReceipt() (RecordedReceipt, bool) {
	if arrival.divertedReceipt == nil {
		return RecordedReceipt{}, false
	}
	return *arrival.divertedReceipt, true
}

// SameDirectContent 回答一次作业事实直接登记是不是和在册的这件到货内容完全相同。
func (arrival RecordedUnclaimedArrival) SameDirectContent(submission DirectUnclaimedArrivalSubmission) bool {
	if arrival.origin != UnclaimedArrivalOriginWorkFact {
		return false
	}
	return arrival.device == submission.Device &&
		arrival.operator == submission.Operator &&
		arrival.occurredAt.Equal(normalizedInstant(submission.OccurredAt)) &&
		arrival.deviceSequence == submission.DeviceSequence &&
		arrival.warehouse == submission.Warehouse &&
		arrival.kind == submission.Kind &&
		arrival.observation == submission.Observation &&
		arrival.ownerClue == submission.OwnerClue &&
		arrival.preAdviceClue == submission.PreAdviceClue &&
		arrival.storageLocation == submission.StorageLocation
}

// UnclaimedArrivalClaim 是一笔认领决定，只增不改。退件的 PreAdvice 为零值；入库到货必带 PreAdvice。
type UnclaimedArrivalClaim struct {
	tenant     TenantID
	decisionID RequestKey
	arrivalID  UnclaimedArrivalID
	owner      OwnerAccountID
	preAdvice  PreAdviceID
	operator   Registrant
	claimedAt  time.Time
}

func NewUnclaimedArrivalClaim(
	tenant TenantID,
	decisionID RequestKey,
	arrivalID UnclaimedArrivalID,
	owner OwnerAccountID,
	preAdvice PreAdviceID,
	operator Registrant,
	claimedAt time.Time,
) (UnclaimedArrivalClaim, error) {
	claim := UnclaimedArrivalClaim{
		tenant:     tenant,
		decisionID: decisionID,
		arrivalID:  arrivalID,
		owner:      owner,
		preAdvice:  preAdvice,
		operator:   operator,
		claimedAt:  normalizedInstant(claimedAt),
	}
	if !claim.tenant.valid() || !claim.decisionID.valid() || !claim.arrivalID.valid() ||
		!claim.owner.valid() || !claim.operator.valid() || claim.claimedAt.IsZero() {
		return UnclaimedArrivalClaim{}, ErrInvalidUnclaimedArrival
	}
	return claim, nil
}

func (claim UnclaimedArrivalClaim) Tenant() TenantID              { return claim.tenant }
func (claim UnclaimedArrivalClaim) DecisionID() RequestKey        { return claim.decisionID }
func (claim UnclaimedArrivalClaim) ArrivalID() UnclaimedArrivalID { return claim.arrivalID }
func (claim UnclaimedArrivalClaim) Owner() OwnerAccountID         { return claim.owner }
func (claim UnclaimedArrivalClaim) PreAdvice() (PreAdviceID, bool) {
	return claim.preAdvice, claim.preAdvice.valid()
}
func (claim UnclaimedArrivalClaim) Operator() Registrant { return claim.operator }
func (claim UnclaimedArrivalClaim) ClaimedAt() time.Time { return claim.claimedAt }

// SameTargetAs 回答两笔认领指名的是不是同一件到货、同一个货主与同一份预报（不比决定标识、操作者与时刻）。
func (claim UnclaimedArrivalClaim) SameTargetAs(arrivalID UnclaimedArrivalID, owner OwnerAccountID, preAdvice PreAdviceID) bool {
	return claim.arrivalID == arrivalID && claim.owner == owner && claim.preAdvice == preAdvice
}

// UnclaimedArrivalClaimRevocation 是一笔认领撤销决定，只增不改，指向被撤销的那一笔认领。
type UnclaimedArrivalClaimRevocation struct {
	tenant          TenantID
	decisionID      RequestKey
	arrivalID       UnclaimedArrivalID
	claimDecisionID RequestKey
	reason          UnclaimedArrivalNote
	operator        Registrant
	revokedAt       time.Time
}

func NewUnclaimedArrivalClaimRevocation(
	tenant TenantID,
	decisionID RequestKey,
	arrivalID UnclaimedArrivalID,
	claimDecisionID RequestKey,
	reason UnclaimedArrivalNote,
	operator Registrant,
	revokedAt time.Time,
) (UnclaimedArrivalClaimRevocation, error) {
	revocation := UnclaimedArrivalClaimRevocation{
		tenant:          tenant,
		decisionID:      decisionID,
		arrivalID:       arrivalID,
		claimDecisionID: claimDecisionID,
		reason:          reason,
		operator:        operator,
		revokedAt:       normalizedInstant(revokedAt),
	}
	if !revocation.tenant.valid() || !revocation.decisionID.valid() || !revocation.arrivalID.valid() ||
		!revocation.claimDecisionID.valid() || !revocation.operator.valid() || revocation.revokedAt.IsZero() {
		return UnclaimedArrivalClaimRevocation{}, ErrInvalidUnclaimedArrival
	}
	return revocation, nil
}

func (rev UnclaimedArrivalClaimRevocation) Tenant() TenantID              { return rev.tenant }
func (rev UnclaimedArrivalClaimRevocation) DecisionID() RequestKey        { return rev.decisionID }
func (rev UnclaimedArrivalClaimRevocation) ArrivalID() UnclaimedArrivalID { return rev.arrivalID }
func (rev UnclaimedArrivalClaimRevocation) ClaimDecisionID() RequestKey   { return rev.claimDecisionID }
func (rev UnclaimedArrivalClaimRevocation) Reason() (UnclaimedArrivalNote, bool) {
	return rev.reason, rev.reason.valid()
}
func (rev UnclaimedArrivalClaimRevocation) Operator() Registrant { return rev.operator }
func (rev UnclaimedArrivalClaimRevocation) RevokedAt() time.Time { return rev.revokedAt }

func (rev UnclaimedArrivalClaimRevocation) SameContentAs(arrivalID UnclaimedArrivalID, reason UnclaimedArrivalNote) bool {
	return rev.arrivalID == arrivalID && rev.reason == reason
}

// UnclaimedArrivalDisposition 是一笔待认领到货的处置决定。每件到货至多处置一次，不改库存账。
type UnclaimedArrivalDisposition struct {
	tenant        TenantID
	arrivalID     UnclaimedArrivalID
	method        UnclaimedDisposalMethod
	ruleWarehouse WarehouseCode
	ruleRevision  int
	note          UnclaimedArrivalNote
	operator      Registrant
	disposedAt    time.Time
}

func NewUnclaimedArrivalDisposition(
	tenant TenantID,
	arrivalID UnclaimedArrivalID,
	method UnclaimedDisposalMethod,
	ruleWarehouse WarehouseCode,
	ruleRevision int,
	note UnclaimedArrivalNote,
	operator Registrant,
	disposedAt time.Time,
) (UnclaimedArrivalDisposition, error) {
	disposition := UnclaimedArrivalDisposition{
		tenant:        tenant,
		arrivalID:     arrivalID,
		method:        method,
		ruleWarehouse: ruleWarehouse,
		ruleRevision:  ruleRevision,
		note:          note,
		operator:      operator,
		disposedAt:    normalizedInstant(disposedAt),
	}
	if !disposition.tenant.valid() || !disposition.arrivalID.valid() || !disposition.method.valid() ||
		!disposition.ruleWarehouse.valid() || disposition.ruleRevision < 1 ||
		!disposition.operator.valid() || disposition.disposedAt.IsZero() {
		return UnclaimedArrivalDisposition{}, ErrInvalidUnclaimedArrival
	}
	return disposition, nil
}

func (d UnclaimedArrivalDisposition) Tenant() TenantID              { return d.tenant }
func (d UnclaimedArrivalDisposition) ArrivalID() UnclaimedArrivalID { return d.arrivalID }
func (d UnclaimedArrivalDisposition) Method() UnclaimedDisposalMethod {
	return d.method
}
func (d UnclaimedArrivalDisposition) RuleWarehouse() WarehouseCode { return d.ruleWarehouse }
func (d UnclaimedArrivalDisposition) RuleRevision() int            { return d.ruleRevision }
func (d UnclaimedArrivalDisposition) Note() (UnclaimedArrivalNote, bool) {
	return d.note, d.note.valid()
}
func (d UnclaimedArrivalDisposition) Operator() Registrant  { return d.operator }
func (d UnclaimedArrivalDisposition) DisposedAt() time.Time { return d.disposedAt }

// ClaimHistoryEntry 是一笔认领以及它的撤销（若有），供查阅与当前有效认领的判断。
type ClaimHistoryEntry struct {
	Claim      UnclaimedArrivalClaim
	Revocation *UnclaimedArrivalClaimRevocation
}

// UnclaimedArrivalDetail 是一件待认领到货的完整全貌：到货自身、认领与撤销各笔、引用它的入账事实标识、处置。
type UnclaimedArrivalDetail struct {
	Arrival           RecordedUnclaimedArrival
	Claims            []ClaimHistoryEntry
	PostingReferences []ReceiptFactID
	Disposition       *UnclaimedArrivalDisposition
}

// ActiveClaim 交回此刻仍生效（未被撤销）的那一笔认领。
func (detail UnclaimedArrivalDetail) ActiveClaim() (UnclaimedArrivalClaim, bool) {
	if detail.Arrival.Status() != UnclaimedArrivalStatusClaimed {
		return UnclaimedArrivalClaim{}, false
	}
	for index := len(detail.Claims) - 1; index >= 0; index-- {
		if detail.Claims[index].Revocation == nil {
			return detail.Claims[index].Claim, true
		}
	}
	return UnclaimedArrivalClaim{}, false
}

// ErrInvalidUnclaimedArrival 是一件待认领到货凑不成自洽的一份。
var ErrInvalidUnclaimedArrival = errInvalidUnclaimedArrival{}

type errInvalidUnclaimedArrival struct{}

func (errInvalidUnclaimedArrival) Error() string {
	return "warehouse fulfillment: invalid unclaimed arrival"
}

// ClonePostingReferences 交回入账引用切片的副本。
func (detail UnclaimedArrivalDetail) ClonePostingReferences() []ReceiptFactID {
	return slices.Clone(detail.PostingReferences)
}
