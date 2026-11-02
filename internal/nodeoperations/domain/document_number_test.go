package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// Covers: ADR-0355 决定三、七与 GLOSSARY「单据号」的无号情形——单据号与无号原因恰有其一：库里号码与原因都空读作上线前开立，只记原因
// 的读作单据编号时区未配置；号码与原因同在、原因认不出都是坏行；零值是「还没定」，既不是有号也不是无号。
func TestDocumentNumberingHoldsExactlyANumberOrAnAbsence(t *testing.T) {
	number, err := domain.NewDocumentNumber("PA-20261102-00045")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := domain.NewDocumentNumber("  "); err == nil {
		t.Fatal("空白单据号被收下")
	}

	numbered := domain.NumberedDocument(number)
	if got, ok := numbered.Number(); !ok || got != number || !numbered.Decided() {
		t.Fatalf("有号 = %q %v", got, ok)
	}
	if _, absent := numbered.Absence(); absent {
		t.Fatal("有号的单据也报了无号原因")
	}

	withheld := domain.UnnumberedForMissingTimeZone()
	if absence, ok := withheld.Absence(); !ok || absence != domain.NumberingTimeZoneNotConfigured || !withheld.Decided() {
		t.Fatalf("单据编号时区未配置 = %v %v", absence, ok)
	}
	if _, numberedToo := withheld.Number(); numberedToo {
		t.Fatal("无号的单据报了号")
	}

	if (domain.DocumentNumbering{}).Decided() {
		t.Fatal("零值被当成定了")
	}

	for name, probe := range map[string]struct {
		number, absence string
		want            domain.DocumentNumberAbsence
		numbered        bool
	}{
		"有号":         {"PA-20261102-00045", "", domain.DocumentNumberAbsenceInvalid, true},
		"都空即上线前开立":   {"", "", domain.IssuedBeforeNumbering, false},
		"只有原因即时区未配置": {"", "NUMBERING_TIME_ZONE_NOT_CONFIGURED", domain.NumberingTimeZoneNotConfigured, false},
	} {
		got, err := domain.RehydrateDocumentNumbering(probe.number, probe.absence)
		if err != nil {
			t.Fatalf("%s：%v", name, err)
		}
		if _, numbered := got.Number(); numbered != probe.numbered {
			t.Fatalf("%s：有号 = %v", name, numbered)
		}
		if absence, _ := got.Absence(); !probe.numbered && absence != probe.want {
			t.Fatalf("%s：原因 = %v", name, absence)
		}
	}
	for name, probe := range map[string][2]string{
		"号码与原因同在":  {"PA-20261102-00045", "NUMBERING_TIME_ZONE_NOT_CONFIGURED"},
		"原因认不出":    {"", "LOST"},
		"上线前开立不入库": {"", "ISSUED_BEFORE_NUMBERING"},
		"空白号码":     {" ", ""},
	} {
		if _, err := domain.RehydrateDocumentNumbering(probe[0], probe[1]); !errors.Is(err, domain.ErrInvalidDocumentNumbering) {
			t.Fatalf("%s：err = %v", name, err)
		}
	}
	for _, absence := range []domain.DocumentNumberAbsence{domain.IssuedBeforeNumbering, domain.NumberingTimeZoneNotConfigured} {
		if absence.String() == "" {
			t.Fatalf("原因 %d 没有线上名", absence)
		}
	}
}
