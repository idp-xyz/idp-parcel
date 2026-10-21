package transportfulfillment_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/transportfulfillment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证交接触发的路由复核（ADR-0215 决定一）：交接按键读回，只有`已交接`且声明了接收节点、对象是当前已接受包裹的
// 那一版走真实复核编排；位置取接收节点，所携投影取受理基线那一版的。复核编排是真的，只把判断库各册换成内存替身。

var handedOverAt = time.Date(2026, 10, 20, 6, 0, 0, 0, time.UTC)

const (
	planOrigin   = "SYN-NODE-SHA-HUB"
	planTransfer = "SYN-NODE-SIN-HUB"
	planLastMile = "SYN-NODE-SIN-LM"
)

func value[T interface{ String() string }](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return built
}

// ---- 交接与登记册替身 ----

type handoverSpec struct {
	object    string
	version   string
	verdict   tfdomain.HandoverVerdict
	receiving string
	releasing string
}

func formHandover(t *testing.T, spec handoverSpec) tfdomain.TransportHandover {
	t.Helper()
	nodes := tfdomain.HandoverNodeDeclarations{}
	if spec.receiving != "" {
		nodes.Receiving = value(t, tfdomain.NewNetworkNodeReference, spec.receiving)
	}
	if spec.releasing != "" {
		nodes.Releasing = value(t, tfdomain.NewNetworkNodeReference, spec.releasing)
	}
	formed := tfdomain.TransportHandoverSpec{
		TenantID:   value(t, tfdomain.NewTenantID, "tenant-1"),
		Object:     value(t, tfdomain.NewCarriedObjectReference, spec.object),
		Scope:      value(t, tfdomain.NewHandoverScopeReference, "scope/1"),
		ReleasedBy: value(t, tfdomain.NewHandoverPartyReference, "party-trunk"),
		ReceivedBy: value(t, tfdomain.NewHandoverPartyReference, "party-sin-hub"),
		Verdict:    spec.verdict,
		Version:    value(t, tfdomain.NewHandoverResultVersion, spec.version),
		JudgedAt:   handedOverAt,
		Nodes:      nodes,
	}
	if spec.verdict == tfdomain.ObjectHandedOver {
		formed.ReleasingEvidence = value(t, tfdomain.NewHandoverEvidenceReference, "evidence-release")
		formed.ReceivingEvidence = value(t, tfdomain.NewHandoverEvidenceReference, "evidence-receive")
		formed.Rule = value(t, tfdomain.NewHandoverRuleReference, "rule/v1")
	} else {
		formed.Basis = value(t, tfdomain.NewHandoverBasisReference, "basis-1")
	}
	handover, err := tfdomain.FormTransportHandover(formed)
	if err != nil {
		t.Fatalf("form handover: %v", err)
	}
	return handover
}

type handoverFinderDouble struct {
	records map[tfports.TransportHandoverKey]tfports.TransportHandoverRecord
	err     error
}

func newHandoverFinder(handovers ...tfdomain.TransportHandover) *handoverFinderDouble {
	finder := &handoverFinderDouble{records: map[tfports.TransportHandoverKey]tfports.TransportHandoverRecord{}}
	for _, handover := range handovers {
		finder.hold(handover)
	}
	return finder
}

func (double *handoverFinderDouble) hold(handover tfdomain.TransportHandover) {
	key := keyOf(handover)
	double.records[key] = tfports.TransportHandoverRecord{Key: key, Handover: handover, RecordedAt: handedOverAt.Add(time.Minute)}
}

func keyOf(handover tfdomain.TransportHandover) tfports.TransportHandoverKey {
	return tfports.TransportHandoverKey{
		TenantID: handover.TenantID(), Object: handover.Object(), Scope: handover.Scope(), Version: handover.Version(),
	}
}

func (double *handoverFinderDouble) FindByKey(
	_ context.Context, key tfports.TransportHandoverKey,
) (tfports.TransportHandoverRecord, bool, error) {
	if double.err != nil {
		return tfports.TransportHandoverRecord{}, false, double.err
	}
	record, found := double.records[key]
	return record, found, nil
}

func envelopeOf(handover tfdomain.TransportHandover) nrinbox.RegisteredTransportHandover {
	return nrinbox.RegisteredTransportHandover{
		TenantID: handover.TenantID().String(),
		Object:   handover.Object().String(),
		Scope:    handover.Scope().String(),
		Version:  handover.Version().String(),
	}
}

// ---- PS 一侧替身 ----

type acceptedLookupDouble struct {
	accepted nrparcelshipment.AcceptedCarriedGeo
	found    bool
	err      error
	asked    []nrdomain.DeclaredParcelID
}

func (double *acceptedLookupDouble) LoadAcceptedCarriedGeo(
	_ context.Context, _ nrdomain.TenantID, parcel nrdomain.DeclaredParcelID,
) (nrparcelshipment.AcceptedCarriedGeo, bool, error) {
	double.asked = append(double.asked, parcel)
	return double.accepted, double.found, double.err
}

func baselineGeo() nrdomain.GeoResolutionProjection {
	return nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide("CN", true, "200000", true),
		nrdomain.NewGeoResolutionSide("SG", true, "018956", true),
	)
}

func acceptedParcel(t *testing.T) *acceptedLookupDouble {
	t.Helper()
	return &acceptedLookupDouble{found: true, accepted: nrparcelshipment.AcceptedCarriedGeo{
		CustomerAccountID:  value(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  value(t, nrdomain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: value(t, nrdomain.NewAcceptanceBaselineReference, "version-1"),
		Geo:                baselineGeo(),
	}}
}

func judgmentKey(t *testing.T) nrdomain.InitialRouteJudgmentKey {
	t.Helper()
	return nrdomain.InitialRouteJudgmentKey{
		TenantID:           value(t, nrdomain.NewTenantID, "tenant-1"),
		CustomerAccountID:  value(t, nrdomain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID:  value(t, nrdomain.NewShipmentRequestID, "request-1"),
		AcceptanceBaseline: value(t, nrdomain.NewAcceptanceBaselineReference, "version-1"),
		DeclaredParcelID:   value(t, nrdomain.NewDeclaredParcelID, "parcel-1"),
		ServicePurpose:     value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	}
}

// ---- 判断库替身 ----

type routeStoreDouble struct {
	records map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord
}

func (double *routeStoreDouble) FindByKey(
	_ context.Context, key nrdomain.InitialRouteJudgmentKey,
) (nrports.InitialRouteRecord, bool, error) {
	record, found := double.records[key]
	return record, found, nil
}

func (double *routeStoreDouble) Save(context.Context, nrports.InitialRouteRecord) (nrports.InitialRouteSaveOutcome, error) {
	return 0, errors.New("not part of this seam")
}

// evidenceSpy 记下复核取证据时交进来的携带内容。err 是取数依赖故障，unconfigured 是判断时点没有适用的网络证据：
// 前者重投推得动，后者推不动，两格要分开演。
type evidenceSpy struct {
	evidence     nrports.InitialRouteEvidence
	seen         []nrports.RequestCarriedContent
	err          error
	unconfigured bool
}

func (spy *evidenceSpy) LoadInitialRouteEvidence(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey, carried nrports.RequestCarriedContent,
) (nrports.InitialRouteEvidence, bool, error) {
	spy.seen = append(spy.seen, carried)
	if spy.err != nil {
		return nrports.InitialRouteEvidence{}, false, spy.err
	}
	return spy.evidence, !spy.unconfigured, nil
}

type applicabilityStoreDouble struct {
	byPlan map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability
}

func (double *applicabilityStoreDouble) FindByPlan(
	_ context.Context, plan nrdomain.RoutePlanVersionID,
) (nrdomain.PlanApplicability, bool, error) {
	applicability, found := double.byPlan[plan]
	return applicability, found, nil
}

func (double *applicabilityStoreDouble) Save(_ context.Context, applicability nrdomain.PlanApplicability) error {
	double.byPlan[applicability.Plan()] = applicability
	return nil
}

type reassessStoreDouble struct {
	byCorrelation map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord
	notJudged     map[nrdomain.RequestCorrelationID]nrports.NotJudgedTrigger
}

// LatestTriggerAt 与真库同一口径：同判断键、带业务时间的复核里取最晚那一次。
func (double *reassessStoreDouble) LatestTriggerAt(
	_ context.Context, key nrdomain.InitialRouteJudgmentKey,
) (time.Time, bool, error) {
	var latest time.Time
	for _, record := range double.byCorrelation {
		if record.Key == key && record.TriggerOccurredAt.After(latest) {
			latest = record.TriggerOccurredAt
		}
	}
	return latest, !latest.IsZero(), nil
}

func (double *reassessStoreDouble) RecordNotJudged(_ context.Context, record nrports.NotJudgedTrigger) error {
	if double.notJudged == nil {
		double.notJudged = map[nrdomain.RequestCorrelationID]nrports.NotJudgedTrigger{}
	}
	if _, exists := double.notJudged[record.Correlation]; !exists {
		double.notJudged[record.Correlation] = record
	}
	return nil
}

func (double *reassessStoreDouble) FindByCorrelation(
	_ context.Context, _ nrdomain.TenantID, correlation nrdomain.RequestCorrelationID,
) (nrports.ReassessmentRecord, bool, error) {
	record, found := double.byCorrelation[correlation]
	return record, found, nil
}

func (double *reassessStoreDouble) Save(
	_ context.Context, correlation nrdomain.RequestCorrelationID, record nrports.ReassessmentRecord,
) (nrports.ReassessmentSaveOutcome, error) {
	if _, exists := double.byCorrelation[correlation]; exists {
		return nrports.ReassessmentAlreadyRecorded, nil
	}
	double.byCorrelation[correlation] = record
	return nrports.ReassessmentSaved, nil
}

type logDouble struct {
	digests map[nrdomain.RequestCorrelationID]string
}

func (double *logDouble) FindDigest(
	_ context.Context, _ nrdomain.TenantID, correlation nrdomain.RequestCorrelationID,
) (string, bool, error) {
	digest, found := double.digests[correlation]
	return digest, found, nil
}

func (double *logDouble) Append(
	_ context.Context, _ nrdomain.TenantID, correlation nrdomain.RequestCorrelationID, digest string,
) error {
	double.digests[correlation] = digest
	return nil
}

type identityDouble struct{ next int }

func (double *identityDouble) NextRoutePlanVersionID(context.Context) (nrdomain.RoutePlanVersionID, error) {
	double.next++
	return nrdomain.NewRoutePlanVersionID(fmt.Sprintf("plan-%d", double.next))
}

type fixedClock struct{ at time.Time }

func (clock fixedClock) Now() time.Time { return clock.at }

// planThroughSingapore 是一份真经 NR 领域形成的计划：上海枢纽 → 新加坡枢纽 → 新加坡末端。
func planThroughSingapore(t *testing.T) nrports.InitialRouteRecord {
	t.Helper()
	leg := func(from, to string, start time.Time) nrdomain.PlannedLeg {
		window, err := nrdomain.NewPlannedTimeWindow(start, start.Add(24*time.Hour),
			value(t, nrdomain.NewWindowBasisReference, "CALENDAR-V1"))
		if err != nil {
			t.Fatalf("window: %v", err)
		}
		built, err := nrdomain.NewPlannedLeg(nrdomain.PlannedLegSpec{
			From:        value(t, nrdomain.NewPlanNodeReference, from),
			To:          value(t, nrdomain.NewPlanNodeReference, to),
			Responsible: value(t, nrdomain.NewResponsiblePartyReference, "party-1"),
			Window:      window,
		})
		if err != nil {
			t.Fatalf("leg: %v", err)
		}
		return built
	}
	qualified, err := nrdomain.NewRouteCandidate(value(t, nrdomain.NewCandidateID, "candidate-1"),
		nrdomain.CandidateQualified, nrdomain.CandidateReason{})
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	plan, err := nrdomain.FormInitialRoutePlan(nrdomain.InitialRoutePlanSpec{
		Key:        judgmentKey(t),
		Version:    value(t, nrdomain.NewRoutePlanVersionID, "plan-1/v1"),
		Selected:   value(t, nrdomain.NewCandidateID, "candidate-1"),
		Candidates: []nrdomain.RouteCandidate{qualified},
		Legs: []nrdomain.PlannedLeg{
			leg(planOrigin, planTransfer, handedOverAt.Add(-48*time.Hour)),
			leg(planTransfer, planLastMile, handedOverAt.Add(-24*time.Hour)),
		},
		Strategy:      value(t, nrdomain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision:  value(t, nrdomain.NewNetworkViewRevision, "net-view-rev-1"),
		JudgedAt:      handedOverAt.Add(-72 * time.Hour),
		EffectiveFrom: handedOverAt.Add(-72 * time.Hour),
	})
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return nrports.InitialRouteRecord{Key: judgmentKey(t), Plan: plan, HasPlan: true}
}

// openFreeze 声明剩余段数形态，限额 0：在新加坡枢纽还剩一段，未越过边界，仍适用。未声明不是未冻结。
func openFreeze(t *testing.T) nrports.InitialRouteEvidence {
	t.Helper()
	limit := 0
	return nrports.InitialRouteEvidence{
		Strategy:                    value(t, nrdomain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision:                value(t, nrdomain.NewNetworkViewRevision, "net-view-rev-1"),
		FreezeForm:                  nrdomain.RemainingSegmentCountFreeze,
		FreezeRemainingSegmentLimit: &limit,
	}
}

type subject struct {
	adapter         *adapter.ReassessOnHandoverAdapter
	evidence        *evidenceSpy
	reassessments   *reassessStoreDouble
	applicabilities *applicabilityStoreDouble
	log             *logDouble
}

func newSubject(
	t *testing.T,
	handovers *handoverFinderDouble,
	accepted *acceptedLookupDouble,
	withHistory bool,
) *subject {
	t.Helper()
	routes := &routeStoreDouble{records: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{}}
	if withHistory {
		routes.records[judgmentKey(t)] = planThroughSingapore(t)
	}
	built := &subject{
		evidence:        &evidenceSpy{evidence: openFreeze(t)},
		reassessments:   &reassessStoreDouble{byCorrelation: map[nrdomain.RequestCorrelationID]nrports.ReassessmentRecord{}},
		applicabilities: &applicabilityStoreDouble{byPlan: map[nrdomain.RoutePlanVersionID]nrdomain.PlanApplicability{}},
		log:             &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
	}
	handler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        routes,
		Evidence:      built.evidence,
		Applicability: built.applicabilities,
		Store:         built.reassessments,
		Log:           built.log,
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: handedOverAt.Add(5 * time.Minute)},
	})
	reassessOnHandover, err := adapter.NewReassessOnHandoverAdapter(handovers, accepted, handler,
		value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"))
	if err != nil {
		t.Fatalf("construct adapter: %v", err)
	}
	built.adapter = reassessOnHandover
	return built
}

func (built *subject) deliver(t *testing.T, handover tfdomain.TransportHandover) error {
	t.Helper()
	return built.adapter.HandleRegisteredTransportHandover(context.Background(), envelopeOf(handover))
}

func (built *subject) only(t *testing.T) nrports.ReassessmentRecord {
	t.Helper()
	if len(built.reassessments.byCorrelation) != 1 {
		t.Fatalf("复核 = %d 条, want 1", len(built.reassessments.byCorrelation))
	}
	for _, record := range built.reassessments.byCorrelation {
		return record
	}
	return nrports.ReassessmentRecord{}
}

// Covers: AT-NR-064——`已交接`且声明接收节点：一条复核，键落在已接受委托的基线上，关联以交接那一版认身份；所携投影就是
// 受理基线那一版的，摘要相同；接收节点在计划上，原计划仍适用。
func TestAHandoverIntoAPlannedNodeIsReassessedWithTheBaselineGeo(t *testing.T) {
	handover := formHandover(t, handoverSpec{
		object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver,
		receiving: planTransfer, releasing: "SYN-NODE-SZX-GATE",
	})
	accepted := acceptedParcel(t)
	built := newSubject(t, newHandoverFinder(handover), accepted, true)

	if err := built.deliver(t, handover); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	record := built.only(t)
	if record.Conclusion != nrports.ReassessmentStillApplicable {
		t.Fatalf("结论 = %v, want 仍适用", record.Conclusion)
	}
	if record.Key != judgmentKey(t) {
		t.Fatalf("判断键 = %+v, want %+v", record.Key, judgmentKey(t))
	}
	if want := "handover/8:tenant-1/8:parcel-1/7:scope/1/11:handover/v1"; record.Correlation.String() != want {
		t.Fatalf("关联 = %q, want %q", record.Correlation, want)
	}
	if len(accepted.asked) != 1 || accepted.asked[0].String() != "parcel-1" {
		t.Fatalf("问 PS 的包裹 = %v, want [parcel-1]", accepted.asked)
	}
	if len(built.evidence.seen) != 1 || built.evidence.seen[0].Geo.ContentDigest() != baselineGeo().ContentDigest() {
		t.Fatalf("复核取证据时所携投影 = %+v, want 受理基线那一版的", built.evidence.seen)
	}
}

// Covers: AT-NR-064（位置那一半）——位置取的是接收节点：接收节点不在计划上，原计划按「实际节点与计划不符」失效，失效依据
// 点名这个节点，失效时刻是交接的业务时间，不是处理时钟。
func TestAHandoverIntoAnUnplannedNodeLapsesThePlanAtThatNode(t *testing.T) {
	handover := formHandover(t, handoverSpec{
		object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: "SYN-NODE-KUL-HUB",
	})
	built := newSubject(t, newHandoverFinder(handover), acceptedParcel(t), true)

	if err := built.deliver(t, handover); err != nil {
		t.Fatalf("deliver: %v", err)
	}
	record := built.only(t)
	if record.Conclusion != nrports.ReassessmentPlanLapsed {
		t.Fatalf("结论 = %v, want 失效", record.Conclusion)
	}
	if record.LapseBasis.String() != "PLAN_ORIGIN_MISMATCH/SYN-NODE-KUL-HUB" {
		t.Fatalf("失效依据 = %q, want 点名接收节点", record.LapseBasis)
	}
	applicability, found := built.applicabilities.byPlan[value(t, nrdomain.NewRoutePlanVersionID, "plan-1/v1")]
	if !found || !applicability.TransitionedAt().Equal(handedOverAt) {
		t.Fatalf("适用性 = %+v found=%v, want 失效于交接业务时间 %s", applicability, found, handedOverAt)
	}
}

// Covers: AT-NR-065——拒收、待确认、`已交接`而没声明接收节点（交给运输方，只声明了交出节点）：都入账，不复核，也不去问 PS。
func TestHandoversThatDoNotEnterANodeSettleWithoutReassessment(t *testing.T) {
	for name, spec := range map[string]handoverSpec{
		"拒收":       {object: "parcel-1", version: "handover/v1", verdict: tfdomain.HandoverRefused, receiving: planTransfer},
		"待确认":      {object: "parcel-1", version: "handover/v1", verdict: tfdomain.HandoverPendingConfirmation, receiving: planTransfer},
		"交给运输方":    {object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, releasing: planOrigin},
		"一格节点都没声明": {object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver},
	} {
		t.Run(name, func(t *testing.T) {
			handover := formHandover(t, spec)
			accepted := acceptedParcel(t)
			built := newSubject(t, newHandoverFinder(handover), accepted, true)
			if err := built.deliver(t, handover); err != nil {
				t.Fatalf("deliver = %v, want 入账", err)
			}
			if n := len(built.reassessments.byCorrelation); n != 0 {
				t.Fatalf("复核 = %d 条, want 0", n)
			}
			if len(accepted.asked) != 0 {
				t.Fatalf("不是复核触发还问了 PS %d 次", len(accepted.asked))
			}
		})
	}
}

// Covers: AT-NR-066——对象不是当前已接受包裹（集运单元号、没有已接受委托声明它）：入账，零复核，不停在未决等它出现。
func TestAHandoverOfAnObjectThatIsNotACurrentAcceptedParcelSettles(t *testing.T) {
	handover := formHandover(t, handoverSpec{
		object: "SYN-UNIT-01", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: planTransfer,
	})
	built := newSubject(t, newHandoverFinder(handover), &acceptedLookupDouble{}, true)
	if err := built.deliver(t, handover); err != nil {
		t.Fatalf("deliver = %v, want 入账", err)
	}
	if n := len(built.reassessments.byCorrelation); n != 0 {
		t.Fatalf("复核 = %d 条, want 0", n)
	}
}

// Covers: AT-NR-067——交接读不回或读不动、PS 读不动、复核停在依赖类未决（证据读不动）：各答本路未决哨兵之一，消费门回滚
// 重投，也不落未判；仓储不变量破了与本包缺陷响亮上抛，不落在那几格里。
func TestUnreadableDependenciesRollBackAndDefectsAreLoud(t *testing.T) {
	handover := formHandover(t, handoverSpec{
		object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: planTransfer,
	})
	retryable := []error{
		adapter.ErrHandoverNotVisible, nrparcelshipment.ErrAcceptedParcelUnreadable, adapter.ErrHandoverReassessmentUndecided,
	}
	isRetryable := func(err error) bool {
		for _, sentinel := range retryable {
			if errors.Is(err, sentinel) {
				return true
			}
		}
		return false
	}

	mismatched := newHandoverFinder()
	other := formHandover(t, handoverSpec{
		object: "parcel-2", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: planTransfer,
	})
	mismatched.records[keyOf(handover)] = tfports.TransportHandoverRecord{Key: keyOf(handover), Handover: other}

	cases := map[string]struct {
		handovers   *handoverFinderDouble
		accepted    *acceptedLookupDouble
		evidenceErr error
		want        error
		retry       bool
	}{
		"交接还读不回":   {newHandoverFinder(), acceptedParcel(t), nil, adapter.ErrHandoverNotVisible, true},
		"交接登记册读不动": {&handoverFinderDouble{err: errors.New("pool closed")}, acceptedParcel(t), nil, adapter.ErrHandoverNotVisible, true},
		"PS 读不动": {
			newHandoverFinder(handover), &acceptedLookupDouble{err: nrparcelshipment.ErrAcceptedParcelUnreadable}, nil,
			nrparcelshipment.ErrAcceptedParcelUnreadable, true,
		},
		"复核未决：证据读不动": {
			newHandoverFinder(handover), acceptedParcel(t), errors.New("catalog read timed out"),
			adapter.ErrHandoverReassessmentUndecided, true,
		},
		"读回的交接指着另一个对象": {mismatched, acceptedParcel(t), nil, adapter.ErrHandoverRecordInconsistent, false},
		"已接受委托自相矛盾": {
			newHandoverFinder(handover), &acceptedLookupDouble{err: nrparcelshipment.ErrAcceptedRequestInconsistent}, nil,
			nrparcelshipment.ErrAcceptedRequestInconsistent, false,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			built := newSubject(t, tc.handovers, tc.accepted, true)
			built.evidence.err = tc.evidenceErr
			err := built.deliver(t, handover)
			if !errors.Is(err, tc.want) {
				t.Fatalf("err = %v, want %v", err, tc.want)
			}
			if isRetryable(err) != tc.retry {
				t.Fatalf("可重投 = %v, want %v：%v", isRetryable(err), tc.retry, err)
			}
			if n := len(built.reassessments.byCorrelation); n != 0 {
				t.Fatalf("停下了却落了 %d 条复核", n)
			}
			if n := len(built.reassessments.notJudged); n != 0 {
				t.Fatalf("重投那几格却落了 %d 行未判", n)
			}
		})
	}
}

// Covers: AT-NR-074、AT-NR-075、AT-NR-076（交接一路）——没有路由历史、证据未配置、冻结形态未配置：重投推不动，入账（交回
// nil，不登本路未决哨兵），原因落未判登记，零复核。
func TestReassessmentsThatCannotBeJudgedYetSettleWithTheirReason(t *testing.T) {
	handover := formHandover(t, handoverSpec{
		object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: planTransfer,
	})
	cases := map[string]struct {
		withHistory  bool
		unconfigured bool
		undeclared   bool
		want         nrports.NotJudgedReason
	}{
		"没有路由历史":  {withHistory: false, want: nrports.NotJudgedNoRoutingHistory},
		"证据未配置":   {withHistory: true, unconfigured: true, want: nrports.NotJudgedEvidenceNotConfigured},
		"冻结形态未配置": {withHistory: true, undeclared: true, want: nrports.NotJudgedFreezeFormUnconfigured},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			built := newSubject(t, newHandoverFinder(handover), acceptedParcel(t), tc.withHistory)
			built.evidence.unconfigured = tc.unconfigured
			if tc.undeclared {
				built.evidence.evidence.FreezeForm = nrdomain.FreezeFormUndeclared
				built.evidence.evidence.FreezeRemainingSegmentLimit = nil
			}
			if err := built.deliver(t, handover); err != nil {
				t.Fatalf("deliver = %v, want 入账", err)
			}
			if n := len(built.reassessments.byCorrelation); n != 0 {
				t.Fatalf("未判却落了 %d 条复核", n)
			}
			if len(built.reassessments.notJudged) != 1 {
				t.Fatalf("未判登记 = %d 行, want 1", len(built.reassessments.notJudged))
			}
			for _, noted := range built.reassessments.notJudged {
				if noted.Reason != tc.want || !noted.TriggerOccurredAt.Equal(handedOverAt) {
					t.Fatalf("未判登记 = %+v, want 原因 %s、业务时间取交接", noted, tc.want)
				}
			}
		})
	}
}

// Covers: AT-NR-068——同一版交接再投一次（另一封信封、或消费门之外的重放）：答已有结果入账，不重复复核。
func TestTheSameHandoverDeliveredAgainAnswersTheExistingResult(t *testing.T) {
	handover := formHandover(t, handoverSpec{
		object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: planTransfer,
	})
	built := newSubject(t, newHandoverFinder(handover), acceptedParcel(t), true)
	for range 2 {
		if err := built.deliver(t, handover); err != nil {
			t.Fatalf("deliver: %v", err)
		}
	}
	built.only(t)
	if len(built.evidence.seen) != 1 {
		t.Fatalf("取证据 %d 次, want 1——重放不该再评一次", len(built.evidence.seen))
	}
}

// Covers: AT-NR-069——更正后仍`已交接`：新版本承继接收节点，另成一次复核，原复核不被覆盖；更正成拒收：入账，不复核，也不
// 回退已有的复核。AT-NR-081（更正那一半）——更正承继原版的判断时刻，与当前位置同一时刻，不算更早，照判。
func TestACorrectionIsReassessedAgainAndAWithdrawalLeavesEarlierReassessmentsAlone(t *testing.T) {
	original := formHandover(t, handoverSpec{
		object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: planTransfer,
	})
	restated, err := original.Correct(tfdomain.HandoverCorrection{
		Verdict:           tfdomain.ObjectHandedOver,
		ReleasingEvidence: value(t, tfdomain.NewHandoverEvidenceReference, "evidence-release-2"),
		ReceivingEvidence: value(t, tfdomain.NewHandoverEvidenceReference, "evidence-receive-2"),
		Rule:              value(t, tfdomain.NewHandoverRuleReference, "rule/v1"),
		Version:           value(t, tfdomain.NewHandoverResultVersion, "handover/v2"),
		CorrectedAt:       handedOverAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	withdrawn, err := restated.Correct(tfdomain.HandoverCorrection{
		Verdict:     tfdomain.HandoverRefused,
		Basis:       value(t, tfdomain.NewHandoverBasisReference, "seal-mismatch"),
		Version:     value(t, tfdomain.NewHandoverResultVersion, "handover/v3"),
		CorrectedAt: handedOverAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("withdraw: %v", err)
	}
	built := newSubject(t, newHandoverFinder(original, restated, withdrawn), acceptedParcel(t), true)

	for _, handover := range []tfdomain.TransportHandover{original, restated, withdrawn} {
		if err := built.deliver(t, handover); err != nil {
			t.Fatalf("deliver %s: %v", handover.Version(), err)
		}
	}
	if n := len(built.reassessments.byCorrelation); n != 2 {
		t.Fatalf("复核 = %d 条, want 2（原版与仍`已交接`的更正各一条，拒收那一版没有）", n)
	}
	for _, record := range built.reassessments.byCorrelation {
		if record.Conclusion != nrports.ReassessmentStillApplicable {
			t.Fatalf("%s 结论 = %v, want 仍适用——拒收的更正不回退已有复核", record.Correlation, record.Conclusion)
		}
	}
}

// 服务目的是装配参数，没配就停下不猜：响亮上抛，不读交接。
func TestAnUnconfiguredPurposeIsLoud(t *testing.T) {
	handover := formHandover(t, handoverSpec{
		object: "parcel-1", version: "handover/v1", verdict: tfdomain.ObjectHandedOver, receiving: planTransfer,
	})
	reassess := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{})
	unconfigured, err := adapter.NewReassessOnHandoverAdapter(newHandoverFinder(handover), acceptedParcel(t), reassess,
		nrdomain.ServicePurpose{})
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	if err := unconfigured.HandleRegisteredTransportHandover(context.Background(), envelopeOf(handover)); !errors.Is(
		err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
}
