package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// documentNumberStub 按类型逐个出号；outcome 给了就直接答那一格。线格式这一层只看号与原因怎样写上答复，取号的跳号与撞号由
// 编排与真库用例钉。
type documentNumberStub struct {
	outcome ports.DocumentNumberIssueOutcome
	issued  map[string]int
}

func (stub *documentNumberStub) IssueDocumentNumber(_ context.Context, request ports.DocumentNumberRequest) (ports.DocumentNumberIssue, error) {
	if stub.outcome != ports.DocumentNumberIssueInvalid {
		return ports.DocumentNumberIssue{Outcome: stub.outcome}, nil
	}
	if stub.issued == nil {
		stub.issued = map[string]int{}
	}
	stub.issued[string(request.Type)]++
	prefix := map[string]string{"TRANSPORT_COMMISSION": "TC", "BOOKING": "BK"}[string(request.Type)]
	number, err := domain.NewDocumentNumber(fmt.Sprintf("%s-202610-%05d", prefix, stub.issued[string(request.Type)]))
	return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberIssued, Number: number}, err
}

func (double *commissionRegistryDouble) CommissionDocumentNumberUsed(context.Context, domain.TenantID, domain.DocumentNumber) (bool, error) {
	return false, nil
}

func (double *commissionRegistryDouble) RecordCommissionDocumentNumber(_ context.Context, key ports.TransportCommissionKey, numbering domain.DocumentNumbering) error {
	storeKey := key.TenantID.String() + "|" + key.Commission.String()
	record, found := double.records[storeKey]
	if !found {
		return errors.New("commission not saved")
	}
	record.DocumentNumber = numbering
	double.records[storeKey] = record
	return nil
}

func (double *bookingRegistryDouble) BookingDocumentNumberUsed(context.Context, domain.TenantID, domain.DocumentNumber) (bool, error) {
	return false, nil
}

func (double *bookingRegistryDouble) RecordBookingDocumentNumber(_ context.Context, key ports.BookingKey, numbering domain.DocumentNumbering) error {
	storeKey := key.TenantID.String() + "|" + key.Booking.String()
	record, found := double.records[storeKey]
	if !found {
		return errors.New("booking not saved")
	}
	record.DocumentNumber = numbering
	double.records[storeKey] = record
	return nil
}

func decodeAnswer(t *testing.T, response *httptest.ResponseRecorder) map[string]any {
	t.Helper()
	var body map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("响应不是 JSON：%v %s", err, response.Body.String())
	}
	return body
}

func numberedCommissionEndpoint(t *testing.T, numbers *documentNumberStub) http.Handler {
	t.Helper()
	commissions := &commissionRegistryDouble{records: map[string]ports.TransportCommissionRecord{}}
	handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions:       commissions,
		Agreements:        agreementCheckStub{supplier: "SYN-PARTY-CARRIER-TRUNK-01"},
		Downstream:        commissionHandoffStub{},
		Clock:             fixedCommissionClock{},
		Numbers:           numbers,
		CommissionNumbers: commissions,
	})
	return tfhttp.NewSubmitTransportCommissionEndpoint(operatorIntake(t, authenticatorFake{}), handler)
}

func postOperator(t *testing.T, endpoint http.Handler, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer presented.operator.token")
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	return response
}

// Covers: AT-TF-371（线格式半边）——委托提交答复带单据号，重放回显同一个号；租户没登单据编号时区时不带号、带无号原因，
// 不答空串、不拿委托标识顶；撞号答 `NUMBERING_RULE_COLLISION` 并指名撞的是哪一类的规则、不回显委托。
func TestTheCommissionSubmissionAnswerCarriesTheDocumentNumberOrWhyItHasNone(t *testing.T) {
	endpoint := numberedCommissionEndpoint(t, &documentNumberStub{})
	first := postOperator(t, endpoint, "/transport-fulfillment-transport-commission-submissions", trunkCommissionBody)
	body := decodeAnswer(t, first)
	commission, _ := body["commission"].(map[string]any)
	if first.Code != http.StatusCreated || commission["documentNumber"] != "TC-202610-00001" || commission["commission"] != "SYN-COMMISSION-01" {
		t.Fatalf("首交 %d %s", first.Code, first.Body.String())
	}
	if _, present := commission["documentNumberAbsence"]; present {
		t.Fatal("有号时不该带无号原因")
	}
	replay := decodeAnswer(t, postOperator(t, endpoint, "/transport-fulfillment-transport-commission-submissions", trunkCommissionBody))
	if replay["outcome"] != "EXISTING_COMMISSION" || replay["commission"].(map[string]any)["documentNumber"] != "TC-202610-00001" {
		t.Fatalf("重放应回显原号：%v", replay)
	}

	unconfigured := decodeAnswer(t, postOperator(t, numberedCommissionEndpoint(t, &documentNumberStub{outcome: ports.DocumentNumberTimeZoneNotConfigured}),
		"/transport-fulfillment-transport-commission-submissions", trunkCommissionBody))
	unnumbered := unconfigured["commission"].(map[string]any)
	if _, present := unnumbered["documentNumber"]; present || unnumbered["documentNumberAbsence"] != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("时区未配置的委托：%v", unnumbered)
	}

	collision := postOperator(t, numberedCommissionEndpoint(t, &documentNumberStub{outcome: ports.DocumentNumberRuleCollision}),
		"/transport-fulfillment-transport-commission-submissions", trunkCommissionBody)
	collided := decodeAnswer(t, collision)
	if collision.Code != http.StatusOK || collided["outcome"] != "NUMBERING_RULE_COLLISION" || collided["numberingRule"] != "TRANSPORT_COMMISSION" {
		t.Fatalf("撞号 %d %v", collision.Code, collided)
	}
	if _, present := collided["commission"]; present {
		t.Fatal("撞号不回显委托")
	}
}

// Covers: AT-TF-372（线格式半边）——订舱提交答复与取消答复带单据号；订舱标识照旧在 `booking`，单据号另在 `documentNumber`。
func TestTheBookingAnswersCarryTheDocumentNumberBesideTheBookingIdentifier(t *testing.T) {
	handler := bookingHandler(t)
	endpoint := tfhttp.NewSubmitBookingRequestEndpoint(operatorIntake(t, authenticatorFake{}), handler)
	response := postOperator(t, endpoint, "/transport-fulfillment-booking-request-submissions", bookingRequestBody)
	booking, _ := decodeAnswer(t, response)["booking"].(map[string]any)
	if response.Code != http.StatusCreated || booking["documentNumber"] != "BK-202610-00001" || booking["booking"] != "SYN-BOOKING-01" {
		t.Fatalf("订舱首交 %d %s", response.Code, response.Body.String())
	}
}

// Covers: AT-TF-372（读口线格式半边）——订舱册每行带订舱的单据号与所属委托的单据号；没有号的行不带 `documentNumber`，按原因
// 带 `documentNumberAbsence`（上线前开立 / 开立时单据编号时区未配置）；委托没有号时 `commissionDocumentNumber` 整键缺席，
// 委托标识照旧在 `commission`。
func TestTheBookingRegistryCarriesBookingAndCommissionNumbersOrTheReasonForNone(t *testing.T) {
	number, _ := domain.NewDocumentNumber("BK-202611-00009")
	row := func(booking string, numbering domain.DocumentNumbering, commissionNumber string) ports.BookingRequestCatalogueRow {
		return ports.BookingRequestCatalogueRow{
			Booking: booking, Commission: "SYN-COMMISSION-1", Schedule: "SYN-SCHED-1/2026-10-21", Quantity: 1, Unit: "KG",
			RequestedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt, Standing: ports.BookingStandingPendingConfirmation,
			Answers: []ports.BookingAnswerCatalogueRow{}, DocumentNumber: numbering, CommissionDocumentNumber: commissionNumber,
		}
	}
	register := &stubReviewCatalogue{bookings: []ports.BookingRequestCatalogueRow{
		row("SYN-BOOKING-1", domain.NumberedDocument(number), "TC-202611-00004"),
		row("SYN-BOOKING-2", domain.UnnumberedDocument(domain.DocumentNumberingTimeZoneNotConfigured), ""),
		row("SYN-BOOKING-3", domain.DocumentNumbering{}, ""),
	}}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=booking-request")
	var body struct {
		Bookings []map[string]any `json:"bookings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Bookings) != 3 {
		t.Fatalf("decode：%v %s", err, response.Body.String())
	}
	numbered, unconfigured, legacy := body.Bookings[0], body.Bookings[1], body.Bookings[2]
	if numbered["documentNumber"] != "BK-202611-00009" || numbered["commissionDocumentNumber"] != "TC-202611-00004" ||
		numbered["booking"] != "SYN-BOOKING-1" || numbered["commission"] != "SYN-COMMISSION-1" {
		t.Fatalf("有号那一行：%v", numbered)
	}
	if _, present := numbered["documentNumberAbsence"]; present {
		t.Fatalf("有号那一行带了无号原因：%v", numbered)
	}
	for want, row := range map[string]map[string]any{"NUMBERING_TIME_ZONE_NOT_CONFIGURED": unconfigured, "ISSUED_BEFORE_NUMBERING": legacy} {
		if _, present := row["documentNumber"]; present || row["documentNumberAbsence"] != want {
			t.Fatalf("无号那一行（%s）：%v", want, row)
		}
		if _, present := row["commissionDocumentNumber"]; present {
			t.Fatalf("委托没有号时不该带委托单据号：%v", row)
		}
	}
}
