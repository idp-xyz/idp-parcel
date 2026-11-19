package application

import (
	"context"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/refdata"
)

var (
	errUnnamedMeasurementSave   = errors.New("node operations: unnamed measurement save")
	errMeasurementReplayMissing = errors.New("node operations: recorded measurement was not readable")

	// ErrMeasurementInvalidationScopeMissing 表示作废命令没带范围。这是装配或译装的错，不是业务结果：漏传的范围若按「不限节点」
	// 办，作业端设备就能作废别的节点的测量，所以编排拒收，不替它挑一种。
	ErrMeasurementInvalidationScopeMissing = errors.New("node operations: measurement invalidation carries no scope")
)

// RecordActualMeasurementOutcome 是一次测量登记的结果。已有结果与来源冲突都已经有了
// 答案，不是依赖故障。
type RecordActualMeasurementOutcome uint8

const (
	RecordActualMeasurementOutcomeInvalid RecordActualMeasurementOutcome = iota
	ActualMeasurementRecorded
	ActualMeasurementExisting
	ActualMeasurementSourceConflict
	ActualMeasurementNotAccepted
)

func (outcome RecordActualMeasurementOutcome) String() string {
	switch outcome {
	case ActualMeasurementRecorded:
		return "MEASUREMENT_RECORDED"
	case ActualMeasurementExisting:
		return "EXISTING_RESULT"
	case ActualMeasurementSourceConflict:
		return "SOURCE_CONFLICT"
	case ActualMeasurementNotAccepted:
		return "NOT_ACCEPTED"
	case RecordActualMeasurementOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// RecordActualMeasurementCommand 是一次测量上报：重量与长宽高都必填。单位按产品内置目录核对，数字原样
// 留下，不在这里换算。录入人与原件只在手工录入时都有。
type RecordActualMeasurementCommand struct {
	TenantID         string
	SourceID         string
	PerformedBy      string
	Evidence         string
	OccurredAt       time.Time
	ObjectKind       string
	ObjectID         string
	Node             string
	Location         string
	Basis            string
	WeightValue      string
	WeightUnit       string
	Length           string
	Width            string
	Height           string
	DimensionUnit    string
	EnteredBy        string
	OriginalDocument string
}

// RecordActualMeasurementResult 交回登记结果。测量只在记下或重放时给出。
type RecordActualMeasurementResult struct {
	outcome     RecordActualMeasurementOutcome
	measurement domain.ActualMeasurement
	present     bool
}

func (result RecordActualMeasurementResult) Outcome() RecordActualMeasurementOutcome {
	return result.outcome
}

func (result RecordActualMeasurementResult) Measurement() (domain.ActualMeasurement, bool) {
	return result.measurement, result.present
}

// RecordActualMeasurementHandler 把一条实际测量写入登记册。它不挑当前有效的那一条。
// 记下或重放时把这次变化交出去，费用重评由下游按那封信另起评价。
type RecordActualMeasurementHandler struct {
	measurements ports.ActualMeasurementStore
	clock        ports.Clock
	notice       ports.MeasurementChangeNotice
}

func NewRecordActualMeasurementHandler(
	measurements ports.ActualMeasurementStore,
	clock ports.Clock,
	notice ports.MeasurementChangeNotice,
) (*RecordActualMeasurementHandler, error) {
	if measurements == nil || clock == nil || notice == nil {
		return nil, errUnnamedMeasurementSave
	}
	return &RecordActualMeasurementHandler{measurements: measurements, clock: clock, notice: notice}, nil
}

func (handler *RecordActualMeasurementHandler) Handle(
	ctx context.Context,
	command RecordActualMeasurementCommand,
) (RecordActualMeasurementResult, error) {
	measurement, err := measurementFromCommand(command)
	if err != nil {
		return RecordActualMeasurementResult{outcome: ActualMeasurementNotAccepted}, nil
	}
	record := ports.ActualMeasurementRecord{
		Key: ports.ActualMeasurementKey{
			TenantID: measurementTenant(command.TenantID),
			SourceID: measurement.Source().SourceID(),
		},
		Measurement:   measurement,
		ContentDigest: measurement.ContentDigest(),
		RecordedAt:    handler.clock.Now(),
	}
	taken, err := handler.invalidationHoldsSource(ctx, record.Key)
	if err != nil {
		return RecordActualMeasurementResult{}, err
	}
	if taken {
		return RecordActualMeasurementResult{outcome: ActualMeasurementSourceConflict}, nil
	}
	saved, err := handler.measurements.SaveMeasurement(ctx, record)
	if err != nil {
		return RecordActualMeasurementResult{}, err
	}
	switch saved {
	case ports.ActualMeasurementSaved:
		if err := handler.noticeChange(ctx, record.Key.TenantID, measurement); err != nil {
			return RecordActualMeasurementResult{}, err
		}
		return RecordActualMeasurementResult{outcome: ActualMeasurementRecorded, measurement: measurement, present: true}, nil
	case ports.ActualMeasurementAlreadyRecorded:
		return handler.replayMeasurement(ctx, record.Key, record.ContentDigest)
	default:
		return RecordActualMeasurementResult{}, errUnnamedMeasurementSave
	}
}

func (handler *RecordActualMeasurementHandler) replayMeasurement(
	ctx context.Context,
	key ports.ActualMeasurementKey,
	digest string,
) (RecordActualMeasurementResult, error) {
	existing, found, err := handler.measurements.FindMeasurement(ctx, key)
	if err != nil {
		return RecordActualMeasurementResult{}, err
	}
	if !found {
		return RecordActualMeasurementResult{}, errMeasurementReplayMissing
	}
	if existing.ContentDigest != digest {
		return RecordActualMeasurementResult{outcome: ActualMeasurementSourceConflict}, nil
	}
	if err := handler.noticeChange(ctx, key.TenantID, existing.Measurement); err != nil {
		return RecordActualMeasurementResult{}, err
	}
	return RecordActualMeasurementResult{outcome: ActualMeasurementExisting, measurement: existing.Measurement, present: true}, nil
}

func (handler *RecordActualMeasurementHandler) noticeChange(
	ctx context.Context,
	tenant domain.TenantID,
	measurement domain.ActualMeasurement,
) error {
	return handler.notice.NoticeMeasurementChanged(ctx, ports.MeasurementChanged{
		Tenant:     tenant,
		SourceID:   measurement.Source().SourceID(),
		ObjectKind: measurement.ObjectKind(),
		ObjectID:   measurement.ObjectID(),
		OccurredAt: measurement.Source().OccurredAt(),
	})
}

func measurementFromCommand(command RecordActualMeasurementCommand) (domain.ActualMeasurement, error) {
	if _, err := domain.NewTenantID(command.TenantID); err != nil {
		return domain.ActualMeasurement{}, err
	}
	source, err := workFactSource(command.SourceID, command.PerformedBy, command.Evidence, command.OccurredAt)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	objectKind, err := domain.MeasurementObjectKindFromName(command.ObjectKind)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	node, err := domain.NewNodeReference(command.Node)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	location, err := domain.NewWorkLocationReference(command.Location)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	basis, err := domain.NewWorkBasisReference(command.Basis)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	weightUnit, dimensionUnit, err := catalogueUnits(command.WeightUnit, command.DimensionUnit)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	weight, err := domain.NewGrossWeight(command.WeightValue, weightUnit)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	dimensions, err := domain.NewOuterDimensions(command.Length, command.Width, command.Height, dimensionUnit)
	if err != nil {
		return domain.ActualMeasurement{}, err
	}
	spec := domain.ActualMeasurementSpec{
		Source: source, ObjectKind: objectKind, ObjectID: command.ObjectID,
		Node: node, Location: location, Basis: basis,
		Weight: weight, Dimensions: dimensions,
		EnteredBy: strings.TrimSpace(command.EnteredBy),
	}
	if strings.TrimSpace(command.OriginalDocument) != "" {
		spec.OriginalDocument, err = domain.NewMeasurementOriginalDocument(command.OriginalDocument)
		if err != nil {
			return domain.ActualMeasurement{}, err
		}
	}
	return domain.NewActualMeasurement(spec)
}

// catalogueUnits 对着产品内置单位目录核重量与长度两个单位，交回目录上的写法。两个都要认得：一条测量同时带重量与尺寸。
func catalogueUnits(weightUnit, dimensionUnit string) (string, string, error) {
	weight, ok := unitCode(weightUnit, refdata.UnitCategoryWeight)
	if !ok {
		return "", "", domain.ErrInvalidActualMeasurement
	}
	length, ok := unitCode(dimensionUnit, refdata.UnitCategoryLength)
	if !ok {
		return "", "", domain.ErrInvalidActualMeasurement
	}
	return weight, length, nil
}

func unitCode(raw string, category refdata.UnitCategory) (string, bool) {
	info, found := refdata.Unit(strings.TrimSpace(raw))
	if !found || info.Category != category {
		return "", false
	}
	return info.Code, true
}

func workFactSource(sourceID, performedBy, evidence string, occurredAt time.Time) (domain.WorkFactSource, error) {
	party, err := domain.NewPerformingPartyReference(performedBy)
	if err != nil {
		return domain.WorkFactSource{}, err
	}
	proof, err := domain.NewExecutionEvidenceReference(evidence)
	if err != nil {
		return domain.WorkFactSource{}, err
	}
	return domain.NewWorkFactSource(sourceID, party, proof, occurredAt)
}

// invalidationHoldsSource 与 measurementHoldsSource 守测量与作废共用的来源身份空间（缘由见 ports.ActualMeasurementKey）：先取
// （租户，来源身份）的锁，再看另一类的册上有没有这个来源身份。锁必须取在读之前：先读后锁，并发到的两条路都读到「没有」。
// 同类之内的判重仍由各自的写口认。
func (handler *RecordActualMeasurementHandler) invalidationHoldsSource(ctx context.Context, key ports.ActualMeasurementKey) (bool, error) {
	if err := handler.measurements.SerializeSourceIdentity(ctx, key); err != nil {
		return false, err
	}
	_, held, err := handler.measurements.FindInvalidation(ctx, key)
	return held, err
}

func (handler *RecordActualMeasurementHandler) measurementHoldsSource(ctx context.Context, key ports.ActualMeasurementKey) (bool, error) {
	if err := handler.measurements.SerializeSourceIdentity(ctx, key); err != nil {
		return false, err
	}
	_, held, err := handler.measurements.FindMeasurement(ctx, key)
	return held, err
}

func measurementTenant(raw string) domain.TenantID {
	tenant, err := domain.NewTenantID(raw)
	if err != nil {
		return domain.TenantID{}
	}
	return tenant
}

// InvalidateActualMeasurementOutcome 是一次作废的结果。目标不在册与已经被作废是两种
// 改不了重发内容的拒绝。
type InvalidateActualMeasurementOutcome uint8

const (
	InvalidateActualMeasurementOutcomeInvalid InvalidateActualMeasurementOutcome = iota
	MeasurementInvalidationRecorded
	MeasurementInvalidationExisting
	MeasurementInvalidationSourceConflict
	MeasurementInvalidationNotAccepted
	MeasurementInvalidationTargetMissing
	MeasurementInvalidationAlreadyVoid
	MeasurementInvalidationTargetAtAnotherNode
)

func (outcome InvalidateActualMeasurementOutcome) String() string {
	switch outcome {
	case MeasurementInvalidationRecorded:
		return "INVALIDATION_RECORDED"
	case MeasurementInvalidationExisting:
		return "EXISTING_RESULT"
	case MeasurementInvalidationSourceConflict:
		return "SOURCE_CONFLICT"
	case MeasurementInvalidationNotAccepted:
		return "NOT_ACCEPTED"
	case MeasurementInvalidationTargetMissing:
		return "TARGET_MISSING"
	case MeasurementInvalidationAlreadyVoid:
		return "ALREADY_VOID"
	case MeasurementInvalidationTargetAtAnotherNode:
		return "TARGET_AT_ANOTHER_NODE"
	case InvalidateActualMeasurementOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// MeasurementInvalidationScope 说一次作废可以指向哪些节点上登记的测量。两种来路在类型上分开：作业端设备只能作废在认证出的
// 那个节点上登记的测量（InvalidationAtNode），手工作废是运营决定、持授予的操作者可以作废本租户任何节点的测量
// （InvalidationAcrossTenantNodes）。零值不是任何一种：用空节点表示「不限」会让漏传悄悄放开范围，所以零值被编排拒收。
type MeasurementInvalidationScope struct {
	node   domain.NodeReference
	across bool
}

// InvalidationAtNode 把作废限定在 node 上登记的测量。node 是零值时得到的是没带范围，不是不限节点。
func InvalidationAtNode(node domain.NodeReference) MeasurementInvalidationScope {
	return MeasurementInvalidationScope{node: node}
}

// InvalidationAcrossTenantNodes 不限节点：本租户任何节点上登记的测量都可以作废。只有手工作废这条来路该用它。
func InvalidationAcrossTenantNodes() MeasurementInvalidationScope {
	return MeasurementInvalidationScope{across: true}
}

// AtNode 交回被限定的节点；不是「限定在某个节点」的范围时 ok 为假。
func (scope MeasurementInvalidationScope) AtNode() (node domain.NodeReference, ok bool) {
	if scope.across || scope.node.String() == "" {
		return domain.NodeReference{}, false
	}
	return scope.node, true
}

// AcrossTenantNodes 说这是不限节点的范围。
func (scope MeasurementInvalidationScope) AcrossTenantNodes() bool {
	return scope.across && scope.node.String() == ""
}

func (scope MeasurementInvalidationScope) set() bool {
	_, atNode := scope.AtNode()
	return atNode || scope.AcrossTenantNodes()
}

// InvalidateActualMeasurementCommand 指回被作废那条的来源身份。它不带重量或尺寸：
// 作废不复制被作废的数字。Scope 必带：作业端与手工两种来路各交各的范围，漏传不收。
type InvalidateActualMeasurementCommand struct {
	TenantID         string
	SourceID         string
	TargetSourceID   string
	Reason           string
	PerformedBy      string
	Evidence         string
	OccurredAt       time.Time
	EnteredBy        string
	OriginalDocument string
	Scope            MeasurementInvalidationScope
}

// InvalidateActualMeasurementResult 交回作废结果。作废事实只在记下或重放时给出。
type InvalidateActualMeasurementResult struct {
	outcome      InvalidateActualMeasurementOutcome
	invalidation domain.MeasurementInvalidation
	present      bool
}

func (result InvalidateActualMeasurementResult) Outcome() InvalidateActualMeasurementOutcome {
	return result.outcome
}

func (result InvalidateActualMeasurementResult) Invalidation() (domain.MeasurementInvalidation, bool) {
	return result.invalidation, result.present
}

// Invalidate 写下一条作废。同一条测量上再来一条不同来源的作废答已经作废，不覆盖先到的那条。判断顺序是：来源身份撞另一类的
// 答来源冲突，目标不在册，目标在别的节点（只限作业端的范围），然后才是已作废与重放；所以别的节点的设备连重放答原结果也拿不到。
func (handler *RecordActualMeasurementHandler) Invalidate(
	ctx context.Context,
	command InvalidateActualMeasurementCommand,
) (InvalidateActualMeasurementResult, error) {
	if !command.Scope.set() {
		return InvalidateActualMeasurementResult{}, ErrMeasurementInvalidationScopeMissing
	}
	invalidation, err := invalidationFromCommand(command)
	if err != nil {
		return InvalidateActualMeasurementResult{outcome: MeasurementInvalidationNotAccepted}, nil
	}
	record := ports.MeasurementInvalidationRecord{
		Key: ports.ActualMeasurementKey{
			TenantID: measurementTenant(command.TenantID),
			SourceID: invalidation.Source().SourceID(),
		},
		Invalidation:  invalidation,
		ContentDigest: invalidation.ContentDigest(),
		RecordedAt:    handler.clock.Now(),
	}
	taken, err := handler.measurementHoldsSource(ctx, record.Key)
	if err != nil {
		return InvalidateActualMeasurementResult{}, err
	}
	if taken {
		return InvalidateActualMeasurementResult{outcome: MeasurementInvalidationSourceConflict}, nil
	}
	if node, limited := command.Scope.AtNode(); limited {
		refusal, refused, err := handler.targetOutsideNode(ctx, record.Key.TenantID, invalidation.TargetSourceID(), node)
		if err != nil {
			return InvalidateActualMeasurementResult{}, err
		}
		if refused {
			return InvalidateActualMeasurementResult{outcome: refusal}, nil
		}
	}
	saved, err := handler.measurements.SaveInvalidation(ctx, record)
	if err != nil {
		return InvalidateActualMeasurementResult{}, err
	}
	switch saved {
	case ports.MeasurementInvalidationSaved:
		if err := handler.noticeInvalidation(ctx, record.Key.TenantID, invalidation); err != nil {
			return InvalidateActualMeasurementResult{}, err
		}
		return InvalidateActualMeasurementResult{
			outcome: MeasurementInvalidationRecorded, invalidation: invalidation, present: true,
		}, nil
	case ports.MeasurementInvalidationAlreadyRecorded:
		return handler.replayInvalidation(ctx, record.Key, record.ContentDigest)
	case ports.MeasurementInvalidationTargetMissing:
		return InvalidateActualMeasurementResult{outcome: MeasurementInvalidationTargetMissing}, nil
	case ports.MeasurementInvalidationAlreadyVoid:
		return InvalidateActualMeasurementResult{outcome: MeasurementInvalidationAlreadyVoid}, nil
	default:
		return InvalidateActualMeasurementResult{}, errUnnamedMeasurementSave
	}
}

// targetOutsideNode 读被作废的那条测量、比它登记的节点：不在册答目标不在册，在别的节点答目标在别的节点，refused 为真；
// 在 node 上登记的不拒。测量落下不改，节点也不会变，读在写之前不会错过并发。目标不在册这一格与写口的同名答案是同一个结果，
// 这里先答只为让它排在节点判断之前。
func (handler *RecordActualMeasurementHandler) targetOutsideNode(
	ctx context.Context,
	tenant domain.TenantID,
	targetSourceID string,
	node domain.NodeReference,
) (outcome InvalidateActualMeasurementOutcome, refused bool, err error) {
	target, found, err := handler.measurements.FindMeasurement(ctx, ports.ActualMeasurementKey{TenantID: tenant, SourceID: targetSourceID})
	if err != nil {
		return InvalidateActualMeasurementOutcomeInvalid, false, err
	}
	if !found {
		return MeasurementInvalidationTargetMissing, true, nil
	}
	if target.Measurement.Node().String() != node.String() {
		return MeasurementInvalidationTargetAtAnotherNode, true, nil
	}
	return InvalidateActualMeasurementOutcomeInvalid, false, nil
}

func (handler *RecordActualMeasurementHandler) replayInvalidation(
	ctx context.Context,
	key ports.ActualMeasurementKey,
	digest string,
) (InvalidateActualMeasurementResult, error) {
	existing, found, err := handler.measurements.FindInvalidation(ctx, key)
	if err != nil {
		return InvalidateActualMeasurementResult{}, err
	}
	if !found {
		return InvalidateActualMeasurementResult{}, errMeasurementReplayMissing
	}
	if existing.ContentDigest != digest {
		return InvalidateActualMeasurementResult{outcome: MeasurementInvalidationSourceConflict}, nil
	}
	if err := handler.noticeInvalidation(ctx, key.TenantID, existing.Invalidation); err != nil {
		return InvalidateActualMeasurementResult{}, err
	}
	return InvalidateActualMeasurementResult{
		outcome: MeasurementInvalidationExisting, invalidation: existing.Invalidation, present: true,
	}, nil
}

func (handler *RecordActualMeasurementHandler) noticeInvalidation(
	ctx context.Context,
	tenant domain.TenantID,
	invalidation domain.MeasurementInvalidation,
) error {
	target, found, err := handler.measurements.FindMeasurement(ctx, ports.ActualMeasurementKey{
		TenantID: tenant, SourceID: invalidation.TargetSourceID(),
	})
	if err != nil {
		return err
	}
	if !found {
		return errMeasurementReplayMissing
	}
	return handler.notice.NoticeMeasurementChanged(ctx, ports.MeasurementChanged{
		Tenant:     tenant,
		SourceID:   invalidation.Source().SourceID(),
		ObjectKind: target.Measurement.ObjectKind(),
		ObjectID:   target.Measurement.ObjectID(),
		OccurredAt: invalidation.Source().OccurredAt(),
	})
}

func invalidationFromCommand(command InvalidateActualMeasurementCommand) (domain.MeasurementInvalidation, error) {
	if _, err := domain.NewTenantID(command.TenantID); err != nil {
		return domain.MeasurementInvalidation{}, err
	}
	source, err := workFactSource(command.SourceID, command.PerformedBy, command.Evidence, command.OccurredAt)
	if err != nil {
		return domain.MeasurementInvalidation{}, err
	}
	spec := domain.MeasurementInvalidationSpec{
		Source: source, TargetSourceID: command.TargetSourceID, Reason: command.Reason,
		EnteredBy: strings.TrimSpace(command.EnteredBy),
	}
	if strings.TrimSpace(command.OriginalDocument) != "" {
		spec.OriginalDocument, err = domain.NewMeasurementOriginalDocument(command.OriginalDocument)
		if err != nil {
			return domain.MeasurementInvalidation{}, err
		}
	}
	return domain.NewMeasurementInvalidation(spec)
}
