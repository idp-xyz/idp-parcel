package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真库走生产装配证待确认的逐对象尝试结果凭同一次上门的后补来源更正成确定结果（票 docs-consistency/code/90，ADR-0297）：执行方在作业
// 事实面后补、复核人在运营决定面认定，两面同一份编排；更正成妥投之后同一次尝试里的那件对象登得出有效交付、更正成失败之后交付登记答`未生效`；
// 更正成揽收到手经对象级揽收登记形成场外揽收、进段、入队交 parcel-shipment 的信封，而待确认那一版留在册上不改写。测试输入是合成数据，只记 `S`。

var attemptResultOperatorDecisions = append([]accessidentity.DecisionKind{
	accessidentity.DecisionPickupAttemptResultConfirmation,
	accessidentity.DecisionDeliveryAttemptResultConfirmation,
}, operationDecisionTransportFaces...)

// registerAttemptResultAdmission 登一次准入对照与本产品的权威区间（每个库一次）：准入走生产的准入范围桥。
func registerAttemptResultAdmission(t *testing.T, db *bentopg.DB) {
	t.Helper()
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, attemptResultOperatorDecisions...)
}

// attemptResultOperatorForTest 装 TF 运营决定口的操作者渠道 Intake，操作者持有 kinds（缺省为认定两种加建派送任务等）。与生产装配的差别
// 只在核验方与操作者册是替身——令牌签发归演示发行方，操作者册的真库往返另有 accessidentity 的用例。
func attemptResultOperatorForTest(t *testing.T, db *bentopg.DB, kinds ...accessidentity.DecisionKind) *tfhttp.OperatorDecisionIntake {
	t.Helper()
	if len(kinds) == 0 {
		kinds = attemptResultOperatorDecisions
	}
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: kinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	return decisions.transport
}

func postAs(endpoint http.Handler, pattern, body string) *httptest.ResponseRecorder {
	request := httptest.NewRequest(http.MethodPost, pattern, strings.NewReader(body))
	stampWorkFact(request, "OFFSITE", "SYN-OFFSITE-01", "SYN-DEVICE-OFFSITE")
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, request)
	return recorder
}

func expectAnswer(t *testing.T, name string, answered *httptest.ResponseRecorder, status int, outcome string) {
	t.Helper()
	if answered.Code != status || registrationOutcome(t, answered) != outcome {
		t.Fatalf("%s：答 %d %s，want %d %s", name, answered.Code, answered.Body.String(), status, outcome)
	}
}

// Covers: `AT-TF-359`、`AT-TF-361`（生产装配半边）——`AT-TF-070` 之后的确定（UC-TF-006「尝试待确认」）的生产装配半边——一次派送尝试里两件对象都停在待确认，交付登记对它们都答
// `派送结果待确认`：parcel-1 由执行方在作业事实面后补成妥投，随后同一份交付登记走得过去（201 DELIVERY_REGISTERED）；parcel-2 由复核人在运营
// 决定面认定为无人签收，交付登记答`未生效`（那一格已有确定结果）。重放答已有更正，换说法答冲突；两件的待确认那一版都留在册上。
func TestAPendingDeliveryIsCorrectedThroughBothFacesAndTheDeliveryRegistrationFollowsTheCurrentResult(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registerAttemptResultAdmission(t, db)
	decisions := attemptResultOperatorForTest(t, db)
	work := workFactTransportForTest(t)
	segmentOps, err := buildSegmentOperations(db)
	if err != nil {
		t.Fatalf("装配段运营编排：%v", err)
	}
	attempts, err := buildDeliveryAttemptOrchestration(db)
	if err != nil {
		t.Fatalf("装配派送尝试编排：%v", err)
	}
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		t.Fatalf("装配交付编排：%v", err)
	}
	corrector, err := buildDeliveryAttemptResultCorrection(db)
	if err != nil {
		t.Fatalf("装配派送结果更正：%v", err)
	}
	attemptEndpoint := tfhttp.NewRecordDeliveryAttemptEndpoint(work, attempts)
	deliveryEndpoint := tfhttp.NewRegisterEffectiveDeliveryEndpoint(work, delivery)
	fromDevice := tfhttp.NewCorrectDeliveryAttemptResultEndpoint(work, corrector)
	fromReviewer := tfhttp.NewCorrectDeliveryAttemptResultEndpoint(decisions, corrector)

	const attemptBody = `{"task":"SYN-DISPATCH-90","attempt":"SYN-DELIVERY-ATTEMPT-90","executedBy":"SYN-COURIER-90",` +
		`"place":"SYN-PLACE/consignee-90","plannedFrom":"2026-09-25T09:00:00+08:00","plannedTo":"2026-09-25T12:00:00+08:00",` +
		`"arrivedAt":"2026-09-25T10:00:00+08:00","evidence":"SYN-EVIDENCE/arrival-90","objects":[` +
		`{"object":"SYN-PARCEL-90-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/signature-scan+address-geofence",` +
		`"basis":"SYN-CONFLICT/signature-vs-address","occurredAt":"2026-09-25T10:05:00+08:00"},` +
		`{"object":"SYN-PARCEL-90-02","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/photo-only",` +
		`"basis":"SYN-GAP/nobody-confirmed","occurredAt":"2026-09-25T10:06:00+08:00"}]}`
	deliver := func(object string) string {
		return `{"attempt":"SYN-DELIVERY-ATTEMPT-90","object":"` + object + `","method":"HANDED_TO_RECIPIENT",` +
			`"recipient":"SYN-RECIPIENT/consignee-90","proof":"SYN-POD/signature-` + object + `"}`
	}
	correct := func(object, outcome, basis string) string {
		body := `{"attempt":"SYN-DELIVERY-ATTEMPT-90","object":"` + object + `","outcome":"` + outcome + `",`
		if basis != "" {
			body += `"basis":"` + basis + `",`
		}
		return body + `"source":"SYN-SOURCE/review-90","occurredAt":"2026-09-25T10:08:00+08:00","correctedAt":"2026-09-25T15:00:00+08:00"}`
	}

	expectAnswer(t, "建派送任务", postAs(tfhttp.NewOpenDispatchTaskEndpoint(decisions, segmentOps.opener), "/transport-fulfillment-dispatch-task-registrations",
		`{"task":"SYN-DISPATCH-90","kind":"DELIVERY","objects":["SYN-PARCEL-90-01","SYN-PARCEL-90-02"],"place":"SYN-PLACE/consignee-90",`+
			`"windowFrom":"2026-09-25T09:00:00+08:00","windowTo":"2026-09-25T12:00:00+08:00",`+
			`"conditions":"SYN-CONDITION/signature-required","openedAt":"2026-09-24T20:00:00+08:00"}`),
		http.StatusCreated, "DISPATCH_TASK_OPENED")
	expectAnswer(t, "登记两件待确认", postAs(attemptEndpoint, "/transport-fulfillment/delivery-attempts", attemptBody),
		http.StatusCreated, "ATTEMPT_RECORDED")
	for _, object := range []string{"SYN-PARCEL-90-01", "SYN-PARCEL-90-02"} {
		expectAnswer(t, "更正之前的交付 "+object, postAs(deliveryEndpoint, "/transport-fulfillment/deliveries", deliver(object)),
			http.StatusOK, "DELIVERY_RESULT_PENDING_CONFIRMATION")
	}

	// 执行方在作业事实面后补：parcel-1 更正成妥投。
	corrected := postAs(fromDevice, "/transport-fulfillment/delivery-attempt-result-corrections", correct("SYN-PARCEL-90-01", "DELIVERED", ""))
	expectAnswer(t, "作业事实面更正成妥投", corrected, http.StatusCreated, "RESULT_CORRECTED")
	if !strings.Contains(corrected.Body.String(), `"gap":"SYN-CONFLICT/signature-vs-address"`) {
		t.Fatalf("答复没有回指被更正的待确认那一版：%s", corrected.Body.String())
	}
	expectAnswer(t, "更正成妥投之后的交付", postAs(deliveryEndpoint, "/transport-fulfillment/deliveries", deliver("SYN-PARCEL-90-01")),
		http.StatusCreated, "DELIVERY_REGISTERED")

	// 复核人在运营决定面认定：parcel-2 更正成无人签收，交付登记答未生效、不答待确认。
	expectAnswer(t, "运营决定面认定无人签收",
		postAs(fromReviewer, "/transport-fulfillment-delivery-attempt-result-confirmations", correct("SYN-PARCEL-90-02", "NO_ONE_TO_RECEIVE", "SYN-REASON/nobody-home")),
		http.StatusCreated, "RESULT_CORRECTED")
	expectAnswer(t, "认定之后的交付", postAs(deliveryEndpoint, "/transport-fulfillment/deliveries", deliver("SYN-PARCEL-90-02")),
		http.StatusOK, "NOT_EFFECTIVE")

	expectAnswer(t, "同内容重放",
		postAs(fromDevice, "/transport-fulfillment/delivery-attempt-result-corrections", correct("SYN-PARCEL-90-01", "DELIVERED", "")),
		http.StatusOK, "EXISTING_CORRECTION")
	expectAnswer(t, "换说法",
		postAs(fromReviewer, "/transport-fulfillment-delivery-attempt-result-confirmations", correct("SYN-PARCEL-90-01", "WRONG_ADDRESS", "SYN-REASON/other")),
		http.StatusOK, "SOURCE_CONFLICT")
	expectAnswer(t, "指名不在这次尝试里的对象",
		postAs(fromDevice, "/transport-fulfillment/delivery-attempt-result-corrections", correct("SYN-PARCEL-90-03", "DELIVERED", "")),
		http.StatusOK, "SOURCE_NOT_ACCEPTED")

	// 重放首次提交的待确认尝试仍答已有结果、不当作冲突：首次提交的结果行一个字没改。
	expectAnswer(t, "重放首次提交", postAs(attemptEndpoint, "/transport-fulfillment/delivery-attempts", attemptBody),
		http.StatusOK, "EXISTING_RESULT")

	var decidedBy *string
	rows, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for object, want := range map[string]string{"SYN-PARCEL-90-01": "", "SYN-PARCEL-90-02": "SYN-OPERATOR-01"} {
		if err := rows.QueryRow(t.Context(),
			`SELECT decided_by FROM transport_fulfillment.delivery_attempt_result_correction
			  WHERE attempt_ref = 'SYN-DELIVERY-ATTEMPT-90' AND object_ref = $1`, object).Scan(&decidedBy); err != nil {
			t.Fatalf("读更正行 %s：%v", object, err)
		}
		got := ""
		if decidedBy != nil {
			got = *decidedBy
		}
		if want == "" && got != "" || want != "" && !strings.HasSuffix(got, "#"+want) {
			t.Fatalf("%s 的操作证据 = %q；作业事实面应留空，运营决定面应记提交操作者（…#%s）", object, got, want)
		}
	}
}

// Covers: `AT-TF-357`、`AT-TF-358`、`AT-TF-360`、`AT-TF-361`（生产装配半边）——UC-TF-002 步骤 6A 与 6B 在更正之后的生产装配半边——一次到访三件对象都停在待确认：parcel-1 在作业事实面更正成客户不在，当前有效
// 结果（失败尝试费读的那条）是更正出来的失败、首次提交的行仍是待确认；parcel-2 在运营决定面认定为揽收到手并点名段，经对象级揽收登记形成场外揽收
// （同（对象，尝试）一行）、进声明的段、入队一封 `offsite-pickup.registered` 信封交 parcel-shipment；parcel-3 的更正带了控制依据却是失败，
// 答`来源未受理`。三件之外，更正读不动、未持有授予的调用方都不落任何行。
func TestAPendingPickupIsCorrectedThroughBothFacesAndTheCorrectedPickupEntersTheSegment(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	work := workFactTransportForTest(t)
	registerAttemptResultAdmission(t, db)
	decisions := attemptResultOperatorForTest(t, db)
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	corrector, err := buildPickupAttemptResultCorrection(db)
	if err != nil {
		t.Fatalf("装配揽收结果更正：%v", err)
	}
	attemptEndpoint := tfhttp.NewPerformOffsitePickupEndpoint(work, controlFacts.pickupAttempt)
	fromDevice := tfhttp.NewCorrectPickupAttemptResultEndpoint(work, corrector)
	fromReviewer := tfhttp.NewCorrectPickupAttemptResultEndpoint(decisions, corrector)

	const visit = `{"sourceId":"SYN-DEVICE-OFFSITE/attempt-91","task":"SYN-TASK-91","attempt":"SYN-ATTEMPT-91",` +
		`"executedBy":"SYN-COURIER-91","place":"SYN-PLACE/shipper-dock-91","plannedFrom":"2026-09-24T08:00:00+08:00",` +
		`"plannedTo":"2026-09-24T10:00:00+08:00","arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-91",` +
		`"objects":[` +
		`{"object":"SYN-PARCEL-91-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/vehicle-arrival","basis":"SYN-GAP/no-receipt","occurredAt":"2026-09-24T08:45:00+08:00"},` +
		`{"object":"SYN-PARCEL-91-02","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/driver-scan","basis":"SYN-GAP/no-signature","occurredAt":"2026-09-24T08:46:00+08:00"},` +
		`{"object":"SYN-PARCEL-91-03","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/photo","basis":"SYN-GAP/unclear","occurredAt":"2026-09-24T08:47:00+08:00"}]}`
	correct := func(object, tail string) string {
		return `{"sourceId":"SYN-DEVICE-OFFSITE/attempt-91","object":"` + object + `",` + tail
	}

	expectAnswer(t, "登记三件待确认", postAs(attemptEndpoint, "/transport-fulfillment/offsite-pickup-attempts", visit),
		http.StatusCreated, "ATTEMPT_RECORDED")

	expectAnswer(t, "作业事实面更正成客户不在",
		postAs(fromDevice, "/transport-fulfillment/offsite-pickup-attempt-result-corrections", correct("SYN-PARCEL-91-01",
			`"outcome":"CUSTOMER_ABSENT","basis":"SYN-REASON/partner-confirms-absent","source":"SYN-SOURCE/partner-late-status",`+
				`"occurredAt":"2026-09-24T08:50:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`)),
		http.StatusCreated, "RESULT_CORRECTED")

	pickedUp := correct("SYN-PARCEL-91-02",
		`"outcome":"PICKED_UP","source":"SYN-SOURCE/partner-late-receipt","control":"SYN-CONTROL/receipt-91-02",`+
			`"occurredAt":"2026-09-24T08:48:00+08:00","correctedAt":"2026-09-24T14:05:00+08:00","segment":"SYN-SEGMENT-91"}`)
	reviewed := postAs(fromReviewer, "/transport-fulfillment-pickup-attempt-result-confirmations", pickedUp)
	expectAnswer(t, "运营决定面认定揽收到手", reviewed, http.StatusCreated, "RESULT_CORRECTED")
	if !strings.Contains(reviewed.Body.String(), `"pickupVersion":"`) || strings.Contains(reviewed.Body.String(), "segmentEntryRefusal") ||
		strings.Contains(reviewed.Body.String(), "segmentContinuationReference") {
		t.Fatalf("更正成揽收到手的答复 = %s；应带场外揽收的版本、进段没有拒绝也没有欠账", reviewed.Body.String())
	}

	expectAnswer(t, "失败带了控制依据",
		postAs(fromDevice, "/transport-fulfillment/offsite-pickup-attempt-result-corrections", correct("SYN-PARCEL-91-03",
			`"outcome":"GOODS_NOT_READY","basis":"SYN-REASON/x","control":"SYN-CONTROL/oops","source":"SYN-SOURCE/s",`+
				`"occurredAt":"2026-09-24T08:50:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`)),
		http.StatusOK, "SOURCE_NOT_ACCEPTED")
	expectAnswer(t, "揽收到手重放", postAs(fromReviewer, "/transport-fulfillment-pickup-attempt-result-confirmations", pickedUp),
		http.StatusOK, "EXISTING_CORRECTION")
	expectAnswer(t, "揽收到手换控制依据",
		postAs(fromReviewer, "/transport-fulfillment-pickup-attempt-result-confirmations",
			strings.Replace(pickedUp, "receipt-91-02", "receipt-91-02-OTHER", 1)),
		http.StatusOK, "SOURCE_CONFLICT")

	// 读回：首次提交仍是待确认、当前有效结果是更正出来的；场外揽收一行、段里有它、信封一封。
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01")
	attempts, err := tfpostgres.NewPickupAttempts(db)
	if err != nil {
		t.Fatal(err)
	}
	key := tfports.PickupAttemptKey{TenantID: tenant, SourceID: "SYN-DEVICE-OFFSITE/attempt-91"}
	original, _, err := attempts.FindByKey(context.Background(), key)
	if err != nil {
		t.Fatalf("按键读回首次提交：%v", err)
	}
	for _, result := range original.Results {
		if !result.Outcome().PendingConfirmation() {
			t.Fatalf("%s 的首次提交被改写成 %q", result.Object(), result.Outcome())
		}
	}
	effective, _, err := attempts.FindEffectiveByKey(context.Background(), key)
	if err != nil {
		t.Fatalf("读当前有效结果：%v", err)
	}
	outcomes := map[string]tfdomain.AttemptObjectOutcome{}
	for _, result := range effective.Results {
		outcomes[result.Object().String()] = result.Outcome()
	}
	if outcomes["SYN-PARCEL-91-01"] != tfdomain.CustomerAbsent || outcomes["SYN-PARCEL-91-02"] != tfdomain.ObjectPickedUp ||
		outcomes["SYN-PARCEL-91-03"] != tfdomain.PickupPendingConfirmation {
		t.Fatalf("当前有效结果 = %v", outcomes)
	}

	reader, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var rows, enqueued int
	if err := reader.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup
		  WHERE tenant_id = 'SYN-TENANT-01' AND object_ref = 'SYN-PARCEL-91-02' AND attempt_ref = 'SYN-ATTEMPT-91'`).Scan(&rows); err != nil || rows != 1 {
		t.Fatalf("场外揽收行数 = %d err=%v, want 1（重放与换说法都没有多登一份）", rows, err)
	}
	if err := reader.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'transport-fulfillment.offsite-pickup.registered' AND subject = $1`,
		"SYN-PARCEL-91-02/SYN-ATTEMPT-91").Scan(&enqueued); err != nil || enqueued != 1 {
		t.Fatalf("入队的 offsite-pickup.registered 信封 = %d err=%v, want 1", enqueued, err)
	}
	if err := reader.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup WHERE object_ref IN ('SYN-PARCEL-91-01', 'SYN-PARCEL-91-03')`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("失败与仍待确认的对象形成了场外揽收：%d err=%v", rows, err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	segment, found, err := segments.FindByKey(context.Background(), tfports.FulfillmentSegmentKey{
		TenantID: tenant, Segment: mustValue(t, tfdomain.NewFulfillmentSegmentReference, "SYN-SEGMENT-91")})
	if err != nil || !found {
		t.Fatalf("声明的段读不回：found=%v err=%v", found, err)
	}
	if _, joined := segment.Segment.ParticipationFor(mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-91-02")); !joined {
		t.Fatal("更正成揽收到手的对象没有进声明的段")
	}
}

// Covers: `AT-TF-361`（生产装配半边）——ADR-0297 决定二——运营决定面按决定种类授予：只持有揽收认定的人认定不了派送，反过来也一样，各答 403；没有令牌答 401；这几口都不落任何更正行。
func TestTheAttemptResultConfirmationDecisionsAreGrantedPerSide(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	pickupCorrector, err := buildPickupAttemptResultCorrection(db)
	if err != nil {
		t.Fatal(err)
	}
	deliveryCorrector, err := buildDeliveryAttemptResultCorrection(db)
	if err != nil {
		t.Fatal(err)
	}
	registerAttemptResultAdmission(t, db)
	pickupOnly := attemptResultOperatorForTest(t, db, accessidentity.DecisionPickupAttemptResultConfirmation)
	deliveryOnly := attemptResultOperatorForTest(t, db, accessidentity.DecisionDeliveryAttemptResultConfirmation)
	pickupBody := `{"sourceId":"s","object":"o","outcome":"CUSTOMER_ABSENT","basis":"b","source":"x",` +
		`"occurredAt":"2026-09-24T08:50:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`
	deliveryBody := `{"attempt":"a","object":"o","outcome":"DELIVERED","source":"x",` +
		`"occurredAt":"2026-09-24T08:50:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`

	if got := postAs(tfhttp.NewCorrectDeliveryAttemptResultEndpoint(pickupOnly, deliveryCorrector), "/x", deliveryBody).Code; got != http.StatusForbidden {
		t.Fatalf("只持有揽收认定的人认定派送 = %d, want 403", got)
	}
	if got := postAs(tfhttp.NewCorrectPickupAttemptResultEndpoint(deliveryOnly, pickupCorrector), "/x", pickupBody).Code; got != http.StatusForbidden {
		t.Fatalf("只持有派送认定的人认定揽收 = %d, want 403", got)
	}
	// 持有自己那一种：越过 Intake，指名一个不存在的尝试答`来源未受理`——形成了的业务答案，不是授予问题。
	expectAnswer(t, "持有自己那一种", postAs(tfhttp.NewCorrectPickupAttemptResultEndpoint(pickupOnly, pickupCorrector), "/x", pickupBody),
		http.StatusOK, "SOURCE_NOT_ACCEPTED")
	expectAnswer(t, "派送持有自己那一种", postAs(tfhttp.NewCorrectDeliveryAttemptResultEndpoint(deliveryOnly, deliveryCorrector), "/x", deliveryBody),
		http.StatusOK, "SOURCE_NOT_ACCEPTED")

	request := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader(pickupBody))
	recorder := httptest.NewRecorder()
	tfhttp.NewCorrectPickupAttemptResultEndpoint(pickupOnly, pickupCorrector).ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("没有令牌 = %d, want 401", recorder.Code)
	}
}

// unwritableCorrections 让更正册读得动、写不进：对象级揽收登记已写在更正追加之前，追加失败时事务壳必须把登记一并撤回。
type unwritableCorrections struct {
	tfports.PickupResultCorrections
}

func (unwritableCorrections) Save(context.Context, tfports.PickupResultCorrectionRecord) (tfports.PickupResultCorrectionSaveOutcome, error) {
	return tfports.PickupResultCorrectionSaveOutcomeInvalid, errors.New("the correction register is down")
}

// Covers: `AT-TF-357`（生产装配半边）——更正成揽收到手时对象级揽收登记写在更正追加之前：追加不出（更正册写不进）答`未决`，事务壳整笔回滚，
// 于是没有留下一份没有更正回指的场外揽收、没有进段、没有入队的信封；册恢复之后同一份更正照常成立。
func TestACorrectionThatCannotBeAppendedLeavesNoPickupBehind(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	work := workFactTransportForTest(t)
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	real, err := tfpostgres.NewPickupResultCorrections(db)
	if err != nil {
		t.Fatal(err)
	}
	broken, err := newPickupAttemptResultCorrection(db, unwritableCorrections{real})
	if err != nil {
		t.Fatal(err)
	}
	healthy, err := buildPickupAttemptResultCorrection(db)
	if err != nil {
		t.Fatal(err)
	}
	const visit = `{"sourceId":"SYN-DEVICE-OFFSITE/attempt-92","task":"SYN-TASK-92","attempt":"SYN-ATTEMPT-92",` +
		`"executedBy":"SYN-COURIER-92","place":"SYN-PLACE/shipper-dock-92","plannedFrom":"2026-09-24T08:00:00+08:00",` +
		`"plannedTo":"2026-09-24T10:00:00+08:00","arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-92",` +
		`"objects":[{"object":"SYN-PARCEL-92-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/vehicle-arrival",` +
		`"basis":"SYN-GAP/no-receipt","occurredAt":"2026-09-24T08:45:00+08:00"}]}`
	const pickedUp = `{"sourceId":"SYN-DEVICE-OFFSITE/attempt-92","object":"SYN-PARCEL-92-01","outcome":"PICKED_UP",` +
		`"source":"SYN-SOURCE/partner-late-receipt","control":"SYN-CONTROL/receipt-92","occurredAt":"2026-09-24T08:48:00+08:00",` +
		`"correctedAt":"2026-09-24T14:05:00+08:00","segment":"SYN-SEGMENT-92"}`
	expectAnswer(t, "登记待确认", postAs(tfhttp.NewPerformOffsitePickupEndpoint(work, controlFacts.pickupAttempt), "/transport-fulfillment/offsite-pickup-attempts", visit),
		http.StatusCreated, "ATTEMPT_RECORDED")

	expectAnswer(t, "追加不出", postAs(tfhttp.NewCorrectPickupAttemptResultEndpoint(work, broken), "/x", pickedUp),
		http.StatusOK, "CORRECTION_UNDECIDED")
	reader, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var pickups, envelopes, segments int
	if err := reader.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.offsite_pickup WHERE object_ref = 'SYN-PARCEL-92-01'`).Scan(&pickups); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'transport-fulfillment.offsite-pickup.registered' AND subject = $1`,
		"SYN-PARCEL-92-01/SYN-ATTEMPT-92").Scan(&envelopes); err != nil {
		t.Fatal(err)
	}
	if err := reader.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.actual_fulfillment_segment WHERE segment_ref = 'SYN-SEGMENT-92'`).Scan(&segments); err != nil {
		t.Fatal(err)
	}
	if pickups != 0 || envelopes != 0 || segments != 0 {
		t.Fatalf("追加失败之后留下 场外揽收 %d 信封 %d 段 %d；事务壳没有整笔回滚", pickups, envelopes, segments)
	}

	expectAnswer(t, "册恢复之后", postAs(tfhttp.NewCorrectPickupAttemptResultEndpoint(work, healthy), "/x", pickedUp),
		http.StatusCreated, "RESULT_CORRECTED")
}

// Covers: `AT-TF-363`（生产装配半边）——评审实测的复现场景在生产装配下——一次到访里 parcel-1 与 parcel-2 都停在待确认；之后 parcel-1 凭另一次尝试的对象级揽收登记（发生时刻
// 晚于这次到访）进了 SEG-A2；晚到的更正把两件都更正成揽收到手并声明 SEG-A1：parcel-2 照常进 SEG-A1（对照，没有更晚的控制事实），parcel-1 的场外揽收照登、信封照入队，
// 但进段那一半一笔不写——答 201 RESULT_CORRECTED 并带 `segmentEntryRefusal: LATER_CONTROL_FACTS`、不带欠账，对象在场的段仍只有 SEG-A2；之后 parcel-1 的交付登记
// 走得过去、结束它在 SEG-A2 的参与，不因多段在场整笔回滚。同内容重放答已有更正。
func TestALateCorrectionToAPickupLeavesTheObjectInItsOneSegmentSoItsDeliveryStillRegisters(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	work := workFactTransportForTest(t)
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	corrector, err := buildPickupAttemptResultCorrection(db)
	if err != nil {
		t.Fatalf("装配揽收结果更正：%v", err)
	}
	delivery, err := buildDeliveryOrchestration(db)
	if err != nil {
		t.Fatalf("装配交付编排：%v", err)
	}
	attemptEndpoint := tfhttp.NewPerformOffsitePickupEndpoint(work, controlFacts.pickupAttempt)
	registrationEndpoint := tfhttp.NewRegisterOffsitePickupEndpoint(work, controlFacts.pickupRegistration)
	fromDevice := tfhttp.NewCorrectPickupAttemptResultEndpoint(work, corrector)

	// 时刻都排在交付夹具的到场时刻之前：参与的终点不得早于起点。
	at := func(offset time.Duration) string { return deliveryAttemptArrivedAt.Add(offset).Format(time.RFC3339) }
	visit := fmt.Sprintf(`{"sourceId":"SYN-DEVICE-OFFSITE/attempt-93","task":"SYN-TASK-93","attempt":"SYN-ATTEMPT-93-A1",`+
		`"executedBy":"SYN-COURIER-93","place":"SYN-PLACE/shipper-dock-93","plannedFrom":%q,"plannedTo":%q,"arrivedAt":%q,`+
		`"evidence":"SYN-EVIDENCE/visit-93","objects":[`+
		`{"object":"SYN-PARCEL-93-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/vehicle-arrival","basis":"SYN-GAP/no-receipt","occurredAt":%q},`+
		`{"object":"SYN-PARCEL-93-02","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/driver-scan","basis":"SYN-GAP/no-signature","occurredAt":%q}]}`,
		at(-7*time.Hour), at(-5*time.Hour), at(-6*time.Hour+40*time.Minute), at(-6*time.Hour+45*time.Minute), at(-6*time.Hour+46*time.Minute))
	otherAttempt := fmt.Sprintf(`{"object":"SYN-PARCEL-93-01","task":"SYN-TASK-93-A2","attempt":"SYN-ATTEMPT-93-A2","place":"SYN-PLACE/shipper-dock-93",`+
		`"control":"SYN-CONTROL/receipt-93-A2","executedBy":"SYN-COURIER-93","occurredAt":%q,"segment":"SYN-SEG-A2"}`, at(-4*time.Hour))
	correct := func(object string) string {
		return fmt.Sprintf(`{"sourceId":"SYN-DEVICE-OFFSITE/attempt-93","object":%q,"outcome":"PICKED_UP","source":"SYN-SOURCE/partner-late-receipt",`+
			`"control":"SYN-CONTROL/receipt-93-A1-%s","occurredAt":%q,"correctedAt":%q,"segment":"SYN-SEG-A1"}`,
			object, object, at(-6*time.Hour+50*time.Minute), at(-1*time.Hour))
	}

	expectAnswer(t, "登记两件待确认", postAs(attemptEndpoint, "/transport-fulfillment/offsite-pickup-attempts", visit), http.StatusCreated, "ATTEMPT_RECORDED")
	expectAnswer(t, "parcel-1 凭另一次尝试进 SEG-A2", postAs(registrationEndpoint, "/transport-fulfillment/offsite-pickups", otherAttempt),
		http.StatusCreated, "PICKUP_REGISTERED")

	late := postAs(fromDevice, "/transport-fulfillment/offsite-pickup-attempt-result-corrections", correct("SYN-PARCEL-93-01"))
	expectAnswer(t, "晚到的更正", late, http.StatusCreated, "RESULT_CORRECTED")
	if !strings.Contains(late.Body.String(), `"segmentEntryRefusal":"LATER_CONTROL_FACTS"`) ||
		!strings.Contains(late.Body.String(), `"pickupVersion":"`) || strings.Contains(late.Body.String(), "segmentContinuationReference") {
		t.Fatalf("晚到的更正答复 = %s；应带场外揽收的版本与 LATER_CONTROL_FACTS，不带欠账", late.Body.String())
	}
	normal := postAs(fromDevice, "/transport-fulfillment/offsite-pickup-attempt-result-corrections", correct("SYN-PARCEL-93-02"))
	expectAnswer(t, "没有更晚控制事实的更正", normal, http.StatusCreated, "RESULT_CORRECTED")
	if strings.Contains(normal.Body.String(), "segmentEntryRefusal") || strings.Contains(normal.Body.String(), "segmentContinuationReference") {
		t.Fatalf("没有更晚控制事实的更正答复 = %s；进段应当顺利", normal.Body.String())
	}
	expectAnswer(t, "晚到的更正重放", postAs(fromDevice, "/transport-fulfillment/offsite-pickup-attempt-result-corrections", correct("SYN-PARCEL-93-01")),
		http.StatusOK, "EXISTING_CORRECTION")

	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01")
	object := mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-93-01")
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	active, err := segments.FindActiveSegments(context.Background(), tenant, object)
	if err != nil || len(active) != 1 || active[0].Segment.String() != "SYN-SEG-A2" {
		t.Fatalf("更正之后 parcel-1 在场的段 = %v err=%v, want 只有 SYN-SEG-A2", active, err)
	}
	seg1, found, err := segments.FindByKey(context.Background(), tfports.FulfillmentSegmentKey{
		TenantID: tenant, Segment: mustValue(t, tfdomain.NewFulfillmentSegmentReference, "SYN-SEG-A1")})
	if err != nil || !found {
		t.Fatalf("SYN-SEG-A1 读不回（parcel-2 该把它立起来）：found=%v err=%v", found, err)
	}
	if _, joined := seg1.Segment.ParticipationFor(object); joined {
		t.Fatal("晚到的更正把 parcel-1 带进了 SYN-SEG-A1：对象两段同时在场")
	}
	if _, joined := seg1.Segment.ParticipationFor(mustValue(t, tfdomain.NewCarriedObjectReference, "SYN-PARCEL-93-02")); !joined {
		t.Fatal("没有更晚控制事实的更正，parcel-2 没有进 SYN-SEG-A1")
	}

	reader, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var pickups, envelopes int
	if err := reader.QueryRow(t.Context(),
		`SELECT count(*) FROM transport_fulfillment.offsite_pickup WHERE object_ref = 'SYN-PARCEL-93-01' AND attempt_ref = 'SYN-ATTEMPT-93-A1'`).Scan(&pickups); err != nil || pickups != 1 {
		t.Fatalf("A1 的场外揽收 = %d err=%v, want 1：揽收事实是真的，段那一半拦下不该连它一起拦", pickups, err)
	}
	if err := reader.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = 'transport-fulfillment.offsite-pickup.registered' AND subject = $1`,
		"SYN-PARCEL-93-01/SYN-ATTEMPT-93-A1").Scan(&envelopes); err != nil || envelopes != 1 {
		t.Fatalf("A1 入队的 offsite-pickup.registered 信封 = %d err=%v, want 1", envelopes, err)
	}

	// 对象此后的交付：按对象找段恰一条，登得出、结束它在 SEG-A2 的参与。多段在场时这里是整笔回滚的 ErrObjectActiveInSeveralSegments。
	seedDeliveryAttemptRow(t, pool, "SYN-TENANT-01", "SYN-ATTEMPT-93-DELIVERY", []string{"SYN-PARCEL-93-01"})
	seedDeliveryResultRow(t, pool, "SYN-TENANT-01", "SYN-ATTEMPT-93-DELIVERY", "SYN-PARCEL-93-01", "DELIVERED", nil)
	command := registerDeliveryCommand(t, "SYN-ATTEMPT-93-DELIVERY", "SYN-PARCEL-93-01")
	command.TenantID = tenant
	delivered, err := delivery.Register(t.Context(), command)
	if err != nil || delivered.Outcome() != tfapp.DeliveryRegistered || delivered.ParticipationEnd() != tfapp.ParticipationEndedNow {
		t.Fatalf("更正之后的交付 outcome=%v participationEnd=%v err=%v, want DELIVERY_REGISTERED 与 PARTICIPATION_ENDED",
			delivered.Outcome(), delivered.ParticipationEnd(), err)
	}
}

// Covers: `AT-TF-361`（生产装配半边）——运营决定面的租户只取信封（同 code/95 的 `TestTheWiredHandlingIsOpenOnlyToItsOwnGrantWithinTheOwnTenant` 那一类探针）——租户 B 的复核人持有两种认定授予，
// 在请求上自报租户 A（`?tenant=`、`X-Reported-Tenant`）去认定租户 A 的待确认结果：认定落在租户 B 里，找不到那一格，答`来源未受理`，租户 A 的两张更正册一行没写；载荷夹带租户答 400；
// 租户 A 自己持授予的复核人认定同一格才成立（对照，证明被拒的不是请求本身有毛病）。
func TestAnOperatorOfAnotherTenantCannotConfirmThisTenantsAttemptResultsByNamingItInTheRequest(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	work := workFactTransportForTest(t)
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	pickupCorrector, err := buildPickupAttemptResultCorrection(db)
	if err != nil {
		t.Fatal(err)
	}
	deliveryCorrector, err := buildDeliveryAttemptResultCorrection(db)
	if err != nil {
		t.Fatal(err)
	}

	// 租户 A（作业事实面的合成租户 SYN-TENANT-01）有一格待确认的揽收与一格待确认的派送。
	const visit = `{"sourceId":"SYN-DEVICE-OFFSITE/attempt-94","task":"SYN-TASK-94","attempt":"SYN-ATTEMPT-94",` +
		`"executedBy":"SYN-COURIER-94","place":"SYN-PLACE/shipper-dock-94","plannedFrom":"2026-09-24T08:00:00+08:00",` +
		`"plannedTo":"2026-09-24T10:00:00+08:00","arrivedAt":"2026-09-24T08:40:00+08:00","evidence":"SYN-EVIDENCE/visit-94",` +
		`"objects":[{"object":"SYN-PARCEL-94-01","outcome":"PENDING_CONFIRMATION","source":"SYN-SOURCE/vehicle-arrival",` +
		`"basis":"SYN-GAP/no-receipt","occurredAt":"2026-09-24T08:45:00+08:00"}]}`
	expectAnswer(t, "登记待确认揽收", postAs(tfhttp.NewPerformOffsitePickupEndpoint(work, controlFacts.pickupAttempt),
		"/transport-fulfillment/offsite-pickup-attempts", visit), http.StatusCreated, "ATTEMPT_RECORDED")
	seedDeliveryAttemptRow(t, pool, "SYN-TENANT-01", "SYN-ATTEMPT-94-D", []string{"SYN-PARCEL-94-02"})
	if _, err := pool.Exec(context.Background(),
		`INSERT INTO transport_fulfillment.delivery_attempt_result
			(tenant_id, attempt_ref, object_ref, outcome, basis, source_ref, occurred_at)
		 VALUES ('SYN-TENANT-01', 'SYN-ATTEMPT-94-D', 'SYN-PARCEL-94-02', 'PENDING_CONFIRMATION',
		         'SYN-GAP/signature-vs-address', 'SYN-SOURCE/signature-scan', $1)`,
		deliveryAttemptArrivedAt.Add(15*time.Minute)); err != nil {
		t.Fatalf("植入待确认的派送结果：%v", err)
	}

	// 两位复核人都持有两种认定授予：一位在租户 B，一位在租户 A。
	operators, err := aipg.NewOperatorRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		t.Fatal(err)
	}
	const foreigner, own = "SYN-REVIEWER-OF-TENANT-B", "SYN-REVIEWER-OF-TENANT-A"
	confirmations := []accessidentity.DecisionKind{accessidentity.DecisionPickupAttemptResultConfirmation, accessidentity.DecisionDeliveryAttemptResultConfirmation}
	registerOperatorIn(t, db, operators, "SYN-TENANT-B", foreigner, interval, confirmations...)
	registerOperatorIn(t, db, operators, "SYN-TENANT-01", own, interval, confirmations...)
	minter, err := buildOperatorMinter(tokenSubjectVerifier{}, operators, admitAll{})
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	pickupConfirmation := tfhttp.NewCorrectPickupAttemptResultEndpoint(decisions.transport, pickupCorrector)
	deliveryConfirmation := tfhttp.NewCorrectDeliveryAttemptResultEndpoint(decisions.transport, deliveryCorrector)

	const pickupBody = `{"sourceId":"SYN-DEVICE-OFFSITE/attempt-94","object":"SYN-PARCEL-94-01","outcome":"CUSTOMER_ABSENT",` +
		`"basis":"SYN-REASON/partner-confirms-absent","source":"SYN-SOURCE/partner-late-status",` +
		`"occurredAt":"2026-09-24T08:50:00+08:00","correctedAt":"2026-09-24T14:00:00+08:00"}`
	deliveryBody := `{"attempt":"SYN-ATTEMPT-94-D","object":"SYN-PARCEL-94-02","outcome":"DELIVERED","source":"SYN-SOURCE/address-review-confirmed",` +
		`"occurredAt":"` + deliveryAttemptArrivedAt.Add(20*time.Minute).Format(time.RFC3339) + `","correctedAt":"` + deliveryAttemptArrivedAt.Add(3*time.Hour).Format(time.RFC3339) + `"}`
	as := func(endpoint http.Handler, target, token, body string, header map[string]string) *httptest.ResponseRecorder {
		request := httptest.NewRequest(http.MethodPost, target, strings.NewReader(body))
		request.Header.Set("Authorization", "Bearer "+token)
		for name, value := range header {
			request.Header.Set(name, value)
		}
		recorder := httptest.NewRecorder()
		endpoint.ServeHTTP(recorder, request)
		return recorder
	}
	corrections := func() (pickups, deliveries int) {
		t.Helper()
		reader, err := db.ReadExecutor(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		if err := reader.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.pickup_attempt_result_correction`).Scan(&pickups); err != nil {
			t.Fatal(err)
		}
		if err := reader.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.delivery_attempt_result_correction`).Scan(&deliveries); err != nil {
			t.Fatal(err)
		}
		return pickups, deliveries
	}

	selfReported := map[string]string{"X-Reported-Tenant": "SYN-TENANT-01"}
	expectAnswer(t, "租户 B 自报租户 A 认定揽收",
		as(pickupConfirmation, "/transport-fulfillment-pickup-attempt-result-confirmations?tenant=SYN-TENANT-01", foreigner, pickupBody, selfReported),
		http.StatusOK, "SOURCE_NOT_ACCEPTED")
	expectAnswer(t, "租户 B 自报租户 A 认定派送",
		as(deliveryConfirmation, "/transport-fulfillment-delivery-attempt-result-confirmations?tenant=SYN-TENANT-01", foreigner, deliveryBody, selfReported),
		http.StatusOK, "SOURCE_NOT_ACCEPTED")
	smuggled := as(pickupConfirmation, "/transport-fulfillment-pickup-attempt-result-confirmations", foreigner,
		strings.Replace(pickupBody, `"object"`, `"tenantId":"SYN-TENANT-01","object"`, 1), nil)
	if smuggled.Code != http.StatusBadRequest || !strings.Contains(smuggled.Body.String(), "MALFORMED_REQUEST") {
		t.Fatalf("载荷夹带租户 = %d %s, want 400 MALFORMED_REQUEST", smuggled.Code, smuggled.Body.String())
	}
	if pickups, deliveries := corrections(); pickups != 0 || deliveries != 0 {
		t.Fatalf("租户 B 的复核人在租户 A 的册上落了更正：揽收 %d 派送 %d", pickups, deliveries)
	}

	// 对照：租户 A 自己的复核人认定同一格。
	expectAnswer(t, "租户 A 的复核人认定揽收",
		as(pickupConfirmation, "/transport-fulfillment-pickup-attempt-result-confirmations", own, pickupBody, nil), http.StatusCreated, "RESULT_CORRECTED")
	expectAnswer(t, "租户 A 的复核人认定派送",
		as(deliveryConfirmation, "/transport-fulfillment-delivery-attempt-result-confirmations", own, deliveryBody, nil), http.StatusCreated, "RESULT_CORRECTED")
	if pickups, deliveries := corrections(); pickups != 1 || deliveries != 1 {
		t.Fatalf("租户 A 自己认定之后：揽收 %d 派送 %d, want 各 1", pickups, deliveries)
	}
}
