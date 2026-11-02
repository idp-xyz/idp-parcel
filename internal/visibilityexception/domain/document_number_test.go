package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

func documentNumber(t *testing.T, raw string) domain.DocumentNumber {
	t.Helper()
	number, err := domain.NewDocumentNumber(raw)
	if err != nil {
		t.Fatalf("document number %q: %v", raw, err)
	}
	return number
}

// Covers: GLOSSARY「单据号」——没有单据号的单据记下是哪一种情形：库里两格都空是上线前开立，「单据编号时区未配置」落值，
// 有号不带原因；读回的无号原因只认那一个值，号与原因同在、空白号都拒。
func TestADocumentNumberCellRebuildsItsThreeShapesAndRefusesTheRest(t *testing.T) {
	legacy, err := domain.RehydrateDocumentNumber("", "")
	if err != nil {
		t.Fatalf("legacy: %v", err)
	}
	if _, numbered := legacy.Number(); numbered || legacy.NumberlessReason() != domain.IssuedBeforeNumbering || legacy.StoredNumberlessReason() != "" {
		t.Fatalf("两格都空应读成上线前开立、落库仍是空：%+v", legacy)
	}

	unconfigured, err := domain.RehydrateDocumentNumber("", "NUMBERING_TIME_ZONE_NOT_CONFIGURED")
	if err != nil {
		t.Fatalf("unconfigured: %v", err)
	}
	if unconfigured.NumberlessReason() != domain.NumberingTimeZoneNotConfigured ||
		unconfigured.StoredNumberlessReason() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" ||
		domain.NumberlessForUnconfiguredTimeZone() != unconfigured {
		t.Fatalf("时区未配置那一格读回不对：%+v", unconfigured)
	}

	assigned, err := domain.RehydrateDocumentNumber("EX-20261102-0003", "")
	if err != nil {
		t.Fatalf("assigned: %v", err)
	}
	if number, numbered := assigned.Number(); !numbered || number != "EX-20261102-0003" || assigned.NumberlessReason() != "" || assigned.StoredNumberlessReason() != "" {
		t.Fatalf("有号那一格读回不对：%+v", assigned)
	}

	for _, broken := range []struct{ number, reason string }{
		{"EX-20261102-0003", "NUMBERING_TIME_ZONE_NOT_CONFIGURED"},
		{"", "ISSUED_BEFORE_NUMBERING"},
		{"", "SOMETHING_ELSE"},
		{"   ", ""},
	} {
		if _, err := domain.RehydrateDocumentNumber(broken.number, broken.reason); !errors.Is(err, domain.ErrInvalidDocumentNumber) {
			t.Errorf("(%q, %q) 读回成立了：%v", broken.number, broken.reason, err)
		}
	}
	if _, err := domain.NewDocumentNumber(" "); !errors.Is(err, domain.ErrInvalidDocumentNumber) {
		t.Fatalf("空白号被收下了：%v", err)
	}
}

// Covers: CONTEXT「异常案件」——建立案件时取得案件单据号：新建案件带着号或「单据编号时区未配置」，不能带着「上线前开立」的
// 零值建；接单、关闭、归并都不换号，归并不改主案件与被归并案件各自的号。
func TestACaseIsEstablishedWithItsNumberAndKeepsItThroughItsLifecycle(t *testing.T) {
	establish := func(id string, number domain.DocumentNumber) (*domain.ExceptionCase, error) {
		return domain.EstablishCase(
			mustValue(t, domain.NewCaseID, id),
			number,
			mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
			mustValue(t, domain.NewImpactScopeReference, "parcel-1"),
			mustValue(t, domain.NewResponsibleTeamReference, "ops-team-1"),
			firstHitAt,
		)
	}
	if _, err := establish("case-0", domain.DocumentNumber{}); !errors.Is(err, domain.ErrInvalidCase) {
		t.Fatalf("零值单据号建起了案件：%v", err)
	}

	main, err := establish("case-1", documentNumber(t, "EX-20260811-0001"))
	if err != nil {
		t.Fatal(err)
	}
	duplicate, err := establish("case-2", domain.NumberlessForUnconfiguredTimeZone())
	if err != nil {
		t.Fatal(err)
	}
	if err := main.TakeUp(firstHitAt.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := duplicate.MergeInto(main.ID(), firstHitAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if err := main.Close("RECOVERED", firstHitAt.Add(2*time.Hour)); err != nil {
		t.Fatal(err)
	}
	if number, numbered := main.DocumentNumber().Number(); !numbered || number != "EX-20260811-0001" || main.Snapshot().DocumentNumber != main.DocumentNumber() {
		t.Fatalf("主案件的号变了：%+v", main.DocumentNumber())
	}
	if duplicate.DocumentNumber().NumberlessReason() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("被归并案件的无号原因变了：%+v", duplicate.DocumentNumber())
	}
}

// Covers: CONTEXT「客户索赔项」——记录提交时取得索赔单据号，客户自己的项标识与批次号照旧在：新受理的索赔不能带着「上线前
// 开立」的零值受理；快照往返带着号；上线前受理的存量行（零值）照旧重建，不补号。
func TestAClaimCarriesItsNumberNextToTheCustomersOwnIdentifiers(t *testing.T) {
	claim := receivedClaim(t)
	if number, numbered := claim.DocumentNumber().Number(); !numbered || number != "CLM-202608-00001" {
		t.Fatalf("number = %q numbered = %v", number, numbered)
	}
	if claim.ID().String() != "claim-1" || claim.Batch().String() != "batch-1" {
		t.Fatal("客户自己的项标识与批次号被取代了")
	}

	spec := domain.ClaimItemSpec{
		ID:          mustValue(t, domain.NewClaimItemID, "claim-9"),
		Batch:       mustValue(t, domain.NewClaimBatchReference, "batch-1"),
		Customer:    mustValue(t, domain.NewCustomerAccountReference, "customer-1"),
		Applicant:   mustValue(t, domain.NewApplicantReference, "applicant-1"),
		Contract:    mustValue(t, domain.NewContractScopeReference, "contract-1/liability"),
		Target:      mustValue(t, domain.NewRequestScopeReference, "parcel-9"),
		Kind:        mustValue(t, domain.NewClaimKindReference, "DAMAGE"),
		SubmittedAt: claimSubmittedAt,
	}
	if _, err := domain.ReceiveClaimItem(spec); !errors.Is(err, domain.ErrInvalidClaim) {
		t.Fatalf("零值单据号受理了索赔：%v", err)
	}
	spec.DocumentNumber = domain.NumberlessForUnconfiguredTimeZone()
	unnumbered, err := domain.ReceiveClaimItem(spec)
	if err != nil {
		t.Fatalf("时区未配置的索赔照常受理：%v", err)
	}
	if unnumbered.DocumentNumber().NumberlessReason() != domain.NumberingTimeZoneNotConfigured {
		t.Fatalf("无号原因 = %q", unnumbered.DocumentNumber().NumberlessReason())
	}

	snapshot := claim.Snapshot()
	snapshot.Revision = 1
	rebuilt, err := domain.RehydrateClaimItem(snapshot)
	if err != nil || rebuilt.DocumentNumber() != claim.DocumentNumber() {
		t.Fatalf("快照往返丢了号：%v %+v", err, rebuilt)
	}
	snapshot.DocumentNumber = domain.DocumentNumber{}
	legacy, err := domain.RehydrateClaimItem(snapshot)
	if err != nil || legacy.DocumentNumber().NumberlessReason() != domain.IssuedBeforeNumbering {
		t.Fatalf("存量行重建不了或被补了号：%v", err)
	}
}
