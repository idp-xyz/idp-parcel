package tfhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

type adoptedSegment struct {
	record ports.SegmentCommissionAdoptionRecord
}

func (view adoptedSegment) FindBySegment(context.Context, ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	return view.record, true, nil
}

type commissionOf struct {
	record ports.TransportCommissionRecord
}

func (view commissionOf) FindByKey(context.Context, ports.TransportCommissionKey) (ports.TransportCommissionRecord, bool, error) {
	return view.record, true, nil
}

// conflictingAdoptedRegistration 走真编排拿一份「采购上下文与委托不一致」的冲突：干线段采用了一份指名 v2 的委托，登记方报的是另一家
// 服务方。revised 为真时委托自参与入场起换到 v3。
func conflictingAdoptedRegistration(t *testing.T, revised bool) application.RegisterChargeOccurrenceResult {
	t.Helper()
	tenant := mustTF(t, domain.NewTenantID, operatorTenant)
	segmentRef := mustTF(t, domain.NewFulfillmentSegmentReference, "SYN-SEG-SZX-SIN-01")
	enteredAt := time.Date(2026, 10, 19, 1, 0, 0, 0, time.UTC)
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  segmentRef,
		Participations: []domain.RehydrateParticipationSpec{{
			Object:     mustTF(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01"),
			EntryKind:  domain.EnteredByTransportHandover,
			EntryBasis: mustTF(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX"),
			EnteredAt:  enteredAt,
			EndKind:    domain.EndedByNextHandover,
			EndBasis:   mustTF(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SIN"),
			EndedAt:    time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC),
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	version, err := domain.NewSupplierAgreementVersion("SYN-SUPPLIER-TRUNK-CN-SG-01", "v2")
	if err != nil {
		t.Fatal(err)
	}
	commission, err := domain.RehydrateTransportCommission(domain.RehydrateTransportCommissionSpec{
		TenantID:         tenant,
		Commission:       mustTF(t, domain.NewTransportCommissionReference, "SYN-COMMISSION-TRUNK-01"),
		Provider:         mustTF(t, domain.NewServiceProviderReference, "SYN-PARTY-CARRIER-TRUNK-01"),
		Agreement:        mustTF(t, domain.NewAgreementSnapshotReference, "SYN-SUPPLIER-TRUNK-CN-SG-01/v2"),
		LegalEntity:      mustTF(t, domain.NewProcurementLegalEntityReference, "SYN-LE-01"),
		Conditions:       mustTF(t, domain.NewConditionsSnapshotReference, "SYN-COND-01"),
		Role:             mustTF(t, domain.NewRoleSnapshotReference, "SYN-ROLE-01"),
		Responsibility:   mustTF(t, domain.NewResponsibilitySnapshotReference, "SYN-RESP-01"),
		Members:          []domain.CarriedObjectReference{mustTF(t, domain.NewCarriedObjectReference, "SYN-PARCEL-01")},
		SubmittedAt:      time.Date(2026, 10, 18, 0, 0, 0, 0, time.UTC),
		AgreementVersion: version,
	})
	if err != nil {
		t.Fatal(err)
	}
	if revised {
		successor, _ := domain.NewSupplierAgreementVersion("SYN-SUPPLIER-TRUNK-CN-SG-01", "v3")
		draft, err := commission.DraftAgreementRevision(domain.CommissionAgreementRevisionSpec{
			Successor: successor, AppliesFrom: enteredAt,
			Basis:     mustTF(t, domain.NewCommissionAgreementRevisionBasisReference, "SYN-DECISION-REV-01"),
			DecidedBy: mustTF(t, domain.NewRegistrantReference, operatorSubject),
		})
		if err != nil {
			t.Fatal(err)
		}
		if commission, _, err = draft.Revise(mustTF(t, domain.NewAgreementSnapshotReference, "SYN-SUPPLIER-TRUNK-CN-SG-01/v3"),
			mustTF(t, domain.NewProcurementLegalEntityReference, "SYN-LE-03"), enteredAt.Add(48*time.Hour)); err != nil {
			t.Fatal(err)
		}
	}
	adoption, err := domain.RehydrateSegmentCommissionAdoption(domain.RehydrateSegmentCommissionAdoptionSpec{
		TenantID: tenant, Segment: segmentRef, Commission: commission.Commission(),
		Provider:   commission.Provider(),
		StartBasis: mustTF(t, domain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX"),
		StartedAt:  enteredAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	handler := application.NewRegisterChargeOccurrenceHandler(application.RegisterChargeOccurrenceDeps{
		Segments:    segmentSource{record: ports.FulfillmentSegmentRecord{Segment: segment}},
		Adoptions:   adoptedSegment{record: ports.SegmentCommissionAdoptionRecord{Adoption: adoption}},
		Commissions: commissionOf{record: ports.TransportCommissionRecord{Commission: commission}},
		Occurrences: &emptyOccurrences{},
		Handoff:     discardHandoff{},
		Clock:       occurrenceRegistrationClock{at: time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)},
	})
	result, err := handler.Register(context.Background(), application.RegisterChargeOccurrenceCommand{
		TenantID: tenant, Registrant: operatorSubject, Reason: domain.ActualFulfillmentOccurrence,
		Segment: segmentRef.String(), Objects: []string{"SYN-PARCEL-01"}, Journey: "JRN/SYN-PARCEL-01",
		Provider: "SYN-PARTY-CARRIER-LM-01",
	})
	if err != nil || result.Detail() != application.ProcurementContextConflict {
		t.Fatalf("夹具冲突 = %s/%s %v", result.Outcome(), result.Detail(), err)
	}
	return result
}

// Covers: AT-TF-299 的线格式那一半（ADR-0231 决定四）——采购上下文冲突回显委托：没换过版时与换版之前逐字节相同，不出
// `agreementRevision`、协议三格是提交那一版；换过版后回显业务时间所凭的那一版，`agreementRevision` 是 2。
func TestTheConflictEchoNamesARevisionOnlyWhenTheCommissionWasRevised(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	echoed := func(revised bool) map[string]json.RawMessage {
		t.Helper()
		endpoint := tfhttp.NewRegisterChargeOccurrenceEndpoint(intake, registrarDouble{result: conflictingAdoptedRegistration(t, revised)})
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, bearerRequest("t", decisionChargeOccurrenceBody))
		if recorder.Code != http.StatusOK {
			t.Fatalf("status = %d：%s", recorder.Code, recorder.Body.String())
		}
		var body struct {
			Commission map[string]json.RawMessage `json:"commission"`
		}
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatal(err)
		}
		return body.Commission
	}

	plain := echoed(false)
	if _, present := plain["agreementRevision"]; present {
		t.Fatalf("没换过版的委托回显了 agreementRevision：%v", plain)
	}
	if string(plain["agreementVersion"]) != `"v2"` || string(plain["agreement"]) != `"SYN-SUPPLIER-TRUNK-CN-SG-01/v2"` ||
		string(plain["legalEntity"]) != `"SYN-LE-01"` || len(plain) != 6 {
		t.Fatalf("没换过版的回显 = %v", plain)
	}

	revised := echoed(true)
	if string(revised["agreementRevision"]) != "2" || string(revised["agreementVersion"]) != `"v3"` ||
		string(revised["agreement"]) != `"SYN-SUPPLIER-TRUNK-CN-SG-01/v3"` || string(revised["legalEntity"]) != `"SYN-LE-03"` {
		t.Fatalf("换过版的回显 = %v", revised)
	}
}
