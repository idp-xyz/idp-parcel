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
	deps          application.FormParcelFinalDeps
	requests      *shipmentRequestRepositoryDouble
	rules         *finalRuleDouble
	finals        *finalStoreDouble
	cancellations *cancellationViewDouble
	downstream    *finalDownstreamDouble
	lock          *boundaryLockDouble
}

// rebuild 按改过的依赖重装编排，同一份替身继续记账。
func (fixture *finalFixture) rebuild(change func(*application.FormParcelFinalDeps)) {
	change(&fixture.deps)
	fixture.handler = application.NewFormParcelFinalHandler(fixture.deps)
}

func newFinalFixture(t *testing.T) *finalFixture {
	t.Helper()
	requests := &shipmentRequestRepositoryDouble{
		records: map[domain.SourceIdentity]domain.ShipmentRequest{},
		record:  func(string) {},
	}
	requests.records[sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")] = acceptedRequest(t)
	fixture := &finalFixture{
		requests: requests,
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
		lock:          &boundaryLockDouble{},
	}
	fixture.deps = application.FormParcelFinalDeps{
		Requests:      requests,
		Rules:         fixture.rules,
		Finals:        fixture.finals,
		Cancellations: fixture.cancellations,
		Identities:    &finalIdentityDouble{},
		Downstream:    fixture.downstream,
		Clock:         fixedClock{at: finalOccurredAt.Add(time.Minute)},
		BoundaryLock:  fixture.lock,
	}
	fixture.handler = application.NewFormParcelFinalHandler(fixture.deps)
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
	fixture.cancellations.onlyFor = mustValue(t, domain.NewDeclaredParcelID, "parcel-1")

	result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-2"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("outcome = %q; 被取消的是另一个成员，本成员的交付照常形成终局", result.Outcome())
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

// Covers: `AT-PS-080`「取消先成立，迟到消息……——取消不被覆盖」在有效交付一路上的对偶，以及 `AT-PS-107`（ADR-0254 决定二第 3 格）
// 「不形成责任起点也不形成非取消终局」——包裹已有取消在册时迟到的有效交付不形成终局：不采用，依据指名那份取消，来源与交付
// 保全在不采用记录上，并交下游一份不采用意图。终局规则放行也一样；规则未配置时同样答不采用，不停在 FINAL_RULE_UNCONFIGURED
// 等一个永远不会开的终局。同一份交付重投答已有结果，更正出的新版本到达也仍不采用。
func TestADeliveryOverAStandingCancellationIsNotAdopted(t *testing.T) {
	arrange := func(t *testing.T) *finalFixture {
		t.Helper()
		fixture := newFinalFixture(t)
		fixture.cancellations.cancelled = true
		fixture.cancellations.cancellation = cancelledAt(t, finalOccurredAt.Add(-2*time.Hour))
		fixture.cancellations.onlyFor = mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
		return fixture
	}

	t.Run("the delivery is refused naming the cancellation", func(t *testing.T) {
		fixture := arrange(t)

		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.FinalSourceNotAdopted ||
			result.Basis().String() != "PARCEL_CANCELLATION/cancel-1" {
			t.Fatalf("outcome = %q basis = %q, want SOURCE_NOT_ADOPTED 指名 PARCEL_CANCELLATION/cancel-1",
				result.Outcome(), result.Basis())
		}
		record, held := result.Record()
		if !held || record.Finalized || record.Key.Kind != domain.EffectiveDeliveryOutcome ||
			record.Key.Version.String() != "delivery-result/v1" {
			t.Fatalf("record = %+v held = %v; 来源（种类与版本）必须保全在不采用记录上、不是终局", record, held)
		}
		if _, found, _ := fixture.finals.FindCurrentFinal(context.Background(), record.Key.TenantID, record.Key.Parcel); found {
			t.Fatal("取消在册的包裹带上了当前有效终局")
		}
		if len(fixture.downstream.intents) != 1 || fixture.downstream.intents[0].Record.Finalized {
			t.Fatalf("intents = %+v; 下游要知道有一份迟到来源没被采用", fixture.downstream.intents)
		}
		summary, present := result.Completion()
		if !present || summary.Finalized() != 1 {
			t.Fatalf("completion = %v %v; 取消在册的包裹以取消终局进汇总", present, summary)
		}
	})

	t.Run("the refusal does not wait for the final rule", func(t *testing.T) {
		fixture := arrange(t)
		fixture.rules.configured = false

		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.FinalSourceNotAdopted {
			t.Fatalf("outcome = %q/%q, want SOURCE_NOT_ADOPTED——取消在册，规则未配置也不该停着等终局",
				result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a replay answers the refusal that was recorded", func(t *testing.T) {
		fixture := arrange(t)
		if _, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1")); err != nil {
			t.Fatalf("first handle: %v", err)
		}

		replay, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		if replay.Outcome() != application.ParcelFinalExistingResult ||
			replay.Basis().String() != "PARCEL_CANCELLATION/cancel-1" {
			t.Fatalf("replay = %q basis = %q, want EXISTING_RESULT 带原依据", replay.Outcome(), replay.Basis())
		}
		if fixture.finals.saved != 1 {
			t.Fatalf("saved = %d; 重投不得再落一份", fixture.finals.saved)
		}
	})

	// 同一包裹上已有同种当前终局、取消又在册，这个组合在真库的包裹边界锁下走不到（取消提交与终局采用按包裹排队，
	// 先到的一方落下记录后，后到的读到的就是它）；但编排里先读取消、后做既有终局分派是契约：取消在册即不采用，
	// 不因手里已有同种终局就改走重派生。真库走不到的次序用替身钉住。
	t.Run("a standing cancellation is judged before the final that already stands", func(t *testing.T) {
		fixture := newFinalFixture(t)
		if first, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1")); err != nil ||
			first.Outcome() != application.ParcelFinalFormed {
			t.Fatalf("first = %q err = %v, want FINAL_FORMED", first.Outcome(), err)
		}
		fixture.cancellations.cancelled = true
		fixture.cancellations.cancellation = cancelledAt(t, finalOccurredAt.Add(-2*time.Hour))

		corrected := finalCommand(t, "parcel-1")
		corrected.Outcome.Version = mustValue(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v2")
		result, err := fixture.handler.Handle(context.Background(), corrected)
		if err != nil {
			t.Fatalf("corrected handle: %v", err)
		}
		if result.Outcome() != application.FinalSourceNotAdopted ||
			result.Basis().String() != "PARCEL_CANCELLATION/cancel-1" {
			t.Fatalf("outcome = %q basis = %q, want SOURCE_NOT_ADOPTED 指名 PARCEL_CANCELLATION/cancel-1——取消在册先于既有终局分派",
				result.Outcome(), result.Basis())
		}
		record, held := result.Record()
		if !held || record.Finalized || record.Key.Version.String() != "delivery-result/v2" {
			t.Fatalf("record = %+v held = %v; v2 应落一份不采用记录而不是重派生出的终局", record, held)
		}
		current, found, _ := fixture.finals.FindCurrentFinal(context.Background(), record.Key.TenantID, record.Key.Parcel)
		if !found || current.Key.Version.String() != "delivery-result/v1" {
			t.Fatalf("current = %+v found = %v; 既有终局应原样在库、不被重派生", current, found)
		}
	})

	t.Run("a corrected version of the delivery is refused too", func(t *testing.T) {
		fixture := arrange(t)
		if _, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1")); err != nil {
			t.Fatalf("first handle: %v", err)
		}
		corrected := finalCommand(t, "parcel-1")
		corrected.Outcome.Version = mustValue(t, domain.NewResponsibilityOutcomeVersion, "delivery-result/v2")

		result, err := fixture.handler.Handle(context.Background(), corrected)
		if err != nil {
			t.Fatalf("corrected handle: %v", err)
		}
		if result.Outcome() != application.FinalSourceNotAdopted ||
			result.Basis().String() != "PARCEL_CANCELLATION/cancel-1" {
			t.Fatalf("outcome = %q basis = %q; 更正出的新版本也不能绕过在册的取消", result.Outcome(), result.Basis())
		}
	})
}

// Covers: `AT-PS-053`「合格有效交付满足网络服务终局规则」不回归——没有取消的包裹交付照常形成终局，取消视图是读过的——「没有」是读出来的答案；读的是命令指名的（租户，包裹）。
func TestADeliveryWithoutACancellationStillFormsTheFinalAfterReadingTheCancellationView(t *testing.T) {
	fixture := newFinalFixture(t)

	result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ParcelFinalFormed {
		t.Fatalf("outcome = %q, want FINAL_FORMED", result.Outcome())
	}
	if fixture.cancellations.asked == 0 {
		t.Fatal("没读过取消视图就形成了终局")
	}
}

// Covers: PS CONTEXT「同一包裹的取消提交、收寄采用与面单渠道服务的终局判断……」在网络服务有效交付终局这一侧——锁指名命令的
// （租户，包裹），取在读已成立的取消与既有终局之前；取不到锁或没装锁就一道都不读、不落记录、不交意图，停在 FINAL_STORE_UNAVAILABLE
// 并带续办引用（锁守的是既有终局分派那几道读，恢复动作同为重试）。取消机制未接入时不读也不取锁，交付照常形成终局。
func TestTheFinalAdoptionQueuesOnItsParcelBeforeReadingTheCancellation(t *testing.T) {
	t.Run("the lock comes before every read it guards", func(t *testing.T) {
		fixture := newFinalFixture(t)
		fixture.lock.onLock = func() {
			if n := fixture.cancellations.asked; n != 0 {
				t.Errorf("取锁之前已读了 %d 次取消——锁外读到的「没有取消」正是要防的那一份旧状态", n)
			}
			if n := len(fixture.finals.byKey); n != 0 {
				t.Errorf("取锁之前终局库已有 %d 行", n)
			}
		}

		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelFinalFormed {
			t.Fatalf("outcome = %q, want FINAL_FORMED", result.Outcome())
		}
		if len(fixture.lock.locked) != 1 || fixture.lock.locked[0] != "tenant-1/parcel-1" {
			t.Fatalf("取锁 = %v, want 恰一次、指名 tenant-1/parcel-1", fixture.lock.locked)
		}
	})

	t.Run("a request that is not accepted takes no lock", func(t *testing.T) {
		fixture := newFinalFixture(t)
		foreign := finalCommand(t, "parcel-1")
		foreign.Identity = sourceIdentity(t, "tenant-1", "customer-2", "source-a", "key-1")

		result, err := fixture.handler.Handle(context.Background(), foreign)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.FinalRequestNotAccepted || len(fixture.lock.locked) != 0 {
			t.Fatalf("outcome = %q locked = %v; 未受理的探测不排队、也不泄露", result.Outcome(), fixture.lock.locked)
		}
	})

	stops := map[string]func(*finalFixture){
		"an unavailable lock": func(fixture *finalFixture) { fixture.lock.err = errors.New("lock wait cancelled") },
		"an unwired lock": func(fixture *finalFixture) {
			fixture.rebuild(func(deps *application.FormParcelFinalDeps) { deps.BoundaryLock = nil })
		},
	}
	for name, arrange := range stops {
		t.Run(name, func(t *testing.T) {
			fixture := newFinalFixture(t)
			arrange(fixture)

			result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.ParcelFinalUndecided ||
				result.UndecidedReason() != application.FinalStoreUnavailable ||
				result.ContinuationReference().String() == "" {
				t.Fatalf("outcome = %q/%q continuation = %q, want FINAL_UNDECIDED/FINAL_STORE_UNAVAILABLE 带续办引用",
					result.Outcome(), result.UndecidedReason(), result.ContinuationReference())
			}
			if fixture.cancellations.asked != 0 || fixture.finals.saved != 0 || len(fixture.downstream.intents) != 0 {
				t.Fatalf("asked = %d saved = %d intents = %d; 没排上队就什么都不读、不落、不交",
					fixture.cancellations.asked, fixture.finals.saved, len(fixture.downstream.intents))
			}
		})
	}

	t.Run("an absent cancellation mechanism neither reads nor locks", func(t *testing.T) {
		fixture := newFinalFixture(t)
		fixture.rebuild(func(deps *application.FormParcelFinalDeps) { deps.Cancellations = nil })

		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelFinalFormed || len(fixture.lock.locked) != 0 {
			t.Fatalf("outcome = %q locked = %v; 取消机制未接入，没有取消可争，排队没有对象",
				result.Outcome(), fixture.lock.locked)
		}
	})
}

// Covers: ADR-0029 读不动才是未决——取消视图读不回，终局不放行也不落不采用记录（没读到不等于没有，更不等于有），停在
// FINAL_STORE_UNAVAILABLE 带续办引用，重投再读。读口答了「有取消」却指不出是哪一份、或指的是别的包裹，是读口坏了，响亮报错，
// 不当作没有取消让交付盖过一份真取消，也不编一个依据。
func TestAnUnreadableOrBrokenCancellationViewNeverLetsTheDeliveryThrough(t *testing.T) {
	t.Run("an unreadable view is undecided", func(t *testing.T) {
		fixture := newFinalFixture(t)
		fixture.cancellations.err = errors.New("cancellation store down")

		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelFinalUndecided ||
			result.UndecidedReason() != application.FinalStoreUnavailable ||
			result.ContinuationReference().String() == "" {
			t.Fatalf("outcome = %q/%q, want FINAL_UNDECIDED/FINAL_STORE_UNAVAILABLE 带续办引用",
				result.Outcome(), result.UndecidedReason())
		}
		if fixture.finals.saved != 0 || len(fixture.downstream.intents) != 0 {
			t.Fatalf("saved = %d intents = %d; 读不回不落终局也不落不采用", fixture.finals.saved, len(fixture.downstream.intents))
		}
	})

	t.Run("a cancellation that cannot be named is an error", func(t *testing.T) {
		fixture := newFinalFixture(t)
		fixture.cancellations.cancelled = true

		if _, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1")); err == nil {
			t.Fatal("读口答了有取消却指不出是哪一份，却没报错")
		}
		if fixture.finals.saved != 0 {
			t.Fatalf("saved = %d", fixture.finals.saved)
		}
	})

	t.Run("a cancellation of another parcel is an error", func(t *testing.T) {
		fixture := newFinalFixture(t)
		fixture.cancellations.cancelled = true
		fixture.cancellations.cancellation = cancelledAt(t, finalOccurredAt.Add(-2*time.Hour))

		if _, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-2")); err == nil {
			t.Fatal("读口交回了别的包裹的取消，却没报错")
		}
		if fixture.finals.saved != 0 {
			t.Fatalf("saved = %d", fixture.finals.saved)
		}
	})
}
