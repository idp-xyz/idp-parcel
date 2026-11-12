package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 证票 declared-goods-lines/19（ADR-0379 决定一、二）：留痕补记服务产品、逐件计价重量与费用行，写得进、
// 读得回；补记之前的旧行读成未记，跨迁移重驱的旧行答重放；按委托读各版估价连同两本账上的占用。

const estimateCard = "pricing-plan:SYN-PLAN-FBA@v2#c0ffee"

type estimateRig struct {
	t      *testing.T
	pool   *pgxpool.Pool
	db     *bentopg.DB
	traces *adapter.PreAcceptanceEstimateTraces
}

func newEstimateRig(t *testing.T) estimateRig {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	traces, err := adapter.NewPreAcceptanceEstimateTraces(db)
	if err != nil {
		t.Fatalf("构造留痕册：%v", err)
	}
	return estimateRig{t: t, pool: pool, db: db, traces: traces}
}

func (rig estimateRig) record(trace ports.PreAcceptanceEstimateTrace) ports.PreAcceptanceEstimateTraceEffect {
	rig.t.Helper()
	var effect ports.PreAcceptanceEstimateTraceEffect
	if err := rig.db.Transactor().WithinTransaction(rig.t.Context(), func(txCtx context.Context) error {
		var err error
		effect, err = rig.traces.RecordEstimate(txCtx, trace)
		return err
	}); err != nil {
		rig.t.Fatalf("写留痕：%v", err)
	}
	return effect
}

func (rig estimateRig) list(tenant, shipmentRequest string) []ports.PreAcceptanceEstimateEntry {
	rig.t.Helper()
	entries, err := rig.traces.ListPreAcceptanceEstimates(rig.t.Context(), mustAccountRef(rig.t, domain.NewTenantID, tenant), shipmentRequest)
	if err != nil {
		rig.t.Fatalf("读留痕：%v", err)
	}
	return entries
}

// detailedEstimate 是补记之后的一份估价：两件，每件带计价重量与费用行（第一件两行、金额多于两位小数也照原文），按产品选卡。
// baseAmount 换掉第一件基础运费那一行的金额，用来造「同合计、改了费用行」的那一份。
func detailedEstimate(t *testing.T, product, baseAmount string) domain.PreAcceptanceEstimate {
	t.Helper()
	cny := mustAccountRef(t, domain.NewCurrencyCode, "CNY")
	line := func(code string, effect domain.ChargeLineEffect, amount string) domain.PreAcceptanceTrialChargeLine {
		built, err := domain.NewPreAcceptanceTrialChargeLine(mustAccountRef(t, domain.NewPricingChargeCode, code), effect, amount, cny)
		if err != nil {
			t.Fatalf("charge line: %v", err)
		}
		return built
	}
	weight := func(value string) domain.PreAcceptanceTrialPricingWeight {
		built, err := domain.NewPreAcceptanceTrialPricingWeight(value, "KG")
		if err != nil {
			t.Fatalf("weight: %v", err)
		}
		return built
	}
	first, err := domain.NewPreAcceptanceTrial("DPCL-1", "EVAL-1", []string{estimateCard}, 1152, cny)
	if err != nil {
		t.Fatalf("trial: %v", err)
	}
	first, err = first.WithPricingWeight(weight("12.5")).WithChargeLines([]domain.PreAcceptanceTrialChargeLine{
		line("BASE_FREIGHT", domain.ChargeLineAdd, baseAmount),
		line("FUEL_SURCHARGE", domain.ChargeLineDeduct, "0.505"),
	})
	if err != nil {
		t.Fatalf("lines: %v", err)
	}
	second, err := domain.NewPreAcceptanceTrial("DPCL-2", "EVAL-2", []string{estimateCard}, 384, cny)
	if err != nil {
		t.Fatalf("trial: %v", err)
	}
	second, err = second.WithPricingWeight(weight("4")).WithChargeLines([]domain.PreAcceptanceTrialChargeLine{
		line("BASE_FREIGHT", domain.ChargeLineAdd, "3.84"),
	})
	if err != nil {
		t.Fatalf("lines: %v", err)
	}
	estimate, gap, err := domain.FormPerParcelSellTrialSum(estimateAdoption(t, "ACCT-1", "SCOPE-SELL-1"), estimateCard, cny,
		[]domain.PreAcceptanceTrial{first, second})
	if err != nil || gap != domain.PreAcceptanceEstimateFormed {
		t.Fatalf("estimate: gap = %s err = %v", gap, err)
	}
	if product == "" {
		return estimate
	}
	estimate, err = estimate.WithServiceProduct(product)
	if err != nil {
		t.Fatalf("product: %v", err)
	}
	return estimate
}

// traceFor 是某委托某版的一份留痕：控制请求标识照施加编排的拼法「委托/版本」。
func traceFor(t *testing.T, shipment, version string, estimate domain.PreAcceptanceEstimate, at time.Time) ports.PreAcceptanceEstimateTrace {
	t.Helper()
	trace := estimateTrace(t, shipment+"/"+version, estimate, at.Add(time.Minute))
	trace.ShipmentRequest, trace.SubmissionVersion, trace.At = shipment, version, at
	return trace
}

// seedLegacyTrace 照迁移 0057 之前的写口落一行：没有产品列，trials 每件只有评价、版本清单与合计，detail_recorded 为假。
func (rig estimateRig) seedLegacyTrace(tenant, shipment, version string, at time.Time) {
	rig.t.Helper()
	if _, err := rig.pool.Exec(rig.t.Context(),
		`INSERT INTO settlement_accounting.pre_acceptance_estimate_trace
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 shipment_request_ref, submission_version_ref, trial_at, form, pricing_scope_id,
			 price_card, total_minor, trials, recorded_at, detail_recorded)
		 VALUES ($1, 'LE-1', 'ACCT-1', 'CNY', $2 || '/' || $3, $2, $3, $4, 'PER_PARCEL_SELL_TRIAL_SUM', 'SCOPE-SELL-1',
		         $5, 1536,
		         '[{"parcel":"DPCL-1","evaluation":"EVAL-1","versions":["pricing-plan:SYN-PLAN-FBA@v2#c0ffee"],"totalMinor":1152,"currency":"CNY"},
		           {"parcel":"DPCL-2","evaluation":"EVAL-2","versions":["pricing-plan:SYN-PLAN-FBA@v2#c0ffee"],"totalMinor":384,"currency":"CNY"}]'::jsonb,
		         $4, false)`,
		tenant, shipment, version, at, estimateCard); err != nil {
		rig.t.Fatalf("落旧行：%v", err)
	}
}

// seedOccupation 照两本账写口的列落一条冻结或信用暴露，键与留痕同形；状态码即账本存的 uint8(status)。
func (rig estimateRig) seedOccupation(kind ports.PreAcceptanceOccupationKind, tenant, request, id string, status uint8,
	amount int64, formedAt time.Time, releasedAt *time.Time, converted int64, convertedAt *time.Time) {
	rig.t.Helper()
	table, idColumn, formedColumn := "funds_freeze", "freeze_id", "frozen_at"
	if kind == ports.OccupationCreditExposure {
		table, idColumn, formedColumn = "credit_exposure", "exposure_id", "exposed_at"
	}
	if _, err := rig.pool.Exec(rig.t.Context(),
		`INSERT INTO settlement_accounting.`+table+`
			(tenant_id, legal_entity, account_id, currency, control_request_id,
			 `+idColumn+`, status, amount_minor, association, request_digest, `+formedColumn+`,
			 released_at, converted_minor, converted_at)
		 VALUES ($1, 'LE-1', 'ACCT-1', 'CNY', $2, $3, $4, $5, 'ASSOC-1', 'digest', $6, $7, $8, $9)`,
		tenant, request, id, int16(status), amount, formedAt, releasedAt, converted, convertedAt); err != nil {
		rig.t.Fatalf("落 %s：%v", kind, err)
	}
}

// Covers: ADR-0379 决定一——补记的几格写得进、读得回：服务产品、逐件计价重量与费用行（金额照原文、带币种）、价卡拆回方案、版本与指纹；
// 补记之后的行 detailRecorded 为真。
func TestTheTraceRecordsTheServiceProductPricingWeightAndChargeLines(t *testing.T) {
	rig := newEstimateRig(t)
	trialAt := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	if effect := rig.record(traceFor(t, "SHR-1", "SUBV-1", detailedEstimate(t, "SYN-PRODUCT-FBA", "11.52"), trialAt)); effect != ports.PreAcceptanceEstimateTraceRecorded {
		t.Fatalf("首写 effect = %d", effect)
	}

	entries := rig.list("tenant-a", "SHR-1")
	if len(entries) != 1 {
		t.Fatalf("读回 %d 份, 想要 1", len(entries))
	}
	entry := entries[0]
	if entry.SubmissionVersion != "SUBV-1" || entry.Form != "PER_PARCEL_SELL_TRIAL_SUM" || entry.PricingScope != "SCOPE-SELL-1" ||
		entry.ServiceProduct != "SYN-PRODUCT-FBA" || !entry.DetailRecorded || entry.TotalMinor != 1536 || entry.Currency != "CNY" ||
		!entry.TrialAt.Equal(trialAt) || !entry.RecordedAt.Equal(trialAt.Add(time.Minute)) {
		t.Fatalf("估价 = %+v", entry)
	}
	if entry.PriceCard != (domain.VersionReferenceParts{Kind: "pricing-plan", ID: "SYN-PLAN-FBA", Version: "v2", Fingerprint: "c0ffee"}) {
		t.Fatalf("价卡 = %+v", entry.PriceCard)
	}
	if len(entry.Parcels) != 2 {
		t.Fatalf("逐件 %d", len(entry.Parcels))
	}
	first := entry.Parcels[0]
	if first.Parcel != "DPCL-1" || first.Evaluation != "EVAL-1" || first.TotalMinor != 1152 || first.Currency != "CNY" ||
		first.PricingWeight == nil || *first.PricingWeight != (ports.PreAcceptanceParcelPricingWeight{Value: "12.5", Unit: "KG"}) {
		t.Fatalf("第一件 = %+v", first)
	}
	if len(first.ChargeLines) != 2 ||
		first.ChargeLines[0] != (ports.PreAcceptanceParcelChargeLine{Code: "BASE_FREIGHT", Effect: "ADD", Amount: "11.52", Currency: "CNY"}) ||
		first.ChargeLines[1] != (ports.PreAcceptanceParcelChargeLine{Code: "FUEL_SURCHARGE", Effect: "DEDUCT", Amount: "0.505", Currency: "CNY"}) {
		t.Fatalf("第一件费用行 = %+v", first.ChargeLines)
	}
	if len(entry.Occupations) != 0 {
		t.Fatalf("没有占用却读出 %+v", entry.Occupations)
	}
}

// Covers: ADR-0379 决定一「重放判等把新格算进去」——同一控制请求带同一份估价再来答重放；只改了一行费用的金额（合计不变）、或换了
// 服务产品，都答冲突；已落的行不动。
func TestARedriveWithChangedChargeLinesOrProductConflicts(t *testing.T) {
	rig := newEstimateRig(t)
	trialAt := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	for _, step := range []struct {
		name     string
		estimate domain.PreAcceptanceEstimate
		effect   ports.PreAcceptanceEstimateTraceEffect
	}{
		{"首写", detailedEstimate(t, "SYN-PRODUCT-FBA", "11.52"), ports.PreAcceptanceEstimateTraceRecorded},
		{"重放", detailedEstimate(t, "SYN-PRODUCT-FBA", "11.52"), ports.PreAcceptanceEstimateTraceReplay},
		{"改了费用行", detailedEstimate(t, "SYN-PRODUCT-FBA", "11.53"), ports.PreAcceptanceEstimateTraceConflict},
		{"换了产品", detailedEstimate(t, "SYN-PRODUCT-OTHER", "11.52"), ports.PreAcceptanceEstimateTraceConflict},
		{"没了产品", detailedEstimate(t, "", "11.52"), ports.PreAcceptanceEstimateTraceConflict},
	} {
		if effect := rig.record(traceFor(t, "SHR-1", "SUBV-1", step.estimate, trialAt)); effect != step.effect {
			t.Fatalf("%s：effect = %d, 想要 %d", step.name, effect, step.effect)
		}
	}
	if line := rig.list("tenant-a", "SHR-1")[0].Parcels[0].ChargeLines[0]; line.Amount != "11.52" {
		t.Fatalf("冲突改写了已落的行：%+v", line)
	}
}

// Covers: ADR-0379 决定一「补记之前的留痕答未记，不回填」——旧行读成 detailRecorded:false，没有产品、计价重量与费用行；同一控制请求
// 在迁移之后带补记重驱是重放（只比旧行当时有的几格），合计或逐件变了仍是冲突；旧行不因重放被补上。
func TestALegacyTraceReadsAsNotRecordedAndARedriveAcrossTheMigrationReplays(t *testing.T) {
	rig := newEstimateRig(t)
	trialAt := time.Date(2026, 11, 10, 8, 0, 0, 0, time.UTC)
	rig.seedLegacyTrace("tenant-a", "SHR-OLD", "SUBV-1", trialAt)

	if effect := rig.record(traceFor(t, "SHR-OLD", "SUBV-1", detailedEstimate(t, "SYN-PRODUCT-FBA", "11.52"), trialAt)); effect != ports.PreAcceptanceEstimateTraceReplay {
		t.Fatalf("跨迁移重驱 effect = %d, 想要重放", effect)
	}
	changed := detailedEstimate(t, "SYN-PRODUCT-FBA", "11.52")
	trials := changed.Trials()
	other, err := domain.NewPreAcceptanceTrial("DPCL-1", "EVAL-9", []string{estimateCard}, 1152, changed.Currency())
	if err != nil {
		t.Fatalf("trial: %v", err)
	}
	changed, _, err = domain.FormPerParcelSellTrialSum(estimateAdoption(t, "ACCT-1", "SCOPE-SELL-1"), estimateCard, changed.Currency(),
		[]domain.PreAcceptanceTrial{other, trials[1]})
	if err != nil {
		t.Fatalf("estimate: %v", err)
	}
	if effect := rig.record(traceFor(t, "SHR-OLD", "SUBV-1", changed, trialAt)); effect != ports.PreAcceptanceEstimateTraceConflict {
		t.Fatalf("换了评价 effect = %d, 想要冲突", effect)
	}

	entry := rig.list("tenant-a", "SHR-OLD")[0]
	if entry.DetailRecorded || entry.ServiceProduct != "" || len(entry.Parcels) != 2 || entry.TotalMinor != 1536 {
		t.Fatalf("旧行 = %+v, 想要未记、不回填", entry)
	}
	for _, parcel := range entry.Parcels {
		if parcel.PricingWeight != nil || parcel.ChargeLines != nil {
			t.Fatalf("旧行的一件长出了补记：%+v", parcel)
		}
	}
}

// Covers: ADR-0379 决定二——按（租户，委托）读：一票两版按试算时点列；冻结（占用中、部分转出）、信用暴露（已释放）、组合策略两本账都占、
// 受限无占用四种都对；别的委托与别的租户的留痕读不到；没有留痕答空列表。
func TestEstimatesAreReadByShipmentRequestWithTheirOccupations(t *testing.T) {
	rig := newEstimateRig(t)
	first := time.Date(2026, 11, 11, 8, 0, 0, 0, time.UTC)
	second := first.Add(3 * time.Hour)
	released := second.Add(time.Hour)
	converted := first.Add(2 * time.Hour)

	// 后一版先写：次序按试算时点，不按写入先后。
	rig.record(traceFor(t, "SHR-1", "SUBV-2", detailedEstimate(t, "", "11.52"), second))
	rig.record(traceFor(t, "SHR-1", "SUBV-1", detailedEstimate(t, "SYN-PRODUCT-FBA", "11.52"), first))
	rig.seedOccupation(ports.OccupationFundsFreeze, "tenant-a", "SHR-1/SUBV-1", "FRZ-1", uint8(domain.FreezeHeld),
		1536, first.Add(time.Minute), nil, 384, &converted)
	rig.seedOccupation(ports.OccupationCreditExposure, "tenant-a", "SHR-1/SUBV-2", "EXP-2", uint8(domain.ExposureReleased),
		1536, second.Add(time.Minute), &released, 0, nil)
	// 组合策略：一份估价两本账都占，冻结在前。
	rig.record(traceFor(t, "SHR-BOTH", "SUBV-1", detailedEstimate(t, "", "11.52"), first))
	rig.seedOccupation(ports.OccupationCreditExposure, "tenant-a", "SHR-BOTH/SUBV-1", "EXP-B", uint8(domain.ExposureRecorded),
		1536, first, nil, 0, nil)
	rig.seedOccupation(ports.OccupationFundsFreeze, "tenant-a", "SHR-BOTH/SUBV-1", "FRZ-B", uint8(domain.FreezeConverted),
		1536, first, nil, 1536, &converted)
	// 受限：留痕在、两本账没有行。
	rig.record(traceFor(t, "SHR-RESTRICTED", "SUBV-1", detailedEstimate(t, "", "11.52"), first))
	// 别的租户同号委托。
	other := traceFor(t, "SHR-1", "SUBV-9", detailedEstimate(t, "", "11.52"), first)
	other.Tenant = mustAccountRef(t, domain.NewTenantID, "tenant-b")
	rig.record(other)

	entries := rig.list("tenant-a", "SHR-1")
	if len(entries) != 2 || entries[0].SubmissionVersion != "SUBV-1" || entries[1].SubmissionVersion != "SUBV-2" {
		t.Fatalf("两版 = %+v, 想要 SUBV-1、SUBV-2 按试算时点", entries)
	}
	if got := entries[0].Occupations; len(got) != 1 || got[0].Kind != ports.OccupationFundsFreeze || got[0].ID != "FRZ-1" ||
		got[0].Status != "HELD" || got[0].AmountMinor != 1536 || got[0].Currency != "CNY" || !got[0].FormedAt.Equal(first.Add(time.Minute)) ||
		got[0].ReleasedAt != nil || got[0].ConvertedMinor != 384 || got[0].ConvertedAt == nil || !got[0].ConvertedAt.Equal(converted) {
		t.Fatalf("冻结 = %+v", got)
	}
	if got := entries[1].Occupations; len(got) != 1 || got[0].Kind != ports.OccupationCreditExposure || got[0].ID != "EXP-2" ||
		got[0].Status != "RELEASED" || got[0].ReleasedAt == nil || !got[0].ReleasedAt.Equal(released) || got[0].ConvertedAt != nil {
		t.Fatalf("信用暴露 = %+v", got)
	}
	both := rig.list("tenant-a", "SHR-BOTH")
	if len(both) != 1 || len(both[0].Occupations) != 2 ||
		both[0].Occupations[0].Kind != ports.OccupationFundsFreeze || both[0].Occupations[0].Status != "CONVERTED" ||
		both[0].Occupations[1].Kind != ports.OccupationCreditExposure || both[0].Occupations[1].Status != "RECORDED" {
		t.Fatalf("组合策略 = %+v", both)
	}
	restricted := rig.list("tenant-a", "SHR-RESTRICTED")
	if len(restricted) != 1 || restricted[0].Occupations == nil || len(restricted[0].Occupations) != 0 {
		t.Fatalf("受限 = %+v, 想要一份估价、占用为空列表", restricted)
	}
	if foreign := rig.list("tenant-b", "SHR-1"); len(foreign) != 1 || foreign[0].SubmissionVersion != "SUBV-9" {
		t.Fatalf("tenant-b 读到 %+v", foreign)
	}
	if none := rig.list("tenant-a", "SHR-NONE"); none == nil || len(none) != 0 {
		t.Fatalf("没有留痕 = %#v, 想要空列表", none)
	}
}
