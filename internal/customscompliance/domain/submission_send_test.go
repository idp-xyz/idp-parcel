package domain_test

import (
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 钉 ADR-0342 的领域半边：申报通道只收 https 或回环上的 http；发送事实只收越过边界之后的三种结果；技术回执只收明确的两种；
// 尝试此刻的技术判断由发送事实与回执折出，不改写前面那几份。

// Covers: AT-CC-453（领域半边：端点）
func TestADeclarationChannelOnlyTakesHTTPSOrALoopback(t *testing.T) {
	for _, endpoint := range []string{"https://broker.syn.example/declarations", "http://127.0.0.1:5590/declarations", "http://localhost:5590/d"} {
		if _, err := domain.NewDeclarationChannel("SYN-CHANNEL-01", endpoint, ""); err != nil {
			t.Fatalf("%s err = %v", endpoint, err)
		}
	}
	for _, endpoint := range []string{"http://broker.syn.example/declarations", "ftp://127.0.0.1/x", "/relative", ""} {
		if _, err := domain.NewDeclarationChannel("SYN-CHANNEL-01", endpoint, ""); !errors.Is(err, domain.ErrInvalidDeclarationChannel) {
			t.Fatalf("%s err = %v，want 拒", endpoint, err)
		}
	}
}

// Covers: AT-CC-455（领域半边：当前判断）
func TestTheCurrentTechnicalJudgmentFoldsTheSendFactAndTheReceipts(t *testing.T) {
	version, err := domain.NewSubmissionVersionID("DECLV-SYN-1")
	if err != nil {
		t.Fatal(err)
	}
	sentAt := time.Date(2026, 10, 28, 6, 0, 0, 0, time.UTC)
	if _, err := domain.RecordSubmissionSend(version, 1, sentAt, domain.AttemptRegisteredPendingSend, "", ""); !errors.Is(err, domain.ErrInvalidSendFact) {
		t.Fatalf("发送事实记成待发送 err = %v", err)
	}
	if _, err := domain.ReceiveTechnicalReceipt("MSG-1", version, 1, domain.AttemptPendingConfirmation, sentAt, sentAt, "d"); !errors.Is(err, domain.ErrInvalidTechnicalReceipt) {
		t.Fatalf("回执记成待确认 err = %v", err)
	}
	if _, err := domain.RecordSubmissionSend(version, 1, sentAt, domain.AttemptAcknowledged, strings.Repeat("M", 257), ""); !errors.Is(err, domain.ErrInvalidSendFact) {
		t.Fatalf("超长消息号 err = %v", err)
	}
	send, err := domain.RecordSubmissionSend(version, 1, sentAt, domain.AttemptPendingConfirmation, "", "timeout")
	if err != nil {
		t.Fatal(err)
	}
	acknowledged, _ := domain.ReceiveTechnicalReceipt("MSG-1", version, 1, domain.AttemptAcknowledged, sentAt, sentAt.Add(time.Minute), "d1")
	failed, _ := domain.ReceiveTechnicalReceipt("MSG-2", version, 1, domain.AttemptFailed, sentAt, sentAt.Add(2*time.Minute), "d2")

	for name, tc := range map[string]struct {
		receipts []domain.TechnicalReceipt
		want     domain.AttemptResult
	}{
		"发了没答":    {nil, domain.AttemptPendingConfirmation},
		"后到明确收到":  {[]domain.TechnicalReceipt{acknowledged}, domain.AttemptAcknowledged},
		"再后到校验未过": {[]domain.TechnicalReceipt{failed, acknowledged}, domain.AttemptFailed},
	} {
		got, err := domain.TechnicalJudgmentAfterSend(send, tc.receipts)
		if err != nil || got != tc.want {
			t.Fatalf("%s = %s err = %v，want %s", name, got, err, tc.want)
		}
	}
	other, _ := domain.NewSubmissionVersionID("DECLV-SYN-2")
	stray, _ := domain.ReceiveTechnicalReceipt("MSG-3", other, 1, domain.AttemptAcknowledged, sentAt, sentAt, "d3")
	if _, err := domain.TechnicalJudgmentAfterSend(send, []domain.TechnicalReceipt{stray}); !errors.Is(err, domain.ErrInvalidTechnicalReceipt) {
		t.Fatalf("别的尝试的回执混进来 err = %v", err)
	}
}
