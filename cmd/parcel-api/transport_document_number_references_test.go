package main

import (
	"net/http"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证委托、订舱的单据号在换版之后不变、订舱撞号整笔回滚，以及引用委托的各口答复带上委托号（ADR-0355
// 决定三、八、十一；票 document-numbering/09 评审修订）。

func objectString(answer map[string]any, object, key string) string {
	body, _ := answer[object].(map[string]any)
	value, _ := body[key].(string)
	return value
}

// Covers: AT-TF-371（生产装配真库面：换版不换号）——委托指名 v1 取得委托号，换版到 v2 之后原样重交委托答已有委托、回显原号，库里那
// 一列不变，序列只取过一个值。
func TestACommissionKeepsItsNumberAcrossAnAgreementRevision(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	registerShanghaiNumberingTimeZone(t, db)
	faces := revisionFacesForTest(t, db, accessidentity.DecisionTransportCommissionSubmission, accessidentity.DecisionTransportCommissionAgreementRevision)
	trunk := publishSeededSupplierAgreements(t, db)["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	body := commissionBody(revisionCommission, provider, trunk.ObjectID, "v1", revisionSubmittedAtTS)
	status, answer := postWithToken(t, faces.commission, commissionSubmissionPath, operatorTokenForTests, body)
	number := objectString(answer, "commission", "documentNumber")
	if status != http.StatusCreated || number != "TC-"+shanghaiMonth(t)+"-00001" {
		t.Fatalf("委托 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, faces.revision, revisionPath, operatorTokenForTests,
		revisionBody(trunk.ObjectID, "v2", revisionSubmittedAtTS, "SYN-DECISION-REV-DOCNUM09")); status != http.StatusCreated {
		t.Fatalf("换版 = %d %v", status, answer)
	}
	if _, replay := postWithToken(t, faces.commission, commissionSubmissionPath, operatorTokenForTests, body); replay["outcome"] != "EXISTING_COMMISSION" ||
		objectString(replay, "commission", "documentNumber") != number {
		t.Fatalf("换版之后重交 = %v，want 原号 %s", replay, number)
	}
	var stored string
	if err := pool.QueryRow(t.Context(), `SELECT document_number FROM transport_fulfillment.transport_commission
		WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = $1`, revisionCommission).Scan(&stored); err != nil || stored != number {
		t.Fatalf("库里的委托号 = %q %v", stored, err)
	}
	if got := sequenceValue(t, pool, "TRANSPORT_COMMISSION"); got != 1 {
		t.Fatalf("委托序列取了 %d 个值，want 1", got)
	}
}

// Covers: AT-TF-373（生产装配真库面：订舱撞号）——订舱的编号规则换成与已用号整段重叠的格式：订舱提交答 200 NUMBERING_RULE_COLLISION
// 并指名订舱的编号规则、不回显订舱，订舱不落、序列取过的流水随回滚释放；委托不受影响。
func TestAWiredBookingThatCollidesIsRolledBackWithItsSequence(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	trunk := publishSeededSupplierAgreements(t, db)["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	registerSeededTrunkSchedule(t, db)
	endpoints := bookingFacesForTest(t, db)
	registerShanghaiNumberingTimeZone(t, db)
	if _, err := documentNumberingComponents(t, db, docnumber.SecretTable{}).Registrations.RegisterRule(t.Context(), docnumber.RegisterRuleCommand{
		Tenant: "SYN-TENANT-01", Type: "BOOKING", Template: "BKX-{seq:5}", EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
	}); err != nil {
		t.Fatalf("登订舱规则：%v", err)
	}
	if status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, operatorTokenForTests,
		commissionBody("SYN-COMMISSION-TRUNK-DCG24", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")); status != http.StatusCreated {
		t.Fatalf("委托 = %d %v", status, answer)
	}
	status, answer := postWithToken(t, endpoints.booking, bookingSubmissionPath, operatorTokenForTests, trunkBookingBody("SYN-BOOKING-FIRST", seededTrunkSchedule))
	if status != http.StatusCreated || objectString(answer, "booking", "documentNumber") != "BKX-00001" {
		t.Fatalf("首个订舱 = %d %v", status, answer)
	}
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.booking_request
		 SELECT (jsonb_populate_record(b, jsonb_build_object(
		            'booking_id', b.booking_id || '-USED-' || n,
		            'document_number', 'BKX-' || lpad(n::text, 5, '0')))).*
		   FROM transport_fulfillment.booking_request AS b, generate_series(2, 21) AS n
		  WHERE b.tenant_id = 'SYN-TENANT-01' AND b.booking_id = 'SYN-BOOKING-FIRST'`); err != nil {
		t.Fatalf("做出已用号段：%v", err)
	}
	before := sequenceValue(t, pool, "BOOKING")

	status, answer = postWithToken(t, endpoints.booking, bookingSubmissionPath, operatorTokenForTests, trunkBookingBody("SYN-BOOKING-COLLIDED", seededTrunkSchedule))
	if status != http.StatusOK || answer["outcome"] != "NUMBERING_RULE_COLLISION" || answer["numberingRule"] != "BOOKING" || answer["booking"] != nil {
		t.Fatalf("订舱撞号 = %d %v", status, answer)
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.booking_request
		WHERE booking_id = 'SYN-BOOKING-COLLIDED'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("撞号的订舱落了 %d 行 %v", rows, err)
	}
	if after := sequenceValue(t, pool, "BOOKING"); after != before {
		t.Fatalf("撞号之后订舱序列 %d → %d：取过的流水没随回滚释放", before, after)
	}
}

// Covers: AT-TF-374（生产装配真库面）——引用委托的各口答复带上委托号：段采用成立与原样再采用的采用体带 commissionDocumentNumber，回显的
// 委托带 documentNumber；运输已开始时取消委托回显的委托带号；解除成立与原样重交的解除体带委托号；撤回之后取消委托答 201，回显的委托
// 带号。原有的委托标识格照旧。
func TestTheAnswersThatReferToACommissionCarryItsNumber(t *testing.T) {
	db := migratedChargeDB(t)
	registerShanghaiNumberingTimeZone(t, db)
	endpoints := correctionFacesForTest(t, db)
	trunk := publishSeededSupplierAgreements(t, db)["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	submitted := mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody(adoptedChargeCommiss, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	number := objectString(submitted, "commission", "documentNumber")
	if number != "TC-"+shanghaiMonth(t)+"-00001" {
		t.Fatalf("委托号 = %q", number)
	}
	saveEndedTrunkSegment(t, db)

	adopted := mustPost(t, endpoints.adoption, adoptionPath, adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider),
		http.StatusCreated, "COMMISSION_ADOPTED")
	replayed := mustPost(t, endpoints.adoption, adoptionPath, adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider),
		http.StatusOK, "EXISTING_ADOPTION")
	for name, answer := range map[string]map[string]any{"采用成立": adopted, "原样再采用": replayed} {
		if objectString(answer, "adoption", "commissionDocumentNumber") != number || objectString(answer, "adoption", "commission") != adoptedChargeCommiss {
			t.Fatalf("%s的采用体 = %v", name, answer)
		}
	}
	if objectString(adopted, "commission", "documentNumber") != number {
		t.Fatalf("采用成立回显的委托 = %v", adopted)
	}

	cancellation := `{"commission": "` + adoptedChargeCommiss + `", "cancelledAt": "2026-10-19T13:00:00Z"}`
	started := mustPost(t, endpoints.cancellation, commissionCancellationPath, cancellation, http.StatusOK, "TRANSPORT_ALREADY_STARTED")
	if objectString(started, "commission", "documentNumber") != number {
		t.Fatalf("运输已开始回显的委托 = %v", started)
	}

	withdrawn := mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1),
		http.StatusCreated, "ADOPTION_WITHDRAWN")
	again := mustPost(t, endpoints.withdrawal, adoptionWithdrawalPath, withdrawalBody(adoptedChargeSegment, adoptedChargeCommiss, 1),
		http.StatusOK, "EXISTING_WITHDRAWAL")
	for name, answer := range map[string]map[string]any{"解除成立": withdrawn, "原样重交": again} {
		if objectString(answer, "withdrawal", "commissionDocumentNumber") != number {
			t.Fatalf("%s的解除体 = %v", name, answer)
		}
	}
	if objectString(withdrawn, "commission", "documentNumber") != number {
		t.Fatalf("解除成立回显的委托 = %v", withdrawn)
	}

	cancelled := mustPost(t, endpoints.cancellation, commissionCancellationPath, cancellation, http.StatusCreated, "COMMISSION_CANCELLED")
	if objectString(cancelled, "commission", "documentNumber") != number || objectString(cancelled, "commission", "cancelledAt") == "" {
		t.Fatalf("取消回显的委托 = %v", cancelled)
	}
}

// Covers: AT-TF-374（生产装配真库面：总单登记）——新登的总单按键读回作答，所引委托在册时带它的号格；所引订舱不在册时那一格为
// nil，答复只带标识。
func TestANewlyRegisteredMasterDocumentAnswersWithItsReferencedNumbers(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	registerShanghaiNumberingTimeZone(t, db)
	trunk := publishSeededSupplierAgreements(t, db)["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	submitted := mustPost(t, commissionSubmissionEndpointForTest(t, db), commissionSubmissionPath,
		commissionBody("SYN-COMMISSION-MAWB", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	registrar, err := buildMasterDocumentRegistration(db)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := registrar.Register(t.Context(), tfapp.RegisterMasterDocumentCommand{
		TenantID: mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"), Document: "SYN-MAWB-DOCNUM09", Version: "SYN-MDV-DOCNUM09-1",
		Issuer: "SYN-CARRIER-X", Scope: "SYN-LANE-1", Commission: "SYN-COMMISSION-MAWB", Booking: "SYN-BOOKING-NOT-REGISTERED",
	})
	if err != nil || registered.Outcome() != tfapp.MasterDocumentRegistered {
		t.Fatalf("登总单 = %v %v", registered.Outcome(), err)
	}
	record, _ := registered.Record()
	if record.CommissionDocumentNumber == nil {
		t.Fatal("新登的总单没带所引委托的号格")
	}
	if number, _ := record.CommissionDocumentNumber.Number(); number.String() != objectString(submitted, "commission", "documentNumber") {
		t.Fatalf("所引委托号 = %s，want %v", number, submitted)
	}
	if record.BookingDocumentNumber != nil {
		t.Fatal("不在册的订舱不该带号格")
	}
}

// Covers: AT-TF-374（生产装配真库面：读回失败）——新登的一版落了而按键读回出错：登记口交错误、装配壳整笔回滚，那一版不落库；不答
// REGISTRATION_UNDECIDED 而把它照常提交。读回错要是事务里的 SQL 错误，事务已中止、照常提交也落不下，分不出两种写法，所以这里让
// 读回在装回那一步出错：拆掉订舱表那道原因 CHECK，落一行原因认不得的订舱，总单引用它，连读所引订舱号格时报错，事务仍健康。
func TestAMasterDocumentWhoseReadBackFailsIsRolledBack(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	registrar, err := buildMasterDocumentRegistration(db)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `
		ALTER TABLE transport_fulfillment.booking_request DROP CONSTRAINT booking_request_document_number_absence_known;
		INSERT INTO transport_fulfillment.booking_request
			(tenant_id, booking_id, commission_id, schedule_id, quantity, unit_ref, requested_at, content_digest, recorded_at, document_number_absence)
		VALUES ('SYN-TENANT-01', 'SYN-BOOKING-UNREADABLE', 'SYN-COMMISSION-ANY', 'SYN-SCHED-ANY', 1, 'KG', now(), 'digest', now(), 'SOMETHING_ELSE')`); err != nil {
		t.Fatalf("落一行装不回的订舱：%v", err)
	}
	result, err := registrar.Register(t.Context(), tfapp.RegisterMasterDocumentCommand{
		TenantID: mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"), Document: "SYN-MAWB-READBACK", Version: "SYN-MDV-READBACK-1",
		Issuer: "SYN-CARRIER-X", Scope: "SYN-LANE-1", Booking: "SYN-BOOKING-UNREADABLE",
	})
	if err == nil {
		t.Fatalf("读回失败应交错误，得到 %s", result.Outcome())
	}
	var rows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.carrier_master_document
		WHERE master_document_ref = 'SYN-MAWB-READBACK'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("读回失败的那一版落了 %d 行 %v，want 整笔回滚", rows, err)
	}
}
