package tfhttp_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉 ADR-0303 复核记录（票 demo-chain-gaps/180）的线格式半边：其余六个运营决定口与补拍、按册重问把认证出的操作者随命令
// 交给编排，载荷里收不进操作者；订舱册带出提交人与取消人，写面回执不带。夹具全为合成引用（S 级）。

// Covers: `AT-TF-348`、`AT-TF-349`、`AT-TF-350`、`AT-TF-351`、`AT-TF-352`、`AT-TF-353`、`AT-TF-354`、`AT-TF-355` 线格式半边——八口各自把信封里的
// 操作者交进命令；载荷里自报一位操作者是坏报文（封闭解码），不会顶替信封给的那一位。
func TestEachRemainingDecisionFaceCarriesTheEnvelopeOperatorIntoItsCommand(t *testing.T) {
	intake := operatorIntake(t, authenticatorFake{})
	ctx := context.Background()
	faces := map[string]struct {
		body     string
		operator func(*http.Request) (string, error)
		smuggled string
	}{
		"booking submission": {`{"booking":"SYN-BOOKING-01","commission":"SYN-COMMISSION-01","schedule":"SYN-SCHED-01","quantity":10,"unit":"KG","requestedAt":"2026-09-25T01:00:00Z"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeBookingRequestSubmission(ctx, request)
				return command.SubmittedBy, err
			}, "submittedBy"},
		"booking cancellation": {`{"booking":"SYN-BOOKING-01","cancelledAt":"2026-09-25T01:00:00Z"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeBookingRequestCancellation(ctx, request)
				return command.CancelledBy, err
			}, "cancelledBy"},
		"commission cancellation": {`{"commission":"SYN-COMMISSION-01","cancelledAt":"2026-09-25T01:00:00Z"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeTransportCommissionCancellation(ctx, request)
				return command.CancelledBy, err
			}, "cancelledBy"},
		"segment commission adoption": {`{"segment":"SYN-SEG-01","commission":"SYN-COMMISSION-01","provider":"SYN-PARTNER-01"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeSegmentCommissionAdoption(ctx, request)
				return command.AdoptedBy, err
			}, "adoptedBy"},
		"capacity reservation": {`{"pool":"SYN-POOL-01","reservation":"SYN-RES-01","quantity":10,"unit":"KG","source":{"kind":"OPERATOR_DECLARATION","reference":"SYN-DECL-01"},` +
			`"validUntil":"2026-09-26T01:00:00Z","reservedAt":"2026-09-25T01:00:00Z"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeCapacityReservation(ctx, request)
				return command.ReservedBy, err
			}, "reservedBy"},
		"capacity release": {`{"pool":"SYN-POOL-01","reservation":"SYN-RES-01","release":"SYN-REL-01","quantity":5,"unit":"KG",` +
			`"source":{"kind":"OPERATOR_DECLARATION","reference":"SYN-DECL-02"},"releasedAt":"2026-09-25T02:00:00Z"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeCapacityRelease(ctx, request)
				return command.ReleasedBy, err
			}, "releasedBy"},
		"delivery dispatch trigger": {`{"segment":"SYN-SEG-01","object":"SYN-OBJ-01","occurredAt":"2026-09-25T01:00:00Z"}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeDeliveryDispatchTrigger(ctx, request)
				return command.TriggeredBy, err
			}, "triggeredBy"},
		"delivery dispatch reask": {`{"scope":"ALL","limit":10}`,
			func(request *http.Request) (string, error) {
				command, err := intake.IntakeDeliveryDispatchReask(ctx, request)
				return command.ReaskedBy, err
			}, "reaskedBy"},
	}
	for name, face := range faces {
		t.Run(name, func(t *testing.T) {
			operator, err := face.operator(bearerRequest("presented.operator.token", face.body))
			if err != nil || operator != operatorSubject {
				t.Fatalf("命令里的操作者 = %q err=%v，want 信封给的 %q", operator, err, operatorSubject)
			}
			smuggling := `{"` + face.smuggled + `":"https://id.syn.example/dex#SYN-OPERATOR-SELF",` + strings.TrimPrefix(face.body, "{")
			if _, err := face.operator(bearerRequest("presented.operator.token", smuggling)); !errors.Is(err, tfhttp.ErrMalformedRequest) {
				t.Fatalf("载荷自报操作者：err = %v，want ErrMalformedRequest", err)
			}
		})
	}
}

// Covers: `AT-TF-348`、`AT-TF-349` 线格式半边——订舱册每一行带出记下的提交人与取消人，没记下的整键缺席；按册重问带操作者时的回执
// 与不带时同形（不含操作者）。
func TestTheBookingRegistryShowsTheOperatorsAndTheReaskReceiptDoesNot(t *testing.T) {
	cancelledAt := catalogueBaseAt.Add(5 * time.Hour)
	register := &stubReviewCatalogue{bookings: []ports.BookingRequestCatalogueRow{
		{
			Booking: "SYN-BOOKING-1", Commission: "SYN-COMMISSION-1", Schedule: "SYN-SCHED-1", Quantity: 120, Unit: "KG",
			RequestedAt: catalogueBaseAt, CancelledAt: &cancelledAt, RecordedAt: catalogueBaseAt, Standing: ports.BookingStandingPendingConfirmation,
			Answers: []ports.BookingAnswerCatalogueRow{}, SubmittedBy: "issuer-1#submitter", CancelledBy: "issuer-1#canceller",
		},
		{
			Booking: "SYN-BOOKING-2", Commission: "SYN-COMMISSION-1", Schedule: "SYN-SCHED-1", Quantity: 30, Unit: "KG",
			RequestedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt, Standing: ports.BookingStandingPendingConfirmation,
			Answers: []ports.BookingAnswerCatalogueRow{},
		},
	}}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=booking-request")
	var body struct {
		Bookings []map[string]any `json:"bookings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil || len(body.Bookings) != 2 {
		t.Fatalf("译订舱册：%v body=%s", err, response.Body.String())
	}
	if body.Bookings[0]["submittedBy"] != "issuer-1#submitter" || body.Bookings[0]["cancelledBy"] != "issuer-1#canceller" {
		t.Fatalf("第一行的提交人与取消人 = %v、%v", body.Bookings[0]["submittedBy"], body.Bookings[0]["cancelledBy"])
	}
	for _, key := range []string{"submittedBy", "cancelledBy"} {
		if _, present := body.Bookings[1][key]; present {
			t.Fatalf("没记下的那一行应整键缺席 %s：%v", key, body.Bookings[1])
		}
	}

	var asks []ports.DeliveryDispatchAskRecord
	endpoint := reaskEndpointRecording(t, operatorIntake(t, authenticatorFake{}), &asks)
	_, receipt := postReask(t, endpoint, `{"scope":"ALL","limit":10}`)
	if len(asks) != 1 || asks[0].AskedBy.String() != operatorSubject {
		t.Fatalf("问询册 = %+v，want 一行、按的人是信封给的那一位", asks)
	}
	raw, _ := json.Marshal(receipt)
	if strings.Contains(string(raw), operatorSubject) || strings.Contains(string(raw), "askedBy") || strings.Contains(string(raw), "reaskedBy") {
		t.Fatalf("重问回执带了操作证据：%s", raw)
	}
}
