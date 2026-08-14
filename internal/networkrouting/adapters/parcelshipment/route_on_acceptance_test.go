package parcelshipment_test

import (
	"context"
	"errors"
	"testing"
	"time"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证 AcceptanceConsumer 的真实处理方：接受决定信封按引用取回接受基线，逐成员
// 走真实的 UC-NR-001 编排，并把编排结果折成消费门认得的两格（入账 / 回滚重投）。

var acceptedAt = time.Date(2026, 8, 14, 9, 0, 0, 0, time.UTC)

// Covers: UC-NR-001 输入组与步骤 4「按接受基线逐包裹建立独立判断范围」经适配器端到端
// ——信封只带引用，基线、成员与接受时间全取自 PS 的委托聚合，逐成员形成真实结果。
func TestAnAcceptedDecisionRoutesTheBaselineMembers(t *testing.T) {
	subject, routes := newRouteOnAcceptance(t, excludedEvidence(t), nil)

	if err := subject.HandleAcceptedDecision(context.Background(), acceptedEnvelope()); err != nil {
		t.Fatalf("处理接受决定：%v", err)
	}

	if len(routes.saved) != 1 {
		t.Fatalf("落库结果 %d 份，want 1（基线只有一个成员）", len(routes.saved))
	}
	for key, record := range routes.saved {
		// 判断键的基线维取自基线本体的提交版本，与 ReassessOnIntakeAdapter 同源：
		// 两条链指不到同一个键，同一个包裹的初判与复核就会分家。
		if key.AcceptanceBaseline.String() != "version-1" {
			t.Fatalf("基线引用 = %q, want version-1", key.AcceptanceBaseline)
		}
		if key.DeclaredParcelID.String() != "parcel-1" {
			t.Fatalf("成员 = %q, want parcel-1", key.DeclaredParcelID)
		}
		if key.ServicePurpose.String() != "NETWORK_SERVICE" {
			t.Fatalf("服务目的 = %q", key.ServicePurpose)
		}
		if !record.HasNoRoute {
			t.Fatal("候选被区域明确排除，应形成`无当前有效路由`")
		}
	}
}

// Covers: UC-NR-001 启动条件「只有状态字符串而没有基线引用时不得继续」——信封说已接受
// 而权威的基线钉在另一个提交版本上时停下，不拿状态字当基线用。
func TestAnEnvelopeDisagreeingWithTheAuthorityStops(t *testing.T) {
	subject, routes := newRouteOnAcceptance(t, excludedEvidence(t), nil)

	stale := acceptedEnvelope()
	stale.SubmissionVersion = "version-0"

	err := subject.HandleAcceptedDecision(context.Background(), stale)
	if !errors.Is(err, adapter.ErrEnvelopeContradictsAuthority) {
		t.Fatalf("err = %v, want ErrEnvelopeContradictsAuthority", err)
	}
	if len(routes.saved) != 0 {
		t.Fatal("信封与权威不一致却还是路由了")
	}
}

// Covers: 同一事件类型同时承载接受与拒绝决定——拒绝没有可路由的东西，是终局答案不是
// 失败：入账收工，不重投也不惊动编排。
func TestARejectedDecisionIsAccountedWithoutRouting(t *testing.T) {
	subject, routes := newRouteOnAcceptance(t, excludedEvidence(t), nil)

	rejected := acceptedEnvelope()
	rejected.State = "REJECTED"

	if err := subject.HandleAcceptedDecision(context.Background(), rejected); err != nil {
		t.Fatalf("拒绝决定应入账收工，实得：%v", err)
	}
	if len(routes.saved) != 0 {
		t.Fatal("拒绝决定却路由了")
	}
}

// Covers: 消费门第三条「处理失败整体回滚可重投」在真实编排上的落点——包裹停在未决时
// 上抛，让投递回滚重投；就此入账会把这个包裹的路由义务永久丢掉，而本仓没有重驱动器
// 会回来捡它。
func TestAnUndecidedParcelRollsTheDeliveryBack(t *testing.T) {
	subject, routes := newRouteOnAcceptance(t, nrports.InitialRouteEvidence{}, errors.New("evidence down"))

	err := subject.HandleAcceptedDecision(context.Background(), acceptedEnvelope())
	if !errors.Is(err, adapter.ErrRouteHandoffUndecided) {
		t.Fatalf("err = %v, want ErrRouteHandoffUndecided", err)
	}
	if len(routes.saved) != 0 {
		t.Fatal("未决却落了库")
	}
}

// Covers: 服务目的未配置停下不猜（与 ReassessOnIntakeAdapter 同一条装配纪律）。
func TestRoutingOnAcceptanceNeedsAConfiguredPurpose(t *testing.T) {
	requests := &acceptedRequestSourceDouble{request: acceptedShipmentRequest(t)}
	routes := newAcceptanceRouteStore()
	handler := nrapplication.NewCreateInitialRouteHandler(nrapplication.CreateInitialRouteDeps{
		Applicability: routingApplicabilityDouble{},
		Evidence:      acceptanceEvidenceDouble{evidence: excludedEvidence(t)},
		Store:         routes,
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Downstream:    routeDownstreamDouble{},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: acceptedAt},
	})
	subject, err := adapter.NewRouteOnAcceptanceAdapter(requests, handler, nrdomain.ServicePurpose{})
	if err != nil {
		t.Fatalf("构造适配器：%v", err)
	}

	if err := subject.HandleAcceptedDecision(context.Background(), acceptedEnvelope()); !errors.Is(
		err, adapter.ErrUntranslatableAnswer,
	) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
}

// ---- 夹具 ----

func acceptedEnvelope() nrinbox.AcceptedDecision {
	return nrinbox.AcceptedDecision{
		TenantID:          "tenant-1",
		CustomerAccountID: "customer-1",
		Source:            "source-a",
		SourceRequestKey:  "key-1",
		ShipmentRequestID: "request-1",
		SubmissionVersion: "version-1",
		DecisionID:        "decision-1",
		State:             "ACCEPTED",
	}
}

func newRouteOnAcceptance(
	t *testing.T,
	evidence nrports.InitialRouteEvidence,
	evidenceErr error,
) (*adapter.RouteOnAcceptanceAdapter, *acceptanceRouteStore) {
	t.Helper()

	routes := newAcceptanceRouteStore()
	handler := nrapplication.NewCreateInitialRouteHandler(nrapplication.CreateInitialRouteDeps{
		Applicability: routingApplicabilityDouble{},
		Evidence:      acceptanceEvidenceDouble{evidence: evidence, err: evidenceErr},
		Store:         routes,
		Log:           &logDouble{digests: map[nrdomain.RequestCorrelationID]string{}},
		Downstream:    routeDownstreamDouble{},
		Identities:    &identityDouble{},
		Clock:         fixedClock{at: acceptedAt},
	})
	subject, err := adapter.NewRouteOnAcceptanceAdapter(
		&acceptedRequestSourceDouble{request: acceptedShipmentRequest(t)},
		handler,
		value(t, nrdomain.NewServicePurpose, "NETWORK_SERVICE"),
	)
	if err != nil {
		t.Fatalf("构造适配器：%v", err)
	}
	return subject, routes
}

// excludedEvidence 造一份「候选被已发布服务区域明确排除」的证据：全部候选确定性淘汰，
// 编排因而形成`无当前有效路由`——一个真实提交结果，比计划少一堆排序输入。
func excludedEvidence(t *testing.T) nrports.InitialRouteEvidence {
	t.Helper()
	resolution, err := nrdomain.NewServiceAreaResolution(nrdomain.ServiceAreaResolutionSpec{
		Candidate:   value(t, nrdomain.NewCandidateID, "candidate-1"),
		Outcome:     nrdomain.AreaExcludesDestination,
		AreaVersion: value(t, nrdomain.NewServiceAreaVersionReference, "area-v1"),
	})
	if err != nil {
		t.Fatalf("service area resolution: %v", err)
	}
	return nrports.InitialRouteEvidence{
		ServiceAreas: []nrdomain.ServiceAreaResolution{resolution},
		Priority:     []nrdomain.RankingCriterion{value(t, nrdomain.NewRankingCriterion, "TRANSIT_TIME")},
		Strategy:     value(t, nrdomain.NewRouteStrategyReference, "strategy-1/v1"),
		ViewRevision: value(t, nrdomain.NewNetworkViewRevision, "net-view-rev-1"),
	}
}

type acceptedRequestSourceDouble struct {
	request psdomain.ShipmentRequest
	err     error
}

func (double *acceptedRequestSourceDouble) FindBySourceIdentity(
	_ context.Context,
	_ psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	if double.err != nil {
		return psdomain.ShipmentRequest{}, false, double.err
	}
	return double.request, true, nil
}

type acceptanceEvidenceDouble struct {
	evidence nrports.InitialRouteEvidence
	err      error
}

func (double acceptanceEvidenceDouble) LoadInitialRouteEvidence(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey,
) (nrports.InitialRouteEvidence, error) {
	if double.err != nil {
		return nrports.InitialRouteEvidence{}, double.err
	}
	return double.evidence, nil
}

type routingApplicabilityDouble struct{}

func (routingApplicabilityDouble) AssessRoutingApplicability(
	_ context.Context, _ nrdomain.InitialRouteJudgmentKey,
) (nrdomain.NetworkEligibility, error) {
	return nrdomain.NewNetworkEligibility(nrdomain.NetworkJudgmentRequired, nrdomain.EligibilityBasisReference{})
}

type routeDownstreamDouble struct{}

func (routeDownstreamDouble) HandOffInitialRoute(_ context.Context, _ nrports.InitialRouteHandoffIntent) error {
	return nil
}

// acceptanceRouteStore 是一份会真正记住写入的路由库替身——本文件要看的正是「哪些判断
// 键落了库」。
type acceptanceRouteStore struct {
	saved map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord
}

func newAcceptanceRouteStore() *acceptanceRouteStore {
	return &acceptanceRouteStore{saved: map[nrdomain.InitialRouteJudgmentKey]nrports.InitialRouteRecord{}}
}

func (double *acceptanceRouteStore) FindByKey(
	_ context.Context, key nrdomain.InitialRouteJudgmentKey,
) (nrports.InitialRouteRecord, bool, error) {
	record, found := double.saved[key]
	return record, found, nil
}

func (double *acceptanceRouteStore) Save(
	_ context.Context, record nrports.InitialRouteRecord,
) (nrports.InitialRouteSaveOutcome, error) {
	if _, exists := double.saved[record.Key]; exists {
		return nrports.InitialRouteAlreadyRecorded, nil
	}
	double.saved[record.Key] = record
	return nrports.InitialRouteSaved, nil
}

// acceptedShipmentRequest 真经 PS 领域把委托推到已接受（假状态钉不住基线取值的来处）。
func acceptedShipmentRequest(t *testing.T) psdomain.ShipmentRequest {
	t.Helper()

	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("source identity: %v", err)
	}
	fingerprint, err := psdomain.NewSourceSubmissionFingerprint(
		identity,
		value(t, psdomain.NewPayloadDigest, "digest-1"),
		acceptedAt.Add(-2*time.Hour),
		acceptedAt.Add(-2*time.Hour+time.Second),
	)
	if err != nil {
		t.Fatalf("fingerprint: %v", err)
	}
	candidate, err := psdomain.NewSubmissionCandidate(
		fingerprint,
		value(t, psdomain.NewSubmissionBatchID, "batch-1"),
		value(t, psdomain.NewShipmentRequestID, "request-1"),
		[]psdomain.DeclaredParcelID{value(t, psdomain.NewDeclaredParcelID, "parcel-1")},
	)
	if err != nil {
		t.Fatalf("candidate: %v", err)
	}
	scope, err := psdomain.NewAdmissionScope(
		value(t, psdomain.NewAdmissionScopeReference, "scope-ref-1"),
		value(t, psdomain.NewAdmissionScopeDigest, "scope-1"),
	)
	if err != nil {
		t.Fatalf("admission scope: %v", err)
	}
	validity, err := psdomain.NewOwnershipValidityInterval(
		acceptedAt.Add(-48*time.Hour), acceptedAt.Add(48*time.Hour))
	if err != nil {
		t.Fatalf("validity: %v", err)
	}
	decidedAt := acceptedAt.Add(-90 * time.Minute)
	ownership, err := psdomain.NewProductionOwnershipDecision(psdomain.ProductionOwnershipDecisionSpec{
		DecisionID:       value(t, psdomain.NewProductionOwnershipDecisionID, "decision-1"),
		Scope:            scope,
		Authority:        psdomain.ProductionAuthorityIDPParcel,
		AdmissionControl: psdomain.AdmissionControlOpen,
		RuleVersion:      value(t, psdomain.NewProductionOwnershipRuleVersion, "rule-1"),
		AsOf:             decidedAt,
		Validity:         validity,
		Revision:         value(t, psdomain.NewProductionOwnershipRevision, "rev-1"),
		DecisionAt:       decidedAt,
	})
	if err != nil {
		t.Fatalf("ownership decision: %v", err)
	}
	gate, err := psdomain.EvaluateFutureSubmissionGate(ownership, scope.Digest(),
		value(t, psdomain.NewProductionOwnershipRevision, "rev-1"), decidedAt)
	if err != nil {
		t.Fatalf("gate: %v", err)
	}
	request, err := psdomain.SubmitShipmentRequest(psdomain.SubmitShipmentRequestSpec{
		Candidate:   candidate,
		Gate:        gate,
		VersionID:   value(t, psdomain.NewSubmissionVersionID, "version-1"),
		TaskID:      value(t, psdomain.NewAcceptanceDecisionTaskID, "task-1"),
		SubmittedAt: decidedAt,
	})
	if err != nil {
		t.Fatalf("submit: %v", err)
	}

	applicable, err := psdomain.NewApplicableCheckGroups(psdomain.NetworkReachabilityCheck)
	if err != nil {
		t.Fatalf("applicable groups: %v", err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: value(t, psdomain.NewCommercialResolutionID, "RES-1"),
		RulePackage:  value(t, psdomain.NewRulePackageReference, "rules-1/v1"),
		ViewRevision: value(t, psdomain.NewCommercialViewRevision, "VIEW-1"),
		Applicable:   applicable,
		ManualReview: psdomain.ManualReviewNotRequiredByRules,
	})
	if err != nil {
		t.Fatalf("snapshot: %v", err)
	}
	check, err := psdomain.NewAcceptanceCheck(
		psdomain.NetworkReachabilityCheck,
		value(t, psdomain.NewDeclaredParcelID, "parcel-1"),
		psdomain.CheckPassed,
		psdomain.CheckReason{},
	)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	accepted, err := request.Decide(psdomain.AcceptanceDecisionSpec{
		DecisionID: value(t, psdomain.NewAcceptanceDecisionID, "decision-1"),
		Checks:     []psdomain.AcceptanceCheck{check},
		Basis:      snapshot,
		DecidedAt:  decidedAt,
	})
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	return accepted
}
