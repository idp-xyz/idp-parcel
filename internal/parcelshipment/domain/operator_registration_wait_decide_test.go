package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件钉 ADR-0094 Decision 五在领域层的两处落点：`等待运营登记`既能由 Decide 从一条带
// 第四格续办路径的未决校验里认出来，也能在**决定形成之前**由一条不经校验的转移写下——
// `*AsOfNotConfigured` 那一族停在判断时点形成之前，没有任何校验可以进 Decide。

// Covers: ADR-0094 Decision 二——重试、客户补件、人工复核都推不动的未决，Decide 落到第四格，
// 而不是折进`等待内部续办`（那一格会对着一个从未登记的参数无休止重投）。
func TestAnUndeterminedCheckWaitingOnRegistrationParksTheTaskOnOperatorRegistration(t *testing.T) {
	checks := append(allGroupsPassing(t), undeterminedCheck(
		t, domain.CustomerRelationshipCheck, "AUTHORITY_RULES_NOT_CONFIGURED", domain.ResumeByOperatorRegistration,
	))

	decided, err := submitted(t).Decide(decisionSpec(t, checks))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decided.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q; 等登记不是决定", decided.State())
	}
	waiting, present := decided.AcceptanceDecisionTask().WaitingOn()
	if !present || waiting != domain.ResumeByOperatorRegistration {
		t.Fatalf("waitingOn = %q (present=%v), want OPERATOR_REGISTRATION", waiting, present)
	}
}

// Covers: 等待态的优先次序。客户侧缺口仍排最前（只有那一条要通知外部并受补充期限约束）；
// 登记压过内部重试——重试产不出一次登记，而登记之后整轮重跑，内部那一格自会再得机会。
func TestOperatorRegistrationOutranksInternalRetryButNotCustomerSupplement(t *testing.T) {
	registration := undeterminedCheck(
		t, domain.CustomerRelationshipCheck, "AUTHORITY_RULES_NOT_CONFIGURED", domain.ResumeByOperatorRegistration)
	retry := undeterminedCheck(
		t, domain.LegalEntityAndContractCheck, "DEPENDENCY_TIMEOUT", domain.ResumeByInternalRetry)
	supplement := undeterminedCheck(
		t, domain.RequiredDocumentCheck, "DOCUMENT_PENDING", domain.ResumeByCustomerSupplement)

	cases := []struct {
		name   string
		checks []domain.AcceptanceCheck
		want   domain.ResumePath
	}{
		{"registration beats internal retry", []domain.AcceptanceCheck{retry, registration}, domain.ResumeByOperatorRegistration},
		{"customer supplement beats registration", []domain.AcceptanceCheck{registration, supplement}, domain.ResumeByCustomerSupplement},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			decided, err := submitted(t).Decide(decisionSpec(t, append(allGroupsPassing(t), tc.checks...)))
			if err != nil {
				t.Fatalf("decide: %v", err)
			}
			waiting, _ := decided.AcceptanceDecisionTask().WaitingOn()
			if waiting != tc.want {
				t.Fatalf("waitingOn = %q, want %q", waiting, tc.want)
			}
		})
	}
}

// Covers: ADR-0094 Decision 五——「落此格前必须先把带等待态的聚合 Save 落库」。要保存就得先有
// 一条在决定之前写下等待态的转移：它只写等待态，不动状态、不形成决定、不动版本、不追加记录。
func TestAwaitOperatorRegistrationParksARunningTaskWithoutDeciding(t *testing.T) {
	request := submitted(t)

	waiting, err := request.AwaitOperatorRegistration()
	if err != nil {
		t.Fatalf("await operator registration: %v", err)
	}
	if waiting.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", waiting.State())
	}
	if _, present := waiting.AcceptanceDecision(); present {
		t.Fatal("等待登记形成了一份决定")
	}
	if waiting.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("等待登记关闭了判断任务")
	}
	path, present := waiting.AcceptanceDecisionTask().WaitingOn()
	if !present || path != domain.ResumeByOperatorRegistration {
		t.Fatalf("waitingOn = %q (present=%v), want OPERATOR_REGISTRATION", path, present)
	}
	if waiting.Revision() != request.Revision() {
		t.Fatalf("revision moved from %d to %d; 转移一律不动版本", request.Revision(), waiting.Revision())
	}
}

// estimateGaps 造「一件缺测量 + 委托级缺收件邮编」两条接受前财务控制判组的缺口。
func estimateGaps(t *testing.T) []domain.SupplementGap {
	t.Helper()
	parcel, err := domain.NewDeclaredParcelID("P-1")
	if err != nil {
		t.Fatalf("parcel: %v", err)
	}
	perParcel, err := domain.NewSupplementGap(domain.PreAcceptanceFinancialControlCheck, parcel,
		checkReason(t, "ESTIMATE_MEASUREMENT_MISSING"))
	if err != nil {
		t.Fatalf("per-parcel gap: %v", err)
	}
	requestLevel, err := domain.NewSupplementGap(domain.PreAcceptanceFinancialControlCheck, domain.DeclaredParcelID{},
		checkReason(t, "ESTIMATE_RECIPIENT_POSTAL_MISSING"))
	if err != nil {
		t.Fatalf("request-level gap: %v", err)
	}
	return []domain.SupplementGap{perParcel, requestLevel}
}

// Covers: ADR-0381 决定四 + ADR-0094 Decision 五——申报不全停在控制结果形成之前，与等登记同理需要一条不经校验
// 的转移：AwaitCustomerSupplement 只写等待态与缺口，不动状态、不形成决定、不动版本、不追加记录；缺口随等待态
// 一起读得回。
func TestAwaitCustomerSupplementParksARunningTaskWithItsGaps(t *testing.T) {
	request := submitted(t)
	gaps := estimateGaps(t)

	waiting, err := request.AwaitCustomerSupplement(gaps)
	if err != nil {
		t.Fatalf("await customer supplement: %v", err)
	}
	if waiting.State() != domain.ShipmentRequestSubmitted {
		t.Fatalf("state = %q, want SUBMITTED", waiting.State())
	}
	if _, present := waiting.AcceptanceDecision(); present {
		t.Fatal("等待补充形成了一份决定")
	}
	if waiting.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("等待补充关闭了判断任务")
	}
	path, present := waiting.AcceptanceDecisionTask().WaitingOn()
	if !present || path != domain.ResumeByCustomerSupplement {
		t.Fatalf("waitingOn = %q (present=%v), want CUSTOMER_SUPPLEMENT", path, present)
	}
	stored := waiting.AcceptanceDecisionTask().SupplementGaps()
	if len(stored) != len(gaps) || stored[0] != gaps[0] || stored[1] != gaps[1] {
		t.Fatalf("gaps = %+v, want %+v 原样随等待态读回", stored, gaps)
	}
	if waiting.Revision() != request.Revision() {
		t.Fatalf("revision moved from %d to %d; 转移一律不动版本", request.Revision(), waiting.Revision())
	}
}

// Covers: AwaitCustomerSupplement 的几道门——空缺口是调用方翻译坏了（计价答申报不全必点名缺项），不是一种可停
// 的等待；已越过决定边界的委托不再进入任何等待态。
func TestAwaitCustomerSupplementRefusesEmptyGapsAndADecidedRequest(t *testing.T) {
	if _, err := submitted(t).AwaitCustomerSupplement(nil); !errors.Is(err, domain.ErrInvalidAcceptanceCheck) {
		t.Fatalf("空缺口 err = %v, want ErrInvalidAcceptanceCheck", err)
	}

	decided, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if _, err := decided.AwaitCustomerSupplement(estimateGaps(t)); !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		t.Fatalf("已决 err = %v, want ErrDecisionAlreadyFormed", err)
	}
}

// Covers: 已越过决定边界的委托不再进入任何等待态——第二次尝试要说出真实原因。
func TestAwaitOperatorRegistrationRefusesADecidedRequest(t *testing.T) {
	decided, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if decided.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("fixture did not accept: %q", decided.State())
	}

	if _, err := decided.AwaitOperatorRegistration(); !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		t.Fatalf("err = %v, want ErrDecisionAlreadyFormed", err)
	}
}
