package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件证可达性证据视图从版本化网络目录折出事实（ADR-0148 决定一、二、三、五、六）：`未配置`由目录修订锚与
// 适用路由策略版本答；首版候选是判断时点适用的一条完整线路；服务区域解析按所携投影逐字比覆盖；路径可执行性
// 看临时调整；关务一格由来源作答、来源未接答状态未知；视图修订就是目录修订锚。领域评估照旧在领域——结论经
// AssessParcelReachabilityHandler 与领域管线得出，这里只替换证据视图的实现。取值一律 SYN- 合成，XA / XB / XC
// 是用户自定义码段里的合成国家码（隔离合成只记 S）。

const catalogPurpose = "SYN-PURPOSE-NETWORK"

var catalogEffectiveFrom = asOfAt.Add(-30 * 24 * time.Hour)

type catalogReadDouble struct {
	snapshot   ports.NetworkCatalogSnapshot
	configured bool
	err        error

	reads     int
	gotTenant domain.TenantID
	gotAsOf   time.Time
}

func (double *catalogReadDouble) LoadDefinitionsAt(
	_ context.Context, tenant domain.TenantID, asOf time.Time,
) (ports.NetworkCatalogSnapshot, bool, error) {
	double.reads++
	double.gotTenant, double.gotAsOf = tenant, asOf
	return double.snapshot, double.configured, double.err
}

// customsDouble 按候选作答；omit 里的候选不答，用来演练「来源漏答一条」。
type customsDouble struct {
	outcome domain.HardConstraintOutcome
	omit    map[string]bool
	err     error

	gotQuery ports.CustomsApplicabilityQuery
}

func (double *customsDouble) AssessCustomsApplicability(
	_ context.Context, query ports.CustomsApplicabilityQuery,
) ([]domain.HardConstraintFinding, error) {
	double.gotQuery = query
	if double.err != nil {
		return nil, double.err
	}
	findings := make([]domain.HardConstraintFinding, 0, len(query.Candidates))
	for _, candidate := range query.Candidates {
		if double.omit[candidate.Candidate.String()] {
			continue
		}
		spec := domain.HardConstraintFindingSpec{Candidate: candidate.Candidate, Outcome: double.outcome}
		if double.outcome == domain.RestrictionApplies {
			restriction, err := domain.NewRestrictionReference("SYN-CUSTOMS-RESTRICTION-1")
			if err != nil {
				return nil, err
			}
			spec.Restriction = restriction
		}
		finding, err := domain.NewHardConstraintFinding(spec)
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

func satisfiedCustoms() *customsDouble {
	return &customsDouble{outcome: domain.ConstraintSatisfied}
}

// syntheticCatalog 是一张最小合成网：寄件国 XA 的收寄枢纽经口岸到收件国 XB 的末端，一条线路两段；XB 的交付
// 区域只覆盖邮编前缀 10。修订锚取 7，证视图修订就是它。
func syntheticCatalog(t *testing.T) *catalogReadDouble {
	t.Helper()
	revision := value(t, domain.NewNetworkViewRevision, "7")
	return &catalogReadDouble{configured: true, snapshot: ports.NetworkCatalogSnapshot{
		Nodes: []ports.NodeDefinitionVersion{
			{Code: "SYN-NODE-ORIGIN", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffectiveFrom},
			{Code: "SYN-NODE-GATE", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffectiveFrom},
			{Code: "SYN-NODE-LAST-MILE", Version: 2, BusinessTimezone: "Asia/Singapore", EffectiveFrom: catalogEffectiveFrom},
		},
		Connections: []ports.ConnectionDefinitionVersion{
			{Code: "SYN-CONN-ORIGIN-GATE", Version: 1, FromNode: "SYN-NODE-ORIGIN", ToNode: "SYN-NODE-GATE",
				BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffectiveFrom},
			{Code: "SYN-CONN-GATE-LAST-MILE", Version: 1, FromNode: "SYN-NODE-GATE", ToNode: "SYN-NODE-LAST-MILE",
				BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogEffectiveFrom},
		},
		Lines: []ports.LineDefinitionVersion{
			{Code: "SYN-LINE-XA-XB", Version: 3, Segments: []string{"SYN-CONN-ORIGIN-GATE", "SYN-CONN-GATE-LAST-MILE"},
				BusinessTimezone: "Asia/Shanghai", ApplicableScope: catalogPurpose, EffectiveFrom: catalogEffectiveFrom},
		},
		ServiceAreas: []ports.ServiceAreaDefinitionVersion{
			{Code: "SYN-AREA-XA", Version: 1, EffectiveFrom: catalogEffectiveFrom,
				HasCoverage: true, CoverageCountry: "XA", OriginNodes: []string{"SYN-NODE-ORIGIN"}},
			{Code: "SYN-AREA-XB-10", Version: 4, EffectiveFrom: catalogEffectiveFrom,
				HasCoverage: true, CoverageCountry: "XB", PostalPrefixes: []string{"10"},
				DestinationNodes: []string{"SYN-NODE-LAST-MILE"}},
		},
		Strategies: []ports.RouteStrategyDefinitionVersion{
			{Code: "SYN-STRATEGY-1", Version: 1, ApplicableScope: catalogPurpose, EffectiveFrom: catalogEffectiveFrom},
		},
		Revision: revision,
	}}
}

func carriedGeo(senderCountry, senderPostal, deliveryCountry, deliveryPostal string) ports.RequestCarriedContent {
	return ports.RequestCarriedContent{Geo: domain.NewGeoResolutionProjection(
		domain.NewGeoResolutionSide(senderCountry, senderCountry != "", senderPostal, senderPostal != ""),
		domain.NewGeoResolutionSide(deliveryCountry, deliveryCountry != "", deliveryPostal, deliveryPostal != ""),
	)}
}

func catalogJudgmentKey(t *testing.T) domain.ReachabilityJudgmentKey {
	t.Helper()
	key := judgmentKey(t, "SYN-PARCEL-1")
	key.ServicePurpose = value(t, domain.NewServicePurpose, catalogPurpose)
	return key
}

func newCatalogEvidence(t *testing.T, catalog ports.NetworkCatalogRead, customs ports.CustomsApplicabilitySource) *application.CatalogNetworkEvidence {
	t.Helper()
	view, err := application.NewCatalogNetworkEvidence(catalog, customs)
	if err != nil {
		t.Fatalf("构造目录证据视图：%v", err)
	}
	return view
}

// assessThroughCatalog 经真领域管线跑一次可达性判断，证据视图换成目录折叠。
func assessThroughCatalog(
	t *testing.T, catalog ports.NetworkCatalogRead, customs ports.CustomsApplicabilitySource, carried ports.RequestCarriedContent,
) (application.AssessParcelReachabilityResult, *storeDouble) {
	t.Helper()
	store := &storeDouble{}
	handler := application.NewAssessParcelReachabilityHandler(
		requiredEligibility(t), newCatalogEvidence(t, catalog, customs), store, &handoffDouble{}, fixedClock{at: judgedAt})
	result, err := handler.Handle(t.Context(), application.AssessParcelReachabilityCommand{
		Correlation: value(t, domain.NewRequestCorrelationID, "SYN-CORRELATION-1"),
		Key:         catalogJudgmentKey(t),
		Carried:     carried,
	})
	if err != nil {
		t.Fatalf("可达性判断：%v", err)
	}
	return result, store
}

func formedFinding(t *testing.T, result application.AssessParcelReachabilityResult) domain.ReachabilityFinding {
	t.Helper()
	finding, ok := result.Finding()
	if result.Outcome() != application.JudgmentFormed || !ok {
		t.Fatalf("outcome = %s reason = %s，想要形成判断", result.Outcome(), result.NotFormedReason())
	}
	return finding
}

func onlyCandidate(t *testing.T, finding domain.ReachabilityFinding) domain.RouteCandidate {
	t.Helper()
	candidates := finding.Candidates()
	if len(candidates) != 1 {
		t.Fatalf("候选 = %+v，想要恰一条", candidates)
	}
	return candidates[0]
}

// Covers: ADR-0148 决定五、一——判断时点适用的完整线路即一个候选，服务区域按所携投影覆盖起止两侧、线路无生效
// 中的停运、关务来源答满足，判断为`可达`；视图修订取目录修订锚、选版时点取判断键的 asOf、关务来源收到的是
// 候选的段链。
func TestACatalogLineCoveringBothEndsIsReachable(t *testing.T) {
	catalog := syntheticCatalog(t)
	customs := satisfiedCustoms()

	result, store := assessThroughCatalog(t, catalog, customs, carriedGeo("XA", "", "XB", "100200"))

	finding := formedFinding(t, result)
	if finding.Value() != domain.Reachable {
		t.Fatalf("value = %s，想要 REACHABLE", finding.Value())
	}
	candidate := onlyCandidate(t, finding)
	if candidate.ID().String() != "SYN-LINE-XA-XB@3" || candidate.Outcome() != domain.CandidateQualified {
		t.Fatalf("候选 = %s/%s，想要线路版本 SYN-LINE-XA-XB@3 合格", candidate.ID(), candidate.Outcome())
	}
	if len(store.saved) != 1 || store.saved[0].ViewRevision.String() != "7" {
		t.Fatalf("落库判断的视图修订应为目录修订锚 7，实得 %+v", store.saved)
	}
	if !catalog.gotAsOf.Equal(asOfAt) || catalog.gotTenant.String() != "tenant-1" {
		t.Fatalf("选版取了 %s / %s，想要判断键的租户与 asOf", catalog.gotTenant, catalog.gotAsOf)
	}
	query := customs.gotQuery
	if query.Tenant.String() != "tenant-1" || !query.AsOf.Equal(asOfAt) || len(query.Candidates) != 1 {
		t.Fatalf("关务来源收到 %+v", query)
	}
	legs := query.Candidates[0].Legs
	if len(legs) != 2 || legs[0] != (ports.CustomsCandidateLeg{
		Connection: "SYN-CONN-ORIGIN-GATE", FromNode: "SYN-NODE-ORIGIN", ToNode: "SYN-NODE-GATE",
	}) || legs[1].ToNode != "SYN-NODE-LAST-MILE" {
		t.Fatalf("关务来源收到的段链 = %+v", legs)
	}
}

// Covers: UC-NR-002 矩阵行 5 与 `AT-NR-023`——已发布区域明确不覆盖寄件或收件地址，判断为`不可达`，淘汰原因
// 点名是哪一头、依据哪一版区域；收件邮编逐字比前缀，不去空白。
func TestACatalogAreaThatExcludesAnEndIsUnreachable(t *testing.T) {
	cases := []struct {
		name    string
		carried ports.RequestCarriedContent
		reason  string
	}{
		{"寄件国不在始发区域", carriedGeo("XC", "", "XB", "100200"), "SERVICE_AREA_EXCLUDES_ORIGIN/SYN-AREA-XA@1"},
		{"收件邮编不在交付前缀内", carriedGeo("XA", "", "XB", "200300"), "SERVICE_AREA_EXCLUDES_DESTINATION/SYN-AREA-XB-10@4"},
		{"收件邮编带前导空白", carriedGeo("XA", "", "XB", " 100200"), "SERVICE_AREA_EXCLUDES_DESTINATION/SYN-AREA-XB-10@4"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, _ := assessThroughCatalog(t, syntheticCatalog(t), satisfiedCustoms(), test.carried)
			finding := formedFinding(t, result)
			candidate := onlyCandidate(t, finding)
			if finding.Value() != domain.Unreachable || candidate.Reason().String() != test.reason {
				t.Fatalf("判断 = %s，候选原因 = %q，想要 UNREACHABLE / %q", finding.Value(), candidate.Reason(), test.reason)
			}
		})
	}
}

// Covers: UC-NR-002 矩阵行 6 与 `AT-NR-018`、ADR-0148 决定二「缺席如实」——一侧缺国家码、前缀覆盖下缺邮编，
// 或发起方根本没带投影，都是`资料不足`；缺口点名缺的是哪一侧哪一格，没带投影单独点名，不读成客户地址缺资料。
func TestACatalogJudgmentWithoutTheGeoElementsItNeedsIsInsufficient(t *testing.T) {
	cases := []struct {
		name    string
		carried ports.RequestCarriedContent
		gap     string
	}{
		{"寄件侧缺国家码", carriedGeo("", "", "XB", "100200"), "GEO_ELEMENTS_MISSING/SENDER_COUNTRY"},
		{"收件侧前缀覆盖下缺邮编", carriedGeo("XA", "", "XB", ""), "GEO_ELEMENTS_MISSING/DELIVERY_POSTAL_CODE"},
		{"两侧都缺", carriedGeo("xa", "", "", ""), "GEO_ELEMENTS_MISSING/SENDER_COUNTRY+DELIVERY_COUNTRY"},
		{"发起方没带投影", ports.RequestCarriedContent{}, "GEO_PROJECTION_NOT_CARRIED"},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			result, _ := assessThroughCatalog(t, syntheticCatalog(t), satisfiedCustoms(), test.carried)
			finding := formedFinding(t, result)
			if finding.Value() != domain.InsufficientEvidence {
				t.Fatalf("value = %s，想要 INSUFFICIENT_EVIDENCE", finding.Value())
			}
			gaps := finding.EvidenceGaps()
			if len(gaps) != 1 || gaps[0].Reference().String() != test.gap ||
				gaps[0].ReassessmentCondition().String() == "" {
				t.Fatalf("缺口 = %+v，想要恰一处 %q 且带再次判断条件", gaps, test.gap)
			}
		})
	}
}

// Covers: ADR-0148 决定三——关务来源未接时逐候选答状态未知，覆盖与可执行都成立的候选因此落`资料不足`而不是
// `可达`；来源答适用限制则确定性淘汰（`AT-NR-031` 的限制半边）。
func TestTheCustomsSourceDecidesTheLastHardConstraint(t *testing.T) {
	notConnected := application.CustomsApplicabilityNotConnected{}
	result, _ := assessThroughCatalog(t, syntheticCatalog(t), notConnected, carriedGeo("XA", "", "XB", "100200"))
	finding := formedFinding(t, result)
	if finding.Value() != domain.InsufficientEvidence {
		t.Fatalf("关务来源未接时 value = %s，想要 INSUFFICIENT_EVIDENCE", finding.Value())
	}
	if gaps := finding.EvidenceGaps(); len(gaps) != 1 || gaps[0].Reference().String() != "CUSTOMS_APPLICABILITY_NOT_CONNECTED" {
		t.Fatalf("缺口 = %+v，想要点名关务来源未接", gaps)
	}

	restricted := &customsDouble{outcome: domain.RestrictionApplies}
	result, _ = assessThroughCatalog(t, syntheticCatalog(t), restricted, carriedGeo("XA", "", "XB", "100200"))
	finding = formedFinding(t, result)
	if candidate := onlyCandidate(t, finding); finding.Value() != domain.Unreachable ||
		candidate.Reason().String() != "HARD_CONSTRAINT_RESTRICTION/SYN-CUSTOMS-RESTRICTION-1" {
		t.Fatalf("判断 = %s，候选原因 = %q", finding.Value(), candidate.Reason())
	}
}

// Covers: CONTEXT「稳定网络定义和临时网络可用性调整必须分离」的折叠半边——生效中的停运或关闭作用于线路、其连接
// 或节点，候选不可执行、淘汰原因引那条调整；恢复陈述不挡路。
func TestAnAdjustmentInForceOnTheLineMakesItNotExecutable(t *testing.T) {
	adjustment := func(kind ports.AvailabilityAdjustmentKind, target ports.CatalogTargetKind, code string) ports.AvailabilityAdjustmentStatement {
		return ports.AvailabilityAdjustmentStatement{
			Code: "SYN-ADJ-1", Version: 2, TargetKind: target, TargetCode: code,
			Kind: kind, Source: "SYN-NET-OPS/EVT-1", EffectiveAt: catalogEffectiveFrom,
		}
	}
	for name, statement := range map[string]ports.AvailabilityAdjustmentStatement{
		"连接停运": adjustment(ports.AdjustmentSuspension, ports.TargetConnection, "SYN-CONN-ORIGIN-GATE"),
		"节点关闭": adjustment(ports.AdjustmentClosure, ports.TargetNode, "SYN-NODE-GATE"),
		"线路停运": adjustment(ports.AdjustmentSuspension, ports.TargetLine, "SYN-LINE-XA-XB"),
	} {
		t.Run(name, func(t *testing.T) {
			catalog := syntheticCatalog(t)
			catalog.snapshot.Adjustments = []ports.AvailabilityAdjustmentStatement{statement}
			result, _ := assessThroughCatalog(t, catalog, satisfiedCustoms(), carriedGeo("XA", "", "XB", "100200"))
			finding := formedFinding(t, result)
			if candidate := onlyCandidate(t, finding); finding.Value() != domain.Unreachable ||
				candidate.Reason().String() != "PATH_NOT_EXECUTABLE/ADJUSTMENT/SYN-ADJ-1@2" {
				t.Fatalf("判断 = %s，候选原因 = %q", finding.Value(), candidate.Reason())
			}
		})
	}

	catalog := syntheticCatalog(t)
	catalog.snapshot.Adjustments = []ports.AvailabilityAdjustmentStatement{
		adjustment(ports.AdjustmentResumption, ports.TargetConnection, "SYN-CONN-ORIGIN-GATE"),
		adjustment(ports.AdjustmentSuspension, ports.TargetConnection, "SYN-CONN-ELSEWHERE"),
	}
	result, _ := assessThroughCatalog(t, catalog, satisfiedCustoms(), carriedGeo("XA", "", "XB", "100200"))
	if finding := formedFinding(t, result); finding.Value() != domain.Reachable {
		t.Fatalf("恢复陈述与不在路上的停运都不该挡路，value = %s", finding.Value())
	}
}

// Covers: ADR-0148 决定五的候选生成边界——适用范围不是这次服务目的的线路、首节点不服务任何始发区域或末节点不服务
// 任何交付区域的线路都不进候选空间；候选空间为空按既有规则停在未形成判断，不编出`不可达`。
func TestOnlyApplicableLinesServingBothEndsBecomeCandidates(t *testing.T) {
	catalog := syntheticCatalog(t)
	line := catalog.snapshot.Lines[0]
	otherScope := line
	otherScope.Code, otherScope.ApplicableScope = "SYN-LINE-OTHER-SCOPE", "SYN-PURPOSE-OTHER"
	noOriginRole := line
	noOriginRole.Code, noOriginRole.Segments = "SYN-LINE-FROM-GATE", []string{"SYN-CONN-GATE-LAST-MILE"}
	catalog.snapshot.Lines = append(catalog.snapshot.Lines, otherScope, noOriginRole)

	result, _ := assessThroughCatalog(t, catalog, satisfiedCustoms(), carriedGeo("XA", "", "XB", "100200"))
	if candidate := onlyCandidate(t, formedFinding(t, result)); candidate.ID().String() != "SYN-LINE-XA-XB@3" {
		t.Fatalf("进候选空间的是 %s", candidate.ID())
	}

	catalog = syntheticCatalog(t)
	catalog.snapshot.Lines = []ports.LineDefinitionVersion{noOriginRole}
	result, _ = assessThroughCatalog(t, catalog, satisfiedCustoms(), carriedGeo("XA", "", "XB", "100200"))
	if result.Outcome() != application.JudgmentNotFormed ||
		result.NotFormedReason() != application.CandidateSpaceNotEstablished {
		t.Fatalf("候选空间为空时 outcome = %s reason = %s", result.Outcome(), result.NotFormedReason())
	}
}

// Covers: ADR-0148 决定六——目录修订锚不存在，或判断时点没有适用于这次服务目的的路由策略版本，证据视图答`未配置`
// 且不问关务来源；目录读不通原样上抛。
func TestTheCatalogEvidenceIsUnconfiguredWithoutAnApplicableStrategy(t *testing.T) {
	noAnchor := &catalogReadDouble{}
	noStrategy := syntheticCatalog(t)
	noStrategy.snapshot.Strategies = nil
	otherScope := syntheticCatalog(t)
	otherScope.snapshot.Strategies[0].ApplicableScope = "SYN-PURPOSE-OTHER"

	for name, catalog := range map[string]*catalogReadDouble{
		"目录修订锚不存在": noAnchor, "没有策略版本": noStrategy, "策略管的是另一个服务目的": otherScope,
	} {
		t.Run(name, func(t *testing.T) {
			customs := satisfiedCustoms()
			_, configured, err := newCatalogEvidence(t, catalog, customs).LoadNetworkEvidence(
				t.Context(), catalogJudgmentKey(t), carriedGeo("XA", "", "XB", "100200"))
			if err != nil || configured {
				t.Fatalf("configured=%v err=%v，想要未配置", configured, err)
			}
			if customs.gotQuery.Tenant.String() != "" {
				t.Fatal("未配置时问了关务来源")
			}
		})
	}

	down := errors.New("catalog store is down")
	if _, _, err := newCatalogEvidence(t, &catalogReadDouble{err: down}, satisfiedCustoms()).LoadNetworkEvidence(
		t.Context(), catalogJudgmentKey(t), carriedGeo("XA", "", "XB", "100200")); !errors.Is(err, down) {
		t.Fatalf("目录读不通应原样上抛，实得 %v", err)
	}
}

// Covers: UC-NR-002 矩阵行 7「必需网络版本未发布、配置损坏 → 未形成判断」与 ADR-0148 决定六「登记了而解不出，照旧
// 响亮报错」——候选线路的连接或节点在判断时点没有适用版本、段链断开、适用范围调整的内容目录里没有列，一律交回
// ErrCatalogUnresolvable，不退成`未配置`，也不退成空证据或一条被淘汰的候选。
func TestACatalogThatCannotBeResolvedIsALoudError(t *testing.T) {
	cases := map[string]func(snapshot *ports.NetworkCatalogSnapshot){
		"线路首段连接没有适用版本": func(snapshot *ports.NetworkCatalogSnapshot) {
			snapshot.Connections = snapshot.Connections[1:]
		},
		"候选的节点没有适用版本": func(snapshot *ports.NetworkCatalogSnapshot) {
			snapshot.Nodes = snapshot.Nodes[:2]
		},
		"段链断开": func(snapshot *ports.NetworkCatalogSnapshot) {
			snapshot.Connections[1].FromNode = "SYN-NODE-ELSEWHERE"
		},
		"适用范围调整作用在候选上": func(snapshot *ports.NetworkCatalogSnapshot) {
			snapshot.Adjustments = []ports.AvailabilityAdjustmentStatement{{
				Code: "SYN-ADJ-SCOPE", Version: 1, TargetKind: ports.TargetLine, TargetCode: "SYN-LINE-XA-XB",
				Kind: ports.AdjustmentScopeAdjustment, Source: "SYN-NET-OPS/EVT-2", EffectiveAt: catalogEffectiveFrom,
			}}
		},
		"区域覆盖读回不成形": func(snapshot *ports.NetworkCatalogSnapshot) {
			snapshot.ServiceAreas[0].CoverageCountry = "xa"
		},
	}
	for name, breakIt := range cases {
		t.Run(name, func(t *testing.T) {
			catalog := syntheticCatalog(t)
			breakIt(&catalog.snapshot)
			_, configured, err := newCatalogEvidence(t, catalog, satisfiedCustoms()).LoadNetworkEvidence(
				t.Context(), catalogJudgmentKey(t), carriedGeo("XA", "", "XB", "100200"))
			if !errors.Is(err, application.ErrCatalogUnresolvable) || configured {
				t.Fatalf("configured=%v err=%v，想要 ErrCatalogUnresolvable", configured, err)
			}
		})
	}
}

// Covers: 关务来源的答复要逐候选成立——漏答一条会让那条候选「没有事实就原样通过」，等于把没问到读成满足；漏答
// 与调不通都响亮上抛。
func TestAnIncompleteCustomsAnswerIsALoudError(t *testing.T) {
	omitting := &customsDouble{outcome: domain.ConstraintSatisfied, omit: map[string]bool{"SYN-LINE-XA-XB@3": true}}
	if _, _, err := newCatalogEvidence(t, syntheticCatalog(t), omitting).LoadNetworkEvidence(
		t.Context(), catalogJudgmentKey(t), carriedGeo("XA", "", "XB", "100200")); !errors.Is(err, application.ErrCustomsAnswerIncomplete) {
		t.Fatalf("漏答应交回 ErrCustomsAnswerIncomplete，实得 %v", err)
	}

	down := errors.New("customs source is down")
	if _, _, err := newCatalogEvidence(t, syntheticCatalog(t), &customsDouble{err: down}).LoadNetworkEvidence(
		t.Context(), catalogJudgmentKey(t), carriedGeo("XA", "", "XB", "100200")); !errors.Is(err, down) {
		t.Fatalf("关务来源调不通应原样上抛，实得 %v", err)
	}
}

// Covers: 构造门——缺目录读口或关务来源的视图什么也答不了，nil 依赖在构造期拒绝。
func TestTheCatalogEvidenceRequiresBothSources(t *testing.T) {
	if _, err := application.NewCatalogNetworkEvidence(nil, satisfiedCustoms()); err == nil {
		t.Fatal("nil 目录读口应在构造期被拒")
	}
	if _, err := application.NewCatalogNetworkEvidence(syntheticCatalog(t), nil); err == nil ||
		!strings.Contains(err.Error(), "customs") {
		t.Fatalf("nil 关务来源应在构造期被拒，实得 %v", err)
	}
}
