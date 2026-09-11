package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

var targetFormedAt = time.Date(2026, 8, 12, 17, 0, 0, 0, time.UTC)

func followUpTarget(t *testing.T, kind domain.FollowUpActionKind) domain.FollowUpTarget {
	t.Helper()
	target, err := domain.FormFollowUpTarget(domain.FollowUpTargetSpec{
		Kind:     kind,
		Trigger:  mustValue(t, domain.NewFollowUpTriggerReference, "regulatory-request/RR-9"),
		CaseRef:  mustValue(t, domain.NewCustomsCaseID, "case-1"),
		Unit:     mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		Version:  mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Scope:    mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		FormedAt: targetFormedAt,
	})
	if err != nil {
		t.Fatalf("form follow-up target (%s): %v", kind, err)
	}
	return target
}

// Covers: CC CONTEXT「后续申报动作目标……必须关联触发依据、原案件、原申报单元、原
// 提交版本、明确范围和拟提交动作；目标形成不等于资料已准备、已经提交或监管结果已经
// 成立」——六件缺一立不起；四道封闭分立（技术再次尝试不在此列，它走受控重发）；类型
// 上没有资料/提交/结果字段。
func TestAFollowUpTargetDemandsItsSixAnchors(t *testing.T) {
	for _, kind := range []domain.FollowUpActionKind{
		domain.InCaseSupplement,
		domain.InCaseCorrection,
		domain.WithdrawalAction,
		domain.ResubmissionReplacement,
	} {
		target := followUpTarget(t, kind)
		if target.Kind() != kind {
			t.Fatalf("kind = %q", target.Kind())
		}
	}

	missingTrigger := domain.FollowUpTargetSpec{
		Kind:     domain.InCaseCorrection,
		CaseRef:  mustValue(t, domain.NewCustomsCaseID, "case-1"),
		Unit:     mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1"),
		Version:  mustValue(t, domain.NewSubmissionVersionID, "submission-1/v1"),
		Scope:    mustValue(t, domain.NewDecisionScopeReference, "declaration-unit-1"),
		FormedAt: targetFormedAt,
	}
	if _, err := domain.FormFollowUpTarget(missingTrigger); !errors.Is(err, domain.ErrInvalidFollowUpTarget) {
		t.Fatalf("err = %v; 没有触发依据的目标被收下了", err)
	}
}

// Covers: CC CONTEXT「原申报单元不能继续使用……建立替代申报单元」「拟
// 替代目标……不得把原申报改成已撤销、已作废或已被有效替代；有效替代必须依据真实程序
// 要求的权威外部结果形成」与生命周期「重报替代目标已形成 → 建立新的逻辑申报目标及拟替代关系」
// ——拟替代只来自重报目标且替代单元必须是新
// 身份；生效必须带外部结果（内部决定与技术成功换不来）；已生效不再生效；原对象全程
// 只有引用（删无可删、改无可改）。
func TestReplacementTakesEffectOnlyByExternalResults(t *testing.T) {
	replacement := followUpTarget(t, domain.ResubmissionReplacement)

	proposed, err := domain.ProposeReplacement(replacement,
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-2"))
	if err != nil {
		t.Fatalf("propose replacement: %v", err)
	}
	if proposed.Effective() {
		t.Fatal("拟替代凭空生效了")
	}
	if _, has := proposed.ExternalResult(); has {
		t.Fatal("拟替代凭空有了外部结果")
	}

	if _, err := domain.ProposeReplacement(followUpTarget(t, domain.InCaseCorrection),
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-2")); !errors.Is(err, domain.ErrInvalidFollowUpTarget) {
		t.Fatalf("err = %v; 更正目标建立了替代关系", err)
	}
	if _, err := domain.ProposeReplacement(replacement,
		mustValue(t, domain.NewDeclarationUnitID, "declaration-unit-1")); !errors.Is(err, domain.ErrInvalidFollowUpTarget) {
		t.Fatalf("err = %v; 替代单元与原单元同身份——原地修改吸收被明禁", err)
	}

	if _, err := proposed.TakeEffect("", targetFormedAt.Add(24*time.Hour)); !errors.Is(err, domain.ErrReplacementNotEffective) {
		t.Fatalf("err = %v; 没有外部结果的替代生效了", err)
	}

	effective, err := proposed.TakeEffect("customs-acceptance/RESUB-77", targetFormedAt.Add(24*time.Hour))
	if err != nil {
		t.Fatalf("take effect: %v", err)
	}
	if !effective.Effective() {
		t.Fatal("生效没落上")
	}
	result, has := effective.ExternalResult()
	if !has || result != "customs-acceptance/RESUB-77" {
		t.Fatalf("external result = %q has = %v", result, has)
	}
	if proposed.Effective() {
		t.Fatal("原拟替代关系被改写了")
	}

	if _, err := effective.TakeEffect("again", targetFormedAt.Add(48*time.Hour)); !errors.Is(err, domain.ErrInvalidFollowUpTarget) {
		t.Fatalf("err = %v; 生效生了两次", err)
	}
}
