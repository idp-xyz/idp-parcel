package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	aipg "go.idp.xyz/idp-parcel/internal/accessidentity/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 证订舱与取消发生项、以及段已采用委托时从委托取采购上下文的生产装配（票 demo-chain-gaps/28，
// ADR-0207 文末补记）：干线协议照种子发布批经 PC 发布（干线 v3 声明订舱与取消按订舱计）、干线那一班照种子登记；委托、订舱、
// 取消订舱、段采用与发生项登记经操作者渠道「运营决定」能力面，承运答复经集成客户端族，都走到真编排。与生产装配的差别只在
// 两族的令牌核验方与操作者册是替身。证据层级 S。

const (
	chargeOccurrencePath  = "/transport-fulfillment-charge-occurrence-registrations"
	bookingChargeParcel   = "SYN-PARCEL-DCG23-01"
	bookingChargeBooking  = "SYN-BOOKING-TRUNK-DCG24"
	bookingChargeTrunkV3  = "v3"
	adoptedChargeSegment  = "SYN-SEG-SZX-SIN-DCG28"
	adoptedChargeParcel   = "SYN-PARCEL-DCG28-01"
	adoptedChargeCommiss  = "SYN-COMMISSION-TRUNK-DCG28"
	lastMileCarrierParty  = "SYN-PARTY-CARRIER-LM-01"
	operatorTokenForTests = "presented.operator.token"
)

type bookingChargeEndpoints struct {
	commission    http.Handler
	booking       http.Handler
	cancelBooking http.Handler
	adoption      http.Handler
	charge        http.Handler
	trunk         http.Handler
}

// bookingChargeFacesForTest 装生产的委托提交、订舱两口、取消订舱、段采用与发生项登记编排；操作者持有其中五种决定，干线
// 承运客户端绑干线承运方。
func bookingChargeFacesForTest(t *testing.T, db *bentopg.DB) bookingChargeEndpoints {
	t.Helper()
	kinds := []accessidentity.DecisionKind{
		accessidentity.DecisionTransportCommissionSubmission,
		accessidentity.DecisionBookingRequestSubmission,
		accessidentity.DecisionBookingRequestCancellation,
		accessidentity.DecisionFulfillmentSegmentCommissionAdoption,
		accessidentity.DecisionTransportChargeOccurrenceRegistration,
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
	registerCarrierAnswerAdmission(t, db)
	registerCarrierClient(t, db, "SYN-CLIENT-CARRIER-TRUNK-01", "SYN-PARTY-CARRIER-TRUNK-01")
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
	cancellations, err := buildCancellationFaces(db)
	if err != nil {
		t.Fatalf("装配取消口：%v", err)
	}
	adopter, err := buildSegmentCommissionAdoption(db)
	if err != nil {
		t.Fatalf("装配采用：%v", err)
	}
	registrar, err := buildChargeOccurrenceRegistration(db)
	if err != nil {
		t.Fatalf("装配发生项登记：%v", err)
	}
	clients, err := aipg.NewIntegrationClientRegistry(db)
	if err != nil {
		t.Fatal(err)
	}
	clientMinter, err := buildIntegrationClientMinter(subjectClientVerifier{subject: "SYN-CLIENT-CARRIER-TRUNK-01"}, clients, admission)
	if err != nil {
		t.Fatal(err)
	}
	clientIntakes, err := buildIntegrationClientIntakes(clientMinter)
	if err != nil {
		t.Fatal(err)
	}
	return bookingChargeEndpoints{
		commission:    tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, commission),
		booking:       tfhttp.NewSubmitBookingRequestEndpoint(decisions.transport, faces),
		cancelBooking: tfhttp.NewCancelBookingRequestEndpoint(decisions.transport, cancellations),
		adoption:      tfhttp.NewAdoptSegmentCommissionEndpoint(decisions.transport, adopter),
		charge:        tfhttp.NewRegisterChargeOccurrenceEndpoint(decisions.transport, registrar),
		trunk:         tfhttp.NewAnswerBookingEndpoint(clientIntakes.transport, faces),
	}
}

func migratedChargeDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

func mustPost(t *testing.T, endpoint http.Handler, path, body string, wantStatus int, wantOutcome string) map[string]any {
	t.Helper()
	status, answer := postWithToken(t, endpoint, path, operatorTokenForTests, body)
	if status != wantStatus || answer["outcome"] != wantOutcome {
		t.Fatalf("%s = %d %v，want %d %s", path, status, answer, wantStatus, wantOutcome)
	}
	return answer
}

// Covers: AT-TF-215（S，票 demo-chain-gaps/28 演示判据）、AT-TF-206、AT-TF-208、AT-TF-210 的生产装配半边——
//   - 演示：干线委托指名种子里的 v3，对种子物化的那一班订舱，合成干线承运客户端答 ACCEPTED；随后登订舱发生项、载荷不带服务方
//     与协议，答 201 REGISTERED：数量 1、单位 BOOKING，协议快照 v3、法人取自委托，依据是那一份接受，信封同笔一封；原样重登
//     答 200 EXISTING、不多一封。
//   - 服务方报成末端承运商答 200 CONFLICT / PROCUREMENT_CONTEXT_CONFLICT，回显委托（服务方与 v3）。
//   - 委托未开始时取消这次订舱答 201，再登取消发生项答 201，业务时间是取消时刻；订舱那一项照旧在册，共两项两封。
func TestABookingAcceptedOnTheSeededTrunkRegistersItsBookingAndCancellationOccurrences(t *testing.T) {
	db := migratedChargeDB(t)
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	if trunk.Version != bookingChargeTrunkV3 {
		t.Fatalf("种子里干线协议的最新一版 = %s，want %s", trunk.Version, bookingChargeTrunkV3)
	}
	registerSeededTrunkSchedule(t, db)
	endpoints := bookingChargeFacesForTest(t, db)
	provider := trunk.Declarations.SupplierAgreementBody.Supplier

	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody("SYN-COMMISSION-TRUNK-DCG24", provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	mustPost(t, endpoints.booking, bookingSubmissionPath, trunkBookingBody(bookingChargeBooking, seededTrunkSchedule),
		http.StatusCreated, "BOOKING_SUBMITTED")
	if status, answer := postWithToken(t, endpoints.trunk, bookingAnswerPath, "presented.client.token", trunkAcceptedAnswer); status != http.StatusCreated ||
		answer["outcome"] != "BOOKING_ANSWERED" {
		t.Fatalf("干线承运客户端答接受 = %d %v", status, answer)
	}

	bookingFee := `{"reason": "BOOKING", "booking": "` + bookingChargeBooking + `", "objects": ["` + bookingChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + bookingChargeParcel + `"}`
	answer := mustPost(t, endpoints.charge, chargeOccurrencePath, bookingFee, http.StatusCreated, "REGISTERED")
	occurrence, _ := answer["occurrence"].(map[string]any)
	if occurrence["reason"] != "BOOKING" || occurrence["quantity"] != float64(1) || occurrence["unit"] != "BOOKING" ||
		occurrence["agreement"] != trunk.ObjectID+"/"+bookingChargeTrunkV3 || occurrence["legalEntity"] != "SYN-LE-01" ||
		occurrence["provider"] != provider || occurrence["scope"] != bookingChargeBooking ||
		occurrence["factBasis"] != "CARRIER-ACCEPTANCE/"+bookingChargeBooking+"/SYN-CARRIER-ANSWER-DCG24" ||
		occurrence["occurredAt"] != "2026-10-19T09:00:00Z" {
		t.Fatalf("订舱发生项 = %v", occurrence)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 1 || envelopes != 1 {
		t.Fatalf("订舱发生项之后 %d 行、%d 封，want 1 / 1", occurrences, envelopes)
	}
	mustPost(t, endpoints.charge, chargeOccurrencePath, bookingFee, http.StatusOK, "EXISTING")
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 1 || envelopes != 1 {
		t.Fatalf("重登之后 %d 行、%d 封，want 1 / 1", occurrences, envelopes)
	}

	conflicting := `{"reason": "BOOKING", "booking": "` + bookingChargeBooking + `", "objects": ["` + bookingChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + bookingChargeParcel + `", "provider": "` + lastMileCarrierParty + `"}`
	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, conflicting, http.StatusOK, "CONFLICT")
	commission, _ := answer["commission"].(map[string]any)
	if answer["detail"] != "PROCUREMENT_CONTEXT_CONFLICT" || commission["provider"] != provider ||
		commission["agreementVersion"] != bookingChargeTrunkV3 || commission["agreement"] != trunk.ObjectID+"/"+bookingChargeTrunkV3 {
		t.Fatalf("服务方不一致 = %v", answer)
	}

	mustPost(t, endpoints.cancelBooking, bookingCancellationPath,
		`{"booking": "`+bookingChargeBooking+`", "cancelledAt": "2026-10-19T10:00:00Z"}`, http.StatusCreated, "BOOKING_CANCELLED")
	cancellationFee := `{"reason": "CANCELLATION", "booking": "` + bookingChargeBooking + `", "objects": ["` + bookingChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + bookingChargeParcel + `"}`
	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, cancellationFee, http.StatusCreated, "REGISTERED")
	occurrence, _ = answer["occurrence"].(map[string]any)
	if occurrence["reason"] != "CANCELLATION" || occurrence["occurredAt"] != "2026-10-19T10:00:00Z" ||
		occurrence["factBasis"] != "BOOKING-CANCELLATION/"+bookingChargeBooking || occurrence["unit"] != "BOOKING" {
		t.Fatalf("取消发生项 = %v", occurrence)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 2 || envelopes != 2 {
		t.Fatalf("取消发生项之后 %d 行、%d 封，want 2 / 2", occurrences, envelopes)
	}
}

// saveEndedTrunkSegment 在真库上立一个干线段：一件包裹 09:00 凭 SZX 交接入场、12:00 凭 SIN 交接离场。
func saveEndedTrunkSegment(t *testing.T, db *bentopg.DB) {
	t.Helper()
	segment, err := tfdomain.RehydrateActualFulfillmentSegment(tfdomain.RehydrateActualFulfillmentSegmentSpec{
		TenantID: chargeValue(t, tfdomain.NewTenantID, chargeTenant),
		Segment:  chargeValue(t, tfdomain.NewFulfillmentSegmentReference, adoptedChargeSegment),
		Participations: []tfdomain.RehydrateParticipationSpec{{
			Object:     chargeValue(t, tfdomain.NewCarriedObjectReference, adoptedChargeParcel),
			EntryKind:  tfdomain.EnteredByTransportHandover,
			EntryBasis: chargeValue(t, tfdomain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SZX-DCG28"),
			EnteredAt:  time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC),
			EndKind:    tfdomain.EndedByNextHandover,
			EndBasis:   chargeValue(t, tfdomain.NewParticipationBasisReference, "TRANSPORT-HANDOVER/SYN-HO-SIN-DCG28"),
			EndedAt:    time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		}},
	})
	if err != nil {
		t.Fatalf("段：%v", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	var outcome tfports.SegmentSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = segments.Save(txCtx, tfports.FulfillmentSegmentRecord{
			Key:        tfports.FulfillmentSegmentKey{TenantID: segment.TenantID(), Segment: segment.Segment()},
			Segment:    segment,
			RecordedAt: time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC),
		})
		return saveErr
	}); err != nil || outcome != tfports.SegmentSaved {
		t.Fatalf("段落库 = %v %v", outcome, err)
	}
}

// Covers: AT-TF-211 的生产装配半边——干线段采用了指名 v3 的干线委托，参与结束之后登实际履约、载荷不带服务方与协议，答 201：
// 服务方、协议快照与法人取自委托，数量按 v3 的按件计；给了另一家服务方答 200 CONFLICT 回显委托，不多一行。
func TestAnAdoptedTrunkSegmentRegistersActualFulfillmentFromTheCommission(t *testing.T) {
	db := migratedChargeDB(t)
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	endpoints := bookingChargeFacesForTest(t, db)

	mustPost(t, endpoints.commission, commissionSubmissionPath,
		commissionBody(adoptedChargeCommiss, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z"),
		http.StatusCreated, "COMMISSION_SUBMITTED")
	saveEndedTrunkSegment(t, db)
	mustPost(t, endpoints.adoption, adoptionPath, adoptionBody(adoptedChargeSegment, adoptedChargeCommiss, provider),
		http.StatusCreated, "COMMISSION_ADOPTED")

	omitted := `{"reason": "ACTUAL_FULFILLMENT", "segment": "` + adoptedChargeSegment + `", "objects": ["` + adoptedChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + adoptedChargeParcel + `"}`
	answer := mustPost(t, endpoints.charge, chargeOccurrencePath, omitted, http.StatusCreated, "REGISTERED")
	occurrence, _ := answer["occurrence"].(map[string]any)
	if occurrence["provider"] != provider || occurrence["agreement"] != trunk.ObjectID+"/"+trunk.Version ||
		occurrence["legalEntity"] != "SYN-LE-01" || occurrence["unit"] != "PIECE" || occurrence["occurredAt"] != "2026-10-19T12:00:00Z" {
		t.Fatalf("从委托取采购上下文的实际履约 = %v", occurrence)
	}

	conflicting := `{"reason": "ACTUAL_FULFILLMENT", "segment": "` + adoptedChargeSegment + `", "objects": ["` + adoptedChargeParcel + `"],` +
		` "journey": "SYN-JRN-` + adoptedChargeParcel + `", "provider": "` + lastMileCarrierParty + `", "agreement": "SYN-SUPPLIER-LASTMILE-SG-01",` +
		` "agreementVersion": "v1"}`
	answer = mustPost(t, endpoints.charge, chargeOccurrencePath, conflicting, http.StatusOK, "CONFLICT")
	if commission, _ := answer["commission"].(map[string]any); answer["detail"] != "PROCUREMENT_CONTEXT_CONFLICT" ||
		commission["commission"] != adoptedChargeCommiss {
		t.Fatalf("与委托不一致 = %v", answer)
	}
	if occurrences, envelopes := countChargeRows(t, db); occurrences != 1 || envelopes != 1 {
		t.Fatalf("%d 行、%d 封，want 1 / 1", occurrences, envelopes)
	}
}
