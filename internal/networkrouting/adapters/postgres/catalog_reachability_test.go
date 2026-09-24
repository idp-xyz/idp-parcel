package postgres_test

import (
	"context"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件是票 routing-first-cut/07 的真库用例：合成目录经登记用例的受理门落进真迁移计划建出的库，可达性判断用例
// 经真取数侧（选版读口 + 应用层目录折叠）与真判断库跑通。关务来源用替身答满足以证`可达`——CC 侧判断口未接
// （routing-first-cut/12），生产装配是逐候选答状态未知的「未接」实现。取值一律 SYN- 合成，证据层级只记 S。

const catalogReachPurpose = "SYN-PURPOSE-NETWORK"

var catalogReachEffective = catalogAsOf.Add(-30 * 24 * time.Hour)

type catalogReachFixture struct {
	catalog      *adapter.NetworkCatalog
	transactor   bentoapp.Transactor
	judgments    *adapter.ReachabilityJudgments
	registration *application.NetworkCatalogRegistration
	tenant       domain.TenantID
}

func newCatalogReachFixture(t *testing.T) *catalogReachFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewNetworkCatalog(db)
	if err != nil {
		t.Fatalf("构造网络目录：%v", err)
	}
	judgments, err := adapter.NewReachabilityJudgments(db)
	if err != nil {
		t.Fatalf("构造判断库：%v", err)
	}
	registration, err := application.NewNetworkCatalogRegistration(catalog)
	if err != nil {
		t.Fatalf("构造目录登记用例：%v", err)
	}
	return &catalogReachFixture{
		catalog: catalog, transactor: db.Transactor(), judgments: judgments, registration: registration,
		tenant: scalar(t, domain.NewTenantID, "SYN-TENANT-RFC07"),
	}
}

// registered 在一个事务里登一笔，并要求受理门放行。
func (fixture *catalogReachFixture) registered(t *testing.T, register func(ctx context.Context) (application.RegisterCatalogResult, error)) {
	t.Helper()
	within(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		result, err := register(txCtx)
		if err != nil {
			return err
		}
		if result.Outcome() != application.CatalogRegistered {
			t.Fatalf("登记被拒：%s", result.RefusalReason())
		}
		return nil
	})
}

// seedNetwork 登一张最小合成网：寄件国 XA 的收寄枢纽经口岸到收件国 XB 的末端，一条线路两段，一版适用于
// 本服务目的的路由策略。共九笔登记，目录修订锚随之到 9。
func (fixture *catalogReachFixture) seedNetwork(t *testing.T) {
	t.Helper()
	registry, tenant := fixture.registration, fixture.tenant
	for _, node := range []ports.NodeDefinitionVersion{
		{Code: "SYN-NODE-ORIGIN", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogReachEffective},
		{Code: "SYN-NODE-GATE", Version: 1, BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogReachEffective},
		{Code: "SYN-NODE-LAST-MILE", Version: 1, BusinessTimezone: "Asia/Singapore", EffectiveFrom: catalogReachEffective},
	} {
		fixture.registered(t, func(ctx context.Context) (application.RegisterCatalogResult, error) {
			return registry.RegisterNodeVersion(ctx, application.RegisterNodeVersionCommand{TenantID: tenant, Node: node})
		})
	}
	for _, connection := range []ports.ConnectionDefinitionVersion{
		{Code: "SYN-CONN-ORIGIN-GATE", Version: 1, FromNode: "SYN-NODE-ORIGIN", ToNode: "SYN-NODE-GATE",
			BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogReachEffective},
		{Code: "SYN-CONN-GATE-LAST-MILE", Version: 1, FromNode: "SYN-NODE-GATE", ToNode: "SYN-NODE-LAST-MILE",
			BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogReachEffective},
	} {
		fixture.registered(t, func(ctx context.Context) (application.RegisterCatalogResult, error) {
			return registry.RegisterConnectionVersion(ctx, application.RegisterConnectionVersionCommand{TenantID: tenant, Connection: connection})
		})
	}
	fixture.registered(t, func(ctx context.Context) (application.RegisterCatalogResult, error) {
		return registry.RegisterLineVersion(ctx, application.RegisterLineVersionCommand{TenantID: tenant, Line: ports.LineDefinitionVersion{
			Code: "SYN-LINE-XA-XB", Version: 1, Segments: []string{"SYN-CONN-ORIGIN-GATE", "SYN-CONN-GATE-LAST-MILE"},
			BusinessTimezone: "Asia/Shanghai", ApplicableScope: catalogReachPurpose, EffectiveFrom: catalogReachEffective,
		}})
	})
	for _, area := range []ports.ServiceAreaDefinitionVersion{
		{Code: "SYN-AREA-XA", Version: 1, EffectiveFrom: catalogReachEffective,
			HasCoverage: true, CoverageCountry: "XA", OriginNodes: []string{"SYN-NODE-ORIGIN"}},
		{Code: "SYN-AREA-XB-10", Version: 1, EffectiveFrom: catalogReachEffective,
			HasCoverage: true, CoverageCountry: "XB", PostalPrefixes: []string{"10"},
			DestinationNodes: []string{"SYN-NODE-LAST-MILE"}},
	} {
		fixture.registered(t, func(ctx context.Context) (application.RegisterCatalogResult, error) {
			return registry.RegisterServiceAreaVersion(ctx, application.RegisterServiceAreaVersionCommand{TenantID: tenant, Area: area})
		})
	}
	fixture.registered(t, func(ctx context.Context) (application.RegisterCatalogResult, error) {
		return registry.RegisterRouteStrategyVersion(ctx, application.RegisterRouteStrategyVersionCommand{TenantID: tenant,
			Strategy: ports.RouteStrategyDefinitionVersion{
				Code: "SYN-STRATEGY-1", Version: 1, ApplicableScope: catalogReachPurpose, EffectiveFrom: catalogReachEffective,
			}})
	})
}

type catalogReachEligibility struct{}

func (catalogReachEligibility) AssessNetworkEligibility(context.Context, domain.ReachabilityJudgmentKey) (domain.NetworkEligibility, error) {
	return domain.NewNetworkEligibility(domain.NetworkJudgmentRequired, domain.EligibilityBasisReference{})
}

type catalogReachHandoff struct{}

func (catalogReachHandoff) HandOffReachabilityJudgment(context.Context, ports.ReachabilityJudgmentHandoffIntent) error {
	return nil
}

type catalogReachCustoms struct{}

func (catalogReachCustoms) AssessCustomsApplicability(
	_ context.Context, query ports.CustomsApplicabilityQuery,
) ([]domain.HardConstraintFinding, error) {
	findings := make([]domain.HardConstraintFinding, 0, len(query.Candidates))
	for _, candidate := range query.Candidates {
		finding, err := domain.NewHardConstraintFinding(domain.HardConstraintFindingSpec{
			Candidate: candidate.Candidate, Outcome: domain.ConstraintSatisfied,
		})
		if err != nil {
			return nil, err
		}
		findings = append(findings, finding)
	}
	return findings, nil
}

type catalogReachClock struct{}

func (catalogReachClock) Now() time.Time { return catalogAsOf.Add(time.Hour) }

func (fixture *catalogReachFixture) evidence(t *testing.T) *application.CatalogNetworkEvidence {
	t.Helper()
	view, err := application.NewCatalogNetworkEvidence(fixture.catalog, catalogReachCustoms{})
	if err != nil {
		t.Fatalf("构造目录证据视图：%v", err)
	}
	return view
}

func (fixture *catalogReachFixture) key(t *testing.T, parcel, purpose string) domain.ReachabilityJudgmentKey {
	t.Helper()
	asOf, err := domain.NewJudgmentAsOf(
		scalar(t, domain.NewAsOfSemantic, "SYN-CURRENT-SUBMISSION-RECEIVED-AT"),
		catalogAsOf,
		scalar(t, domain.NewAsOfStrategyVersion, "SYN-ASOF-STRATEGY-1"),
	)
	if err != nil {
		t.Fatalf("构造判断时点：%v", err)
	}
	return domain.ReachabilityJudgmentKey{
		TenantID:          fixture.tenant,
		CustomerAccountID: scalar(t, domain.NewCustomerAccountID, "SYN-CUSTOMER-1"),
		ShipmentRequestID: scalar(t, domain.NewShipmentRequestID, "SYN-REQUEST-1"),
		SubmissionVersion: scalar(t, domain.NewSubmissionVersionID, "SYN-SUBMISSION-1"),
		DeclaredParcelID:  scalar(t, domain.NewDeclaredParcelID, parcel),
		ServicePurpose:    scalar(t, domain.NewServicePurpose, purpose),
		AsOf:              asOf,
	}
}

func catalogReachGeo(deliveryCountry, deliveryPostal string) ports.RequestCarriedContent {
	return ports.RequestCarriedContent{Geo: domain.NewGeoResolutionProjection(
		domain.NewGeoResolutionSide("XA", true, "", false),
		domain.NewGeoResolutionSide(deliveryCountry, deliveryCountry != "", deliveryPostal, deliveryPostal != ""),
	)}
}

// assess 在一个事务里跑一次可达性判断用例：判断落库走 RequireExecutor，与生产上由进程级入口给出环境事务同形。
func (fixture *catalogReachFixture) assess(
	t *testing.T, key domain.ReachabilityJudgmentKey, correlation string, carried ports.RequestCarriedContent,
) application.AssessParcelReachabilityResult {
	t.Helper()
	handler := application.NewAssessParcelReachabilityHandler(
		catalogReachEligibility{}, fixture.evidence(t), fixture.judgments, catalogReachHandoff{}, catalogReachClock{})
	var result application.AssessParcelReachabilityResult
	within(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var err error
		result, err = handler.Handle(txCtx, application.AssessParcelReachabilityCommand{
			Correlation: scalar(t, domain.NewRequestCorrelationID, correlation),
			Key:         key,
			Carried:     carried,
		})
		return err
	})
	return result
}

// Covers: 完成判据一的三值——同一张合成目录上，收件地址落在交付区域内得`可达`，落在区域覆盖之外得`不可达`
// （终点侧排除，依据引区域版本），收件侧缺国家码得`资料不足`；三份判断各自经真判断库落库，视图修订是目录
// 修订锚。完成判据二：应用层用例经真取数侧跑通。
func TestReachabilityOverARealCatalogFormsEachOfTheThreeValues(t *testing.T) {
	fixture := newCatalogReachFixture(t)
	fixture.seedNetwork(t)

	cases := []struct {
		parcel  string
		carried ports.RequestCarriedContent
		want    domain.ReachabilityValue
		reason  string
	}{
		{"SYN-PARCEL-REACHABLE", catalogReachGeo("XB", "100200"), domain.Reachable, ""},
		{"SYN-PARCEL-UNREACHABLE", catalogReachGeo("XC", "100200"), domain.Unreachable,
			"SERVICE_AREA_EXCLUDES_DESTINATION/SYN-AREA-XB-10@1"},
		{"SYN-PARCEL-INSUFFICIENT", catalogReachGeo("", "100200"), domain.InsufficientEvidence,
			"ADDRESS_INFORMATION_INSUFFICIENT"},
	}
	for _, test := range cases {
		t.Run(test.parcel, func(t *testing.T) {
			key := fixture.key(t, test.parcel, catalogReachPurpose)
			correlation := "SYN-CORRELATION-" + test.parcel
			result := fixture.assess(t, key, correlation, test.carried)
			finding, ok := result.Finding()
			if result.Outcome() != application.JudgmentFormed || !ok {
				t.Fatalf("outcome = %s reason = %s，想要形成判断", result.Outcome(), result.NotFormedReason())
			}
			candidates := finding.Candidates()
			if finding.Value() != test.want || len(candidates) != 1 ||
				candidates[0].ID().String() != "SYN-LINE-XA-XB@1" || candidates[0].Reason().String() != test.reason {
				t.Fatalf("判断 = %s，候选 = %+v，想要 %s / 原因 %q", finding.Value(), candidates, test.want, test.reason)
			}

			record, found, err := fixture.judgments.FindByCorrelation(t.Context(), fixture.tenant,
				scalar(t, domain.NewRequestCorrelationID, correlation))
			if err != nil || !found {
				t.Fatalf("按关联读回判断：found=%v err=%v", found, err)
			}
			if record.Finding.Value() != test.want || record.ViewRevision.String() != "9" {
				t.Fatalf("落库判断 = %s / 修订 %q，想要 %s / 目录修订锚 9", record.Finding.Value(), record.ViewRevision, test.want)
			}
		})
	}
}

// Covers: 完成判据一的`未配置`两格（ADR-0148 决定六）——从未登记过目录的租户，与目录在而判断时点没有适用于
// 这个服务目的的路由策略版本，各答`未配置`，不形成判断、不落库。
func TestReachabilityOverARealCatalogAnswersUnconfigured(t *testing.T) {
	fixture := newCatalogReachFixture(t)

	result := fixture.assess(t, fixture.key(t, "SYN-PARCEL-EMPTY", catalogReachPurpose),
		"SYN-CORRELATION-EMPTY", catalogReachGeo("XB", "100200"))
	if result.Outcome() != application.JudgmentNotFormed ||
		result.NotFormedReason() != application.NetworkEvidenceNotConfigured {
		t.Fatalf("空目录：outcome = %s reason = %s，想要 NETWORK_EVIDENCE_NOT_CONFIGURED", result.Outcome(), result.NotFormedReason())
	}

	fixture.seedNetwork(t)
	result = fixture.assess(t, fixture.key(t, "SYN-PARCEL-OTHER-PURPOSE", "SYN-PURPOSE-EXPRESS"),
		"SYN-CORRELATION-OTHER-PURPOSE", catalogReachGeo("XB", "100200"))
	if result.Outcome() != application.JudgmentNotFormed ||
		result.NotFormedReason() != application.NetworkEvidenceNotConfigured {
		t.Fatalf("无适用策略：outcome = %s reason = %s，想要 NETWORK_EVIDENCE_NOT_CONFIGURED", result.Outcome(), result.NotFormedReason())
	}

	for _, correlation := range []string{"SYN-CORRELATION-EMPTY", "SYN-CORRELATION-OTHER-PURPOSE"} {
		if _, found, err := fixture.judgments.FindByCorrelation(t.Context(), fixture.tenant,
			scalar(t, domain.NewRequestCorrelationID, correlation)); err != nil || found {
			t.Fatalf("%s：未配置却落了判断（found=%v err=%v）", correlation, found, err)
		}
	}
}

// Covers: 完成判据一的第三格与 first-tenant-runway/03 Answer「视图修订改由目录修订锚派生」——目录改一笔，视图
// 修订随之变，提交前重校据此把原判断判`已换代`；目录没改则`仍然当前`。漏掉这一格的症状是静默错判。
func TestAOneRowCatalogChangeSupersedesTheJudgment(t *testing.T) {
	fixture := newCatalogReachFixture(t)
	fixture.seedNetwork(t)
	key := fixture.key(t, "SYN-PARCEL-REVISION", catalogReachPurpose)
	if result := fixture.assess(t, key, "SYN-CORRELATION-REVISION", catalogReachGeo("XB", "100200")); result.Outcome() != application.JudgmentFormed {
		t.Fatalf("形成判断：outcome = %s reason = %s", result.Outcome(), result.NotFormedReason())
	}

	validate := application.NewValidateReachabilityJudgmentHandler(fixture.evidence(t), fixture.judgments)
	command := application.ValidateReachabilityJudgmentCommand{
		Correlation: scalar(t, domain.NewRequestCorrelationID, "SYN-CORRELATION-REVISION"),
		Key:         key,
	}
	still, err := validate.Handle(t.Context(), command)
	if err != nil || still.Outcome() != application.JudgmentStillCurrent {
		t.Fatalf("目录未改时重校 = %s（%v），想要 JUDGMENT_STILL_CURRENT", still.Outcome(), err)
	}

	fixture.registered(t, func(ctx context.Context) (application.RegisterCatalogResult, error) {
		return fixture.registration.RegisterAvailabilityAdjustment(ctx, application.RegisterAvailabilityAdjustmentCommand{
			TenantID: fixture.tenant,
			Adjustment: ports.AvailabilityAdjustmentStatement{
				Code: "SYN-ADJ-ELSEWHERE", Version: 1, TargetKind: ports.TargetNode, TargetCode: "SYN-NODE-ELSEWHERE",
				Kind: ports.AdjustmentSuspension, Source: "SYN-NET-OPS/EVT-1", EffectiveAt: catalogReachEffective,
			},
		})
	})
	evidence, configured, err := fixture.evidence(t).LoadNetworkEvidence(t.Context(), key, catalogReachGeo("XB", "100200"))
	if err != nil || !configured || evidence.ViewRevision.String() != "10" {
		t.Fatalf("改一笔后视图修订 = %q（configured=%v err=%v），想要 10", evidence.ViewRevision, configured, err)
	}
	superseded, err := validate.Handle(t.Context(), command)
	if err != nil || superseded.Outcome() != application.JudgmentSuperseded {
		t.Fatalf("目录改一笔后重校 = %s（%v），想要 JUDGMENT_SUPERSEDED", superseded.Outcome(), err)
	}
}
