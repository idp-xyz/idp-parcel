package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var claimSubmittedAt = time.Date(2026, 8, 12, 9, 0, 0, 0, time.UTC)

func receivedClaim(t *testing.T) *domain.ClaimItem {
	t.Helper()
	claim, err := domain.ReceiveClaimItem(domain.ClaimItemSpec{
		ID:          mustValue(t, domain.NewClaimItemID, "claim-1"),
		Batch:       mustValue(t, domain.NewClaimBatchReference, "batch-1"),
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Applicant:   mustValue(t, domain.NewApplicantReference, "applicant-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-1/liability"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-1"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "DAMAGE"),
		SubmittedAt: claimSubmittedAt,
	})
	if err != nil {
		t.Fatalf("receive claim item: %v", err)
	}
	return claim
}

// Covers: CONTEXT 硬句 186「按申请人授权、客户账户……判断资格」与 `AT-VE-125`（申请
// 人授权与客户账户并列，两者不是一回事）——申请人是原始提交事实的一格，受理必带；
// 存量行没有这一格：重建门容缺，Applicant() 如实报缺席，授权维据此答核不了而不是
// 拿客户账户顶替。
func TestAClaimCarriesItsApplicantAndLegacyRowsRebuildWithoutOne(t *testing.T) {
	claim := receivedClaim(t)
	applicant, carried := claim.Applicant()
	if !carried || applicant.String() != "applicant-1" {
		t.Fatalf("applicant = %s carried = %v", applicant, carried)
	}

	missing := domain.ClaimItemSpec{
		ID:          mustValue(t, domain.NewClaimItemID, "claim-2"),
		Batch:       mustValue(t, domain.NewClaimBatchReference, "batch-1"),
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-1/liability"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-2"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "DAMAGE"),
		SubmittedAt: claimSubmittedAt,
	}
	if _, err := domain.ReceiveClaimItem(missing); !errors.Is(err, domain.ErrInvalidClaim) {
		t.Fatalf("err = %v; 不带申请人的提交被受理了", err)
	}

	legacy := domain.ClaimItemSnapshot{
		Revision:    1,
		ID:          mustValue(t, domain.NewClaimItemID, "claim-legacy"),
		Batch:       mustValue(t, domain.NewClaimBatchReference, "batch-1"),
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-1/liability"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-3"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "DAMAGE"),
		SubmittedAt: claimSubmittedAt,
	}
	rebuilt, err := domain.RehydrateClaimItem(legacy)
	if err != nil {
		t.Fatalf("重建存量行：%v", err)
	}
	if _, carried := rebuilt.Applicant(); carried {
		t.Fatal("存量行凭空长出了申请人")
	}

	legacy.Applicant = mustValue(t, domain.NewApplicantReference, "applicant-9")
	withApplicant, err := domain.RehydrateClaimItem(legacy)
	if err != nil {
		t.Fatalf("重建带申请人的行：%v", err)
	}
	if got, carried := withApplicant.Applicant(); !carried || got.String() != "applicant-9" {
		t.Fatalf("applicant = %s carried = %v; 快照往返丢了申请人", got, carried)
	}
}

// Covers: VE CONTEXT 硬句 173「收到客户索赔、通过资格审核和确认赔偿责任是不同判断。
// 系统必须先保留原始提交事实，再按……判断资格」——受理只留提交事实（无资格无结论）；
// 资格审核带依据且不审第二次；资格未通过形不成责任结论；类型上没有赔付金额字段。
func TestReceiptScreeningAndLiabilityAreThreeJudgments(t *testing.T) {
	claim := receivedClaim(t)
	if _, screened := claim.Screen(); screened {
		t.Fatal("刚受理的索赔凭空有了资格结论")
	}

	if err := claim.ConcludeLiability(domain.LiabilityFullyEstablished,
		claimSubmittedAt.Add(30*24*time.Hour), claimSubmittedAt.Add(time.Hour)); !errors.Is(err, domain.ErrClaimNotScreened) {
		t.Fatalf("err = %v; 资格未审就下了责任结论", err)
	}

	if err := claim.ScreenEligibility(domain.ClaimEligible, "authorized; within window; materials complete", claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("screen eligibility: %v", err)
	}
	if err := claim.ScreenEligibility(domain.ClaimIneligible, "again", claimSubmittedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrClaimAlreadyScreened) {
		t.Fatalf("err = %v; 资格审了两次", err)
	}

	if err := claim.ConcludeLiability(domain.LiabilityPartiallyEstablished,
		claimSubmittedAt.Add(30*24*time.Hour), claimSubmittedAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("conclude liability: %v", err)
	}
	conclusion, concluded := claim.Conclusion()
	if !concluded || conclusion != domain.LiabilityPartiallyEstablished {
		t.Fatalf("conclusion = %q concluded = %v", conclusion, concluded)
	}

	ineligible := receivedClaim(t)
	if err := ineligible.ScreenEligibility(domain.ClaimIneligible, "outside claim window", claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("screen ineligible: %v", err)
	}
	if err := ineligible.ConcludeLiability(domain.LiabilityNotEstablished,
		claimSubmittedAt.Add(30*24*time.Hour), claimSubmittedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrClaimNotScreened) {
		t.Fatalf("err = %v; 资格未通过形成了责任结论", err)
	}
}

// Covers: VE CONTEXT 生命周期 251「最终责任结论前收到有效撤回——索赔项以已撤回结束
// 后续审核，保留提交和证据；撤回不取消异常案件或独立追偿事项」——结论前可撤（提交
// 事实保留）、撤后审核与结论都拒；已有结论撤不回；类型上没有案件/追偿字段。
func TestWithdrawalEndsReviewButNotTheCase(t *testing.T) {
	claim := receivedClaim(t)
	if err := claim.Withdraw(claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	if !claim.Withdrawn() {
		t.Fatal("撤回没落上")
	}
	if claim.ID().String() != "claim-1" {
		t.Fatal("提交事实没保留")
	}
	if err := claim.ScreenEligibility(domain.ClaimEligible, "basis", claimSubmittedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrClaimWithdrawn) {
		t.Fatalf("err = %v; 已撤回的索赔还在审", err)
	}

	concluded := receivedClaim(t)
	if err := concluded.ScreenEligibility(domain.ClaimEligible, "ok", claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("screen: %v", err)
	}
	if err := concluded.ConcludeLiability(domain.LiabilityFullyEstablished,
		claimSubmittedAt.Add(30*24*time.Hour), claimSubmittedAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("conclude: %v", err)
	}
	if err := concluded.Withdraw(claimSubmittedAt.Add(3 * time.Hour)); !errors.Is(err, domain.ErrClaimAlreadyConcluded) {
		t.Fatalf("err = %v; 已有结论的索赔被撤回了", err)
	}
}

// Covers: VE CONTEXT 生命周期 253/254「复核期限内出现有效异议或关键新证据——形成
// 受控复核和新的结论版本；原结论保留」「复核期限届满……后续复核请求形成有依据的不
// 受理，不改变原责任结论」——期限内换结论原结论进 PriorConclusion；届满独立哨兵拒；
// 同值复核是矛盾输入。
func TestReviewIsControlledByItsWindow(t *testing.T) {
	claim := receivedClaim(t)
	if err := claim.ScreenEligibility(domain.ClaimEligible, "ok", claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("screen: %v", err)
	}
	reviewBy := claimSubmittedAt.Add(30 * 24 * time.Hour)
	if err := claim.ConcludeLiability(domain.LiabilityNotEstablished, reviewBy, claimSubmittedAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("conclude: %v", err)
	}

	if err := claim.ReviewConclusion(domain.LiabilityPartiallyEstablished, reviewBy.Add(-time.Hour)); err != nil {
		t.Fatalf("review: %v", err)
	}
	prior, reviewed := claim.PriorConclusion()
	if !reviewed || prior != domain.LiabilityNotEstablished {
		t.Fatalf("prior = %q reviewed = %v; 原结论必须保留", prior, reviewed)
	}
	current, _ := claim.Conclusion()
	if current != domain.LiabilityPartiallyEstablished {
		t.Fatalf("current = %q", current)
	}

	if err := claim.ReviewConclusion(domain.LiabilityFullyEstablished, reviewBy.Add(time.Hour)); !errors.Is(err, domain.ErrReviewWindowClosed) {
		t.Fatalf("err = %v; 届满后复核还改了结论", err)
	}
	if err := claim.ReviewConclusion(domain.LiabilityPartiallyEstablished, reviewBy.Add(-time.Minute)); !errors.Is(err, domain.ErrInvalidClaim) {
		t.Fatalf("err = %v; 同值复核分不出新旧", err)
	}
}

func supplementRequirement(t *testing.T, deadline time.Time) domain.SupplementRequirement {
	t.Helper()
	requirement, err := domain.NewSupplementRequirement(
		mustValue(t, domain.NewMissingMaterialsReference, "photos/damage"),
		mustValue(t, domain.NewSupplementScopeReference, "parcel-1/DAMAGE"),
		mustValue(t, domain.NewSupplementNoticeReference, "notify-policy/v1"),
		deadline,
	)
	if err != nil {
		t.Fatalf("supplement requirement: %v", err)
	}
	return requirement
}

// Covers: CONTEXT 生命周期「资格审核 → 等待补充、不予受理或进入责任审核」与硬句
// 「资料不足时，索赔项进入限期补充，并固定缺少材料、补充范围、通知依据和当前截止
// 时间」——等待补充可重判到终局；终局格不可再走补充（ADR-0051）。
func TestAwaitingSupplementCanBeRejudgedUntilATerminalScreen(t *testing.T) {
	claim := receivedClaim(t)
	deadline := claimSubmittedAt.Add(7 * 24 * time.Hour)
	if err := claim.AwaitSupplement("materials incomplete", supplementRequirement(t, deadline), claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("await supplement: %v", err)
	}
	screen, ok := claim.Screen()
	if !ok || screen != domain.ClaimAwaitingSupplement {
		t.Fatalf("screen = %q ok = %v", screen, ok)
	}
	requirement, present := claim.Supplement()
	if !present || requirement.MissingMaterials.String() != "photos/damage" || !requirement.Deadline.Equal(deadline.UTC()) {
		t.Fatalf("四件落点没固定：present=%v deadline=%s", present, requirement.Deadline)
	}
	if err := claim.ConcludeLiability(domain.LiabilityFullyEstablished,
		claimSubmittedAt.Add(30*24*time.Hour), claimSubmittedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrClaimNotScreened) {
		t.Fatalf("err = %v; 等待补充形成了责任结论", err)
	}

	if err := claim.ScreenEligibility(domain.ClaimEligible, "materials now complete", claimSubmittedAt.Add(3*time.Hour)); err != nil {
		t.Fatalf("rejudge to eligible: %v", err)
	}
	if _, stillAwaiting := claim.Supplement(); stillAwaiting {
		t.Fatal("终局后还带着等待补充的四件落点")
	}
	if err := claim.AwaitSupplement("again", supplementRequirement(t, deadline), claimSubmittedAt.Add(4*time.Hour)); !errors.Is(err, domain.ErrClaimAlreadyScreened) {
		t.Fatalf("err = %v; 通过后经补充翻案", err)
	}

	ineligible := receivedClaim(t)
	if err := ineligible.ScreenEligibility(domain.ClaimIneligible, "kind not in contract", claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("screen ineligible: %v", err)
	}
	if err := ineligible.AwaitSupplement("try supplement", supplementRequirement(t, deadline), claimSubmittedAt.Add(2*time.Hour)); !errors.Is(err, domain.ErrClaimAlreadyScreened) {
		t.Fatalf("err = %v; 永久不予受理经补充翻案", err)
	}
}

// Covers: CONTEXT「获批延期形成新补充期限版本，原期限保留」——新截止入列，旧截止
// 仍在历史上。
func TestApprovedExtensionKeepsTheOriginalSupplementDeadline(t *testing.T) {
	claim := receivedClaim(t)
	first := claimSubmittedAt.Add(7 * 24 * time.Hour)
	second := claimSubmittedAt.Add(14 * 24 * time.Hour)
	if err := claim.AwaitSupplement("materials incomplete", supplementRequirement(t, first), claimSubmittedAt.Add(time.Hour)); err != nil {
		t.Fatalf("await: %v", err)
	}
	if err := claim.ExtendSupplementDeadline(second, claimSubmittedAt.Add(2*time.Hour)); err != nil {
		t.Fatalf("extend: %v", err)
	}
	history := claim.SupplementDeadlineHistory()
	if len(history) != 2 || !history[0].Deadline.Equal(first.UTC()) || !history[1].Deadline.Equal(second.UTC()) {
		t.Fatalf("期限历史 = %#v", history)
	}
	requirement, _ := claim.Supplement()
	if !requirement.Deadline.Equal(second.UTC()) {
		t.Fatalf("当前截止 = %s", requirement.Deadline)
	}
	if err := claim.AwaitSupplement("still incomplete", supplementRequirement(t, first), claimSubmittedAt.Add(3*time.Hour)); !errors.Is(err, domain.ErrInvalidClaim) {
		t.Fatalf("err = %v; 换截止应走延期而不是重新判断", err)
	}
}

// Covers: 重建的来源只有已落库的行，而落库的行必有首版修订。零修订的快照不是从库里
// 折出来的——放它过去，那份索赔会带着零去作条件更新，而零谁都对不上，于是整行重写
// 又一次谁都拦不住：丢更新正是从这里回来的。
func TestRehydrationRefusesASnapshotThatWasNeverPersisted(t *testing.T) {
	snapshot := receivedClaim(t).Snapshot()
	if snapshot.Revision != 0 {
		t.Fatalf("受理出来的索赔修订 = %d，应为零（尚未落库）", snapshot.Revision)
	}
	if _, err := domain.RehydrateClaimItem(snapshot); !errors.Is(err, domain.ErrInvalidClaim) {
		t.Fatalf("err = %v；零修订的快照不该重建得出来", err)
	}

	snapshot.Revision = 1
	rehydrated, err := domain.RehydrateClaimItem(snapshot)
	if err != nil {
		t.Fatalf("首版修订的快照重建失败：%v", err)
	}
	if rehydrated.Revision() != 1 {
		t.Fatalf("修订没随快照往返：%d", rehydrated.Revision())
	}
}
