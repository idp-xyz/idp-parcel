package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var continuedAttemptAt = time.Date(2026, 9, 2, 10, 0, 0, 0, time.UTC)

func emptyRegister(t *testing.T) domain.ContinuedAttemptRegister {
	t.Helper()

	register, err := domain.OpenContinuedAttemptRegister(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
	)
	if err != nil {
		t.Fatalf("开册：%v", err)
	}
	return register
}

func closureSpec(t *testing.T, id string, effectiveAt time.Time) domain.ContinuedAttemptDecisionSpec {
	t.Helper()

	return domain.ContinuedAttemptDecisionSpec{
		ID:                mustValue(t, domain.NewContinuedAttemptDecisionID, id),
		Kind:              domain.ControlledClosureDecision,
		Decider:           mustValue(t, domain.NewDeciderReference, "OPS-MANAGER-1"),
		AuthorityRole:     mustValue(t, domain.NewContinuedAttemptAuthorityRoleReference, "PC-CLOSURE-ROLE-1"),
		AuthoritySnapshot: mustValue(t, domain.NewContinuedAttemptAuthoritySnapshot, "PC-AUTH-SNAPSHOT-1"),
		Reason:            mustValue(t, domain.NewContinuedAttemptReasonReference, "CHANNEL_SUSPENDED"),
		EffectiveAt:       effectiveAt,
		CutoffBoundary:    mustValue(t, domain.NewAuthoritativeCutoffBoundary, "PS-CUTOFF-1"),
	}
}

func reopeningSpec(t *testing.T, id, priorClosure string, effectiveAt time.Time) domain.ContinuedAttemptDecisionSpec {
	t.Helper()

	return domain.ContinuedAttemptDecisionSpec{
		ID:                  mustValue(t, domain.NewContinuedAttemptDecisionID, id),
		Kind:                domain.ReopeningDecision,
		Decider:             mustValue(t, domain.NewDeciderReference, "OPS-DIRECTOR-1"),
		AuthorityRole:       mustValue(t, domain.NewContinuedAttemptAuthorityRoleReference, "PC-REOPEN-ROLE-1"),
		AuthoritySnapshot:   mustValue(t, domain.NewContinuedAttemptAuthoritySnapshot, "PC-AUTH-SNAPSHOT-2"),
		Reason:              mustValue(t, domain.NewContinuedAttemptReasonReference, "RESTRICTION_LIFTED"),
		EffectiveAt:         effectiveAt,
		RelatedPriorClosure: mustValue(t, domain.NewContinuedAttemptDecisionID, priorClosure),
	}
}

// 空册派生`开放`，而这一格与「关过又重开了」派生出的是同一格。两者在页面上要分得开，靠的是
// HasAnyDecision 而不是给判断加第三格——加一格就是新造领域语言。
func TestAnEmptyRegisterIsOpenAndSaysSoWithoutInventingAThirdGrade(t *testing.T) {
	t.Parallel()

	register := emptyRegister(t)
	if got := register.Judge(false); got != domain.ContinuedAttemptOpen {
		t.Errorf("空册无终局应派生开放，实得 %q", got)
	}
	if register.HasAnyDecision() {
		t.Error("空册不该报告有决定历史——「没有人作过决定」正是靠这一句说出口的")
	}

	closed, err := register.Append(closureSpec(t, "decision-1", continuedAttemptAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	reopened, err := closed.Append(reopeningSpec(t, "decision-2", "decision-1", continuedAttemptAt.Add(time.Hour)), false)
	if err != nil {
		t.Fatalf("追加重开：%v", err)
	}

	if got := reopened.Judge(false); got != domain.ContinuedAttemptOpen {
		t.Errorf("重开之后应派生开放，实得 %q", got)
	}
	if !reopened.HasAnyDecision() {
		t.Error("关过又重开的一册应报告有决定历史——它与空册同格但现场处置不同")
	}
}

func TestAStandingClosureDerivesControlledClosed(t *testing.T) {
	t.Parallel()

	closed, err := emptyRegister(t).Append(closureSpec(t, "decision-1", continuedAttemptAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	if got := closed.Judge(false); got != domain.ContinuedAttemptControlledClosed {
		t.Errorf("生效关闭在场应派生受控关闭，实得 %q", got)
	}
}

// 终局在场时不派生开放：那一格的含义是「允许申请新的重试、替代或换单」，而终局已经关掉了
// 这件事。这一条与决定历史无关，空册也一样。
func TestACurrentFinalOutcomeKeepsTheJudgmentFromBeingOpen(t *testing.T) {
	t.Parallel()

	if got := emptyRegister(t).Judge(true); got == domain.ContinuedAttemptOpen {
		t.Error("当前有效终局在场时不该派生开放")
	}
}

// CONTEXT：「只有当前不存在有效终局服务结果时，才能依据适用授权追加重开决定」。
func TestNoReopeningWhileACurrentFinalOutcomeStands(t *testing.T) {
	t.Parallel()

	closed, err := emptyRegister(t).Append(closureSpec(t, "decision-1", continuedAttemptAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	_, err = closed.Append(reopeningSpec(t, "decision-2", "decision-1", continuedAttemptAt.Add(time.Hour)), true)
	if !errors.Is(err, domain.ErrContinuedAttemptDecisionNotAdmitted) {
		t.Errorf("终局在场时重开应不成立，实得 %v", err)
	}
}

func TestAReopeningMustNameAClosureThatIsStillStanding(t *testing.T) {
	t.Parallel()

	// 没关过就重开：说不出它在重开什么。
	_, err := emptyRegister(t).Append(reopeningSpec(t, "decision-1", "decision-0", continuedAttemptAt), false)
	if !errors.Is(err, domain.ErrContinuedAttemptDecisionNotAdmitted) {
		t.Errorf("无关闭时重开应不成立，实得 %v", err)
	}

	closed, err := emptyRegister(t).Append(closureSpec(t, "decision-1", continuedAttemptAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	reopened, err := closed.Append(reopeningSpec(t, "decision-2", "decision-1", continuedAttemptAt.Add(time.Hour)), false)
	if err != nil {
		t.Fatalf("追加重开：%v", err)
	}
	// 同一份关闭被解两次：第二次指向的那份已经不再生效。
	_, err = reopened.Append(reopeningSpec(t, "decision-3", "decision-1", continuedAttemptAt.Add(2*time.Hour)), false)
	if !errors.Is(err, domain.ErrContinuedAttemptDecisionNotAdmitted) {
		t.Errorf("重复解同一份关闭应不成立，实得 %v", err)
	}
}

// 「只面向未来生效」：与所解的那份关闭同刻或更早生效的重开，会让「边界后的新尝试被拒绝」
// 这条在时间上自相矛盾。
func TestAReopeningOnlyTakesEffectAfterTheClosureItLifts(t *testing.T) {
	t.Parallel()

	closed, err := emptyRegister(t).Append(closureSpec(t, "decision-1", continuedAttemptAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	for name, at := range map[string]time.Time{
		"同刻": continuedAttemptAt,
		"更早": continuedAttemptAt.Add(-time.Hour),
	} {
		if _, err := closed.Append(reopeningSpec(t, "decision-2", "decision-1", at), false); !errors.Is(err, domain.ErrInvalidContinuedAttemptDecision) {
			t.Errorf("%s生效的重开应被拒，实得 %v", name, err)
		}
	}
}

// 截断边界是关闭独有的必备项。重开不带它——重开「只允许未来形成新交易」，不裁决任何并发
// 尝试的合法性；给它一个边界会让读的人以为重开也在裁决前后关系。
func TestTheCutoffBoundaryBelongsToClosureAlone(t *testing.T) {
	t.Parallel()

	withoutBoundary := closureSpec(t, "decision-1", continuedAttemptAt)
	withoutBoundary.CutoffBoundary = domain.AuthoritativeCutoffBoundary{}
	if _, err := emptyRegister(t).Append(withoutBoundary, false); !errors.Is(err, domain.ErrInvalidContinuedAttemptDecision) {
		t.Errorf("关闭没有截断边界应被拒，实得 %v", err)
	}

	closed, err := emptyRegister(t).Append(closureSpec(t, "decision-1", continuedAttemptAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	withBoundary := reopeningSpec(t, "decision-2", "decision-1", continuedAttemptAt.Add(time.Hour))
	withBoundary.CutoffBoundary = mustValue(t, domain.NewAuthoritativeCutoffBoundary, "PS-CUTOFF-2")
	if _, err := closed.Append(withBoundary, false); !errors.Is(err, domain.ErrInvalidContinuedAttemptDecision) {
		t.Errorf("重开带截断边界应被拒，实得 %v", err)
	}
}

// 请求方可缺（CONTEXT 原文「请求方（如有）」——运营企业自行发起的关闭没有外部请求方），
// 实际决定方与授权角色不可缺：「登录操作人可以作为操作证据，但不能替代实际决定方和授权角色」。
func TestTheRequesterMayBeAbsentButTheDeciderAndAuthorityMayNot(t *testing.T) {
	t.Parallel()

	withoutRequester := closureSpec(t, "decision-1", continuedAttemptAt)
	if _, err := emptyRegister(t).Append(withoutRequester, false); err != nil {
		t.Fatalf("无请求方的关闭应成立：%v", err)
	}

	for name, mutate := range map[string]func(domain.ContinuedAttemptDecisionSpec) domain.ContinuedAttemptDecisionSpec{
		"实际决定方": func(s domain.ContinuedAttemptDecisionSpec) domain.ContinuedAttemptDecisionSpec {
			s.Decider = domain.DeciderReference{}
			return s
		},
		"授权角色": func(s domain.ContinuedAttemptDecisionSpec) domain.ContinuedAttemptDecisionSpec {
			s.AuthorityRole = domain.ContinuedAttemptAuthorityRoleReference{}
			return s
		},
		"授权依据快照": func(s domain.ContinuedAttemptDecisionSpec) domain.ContinuedAttemptDecisionSpec {
			s.AuthoritySnapshot = domain.ContinuedAttemptAuthoritySnapshot{}
			return s
		},
		"原因": func(s domain.ContinuedAttemptDecisionSpec) domain.ContinuedAttemptDecisionSpec {
			s.Reason = domain.ContinuedAttemptReasonReference{}
			return s
		},
	} {
		if _, err := emptyRegister(t).Append(mutate(closureSpec(t, "decision-1", continuedAttemptAt)), false); !errors.Is(err, domain.ErrInvalidContinuedAttemptDecision) {
			t.Errorf("缺%s应被拒，实得 %v", name, err)
		}
	}
}

// 追加式不可覆盖：更正走版本链，原条留着。值语义同本包其余追加清单——在派生值上追加不得
// 回头改动原值。
func TestDecisionsAreAppendedAndTheEarlierOnesStay(t *testing.T) {
	t.Parallel()

	closed, err := emptyRegister(t).Append(closureSpec(t, "decision-1", continuedAttemptAt), false)
	if err != nil {
		t.Fatalf("追加关闭：%v", err)
	}
	reopened, err := closed.Append(reopeningSpec(t, "decision-2", "decision-1", continuedAttemptAt.Add(time.Hour)), false)
	if err != nil {
		t.Fatalf("追加重开：%v", err)
	}

	if got := len(reopened.Decisions()); got != 2 {
		t.Fatalf("应有两条决定，实得 %d 条", got)
	}
	if got := len(closed.Decisions()); got != 1 {
		t.Errorf("原册值不该被后一次追加改动，实得 %d 条", got)
	}
	if reopened.Decisions()[0].Kind() != domain.ControlledClosureDecision {
		t.Error("顺序应即追加顺序——它是「最近适用决定是哪一条」的全部依据")
	}

	duplicate := closureSpec(t, "decision-1", continuedAttemptAt.Add(2*time.Hour))
	if _, err := reopened.Append(duplicate, false); !errors.Is(err, domain.ErrInvalidContinuedAttemptDecision) {
		t.Errorf("同一决定身份重复追加应被拒，实得 %v", err)
	}
}

// 重建门以结果自证一致：关闭 → 重开 → 其后才形成终局，这行历史完全合法，用当前终局去卡它
// 会让它读不回来。
func TestRehydrationAcceptsAHistoryThatEndedUnderAFinalOutcome(t *testing.T) {
	t.Parallel()

	spec := domain.RehydrateContinuedAttemptRegisterSpec{
		Revision: 1,
		Tenant:   mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:   mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Decisions: []domain.ContinuedAttemptDecisionSpec{
			closureSpec(t, "decision-1", continuedAttemptAt),
			reopeningSpec(t, "decision-2", "decision-1", continuedAttemptAt.Add(time.Hour)),
		},
	}
	register, err := domain.RehydrateContinuedAttemptRegister(spec)
	if err != nil {
		t.Fatalf("重建登记册：%v", err)
	}
	if got := len(register.Decisions()); got != 2 {
		t.Fatalf("应读回两条决定，实得 %d 条", got)
	}
	if register.Revision() != 1 {
		t.Errorf("版本应读回 1，实得 %d", register.Revision())
	}
	// 读回之后终局才现取：同一册在两种终局状态下派生不同的判断，而决定历史一字未变。
	if got := register.Judge(false); got != domain.ContinuedAttemptOpen {
		t.Errorf("无终局时应派生开放，实得 %q", got)
	}
	if got := register.Judge(true); got != domain.ContinuedAttemptControlledClosed {
		t.Errorf("终局在场时不该派生开放，实得 %q", got)
	}
}
