package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证各编排遇授权`适用冲突`（ADR-0261 决定三：同一刻多于一条授予允许、或委派方不同的多条委派命中）时的落点：
// 有未决格的四只停在未决、原因指名授权适用冲突；没有未决格的两只各答 AUTHORITY_APPLICABILITY_CONFLICT。六只都不读作
// 已授权（什么都不写、不签标识），也不读作拒绝（不落`未获授权`）；与`授权规则未配置`分得开。

// Covers: ADR-0261 越权风险点 3——主动拒绝停在未决，原因 REJECTION_AUTHORITY_APPLICABILITY_CONFLICT。
func TestAnApplicabilityConflictStallsTheRejection(t *testing.T) {
	fixture := newRejectionFixture(t)
	fixture.authorizer.applicabilityConflict = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ActiveRejectionUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED——适用冲突不是`未获授权`", result.Outcome())
	}
	assertConflictReason(t, result.PendingReason(), application.RejectionAuthorityApplicabilityConflict,
		"REJECTION_AUTHORITY_APPLICABILITY_CONFLICT", application.RejectionAuthorityRulesNotConfigured)
	if result.State() == domain.ShipmentRequestRejected || fixture.identities.issued != 0 {
		t.Fatalf("state = %q, issued = %d——适用冲突被读成了已授权", result.State(), fixture.identities.issued)
	}
}

// Covers: 同上，撤回停在未决，原因 WITHDRAWAL_AUTHORITY_APPLICABILITY_CONFLICT。
func TestAnApplicabilityConflictStallsTheWithdrawal(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.authorizer.applicabilityConflict = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.WithdrawalUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED——适用冲突不是`未获授权`", result.Outcome())
	}
	assertConflictReason(t, result.PendingReason(), application.WithdrawalAuthorityApplicabilityConflict,
		"WITHDRAWAL_AUTHORITY_APPLICABILITY_CONFLICT", application.WithdrawalAuthorityRulesNotConfigured)
	if fixture.identities.issued != 0 || fixture.requests.saved != nil {
		t.Fatal("适用冲突被读成了已授权：签了标识或写回了委托")
	}
}

// Covers: 同上，资料修订停在未决，原因 SOURCE_DATA_AMENDMENT_AUTHORITY_APPLICABILITY_CONFLICT。
func TestAnApplicabilityConflictStallsTheAmendment(t *testing.T) {
	fixture := newAmendmentFixture(t)
	fixture.authorizer.applicabilityConflict = true

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AmendmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED——适用冲突不是`未获授权`", result.Outcome())
	}
	assertConflictReason(t, result.PendingReason(), application.SourceDataAmendmentAuthorityApplicabilityConflict,
		"SOURCE_DATA_AMENDMENT_AUTHORITY_APPLICABILITY_CONFLICT", application.SourceDataAmendmentAuthorityRulesNotConfigured)
	if _, present := result.Version(); present || fixture.identities.issued != 0 {
		t.Fatal("适用冲突被读成了已授权：形成了版本或签了标识")
	}
}

func assertConflictReason(
	t *testing.T,
	got, want application.JudgmentPendingReason,
	name string,
	notConfigured application.JudgmentPendingReason,
) {
	t.Helper()
	if got != want {
		t.Fatalf("pending reason = %q, want %s", got, name)
	}
	if got.String() != name {
		t.Fatalf("String() = %q, want %q", got.String(), name)
	}
	if got == notConfigured {
		t.Fatal("适用冲突与授权规则未配置共用一个原因——恢复动作不同（收尾旧版对登记规则）")
	}
}

// Covers: 同上，受控关闭与重开停在未决，原因 AUTHORITY_APPLICABILITY_CONFLICT；什么都不写、不交接、不签标识。
func TestAnApplicabilityConflictStallsTheContinuedAttemptDecision(t *testing.T) {
	fixture := newContinuedAttemptFixture(t)
	fixture.authorizer.answer = ports.ContinuedAttemptDecisionAuthorization{Outcome: ports.AuthorizationApplicabilityConflict}

	result, err := fixture.handler.FormControlledClosure(t.Context(), fixture.closureCommand(t))
	if err != nil {
		t.Fatalf("形成关闭：%v", err)
	}
	if result.Outcome() != application.ContinuedAttemptDecisionUndecided {
		t.Fatalf("outcome = %v, want UNDECIDED——适用冲突不是`不允许`", result.Outcome())
	}
	if result.PendingReason() != application.ContinuedAttemptAuthorityApplicabilityConflict ||
		result.PendingReason().String() != "AUTHORITY_APPLICABILITY_CONFLICT" {
		t.Fatalf("pending = %v, want AUTHORITY_APPLICABILITY_CONFLICT", result.PendingReason())
	}
	if application.ContinuedAttemptAuthorityApplicabilityConflict == application.ContinuedAttemptAuthorityRulesNotConfigured {
		t.Fatal("适用冲突与授权规则未配置共用一个原因")
	}
	if _, present := result.Decision(); present ||
		fixture.registers.inserts != 0 || fixture.registers.saves != 0 || len(fixture.handoff.intents) != 0 || fixture.identities.next != 0 {
		t.Fatal("适用冲突被读成了已授权：形成了决定、写了册、交接了或签了标识")
	}
}

// Covers: ADR-0261 越权风险点 3——授权处置没有未决格，适用冲突答它自己的一码 AUTHORITY_APPLICABILITY_CONFLICT：
// 不落库、状态不变。生产上装的是不问 PC 的未配置授权口（ADR-0132 越权风险点 2），这一码今天走不到，只为闭集穷尽。
func TestAnApplicabilityConflictAnswersTheDispositionWithItsOwnCode(t *testing.T) {
	fixture := newDispositionFixture(t)
	fixture.authorizer.outcome = ports.AuthorizationApplicabilityConflict

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t, domain.DisposeByRejection))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AuthorizedDispositionAuthorityApplicabilityConflict ||
		result.Outcome().String() != "AUTHORITY_APPLICABILITY_CONFLICT" {
		t.Fatalf("outcome = %q, want AUTHORITY_APPLICABILITY_CONFLICT", result.Outcome())
	}
	fixture.assertNothingWritten(t)
}

// Covers: 同上，完成人工复核答 AUTHORITY_APPLICABILITY_CONFLICT：不落库、状态不变，不压成`未获授权`。
func TestAnApplicabilityConflictAnswersTheReviewCompletionWithItsOwnCode(t *testing.T) {
	fixture := newReviewCompletionFixture(t)
	fixture.authorizer.outcome = ports.AuthorizationApplicabilityConflict

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ManualReviewAuthorityApplicabilityConflict ||
		result.Outcome().String() != "AUTHORITY_APPLICABILITY_CONFLICT" {
		t.Fatalf("outcome = %q, want AUTHORITY_APPLICABILITY_CONFLICT", result.Outcome())
	}
	if _, present := result.Completion(); present || fixture.requests.saved != nil {
		t.Fatal("适用冲突被读成了已授权：带了完成留痕或写回了委托")
	}
	if result.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", result.State())
	}
}
