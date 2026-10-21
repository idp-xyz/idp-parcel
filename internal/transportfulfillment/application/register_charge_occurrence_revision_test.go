package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证运输收费发生项登记从委托取采购上下文时，按事实的业务时间取委托那一刻所凭的那一版（ADR-0231 决定四）：起用不晚于业务
// 时间的换版管得着它，晚于它的管不着。夹具全是合成串，证据只记 S。

const (
	revisedTag      = "v3"
	revisedSnapshot = "SYN-SUPPLIER-TRUNK-CN-SG-01/v3"
	revisedLegal    = "SYN-LE-03"
)

// revisedTrunkCommission 给干线委托换一次版：自 appliesFrom 起改按 v3。
func revisedTrunkCommission(t *testing.T, appliesFrom time.Time) ports.TransportCommissionRecord {
	t.Helper()
	record := trunkCommission(t, commissionShape{startedAt: occurrenceEnteredAt})
	successor, err := domain.NewSupplierAgreementVersion(trunkAgreement, revisedTag)
	if err != nil {
		t.Fatal(err)
	}
	draft, err := record.Commission.DraftAgreementRevision(domain.CommissionAgreementRevisionSpec{
		Successor:   successor,
		AppliesFrom: appliesFrom,
		Basis:       occurrenceValue(t, domain.NewCommissionAgreementRevisionBasisReference, "SYN-DECISION-REV-TRUNK-01"),
		DecidedBy:   occurrenceValue(t, domain.NewRegistrantReference, registrant),
	})
	if err != nil {
		t.Fatalf("换版草稿：%v", err)
	}
	revised, _, err := draft.Revise(
		occurrenceValue(t, domain.NewAgreementSnapshotReference, revisedSnapshot),
		occurrenceValue(t, domain.NewProcurementLegalEntityReference, revisedLegal),
		occurrenceEndedAt.Add(24*time.Hour),
	)
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	record.Commission = revised
	return record
}

// Covers: AT-TF-299——段已采用委托、委托换了版：参与终点不早于起用时刻的，问 PC 的是后继那一版、发生项记后继的快照与法人；起用晚于
// 参与终点的照旧问提交那一版。答复带回作答的那一版在委托上的号与问的那一版；登记方报提交那一版而事实已归后继，答冲突、回显后继。
func TestAnAdoptedSegmentRegistersUnderTheRevisionInForceAtItsEnd(t *testing.T) {
	inForce := adoptedTrunkRig(t, revisedTrunkCommission(t, occurrenceEnteredAt))
	inForce.agreements.check.Snapshot = occurrenceValue(t, domain.NewAgreementSnapshotReference, revisedSnapshot)
	omitted := actualFulfillmentCommand(t)
	omitted.Provider, omitted.AgreementObject, omitted.AgreementVersion = "", "", ""
	result := inForce.register(t, omitted)
	occurrence := registeredOccurrence(t, result)
	if occurrence.Agreement().String() != revisedSnapshot || occurrence.LegalEntity().String() != revisedLegal {
		t.Fatalf("按后继形成的发生项 = %s %s", occurrence.Agreement(), occurrence.LegalEntity())
	}
	if len(inForce.agreements.asked) != 1 || inForce.agreements.asked[0] != trunkAgreement+" "+revisedTag ||
		!inForce.agreements.askedAt[0].Equal(occurrenceEndedAt) {
		t.Fatalf("问 PC = %v %v, want 后继那一版、在参与终点", inForce.agreements.asked, inForce.agreements.askedAt)
	}
	if terms, named := result.CommissionTerms(); !named || terms.Revision() != 2 {
		t.Fatalf("答复带的那一版 = %+v %v", terms, named)
	}
	if asked, present := result.AskedAgreement(); !present || asked.Version() != revisedTag {
		t.Fatalf("答复带的所问那一版 = %+v %v", asked, present)
	}

	later := adoptedTrunkRig(t, revisedTrunkCommission(t, occurrenceEndedAt.Add(time.Second)))
	notYet := later.register(t, omitted)
	occurrence = registeredOccurrence(t, notYet)
	if occurrence.Agreement().String() != trunkSnapshot || later.agreements.asked[0] != trunkAgreement+" "+trunkAgreementTag {
		t.Fatalf("起用晚于参与终点时按 %s 问、记 %s, want 提交那一版", later.agreements.asked, occurrence.Agreement())
	}
	if terms, _ := notYet.CommissionTerms(); terms.Revision() != 1 {
		t.Fatalf("起用晚于参与终点时作答的是第 %d 版", terms.Revision())
	}

	stale := adoptedTrunkRig(t, revisedTrunkCommission(t, occurrenceEnteredAt))
	conflict := stale.register(t, actualFulfillmentCommand(t))
	if conflict.Outcome() != application.ChargeOccurrenceConflict || conflict.Detail() != application.ProcurementContextConflict {
		t.Fatalf("报提交那一版而事实已归后继 = %s/%s", conflict.Outcome(), conflict.Detail())
	}
	if terms, named := conflict.CommissionTerms(); !named || terms.Revision() != 2 {
		t.Fatalf("冲突回显的那一版 = %+v", terms)
	}
	assertNothingWritten(t, stale)
}

// Covers: AT-TF-299——订舱发生项同一判法：按接受那一刻取委托所凭的那一版。换版起用晚于接受，仍按提交那一版；起用不晚于接受，按后继。
func TestABookingOccurrenceRegistersUnderTheRevisionInForceAtAcceptance(t *testing.T) {
	booking := trunkBooking(t, bkRequestedAt, time.Time{})
	answer := trunkAnswer(t, booking, domain.BookingAccepted, bkAcceptedAt)

	after := bookingRig(t, revisedTrunkCommission(t, bkAcceptedAt.Add(time.Minute)), booking, &answer)
	registeredOccurrence(t, after.register(t, bookingCommand(t, domain.BookingOccurrence)))
	if after.agreements.asked[0] != trunkAgreement+" "+trunkAgreementTag {
		t.Fatalf("起用晚于接受时问了 %s", after.agreements.asked[0])
	}

	before := bookingRig(t, revisedTrunkCommission(t, bkAcceptedAt), booking, &answer)
	before.agreements.check.Snapshot = occurrenceValue(t, domain.NewAgreementSnapshotReference, revisedSnapshot)
	occurrence := registeredOccurrence(t, before.register(t, bookingCommand(t, domain.BookingOccurrence)))
	if before.agreements.asked[0] != trunkAgreement+" "+revisedTag || occurrence.LegalEntity().String() != revisedLegal {
		t.Fatalf("起用不晚于接受时问了 %s、记法人 %s", before.agreements.asked[0], occurrence.LegalEntity())
	}
}
