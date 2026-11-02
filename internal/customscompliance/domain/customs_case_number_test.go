package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func establishedCustomsCase(t *testing.T) domain.CustomsCase {
	t.Helper()
	customsCase, err := domain.EstablishCustomsCase(domain.CustomsCaseSpec{
		ID:            mustValue(t, domain.NewCustomsCaseID, "CASE-1"),
		Jurisdiction:  mustValue(t, domain.NewRegulatoryJurisdictionReference, "CN"),
		Direction:     domain.ExportManifest,
		Procedure:     mustValue(t, domain.NewCustomsProcedureReference, "CN-EXPORT/9610"),
		Obligation:    mustValue(t, domain.NewObligationScopeReference, "EXPORT-DECLARATION"),
		Parcels:       []domain.CaseParcelAssociation{{Parcel: "parcel-1", Customer: "customer-1", SourceRef: "PS/parcel-1"}},
		EstablishedAt: time.Date(2026, 11, 2, 3, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	return customsCase
}

// Covers: CC CONTEXT「关务案件」生命周期——建立案件时取得案件单据号，重新打开不换号；GLOSSARY「单据号」——单据号开立后
// 不变，没有单据号的单据记下是哪一种情形（ADR-0355 决定三、七）。刚建立的案件在号这一格上未落定；落定为有号或无号之后
// 不能再改成另一种，也不能换号；单据号不拿案件标识顶。
func TestACaseNumberIsSettledOnceAndNeverReplaced(t *testing.T) {
	fresh := establishedCustomsCase(t)
	if fresh.NumberingSettled() {
		t.Fatal("刚建立的案件不该已落定单据号")
	}
	if number, present := fresh.DocumentNumber(); present || number != "" {
		t.Fatalf("没取号的案件答出了单据号 %q", number)
	}

	numbered, err := fresh.Numbered("CC-20261102-0001")
	if err != nil {
		t.Fatal(err)
	}
	if number, present := numbered.DocumentNumber(); !present || number != "CC-20261102-0001" ||
		numbered.DocumentNumberAbsence() != domain.DocumentNumberAbsenceNone || !numbered.NumberingSettled() {
		t.Fatalf("numbered = %q %v %v", number, present, numbered.DocumentNumberAbsence())
	}
	if numbered.ID() != fresh.ID() {
		t.Fatal("取号改了案件标识")
	}
	if _, err := numbered.Numbered("CC-20261102-0002"); !errors.Is(err, domain.ErrInvalidCustomsCase) {
		t.Fatalf("换号没有被拒：%v", err)
	}
	if _, err := numbered.Unnumbered(domain.NumberingTimeZoneNotConfigured); !errors.Is(err, domain.ErrInvalidCustomsCase) {
		t.Fatalf("有号的案件被改成了无号：%v", err)
	}

	unnumbered, err := fresh.Unnumbered(domain.NumberingTimeZoneNotConfigured)
	if err != nil {
		t.Fatal(err)
	}
	if _, present := unnumbered.DocumentNumber(); present || !unnumbered.NumberingSettled() ||
		unnumbered.DocumentNumberAbsence().String() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("unnumbered = %v", unnumbered.DocumentNumberAbsence())
	}
	if _, err := unnumbered.Numbered("CC-20261102-0001"); !errors.Is(err, domain.ErrInvalidCustomsCase) {
		t.Fatalf("无号的案件事后被补了号：%v", err)
	}
	if before, err := fresh.Unnumbered(domain.IssuedBeforeNumbering); err != nil ||
		before.DocumentNumberAbsence().String() != "ISSUED_BEFORE_NUMBERING" {
		t.Fatalf("上线前开立：%v %v", before.DocumentNumberAbsence(), err)
	}
}

// Covers: 单据号是给人念、给人检索的号——空串、首尾空白、控制字符都不是号；「无号」必须说是哪一种情形。
func TestACaseRefusesMalformedNumbersAndUnnamedAbsence(t *testing.T) {
	fresh := establishedCustomsCase(t)
	for _, number := range []string{"", " CC-1", "CC-1 ", "CC-\u00011"} {
		if _, err := fresh.Numbered(number); !errors.Is(err, domain.ErrInvalidCustomsCase) {
			t.Fatalf("number %q was accepted", number)
		}
	}
	if _, err := fresh.Unnumbered(domain.DocumentNumberAbsenceNone); !errors.Is(err, domain.ErrInvalidCustomsCase) {
		t.Fatalf("没说情形的无号被收下了：%v", err)
	}
}
