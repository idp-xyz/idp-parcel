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
// 保存，挂到移动或到达上领域构造门会拒。
//
// 命令不带设备自报的「要不要门禁」：设门与否由关务按对象与起讫判，设备不能凭自报把门关掉（ADR-0205 决定三）。收而不读的
// 一格迟早会被人读，所以不留；线格式上那个键退役在适配器一层（tfhttp.MovementFactPayload），照收照弃、到不了这里。
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

// RecordMovementFactDeps 的 Handoff 交移动事实已形成信封（ADR-0205 决定五），与事实同一笔入队。
type RecordMovementFactDeps struct {
	Facts   ports.MovementFactRegistry
	Handoff ports.MovementFactHandoff
	Clock   ports.Clock
}

type RecordMovementFactHandler struct {
	deps RecordMovementFactDeps
}

func NewRecordMovementFactHandler(deps RecordMovementFactDeps) *RecordMovementFactHandler {
	return &RecordMovementFactHandler{deps: deps}
}

// Record 登记一条实际移动事实：受理（各件由 RecordMovementFact 构造门把门）→ 按（租户 + 事实 + 版本）查册，在册则按内容分
// 重放与 SOURCE_CONFLICT → 登记 → 交已形成信封。出发连同 reading 同一笔落册，信封也在这一笔里入队。
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
		return movementFactUndecided(command, movementFactRegistryUnavailable), nil
	}
	if found {
		return handler.replayOrConflict(ctx, command, existing, requested)
	}

	record := ports.MovementFactRecord{Key: key, Fact: fact, RecordedAt: handler.deps.Clock.Now()}
	saved, err := handler.deps.Facts.Save(ctx, record)
	if err != nil {
		return movementFactUndecided(command, movementFactRegistryUnavailable), nil
	}
	if saved == ports.MovementFactVersionAlreadyRegistered {
		// 并发下另一方先登记：读回赢家而不是宣称自己记下了它，再照在册那一支比内容——输家带的可能是另一份作业。
		winner, found, err := handler.deps.Facts.FindByKey(ctx, key)
		if err != nil || !found {
			return movementFactUndecided(command, movementFactRegistryUnavailable), nil
		}
		return handler.replayOrConflict(ctx, command, winner, requested)
	}
	if err := handler.deps.Handoff.HandOffMovementFact(ctx, ports.MovementFactIntent{Record: record}); err != nil {
		// 答未决而不上抛，与登记册读不动那几格同一口径：2xx 出队，`未决`连同续办引用呈现给操作员（ADR-0022），同一份再报
		// 一次走在册那一支、重交同一份意图，把这一次没落成的补上。真库上入队失败是 SQL 级错误，整笔已进中止态，装配点提交
		// 失败随之上抛、事实一并回滚，离线队列按非 2xx 重发——这一格在那里走不到；答得出未决的只有没把事务带进中止态的交出失败。
		return movementFactUndecided(command, movementFactHandoffUnavailable), nil
	}
	return RecordMovementFactResult{outcome: MovementFactRecorded, record: record, hasRecord: true}, nil
}

// RecordedDepartureReading 答这条命令是不是一条在册出发的同内容重放；是则交回在册那一版首次登记时问来的读数。
//
// 组合根在问读数之前先问它：重放终归回显首次读数（ADR-0205 决定六），重问关务只是为每件对象多做一笔判断——离线队列
// 重投一条大段的出发，每次都要逐件串行问一遍，重投越慢越容易超时，超时又招来更多重投。查不了册、不在册、不是出发、
// 内容不同或受理不了都答 false：照常问读数，答复仍由 Record 在写事务里按在册那一版定。交回的读数只会被重放那一支
// 丢弃——在册即不再落一版（册只插不改），所以它不会被当成新读数存下。
func (handler *RecordMovementFactHandler) RecordedDepartureReading(
	ctx context.Context,
	command RecordMovementFactCommand,
) (domain.DepartureGateReading, bool) {
	spec, key, accepted := movementFactSpecFrom(command)
	if !accepted || spec.Kind != domain.DepartureFact {
		return domain.DepartureGateReading{}, false
	}
	existing, found, err := handler.deps.Facts.FindByKey(ctx, key)
	if err != nil || !found {
		return domain.DepartureGateReading{}, false
	}
	reading, read := existing.Fact.GateReading()
	if !read {
		return domain.DepartureGateReading{}, false
	}
	_, recorded, err := canonicalMovementFact(existing.Fact)
	if err != nil {
		return domain.DepartureGateReading{}, false
	}
	_, requested, err := domain.CanonicalizeMovementFactPayload(
		spec.Schedule, spec.Kind, spec.Location, spec.Segment, spec.Resource, spec.GateClearance)
	if err != nil || recorded != requested {
		return domain.DepartureGateReading{}, false
	}
	return reading, true
}

func canonicalMovementFact(fact domain.TransportMovementFact) ([]byte, string, error) {
	segment, _ := fact.Segment()
	resource, _ := fact.Resource()
	clearance, _ := fact.GateClearance()
	return domain.CanonicalizeMovementFactPayload(fact.Schedule(), fact.Kind(), fact.Location(), segment, resource, clearance)
}

// replayOrConflict 按 TFC-1 形状比在册版本与本次命令。册上不存摘要，在册一侧从本体现算，照票 operator-channel/20 承运商判断口
// 的做法，不加摘要列。
//
// 冲突不带记录：身份键不含设备与作业范围，在册那一份可能是另一作业范围登的，回显就把它的内容交给了这一方。作业事实族的调用方
// 是离线队列、2xx 即出队（ADR-0022），答成已有版本会让这份异内容被静默吞掉（ADR-0023 后果节）。
//
// 已有版本那一格重交同一份意图：意图按信封 ID 认领（ADR-0043），已入队的那一封不会多出第二封；先前若有一次交出没落成，或这
// 一版落册早于信封上线，这一次把它补上。冲突那一格不交：异内容的这一份没有落册，在册那一份的信封归它自己那一次登记。
func (handler *RecordMovementFactHandler) replayOrConflict(
	ctx context.Context,
	command RecordMovementFactCommand,
	current ports.MovementFactRecord,
	requested string,
) (RecordMovementFactResult, error) {
	_, recorded, err := canonicalMovementFact(current.Fact)
	if err != nil {
		return RecordMovementFactResult{}, err
	}
	if recorded != requested {
		return RecordMovementFactResult{outcome: MovementFactSourceConflict}, nil
	}
	if err := handler.deps.Handoff.HandOffMovementFact(ctx, ports.MovementFactIntent{Record: current}); err != nil {
		return movementFactUndecided(command, movementFactHandoffUnavailable), nil
	}
	return RecordMovementFactResult{outcome: MovementFactVersionExists, record: current, hasRecord: true}, nil
}

// movementFactSpecFrom 逐件过构造器。**段、资源与放行凭据是可缺的三件**：段可以先于事实登记、也可以不声明（ADR-0205
// 决定四），资源同理，放行凭据只是设备出示的证据。空串在这里读作「没有声明」；门禁读数不从命令来，由编排另行交入。
func movementFactSpecFrom(
	command RecordMovementFactCommand,
) (domain.MovementFactSpec, ports.MovementFactKey, bool) {
	none := ports.MovementFactKey{}
	schedule, segment, accepted := gateReferencesFrom(command.TenantID, command.Schedule, command.Segment)
	if !accepted {
		return domain.MovementFactSpec{}, none, false
	}
	fact, err := domain.NewMovementFactReference(command.Fact)
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
		Segment:    segment,
		Kind:       command.Kind,
		Location:   location,
		Source:     source,
		Version:    version,
		OccurredAt: command.OccurredAt,
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

// 未决停在哪一口。续办引用按它分开：同一口重投引用不变，引用变了说明这一份已越过登记册、停在了交出。引用是不透明摘要、结果
// 也不带原因码，单看一个引用分不出是哪一口；要直接分辨，得照 RegisterChargeOccurrenceResult 那样在结果里带出原因，线格式随之加一格。
const (
	movementFactRegistryUnavailable = "MOVEMENT_FACT_REGISTRY_UNAVAILABLE"
	movementFactHandoffUnavailable  = "MOVEMENT_FACT_HANDOFF_UNAVAILABLE"
)

func movementFactUndecided(command RecordMovementFactCommand, cause string) RecordMovementFactResult {
	digest := sha256.Sum256([]byte(strings.Join([]string{
		cause,
		command.TenantID.String(),
		command.Fact,
		command.Version,
	}, "\x00")))
	return RecordMovementFactResult{
		outcome:      MovementFactUndecided,
		continuation: "CONT-" + hex.EncodeToString(digest[:8]),
	}
}
