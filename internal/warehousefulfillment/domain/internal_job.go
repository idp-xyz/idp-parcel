package domain

import (
	"slices"
	"time"
	"unicode/utf8"
)

// InternalJobKind 是库内作业事实的种类。上架、移库与拣选位补货都是库内移动，不在载荷里另报
// （UC-WF-007）。次品转良品不是作业事实，是运营决定。
type InternalJobKind uint8

const (
	InternalJobKindInvalid InternalJobKind = iota
	InternalJobMove
	InternalJobForm
	InternalJobCondition
	InternalJobShortage
)

func (kind InternalJobKind) String() string {
	switch kind {
	case InternalJobMove:
		return "INTERNAL_MOVE"
	case InternalJobForm:
		return "FORM_CONVERSION"
	case InternalJobCondition:
		return "CONDITION_CONVERSION"
	case InternalJobShortage:
		return "SHORTAGE"
	default:
		return ""
	}
}

// InternalJobFactID 是设备签发的库内作业事实身份。
type InternalJobFactID struct{ requiredValue }

func NewInternalJobFactID(value string) (InternalJobFactID, error) {
	required, err := newRequiredValue("internal job fact ID", value)
	return InternalJobFactID{required}, err
}

// StockDiscrepancyID 是一份待处理库存差异的身份，由本上下文签发。
type StockDiscrepancyID struct{ requiredValue }

func NewStockDiscrepancyID(value string) (StockDiscrepancyID, error) {
	required, err := newRequiredValue("stock discrepancy ID", value)
	return StockDiscrepancyID{required}, err
}

// ConditionRestoreID 是次品转良品的决定标识，由调用方给。
type ConditionRestoreID struct{ requiredValue }

func NewConditionRestoreID(value string) (ConditionRestoreID, error) {
	required, err := newRequiredValue("condition restore ID", value)
	return ConditionRestoreID{required}, err
}

// InternalJobLine 是一条库内作业事实的一行，只含提交内容，不含入账结果。
// 整箱行只填货主、箱号与目标库位。商品行填格子。数量不为正留给筛查指出来，不拦在构造器里。
type InternalJobLine struct {
	owner      OwnerAccountID
	goods      GoodsCode
	unit       GoodsUnitName
	state      StockState
	alias      GoodsAliasCode
	batch      BatchNumber
	expiry     ExpiryDate
	namedBatch InboundBatchID
	source     LocationCode
	target     LocationCode
	quantity   int64
	toUnit     GoodsUnitName
	toQuantity int64
	toState    StockState
	carton     CartonNumber
	note       string
}

// InternalJobLineSpec 是构造一行的输入。说明逐字保存，不去空白。
type InternalJobLineSpec struct {
	Owner      OwnerAccountID
	Goods      GoodsCode
	Unit       GoodsUnitName
	State      StockState
	Alias      GoodsAliasCode
	Batch      BatchNumber
	Expiry     ExpiryDate
	NamedBatch InboundBatchID
	Source     LocationCode
	Target     LocationCode
	Quantity   int64
	ToUnit     GoodsUnitName
	ToQuantity int64
	ToState    StockState
	Carton     CartonNumber
	Note       string
}

func NewInternalJobLine(spec InternalJobLineSpec) (InternalJobLine, error) {
	if !spec.Owner.valid() {
		return InternalJobLine{}, ErrBlankValue
	}
	if spec.Note != "" && (!utf8.ValidString(spec.Note) || containsNUL(spec.Note)) {
		return InternalJobLine{}, ErrUnstorableValue
	}
	return InternalJobLine{
		owner: spec.Owner, goods: spec.Goods, unit: spec.Unit, state: spec.State, alias: spec.Alias,
		batch: spec.Batch, expiry: spec.Expiry, namedBatch: spec.NamedBatch, source: spec.Source, target: spec.Target,
		quantity: spec.Quantity, toUnit: spec.ToUnit, toQuantity: spec.ToQuantity, toState: spec.ToState,
		carton: spec.Carton, note: spec.Note,
	}, nil
}

func containsNUL(value string) bool {
	for _, r := range value {
		if r == 0 {
			return true
		}
	}
	return false
}

func (line InternalJobLine) Owner() OwnerAccountID      { return line.owner }
func (line InternalJobLine) Goods() GoodsCode           { return line.goods }
func (line InternalJobLine) Unit() GoodsUnitName        { return line.unit }
func (line InternalJobLine) State() StockState          { return line.state }
func (line InternalJobLine) Alias() GoodsAliasCode      { return line.alias }
func (line InternalJobLine) Batch() BatchNumber         { return line.batch }
func (line InternalJobLine) Expiry() ExpiryDate         { return line.expiry }
func (line InternalJobLine) NamedBatch() InboundBatchID { return line.namedBatch }
func (line InternalJobLine) Source() LocationCode       { return line.source }
func (line InternalJobLine) Target() LocationCode       { return line.target }
func (line InternalJobLine) Quantity() int64            { return line.quantity }
func (line InternalJobLine) ToUnit() GoodsUnitName      { return line.toUnit }
func (line InternalJobLine) ToQuantity() int64          { return line.toQuantity }
func (line InternalJobLine) ToState() StockState        { return line.toState }
func (line InternalJobLine) Carton() CartonNumber       { return line.carton }
func (line InternalJobLine) Note() string               { return line.note }
func (line InternalJobLine) CartonLine() bool           { return line.carton.valid() }

func (line InternalJobLine) sameInput(other InternalJobLine) bool {
	return line == other
}

// InternalJobLineResult 是一行入账之后留下的结果。没入账的数量拆成账上不够与因占用取不走。
// 整箱对不上账时 CartonMissing 与 Held 互斥。Movement 没入账时为空。
type InternalJobLineResult struct {
	Posted        int64
	PostedInbound int64
	ShortOnBook   int64
	Held          int64
	CartonMissing bool
	Batches       []InboundBatchID
	Movement      string
	Discrepancy   StockDiscrepancyID
}

// RecordedInternalJobLine 是落册的一行：提交内容加结果。
type RecordedInternalJobLine struct {
	input  InternalJobLine
	result InternalJobLineResult
}

func NewRecordedInternalJobLine(input InternalJobLine, result InternalJobLineResult) RecordedInternalJobLine {
	result.Batches = slices.Clone(result.Batches)
	return RecordedInternalJobLine{input: input, result: result}
}

func (line RecordedInternalJobLine) Input() InternalJobLine { return line.input }
func (line RecordedInternalJobLine) Result() InternalJobLineResult {
	result := line.result
	result.Batches = slices.Clone(line.result.Batches)
	return result
}

// InternalJobSubmission 是一次登记的提交内容，用来和在册事实比是不是同一份。
type InternalJobSubmission struct {
	Kind           InternalJobKind
	Device         ReceiptDeviceID
	Operator       ReceiptOperatorID
	OccurredAt     time.Time
	DeviceSequence int64
	Warehouse      WarehouseCode
	Lines          []InternalJobLine
}

// RecordedInternalJob 是已经落册的一条库内作业事实。接收时刻不参与重放比较。
type RecordedInternalJob struct {
	tenant         TenantID
	factID         InternalJobFactID
	kind           InternalJobKind
	device         ReceiptDeviceID
	operator       ReceiptOperatorID
	occurredAt     time.Time
	deviceSequence int64
	receivedAt     time.Time
	warehouse      WarehouseCode
	lines          []RecordedInternalJobLine
}

func NewRecordedInternalJob(
	tenant TenantID,
	factID InternalJobFactID,
	submission InternalJobSubmission,
	receivedAt time.Time,
	lines []RecordedInternalJobLine,
) (RecordedInternalJob, error) {
	if !tenant.valid() || !factID.valid() || submission.Kind.String() == "" || !submission.Device.valid() ||
		!submission.Operator.valid() || submission.OccurredAt.IsZero() || submission.DeviceSequence < 1 ||
		!submission.Warehouse.valid() || receivedAt.IsZero() || len(lines) == 0 || len(lines) != len(submission.Lines) {
		return RecordedInternalJob{}, ErrBlankValue
	}
	stored := slices.Clone(lines)
	return RecordedInternalJob{
		tenant: tenant, factID: factID, kind: submission.Kind, device: submission.Device, operator: submission.Operator,
		occurredAt: normalizedInstant(submission.OccurredAt), deviceSequence: submission.DeviceSequence,
		receivedAt: normalizedInstant(receivedAt), warehouse: submission.Warehouse, lines: stored,
	}, nil
}

func (job RecordedInternalJob) Tenant() TenantID            { return job.tenant }
func (job RecordedInternalJob) FactID() InternalJobFactID   { return job.factID }
func (job RecordedInternalJob) Kind() InternalJobKind       { return job.kind }
func (job RecordedInternalJob) Device() ReceiptDeviceID     { return job.device }
func (job RecordedInternalJob) Operator() ReceiptOperatorID { return job.operator }
func (job RecordedInternalJob) OccurredAt() time.Time       { return job.occurredAt }
func (job RecordedInternalJob) DeviceSequence() int64       { return job.deviceSequence }
func (job RecordedInternalJob) ReceivedAt() time.Time       { return job.receivedAt }
func (job RecordedInternalJob) Warehouse() WarehouseCode    { return job.warehouse }
func (job RecordedInternalJob) Lines() []RecordedInternalJobLine {
	return slices.Clone(job.lines)
}

// SameContent 按提交内容比。接收时刻、入账数量与差异标识不参与：原样重投要回显在册结果，而不是另记一份。
func (job RecordedInternalJob) SameContent(submission InternalJobSubmission) bool {
	if job.kind != submission.Kind || job.device != submission.Device || job.operator != submission.Operator ||
		job.deviceSequence != submission.DeviceSequence || job.warehouse != submission.Warehouse ||
		len(job.lines) != len(submission.Lines) {
		return false
	}
	if !job.occurredAt.Equal(normalizedInstant(submission.OccurredAt)) {
		return false
	}
	for index := range job.lines {
		if !job.lines[index].input.sameInput(submission.Lines[index]) {
			return false
		}
	}
	return true
}

// InternalJobIssueKind 是整条事实不成立时逐行指出的种类。字面量进 CONTENT_INVALID 的 issues。
type InternalJobIssueKind uint8

const (
	InternalJobIssueKindInvalid InternalJobIssueKind = iota
	InternalJobIssueLocationNotFound
	InternalJobIssueOutboundStaging
	InternalJobIssueGoodsNotRegistered
	InternalJobIssueUnitNotDeclared
	InternalJobIssueQuantityNotPositive
	InternalJobIssueBatchRequired
	InternalJobIssueBatchNotTracked
	InternalJobIssueExpiryRequired
	InternalJobIssueExpiryNotTracked
	InternalJobIssueLabelAliasNotPlatform
	InternalJobIssueZoneIncompatible
	InternalJobIssueCartonZone
	InternalJobIssueFactorsUnequal
	InternalJobIssueSameUnit
	InternalJobIssueConditionNotAllowed
	InternalJobIssueReplenishShape
	InternalJobIssueLineShape
	InternalJobIssueNotEnough
	InternalJobIssueHeld
)

func (kind InternalJobIssueKind) String() string {
	switch kind {
	case InternalJobIssueLocationNotFound:
		return "LOCATION_NOT_FOUND"
	case InternalJobIssueOutboundStaging:
		return "OUTBOUND_STAGING"
	case InternalJobIssueGoodsNotRegistered:
		return "GOODS_NOT_REGISTERED"
	case InternalJobIssueUnitNotDeclared:
		return "UNIT_NOT_DECLARED"
	case InternalJobIssueQuantityNotPositive:
		return "QUANTITY_NOT_POSITIVE"
	case InternalJobIssueBatchRequired:
		return "BATCH_NUMBER_REQUIRED"
	case InternalJobIssueBatchNotTracked:
		return "BATCH_NUMBER_NOT_TRACKED"
	case InternalJobIssueExpiryRequired:
		return "EXPIRY_REQUIRED"
	case InternalJobIssueExpiryNotTracked:
		return "EXPIRY_NOT_TRACKED"
	case InternalJobIssueLabelAliasNotPlatform:
		return "LABEL_ALIAS_NOT_PLATFORM"
	case InternalJobIssueZoneIncompatible:
		return "ZONE_INCOMPATIBLE"
	case InternalJobIssueCartonZone:
		return "CARTON_ZONE"
	case InternalJobIssueFactorsUnequal:
		return "FACTORS_UNEQUAL"
	case InternalJobIssueSameUnit:
		return "SAME_UNIT"
	case InternalJobIssueConditionNotAllowed:
		return "CONDITION_NOT_ALLOWED"
	case InternalJobIssueReplenishShape:
		return "REPLENISH_SHAPE"
	case InternalJobIssueLineShape:
		return "LINE_SHAPE"
	case InternalJobIssueNotEnough:
		return "NOT_ENOUGH"
	case InternalJobIssueHeld:
		return "HELD"
	default:
		return ""
	}
}

// InternalJobIssue 是整条事实的一处不合。Line 是行下标。
type InternalJobIssue struct {
	line int
	kind InternalJobIssueKind
}

func NewInternalJobIssue(line int, kind InternalJobIssueKind) InternalJobIssue {
	return InternalJobIssue{line: line, kind: kind}
}

func (issue InternalJobIssue) Line() int                  { return issue.line }
func (issue InternalJobIssue) Kind() InternalJobIssueKind { return issue.kind }

// StockDiscrepancy 是库内作业对不上账的那一部分，停在待处理。记在移入或转成的那一格，另记源头。
// 短少是观察，不与账核对，ShortOnBook 与 Held 都是零。
type StockDiscrepancy struct {
	tenant        TenantID
	id            StockDiscrepancyID
	factID        InternalJobFactID
	line          int
	owner         OwnerAccountID
	warehouse     WarehouseCode
	location      LocationCode
	goods         GoodsCode
	unit          GoodsUnitName
	state         StockState
	alias         GoodsAliasCode
	batch         BatchNumber
	expiry        ExpiryDate
	inboundBatch  InboundBatchID
	source        LocationCode
	carton        CartonNumber
	note          string
	quantity      int64
	shortOnBook   int64
	held          int64
	cartonMissing bool
}

// StockDiscrepancySpec 是构造一份库存差异的输入。说明逐字保存。
type StockDiscrepancySpec struct {
	Tenant        TenantID
	ID            StockDiscrepancyID
	FactID        InternalJobFactID
	Line          int
	Owner         OwnerAccountID
	Warehouse     WarehouseCode
	Location      LocationCode
	Goods         GoodsCode
	Unit          GoodsUnitName
	State         StockState
	Alias         GoodsAliasCode
	Batch         BatchNumber
	Expiry        ExpiryDate
	InboundBatch  InboundBatchID
	Source        LocationCode
	Carton        CartonNumber
	Note          string
	Quantity      int64
	ShortOnBook   int64
	Held          int64
	CartonMissing bool
}

func NewStockDiscrepancy(spec StockDiscrepancySpec) (StockDiscrepancy, error) {
	if !spec.Tenant.valid() || !spec.ID.valid() || !spec.FactID.valid() || spec.Line < 0 ||
		!spec.Owner.valid() || !spec.Warehouse.valid() || spec.Quantity <= 0 || spec.ShortOnBook < 0 || spec.Held < 0 {
		return StockDiscrepancy{}, ErrBlankValue
	}
	if spec.Note != "" && (!utf8.ValidString(spec.Note) || containsNUL(spec.Note)) {
		return StockDiscrepancy{}, ErrUnstorableValue
	}
	return StockDiscrepancy{
		tenant: spec.Tenant, id: spec.ID, factID: spec.FactID, line: spec.Line, owner: spec.Owner, warehouse: spec.Warehouse,
		location: spec.Location, goods: spec.Goods, unit: spec.Unit, state: spec.State, alias: spec.Alias, batch: spec.Batch,
		expiry: spec.Expiry, inboundBatch: spec.InboundBatch, source: spec.Source, carton: spec.Carton, note: spec.Note,
		quantity: spec.Quantity, shortOnBook: spec.ShortOnBook, held: spec.Held, cartonMissing: spec.CartonMissing,
	}, nil
}

func (discrepancy StockDiscrepancy) Tenant() TenantID             { return discrepancy.tenant }
func (discrepancy StockDiscrepancy) ID() StockDiscrepancyID       { return discrepancy.id }
func (discrepancy StockDiscrepancy) FactID() InternalJobFactID    { return discrepancy.factID }
func (discrepancy StockDiscrepancy) Line() int                    { return discrepancy.line }
func (discrepancy StockDiscrepancy) Owner() OwnerAccountID        { return discrepancy.owner }
func (discrepancy StockDiscrepancy) Warehouse() WarehouseCode     { return discrepancy.warehouse }
func (discrepancy StockDiscrepancy) Location() LocationCode       { return discrepancy.location }
func (discrepancy StockDiscrepancy) Goods() GoodsCode             { return discrepancy.goods }
func (discrepancy StockDiscrepancy) Unit() GoodsUnitName          { return discrepancy.unit }
func (discrepancy StockDiscrepancy) State() StockState            { return discrepancy.state }
func (discrepancy StockDiscrepancy) Alias() GoodsAliasCode        { return discrepancy.alias }
func (discrepancy StockDiscrepancy) Batch() BatchNumber           { return discrepancy.batch }
func (discrepancy StockDiscrepancy) Expiry() ExpiryDate           { return discrepancy.expiry }
func (discrepancy StockDiscrepancy) InboundBatch() InboundBatchID { return discrepancy.inboundBatch }
func (discrepancy StockDiscrepancy) Source() LocationCode         { return discrepancy.source }
func (discrepancy StockDiscrepancy) Carton() CartonNumber         { return discrepancy.carton }
func (discrepancy StockDiscrepancy) Note() string                 { return discrepancy.note }
func (discrepancy StockDiscrepancy) Quantity() int64              { return discrepancy.quantity }
func (discrepancy StockDiscrepancy) ShortOnBook() int64           { return discrepancy.shortOnBook }
func (discrepancy StockDiscrepancy) Held() int64                  { return discrepancy.held }
func (discrepancy StockDiscrepancy) CartonMissing() bool          { return discrepancy.cartonMissing }

// ConditionRestore 是成立的次品转良品。整份成立才落册；不成立的不占决定标识。
type ConditionRestore struct {
	tenant     TenantID
	id         ConditionRestoreID
	operator   Registrant
	occurredAt time.Time
	receivedAt time.Time
	warehouse  WarehouseCode
	owner      OwnerAccountID
	location   LocationCode
	goods      GoodsCode
	unit       GoodsUnitName
	alias      GoodsAliasCode
	batch      BatchNumber
	expiry     ExpiryDate
	namedBatch InboundBatchID
	quantity   int64
	reason     string
	movement   StockMovementID
}

// ConditionRestoreSpec 是构造一条次品转良品的输入。原因逐字保存。
type ConditionRestoreSpec struct {
	Tenant     TenantID
	ID         ConditionRestoreID
	Operator   Registrant
	OccurredAt time.Time
	ReceivedAt time.Time
	Warehouse  WarehouseCode
	Owner      OwnerAccountID
	Location   LocationCode
	Goods      GoodsCode
	Unit       GoodsUnitName
	Alias      GoodsAliasCode
	Batch      BatchNumber
	Expiry     ExpiryDate
	NamedBatch InboundBatchID
	Quantity   int64
	Reason     string
	Movement   StockMovementID
}

func NewConditionRestore(spec ConditionRestoreSpec) (ConditionRestore, error) {
	if !spec.Tenant.valid() || !spec.ID.valid() || !spec.Operator.valid() || spec.OccurredAt.IsZero() ||
		spec.ReceivedAt.IsZero() || !spec.Warehouse.valid() || !spec.Owner.valid() || !spec.Location.valid() ||
		!spec.Goods.valid() || !spec.Unit.valid() || spec.Quantity <= 0 || !spec.Movement.valid() {
		return ConditionRestore{}, ErrBlankValue
	}
	if spec.Reason == "" || !utf8.ValidString(spec.Reason) || containsNUL(spec.Reason) {
		return ConditionRestore{}, ErrUnstorableValue
	}
	return ConditionRestore{
		tenant: spec.Tenant, id: spec.ID, operator: spec.Operator, occurredAt: normalizedInstant(spec.OccurredAt),
		receivedAt: normalizedInstant(spec.ReceivedAt), warehouse: spec.Warehouse, owner: spec.Owner, location: spec.Location,
		goods: spec.Goods, unit: spec.Unit, alias: spec.Alias, batch: spec.Batch, expiry: spec.Expiry, namedBatch: spec.NamedBatch,
		quantity: spec.Quantity, reason: spec.Reason, movement: spec.Movement,
	}, nil
}

func (restore ConditionRestore) Tenant() TenantID           { return restore.tenant }
func (restore ConditionRestore) ID() ConditionRestoreID     { return restore.id }
func (restore ConditionRestore) Operator() Registrant       { return restore.operator }
func (restore ConditionRestore) OccurredAt() time.Time      { return restore.occurredAt }
func (restore ConditionRestore) ReceivedAt() time.Time      { return restore.receivedAt }
func (restore ConditionRestore) Warehouse() WarehouseCode   { return restore.warehouse }
func (restore ConditionRestore) Owner() OwnerAccountID      { return restore.owner }
func (restore ConditionRestore) Location() LocationCode     { return restore.location }
func (restore ConditionRestore) Goods() GoodsCode           { return restore.goods }
func (restore ConditionRestore) Unit() GoodsUnitName        { return restore.unit }
func (restore ConditionRestore) Alias() GoodsAliasCode      { return restore.alias }
func (restore ConditionRestore) Batch() BatchNumber         { return restore.batch }
func (restore ConditionRestore) Expiry() ExpiryDate         { return restore.expiry }
func (restore ConditionRestore) NamedBatch() InboundBatchID { return restore.namedBatch }
func (restore ConditionRestore) Quantity() int64            { return restore.quantity }
func (restore ConditionRestore) Reason() string             { return restore.reason }
func (restore ConditionRestore) Movement() StockMovementID  { return restore.movement }

// ConditionRestoreSubmission 是一次次品转良品的提交内容。接收时刻、决定时刻与流水不参与重放比较：
// 时刻是本上下文收下决定时盖的，重投若再盖一次就会把原样重投判成来源冲突。
type ConditionRestoreSubmission struct {
	Operator   Registrant
	Warehouse  WarehouseCode
	Owner      OwnerAccountID
	Location   LocationCode
	Goods      GoodsCode
	Unit       GoodsUnitName
	Alias      GoodsAliasCode
	Batch      BatchNumber
	Expiry     ExpiryDate
	NamedBatch InboundBatchID
	Quantity   int64
	Reason     string
}

// SameContent 比决定的业务内容。操作者参与：换了人提交同一标识是来源冲突。
func (restore ConditionRestore) SameContent(submission ConditionRestoreSubmission) bool {
	return restore.operator == submission.Operator && restore.warehouse == submission.Warehouse && restore.owner == submission.Owner &&
		restore.location == submission.Location && restore.goods == submission.Goods && restore.unit == submission.Unit &&
		restore.alias == submission.Alias && restore.batch == submission.Batch && restore.expiry == submission.Expiry &&
		restore.namedBatch == submission.NamedBatch && restore.quantity == submission.Quantity && restore.reason == submission.Reason
}
