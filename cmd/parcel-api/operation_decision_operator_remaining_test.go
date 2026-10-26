package main

import (
	"net/http"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证 ADR-0303 复核记录（票 demo-chain-gaps/180）的生产装配半边：订舱提交与取消、取消运输委托、履约段采用
// 运输委托、容量预占与释放、派送补拍与按册重问八口经操作者渠道「运营决定」能力面走到真编排，令牌认出的操作者随决定落进库里那一列
// （补拍与重问落进问询册）；同一份载荷换一位操作者再投，答复是各口既有的重放或已有格、库里那一列不变，回执里没有任何一位操作者。
// 与生产装配的差别只在令牌核验方与操作者册是替身。夹具全为合成（S 级）。

// twoOperatorDecisions 在一份库上登记给定决定种类的准入对照，交回挂着两位合成操作者的运营决定 Intake。
func twoOperatorDecisions(t *testing.T, db *bentopg.DB, kinds ...accessidentity.DecisionKind) *tfhttp.OperatorDecisionIntake {
	t.Helper()
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(twoOperatorVerifier{}, decisionRegister{kinds: kinds}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	return decisions.transport
}

// Covers: `AT-TF-348`、`AT-TF-349`、`AT-TF-350`、`AT-TF-351` 生产装配半边（真库）——订舱提交、取消订舱、取消运输委托与履约段采用运输委托
// 四口：首投的操作者落进各自那一列（订舱的提交人与取消人各记各的），另一位操作者原样再投答各口既有的已有格、那一列不变、回执里
// 没有操作者；订舱册读面带出订舱的提交人与取消人。
func TestTheBookingCancellationAndAdoptionDecisionsRecordTheirOperatorsAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	trunk := publishSeededSupplierAgreements(t, db)["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	registerSeededTrunkSchedule(t, db)
	decisions := twoOperatorDecisions(t, db,
		accessidentity.DecisionTransportCommissionSubmission,
		accessidentity.DecisionBookingRequestSubmission,
		accessidentity.DecisionBookingRequestCancellation,
		accessidentity.DecisionTransportCommissionCancellation,
		accessidentity.DecisionFulfillmentSegmentCommissionAdoption,
	)
	submitter, err := buildTransportCommissionSubmission(db)
	if err != nil {
		t.Fatalf("装配委托提交：%v", err)
	}
	bookings, err := buildBookingFaces(db)
	if err != nil {
		t.Fatalf("装配订舱：%v", err)
	}
	cancellations, err := buildCancellationFaces(db)
	if err != nil {
		t.Fatalf("装配两个取消口：%v", err)
	}
	adopter, err := buildSegmentCommissionAdoption(db)
	if err != nil {
		t.Fatalf("装配采用：%v", err)
	}
	commissionEndpoint := tfhttp.NewSubmitTransportCommissionEndpoint(decisions, submitter)

	for _, commission := range []string{"SYN-COMMISSION-OE-CANCEL", "SYN-COMMISSION-OE-ADOPT"} {
		if status, answer := postWithToken(t, commissionEndpoint, commissionSubmissionPath, firstOperatorToken,
			commissionBody(commission, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")); status != http.StatusCreated {
			t.Fatalf("委托 %s = %d %v", commission, status, answer)
		}
	}

	booking := `{"booking": "SYN-BOOKING-OE", "commission": "SYN-COMMISSION-OE-CANCEL", "schedule": "` + seededTrunkSchedule + `",
		"quantity": 120, "unit": "KG", "requestedAt": "2026-10-19T08:30:00Z"}`
	decideTwice(t, tfhttp.NewSubmitBookingRequestEndpoint(decisions, bookings), bookingSubmissionPath, booking,
		[2]any{http.StatusCreated, "BOOKING_SUBMITTED"}, [2]any{http.StatusOK, "EXISTING_BOOKING"})
	if got := recordedOperator(t, pool, `SELECT submitted_by FROM transport_fulfillment.booking_request
		 WHERE tenant_id = 'SYN-TENANT-01' AND booking_id = 'SYN-BOOKING-OE'`); got != firstOperatorRecorded {
		t.Fatalf("订舱提交人 = %q，want %q", got, firstOperatorRecorded)
	}

	bookingCancellation := `{"booking": "SYN-BOOKING-OE", "cancelledAt": "2026-10-19T09:00:00Z"}`
	decideTwice(t, tfhttp.NewCancelBookingRequestEndpoint(decisions, cancellations), bookingCancellationPath, bookingCancellation,
		[2]any{http.StatusCreated, "BOOKING_CANCELLED"}, [2]any{http.StatusOK, "EXISTING_CANCELLATION"})
	if got := recordedOperator(t, pool, `SELECT cancelled_by FROM transport_fulfillment.booking_request
		 WHERE tenant_id = 'SYN-TENANT-01' AND booking_id = 'SYN-BOOKING-OE'`); got != firstOperatorRecorded {
		t.Fatalf("订舱取消人 = %q，want %q", got, firstOperatorRecorded)
	}

	commissionCancellation := `{"commission": "SYN-COMMISSION-OE-CANCEL", "cancelledAt": "2026-10-19T09:30:00Z"}`
	decideTwice(t, tfhttp.NewCancelTransportCommissionEndpoint(decisions, cancellations), commissionCancellationPath, commissionCancellation,
		[2]any{http.StatusCreated, "COMMISSION_CANCELLED"}, [2]any{http.StatusOK, "EXISTING_CANCELLATION"})
	if got := recordedOperator(t, pool, `SELECT cancelled_by FROM transport_fulfillment.transport_commission
		 WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = 'SYN-COMMISSION-OE-CANCEL'`); got != firstOperatorRecorded {
		t.Fatalf("委托取消人 = %q，want %q", got, firstOperatorRecorded)
	}

	establishTrunkSegmentByHandover(t, db)
	decideTwice(t, tfhttp.NewAdoptSegmentCommissionEndpoint(decisions, adopter), adoptionPath,
		adoptionBody(trunkSegmentDCG25, "SYN-COMMISSION-OE-ADOPT", provider),
		[2]any{http.StatusCreated, "COMMISSION_ADOPTED"}, [2]any{http.StatusOK, "EXISTING_ADOPTION"})
	if got := recordedOperator(t, pool, `SELECT adopted_by FROM transport_fulfillment.segment_commission_adoption
		 WHERE tenant_id = 'SYN-TENANT-01' AND segment_ref = '`+trunkSegmentDCG25+`'`); got != firstOperatorRecorded {
		t.Fatalf("采用人 = %q，want %q", got, firstOperatorRecorded)
	}
	if got := recordedOperator(t, pool, `SELECT coalesce(cancelled_by, '<NULL>') FROM transport_fulfillment.transport_commission
		 WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = 'SYN-COMMISSION-OE-ADOPT'`); got != "<NULL>" {
		t.Fatalf("采用带出的开始不是一位操作者的取消，被采用的委托不该有取消人：%q", got)
	}

	catalogue, err := tfpostgres.NewReviewCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	rows, err := catalogue.ListBookingRequests(t.Context(), mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"), 10)
	if err != nil || len(rows) != 1 || rows[0].SubmittedBy != firstOperatorRecorded || rows[0].CancelledBy != firstOperatorRecorded {
		t.Fatalf("订舱册读回 = %+v err=%v，want 一行带提交人与取消人", rows, err)
	}
}

// Covers: `AT-TF-352`、`AT-TF-353` 生产装配半边（真库）——容量预占与容量释放两口：首投的操作者落进预占行、释放行，另一位操作者原样
// 再投答已有预占、已有释放，那一列不变、回执里没有操作者；之后别的预占与释放换存池（明细整组删了再插回），先到的预占人与释放人
// 原样带着。
func TestTheCapacityDecisionsRecordTheirOperatorsAgainstARealDatabase(t *testing.T) {
	pool, db := capacityDB(t)
	decisions := twoOperatorDecisions(t, db, accessidentity.DecisionCapacityReservation, accessidentity.DecisionCapacityRelease)
	capacity, err := buildCapacityDecisions(db)
	if err != nil {
		t.Fatalf("装配预占与释放编排：%v", err)
	}
	reserve := tfhttp.NewReserveCapacityEndpoint(decisions, capacity)
	release := tfhttp.NewReleaseCapacityEndpoint(decisions, capacity)
	const reservationPath, releasePath = "/transport-fulfillment-capacity-reservations", "/transport-fulfillment-capacity-releases"

	reservation := func(id string, quantity string) string {
		return `{"pool":"` + capacityPool + `","reservation":"` + id + `","quantity":` + quantity + `,"unit":"KG",` +
			`"source":{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-01/v1"},` +
			`"validUntil":"2026-10-21T12:00:00Z","reservedAt":"2026-10-19T08:00:00Z"}`
	}
	decideTwice(t, reserve, reservationPath, reservation("SYN-RES-OE-1", "1200"),
		[2]any{http.StatusCreated, "CAPACITY_RESERVED"}, [2]any{http.StatusOK, "EXISTING_RESERVATION"})
	if status, answer := postWithToken(t, reserve, reservationPath, secondOperatorToken, reservation("SYN-RES-OE-2", "300")); status != http.StatusCreated {
		t.Fatalf("第二份预占 = %d %v", status, answer)
	}

	releaseBody := `{"pool":"` + capacityPool + `","reservation":"SYN-RES-OE-1","release":"SYN-REL-OE-1","quantity":200,"unit":"KG",` +
		`"source":{"kind":"OPERATOR_DECLARATION","reference":"SYN-DEMAND-DECL-01/v2"},"releasedAt":"2026-10-19T09:00:00Z"}`
	decideTwice(t, release, releasePath, releaseBody,
		[2]any{http.StatusCreated, "CAPACITY_RELEASED"}, [2]any{http.StatusOK, "EXISTING_RELEASE"})

	if got := recordedOperator(t, pool, `SELECT reserved_by FROM transport_fulfillment.capacity_reservation
		 WHERE tenant_id = 'SYN-TENANT-01' AND reservation_id = 'SYN-RES-OE-1'`); got != firstOperatorRecorded {
		t.Fatalf("预占人 = %q，want %q（释放换存池之后仍是首投的那一位）", got, firstOperatorRecorded)
	}
	if got := recordedOperator(t, pool, `SELECT reserved_by FROM transport_fulfillment.capacity_reservation
		 WHERE tenant_id = 'SYN-TENANT-01' AND reservation_id = 'SYN-RES-OE-2'`); got != operatorIssuerForEvidence+"#SYN-OPERATOR-B" {
		t.Fatalf("第二份预占的预占人 = %q，want 另一位操作者各记各的", got)
	}
	if got := recordedOperator(t, pool, `SELECT released_by FROM transport_fulfillment.capacity_release
		 WHERE tenant_id = 'SYN-TENANT-01' AND release_id = 'SYN-REL-OE-1'`); got != firstOperatorRecorded {
		t.Fatalf("释放人 = %q，want %q", got, firstOperatorRecorded)
	}
}

// Covers: `AT-TF-354`、`AT-TF-355` 生产装配半边（真库）——派送补拍与按册重问两口：每一次按下在问询册记一行，按的人是令牌认出的操作者；
// 两位操作者对同一件各补拍一次、各按一次重问都留下各自的一行，答复与不带这一格时相同、回执里没有操作者；执行器经补拍或重问形成的
// 任务不记建任务人；按册重问一次一行，不是逐件一行。
func TestTheDeliveryDispatchAsksRecordWhoPressedThemAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	controlFacts, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	owners := newTaskReferenceOwners()
	owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = true })
	triggerer, err := deliveryDispatchTriggerOver(db, owners, owners, owners)
	if err != nil {
		t.Fatalf("装配派送发起执行器：%v", err)
	}
	reasker, err := deliveryDispatchReaskOver(db, triggerer)
	if err != nil {
		t.Fatalf("装配按册重问：%v", err)
	}
	decisions := twoOperatorDecisions(t, db, accessidentity.DecisionDeliveryDispatchReask)
	trigger := tfhttp.NewTriggerDeliveryDispatchEndpoint(decisions, triggerer)
	reask := tfhttp.NewReaskDeliveryDispatchesEndpoint(decisions, reasker)
	rig := &taskReferenceRig{controlFacts: controlFacts, db: db}
	rig.enter(t, "SYN-PARCEL-OE-A", reaskSegment, "SYN-HANDOVER-SCOPE-OE", "SYN-HANDOVER-RESULT/OE-A/v1", tfdomain.SegmentServesFinalDelivery)
	rig.enter(t, "SYN-PARCEL-OE-B", reaskSegment, "SYN-HANDOVER-SCOPE-OE", "SYN-HANDOVER-RESULT/OE-B/v1", tfdomain.SegmentServesFinalDelivery)

	const triggerPath, reaskPath = "/transport-fulfillment-delivery-dispatch-triggers", "/transport-fulfillment-delivery-dispatch-reasks"
	triggerBody := `{"segment":"` + reaskSegment + `","object":"SYN-PARCEL-OE-A","occurredAt":"2026-10-20T09:00:00Z"}`
	first := decideTwice(t, trigger, triggerPath, triggerBody,
		[2]any{http.StatusOK, "REQUIREMENT_MISSING"}, [2]any{http.StatusOK, "REQUIREMENT_MISSING"})
	if strings.Contains(first, "askedBy") || strings.Contains(first, "triggeredBy") {
		t.Fatalf("补拍回执带了操作证据：%s", first)
	}

	owners.change(func(owners *taskReferenceOwners) { owners.placeMissing = false })
	reaskBody := `{"scope":"ALL","limit":10}`
	status, answer := postWithToken(t, reask, reaskPath, firstOperatorToken, reaskBody)
	if status != http.StatusOK || answer["outcome"] != "PENDING_DELIVERY_DISPATCHES_REASKED" || len(answer["items"].([]any)) != 1 {
		t.Fatalf("第一位操作者按册重问 = %d %v", status, answer)
	}
	status, raw := postWithTokenRaw(reask, reaskPath, secondOperatorToken, reaskBody)
	if status != http.StatusOK || !strings.Contains(raw, `"items":[]`) || strings.Contains(raw, "SYN-OPERATOR") {
		t.Fatalf("第二位操作者再按 = %d %s，want 册上已空、回执不含操作者", status, raw)
	}

	asks, err := tfpostgres.NewDeliveryDispatchAsks(db)
	if err != nil {
		t.Fatal(err)
	}
	listed, err := asks.ListAsks(t.Context(), mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"), 10)
	if err != nil || len(listed) != 4 {
		t.Fatalf("问询册 = %+v err=%v，want 四次按下（两次补拍、两次重问，重问不逐件记）", listed, err)
	}
	pressedBy := []string{}
	for _, ask := range listed {
		pressedBy = append(pressedBy, ask.Kind.String()+"|"+ask.AskedBy.String())
	}
	want := []string{
		"REASK|" + operatorIssuerForEvidence + "#SYN-OPERATOR-B",
		"REASK|" + firstOperatorRecorded,
		"TRIGGER|" + operatorIssuerForEvidence + "#SYN-OPERATOR-B",
		"TRIGGER|" + firstOperatorRecorded,
	}
	if strings.Join(pressedBy, ";") != strings.Join(want, ";") {
		t.Fatalf("问询册由近及远 = %v，want %v", pressedBy, want)
	}

	var formed, withOpener int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*), count(opened_by) FROM transport_fulfillment.dispatch_task WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&formed, &withOpener); err != nil {
		t.Fatal(err)
	}
	if formed != 1 || withOpener != 0 {
		t.Fatalf("重问形成的任务 %d 项、带建任务人 %d 项，want 1 项、都不带", formed, withOpener)
	}
}
