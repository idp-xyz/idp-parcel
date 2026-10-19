package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sort"
	"strings"
	"testing"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ---- 订舱与承运答复两口的端口替身 ----

type bookingRegistryDouble struct {
	records map[string]ports.BookingRecord
}

func (double *bookingRegistryDouble) FindByKey(_ context.Context, key ports.BookingKey) (ports.BookingRecord, bool, error) {
	record, found := double.records[key.TenantID.String()+"|"+key.Booking.String()]
	return record, found, nil
}

func (double *bookingRegistryDouble) Save(_ context.Context, record ports.BookingRecord) (ports.BookingSaveOutcome, error) {
	storeKey := record.Key.TenantID.String() + "|" + record.Key.Booking.String()
	if _, exists := double.records[storeKey]; exists {
		return ports.BookingAlreadyRegistered, nil
	}
	double.records[storeKey] = record
	return ports.BookingSaved, nil
}

type answerLedgerDouble struct {
	records []ports.BookingAnswerRecord
}

func (double *answerLedgerDouble) FindAnswersForUpdate(_ context.Context, key ports.BookingKey) ([]ports.BookingAnswerRecord, error) {
	var answers []ports.BookingAnswerRecord
	for _, record := range double.records {
		if record.Key == key {
			answers = append(answers, record)
		}
	}
	sort.Slice(answers, func(i, j int) bool {
		return answers[i].Acceptance.DecidedAt().Before(answers[j].Acceptance.DecidedAt())
	})
	return answers, nil
}

func (double *answerLedgerDouble) Save(_ context.Context, record ports.BookingAnswerRecord) (ports.BookingAnswerSaveOutcome, error) {
	for _, stored := range double.records {
		if stored.Key == record.Key && (stored.Acceptance.Acceptance() == record.Acceptance.Acceptance() ||
			stored.Acceptance.Settles() == record.Acceptance.Settles()) {
			return ports.BookingAlreadyAnswered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.BookingAnswerSaved, nil
}

type scheduleRegisterStub struct{ registered string }

func (stub scheduleRegisterStub) FindByKey(_ context.Context, key ports.ScheduleKey) (ports.ScheduleRecord, bool, error) {
	if key.Schedule.String() != stub.registered {
		return ports.ScheduleRecord{}, false, nil
	}
	return ports.ScheduleRecord{Key: key}, true, nil
}

const (
	trunkSchedule = "SYN-SCHED-SZX-SIN-DAILY/2026-10-21"
	trunkCarrier  = "SYN-PARTY-CARRIER-TRUNK-01"
)

// bookingHandler 交回一只已提交过干线委托（服务方干线承运方）的编排：订舱与答复两口共用它。
func bookingHandler(t *testing.T) *application.CommissionTransportHandler {
	t.Helper()
	handler := application.NewCommissionTransportHandler(application.CommissionTransportDeps{
		Commissions: &commissionRegistryDouble{records: map[string]ports.TransportCommissionRecord{}},
		Bookings:    &bookingRegistryDouble{records: map[string]ports.BookingRecord{}},
		Answers:     &answerLedgerDouble{},
		Schedules:   scheduleRegisterStub{registered: trunkSchedule},
		Agreements:  agreementCheckStub{supplier: trunkCarrier},
		Downstream:  commissionHandoffStub{},
		Clock:       fixedCommissionClock{},
	})
	tenant, err := domain.NewTenantID(operatorTenant)
	if err != nil {
		t.Fatal(err)
	}
	var payload tfhttp.TransportCommissionSubmissionPayload
	if err := json.Unmarshal([]byte(trunkCommissionBody), &payload); err != nil {
		t.Fatal(err)
	}
	command, err := payload.Command(tenant)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := handler.SubmitCommission(context.Background(), command); err != nil || result.Outcome() != application.CommissionSubmitted {
		t.Fatalf("委托：%q %v", result.Outcome(), err)
	}
	return handler
}

const bookingRequestBody = `{"booking": "SYN-BOOKING-01", "commission": "SYN-COMMISSION-01", "schedule": "` + trunkSchedule + `",
	"quantity": 120, "unit": "KG", "requestedAt": "2026-10-19T08:30:00Z"}`

func postBookingFace(t *testing.T, endpoint http.Handler, path, token, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	if token != "" {
		request.Header.Set("Authorization", "Bearer "+token)
	}
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return response.Code, answer
}

// Covers: ADR-0202 决定一、五的线格式——订舱提交口按「运营决定」的订舱提交认证、租户只取认证结果；就在册班次订舱答 201，
// 回显班次与委托；原样重交 200 已有订舱；班次不在册 200 SCHEDULE_NOT_FOUND、委托不在册 200 COMMISSION_NOT_FOUND，两格
// 不回显订舱；载荷自报租户、时刻写坏是坏报文（400）。
func TestTheBookingRequestSubmissionFaceAnswersOnTheWire(t *testing.T) {
	endpoint := tfhttp.NewSubmitBookingRequestEndpoint(operatorIntake(t, authenticatorFake{}), bookingHandler(t))
	const path = "/transport-fulfillment-booking-request-submissions"

	status, answer := postBookingFace(t, endpoint, path, "presented.operator.token", bookingRequestBody)
	if status != http.StatusCreated || answer["outcome"] != "BOOKING_SUBMITTED" {
		t.Fatalf("首交 = %d %v", status, answer)
	}
	booking, _ := answer["booking"].(map[string]any)
	if booking["schedule"] != trunkSchedule || booking["commission"] != "SYN-COMMISSION-01" ||
		booking["quantity"] != float64(120) || booking["requestedAt"] != "2026-10-19T08:30:00Z" {
		t.Fatalf("回显的订舱 = %v", booking)
	}
	if status, answer := postBookingFace(t, endpoint, path, "presented.operator.token", bookingRequestBody); status != http.StatusOK || answer["outcome"] != "EXISTING_BOOKING" {
		t.Fatalf("原样重交 = %d %v", status, answer)
	}

	for name, testCase := range map[string]struct {
		body string
		want string
	}{
		"班次不在册": {strings.Replace(strings.Replace(bookingRequestBody, trunkSchedule, "SYN-SCHED-NOWHERE/2026-10-21", 1),
			"SYN-BOOKING-01", "SYN-BOOKING-02", 1), "SCHEDULE_NOT_FOUND"},
		"委托不在册": {strings.Replace(strings.Replace(bookingRequestBody, `"SYN-COMMISSION-01"`, `"SYN-COMMISSION-99"`, 1),
			"SYN-BOOKING-01", "SYN-BOOKING-03", 1), "COMMISSION_NOT_FOUND"},
	} {
		status, answer := postBookingFace(t, endpoint, path, "presented.operator.token", testCase.body)
		if status != http.StatusOK || answer["outcome"] != testCase.want {
			t.Fatalf("%s = %d %v, want 200 %s", name, status, answer, testCase.want)
		}
		if _, present := answer["booking"]; present {
			t.Fatalf("%s 却回显了订舱：%v", name, answer)
		}
	}

	for name, body := range map[string]string{
		"self-reported tenant": strings.Replace(bookingRequestBody, `{"booking"`, `{"tenantId": "SYN-TENANT-02", "booking"`, 1),
		"unparseable instant":  strings.Replace(bookingRequestBody, `"2026-10-19T08:30:00Z"`, `"soon"`, 1),
	} {
		status, answer := postBookingFace(t, endpoint, path, "presented.operator.token", body)
		problem, _ := answer["error"].(map[string]any)
		if status != http.StatusBadRequest || problem["code"] != "MALFORMED_REQUEST" {
			t.Fatalf("%s = %d %v, want 400 MALFORMED_REQUEST", name, status, answer)
		}
	}
}

// carrierAuthenticatorFake 替身认证方：没带令牌答令牌不过；否则答 err，或租户与 source 这个来源身份。
type carrierAuthenticatorFake struct {
	source string
	err    error
}

func (fake carrierAuthenticatorFake) AuthenticateCarrierBookingAnswer(_ context.Context, credential tfhttp.PresentedClientCredential) (tfhttp.CarrierAnswerIdentity, error) {
	if credential.Token == "" {
		return tfhttp.CarrierAnswerIdentity{}, tfhttp.ErrIntegrationClientCredentialRejected
	}
	if fake.err != nil {
		return tfhttp.CarrierAnswerIdentity{}, fake.err
	}
	tenant, err := domain.NewTenantID(operatorTenant)
	return tfhttp.CarrierAnswerIdentity{Tenant: tenant, Source: fake.source}, err
}

func clientIntake(t *testing.T, authenticator tfhttp.IntegrationClientAuthenticator) *tfhttp.IntegrationClientIntake {
	t.Helper()
	intake, err := tfhttp.NewIntegrationClientIntake(authenticator)
	if err != nil {
		t.Fatal(err)
	}
	return intake
}

const acceptedAnswerBody = `{"booking": "SYN-BOOKING-01", "answer": "SYN-CARRIER-ANSWER-01", "outcome": "ACCEPTED",
	"quantity": 100, "decidedAt": "2026-10-19T09:00:00Z"}`

// Covers: ADR-0202 决定二、五的线格式——承运答复口按集成客户端认证，来源身份取自绑定；服务方自己答接受 201、回显接受量；
// 原样重交 200 已有答复；别的承运商答 200 SOURCE_NOT_SERVICE_PROVIDER、不回显在册答复；订舱不在册 200 BOOKING_NOT_FOUND；
// 五值之外的词、自报租户或来源、时刻写坏是坏报文（400）；没令牌 401、无授予 403，各用集成客户端族自己的码。
func TestTheBookingAnswerFaceAnswersOnTheWire(t *testing.T) {
	handler := bookingHandler(t)
	booking := tfhttp.NewSubmitBookingRequestEndpoint(operatorIntake(t, authenticatorFake{}), handler)
	if status, answer := postBookingFace(t, booking, "/transport-fulfillment-booking-request-submissions", "presented.operator.token", bookingRequestBody); status != http.StatusCreated {
		t.Fatalf("订舱 = %d %v", status, answer)
	}
	const path = "/transport-fulfillment/booking-answers"
	carrier := tfhttp.NewAnswerBookingEndpoint(clientIntake(t, carrierAuthenticatorFake{source: trunkCarrier}), handler)
	stranger := tfhttp.NewAnswerBookingEndpoint(clientIntake(t, carrierAuthenticatorFake{source: "SYN-PARTY-CARRIER-LM-01"}), handler)

	status, answer := postBookingFace(t, stranger, path, "presented.client.token", acceptedAnswerBody)
	if status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_SERVICE_PROVIDER" {
		t.Fatalf("别的承运商答 = %d %v", status, answer)
	}
	status, answer = postBookingFace(t, carrier, path, "presented.client.token", acceptedAnswerBody)
	if status != http.StatusCreated || answer["outcome"] != "BOOKING_ANSWERED" {
		t.Fatalf("服务方答接受 = %d %v", status, answer)
	}
	body, _ := answer["answer"].(map[string]any)
	if body["outcome"] != "ACCEPTED" || body["quantity"] != float64(100) || body["answer"] != "SYN-CARRIER-ANSWER-01" {
		t.Fatalf("回显的答复 = %v", body)
	}
	if status, answer := postBookingFace(t, carrier, path, "presented.client.token", acceptedAnswerBody); status != http.StatusOK || answer["outcome"] != "EXISTING_ANSWER" {
		t.Fatalf("原样重交 = %d %v", status, answer)
	}
	if status, answer := postBookingFace(t, stranger, path, "presented.client.token", acceptedAnswerBody); status != http.StatusOK ||
		answer["outcome"] != "SOURCE_NOT_SERVICE_PROVIDER" || answer["answer"] != nil {
		t.Fatalf("别的承运商原样重放 = %d %v——读回了别人的答复", status, answer)
	}
	missing := strings.Replace(acceptedAnswerBody, `"SYN-BOOKING-01"`, `"SYN-BOOKING-77"`, 1)
	if status, answer := postBookingFace(t, carrier, path, "presented.client.token", missing); status != http.StatusOK || answer["outcome"] != "BOOKING_NOT_FOUND" {
		t.Fatalf("订舱不在册 = %d %v", status, answer)
	}

	for name, body := range map[string]string{
		"五值之外的词":               strings.Replace(acceptedAnswerBody, `"ACCEPTED"`, `"PENDING"`, 1),
		"self-reported tenant": strings.Replace(acceptedAnswerBody, `{"booking"`, `{"tenantId": "SYN-TENANT-02", "booking"`, 1),
		"self-reported source": strings.Replace(acceptedAnswerBody, `{"booking"`, `{"source": "`+trunkCarrier+`", "booking"`, 1),
		"unparseable instant":  strings.Replace(acceptedAnswerBody, `"2026-10-19T09:00:00Z"`, `"now"`, 1),
	} {
		status, answer := postBookingFace(t, carrier, path, "presented.client.token", body)
		problem, _ := answer["error"].(map[string]any)
		if status != http.StatusBadRequest || problem["code"] != "MALFORMED_REQUEST" {
			t.Fatalf("%s = %d %v, want 400 MALFORMED_REQUEST", name, status, answer)
		}
	}

	for name, testCase := range map[string]struct {
		endpoint http.Handler
		token    string
		status   int
		code     string
	}{
		"没令牌": {carrier, "", http.StatusUnauthorized, "INTEGRATION_CLIENT_CREDENTIAL_REJECTED"},
		"无授予": {tfhttp.NewAnswerBookingEndpoint(clientIntake(t, carrierAuthenticatorFake{err: tfhttp.ErrIntegrationClientNotGranted}), handler),
			"presented.client.token", http.StatusForbidden, "INTEGRATION_CLIENT_NOT_GRANTED"},
		"不在准入范围": {tfhttp.NewAnswerBookingEndpoint(clientIntake(t, carrierAuthenticatorFake{err: tfhttp.ErrOutsideAdmissionScope}), handler),
			"presented.client.token", http.StatusForbidden, "OUTSIDE_ADMISSION_SCOPE"},
	} {
		status, answer := postBookingFace(t, testCase.endpoint, path, testCase.token, acceptedAnswerBody)
		problem, _ := answer["error"].(map[string]any)
		if status != testCase.status || problem["code"] != testCase.code {
			t.Fatalf("%s = %d %v, want %d %s", name, status, answer, testCase.status, testCase.code)
		}
	}
}

// Covers: 先认证、后解载荷——令牌不过的调用方看不到载荷校验的结果；没认出身份就交来翻译是装配违约，不是坏报文。
func TestTheBookingAnswerIntakeAuthenticatesBeforeReadingThePayload(t *testing.T) {
	intake := clientIntake(t, carrierAuthenticatorFake{err: tfhttp.ErrIntegrationClientNotGranted})
	if _, err := intake.IntakeBookingAnswer(context.Background(), bearerRequest("t", "!!not-json!!")); !errors.Is(err, tfhttp.ErrIntegrationClientNotGranted) {
		t.Fatalf("err = %v, want the authenticator's answer", err)
	}
	var payload tfhttp.BookingAnswerPayload
	if err := json.Unmarshal([]byte(acceptedAnswerBody), &payload); err != nil {
		t.Fatal(err)
	}
	if _, err := payload.Command(tfhttp.CarrierAnswerIdentity{}); err == nil || errors.Is(err, tfhttp.ErrMalformedRequest) {
		t.Fatalf("没有身份：err = %v，want 非坏报文的装配违约", err)
	}
}
