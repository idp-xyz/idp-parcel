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

// Record 登记一条实际移动事实：受理（八件由 RecordMovementFact 构造门把门，门禁那一格单独
// 分出来）→ 幂等按（租户 + 事实 + 版本）分重放 → 登记。
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
	if errors.Is(err, domain.ErrDepartureGateBlocked) {
		return RecordMovementFactResult{outcome: MovementFactGateBlocked}, nil
	}
	if err != nil {
		return RecordMovementFactResult{outcome: MovementFactNotAccepted}, nil
	}

	existing, found, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil {
		return movementFactUndecided(command), nil
	}
	if found {
		// 同一版本重投：交回原版本，不顶替。改内容要换版本号，那是更正不是重投。
		return RecordMovementFactResult{
			outcome:   MovementFactVersionExists,
			record:    existing,
			hasRecord: true,
		}, nil
	}

	record := ports.MovementFactRecord{Key: key, Fact: fact, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return movementFactUndecided(command), nil
	}
	if saved == ports.MovementFactVersionAlreadyRegistered {
		// 并发下另一方先登记：读回赢家而不是宣称自己记下了它。
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return movementFactUndecided(command), nil
		}
		return RecordMovementFactResult{
			outcome:   MovementFactVersionExists,
			record:    winner,
			hasRecord: true,
		}, nil
	}
	return RecordMovementFactResult{outcome: MovementFactRecorded, record: record, hasRecord: true}, nil
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
