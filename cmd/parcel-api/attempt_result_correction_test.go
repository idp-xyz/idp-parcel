package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
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

// Covers: `AT-TF-070` 之后的确定（UC-TF-006「尝试待确认」）的生产装配半边——一次派送尝试里两件对象都停在待确认，交付登记对它们都答
// `派送结果待确认`：parcel-1 由执行方在作业事实面后补成妥投，随后同一份交付登记走得过去（201 DELIVERY_REGISTERED）；parcel-2 由复核人在运营
// 决定面认定为无人签收，交付登记改答`未生效`（它不再是待确认）。重放答已有更正，换说法答冲突；两件的待确认那一版都留在册上。
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

	// 复核人在运营决定面认定：parcel-2 更正成无人签收，交付登记不再答待确认，改答未生效。
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
	expectAnswer(t, "确定结果不再更正",
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

// Covers: UC-TF-002 步骤 6A 与 6B 在更正之后的生产装配半边——一次到访三件对象都停在待确认：parcel-1 在作业事实面更正成客户不在，当前有效
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

// Covers: ADR-0297 决定二——运营决定面按决定种类授予：只持有揽收认定的人认定不了派送，反过来也一样，各答 403；没有令牌答 401；这几口都不落任何更正行。
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
