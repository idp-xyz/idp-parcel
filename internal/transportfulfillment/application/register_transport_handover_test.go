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

var (
	handoverJudgedTime   = time.Date(2026, 8, 13, 14, 0, 0, 0, time.UTC)
	handoverRegisteredAt = time.Date(2026, 8, 13, 14, 30, 0, 0, time.UTC)
)

type handoverRegistryDouble struct {
	records     map[string]ports.TransportHandoverRecord
	findErr     error
	saveErr     error
	saveResult  ports.HandoverSaveOutcome
	forceResult bool
	saves       int
}

func newHandoverRegistry() *handoverRegistryDouble {
	return &handoverRegistryDouble{records: map[string]ports.TransportHandoverRecord{}}
}

func handoverRegistryKey(key ports.TransportHandoverKey) string {
	return key.TenantID.String() + "|" + key.Object.String() + "|" + key.Scope.String() + "|" + key.Version.String()
}

func (double *handoverRegistryDouble) FindByKey(
	_ context.Context,
	key ports.TransportHandoverKey,
) (ports.TransportHandoverRecord, bool, error) {
	if double.findErr != nil {
		return ports.TransportHandoverRecord{}, false, double.findErr
	}
	record, found := double.records[handoverRegistryKey(key)]
	return record, found, nil
}

func (double *handoverRegistryDouble) Save(
	_ context.Context,
	record ports.TransportHandoverRecord,
) (ports.HandoverSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.HandoverSaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[handoverRegistryKey(record.Key)]; exists {
		return ports.HandoverAlreadyRegistered, nil
	}
	double.records[handoverRegistryKey(record.Key)] = record
	return ports.HandoverSaved, nil
}

type handoverHandoffDouble struct {
	intents []ports.TransportHandoverRegistrationIntent
	err     error
}

func (double *handoverHandoffDouble) HandOffTransportHandover(
	_ context.Context,
	intent ports.TransportHandoverRegistrationIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type handoverClock struct{ at time.Time }

func (clock handoverClock) Now() time.Time { return clock.at }

type handoverFixture struct {
	registry *handoverRegistryDouble
	handoff  *handoverHandoffDouble
	handler  *application.RegisterTransportHandoverHandler
}

func newHandoverFixture(t *testing.T) *handoverFixture {
	t.Helper()
	fixture := &handoverFixture{
		registry: newHandoverRegistry(),
		handoff:  &handoverHandoffDouble{},
	}
	fixture.handler = application.NewRegisterTransportHandoverHandler(application.RegisterTransportHandoverDeps{
		Handovers:  fixture.registry,
		Downstream: fixture.handoff,
		Clock:      handoverClock{at: handoverRegisteredAt},
	})
	return fixture
}

func registerHandoverCommand(t *testing.T) application.RegisterTransportHandoverCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.RegisterTransportHandoverCommand{
		TenantID:          tenant,
		Object:            "parcel-1",
		Scope:             "handover-scope-1",
		ReleasedBy:        "node-1",
		ReceivedBy:        "carrier-1",
		Verdict:           domain.ObjectHandedOver,
		ReleasingEvidence: "evidence-release-1",
		ReceivingEvidence: "evidence-receive-1",
		Rule:              "handover-rule/v1",
		Version:           "handover-result/parcel-1/v1",
		JudgedAt:          handoverJudgedTime,
	}
}

// 同一判断版本只登一次：首登交意图（一份，NO/NR 自分），重放返原重发，异裁决同键
// 冲突不顶替（改判走更正入口换新版）。
func TestAHandoverVersionRegistersOnce(t *testing.T) {
	fixture := newHandoverFixture(t)
	command := registerHandoverCommand(t)

	first, err := fixture.handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if first.Outcome() != application.HandoverRegistered {
		t.Fatalf("outcome = %q, want HANDOVER_REGISTERED", first.Outcome())
	}
	record, _ := first.Record()
	if reference, ok := record.Handover.TransferOutBasis(); !ok || reference == "" {
		t.Fatal("已交接的登记丢了转出引用——NO 侧无从消费")
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}

	replay, err := fixture.handler.Register(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.HandoverExistingVersion || fixture.registry.saves != 1 {
		t.Fatalf("outcome = %q saves = %d", replay.Outcome(), fixture.registry.saves)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（重放重发同一份）", len(fixture.handoff.intents))
	}

	t.Run("a different verdict under the same version is a conflict", func(t *testing.T) {
		flipped := registerHandoverCommand(t)
		flipped.Verdict = domain.HandoverRefused
		flipped.ReleasingEvidence = ""
		flipped.ReceivingEvidence = ""
		flipped.Rule = ""
		flipped.Basis = "refusal-basis-1"
		result, err := fixture.handler.Register(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict register: %v", err)
		}
		if result.Outcome() != application.HandoverRegistrationConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（改判走更正换新版，不按最后到达顶替）", result.Outcome())
		}
	})

	t.Run("a verdict flip alone under the same version is still a conflict", func(t *testing.T) {
		// REFUSED 与 PENDING_CONFIRMATION 的完备性同形（依据必备、证据可选）——只翻
		// 裁决、其余字段一字不动的提交必须撞冲突，指纹不带裁决就会放过它。
		base := registerHandoverCommand(t)
		base.Object = "parcel-flip"
		base.Verdict = domain.HandoverRefused
		base.ReleasingEvidence = ""
		base.ReceivingEvidence = ""
		base.Rule = ""
		base.Basis = "contested-basis-1"
		base.Version = "handover-result/parcel-flip/v1"
		if _, err := fixture.handler.Register(context.Background(), base); err != nil {
			t.Fatalf("register refused: %v", err)
		}
		flipped := base
		flipped.Verdict = domain.HandoverPendingConfirmation
		result, err := fixture.handler.Register(context.Background(), flipped)
		if err != nil {
			t.Fatalf("register flipped: %v", err)
		}
		if result.Outcome() != application.HandoverRegistrationConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（只翻裁决也是另一份内容）", result.Outcome())
		}
	})

	t.Run("a refused judgment registers as its own fact", func(t *testing.T) {
		refused := registerHandoverCommand(t)
		refused.Object = "parcel-2"
		refused.Verdict = domain.HandoverRefused
		refused.ReleasingEvidence = ""
		refused.ReceivingEvidence = ""
		refused.Rule = ""
		refused.Basis = "refusal-basis-1"
		refused.Version = "handover-result/parcel-2/v1"
		result, err := fixture.handler.Register(context.Background(), refused)
		if err != nil {
			t.Fatalf("register refused: %v", err)
		}
		if result.Outcome() != application.HandoverRegistered {
			t.Fatalf("outcome = %q（拒收也是下游要看的判断）", result.Outcome())
		}
		record, _ := result.Record()
		if _, leaks := record.Handover.TransferOutBasis(); leaks {
			t.Fatal("拒收登记交出了转出引用")
		}
	})

	t.Run("an incomplete handed-over judgment is not accepted", func(t *testing.T) {
		broken := registerHandoverCommand(t)
		broken.ReceivingEvidence = ""
		result, err := fixture.handler.Register(context.Background(), broken)
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.Outcome() != application.HandoverNotAccepted {
			t.Fatalf("outcome = %q（单侧证据成不了已交接，领域把门编排不绕）", result.Outcome())
		}
	})
}

// 更正走领域版本链：新版回指前版并以新键登记、意图重新交付；没有可更正的判断更正不出
// 交接；同版本覆盖被领域拒。
func TestAHandoverCorrectionRegistersTheNewVersion(t *testing.T) {
	fixture := newHandoverFixture(t)
	if _, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t)); err != nil {
		t.Fatalf("register: %v", err)
	}
	tenant, _ := domain.NewTenantID("tenant-1")

	corrected, err := fixture.handler.Correct(context.Background(), application.CorrectTransportHandoverCommand{
		TenantID:           tenant,
		Object:             "parcel-1",
		Scope:              "handover-scope-1",
		PredecessorVersion: "handover-result/parcel-1/v1",
		Verdict:            domain.HandoverRefused,
		Basis:              "late-correction-1",
		NewVersion:         "handover-result/parcel-1/v2",
		CorrectedAt:        handoverJudgedTime.Add(24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if corrected.Outcome() != application.HandoverCorrected {
		t.Fatalf("outcome = %q, want HANDOVER_CORRECTED", corrected.Outcome())
	}
	record, _ := corrected.Record()
	predecessor, present := record.Handover.Corrects()
	if !present || predecessor.String() != "handover-result/parcel-1/v1" {
		t.Fatalf("corrects = %q present=%v（新版回指前版）", predecessor, present)
	}
	if record.Key.Version.String() != "handover-result/parcel-1/v2" {
		t.Fatalf("key version = %q（更正以新版本键登记）", record.Key.Version)
	}
	if len(fixture.registry.records) != 2 {
		t.Fatalf("records = %d, want 2（原判断保留）", len(fixture.registry.records))
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（更正版本重新交付下游）", len(fixture.handoff.intents))
	}

	t.Run("correcting an absent judgment is refused", func(t *testing.T) {
		missing, err := fixture.handler.Correct(context.Background(), application.CorrectTransportHandoverCommand{
			TenantID:           tenant,
			Object:             "parcel-9",
			Scope:              "handover-scope-1",
			PredecessorVersion: "handover-result/parcel-9/v1",
			Verdict:            domain.HandoverRefused,
			Basis:              "late-correction-1",
			NewVersion:         "handover-result/parcel-9/v2",
			CorrectedAt:        handoverJudgedTime.Add(24 * time.Hour),
		})
		if err != nil {
			t.Fatalf("correct absent: %v", err)
		}
		if missing.Outcome() != application.HandoverNotAccepted {
			t.Fatalf("outcome = %q; 更正出了无中生有的交接", missing.Outcome())
		}
	})

	t.Run("a correction to handed-over still needs both evidences", func(t *testing.T) {
		incomplete, err := fixture.handler.Correct(context.Background(), application.CorrectTransportHandoverCommand{
			TenantID:           tenant,
			Object:             "parcel-1",
			Scope:              "handover-scope-1",
			PredecessorVersion: "handover-result/parcel-1/v1",
			Verdict:            domain.ObjectHandedOver,
			ReleasingEvidence:  "evidence-release-recheck",
			NewVersion:         "handover-result/parcel-1/v3",
			CorrectedAt:        handoverJudgedTime.Add(48 * time.Hour),
		})
		if err != nil {
			t.Fatalf("correct incomplete: %v", err)
		}
		if incomplete.Outcome() != application.HandoverNotAccepted {
			t.Fatalf("outcome = %q（更正成已交接的完备性不得低于首次裁决）", incomplete.Outcome())
		}
	})
}

// 恢复纪律：库故障未决；投递失败不翻结果重放重发；写入代数外是编程错误；原因集封闭。
func TestHandoverRegistrationRecoveryDiscipline(t *testing.T) {
	t.Run("a registry failure is undecided", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		fixture.registry.findErr = errors.New("registry down")
		result, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if result.Outcome() != application.HandoverUndecided ||
			result.UndecidedReason() != application.HandoverRegistryUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		first, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
		if err != nil {
			t.Fatalf("register: %v", err)
		}
		if first.Outcome() != application.HandoverRegistered || first.HandoverHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.HandoverHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.HandoverHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q（重放重发同一份）", len(fixture.handoff.intents), replay.HandoverHandoffReference())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		fixture.registry.forceResult = true
		fixture.registry.saveResult = ports.HandoverSaveOutcome(99)
		if _, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t)); !errors.Is(err, application.ErrUnexpectedHandoverSave) {
			t.Fatalf("error = %v, want ErrUnexpectedHandoverSave", err)
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		if application.HandoverRegistryUnavailable.String() == "" {
			t.Fatal("reason has no label")
		}
		if application.HandoverRegistrationUndecidedReason(2).String() != "" {
			t.Fatal("第二个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
