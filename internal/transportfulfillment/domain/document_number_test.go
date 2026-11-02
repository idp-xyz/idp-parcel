package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: ADR-0355 决定一、三——单据上那一格有号与无号互斥；零值读作本规则生效前开立；单据号与订舱标识同过字节界与 NUL 门。
func TestADocumentEitherCarriesANumberOrSaysWhyItHasNone(t *testing.T) {
	number, err := domain.NewDocumentNumber("BK-202611-00009")
	if err != nil {
		t.Fatal(err)
	}
	numbered := domain.NumberedDocument(number)
	if got, ok := numbered.Number(); !ok || got.String() != "BK-202611-00009" {
		t.Fatalf("有号读回 %q %v", got, ok)
	}
	if _, ok := numbered.Absence(); ok {
		t.Fatal("有号的单据不该同时有无号原因")
	}

	var legacy domain.DocumentNumbering
	if _, ok := legacy.Number(); ok {
		t.Fatal("零值不该有号")
	}
	if absence, ok := legacy.Absence(); !ok || absence != domain.IssuedBeforeDocumentNumbering || absence.String() != "ISSUED_BEFORE_NUMBERING" {
		t.Fatalf("零值读作本规则生效前开立：%s %v", absence, ok)
	}

	unconfigured := domain.UnnumberedDocument(domain.DocumentNumberingTimeZoneNotConfigured)
	if absence, ok := unconfigured.Absence(); !ok || absence.String() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("时区未配置读回 %s %v", absence, ok)
	}

	for _, bad := range []string{"", "  ", "BK\x00-1", strings.Repeat("9", 513)} {
		if _, err := domain.NewDocumentNumber(bad); err == nil {
			t.Fatalf("%q 不该立得起单据号", bad)
		}
	}
	if _, err := domain.NewDocumentNumber(strings.Repeat("9", 513)); !errors.Is(err, domain.ErrValueTooLongToStore) {
		t.Fatalf("超界答 %v", err)
	}
}
