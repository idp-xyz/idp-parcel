package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var followUpAt = time.Date(2026, 8, 13, 19, 0, 0, 0, time.UTC)

type followUpStoreDouble struct {
	targets   map[ports.FollowUpTargetKey]domain.FollowUpTarget
	relations map[ports.FollowUpTargetKey]domain.ReplacementRelation
}

func (double *followUpStoreDouble) FindTarget(
	_ context.Context,
	key ports.FollowUpTargetKey,
) (domain.FollowUpTarget, bool, error) {
	target, found := double.targets[key]
	return target, found, nil
}

func (double *followUpStoreDouble) SaveTarget(
	_ context.Context,
	key ports.FollowUpTargetKey,
	target domain.FollowUpTarget,
) (ports.FollowUpSaveOutcome, error) {
	if _, exists := double.targets[key]; exists {
		return ports.FollowUpAlreadyRecorded, nil
	}
	double.targets[key] = target
	return ports.FollowUpSaved, nil
}

func (double *followUpStoreDouble) FindRelation(
	_ context.Context,
	key ports.FollowUpTargetKey,
) (domain.ReplacementRelation, bool, error) {
	relation, found := double.relations[key]
	return relation, found, nil
}

func (double *followUpStoreDouble) SaveRelation(
	_ context.Context,
	key ports.FollowUpTargetKey,
	relation domain.ReplacementRelation,
) (ports.FollowUpSaveOutcome, error) {
	if _, exists := double.relations[key]; exists {
		return ports.FollowUpAlreadyRecorded, nil
	}
	double.relations[key] = relation
	return ports.FollowUpSaved, nil
}

func (double *followUpStoreDouble) UpdateRelation(
	_ context.Context,
	key ports.FollowUpTargetKey,
	relation domain.ReplacementRelation,
) error {
	double.relations[key] = relation
	return nil
}

type followUpDownstreamDouble struct {
	intents []ports.FollowUpHandoffIntent
}

func (double *followUpDownstreamDouble) HandOffFollowUp(
	_ context.Context,
	intent ports.FollowUpHandoffIntent,
) error {
	double.intents = append(double.intents, intent)
	return nil
}

type followUpFixture struct {
	handler *application.ManageFollowUpHandler
	store   *followUpStoreDouble
}

func newFollowUpFixture(t *testing.T) *followUpFixture {
	t.Helper()
	fixture := &followUpFixture{
		store: &followUpStoreDouble{
			targets:   map[ports.FollowUpTargetKey]domain.FollowUpTarget{},
			relations: map[ports.FollowUpTargetKey]domain.ReplacementRelation{},
		},
	}
	fixture.handler = application.NewManageFollowUpHandler(application.ManageFollowUpDeps{
		Store:      fixture.store,
		Downstream: &followUpDownstreamDouble{},
		Clock:      fixedClock{at: followUpAt},
	})
	return fixture
}

func followUpSpec(t *testing.T, kind domain.FollowUpActionKind) domain.FollowUpTargetSpec {
	t.Helper()
	return domain.FollowUpTargetSpec{
		Kind:     kind,
		Trigger:  mustValue(t, domain.NewFollowUpTriggerReference, "REGULATORY-REQUIREMENT/refile"),
		CaseRef:  "customs-case-1",
		Unit:     mustValue(t, domain.NewDeclarationUnitID, "unit-1"),
		Version:  mustValue(t, domain.NewSubmissionVersionID, "submission-v1"),
		Scope:    mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		FormedAt: followUpAt.Add(-time.Hour),
	}
}

func followUpKey(t *testing.T, kind domain.FollowUpActionKind) ports.FollowUpTargetKey {
	t.Helper()
	return ports.FollowUpTargetKey{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Trigger:  mustValue(t, domain.NewFollowUpTriggerReference, "REGULATORY-REQUIREMENT/refile"),
		Version:  mustValue(t, domain.NewSubmissionVersionID, "submission-v1"),
		Kind:     kind,
	}
}

// Covers: UC-CC-007 的编排面——同触发依据同版本同类动作只立一个目标（重放返原，
// `AT-CC-208`「相同请求、触发版本、原申报、范围、规则和输入再次到达→返回已有决定、
// 目标和关系」）；重报目标至多一份拟替代（同替代单元重放返原、异替代单元冲突不顶替
// ——换单元先处置原拟替代）；生效只凭外部结果（`AT-CC-201` 的生效半边「全部替代生效
// 条件满足→形成范围明确的有效替代关系」与 `AT-CC-202` 的拒生效半边「只取得技术成功、
// 监管接收……→不形成有效替代」）且已生效重放按已生效作答（CONTEXT 267：内部决定、
// 请求发出或技术成功都不等于替代成立）。
func TestFollowUpTargetsProposalsAndEffectsStayDisciplined(t *testing.T) {
	fixture := newFollowUpFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")

	formed, err := fixture.handler.FormTarget(context.Background(), application.FormFollowUpTargetCommand{
		TenantID: tenant,
		Spec:     followUpSpec(t, domain.ResubmissionReplacement),
	})
	if err != nil {
		t.Fatalf("form target: %v", err)
	}
	if formed.Outcome() != application.FollowUpTargetFormed {
		t.Fatalf("outcome = %q", formed.Outcome())
	}

	replayed, err := fixture.handler.FormTarget(context.Background(), application.FormFollowUpTargetCommand{
		TenantID: tenant,
		Spec:     followUpSpec(t, domain.ResubmissionReplacement),
	})
	if err != nil {
		t.Fatalf("replay target: %v", err)
	}
	if replayed.Outcome() != application.FollowUpTargetExisting {
		t.Fatalf("replay = %q", replayed.Outcome())
	}

	key := followUpKey(t, domain.ResubmissionReplacement)
	proposed, err := fixture.handler.Propose(context.Background(), application.ProposeReplacementCommand{
		TenantID:        tenant,
		Key:             key,
		ReplacementUnit: mustValue(t, domain.NewDeclarationUnitID, "unit-2"),
	})
	if err != nil {
		t.Fatalf("propose: %v", err)
	}
	if proposed.Outcome() != application.ReplacementProposed {
		t.Fatalf("outcome = %q", proposed.Outcome())
	}
	relation, _ := proposed.Relation()
	if relation.Effective() {
		t.Fatal("拟替代一建立就生效了——那要等外部结果")
	}

	sameUnit, err := fixture.handler.Propose(context.Background(), application.ProposeReplacementCommand{
		TenantID:        tenant,
		Key:             key,
		ReplacementUnit: mustValue(t, domain.NewDeclarationUnitID, "unit-2"),
	})
	if err != nil {
		t.Fatalf("propose replay: %v", err)
	}
	if sameUnit.Outcome() != application.ReplacementExisting {
		t.Fatalf("replay = %q", sameUnit.Outcome())
	}

	otherUnit, err := fixture.handler.Propose(context.Background(), application.ProposeReplacementCommand{
		TenantID:        tenant,
		Key:             key,
		ReplacementUnit: mustValue(t, domain.NewDeclarationUnitID, "unit-3"),
	})
	if err != nil {
		t.Fatalf("propose conflict: %v", err)
	}
	if otherUnit.Outcome() != application.ReplacementUnitConflict {
		t.Fatalf("conflict = %q; 换替代单元必须先处置原拟替代", otherUnit.Outcome())
	}

	effective, err := fixture.handler.RecordEffect(context.Background(), application.RecordReplacementEffectCommand{
		TenantID:       tenant,
		Key:            key,
		ExternalResult: "EXTERNAL-RESULT/withdrawal-and-refile-accepted",
		At:             followUpAt,
	})
	if err != nil {
		t.Fatalf("record effect: %v", err)
	}
	if effective.Outcome() != application.ReplacementEffective {
		t.Fatalf("outcome = %q", effective.Outcome())
	}

	again, err := fixture.handler.RecordEffect(context.Background(), application.RecordReplacementEffectCommand{
		TenantID:       tenant,
		Key:            key,
		ExternalResult: "EXTERNAL-RESULT/duplicate",
		At:             followUpAt.Add(time.Minute),
	})
	if err != nil {
		t.Fatalf("record effect again: %v", err)
	}
	if again.Outcome() != application.ReplacementAlreadyEffective {
		t.Fatalf("again = %q", again.Outcome())
	}
	kept, _ := again.Relation()
	if result, _ := kept.ExternalResult(); result != "EXTERNAL-RESULT/withdrawal-and-refile-accepted" {
		t.Fatalf("external result = %q; 重复生效改写了原成立依据", result)
	}
}

// Covers: 边界纪律——非重报目标建立不了替代关系（领域拦编排透出未受理）；没有拟替代
// 无从生效；没有目标无从拟替代。
func TestReplacementDemandsItsResubmissionTarget(t *testing.T) {
	fixture := newFollowUpFixture(t)
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")

	if _, err := fixture.handler.FormTarget(context.Background(), application.FormFollowUpTargetCommand{
		TenantID: tenant,
		Spec:     followUpSpec(t, domain.InCaseCorrection),
	}); err != nil {
		t.Fatalf("form correction target: %v", err)
	}
	correctionKey := followUpKey(t, domain.InCaseCorrection)
	refused, err := fixture.handler.Propose(context.Background(), application.ProposeReplacementCommand{
		TenantID:        tenant,
		Key:             correctionKey,
		ReplacementUnit: mustValue(t, domain.NewDeclarationUnitID, "unit-2"),
	})
	if err != nil {
		t.Fatalf("propose on correction: %v", err)
	}
	if refused.Outcome() != application.FollowUpNotAccepted {
		t.Fatalf("outcome = %q; 原案内更正立不了替代关系", refused.Outcome())
	}

	missing, err := fixture.handler.RecordEffect(context.Background(), application.RecordReplacementEffectCommand{
		TenantID:       tenant,
		Key:            followUpKey(t, domain.ResubmissionReplacement),
		ExternalResult: "EXTERNAL-RESULT/anything",
		At:             followUpAt,
	})
	if err != nil {
		t.Fatalf("record effect without relation: %v", err)
	}
	if missing.Outcome() != application.FollowUpTargetNotFound {
		t.Fatalf("outcome = %q", missing.Outcome())
	}
}
