package postgres_test

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证价卡版本仓储（票 07 件①②）：登记行只增不改、同键
// 重复按（规范化版本+内容摘要）译成结果代数、装载按（方向+适用范围+计价基准时点）
// 过滤且读回经领域整图重验、同一方案身份两版同时适用交回错误不挑一个。
// 夹具全部为 SYN-PRC 合成价卡（S 级），零生产默认。

const catalogSourceSHA = "9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d"

func newPriceCards(t *testing.T) (*adapter.PriceCards, bentoapp.Transactor) {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	catalog, err := adapter.NewPriceCards(db)
	if err != nil {
		t.Fatalf("构造价卡仓储：%v", err)
	}
	return catalog, db.Transactor()
}

func inCatalogTx(t *testing.T, transactor bentoapp.Transactor, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

// catalogPlan 造一张可参数化身份与内容的合成价卡。方向与计算目的按首发配对声明。
func catalogPlan(t *testing.T, planID, planVersion string, direction domain.PricingDirection, amount string) domain.PricingPlanVersion {
	t.Helper()
	var purpose domain.PricingPurpose
	switch direction {
	case domain.PricingDirectionSell:
		purpose = domain.PricingPurposeCustomerCharge
	case domain.PricingDirectionBuy:
		purpose = domain.PricingPurposeSupplierCost
	default:
		purpose = domain.PricingPurposeInternalPrice
	}
	currency := evaluationValue(t, domain.NewCurrency, "USD")
	entry, err := domain.NewRateEntry(
		evaluationValue(t, domain.NewRateEntryID, "entry-"+planID),
		"Z1",
		catalogWeight(t, "0"),
		catalogWeight(t, "10"),
		catalogMoney(t, amount, currency),
	)
	if err != nil {
		t.Fatalf("构造费率段：%v", err)
	}
	period, err := domain.NewEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造适用期：%v", err)
	}
	tableRef, err := domain.NewVersionReference(domain.ArtifactRateTable, "table-"+planID, planVersion, "sha256:syn-table-"+planID)
	if err != nil {
		t.Fatalf("构造价表引用：%v", err)
	}
	table, err := domain.NewRateTableVersion(
		tableRef, domain.RateTableFamilyWeightZone, currency, domain.WeightUnitKilogram,
		period, []domain.RateEntry{entry})
	if err != nil {
		t.Fatalf("构造价表：%v", err)
	}
	rounding, err := domain.NewWeightRoundingPolicy(domain.RoundingCeiling, catalogWeight(t, "0.5"))
	if err != nil {
		t.Fatalf("构造取整策略：%v", err)
	}
	weightRef, err := domain.NewVersionReference(domain.ArtifactWeightPolicy, "weight-"+planID, planVersion, "sha256:syn-weight-"+planID)
	if err != nil {
		t.Fatalf("构造计价重引用：%v", err)
	}
	weightPolicy, err := domain.NewPricingWeightPolicy(weightRef, domain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("构造计价重策略：%v", err)
	}
	planRef, err := domain.NewVersionReference(domain.ArtifactPricingPlan, planID, planVersion, "sha256:syn-"+planID)
	if err != nil {
		t.Fatalf("构造方案引用：%v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		planRef,
		evaluationValue(t, domain.NewPricingScopeID, "scope-1"),
		direction,
		purpose,
		evaluationValue(t, domain.NewChargeCode, "BASE_FREIGHT"),
		period,
		table,
		weightPolicy,
		nil,
		domain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("构造价卡：%v", err)
	}
	return plan
}

func catalogWeight(t *testing.T, value string) domain.Weight {
	t.Helper()
	weight, err := domain.NewWeight(evaluationValue(t, domain.ParseDecimal, value), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("构造重量 %s：%v", value, err)
	}
	return weight
}

func catalogMoney(t *testing.T, value string, currency domain.Currency) domain.Money {
	t.Helper()
	money, err := domain.NewMoneyFromString(value, currency)
	if err != nil {
		t.Fatalf("构造金额 %s：%v", value, err)
	}
	return money
}

func cardRegistration(t *testing.T, tenant string, plan domain.PricingPlanVersion) domain.PriceCardRegistration {
	t.Helper()
	source, err := domain.NewSourceFileIdentity("SYN-PRC-CARD-260820.xlsx", catalogSourceSHA)
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	grant, err := domain.NewVersionReference(domain.ArtifactCommercialAuthorization, "SYN-PRC-GRANT", "v1", "sha256:syn-grant")
	if err != nil {
		t.Fatalf("构造授权引用：%v", err)
	}
	registration, err := domain.NewPriceCardRegistration(
		evaluationValue(t, domain.NewTenantID, tenant),
		plan,
		source,
		grant,
		"SYN-PRC-GOVERNANCE",
	)
	if err != nil {
		t.Fatalf("构造价卡登记：%v", err)
	}
	return registration
}

func registerCard(t *testing.T, catalog *adapter.PriceCards, transactor bentoapp.Transactor, ctx context.Context, registration domain.PriceCardRegistration) ports.PriceCardRegistrationOutcome {
	t.Helper()
	var outcome ports.PriceCardRegistrationOutcome
	inCatalogTx(t, transactor, ctx, func(txCtx context.Context) error {
		saved, err := catalog.Register(txCtx, registration)
		outcome = saved
		return err
	})
	return outcome
}

func planSnapshotBytes(t *testing.T, plan domain.PricingPlanVersion) []byte {
	t.Helper()
	raw, err := domain.MarshalPricingPlanSnapshot(plan)
	if err != nil {
		t.Fatalf("折装方案快照：%v", err)
	}
	return raw
}

var catalogAsOf = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

// TestPriceCardRegisterAndLoadApplicable 证登记后按（方向+适用范围+计价基准时点）
// 原样装回：读回经领域整图重验，快照逐字节同答。
func TestPriceCardRegisterAndLoadApplicable(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	plan := catalogPlan(t, "plan-round", "v1", domain.PricingDirectionSell, "10")
	outcome := registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", plan))
	if outcome != ports.PriceCardRegistered {
		t.Fatalf("首登 outcome = %d, 想要 PriceCardRegistered", outcome)
	}

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	scope := evaluationValue(t, domain.NewPricingScopeID, "scope-1")
	loaded, err := catalog.LoadApplicable(ctx, tenant, domain.PricingDirectionSell, scope, catalogAsOf)
	if err != nil {
		t.Fatalf("装载适用价卡：%v", err)
	}
	if len(loaded) != 1 {
		t.Fatalf("适用价卡数 = %d, 想要 1", len(loaded))
	}
	if !bytes.Equal(planSnapshotBytes(t, loaded[0]), planSnapshotBytes(t, plan)) {
		t.Fatalf("装回快照与登记不同答")
	}
	if loaded[0].ContentDigest() != plan.ContentDigest() {
		t.Fatalf("内容摘要漂移")
	}
}

// TestPriceCardDirectionIsolation 证价格方向隔离：SELL 价卡在册不使 BUY 侧多出任何
// 适用候选，反之亦然。
func TestPriceCardDirectionIsolation(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-sell", "v1", domain.PricingDirectionSell, "10")))

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	scope := evaluationValue(t, domain.NewPricingScopeID, "scope-1")
	buySide, err := catalog.LoadApplicable(ctx, tenant, domain.PricingDirectionBuy, scope, catalogAsOf)
	if err != nil {
		t.Fatalf("装载 BUY 侧：%v", err)
	}
	if len(buySide) != 0 {
		t.Fatalf("BUY 侧不该有适用候选，得到 %d 份", len(buySide))
	}
}

// TestPriceCardApplicabilityFilters 证适用范围与计价基准时点都参与选卡：范围不同或
// 时点落在适用期外都不是适用候选。
func TestPriceCardApplicabilityFilters(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-window", "v1", domain.PricingDirectionSell, "10")))

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	scope := evaluationValue(t, domain.NewPricingScopeID, "scope-1")
	otherScope := evaluationValue(t, domain.NewPricingScopeID, "scope-2")

	late, err := catalog.LoadApplicable(ctx, tenant, domain.PricingDirectionSell, scope,
		time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("装载期外时点：%v", err)
	}
	if len(late) != 0 {
		t.Fatalf("适用期外不该有候选，得到 %d 份", len(late))
	}

	elsewhere, err := catalog.LoadApplicable(ctx, tenant, domain.PricingDirectionSell, otherScope, catalogAsOf)
	if err != nil {
		t.Fatalf("装载别范围：%v", err)
	}
	if len(elsewhere) != 0 {
		t.Fatalf("别的适用范围不该有候选，得到 %d 份", len(elsewhere))
	}
}

// TestPriceCardDuplicateRegistrationIsIdempotent 证同内容重复登记答`已登记`且原行
// 不被顶替。
func TestPriceCardDuplicateRegistrationIsIdempotent(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	registration := cardRegistration(t, "tenant-a", catalogPlan(t, "plan-dup", "v1", domain.PricingDirectionSell, "10"))
	if outcome := registerCard(t, catalog, transactor, ctx, registration); outcome != ports.PriceCardRegistered {
		t.Fatalf("首登 outcome = %d", outcome)
	}
	if outcome := registerCard(t, catalog, transactor, ctx, registration); outcome != ports.PriceCardAlreadyRegistered {
		t.Fatalf("重登 outcome = %d, 想要 PriceCardAlreadyRegistered", outcome)
	}
}

// TestPriceCardContentConflictIsAnswered 证版本内容冲突按业务答案交回：版本引用相同、
// 规范化版本相同而内容摘要不同，答`版本内容冲突`，原行保持原样。
func TestPriceCardContentConflictIsAnswered(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	original := catalogPlan(t, "plan-conflict", "v1", domain.PricingDirectionSell, "10")
	registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", original))

	// 同版本引用装了不同金额——内容摘要必然不同。
	impostor := catalogPlan(t, "plan-conflict", "v1", domain.PricingDirectionSell, "99")
	if impostor.ContentDigest() == original.ContentDigest() {
		t.Fatalf("夹具没造出内容分歧")
	}
	outcome := registerCard(t, catalog, transactor, ctx, cardRegistration(t, "tenant-a", impostor))
	if outcome != ports.PriceCardContentConflict {
		t.Fatalf("冒名登记 outcome = %d, 想要 PriceCardContentConflict", outcome)
	}

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	scope := evaluationValue(t, domain.NewPricingScopeID, "scope-1")
	loaded, err := catalog.LoadApplicable(ctx, tenant, domain.PricingDirectionSell, scope, catalogAsOf)
	if err != nil || len(loaded) != 1 {
		t.Fatalf("装载原行：err=%v n=%d", err, len(loaded))
	}
	if loaded[0].ContentDigest() != original.ContentDigest() {
		t.Fatalf("原行被顶替了")
	}
}

// TestPriceCardAmbiguousVersionsAreRefused 证同一方案身份两版同时适用交回错误：挑
// 任何一版都是替发布责任方作它没作的决定（先例：NR 目录的 ErrAmbiguousNetworkCatalog）。
func TestPriceCardAmbiguousVersionsAreRefused(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-amb", "v1", domain.PricingDirectionSell, "10")))
	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-amb", "v2", domain.PricingDirectionSell, "12")))

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	scope := evaluationValue(t, domain.NewPricingScopeID, "scope-1")
	if _, err := catalog.LoadApplicable(ctx, tenant, domain.PricingDirectionSell, scope, catalogAsOf); !errors.Is(err, adapter.ErrAmbiguousPriceCard) {
		t.Fatalf("err = %v, 想要 ErrAmbiguousPriceCard", err)
	}
}

// TestPriceCardMultipleCandidatesAllReturned 证多份适用价卡全部返回：每个候选各自
// 评价、评价之间没有优劣关系（CONTEXT），装载口不做择优。
func TestPriceCardMultipleCandidatesAllReturned(t *testing.T) {
	catalog, transactor := newPriceCards(t)
	ctx := t.Context()

	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-a", "v1", domain.PricingDirectionSell, "10")))
	registerCard(t, catalog, transactor, ctx,
		cardRegistration(t, "tenant-a", catalogPlan(t, "plan-b", "v1", domain.PricingDirectionSell, "12")))

	tenant := evaluationValue(t, domain.NewTenantID, "tenant-a")
	scope := evaluationValue(t, domain.NewPricingScopeID, "scope-1")
	loaded, err := catalog.LoadApplicable(ctx, tenant, domain.PricingDirectionSell, scope, catalogAsOf)
	if err != nil {
		t.Fatalf("装载多候选：%v", err)
	}
	if len(loaded) != 2 {
		t.Fatalf("候选数 = %d, 想要 2", len(loaded))
	}
	if loaded[0].Reference().ID() != "plan-a" || loaded[1].Reference().ID() != "plan-b" {
		t.Fatalf("候选次序不稳定：%s, %s", loaded[0].Reference().ID(), loaded[1].Reference().ID())
	}
}

// TestPriceCardRegisterOutsideTransactionRejected 证事务纪律：登记必须在事务内。
func TestPriceCardRegisterOutsideTransactionRejected(t *testing.T) {
	catalog, _ := newPriceCards(t)
	ctx := t.Context()

	registration := cardRegistration(t, "tenant-a", catalogPlan(t, "plan-notx", "v1", domain.PricingDirectionSell, "10"))
	if _, err := catalog.Register(ctx, registration); err == nil {
		t.Fatalf("无事务登记被接受了")
	}
}
