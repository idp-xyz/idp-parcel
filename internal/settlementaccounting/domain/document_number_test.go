package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// Covers: ADR-0355 决定一、三、七、十一——单据上那一格要么是号、要么是一个具名的无号原因，两者恰有其一：空白的号与没名字的原因都
// 造不出来；答复与读口的两格词形只出其一；零值两格都空，落库前据此拦下漏接取号的路径。
func TestADocumentNumberingIsANumberOrANamedAbsence(t *testing.T) {
	if _, err := domain.NewDocumentNumber("  "); err == nil {
		t.Fatal("空白的单据号造了出来")
	}
	if _, err := domain.NumberedDocument(domain.DocumentNumber{}); err == nil {
		t.Fatal("零值单据号造成了一格有号")
	}
	if _, err := domain.UnnumberedDocument(domain.DocumentNumberAbsenceInvalid); err == nil {
		t.Fatal("没名字的无号原因造了出来")
	}

	numbered, err := domain.NumberedDocument(settlementValue(t, domain.NewDocumentNumber, "DSP-202611-00001"))
	if err != nil {
		t.Fatal(err)
	}
	if number, ok := numbered.Number(); !ok || number.String() != "DSP-202611-00001" {
		t.Fatalf("号 = %v %v", number, ok)
	}
	if _, absent := numbered.Absence(); absent {
		t.Fatal("有号的格还答了无号原因")
	}
	if number, absence := numbered.Words(); number != "DSP-202611-00001" || absence != "" {
		t.Fatalf("有号的词形 = %q %q", number, absence)
	}

	for absence, word := range map[domain.DocumentNumberAbsence]string{
		domain.IssuedBeforeNumbering:         "ISSUED_BEFORE_NUMBERING",
		domain.NumberingTimeZoneUnconfigured: "NUMBERING_TIME_ZONE_NOT_CONFIGURED",
	} {
		unnumbered, err := domain.UnnumberedDocument(absence)
		if err != nil {
			t.Fatal(err)
		}
		if _, numbered := unnumbered.Number(); numbered {
			t.Fatalf("%s 的格答了有号", word)
		}
		if got, absent := unnumbered.Absence(); !absent || got != absence {
			t.Fatalf("%s 的原因 = %v %v", word, got, absent)
		}
		if number, got := unnumbered.Words(); number != "" || got != word {
			t.Fatalf("%s 的词形 = %q %q", word, number, got)
		}
	}

	if _, numbered := (domain.DocumentNumbering{}).Number(); numbered {
		t.Fatal("零值答了有号")
	}
	if _, absent := (domain.DocumentNumbering{}).Absence(); absent {
		t.Fatal("零值答了无号原因")
	}
	if number, absence := (domain.DocumentNumbering{}).Words(); number != "" || absence != "" {
		t.Fatalf("零值的词形 = %q %q", number, absence)
	}
}

// Covers: ADR-0355 决定三（领域半边）——发布前的判断不要单号：勾稽不平与发布时刻早于截单都在取号之前答出，PublishStatement 判同一套，
// 另外要一个单号；两处判法一致，编排先判后取号不会与发布那一步各说一套。
func TestAStatementDraftIsCheckedForPublicationBeforeItHasANumber(t *testing.T) {
	draft := cutDraft(t)
	if err := draft.CheckPublication(15000, statementPublishedAt); err != nil {
		t.Fatalf("勾稽平、时刻对：%v", err)
	}
	if err := draft.CheckPublication(15001, statementPublishedAt); !errors.Is(err, domain.ErrStatementImbalance) {
		t.Fatalf("勾稽不平 = %v", err)
	}
	if err := draft.CheckPublication(15000, statementCutOffAt.Add(-1)); !errors.Is(err, domain.ErrInvalidStatement) {
		t.Fatalf("发布早于截单 = %v", err)
	}
	if err := (domain.StatementDraft{}).CheckPublication(0, statementPublishedAt); !errors.Is(err, domain.ErrInvalidStatement) {
		t.Fatalf("零值草稿 = %v", err)
	}
	if _, err := domain.PublishStatement(draft, domain.StatementNumber{}, 15000, statementPublishedAt); !errors.Is(err, domain.ErrInvalidStatement) {
		t.Fatalf("没有单号也发布了：%v", err)
	}
	if _, err := domain.PublishStatement(draft, settlementValue(t, domain.NewStatementNumber, "STMT-202608-00001"), 15001, statementPublishedAt); !errors.Is(err, domain.ErrStatementImbalance) {
		t.Fatalf("发布那一步的勾稽 = %v", err)
	}
}
