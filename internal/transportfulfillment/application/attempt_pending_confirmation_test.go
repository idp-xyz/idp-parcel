package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证履约尝试逐对象结果的「待确认」与揽收的「执行方拒收」在编排面上的走法（票 docs-consistency/code/01）：两格随
// 尝试照常落册、不转移控制；待确认不是失败、不被后到的同来源消息改写。夹具全是合成串，证据只记 S。

func pendingPickupSubmission(t *testing.T, object string) application.ObjectPickupSubmission {
	t.Helper()
	return application.ObjectPickupSubmission{
		Object:     value(t, domain.NewCarriedObjectReference, object),
		Outcome:    domain.PickupPendingConfirmation,
		Source:     value(t, domain.NewAttemptResultSourceReference, "source/vehicle-arrival+driver-scan"),
		Basis:      value(t, domain.NewAttemptResultBasisReference, "gap/no-receipt-or-control-evidence"),
		OccurredAt: arrivedAt.Add(4 * time.Minute),
	}
}

type pendingPickupFixture struct {
	store    *pickupStoreDouble
	segments *segmentRegistryDouble
	versions *versionFactoryDouble
	handoff  *pickupHandoffDouble
	handler  *application.PerformOffsitePickupHandler
}

func newPendingPickupFixture(t *testing.T) *pendingPickupFixture {
	t.Helper()
	fixture := &pendingPickupFixture{
		store:    newPickupStore(),
		segments: newSegmentRegistry(),
		versions: &versionFactoryDouble{},
		handoff:  &pickupHandoffDouble{},
	}
	fixture.handler = application.NewPerformOffsitePickupHandler(application.PerformOffsitePickupDeps{
		Attempts:   fixture.store,
		Segments:   fixture.segments,
		Versions:   fixture.versions,
		Downstream: fixture.handoff,
		Clock:      fixedClock{at: recordedAt},
	})
	return fixture
}

func (fixture *pendingPickupFixture) handle(t *testing.T, command application.PerformOffsitePickupCommand) application.PerformOffsitePickupResult {
	t.Helper()
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	return result
}

// Covers: `AT-TF-018`「只有车辆到场和司机扫描 → 保存来源，结果待确认，不推导实际接收」——待确认对象随这次到访落册，
// 带着来源与缺口；它不形成场外揽收、不签版本、不进段、不交 parcel-shipment 采认的意图；同一次到访里取得控制的
// 另一件照常进段。
func TestAVisitWithOnlyArrivalAndScanRecordsAPendingResultAndTakesNoControl(t *testing.T) {
	fixture := newPendingPickupFixture(t)
	command := pickupCommand(t, "source-1", "attempt-1",
		pendingPickupSubmission(t, "parcel-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"))
	command.Segment = "segment-1"

	result := fixture.handle(t, command)
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED（待确认是要保全的结果，不是不登这次尝试）", result.Outcome())
	}
	record, _ := result.Record()
	var pending domain.AttemptObjectResult
	for _, objectResult := range record.Results {
		if objectResult.Object().String() == "parcel-1" {
			pending = objectResult
		}
	}
	if pending.Outcome() != domain.PickupPendingConfirmation {
		t.Fatalf("parcel-1 outcome = %q, want PENDING_CONFIRMATION", pending.Outcome())
	}
	if source, present := pending.Source(); !present || source.String() != "source/vehicle-arrival+driver-scan" {
		t.Fatal("待确认丢了来源")
	}
	if gap, present := pending.Basis(); !present || gap.String() != "gap/no-receipt-or-control-evidence" {
		t.Fatal("待确认丢了缺口")
	}
	for _, pickup := range record.Pickups {
		if pickup.Object().String() == "parcel-1" {
			t.Fatal("待确认对象形成了场外揽收——从来源推导了实际接收")
		}
	}
	if len(record.Pickups) != 1 || fixture.versions.minted != 1 {
		t.Fatalf("pickups = %d, versions = %d, want 1/1（只有取得控制的那一件）", len(record.Pickups), fixture.versions.minted)
	}
	segment := fixture.segments.saved(t, "tenant-1", "segment-1")
	if _, joined := segment.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-1")); joined {
		t.Fatal("待确认对象进了实际履约段——控制转移了")
	}
	if _, joined := segment.Segment.ParticipationFor(value(t, domain.NewCarriedObjectReference, "parcel-2")); !joined {
		t.Fatal("同一次到访取得控制的那一件没有进段")
	}

	t.Run("an all-pending visit hands nothing to parcel-shipment", func(t *testing.T) {
		fixture := newPendingPickupFixture(t)
		command := pickupCommand(t, "source-1", "attempt-1", pendingPickupSubmission(t, "parcel-1"))
		command.Segment = "segment-1"
		result := fixture.handle(t, command)
		if result.Outcome() != application.PickupAttemptRecorded {
			t.Fatalf("outcome = %q", result.Outcome())
		}
		if len(fixture.handoff.intents) != 0 || fixture.versions.minted != 0 || fixture.segments.saves != 0 {
			t.Fatalf("intents = %d versions = %d segment saves = %d, want 0/0/0",
				len(fixture.handoff.intents), fixture.versions.minted, fixture.segments.saves)
		}
	})
}

// Covers: `AT-TF-018` 与 CONTEXT「不按最后消息覆盖」——待确认那一格不改写：同一来源身份换成「揽收到手」再交是冲突，
// 原记录原样；之后真的再上门一次，是一次新尝试，揽收只在新尝试上形成，第一次尝试照旧是待确认。
func TestAPendingResultIsNotRewrittenByALaterMessageAndANewVisitIsANewAttempt(t *testing.T) {
	fixture := newPendingPickupFixture(t)
	fixture.handle(t, pickupCommand(t, "source-1", "attempt-1", pendingPickupSubmission(t, "parcel-1")))

	flipped := fixture.handle(t, pickupCommand(t, "source-1", "attempt-1",
		successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1")))
	if flipped.Outcome() != application.PickupSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", flipped.Outcome())
	}
	kept := fixture.store.records["source-1"]
	if len(kept.Pickups) != 0 || len(kept.Results) != 1 || kept.Results[0].Outcome() != domain.PickupPendingConfirmation {
		t.Fatal("后到的消息改写了待确认那一格")
	}

	retry := pickupCommand(t, "source-2", "attempt-2", successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1"))
	retry.RescheduledFrom = "attempt-1"
	retry.PlannedFrom = plannedFrom.Add(24 * time.Hour)
	retry.PlannedTo = plannedTo.Add(24 * time.Hour)
	retry.ArrivedAt = arrivedAt.Add(24 * time.Hour)
	retry.Objects[0].OccurredAt = arrivedAt.Add(24*time.Hour + 5*time.Minute)
	second := fixture.handle(t, retry)
	if second.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("second outcome = %q", second.Outcome())
	}
	if record, _ := second.Record(); len(record.Pickups) != 1 {
		t.Fatal("新尝试上没有形成揽收")
	}
	if first := fixture.store.records["source-1"]; first.Results[0].Outcome() != domain.PickupPendingConfirmation || len(first.Pickups) != 0 {
		t.Fatal("新尝试改写了第一次尝试的待确认")
	}
}

// Covers: `AT-TF-017`「包装不合格且执行方明确拒收 → 保存对象级拒收和证据，控制不转移」——拒收逐对象成立、带原因与
// 证据依据，不形成揽收、不进段；同一次到访另一件照常揽到手。
func TestAnExecutorRefusalIsKeptPerObjectAndTakesNoControl(t *testing.T) {
	fixture := newPendingPickupFixture(t)
	command := pickupCommand(t, "source-1", "attempt-1",
		failureSubmission(t, "parcel-1", domain.ExecutorRefused, "reason/packaging-unacceptable+photo-1"),
		successSubmission(t, "parcel-2", "TRANSPORT-CONTROL/TF-2"))
	command.Segment = "segment-1"

	result := fixture.handle(t, command)
	if result.Outcome() != application.PickupAttemptRecorded {
		t.Fatalf("outcome = %q", result.Outcome())
	}
	record, _ := result.Record()
	refusals := 0
	for _, objectResult := range record.Results {
		if objectResult.Object().String() != "parcel-1" {
			continue
		}
		refusals++
		if objectResult.Outcome() != domain.ExecutorRefused {
			t.Fatalf("parcel-1 outcome = %q, want EXECUTOR_REFUSED", objectResult.Outcome())
		}
		if basis, present := objectResult.Basis(); !present || basis.String() != "reason/packaging-unacceptable+photo-1" {
			t.Fatal("拒收丢了原因与证据")
		}
	}
	if refusals != 1 {
		t.Fatalf("parcel-1 results = %d, want 1", refusals)
	}
	for _, pickup := range record.Pickups {
		if pickup.Object().String() == "parcel-1" {
			t.Fatal("拒收的对象形成了揽收")
		}
	}
	if _, joined := fixture.segments.saved(t, "tenant-1", "segment-1").Segment.ParticipationFor(
		value(t, domain.NewCarriedObjectReference, "parcel-1")); joined {
		t.Fatal("拒收的对象进了实际履约段")
	}
}

// 待确认与拒收的提交自身矛盾落`来源未受理`，与其余矛盾同格（ADR-0029 按恢复动作分格：改请求）：待确认带控制依据
// 就是把待确认伪装成取得控制；缺来源或缺口的待确认说不出凭什么、差什么；确定结果带来源是一格只属待确认的内容出现
// 在别处。
func TestAContradictoryPendingOrRefusalSubmissionIsNotAccepted(t *testing.T) {
	broken := map[string]func(*testing.T) application.ObjectPickupSubmission{
		"pending carrying control": func(t *testing.T) application.ObjectPickupSubmission {
			submission := pendingPickupSubmission(t, "parcel-1")
			submission.Control = value(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-9")
			return submission
		},
		"pending without a source": func(t *testing.T) application.ObjectPickupSubmission {
			submission := pendingPickupSubmission(t, "parcel-1")
			submission.Source = domain.AttemptResultSourceReference{}
			return submission
		},
		"pending without a gap": func(t *testing.T) application.ObjectPickupSubmission {
			submission := pendingPickupSubmission(t, "parcel-1")
			submission.Basis = domain.AttemptResultBasisReference{}
			return submission
		},
		"a failure carrying a source": func(t *testing.T) application.ObjectPickupSubmission {
			submission := failureSubmission(t, "parcel-1", domain.CustomerAbsent, "reason-absent-1")
			submission.Source = value(t, domain.NewAttemptResultSourceReference, "source/1")
			return submission
		},
		"a success carrying a source": func(t *testing.T) application.ObjectPickupSubmission {
			submission := successSubmission(t, "parcel-1", "TRANSPORT-CONTROL/TF-1")
			submission.Source = value(t, domain.NewAttemptResultSourceReference, "source/1")
			return submission
		},
		"a refusal carrying control": func(t *testing.T) application.ObjectPickupSubmission {
			submission := failureSubmission(t, "parcel-1", domain.ExecutorRefused, "reason/refused-1")
			submission.Control = value(t, domain.NewTransportControlReference, "TRANSPORT-CONTROL/TF-9")
			return submission
		},
	}
	for name, build := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newPendingPickupFixture(t)
			result := fixture.handle(t, pickupCommand(t, "source-1", "attempt-1", build(t)))
			if result.Outcome() != application.PickupNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
			if len(fixture.store.records) != 0 || fixture.versions.minted != 0 || len(fixture.handoff.intents) != 0 {
				t.Fatal("矛盾的提交落了库、签了版本或交了意图")
			}
		})
	}
}

func (fixture *deliveryAttemptFixture) pending(object string) application.ObjectDeliverySubmission {
	fixture.t.Helper()
	return application.ObjectDeliverySubmission{
		Object:     value(fixture.t, domain.NewCarriedObjectReference, object),
		Outcome:    domain.DeliveryPendingConfirmation,
		Source:     value(fixture.t, domain.NewAttemptResultSourceReference, "source/signature-scan+address-geofence"),
		Basis:      value(fixture.t, domain.NewAttemptResultBasisReference, "conflict/signature-vs-address"),
		OccurredAt: arrivedAt.Add(6 * time.Minute),
	}
}

// Covers: `AT-TF-070`「本人签收和地址来源冲突 → 形成冲突或待确认，不按最后消息覆盖」的登记面——冲突的对象记待确认、
// 带全部来源与冲突；同一尝试身份再交一份说「妥投」的是冲突，原记录原样。
func TestAConflictingSignatureAndAddressIsRecordedPendingAndNotOverwritten(t *testing.T) {
	fixture := newDeliveryAttemptFixture(t)
	result := fixture.handle(fixture.command(fixture.pending("parcel-1"), fixture.delivered("parcel-2")))
	if got := result.Outcome().String(); got != "ATTEMPT_RECORDED" {
		t.Fatalf("outcome = %q, want ATTEMPT_RECORDED", got)
	}
	record, _ := result.Record()
	for _, objectResult := range record.Results {
		if objectResult.Object().String() != "parcel-1" {
			continue
		}
		if objectResult.Outcome() != domain.DeliveryPendingConfirmation {
			t.Fatalf("parcel-1 outcome = %q", objectResult.Outcome())
		}
		if source, present := objectResult.Source(); !present || source.String() != "source/signature-scan+address-geofence" {
			t.Fatal("待确认丢了全部来源")
		}
		if conflict, present := objectResult.Basis(); !present || conflict.String() != "conflict/signature-vs-address" {
			t.Fatal("待确认丢了冲突")
		}
	}

	replay := fixture.handle(fixture.command(fixture.delivered("parcel-2"), fixture.pending("parcel-1")))
	if got := replay.Outcome().String(); got != "EXISTING_RESULT" {
		t.Fatalf("同内容重投 outcome = %q, want EXISTING_RESULT", got)
	}

	changedSource := fixture.pending("parcel-1")
	changedSource.Source = value(t, domain.NewAttemptResultSourceReference, "source/signature-scan-only")
	if got := fixture.handle(fixture.command(changedSource, fixture.delivered("parcel-2"))).Outcome().String(); got != "SOURCE_CONFLICT" {
		t.Fatalf("换了来源再交 outcome = %q, want SOURCE_CONFLICT——来源是内容的一部分", got)
	}

	flipped := fixture.handle(fixture.command(fixture.delivered("parcel-1"), fixture.delivered("parcel-2")))
	if got := flipped.Outcome().String(); got != "SOURCE_CONFLICT" {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT——后到的「妥投」不覆盖待确认", got)
	}
	kept := fixture.store.records["tenant-1/attempt-1"]
	for _, objectResult := range kept.Results {
		if objectResult.Object().String() == "parcel-1" && objectResult.Outcome() != domain.DeliveryPendingConfirmation {
			t.Fatalf("stored parcel-1 = %q, want the pending result untouched", objectResult.Outcome())
		}
	}
}

// 派送一侧的矛盾提交同揽收那一侧落`来源未受理`。
func TestAContradictoryPendingDeliverySubmissionIsNotAccepted(t *testing.T) {
	broken := map[string]func(*deliveryAttemptFixture) application.ObjectDeliverySubmission{
		"pending without a source": func(fixture *deliveryAttemptFixture) application.ObjectDeliverySubmission {
			submission := fixture.pending("parcel-1")
			submission.Source = domain.AttemptResultSourceReference{}
			return submission
		},
		"pending without a conflict": func(fixture *deliveryAttemptFixture) application.ObjectDeliverySubmission {
			submission := fixture.pending("parcel-1")
			submission.Basis = domain.AttemptResultBasisReference{}
			return submission
		},
		"a delivered result carrying a source": func(fixture *deliveryAttemptFixture) application.ObjectDeliverySubmission {
			submission := fixture.delivered("parcel-1")
			submission.Source = value(fixture.t, domain.NewAttemptResultSourceReference, "source/1")
			return submission
		},
		"a refusal carrying a source": func(fixture *deliveryAttemptFixture) application.ObjectDeliverySubmission {
			submission := fixture.failed("parcel-1", domain.DeliveryRefused, "basis/refused")
			submission.Source = value(fixture.t, domain.NewAttemptResultSourceReference, "source/1")
			return submission
		},
	}
	for name, build := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryAttemptFixture(t)
			result := fixture.handle(fixture.command(build(fixture)))
			if got := result.Outcome().String(); got != "SOURCE_NOT_ACCEPTED" {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", got)
			}
			if fixture.store.saves != 0 {
				t.Fatal("矛盾的提交落了库")
			}
		})
	}
}

// Covers: `AT-TF-070` 的交付面——待确认的对象登不出有效交付：答`未生效`，不落交付、不交意图，段里的参与照旧在场
// （控制不转移给收件方）。证据补齐之后同一次上门的确定结果以更正形成，那条口另票（code/90）。
func TestAPendingDeliveryResultCannotRegisterAnEffectiveDeliveryOrEndParticipation(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)
	fixture.joinEarlyMember(t, "parcel-3")
	handoff := &deliveryHandoffDouble{}
	handler := gatedDeliveryHandler{application.NewRegisterEffectiveDeliveryHandler(application.RegisterEffectiveDeliveryDeps{
		Attempts:          &deliveryViewDouble{outcome: domain.DeliveryPendingConfirmation, found: true},
		Deliveries:        fixture.deliveries,
		Versions:          &deliveryVersionFactory{},
		Downstream:        handoff,
		Clock:             deliveryClock{at: deliveryRecordedAt},
		ParticipationEnds: fixture.handler,
	})}

	result, err := handler.Register(t.Context(), deliveryCommandFor(t, "parcel-3", "attempt-1"))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.Outcome() != application.DeliveryNotEffective {
		t.Fatalf("outcome = %q, want NOT_EFFECTIVE（证据不足不构成有效交付）", result.Outcome())
	}
	if len(fixture.deliveries.records) != 0 || len(handoff.intents) != 0 {
		t.Fatal("待确认落了交付或交了意图")
	}
	if !fixture.participation(t, "parcel-3").Active() {
		t.Fatal("待确认结束了对象的参与——控制转移给了收件方")
	}
}

func pendingPickupAt(t *testing.T, attemptRef string) pickupResultSourceDouble {
	t.Helper()
	attempt := attemptFixture(t, attemptRef)
	result, err := domain.FormPendingAttemptObjectResult(attempt,
		occurrenceValue(t, domain.NewCarriedObjectReference, trunkParcel),
		occurrenceValue(t, domain.NewAttemptResultSourceReference, "SYN-SOURCE/vehicle-arrival+driver-scan"),
		occurrenceValue(t, domain.NewAttemptResultBasisReference, "SYN-GAP/no-receipt"),
		attemptArrivedAt.Add(10*time.Minute))
	if err != nil {
		t.Fatalf("待确认揽收结果夹具：%v", err)
	}
	return pickupResultSourceDouble{
		record: ports.PickupAttemptRecord{Attempt: attempt, Results: []domain.AttemptObjectResult{result}},
		found:  true,
	}
}

// Covers: 领域 ErrAttemptResultPendingConfirmation 的编排面——待确认不是失败，失败尝试费不拿它作事实依据：答`未受理`
// 带自己那一格细格（ATTEMPT_RESULT_PENDING_CONFIRMATION），不并进「没有失败」（ADR-0029 按恢复动作分格：那一格放弃，
// 这一格等它以更正或新尝试确定之后再登），不落发生项、不交意图。执行方拒收与收件人拒收同一族，照常形成发生项。
func TestAPendingAttemptResultIsNotAFailedAttemptFact(t *testing.T) {
	pickup := newOccurrenceRig(t)
	pickup.pickups = pendingPickupAt(t, "SYN-PICKUP-ATTEMPT-01")
	command := failedAttemptCommand(t)
	command.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
	if result := pickup.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.AttemptResultPendingConfirmation ||
		result.Detail().String() != "ATTEMPT_RESULT_PENDING_CONFIRMATION" {
		t.Fatalf("揽收待确认 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, pickup)

	delivery := newOccurrenceRig(t)
	delivery.deliveries = deliveryWith(t, domain.DeliveryPendingConfirmation)
	command = failedAttemptCommand(t)
	command.DeliveryAttempt = "SYN-DELIVERY-ATTEMPT-01"
	if result := delivery.register(t, command); result.Outcome() != application.ChargeOccurrenceInputNotAccepted ||
		result.Detail() != application.AttemptResultPendingConfirmation {
		t.Fatalf("派送待确认 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
	assertNothingWritten(t, delivery)

	refused := newOccurrenceRig(t)
	refused.pickups = pickupWith(t, domain.ExecutorRefused)
	command = failedAttemptCommand(t)
	command.PickupAttemptSource = "SYN-PICKUP-SOURCE-01"
	if result := refused.register(t, command); result.Outcome() != application.ChargeOccurrenceRegistered {
		t.Fatalf("执行方拒收 outcome/detail = %q/%q", result.Outcome(), result.Detail())
	}
}
