package main

import (
	"context"
	"net/http"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
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

// Covers: `AT-TF-181`、`AT-TF-182`、`AT-TF-184`、`AT-TF-187` 真库面，票 demo-chain-gaps/25 演示判据（S）——
//   - 演示：干线委托提交后，用现有交接口在 SZX 立干线段，采用干线委托答 201，委托行的开始依据是那一次交接、开始时刻是交接
//     时刻，采用行一行；再取消委托答 200 TRANSPORT_ALREADY_STARTED，委托行没有取消时刻。
//   - 另一份未开始的委托可以取消，答 201。
//   - 原样再采用答 200 EXISTING_ADOPTION。
//   - 采用登记之前的取消：第二段 11:00 由交接立起，那份委托 12:00 登记取消时本上下文还不知道两者相连，取消照常成立；此后拿
//     第二段去采用它答 200 COMMISSION_CANCELLED_AFTER_START，并排回显取消时刻与段的首个参与，不多一行采用（领域在落连线之前
//     就判出这一格，这里证的是答法与零写入，不是装配回滚；回滚见下一个用例）。
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

	// 另立一段去采用那份已取消的委托：段在取消之前就已开始。
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
	commission, _ := answer["commission"].(map[string]any)
	segmentStart, _ := answer["segmentStart"].(map[string]any)
	if status != http.StatusOK || answer["outcome"] != "COMMISSION_CANCELLED_AFTER_START" ||
		commission["cancelledAt"] != "2026-10-19T12:00:00Z" || segmentStart["startedAt"] != "2026-10-19T11:00:00Z" {
		t.Fatalf("采用段开始之后才登记取消的委托 = %d %v", status, answer)
	}
	var adoptions int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.segment_commission_adoption`).Scan(&adoptions); err != nil || adoptions != 1 {
		t.Fatalf("采用 %d 行 %v，want 1", adoptions, err)
	}
}

// Covers: `AT-TF-188` 授予那一半——采用口没有持有采用授予的操作者进不来——只持有委托提交与取消委托的人答 403 OPERATOR_NOT_GRANTED（ADR-0202 决定一
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

// raceCancellingCommissions 在采用编排写委托开始的那一刻之前，先在采用那一笔事务之外经生产的取消口取消这份委托并提交，再照常
// 写：模拟「采用读到委托时它尚未取消，写的那一刻它已被另一笔取消」。probe 在同一刻用采用那一笔的上下文读连线，证读回之前
// 连线确实已经落了——下面的零行才是回滚撤掉的，不是从没写过。
type raceCancellingCommissions struct {
	*tfpostgres.TransportCommissions
	race  func()
	probe func(ctx context.Context)
}

func (store *raceCancellingCommissions) Replace(ctx context.Context, record ports.TransportCommissionRecord) (bool, error) {
	if store.race != nil {
		store.probe(ctx)
		store.race()
		store.race = nil
	}
	return store.TransportCommissions.Replace(ctx, record)
}

// Covers: `AT-TF-187` 的装配回滚（ADR-0204 决定二）——采用把连线落进事务之后、写委托开始之前，委托被另一笔提交取消（取消时刻
// 早于段的首个参与）：编排读回答 200 COMMISSION_NOT_ADOPTABLE，装配整笔回滚，已落的连线随之撤掉，采用册零行；委托行仍是已
// 取消、没有开始。
func TestTheAdoptionShellRollsBackALinkWhenTheCommissionIsCancelledUnderneath(t *testing.T) {
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
	if status, answer := postWithToken(t, endpoints.commission, commissionSubmissionPath, token,
		commissionBody("SYN-COMMISSION-TRUNK-DCG25", provider, trunk.ObjectID, trunk.Version, "2026-10-19T08:00:00Z")); status != http.StatusCreated {
		t.Fatalf("委托 = %d %v", status, answer)
	}
	establishTrunkSegmentByHandover(t, db)

	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		t.Fatal(err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		t.Fatal(err)
	}
	cancellations, err := buildCancellationFaces(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-01")
	adoptionKey := ports.SegmentCommissionAdoptionKey{TenantID: tenant, Segment: mustValue(t, tfdomain.NewFulfillmentSegmentReference, trunkSegmentDCG25)}
	linkedBeforeTheRace := false
	racing := &raceCancellingCommissions{
		TransportCommissions: commissions,
		probe: func(txCtx context.Context) {
			_, found, err := adoptions.FindBySegment(txCtx, adoptionKey)
			linkedBeforeTheRace = err == nil && found
		},
		race: func() {
			// t.Context() 不带采用那一笔的事务，取消口另开一笔、提交后才回来。
			cancelled, err := cancellations.CancelCommission(t.Context(), tfapp.CancelCommissionCommand{
				TenantID: tenant, Commission: "SYN-COMMISSION-TRUNK-DCG25", CancelledAt: trunkHandoverJudgedAt.Add(-time.Hour),
			})
			if err != nil || cancelled.Outcome() != tfapp.CommissionCancelled {
				t.Fatalf("另一笔取消 = %v %v", cancelled.Outcome(), err)
			}
		},
	}
	adopter := transactionalSegmentCommissionAdopter{
		transactor: db.Transactor(),
		inner: tfapp.NewAdoptSegmentCommissionHandler(tfapp.AdoptSegmentCommissionDeps{
			Adoptions: adoptions, Segments: segments, Commissions: racing, StartAdvancement: commissions, Clock: systemClock{},
		}),
	}
	result, err := adopter.Adopt(t.Context(), tfapp.AdoptSegmentCommissionCommand{
		TenantID: tenant, Segment: trunkSegmentDCG25, Commission: "SYN-COMMISSION-TRUNK-DCG25", Provider: provider,
	})
	if err != nil || result.Outcome() != tfapp.CommissionNotAdoptable {
		t.Fatalf("写开始之前委托被取消 = %v %v，want COMMISSION_NOT_ADOPTABLE", result.Outcome(), err)
	}
	if !linkedBeforeTheRace {
		t.Fatal("连线在写开始之前没落进事务——零行就证不了回滚")
	}
	var adoptionRows int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM transport_fulfillment.segment_commission_adoption`).Scan(&adoptionRows); err != nil || adoptionRows != 0 {
		t.Fatalf("采用 %d 行 %v，want 0（已落的连线应随整笔回滚撤掉）", adoptionRows, err)
	}
	var startedAt, cancelledAt *time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT started_at, cancelled_at FROM transport_fulfillment.transport_commission
		  WHERE tenant_id = 'SYN-TENANT-01' AND commission_id = 'SYN-COMMISSION-TRUNK-DCG25'`).Scan(&startedAt, &cancelledAt); err != nil {
		t.Fatal(err)
	}
	if startedAt != nil || cancelledAt == nil {
		t.Fatalf("委托行 = 开始 %v、取消 %v，want 只有取消", startedAt, cancelledAt)
	}
}
