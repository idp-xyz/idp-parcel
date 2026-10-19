package main

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	pgpostgres "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	pgdomain "go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证订舱提交与承运答复两口的生产装配（票 demo-chain-gaps/24，ADR-0202 决定一、二、五）：干线
// 供应商协议照种子发布批经 PC 发布、干线周期安排与 SZX→SIN 那一班照种子文件经班次登记编排落库；订舱口经操作者渠道的
// 「运营决定」能力面、答复口经集成客户端族（真库客户端册、对照与区间）走到真编排。与生产装配的差别只在两族的令牌核验方
// 是替身——令牌由谁签不是这里要证的。种子从仓库里读，不在测试里抄一份，理由同委托提交那份用例。

const (
	bookingSubmissionPath = "/transport-fulfillment-booking-request-submissions"
	bookingAnswerPath     = "/transport-fulfillment/booking-answers"
	seededTrunkSchedule   = "SYN-SCHED-SZX-SIN-DAILY/2026-10-21"
	syntheticClientIssuer = "https://id.syn.example/dex"
)

// subjectClientVerifier 认任何非空令牌为 subject 这一个客户端主体：两个承运客户端各用一只。
type subjectClientVerifier struct{ subject string }

func (verifier subjectClientVerifier) VerifyIntegrationClientCredential(
	_ context.Context,
	credential accessidentity.IntegrationClientCredential,
) (accessidentity.VerifiedClientToken, error) {
	if credential.Token() == "" {
		return accessidentity.VerifiedClientToken{}, accessidentity.ErrCredentialRejected
	}
	subject, err := accessidentity.NewIntegrationClientSubject(syntheticClientIssuer, verifier.subject)
	if err != nil {
		return accessidentity.VerifiedClientToken{}, err
	}
	return accessidentity.NewVerifiedClientToken(subject, false), nil
}

// registerSeededTrunkSchedule 照种子文件登干线周期安排，再按日期物化登记 SZX→SIN 那一班。
func registerSeededTrunkSchedule(t *testing.T, db *bentopg.DB) {
	t.Helper()
	registrar, err := buildTransportScheduleRegistration(db)
	if err != nil {
		t.Fatalf("装配班次登记：%v", err)
	}
	read := func(name string, target any) {
		t.Helper()
		raw, err := os.ReadFile("../../scripts/demo-seeds/data/transport/" + name)
		if err != nil {
			t.Fatalf("读种子 %s：%v", name, err)
		}
		decoder := json.NewDecoder(bytes.NewReader(raw))
		decoder.DisallowUnknownFields()
		if err := decoder.Decode(target); err != nil {
			t.Fatalf("解种子 %s：%v", name, err)
		}
	}
	var template struct {
		TenantID string `json:"tenantId"`
		tfhttp.ScheduleTemplateRegistrationPayload
	}
	read("02-schedule-template-szx-sin.json", &template)
	var schedule struct {
		TenantID string `json:"tenantId"`
		tfhttp.TransportScheduleRegistrationPayload
	}
	read("04-materialized-schedule-szx-sin.json", &schedule)
	tenant, err := tfdomain.NewTenantID(template.TenantID)
	if err != nil {
		t.Fatal(err)
	}
	templateCommand, err := template.ScheduleTemplateRegistrationPayload.Command(tenant)
	if err != nil {
		t.Fatalf("周期安排译装：%v", err)
	}
	if result, err := registrar.RegisterTemplate(t.Context(), templateCommand); err != nil || result.Outcome() != tfapp.ScheduleTemplateRegistered {
		t.Fatalf("登周期安排 = %s %v", result.Outcome(), err)
	}
	scheduleCommand, err := schedule.TransportScheduleRegistrationPayload.Command(tenant)
	if err != nil {
		t.Fatalf("班次译装：%v", err)
	}
	result, err := registrar.RegisterSchedule(t.Context(), scheduleCommand)
	if err != nil || result.Outcome() != tfapp.TransportScheduleRegistered {
		t.Fatalf("登班次 = %s %v", result.Outcome(), err)
	}
	if record, present := result.Schedule(); !present || record.Key.Schedule.String() != seededTrunkSchedule {
		t.Fatalf("物化的班次 = %+v，want %s", record.Key, seededTrunkSchedule)
	}
}

// registerCarrierClient 在真库客户端册上登一个承运客户端：绑定 SYN-TENANT-01 与 source，授予承运订舱答复这一类。
func registerCarrierClient(t *testing.T, db *bentopg.DB, subject, source string) {
	t.Helper()
	registry, err := aipg.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	clientSubject, err := accessidentity.NewIntegrationClientSubject(syntheticClientIssuer, subject)
	if err != nil {
		t.Fatal(err)
	}
	reference, err := accessidentity.NewCredentialReference("SYN-CREDENTIAL-STORE/" + subject)
	if err != nil {
		t.Fatal(err)
	}
	binding, err := accessidentity.NewIntegrationClientBinding(clientSubject, "SYN-TENANT-01", source, reference, false, "SYN-INTEGRATION-CLIENT-BASIS-DCG24")
	if err != nil {
		t.Fatal(err)
	}
	validity, err := accessidentity.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	grant, err := accessidentity.NewIntegrationClientGrant("SYN-TENANT-01", "SYN-GRANT-"+subject, clientSubject,
		accessidentity.FactCarrierBookingAnswer, validity, "SYN-INTEGRATION-GRANT-BASIS-DCG24")
	if err != nil {
		t.Fatal(err)
	}
	var bound, granted accessidentity.IntegrationClientRegistrationOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var registerErr error
		if bound, registerErr = registry.RegisterIntegrationClient(txCtx, binding); registerErr != nil {
			return registerErr
		}
		granted, registerErr = registry.RegisterIntegrationClientGrant(txCtx, grant)
		return registerErr
	}); err != nil {
		t.Fatalf("登客户端事务：%v", err)
	}
	if bound != accessidentity.IntegrationClientRegistrationRecorded || granted != accessidentity.IntegrationClientRegistrationRecorded {
		t.Fatalf("登客户端 %s = %s，授予 = %s", subject, bound, granted)
	}
}

// registerCarrierAnswerAdmission 登 SYN-TENANT-01 在承运订舱答复上的准入对照与本产品区间，登法与演示种子相同。
func registerCarrierAnswerAdmission(t *testing.T, db *bentopg.DB) {
	t.Helper()
	mappings, err := aipg.NewAdmissionScopeMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	intervals, err := pgpostgres.NewAuthorityIntervals(db)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	validity, err := accessidentity.NewEffectiveInterval(from, time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	requirement := accessidentity.AdmissionRequirement{
		Capability: accessidentity.ExternalFactAdmissionCapability,
		FactKind:   accessidentity.FactCarrierBookingAnswer.String(),
	}
	mapping, err := accessidentity.NewAdmissionScopeMapping("SYN-TENANT-01", "SYN-MAP-EF-CARRIER-BOOKING-ANSWER-DCG24", requirement,
		"SYN-PILOT-SCOPE/external-facts@v1", validity, "SYN-MAP-BASIS-DCG24")
	if err != nil {
		t.Fatal(err)
	}
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		if _, err := mappings.RegisterAdmissionScopeMapping(txCtx, mapping); err != nil {
			return err
		}
		return intervals.Append(txCtx, pgdomain.AuthorityInterval{
			ObjectScope: "SYN-PILOT-SCOPE/external-facts@v1",
			Capability:  requirement.Capability,
			FactKind:    requirement.FactKind,
			Authority:   isolatedSelfAuthority,
			From:        from,
		})
	}); err != nil {
		t.Fatalf("登对照与区间：%v", err)
	}
}

type bookingFaceEndpoints struct {
	commission http.Handler
	booking    http.Handler
	trunk      http.Handler
	lastMile   http.Handler
}

// bookingFacesForTest 装生产的委托提交、订舱提交与承运答复编排；操作者只持有委托提交与订舱提交两种决定，干线与末端两个
// 承运客户端各绑自己的承运方。
func bookingFacesForTest(t *testing.T, db *bentopg.DB) bookingFaceEndpoints {
	t.Helper()
	kinds := []accessidentity.DecisionKind{accessidentity.DecisionTransportCommissionSubmission, accessidentity.DecisionBookingRequestSubmission}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
	registerCarrierAnswerAdmission(t, db)
	registerCarrierClient(t, db, "SYN-CLIENT-CARRIER-TRUNK-01", "SYN-PARTY-CARRIER-TRUNK-01")
	registerCarrierClient(t, db, "SYN-CLIENT-CARRIER-LM-01", "SYN-PARTY-CARRIER-LM-01")
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	operatorMinter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: kinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(operatorMinter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	commission, err := buildTransportCommissionSubmission(db)
	if err != nil {
		t.Fatalf("装配委托提交：%v", err)
	}
	faces, err := buildBookingFaces(db)
	if err != nil {
		t.Fatalf("装配订舱两口：%v", err)
	}
	clients, err := aipg.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	carrierEndpoint := func(subject string) http.Handler {
		t.Helper()
		minter, err := buildIntegrationClientMinter(subjectClientVerifier{subject: subject}, clients, admission)
		if err != nil {
			t.Fatal(err)
		}
		intakes, err := buildIntegrationClientIntakes(minter)
		if err != nil {
			t.Fatal(err)
		}
		return tfhttp.NewAnswerBookingEndpoint(intakes.transport, faces)
	}
	return bookingFaceEndpoints{
		commission: tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, commission),
		booking:    tfhttp.NewSubmitBookingRequestEndpoint(decisions.transport, faces),
		trunk:      carrierEndpoint("SYN-CLIENT-CARRIER-TRUNK-01"),
		lastMile:   carrierEndpoint("SYN-CLIENT-CARRIER-LM-01"),
	}
}

func postWithToken(t *testing.T, endpoint http.Handler, path, token, body string) (int, map[string]any) {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	request.Header.Set("Authorization", "Bearer "+token)
	response := httptest.NewRecorder()
	endpoint.ServeHTTP(response, request)
	var answer map[string]any
	if err := json.Unmarshal(response.Body.Bytes(), &answer); err != nil {
		t.Fatalf("decode %s: %v", response.Body.String(), err)
	}
	return response.Code, answer
}

func trunkBookingBody(booking, schedule string) string {
	return `{"booking": "` + booking + `", "commission": "SYN-COMMISSION-TRUNK-DCG24", "schedule": "` + schedule + `",
		"quantity": 120, "unit": "KG", "requestedAt": "2026-10-19T08:30:00Z"}`
}

const trunkAcceptedAnswer = `{"booking": "SYN-BOOKING-TRUNK-DCG24", "answer": "SYN-CARRIER-ANSWER-DCG24", "outcome": "ACCEPTED",
	"quantity": 120, "decidedAt": "2026-10-19T09:00:00Z"}`

// Covers: `AT-TF-146`、`AT-TF-148`、`AT-TF-149`、`AT-TF-152` 真库面，票 demo-chain-gaps/24 完成判据（S）——
//   - 演示：干线委托提交后，对种子物化的 SZX→SIN 那一班订舱答 201，库里订舱行带着那一班；对不在册的班次订舱答 200
//     SCHEDULE_NOT_FOUND、零写入。
//   - 演示：合成干线承运客户端（来源身份即干线承运方）用令牌答 ACCEPTED 答 201；原样重交答 200 EXISTING_ANSWER，不多一行。
//   - 末端承运客户端答同一订舱答 200 SOURCE_NOT_SERVICE_PROVIDER，不回显干线那份答复、不落行。
func TestTheWiredBookingFacesBookTheSeededTrunkScheduleAndTakeTheCarriersAnswer(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	registerSeededTrunkSchedule(t, db)
	endpoints := bookingFacesForTest(t, db)

	status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, "presented.operator.token",
		commissionBody("SYN-COMMISSION-TRUNK-DCG24", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"))
	if status != http.StatusCreated || answer["outcome"] != "COMMISSION_SUBMITTED" {
		t.Fatalf("干线委托 = %d %v", status, answer)
	}

	status, answer = postWithToken(t, endpoints.booking, bookingSubmissionPath, "presented.operator.token",
		trunkBookingBody("SYN-BOOKING-TRUNK-DCG24", seededTrunkSchedule))
	if status != http.StatusCreated || answer["outcome"] != "BOOKING_SUBMITTED" {
		t.Fatalf("对物化班次订舱 = %d %v", status, answer)
	}
	status, answer = postWithToken(t, endpoints.booking, bookingSubmissionPath, "presented.operator.token",
		trunkBookingBody("SYN-BOOKING-TRUNK-DCG24-NOWHERE", "SYN-SCHED-SZX-SIN-DAILY/2099-01-01"))
	if status != http.StatusOK || answer["outcome"] != "SCHEDULE_NOT_FOUND" {
		t.Fatalf("对不在册班次订舱 = %d %v", status, answer)
	}

	status, answer = postWithToken(t, endpoints.trunk, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer)
	if status != http.StatusCreated || answer["outcome"] != "BOOKING_ANSWERED" {
		t.Fatalf("干线承运客户端答接受 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, endpoints.trunk, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer); status != http.StatusOK || answer["outcome"] != "EXISTING_ANSWER" {
		t.Fatalf("原样重交 = %d %v", status, answer)
	}
	status, answer = postWithToken(t, endpoints.lastMile, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer)
	if status != http.StatusOK || answer["outcome"] != "SOURCE_NOT_SERVICE_PROVIDER" || answer["answer"] != nil {
		t.Fatalf("末端承运客户端答干线订舱 = %d %v", status, answer)
	}

	var schedule string
	var bookings, answers int
	if err := pool.QueryRow(t.Context(),
		`SELECT schedule_id, (SELECT count(*) FROM transport_fulfillment.booking_request)
		   FROM transport_fulfillment.booking_request WHERE tenant_id = 'SYN-TENANT-01' AND booking_id = 'SYN-BOOKING-TRUNK-DCG24'`,
	).Scan(&schedule, &bookings); err != nil {
		t.Fatalf("读订舱行：%v", err)
	}
	var outcome string
	if err := pool.QueryRow(t.Context(),
		`SELECT outcome, (SELECT count(*) FROM transport_fulfillment.booking_answer)
		   FROM transport_fulfillment.booking_answer WHERE tenant_id = 'SYN-TENANT-01' AND booking_id = 'SYN-BOOKING-TRUNK-DCG24'`,
	).Scan(&outcome, &answers); err != nil {
		t.Fatalf("读答复行：%v", err)
	}
	if schedule != seededTrunkSchedule || bookings != 1 || outcome != "ACCEPTED" || answers != 1 {
		t.Fatalf("库里：班次 %q、订舱 %d 行、答复 %s、答复 %d 行", schedule, bookings, outcome, answers)
	}
}

// 班次登记读口在事务里报 SQL 错误时，PostgreSQL 把整笔事务置为 aborted：答复若照常提交，只换回提交错误，线上落成 500。
// 这里让班次表在真库上读不到（改表名），钉住订舱方拿到的是未决、指名等班次登记，且零写入。
// Covers: `AT-TF-146`「班次登记读不动为未形成」那一支（真库面，事务已中止）。
func TestTheWiredBookingSubmissionAnswersUndecidedWhenTheScheduleReadAbortsTheTransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	endpoints := bookingFacesForTest(t, db)
	if status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, "presented.operator.token",
		commissionBody("SYN-COMMISSION-TRUNK-DCG24", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")); status != http.StatusCreated {
		t.Fatalf("干线委托 = %d %v", status, answer)
	}
	if _, err := pool.Exec(t.Context(),
		`ALTER TABLE transport_fulfillment.transport_schedule RENAME TO transport_schedule_unreachable`); err != nil {
		t.Fatalf("让班次表读不到：%v", err)
	}

	status, answer := postWithToken(t, endpoints.booking, bookingSubmissionPath, "presented.operator.token",
		trunkBookingBody("SYN-BOOKING-TRUNK-DCG24", seededTrunkSchedule))
	if status != http.StatusOK || answer["outcome"] != "COMMISSION_UNDECIDED" || answer["undecidedReason"] != "SCHEDULE_STORE_UNAVAILABLE" {
		t.Fatalf("班次读不动 = %d %v，want 200 COMMISSION_UNDECIDED / SCHEDULE_STORE_UNAVAILABLE", status, answer)
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.booking_request`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("订舱表 %d 行 %v，want 0——读不动的订舱落了行", rows, err)
	}
}
