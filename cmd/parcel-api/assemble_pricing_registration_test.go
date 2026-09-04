package main

import (
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: `/pricing-price-card-registrations` 与 `/pricing-reference-series-registrations`
// 的第二参是真编排——登记用例、真库登记册与装配点的事务包装在真实 PostgreSQL 上装得
// 起来。各钉两格：首登「已入册」，随后的同内容重放答「已在册」——重放读得到首行即证
// 首登那笔事务确实提交了；幂等重放是业务答案不是失败（判据同登记 CLI 退出码 0）。
// 测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredPricingRegistrationsRecordAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	cards, err := buildPriceCardRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配价卡登记编排：%v", err)
	}
	series, err := buildReferenceSeriesRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配序列登记编排：%v", err)
	}

	cardCommand := pricingapp.RegisterPriceCardCommand{Registration: syntheticPriceCardRegistration(t)}
	cardRecorded, err := cards.Handle(t.Context(), cardCommand)
	if err != nil {
		t.Fatalf("价卡首登：%v", err)
	}
	if cardRecorded != pricingapp.PriceCardRecorded {
		t.Fatalf("价卡首登 outcome = %s, 想要 RECORDED", cardRecorded)
	}
	cardReplayed, err := cards.Handle(t.Context(), cardCommand)
	if err != nil {
		t.Fatalf("价卡重放：%v", err)
	}
	if cardReplayed != pricingapp.PriceCardAlreadyOnRegister {
		t.Fatalf("价卡重放 outcome = %s, 想要 ALREADY_ON_REGISTER——没读到首行说明首登事务没提交", cardReplayed)
	}

	seriesCommand := pricingapp.RegisterReferenceSeriesCommand{Registration: syntheticFuelSeriesRegistration(t)}
	seriesRecorded, err := series.Handle(t.Context(), seriesCommand)
	if err != nil {
		t.Fatalf("序列首登：%v", err)
	}
	if seriesRecorded != pricingapp.ReferenceSeriesRecorded {
		t.Fatalf("序列首登 outcome = %s, 想要 RECORDED", seriesRecorded)
	}
	seriesReplayed, err := series.Handle(t.Context(), seriesCommand)
	if err != nil {
		t.Fatalf("序列重放：%v", err)
	}
	if seriesReplayed != pricingapp.ReferenceSeriesAlreadyOnRegister {
		t.Fatalf("序列重放 outcome = %s, 想要 ALREADY_ON_REGISTER", seriesReplayed)
	}
}

// Covers: ADR-0101 决定四那句硬句在真库上的钉（票 pricing-reference-series-operations/08）——
// **同一份运营载荷**经解码器分别走预览编排与登记编排，预览答复的 contentDigest 与登记册随后
// 上列的 contentDigest 逐字节相同；预览不写库（预览之后目录仍是空的）；更正版本的预览按回指
// 取回册上那一版并逐期比对。输入是隔离合成，只记 `S`。
func TestThePreviewAndTheRegisterAgreeOnTheDigestForOnePayload(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	preview, err := buildReferenceSeriesPreviewOrchestration(db)
	if err != nil {
		t.Fatalf("装配预览编排：%v", err)
	}
	register, err := buildReferenceSeriesRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配登记编排：%v", err)
	}
	catalogue, err := pppostgres.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面：%v", err)
	}
	tenant := mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-1")
	const registrant = "SYN-PRC-SERIES-REGISTRAR"

	first := decodeSeriesPayload(t, `{
	  "seriesId": "SYN-PRC-PREVIEW-FUEL", "seriesVersion": "v1", "kind": "FUEL_RATE",
	  "sourceIdentifier": "SYN-CARRIER/fuel-weekly-bulletin",
	  "periods": [
	    {"startsAt": "2026-08-03T00:00:00Z", "endsAt": "2026-08-10T00:00:00Z", "value": "0.22", "evidenceRef": "SYN-EVIDENCE/fuel-2026-W32"},
	    {"startsAt": "2026-08-10T00:00:00Z", "value": "0.24", "evidenceRef": "SYN-EVIDENCE/fuel-2026-W33"}
	  ]
	}`)
	previewCommand, err := first.PreviewCommand(tenant, registrant)
	if err != nil {
		t.Fatalf("预览命令：%v", err)
	}
	previewed, err := preview.Handle(t.Context(), previewCommand)
	if err != nil {
		t.Fatalf("预览：%v", err)
	}
	if previewed.Outcome != pricingapp.ReferenceSeriesPreviewed || previewed.EvidenceGrade != pricingdomain.SeriesEvidenceVerifiable {
		t.Fatalf("预览 outcome/等级 = %s/%s", previewed.Outcome, previewed.EvidenceGrade)
	}
	if rows, err := catalogue.ListReferenceSeries(t.Context(), tenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("预览之后目录应仍为空：rows=%d err=%v", len(rows), err)
	}

	registerCommand, err := first.RegistrationCommand(tenant, registrant)
	if err != nil {
		t.Fatalf("登记命令：%v", err)
	}
	if outcome, err := register.Handle(t.Context(), registerCommand); err != nil || outcome != pricingapp.ReferenceSeriesRecorded {
		t.Fatalf("登记 outcome = %s err = %v", outcome, err)
	}
	rows, err := catalogue.ListReferenceSeries(t.Context(), tenant, 10)
	if err != nil || len(rows) != 1 {
		t.Fatalf("登记后上列：rows=%d err=%v", len(rows), err)
	}
	if rows[0].ContentDigest != previewed.ContentDigest || rows[0].Canonicalization != previewed.Canonicalization {
		t.Fatalf("预览摘要 %q 与登记册摘要 %q 不同——决定四那句硬句破了", previewed.ContentDigest, rows[0].ContentDigest)
	}
	if rows[0].EvidenceGrade != previewed.EvidenceGrade.String() {
		t.Fatalf("预览等级 %s 与登记册等级 %s 不同", previewed.EvidenceGrade, rows[0].EvidenceGrade)
	}
	if rows[0].ReferenceDigest != registerCommand.Registration.Reference().Digest() ||
		rows[0].ReferenceDigest != "declared:reference-series/SYN-PRC-PREVIEW-FUEL@v1" {
		t.Fatalf("册上的引用 digest = %q，想要解码器铸的声明令牌", rows[0].ReferenceDigest)
	}

	// 更正版本：回指 v1 并带上册上透出的引用 digest，预览要取回 v1 逐期比对。
	correction := decodeSeriesPayload(t, `{
	  "seriesId": "SYN-PRC-PREVIEW-FUEL", "seriesVersion": "v2", "kind": "FUEL_RATE",
	  "sourceIdentifier": "SYN-CARRIER/fuel-weekly-bulletin",
	  "periods": [
	    {"startsAt": "2026-08-03T00:00:00Z", "endsAt": "2026-08-10T00:00:00Z", "value": "0.23", "evidenceRef": "SYN-EVIDENCE/fuel-2026-W32"},
	    {"startsAt": "2026-08-10T00:00:00Z", "value": "0.24", "evidenceRef": "SYN-EVIDENCE/fuel-2026-W33"}
	  ],
	  "correction": {"priorVersion": "v1", "priorReferenceDigest": "`+rows[0].ReferenceDigest+`", "basis": "SYN-CORRECTION/fuel-w32-transcription"}
	}`)
	correctionPreview, err := correction.PreviewCommand(tenant, registrant)
	if err != nil {
		t.Fatalf("更正预览命令：%v", err)
	}
	compared, err := preview.Handle(t.Context(), correctionPreview)
	if err != nil {
		t.Fatalf("更正预览：%v", err)
	}
	if compared.Comparison != pricingapp.SeriesComparisonCompared || compared.BaseVersion != "v1" {
		t.Fatalf("更正预览对照 = %s/%q", compared.Comparison, compared.BaseVersion)
	}
	if len(compared.Changes) != 2 || compared.Changes[0].Kind() != pricingdomain.SeriesPeriodChanged ||
		!compared.Changes[0].ValueChanged() || compared.Changes[1].Kind() != pricingdomain.SeriesPeriodUnchanged {
		t.Fatalf("更正预览逐期差异变形：%+v", compared.Changes)
	}
	if rows, err := catalogue.ListReferenceSeries(t.Context(), tenant, 10); err != nil || len(rows) != 1 {
		t.Fatalf("更正预览不该写库：rows=%d err=%v", len(rows), err)
	}
}

func decodeSeriesPayload(t *testing.T, raw string) pricinghttp.ReferenceSeriesRegistrationPayload {
	t.Helper()
	payload, err := pricinghttp.DecodeReferenceSeriesRegistrationPayload(strings.NewReader(raw))
	if err != nil {
		t.Fatalf("解码载荷：%v", err)
	}
	return payload
}

func syntheticPricingReference(t *testing.T, artifact pricingdomain.ArtifactKind, id, version string) pricingdomain.VersionReference {
	t.Helper()
	reference, err := pricingdomain.NewVersionReference(artifact, id, version, "sha256:syn-"+id+"-"+version)
	if err != nil {
		t.Fatalf("构造版本引用 %s/%s：%v", id, version, err)
	}
	return reference
}

func syntheticPricingWeight(t *testing.T, value string) pricingdomain.Weight {
	t.Helper()
	weight, err := pricingdomain.NewWeight(mustValue(t, pricingdomain.ParseDecimal, value), pricingdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("构造重量 %s：%v", value, err)
	}
	return weight
}

// syntheticPriceCardRegistration 造一份立得住的最小合成价卡登记：单费率段、整年适用
// 期。形状抄计价真库测试的 catalogPlan，身份全取 SYN-PRC-REG 前缀避免与其它测试相撞。
func syntheticPriceCardRegistration(t *testing.T) pricingdomain.PriceCardRegistration {
	t.Helper()
	currency := mustValue(t, pricingdomain.NewCurrency, "USD")
	amount, err := pricingdomain.NewMoneyFromString("10", currency)
	if err != nil {
		t.Fatalf("构造金额：%v", err)
	}
	entry, err := pricingdomain.NewRateEntry(
		mustValue(t, pricingdomain.NewRateEntryID, "SYN-PRC-REG-ENTRY-1"),
		"Z1",
		syntheticPricingWeight(t, "0"),
		syntheticPricingWeight(t, "10"),
		amount,
	)
	if err != nil {
		t.Fatalf("构造费率段：%v", err)
	}
	period, err := pricingdomain.NewEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("构造适用期：%v", err)
	}
	table, err := pricingdomain.NewRateTableVersion(
		syntheticPricingReference(t, pricingdomain.ArtifactRateTable, "SYN-PRC-REG-TABLE", "v1"),
		pricingdomain.RateTableFamilyWeightZone, currency, pricingdomain.WeightUnitKilogram,
		period, []pricingdomain.RateEntry{entry})
	if err != nil {
		t.Fatalf("构造价表：%v", err)
	}
	rounding, err := pricingdomain.NewWeightRoundingPolicy(pricingdomain.RoundingCeiling, syntheticPricingWeight(t, "0.5"))
	if err != nil {
		t.Fatalf("构造取整策略：%v", err)
	}
	weightPolicy, err := pricingdomain.NewPricingWeightPolicy(
		syntheticPricingReference(t, pricingdomain.ArtifactWeightPolicy, "SYN-PRC-REG-WEIGHT", "v1"),
		pricingdomain.PricingWeightActualOnly, rounding, nil)
	if err != nil {
		t.Fatalf("构造计价重策略：%v", err)
	}
	plan, err := pricingdomain.NewPricingPlanVersion(
		syntheticPricingReference(t, pricingdomain.ArtifactPricingPlan, "SYN-PRC-REG-PLAN", "v1"),
		mustValue(t, pricingdomain.NewPricingScopeID, "SYN-PRC-REG-SCOPE"),
		pricingdomain.PricingDirectionSell,
		pricingdomain.PricingPurposeCustomerCharge,
		mustValue(t, pricingdomain.NewChargeCode, "BASE_FREIGHT"),
		period,
		table,
		weightPolicy,
		nil,
		pricingdomain.PricingPlanStructures{},
	)
	if err != nil {
		t.Fatalf("构造价卡：%v", err)
	}
	source, err := pricingdomain.NewSourceFileIdentity(
		"SYN-PRC-REG-CARD-260831.xlsx",
		"9edaf27ef93004e00f73a65471897f2cf7064d5d4df05014934ef7ac5861d33d")
	if err != nil {
		t.Fatalf("构造源文件身份：%v", err)
	}
	registration, err := pricingdomain.NewPriceCardRegistration(
		mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-1"),
		plan,
		source,
		syntheticPricingReference(t, pricingdomain.ArtifactCommercialAuthorization, "SYN-PRC-REG-GRANT", "v1"),
		"SYN-PRC-GOVERNANCE",
	)
	if err != nil {
		t.Fatalf("构造价卡登记：%v", err)
	}
	return registration
}

// syntheticFuelSeriesRegistration 造一份最小合成燃油序列登记：单期次带凭证引用。
// 燃油种类不需要报价口径，夹具因此不涉商业价格政策。
func syntheticFuelSeriesRegistration(t *testing.T) pricingdomain.ReferenceSeriesRegistration {
	t.Helper()
	period, err := pricingdomain.NewSeriesPeriodValue(
		time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC),
		time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		mustValue(t, pricingdomain.ParseDecimal, "0.22"),
		"SYN-EVIDENCE/fuel-2026-W32",
	)
	if err != nil {
		t.Fatalf("构造期次：%v", err)
	}
	registration, err := pricingdomain.NewReferenceSeriesRegistration(pricingdomain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, pricingdomain.NewTenantID, "SYN-TENANT-1"),
		Kind:             pricingdomain.ReferenceSeriesFuelRate,
		Reference:        syntheticPricingReference(t, pricingdomain.ArtifactReferenceSeries, "SYN-PRC-REG-FUEL", "v1"),
		SourceIdentifier: "SYN-CARRIER/fuel-weekly-bulletin",
		Registrant:       "SYN-PRC-SERIES-REGISTRAR",
		Periods:          []pricingdomain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("构造燃油序列登记：%v", err)
	}
	return registration
}
