package application

import (
	"context"
	"errors"
	"slices"
	"strings"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件是出发门禁补问（ADR-0305）：出发落册那一刻停在本侧（段不在册、那一刻段里没有在控对象）或某一口读不动，读数里就没有
// 这件对象的定论；日后它凭`已交接`或对象级场外揽收进了段、或又有一份这样的控制事实时，按原出发时点替它再问一次关务，问来的读数
// 追加一版、首版不改。补问读数里的阻断由全程追踪形成信号，业务时间仍是那次出发（信号的因果条件与事实依据都指那条移动事实）。

// MaxDepartureGateReasks 是同一条出发替同一件对象最多补问几次（ADR-0305，产品策略，不是租户取值）。补问只由这件对象自己的控制事实
// 唤醒，次数本就随它往后走的段数有限；上限兜住的是再问也问不出定论的那几格——交接册那一行坏了却与连不上同记（票
// demo-chain-gaps/114）、班次一直不登、关务一直读不动。三次是进段那一份加往后两程：一件对象出发之后通常两程之内就有下一份控制
// 事实，关务读不动的那一阵过去了就该问得到；三次都问不到定论，就不是再等一程能好的事，要人去查。
const MaxDepartureGateReasks = 3

// DepartureGateReaskOutcome 是一份控制事实唤醒的补问这一拍的结果代数。
type DepartureGateReaskOutcome uint8

const (
	DepartureGateReaskOutcomeInvalid DepartureGateReaskOutcome = iota
	// DepartureGateReaskScanned：扫过了这件对象有参与的每一段，该补问的都问过了——可能一件也没有。
	DepartureGateReaskScanned
	// DepartureGateReaskNotHandedOver：交接是拒收或待确认，没让对象进段，连段都不读。
	DepartureGateReaskNotHandedOver
	// DepartureGateReaskUndecided：某一口此刻读不动、写不进，原样重投可能就过；停在哪一口在 Stop 里。
	DepartureGateReaskUndecided
	// DepartureGateReaskDefect：某一行装不回、记录与键不符，或本上下文自己交出的键立不起来——原样重投一万次都一样。
	DepartureGateReaskDefect
)

func (outcome DepartureGateReaskOutcome) String() string {
	switch outcome {
	case DepartureGateReaskScanned:
		return "DEPARTURE_GATES_REASKED"
	case DepartureGateReaskNotHandedOver:
		return "HANDOVER_NOT_HANDED_OVER"
	case DepartureGateReaskUndecided:
		return "DEPARTURE_GATE_REASK_UNDECIDED"
	case DepartureGateReaskDefect:
		return "DEPARTURE_GATE_REASK_DEFECT"
	default:
		return ""
	}
}

// DepartureGateReaskStop 指名`未决`或缺陷停在哪一口。
type DepartureGateReaskStop uint8

const (
	DepartureGateReaskStopNone DepartureGateReaskStop = iota

	// 未决：读不动、写不进
	DepartureGateReaskHandoverRegistryUnavailable
	DepartureGateReaskHandoverNotVisible
	DepartureGateReaskPickupRegistryUnavailable
	DepartureGateReaskPickupNotVisible
	DepartureGateReaskSegmentRegistryUnavailable
	DepartureGateReaskMovementFactRegistryUnavailable
	DepartureGateReaskRegistryUnavailable
	DepartureGateReaskHandoffUnavailable

	// 缺陷：重投不会变
	DepartureGateReaskKeyNotAccepted
	DepartureGateReaskControlFactInconsistent
	DepartureGateReaskSegmentRecordUnreadable
	DepartureGateReaskSegmentInconsistent
	DepartureGateReaskMovementFactRecordUnreadable
	DepartureGateReaskMovementFactInconsistent
	DepartureGateReaskRecordUnreadable
)

func (stop DepartureGateReaskStop) String() string {
	switch stop {
	case DepartureGateReaskHandoverRegistryUnavailable:
		return "HANDOVER_REGISTRY_UNAVAILABLE"
	case DepartureGateReaskHandoverNotVisible:
		return "HANDOVER_NOT_VISIBLE"
	case DepartureGateReaskPickupRegistryUnavailable:
		return "PICKUP_REGISTRY_UNAVAILABLE"
	case DepartureGateReaskPickupNotVisible:
		return "PICKUP_NOT_VISIBLE"
	case DepartureGateReaskSegmentRegistryUnavailable:
		return "SEGMENT_REGISTRY_UNAVAILABLE"
	case DepartureGateReaskMovementFactRegistryUnavailable:
		return "MOVEMENT_FACT_REGISTRY_UNAVAILABLE"
	case DepartureGateReaskRegistryUnavailable:
		return "DEPARTURE_GATE_REASK_REGISTRY_UNAVAILABLE"
	case DepartureGateReaskHandoffUnavailable:
		return "DEPARTURE_GATE_REASK_HANDOFF_UNAVAILABLE"
	case DepartureGateReaskKeyNotAccepted:
		return "CONTROL_FACT_KEY_NOT_ACCEPTED"
	case DepartureGateReaskControlFactInconsistent:
		return "CONTROL_FACT_INCONSISTENT"
	case DepartureGateReaskSegmentRecordUnreadable:
		return "SEGMENT_RECORD_UNREADABLE"
	case DepartureGateReaskSegmentInconsistent:
		return "SEGMENT_INCONSISTENT"
	case DepartureGateReaskMovementFactRecordUnreadable:
		return "MOVEMENT_FACT_RECORD_UNREADABLE"
	case DepartureGateReaskMovementFactInconsistent:
		return "MOVEMENT_FACT_INCONSISTENT"
	case DepartureGateReaskRecordUnreadable:
		return "DEPARTURE_GATE_REASK_RECORD_UNREADABLE"
	default:
		return ""
	}
}

// DepartureGateReaskDisposition 是这一拍的答复对唤醒它的那封控制事实信封意味着什么，判据同 DispatchOnHandoverDisposition：原样
// 重投会变的才重投；不会变的，业务答案入账，缺陷响亮失败而不入账。
type DepartureGateReaskDisposition uint8

const (
	DepartureGateReaskDispositionInvalid DepartureGateReaskDisposition = iota
	DepartureGateReaskSettled
	DepartureGateReaskRetry
	DepartureGateReaskFailed
)

func (disposition DepartureGateReaskDisposition) String() string {
	switch disposition {
	case DepartureGateReaskSettled:
		return "SETTLED"
	case DepartureGateReaskRetry:
		return "RETRY"
	case DepartureGateReaskFailed:
		return "DEFECT"
	default:
		return ""
	}
}

// DepartureGateReaskResult 是一拍的答复：先由 Plan 扫段、问关务，再由 Record 在写事务里把问来的各份追加落册。
type DepartureGateReaskResult struct {
	outcome   DepartureGateReaskOutcome
	stop      DepartureGateReaskStop
	drafts    []ports.DepartureGateReaskRecord
	exhausted int
	recorded  int
}

func (result DepartureGateReaskResult) Outcome() DepartureGateReaskOutcome { return result.outcome }

// Stop 只在`未决`或缺陷时非零。
func (result DepartureGateReaskResult) Stop() DepartureGateReaskStop { return result.stop }

// Drafts 是 Plan 问来、还没落册的各份补问；Record 之后交回真正追加了的那几份。
func (result DepartureGateReaskResult) Drafts() []ports.DepartureGateReaskRecord {
	return append([]ports.DepartureGateReaskRecord(nil), result.drafts...)
}

// Exhausted 是到了次数上限、这一拍没再问的出发有几条。
func (result DepartureGateReaskResult) Exhausted() int { return result.exhausted }

// Recorded 是 Record 真正追加了几份（另一方先补了同一次的不算）。
func (result DepartureGateReaskResult) Recorded() int { return result.recorded }

func (result DepartureGateReaskResult) Disposition() DepartureGateReaskDisposition {
	switch result.outcome {
	case DepartureGateReaskScanned, DepartureGateReaskNotHandedOver:
		return DepartureGateReaskSettled
	case DepartureGateReaskUndecided:
		return DepartureGateReaskRetry
	default:
		return DepartureGateReaskFailed
	}
}

func departureGateReaskStopped(outcome DepartureGateReaskOutcome, stop DepartureGateReaskStop) DepartureGateReaskResult {
	return DepartureGateReaskResult{outcome: outcome, stop: stop}
}

// ReaskOnHandoverCommand 是交接信封上的判断键，四维同交接登记册的键。
type ReaskOnHandoverCommand struct {
	TenantID domain.TenantID
	Object   string
	Scope    string
	Version  string
}

// ReaskOnPickupCommand 是对象级揽收信封上的判断键，三维同揽收登记册的键。
type ReaskOnPickupCommand struct {
	TenantID domain.TenantID
	Object   string
	Attempt  string
}

// OffsitePickupFinder 是揽收登记册按键取当前版的那一口。本编排对揽收只读。
type OffsitePickupFinder interface {
	FindByKey(ctx context.Context, key ports.OffsitePickupKey) (ports.OffsitePickupRecord, bool, error)
}

// ObjectSegmentView 答对象在哪些段里有参与，并按键取回段连同全部参与。段登记册天然满足它。
type ObjectSegmentView interface {
	FindSegmentsForObject(ctx context.Context, tenant domain.TenantID, object domain.CarriedObjectReference) ([]ports.FulfillmentSegmentKey, error)
	FindByKey(ctx context.Context, key ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error)
}

// DepartureGateReasker 按原出发时点替一件对象再问一次，真实装配接 DepartureGateReader.Reask。
type DepartureGateReasker interface {
	Reask(ctx context.Context, fact domain.TransportMovementFact, object domain.CarriedObjectReference) (domain.DepartureGateReading, error)
}

// ReaskDepartureGatesDeps 全部必备。
type ReaskDepartureGatesDeps struct {
	Handovers TransportHandoverFinder
	Pickups   OffsitePickupFinder
	Segments  ObjectSegmentView
	Facts     ports.MovementFactsOfSegment
	Reasks    ports.DepartureGateReaskRegistry
	Gate      DepartureGateReasker
	Handoff   ports.DepartureGateReaskHandoff
	Clock     ports.Clock
}

// ReaskDepartureGatesHandler 是补问的进程内入口：本上下文消费自己的交接与对象级揽收信封，替那件对象把该补问的出发问一遍。
//
// 一拍分两步，因为关务的判断在它自己的事务里记下、在调用方事务里即拒（ADR-0206 决定六）：Plan 在任何写事务之外读段、读事实、
// 问关务；Record 在写事务里把问来的各份追加落册、各交一封信。两步之间另一方先补了同一次，Record 撞键认出、不覆盖、不另交信封。
type ReaskDepartureGatesHandler struct {
	deps ReaskDepartureGatesDeps
}

var errReaskDepartureGatesDepsMissing = errors.New(
	"transport fulfillment: the departure gate reask needs handovers, pickups, segments, facts, reasks, the gate, a handoff and a clock")

// NewReaskDepartureGatesHandler 构造期拒缺件：漏装的一口到第一封信才暴露，会被当成读不动反复重投。
func NewReaskDepartureGatesHandler(deps ReaskDepartureGatesDeps) (*ReaskDepartureGatesHandler, error) {
	if deps.Handovers == nil || deps.Pickups == nil || deps.Segments == nil || deps.Facts == nil || deps.Reasks == nil ||
		deps.Gate == nil || deps.Handoff == nil || deps.Clock == nil {
		return nil, errReaskDepartureGatesDepsMissing
	}
	return &ReaskDepartureGatesHandler{deps: deps}, nil
}

// PlanOnHandover 按键读回那一版交接：只认转出控制的那一版（`已交接`，含更正成`已交接`的那一版）——拒收与待确认不让对象进段。
func (handler *ReaskDepartureGatesHandler) PlanOnHandover(
	ctx context.Context,
	command ReaskOnHandoverCommand,
) (DepartureGateReaskResult, error) {
	key, accepted := transportHandoverKeyOf(command.TenantID, command.Object, command.Scope, command.Version)
	if !accepted {
		return departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskKeyNotAccepted), nil
	}
	record, found, err := handler.deps.Handovers.FindByKey(ctx, key)
	if err != nil {
		return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskHandoverRegistryUnavailable), nil
	}
	if !found {
		return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskHandoverNotVisible), nil
	}
	handover := record.Handover
	if record.Key != key || handover.TenantID() != key.TenantID || handover.Object() != key.Object {
		return departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskControlFactInconsistent), nil
	}
	if !handover.TransfersControl() {
		return DepartureGateReaskResult{outcome: DepartureGateReaskNotHandedOver}, nil
	}
	trigger := "transport-handover/" + key.Object.String() + "/" + key.Scope.String() + "/" + key.Version.String()
	return handler.plan(ctx, key.TenantID, key.Object, trigger)
}

// PlanOnPickup 按键读回对象级揽收的当前版。对象级揽收只为成功收寄形成，没有「不转出控制」的那一格。
func (handler *ReaskDepartureGatesHandler) PlanOnPickup(
	ctx context.Context,
	command ReaskOnPickupCommand,
) (DepartureGateReaskResult, error) {
	key, accepted := offsitePickupKeyOf(command)
	if !accepted {
		return departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskKeyNotAccepted), nil
	}
	record, found, err := handler.deps.Pickups.FindByKey(ctx, key)
	if err != nil {
		return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskPickupRegistryUnavailable), nil
	}
	if !found {
		return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskPickupNotVisible), nil
	}
	pickup := record.Pickup
	if record.Key != key || pickup.TenantID() != key.TenantID || pickup.Object() != key.Object {
		return departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskControlFactInconsistent), nil
	}
	trigger := "offsite-pickup/" + key.Object.String() + "/" + key.Attempt.String()
	return handler.plan(ctx, key.TenantID, key.Object, trigger)
}

func offsitePickupKeyOf(command ReaskOnPickupCommand) (ports.OffsitePickupKey, bool) {
	if strings.TrimSpace(command.TenantID.String()) == "" {
		return ports.OffsitePickupKey{}, false
	}
	object, err := domain.NewCarriedObjectReference(command.Object)
	if err != nil {
		return ports.OffsitePickupKey{}, false
	}
	attempt, err := domain.NewAttemptReference(command.Attempt)
	if err != nil {
		return ports.OffsitePickupKey{}, false
	}
	return ports.OffsitePickupKey{TenantID: command.TenantID, Object: object, Attempt: attempt}, true
}

// plan 扫这件对象有参与的每一段（与全程追踪补派生同一个扫法，ADR-0302）：每段上按段列出声明了它的在册出发，凡出发那一刻这件
// 对象在控、读数里还没有它的定论、补问次数没到上限的，按原出发时点替它问一次。不问对象凭哪一份控制事实进的段：唤醒它的这一份
// 可能正是让它进段的那一份（段日后才登、对象后进段），也可能是它此后的下一份（出发那一刻关务读不动，下一程再问）。
//
// 只看原版事实：更正版沿用原登记那一刻的读数（ADR-0205 越权风险点 8），补问补在原版上，与全程追踪只在原版上形成信号同一口径。
func (handler *ReaskDepartureGatesHandler) plan(
	ctx context.Context,
	tenant domain.TenantID,
	object domain.CarriedObjectReference,
	trigger string,
) (DepartureGateReaskResult, error) {
	keys, err := handler.deps.Segments.FindSegmentsForObject(ctx, tenant, object)
	if err != nil {
		return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskSegmentRegistryUnavailable), nil
	}
	result := DepartureGateReaskResult{outcome: DepartureGateReaskScanned}
	for _, key := range keys {
		segment, stopped := handler.segmentOf(ctx, key, tenant)
		if stopped.outcome != DepartureGateReaskOutcomeInvalid {
			return stopped, nil
		}
		records, err := handler.deps.Facts.FindBySegment(ctx, key)
		if errors.Is(err, ports.ErrMovementFactRecordUnreadable) {
			return departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskMovementFactRecordUnreadable), nil
		}
		if err != nil {
			return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskMovementFactRegistryUnavailable), nil
		}
		for _, record := range records {
			step, err := handler.reaskOne(ctx, key, segment, record, object, trigger)
			if err != nil {
				return DepartureGateReaskResult{}, err
			}
			if step.stopped.outcome != DepartureGateReaskOutcomeInvalid {
				return step.stopped, nil
			}
			if step.exhausted {
				result.exhausted++
			}
			if step.draft != nil {
				result.drafts = append(result.drafts, *step.draft)
			}
		}
	}
	return result, nil
}

// departureGateReaskStep 是 reaskOne 对一条事实的答复：问来的一份、到了次数上限，或停在哪一口；三样都没有即这一条不补问。
type departureGateReaskStep struct {
	draft     *ports.DepartureGateReaskRecord
	exhausted bool
	stopped   DepartureGateReaskResult
}

// segmentOf 按键取回对象有参与的一段。段不能被删除（CONTEXT），按对象列得出的段按键却答没有，是段册自相矛盾。
func (handler *ReaskDepartureGatesHandler) segmentOf(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
	tenant domain.TenantID,
) (domain.ActualFulfillmentSegment, DepartureGateReaskResult) {
	record, found, err := handler.deps.Segments.FindByKey(ctx, key)
	switch {
	case errors.Is(err, ports.ErrFulfillmentSegmentRecordUnreadable):
		return domain.ActualFulfillmentSegment{}, departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskSegmentRecordUnreadable)
	case err != nil:
		return domain.ActualFulfillmentSegment{}, departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskSegmentRegistryUnavailable)
	case !found || key.TenantID != tenant || record.Key != key || record.Segment.Segment() != key.Segment ||
		record.Segment.TenantID() != key.TenantID:
		return domain.ActualFulfillmentSegment{}, departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskSegmentInconsistent)
	}
	return record.Segment, DepartureGateReaskResult{}
}

// reaskOne 判一条按段列出的事实要不要替 object 补问，要就问。
func (handler *ReaskDepartureGatesHandler) reaskOne(
	ctx context.Context,
	key ports.FulfillmentSegmentKey,
	segment domain.ActualFulfillmentSegment,
	record ports.MovementFactRecord,
	object domain.CarriedObjectReference,
	trigger string,
) (departureGateReaskStep, error) {
	fact := record.Fact
	declared, has := fact.Segment()
	if record.Key.TenantID != key.TenantID || fact.TenantID() != record.Key.TenantID || fact.Fact() != record.Key.Fact ||
		fact.Version() != record.Key.Version || !has || declared != key.Segment {
		return departureGateReaskStep{stopped: departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskMovementFactInconsistent)}, nil
	}
	if fact.Kind() != domain.DepartureFact {
		return departureGateReaskStep{}, nil
	}
	if _, corrected := fact.Corrects(); corrected {
		return departureGateReaskStep{}, nil
	}
	first, read := fact.GateReading()
	if !read {
		// 出发必带读数（迁移 0026 的 CHECK 与领域构造门）：没有就是记录坏了。
		return departureGateReaskStep{stopped: departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskMovementFactInconsistent)}, nil
	}
	if !slices.Contains(segment.ObjectsUnderControlAt(fact.OccurredAt()), object) {
		return departureGateReaskStep{}, nil
	}

	all, err := handler.deps.Reasks.FindByDeparture(ctx, record.Key)
	if errors.Is(err, ports.ErrDepartureGateReaskRecordUnreadable) {
		return departureGateReaskStep{stopped: departureGateReaskStopped(DepartureGateReaskDefect, DepartureGateReaskRecordUnreadable)}, nil
	}
	if err != nil {
		return departureGateReaskStep{stopped: departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskRegistryUnavailable)}, nil
	}
	var earlier []domain.DepartureGateReading
	for _, reask := range all {
		if reask.Key.Object == object {
			earlier = append(earlier, reask.Reading)
		}
	}
	if !departureGateReaskDue(first, earlier, object) {
		return departureGateReaskStep{}, nil
	}
	if len(earlier) >= MaxDepartureGateReasks {
		return departureGateReaskStep{exhausted: true}, nil
	}

	reading, err := handler.deps.Gate.Reask(ctx, fact, object)
	if err != nil {
		return departureGateReaskStep{}, err
	}
	return departureGateReaskStep{draft: &ports.DepartureGateReaskRecord{
		Key: ports.DepartureGateReaskKey{
			TenantID: record.Key.TenantID,
			Fact:     record.Key.Fact,
			Version:  record.Key.Version,
			Object:   object,
			Sequence: len(earlier) + 1,
		},
		Reading: reading,
		Trigger: trigger,
	}}, nil
}

// Record 在调用方的写事务里把 Plan 问来的各份追加落册，每份同一笔交一封信。撞键是另一方在两步之间先补了同一次：认它在先，
// 不覆盖、不另交。Plan 没答「扫过了」的那一拍原样交回。
func (handler *ReaskDepartureGatesHandler) Record(
	ctx context.Context,
	planned DepartureGateReaskResult,
) (DepartureGateReaskResult, error) {
	if planned.outcome != DepartureGateReaskScanned {
		return planned, nil
	}
	recorded := DepartureGateReaskResult{outcome: DepartureGateReaskScanned, exhausted: planned.exhausted}
	for _, draft := range planned.drafts {
		draft.RecordedAt = handler.deps.Clock.Now()
		saved, err := handler.deps.Reasks.Save(ctx, draft)
		if err != nil {
			return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskRegistryUnavailable), nil
		}
		if saved == ports.DepartureGateReaskAlreadyRegistered {
			continue
		}
		if err := handler.deps.Handoff.HandOffDepartureGateReask(ctx, draft); err != nil {
			return departureGateReaskStopped(DepartureGateReaskUndecided, DepartureGateReaskHandoffUnavailable), nil
		}
		recorded.drafts = append(recorded.drafts, draft)
		recorded.recorded++
	}
	return recorded, nil
}

// departureGateReaskDue 判这条出发此刻该不该替 object 补问：看它最近那一份读数——补问过就看最近一次补问，没补问过就看首版。
//
//   - 那一份里有它的一格：只有那一格是可再问的读不动才补问。放行、不受门禁、阻断与关务答的未决都是关务给过的定论，按原出发时点再问
//     不会变——阻断已由首版或那一次补问形成过信号；
//   - 那一份停在本侧：每个停点都可再问才补问（domain.DepartureGateStop.Reaskable）；
//   - 那一份问了关务、却没有它的一格：那时它还不在段里（控制事实的业务时间早于出发、登记晚于出发），补问。
func departureGateReaskDue(
	first domain.DepartureGateReading,
	earlier []domain.DepartureGateReading,
	object domain.CarriedObjectReference,
) bool {
	latest := first
	if len(earlier) > 0 {
		latest = earlier[len(earlier)-1]
	}
	findings := latest.Findings()
	byObject := make(map[domain.CarriedObjectReference]domain.GateFinding, len(findings))
	for _, finding := range findings {
		byObject[finding.Object()] = finding
	}
	if finding, found := byObject[object]; found {
		return reaskableFinding(byObject, finding, map[domain.CarriedObjectReference]bool{object: true})
	}
	if stops := latest.Stops(); len(stops) > 0 {
		for _, stop := range stops {
			if !stop.Reaskable() {
				return false
			}
		}
		return true
	}
	return true
}

// deterministicGateCauses 是读不动那一格里重问也是同一个结果的依据种类：答复译不回来（要查两边的口径）与成员关系成环（册子坏了）。
// 其余读不动——关务或成员关系调用失败、关务自己指名某一口读不动、成员身份读不动——过一会儿可能就好。
var deterministicGateCauses = map[string]bool{
	departureGateCustomsUntranslatable:           true,
	departureGateMembershipUntranslatable:        true,
	domain.ConsolidationMembershipCycle.String(): true,
}

// reaskableFinding 判一格是不是可再问的读不动。集运单元那一格由成员推出：它的缘由里有成员身份读不动，或它的某件成员那一格是可再问
// 的读不动，才算；只因成环或成员都已有定论而读不动的不算。
func reaskableFinding(
	byObject map[domain.CarriedObjectReference]domain.GateFinding,
	finding domain.GateFinding,
	visited map[domain.CarriedObjectReference]bool,
) bool {
	if finding.Answer() != domain.GateUnreadable {
		return false
	}
	members, unit := finding.ConsolidationUnit()
	if !unit {
		for _, basis := range finding.Bases() {
			if deterministicGateCauses[basis.Cause] {
				return false
			}
		}
		return true
	}
	for _, basis := range finding.Bases() {
		if basis.Cause == domain.ConsolidationMemberIdentityUnreadable.String() {
			return true
		}
	}
	for _, member := range members {
		cell, found := byObject[member]
		if !found || visited[member] {
			continue
		}
		visited[member] = true
		if reaskableFinding(byObject, cell, visited) {
			return true
		}
	}
	return false
}
