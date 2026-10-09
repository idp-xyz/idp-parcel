package main

import (
	"context"
	"slices"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 票 routing-first-cut/12 完成判据四：接到 NR 取数侧后，合成网络上含关务段的候选不再停在
// 状态未知。网络与 CC 两本目录照 scripts/demo-seeds 的节点、连接、线路、口岸与申报路径形状
// 登（SYN- 合成值），线路与路由策略的适用范围取派发进程的服务目的；初始路由证据视图与关务
// 来源都走生产装配函数，证的是 parcel-dispatch 真接上的那一条，不是替身。

const wiringPurpose = "NETWORK_SERVICE"

var wiringEffective = time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)

func newWiringDB(t *testing.T) *bentopg.DB {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}

// seedWiringNetwork 登一份 CN→SG 合成网络：上海枢纽经深圳口岸到新加坡枢纽再到末端，一条线路串起三段。
func seedWiringNetwork(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		t.Fatalf("构造网络目录：%v", err)
	}
	id := mustNR(t, nrdomain.NewTenantID, tenant)
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		for _, node := range []struct{ code, zone string }{
			{"SYN-NODE-SHA-HUB", "Asia/Shanghai"},
			{"SYN-NODE-SZX-GATE", "Asia/Shanghai"},
			{"SYN-NODE-SIN-HUB", "Asia/Singapore"},
			{"SYN-NODE-SIN-LM", "Asia/Singapore"},
		} {
			if err := catalog.RegisterNodeVersion(ctx, id, nrports.NodeDefinitionVersion{
				Code: node.code, Version: 1, BusinessTimezone: node.zone, EffectiveFrom: wiringEffective,
			}); err != nil {
				return err
			}
		}
		for _, connection := range []struct{ code, from, to, zone string }{
			{"SYN-CONN-SHA-SZX", "SYN-NODE-SHA-HUB", "SYN-NODE-SZX-GATE", "Asia/Shanghai"},
			{"SYN-CONN-SZX-SIN", "SYN-NODE-SZX-GATE", "SYN-NODE-SIN-HUB", "Asia/Shanghai"},
			{"SYN-CONN-SIN-LM", "SYN-NODE-SIN-HUB", "SYN-NODE-SIN-LM", "Asia/Singapore"},
		} {
			if err := catalog.RegisterConnectionVersion(ctx, id, nrports.ConnectionDefinitionVersion{
				Code: connection.code, Version: 1, FromNode: connection.from, ToNode: connection.to,
				BusinessTimezone: connection.zone, EffectiveFrom: wiringEffective,
			}); err != nil {
				return err
			}
		}
		if err := catalog.RegisterLineVersion(ctx, id, nrports.LineDefinitionVersion{
			Code: "SYN-LINE-CN-SG-01", Version: 1,
			Segments:         []string{"SYN-CONN-SHA-SZX", "SYN-CONN-SZX-SIN", "SYN-CONN-SIN-LM"},
			BusinessTimezone: "Asia/Shanghai", ApplicableScope: wiringPurpose, EffectiveFrom: wiringEffective,
		}); err != nil {
			return err
		}
		if err := catalog.RegisterServiceAreaVersion(ctx, id, nrports.ServiceAreaDefinitionVersion{
			Code: "SYN-AREA-CN-EAST", Version: 1, EffectiveFrom: wiringEffective,
			HasCoverage: true, CoverageCountry: "CN", OriginNodes: []string{"SYN-NODE-SHA-HUB"},
		}); err != nil {
			return err
		}
		if err := catalog.RegisterServiceAreaVersion(ctx, id, nrports.ServiceAreaDefinitionVersion{
			Code: "SYN-AREA-SG", Version: 1, EffectiveFrom: wiringEffective,
			HasCoverage: true, CoverageCountry: "SG", DestinationNodes: []string{"SYN-NODE-SIN-LM"},
		}); err != nil {
			return err
		}
		return catalog.RegisterRouteStrategyVersion(ctx, id, nrports.RouteStrategyDefinitionVersion{
			Code: "SYN-RS-CN-SG-01", Version: 1, ApplicableScope: wiringPurpose,
			RankingForm: nrdomain.CostSingleDimensionRanking, EffectiveFrom: wiringEffective,
		})
	})
	if err != nil {
		t.Fatalf("登记合成网络：%v", err)
	}
}

// seedWiringCustomsCatalog 照演示种子登 CC 两本目录：深圳口岸两版、新加坡口岸一版，出口路径经深圳；
// withImportPath 为 false 时不登经新加坡的进口路径。
func seedWiringCustomsCatalog(t *testing.T, db *bentopg.DB, tenant string, withImportPath bool) {
	t.Helper()
	registry, err := ccpostgres.NewPortsPathsRegistrations(db)
	if err != nil {
		t.Fatalf("构造口岸路径写口：%v", err)
	}
	id := mustCC(t, ccdomain.NewTenantID, tenant)
	type registration func(ctx context.Context) (ccports.CaseConfigurationSaveOutcome, error)
	port := func(ref string, from time.Time) registration {
		return func(ctx context.Context) (ccports.CaseConfigurationSaveOutcome, error) {
			return registry.RegisterCandidatePort(ctx, id, mustCC(t, ccdomain.NewCustomsPortReference, ref), from)
		}
	}
	path := func(ref, portRef string, direction ccdomain.ManifestDirection, mode string) registration {
		return func(ctx context.Context) (ccports.CaseConfigurationSaveOutcome, error) {
			route, err := ccdomain.NewDeclarationPathRoute(
				mustCC(t, ccdomain.NewCustomsPortReference, portRef), direction,
				mustCC(t, ccdomain.NewDeclarationModeReference, mode))
			if err != nil {
				return ccports.CaseConfigurationSaveOutcomeInvalid, err
			}
			return registry.RegisterDeclarationPath(ctx, id, mustCC(t, ccdomain.NewDeclarationPathReference, ref), route, wiringEffective)
		}
	}
	registrations := []registration{
		port("SYN-PORT-SZX-01", wiringEffective),
		port("SYN-PORT-SZX-01", time.Date(2026, 3, 1, 0, 0, 0, 0, time.UTC)),
		port("SYN-PORT-SIN-01", wiringEffective),
		path("SYN-PATH-CN-EXPORT-01", "SYN-PORT-SZX-01", ccdomain.ExportManifest, "SYN-MODE-MANIFEST-01"),
	}
	if withImportPath {
		registrations = append(registrations,
			path("SYN-PATH-SG-IMPORT-01", "SYN-PORT-SIN-01", ccdomain.ImportManifest, "SYN-MODE-FORMAL-01"))
	}
	for _, register := range registrations {
		var outcome ccports.CaseConfigurationSaveOutcome
		err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			var registerErr error
			outcome, registerErr = register(ctx)
			return registerErr
		})
		if err != nil || outcome != ccports.CaseConfigurationRegistered {
			t.Fatalf("登记关务目录行：outcome=%v err=%v", outcome, err)
		}
	}
}

// loadWiringEvidence 经生产装配取初始路由证据，请求随带 CN→SG 的地理解析投影。
func loadWiringEvidence(t *testing.T, db *bentopg.DB, tenant string) nrports.InitialRouteEvidence {
	t.Helper()
	evidence, err := initialRouteEvidence(db, systemClock{}, customsApplicabilitySource(db))
	if err != nil {
		t.Fatalf("构造初始路由证据视图：%v", err)
	}
	key := nrdomain.InitialRouteJudgmentKey{
		TenantID:           mustNR(t, nrdomain.NewTenantID, tenant),
		CustomerAccountID:  mustNR(t, nrdomain.NewCustomerAccountID, "SYN-ACCOUNT-01"),
		ShipmentRequestID:  mustNR(t, nrdomain.NewShipmentRequestID, "SYN-REQUEST-01"),
		AcceptanceBaseline: mustNR(t, nrdomain.NewAcceptanceBaselineReference, "SYN-VER-01"),
		DeclaredParcelID:   mustNR(t, nrdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		ServicePurpose:     mustNR(t, nrdomain.NewServicePurpose, wiringPurpose),
	}
	geo := nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide("CN", true, "", false),
		nrdomain.NewGeoResolutionSide("SG", true, "", false))
	got, configured, err := evidence.LoadInitialRouteEvidence(t.Context(), key, nrports.RequestCarriedContent{Geo: geo})
	if err != nil {
		t.Fatalf("LoadInitialRouteEvidence：%v", err)
	}
	if !configured {
		t.Fatal("合成网络被答成未配置：证据视图停在未配置，关务那一格根本没问到")
	}
	if len(got.HardConstraints) == 0 {
		t.Fatal("候选空间为空：没有含关务段的候选可证")
	}
	return got
}

// expectCustomsCitations 核出处逐候选一条、带 CC 的判断标识，且引用含 wantVersions 各条。
func expectCustomsCitations(t *testing.T, evidence nrports.InitialRouteEvidence, wantVersions ...string) {
	t.Helper()
	cited := make(map[string]nrdomain.CustomsApplicabilityCitation, len(evidence.CustomsCitations))
	for _, citation := range evidence.CustomsCitations {
		cited[citation.Candidate().String()] = citation
	}
	for _, finding := range evidence.HardConstraints {
		citation, ok := cited[finding.Candidate().String()]
		if !ok {
			t.Errorf("候选 %s 的关务事实没有出处", finding.Candidate())
			continue
		}
		if citation.Judgment() == "" {
			t.Errorf("候选 %s 的关务出处没有判断标识", finding.Candidate())
		}
		for _, want := range wantVersions {
			if !slices.Contains(citation.Versions(), want) {
				t.Errorf("候选 %s 的关务出处缺目录版本引用 %s（得到 %v）", finding.Candidate(), want, citation.Versions())
			}
		}
	}
}

// Covers: 判据四——合成网络上 CN→SG 候选的关务一格由 CC 按该租户在册的口岸与申报路径作答为满足，
// 出处带 CC 判断标识与所依目录版本，不再是来源未接时的状态未知。
func TestSyntheticCrossBorderCandidatesGetCustomsAnswersThroughProductionWiring(t *testing.T) {
	db := newWiringDB(t)
	seedWiringNetwork(t, db, "SYN-TENANT-01")
	seedWiringCustomsCatalog(t, db, "SYN-TENANT-01", true)

	evidence := loadWiringEvidence(t, db, "SYN-TENANT-01")
	for _, finding := range evidence.HardConstraints {
		if finding.Outcome() != nrdomain.ConstraintSatisfied {
			t.Errorf("候选 %s 的关务事实 = %s，要 SATISFIED", finding.Candidate(), finding.Outcome())
		}
	}
	expectCustomsCitations(t, evidence,
		"PORT:SYN-PORT-SZX-01@2026-03-01T00:00:00Z",
		"PATH:SYN-PATH-CN-EXPORT-01@2026-01-01T00:00:00Z",
		"PATH:SYN-PATH-SG-IMPORT-01@2026-01-01T00:00:00Z")
}

// Covers: 判据四的反事实——同一份合成网络换成没登关务目录、或缺进口路径的租户，答案随该租户的
// 目录变成状态未知（目录为空）与适用限制。答案跟着租户目录走，才说明上面那份满足出自 CC，
// 而不是一个恒答满足的来源。
func TestSyntheticNetworkCustomsAnswersFollowTheTenantsCatalog(t *testing.T) {
	db := newWiringDB(t)
	seedWiringNetwork(t, db, "SYN-TENANT-NOCAT")
	seedWiringNetwork(t, db, "SYN-TENANT-NOIMP")
	seedWiringCustomsCatalog(t, db, "SYN-TENANT-NOIMP", false)

	empty := loadWiringEvidence(t, db, "SYN-TENANT-NOCAT")
	candidates := make([]nrdomain.RouteCandidate, 0, len(empty.HardConstraints))
	for _, finding := range empty.HardConstraints {
		if finding.Outcome() != nrdomain.ConstraintStatusUnknown {
			t.Errorf("没登关务目录：候选 %s 的关务事实 = %s，要 STATUS_UNKNOWN", finding.Candidate(), finding.Outcome())
		}
		candidate, err := nrdomain.NewRouteCandidate(finding.Candidate(), nrdomain.CandidateQualified, nrdomain.CandidateReason{})
		if err != nil {
			t.Fatalf("NewRouteCandidate：%v", err)
		}
		candidates = append(candidates, candidate)
	}
	_, gaps, err := nrdomain.EvaluateHardConstraints(candidates, empty.HardConstraints)
	if err != nil {
		t.Fatalf("EvaluateHardConstraints：%v", err)
	}
	if len(gaps) == 0 {
		t.Fatal("没登关务目录却没有留下缺口")
	}
	for _, gap := range gaps {
		if gap.Reference().String() != "CUSTOMS_PORT_PATH_CATALOG_EMPTY" {
			t.Errorf("缺口 = %s，要 CUSTOMS_PORT_PATH_CATALOG_EMPTY（来源未接时是 CUSTOMS_APPLICABILITY_NOT_CONNECTED）", gap.Reference())
		}
	}
	expectCustomsCitations(t, empty)

	noImport := loadWiringEvidence(t, db, "SYN-TENANT-NOIMP")
	for _, finding := range noImport.HardConstraints {
		if finding.Outcome() != nrdomain.RestrictionApplies {
			t.Errorf("缺进口路径：候选 %s 的关务事实 = %s，要 RESTRICTION_APPLIES", finding.Candidate(), finding.Outcome())
		}
	}
	expectCustomsCitations(t, noImport, "PATH:SYN-PATH-CN-EXPORT-01@2026-01-01T00:00:00Z")
}
