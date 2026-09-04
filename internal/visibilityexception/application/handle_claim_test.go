package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

var claimSubmittedAt = time.Date(2026, 8, 13, 6, 0, 0, 0, time.UTC)

type claimKey struct {
	tenant domain.TenantID
	batch  domain.ClaimBatchReference
	item   domain.ClaimItemID
}

type claimStoreDouble struct {
	claims map[claimKey]*domain.ClaimItem
	// saveConflicts 是接下来几次写入答修订冲突：真库里的冲突要两个写入方交错才造得
	// 出来，双替身按次数摆出同一格答案，让编排侧的译法单独可证。
	saveConflicts int
	// conflictWinner 在冲突那一刻落进库，模拟抢先的另一方——受理侧撞冲突后要读回赢家，
	// 没有它那次读回只会扑空，走的就不是本该证的那条路。
	conflictWinner *domain.ClaimItem
	findErr        error
	saveErr        error
	saves          int
}

func newClaimStore() *claimStoreDouble {
	return &claimStoreDouble{claims: map[claimKey]*domain.ClaimItem{}}
}

func (double *claimStoreDouble) FindByBatchItem(
	_ context.Context,
	tenant domain.TenantID,
	batch domain.ClaimBatchReference,
	item domain.ClaimItemID,
) (*domain.ClaimItem, bool, error) {
	if double.findErr != nil {
		return nil, false, double.findErr
	}
	claim, found := double.claims[claimKey{tenant: tenant, batch: batch, item: item}]
	return claim, found, nil
}

// CountLiveScopeClaims 按（客户账户+目标范围+索赔类型）数同租户下未撤回的其他索赔，
// 与真库那条 SQL 同口径——重复关系那一维的事实由它给。
func (double *claimStoreDouble) CountLiveScopeClaims(
	_ context.Context,
	tenant domain.TenantID,
	customer domain.CustomerAccountReference,
	target domain.RequestScopeReference,
	kind domain.ClaimKindReference,
	excluding domain.ClaimItemID,
) (int, error) {
	if double.findErr != nil {
		return 0, double.findErr
	}
	count := 0
	for key, claim := range double.claims {
		if key.tenant != tenant || claim.ID() == excluding || claim.Withdrawn() {
			continue
		}
		if claim.Customer() == customer && claim.Target() == target && claim.Kind() == kind {
			count++
		}
	}
	return count, nil
}

func (double *claimStoreDouble) Save(
	_ context.Context,
	tenant domain.TenantID,
	claim *domain.ClaimItem,
) (ports.ClaimSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.ClaimSaveOutcomeInvalid, double.saveErr
	}
	if double.saveConflicts > 0 {
		double.saveConflicts--
		if double.conflictWinner != nil {
			winner := double.conflictWinner
			double.claims[claimKey{tenant: tenant, batch: winner.Batch(), item: winner.ID()}] = winner
		}
		return ports.ClaimRevisionConflict, nil
	}
	double.claims[claimKey{tenant: tenant, batch: claim.Batch(), item: claim.ID()}] = claim
	double.saves++
	return ports.ClaimSaved, nil
}

type eligibilityRuleDouble struct {
	rules     ports.EligibilityRules
	declared  bool
	err       error
	calls     int
	lastQuery ports.EligibilityQuery
}

func (double *eligibilityRuleDouble) RulesForClaim(
	_ context.Context,
	query ports.EligibilityQuery,
) (ports.EligibilityRules, bool, error) {
	double.calls++
	double.lastQuery = query
	if double.err != nil {
		return ports.EligibilityRules{}, false, double.err
	}
	return double.rules, double.declared, nil
}

// evidenceViewDouble 摆出「这项索赔已收到哪些材料」。known 为假即归集无从查起，与
// 「一件都还没收到」（received 为空且 known 为真）是两回事——后者是有效事实。
type evidenceViewDouble struct {
	received []domain.MaterialRequirementReference
	known    bool
	err      error
}

func (double *evidenceViewDouble) ReceivedMaterials(
	_ context.Context,
	_ domain.TenantID,
	_ domain.ClaimBatchReference,
	_ domain.ClaimItemID,
) ([]domain.MaterialRequirementReference, bool, error) {
	if double.err != nil {
		return nil, false, double.err
	}
	return double.received, double.known, nil
}

type recoveryKey struct {
	tenant       domain.TenantID
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
	_ domain.TenantID,
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
	tenant domain.TenantID,
	caseID domain.CaseID,
	counterparty domain.CounterpartyReference,
	scope domain.RequestScopeReference,
) (domain.RecoveryMatter, bool, error) {
	if double.findErr != nil {
		return domain.RecoveryMatter{}, false, double.findErr
	}
	matter, found := double.current[recoveryKey{tenant: tenant, caseID: caseID, counterparty: counterparty, scope: scope}]
	return matter, found, nil
}

func (double *recoveryStoreDouble) Save(_ context.Context, tenant domain.TenantID, matter domain.RecoveryMatter) (ports.RecoverySaveOutcome, error) {
	if double.saveErr != nil {
		return ports.RecoverySaveOutcomeInvalid, double.saveErr
	}
	key := recoveryKey{tenant: tenant, caseID: matter.Case(), counterparty: matter.Counterparty(), scope: matter.Scope()}
	if _, exists := double.current[key]; exists {
		return ports.RecoveryAlreadyRecorded, nil
	}
	double.byID[matter.ID()] = matter
	double.current[key] = matter
	return ports.RecoverySaved, nil
}

func (double *recoveryStoreDouble) CountActions(
	_ context.Context,
	_ domain.TenantID,
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

func (double *recoveryStoreDouble) AppendAction(_ context.Context, _ domain.TenantID, action domain.RecoveryAction) error {
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
	evidence    *evidenceViewDouble
	recoveries  *recoveryStoreDouble
	identities  *recoveryIdentityDouble
	settlement  *liabilityDownstreamDouble
}

// registeredRules 是每一维都登记齐备、且事实都对得上的那份规则：合同覆盖该类型、
// 首次索赔期限在提交之后、最低材料清单为空（因而恒齐备）、授权目录已登记。各测试
// 只改自己要证的那一维，改动才读得出是在证什么。
func registeredRules(t *testing.T) ports.EligibilityRules {
	t.Helper()
	return ports.EligibilityRules{
		RuleVersion: "claim-eligibility/v1",
		KindCovered: true,
		FilingDeadline: ports.FilingDeadlineRule{
			Registered:  true,
			RuleVersion: "filing-deadline/v1",
			StartEvent:  "DELIVERY_EXCEPTION_CONFIRMED",
			Calendar:    "Asia/Shanghai",
			Scope:       "parcel-1/loss",
			Deadline:    claimSubmittedAt.Add(24 * time.Hour),
		},
		Materials: ports.MinimumMaterialsRule{
			Registered:         true,
			RuleVersion:        "minimum-materials/v1",
			Notice:             mustValue(t, domain.NewSupplementNoticeReference, "notify-policy/v1"),
			SupplementDeadline: claimSubmittedAt.Add(14 * 24 * time.Hour),
		},
		Authorization: ports.AuthorizationCatalogue{
			Registered:  true,
			RuleVersion: "claim-authorization/v1",
			AuthorizedApplicants: []domain.ApplicantReference{
				mustValue(t, domain.NewApplicantReference, "applicant-1"),
			},
		},
	}
}

func newClaimFixture(t *testing.T) *claimFixture {
	t.Helper()
	fixture := &claimFixture{
		claims: newClaimStore(),
		eligibility: &eligibilityRuleDouble{
			rules:    registeredRules(t),
			declared: true,
		},
		evidence:   &evidenceViewDouble{known: true},
		recoveries: newRecoveryStore(),
		identities: &recoveryIdentityDouble{},
		settlement: &liabilityDownstreamDouble{},
	}
	fixture.handler = application.NewHandleClaimHandler(application.HandleClaimDeps{
		Claims:      fixture.claims,
		Eligibility: fixture.eligibility,
		Evidence:    fixture.evidence,
		Recoveries:  fixture.recoveries,
		Identities:  fixture.identities,
		Settlement:  fixture.settlement,
		Clock:       fixedClock{at: claimSubmittedAt.Add(time.Hour)},
	})
	return fixture
}

// seedUnscreenedLegacyClaim 摆出一项申请人维接通（切块 (c)）之前受理的存量索赔：
// 没有申请人、未审。它只能从重建口来——受理口如今必带申请人。
func seedUnscreenedLegacyClaim(t *testing.T, fixture *claimFixture, item string) {
	t.Helper()
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	batch := mustValue(t, domain.NewClaimBatchReference, "claim-batch-1")
	id := mustValue(t, domain.NewClaimItemID, item)
	claim, err := domain.RehydrateClaimItem(domain.ClaimItemSnapshot{
		Revision:    1,
		ID:          id,
		Batch:       batch,
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: claimSubmittedAt,
	})
	if err != nil {
		t.Fatalf("摆出存量索赔：%v", err)
	}
	fixture.claims.claims[claimKey{tenant: tenant, batch: batch, item: id}] = claim
}

// seedEligibleClaim 直接把一项已过审的索赔放进库，供责任结论与复核那几个用例作前置。
//
// 过审如今经 ScreenClaim 也到得了（切块 (c) 接通了申请人授权维），但那几个用例证的
// 是结论与复核，不是资格审核——前置用重建口摆出来，读用例的人不必先追一遍五维核对。
func seedEligibleClaim(t *testing.T, fixture *claimFixture, item string) {
	t.Helper()
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	batch := mustValue(t, domain.NewClaimBatchReference, "claim-batch-1")
	id := mustValue(t, domain.NewClaimItemID, item)
	claim, err := domain.RehydrateClaimItem(domain.ClaimItemSnapshot{
		Revision:    1,
		ID:          id,
		Batch:       batch,
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: claimSubmittedAt,
		Screen:      domain.ClaimEligible,
		ScreenBasis: "claim-eligibility/v1; CONTRACT_SCOPE_COVERS_KIND",
	})
	if err != nil {
		t.Fatalf("摆出已过审索赔：%v", err)
	}
	fixture.claims.claims[claimKey{tenant: tenant, batch: batch, item: id}] = claim
}

func receiveCommand(t *testing.T, item string) application.ReceiveClaimCommand {
	t.Helper()
	return application.ReceiveClaimCommand{
		TenantID:    mustValue(t, domain.NewTenantID, "tenant-1"),
		Batch:       mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:        mustValue(t, domain.NewClaimItemID, item),
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Applicant:   mustValue(t, domain.NewApplicantReference, "applicant-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: claimSubmittedAt,
	}
}

func screenCommand(t *testing.T, item string) application.ScreenClaimCommand {
	t.Helper()
	return application.ScreenClaimCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Batch:    mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:     mustValue(t, domain.NewClaimItemID, item),
	}
}

func concludeCommand(t *testing.T, item string) application.ConcludeClaimCommand {
	t.Helper()
	return application.ConcludeClaimCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Batch:      mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:       mustValue(t, domain.NewClaimItemID, item),
		Conclusion: domain.LiabilityPartiallyEstablished,
		ReviewBy:   claimSubmittedAt.Add(30 * 24 * time.Hour),
	}
}

func openRecoveryCommand(t *testing.T) application.OpenRecoveryCommand {
	t.Helper()
	return application.OpenRecoveryCommand{
		TenantID:     mustValue(t, domain.NewTenantID, "tenant-1"),
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

// Covers: CONTEXT「资格审核 → 等待补充、不予受理或进入责任审核：**结果保存合同、
// 首次索赔期限、授权、重复关系和材料依据**」——五维依据各占一段随结果入账；已落终局
// 的不再审。合同不承担该索赔类型是 ADR-0051 认可的永久格，走它取终局。
func TestAScreenRecordsEveryDimensionsBasisAndTerminalIsScreenedOnce(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	fixture.eligibility.rules.KindCovered = false

	screened, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if screened.Outcome() != application.ClaimScreened {
		t.Fatalf("outcome = %q, want CLAIM_SCREENED", screened.Outcome())
	}
	claim, _ := screened.Claim()
	if screen, ok := claim.Screen(); !ok || screen != domain.ClaimIneligible {
		t.Fatalf("screen = %q ok = %v，want 合同不承担该类型落 INELIGIBLE", screen, ok)
	}
	// 目录查询自带租户（ve-claims-read-seams/01）：多租户读适配器凭这一格按册作答，
	// 编排漏装它，视图就只能报错或答错人的册。
	if got := fixture.eligibility.lastQuery.Tenant; got != screenCommand(t, "item-1").TenantID {
		t.Fatalf("资格查询携带的租户 = %q，编排没把命令租户装进查询", got)
	}

	// 五维缺一段，就说明有一维没核过而结果照样写下了。
	basis := claim.Snapshot().ScreenBasis
	for _, dimension := range []string{
		"CLAIM_KIND_NOT_IN_CONTRACT_SCOPE",
		"FILING_DEADLINE_MET",
		"APPLICANT_AUTHORIZED",
		"DUPLICATE_NONE",
		"MATERIALS_COMPLETE",
	} {
		if !strings.Contains(basis, dimension) {
			t.Fatalf("依据里没有 %s 那一维：%q", dimension, basis)
		}
	}

	again, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("repeat screen: %v", err)
	}
	if again.Outcome() != application.ClaimScreenAlreadyRecorded {
		t.Fatalf("outcome = %q, want SCREEN_ALREADY_RECORDED", again.Outcome())
	}
}

// Covers: 票面第三节要害与 ADR-0051 第五条——**资料不足不得答不予受理**。这一格写下去
// 就永久拒赔且再也审不了，而 CONTEXT 恰恰要求这种情形进限期补充。把 parcel-shipment
// 的 IntakeEligibilityView 写法照搬过来正好落进这里，且编译与测试都不会拦，所以单证
// 一次：材料缺口在场时结果只能是第三态。
func TestMaterialsShortOfTheMinimumNeverLandsOnIneligible(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	fixture.eligibility.rules.Materials.Required = []domain.MaterialRequirementReference{
		mustValue(t, domain.NewMaterialRequirementReference, "photos/damage"),
		mustValue(t, domain.NewMaterialRequirementReference, "invoice/purchase"),
	}
	fixture.evidence.received = []domain.MaterialRequirementReference{
		mustValue(t, domain.NewMaterialRequirementReference, "invoice/purchase"),
	}

	result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if result.Outcome() != application.ClaimAwaitingSupplement {
		t.Fatalf("outcome = %q, want CLAIM_AWAITING_SUPPLEMENT", result.Outcome())
	}
	claim, _ := result.Claim()
	screen, _ := claim.Screen()
	if screen == domain.ClaimIneligible {
		t.Fatal("资料不足被答成不予受理——那是不可逆的默认拒赔")
	}
	if screen != domain.ClaimAwaitingSupplement {
		t.Fatalf("screen = %q, want AWAITING_SUPPLEMENT", screen)
	}

	// 四件落点：缺少材料只列真正缺的那一件，已收到的不再要一遍；补充范围取索赔自己
	// 固定的目标范围；通知依据与截止由规则给。
	requirement, present := claim.Supplement()
	if !present {
		t.Fatal("第三态没有带四件落点")
	}
	if requirement.MissingMaterials.String() != "photos/damage" {
		t.Fatalf("缺少材料 = %q，want 只差 photos/damage", requirement.MissingMaterials.String())
	}
	if requirement.Scope.String() != "parcel-1/loss" {
		t.Fatalf("补充范围 = %q，want 索赔的目标范围", requirement.Scope.String())
	}
	if requirement.Notice.String() != "notify-policy/v1" {
		t.Fatalf("通知依据 = %q", requirement.Notice.String())
	}
	if !requirement.Deadline.Equal(fixture.eligibility.rules.Materials.SupplementDeadline.UTC()) {
		t.Fatalf("当前截止 = %s，want 规则给的补充期限", requirement.Deadline)
	}
	if history := claim.SupplementDeadlineHistory(); len(history) != 1 {
		t.Fatalf("期限历史 = %d 版，want 首版入账", len(history))
	}

	// 可重入：材料到齐后重判不撞`已审过`——那正是第三态存在的理由（ADR-0051 第二条）。
	// 其余四维本就全过，重判因此直达终局`通过`：等待补充 → 通过这条路自此可走。
	fixture.evidence.received = append(fixture.evidence.received,
		mustValue(t, domain.NewMaterialRequirementReference, "photos/damage"))
	rejudged, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("re-screen: %v", err)
	}
	if rejudged.Outcome() != application.ClaimScreened {
		t.Fatalf("outcome = %q, want CLAIM_SCREENED", rejudged.Outcome())
	}
	rescreened, _ := rejudged.Claim()
	if screen, _ := rescreened.Screen(); screen != domain.ClaimEligible {
		t.Fatalf("screen = %q, want ELIGIBLE；材料补齐且其余维全过", screen)
	}
}

// Covers: 逐维核对里每一维「核不了」各停在自己的原因上（CONTEXT 要求资格结果保存
// 五样依据，缺哪一样就该指名去补哪一样）。压成一个笼统的「资格未配置」，看到未决的
// 人就不知道该去登记时限、去补申请人、去裁重复关系还是去接证据归集。
func TestEachUncheckableDimensionStopsOnItsOwnReason(t *testing.T) {
	ctx := context.Background()
	cases := []struct {
		name    string
		arrange func(fixture *claimFixture)
		want    application.HandleClaimUndecidedReason
	}{
		{
			name: "首次索赔期限规则未登记",
			arrange: func(fixture *claimFixture) {
				fixture.eligibility.rules.FilingDeadline = ports.FilingDeadlineRule{}
			},
			want: application.EligibilityFilingDeadlineNotRegistered,
		},
		{
			// 规则登了（版本、起算事件、日历、范围俱在）而截止算不出——起算事实源与业务日历
			// 今天 VE 没有（票 ve-claims-read-seams/03「裁决」）。与「没登」分格：前者去 PC 登
			// 一版规则，后者要等派生能力；看着 NOT_REGISTERED 去补规则的人会发现规则早就在。
			name: "首次索赔期限规则已登记但截止算不出",
			arrange: func(fixture *claimFixture) {
				fixture.eligibility.rules.FilingDeadline.Deadline = time.Time{}
			},
			want: application.EligibilityFilingDeadlineUnderivable,
		},
		{
			name: "授权目录未登记",
			arrange: func(fixture *claimFixture) {
				fixture.eligibility.rules.Authorization = ports.AuthorizationCatalogue{}
			},
			want: application.EligibilityAuthorizationNotRegistered,
		},
		{
			name: "授权目录已登记但存量索赔未带申请人",
			arrange: func(fixture *claimFixture) {
				// 覆写成接通前受理的存量行：申请人缺席没有登记可补，这一维只能如实
				// 停下——按正确申请人重提才是出路。
				seedUnscreenedLegacyClaim(t, fixture, "item-1")
			},
			want: application.EligibilityApplicantNotCarried,
		},
		{
			name: "最低材料清单未登记",
			arrange: func(fixture *claimFixture) {
				fixture.eligibility.rules.Materials = ports.MinimumMaterialsRule{}
			},
			want: application.EligibilityMaterialsNotRegistered,
		},
		{
			name: "证据归集查不到",
			arrange: func(fixture *claimFixture) {
				fixture.evidence.known = false
			},
			want: application.EligibilityEvidenceUnavailable,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newClaimFixture(t)
			if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
				t.Fatalf("receive: %v", err)
			}
			testCase.arrange(fixture)

			result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
			if err != nil {
				t.Fatalf("screen: %v", err)
			}
			if result.Outcome() != application.HandleClaimUndecided {
				t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
			}
			if result.UndecidedReason() != testCase.want {
				t.Fatalf("reason = %q, want %q", result.UndecidedReason(), testCase.want)
			}
			claim, _, _ := fixture.claims.FindByBatchItem(ctx,
				mustValue(t, domain.NewTenantID, "tenant-1"),
				mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
				mustValue(t, domain.NewClaimItemID, "item-1"))
			if _, screened := claim.Screen(); screened {
				t.Fatal("一维核不了却仍在索赔上记下了资格结果")
			}
		})
	}
}

// Covers: 五维全部肯定通过才答`通过`，且这一格自切块 (c) 接通申请人授权维后第一次
// 可达（此前恒有一维核不了，票面第三节把「少核一维就答通过」与默认拒赔并列为同一个
// 错的两面）。终局一次性：通过之后再审答`已审过`（ADR-0051 只让等待补充重入）。
func TestAFullyCheckedClaimScreensEligible(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}

	result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if result.Outcome() != application.ClaimScreened {
		t.Fatalf("outcome = %q, want CLAIM_SCREENED", result.Outcome())
	}
	claim, _ := result.Claim()
	if screen, ok := claim.Screen(); !ok || screen != domain.ClaimEligible {
		t.Fatalf("screen = %q ok = %v, want ELIGIBLE；五维全过", screen, ok)
	}
	basis := claim.Snapshot().ScreenBasis
	for _, dimension := range []string{
		"CONTRACT_SCOPE_COVERS_KIND",
		"FILING_DEADLINE_MET",
		"APPLICANT_AUTHORIZED/claim-authorization/v1",
		"DUPLICATE_NONE",
		"MATERIALS_COMPLETE",
	} {
		if !strings.Contains(basis, dimension) {
			t.Fatalf("通过的依据里没有 %s 那一维：%q", dimension, basis)
		}
	}

	again, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("repeat screen: %v", err)
	}
	if again.Outcome() != application.ClaimScreenAlreadyRecorded {
		t.Fatalf("outcome = %q, want SCREEN_ALREADY_RECORDED；通过是终局格", again.Outcome())
	}
}

// Covers: `AT-VE-125`「客户账户或申请人授权不匹配 → 不受理且不泄露其他客户资料」。
// 「不受理」与 `AT-VE-124` 的「不予受理」是两个词：核出不匹配不落 ADR-0051 的永久格
// （名单换版或换对申请人后照常再审），不折进「核不了」（核对已经作出了答案），也不写
// 任何东西——索赔项一字不动，账户的合同覆盖与期限判断都不交到未获授权的申请人手里。
func TestAnUnauthorizedApplicantIsRefusedWithoutAScreen(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	savesAfterReceipt := fixture.claims.saves
	fixture.eligibility.rules.Authorization.AuthorizedApplicants = []domain.ApplicantReference{
		mustValue(t, domain.NewApplicantReference, "applicant-9"),
	}

	result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if result.Outcome() != application.HandleClaimNotAccepted {
		t.Fatalf("outcome = %q, want NOT_ACCEPTED", result.Outcome())
	}
	if fixture.claims.saves != savesAfterReceipt {
		t.Fatalf("不受理却写了库：saves %d → %d", savesAfterReceipt, fixture.claims.saves)
	}
	claim, _, _ := fixture.claims.FindByBatchItem(ctx,
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		mustValue(t, domain.NewClaimItemID, "item-1"))
	if screen, screened := claim.Screen(); screened {
		t.Fatalf("不受理却把索赔记成了 %q——授权不匹配不是资格结论", screen)
	}

	// 名单换版补上申请人后，同一项索赔照常再审并可达通过——不受理不烧掉这项索赔。
	fixture.eligibility.rules.Authorization.AuthorizedApplicants = append(
		fixture.eligibility.rules.Authorization.AuthorizedApplicants,
		mustValue(t, domain.NewApplicantReference, "applicant-1"))
	rejudged, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("re-screen: %v", err)
	}
	if rejudged.Outcome() != application.ClaimScreened {
		t.Fatalf("outcome = %q, want CLAIM_SCREENED", rejudged.Outcome())
	}
}

// Covers: `AT-VE-124`「首次索赔超过合同期限 → 形成有依据的不予受理，不借补充或复核
// 期限绕过」。它与合同不覆盖并列为 ADR-0051 认可的两个永久格，且**胜过材料缺口**：
// 已经超期的索赔不该再被叫去补材料。
func TestAClaimFiledAfterTheDeadlineIsRefusedRatherThanAskedForMaterials(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	fixture.eligibility.rules.FilingDeadline.Deadline = claimSubmittedAt.Add(-time.Hour)
	fixture.eligibility.rules.Materials.Required = []domain.MaterialRequirementReference{
		mustValue(t, domain.NewMaterialRequirementReference, "photos/damage"),
	}

	result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	claim, _ := result.Claim()
	screen, _ := claim.Screen()
	if screen != domain.ClaimIneligible {
		t.Fatalf("screen = %q, want INELIGIBLE；超首次期限是永久格", screen)
	}
	if !strings.Contains(claim.Snapshot().ScreenBasis, "FILING_DEADLINE_EXCEEDED") {
		t.Fatalf("不予受理没点名超期依据：%q", claim.Snapshot().ScreenBasis)
	}
}

// Covers: 重复关系成立时停在未决而不是不予受理。ADR-0051 只认可两个永久格，重复不在
// 其中——重复成立意味着什么是一次尚未作出的领域裁断，拿一个计数替它拍板就是造出第三个
// 永久拒赔格。没有重复则这一维由事实本身通过。
func TestADuplicateRelationEscalatesInsteadOfDenying(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	// 同客户、同目标范围、同索赔类型的另一项：receiveCommand 的这三样逐项相同。
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-2")); err != nil {
		t.Fatalf("receive duplicate: %v", err)
	}

	result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if result.Outcome() != application.HandleClaimUndecided ||
		result.UndecidedReason() != application.EligibilityDuplicateUnresolved {
		t.Fatalf("result = %q/%q, want UNDECIDED/ELIGIBILITY_DUPLICATE_UNRESOLVED",
			result.Outcome(), result.UndecidedReason())
	}
	claim, _, _ := fixture.claims.FindByBatchItem(ctx,
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		mustValue(t, domain.NewClaimItemID, "item-1"))
	if screen, screened := claim.Screen(); screened {
		t.Fatalf("重复关系把索赔判成了 %q——那一格没有任何裁断支撑", screen)
	}
}

// Covers: `AT-VE-123`「客户撤回后重新提交同一范围 → 建立新索赔项并重新检查……重复
// 关系」——撤回那项不算重复，否则同一范围再也提不了第二次。
func TestAWithdrawnClaimDoesNotCountAsADuplicate(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	batch := mustValue(t, domain.NewClaimBatchReference, "claim-batch-1")

	withdrawn, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	earlier, _ := withdrawn.Claim()
	if err := earlier.Withdraw(claimSubmittedAt.Add(30 * time.Minute)); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-2")); err != nil {
		t.Fatalf("receive resubmission: %v", err)
	}

	count, err := fixture.claims.CountLiveScopeClaims(ctx, tenant,
		mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		mustValue(t, domain.NewRequestScopeReference, "parcel-1/loss"),
		mustValue(t, domain.NewClaimKindReference, "LOSS"),
		mustValue(t, domain.NewClaimItemID, "item-2"))
	if err != nil {
		t.Fatalf("count: %v", err)
	}
	if count != 0 {
		t.Fatalf("撤回那项被数成了重复：count = %d", count)
	}

	result, err := fixture.handler.ScreenClaim(ctx, application.ScreenClaimCommand{
		TenantID: tenant, Batch: batch, Item: mustValue(t, domain.NewClaimItemID, "item-2"),
	})
	if err != nil {
		t.Fatalf("screen resubmission: %v", err)
	}
	if result.UndecidedReason() == application.EligibilityDuplicateUnresolved {
		t.Fatal("撤回后重新提交同一范围被重复关系挡住了")
	}
}

// Covers: 差材料但四件落点凑不齐时停在未决——那时`等待补充`与`资格尚未审核`分不开
// （ADR-0051 第三条），既不能记一个残缺的第三态，也不能借 `ErrInvalidClaim` 上抛成
// 技术故障：客户其实在等一份材料清单，而一次技术错误会让这项索赔从待办里消失。
//
// 缺的两件各有出处：通知依据由规则给，规则没给就补规则；当前截止已经不在未来时按
// CONTEXT「补充期限届满只触发资格复核……不能默认拒赔」保持待决定并升级。
func TestMaterialsShortWithoutTheFourLandingPointsStaysUndecided(t *testing.T) {
	ctx := context.Background()
	short := []domain.MaterialRequirementReference{
		mustValue(t, domain.NewMaterialRequirementReference, "photos/damage"),
	}
	cases := []struct {
		name    string
		arrange func(fixture *claimFixture)
		want    application.HandleClaimUndecidedReason
	}{
		{
			name: "规则没给通知依据",
			arrange: func(fixture *claimFixture) {
				fixture.eligibility.rules.Materials.Notice = domain.SupplementNoticeReference{}
			},
			want: application.EligibilitySupplementIncomplete,
		},
		{
			name: "补充期限已经不在未来",
			arrange: func(fixture *claimFixture) {
				fixture.eligibility.rules.Materials.SupplementDeadline = claimSubmittedAt
			},
			want: application.EligibilitySupplementWindowClosed,
		},
		{
			// 零值截止不是一个过去的时刻：材料清单登了、补充截止算不出（起算事实与日历今天
			// VE 没有，票 ve-claims-read-seams/03「裁决」）。说成「窗口已关」会让人去查一份
			// 从未存在过的期限；这一格要等派生能力，恢复动作与前一格不同。
			name: "补充截止算不出",
			arrange: func(fixture *claimFixture) {
				fixture.eligibility.rules.Materials.SupplementDeadline = time.Time{}
			},
			want: application.EligibilitySupplementDeadlineUnderivable,
		},
	}

	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newClaimFixture(t)
			if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
				t.Fatalf("receive: %v", err)
			}
			fixture.eligibility.rules.Materials.Required = short
			testCase.arrange(fixture)

			result, err := fixture.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
			if err != nil {
				t.Fatalf("screen: %v", err)
			}
			if result.Outcome() != application.HandleClaimUndecided ||
				result.UndecidedReason() != testCase.want {
				t.Fatalf("result = %q/%q, want UNDECIDED/%q",
					result.Outcome(), result.UndecidedReason(), testCase.want)
			}
			claim, _, _ := fixture.claims.FindByBatchItem(ctx,
				mustValue(t, domain.NewTenantID, "tenant-1"),
				mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
				mustValue(t, domain.NewClaimItemID, "item-1"))
			if screen, screened := claim.Screen(); screened {
				t.Fatalf("四件落点凑不齐却把索赔记成了 %q", screen)
			}
		})
	}
}

// Covers: 实例半边红线——合同的索赔资格声明不在场时未决等租户登记：默认受理与默认
// 拒赔都是虚构；与目录调不通分开（恢复动作不同）。
func TestAnUndeclaredEligibilityCatalogueIsUndecidedNotDefaulted(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	if _, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	fixture.eligibility.declared = false

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
		mustValue(t, domain.NewTenantID, "tenant-1"),
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

	seedEligibleClaim(t, fixture, "item-1")
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
	seedEligibleClaim(t, fixture, "item-1")
	if _, err := fixture.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1")); err != nil {
		t.Fatalf("conclude: %v", err)
	}
	intentsBefore := len(fixture.settlement.intents)

	reviewed, err := fixture.handler.ReviewClaim(ctx, application.ReviewClaimCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
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
	seedEligibleClaim(t, fixture, "item-1")
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
		Evidence:    fixture.evidence,
		Recoveries:  fixture.recoveries,
		Identities:  fixture.identities,
		Settlement:  fixture.settlement,
		Clock:       fixedClock{at: claimSubmittedAt.Add(2 * time.Hour)},
	})
	result, err := late.handler.ReviewClaim(ctx, application.ReviewClaimCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
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
			TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
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

// Covers: 三判各步撞上修订冲突时交`并发改动`，不折成未决。未决是「等依赖」，恢复动作
// 是原样重试同一份；冲突要的是重读当前那一版再重判——压成一格，调用方的恢复动作就指
// 错了，而它手里那份判断是在一份已经过期的快照上作出的。
//
// 结论与复核还多一条：写入没落库就不得交结算意图，否则下游会按一份不存在的结论算钱。
func TestAConcurrentChangeIsItsOwnAnswerNotUndecided(t *testing.T) {
	ctx := context.Background()

	screen := newClaimFixture(t)
	if _, err := screen.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1")); err != nil {
		t.Fatalf("receive: %v", err)
	}
	// 合同不承担该类型这一维走得到终局，资格审核才有东西要落库。
	screen.eligibility.rules.KindCovered = false
	screen.claims.saveConflicts = 1
	screened, err := screen.handler.ScreenClaim(ctx, screenCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("screen: %v", err)
	}
	if screened.Outcome() != application.ClaimConcurrentlyChanged {
		t.Fatalf("资格审核撞冲突后 outcome = %q，want CLAIM_CONCURRENTLY_CHANGED", screened.Outcome())
	}

	conclude := newClaimFixture(t)
	seedEligibleClaim(t, conclude, "item-1")
	conclude.claims.saveConflicts = 1
	concluded, err := conclude.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if concluded.Outcome() != application.ClaimConcurrentlyChanged {
		t.Fatalf("责任结论撞冲突后 outcome = %q，want CLAIM_CONCURRENTLY_CHANGED", concluded.Outcome())
	}
	if len(conclude.settlement.intents) != 0 {
		t.Fatalf("结论没落库却交了 %d 份结算意图", len(conclude.settlement.intents))
	}

	review := newClaimFixture(t)
	seedEligibleClaim(t, review, "item-1")
	if _, err := review.handler.ConcludeClaim(ctx, concludeCommand(t, "item-1")); err != nil {
		t.Fatalf("conclude: %v", err)
	}
	concludedHandoffs := len(review.settlement.intents)
	review.claims.saveConflicts = 1
	reviewed, err := review.handler.ReviewClaim(ctx, application.ReviewClaimCommand{
		TenantID:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Batch:      mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Item:       mustValue(t, domain.NewClaimItemID, "item-1"),
		Conclusion: domain.LiabilityFullyEstablished,
	})
	if err != nil {
		t.Fatalf("review: %v", err)
	}
	if reviewed.Outcome() != application.ClaimConcurrentlyChanged {
		t.Fatalf("复核撞冲突后 outcome = %q，want CLAIM_CONCURRENTLY_CHANGED", reviewed.Outcome())
	}
	if len(review.settlement.intents) != concludedHandoffs {
		t.Fatalf("复核没落库却又交了结算意图：%d → %d", concludedHandoffs, len(review.settlement.intents))
	}
}

// Covers: 受理这一步的冲突只有一个来源——取回之后、写入之前另一方把同（批次+项）建了
// 出来。幂等按（批次+项），因此答案与「取回时就已存在」同格：读回赢家如实交出，不新
// 开一格让调用方以为发生了别的事。
func TestAReceiptLosingTheCreateRaceAnswersWithTheWinner(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()

	winner, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:          mustValue(t, domain.NewClaimItemID, "item-1"),
		Batch:       mustValue(t, domain.NewClaimBatchReference, "claim-batch-1"),
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Applicant:   mustValue(t, domain.NewApplicantReference, "applicant-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-scope/v1"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-9/loss"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "LOSS"),
		SubmittedAt: claimSubmittedAt,
	})
	if err != nil {
		t.Fatalf("构造赢家索赔：%v", err)
	}
	fixture.claims.saveConflicts = 1
	fixture.claims.conflictWinner = winner

	result, err := fixture.handler.ReceiveClaim(ctx, receiveCommand(t, "item-1"))
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if result.Outcome() != application.ClaimExistingResult {
		t.Fatalf("受理输掉建行竞争后 outcome = %q，want CLAIM_EXISTING_RESULT", result.Outcome())
	}
	claim, ok := result.Claim()
	if !ok {
		t.Fatal("受理输掉建行竞争后没有交出任何索赔")
	}
	if claim.Target().String() != "parcel-9/loss" {
		t.Fatalf("交出的是本方那份而不是赢家：target = %q", claim.Target().String())
	}
}

// Covers: ADR-0043「首次交付失败不改写业务结果……另留一条发布续办引用」——结算意图
// 交不出去时结论保持已成立，只留可续办引用。
func TestAFailedSettlementHandoffKeepsTheConclusionWithAResumableReference(t *testing.T) {
	fixture := newClaimFixture(t)
	ctx := context.Background()
	seedEligibleClaim(t, fixture, "item-1")
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
