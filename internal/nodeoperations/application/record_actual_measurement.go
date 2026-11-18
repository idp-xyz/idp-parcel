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
	spec := domain.ActualMeasurementSpec{
		Source: source, ObjectKind: objectKind, ObjectID: command.ObjectID,
		Node: node, Location: location, Basis: basis,
		WeightValue: command.WeightValue, WeightUnit: weightUnit,
		Length: command.Length, Width: command.Width, Height: command.Height, DimensionUnit: dimensionUnit,
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
	case InvalidateActualMeasurementOutcomeInvalid:
		return ""
	default:
		return ""
	}
}

// InvalidateActualMeasurementCommand 指回被作废那条的来源身份。它不带重量或尺寸：
// 作废不复制被作废的数字。
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

// Invalidate 写下一条作废。同一条测量上再来一条不同来源的作废答已经作废，不覆盖先到的那条。
func (handler *RecordActualMeasurementHandler) Invalidate(
	ctx context.Context,
	command InvalidateActualMeasurementCommand,
) (InvalidateActualMeasurementResult, error) {
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
