package tfhttp_test

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ---- 采用口与两个取消口的端口替身 ----

// transitionCommissionStore 是能落开始与取消的委托册替身：Replace 只作用于既未开始也未取消的那一份。
type transitionCommissionStore struct {
	commissionRegistryDouble
}

func (double *transitionCommissionStore) Replace(_ context.Context, record ports.TransportCommissionRecord) (bool, error) {
	storeKey := record.Key.TenantID.String() + "|" + record.Key.Commission.String()
	stored, exists := double.records[storeKey]
	if !exists {
		return false, nil
	}
	if _, _, started := stored.Commission.TransportStarted(); started {
		return false, nil
	}
	if _, cancelled := stored.Commission.Cancelled(); cancelled {
		return false, nil
	}
	double.records[storeKey] = record
	return true, nil
}

// cancellableBookingStore 是能落取消的订舱册替身。
type cancellableBookingStore struct {
	bookingRegistryDouble
}

func (double *cancellableBookingStore) Replace(_ context.Context, record ports.BookingRecord) (bool, error) {
	storeKey := record.Key.TenantID.String() + "|" + record.Key.Booking.String()
	stored, exists := double.records[storeKey]
	if !exists {
		return false, nil
	}
	if _, cancelled := stored.Booking.Cancelled(); cancelled {
		return false, nil
	}
	double.records[storeKey] = record
	return true, nil
}

type adoptionRegisterDouble struct {
	records map[string]ports.SegmentCommissionAdoptionRecord
}

func (double *adoptionRegisterDouble) FindBySegment(_ context.Context, key ports.SegmentCommissionAdoptionKey) (ports.SegmentCommissionAdoptionRecord, bool, error) {
	record, found := double.records[key.TenantID.String()+"|"+key.Segment.String()]
	return record, found, nil
}

func (double *adoptionRegisterDouble) Save(_ context.Context, record ports.SegmentCommissionAdoptionRecord) (ports.AdoptionSaveOutcome, error) {
	storeKey := record.Key.TenantID.String() + "|" + record.Key.Segment.String()
	if _, exists := double.records[storeKey]; exists {
		return ports.AdoptionAlreadyRegistered, nil
	}
	double.records[storeKey] = record
	return ports.AdoptionSaved, nil
}

// trunkSegmentView 只认一个段：SYN-SEGMENT-TRUNK-01，由 SZX 交给干线承运商立起来，首个参与在委托提交之后。
type trunkSegmentView struct {
	segment domain.ActualFulfillmentSegment
}

func (view trunkSegmentView) FindByKey(_ context.Context, key ports.FulfillmentSegmentKey) (ports.FulfillmentSegmentRecord, bool, error) {
	if key.Segment != view.segment.Segment() || key.TenantID != view.segment.TenantID() {
		return ports.FulfillmentSegmentRecord{}, false, nil
	}
	return ports.FulfillmentSegmentRecord{Key: key, Segment: view.segment}, true, nil
}

var trunkSegmentEnteredAt = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

const (
	segmentCommissionAdoptionBody = `{"segment": "SYN-SEGMENT-TRUNK-01", "commission": "SYN-COMMISSION-01", "provider": "SYN-PARTY-CARRIER-TRUNK-01"}`
	commissionCancellationBody    = `{"commission": "SYN-COMMISSION-01", "cancelledAt": "2026-10-19T12:00:00Z"}`
	bookingCancellationBody       = `{"booking": "SYN-BOOKING-01", "cancelledAt": "2026-10-19T12:00:00Z"}`
)

type adoptionFaces struct {
	submit, booking, adopt, cancelCommission, cancelBooking http.Handler
}

func newAdoptionFaces(t *testing.T) adoptionFaces {
	t.Helper()
	tenant, err := domain.NewTenantID(operatorTenant)
	if err != nil {
		t.Fatal(err)
	}
	segmentRef, _ := domain.NewFulfillmentSegmentReference("SYN-SEGMENT-TRUNK-01")
	object, _ := domain.NewCarriedObjectReference("SYN-PARCEL-01")
	basis, _ := domain.NewParticipationBasisReference("TRANSPORT-HANDOVER/SYN-HANDOVER-SZX-01/v1")
	segment, err := domain.RehydrateActualFulfillmentSegment(domain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: tenant,
		Segment:  segmentRef,
		Participations: []domain.RehydrateParticipationSpec{{
			Object: object, EntryKind: domain.EnteredByTransportHandover, EntryBasis: basis, EnteredAt: trunkSegmentEnteredAt,
		}},
	})
	if err != nil {
		t.Fatal(err)
	}
	commissions := &transitionCommissionStore{commissionRegistryDouble{records: map[string]ports.TransportCommissionRecord{}}}
	bookings := &cancellableBookingStore{bookingRegistryDouble{records: map[string]ports.BookingRecord{}}}
	handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions:          commissions,
		Bookings:             bookings,
		BookingCancellations: bookings,
		Schedules:            scheduleRegisterStub{registered: trunkSchedule},
		Agreements:           agreementCheckStub{supplier: trunkCarrier},
		Downstream:           commissionHandoffStub{},
		Clock:                fixedCommissionClock{},
	})
	adopter := application.NewAdoptSegmentCommissionHandler(application.AdoptSegmentCommissionDeps{
		Adoptions:   &adoptionRegisterDouble{records: map[string]ports.SegmentCommissionAdoptionRecord{}},
		Segments:    trunkSegmentView{segment: segment},
		Commissions: commissions,
		Clock:       fixedCommissionClock{},
	})
	intake := operatorIntake(t, authenticatorFake{})
	return adoptionFaces{
		submit:           tfhttp.NewSubmitTransportCommissionEndpoint(intake, handler),
		booking:          tfhttp.NewSubmitBookingRequestEndpoint(intake, handler),
		adopt:            tfhttp.NewAdoptSegmentCommissionEndpoint(intake, adopter),
		cancelCommission: tfhttp.NewCancelTransportCommissionEndpoint(intake, handler),
		cancelBooking:    tfhttp.NewCancelBookingRequestEndpoint(intake, handler),
	}
}

const (
	adoptionPath               = "/transport-fulfillment-segment-commission-adoptions"
	commissionCancellationPath = "/transport-fulfillment-transport-commission-cancellations"
	bookingCancellationPath    = "/transport-fulfillment-booking-request-cancellations"
)

// Covers: `AT-TF-181`、`AT-TF-183`（ADR-0204）的线格式，票 demo-chain-gaps/25 演示判据（S）——干线段采用干线委托答 201，回显这
// 一段的开始依据与委托此刻的开始；随后取消委托答 200 TRANSPORT_ALREADY_STARTED、回显开始依据；开始之前提出的订舱取消同样
// 答已开始；原样再采用 200 已有采用；服务方不符 200 带委托的服务提供方；段不在册 200 SEGMENT_NOT_FOUND、不回显采用。
func TestTheAdoptionFaceStartsTheCommissionAndTheCancellationFaceThenAnswersAlreadyStarted(t *testing.T) {
	faces := newAdoptionFaces(t)
	const token = "presented.operator.token"
	if status, answer := postBookingFace(t, faces.submit, "/transport-fulfillment-transport-commission-submissions", token, trunkCommissionBody); status != http.StatusCreated {
		t.Fatalf("委托 = %d %v", status, answer)
	}
	if status, answer := postBookingFace(t, faces.booking, "/transport-fulfillment-booking-request-submissions", token, bookingRequestBody); status != http.StatusCreated {
		t.Fatalf("订舱 = %d %v", status, answer)
	}

	status, answer := postBookingFace(t, faces.adopt, adoptionPath, token, strings.Replace(segmentCommissionAdoptionBody, trunkCarrier, "SYN-PARTY-CARRIER-LM-01", 1))
	if status != http.StatusOK || answer["outcome"] != "PROVIDER_MISMATCH" || answer["commissionProvider"] != trunkCarrier || answer["adoption"] != nil {
		t.Fatalf("服务方不符 = %d %v", status, answer)
	}
	status, answer = postBookingFace(t, faces.adopt, adoptionPath, token, strings.Replace(segmentCommissionAdoptionBody, "SYN-SEGMENT-TRUNK-01", "SYN-SEGMENT-NOWHERE", 1))
	if status != http.StatusOK || answer["outcome"] != "SEGMENT_NOT_FOUND" || answer["adoption"] != nil {
		t.Fatalf("段不在册 = %d %v", status, answer)
	}

	status, answer = postBookingFace(t, faces.adopt, adoptionPath, token, segmentCommissionAdoptionBody)
	if status != http.StatusCreated || answer["outcome"] != "COMMISSION_ADOPTED" {
		t.Fatalf("采用 = %d %v", status, answer)
	}
	adoption, _ := answer["adoption"].(map[string]any)
	commission, _ := answer["commission"].(map[string]any)
	if adoption["startBasis"] != "TRANSPORT-HANDOVER/SYN-HANDOVER-SZX-01/v1" || adoption["startedAt"] != "2026-10-19T10:00:00Z" ||
		commission["startedAt"] != "2026-10-19T10:00:00Z" || commission["startedBasis"] != "TRANSPORT-HANDOVER/SYN-HANDOVER-SZX-01/v1" {
		t.Fatalf("回显的采用 %v、委托 %v", adoption, commission)
	}
	if status, answer := postBookingFace(t, faces.adopt, adoptionPath, token, segmentCommissionAdoptionBody); status != http.StatusOK || answer["outcome"] != "EXISTING_ADOPTION" {
		t.Fatalf("原样再采用 = %d %v", status, answer)
	}

	status, answer = postBookingFace(t, faces.cancelCommission, commissionCancellationPath, token, commissionCancellationBody)
	commission, _ = answer["commission"].(map[string]any)
	if status != http.StatusOK || answer["outcome"] != "TRANSPORT_ALREADY_STARTED" || commission["startedAt"] != "2026-10-19T10:00:00Z" || commission["cancelledAt"] != nil {
		t.Fatalf("采用之后取消委托 = %d %v", status, answer)
	}
	status, answer = postBookingFace(t, faces.cancelBooking, bookingCancellationPath, token, bookingCancellationBody)
	booking, _ := answer["booking"].(map[string]any)
	if status != http.StatusOK || answer["outcome"] != "TRANSPORT_ALREADY_STARTED" || booking["cancelledAt"] != nil {
		t.Fatalf("开始之前提出的订舱取消 = %d %v", status, answer)
	}
}

// Covers: `AT-TF-184`、`AT-TF-185`（ADR-0204 决定三）的线格式——未开始的委托与订舱可以取消，各答 201 并回显取消时刻；重放答 200
// 原取消；委托不在册 200 COMMISSION_NOT_FOUND、订舱不在册 200 BOOKING_NOT_FOUND；取消之后再采用答 200
// COMMISSION_NOT_ADOPTABLE；时刻写坏、自报租户是坏报文（400）。
func TestTheCancellationFacesStopUnstartedIntents(t *testing.T) {
	faces := newAdoptionFaces(t)
	const token = "presented.operator.token"
	if status, answer := postBookingFace(t, faces.submit, "/transport-fulfillment-transport-commission-submissions", token, trunkCommissionBody); status != http.StatusCreated {
		t.Fatalf("委托 = %d %v", status, answer)
	}
	if status, answer := postBookingFace(t, faces.booking, "/transport-fulfillment-booking-request-submissions", token, bookingRequestBody); status != http.StatusCreated {
		t.Fatalf("订舱 = %d %v", status, answer)
	}

	status, answer := postBookingFace(t, faces.cancelBooking, bookingCancellationPath, token, bookingCancellationBody)
	booking, _ := answer["booking"].(map[string]any)
	if status != http.StatusCreated || answer["outcome"] != "BOOKING_CANCELLED" || booking["cancelledAt"] != "2026-10-19T12:00:00Z" {
		t.Fatalf("取消订舱 = %d %v", status, answer)
	}
	if status, answer := postBookingFace(t, faces.cancelBooking, bookingCancellationPath, token, bookingCancellationBody); status != http.StatusOK || answer["outcome"] != "EXISTING_BOOKING" {
		t.Fatalf("再取消订舱 = %d %v", status, answer)
	}
	status, answer = postBookingFace(t, faces.cancelCommission, commissionCancellationPath, token, commissionCancellationBody)
	commission, _ := answer["commission"].(map[string]any)
	if status != http.StatusCreated || answer["outcome"] != "COMMISSION_CANCELLED" || commission["cancelledAt"] != "2026-10-19T12:00:00Z" {
		t.Fatalf("取消委托 = %d %v", status, answer)
	}
	if status, answer := postBookingFace(t, faces.cancelCommission, commissionCancellationPath, token, commissionCancellationBody); status != http.StatusOK || answer["outcome"] != "EXISTING_COMMISSION" {
		t.Fatalf("再取消委托 = %d %v", status, answer)
	}
	if status, answer := postBookingFace(t, faces.adopt, adoptionPath, token, segmentCommissionAdoptionBody); status != http.StatusOK || answer["outcome"] != "COMMISSION_NOT_ADOPTABLE" {
		t.Fatalf("取消之后采用 = %d %v", status, answer)
	}

	for name, testCase := range map[string]struct {
		endpoint http.Handler
		path     string
		body     string
		want     string
	}{
		"委托不在册": {faces.cancelCommission, commissionCancellationPath, strings.Replace(commissionCancellationBody, "SYN-COMMISSION-01", "SYN-COMMISSION-99", 1), "COMMISSION_NOT_FOUND"},
		"订舱不在册": {faces.cancelBooking, bookingCancellationPath, strings.Replace(bookingCancellationBody, "SYN-BOOKING-01", "SYN-BOOKING-99", 1), "BOOKING_NOT_FOUND"},
	} {
		if status, answer := postBookingFace(t, testCase.endpoint, testCase.path, token, testCase.body); status != http.StatusOK || answer["outcome"] != testCase.want {
			t.Fatalf("%s = %d %v, want 200 %s", name, status, answer, testCase.want)
		}
	}
	for name, testCase := range map[string]struct {
		endpoint http.Handler
		path     string
		body     string
	}{
		"取消委托时刻写坏": {faces.cancelCommission, commissionCancellationPath, strings.Replace(commissionCancellationBody, "2026-10-19T12:00:00Z", "noon", 1)},
		"取消订舱自报租户": {faces.cancelBooking, bookingCancellationPath, strings.Replace(bookingCancellationBody, `{"booking"`, `{"tenantId": "SYN-TENANT-02", "booking"`, 1)},
		"采用自带开始时刻": {faces.adopt, adoptionPath, strings.Replace(segmentCommissionAdoptionBody, `{"segment"`, `{"startedAt": "2026-10-19T09:00:00Z", "segment"`, 1)},
	} {
		status, answer := postBookingFace(t, testCase.endpoint, testCase.path, token, testCase.body)
		problem, _ := answer["error"].(map[string]any)
		if status != http.StatusBadRequest || problem["code"] != "MALFORMED_REQUEST" {
			t.Fatalf("%s = %d %v, want 400 MALFORMED_REQUEST", name, status, answer)
		}
	}
}
