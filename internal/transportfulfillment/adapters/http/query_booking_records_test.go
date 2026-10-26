package tfhttp_test

import (
	"encoding/json"
	"net/http"
	"reflect"
	"strings"
	"testing"
	"time"

	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// Covers: AT-TF-332 的线格式那一半（票 demo-chain-gaps/48）——订舱册逐格转写：订舱各格与订舱提交口回显同一形状（申请量是数），
// 状态词照读口给的透出、页面不另算；有答复的那一行带当前那一份与名下全部答复（按读口给的次序，候补在前、它的后继在后），答复与
// 承运答复口回显同一形状；没有答复的那一行不带 currentAnswer、answers 是 []；取消时刻只在取消过的那一行出现。租户与页大小取
// 认证结果。
func TestTheBookingRequestRegistryTranscribesRowsWithTheirStanding(t *testing.T) {
	cancelledAt := catalogueBaseAt.Add(5 * time.Hour)
	confirmed := ports.BookingRequestCatalogueRow{
		Booking: "SYN-BOOKING-1", Commission: "SYN-COMMISSION-1", Schedule: "SYN-SCHED-1/2026-10-21",
		Quantity: 120, Unit: "KG", RequestedAt: catalogueBaseAt, RecordedAt: catalogueBaseAt.Add(time.Minute),
		Standing: ports.BookingStandingConfirmed,
		Answers: []ports.BookingAnswerCatalogueRow{
			{Answer: "SYN-ANSWER-WAIT", Outcome: "WAITLISTED", Basis: "SYN-WAITLIST-BASIS",
				DecidedAt: catalogueBaseAt.Add(time.Hour), RecordedAt: catalogueBaseAt.Add(time.Hour)},
			{Answer: "SYN-ANSWER-ACCEPT", Outcome: "ACCEPTED", Quantity: 120,
				DecidedAt: catalogueBaseAt.Add(2 * time.Hour), RecordedAt: catalogueBaseAt.Add(2 * time.Hour)},
		},
	}
	confirmed.Current = &confirmed.Answers[1]
	register := &stubReviewCatalogue{bookings: []ports.BookingRequestCatalogueRow{
		confirmed,
		{
			Booking: "SYN-BOOKING-2", Commission: "SYN-COMMISSION-1", Schedule: "SYN-SCHED-1/2026-10-21",
			Quantity: 30, Unit: "KG", RequestedAt: catalogueBaseAt.Add(-time.Hour), CancelledAt: &cancelledAt,
			RecordedAt: catalogueBaseAt, Standing: ports.BookingStandingPendingConfirmation,
			Answers: []ports.BookingAnswerCatalogueRow{},
		},
	}}
	response := serveGet(recordsEndpoint(register), "/transport-fulfillment-records?registry=booking-request")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d", register.gotTenant, register.gotLimit)
	}
	var body struct {
		Outcome  string                       `json:"outcome"`
		Bookings []map[string]json.RawMessage `json:"bookings"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	if body.Outcome != "BOOKING_REQUESTS_LISTED" || len(body.Bookings) != 2 {
		t.Fatalf("答复走样：%s", response.Body.String())
	}
	first := body.Bookings[0]
	for key, want := range map[string]string{
		"booking": `"SYN-BOOKING-1"`, "commission": `"SYN-COMMISSION-1"`, "schedule": `"SYN-SCHED-1/2026-10-21"`,
		"quantity": `120`, "unit": `"KG"`, "standing": `"CONFIRMED"`,
		"requestedAt": `"` + catalogueBaseAt.Format(time.RFC3339Nano) + `"`,
		"recordedAt":  `"` + catalogueBaseAt.Add(time.Minute).Format(time.RFC3339Nano) + `"`,
	} {
		if got := string(first[key]); got != want {
			t.Fatalf("%s = %s, want %s：%s", key, got, want, response.Body.String())
		}
	}
	if _, present := first["cancelledAt"]; present {
		t.Fatalf("没取消的那一行带了取消时刻：%s", response.Body.String())
	}
	var current map[string]json.RawMessage
	if err := json.Unmarshal(first["currentAnswer"], &current); err != nil {
		t.Fatalf("当前答复：%v（%s）", err, response.Body.String())
	}
	if string(current["answer"]) != `"SYN-ANSWER-ACCEPT"` || string(current["outcome"]) != `"ACCEPTED"` ||
		string(current["quantity"]) != `120` || string(current["booking"]) != `"SYN-BOOKING-1"` ||
		string(current["decidedAt"]) != `"`+catalogueBaseAt.Add(2*time.Hour).Format(time.RFC3339Nano)+`"` {
		t.Fatalf("当前答复走样：%s", first["currentAnswer"])
	}
	var answers []map[string]json.RawMessage
	if err := json.Unmarshal(first["answers"], &answers); err != nil || len(answers) != 2 {
		t.Fatalf("全部答复：%v（%s）", err, first["answers"])
	}
	if string(answers[0]["outcome"]) != `"WAITLISTED"` || string(answers[0]["basis"]) != `"SYN-WAITLIST-BASIS"` ||
		string(answers[1]["answer"]) != `"SYN-ANSWER-ACCEPT"` {
		t.Fatalf("答复次序或转写走样：%s", first["answers"])
	}
	if _, present := answers[0]["quantity"]; present {
		t.Fatalf("候补带了接受量：%s", first["answers"])
	}

	second := body.Bookings[1]
	if string(second["standing"]) != `"PENDING_CONFIRMATION"` || string(second["answers"]) != "[]" ||
		string(second["cancelledAt"]) != `"`+cancelledAt.Format(time.RFC3339Nano)+`"` {
		t.Fatalf("没答复、已取消那一行走样：%s", response.Body.String())
	}
	if _, present := second["currentAnswer"]; present {
		t.Fatalf("没答复的那一行带了当前答复：%s", response.Body.String())
	}
}

// Covers: AT-TF-333 的线格式那一半（票 demo-chain-gaps/167，ADR-0239 决定二）——挡回册逐格转写：答复方、订舱、答复标识、依据词、首末
// 挡回时刻、次数（十进制计数串）；两个筛选参数原样交给读口，缺席即不筛。
func TestTheBookingAnswerBounceRegistryTranscribesRowsAndPassesTheFilter(t *testing.T) {
	register := &stubReviewCatalogue{bounces: []ports.BookingAnswerBounceCatalogueRow{
		{Source: "SYN-PARTY-CARRIER-LM-01", Booking: "SYN-BOOKING-1", Answer: "SYN-ANSWER-1", Basis: "SOURCE_NOT_SERVICE_PROVIDER",
			FirstBouncedAt: catalogueBaseAt, LastBouncedAt: catalogueBaseAt.Add(time.Hour), Count: 2},
	}}
	response := serveGet(recordsEndpoint(register),
		"/transport-fulfillment-records?registry=booking-answer-bounce&booking=SYN-BOOKING-1&source=SYN-PARTY-CARRIER-LM-01")
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d body = %s", response.Code, response.Body.String())
	}
	if register.gotTenant != "TENANT-1" || register.gotLimit != 25 ||
		register.gotFilter != (ports.BookingAnswerBounceFilter{Booking: "SYN-BOOKING-1", Source: "SYN-PARTY-CARRIER-LM-01"}) {
		t.Fatalf("读口收到的键走样：tenant=%q limit=%d filter=%+v", register.gotTenant, register.gotLimit, register.gotFilter)
	}
	var body struct {
		Outcome string              `json:"outcome"`
		Bounces []map[string]string `json:"bounces"`
	}
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode %s: %v", response.Body.Bytes(), err)
	}
	want := map[string]string{
		"source": "SYN-PARTY-CARRIER-LM-01", "booking": "SYN-BOOKING-1", "answer": "SYN-ANSWER-1",
		"basis": "SOURCE_NOT_SERVICE_PROVIDER", "count": "2",
		"firstBouncedAt": catalogueBaseAt.Format(time.RFC3339Nano),
		"lastBouncedAt":  catalogueBaseAt.Add(time.Hour).Format(time.RFC3339Nano),
	}
	if body.Outcome != "BOOKING_ANSWER_BOUNCES_LISTED" || len(body.Bounces) != 1 || !reflect.DeepEqual(body.Bounces[0], want) {
		t.Fatalf("答复走样：%s", response.Body.String())
	}

	unfiltered := &stubReviewCatalogue{}
	if response := serveGet(recordsEndpoint(unfiltered), "/transport-fulfillment-records?registry=booking-answer-bounce"); response.Code != http.StatusOK ||
		unfiltered.gotFilter != (ports.BookingAnswerBounceFilter{}) {
		t.Fatalf("不带筛选 = %d，读口收到 %+v", response.Code, unfiltered.gotFilter)
	}
}

// Covers: AT-TF-333 筛选那一支——给了却立不起来的筛选（空串、全空白、带 NUL、订舱标识超过字节界）是坏请求（400），不静默放宽成
// 全册；判它是请求形状的事，先于渠道作答，读口不被触到。
func TestABookingAnswerBounceFilterThatCannotStandIsRefusedBeforeTheChannel(t *testing.T) {
	for name, query := range map[string]string{
		"空订舱":      "&booking=",
		"空白订舱":     "&booking=%20%20",
		"订舱带 NUL":  "&booking=SYN%00BOOKING",
		"订舱超界":     "&booking=" + strings.Repeat("B", 513),
		"空答复方":     "&source=",
		"空白答复方":    "&source=%20",
		"答复方带 NUL": "&source=SYN%00CARRIER",
	} {
		t.Run(name, func(t *testing.T) {
			for _, intake := range []tfhttp.CatalogueQueryIntake{grantedIntake(), tfhttp.UnconfiguredIntake{}} {
				endpoint := tfhttp.NewQueryTransportFulfillmentRecordsEndpoint(intake, unreachableReviewCatalogue{t: t})
				response := serveGet(endpoint, "/transport-fulfillment-records?registry=booking-answer-bounce"+query)
				if response.Code != http.StatusBadRequest || problemCode(t, response) != "MALFORMED_REQUEST" {
					t.Fatalf("%T: status = %d body = %s", intake, response.Code, response.Body.String())
				}
				assertNoOutcome(t, response)
			}
		})
	}
}

// Covers: AT-TF-334 的类型那一半（ADR-0239 决定一）——集成客户端族的 Intake 认不了查阅：它没有查阅那一口的方法，挡回册所在的这个
// 端点在类型上就接不到它。哪天有人给它补上查阅的方法，这里先红。
func TestTheIntegrationClientIntakeCannotServeTheRecordsQuery(t *testing.T) {
	catalogue := reflect.TypeFor[tfhttp.CatalogueQueryIntake]()
	if reflect.TypeFor[*tfhttp.IntegrationClientIntake]().Implements(catalogue) {
		t.Fatal("集成客户端族的 Intake 认得了查阅：挡回册会开给承运方")
	}
	if !reflect.TypeFor[*tfhttp.OperatorOperationsReadIntake]().Implements(catalogue) {
		t.Fatal("操作者查阅读认不了查阅：对照不成立")
	}
}
