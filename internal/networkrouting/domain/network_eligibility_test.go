package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

// Covers: UC-NR-002 结果语义「不适用：不得以不适用代替不可达，也不得虚构运营网络」——
// 没有依据的`不要求判断`正是这两件事共同的做法，它让一次未作出的判断看起来像一个结论。
func TestNotRequiringANetworkJudgmentDemandsAnExplicitBasis(t *testing.T) {
	_, err := domain.NewNetworkEligibility(domain.NetworkJudgmentNotRequired, domain.EligibilityBasisReference{})
	if !errors.Is(err, domain.ErrInvalidNetworkEligibility) {
		t.Fatalf("err = %v, want ErrInvalidNetworkEligibility——不带依据的不适用被接受了", err)
	}

	basis := mustValue(t, domain.NewEligibilityBasisReference, "LABEL_ONLY_CHANNEL_SERVICE")
	eligibility, err := domain.NewNetworkEligibility(domain.NetworkJudgmentNotRequired, basis)
	if err != nil {
		t.Fatalf("new network eligibility: %v", err)
	}
	if eligibility.JudgmentRequired() {
		t.Fatal("不要求判断却报告需要判断")
	}
	if eligibility.Basis() != basis {
		t.Fatalf("basis = %q, want %q", eligibility.Basis(), basis)
	}
}

// Covers: UC-NR-002 步骤 4——`要求判断`不需要依据，它只是让流程继续，不是一个结论。
func TestRequiringANetworkJudgmentNeedsNoBasis(t *testing.T) {
	eligibility, err := domain.NewNetworkEligibility(domain.NetworkJudgmentRequired, domain.EligibilityBasisReference{})
	if err != nil {
		t.Fatalf("new network eligibility: %v", err)
	}
	if !eligibility.JudgmentRequired() {
		t.Fatal("要求判断却报告不需要判断")
	}
}

// Covers: 零值不得被读成一个回答。端口没答话时应用层拿到的是零值，它必须报告「不要求」
// 为假——否则一次沉默会变成`不适用`。
func TestTheZeroEligibilityDoesNotAnswerNotRequired(t *testing.T) {
	var unanswered domain.NetworkEligibility
	if unanswered.JudgmentRequired() {
		t.Fatal("零值报告需要判断，那会让端口的沉默看起来像一个肯定回答")
	}
	if unanswered.Basis().String() != "" {
		t.Fatal("零值携带了依据")
	}

	_, err := domain.NewNetworkEligibility(domain.NetworkJudgmentRequirementInvalid, domain.EligibilityBasisReference{})
	if !errors.Is(err, domain.ErrInvalidNetworkEligibility) {
		t.Fatalf("err = %v, want ErrInvalidNetworkEligibility", err)
	}
}
