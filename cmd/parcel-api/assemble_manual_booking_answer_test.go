package main

import (
	"net/http"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
)

// Covers: dox/decisions.md「产品形态」——订舱答复的手工录入口在真库上经生产装配走一遍：干线委托与订舱经生产两口提交；
// 操作员照承运方的邮件录入「接受」答 201，答复落在同一张答复册上、答复方是委托的服务提供方，另有一条留痕；原样再录
// 答已有答复；订舱已有定局之后承运客户端再交一份答冲突；订舱不在册如实答、不往挡回册里记。合成标识只记 `S`。
func TestAManuallyEnteredBookingAnswerRunsThroughTheProductionAssembly(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	registerSeededTrunkSchedule(t, db)
	endpoints := bookingFacesForTest(t, db)
	if status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, "presented.operator.token",
		commissionBody("SYN-COMMISSION-TRUNK-DCG24", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")); status != http.StatusCreated {
		t.Fatalf("干线委托 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, endpoints.booking, bookingSubmissionPath, "presented.operator.token",
		trunkBookingBody("SYN-BOOKING-TRUNK-DCG24", seededTrunkSchedule)); status != http.StatusCreated {
		t.Fatalf("订舱 = %d %v", status, answer)
	}

	kind := accessidentity.DecisionBookingAnswerEntry
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kind)
	admission, err := buildAdmissionScope(db, isolatedSelfAuthority)
	if err != nil {
		t.Fatal(err)
	}
	minter, err := buildOperatorMinter(acceptingVerifier{}, decisionRegister{kinds: []accessidentity.DecisionKind{kind}}, admission)
	if err != nil {
		t.Fatal(err)
	}
	decisions, err := buildOperatorDecisionIntakes(minter, unwiredDecisionTargets{}, unwiredDecisionTargets{})
	if err != nil {
		t.Fatal(err)
	}
	faces, err := buildBookingFaces(db, documentNumberingComponents(t, db, docnumber.SecretTable{}))
	if err != nil {
		t.Fatal(err)
	}
	const path = "/transport-fulfillment-booking-answer-manual-entries"
	manual := tfhttp.NewEnterBookingAnswerEndpoint(decisions.transport, faces, systemClock{}.Now)
	body := `{"booking":"SYN-BOOKING-TRUNK-DCG24","answer":"SYN-MAIL-ANSWER-1","outcome":"ACCEPTED","quantity":120,"decidedAt":"2026-10-19T09:00:00Z","original":"SYN-CARRIER-MAIL-20261019"}`

	if status, answer := postWithToken(t, manual, path, "presented.operator.token",
		`{"booking":"SYN-BOOKING-NOWHERE","answer":"SYN-MAIL-ANSWER-1","outcome":"ACCEPTED","quantity":120,"decidedAt":"2026-10-19T09:00:00Z","original":"SYN-CARRIER-MAIL-20261019"}`); status != http.StatusOK ||
		answer["outcome"] != "BOOKING_NOT_FOUND" || len(bounceRows(t, pool)) != 0 {
		t.Fatalf("订舱不在册 = %d %v；挡回册 %v", status, answer, bounceRows(t, pool))
	}
	if status, answer := postWithToken(t, manual, path, "presented.operator.token", body); status != http.StatusCreated || answer["outcome"] != "BOOKING_ANSWERED" {
		t.Fatalf("人工录入接受 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, manual, path, "presented.operator.token", body); status != http.StatusOK || answer["outcome"] != "EXISTING_ANSWER" {
		t.Fatalf("原样再录 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, endpoints.trunk, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer); status != http.StatusOK ||
		answer["outcome"] != "ANSWER_CONFLICT" {
		t.Fatalf("已有定局后承运客户端再交一份 = %d %v", status, answer)
	}

	var answers, trails int
	var enteredBy, original string
	if err := pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM transport_fulfillment.booking_answer WHERE tenant_id = 'SYN-TENANT-01' AND booking_id = 'SYN-BOOKING-TRUNK-DCG24'),
		        count(*), min(entered_by), min(original_ref)
		   FROM transport_fulfillment.booking_answer_manual_entry
		  WHERE tenant_id = 'SYN-TENANT-01' AND booking_id = 'SYN-BOOKING-TRUNK-DCG24' AND acceptance_ref = 'BKGMANUAL/SYN-MAIL-ANSWER-1'`,
	).Scan(&answers, &trails, &enteredBy, &original); err != nil {
		t.Fatalf("读答复与留痕：%v", err)
	}
	if answers != 1 || trails != 1 || original != "SYN-CARRIER-MAIL-20261019" || enteredBy == "" {
		t.Fatalf("答复 %d 份、留痕 %d 条（%q / %q）", answers, trails, enteredBy, original)
	}
}
