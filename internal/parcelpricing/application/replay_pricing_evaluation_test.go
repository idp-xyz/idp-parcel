package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// planLoaderDouble 按方案版本引用交回一版方案；不在册答 false，err 非空时原样交回。
type planLoaderDouble struct {
	byReference map[string]domain.PricingPlanVersion
	err         error
	calls       int
}

func (double *planLoaderDouble) FindByReference(
	_ context.Context,
	_ domain.TenantID,
	reference domain.VersionReference,
) (domain.PricingPlanVersion, bool, error) {
	double.calls++
	if double.err != nil {
		return domain.PricingPlanVersion{}, false, double.err
	}
	plan, found := double.byReference[reference.ID()+"@"+reference.Version()]
	return plan, found, nil
}

type replayFixture struct {
	handler *application.ReplayPricingEvaluationHandler
	store   *evaluationStoreDouble
	plans   *planLoaderDouble
}

func newReplayFixture(t *testing.T) *replayFixture {
	t.Helper()
	fixture := &replayFixture{
		store: &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		plans: &planLoaderDouble{byReference: map[string]domain.PricingPlanVersion{}},
	}
	fixture.handler = application.NewReplayPricingEvaluationHandler(application.ReplayPricingEvaluationDeps{
		Store: fixture.store,
		Plans: fixture.plans,
	})
	return fixture
}

// recordOriginal 把一份按 plan 形成的 `S` 评价放进评价册，并把 plan 登进方案读口——回放的起点。
func (fixture *replayFixture) recordOriginal(t *testing.T, id string, plan domain.PricingPlanVersion, zone string) domain.PricingEvaluation {
	t.Helper()
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), plan, minimalInput(t, zone), domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("evaluation request: %v", err)
	}
	original := domain.EvaluatePricing(request)
	fixture.store.byID[id] = original
	fixture.plans.byReference[plan.Reference().ID()+"@"+plan.Reference().Version()] = plan
	return original
}

func replayCommand(t *testing.T, tenant, original, replayID string, evidence domain.EvidenceKind) application.ReplayPricingEvaluationCommand {
	t.Helper()
	return application.ReplayPricingEvaluationCommand{
		Tenant:   mustValue(t, domain.NewTenantID, tenant),
		Original: mustValue(t, domain.NewEvaluationID, original),
		ReplayID: mustValue(t, domain.NewEvaluationID, replayID),
		Evidence: evidence,
	}
}

// Covers: PP CONTEXT「同一评价输入……必须产生相同结果；重放必须使用新的评价引用，不得复用原评价 ID，
// 不修改原评价或来源事实」经编排端到端——按原评价记录的方案版本引用取回原方案，回放评价以新引用
// 入册、回指原评价、语义摘要与原评价逐字相等、原评价一字不动；同回放引用第二次到达返原记录不重入册；
// 同回放引用装另一份原评价是冒名冲突不顶替。回放**不交 EvaluationHandoff**：依赖结构上没有这一格。
func TestReplayReproducesTheOriginalUnderANewReferenceAndHandsNothingOff(t *testing.T) {
	fixture := newReplayFixture(t)
	original := fixture.recordOriginal(t, "eval-original", minimalPlan(t), "Z1")
	if original.Status() != domain.EvaluationCompleted {
		t.Fatalf("夹具没造出一份已完成的原评价：%s", original.Status())
	}

	result, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-1", domain.EvidenceSynthetic))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if result.Outcome != application.ReplayRecorded {
		t.Fatalf("outcome = %s, want RECORDED", result.Outcome)
	}
	replayed, ok := result.Evaluation, result.HasEvaluation
	if !ok {
		t.Fatal("已入册的回放没交回评价")
	}
	if replayed.Status() != domain.EvaluationCompleted || replayed.SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("重现失败：status=%s digest 相等=%t", replayed.Status(), replayed.SemanticDigest() == original.SemanticDigest())
	}
	if replayOf, isReplay := replayed.ReplayOf(); !isReplay || replayOf != original.ID() || replayed.ID() == original.ID() {
		t.Fatalf("回放评价没有以新引用回指原评价：id=%s replayOf=%v", replayed.ID(), replayOf)
	}
	if replayed.Evidence() != domain.EvidenceSynthetic {
		t.Fatalf("evidence = %s, want S", replayed.Evidence())
	}
	if fixture.store.saved != 1 || fixture.store.byID["eval-original"].SemanticDigest() != original.SemanticDigest() {
		t.Fatalf("saved=%d；回放要么没入册、要么动了原评价", fixture.store.saved)
	}

	again, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-1", domain.EvidenceSynthetic))
	if err != nil {
		t.Fatalf("repeat replay: %v", err)
	}
	if again.Outcome != application.ReplayExistingResult || fixture.store.saved != 1 {
		t.Fatalf("重复请求 outcome=%s saved=%d；同回放引用第二次到达该返原记录不重入册", again.Outcome, fixture.store.saved)
	}
	returned := again.Evaluation
	if returned.ID() != replayed.ID() {
		t.Fatalf("返回的不是在册那份回放：%s", returned.ID())
	}

	fixture.recordOriginal(t, "eval-other-original", minimalPlan(t), "Z1")
	impostor, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-other-original", "eval-replay-1", domain.EvidenceSynthetic))
	if err != nil {
		t.Fatalf("impostor replay: %v", err)
	}
	if impostor.Outcome != application.ReplayIdentityConflict || fixture.store.saved != 1 {
		t.Fatalf("同回放引用装另一份原评价 outcome=%s saved=%d；必须是冲突且不顶替", impostor.Outcome, fixture.store.saved)
	}

	handoff := reflect.TypeOf((*ports.EvaluationHandoff)(nil)).Elem()
	deps := reflect.TypeOf(application.ReplayPricingEvaluationDeps{})
	for index := 0; index < deps.NumField(); index++ {
		if field := deps.Field(index); field.Type == handoff || field.Type.Implements(handoff) {
			t.Fatalf("ReplayPricingEvaluationDeps.%s 是一格交付口：回放结果不是新费用，一份带新引用的回放交出去就是一笔重复的费用采用", field.Name)
		}
	}
}

// Covers: PP CONTEXT「重放必须使用原版本清单，不读取当前最新版本替代」的另一半——原方案版本不在册、
// 或其快照按本构建重建不了时，是**结构上重放不了**：不形成评价、不入册、不拿在用版本顶上，各自成格
// （恢复动作不同：前者能登，后者只能等支持该形状的构建）；原评价不在本租户册上（含别人租户的评价）
// 答不在册；读口故障答未决并交回成因。
func TestReplayAnswersStructuralImpossibilityWithoutFormingAnEvaluation(t *testing.T) {
	fixture := newReplayFixture(t)

	missing, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-nowhere", "eval-replay-x", domain.EvidenceSynthetic))
	if err != nil || missing.Outcome != application.ReplayOriginalNotFound {
		t.Fatalf("原评价不在册：outcome=%s err=%v", missing.Outcome, err)
	}
	if fixture.plans.calls != 0 {
		t.Fatalf("原评价都没有还去读了方案：calls=%d", fixture.plans.calls)
	}

	fixture.recordOriginal(t, "eval-original", minimalPlan(t), "Z1")
	stranger, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-9", "eval-original", "eval-replay-x", domain.EvidenceSynthetic))
	if err != nil || stranger.Outcome != application.ReplayOriginalNotFound {
		t.Fatalf("别人租户的评价：outcome=%s err=%v，想要 ORIGINAL_NOT_FOUND（越权探针与真不存在同答）", stranger.Outcome, err)
	}

	delete(fixture.plans.byReference, "plan-1@v1")
	unregistered, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-x", domain.EvidenceSynthetic))
	if err != nil || unregistered.Outcome != application.ReplayPlanVersionNotOnRegister {
		t.Fatalf("原方案版本不在册：outcome=%s err=%v", unregistered.Outcome, err)
	}
	if formed := unregistered.HasEvaluation; formed || fixture.store.saved != 0 {
		t.Fatalf("结构上重放不了却形成了评价：formed=%t saved=%d", formed, fixture.store.saved)
	}

	fixture.plans.err = fmt.Errorf("find price card version: %w", domain.ErrCanonicalizationVersionUnsupported)
	unsupported, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-x", domain.EvidenceSynthetic))
	if err != nil || unsupported.Outcome != application.ReplayPlanCanonicalizationUnsupported {
		t.Fatalf("原方案快照重建不了：outcome=%s err=%v", unsupported.Outcome, err)
	}
	if fixture.store.saved != 0 {
		t.Fatalf("规范化不支持却入册了：saved=%d", fixture.store.saved)
	}

	fixture.plans.err = errors.New("register unreachable")
	stalled, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-x", domain.EvidenceSynthetic))
	if err == nil || stalled.Outcome != application.ReplayUndecided {
		t.Fatalf("读口故障：outcome=%s err=%v，想要 UNDECIDED 且带成因", stalled.Outcome, err)
	}
}

// Covers: 领域门在编排上的形状——新引用等于原引用、`S` 想重放成 `R`、命令缺租户，都是改请求那一格
// （NOT_ACCEPTED），未到达评价册。
func TestReplayRefusesReferenceReuseEvidenceUpgradeAndBlankCommands(t *testing.T) {
	fixture := newReplayFixture(t)
	fixture.recordOriginal(t, "eval-original", minimalPlan(t), "Z1")

	reused, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-original", domain.EvidenceSynthetic))
	if err != nil || reused.Outcome != application.ReplayNotAccepted {
		t.Fatalf("复用原引用：outcome=%s err=%v", reused.Outcome, err)
	}

	upgraded, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-r", domain.EvidenceReplay))
	if err != nil || upgraded.Outcome != application.ReplayNotAccepted {
		t.Fatalf("S 重放成 R：outcome=%s err=%v", upgraded.Outcome, err)
	}

	blank := replayCommand(t, "tenant-1", "eval-original", "eval-replay-b", domain.EvidenceSynthetic)
	blank.Tenant = domain.TenantID{}
	unaddressed, err := fixture.handler.Handle(context.Background(), blank)
	if err != nil || unaddressed.Outcome != application.ReplayNotAccepted {
		t.Fatalf("缺租户：outcome=%s err=%v", unaddressed.Outcome, err)
	}
	if fixture.store.saved != 0 {
		t.Fatalf("被拒的请求入册了：saved=%d", fixture.store.saved)
	}
}

// Covers: PP CONTEXT「版本引用相同、规范化版本也相同而内容摘要不同，视为版本内容冲突，不得继续重放」——
// 登记册上那一版的内容与原评价冻结的内容摘要不符时，回放照样入册（冲突是版本化的计算事实），
// 状态为冲突、问题项 PLAN_CONTENT_MISMATCH、无金额；编排不把它折成成功。评价册故障答未决并交回成因。
func TestReplayAgainstChangedPlanContentRecordsAConflictNotASuccess(t *testing.T) {
	fixture := newReplayFixture(t)
	fixture.recordOriginal(t, "eval-original", minimalPlan(t), "Z1")
	changed := minimalPlanPriced(t, "11")
	if changed.ContentDigest() == minimalPlan(t).ContentDigest() || !changed.Manifest().Equal(minimalPlan(t).Manifest()) {
		t.Fatal("夹具没造出「同版本引用、内容不同」的方案")
	}
	fixture.plans.byReference["plan-1@v1"] = changed

	result, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-c", domain.EvidenceSynthetic))
	if err != nil || result.Outcome != application.ReplayRecorded {
		t.Fatalf("outcome=%s err=%v", result.Outcome, err)
	}
	replayed := result.Evaluation
	if replayed.Status() != domain.EvaluationConflict {
		t.Fatalf("status = %s, want CONFLICT", replayed.Status())
	}
	if _, hasTotal := replayed.Total(); hasTotal {
		t.Fatal("冲突的回放带了金额")
	}
	if issues := replayed.Issues(); len(issues) != 1 || issues[0].Code() != "PLAN_CONTENT_MISMATCH" {
		t.Fatalf("issues = %+v, want PLAN_CONTENT_MISMATCH", issues)
	}

	fixture.store.err = errors.New("store unreachable")
	stalled, err := fixture.handler.Handle(context.Background(), replayCommand(t, "tenant-1", "eval-original", "eval-replay-d", domain.EvidenceSynthetic))
	if err == nil || stalled.Outcome != application.ReplayUndecided {
		t.Fatalf("评价册故障：outcome=%s err=%v", stalled.Outcome, err)
	}
}
