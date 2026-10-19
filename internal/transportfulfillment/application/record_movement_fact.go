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
// 没有「门禁未放行」一格：出发一律落册，门禁读数是随事实保存的一格，不是登记的答复（ADR-0205 决定一）。拒收一条已经
// 发生的出发不会让车回来，只会让离线队列把它出队丢掉。
type MovementFactOutcome uint8

const (
	MovementFactOutcomeInvalid MovementFactOutcome = iota
	MovementFactRecorded
	MovementFactVersionExists
	MovementFactSourceConflict
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
// Segment、Resource 是登记方声明的段与资源，可缺（ADR-0205 决定四）。GateClearance 是设备出示的放行凭据，只作证据原样
// 保存，挂到移动或到达上领域构造门会拒。GateRequired 收而不读（ADR-0205 决定三）：设门与否由关务按对象与起讫判，设备
// 不能凭自报把门关掉；字段暂留，是因为一线入口仍按原形透传它，删它要随那一侧一起改。
type RecordMovementFactCommand struct {
	TenantID      domain.TenantID
	Fact          string
	Schedule      string
	Kind          domain.MovementFactKind
	Location      string
	Source        string
	Version       string
	OccurredAt    time.Time
	Segment       string
	Resource      string
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

// Record 登记一条实际移动事实：受理（各件由 RecordMovementFact 构造门把门）→ 按（租户 + 事实 + 版本）查册，在册则按内容分
// 重放与 SOURCE_CONFLICT → 登记。出发连同 reading 同一笔落册。
//
// reading 是组合根在写事务之前用 DepartureGateReader 问好的门禁读数，只属出发：移动与到达交零值。重放与冲突用不上它——
// 在册那一版早已带着它首次登记时问来的读数，同身份再到只比内容（票 tf-face-algebra/08），不另记一份读数。
//
// **移动是段内的事实，不是段的成立或结束依据。** CONTEXT 逐项列举过扫描、车辆到场、物理装载都不能替代控制边界，所以这里
// 既不写段也不核段在不在册——段是登记方声明的引用，不是状态机（ADR-0205 决定四）；读段只发生在读数那一侧，为的是知道问
// 关务哪几件。
func (handler *RecordMovementFactHandler) Record(
	ctx context.Context,
	command RecordMovementFactCommand,
	reading domain.DepartureGateReading,
) (RecordMovementFactResult, error) {
	spec, key, accepted := movementFactSpecFrom(command)
	if !accepted {
		return RecordMovementFactResult{outcome: MovementFactNotAccepted}, nil
	}
	spec.GateReading = reading
	fact, err := domain.RecordMovementFact(spec)
	if errors.Is(err, domain.ErrDepartureGateReadingMisplaced) {
		// 构造门先判本体、后判读数：本体不成形的那一条仍答未受理；走到这里说明本体成形而读数不在该在的地方——
		// 组合根没在写事务之前问，或把读数交给了移动与到达。那是接线错误，上抛，不答一格。
		return RecordMovementFactResult{}, err
	}
	if err != nil {
		return RecordMovementFactResult{outcome: MovementFactNotAccepted}, nil
	}
	_, requested, err := canonicalMovementFact(fact)
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

func canonicalMovementFact(fact domain.TransportMovementFact) ([]byte, string, error) {
	segment, _ := fact.Segment()
	resource, _ := fact.Resource()
	clearance, _ := fact.GateClearance()
	return domain.CanonicalizeMovementFactPayload(fact.Schedule(), fact.Kind(), fact.Location(), segment, resource, clearance)
}

// movementFactReplayOrConflict 按 TFC-1 形状比在册版本与本次命令。册上不存摘要，在册一侧从本体现算，照票 operator-channel/20
// 承运商判断口的做法，不加摘要列。
//
// 冲突不带记录：身份键不含设备与作业范围，在册那一份可能是另一作业范围登的，回显就把它的内容交给了这一方。作业事实族的调用方
// 是离线队列、2xx 即出队（ADR-0022），答成已有版本会让这份异内容被静默吞掉（ADR-0023 后果节）。
func movementFactReplayOrConflict(current ports.MovementFactRecord, requested string) (RecordMovementFactResult, error) {
	_, recorded, err := canonicalMovementFact(current.Fact)
	if err != nil {
		return RecordMovementFactResult{}, err
	}
	if recorded != requested {
		return RecordMovementFactResult{outcome: MovementFactSourceConflict}, nil
	}
	return RecordMovementFactResult{outcome: MovementFactVersionExists, record: current, hasRecord: true}, nil
}

// movementFactSpecFrom 逐件过构造器。**段、资源与放行凭据是可缺的三件**：段可以先于事实登记、也可以不声明（ADR-0205
// 决定四），资源同理，放行凭据只是设备出示的证据。空串在这里读作「没有声明」；门禁读数不从命令来，由编排另行交入。
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
		TenantID:   command.TenantID,
		Fact:       fact,
		Schedule:   schedule,
		Kind:       command.Kind,
		Location:   location,
		Source:     source,
		Version:    version,
		OccurredAt: command.OccurredAt,
	}
	if strings.TrimSpace(command.Segment) != "" {
		if spec.Segment, err = domain.NewFulfillmentSegmentReference(command.Segment); err != nil {
			return domain.MovementFactSpec{}, none, false
		}
	}
	if strings.TrimSpace(command.Resource) != "" {
		if spec.Resource, err = domain.NewTransportResourceReference(command.Resource); err != nil {
			return domain.MovementFactSpec{}, none, false
		}
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
