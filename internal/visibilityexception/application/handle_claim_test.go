package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var claimSubmittedAt = time.Date(2026, 8, 13, 6, 0, 0, 0, time.UTC)

type claimKey struct {
	batch domain.ClaimBatchReference
	item  domain.ClaimItemID
}

type claimStoreDouble struct {
	claims  map[claimKey]*domain.ClaimItem
	findErr error
	saveErr error
	saves   int
}

func newClaimStore() *claimStoreDouble {
	return &claimStoreDouble{claims: map[claimKey]*domain.ClaimItem{}}
}

func (double *claimStoreDouble) FindByBatchItem(
	_ context.Context,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
) (*domain.ClaimItem, bool, error) {
	if double.findErr != nil {
		return nil, false, double.findErr
	}
	claim, found := double.claims[claimKey{batch: batch, item: item}]
	return claim, found, nil
}

func (double *claimStoreDouble) Save(_ context.Context, claim *domain.ClaimItem) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.claims[claimKey{batch: claim.Batch(), item: claim.ID()}] = claim
	double.saves++
	return nil
}

type eligibilityRuleDouble struct {
	answer     ports.EligibilityAnswer
	configured bool
	err        error
	calls      int
}

func (double *eligibilityRuleDouble) ScreenClaim(
	_ context.Context,
	_ ports.EligibilityQuery,
) (ports.EligibilityAnswer, bool, error) {
	double.calls++
	if double.err != nil {
		return ports.EligibilityAnswer{}, false, double.err
	}
	return double.answer, double.configured, nil
}

type recoveryKey struct {
	caseID       domain.CaseID
	counterparty domain.CounterpartyReference
	scope        domain.RequestScopeReference
}

type recoveryStoreDouble struct {
	byID    map[domain.RecoveryMatterID]domain.RecoveryMatter
	current map[recoveryKey]domain.RecoveryMatter
	actions []domain.RecoveryAction
	findErr error
	saveErr error
}

func newRecoveryStore() *recoveryStoreDouble {
	return &recoveryStoreDouble{
		byID:    map[domain.RecoveryMatterID]domain.RecoveryMatter{},
		current: map[recoveryKey]domain.RecoveryMatter{},
	}
}

func (double *recoveryStoreDouble) FindByID(
	_ context.Context,
	id domain.RecoveryMatterID,
) (domain.RecoveryMatter, bool, error) {
	if double.findErr != nil {
		return domain.RecoveryMatter{}, false, double.findErr
	}
	matter, found := double.byID[id]
	return matter, found, nil
}

func (double *recoveryStoreDouble) FindCurrent(
	_ context.Context,
	caseID domain.CaseID,
	counterparty domain.CounterpartyReference,
	scope domain.RequestScopeReference,
) (domain.RecoveryMatter, bool, error) {
	if double.findErr != nil {
		return domain.RecoveryMatter{}, false, double.findErr
	}
	matter, found := double.current[recoveryKey{caseID: caseID, counterparty: counterparty, scope: scope}]
	return matter, found, nil
}

func (double *recoveryStoreDouble) Save(_ context.Context, matter domain.RecoveryMatter) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.byID[matter.ID()] = matter
	double.current[recoveryKey{caseID: matter.Case(), counterparty: matter.Counterparty(), scope: matter.Scope()}] = matter
	return nil
}

func (double *recoveryStoreDouble) CountActions(
	_ context.Context,
	matter domain.RecoveryMatterID,
	kind domain.RecoveryActionKind,
) (int, error) {
	if double.findErr != nil {
		return 0, double.findErr
	}
	count := 0
	for _, action := range double.actions {
		if action.Matter() == matter && action.Kind() == kind {
			count++
		}
	}
	return count, nil
}

func (double *recoveryStoreDouble) AppendAction(_ context.Context, action domain.RecoveryAction) error {
	if double.saveErr != nil {
		return double.saveErr
	}
	double.actions = append(double.actions, action)
	return nil
}

type recoveryIdentityDouble struct {
	next int
	err  error
}

func (double *recoveryIdentityDouble) NextRecoveryMatterID(_ context.Context) (domain.RecoveryMatterID, error) {
	if double.err != nil {
		return domain.RecoveryMatterID{}, double.err
	}
	double.next++
	return domain.NewRecoveryMatterID("recovery-" + string(rune('0'+double.next)))
}

type liabilityDownstreamDouble struct {
	intents []ports.LiabilityHandoffIntent
	err     error
}

func (double *liabilityDownstreamDouble) HandOffLiability(
	_ context.Context,
	intent ports.LiabilityHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type claimFixture struct {
	handler     *application.HandleClaimHandler
	claims      *claimStoreDouble
	eligibility *eligibilityRuleDouble
	recoveries  *recoveryStoreDouble
	identities  *recoveryIdentityDouble
	settlement  *liabilityDownstreamDouble
}

func newClaimFixture(t *testing.T) *claimFixture {
	t.Helper()
	fixture := &claimFixture{
		claims: newClaimStore(),
		eligibility: &eligibilityRuleDouble{
			answer:     ports.EligibilityAnswer{Screen: domain.ClaimEligible, Basis: "eligibility-rules/v1"},
			configured: true,
		},
		recoveries: newRecoveryStore(),
		identities: &recoveryIdentityDouble{},
		settlement: &liabilityDownstreamDouble{},
	}
	fixture.handler = application.NewHandleClaimHandler(application.HandleClaimDeps{
		Claims:      fixture.claims,
		Eligibility: fixture.eligibility,
		Recoveries:  fixture.recoveries,
		Identities:  fixture.identities,
		Settlement:  fixture.settlement,
		Clock:       fixedClock{at: claimSubmittedAt.Add(time.Hour)},
	})
	return fixture
}

func receiveCommand(t *testing.T, item string) application.ReceiveClaimCommand {
	t.Helper()
	return application.ReceiveClaimCommand{
		Batch:       mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:        mustValue(t, domain.NewClaimItemID, item),
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: claimSubmittedAt,
	}
}

func screenCommand(t *testing.T, item string) application.ScreenClaimCommand {
	t.Helper()
	return application.ScreenClaimCommand{
		Batch: mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:  mustValue(t, domain.NewClaimItemID, item),
	}
}

func concludeCommand(t *testing.T, item string) application.ConcludeClaimCommand {
	t.Helper()
	return application.ConcludeClaimCommand{
		Batch:      mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:       mustValue(t, domain.NewClaimItemID, item),
		Conclusion: domain.LiabilityPartiallyEstablished,
		ReviewBy:   claimSubmittedAt.Add(30 * 24 * time.Hour),
	}
}

func openRecoveryCommand(t *testing.T) application.OpenRecoveryCommand {
	t.Helper()
	return application.OpenRecoveryCommand{
		Case:         mustValue(t, domain.NewCaseID, "case-1"),
		Counterparty: mustValue(t, domain.NewCounterpartyReference, "supplier-1"),
		Basis:        mustValue(t, domain.NewLiabilityBasisReference, "supplier-agreement/v1"),
		LegalEntity:  mustValue(t, domain.NewLegalEntityReference, "entity-1"),
		Scope:        mustValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Evidence:     mustValue(t, domain.NewRequestEvidenceReference, "evidence/loss-1"),
		Deadline:     claimSubmittedAt.Add(14 * 24 * time.Hour),
	}
}

// Covers: CONTEXT「客户可以通过一个索赔提交批次提交多个索赔项。每个索赔项必须固定
// 一个货主客户账户、合同责任范围、目标包裹……并独立受理」与「先保留原始提交事实」
// ——批内两项各自独立、重复到达返回原项不重存。点名 `AT-VE-114`「一个提交批次包含
// 多个索赔项→逐项审核，允许部分待补、部分受理和部分不予受理」的逐项独立半边。
func TestClaimsInOneBatchAreReceivedItemByItem(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()

	first, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive first: %v", err)
	}
	second, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-2"))
	if err != nil {
		t.Fatalf("receive second: %v", err)
	}
	if first.Outcome() != application.ClaimReceived || second.Outcome() != application.ClaimReceived {
		t.Fatalf("outcomes = %q/%q, want two independent receipts", first.Outcome(), second.Outcome())
	}
	firstClaim, _ := first.Claim()
	if _, screened := firstClaim.Screen(); screened {
		t.Fatal("receiving already claims an eligibility screen; the three judgments must stay separate")
	}

	replay, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.ClaimExistingResult {
		t.Fatalf("outcome = %q, want CLAIM_EXISTING_RESULT", replay.Outcome())
	}
	if fixture.claims.saves != 2 {
		t.Fatalf("saves = %d, want 2; the replay must not re-save", fixture.claims.saves)
	}
}

// Covers: CONTEXT「系统必须先保留原始提交事实，再按申请人授权、客户账户、合同版本、
// 索赔时限、目标范围、重复关系和最低材料要求判断资格」——资格由版本化目录判，依据
// 入账；已审过的不再审。
func TestEligibilityIsScreenedOnceWithItsBasis(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()

	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}

	screened, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if screened.Outcome() != application.ClaimScreened {
		t.Fatalf("outcome = %q, want CLAIM_SCREENED", screened.Outcome())
	}
	claim, _ := screened.Claim()
	if screen, ok := claim.Screen(); !ok || screen != domain.ClaimEligible {
		t.Fatal("the screen result was not recorded on the claim")
	}

	again, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("repeat screen: %v", err)
	}
	if again.Outcome() != application.ClaimScreenAlreadyRecorded {
		t.Fatalf("outcome = %q, want SCREEN_ALREADY_RECORDED", again.Outcome())
	}
}

// Covers: 实例半边红线——资格目录（索赔时限、材料要求、授权）是待登记实例参数，未
// 配置时未决等租户登记：默认受理与默认拒赔都是虚构；与目录调不通分开（恢复动作不同）。
func TestUnconfiguredEligibilityCatalogueIsUndecidedNotDefaulted(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	fixture.eligibility.configured = false

	result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if result.Outcome() != application.HandleClaimUndecided ||
		result.UndecidedReason() != application.EligibilityCatalogueNotConfigured {
		t.Fatalf("result = %q/%q, want UNDECIDED/ELIGIBILITY_CATALOGUE_NOT_CONFIGURED",
			result.Outcome(), result.UndecidedReason())
	}
	claim, _, _ := fixture.claims.FindByBatchItem(ctx,
		mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		mustValue(t, domain.NewClaimItemID, "item-1"))
	if _, screened := claim.Screen(); screened {
		t.Fatal("an unconfigured catalogue still recorded a screen result")
	}
	if application.EligibilityCatalogueNotConfigured == application.EligibilityRulesUnavailable {
		t.Fatal("not-configured and unavailable share one reason; their recovery actions diverge")
	}
}

// Covers: CONTEXT「收到客户索赔、通过资格审核和确认赔偿责任是不同判断」——资格未审
// 形不成责任结论（领域把门，编排答未受理）；过审后结论入账并把意图交给结算（`UC-SA-007`
// 的上游源——金额由结算形成，这里只交结论）。
func TestLiabilityConcludesOnlyAfterEligibilityAndHandsOffToSettlement(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}

	premature, err := fixture.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("premature conclude: %v", err)
	}
	if premature.Outcome() != application.HandleClaimNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED before eligibility", premature.Outcome())
	}

	if _, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1")); err != nil {
		t.Fatalf("screen: %v", err)
	}
	concluded, err := fixture.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if concluded.Outcome() != application.ClaimConcluded {
		t.Fatalf("outcome = %q, want CLAIM_CONCLUDED", concluded.Outcome())
	}
	if len(fixture.settlement.intents) != 1 ||
		fixture.settlement.intents[0].Claim.ID().String() != "item-1" {
		t.Fatal("exactly one settlement intent claimed by the claim item was expected")
	}

	again, err := fixture.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("repeat conclude: %v", err)
	}
	if again.Outcome() != application.ClaimConclusionAlreadyRecorded {
		t.Fatalf("outcome = %q, want CONCLUSION_ALREADY_RECORDED", again.Outcome())
	}
	if len(fixture.settlement.intents) != 2 {
		t.Fatal("the replay must resend the same settlement intent (ADR-0043)")
	}
}

// Covers: CONTEXT「复核形成新的索赔结论版本，关联原结论……原结论不得覆盖」——期限内
// 复核换出新版本、原结论保留在 PriorConclusion，新版本随意图再交结算。点名 `AT-VE-127`
// 「同一范围在复核期限内出现有效异议或关键新证据→形成新责任结论版本，原结论保留」。
func TestAReviewWithinTheWindowFormsANewConclusionVersionKeepingThePrior(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if _, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1")); err != nil {
		t.Fatalf("screen: %v", err)
	}
	if _, err := fixture.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1")); err != nil {
		t.Fatalf("conclude: %v", err)
	}
	intentsBefore := len(fixture.settlement.intents)

	reviewed, err := fixture.handler.ReviewClaim(ctx, application.ReviewClaimCommand{
		Batch:      mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:       mustValue(t, domain.NewClaimItemID, "item-1"),
		Conclusion: domain.LiabilityFullyEstablished,
	})
	if err != nil {
		t.Fatalf("review: %v", err)
	}

	if reviewed.Outcome() != application.ClaimReviewed {
		t.Fatalf("outcome = %q, want CLAIM_REVIEWED", reviewed.Outcome())
	}
	claim, _ := reviewed.Claim()
	conclusion, _ := claim.Conclusion()
	if conclusion != domain.LiabilityFullyEstablished {
		t.Fatalf("conclusion = %q, want the reviewed version", conclusion)
	}
	prior, kept := claim.PriorConclusion()
	if !kept || prior != domain.LiabilityPartiallyEstablished {
		t.Fatal("the review discarded the original conclusion instead of keeping it")
	}
	if len(fixture.settlement.intents) != intentsBefore+1 {
		t.Fatal("the reviewed conclusion version must reach settlement through the same seam")
	}
}

// Covers: CONTEXT「超过复核期限且不存在有效延期或其他恢复依据时，形成有依据的不受理
// 复核，不改变原责任结论」——届满是业务答案不是故障。点名 `AT-VE-129`「复核请求超过
// 期限且无有效延期或恢复依据→不受理复核，原责任结论不变」。
func TestAReviewAfterTheWindowIsRefusedKeepingTheOriginalConclusion(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if _, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1")); err != nil {
		t.Fatalf("screen: %v", err)
	}
	command := concludeCommand(t, "item-1")
	command.ReviewBy = claimSubmittedAt.Add(90 * time.Minute) // 时钟在 +1h，复核截止 +1.5h
	if _, err := fixture.handler.ConcludeClaim(ctx, command); err != nil {
		t.Fatalf("conclude: %v", err)
	}

	// 把时钟拨过复核截止再复核。
	late := newClaimFixture(t)
	late.claims = fixture.claims
	late.handler = application.NewHandleClaimHandler(application.HandleClaimDeps{
		Claims:      fixture.claims,
		Eligibility: fixture.eligibility,
		Recoveries:  fixture.recoveries,
		Identities:  fixture.identities,
		Settlement:  fixture.settlement,
		Clock:       fixedClock{at: claimSubmittedAt.Add(2 * time.Hour)},
	})
	result, err := late.handler.ReviewClaim(ctx, application.ReviewClaimCommand{
		Batch:      mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:       mustValue(t, domain.NewClaimItemID, "item-1"),
		Conclusion: domain.LiabilityFullyEstablished,
	})
	if err != nil {
		t.Fatalf("late review: %v", err)
	}

	if result.Outcome() != application.ClaimReviewWindowClosed {
		t.Fatalf("outcome = %q, want REVIEW_WINDOW_CLOSED", result.Outcome())
	}
	claim, _ := result.Claim()
	conclusion, _ := claim.Conclusion()
	if conclusion != domain.LiabilityPartiallyEstablished {
		t.Fatal("a refused review still changed the original conclusion")
	}
}

// Covers: CONTEXT「供应商或保险追偿在相应通知或主张条件成立时即可独立发起，无需等待
// 客户提出索赔、客户责任结论或客户赔付」——索赔库里空无一物照样开得出事项；幂等按
// （案件+相对方+范围）。点名 `AT-VE-131`「供应商责任通知条件先成立→建立追偿事项并
// 进入准备，不等待客户索赔或赔付」。
func TestARecoveryMatterOpensIndependentlyOfAnyClaim(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()

	opened, err := fixture.handler.OpenRecovery(ctx, openRecoveryCommand(t))
	if err != nil {
		t.Fatalf("open recovery: %v", err)
	}
	if opened.Outcome() != application.RecoveryOpened {
		t.Fatalf("outcome = %q, want RECOVERY_OPENED", opened.Outcome())
	}
	if len(fixture.claims.claims) != 0 {
		t.Fatal("opening a recovery touched the claim store")
	}

	replay, err := fixture.handler.OpenRecovery(ctx, openRecoveryCommand(t))
	if err != nil {
		t.Fatalf("replay open: %v", err)
	}
	if replay.Outcome() != application.RecoveryExistingResult {
		t.Fatalf("outcome = %q, want RECOVERY_EXISTING_RESULT", replay.Outcome())
	}
	if fixture.identities.next != 1 {
		t.Fatalf("issued %d matter identities, want 1", fixture.identities.next)
	}
}

// Covers: CONTEXT「每个追偿通知或主张动作必须分别记录准备完成、对外提交……提交失败
// 和送达失败」与「期限内可以按规则重试……所有尝试和内容版本保留」——失败后的重试是
// 新记录 attempt 递增；预先通知与正式主张各有各的尝试序列。点名 `AT-VE-136`「提交
// 或送达失败后在期限内重试→保留全部尝试，失败不解释为对方拒绝」与 `AT-VE-133`
// 「协议同时要求预先通知和后续正式主张→分别建动作和期限，一个动作不能替代另一个」
// 的分序半边。
func TestRecoveryActionsKeepEveryAttemptPerKind(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	opened, err := fixture.handler.OpenRecovery(ctx, openRecoveryCommand(t))
	if err != nil {
		t.Fatalf("open recovery: %v", err)
	}
	matter, _ := opened.Recovery()
	record := func(kind domain.RecoveryActionKind, milestone domain.RecoveryActionMilestone) application.HandleClaimResult {
		result, err := fixture.handler.RecordRecovery(ctx, application.RecordRecoveryCommand{
			Matter:     matter.ID(),
			Kind:       kind,
			ContentRef: "notice-content/v1",
			Milestone:  milestone,
			OccurredAt: claimSubmittedAt.Add(2 * time.Hour),
		})
		if err != nil {
			t.Fatalf("record recovery: %v", err)
		}
		return result
	}

	failed := record(domain.PreliminaryNotice, domain.SubmissionFailed)
	action, _ := failed.RecoveryAction()
	if action.Attempt() != 1 {
		t.Fatalf("attempt = %d, want 1", action.Attempt())
	}

	retried := record(domain.PreliminaryNotice, domain.ActionSubmitted)
	action, _ = retried.RecoveryAction()
	if action.Attempt() != 2 {
		t.Fatalf("attempt = %d, want 2; the retry is a new record", action.Attempt())
	}

	assertion := record(domain.FormalAssertion, domain.ActionPrepared)
	action, _ = assertion.RecoveryAction()
	if action.Attempt() != 1 {
		t.Fatalf("assertion attempt = %d, want its own sequence starting at 1", action.Attempt())
	}
	if len(fixture.recoveries.actions) != 3 {
		t.Fatalf("actions kept = %d, want all 3 attempts", len(fixture.recoveries.actions))
	}
}

// Covers: 未决语义——索赔库、资格规则与追偿库调不通分别停在各自原因，不虚构受理、
// 资格或事项。
func TestUnavailableClaimDependenciesAreUndecidedUnderTheirOwnReasons(t *testing.T) {
	store := newClaimFixture(t)
	store.claims.findErr = errors.New("claim store unavailable")
	result, err := store.handler.ReceiveClaim(context.Background(), receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if result.Outcome() != application.HandleClaimUndecided ||
		result.UndecidedReason() != application.ClaimStoreUnavailable {
		t.Fatalf("result = %q/%q, want UNDECIDED/CLAIM_STORE_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
	}

	rules := newClaimFixture(t)
	if _, err := rules.handler.ReceiveClaim(context.Background(), receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	rules.eligibility.err = errors.New("eligibility rules unavailable")
	result, err = rules.handler.ScreenClaim(context.Background(), screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if result.UndecidedReason() != application.EligibilityRulesUnavailable {
		t.Fatalf("reason = %q, want ELIGIBILITY_RULES_UNAVAILABLE", result.UndecidedReason())
	}

	recovery := newClaimFixture(t)
	recovery.recoveries.findErr = errors.New("recovery store unavailable")
	result, err = recovery.handler.OpenRecovery(context.Background(), openRecoveryCommand(t))
	if err != nil {
		t.Fatalf("open recovery: %v", err)
	}
	if result.UndecidedReason() != application.RecoveryStoreUnavailable {
		t.Fatalf("reason = %q, want RECOVERY_STORE_UNAVAILABLE", result.UndecidedReason())
	}
}

// Covers: ADR-0043「首次交付失败不改写业务结果……另留一条发布续办引用」——结算意图
// 交不出去时结论保持已成立，只留可续办引用。
func TestAFailedSettlementHandoffKeepsTheConclusionWithAResumableReference(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	if _, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1")); err != nil {
		t.Fatalf("screen: %v", err)
	}
	fixture.settlement.err = errors.New("settlement seam unavailable")

	result, err := fixture.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}

	if result.Outcome() != application.ClaimConcluded {
		t.Fatalf("outcome = %q, want CLAIM_CONCLUDED; a failed handoff must not rewrite the conclusion", result.Outcome())
	}
	if result.HandoffReference() == "" {
		t.Fatal("a failed handoff left no resumable reference")
	}
}

// Covers: 受理半边——要件缺一即未受理，不读依赖不补默认。
func TestAClaimCommandMissingAnEssentialIsNotAccepted(t *testing.T) {
	fixture := newClaimFixture(t)

	missing := receiveCommand(t, "item-1")
	missing.Contract = domain.ContractScopeReference{}
	result, err := fixture.handler.ReceiveClaim(context.Background(), missing)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if result.Outcome() != application.HandleClaimNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}
	if fixture.claims.saves != 0 {
		t.Fatal("an unaccepted command still wrote the claim store")
	}
}
