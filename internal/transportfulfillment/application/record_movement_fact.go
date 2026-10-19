package application

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// MovementFactOutcome 是登记一条实际移动事实的结果代数。
//
// **`门禁未放行`与`输入未受理`分开，因为续办动作相反**：前者要去取放行结果（门禁判断本身属
// customs-compliance），后者要去改输入。并成一格会让调用方读不出该做哪一件。
type MovementFactOutcome uint8

const (
	MovementFactOutcomeInvalid MovementFactOutcome = iota
	MovementFactRecorded
	MovementFactVersionExists
	MovementFactSourceConflict
	MovementFactGateBlocked
	MovementFactNotAccepted
	MovementFactUndecided
)

func (outcome MovementFactOutcome) String() string {
	switch outcome {
	case MovementFactRecorded:
		return "MOVEMENT_FACT_RECORDED"
	case MovementFactVersionExists:
		return "MOVEMENT_FACT_VERSION_EXISTS"
	case MovementFactSourceConflict:
		return "SOURCE_CONFLICT"
	case MovementFactGateBlocked:
		return "DEPARTURE_GATE_BLOCKED"
	case MovementFactNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	case MovementFactUndecided:
		return "MOVEMENT_FACT_UNDECIDED"
	default:
		return ""
	}
}

// RecordMovementFactCommand 携带登记一条实际移动事实的全部输入。
//
// GateRequired 只对出发有意义：门禁约束的是装载出发动作，移动与到达不受它管。把放行依据挂到
// 后两者上等于造了一个不存在的门，领域构造门会拒。
type RecordMovementFactCommand struct {
	TenantID      domain.TenantID
	Fact          string
	Schedule      string
	Kind          domain.MovementFactKind
	Location      string
	Source        string
	Version       string
	OccurredAt    time.Time
	GateRequired  bool
	GateClearance string
}

type RecordMovementFactResult struct {
	outcome      MovementFactOutcome
	record       ports.MovementFactRecord
	hasRecord    bool
	continuation string
}

func (result RecordMovementFactResult) Outcome() MovementFactOutcome {
	return result.outcome
}

func (result RecordMovementFactResult) Record() (ports.MovementFactRecord, bool) {
	return result.record, result.hasRecord
}

// ContinuationReference 只在`未决`时非空，供调用方续办同一次登记。
func (result RecordMovementFactResult) ContinuationReference() string {
	return result.continuation
}

type RecordMovementFactDeps struct {
	Facts ports.MovementFactRegistry
	Clock ports.Clock
}

type RecordMovementFactHandler struct {
	deps RecordMovementFactDeps
}

func NewRecordMovementFactHandler(deps RecordMovementFactDeps) *RecordMovementFactHandler {
	return &RecordMovementFactHandler{deps: deps}
}

// Record 登记一条实际移动事实：受理（八件由 RecordMovementFact 构造门把门，门禁那一格单独分出来）→ 按（租户 + 事实 + 版本）
// 查册，在册则按内容分重放与 SOURCE_CONFLICT → 登记。
//
// **身份在册时先比内容、后判门禁**（票 tf-face-algebra/08 的裁定）：在册那一版早已成立，同身份再到只问内容同不同；先判门禁会把
// 一条只多声称「要门禁」的重放支去取放行。所以门禁挡着时也先查册，查不了册就答未决——此刻判断不了它是不是重放。
//
// **移动是段内的事实，不是段的成立或结束依据。** CONTEXT 逐项列举过扫描、车辆到场、物理装载
// 都不能替代控制边界，所以这里既不读段也不写段——两者的关系是引用不是状态机。
func (handler *RecordMovementFactHandler) Record(
	ctx context.Context,
	command RecordMovementFactCommand,
) (RecordMovementFactResult, error) {
	spec, key, accepted := movementFactSpecFrom(command)
	if !accepted {
		return RecordMovementFactResult{outcome: MovementFactNotAccepted}, nil
	}
	fact, err := domain.RecordMovementFact(spec)
	gateBlocked := errors.Is(err, domain.ErrDepartureGateBlocked)
	if err != nil && !gateBlocked {
		return RecordMovementFactResult{outcome: MovementFactNotAccepted}, nil
	}
	_, requested, err := domain.CanonicalizeMovementFactPayload(spec.Schedule, spec.Kind, spec.Location, spec.GateClearance)
	if err != nil {
		return RecordMovementFactResult{}, err
	}

	existing, found, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil {
		return movementFactUndecided(command), nil
	}
	if found {
		return movementFactReplayOrConflict(existing, requested)
	}
	if gateBlocked {
		return RecordMovementFactResult{outcome: MovementFactGateBlocked}, nil
	}

	record := ports.MovementFactRecord{Key: key, Fact: fact, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return movementFactUndecided(command), nil
	}
	if saved == ports.MovementFactVersionAlreadyRegistered {
		// 并发下另一方先登记：读回赢家而不是宣称自己记下了它，再照在册那一支比内容——输家带的可能是另一份作业。
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return movementFactUndecided(command), nil
		}
		return movementFactReplayOrConflict(winner, requested)
	}
	return RecordMovementFactResult{outcome: MovementFactRecorded, record: record, hasRecord: true}, nil
}

// movementFactReplayOrConflict 按 TFC-1 形状比在册版本与本次命令。册上不存摘要，在册一侧从本体现算，照票 operator-channel/20
// 承运商判断口的做法，不加摘要列、不动迁移。
//
// 冲突不带记录：身份键不含设备与作业范围，在册那一份可能是另一作业范围登的，回显就把它的内容交给了这一方。作业事实族的调用方
// 是离线队列、2xx 即出队（ADR-0022），答成已有版本会让这份异内容被静默吞掉（ADR-0023 后果节）。
func movementFactReplayOrConflict(current ports.MovementFactRecord, requested string) (RecordMovementFactResult, error) {
	clearance, _ := current.Fact.GateClearance()
	_, recorded, err := domain.CanonicalizeMovementFactPayload(current.Fact.Schedule(), current.Fact.Kind(), current.Fact.Location(), clearance)
	if err != nil {
		return RecordMovementFactResult{}, err
	}
	if recorded != requested {
		return RecordMovementFactResult{outcome: MovementFactSourceConflict}, nil
	}
	return RecordMovementFactResult{outcome: MovementFactVersionExists, record: current, hasRecord: true}, nil
}

// movementFactSpecFrom 逐件过构造器。**门禁放行引用是唯一可缺席的一件**——CONTEXT 说门禁出发
// 可阻断，但不是每次移动都经门禁；空串在这里读作「没有门禁依据」，交给领域按种类与 GateRequired
// 去判它该不该在场，本函数不重判一遍。
func movementFactSpecFrom(
	command RecordMovementFactCommand,
) (domain.MovementFactSpec, ports.MovementFactKey, bool) {
	none := ports.MovementFactKey{}
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return domain.MovementFactSpec{}, none, false
	}
	fact, err := domain.NewMovementFactReference(command.Fact)
	if err != nil {
		return domain.MovementFactSpec{}, none, false
	}
	schedule, err := domain.NewScheduleReference(command.Schedule)
	if err != nil {
		return domain.MovementFactSpec{}, none, false
	}
	location, err := domain.NewMovementLocationReference(command.Location)
	if err != nil {
		return domain.MovementFactSpec{}, none, false
	}
	source, err := domain.NewMovementSourceReference(command.Source)
	if err != nil {
		return domain.MovementFactSpec{}, none, false
	}
	version, err := domain.NewMovementFactVersion(command.Version)
	if err != nil {
		return domain.MovementFactSpec{}, none, false
	}
	spec := domain.MovementFactSpec{
		TenantID:     command.TenantID,
		Fact:         fact,
		Schedule:     schedule,
		Kind:         command.Kind,
		Location:     location,
		Source:       source,
		Version:      version,
		OccurredAt:   command.OccurredAt,
		GateRequired: command.GateRequired,
	}
	if strings.TrimSpace(command.GateClearance) != "" {
		if spec.GateClearance, err = domain.NewGateClearanceReference(command.GateClearance); err != nil {
			return domain.MovementFactSpec{}, none, false
		}
	}
	return spec, ports.MovementFactKey{TenantID: command.TenantID, Fact: fact, Version: version}, true
}

func movementFactUndecided(command RecordMovementFactCommand) RecordMovementFactResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		"MOVEMENT_FACT_REGISTRY_UNAVAILABLE",
		command.TenantID.String(),
		command.Fact,
		command.Version,
	}, "\x00")))
	return RecordMovementFactResult{
		outcome:      MovementFactUndecided,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
