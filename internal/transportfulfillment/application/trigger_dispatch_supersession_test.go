package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证票 demo-chain-gaps/129 的两件：随交接信封发起的一拍核期望入场依据，链尾不是凭这一版交接入场时答`不是触发事实`；
// 查在册沿参与替代链回看，同段同对象的更正不另立第二项任务。另证票 demo-chain-gaps/130 落地之后，待确认更正成`已交接`
// 那一封是对象进派送段的触发事实。夹具全部合成。

// correctToHandedOver 对 object 在 scope 下的 predecessor 那一版登一份仍为`已交接`的更正，新版本是 successor。
func (fixture *dispatchOnHandoverFixture) correctToHandedOver(t *testing.T, object, scope, predecessor, successor string) {
	t.Helper()
	command := correctHandoverCommand(t, predecessor, successor)
	command.Object = object
	command.Scope = scope
	command.Verdict = domain.ObjectHandedOver
	command.ReleasingEvidence = "evidence-release-corrected"
	command.ReceivingEvidence = "evidence-receive-corrected"
	command.Rule = "handover-rule/v1"
	command.Basis = ""
	result, err := fixture.handovers.handler.Correct(t.Context(), command)
	if err != nil || result.Outcome() != application.HandoverCorrected {
		t.Fatalf("更正 %s → %s：%v %s", predecessor, successor, err, result.Outcome())
	}
}

func (fixture *dispatchOnHandoverFixture) runVersion(t *testing.T, object, scope, version string) application.TriggerDispatchOnHandoverResult {
	t.Helper()
	command := onHandoverCommand(t, object)
	command.Scope, command.Version = scope, version
	result, err := application.NewTriggerDispatchOnHandoverHandler(fixture.deps).Trigger(t.Context(), command)
	if err != nil {
		t.Fatalf("%s 那封交接信封：%v", version, err)
	}
	return result
}

// handOnward 经结束参与的真编排登 object 在 scope 下 version 那一版`已交接`：结束它此刻在场的参与，交进 segment（首个对象
// 立段时声明派送）。夹具其余各处用的交接编排接的是不结束参与的替身，这一笔要的正是「对象已被下一次交接带走」那个库面。
func (fixture *dispatchOnHandoverFixture) handOnward(t *testing.T, object, scope, version, segment string, at time.Time) {
	t.Helper()
	handler := application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.handovers.handovers,
		Segments:   fixture.handovers.segments,
		Downstream: &handoverHandoffDouble{},
		Clock:      handoverClock{at: handoverRegisteredAt},
		ParticipationEnds: application.NewEndFulfillmentParticipationHandler(application.EndFulfillmentParticipationDeps{
			Segments:  fixture.handovers.segments,
			Handovers: fixture.handovers.handovers,
			Clock:     handoverClock{at: handoverRegisteredAt},
		}),
	})
	command := registerHandoverCommand(t)
	command.Object, command.Scope, command.Version, command.JudgedAt = object, scope, version, at
	command.Segment, command.SegmentServiceAction = segment, domain.SegmentServesFinalDelivery.String()
	result, err := handler.Register(t.Context(), command)
	if err != nil || result.ParticipationEnd() != application.ParticipationEndedNow || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("%s 交进 %s：%v end=%s refusal=%s", object, segment, err, result.ParticipationEnd(), result.SegmentEntryRefusal())
	}
}

// Covers: AT-TF-289（票 demo-chain-gaps/129；ADR-0230 决定六）——信封晚到：对象已被下一次交接带进别的派送段，前一版那封交接
// 信封此刻才到，按对象找到的「此刻在场的段」不是它带对象进的段。执行器按期望交接核链尾，答`不是触发事实`
// （ENTRY_NOT_THIS_HANDOVER），不问所有者、不形成任务，信封入账；带对象进后一段的那一版照常形成任务。
//
// 待确认更正为`已交接`而对象仍挂在先前段里的那一种自票 demo-chain-gaps/130 起不再出现：更正那一版结束先前段的参与、带对象
// 进声明的段（见 TestAHandoverCorrectedFromPendingIntoADeliverySegmentTriggersOnTheCorrection）。
func TestAHandoverThatDidNotBringTheObjectIntoTheSegmentIsNotItsTrigger(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
	fixture.handOnward(t, "parcel-1", "handover-scope-2", "handover-result/parcel-1/onward", "segment-2", handoverJudgedTime.Add(2*time.Hour))
	asked := fixture.places.calls

	late := fixture.run(t, "parcel-1")
	triggered := late.Trigger()
	if late.Outcome() != application.DispatchOnHandoverTriggered ||
		triggered.Outcome() != application.DeliveryDispatchNotTriggered ||
		triggered.Refusal() != application.DeliveryTriggerRefusedEntryNotThisHandover {
		t.Fatalf("outcome = %s / %s / %s, want DISPATCH_TRIGGERED / NOT_A_DELIVERY_TRIGGER / ENTRY_NOT_THIS_HANDOVER",
			late.Outcome(), triggered.Outcome(), triggered.Refusal())
	}
	if late.Disposition() != application.DispatchOnHandoverSettled {
		t.Fatalf("disposition = %s, want SETTLED", late.Disposition())
	}
	if fixture.places.calls != asked || fixture.tasks.saves != 0 {
		t.Fatalf("所有者被问 %d 次、任务写了 %d 次, want 都不动", fixture.places.calls-asked, fixture.tasks.saves)
	}

	// 对照：带对象进后一段的那一版照常形成任务——核的是「是不是这一版」，不是一律拒。
	own := fixture.runVersion(t, "parcel-1", "handover-scope-2", "handover-result/parcel-1/onward")
	if own.Trigger().Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("带对象进段的那一版 = %s / %s, want DISPATCH_TASK_FORMED", own.Trigger().Outcome(), own.Trigger().Refusal())
	}
}

// Covers: AT-TF-329 派送那一半（票 demo-chain-gaps/130；ADR-0307 决定一，ADR-0230 决定六）——交接首登为待确认、声明进一段派送段：
// 那一封信不是`已交接`、入账。更正成`已交接`之后对象凭这一版进了那一段，更正版那封信核得过期望交接，形成一项任务，键取这一版
// 的入场依据。往回更正成待确认那一封入账；再更正成`已交接`，对象从失效版本回到同一段，那一封沿替代链认出先前那一项、答
// `已在册`，不另立第二项。
func TestAHandoverCorrectedFromPendingIntoADeliverySegmentTriggersOnTheCorrection(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	pending := registerHandoverCommand(t)
	pending.Verdict, pending.Rule, pending.Basis = domain.HandoverPendingConfirmation, "", "handover-basis-pending"
	pending.Segment, pending.SegmentServiceAction = "segment-1", domain.SegmentServesFinalDelivery.String()
	if registered, err := fixture.handovers.handler.Register(t.Context(), pending); err != nil ||
		registered.Outcome() != application.HandoverRegistered {
		t.Fatalf("登记待确认：%v %s", err, registered.Outcome())
	}
	if first := fixture.run(t, "parcel-1"); first.Outcome() != application.DispatchOnHandoverNotHandedOver ||
		first.Disposition() != application.DispatchOnHandoverSettled {
		t.Fatalf("待确认那一封 = %s / %s, want HANDOVER_NOT_HANDED_OVER / SETTLED", first.Outcome(), first.Disposition())
	}

	fixture.correctToHandedOver(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v1", "handover-result/parcel-1/confirmed")
	confirmed := fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/confirmed")
	formed, _ := confirmed.Trigger().Record()
	if confirmed.Trigger().Outcome() != application.DeliveryDispatchTaskFormed || fixture.tasks.saves != 1 {
		t.Fatalf("更正版那一封 = %s / %s、任务写了 %d 次, want DISPATCH_TASK_FORMED、1 次",
			confirmed.Trigger().Outcome(), confirmed.Trigger().Refusal(), fixture.tasks.saves)
	}

	withdrawing := correctHandoverCommand(t, "handover-result/parcel-1/confirmed", "handover-result/parcel-1/withdrawn")
	withdrawing.Verdict, withdrawing.Basis = domain.HandoverPendingConfirmation, "handover-basis-withdrawn"
	if withdrawn, err := fixture.handovers.handler.Correct(t.Context(), withdrawing); err != nil || withdrawn.Outcome() != application.HandoverCorrected {
		t.Fatalf("更正回待确认：%v %s", err, withdrawn.Outcome())
	}
	if back := fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/withdrawn"); back.Outcome() != application.DispatchOnHandoverNotHandedOver {
		t.Fatalf("撤回那一封 = %s, want HANDOVER_NOT_HANDED_OVER", back.Outcome())
	}

	fixture.correctToHandedOver(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/withdrawn", "handover-result/parcel-1/reconfirmed")
	again := fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/reconfirmed")
	returned, _ := again.Trigger().Record()
	if again.Trigger().Outcome() != application.DeliveryDispatchTaskAlreadyFormed || returned.Key.Task != formed.Key.Task || fixture.tasks.saves != 1 {
		t.Fatalf("再确认那一封 = %s 交回 %s、任务写了 %d 次, want DISPATCH_TASK_ALREADY_FORMED 交回 %s、1 次",
			again.Trigger().Outcome(), returned.Key.Task, fixture.tasks.saves, formed.Key.Task)
	}
}

// Covers: AT-TF-290（票 demo-chain-gaps/129；ADR-0230 决定六）——更正后仍为`已交接`时参与在同段长出替代版本、入场依据换成更正
// 那一版的键。更正版信封核得过期望交接，查在册沿替代链回看到前版已形成的那一项，答`已在册`交回它，不另铸第二项。前版任务从没
// 形成过时（前版那一拍答了要求缺失），更正版照链尾的入场依据铸、形成一项。
func TestACorrectionOfTheHandoverThatBroughtTheObjectInDoesNotMintASecondTask(t *testing.T) {
	t.Run("前版已形成任务：答已在册、交回那一项", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		first := fixture.run(t, "parcel-1")
		formed, _ := first.Trigger().Record()
		if first.Trigger().Outcome() != application.DeliveryDispatchTaskFormed {
			t.Fatalf("前版一拍 = %s, want DISPATCH_TASK_FORMED", first.Trigger().Outcome())
		}
		fixture.correctToHandedOver(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")

		result := fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v2")
		again, _ := result.Trigger().Record()
		if result.Trigger().Outcome() != application.DeliveryDispatchTaskAlreadyFormed || again.Key.Task != formed.Key.Task {
			t.Fatalf("更正版一拍 = %s 交回 %s, want DISPATCH_TASK_ALREADY_FORMED 交回 %s",
				result.Trigger().Outcome(), again.Key.Task, formed.Key.Task)
		}
		if fixture.tasks.saves != 1 {
			t.Fatalf("任务写了 %d 次, want 1——同段的替代版本不是进段", fixture.tasks.saves)
		}
	})

	t.Run("前版信封晚到：链尾已是更正版，前版那封不是触发", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		fixture.correctToHandedOver(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")

		late := fixture.run(t, "parcel-1")
		if late.Trigger().Refusal() != application.DeliveryTriggerRefusedEntryNotThisHandover {
			t.Fatalf("前版晚到 = %s / %s, want ENTRY_NOT_THIS_HANDOVER", late.Trigger().Outcome(), late.Trigger().Refusal())
		}
		result := fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v2")
		if result.Trigger().Outcome() != application.DeliveryDispatchTaskFormed || fixture.tasks.saves != 1 {
			t.Fatalf("更正版一拍 = %s、任务写了 %d 次, want DISPATCH_TASK_FORMED、1 次",
				result.Trigger().Outcome(), fixture.tasks.saves)
		}
	})

	t.Run("补拍不带期望交接：问的是对象此刻在段里的那条参与", func(t *testing.T) {
		fixture := newDispatchOnHandoverFixture(t)
		fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)
		fixture.run(t, "parcel-1")
		fixture.correctToHandedOver(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")

		retick, err := fixture.handler().Trigger(t.Context(), triggerCommand(t, "parcel-1"))
		if err != nil || retick.Outcome() != application.DeliveryDispatchTaskAlreadyFormed || fixture.tasks.saves != 1 {
			t.Fatalf("补拍 = %s err=%v、任务写了 %d 次, want DISPATCH_TASK_ALREADY_FORMED、1 次",
				retick.Outcome(), err, fixture.tasks.saves)
		}
	})
}
