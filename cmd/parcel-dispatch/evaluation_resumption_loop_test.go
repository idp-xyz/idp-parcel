package main

import (
	"context"
	"strings"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppsettlement "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/settlementaccounting"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapricing "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件对真实 PostgreSQL 16 在生产路由表上证 ADR-0229 的续办：停在计价那一段的请求与停在发起之前的那一拍，租户补登
// 参数之后经「参数已登记」信封续办——另起新一代请求、回指前一代，评价与费用（成本）照常形成；旧请求一个字不改；重投
// 不出第二代；没有一封信因为答`未配置`而按未决重投到耗尽。夹具全是 `SYN-` 合成值，证据只记 S。

// drainBeats 一拍接一拍地派发，直到一拍什么都没发出去或拍满上限。夹具里另有几封停在别的格子上的信（受理链的路由证据
// 未配置一类），它们按失败预算重试、与本文件无关；上限只防它们让循环转个没完。
func drainBeats(t *testing.T, beat Beat) {
	t.Helper()
	for round := 0; round < 12; round++ {
		published, err := beat.DispatchOnce(t.Context())
		if err != nil {
			t.Fatalf("第 %d 拍：%v", round+1, err)
		}
		if published == 0 {
			return
		}
	}
}

// buyRequestRow 是 BUY 请求册上一行的续办相关格。
type buyRequestRow struct {
	id, previous, basis, stopCause string
	generation                     int
}

func buyRequestsOf(t *testing.T, db *bentopg.DB, occurrence string) []buyRequestRow {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(), `
		SELECT request_id, generation, COALESCE(previous_request_id, ''), COALESCE(resume_basis, ''), COALESCE(stop_cause, '')
		  FROM settlement_accounting.evaluation_request
		 WHERE tenant_id = $1 AND occurrence_id = $2
		 ORDER BY generation`, demoTenant, occurrence)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var found []buyRequestRow
	for rows.Next() {
		var row buyRequestRow
		if err := rows.Scan(&row.id, &row.generation, &row.previous, &row.basis, &row.stopCause); err != nil {
			t.Fatal(err)
		}
		found = append(found, row)
	}
	return found
}

func supplierCostVersions(t *testing.T, db *bentopg.DB) []string {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	rows, err := querier.Query(t.Context(),
		`SELECT version FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1 ORDER BY version`, demoTenant)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var versions []string
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err != nil {
			t.Fatal(err)
		}
		versions = append(versions, version)
	}
	return versions
}

// assertNothingAbandoned 断言这几类信封一封都没有落在失败码上：答`未配置`不再按未决重投，参数已登记与续办出来的那几封
// 都入了账。
func assertNothingAbandoned(t *testing.T, db *bentopg.DB, eventTypes ...string) {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var failed int
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*) FROM `+migrate.SchemaBento+`.outbox WHERE event_type = ANY($1) AND (failure_code IS NOT NULL OR status <> 'PUBLISHED')`,
		eventTypes).Scan(&failed); err != nil {
		t.Fatal(err)
	}
	if failed != 0 {
		t.Fatalf("%d 封 %v 没有入账（落在失败码上或还在排队）", failed, eventTypes)
	}
}

// buyResumptionFixture 铺一条 BUY 能走到底的链：一件经受理链接受、申报了重量尺寸与寄收邮编的包裹（SYN-PARCEL-01）、
// 分区目录与燃油序列在用、干线协议（商业范围 SYN-SCOPE-01）。演示成本卡登不登、触发册怎么登由各用例自己定。
func buyResumptionFixture(t *testing.T) (*synVerticalFixture, Beat, *outbox.Store) {
	t.Helper()
	fixture := newSYNVerticalFixture(t)
	acceptedSellParcel(t, fixture)
	drainBeats(t, fixture.beat)
	registerDemoSellPricing(t, fixture.db)
	registerTrunkAgreement(t, fixture.db, demoTenant)
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	return fixture, fixture.beat, store
}

func enqueueOccurrenceFormed(t *testing.T, db *bentopg.DB, store *outbox.Store, id, occurrence string) {
	t.Helper()
	enqueueForBeat(t, db, store, id, sainbox.ChargeOccurrenceFormedEventType,
		`{"tenantId":"`+demoTenant+`","occurrence":"`+occurrence+`","version":"V1"}`)
}

// Covers: AT-SA-195、AT-PP-005——票面现象那一路（BUY）：发生项发起的请求在计价那一侧答「没有在用价卡」，PP 入账交「评价
// 未形成」、SA 在第一代上记停因（不再按未决重投到 ABANDONED）；租户补登演示成本卡，价卡登记同笔交「参数已登记」，SA 按
// 停因与计价范围续办出第二代（回指第一代、带依据），PP 选到卡形成评价，SA 以第二代派生的版本标识形成供应商预期成本。
// 第一代的停因原样、它名下没有成本；再登一张不相干的卡不再推出第三代。
func TestABuyRequestStalledOnAMissingPriceCardResumesAfterTheCardIsRegistered(t *testing.T) {
	fixture, beat, store := buyResumptionFixture(t)
	registerBuyTriggers(t, fixture.db, demoTenant)
	insertChargeOccurrence(t, fixture.db, demoTenant, "SYN-OCC-TRUNK-01", tfdomain.ActualFulfillmentOccurrence, trunkAgreementObject+"/v1")
	enqueueOccurrenceFormed(t, fixture.db, store, "occ-trunk", "SYN-OCC-TRUNK-01")
	drainBeats(t, beat)

	stalled := buyRequestsOf(t, fixture.db, "SYN-OCC-TRUNK-01")
	if len(stalled) != 1 || stalled[0].generation != 1 || stalled[0].stopCause != "PRICE_CARD_NOT_CONFIGURED" {
		t.Fatalf("没卡时的 BUY 请求册 = %+v，want 一份第一代、停在 PRICE_CARD_NOT_CONFIGURED", stalled)
	}
	assertNothingAbandoned(t, fixture.db, "settlement-accounting.evaluation-request.submitted", "parcel-pricing.evaluation-request.unformed")
	if versions := supplierCostVersions(t, fixture.db); len(versions) != 0 {
		t.Fatalf("没卡却形成了成本：%v", versions)
	}

	registerDemoCostCard(t, fixture.db)
	drainBeats(t, beat)

	lineage := buyRequestsOf(t, fixture.db, "SYN-OCC-TRUNK-01")
	if len(lineage) != 2 {
		t.Fatalf("补登卡之后的 BUY 请求册 = %+v，want 两代", lineage)
	}
	first, second := lineage[0], lineage[1]
	if first.stopCause != "PRICE_CARD_NOT_CONFIGURED" || first.previous != "" || first.basis != "" {
		t.Fatalf("第一代被改写：%+v", first)
	}
	if second.generation != 2 || second.previous != first.id || second.stopCause != "" ||
		!strings.HasPrefix(second.basis, "PARAMREG/PRICE_CARD/") {
		t.Fatalf("第二代 = %+v，want 回指第一代、凭价卡登记续办、没有停因", second)
	}
	if versions := supplierCostVersions(t, fixture.db); len(versions) != 1 || versions[0] != "BUYSC-"+second.id {
		t.Fatalf("预期成本版本 = %v，want 恰一份 BUYSC-%s", versions, second.id)
	}
	assertNothingAbandoned(t, fixture.db,
		"settlement-accounting.evaluation-request.submitted", "parcel-pricing.evaluation-request.unformed",
		"parcel-pricing.pricing-parameter.registered", "parcel-pricing.evaluation.recorded")

	// 又一次价卡登记（别的范围的一张卡）：没有在等的请求，不推出第三代。
	enqueueForBeat(t, fixture.db, store, "pricing-parameter-again", sainbox.PricingParameterRegisteredResumption.EventType,
		`{"tenantId":"`+demoTenant+`","registrationKind":"PRICE_CARD","pricingScope":"SYN-SCOPE-01"}`)
	drainBeats(t, beat)
	if again := buyRequestsOf(t, fixture.db, "SYN-OCC-TRUNK-01"); len(again) != 2 {
		t.Fatalf("没有在等的请求，第二封参数已登记却又续办：%+v", again)
	}
}

// Covers: AT-SA-194、AT-SA-197（M15）——发起之前那一段：触发册先登了没有费用项目的一行，发生项到来答`未配置`、记进 BUY
// 发起停办册、零请求；租户给那一行追加一版带费用项目的登记（旧版留在册上），登记同笔交「评价参数已登记」，SA 按记下的
// 发生项与协议重跑发起编排，发出首代请求（带依据），评价与预期成本照常形成。同一份登记重放不出第二份请求。
func TestABuyOccurrenceStalledOnAMissingFeeItemResumesAfterTheTriggerIsRevised(t *testing.T) {
	fixture, beat, store := buyResumptionFixture(t)
	registerDemoCostCard(t, fixture.db)

	handoff, err := sapostgres.NewOutboxEvaluationParameterRegisteredHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	triggers, err := sapostgres.NewBuyEvaluationTriggers(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	register, err := saapplication.NewRegisterBuyEvaluationTriggerHandler(triggers, handoff, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	bare, err := sadomain.NewBuyEvaluationTriggerRegistration(
		saValue(t, sadomain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"), sadomain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatal(err)
	}
	tenant := saValue(t, sadomain.NewTenantID, demoTenant)
	registerTrigger := func(registration sadomain.BuyEvaluationTriggerRegistration, want saports.CatalogueRegistrationEffect) {
		t.Helper()
		mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
			effect, err := register.RegisterBuyEvaluationTrigger(ctx, tenant, registration)
			if err == nil && effect != want {
				t.Errorf("触发册登记 = %d，want %d", effect, want)
			}
			return err
		})
	}
	registerTrigger(bare, saports.CatalogueRegistered)
	drainBeats(t, beat)

	insertChargeOccurrence(t, fixture.db, demoTenant, "SYN-OCC-TRUNK-02", tfdomain.ActualFulfillmentOccurrence, trunkAgreementObject+"/v1")
	enqueueOccurrenceFormed(t, fixture.db, store, "occ-trunk-2", "SYN-OCC-TRUNK-02")
	drainBeats(t, beat)
	if requests := buyRequestsOf(t, fixture.db, "SYN-OCC-TRUNK-02"); len(requests) != 0 {
		t.Fatalf("费用项目没登却发了请求：%+v", requests)
	}
	stalls, err := sapostgres.NewEvaluationResumptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	stall, found, err := stalls.FindBuyInitiationStall(t.Context(), tenant,
		saValue(t, sadomain.NewChargeOccurrenceID, "SYN-OCC-TRUNK-02"), saValue(t, sadomain.NewOccurrenceVersion, "V1"))
	if err != nil || !found || stall.Cause() != sadomain.StopTriggerFeeItemNotRegistered {
		t.Fatalf("BUY 发起停办 = %+v found=%v err=%v", stall, found, err)
	}

	revised, err := bare.WithFeeItem(saValue(t, sadomain.NewFeeItemReference, trunkFeeItem))
	if err != nil {
		t.Fatal(err)
	}
	registerTrigger(revised, saports.CatalogueRegistered)
	registerTrigger(revised, saports.CatalogueReplay)
	drainBeats(t, beat)

	requests := buyRequestsOf(t, fixture.db, "SYN-OCC-TRUNK-02")
	if len(requests) != 1 || requests[0].generation != 1 || requests[0].previous != "" ||
		!strings.HasPrefix(requests[0].basis, "PARAMREG/BUY_EVALUATION_TRIGGER/") {
		t.Fatalf("补登费用项目之后的 BUY 请求册 = %+v，want 一份带依据的首代", requests)
	}
	if versions := supplierCostVersions(t, fixture.db); len(versions) != 1 || versions[0] != "BUYSC-"+requests[0].id {
		t.Fatalf("预期成本版本 = %v，want 恰一份 BUYSC-%s", versions, requests[0].id)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.buy_evaluation_trigger WHERE occurrence_reason = 'ACTUAL_FULFILLMENT'`); n != 2 {
		t.Fatalf("触发册 ACTUAL_FULFILLMENT %d 版，want 旧版留着、新追加一版", n)
	}
	assertNothingAbandoned(t, fixture.db,
		"settlement-accounting.evaluation-parameter.registered", "settlement-accounting.evaluation-request.submitted",
		"parcel-pricing.evaluation.recorded")
}

// Covers: AT-SA-194 的 SELL 半边——包裹计费时点到来时这一户还没登记采用：SA 入账、零请求，这一拍记进 SELL 发起停办册；
// 租户补登采用，登记同笔交「评价参数已登记」，SA 按记下的包裹、时点、来源事实与计价基准时点重跑发起编排，发出首代请求
// （带依据）。参数已登记那封信重投（同一封）不出第二份请求。
func TestASellMomentStalledOnAMissingAdoptionResumesAfterTheAdoptionIsRegistered(t *testing.T) {
	fixture := newSellChainFixture(t, false)
	eventID, occurredAt := recordAdoptedNetworkIntake(t, fixture)
	drainBeats(t, fixture.beat)
	assertSellLaneProcessed(t, fixture, eventID)
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("没登记采用却落了 %d 份请求", n)
	}
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	stalls, err := sapostgres.NewEvaluationResumptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	key := sadomain.SellEvaluationRequestNaturalKey{
		Parcel: mustPS(t, sadomain.NewParcelReference, "SYN-PARCEL-01"), Moment: sadomain.BillingMomentNetworkIntakeAdopted,
	}
	stall, found, err := stalls.FindSellInitiationStall(t.Context(), tenant, key)
	if err != nil || !found || stall.Cause() != sadomain.StopSellBillingMomentNotAdopted {
		t.Fatalf("SELL 发起停办 = %+v found=%v err=%v", stall, found, err)
	}

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := sapostgres.NewOutboxEvaluationParameterRegisteredHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := sapostgres.NewSellBillingMomentAdoptions(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	register, err := saapplication.NewRegisterSellBillingMomentAdoptionHandler(adoptions, handoff, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	adoption, err := sadomain.NewSellBillingMomentAdoption(synRReceivableAccount(t).ID(), sadomain.BillingMomentNetworkIntakeAdopted,
		mustPS(t, sadomain.NewEstimatePricingScope, sellChainPricingScope))
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
			_, err := register.Register(ctx, saapplication.RegisterSellBillingMomentAdoptionCommand{Tenant: tenant, Adoption: adoption})
			return err
		})
	}
	drainBeats(t, fixture.beat)

	requests, err := sapostgres.NewSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	record, found, err := requests.FindByNaturalKey(t.Context(), tenant, key)
	if err != nil || !found {
		t.Fatalf("补登采用之后读请求：found=%v err=%v", found, err)
	}
	if record.Request.Generation() != 1 || !strings.HasPrefix(record.Request.ResumeBasis().String(), "PARAMREG/SELL_BILLING_MOMENT_ADOPTION/") ||
		record.Request.Source().String() != stall.Source().String() || !record.Request.PricingBasisAt().Equal(stall.PricingBasisAt()) {
		t.Fatalf("续办出来的请求 = 第 %d 代 依据 %q 来源 %q 基准 %s；发起停办记下的是 %q / %s（收寄于 %s）",
			record.Request.Generation(), record.Request.ResumeBasis(), record.Request.Source(), record.Request.PricingBasisAt(),
			stall.Source(), stall.PricingBasisAt(), occurredAt)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 1 {
		t.Fatalf("SELL 请求 %d 份，want 恰一份（登记重放不出第二份）", n)
	}
	if n := fixture.countOutboxOfType(t, sellEvaluationRequestType); n != 1 {
		t.Fatalf("SELL 请求信封 %d 封，want 1", n)
	}
	assertNothingAbandoned(t, fixture.db, "settlement-accounting.evaluation-parameter.registered")
}

// Covers: AT-SA-195 的 SELL 半边——评价形成了、映射不全：SA 记`待判断`连同停因「映射不全」，入账；租户补登那几个计价
// 费用代码的映射，登记同笔交「评价参数已登记」，SA 续办出第二代（回指第一代），PP 再形成一份评价，SA 以第二代派生的
// 标识形成预估客户费用。第一代仍是`待判断`、不带费用；第二代`已形成`。
func TestASellRequestPendingOnAnIncompleteMappingResumesAfterTheMappingIsRegistered(t *testing.T) {
	fixture := newSYNVerticalFixture(t)
	acceptedSellParcel(t, fixture)
	drainBeats(t, fixture.beat)
	registerDemoSellPricing(t, fixture.db)

	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := sapostgres.NewSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	requestHandoff, err := sapostgres.NewOutboxSellEvaluationRequestHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	first := sellRequestRecord(t, "SELLREQ-SYN-MAP-1", "SYN-PARCEL-01", "SYN-SCOPE-01")
	mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
		if _, err := requests.Save(ctx, first); err != nil {
			return err
		}
		return requestHandoff.HandOffSellEvaluationRequest(ctx, saports.SellEvaluationRequestIntent{Record: first})
	})
	drainBeats(t, fixture.beat)

	stored, found, err := requests.FindByID(t.Context(), first.Key)
	if err != nil || !found || stored.State != sadomain.SellRequestPendingJudgment || stored.StopCause != sadomain.StopFeeItemMappingIncomplete {
		t.Fatalf("映射没登时第一代 = %s / %s found=%v err=%v", stored.State, stored.StopCause, found, err)
	}

	evaluations, err := pppostgres.NewEvaluations(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	evaluation, found, err := evaluations.FindByRequestReference(t.Context(),
		ppValue(t, ppdomain.NewTenantID, demoTenant), ppValue(t, ppdomain.NewEvaluationRequestReference, "SELLREQ-SYN-MAP-1"))
	if err != nil || !found {
		t.Fatalf("第一代的评价 found=%v err=%v", found, err)
	}
	handoff, err := sapostgres.NewOutboxEvaluationParameterRegisteredHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	mappings, err := sapostgres.NewFeeItemMappings(fixture.db)
	if err != nil {
		t.Fatal(err)
	}
	register, err := saapplication.NewRegisterFeeItemMappingHandler(mappings, handoff, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	tenant := saValue(t, sadomain.NewTenantID, demoTenant)
	seen := map[string]bool{}
	for _, line := range evaluation.ChargeLines() {
		code := line.Code().String()
		if seen[code] {
			continue
		}
		seen[code] = true
		mapping, err := sadomain.NewFeeItemMapping(
			saValue(t, sadomain.NewPricingChargeCode, code),
			saValue(t, sadomain.NewFeeItemReference, "SYN-FEE-ITEM-FREIGHT-01"),
			saValue(t, sadomain.NewFeeItemMappingBasisReference, "SYN-CLAUSE-"+code),
		)
		if err != nil {
			t.Fatal(err)
		}
		mustWithinTX(t, fixture.transactor, t.Context(), func(ctx context.Context) error {
			_, err := register.RegisterFeeItemMapping(ctx, tenant, mapping)
			return err
		})
	}
	drainBeats(t, fixture.beat)

	successor, found, err := requests.FindSuccessor(t.Context(), first.Key)
	if err != nil || !found {
		t.Fatalf("补登映射之后没有第二代：found=%v err=%v", found, err)
	}
	if successor.Request.Generation() != 2 || successor.State != sadomain.SellRequestFormed ||
		successor.FormedCharge.String() != "SELLCC-"+successor.Key.Request.String() ||
		!strings.HasPrefix(successor.Request.ResumeBasis().String(), "PARAMREG/FEE_ITEM_MAPPING/") {
		t.Fatalf("第二代 = 第 %d 代 %s / %q 依据 %q", successor.Request.Generation(), successor.State,
			successor.FormedCharge, successor.Request.ResumeBasis())
	}
	old, _, err := requests.FindByID(t.Context(), first.Key)
	if err != nil || old.State != sadomain.SellRequestPendingJudgment || old.FormedCharge.String() != "" {
		t.Fatalf("第一代被改写：%s / %q err=%v", old.State, old.FormedCharge, err)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request WHERE parcel_id = 'SYN-PARCEL-01'`); n != 2 {
		t.Fatalf("SELL 请求 %d 份，want 两代（几行映射各发一封参数已登记，只续办出一代）", n)
	}
	assertNothingAbandoned(t, fixture.db,
		"settlement-accounting.evaluation-parameter.registered", "settlement-accounting.sell-evaluation-request.submitted",
		"parcel-pricing.evaluation.recorded")
}

// Covers: AT-PP-001 未决名单的两头——PP 请求那两路与续办几路只收真会自愈的依赖：请求还读不回、形成依赖故障、记停因或续办时某个
// 依赖答不出；答不出评价的格（未配置一类）不在任何一份名单里——它们不再是错误，不需要被翻译。名单取生产切片本身。
func TestTheResumptionLanesListOnlyDependenciesThatHealOnRetry(t *testing.T) {
	for name, lane := range map[string]struct {
		sentinels []error
		want      []error
	}{
		"BUY 请求": {evaluationRequestSubmittedUndecidedSentinels, []error{
			ppsettlement.ErrEvaluationRequestNotVisible, ppsettlement.ErrEvaluationFormationUndecided,
		}},
		"SELL 请求": {sellEvaluationRequestSubmittedUndecidedSentinels, []error{
			ppsettlement.ErrEvaluationRequestNotVisible, ppsettlement.ErrEvaluationFormationUndecided,
		}},
		"评价未形成":   {evaluationRequestUnformedUndecidedSentinels, []error{sapricing.ErrEvaluationUnformedRecordingUndecided}},
		"参数已登记续办": {evaluationResumptionUndecidedSentinels, []error{sainbox.ErrEvaluationResumptionUndecided}},
	} {
		t.Run(name, func(t *testing.T) {
			if len(lane.sentinels) == 0 {
				t.Fatal("名单是空的——依赖故障会塌成 publish_failed")
			}
			for _, sentinel := range lane.want {
				listed := false
				for _, candidate := range lane.sentinels {
					listed = listed || candidate == sentinel
				}
				if !listed {
					t.Fatalf("%v 不在名单里", sentinel)
				}
			}
		})
	}
	if len(evaluationRequestSubmittedUndecidedSentinels) != 2 || len(sellEvaluationRequestSubmittedUndecidedSentinels) != 2 {
		t.Fatalf("PP 请求两路的未决名单 = %v / %v，want 只剩看不见与形成未决两枚",
			evaluationRequestSubmittedUndecidedSentinels, sellEvaluationRequestSubmittedUndecidedSentinels)
	}
}
