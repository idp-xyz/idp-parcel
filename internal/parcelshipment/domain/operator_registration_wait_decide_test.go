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
	if got := len(waiting.AcceptanceDecisionTask().ProcessingAttempts()); got != 0 {
		t.Fatalf("等待登记追加了 %d 条处理记录；那是 RecordProcessingAttempt 的事", got)
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
