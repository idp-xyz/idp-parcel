package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: AT-SA-200（编排半边）——首版与更正各把命令上的操作证据记在自己那一版上；操作证据不进内容摘要，同一版本由另一位操作者
// 重放答已存在，交回的仍是首次那一位；不带操作证据的一路（集成客户端族、受控 CLI）记空。
func TestAdoptionRecordsTheOperationEvidenceOnItsVersionAndAReplayKeepsTheFirstOne(t *testing.T) {
	fixture := newFundsFixture(t)
	first := adoptCommand(t, domain.FundsReceiptConfirmed)
	first.OperationEvidence = "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-01"

	adopted, err := fixture.handler.AdoptFact(context.Background(), first)
	if err != nil || adopted.Outcome() != application.FundsFactAdopted {
		t.Fatalf("首版采用：outcome = %q err = %v", adopted.Outcome(), err)
	}
	record, _ := adopted.Fact()
	if record.OperationEvidence != first.OperationEvidence {
		t.Fatalf("首版的操作证据 = %q, want %q", record.OperationEvidence, first.OperationEvidence)
	}

	unattributed := first
	unattributed.OperationEvidence = ""
	if digestless, err := fixture.handler.AdoptFact(context.Background(), unattributed); err != nil ||
		digestless.Outcome() != application.FundsFactExisting {
		t.Fatalf("不带操作证据的同内容重放：outcome = %q err = %v——操作证据进了内容比对", digestless.Outcome(), err)
	}
	replay := first
	replay.OperationEvidence = "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-02"
	replayed, err := fixture.handler.AdoptFact(context.Background(), replay)
	if err != nil || replayed.Outcome() != application.FundsFactExisting {
		t.Fatalf("另一位操作者重放：outcome = %q err = %v", replayed.Outcome(), err)
	}
	if kept, _ := replayed.Fact(); kept.OperationEvidence != first.OperationEvidence {
		t.Fatalf("重放顶替了首次的操作证据：%q", kept.OperationEvidence)
	}

	correction := correctCommand(t)
	correction.OperationEvidence = "OPERATOR/https://id.syn.example/dex#SYN-FINANCE-02"
	corrected, err := fixture.handler.CorrectFact(context.Background(), correction)
	if err != nil || corrected.Outcome() != application.FundsFactAdopted {
		t.Fatalf("更正：outcome = %q err = %v", corrected.Outcome(), err)
	}
	if version, _ := corrected.Fact(); version.OperationEvidence != correction.OperationEvidence {
		t.Fatalf("更正那一版的操作证据 = %q, want %q", version.OperationEvidence, correction.OperationEvidence)
	}
	head, _, _ := fixture.facts.FindVersion(context.Background(), record.Key, record.Fact.Version())
	if head.OperationEvidence != first.OperationEvidence {
		t.Fatalf("更正改写了首版的操作证据：%q", head.OperationEvidence)
	}

	other := adoptCommand(t, domain.FundsReceiptConfirmed)
	other.Fact, other.Version = "bank-receipt-2", "bank-receipt-2/v1"
	plain, err := fixture.handler.AdoptFact(context.Background(), other)
	if err != nil || plain.Outcome() != application.FundsFactAdopted {
		t.Fatalf("不记提交方的一路：outcome = %q err = %v", plain.Outcome(), err)
	}
	if unrecorded, _ := plain.Fact(); unrecorded.OperationEvidence != "" {
		t.Fatalf("没给操作证据却记了 %q", unrecorded.OperationEvidence)
	}
}
