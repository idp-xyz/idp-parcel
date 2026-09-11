package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var (
	resultOccurredAt = time.Date(2026, 8, 12, 16, 0, 0, 0, time.UTC)
	resultRecordedAt = time.Date(2026, 8, 12, 16, 30, 0, 0, time.UTC)
)

type resultStoreDouble struct {
	records     map[string]ports.ExternalResultRecord
	layerFacts  []domain.ExternalResult
	findErr     error
	factsErr    error
	saveErr     error
	saveResult  ports.ExternalResultSaveOutcome
	forceResult bool
	saves       int
}

func newResultStore() *resultStoreDouble {
	return &resultStoreDouble{records: map[string]ports.ExternalResultRecord{}}
}

func resultKey(key ports.ExternalResultKey) string {
	return key.TenantID.String() + "|" + key.SourceID
}

func (double *resultStoreDouble) FindByKey(
	_ context.Context,
	key ports.ExternalResultKey,
) (ports.ExternalResultRecord, bool, error) {
	if double.findErr != nil {
		return ports.ExternalResultRecord{}, false, double.findErr
	}
	record, found := double.records[resultKey(key)]
	return record, found, nil
}

func (double *resultStoreDouble) Save(
	_ context.Context,
	record ports.ExternalResultRecord,
) (ports.ExternalResultSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.ExternalResultSaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[resultKey(record.Key)]; exists {
		return ports.ExternalResultAlreadyRecorded, nil
	}
	double.records[resultKey(record.Key)] = record
	return ports.ExternalResultSaved, nil
}

func (double *resultStoreDouble) LoadForSubmission(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SubmissionVersionID,
) ([]domain.ExternalResult, error) {
	if double.factsErr != nil {
		return nil, double.factsErr
	}
	return double.layerFacts, nil
}

type submissionIndexDouble struct {
	found bool
	err   error
}

func (double *submissionIndexDouble) FindSubmission(
	_ context.Context,
	_ domain.TenantID,
	_ domain.SubmissionVersionID,
) (bool, error) {
	if double.err != nil {
		return false, double.err
	}
	return double.found, nil
}

type ruleViewDouble struct {
	configured       bool
	err              error
	seenJurisdiction domain.RegulatoryJurisdictionReference
	seenEvaluatedAt  time.Time
}

func (double *ruleViewDouble) LoadInterpretationRule(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ResultLayer,
	jurisdiction domain.RegulatoryJurisdictionReference,
	evaluatedAt time.Time,
) (domain.InterpretationRuleReference, bool, error) {
	double.seenJurisdiction = jurisdiction
	double.seenEvaluatedAt = evaluatedAt
	if double.err != nil {
		return domain.InterpretationRuleReference{}, false, double.err
	}
	if !double.configured {
		return domain.InterpretationRuleReference{}, false, nil
	}
	rule, err := domain.NewInterpretationRuleReference("interpretation-rule/v1")
	return rule, true, err
}

// caseStoreDouble 与提交链共用的 unitStoreDouble（见 submit_declaration_test.go）
// 一起支起辖区回指链：范围→单元→案件→辖区。
type caseStoreDouble struct {
	cases   map[string]domain.CustomsCase
	findErr error
}

func (double *caseStoreDouble) FindByKey(
	_ context.Context,
	_ ports.CustomsCaseKey,
) (domain.CustomsCase, bool, error) {
	return domain.CustomsCase{}, false, nil
}

func (double *caseStoreDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.CustomsCaseID,
) (domain.CustomsCase, bool, error) {
	if double.findErr != nil {
		return domain.CustomsCase{}, false, double.findErr
	}
	found, ok := double.cases[id.String()]
	return found, ok, nil
}

func (double *caseStoreDouble) Save(
	_ context.Context,
	_ ports.CustomsCaseKey,
	_ domain.CustomsCase,
) (ports.CustomsCaseSaveOutcome, error) {
	return ports.CustomsCaseSaved, nil
}

type resultHandoffDouble struct {
	intents []ports.ExternalResultHandoffIntent
	err     error
}

func (double *resultHandoffDouble) HandOffExternalResult(
	_ context.Context,
	intent ports.ExternalResultHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type resultClock struct{ at time.Time }

func (clock resultClock) Now() time.Time { return clock.at }

type resultFixture struct {
	store       *resultStoreDouble
	submissions *submissionIndexDouble
	rules       *ruleViewDouble
	units       *unitStoreDouble
	cases       *caseStoreDouble
	handoff     *resultHandoffDouble
	handler     *application.ReceiveExternalResultHandler
}

// newResultFixture 支好整条辖区回指链：命令的范围 scope-unit-1 指名在册单元，单元
// 属案件 case-1，案件辖区 jurisdiction-1。
func newResultFixture(t *testing.T) *resultFixture {
	t.Helper()
	fixture := &resultFixture{
		store:       newResultStore(),
		submissions: &submissionIndexDouble{found: true},
		rules:       &ruleViewDouble{configured: true},
		units: &unitStoreDouble{units: map[string]domain.DeclarationUnit{
			"tenant-1|scope-unit-1": scopedUnit(t, "scope-unit-1", "case-1"),
		}},
		cases: &caseStoreDouble{cases: map[string]domain.CustomsCase{
			"case-1": jurisdictionCase(t, "case-1", "jurisdiction-1"),
		}},
		handoff: &resultHandoffDouble{},
	}
	fixture.handler = application.NewReceiveExternalResultHandler(application.ReceiveExternalResultDeps{
		Results:     fixture.store,
		Submissions: fixture.submissions,
		Rules:       fixture.rules,
		Units:       fixture.units,
		Cases:       fixture.cases,
		Downstream:  fixture.handoff,
		Clock:       resultClock{at: resultRecordedAt},
	})
	return fixture
}

func scopedUnit(t *testing.T, unitID, caseID string) domain.DeclarationUnit {
	t.Helper()
	unit, err := domain.FormDeclarationUnit(
		mustValue(t, domain.NewDeclarationUnitID, unitID),
		mustValue(t, domain.NewCustomsCaseID, caseID),
		mustValue(t, domain.NewCustomsProcedureReference, "procedure-1"),
		[]domain.DeclaredParcelReference{mustValue(t, domain.NewDeclaredParcelReference, "parcel-1")},
	)
	if err != nil {
		t.Fatalf("form declaration unit: %v", err)
	}
	return unit
}

func jurisdictionCase(t *testing.T, caseID, jurisdiction string) domain.CustomsCase {
	t.Helper()
	customsCase, err := domain.EstablishCustomsCase(domain.CustomsCaseSpec{
		ID:           mustValue(t, domain.NewCustomsCaseID, caseID),
		Jurisdiction: mustValue(t, domain.NewRegulatoryJurisdictionReference, jurisdiction),
		Direction:    domain.ImportManifest,
		Procedure:    mustValue(t, domain.NewCustomsProcedureReference, "procedure-1"),
		Obligation:   mustValue(t, domain.NewObligationScopeReference, "obligation-1"),
		Parcels: []domain.CaseParcelAssociation{{
			Parcel: "parcel-1", Customer: "customer-1", SourceRef: "source-ref-1",
		}},
		EstablishedAt: resultOccurredAt.Add(-24 * time.Hour),
	})
	if err != nil {
		t.Fatalf("establish customs case: %v", err)
	}
	return customsCase
}

func mustTenant(t *testing.T) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("new tenant: %v", err)
	}
	return tenant
}

func resultCommand(t *testing.T, sourceID string) application.ReceiveExternalResultCommand {
	t.Helper()
	return application.ReceiveExternalResultCommand{
		TenantID:       mustTenant(t),
		SourceID:       sourceID,
		Layer:          domain.RegulatoryReceiptLayer,
		Role:           "customs-authority",
		RawSemantics:   "RECEIVED",
		ClaimedVersion: "submission/v1",
		Attempt:        1,
		Scope:          "scope-unit-1",
		OccurredAt:     resultOccurredAt,
		ReceivedAt:     resultOccurredAt.Add(time.Minute),
	}
}

func existingLayerFact(t *testing.T, rawSemantics string) domain.ExternalResult {
	t.Helper()
	role, err := domain.NewSourceAuthorityRole("customs-authority")
	if err != nil {
		t.Fatalf("role: %v", err)
	}
	rule, err := domain.NewInterpretationRuleReference("interpretation-rule/v1")
	if err != nil {
		t.Fatalf("rule: %v", err)
	}
	version, err := domain.NewSubmissionVersionID("submission/v1")
	if err != nil {
		t.Fatalf("version: %v", err)
	}
	scope, err := domain.NewDecisionScopeReference("scope-unit-1")
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	fact, err := domain.InterpretExternalResult(domain.ExternalResultSpec{
		Layer:        domain.RegulatoryReceiptLayer,
		SourceID:     "source-earlier",
		Role:         role,
		RawSemantics: rawSemantics,
		Rule:         rule,
		Version:      version,
		Attempt:      1,
		Scope:        scope,
		OccurredAt:   resultOccurredAt.Add(-time.Hour),
		ReceivedAt:   resultOccurredAt.Add(-time.Hour),
	})
	if err != nil {
		t.Fatalf("interpret existing fact: %v", err)
	}
	return fact
}

// 已归属、规则已配置、同层无冲突的响应形成分层事实并交意图；层与解释规则随记录保全。
func TestAnAttributedResultIsInterpretedAndHandedOff(t *testing.T) {
	fixture := newResultFixture(t)
	result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ResultRecorded {
		t.Fatalf("outcome = %q, want RESULT_RECORDED", result.Outcome())
	}
	record, present := result.Record()
	if !present || record.Unattributable || record.LayerConflict {
		t.Fatalf("record = %+v", record)
	}
	if record.Result.Layer() != domain.RegulatoryReceiptLayer {
		t.Fatalf("layer = %q", record.Result.Layer())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}
}

// Covers: CONTEXT「与同层现有事实冲突时，不得据此猜测提交、补造缺失层次或按最后到达直接改变当前判断」——归属不上留存不猜（无意图）；同层冲突留存双方带标记。
func TestUnattributableAndLayerConflictsAreKeptNotGuessed(t *testing.T) {
	t.Run("an unattributable response is kept without guessing", func(t *testing.T) {
		fixture := newResultFixture(t)
		fixture.submissions.found = false
		result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultUnattributable {
			t.Fatalf("outcome = %q, want UNATTRIBUTABLE", result.Outcome())
		}
		record, _ := result.Record()
		if !record.Unattributable || record.RawSemantics != "RECEIVED" || record.ClaimedVersion != "submission/v1" {
			t.Fatalf("留存不全: %+v", record)
		}
		if len(fixture.store.records) != 1 {
			t.Fatal("归属不上的响应没有留存")
		}
		if len(fixture.handoff.intents) != 0 {
			t.Fatal("留存的原始响应竟然交了意图——下游会把它当监管事实")
		}
	})

	t.Run("a same-layer conflict keeps both facts", func(t *testing.T) {
		fixture := newResultFixture(t)
		fixture.store.layerFacts = []domain.ExternalResult{existingLayerFact(t, "REJECTED")}
		result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-2"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultLayerConflict {
			t.Fatalf("outcome = %q, want LAYER_CONFLICT", result.Outcome())
		}
		record, _ := result.Record()
		if !record.LayerConflict {
			t.Fatal("冲突没有随记录标出")
		}
		if len(fixture.store.records) != 1 {
			t.Fatal("冲突的新事实没有留存——双方留存才成立冲突关系")
		}
		if len(fixture.store.layerFacts) != 1 {
			t.Fatal("既有事实被动了")
		}
		if len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d, want 1（冲突记录也要交给核对消费）", len(fixture.handoff.intents))
		}
	})
}

// 解释规则是实例半边：未配置停在未决不猜语义；依赖故障各归其格（ADR-0029 分格）。
func TestUnconfiguredRulesAndDependencyFailuresStayUndecided(t *testing.T) {
	t.Run("an unconfigured interpretation rule is undecided", func(t *testing.T) {
		fixture := newResultFixture(t)
		fixture.rules.configured = false
		result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultUndecided ||
			result.UndecidedReason() != application.InterpretationRuleUnconfigured {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		if len(fixture.store.records) != 0 {
			t.Fatal("未决的提交落了库")
		}
	})

	t.Run("a submission index failure is undecided with its reason", func(t *testing.T) {
		fixture := newResultFixture(t)
		fixture.submissions.err = errors.New("index down")
		result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.SubmissionIndexUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a layer facts failure is undecided with its reason", func(t *testing.T) {
		fixture := newResultFixture(t)
		fixture.store.factsErr = errors.New("facts down")
		result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.LayerFactsUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.ExternalResultUndecidedReason{
			application.ResultStoreUnavailable, application.SubmissionIndexUnavailable,
			application.InterpretationRuleUnconfigured, application.LayerFactsUnavailable,
			application.EvaluationInstantUntrusted, application.CaseChainUnavailable,
			application.JurisdictionUnresolved, application.ReleaseSemanticsUninterpreted,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 8 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.ExternalResultUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第九个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}

// 规则选择侧的输入按 ADR-0070 取值：评估时点 = 业务发生或适用时间（问二甲），适用
// 辖区 = 范围→单元→案件回指（问三甲）。任何一样取不出都显式停在未决——绝不拿消息
// 到达时间当适用时点（CONTEXT「不能统一替代规则的法定适用时点」点名禁止），也不留「当前唯一辖区」的兜底缝。
func TestRuleSelectionInputsAreResolvedOrUndecided(t *testing.T) {
	t.Run("the rule is resolved with the case jurisdiction at the occurrence instant", func(t *testing.T) {
		fixture := newResultFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1")); err != nil {
			t.Fatalf("handle: %v", err)
		}
		if fixture.rules.seenJurisdiction.String() != "jurisdiction-1" {
			t.Fatalf("辖区没走案件回指链: %q", fixture.rules.seenJurisdiction)
		}
		if !fixture.rules.seenEvaluatedAt.Equal(resultOccurredAt) {
			t.Fatalf("评估时点 = %v, want 业务发生时间 %v", fixture.rules.seenEvaluatedAt, resultOccurredAt)
		}
	})

	t.Run("a missing occurrence instant is undecided, never defaulted", func(t *testing.T) {
		fixture := newResultFixture(t)
		command := resultCommand(t, "source-1")
		command.OccurredAt = time.Time{}
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultUndecided ||
			result.UndecidedReason() != application.EvaluationInstantUntrusted {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		if !fixture.rules.seenEvaluatedAt.IsZero() {
			t.Fatal("评估时点缺席时仍去选了版——兜底缝没堵住")
		}
	})

	t.Run("an occurrence instant later than the receipt is untrusted", func(t *testing.T) {
		fixture := newResultFixture(t)
		command := resultCommand(t, "source-1")
		command.OccurredAt = command.ReceivedAt.Add(time.Minute)
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.EvaluationInstantUntrusted {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a scope naming no persisted unit leaves the jurisdiction unresolved", func(t *testing.T) {
		fixture := newResultFixture(t)
		command := resultCommand(t, "source-1")
		command.Scope = "scope-unknown"
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultUndecided ||
			result.UndecidedReason() != application.JurisdictionUnresolved {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a unit whose case is not persisted leaves the jurisdiction unresolved", func(t *testing.T) {
		fixture := newResultFixture(t)
		delete(fixture.cases.cases, "case-1")
		result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.JurisdictionUnresolved {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})

	t.Run("a chain store failure is undecided with its own reason", func(t *testing.T) {
		fixture := newResultFixture(t)
		fixture.units.findErr = errors.New("unit store down")
		result, err := fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.CaseChainUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}

		fixture = newResultFixture(t)
		fixture.cases.findErr = errors.New("case store down")
		result, err = fixture.handler.Handle(context.Background(), resultCommand(t, "source-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.UndecidedReason() != application.CaseChainUnavailable {
			t.Fatalf("reason = %q", result.UndecidedReason())
		}
	})
}

// 幂等/冲突按内容指纹分界；投递失败不翻结果、重放重发同一份；未受理与并发落败各守其格。
func TestReplayConflictAndRecoveryDiscipline(t *testing.T) {
	fixture := newResultFixture(t)
	fixture.handoff.err = errors.New("downstream unavailable")
	command := resultCommand(t, "source-1")

	first, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ResultRecorded || first.ResultHandoffReference() == "" {
		t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.ResultHandoffReference())
	}

	fixture.handoff.err = nil
	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ResultExistingResult || len(fixture.handoff.intents) != 1 {
		t.Fatalf("outcome = %q intents = %d（重放重发同一份）", replay.Outcome(), len(fixture.handoff.intents))
	}
	if fixture.store.saves != 1 {
		t.Fatalf("saves = %d, want 1", fixture.store.saves)
	}

	t.Run("a different content under the same source identity is a conflict", func(t *testing.T) {
		flipped := resultCommand(t, "source-1")
		flipped.RawSemantics = "REJECTED"
		result, err := fixture.handler.Handle(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict handle: %v", err)
		}
		if result.Outcome() != application.ResultSourceConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
		}
		kept := fixture.store.records[resultKey(ports.ExternalResultKey{TenantID: flipped.TenantID, SourceID: "source-1"})]
		if kept.Result.RawSemantics() != "RECEIVED" {
			t.Fatal("冲突覆盖了原结果")
		}
	})

	t.Run("a malformed submission is not accepted", func(t *testing.T) {
		blank := resultCommand(t, "source-x")
		blank.RawSemantics = "   "
		result, err := fixture.handler.Handle(context.Background(), blank)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
		zeroAttempt := resultCommand(t, "source-y")
		zeroAttempt.Attempt = 0
		result, err = fixture.handler.Handle(context.Background(), zeroAttempt)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultNotAccepted {
			t.Fatalf("outcome = %q; 零序号尝试立不起结果", result.Outcome())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		broken := newResultFixture(t)
		broken.store.forceResult = true
		broken.store.saveResult = ports.ExternalResultSaveOutcome(99)
		if _, err := broken.handler.Handle(context.Background(), resultCommand(t, "source-z")); !errors.Is(err, application.ErrUnexpectedResultSave) {
			t.Fatalf("error = %v, want ErrUnexpectedResultSave", err)
		}
	})
}
