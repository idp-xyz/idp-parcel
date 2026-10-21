package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证票 demo-chain-gaps/129 的两件：随交接信封发起的一拍核期望入场依据，链尾不是凭这一版交接入场时答`不是触发事实`；
// 查在册沿参与替代链回看，同段同对象的更正不另立第二项任务。夹具全部合成。

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

// Covers: AT-TF-289（票 demo-chain-gaps/129；ADR-0230 决定六）——待确认更正为`已交接`时对象没有进新段、仍挂在先前的派送段里：
// 这封更正版信封找到的「此刻在场的段」不是它带对象进的段。执行器按期望交接核链尾，答`不是触发事实`
// （ENTRY_NOT_THIS_HANDOVER），不问所有者、不形成任务，信封入账。
func TestAHandoverThatDidNotBringTheObjectIntoTheSegmentIsNotItsTrigger(t *testing.T) {
	fixture := newDispatchOnHandoverFixture(t)
	fixture.enterByHandover(t, "parcel-1", "FINAL_DELIVERY", handoverJudgedTime)

	pending := registerHandoverCommand(t)
	pending.Scope = "handover-scope-2"
	pending.Version = "handover-result/parcel-1/pending"
	pending.Verdict = domain.HandoverPendingConfirmation
	pending.Rule = ""
	pending.Basis = "handover-basis-pending"
	pending.JudgedAt = handoverJudgedTime.Add(2 * time.Hour)
	if registered, err := fixture.handovers.handler.Register(t.Context(), pending); err != nil ||
		registered.Outcome() != application.HandoverRegistered {
		t.Fatalf("登记待确认：%v %s", err, registered.Outcome())
	}
	fixture.correctToHandedOver(t, "parcel-1", "handover-scope-2", "handover-result/parcel-1/pending", "handover-result/parcel-1/confirmed")
	asked := fixture.places.calls

	result := fixture.runVersion(t, "parcel-1", "handover-scope-2", "handover-result/parcel-1/confirmed")
	triggered := result.Trigger()
	if result.Outcome() != application.DispatchOnHandoverTriggered ||
		triggered.Outcome() != application.DeliveryDispatchNotTriggered ||
		triggered.Refusal() != application.DeliveryTriggerRefusedEntryNotThisHandover {
		t.Fatalf("outcome = %s / %s / %s, want DISPATCH_TRIGGERED / NOT_A_DELIVERY_TRIGGER / ENTRY_NOT_THIS_HANDOVER",
			result.Outcome(), triggered.Outcome(), triggered.Refusal())
	}
	if result.Disposition() != application.DispatchOnHandoverSettled {
		t.Fatalf("disposition = %s, want SETTLED", result.Disposition())
	}
	if fixture.places.calls != asked || fixture.tasks.saves != 0 {
		t.Fatalf("所有者被问 %d 次、任务写了 %d 次, want 都不动", fixture.places.calls-asked, fixture.tasks.saves)
	}

	// 对照：带对象进段的那一版照常形成任务——核的是「是不是这一版」，不是一律拒。
	own := fixture.runVersion(t, "parcel-1", "handover-scope-1", "handover-result/parcel-1/v1")
	if own.Trigger().Outcome() != application.DeliveryDispatchTaskFormed {
		t.Fatalf("带对象进段的那一版 = %s / %s, want DISPATCH_TASK_FORMED", own.Trigger().Outcome(), own.Trigger().Refusal())
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
