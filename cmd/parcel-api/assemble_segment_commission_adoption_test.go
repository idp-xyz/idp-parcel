package main

import (
	"net/http"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 证采用口与两个取消口的生产装配（票 demo-chain-gaps/25，ADR-0204）：干线供应商协议照种子发布批经 PC
// 发布，干线委托经委托提交口提交，干线段经现有交接口的生产编排立起来（SZX 交给干线承运方），三口经操作者渠道的「运营决定」
// 能力面走到真编排。与生产装配的差别只在令牌核验方与操作者册是替身。证据层级 S。

const (
	adoptionPath               = "/transport-fulfillment-segment-commission-adoptions"
	commissionCancellationPath = "/transport-fulfillment-transport-commission-cancellations"
	bookingCancellationPath    = "/transport-fulfillment-booking-request-cancellations"
	trunkSegmentDCG25          = "SYN-SEGMENT-SZX-SIN-DCG25"
)

var trunkHandoverJudgedAt = time.Date(2026, 10, 19, 10, 0, 0, 0, time.UTC)

type adoptionFaceEndpoints struct {
	commission       http.Handler
	adoption         http.Handler
	cancelCommission http.Handler
	cancelBooking    http.Handler
}

// adoptionFacesForTest 装生产的委托提交、采用与两个取消编排；操作者持有委托提交、采用、取消委托、取消订舱四种决定。
func adoptionFacesForTest(t *testing.T, db *bentopg.DB) adoptionFaceEndpoints {
	t.Helper()
	kinds := []accessidentity.DecisionKind{
		accessidentity.DecisionTransportCommissionSubmission,
		accessidentity.DecisionFulfillmentSegmentCommissionAdoption,
		accessidentity.DecisionTransportCommissionCancellation,
		accessidentity.DecisionBookingRequestCancellation,
	}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
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
	submitter, err := buildTransportCommissionSubmission(db)
	if err != nil {
		t.Fatalf("装配委托提交：%v", err)
	}
	adopter, err := buildSegmentCommissionAdoption(db)
	if err != nil {
		t.Fatalf("装配采用：%v", err)
	}
	cancellations, err := buildCancellationFaces(db)
	if err != nil {
		t.Fatalf("装配两个取消口：%v", err)
	}
	return adoptionFaceEndpoints{
		commission:       tfhttp.NewSubmitTransportCommissionEndpoint(decisions.transport, submitter),
		adoption:         tfhttp.NewAdoptSegmentCommissionEndpoint(decisions.transport, adopter),
		cancelCommission: tfhttp.NewCancelTransportCommissionEndpoint(decisions.transport, cancellations),
		cancelBooking:    tfhttp.NewCancelBookingRequestEndpoint(decisions.transport, cancellations),
	}
}

// establishTrunkSegmentByHandover 经现有交接口的生产编排在 SZX 把一件包裹交给干线承运方，立起干线段。
func establishTrunkSegmentByHandover(t *testing.T, db *bentopg.DB) {
	t.Helper()
	orchestrations, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	registered, err := orchestrations.handover.Register(t.Context(), tfapp.RegisterTransportHandoverCommand{
		TenantID:          mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"),
		Object:            "SYN-PARCEL-DCG25-01",
		Scope:             "SYN-HANDOVER-SCOPE-SZX-TRUNK-DCG25",
		ReleasedBy:        "SYN-NODE-SZX-GATE",
		ReceivedBy:        "SYN-PARTY-CARRIER-TRUNK-01",
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: "SYN-EVIDENCE-RELEASE-SZX-DCG25",
		ReceivingEvidence: "SYN-EVIDENCE-RECEIVE-TRUNK-DCG25",
		Rule:              "SYN-HANDOVER-RULE/v1",
		Version:           "SYN-HANDOVER-RESULT/SZX-TRUNK-DCG25/v1",
		JudgedAt:          trunkHandoverJudgedAt,
		Segment:           trunkSegmentDCG25,
		PlannedSegment:    "SYN-PLANNED-SZX-SIN-DCG25",
	})
	if err != nil || registered.Outcome() != tfapp.HandoverRegistered || registered.SegmentContinuationReference() != "" {
		t.Fatalf("交接立段 = %v %v（段欠账 %q）", registered.Outcome(), err, registered.SegmentContinuationReference())
	}
}

func adoptionBody(segment, commission, provider string) string {
	return `{"segment": "` + segment + `", "commission": "` + commission + `", "provider": "` + provider + `"}`
}

// Covers: `AT-TF-181`、`AT-TF-182` 真库面，票 demo-chain-gaps/25 演示判据（S）——
//   - 演示：干线委托提交后，用现有交接口在 SZX 立干线段，采用干线委托答 201，委托行的开始依据是那一次交接、开始时刻是交接
//     时刻，采用行一行；再取消委托答 200 TRANSPORT_ALREADY_STARTED，委托行没有取消时刻。
//   - 另一份未开始的委托可以取消，答 201；取消之后拿它来采用答 200 COMMISSION_NOT_ADOPTABLE，整笔回滚、不多一行采用。
//   - 原样再采用答 200 EXISTING_ADOPTION。
func TestTheWiredAdoptionFaceStartsTheCommissionOfASegmentEstablishedByHandover(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	seeded := publishSeededSupplierAgreements(t, db)
	trunk := seeded["SYN-SUPPLIER-TRUNK-CN-SG-01"]
	provider := trunk.Declarations.SupplierAgreementBody.Supplier
	endpoints := adoptionFacesForTest(t, db)
	const token = "presented.operator.token"

	for _, commission := range []string{"SYN-COMMISSION-TRUNK-DCG25", "SYN-COMMISSION-TRUNK-DCG25-SPARE"} {
		if status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, token,
			commissionBody(commission, provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")); status != http.StatusCreated {
			t.Fatalf("委托 %s = %d %v", commission, status, answer)
		}
	}
	establishTrunkSegmentByHandover(t, db)

	status, answer := postWithToken(t, endpoints.adoption, adoptionPath, token, adoptionBody(trunkSegmentDCG25, "SYN-COMMISSION-TRUNK-DCG25", provider))
	if status != http.StatusCreated || answer["outcome"] != "COMMISSION_ADOPTED" {
		t.Fatalf("采用 = %d %v", status, answer)
	}
	var startedBasis string
	var startedAt time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT started_basis, started_at FROM transport_fulfillment.transport_commission
		  WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = 'SYN-COMMISSION-TRUNK-DCG25'`).Scan(&startedBasis, &startedAt); err != nil {
		t.Fatalf("读委托行：%v", err)
	}
	if startedBasis != "TRANSPORT-HANDOVER/SYN-HANDOVER-RESULT/SZX-TRUNK-DCG25/v1" || !startedAt.Equal(trunkHandoverJudgedAt) {
		t.Fatalf("委托的开始 = (%s, %s)，want 那一次交接", startedBasis, startedAt)
	}

	status, answer = postWithToken(t, endpoints.cancelCommission, commissionCancellationPath, token,
		`{"commission": "SYN-COMMISSION-TRUNK-DCG25", "cancelledAt": "2026-10-19T12:00:00Z"}`)
	if status != http.StatusOK || answer["outcome"] != "TRANSPORT_ALREADY_STARTED" {
		t.Fatalf("采用之后取消委托 = %d %v", status, answer)
	}

	status, answer = postWithToken(t, endpoints.cancelCommission, commissionCancellationPath, token,
		`{"commission": "SYN-COMMISSION-TRUNK-DCG25-SPARE", "cancelledAt": "2026-10-19T12:00:00Z"}`)
	if status != http.StatusCreated || answer["outcome"] != "COMMISSION_CANCELLED" {
		t.Fatalf("取消未开始的委托 = %d %v", status, answer)
	}
	if status, answer := postWithToken(t, endpoints.adoption, adoptionPath, token,
		adoptionBody(trunkSegmentDCG25, "SYN-COMMISSION-TRUNK-DCG25", provider)); status != http.StatusOK || answer["outcome"] != "EXISTING_ADOPTION" {
		t.Fatalf("原样再采用 = %d %v", status, answer)
	}

	// 另立一段去采用那份已取消的委托：答不可采用，且整笔回滚。
	orchestrations, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatal(err)
	}
	second := tfapp.RegisterTransportHandoverCommand{
		TenantID: mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01"), Object: "SYN-PARCEL-DCG25-02",
		Scope: "SYN-HANDOVER-SCOPE-SZX-TRUNK-DCG25-2", ReleasedBy: "SYN-NODE-SZX-GATE", ReceivedBy: "SYN-PARTY-CARRIER-TRUNK-01",
		Verdict: tfdomain.ObjectHandedOver, ReleasingEvidence: "SYN-EVIDENCE-RELEASE-SZX-DCG25-2", ReceivingEvidence: "SYN-EVIDENCE-RECEIVE-TRUNK-DCG25-2",
		Rule: "SYN-HANDOVER-RULE/v1", Version: "SYN-HANDOVER-RESULT/SZX-TRUNK-DCG25-2/v1", JudgedAt: trunkHandoverJudgedAt.Add(time.Hour),
		Segment: trunkSegmentDCG25 + "-2", PlannedSegment: "SYN-PLANNED-SZX-SIN-DCG25",
	}
	if registered, err := orchestrations.handover.Register(t.Context(), second); err != nil || registered.Outcome() != tfapp.HandoverRegistered {
		t.Fatalf("第二段 = %v %v", registered.Outcome(), err)
	}
	status, answer = postWithToken(t, endpoints.adoption, adoptionPath, token, adoptionBody(trunkSegmentDCG25+"-2", "SYN-COMMISSION-TRUNK-DCG25-SPARE", provider))
	if status != http.StatusOK || answer["outcome"] != "COMMISSION_NOT_ADOPTABLE" {
		t.Fatalf("采用已取消的委托 = %d %v", status, answer)
	}
	var adoptions int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.segment_commission_adoption`).Scan(&adoptions); err != nil || adoptions != 1 {
		t.Fatalf("采用 %d 行 %v，want 1", adoptions, err)
	}
}

// Covers: 采用口没有持有采用授予的操作者进不来——只持有委托提交与取消委托的人答 403 OPERATOR_NOT_GRANTED（ADR-0202 决定一
// 「授予的粒度等于权力的粒度」），不落任何行。
func TestTheWiredAdoptionFaceRefusesAnOperatorWithoutTheAdoptionGrant(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	kinds := []accessidentity.DecisionKind{accessidentity.DecisionTransportCommissionSubmission, accessidentity.DecisionTransportCommissionCancellation}
	registerOperationDecisionAdmission(t, db, isolatedSelfAuthority, kinds...)
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
	adopter, err := buildSegmentCommissionAdoption(db)
	if err != nil {
		t.Fatal(err)
	}
	endpoint := tfhttp.NewAdoptSegmentCommissionEndpoint(decisions.transport, adopter)
	status, answer := postWithToken(t, endpoint, adoptionPath, "presented.operator.token", adoptionBody(trunkSegmentDCG25, "SYN-COMMISSION-TRUNK-DCG25", "SYN-PARTY-CARRIER-TRUNK-01"))
	problem, _ := answer["error"].(map[string]any)
	if status != http.StatusForbidden || problem["code"] != "OPERATOR_NOT_GRANTED" {
		t.Fatalf("无采用授予 = %d %v", status, answer)
	}
}
