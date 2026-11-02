package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func numberedShipment(t *testing.T, raw string) domain.ShipmentDocumentNumbering {
	t.Helper()
	number, err := domain.NewShipmentDocumentNumber(raw)
	if err != nil {
		t.Fatalf("document number: %v", err)
	}
	numbering, err := domain.NumberedShipment(number)
	if err != nil {
		t.Fatalf("numbering: %v", err)
	}
	return numbering
}

// Covers: PS CONTEXT「委托」草稿 → 已提交时取得单据号；GLOSSARY「单据号」两种无号情形各记一格；ADR-0355 决定三
// 「开立后不变、事后不补号」——刚建成的委托这一格是空的，记一次之后再记就拒，读回的上线前委托也拒。
func TestShipmentDocumentNumberingIsRecordedOnceOnTheFreshRequest(t *testing.T) {
	fresh := submitted(t)
	if fresh.DocumentNumbering().Recorded() {
		t.Fatal("a freshly submitted request already carries document numbering")
	}

	numbered, err := fresh.RecordDocumentNumbering(numberedShipment(t, "P20261102000001"))
	if err != nil {
		t.Fatalf("record number: %v", err)
	}
	number, present := numbered.DocumentNumbering().Number()
	if !present || number.String() != "P20261102000001" {
		t.Fatalf("number = %q, %v", number, present)
	}
	if _, absent := numbered.DocumentNumbering().Absence(); absent {
		t.Fatal("a numbered request also reports an absence")
	}
	if _, err := numbered.RecordDocumentNumbering(numberedShipment(t, "P20261102000002")); !errors.Is(err, domain.ErrShipmentDocumentNumberingAlreadyRecorded) {
		t.Fatalf("second record: %v", err)
	}

	unconfigured, err := domain.UnnumberedShipment(domain.DocumentNumberingTimeZoneNotConfigured)
	if err != nil {
		t.Fatalf("absence: %v", err)
	}
	absent, err := fresh.RecordDocumentNumbering(unconfigured)
	if err != nil {
		t.Fatalf("record absence: %v", err)
	}
	if reason, present := absent.DocumentNumbering().Absence(); !present || reason.String() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("absence = %v, %v", reason, present)
	}
	if _, numbered := absent.DocumentNumbering().Number(); numbered {
		t.Fatal("an unnumbered request reports a number")
	}
	// 时区事后补登也不补号：已记了无号原因的委托再记一个号同样拒。
	if _, err := absent.RecordDocumentNumbering(numberedShipment(t, "P20261102000003")); !errors.Is(err, domain.ErrShipmentDocumentNumberingAlreadyRecorded) {
		t.Fatalf("number after absence: %v", err)
	}

	before, err := domain.UnnumberedShipment(domain.IssuedBeforeDocumentNumbering)
	if err != nil {
		t.Fatalf("issued before numbering: %v", err)
	}
	snapshot := submittedSnapshot(t)
	snapshot.DocumentNumbering = before
	legacy, err := domain.RehydrateShipmentRequest(snapshot)
	if err != nil {
		t.Fatalf("rehydrate legacy: %v", err)
	}
	if reason, _ := legacy.DocumentNumbering().Absence(); reason != domain.IssuedBeforeDocumentNumbering {
		t.Fatalf("legacy absence = %v", reason)
	}
	if _, err := legacy.RecordDocumentNumbering(numberedShipment(t, "P20261102000004")); !errors.Is(err, domain.ErrShipmentDocumentNumberingAlreadyRecorded) {
		t.Fatalf("a request issued before numbering got a number afterwards: %v", err)
	}
}

// Covers: 单据号这一格立不起来的三种——空号、零值原因、什么也不记。
func TestShipmentDocumentNumberingRefusesAnEmptyCell(t *testing.T) {
	if _, err := domain.NewShipmentDocumentNumber("  "); err == nil {
		t.Fatal("a blank document number was accepted")
	}
	if _, err := domain.NumberedShipment(domain.ShipmentDocumentNumber{}); !errors.Is(err, domain.ErrInvalidShipmentDocumentNumbering) {
		t.Fatalf("zero number: %v", err)
	}
	if _, err := domain.UnnumberedShipment(domain.DocumentNumberAbsenceInvalid); !errors.Is(err, domain.ErrInvalidShipmentDocumentNumbering) {
		t.Fatalf("zero absence: %v", err)
	}
	if _, err := submitted(t).RecordDocumentNumbering(domain.ShipmentDocumentNumbering{}); !errors.Is(err, domain.ErrInvalidShipmentDocumentNumbering) {
		t.Fatalf("nothing recorded: %v", err)
	}
}

// Covers: 号只在草稿 → 已提交那一刻开立——已离开`已提交`的委托不接受这一格（读回的这类委托由仓储标为上线前开立，
// 这里钉的是不经库的那一路也挡得住）。
func TestShipmentDocumentNumberingIsOnlyRecordedWhileSubmitted(t *testing.T) {
	accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if _, err := accepted.RecordDocumentNumbering(numberedShipment(t, "P20261102000005")); !errors.Is(err, domain.ErrInvalidShipmentDocumentNumbering) {
		t.Fatalf("accepted request: %v", err)
	}
}
