package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var (
	collabDecidedAt  = time.Date(2026, 8, 13, 12, 0, 0, 0, time.UTC)
	collabRecordedAt = time.Date(2026, 8, 13, 12, 30, 0, 0, time.UTC)
)

type acceptanceStoreDouble struct {
	records map[string]ports.CollaborationAcceptanceRecord
	findErr error
	saves   int
}

func newAcceptanceStore() *acceptanceStoreDouble {
	return &acceptanceStoreDouble{records: map[string]ports.CollaborationAcceptanceRecord{}}
}

func acceptanceKey(key ports.CollaborationAcceptanceKey) string {
	return key.TenantID.String() + "|" + key.Item.String()
}

func (double *acceptanceStoreDouble) FindByKey(
	_ context.Context,
	key ports.CollaborationAcceptanceKey,
) (ports.CollaborationAcceptanceRecord, bool, error) {
	if double.findErr != nil {
		return ports.CollaborationAcceptanceRecord{}, false, double.findErr
	}
	record, found := double.records[acceptanceKey(key)]
	return record, found, nil
}

func (double *acceptanceStoreDouble) Save(
	_ context.Context,
	record ports.CollaborationAcceptanceRecord,
) (ports.AcceptanceSaveOutcome, error) {
	double.saves++
	if _, exists := double.records[acceptanceKey(record.Key)]; exists {
		return ports.AcceptanceAlreadyDecided, nil
	}
	double.records[acceptanceKey(record.Key)] = record
	return ports.AcceptanceSaved, nil
}

type factStoreDouble struct {
	records map[string]ports.ExecutionFactRecord
	findErr error
	saves   int
}

func newFactStore() *factStoreDouble {
	return &factStoreDouble{records: map[string]ports.ExecutionFactRecord{}}
}

func factKey(key ports.ExecutionFactKey) string {
	return key.TenantID.String() + "|" + key.Item.String() + "|" + key.Unit.String() + "|" + key.Action.String()
}

func (double *factStoreDouble) FindByKey(
	_ context.Context,
	key ports.ExecutionFactKey,
) (ports.ExecutionFactRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExecutionFactRecord{}, false, double.findErr
	}
	record, found := double.records[factKey(key)]
	return record, found, nil
}

func (double *factStoreDouble) Save(
	_ context.Context,
	record ports.ExecutionFactRecord,
) (ports.ExecutionFactSaveOutcome, error) {
	double.saves++
	if _, exists := double.records[factKey(record.Key)]; exists {
		return ports.ExecutionFactAlreadyRecorded, nil
	}
	double.records[factKey(record.Key)] = record
	return ports.ExecutionFactSaved, nil
}

type acceptanceHandoffDouble struct {
	intents []ports.CollaborationAcceptanceHandoffIntent
	err     error
}

func (double *acceptanceHandoffDouble) HandOffCollaborationAcceptance(
	_ context.Context,
	intent ports.CollaborationAcceptanceHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type factHandoffDouble struct {
	intents []ports.ExecutionFactHandoffIntent
	err     error
}

func (double *factHandoffDouble) HandOffExecutionFact(
	_ context.Context,
	intent ports.ExecutionFactHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type collabClock struct{ at time.Time }

func (clock collabClock) Now() time.Time { return clock.at }

type collabFixture struct {
	acceptances *acceptanceStoreDouble
	facts       *factStoreDouble
	acknowledge *acceptanceHandoffDouble
	executions  *factHandoffDouble
	handler     *application.AcceptCollaborationHandler
}

func newCollabFixture(t *testing.T) *collabFixture {
	t.Helper()
	fixture := &collabFixture{
		acceptances: newAcceptanceStore(),
		facts:       newFactStore(),
		acknowledge: &acceptanceHandoffDouble{},
		executions:  &factHandoffDouble{},
	}
	fixture.handler = application.NewAcceptCollaborationHandler(application.AcceptCollaborationDeps{
		Acceptances: fixture.acceptances,
		Facts:       fixture.facts,
		Acknowledge: fixture.acknowledge,
		Executions:  fixture.executions,
		Clock:       collabClock{at: collabRecordedAt},
	})
	return fixture
}

func collabTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return tenant
}

func acceptCommand(t *testing.T) application.AcceptCollaborationCommand {
	t.Helper()
	return application.AcceptCollaborationCommand{
		TenantID:        collabTenant(t),
		Node:            "node-1",
		Item:            "collaboration-item-1",
		Decision:        domain.CollaborationAccepted,
		AcceptedUnits:   []string{"unit-1", "unit-2"},
		AcceptedActions: []domain.CollaborationActionKind{domain.UnsealAction, domain.PresentAction},
		Authority:       "node-authority-1",
		DecidedAt:       collabDecidedAt,
	}
}

func executeCommand(t *testing.T) application.RecordExecutionCommand {
	t.Helper()
	return application.RecordExecutionCommand{
		TenantID:    collabTenant(t),
		Item:        "collaboration-item-1",
		Unit:        "unit-1",
		Action:      domain.UnsealAction,
		Evidence:    "execution-evidence-1",
		PerformedAt: collabDecidedAt.Add(time.Hour),
	}
}

// 同一事项只决定一次：首决交意图，重放返原决定重发同一份，异决定同事项冲突不顶替。
// 点名 `AT-NO-008`「相同事项版本、对象、动作和请求重复到达→返回已有承接」与
// `AT-NO-009`「同一请求身份携带不同对象、动作或范围→形成节点协作冲突……不按最后
// 到达覆盖」。
func TestOneDecisionPerCollaborationItem(t *testing.T) {
	fixture := newCollabFixture(t)
	command := acceptCommand(t)

	first, err := fixture.handler.Accept(context.Background(), command)
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	if first.Outcome() != application.CollaborationDecided {
		t.Fatalf("outcome = %q, want COLLABORATION_DECIDED", first.Outcome())
	}
	if len(fixture.acknowledge.intents) != 1 {
		t.Fatalf("intents = %d, want 1（承接结果是 CC 协作链的回执信号）", len(fixture.acknowledge.intents))
	}

	replay, err := fixture.handler.Accept(context.Background(), command)
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.CollaborationExistingDecision {
		t.Fatalf("outcome = %q, want EXISTING_DECISION", replay.Outcome())
	}
	if fixture.acceptances.saves != 1 || len(fixture.acknowledge.intents) != 2 {
		t.Fatalf("saves = %d intents = %d（重放不重存、重发同一份）", fixture.acceptances.saves, len(fixture.acknowledge.intents))
	}

	t.Run("a different decision for the same item is a conflict", func(t *testing.T) {
		flipped := acceptCommand(t)
		flipped.Decision = domain.CollaborationDeclined
		flipped.AcceptedUnits = nil
		flipped.AcceptedActions = nil
		flipped.Basis = "capacity-shortage-1"
		result, err := fixture.handler.Accept(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict accept: %v", err)
		}
		if result.Outcome() != application.CollaborationDecisionConflict {
			t.Fatalf("outcome = %q, want DECISION_CONFLICT（改主意走事项方重派，不顶替）", result.Outcome())
		}
		record, _ := fixture.handler.Accept(context.Background(), command)
		acceptance, _ := record.Acceptance()
		if acceptance.Acceptance.Decision() != domain.CollaborationAccepted {
			t.Fatal("冲突覆盖了原决定")
		}
	})

	t.Run("a malformed decision is not accepted", func(t *testing.T) {
		broken := acceptCommand(t)
		broken.Decision = domain.CollaborationDeclined // 拒接却带范围：领域把门
		result, err := fixture.handler.Accept(context.Background(), broken)
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if result.Outcome() != application.CollaborationNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
	})

	t.Run("a store failure is undecided", func(t *testing.T) {
		fixture := newCollabFixture(t)
		fixture.acceptances.findErr = errors.New("store down")
		result, err := fixture.handler.Accept(context.Background(), acceptCommand(t))
		if err != nil {
			t.Fatalf("accept: %v", err)
		}
		if result.Outcome() != application.CollaborationUndecided ||
			result.UndecidedReason() != application.AcceptanceStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})
}

// 执行事实登记：承接为界（拒接/越权各归业务负向格，领域把门编排分格），幂等按
// （事项+实物+动作），意图交 CC 处置执行核对。点名 `AT-NO-008` 的不重复累计半边
// （同键重放不重存）与 `AT-NO-012` 的冲突半边「多个合格来源相互冲突→保留原事实……
// 不按最后到达覆盖」（同键异证据成冲突格）。
func TestExecutionFactsAreRecordedWithinTheDecision(t *testing.T) {
	fixture := newCollabFixture(t)
	if _, err := fixture.handler.Accept(context.Background(), acceptCommand(t)); err != nil {
		t.Fatalf("accept: %v", err)
	}

	first, err := fixture.handler.RecordExecution(context.Background(), executeCommand(t))
	if err != nil {
		t.Fatalf("record execution: %v", err)
	}
	if first.Outcome() != application.ExecutionRecorded {
		t.Fatalf("outcome = %q, want EXECUTION_RECORDED", first.Outcome())
	}
	if len(fixture.executions.intents) != 1 {
		t.Fatalf("intents = %d, want 1（CC ExecutionFactView 的上游源）", len(fixture.executions.intents))
	}

	replay, err := fixture.handler.RecordExecution(context.Background(), executeCommand(t))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.ExecutionExistingFact || fixture.facts.saves != 1 {
		t.Fatalf("outcome = %q saves = %d", replay.Outcome(), fixture.facts.saves)
	}

	t.Run("a different evidence under the same key is a conflict", func(t *testing.T) {
		flipped := executeCommand(t)
		flipped.Evidence = "execution-evidence-2"
		result, err := fixture.handler.RecordExecution(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict record: %v", err)
		}
		if result.Outcome() != application.ExecutionFactConflict {
			t.Fatalf("outcome = %q, want FACT_CONFLICT", result.Outcome())
		}
	})

	t.Run("an out-of-scope unit is its own grid", func(t *testing.T) {
		outside := executeCommand(t)
		outside.Unit = "unit-9"
		result, err := fixture.handler.RecordExecution(context.Background(), outside)
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if result.Outcome() != application.ExecutionOutsideScope {
			t.Fatalf("outcome = %q, want OUTSIDE_ACCEPTED_SCOPE（恢复动作是扩承接范围）", result.Outcome())
		}
		if len(fixture.facts.records) != 1 {
			t.Fatal("越权执行落了库")
		}
	})

	t.Run("an action outside the accepted set is the same grid", func(t *testing.T) {
		outside := executeCommand(t)
		outside.Action = domain.IsolateAction
		result, err := fixture.handler.RecordExecution(context.Background(), outside)
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if result.Outcome() != application.ExecutionOutsideScope {
			t.Fatalf("outcome = %q", result.Outcome())
		}
	})

	t.Run("a declined item has nothing to execute", func(t *testing.T) {
		declinedFixture := newCollabFixture(t)
		declined := acceptCommand(t)
		declined.Item = "collaboration-item-2"
		declined.Decision = domain.CollaborationDeclined
		declined.AcceptedUnits = nil
		declined.AcceptedActions = nil
		declined.Basis = "capacity-shortage-1"
		if _, err := declinedFixture.handler.Accept(context.Background(), declined); err != nil {
			t.Fatalf("accept declined: %v", err)
		}
		execute := executeCommand(t)
		execute.Item = "collaboration-item-2"
		result, err := declinedFixture.handler.RecordExecution(context.Background(), execute)
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if result.Outcome() != application.ExecutionOnDeclinedItem {
			t.Fatalf("outcome = %q, want ITEM_DECLINED（恢复动作是重新承接）", result.Outcome())
		}
	})

	t.Run("execution without a decision is not accepted", func(t *testing.T) {
		bare := newCollabFixture(t)
		result, err := bare.handler.RecordExecution(context.Background(), executeCommand(t))
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if result.Outcome() != application.CollaborationNotAccepted {
			t.Fatalf("outcome = %q; 没有承接决定就没有可执行的范围", result.Outcome())
		}
	})

	t.Run("a fact handoff failure keeps the outcome and is resent", func(t *testing.T) {
		fresh := newCollabFixture(t)
		if _, err := fresh.handler.Accept(context.Background(), acceptCommand(t)); err != nil {
			t.Fatalf("accept: %v", err)
		}
		fresh.executions.err = errors.New("downstream unavailable")
		first, err := fresh.handler.RecordExecution(context.Background(), executeCommand(t))
		if err != nil {
			t.Fatalf("record: %v", err)
		}
		if first.Outcome() != application.ExecutionRecorded || first.HandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.HandoffReference())
		}
		fresh.executions.err = nil
		replay, err := fresh.handler.RecordExecution(context.Background(), executeCommand(t))
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.HandoffReference() != "" || len(fresh.executions.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q（重放重发同一份）", len(fresh.executions.intents), replay.HandoffReference())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.CollaborationUndecidedReason{
			application.AcceptanceStoreUnavailable, application.FactStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 2 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.CollaborationUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第三个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}
