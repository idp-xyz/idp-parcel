package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var finalOccurredAt = time.Date(2026, 8, 10, 15, 0, 0, 0, time.UTC)

type finalRuleDouble struct {
	judgment   ports.FinalRuleJudgment
	configured bool
	err        error
}

func (double *finalRuleDouble) JudgeFinalOutcome(
	_ context.Context,
	_ domain.SourceIdentity,
	_ domain.ResponsibilityOutcome,
) (ports.FinalRuleJudgment, bool, error) {
	if double.err != nil {
		return ports.FinalRuleJudgment{}, false, double.err
	}
	return double.judgment, double.configured, nil
}

type finalStoreDouble struct {
	byKey map[ports.FinalAdoptionKey]ports.FinalOutcomeRecord
	saved int
}

func newFinalStore() *finalStoreDouble {
	return &finalStoreDouble{byKey: map[ports.FinalAdoptionKey]ports.FinalOutcomeRecord{}}
}

func (double *finalStoreDouble) FindByKey(
	_ context.Context,
	key ports.FinalAdoptionKey,
) (ports.FinalOutcomeRecord, bool, error) {
	record, found := double.byKey[key]
	return record, found, nil
}

func (double *finalStoreDouble) FindCurrentFinal(
	_ context.Context,
	tenant domain.TenantID,
	parcel domain.DeclaredParcelID,
) (ports.FinalOutcomeRecord, bool, error) {
	var current ports.FinalOutcomeRecord
	found := false
	for _, record := range double.byKey {
		if record.Key.TenantID == tenant && record.Key.Parcel == parcel && record.Finalized {
			// 多版本时取最新采用的一份——重派生的当前判断。
			if !found || record.AdoptedAt.After(current.AdoptedAt) {
				current = record
				found = true
			}
		}
	}
	return current, found, nil
}

func (double *finalStoreDouble) Save(
	_ context.Context,
	record ports.FinalOutcomeRecord,
) (ports.FinalOutcomeSaveOutcome, error) {
	if _, exists := double.byKey[record.Key]; exists {
		return ports.FinalOutcomeAlreadyRecorded, nil
	}
	double.byKey[record.Key] = record
	double.saved++
	return ports.FinalOutcomeSaved, nil
}

type finalDownstreamDouble struct {
	intents []ports.FinalOutcomeHandoffIntent
	err     error
}

func (double *finalDownstreamDouble) HandOffFinalOutcome(
	_ context.Context,
	intent ports.FinalOutcomeHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type finalIdentityDouble struct{ next int }

func (double *finalIdentityDouble) NextFinalOutcomeVersionID(
	_ context.Context,
) (domain.FinalOutcomeVersionID, error) {
	double.next++
	return domain.NewFinalOutcomeVersionID("final-" + string(rune('0'+double.next)))
}

type finalFixture struct {
	handler       *application.FormParcelFinalHandler
	rules         *finalRuleDouble
	finals        *finalStoreDouble
	cancellations *cancellationViewDouble
	downstream    *finalDownstreamDouble
}

func newFinalFixture(t *testing.T) *finalFixture {
	t.Helper()
	requests := &shipmentRequestRepositoryDouble{
		records: map[domain.SourceIdentity]domain.ShipmentRequest{},
		record:  func(string) {},
	}
	requests.records[sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")] = acceptedRequest(t)
	fixture := &finalFixture{
		rules: &finalRuleDouble{
			judgment: ports.FinalRuleJudgment{
				Satisfied:   true,
				Kind:        mustValue(t, domain.NewFinalKindReference, "NETWORK_SERVICE_DELIVERED"),
				RuleVersion: mustValue(t, domain.NewFinalRuleVersionReference, "final-rules/v1"),
			},
			configured: true,
		},
		finals:        newFinalStore(),
		cancellations: &cancellationViewDouble{},
		downstream:    &finalDownstreamDouble{},
	}
	fixture.handler = application.NewFormParcelFinalHandler(application.FormParcelFinalDeps{
		Requests:      requests,
		Rules:         fixture.rules,
		Finals:        fixture.finals,
		Cancellations: fixture.cancellations,
		Identities:    &finalIdentityDouble{},
		Downstream:    fixture.downstream,
		Clock:         fixedClock{at: finalOccurredAt.Add(time.Minute)},
	})
	return fixture
}

func finalCommand(t *testing.T, parcel string) application.FormParcelFinalCommand {
	t.Helper()
	return application.FormParcelFinalCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		Outcome: domain.ResponsibilityOutcomeSpec{
			Kind:       domain.EffectiveDeliveryOutcome,
			Parcel:     mustValue(t, domain.NewDeclaredParcelID, parcel),
			Decision:   mustValue(t, domain.NewResponsibilityDecisionReference, "DELIVERY-JUDGMENT/TF-11"),
			Execution:  mustValue(t, domain.NewExecutionEvidenceReference, "EFFECTIVE-DELIVERY/TF-11/POD-3"),
			Version:    mustValue(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v1"),
			OccurredAt: finalOccurredAt,
		},
	}
}

// Covers: `AT-PS-053`「合格有效交付满足网络服务终局规则——为该包裹形成终局，保留交付
// 来源引用」与 `AT-PS-054`「三个包裹只有两个终局——委托派生部分完成」——本夹具委托两
// 成员，一个终局即部分完成；终局类型与规则版本来自规则答复不是本域私定。
func TestAQualifiedDeliveryFormsTheFinalAndDerivesCompletion(t *testing.T) {
	fixture := newFinalFixture(t)

	result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("outcome = %q, want FINAL_FORMED", result.Outcome())
	}
	record, _ := result.Record()
	if record.Final.Kind().String() != "NETWORK_SERVICE_DELIVERED" ||
		record.Final.RuleVersion().String() != "final-rules/v1" {
		t.Fatalf("final = %#v; 终局类型与规则版本必须来自规则答复", record.Final)
	}
	if !record.Final.EffectiveAt().Equal(finalOccurredAt) {
		t.Fatalf("effective at = %s", record.Final.EffectiveAt())
	}
	summary, present := result.Completion()
	if !present || summary.State() != domain.CompletionPartial || summary.Finalized() != 1 || summary.Total() != 2 {
		t.Fatalf("completion = %v %s; 两成员一终局应派生部分完成", present, summary.State())
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("intents = %d", len(fixture.downstream.intents))
	}
}

// Covers: `AT-PS-054` 的全部完成半边——第二个成员经取消终局进汇总（UC-PS-006 的取消
// 直接参与统一汇总），两成员都有适用终局即派生全部完成。
func TestACancelledSiblingCountsTowardCompleteness(t *testing.T) {
	fixture := newFinalFixture(t)
	fixture.cancellations.cancelled = true
	fixture.cancellations.cancellation = cancelledAt(t, finalOccurredAt.Add(-2*time.Hour))

	result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	summary, present := result.Completion()
	if !present || summary.State() != domain.CompletionComplete {
		t.Fatalf("completion = %v %s; 取消终局与履约终局同格进汇总", present, summary.State())
	}
}

// Covers: `AT-PS-056`「POD 已上传但交付判断待确认——保持终局未决，不默认完成」的规则面
// 与首发红线「不得默认有效交付即所有产品终局」——规则未配置即未决；规则明确不满足即
// 未决带缺口依据；两格都不落库。
func TestUnconfiguredOrUnsatisfiedRulesStallWithoutDefaulting(t *testing.T) {
	t.Run("rules not configured", func(t *testing.T) {
		fixture := newFinalFixture(t)
		fixture.rules.configured = false

		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelFinalUndecided ||
			result.UndecidedReason() != application.FinalRuleUnconfigured {
			t.Fatalf("outcome = %q/%q", result.Outcome(), result.UndecidedReason())
		}
		if fixture.finals.saved != 0 {
			t.Fatal("未决落了库")
		}
	})

	t.Run("rule not satisfied", func(t *testing.T) {
		fixture := newFinalFixture(t)
		fixture.rules.judgment = ports.FinalRuleJudgment{
			Satisfied: false,
			Basis:     mustValue(t, domain.NewCheckReason, "DELIVERY_CONFIRMATION_PENDING"),
		}

		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelFinalUndecided ||
			result.UndecidedReason() != application.FinalRuleNotSatisfiedYet {
			t.Fatalf("outcome = %q/%q", result.Outcome(), result.UndecidedReason())
		}
		if result.Basis().String() != "DELIVERY_CONFIRMATION_PENDING" {
			t.Fatal("未决没带规则缺口依据")
		}
	})
}

// Covers: `AT-PS-061`「同一履约结果重复到达——返回原终局判断」、`AT-PS-062`「同一来源
// 身份携带不同结果——形成冲突不覆盖」与 `AT-PS-065`「终局保存成功但发布失败——只重试
// 原发布意图，不重复终局」。
func TestReplayConflictAndFailedIntentStayDisciplinedForFinals(t *testing.T) {
	fixture := newFinalFixture(t)
	fixture.downstream.err = errors.New("downstream unreachable")

	first, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	if first.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("outcome = %q; 投递失败不得翻终局", first.Outcome())
	}
	if first.FinalHandoffReference().String() == "" {
		t.Fatal("首投失败没有留发布续办引用")
	}

	fixture.downstream.err = nil
	replay, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.ParcelFinalExistingResult {
		t.Fatalf("replay = %q", replay.Outcome())
	}
	if fixture.finals.saved != 1 || len(fixture.downstream.intents) != 1 {
		t.Fatalf("saved = %d intents = %d; 重发的必须是原判断那一份", fixture.finals.saved, len(fixture.downstream.intents))
	}

	conflicting := finalCommand(t, "parcel-1")
	conflicting.Outcome.Execution = mustValue(t, domain.NewExecutionEvidenceReference, "EFFECTIVE-DELIVERY/TF-11/POD-9")
	conflict, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("conflict handle: %v", err)
	}
	if conflict.Outcome() != application.FinalSourceConflict {
		t.Fatalf("conflict = %q", conflict.Outcome())
	}
}

// Covers: `AT-PS-063`「原有效交付因 POD 错误被失效——保留原终局历史，形成新判断版本并
// 重新派生当前结果」的编排面——同源新版本走重派生：新判断指回原版、原记录原样在库；
// 异源结果在已有终局上不采用（终局不回退，`AT-PS-064` 的对偶）。
func TestSourceRevisionRederivesWhileForeignSourcesAreRefused(t *testing.T) {
	fixture := newFinalFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1")); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	corrected := finalCommand(t, "parcel-1")
	corrected.Outcome.Version = mustValue(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v2")
	corrected.Outcome.Execution = mustValue(t, domain.NewExecutionEvidenceReference, "EFFECTIVE-DELIVERY/TF-11/POD-4")
	corrected.Outcome.OccurredAt = finalOccurredAt.Add(time.Hour)

	rederived, err := fixture.handler.Handle(context.Background(), corrected)
	if err != nil {
		t.Fatalf("rederive handle: %v", err)
	}
	if rederived.Outcome() != application.ParcelFinalRederived {
		t.Fatalf("outcome = %q, want FINAL_REDERIVED", rederived.Outcome())
	}
	record, _ := rederived.Record()
	prior, present := record.Final.PriorVersion()
	if !present || prior.String() != "final-1" {
		t.Fatalf("prior = %s present = %v; 重派生必须指回原版", prior, present)
	}
	if fixture.finals.saved != 2 {
		t.Fatalf("saved = %d; 原判断必须原样在库", fixture.finals.saved)
	}

	foreign := finalCommand(t, "parcel-1")
	foreign.Outcome.Kind = domain.ReturnCompletedOutcome
	foreign.Outcome.Version = mustValue(t, domain.NewResponsibilityOutcomeVersion, "return-result/v1")
	refused, err := fixture.handler.Handle(context.Background(), foreign)
	if err != nil {
		t.Fatalf("foreign handle: %v", err)
	}
	if refused.Outcome() != application.FinalSourceNotAdopted {
		t.Fatalf("outcome = %q, want SOURCE_NOT_ADOPTED", refused.Outcome())
	}
	if !errorsContains(refused.Basis().String(), "FINAL_ALREADY_FORMED/") {
		t.Fatalf("basis = %q; 不采用必须指名既有终局", refused.Basis())
	}
}

func errorsContains(value, prefix string) bool {
	return len(value) >= len(prefix) && value[:len(prefix)] == prefix
}

// Covers: `AT-PS-066`「跨客户查询委托完成摘要——严格隔离」的提交面与统一不可见纪律——
// 越权探与真查无探整结构同形；结果联合构造不出的输入（缺执行事实）未受理。
func TestForeignProbesAndHalfShapesAreRefusedUniformly(t *testing.T) {
	fixture := newFinalFixture(t)

	foreign := finalCommand(t, "parcel-1")
	foreign.Identity = sourceIdentity(t, "tenant-1", "customer-2", "source-a", "key-1")
	foreignResult, err := fixture.handler.Handle(context.Background(), foreign)
	if err != nil {
		t.Fatalf("foreign probe: %v", err)
	}

	miss := finalCommand(t, "parcel-1")
	miss.Identity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-unknown")
	missResult, err := fixture.handler.Handle(context.Background(), miss)
	if err != nil {
		t.Fatalf("miss probe: %v", err)
	}
	if foreignResult != missResult {
		t.Fatalf("foreign = %#v miss = %#v; 两探必须同形", foreignResult, missResult)
	}

	halfShape := finalCommand(t, "parcel-1")
	halfShape.Outcome.Execution = domain.ExecutionEvidenceReference{}
	refused, err := fixture.handler.Handle(context.Background(), halfShape)
	if err != nil {
		t.Fatalf("half shape handle: %v", err)
	}
	if refused.Outcome() != application.FinalRequestNotAccepted {
		t.Fatalf("outcome = %q; 缺执行事实的结果构造不出", refused.Outcome())
	}
}
