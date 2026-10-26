package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉 ADR-0303 在七个运营决定口的编排半边：信封里的提交操作者随决定落在决定记录上；同一个决定换一位操作者重放，
// 答复与首次重放同格、在册的仍是先到的那一位；命令不带操作者照样形成——它只进审计，不进重放比对，也不进任何门。
// 夹具全为合成引用（S 级）。

const (
	firstOperator  = "https://id.syn.example/dex#SYN-OPERATOR-A"
	secondOperator = "https://id.syn.example/dex#SYN-OPERATOR-B"
)

// Covers: `AT-TF-337` 编排半边——关段记下作出关段决定的人；另一位操作者再关答`早已关闭`，在册的关闭时刻与关段人都不被
// 顶替；不带操作者的关段照样关上；带 NUL 的操作者存不进登记册，在构造期答未受理、不动库。
func TestASegmentClosureRecordsItsOperatorAndAReplayByAnotherKeepsIt(t *testing.T) {
	t.Run("the closer is recorded and a second operator's replay keeps it", func(t *testing.T) {
		fixture := newClosureFixture(t)
		fixture.twoMemberSegment(t)
		fixture.endBoth(t)

		command := closeCommand(t)
		command.ClosedBy = firstOperator
		if result, err := fixture.closer.Close(t.Context(), command); err != nil || result.Outcome() != application.SegmentClosedNow {
			t.Fatalf("关段：outcome=%s err=%v", result.Outcome(), err)
		}
		replay := command
		replay.ClosedBy = secondOperator
		replay.ClosedAt = command.ClosedAt.Add(time.Minute)
		result, err := fixture.closer.Close(t.Context(), replay)
		if err != nil || result.Outcome() != application.SegmentAlreadyClosed {
			t.Fatalf("另一位操作者再关：outcome=%s err=%v，want SEGMENT_ALREADY_CLOSED", result.Outcome(), err)
		}
		closed := fixture.segments.saved(t, "tenant-1", "segment-1").Segment
		if by, recorded := closed.ClosedBy(); !recorded || by.String() != firstOperator {
			t.Fatalf("在册关段人 = %q (%v)，want 先关的那一位 %q", by.String(), recorded, firstOperator)
		}
	})

	t.Run("a closure without an operator closes the same", func(t *testing.T) {
		fixture := newClosureFixture(t)
		fixture.twoMemberSegment(t)
		fixture.endBoth(t)
		if result, err := fixture.closer.Close(t.Context(), closeCommand(t)); err != nil || result.Outcome() != application.SegmentClosedNow {
			t.Fatalf("不带操作者关段：outcome=%s err=%v", result.Outcome(), err)
		}
		closed := fixture.segments.saved(t, "tenant-1", "segment-1").Segment
		if _, recorded := closed.ClosedBy(); recorded || !closed.Closed() {
			t.Fatalf("不带操作者应照样关上且不记人：closed=%v recorded=%v", closed.Closed(), recorded)
		}
	})

	t.Run("an operator that cannot be stored is refused before anything is written", func(t *testing.T) {
		fixture := newClosureFixture(t)
		fixture.twoMemberSegment(t)
		fixture.endBoth(t)
		command := closeCommand(t)
		command.ClosedBy = "issuer#sub\x00"
		if result, err := fixture.closer.Close(t.Context(), command); err != nil || result.Outcome() != application.SegmentCloseNotAccepted {
			t.Fatalf("带 NUL 的操作者：outcome=%s err=%v，want INPUT_NOT_ACCEPTED", result.Outcome(), err)
		}
		if fixture.segments.saved(t, "tenant-1", "segment-1").Segment.Closed() {
			t.Fatal("未受理的关段不得落库")
		}
	})
}

// Covers: `AT-TF-338` 编排半边——明确终止记下作出终止的人；另一位操作者同一次终止重投答`已离场`、回显在册终点，在册的终止
// 人不被顶替；不带操作者的终止照样结束参与；以下一次交接结束的参与没有终止人。
func TestATerminationRecordsItsOperatorAndAReplayByAnotherKeepsIt(t *testing.T) {
	fixture := newParticipationFixture(t)
	fixture.twoMemberSegment(t)

	command := terminationCommand(t, "parcel-1")
	command.TerminatedBy = firstOperator
	if result, err := fixture.handler.End(t.Context(), command); err != nil || result.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("终止：outcome=%s err=%v", result.Outcome(), err)
	}
	replay := command
	replay.TerminatedBy = secondOperator
	result, err := fixture.handler.End(t.Context(), replay)
	if err != nil || result.Outcome() != application.ParticipationAlreadyEnded {
		t.Fatalf("另一位操作者重投：outcome=%s err=%v，want PARTICIPATION_ALREADY_ENDED", result.Outcome(), err)
	}
	if by, recorded := fixture.participation(t, "parcel-1").TerminatedBy(); !recorded || by.String() != firstOperator {
		t.Fatalf("在册终止人 = %q (%v)，want 先终止的那一位 %q", by.String(), recorded, firstOperator)
	}

	if result, err := fixture.handler.End(t.Context(), terminationCommand(t, "parcel-2")); err != nil || result.Outcome() != application.ParticipationEndedNow {
		t.Fatalf("不带操作者终止：outcome=%s err=%v", result.Outcome(), err)
	}
	unattributed := fixture.participation(t, "parcel-2")
	if _, recorded := unattributed.TerminatedBy(); recorded || unattributed.Active() {
		t.Fatalf("不带操作者应照样结束且不记人：active=%v recorded=%v", unattributed.Active(), recorded)
	}
}

// Covers: `AT-TF-341` 编排半边——建派送任务记下建任务的人；另一位操作者同一任务重投答`已在册`、交回的原任务连同原建任务人
// 都不被顶替；不带操作者的任务（派送发起执行器经本编排形成任务时就不带）照样建立、不记人。
func TestADispatchTaskRecordsItsOpenerAndAReplayByAnotherKeepsIt(t *testing.T) {
	registry := newDispatchTaskRegistry()
	handler := newDispatchTaskHandler(registry)

	command := openTaskCommand(t)
	command.OpenedBy = firstOperator
	opened, err := handler.Open(t.Context(), command)
	if err != nil || opened.Outcome() != application.DispatchTaskOpened {
		t.Fatalf("建任务：outcome=%s err=%v", opened.Outcome(), err)
	}
	replay := command
	replay.OpenedBy = secondOperator
	again, err := handler.Open(t.Context(), replay)
	if err != nil || again.Outcome() != application.DispatchTaskAlreadyRegistered {
		t.Fatalf("另一位操作者重投：outcome=%s err=%v，want DISPATCH_TASK_ALREADY_OPEN", again.Outcome(), err)
	}
	if echoed, _ := again.Record(); echoed.OpenedBy.String() != firstOperator {
		t.Fatalf("重放交回的原任务建任务人 = %q，want %q", echoed.OpenedBy.String(), firstOperator)
	}
	if stored := registry.records[dispatchTaskRegistryKey(echoed(t, opened))]; stored.OpenedBy.String() != firstOperator || registry.saves != 1 {
		t.Fatalf("在册建任务人 = %q、写了 %d 次，want %q、1 次", stored.OpenedBy.String(), registry.saves, firstOperator)
	}

	unattributed := openTaskCommand(t)
	unattributed.Task = "dispatch-task-2"
	formed, err := handler.Open(t.Context(), unattributed)
	if err != nil || formed.Outcome() != application.DispatchTaskOpened {
		t.Fatalf("不带操作者建任务：outcome=%s err=%v", formed.Outcome(), err)
	}
	if record, _ := formed.Record(); record.OpenedBy.Present() {
		t.Fatalf("不带操作者不应记人：%q", record.OpenedBy.String())
	}
}

func echoed(t *testing.T, result application.OpenDispatchTaskResult) ports.DispatchTaskKey {
	t.Helper()
	record, present := result.Record()
	if !present {
		t.Fatal("建任务的结果没带记录")
	}
	return record.Key
}

// Covers: `AT-TF-336` 编排半边——装载分配首版记下作出分配的人；另一位操作者同一版本重投答`已在册`、交回原版本连同原
// 分配人；不带操作者的分配照样形成、不记人。
func TestALoadAssignmentRecordsItsDeciderAndAReplayByAnotherKeepsIt(t *testing.T) {
	registry := newLoadAssignmentRegistry()
	handler := newLoadAssignmentHandler(registry)

	command := formAssignmentCommand(t)
	command.DecidedBy = firstOperator
	if formed, err := handler.Form(t.Context(), command); err != nil || formed.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("形成分配：outcome=%s err=%v", formed.Outcome(), err)
	}
	replay := command
	replay.DecidedBy = secondOperator
	again, err := handler.Form(t.Context(), replay)
	if err != nil || again.Outcome() != application.LoadAssignmentVersionExists {
		t.Fatalf("另一位操作者重投：outcome=%s err=%v，want LOAD_ASSIGNMENT_VERSION_EXISTS", again.Outcome(), err)
	}
	if record, _ := again.Record(); record.DecidedBy.String() != firstOperator || registry.saves != 1 {
		t.Fatalf("在册分配人 = %q、写了 %d 次，want %q、1 次", record.DecidedBy.String(), registry.saves, firstOperator)
	}

	unattributed := formAssignmentCommand(t)
	unattributed.Assignment = "load-assignment-2"
	formed, err := handler.Form(t.Context(), unattributed)
	if err != nil || formed.Outcome() != application.LoadAssignmentFormed {
		t.Fatalf("不带操作者形成分配：outcome=%s err=%v", formed.Outcome(), err)
	}
	if record, _ := formed.Record(); record.DecidedBy.Present() {
		t.Fatalf("不带操作者不应记人：%q", record.DecidedBy.String())
	}
}

// Covers: `AT-TF-340` 编排半边——显式判断有效时间形成的那一版记下判断人；另一位操作者按同值再判答`已按同值判过`、不长新
// 版本，回显的仍是先判的那一位；不带操作者的判断照样形成新版本、不记人。素材到达形成的那一版本就没有判断人。
func TestAnEffectiveTimeJudgmentRecordsItsJudgeAndASameValueJudgmentByAnotherKeepsIt(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	tenant := mustTenant(t, "tenant-1")
	handler := newJudgeHandler(fixture)

	command := application.JudgeEffectiveTimeCommand{
		TenantID: tenant, Fact: factRef.String(), EffectiveAt: trackingReceivedAt, JudgedBy: firstOperator,
	}
	judged, err := handler.Judge(t.Context(), command)
	if err != nil || judged.Outcome() != application.EffectiveTimeJudged {
		t.Fatalf("判断：outcome=%s err=%v", judged.Outcome(), err)
	}
	if record, _ := judged.Record(); record.EffectiveJudgedBy.String() != firstOperator {
		t.Fatalf("判断版本的判断人 = %q，want %q", record.EffectiveJudgedBy.String(), firstOperator)
	}
	replay := command
	replay.JudgedBy = secondOperator
	again, err := handler.Judge(t.Context(), replay)
	if err != nil || again.Outcome() != application.EffectiveTimeAlreadyJudgedAsGiven {
		t.Fatalf("另一位操作者按同值再判：outcome=%s err=%v，want ALREADY_JUDGED_AS_GIVEN", again.Outcome(), err)
	}
	if record, _ := again.Record(); record.EffectiveJudgedBy.String() != firstOperator || len(fixture.registry.records) != 2 {
		t.Fatalf("回显判断人 = %q、事实库 %d 版，want %q、2 版", record.EffectiveJudgedBy.String(), len(fixture.registry.records), firstOperator)
	}
	for _, record := range fixture.registry.records {
		if record.Fact.Origin() == domain.VersionFromMaterial && record.EffectiveJudgedBy.Present() {
			t.Fatalf("素材到达的那一版不该有判断人：%q", record.EffectiveJudgedBy.String())
		}
	}

	unattributed := command
	unattributed.JudgedBy = ""
	unattributed.EffectiveAt = trackingReceivedAt.Add(time.Minute)
	changed, err := handler.Judge(t.Context(), unattributed)
	if err != nil || changed.Outcome() != application.EffectiveTimeJudged {
		t.Fatalf("不带操作者改判：outcome=%s err=%v", changed.Outcome(), err)
	}
	if record, _ := changed.Record(); record.EffectiveJudgedBy.Present() {
		t.Fatalf("不带操作者不应记人：%q", record.EffectiveJudgedBy.String())
	}
}

// Covers: `AT-TF-339` 编排半边——承运商首次有效收寄判断落下的那一版记下判断人；另一位操作者同依据同内容重放答`已在册`、
// 链尾不动，在册的仍是先判的那一位；异内容照旧答`内容冲突`，不因换了人而改判；不带操作者的判断照样形成、不记人。
func TestACarrierPickupJudgmentRecordsItsJudgeAndAReplayByAnotherKeepsIt(t *testing.T) {
	t.Run("the judge is recorded and a replay or a conflict by another keeps the chain", func(t *testing.T) {
		fixture := newCarrierPickupFixture(t, carrierX(t))
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
		command := trackingPickupCommand(t)
		command.JudgedBy = firstOperator

		first := fixture.judge(t, command)
		if first.Outcome() != application.CarrierPickupFormedOutcome {
			t.Fatalf("首判 = %s", first.Outcome())
		}
		if record, _ := first.Record(); record.JudgedBy.String() != firstOperator {
			t.Fatalf("首判那一版的判断人 = %q，want %q", record.JudgedBy.String(), firstOperator)
		}
		replay := command
		replay.JudgedBy = secondOperator
		again := fixture.judge(t, replay)
		if again.Outcome() != application.CarrierPickupAlreadyRecorded || len(fixture.pickups.rows) != 1 {
			t.Fatalf("另一位操作者重放 = %s、链上 %d 版，want PICKUP_ALREADY_RECORDED、1 版", again.Outcome(), len(fixture.pickups.rows))
		}
		conflicting := replay
		conflicting.SubjectReference = "party/carrier-y"
		if conflict := fixture.judge(t, conflicting); conflict.Outcome() != application.CarrierPickupContentConflict || len(fixture.pickups.rows) != 1 {
			t.Fatalf("另一位操作者异内容 = %s、链上 %d 版，want PICKUP_CONTENT_CONFLICT、1 版", conflict.Outcome(), len(fixture.pickups.rows))
		}
		chain, err := fixture.pickups.ListByObject(t.Context(), mustTenant(t, "tenant-1"), mustRefValue(t, domain.NewCarriedObjectReference, "PCL-1"))
		if err != nil || len(chain) != 1 || chain[0].JudgedBy.String() != firstOperator {
			t.Fatalf("链读回：%v %+v，want 一版、判断人 %q", err, chain, firstOperator)
		}
	})

	t.Run("a judgment without an operator forms the same", func(t *testing.T) {
		fixture := newCarrierPickupFixture(t, carrierX(t))
		fixture.addTrackingFact(t, "EXTF-1", "EXTV-1", "PCL-1", carrierPickupEffectiveAt, "")
		result := fixture.judge(t, trackingPickupCommand(t))
		if result.Outcome() != application.CarrierPickupFormedOutcome {
			t.Fatalf("不带操作者首判 = %s", result.Outcome())
		}
		if record, _ := result.Record(); record.JudgedBy.Present() {
			t.Fatalf("不带操作者不应记人：%q", record.JudgedBy.String())
		}
	})
}

// Covers: `AT-TF-335` 编排半边——运输委托提交记下提交人；另一位操作者原样再交答`已有结果`、交回原委托连同原提交人，内容
// 摘要不含提交人；异内容照旧答冲突；不带操作者的提交照样成立、不记人。
func TestACommissionSubmissionRecordsItsSubmitterAndAReplayByAnotherKeepsIt(t *testing.T) {
	fixture := newCommissionFixture(t)
	command := submitCommissionCommand(t)
	command.SubmittedBy = firstOperator
	submitted, err := fixture.handler.SubmitCommission(t.Context(), command)
	if err != nil || submitted.Outcome() != application.CommissionSubmitted {
		t.Fatalf("提交：outcome=%s err=%v", submitted.Outcome(), err)
	}
	original, _ := submitted.Commission()

	replay := command
	replay.SubmittedBy = secondOperator
	again, err := fixture.handler.SubmitCommission(t.Context(), replay)
	if err != nil || again.Outcome() != application.CommissionExistingResult {
		t.Fatalf("另一位操作者原样再交：outcome=%s err=%v，want EXISTING_RESULT", again.Outcome(), err)
	}
	echoedCommission, _ := again.Commission()
	if echoedCommission.SubmittedBy.String() != firstOperator || echoedCommission.ContentDigest != original.ContentDigest || fixture.commissions.saves != 1 {
		t.Fatalf("回显提交人 = %q、摘要相同 = %v、写了 %d 次，want %q、相同、1 次",
			echoedCommission.SubmittedBy.String(), echoedCommission.ContentDigest == original.ContentDigest, fixture.commissions.saves, firstOperator)
	}

	conflicting := replay
	conflicting.Members = []string{"parcel-1", "parcel-2"}
	if conflict, err := fixture.handler.SubmitCommission(t.Context(), conflicting); err != nil || conflict.Outcome() != application.CommissionConflict {
		t.Fatalf("另一位操作者异内容：outcome=%s err=%v，want CONFLICT", conflict.Outcome(), err)
	}

	unattributed := submitCommissionCommand(t)
	unattributed.Commission = "commission-2"
	formed, err := fixture.handler.SubmitCommission(t.Context(), unattributed)
	if err != nil || formed.Outcome() != application.CommissionSubmitted {
		t.Fatalf("不带操作者提交：outcome=%s err=%v", formed.Outcome(), err)
	}
	if record, _ := formed.Commission(); record.SubmittedBy.Present() {
		t.Fatalf("不带操作者不应记人：%q", record.SubmittedBy.String())
	}
}
