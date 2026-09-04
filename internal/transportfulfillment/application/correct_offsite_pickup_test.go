package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

var pickupCorrectedAt = pickupRegisteredAt.Add(36 * time.Hour)

func pickupCorrectionCommand(t *testing.T, predecessor string) application.CorrectOffsitePickupCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.CorrectOffsitePickupCommand{
		TenantID:           tenant,
		Object:             "parcel-1",
		Attempt:            "attempt-1",
		PredecessorVersion: predecessor,
		Place:              "customer-warehouse-2",
		Control:            "TRANSPORT-CONTROL/TF-1-RECHECK",
		ExecutedBy:         "courier-2",
		OccurredAt:         pickupOccurredAt.Add(-2 * time.Hour),
		CorrectedAt:        pickupCorrectedAt,
	}
}

// seedRegisteredPickup 先登一版，交回它的记录键；更正用例都从一份已登记的揽收起步。
func seedRegisteredPickup(t *testing.T, fixture *pickupRegFixture) ports.OffsitePickupKey {
	t.Helper()
	registered, err := fixture.handler.Register(context.Background(), pickupRegistrationCommand(t))
	if err != nil {
		t.Fatalf("seed register: %v", err)
	}
	if registered.Outcome() != application.PickupRegistered {
		t.Fatalf("seed outcome = %q", registered.Outcome())
	}
	record, _ := registered.Record()
	return record.Key
}

// Covers: 票 tf-segment-lifecycle-closure/08 裁决 A 的编排面——读回前版 → 领域 Correct → 以新版本落新行 →
// 意图重新交 parcel-shipment 采认（UC-PS-003「来源更正形成新的采用判断版本」的上游），不进段（段侧
// 重派生另立票）。新版本号由编排签发，同首登；原版本留在册上一字不动。
func TestAPickupCorrectionFormsANewVersionAndResubmitsTheIntent(t *testing.T) {
	fixture := newPickupRegFixture(t)
	key := seedRegisteredPickup(t, fixture)

	result, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.PickupCorrected {
		t.Fatalf("outcome = %q, want PICKUP_CORRECTED", result.Outcome())
	}
	record, present := result.Record()
	if !present {
		t.Fatal("更正成功却没有交回记录")
	}
	if record.Pickup.Version().String() != "pickup-result/v2" {
		t.Fatalf("version = %q, want v2（新版本由编排签发）", record.Pickup.Version())
	}
	if predecessor, corrected := record.Pickup.Corrects(); !corrected || predecessor.String() != "pickup-result/v1" {
		t.Fatalf("corrects = %q present=%v, want v1", predecessor, corrected)
	}
	if record.Pickup.Control().String() != "TRANSPORT-CONTROL/TF-1-RECHECK" ||
		record.Pickup.Place().String() != "customer-warehouse-2" ||
		record.Pickup.ExecutedBy().String() != "courier-2" ||
		!record.Pickup.OccurredAt().Equal(pickupOccurredAt.Add(-2*time.Hour)) {
		t.Fatalf("更正给出的四格没有进新版本：%+v", record.Pickup)
	}
	if record.Pickup.Object().String() != "parcel-1" || record.Pickup.Task().String() != "pickup-task-1" || record.Pickup.Attempt().String() != "attempt-1" {
		t.Fatal("对象、任务、尝试必须沿用被更正版本")
	}
	if record.Key != key {
		t.Fatalf("更正版本换了记录键：%+v", record.Key)
	}
	if !record.RecordedAt.Equal(pickupRegisteredAt) {
		t.Fatalf("recorded at = %s, want 编排时钟", record.RecordedAt)
	}

	chain := fixture.registry.chainOf(t, key)
	if len(chain) != 2 {
		t.Fatalf("chain = %d, want 2（原版本保留，更正是新行）", len(chain))
	}
	if chain[0].Pickup.Control().String() != "TRANSPORT-CONTROL/TF-1" {
		t.Fatal("更正改写了原版本")
	}
	if _, corrected := chain[0].Pickup.Corrects(); corrected {
		t.Fatal("原版本被反向打上了更正标记")
	}
	current, found, err := fixture.registry.FindByKey(context.Background(), key)
	if err != nil || !found || current.Pickup.Version().String() != "pickup-result/v2" {
		t.Fatalf("按键读回的当前版 = %+v found=%v err=%v, want v2", current.Pickup, found, err)
	}

	if fixture.versions.minted != 2 {
		t.Fatalf("minted = %d, want 2（首登一版、更正一版）", fixture.versions.minted)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（更正版本要重新交 PS 采认）", len(fixture.handoff.intents))
	}
	if resent := fixture.handoff.intents[1].Record; resent.Pickup.Version().String() != "pickup-result/v2" {
		t.Fatalf("重交的意图带的是 %q，want v2", resent.Pickup.Version())
	}

	t.Run("replaying the same correction returns the corrected version without minting", func(t *testing.T) {
		replay, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.PickupExistingVersion {
			t.Fatalf("outcome = %q, want EXISTING_VERSION", replay.Outcome())
		}
		existing, _ := replay.Record()
		if existing.Pickup.Version().String() != "pickup-result/v2" {
			t.Fatalf("replay record = %q, want v2", existing.Pickup.Version())
		}
		if fixture.versions.minted != 2 || len(fixture.registry.chainOf(t, key)) != 2 {
			t.Fatalf("minted = %d chain = %d（重放不重签不重存）", fixture.versions.minted, len(fixture.registry.chainOf(t, key)))
		}
		if len(fixture.handoff.intents) != 3 {
			t.Fatalf("intents = %d, want 3（重放重发同一份）", len(fixture.handoff.intents))
		}
	})

	t.Run("a different correction naming a superseded version is a conflict", func(t *testing.T) {
		stale := pickupCorrectionCommand(t, "pickup-result/v1")
		stale.Control = "TRANSPORT-CONTROL/TF-1-THIRD-OPINION"
		result, err := fixture.handler.Correct(context.Background(), stale)
		if err != nil {
			t.Fatalf("stale correction: %v", err)
		}
		if result.Outcome() != application.PickupRegistrationConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（v1 已被 v2 更正为不同内容——要改请对当前版提更正）", result.Outcome())
		}
		if fixture.versions.minted != 2 {
			t.Fatal("冲突还签了新版本")
		}
	})

	t.Run("the current version can be corrected again and the chain keeps every predecessor", func(t *testing.T) {
		third, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v2"))
		if err != nil {
			t.Fatalf("second correction: %v", err)
		}
		if third.Outcome() != application.PickupCorrected {
			t.Fatalf("outcome = %q, want PICKUP_CORRECTED", third.Outcome())
		}
		record, _ := third.Record()
		if predecessor, _ := record.Pickup.Corrects(); predecessor.String() != "pickup-result/v2" || record.Pickup.Version().String() != "pickup-result/v3" {
			t.Fatalf("chain broke: %q corrects %q", record.Pickup.Version(), predecessor)
		}
		if len(fixture.registry.chainOf(t, key)) != 3 {
			t.Fatalf("chain = %d, want 3", len(fixture.registry.chainOf(t, key)))
		}
	})

	t.Run("a first registration replayed after a correction no longer matches the current content", func(t *testing.T) {
		replay, err := fixture.handler.Register(context.Background(), pickupRegistrationCommand(t))
		if err != nil {
			t.Fatalf("register replay: %v", err)
		}
		if replay.Outcome() != application.PickupRegistrationConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（首登内容已被更正取代，重放不再是同一份）", replay.Outcome())
		}
	})
}

// Covers: 裁决「无前版则未受理，更正不出无中生有的揽收」——没有可更正的登记时不签版本、不落库、不交意图。
func TestAPickupCorrectionNeedsARegisteredPredecessor(t *testing.T) {
	fixture := newPickupRegFixture(t)

	result, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.PickupRegistrationNotAccepted {
		t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
	}
	if fixture.versions.minted != 0 || fixture.registry.saves != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("无中生有：minted=%d saves=%d intents=%d", fixture.versions.minted, fixture.registry.saves, len(fixture.handoff.intents))
	}
}

// Covers: 裁决「每格完备性同首登（控制依据仍必备）」的编排面——缺四格任一、缺更正时刻或指名不全在受理处
// 即未受理，编排不绕领域构造器；更正不能把一次揽收更正成一次失败到访。
func TestAPickupCorrectionIsAsCompleteAsAFirstRegistration(t *testing.T) {
	broken := map[string]func(*application.CorrectOffsitePickupCommand){
		"no control":      func(command *application.CorrectOffsitePickupCommand) { command.Control = " " },
		"no place":        func(command *application.CorrectOffsitePickupCommand) { command.Place = " " },
		"no executor":     func(command *application.CorrectOffsitePickupCommand) { command.ExecutedBy = " " },
		"no time":         func(command *application.CorrectOffsitePickupCommand) { command.OccurredAt = time.Time{} },
		"no corrected at": func(command *application.CorrectOffsitePickupCommand) { command.CorrectedAt = time.Time{} },
		"no predecessor":  func(command *application.CorrectOffsitePickupCommand) { command.PredecessorVersion = " " },
		"no object":       func(command *application.CorrectOffsitePickupCommand) { command.Object = " " },
		"no attempt":      func(command *application.CorrectOffsitePickupCommand) { command.Attempt = " " },
		"no tenant":       func(command *application.CorrectOffsitePickupCommand) { command.TenantID = domain.TenantID{} },
	}
	for name, breakCommand := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newPickupRegFixture(t)
			key := seedRegisteredPickup(t, fixture)
			command := pickupCorrectionCommand(t, "pickup-result/v1")
			breakCommand(&command)

			result, err := fixture.handler.Correct(context.Background(), command)
			if err != nil {
				t.Fatalf("correct: %v", err)
			}
			if result.Outcome() != application.PickupRegistrationNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
			if fixture.versions.minted != 1 || len(fixture.registry.chainOf(t, key)) != 1 || len(fixture.handoff.intents) != 1 {
				t.Fatal("未受理的更正签了版本、落了库或交了意图")
			}
		})
	}
}

// Covers: 裁决「更正时刻不得早于被更正版本的登记时刻」——下界取登记时刻而不取发生时刻：发生时刻本身是
// 可更正的四格之一，被更正的那一版可能恰恰把它记晚了。
func TestAPickupCorrectionCannotPredateTheRegistrationItCorrects(t *testing.T) {
	fixture := newPickupRegFixture(t)
	seedRegisteredPickup(t, fixture)

	early := pickupCorrectionCommand(t, "pickup-result/v1")
	early.CorrectedAt = pickupRegisteredAt.Add(-time.Minute)
	result, err := fixture.handler.Correct(context.Background(), early)
	if err != nil {
		t.Fatalf("early correction: %v", err)
	}
	if result.Outcome() != application.PickupRegistrationNotAccepted {
		t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED（更正不可能发生在被更正的登记之前）", result.Outcome())
	}
	if fixture.versions.minted != 1 {
		t.Fatal("被拒的更正签了新版本")
	}

	t.Run("a correction at the registration instant is accepted", func(t *testing.T) {
		atRegistration := pickupCorrectionCommand(t, "pickup-result/v1")
		atRegistration.CorrectedAt = pickupRegisteredAt
		result, err := fixture.handler.Correct(context.Background(), atRegistration)
		if err != nil {
			t.Fatalf("correction at registration: %v", err)
		}
		if result.Outcome() != application.PickupCorrected {
			t.Fatalf("outcome = %q, want PICKUP_CORRECTED", result.Outcome())
		}
	})

	t.Run("an occurrence earlier than the original registration is still correctable", func(t *testing.T) {
		// 首登把发生时刻记在登记之后（记错了）；更正把它改回登记之前。以发生时刻为下界会把这次正当更正拒掉。
		fixture := newPickupRegFixture(t)
		late := pickupRegistrationCommand(t)
		late.OccurredAt = pickupRegisteredAt.Add(48 * time.Hour)
		if _, err := fixture.handler.Register(context.Background(), late); err != nil {
			t.Fatalf("seed: %v", err)
		}
		command := pickupCorrectionCommand(t, "pickup-result/v1")
		command.OccurredAt = pickupRegisteredAt.Add(-time.Hour)
		command.CorrectedAt = pickupRegisteredAt.Add(time.Hour)
		result, err := fixture.handler.Correct(context.Background(), command)
		if err != nil {
			t.Fatalf("correct: %v", err)
		}
		if result.Outcome() != application.PickupCorrected {
			t.Fatalf("outcome = %q, want PICKUP_CORRECTED", result.Outcome())
		}
	})
}

// Covers: 更正编排的恢复纪律——依赖故障各归其因；意图投递失败更正不翻、重放重发同一份；并发落败读回
// 赢家（同一前版被另一方先更正）；写入代数外是编程错误。与首登那一侧同一套。
func TestPickupCorrectionRecoveryDiscipline(t *testing.T) {
	t.Run("dependency failures are undecided with their reasons", func(t *testing.T) {
		registry := newPickupRegFixture(t)
		seedRegisteredPickup(t, registry)
		registry.registry.findErr = errors.New("registry down")
		result, err := registry.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
		if err != nil {
			t.Fatalf("correct: %v", err)
		}
		if result.Outcome() != application.PickupRegistrationUndecided || result.UndecidedReason() != application.PickupRegistryUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("未决没有续办引用")
		}

		factory := newPickupRegFixture(t)
		seedRegisteredPickup(t, factory)
		factory.versions.err = errors.New("factory down")
		result, err = factory.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
		if err != nil {
			t.Fatalf("correct: %v", err)
		}
		if result.Outcome() != application.PickupRegistrationUndecided || result.UndecidedReason() != application.PickupVersionUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newPickupRegFixture(t)
		seedRegisteredPickup(t, fixture)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
		if err != nil {
			t.Fatalf("correct: %v", err)
		}
		if first.Outcome() != application.PickupCorrected || first.PickupHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.PickupHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.Outcome() != application.PickupExistingVersion || replay.PickupHandoffReference() != "" {
			t.Fatalf("outcome = %q handoff = %q（重放重发同一份）", replay.Outcome(), replay.PickupHandoffReference())
		}
		if resent := fixture.handoff.intents[len(fixture.handoff.intents)-1].Record; resent.Pickup.Version().String() != "pickup-result/v2" {
			t.Fatalf("重发的意图带的是 %q，want v2", resent.Pickup.Version())
		}
	})

	t.Run("a concurrent loser reads back the winner", func(t *testing.T) {
		fixture := newPickupRegFixture(t)
		seedRegisteredPickup(t, fixture)
		if _, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1")); err != nil {
			t.Fatalf("winner: %v", err)
		}
		// 强制 Save 撞 AlreadyRegistered，并让前版核对通不过之前先读回：这里直接模拟「核对时 v1 还是当前版、
		// 落库时另一方已把 v1 更正成 v2」那一格——读回赢家 v2。
		fixture.registry.forceResult = true
		fixture.registry.saveResult = ports.OffsitePickupAlreadyRegistered
		loser := pickupCorrectionCommand(t, "pickup-result/v2")
		result, err := fixture.handler.Correct(context.Background(), loser)
		if err != nil {
			t.Fatalf("loser: %v", err)
		}
		if result.Outcome() != application.PickupExistingVersion {
			t.Fatalf("outcome = %q, want EXISTING_VERSION", result.Outcome())
		}
		if winner, _ := result.Record(); winner.Pickup.Version().String() != "pickup-result/v2" {
			t.Fatalf("读回的赢家 = %q, want v2", winner.Pickup.Version())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newPickupRegFixture(t)
		seedRegisteredPickup(t, fixture)
		fixture.registry.forceResult = true
		fixture.registry.saveResult = ports.OffsitePickupSaveOutcome(99)
		if _, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1")); !errors.Is(err, application.ErrUnexpectedPickupRegistrySave) {
			t.Fatalf("error = %v, want ErrUnexpectedPickupRegistrySave", err)
		}
	})
}

// Covers: 裁决附问——段侧「来源更正 → 参与关系重派生」不在本票；更正编排照交接那一侧的现状，落新版本 +
// 重交意图，不碰段登记册（段登记册在场也不碰）。
func TestAPickupCorrectionLeavesTheSegmentRegistryAlone(t *testing.T) {
	fixture := newPickupSegmentFixture(t)
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"
	if _, err := fixture.handler.Register(context.Background(), command); err != nil {
		t.Fatalf("seed: %v", err)
	}
	savesBefore, joinsBefore := fixture.segments.saves, fixture.segments.joins

	result, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.PickupCorrected {
		t.Fatalf("outcome = %q, want PICKUP_CORRECTED", result.Outcome())
	}
	if fixture.segments.saves != savesBefore || fixture.segments.joins != joinsBefore {
		t.Fatalf("更正动了段登记册：saves %d→%d joins %d→%d", savesBefore, fixture.segments.saves, joinsBefore, fixture.segments.joins)
	}
	if result.SegmentContinuationReference() != "" || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("更正答了段那一半：%q %q", result.SegmentContinuationReference(), result.SegmentEntryRefusal())
	}
}
