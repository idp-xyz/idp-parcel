package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nrpricing "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelpricing"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/networkrouting/adapters/registrationjson"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 票 routing-first-cut/11 判据一的取证：演示租户照 scripts/demo-seeds/data/network 的登记行逐行登记（稳定定义都是
// 采用 network-routing/network-catalog/SYN-CN-SG@1 的采用行），经登记用例与目录适配器落库；之后可达性与初始路由两个
// 证据视图、候选成本取数侧都走生产装配函数。证的是 parcel-dispatch 真接上的那一条：采用之后证据视图不再答`未配置`，
// 唯一的候选过得了区域、可执行性、关务与时间可行性；下一个停点是成本一格的计价输入（ADR-0148 决定四第 7 条），
// 不在本票。

const demoTenant = "SYN-TENANT-01"

// demoNetworkSeeds 是演示种子的网络段目录（相对本包）。
var demoNetworkSeeds = filepath.Join("..", "..", "scripts", "demo-seeds", "data", "network")

// registerDemoNetworkSeeds 按文件名把每份登记行送进与 parcel-network-register 同一条译装与登记用例。自动改路事实那一份
// 是另一册，不在目录里，跳过。
func registerDemoNetworkSeeds(t *testing.T, db *bentopg.DB) {
	t.Helper()
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		t.Fatalf("构造网络目录：%v", err)
	}
	registration, err := nrapplication.NewNetworkCatalogRegistration(catalog)
	if err != nil {
		t.Fatalf("构造登记用例：%v", err)
	}
	entries, err := os.ReadDir(demoNetworkSeeds)
	if err != nil {
		t.Fatalf("读种子目录：%v", err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	adopted := 0
	for _, name := range names {
		raw, err := os.ReadFile(filepath.Join(demoNetworkSeeds, name))
		if err != nil {
			t.Fatalf("读 %s：%v", name, err)
		}
		if strings.Contains(string(raw), `"adopt"`) {
			adopted++
		}
		var register func(ctx context.Context) (nrapplication.RegisterCatalogResult, error)
		switch {
		case strings.Contains(name, "-auto-reroute-facts-"):
			continue
		case strings.Contains(name, "-calendar-"):
			command, err := registrationjson.ServiceCalendarVersionFromJSON(raw)
			mustTranslate(t, name, err)
			register = func(ctx context.Context) (nrapplication.RegisterCatalogResult, error) {
				return registration.RegisterServiceCalendarVersion(ctx, command)
			}
		case strings.Contains(name, "-adjustment-"):
			command, err := registrationjson.AvailabilityAdjustmentFromJSON(raw)
			mustTranslate(t, name, err)
			register = func(ctx context.Context) (nrapplication.RegisterCatalogResult, error) {
				return registration.RegisterAvailabilityAdjustment(ctx, command)
			}
		case strings.Contains(name, "-route-strategy-"):
			command, err := registrationjson.RouteStrategyVersionFromJSON(raw)
			mustTranslate(t, name, err)
			register = func(ctx context.Context) (nrapplication.RegisterCatalogResult, error) {
				return registration.RegisterRouteStrategyVersion(ctx, command)
			}
		case strings.Contains(name, "-line-"):
			command, err := registrationjson.LineVersionFromJSON(raw)
			mustTranslate(t, name, err)
			register = func(ctx context.Context) (nrapplication.RegisterCatalogResult, error) {
				return registration.RegisterLineVersion(ctx, command)
			}
		case strings.Contains(name, "-area-"):
			command, err := registrationjson.ServiceAreaVersionFromJSON(raw)
			mustTranslate(t, name, err)
			register = func(ctx context.Context) (nrapplication.RegisterCatalogResult, error) {
				return registration.RegisterServiceAreaVersion(ctx, command)
			}
		case strings.Contains(name, "-conn-"):
			command, err := registrationjson.ConnectionVersionFromJSON(raw)
			mustTranslate(t, name, err)
			register = func(ctx context.Context) (nrapplication.RegisterCatalogResult, error) {
				return registration.RegisterConnectionVersion(ctx, command)
			}
		case strings.Contains(name, "-node-"):
			command, err := registrationjson.NodeVersionFromJSON(raw)
			mustTranslate(t, name, err)
			register = func(ctx context.Context) (nrapplication.RegisterCatalogResult, error) {
				return registration.RegisterNodeVersion(ctx, command)
			}
		default:
			t.Fatalf("种子 %s 认不出是哪一族", name)
		}
		var result nrapplication.RegisterCatalogResult
		err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			var registerErr error
			result, registerErr = register(ctx)
			return registerErr
		})
		if err != nil || result.Outcome() != nrapplication.CatalogRegistered {
			t.Fatalf("登记 %s：outcome=%s refusal=%s err=%v", name, result.Outcome(), result.RefusalReason(), err)
		}
	}
	if adopted == 0 {
		t.Fatal("种子里没有一行是采用行：这条取证要的是经参考配置采用的网络")
	}
}

func mustTranslate(t *testing.T, name string, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("译装 %s：%v", name, err)
	}
}

// Covers: 判据一「不再停在路由证据未配置」——演示租户采用演示网络之后，初始路由证据视图经生产装配答已配置：候选
// SYN-LINE-CN-SG-01@1 三段、策略声明成本单维、带时间投影，按处理器的层次逐层评估仍是合格候选。往下一格是成本：生产
// 装配的候选成本取数侧答计价输入未配置（初始路由编排据此形成未决 COST_SOURCE_NOT_CONFIGURED），那是 ADR-0148
// 决定四第 7 条的消费方适配器，不在本票。
func TestTheAdoptedDemoNetworkGetsPastRouteEvidenceThroughProductionWiring(t *testing.T) {
	db := newWiringDB(t)
	registerDemoNetworkSeeds(t, db)
	seedWiringCustomsCatalog(t, db, demoTenant, true)

	evidence := loadWiringEvidence(t, db, demoTenant)
	if evidence.RankingForm != nrdomain.CostSingleDimensionRanking || !evidence.Strategy.Valid() {
		t.Fatalf("策略：排序形态 %s、引用 %q", evidence.RankingForm, evidence.Strategy)
	}
	if len(evidence.Paths) != 1 || evidence.Paths[0].Candidate.String() != "SYN-LINE-CN-SG-01@1" || len(evidence.Paths[0].Legs) != 3 {
		t.Fatalf("候选段链 = %+v，想要 SYN-LINE-CN-SG-01@1 三段", evidence.Paths)
	}
	if len(evidence.Projections) != 1 {
		t.Fatalf("时间投影 %d 份，想要候选一份（四个节点都有处理时长）", len(evidence.Projections))
	}

	candidates, _, err := nrdomain.EvaluateServiceAreas(evidence.ServiceAreas)
	if err != nil || len(candidates) == 0 {
		t.Fatalf("服务区域：候选 %d 个 err=%v", len(candidates), err)
	}
	if candidates, err = nrdomain.EvaluateRouteRequirements(candidates, evidence.RouteRequirements); err != nil {
		t.Fatalf("服务要求：%v", err)
	}
	if candidates, err = nrdomain.EvaluatePathExecutability(candidates, evidence.PathExecutability); err != nil {
		t.Fatalf("可执行性：%v", err)
	}
	if candidates, _, err = nrdomain.EvaluateHardConstraints(candidates, evidence.HardConstraints); err != nil {
		t.Fatalf("硬约束：%v", err)
	}
	if candidates, err = nrdomain.EvaluateTimeFeasibility(candidates, evidence.Projections, evidence.CommittedBound); err != nil {
		t.Fatalf("时间可行性：%v", err)
	}
	qualified := 0
	for _, candidate := range candidates {
		if candidate.Outcome() == nrdomain.CandidateQualified {
			qualified++
		}
	}
	if qualified != 1 {
		t.Fatalf("合格候选 %d 个，想要 1：%+v", qualified, candidates)
	}

	_, err = routeCosts(db).LoadCandidateCosts(t.Context(), wiringRouteKey(t, demoTenant), evidence)
	if !errors.Is(err, nrports.ErrRouteCostSourceNotConfigured) {
		t.Fatalf("候选成本取数侧：err = %v，今天应答计价输入未配置", err)
	}
}

// Covers: 票 routing-first-cut/15 判据二——目录折叠经生产装配铸出的候选标识原样交候选成本取数侧，取数侧解得回同一条
// 线路版本并拿它去读逐段依据。计价输入换成已配置的替身，只为越过「计价输入未配置」那一格走到解标识；读依据那一步
// 截住，之后的评价不在本用例。取数侧若与铸造侧各自约定分隔符，这里在读依据之前就答 carries no line reference。
func TestTheCostSideResolvesTheLineOfACandidateMintedByCatalogFolding(t *testing.T) {
	db := newWiringDB(t)
	registerDemoNetworkSeeds(t, db)
	seedWiringCustomsCatalog(t, db, demoTenant, true)
	evidence := loadWiringEvidence(t, db, demoTenant)

	bases := &stoppingLineBases{}
	costs := nrpricing.NewRouteCandidateCostAdapter(nrpricing.RouteCandidateCostDeps{
		Bases: bases,
		Input: configuredRoutePricingInput{snapshot: syntheticPricingInput(t)},
	})
	_, err := costs.LoadCandidateCosts(t.Context(), wiringRouteKey(t, demoTenant), evidence)
	if !errors.Is(err, errStopAtLineBases) {
		t.Fatalf("候选成本取数侧没走到读逐段依据：err = %v", err)
	}
	if len(bases.asked) != 1 || bases.asked[0] != (askedLine{code: "SYN-LINE-CN-SG-01", version: 1}) {
		t.Fatalf("取数侧读依据 = %+v，想要 SYN-LINE-CN-SG-01 版本 1", bases.asked)
	}
}

var errStopAtLineBases = errors.New("stop at line cost bases")

type askedLine struct {
	code    string
	version int32
}

// stoppingLineBases 记下取数侧拿哪条线路版本来读依据，随即截住，不往下评价。
type stoppingLineBases struct{ asked []askedLine }

func (bases *stoppingLineBases) LoadLineCostBases(
	_ context.Context, _ nrdomain.TenantID, lineCode string, version int32,
) ([]nrports.LineSegmentCostBasis, error) {
	bases.asked = append(bases.asked, askedLine{code: lineCode, version: version})
	return nil, errStopAtLineBases
}

// configuredRoutePricingInput 答已配置。快照须是一份合法输入：判断声明了比较币时取数侧先拿它声明比较币，在解标识之前。
type configuredRoutePricingInput struct{ snapshot ppdomain.PricingInputSnapshot }

func (input configuredRoutePricingInput) PricingInputFor(
	context.Context, nrdomain.InitialRouteJudgmentKey,
) (ppdomain.PricingInputSnapshot, bool, error) {
	return input.snapshot, true, nil
}

func syntheticPricingInput(t *testing.T) ppdomain.PricingInputSnapshot {
	t.Helper()
	check := func(what string, err error) {
		t.Helper()
		if err != nil {
			t.Fatalf("合成计价输入的%s：%v", what, err)
		}
	}
	tenant, err := ppdomain.NewTenantID(demoTenant)
	check("租户", err)
	scope, err := ppdomain.NewPricingScopeID("SYN-SCOPE-01")
	check("计价范围", err)
	subject, err := ppdomain.NewEstimateSubject("SYN-ESTIMATE-01")
	check("估价主体", err)
	weight, err := ppdomain.NewWeightFromString("1", ppdomain.WeightUnitKilogram)
	check("重量", err)
	snapshot, err := ppdomain.NewPricingInputSnapshot(tenant, scope, subject, "Z1", weight, nil,
		time.Date(2026, time.October, 10, 0, 0, 0, 0, time.UTC))
	check("快照", err)
	return snapshot
}

// Covers: 判据一的反事实——没采用演示网络的租户，初始路由与可达性两个证据视图经生产装配照旧答`未配置`；参考配置随产品
// 发布，不自行生效（ADR-0146 决定三）。
func TestATenantThatDidNotAdoptTheDemoNetworkStaysUnconfigured(t *testing.T) {
	db := newWiringDB(t)
	registerDemoNetworkSeeds(t, db)
	const other = "SYN-TENANT-02"

	route, err := initialRouteEvidence(db, systemClock{}, customsApplicabilitySource(db))
	if err != nil {
		t.Fatalf("构造初始路由证据视图：%v", err)
	}
	if _, configured, err := route.LoadInitialRouteEvidence(t.Context(), wiringRouteKey(t, other), wiringCarried()); err != nil || configured {
		t.Fatalf("没采用的租户：初始路由证据 configured=%v err=%v，想要未配置", configured, err)
	}
	catalog, err := nrpostgres.NewNetworkCatalog(db)
	if err != nil {
		t.Fatalf("构造网络目录：%v", err)
	}
	reachability, err := nrapplication.NewCatalogNetworkEvidence(catalog, customsApplicabilitySource(db))
	if err != nil {
		t.Fatalf("构造可达性证据视图：%v", err)
	}
	if _, configured, err := reachability.LoadNetworkEvidence(t.Context(), wiringReachabilityKey(t, other), wiringCarried()); err != nil || configured {
		t.Fatalf("没采用的租户：可达性证据 configured=%v err=%v，想要未配置", configured, err)
	}
	if _, configured, err := reachability.LoadNetworkEvidence(t.Context(), wiringReachabilityKey(t, demoTenant), wiringCarried()); err != nil || !configured {
		t.Fatalf("采用了的演示租户：可达性证据 configured=%v err=%v，想要已配置", configured, err)
	}
}
