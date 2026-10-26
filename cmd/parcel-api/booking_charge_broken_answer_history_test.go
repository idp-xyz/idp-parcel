package main

import (
	"net/http"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 票 demo-chain-gaps/177 写面的答法，生产装配真库面（S）——干线订舱被承运客户端接受之后，绕过答复编排往答复册直插一份
// 答复时刻在接受之后的候补：定局早于候补，库上的约束拦不住，答复史却凑不成。这时登订舱费答 500 NO_ANSWER_FORMED，不按那份接受
// 登、也不答未决，发生项与信封都不落；删掉那一行，同一请求照常答 201 REGISTERED——500 出自答复史，不出自别处的接线。
func TestABookingChargeOnABrokenAnswerHistoryFormsNoAnswer(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	registerSeededTrunkSchedule(t, db)
	endpoints := bookingChargeFacesForTest(t, db)

	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody("SYN-COMMISSION-TRUNK-DCG24", trunk.Declarations.SupplierAgreementBody.Supplier, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	mustPost(t, endpoints.booking, bookingSubmissionPath, trunkBookingBody(bookingChargeBooking, seededTrunkSchedule),
		http.StatusCreated, "BOOKING_SUBMITTED")
	if status, answer := postWithToken(t, endpoints.trunk, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer); status != http.StatusCreated ||
		answer["outcome"] != "BOOKING_ANSWERED" {
		t.Fatalf("干线承运客户端答接受 = %d %v", status, answer)
	}
	const late = "SYN-CARRIER-ANSWER-DCG177-LATE"
	if _, err := pool.Exec(t.Context(),
		`INSERT INTO transport_fulfillment.booking_answer
			(tenant_id, booking_id, acceptance_id, outcome, quantity, basis, decided_at, content_digest, recorded_at)
		 SELECT tenant_id, booking_id, $2, 'WAITLISTED', 0, 'SYN-DIRECT-INSERT', '2026-10-19T09:30:00Z', 'sha256:direct', now()
		   FROM transport_fulfillment.booking_request
		  WHERE booking_id = $1`,
		bookingChargeBooking, late); err != nil {
		t.Fatalf("直插接受之后的候补：%v", err)
	}

	bookingFee := `{"reason": "BOOKING", "booking": "` + bookingChargeBooking + `", "objects": ["` + bookingChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + bookingChargeParcel + `"}`
	status, answer := postWithToken(t, endpoints.charge, chargeOccurrencePath, operatorTokenForTests, bookingFee)
	if problem, _ := answer["error"].(map[string]any); status != http.StatusInternalServerError || problem["code"] != "NO_ANSWER_FORMED" ||
		answer["outcome"] != nil {
		t.Fatalf("答复史坏了登订舱费 = %d %v，want 500 NO_ANSWER_FORMED", status, answer)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 0 || envelopes != 0 {
		t.Fatalf("答复史坏了却落了 %d 行、%d 封", occurrences, envelopes)
	}

	if _, err := pool.Exec(t.Context(), `DELETE FROM transport_fulfillment.booking_answer WHERE acceptance_id = $1`, late); err != nil {
		t.Fatalf("删直插的那一行：%v", err)
	}
	mustPost(t, endpoints.charge, chargeOccurrencePath, bookingFee, http.StatusCreated, "REGISTERED")
}
