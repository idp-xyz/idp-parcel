package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// RePrepareOnMeasurementChangeCommand 是实测变化入口的命令：只问这个对象此刻的当前有效实测，不按事件里点名的那一条。
type RePrepareOnMeasurementChangeCommand struct {
	TenantID domain.TenantID
	Object   string
}

// RePrepareRunOutcome 是实测变化入口的整体答案。未决要重投；其余各格都已经入账，或本就不该写。
type RePrepareRunOutcome uint8

const (
	RePrepareRunOutcomeInvalid RePrepareRunOutcome = iota
	// RePrepareRunAnswered：窗口还开着的申报段都按这条实测作了答。
	RePrepareRunAnswered
	// RePrepareRunNoMeasurement：此刻没有当前有效实测，无可重做。
	RePrepareRunNoMeasurement
	// RePrepareRunMeasurementUndetermined：实测定不下。不退回申报，也不猜一条。
	RePrepareRunMeasurementUndetermined
	// RePrepareRunNoDeclaration：这个对象还没有申报来源的准备结果。初始准备还没跑到，由 Prepare 兜住。
	RePrepareRunNoDeclaration
	// RePrepareRunWindowClosed：申报段的窗口都在这条实测发生之前已经结束，按这次实测重做没有意义。
	RePrepareRunWindowClosed
	RePrepareRunNotAccepted
	RePrepareRunUndecided
)

func (outcome RePrepareRunOutcome) String() string {
	switch outcome {
	case RePrepareRunAnswered:
		return "MEASURED_PREPARATIONS_ANSWERED"
	case RePrepareRunNoMeasurement:
		return "NO_MEASUREMENT_NOTHING_TO_REPREPARE"
	case RePrepareRunMeasurementUndetermined:
		return "MEASUREMENT_UNDETERMINED"
	case RePrepareRunNoDeclaration:
		return "NO_DECLARATION_PREPARATION"
	case RePrepareRunWindowClosed:
		return "WINDOW_ALREADY_CLOSED"
	case RePrepareRunNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case RePrepareRunUndecided:
		return "PREPARATION_UNDECIDED"
	default:
		return ""
	}
}

// RePrepareOnMeasurementChangeResult 是实测变化入口的一次答复。
type RePrepareOnMeasurementChangeResult struct {
	outcome      RePrepareRunOutcome
	reason       PreparationUndecidedReason
	legs         []LegPreparation
	continuation string
}

func (result RePrepareOnMeasurementChangeResult) Outcome() RePrepareRunOutcome {
	return result.outcome
}

func (result RePrepareOnMeasurementChangeResult) UndecidedReason() PreparationUndecidedReason {
	return result.reason
}

// Legs 按段引用次序交回这次按实测作答的段。零写入的各格为空。
func (result RePrepareOnMeasurementChangeResult) Legs() []LegPreparation {
	return append([]LegPreparation(nil), result.legs...)
}

func (result RePrepareOnMeasurementChangeResult) ContinuationReference() string {
	return result.continuation
}

// RePrepareOnMeasurementChange 在已有的申报准备旁边，按此刻恰一条当前有效实测再判一份。不覆盖申报，也不覆盖按别的实测
// 判过的那一份。比较窗口用的是这条实测自己的发生时刻，不用处理时钟：重投不该改变哪些段被跳过。
func (handler *PrepareOnPlannedLegsHandler) RePrepareOnMeasurementChange(
	ctx context.Context,
	command RePrepareOnMeasurementChangeCommand,
) (RePrepareOnMeasurementChangeResult, error) {
	notAccepted := RePrepareOnMeasurementChangeResult{outcome: RePrepareRunNotAccepted}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return notAccepted, nil
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return notAccepted, nil
	}
	// 锁在读实测与列申报段之前：申报行还没提交时列到的零段不能入账，否则这一次实测补不回来。
	if err := handler.deps.Preparations.HoldPreparationObject(ctx, command.TenantID, object); err != nil {
		return reprepareUndecided(preparationUndecidedPointer(TransportPreparationStoreUnavailable, command.TenantID.String(), object.String())), nil
	}
	reading, stopped, err := handler.readMeasuredDemand(ctx, command.TenantID, object)
	if err != nil {
		return RePrepareOnMeasurementChangeResult{}, err
	}
	if stopped != nil {
		return reprepareUndecided(stopped), nil
	}
	switch reading.Answer {
	case ports.MeasuredDemandNone:
		return RePrepareOnMeasurementChangeResult{outcome: RePrepareRunNoMeasurement}, nil
	case ports.MeasuredDemandUndetermined:
		return RePrepareOnMeasurementChangeResult{outcome: RePrepareRunMeasurementUndetermined}, nil
	case ports.MeasuredDemandOne:
	default:
		return RePrepareOnMeasurementChangeResult{}, fmt.Errorf(
			"transport fulfillment: measured demand answered %q", reading.Answer)
	}
	if err := measuredSnapshotUsable(reading.Snapshot); err != nil {
		return RePrepareOnMeasurementChangeResult{}, err
	}
	rule := domain.BuiltInPreparationRuleVersion()
	listed, err := handler.deps.Preparations.ListDeclaredPreparationLegs(ctx, command.TenantID, object, rule)
	if errors.Is(err, ports.ErrTransportPreparationRecordUnreadable) {
		return RePrepareOnMeasurementChangeResult{}, err
	}
	if err != nil {
		return reprepareUndecided(preparationUndecidedPointer(TransportPreparationStoreUnavailable, command.TenantID.String(), object.String())), nil
	}
	if len(listed) == 0 {
		return RePrepareOnMeasurementChangeResult{outcome: RePrepareRunNoDeclaration}, nil
	}
	open := legsStillOpen(listed, reading.OccurredAt)
	if len(open) == 0 {
		return RePrepareOnMeasurementChangeResult{outcome: RePrepareRunWindowClosed}, nil
	}
	answered, stopped, err := handler.judgeMeasuredLegs(ctx, command.TenantID, object, rule, reading.Snapshot, open)
	if err != nil {
		return RePrepareOnMeasurementChangeResult{}, err
	}
	if stopped != nil {
		return reprepareUndecided(stopped), nil
	}
	return RePrepareOnMeasurementChangeResult{outcome: RePrepareRunAnswered, legs: answered}, nil
}

// supplementMeasured 是初始准备在申报各段判完之后走的那一遍。没有实测或定不下时什么都不补；读不动把整次变成未决。
func (handler *PrepareOnPlannedLegsHandler) supplementMeasured(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	rule domain.PreparationRuleVersion,
	legs []domain.PreparationLeg,
) ([]LegPreparation, *PrepareOnPlannedLegsResult, error) {
	reading, stopped, err := handler.readMeasuredDemand(ctx, tenant, object)
	if err != nil || stopped != nil {
		return nil, stopped, err
	}
	switch reading.Answer {
	case ports.MeasuredDemandNone, ports.MeasuredDemandUndetermined:
		return nil, nil, nil
	case ports.MeasuredDemandOne:
	default:
		return nil, nil, fmt.Errorf("transport fulfillment: measured demand answered %q", reading.Answer)
	}
	if err := measuredSnapshotUsable(reading.Snapshot); err != nil {
		return nil, nil, err
	}
	return handler.judgeMeasuredLegs(ctx, tenant, object, rule, reading.Snapshot, legsStillOpen(legs, reading.OccurredAt))
}

func (handler *PrepareOnPlannedLegsHandler) readMeasuredDemand(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
) (ports.MeasuredDemandReading, *PrepareOnPlannedLegsResult, error) {
	if handler.deps.Measured == nil {
		return ports.MeasuredDemandReading{}, nil, fmt.Errorf("%w", ErrMeasuredDemandSourceUnwired)
	}
	reading, err := handler.deps.Measured.LoadMeasuredDemand(ctx, tenant, object)
	if errors.Is(err, ports.ErrMeasuredDemandUntranslatable) {
		return ports.MeasuredDemandReading{}, nil, err
	}
	if err != nil {
		return ports.MeasuredDemandReading{}, preparationUndecidedPointer(MeasuredDemandUnavailable, tenant.String(), object.String()), nil
	}
	return reading, nil, nil
}

func (handler *PrepareOnPlannedLegsHandler) judgeMeasuredLegs(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	rule domain.PreparationRuleVersion,
	demand domain.DemandSnapshot,
	legs []domain.PreparationLeg,
) ([]LegPreparation, *PrepareOnPlannedLegsResult, error) {
	answered := make([]LegPreparation, 0, len(legs))
	for _, leg := range legs {
		one, stopped, err := handler.prepareLeg(ctx, tenant, object, rule, demand, leg)
		if err != nil {
			return nil, nil, err
		}
		if stopped != nil {
			return nil, stopped, nil
		}
		answered = append(answered, one)
	}
	return answered, nil, nil
}

// legsStillOpen 留下窗口结束时刻不早于实测发生时刻的段。等于发生时刻的那段还没过完，按这次实测重做仍然有意义。
func legsStillOpen(legs []domain.PreparationLeg, occurredAt time.Time) []domain.PreparationLeg {
	open := make([]domain.PreparationLeg, 0, len(legs))
	for _, leg := range legs {
		if leg.WindowEnd().Before(occurredAt) {
			continue
		}
		open = append(open, leg)
	}
	return open
}

func measuredSnapshotUsable(snapshot domain.DemandSnapshot) error {
	anchor, present := snapshot.Anchor()
	if snapshot.Source() != domain.DemandFromActualMeasurement || !present || strings.TrimSpace(anchor) == "" {
		return fmt.Errorf("%w: the reading is not one measured demand snapshot", ports.ErrMeasuredDemandUntranslatable)
	}
	return nil
}

func preparationKey(
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	rule domain.PreparationRuleVersion,
	leg domain.PreparationLeg,
	demand domain.DemandSnapshot,
) ports.TransportPreparationKey {
	key := ports.TransportPreparationKey{
		TenantID: tenant, Leg: leg.Reference(), Object: object, Rule: rule, Source: demand.Source(),
	}
	if demand.Source() == domain.DemandFromActualMeasurement {
		anchor, _ := demand.Anchor()
		key.MeasurementRef = anchor
	}
	return key
}

func preparationUndecidedPointer(reason PreparationUndecidedReason, subject ...string) *PrepareOnPlannedLegsResult {
	result := preparationUndecided(reason, subject...)
	return &result
}

func reprepareUndecided(stopped *PrepareOnPlannedLegsResult) RePrepareOnMeasurementChangeResult {
	return RePrepareOnMeasurementChangeResult{
		outcome:      RePrepareRunUndecided,
		reason:       stopped.reason,
		continuation: stopped.continuation,
	}
}
