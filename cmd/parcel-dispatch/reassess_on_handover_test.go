package main

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
)

// 本文件对真实 PostgreSQL 证交接触发的路由复核（ADR-0215）。已接受委托由 PS 应用编排形成（复用 SYN-V0 夹具），交接经
// TF 生产登记编排登记并真交出信封，复核落 NR 真判断库。生产路由那一条全走生产装配；其余几条经
// reassessOnHandoverConsumerOver 接同一份交接读取、PS 读取与消费门，只把证据视图换成记下所携投影的替身，好让计划与
// 冻结形态由用例给定。夹具全部合成，只记 `S`。

const (
	reassessOnHandoverConsumerName = "network-routing/reassess-on-transport-handover"
	reassessHandoverScope          = "SYN-HANDOVER-SCOPE-63"
	reassessHandoverParcel         = "SYN-PARCEL-01"
	reassessHandoverBaseline       = "SYN-VER-01"
	reassessHandoverReceiving      = "SYN-NODE-SIN-HUB"
)

var reassessHandoverAt = time.Date(2026, 10, 21, 2, 0, 0, 0, time.UTC)

// carriedGeoEvidence 是证据视图替身：交回给定的证据，并记下复核取证据时所携的投影。
type carriedGeoEvidence struct {
	evidence nrports.InitialRouteEvidence
	seen     []nrports.RequestCarriedContent
}

func (spy *carriedGeoEvidence) LoadInitialRouteEvidence(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey, carried nrports.RequestCarriedContent,
) (nrports.InitialRouteEvidence, bool, error) {
	spy.seen = append(spy.seen, carried)
	return spy.evidence, true, nil
}

type reassessOnHandoverFixture struct {
	syn      *synVerticalFixture
	register *tfapplication.RegisterTransportHandoverHandler
	evidence *carriedGeoEvidence
	lane     dispatch.Consumer
}

// newReassessOnHandoverFixture 接一条与生产同形的复核路（同一个 Over、同一份哨兵名单），只换证据视图。accepted 为真时先由
// PS 编排把 SYN 委托推到已接受（首个提交版本两端报国家与邮编）；withPlan 为真时给它的判断键落一份上海→新加坡枢纽→新加坡
// 末端的计划。
func newReassessOnHandoverFixture(t *testing.T, accepted, withPlan bool) *reassessOnHandoverFixture {
	t.Helper()
	syn := newSYNVerticalFixture(t)
	if accepted {
		acceptWithDeclaredGeo(t, syn)
	}
	if withPlan {
		saveThroughSingaporePlan(t, syn)
	}
	outboxStore, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	inboxStore, err := inbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Inbox Store：%v", err)
	}
	limit := 0
	evidence := &carriedGeoEvidence{evidence: nrports.InitialRouteEvidence{
		Strategy:                    mustNR(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
		ViewRevision:                mustNR(t, nrdomain.NewNetworkViewRevision, "SYN-VIEW-REV-1"),
		FreezeForm:                  nrdomain.RemainingSegmentCountFreeze,
		FreezeRemainingSegmentLimit: &limit,
	}}
	reassess, err := reassessRouteHandlerOver(syn.db, systemClock{}, evidence)
	if err != nil {
		t.Fatalf("装配复核编排：%v", err)
	}
	consumer, err := reassessOnHandoverConsumerOver(syn.db, inboxStore, reassess,
		mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
	if err != nil {
		t.Fatalf("装配复核这一路：%v", err)
	}
	lane, err := dispatch.WithUndecidedSentinels(consumer, reassessOnHandoverUndecidedSentinels...)
	if err != nil {
		t.Fatalf("包未决哨兵：%v", err)
	}
	return &reassessOnHandoverFixture{
		syn:      syn,
		register: newHandoverRegistration(t, syn.db, outboxStore),
		evidence: evidence,
		lane:     lane,
	}
}

// acceptWithDeclaredGeo 照 SYN-V0 的提交、判断与形成决定走一遍，只在提交命令上加两端地址要素：受理基线钉在这一版上。
func acceptWithDeclaredGeo(t *testing.T, syn *synVerticalFixture) {
	t.Helper()
	command := syn.submitCommand(t)
	command.DeclaredElements = synDeclaredCNToSG(t)
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, syn.transactor, t.Context(), func(txCtx context.Context) error {
		result, err := syn.submitHandler.Handle(txCtx, command)
		outcome = result.Outcome()
		return err
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("提交 = %q, want SUBMITTED", outcome)
	}
	syn.recordPassingJudgments(t, t.Context())
	if result := syn.formDecision(t, t.Context()); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
}

func synDeclaredCNToSG(t *testing.T) psdomain.DeclaredAddressElements {
	t.Helper()
	entry := func(group psdomain.SourceDataGroupReference, element psdomain.AddressElementName, raw string) psdomain.CanonicalContentEntry {
		built, err := psdomain.NewCanonicalContentEntry(psdomain.AddressElementEntryName(group, element), raw)
		if err != nil {
			t.Fatalf("地址要素条目：%v", err)
		}
		return built
	}
	sender, delivery := psdomain.SenderPlaceDataGroup(), psdomain.DeliveryPlaceDataGroup()
	return psdomain.NewDeclaredAddressElements(
		psdomain.AddressElementsOf(sender, []psdomain.CanonicalContentEntry{
			entry(sender, psdomain.CountryCodeElement, "CN"), entry(sender, psdomain.PostalCodeElement, "200000"),
		}),
		psdomain.AddressElementsOf(delivery, []psdomain.CanonicalContentEntry{
			entry(delivery, psdomain.CountryCodeElement, "SG"), entry(delivery, psdomain.PostalCodeElement, "018956"),
		}),
	)
}

func reassessHandoverKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           mustNR(t, nrdomain.NewTenantID, "SYN-TENANT-01"),
		CustomerAccountID:  mustNR(t, nrdomain.NewCustomerAccountID, "SYN-CUSTOMER-01"),
		ShipmentRequestID:  mustNR(t, nrdomain.NewShipmentRequestID, "SYN-REQ-01"),
		AcceptanceBaseline: mustNR(t, nrdomain.NewAcceptanceBaselineReference, reassessHandoverBaseline),
		DeclaredParcelID:   mustNR(t, nrdomain.NewDeclaredParcelID, reassessHandoverParcel),
		ServicePurpose:     mustNR(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

// saveThroughSingaporePlan 给 SYN 委托的判断键落一份上海枢纽 → 新加坡枢纽 → 新加坡末端的计划。
func saveThroughSingaporePlan(t *testing.T, syn *synVerticalFixture) {
	t.Helper()
	savePlan(t, syn, "SYN-CANDIDATE-01", "SYN-NODE-SHA-HUB", reassessHandoverReceiving, "SYN-NODE-SIN-LM")
}

// savePlan 给 SYN 委托的判断键落一份真经 NR 领域形成的计划，沿给定节点逐段连成节点间段，选中给定候选。
func savePlan(t *testing.T, syn *synVerticalFixture, selected string, nodes ...string) {
	t.Helper()
	legs := make([]nrdomain.PlannedLeg, 0, len(nodes)-1)
	for i := 1; i < len(nodes); i++ {
		start := reassessHandoverAt.Add(time.Duration(i-len(nodes)) * 24 * time.Hour)
		window, err := nrdomain.NewPlannedTimeWindow(start, start.Add(24*time.Hour),
			mustNR(t, nrdomain.NewWindowBasisReference, "SYN-CALENDAR/v1"))
		if err != nil {
			t.Fatalf("段窗口：%v", err)
		}
		leg, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
			From:        mustNR(t, nrdomain.NewPlanNodeReference, nodes[i-1]),
			To:          mustNR(t, nrdomain.NewPlanNodeReference, nodes[i]),
			Responsible: mustNR(t, nrdomain.NewResponsiblePartyReference, "SYN-PARTY-01"),
			Window:      window,
		})
		if err != nil {
			t.Fatalf("计划段：%v", err)
		}
		legs = append(legs, leg)
	}
	candidate, err := nrdomain.NewRouteCandidate(mustNR(t, nrdomain.NewCandidateID, selected),
		nrdomain.CandidateQualified, nrdomain.CandidateReason{})
	if err != nil {
		t.Fatalf("候选：%v", err)
	}
	plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
		Key:           reassessHandoverKey(t),
		Version:       mustNR(t, nrdomain.NewRoutePlanVersionID, "SYN-RPV-63"),
		Selected:      candidate.ID(),
		Candidates:    []nrdomain.RouteCandidate{candidate},
		Legs:          legs,
		Strategy:      mustNR(t, nrdomain.NewRouteStrategyReference, "SYN-STRATEGY/v1"),
		ViewRevision:  mustNR(t, nrdomain.NewNetworkViewRevision, "SYN-VIEW-REV-1"),
		JudgedAt:      reassessHandoverAt.Add(-72 * time.Hour),
		EffectiveFrom: reassessHandoverAt.Add(-72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("计划：%v", err)
	}
	routes, err := nrpostgres.NewInitialRoutes(syn.db)
	if err != nil {
		t.Fatalf("初始路由库：%v", err)
	}
	mustWithinTX(t, syn.transactor, t.Context(), func(txCtx context.Context) error {
		saved, err := routes.Save(txCtx, nrports.InitialRouteRecord{Key: reassessHandoverKey(t), Plan: plan, HasPlan: true})
		if err == nil && saved != nrports.InitialRouteSaved {
			t.Errorf("落计划 = %v, want SAVED", saved)
		}
		return err
	})
}

// handoverCommand 是一次交接的登记命令。receiving 空即不声明接收节点；releasing 同理。
func handoverCommand(
	t *testing.T, object, version string, verdict tfdomain.HandoverVerdict, releasing, receiving string,
) tfapplication.RegisterTransportHandoverCommand {
	t.Helper()
	command := tfapplication.RegisterTransportHandoverCommand{
		TenantID:      mustTF(t, tfdomain.NewTenantID, "SYN-TENANT-01"),
		Object:        object,
		Scope:         reassessHandoverScope,
		ReleasedBy:    "SYN-PARTY-CARRIER-TRUNK-01",
		ReceivedBy:    "SYN-PARTY-SIN-HUB",
		Verdict:       verdict,
		Version:       version,
		JudgedAt:      reassessHandoverAt,
		ReleasingNode: releasing,
		ReceivingNode: receiving,
	}
	if verdict == tfdomain.ObjectHandedOver {
		command.ReleasingEvidence = "SYN-EVIDENCE-RELEASE-63"
		command.ReceivingEvidence = "SYN-EVIDENCE-RECEIVE-63"
		command.Rule = "SYN-HANDOVER-RULE/v1"
	} else {
		command.Basis = "SYN-HANDOVER-BASIS-63"
	}
	return command
}

// registerHandover 经生产登记编排登一次交接，交回它真交出的那封信的 ID：前后两次列 outbox 取差，不在测试里重拼 ID。
func registerHandover(
	t *testing.T, syn *synVerticalFixture, register *tfapplication.RegisterTransportHandoverHandler,
	command tfapplication.RegisterTransportHandoverCommand,
) string {
	t.Helper()
	before := syn.outboxIDsOfType(t, string(tfinbox.TransportHandoverRegisteredEventType))
	var result tfapplication.RegisterTransportHandoverResult
	mustWithinTX(t, syn.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = register.Register(txCtx, command)
		return err
	})
	if result.Outcome() != tfapplication.HandoverRegistered {
		t.Fatalf("登记交接 %s = %s", command.Version, result.Outcome())
	}
	for _, id := range syn.outboxIDsOfType(t, string(tfinbox.TransportHandoverRegisteredEventType)) {
		if !slices.Contains(before, id) {
			return id
		}
	}
	t.Fatalf("登记交接 %s 没有交出信封", command.Version)
	return ""
}

// outboxEnvelope 从 outbox 原样读回一封信，可换一个信封 ID——模拟同一份内容另一封投递。
func outboxEnvelope(t *testing.T, syn *synVerticalFixture, eventID, asID string) eventing.Envelope {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	envelope := eventing.Envelope{ID: eventing.EventID(asID)}
	var (
		eventType string
		version   int64
		payload   []byte
	)
	if err := querier.QueryRow(t.Context(),
		`SELECT source, spec_version, event_type, event_version, scope, subject, partition_key,
		        occurred_at, recorded_at, content_type, payload
		   FROM `+migrate.SchemaBento+`.outbox WHERE event_id = $1`, eventID,
	).Scan(&envelope.Source, &envelope.SpecVersion, &eventType, &version, &envelope.Scope, &envelope.Subject,
		&envelope.PartitionKey, &envelope.OccurredAt, &envelope.RecordedAt, &envelope.ContentType, &payload,
	); err != nil {
		t.Fatalf("读回信封 %s：%v", eventID, err)
	}
	envelope.Type = eventing.EventType(eventType)
	envelope.Version = eventing.EventVersion(version)
	envelope.Payload = payload
	return envelope
}

type reassessmentRow struct {
	correlation, customer, request, baseline, parcel, purpose, conclusion string
	lapseBasis                                                            *string
}

func reassessmentRows(t *testing.T, syn *synVerticalFixture) []reassessmentRow {
	t.Helper()
	querier, err := syn.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT correlation_id, customer_account_id, shipment_request_id, acceptance_baseline, declared_parcel_id,
		        service_purpose, conclusion, lapse_basis
		   FROM network_routing.route_reassessment WHERE tenant_id = $1 ORDER BY correlation_id`, "SYN-TENANT-01")
	if err != nil {
		t.Fatalf("列复核：%v", err)
	}
	defer rows.Close()
	var found []reassessmentRow
	for rows.Next() {
		var row reassessmentRow
		if err := rows.Scan(&row.correlation, &row.customer, &row.request, &row.baseline, &row.parcel,
			&row.purpose, &row.conclusion, &row.lapseBasis); err != nil {
			t.Fatalf("扫复核行：%v", err)
		}
		found = append(found, row)
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("遍历复核：%v", err)
	}
	return found
}

// baselineGeoDigest 从 PS 读回已接受委托，取受理基线那一版提交的投影，按 NR 的摘要形状算——期望值出自权威，不出自测试。
func baselineGeoDigest(t *testing.T, syn *synVerticalFixture) string {
	t.Helper()
	request := syn.mustLoadRequest(t, t.Context())
	baseline, present := request.AcceptanceBaseline()
	if !present {
		t.Fatal("已接受委托没有受理基线")
	}
	version, present := request.SubmissionVersionByID(baseline.SubmissionVersionID())
	if !present {
		t.Fatal("基线那一版提交读不回")
	}
	projection := version.GeoProjection()
	senderCountry, hasSenderCountry := projection.Sender().Country()
	senderPostal, hasSenderPostal := projection.Sender().PostalCode()
	deliveryCountry, hasDeliveryCountry := projection.Delivery().Country()
	deliveryPostal, hasDeliveryPostal := projection.Delivery().PostalCode()
	if !hasDeliveryCountry || deliveryCountry != "SG" {
		t.Fatalf("基线那一版的收件国家 = %q（在场 %v）——夹具没把地址要素报上", deliveryCountry, hasDeliveryCountry)
	}
	return nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide(senderCountry, hasSenderCountry, senderPostal, hasSenderPostal),
		nrdomain.NewGeoResolutionSide(deliveryCountry, hasDeliveryCountry, deliveryPostal, hasDeliveryPostal),
	).ContentDigest()
}

// Covers: AT-NR-064、AT-NR-068——`已交接`且声明接收节点：一条复核落库，键落在已接受委托的受理基线上，结论仍适用；复核取证据
// 时所携投影的摘要等于 PS 受理基线那一版提交的投影摘要。同一封再投在消费门跳过；同一版交接另一封信封答已有结果，仍一条。
func TestAHandoverIntoANodeReassessesTheAcceptedParcelsRouteOnTheDatabase(t *testing.T) {
	fixture := newReassessOnHandoverFixture(t, true, true)
	eventID := registerHandover(t, fixture.syn, fixture.register, handoverCommand(t, reassessHandoverParcel,
		"SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "SYN-NODE-SZX-GATE", reassessHandoverReceiving))

	for _, asID := range []string{eventID, eventID, eventID + "/again"} {
		if err := fixture.lane.Consume(t.Context(), outboxEnvelope(t, fixture.syn, eventID, asID)); err != nil {
			t.Fatalf("投递 %s：%v", asID, err)
		}
	}
	rows := reassessmentRows(t, fixture.syn)
	if len(rows) != 1 {
		t.Fatalf("复核 = %d 行, want 1——同一版交接只复核一次", len(rows))
	}
	row := rows[0]
	if row.conclusion != "STILL_APPLICABLE" || row.request != "SYN-REQ-01" || row.customer != "SYN-CUSTOMER-01" ||
		row.baseline != reassessHandoverBaseline || row.parcel != reassessHandoverParcel || row.purpose != "NETWORK_SERVICE" {
		t.Fatalf("复核行 = %+v", row)
	}
	if want := "handover/13:SYN-TENANT-01/13:SYN-PARCEL-01/21:SYN-HANDOVER-SCOPE-63/18:SYN-HANDOVER/63/v1"; row.correlation != want {
		t.Fatalf("关联 = %q, want %q", row.correlation, want)
	}
	if len(fixture.evidence.seen) != 1 {
		t.Fatalf("取证据 %d 次, want 1", len(fixture.evidence.seen))
	}
	if got, want := fixture.evidence.seen[0].Geo.ContentDigest(), baselineGeoDigest(t, fixture.syn); got != want {
		t.Fatalf("所携投影摘要 = %s, want 受理基线那一版的 %s", got, want)
	}
	for _, id := range []string{eventID, eventID + "/again"} {
		if n := inboxRowsOf(t, fixture.syn.db, reassessOnHandoverConsumerName, id); n != 1 {
			t.Fatalf("%s 的 inbox 行 = %d, want 1", id, n)
		}
	}
}

// Covers: AT-NR-064（位置那一半）——位置取接收节点：声明的接收节点不在计划上，原计划失效，失效依据点名这个节点。
func TestAHandoverIntoAnUnplannedNodeLapsesThePlanOnTheDatabase(t *testing.T) {
	fixture := newReassessOnHandoverFixture(t, true, true)
	eventID := registerHandover(t, fixture.syn, fixture.register, handoverCommand(t, reassessHandoverParcel,
		"SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "", "SYN-NODE-KUL-HUB"))

	if err := fixture.lane.Consume(t.Context(), outboxEnvelope(t, fixture.syn, eventID, eventID)); err != nil {
		t.Fatalf("投递：%v", err)
	}
	rows := reassessmentRows(t, fixture.syn)
	if len(rows) != 1 || rows[0].conclusion != "PLAN_LAPSED" || rows[0].lapseBasis == nil ||
		*rows[0].lapseBasis != "PLAN_ORIGIN_MISMATCH/SYN-NODE-KUL-HUB" {
		t.Fatalf("复核 = %+v, want 一行失效、依据点名接收节点", rows)
	}
}

// Covers: AT-NR-065、AT-NR-066——拒收、交给运输方（只声明交出节点）、对象不是当前已接受包裹：都入账，零复核。
func TestHandoversThatAreNotAReassessmentTriggerSettleOnTheDatabase(t *testing.T) {
	cases := map[string]struct {
		accepted bool
		command  func(t *testing.T) tfapplication.RegisterTransportHandoverCommand
	}{
		"拒收": {true, func(t *testing.T) tfapplication.RegisterTransportHandoverCommand {
			return handoverCommand(t, reassessHandoverParcel, "SYN-HANDOVER/63/refused", tfdomain.HandoverRefused,
				"", reassessHandoverReceiving)
		}},
		"交给运输方": {true, func(t *testing.T) tfapplication.RegisterTransportHandoverCommand {
			return handoverCommand(t, reassessHandoverParcel, "SYN-HANDOVER/63/out", tfdomain.ObjectHandedOver,
				"SYN-NODE-SHA-HUB", "")
		}},
		"委托还没接受": {false, func(t *testing.T) tfapplication.RegisterTransportHandoverCommand {
			return handoverCommand(t, reassessHandoverParcel, "SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver,
				"", reassessHandoverReceiving)
		}},
		"集运单元": {true, func(t *testing.T) tfapplication.RegisterTransportHandoverCommand {
			return handoverCommand(t, "SYN-UNIT-63", "SYN-HANDOVER/63/unit", tfdomain.ObjectHandedOver,
				"", reassessHandoverReceiving)
		}},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReassessOnHandoverFixture(t, tc.accepted, true)
			eventID := registerHandover(t, fixture.syn, fixture.register, tc.command(t))
			if err := fixture.lane.Consume(t.Context(), outboxEnvelope(t, fixture.syn, eventID, eventID)); err != nil {
				t.Fatalf("投递 = %v, want 入账", err)
			}
			if n := inboxRowsOf(t, fixture.syn.db, reassessOnHandoverConsumerName, eventID); n != 1 {
				t.Fatalf("inbox 行 = %d, want 1——该入账", n)
			}
			if rows := reassessmentRows(t, fixture.syn); len(rows) != 0 {
				t.Fatalf("复核 = %+v, want 零行", rows)
			}
			if len(fixture.evidence.seen) != 0 {
				t.Fatalf("不是复核触发还取了 %d 次证据", len(fixture.evidence.seen))
			}
		})
	}
}

// Covers: AT-NR-067——复核停在未决（已接受包裹没有路由历史）：本路哨兵翻成派发器认的未决，整笔回滚，inbox 无痕、零复核。
func TestAnUndecidedReassessmentOnAHandoverRollsBackOnTheDatabase(t *testing.T) {
	fixture := newReassessOnHandoverFixture(t, true, false)
	eventID := registerHandover(t, fixture.syn, fixture.register, handoverCommand(t, reassessHandoverParcel,
		"SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "", reassessHandoverReceiving))

	err := fixture.lane.Consume(t.Context(), outboxEnvelope(t, fixture.syn, eventID, eventID))
	if !errors.Is(err, dispatch.ErrConsumerUndecided) {
		t.Fatalf("投递 = %v, want 翻成 ErrConsumerUndecided", err)
	}
	if n := inboxRowsOf(t, fixture.syn.db, reassessOnHandoverConsumerName, eventID); n != 0 {
		t.Fatalf("未决必须回滚：inbox 行 = %d", n)
	}
	if rows := reassessmentRows(t, fixture.syn); len(rows) != 0 {
		t.Fatalf("未决却落了复核：%+v", rows)
	}
}

// Covers: AT-NR-064（路由表那一半）——生产路由表上交接信封扇出到 NR 复核这一路：信封是生产交接口真交出的那一封，三路认的
// 是同一个事件类型；空库里对象不是当前已接受包裹，复核这一路入账、零复核，信封定稿 PUBLISHED。
func TestAHandoverReachesRouteReassessmentThroughTheProductionRoute(t *testing.T) {
	for lane, eventType := range map[string]eventing.EventType{
		"VE": veinbox.TransportHandoverRegisteredEventType,
		"TF": tfinbox.TransportHandoverRegisteredEventType,
	} {
		if eventType != nrinbox.TransportHandoverRegisteredEventType {
			t.Fatalf("NR 认 %q、%s 认 %q——同一封信几路认的不是同一个类型", nrinbox.TransportHandoverRegisteredEventType, lane, eventType)
		}
	}
	syn := newSYNVerticalFixture(t)
	store, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	eventID := registerHandover(t, syn, newHandoverRegistration(t, syn.db, store), handoverCommand(t,
		reassessHandoverParcel, "SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "", reassessHandoverReceiving))

	if _, err := syn.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if status := outboxStatus(t, syn.db, eventID); status != "PUBLISHED" {
		t.Fatalf("交接信封 status = %s，失败码 = %q, want PUBLISHED", status, recordedFailureCode(t, syn.db, eventID))
	}
	if n := inboxRowsOf(t, syn.db, reassessOnHandoverConsumerName, eventID); n != 1 {
		t.Fatalf("NR 复核名下 inbox 行 = %d, want 1——FanOut 少了这一路", n)
	}
	if rows := reassessmentRows(t, syn); len(rows) != 0 {
		t.Fatalf("对象不是当前已接受包裹却复核了：%+v", rows)
	}
}

// Covers: AT-NR-064（生产装配那一半）——演示租户采用了演示网络，委托已接受，计划经深圳口岸到新加坡枢纽再到末端：交进新加坡
// 枢纽的交接经生产路由表一拍，形成一条仍适用的复核。证据视图、关务来源、复核编排与 PS 读取全是生产接法。
func TestAHandoverIntoSingaporeHubIsStillApplicableUnderTheDemoNetworkThroughTheProductionRoute(t *testing.T) {
	syn := newSYNVerticalFixture(t)
	acceptWithDeclaredGeo(t, syn)
	registerDemoNetworkSeeds(t, syn.db)
	seedWiringCustomsCatalog(t, syn.db, demoTenant, true)
	savePlan(t, syn, "SYN-LINE-CN-SG-01@1",
		"SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", reassessHandoverReceiving, "SYN-NODE-SIN-LM")
	store, err := outbox.NewStore(syn.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	eventID := registerHandover(t, syn, newHandoverRegistration(t, syn.db, store), handoverCommand(t,
		reassessHandoverParcel, "SYN-HANDOVER/63/v1", tfdomain.ObjectHandedOver, "SYN-NODE-SZX-GATE", reassessHandoverReceiving))

	if _, err := syn.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if status := outboxStatus(t, syn.db, eventID); status != "PUBLISHED" {
		t.Fatalf("交接信封 status = %s，失败码 = %q, want PUBLISHED", status, recordedFailureCode(t, syn.db, eventID))
	}
	rows := reassessmentRows(t, syn)
	if len(rows) != 1 || rows[0].conclusion != "STILL_APPLICABLE" || rows[0].baseline != reassessHandoverBaseline {
		t.Fatalf("复核 = %+v, want 一行仍适用、落在受理基线上", rows)
	}
}
