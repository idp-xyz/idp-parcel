package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件钉 ADR-0381 决定四在编排里的落点：控制端口答`等受控补充`时，带缺口的等待态先落库（ADR-0094
// Decision 五那道护栏原样扩用），本轮交回`等待客户补充`；计价点名的缺项码原样记在处理尝试上，等运营
// 登记那一路同样记。

// supplementStallGaps 造适配器译好的两条缺口：P-1 缺测量（按件）+ 委托级缺收件邮编。
func supplementStallGaps(t *testing.T) []domain.SupplementGap {
	t.Helper()
	parcel, err := domain.NewDeclaredParcelID("P-1")
	if err != nil {
		t.Fatalf("parcel: %v", err)
	}
	perParcel, err := domain.NewSupplementGap(domain.PreAcceptanceFinancialControlCheck, parcel,
		mustValue(t, domain.NewCheckReason, "ESTIMATE_MEASUREMENT_MISSING"))
	if err != nil {
		t.Fatalf("per-parcel gap: %v", err)
	}
	requestLevel, err := domain.NewSupplementGap(domain.PreAcceptanceFinancialControlCheck, domain.DeclaredParcelID{},
		mustValue(t, domain.NewCheckReason, "ESTIMATE_RECIPIENT_POSTAL_MISSING"))
	if err != nil {
		t.Fatalf("request-level gap: %v", err)
	}
	return []domain.SupplementGap{perParcel, requestLevel}
}

// incompleteFixture 把财务控制夹具拨到「申报不全、缺项全是客户补得上的」那一格。
func incompleteFixture(t *testing.T) *financialControlFixture {
	t.Helper()
	fixture := newFinancialControlFixture(t)
	fixture.controller.portOutcome = ports.PreAcceptanceControlCustomerSupplementRequired
	fixture.controller.reason = "SA-ESTIMATE_DECLARATION_INCOMPLETE"
	fixture.controller.settlementReason = "ESTIMATE_DECLARATION_INCOMPLETE"
	fixture.controller.supplementGaps = supplementStallGaps(t)
	fixture.controller.estimateGapCodes = []string{"MEASUREMENT", "POSTAL_ROUTE"}
	return fixture
}

// Covers: ADR-0381 决定四 + ADR-0094 Decision 五——`等受控补充`的停法：带缺口的等待态先 Save 落库，本轮交回
// `等待客户补充`；处理尝试带该格的续办路径、结算原因码原样、这一版申报的产品与缺项码原样；控制结果一份不记。
func TestACustomerSupplementRequiredOutcomeParksTheRequestWithItsGaps(t *testing.T) {
	fixture := incompleteFixture(t)
	fixture.repository.product = "SYN-PROD-01"

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceJudgmentUndecided {
		t.Fatalf("outcome = %q, want UNDECIDED", result.Outcome())
	}
	if result.PendingReason() != application.CustomerSupplementPending {
		t.Fatalf("pending reason = %q, want CUSTOMER_SUPPLEMENT_PENDING", result.PendingReason())
	}
	saved := fixture.repository.saved
	if saved == nil {
		t.Fatal("等待态没有落库——交回`等待客户补充`之前要先 Save")
	}
	if waiting, present := saved.AcceptanceDecisionTask().WaitingOn(); !present || waiting != domain.ResumeByCustomerSupplement {
		t.Fatalf("saved waitingOn = %v/%v, want CUSTOMER_SUPPLEMENT", waiting, present)
	}
	gaps := saved.AcceptanceDecisionTask().SupplementGaps()
	want := supplementStallGaps(t)
	if len(gaps) != len(want) || gaps[0] != want[0] || gaps[1] != want[1] {
		t.Fatalf("saved gaps = %+v, want 适配器译好的那两条原样落库", gaps)
	}
	if saved.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("saved state = %q, want SUBMITTED——停等不是决定", saved.State())
	}
	if len(fixture.requests.recordedAttempts) != 1 {
		t.Fatalf("processing attempts = %d, want 1", len(fixture.requests.recordedAttempts))
	}
	attempt := fixture.requests.recordedAttempts[0]
	if attempt.ResumePath() != domain.ResumeByCustomerSupplement {
		t.Fatalf("attempt resume path = %q, want CUSTOMER_SUPPLEMENT", attempt.ResumePath())
	}
	if reason, present := attempt.SettlementReason(); !present || reason.String() != "ESTIMATE_DECLARATION_INCOMPLETE" {
		t.Fatalf("settlement reason = %q（present=%v），want ESTIMATE_DECLARATION_INCOMPLETE", reason, present)
	}
	if attempt.ServiceProduct().String() != "SYN-PROD-01" {
		t.Fatalf("service product = %q, want SYN-PROD-01", attempt.ServiceProduct())
	}
	if !slices.Equal(attempt.EstimateGapCodes(), []string{"MEASUREMENT", "POSTAL_ROUTE"}) {
		t.Fatalf("estimate gap codes = %v, want 计价点名的缺项码原样", attempt.EstimateGapCodes())
	}
	if attempt.ContinuationReference() != result.ContinuationReference() {
		t.Fatalf("attempt continuation = %q, result continuation = %q", attempt.ContinuationReference(), result.ContinuationReference())
	}
	if len(fixture.requests.recordedControl) != 0 {
		t.Fatalf("recorded %d control results for a control that never formed", len(fixture.requests.recordedControl))
	}
}

// Covers: 同一道护栏的反面——带缺口的等待态没落库时不得交回`等待客户补充`（那是消费门提交暂停的凭据），
// 改交这条等待自己的保存格 CUSTOMER_SUPPLEMENT_WAIT_NOT_SAVED，归内部重试；结算原因、产品与缺项码随范围
// 一起放下。版本被抢先照旧是重读那一格。
func TestACustomerSupplementWaitThatDidNotLandFallsBackToItsOwnReason(t *testing.T) {
	unsaved := incompleteFixture(t)
	unsaved.repository.product = "SYN-PROD-01"
	unsaved.repository.saveErr = errors.New("库不可达")

	result, err := unsaved.handler.Handle(context.Background(), unsaved.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() != application.CustomerSupplementWaitNotSaved {
		t.Fatalf("pending reason = %q, want CUSTOMER_SUPPLEMENT_WAIT_NOT_SAVED", result.PendingReason())
	}
	if len(unsaved.requests.recordedAttempts) != 1 {
		t.Fatalf("processing attempts = %d, want 1", len(unsaved.requests.recordedAttempts))
	}
	attempt := unsaved.requests.recordedAttempts[0]
	if attempt.ResumePath() != domain.ResumeByInternalRetry {
		t.Fatalf("attempt resume path = %q, want INTERNAL_RETRY——没落库的停等只有本方推得动", attempt.ResumePath())
	}
	if _, present := attempt.SettlementReason(); present || attempt.ServiceProduct().Declared() || len(attempt.EstimateGapCodes()) != 0 {
		t.Fatalf("等待没落库的那一轮带着结算原因、产品或缺项码：%+v", attempt)
	}

	conflicted := incompleteFixture(t)
	conflicted.repository.conflicted = true
	second, err := conflicted.handler.Handle(context.Background(), conflicted.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if second.PendingReason() != application.StaleShipmentRequestRevision {
		t.Fatalf("pending reason = %q, want STALE_SHIPMENT_REQUEST_REVISION", second.PendingReason())
	}
	if second.ContinuationReference() == result.ContinuationReference() {
		t.Fatal("保存没落库与版本被抢先共用续办引用——查回来分不出该重试还是该重读")
	}
}

// Covers: ADR-0381 决定一、四——混着登记侧缺项时登记优先：`未配置`那一格照旧停成等运营登记，缺项码同样
// 原样记在处理尝试上（登记侧也要知道这一停是缺哪几格），不落补充缺口。
func TestARegistrationFirstIncompleteStallCarriesTheGapCodesToo(t *testing.T) {
	fixture := newFinancialControlFixture(t)
	fixture.controller.portOutcome = ports.PreAcceptanceControlNotConfigured
	fixture.controller.reason = "SA-ESTIMATE_DECLARATION_INCOMPLETE"
	fixture.controller.settlementReason = "ESTIMATE_DECLARATION_INCOMPLETE"
	fixture.controller.estimateGapCodes = []string{"MEASUREMENT", "ZONE"}

	result, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.PendingReason() != application.FinancialControlNotConfigured {
		t.Fatalf("pending reason = %q, want FINANCIAL_CONTROL_NOT_CONFIGURED", result.PendingReason())
	}
	saved := fixture.repository.saved
	if saved == nil {
		t.Fatal("等待态没有落库")
	}
	if waiting, present := saved.AcceptanceDecisionTask().WaitingOn(); !present || waiting != domain.ResumeByOperatorRegistration {
		t.Fatalf("saved waitingOn = %v/%v, want OPERATOR_REGISTRATION——登记优先", waiting, present)
	}
	if len(saved.AcceptanceDecisionTask().SupplementGaps()) != 0 {
		t.Fatalf("登记那一路落了补充缺口：%+v", saved.AcceptanceDecisionTask().SupplementGaps())
	}
	attempt := fixture.requests.recordedAttempts[0]
	if !slices.Equal(attempt.EstimateGapCodes(), []string{"MEASUREMENT", "ZONE"}) {
		t.Fatalf("estimate gap codes = %v, want 缺项码在登记那一路同样留痕", attempt.EstimateGapCodes())
	}
}
