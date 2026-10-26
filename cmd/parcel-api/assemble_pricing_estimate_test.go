package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	pricinghttp "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/http"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/buildinfo"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件钉 `/pricing-estimates` 换上操作者渠道之后装配点那一半（ADR-0152 决定六；票 operator-channel/06）。它的能力面是主数据
// 与运营查阅读，所以不并进 swappedRegistryFaces：TestSwappedRegistryFacesMapTheRefusalAndDependencyGrades 里「只持查阅授予」
// 答未授予的那一格，在这一口恰是授予齐备。

// estimateDeclaration 与 syntheticPriceCardRegistration 那张卡对得上：同范围、同方向、适用期内，分区与重量落在它的费率段里。
const estimateDeclaration = `{"scope":"SYN-PRC-REG-SCOPE","direction":"SELL","basisAt":"2026-08-07T10:00:00Z","weight":{"value":"5","unit":"KG"},"zone":"Z1"}`

// Covers: ADR-0152 决定六——试算口的各格：未带令牌 401，只持登记册配置写 403，操作者册读不动 503，持查阅读而载荷自报租户
// 400；声明立得住时越过 Intake 到编排（未接线占位答 NO_ANSWER_FORMED）。发行方未配置仍答 403 未配置，带不带令牌都一样。
func TestThePricingEstimateAnswersFromTheOperatorReadChannel(t *testing.T) {
	cases := map[string]struct {
		register accessidentity.OperatorRegistry
		token    string
		body     string
		status   int
		code     string
	}{
		"no bearer token":               {readerRegister{}, "", estimateDeclaration, http.StatusUnauthorized, "OPERATOR_CREDENTIAL_REJECTED"},
		"registry write grant only":     {registryWriterRegister{}, "presented.operator.token", estimateDeclaration, http.StatusForbidden, "OPERATOR_NOT_GRANTED"},
		"register is down":              {failingRegister{}, "presented.operator.token", estimateDeclaration, http.StatusServiceUnavailable, "IDENTITY_DEPENDENCY_UNAVAILABLE"},
		"granted, tenant self-reported": {readerRegister{}, "presented.operator.token", `{"tenantId": "SYN-TENANT-02"}`, http.StatusBadRequest, "MALFORMED_REQUEST"},
		"granted":                       {readerRegister{}, "presented.operator.token", estimateDeclaration, http.StatusInternalServerError, "NO_ANSWER_FORMED"},
	}
	for name, testCase := range cases {
		response := serveThroughOperatorRegistries(t, testCase.register, http.MethodPost, "/pricing-estimates", testCase.token, testCase.body)
		if response.Code != testCase.status || problemCode(t, response) != testCase.code {
			t.Fatalf("%s: answer = %d %q, want %d %q", name, response.Code, problemCode(t, response), testCase.status, testCase.code)
		}
		assertNoOutcome(t, response, name)
	}

	router := httpapi.NewWithEndpoints(buildinfo.Info{}, assembleUnconfiguredBusinessEndpoints())
	for _, token := range []string{"", "presented.operator.token"} {
		request := httptest.NewRequest(http.MethodPost, "/pricing-estimates", strings.NewReader(estimateDeclaration))
		if token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
		response := httptest.NewRecorder()
		router.ServeHTTP(response, request)
		if response.Code != http.StatusForbidden || problemCode(t, response) != "ACCESS_CHANNEL_NOT_CONFIGURED" {
			t.Fatalf("issuer unconfigured, token %q: answer = %d %q", token, response.Code, problemCode(t, response))
		}
	}
}

// tenantReader 是操作者册的替身：主体绑在 tenant 上，持有主数据与运营查阅读的授予。readerRegister 同形，只是钉死在
// SYN-TENANT-01 上。
type tenantReader struct{ tenant string }

func (register tenantReader) FindOperator(_ context.Context, subject accessidentity.OperatorSubject) (accessidentity.OperatorStanding, bool, error) {
	binding, err := accessidentity.NewOperatorBinding(subject, register.tenant, "SYN-BASIS-BINDING")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	interval, err := accessidentity.NewEffectiveInterval(time.Now().Add(-time.Hour), time.Time{})
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	grant, err := accessidentity.NewOperatorGrant(register.tenant, "SYN-GRANT-READ", subject, accessidentity.CapabilityMasterDataAndOperationsRead, interval, "SYN-BASIS-GRANT")
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	recorded, err := accessidentity.NewRecordedGrant(grant, nil)
	if err != nil {
		return accessidentity.OperatorStanding{}, false, err
	}
	standing, err := accessidentity.NewOperatorStanding(binding, []accessidentity.RecordedGrant{recorded})
	return standing, err == nil, err
}

// Covers: ADR-0152 决定六与票 operator-channel/06——真铸造、PP 查阅读认证方、操作者 Intake 与真库价卡册上的试算编排连起来：
// 操作者认证到哪个租户，就按哪个租户的价卡试算。绑夹具价卡所在租户的操作者得到那张卡的一份 `S` 试算评价；绑另一个租户
// （readerRegister）的操作者递同一份声明，答价卡未配置——声明里没有租户的位置。测试输入是隔离合成，只记 `S`。
func TestTheOperatorReadChannelEstimatesForTheEnvelopeTenantAgainstARealDatabase(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	cards, err := buildPriceCardRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配价卡登记编排：%v", err)
	}
	registration := syntheticPriceCardRegistration(t)
	if outcome, err := cards.Handle(t.Context(), pricingapp.RegisterPriceCardCommand{Registration: registration}); err != nil || outcome != pricingapp.PriceCardRecorded {
		t.Fatalf("价卡首登 outcome = %s err = %v", outcome, err)
	}
	estimate, err := buildPricingEstimateOrchestration(db)
	if err != nil {
		t.Fatalf("装配试算编排：%v", err)
	}
	post := func(register accessidentity.OperatorRegistry) *httptest.ResponseRecorder {
		t.Helper()
		minter, err := buildOperatorMinter(acceptingVerifier{}, register, admissionScopeUnset())
		if err != nil {
			t.Fatal(err)
		}
		registries, err := buildOperatorRegistryIntakes(minter)
		if err != nil {
			t.Fatal(err)
		}
		request := httptest.NewRequest(http.MethodPost, "/pricing-estimates", strings.NewReader(estimateDeclaration))
		request.Header.Set("Authorization", "Bearer presented.operator.token")
		response := httptest.NewRecorder()
		pricinghttp.NewEstimateEndpoint(registries.pricingOperationsRead, estimate).ServeHTTP(response, request)
		return response
	}

	var formed struct {
		Outcome    string `json:"outcome"`
		Candidates []struct {
			Plan       struct{ ID, Version string } `json:"plan"`
			Answer     string                       `json:"answer"`
			Evaluation *struct {
				Status   string                             `json:"status"`
				Evidence string                             `json:"evidence"`
				Total    *struct{ Amount, Currency string } `json:"total"`
			} `json:"evaluation"`
		} `json:"candidates"`
	}
	response := post(tenantReader{tenant: registration.Tenant().String()})
	if err := json.Unmarshal(response.Body.Bytes(), &formed); err != nil || response.Code != http.StatusOK ||
		formed.Outcome != "FORMED" || len(formed.Candidates) != 1 {
		t.Fatalf("绑价卡所在租户的操作者 = %d %s", response.Code, response.Body.Bytes())
	}
	candidate := formed.Candidates[0]
	if candidate.Answer != "EVALUATED" || candidate.Plan.ID != "SYN-PRC-REG-PLAN" || candidate.Plan.Version != "v1" ||
		candidate.Evaluation == nil || candidate.Evaluation.Status != "COMPLETED" || candidate.Evaluation.Evidence != "S" ||
		candidate.Evaluation.Total == nil || candidate.Evaluation.Total.Amount != "10" || candidate.Evaluation.Total.Currency != "USD" {
		t.Fatalf("试算那一格 = %s", response.Body.Bytes())
	}

	if other := post(readerRegister{}); other.Code != http.StatusOK || answeredOutcome(t, other) != "PRICE_CARD_NOT_CONFIGURED" {
		t.Fatalf("绑另一个租户的操作者 = %d %s", other.Code, other.Body.Bytes())
	}
}

// 「认价卡」这一拍的合成身份：同一租户、同一范围，PC 的销售价格政策与 PP 的售价卡、汇率序列都登在这里。
const (
	cardTermsTenant = "SYN-TENANT-API-CARD-TERMS"
	cardTermsScope  = "SYN-API-CARD-TERMS-SCOPE"
	cardTermsPolicy = "SYN-API-CARD-TERMS-RULE"
	cardTermsPlan   = "SYN-API-CARD-TERMS-PLAN"
	cardTermsSeries = "SYN-API-CARD-TERMS-FX"
)

// Covers: AT-PP-013（完整版）——同库、同租户、同范围上并存两份计价条款：PC 经运营操作者面发布编排登了一份销售价格政策，
// 口径的汇率格选「提交接收」时点形态，口径行上另有一个 0041 迁移之前登下的历史体积系数 6000；PP 登了一张在用 SELL 卡，
// 体积系数除数 5000、合计 HALF_UP 到 0.01、绑一串汇率序列（口径引用指名那一版政策，两期各一个牌价）。按生产装配的试算
// 编排声明一件带尺寸的包裹：体积重按卡上的 5000 得 4.8 KG（按政策上的 6000 会是 4 KG，落进另一档），计费重 5 KG 落 20 CNY
// 那一档；期次按计价基准时点取后一期的 0.18765，不按政策上的取值时点；合计按卡上的策略取整成 3.75 SGD；版本清单里只有卡
// 与卡绑定的序列，没有商业政策或商业授权的引用（ADR-0260 决定一至三）。评价读不到政策，结构上由
// TestParcelPricingHasNoPartyCommercialAdapter 守。测试输入是隔离合成，只记 `S`。
func TestAnEstimateReadsOnlyTheCardWhileASellPolicyDeclaresOtherwiseAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()

	publishCardTermsSellPolicy(t, db)
	declareHistoricalVolumetricFactor(t, pool, "6000")
	registerCardTermsPricing(t, db)

	estimate, err := buildPricingEstimateOrchestration(db)
	if err != nil {
		t.Fatalf("装配试算编排：%v", err)
	}
	weight, err := pricingdomain.NewWeightFromString("1", pricingdomain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("重量：%v", err)
	}
	sides, err := pricingdomain.NewDimensions(mustValue(t, pricingdomain.ParseDecimal, "40"),
		mustValue(t, pricingdomain.ParseDecimal, "30"), mustValue(t, pricingdomain.ParseDecimal, "20"), pricingdomain.LengthUnitCentimeter)
	if err != nil {
		t.Fatalf("尺寸：%v", err)
	}
	settlement := mustValue(t, pricingdomain.NewCurrency, "SGD")
	basisAt := time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)
	result, err := estimate.Handle(ctx, pricingapp.FormEstimateEvaluationsCommand{
		Tenant:     mustValue(t, pricingdomain.NewTenantID, cardTermsTenant),
		Scope:      mustValue(t, pricingdomain.NewPricingScopeID, cardTermsScope),
		Direction:  pricingdomain.PricingDirectionSell,
		BasisAt:    basisAt,
		Weight:     weight,
		Dimensions: &sides,
		Zone:       "Z1",
		Settlement: &settlement,
	})
	if err != nil || result.Outcome != pricingapp.EstimateEvaluationsFormed || len(result.Candidates) != 1 {
		t.Fatalf("试算 = %+v err = %v", result, err)
	}
	evaluation := result.Candidates[0].Evaluation
	if evaluation.Status() != pricingdomain.EvaluationCompleted {
		t.Fatalf("status = %s issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	pricingWeight, ok := evaluation.PricingWeight()
	volumetric, derived := pricingWeight.VolumetricWeight()
	if !ok || !derived || volumetric.Value().String() != "4.8" {
		t.Fatalf("体积重 = %s derived=%v，want 卡上除数 5000 得的 4.8 KG（政策上的 6000 会是 4）", volumetric.Value(), derived)
	}
	if rate, matched := evaluation.MatchedRate(); !matched || rate.Amount().Amount().String() != "20" {
		t.Fatalf("命中费率 = %#v，want 计费重 5 KG 落的 20 CNY 那一档", rate)
	}
	step, converted := evaluation.ConversionStep()
	if !converted || step.SeriesReference().ID() != cardTermsSeries || step.Rate().String() != "0.18765" {
		t.Fatalf("换算 = %#v，want 卡绑定序列按计价基准时点取的后一期 0.18765", step)
	}
	if total, _ := evaluation.Total(); total.Currency().String() != "SGD" || total.Amount().String() != "3.75" {
		t.Fatalf("合计 = %s %s，want 3.75 SGD（3.753 按卡上的 HALF_UP 到 0.01）", total.Amount(), total.Currency())
	}
	cited := map[pricingdomain.ArtifactKind]string{}
	for _, reference := range evaluation.Manifest().References() {
		switch reference.Kind() {
		case pricingdomain.ArtifactCommercialPolicy, pricingdomain.ArtifactCommercialAuthorization:
			t.Fatalf("版本清单引了 %s %s——评价不引用商业价格政策或已解析商业依据", reference.Kind(), reference.ID())
		}
		cited[reference.Kind()] = reference.ID() + "@" + reference.Version()
	}
	if cited[pricingdomain.ArtifactPricingPlan] != cardTermsPlan+"@v1" || cited[pricingdomain.ArtifactReferenceSeries] != cardTermsSeries+"@v1" {
		t.Fatalf("版本清单 = %v，want 卡与卡绑定序列的在用版本", cited)
	}
}

// publishCardTermsSellPolicy 经运营操作者面的发布编排（录入 → 批准 → 发布）登一份销售价格政策：绑那张售价卡，口径未税带
// 分类，汇率格选随产品发布的「提交接收」时点形态。审批职责规则由写口直接登一条，判据同 TestTheWiredPublicationDraftPathLandsAgainstARealDatabase。
func publishCardTermsSellPolicy(t *testing.T, db *bentopg.DB) {
	t.Helper()
	ctx := t.Context()
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	tenant := pcSynthetic(t, commercialdomain.NewTenantID, cardTermsTenant)
	rules, err := pcpostgres.NewApprovalDutyRules(db)
	if err != nil {
		t.Fatalf("规则册：%v", err)
	}
	rule, err := commercialdomain.NewApprovalDutyRule(tenant, true, commercialdomain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := rules.SaveApprovalDutyRule(txCtx, rule)
		return err
	}); err != nil {
		t.Fatalf("登记审批职责规则：%v", err)
	}

	interval, err := commercialdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	tax, err := commercialdomain.NewTaxCaliber(commercialdomain.TaxExclusive,
		pcSynthetic(t, commercialdomain.NewTaxClassificationReference, "SYN-TAX-VAT-STANDARD"))
	if err != nil {
		t.Fatalf("税务口径：%v", err)
	}
	fx, err := commercialdomain.NewFxCaliber(
		pcSynthetic(t, commercialdomain.NewFxQuoteTypeReference, "SYN-FX-QUOTE-MID"),
		pcSynthetic(t, commercialdomain.NewAsOfSemanticsReference, "REFCFG-1:parcel-shipment/as-of-semantics/submission-receipt@1"),
		pcSynthetic(t, commercialdomain.NewAsOfPolicyVersion, "SYN-ASOF-POLICY-01"))
	if err != nil {
		t.Fatalf("汇率口径：%v", err)
	}
	shell := commercialdomain.PublicationDraftShell{
		TenantID:  tenant,
		Kind:      commercialdomain.PriceRuleObject,
		ObjectID:  pcSynthetic(t, commercialdomain.NewCommercialObjectID, cardTermsPolicy),
		Version:   pcSynthetic(t, commercialdomain.NewCommercialVersionLabel, "v1"),
		Scope:     pcSynthetic(t, commercialdomain.NewCommercialScopeReference, cardTermsScope),
		Effective: interval,
	}
	content := commercialdomain.PublicationContent{Kind: commercialdomain.PriceRuleObject, PricePolicy: &commercialdomain.PricePolicyBody{
		Direction:     commercialdomain.SellDirection,
		PricingPlan:   pcSynthetic(t, commercialdomain.NewPricingPlanReference, cardTermsPlan+"@v1"),
		PlanDirection: commercialdomain.SellDirection,
		Conversion:    commercialdomain.PlanBindingConversionNone,
		Scope:         pcSynthetic(t, commercialdomain.NewCommercialScopeReference, cardTermsScope),
		Effective:     interval,
		Caliber:       &commercialdomain.PricePolicyCaliberBody{Tax: tax, Fx: &fx},
	}}
	submitted, err := registration.publicationDrafts.Submit(ctx, commercialapp.SubmitPublicationDraftCommand{
		Shell: shell, Content: content, Submitter: pcSynthetic(t, commercialdomain.NewOperatorSubjectReference, "SYN-OP-SUBMITTER"),
	})
	if err != nil || submitted.Outcome() != commercialapp.PublicationDraftSubmitted {
		t.Fatalf("录入：outcome = %s, err = %v", submitted.Outcome(), err)
	}
	approver, err := commercialdomain.NewOperatorSubject(pcSynthetic(t, commercialdomain.NewOperatorSubjectReference, "SYN-OP-APPROVER"), nil)
	if err != nil {
		t.Fatalf("批准者：%v", err)
	}
	if approved, err := registration.publicationDrafts.Approve(ctx, commercialapp.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version, Approver: approver,
	}); err != nil || approved.Outcome() != commercialapp.PublicationDraftApproved {
		t.Fatalf("批准：outcome = %s, err = %v", approved.Outcome(), err)
	}
	published, err := registration.publicationDrafts.Publish(ctx, commercialapp.PublishPublicationDraftCommand{
		Tenant: tenant, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
	})
	if err != nil || published.Outcome() != commercialapp.PublicationDraftPublished {
		t.Fatalf("发布：outcome = %s, err = %v", published.Outcome(), err)
	}
	if publication, _ := published.Publication(); publication.Outcome() != commercialapp.CommercialVersionPublishedEffective {
		t.Fatalf("受控发布 outcome = %s, want PUBLISHED_EFFECTIVE", publication.Outcome())
	}
}

// declareHistoricalVolumetricFactor 给那份政策的口径行落一个 0041 迁移之前才登得进的体积系数：撤 0041 的约束、改行、按 0022
// 原文加回方向耦合（表回到 0041 之前的样子，那一行在当年的约束下合法），再原样施加嵌入的 0041 正文——手法同
// TestMigration0041KeepsHistoricalFactorsListedAndRefusesNewOnes。干净库上造不出这样的行，只能在测试里退表。
func declareHistoricalVolumetricFactor(t *testing.T, pool *pgxpool.Pool, factor string) {
	t.Helper()
	ctx := t.Context()
	exec := func(step, statement string, arguments ...any) {
		t.Helper()
		if _, err := pool.Exec(ctx, statement, arguments...); err != nil {
			t.Fatalf("%s：%v", step, err)
		}
	}
	exec("撤 0041 的约束", `ALTER TABLE party_commercial.price_policy_caliber
		DROP CONSTRAINT price_policy_caliber_volumetric_factor_retired`)
	exec("落历史体积系数", `UPDATE party_commercial.price_policy_caliber SET volumetric_factor_ref = $1
		WHERE tenant_id = $2 AND object_id = $3 AND version_label = 'v1'`, factor, cardTermsTenant, cardTermsPolicy)
	exec("按 0022 原文加回方向耦合", `ALTER TABLE party_commercial.price_policy_caliber
		ADD CONSTRAINT price_policy_caliber_volumetric_factor_coupled
		CHECK ((direction = 'SELL') = (volumetric_factor_ref IS NOT NULL))`)
	assets, err := migrations.PartyCommercial()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	applied := false
	for _, asset := range assets {
		if asset.Name == "0041_price_policy_caliber_volumetric_factor_retired" {
			exec("施加 "+asset.Name, asset.SQL)
			applied = true
		}
	}
	if !applied {
		t.Fatal("没有找到迁移 0041")
	}
	var stored string
	if err := pool.QueryRow(ctx, `SELECT volumetric_factor_ref FROM party_commercial.price_policy_caliber
		WHERE tenant_id = $1 AND object_id = $2`, cardTermsTenant, cardTermsPolicy).Scan(&stored); err != nil || stored != factor {
		t.Fatalf("政策口径行上的历史系数 = %q, err = %v, want %s", stored, err, factor)
	}
}

// registerCardTermsPricing 经生产装配的登记编排登那张售价卡与它绑的汇率序列，并复核序列使之在用。
//
// 卡：Z1 两档（0–4.5 KG 10 CNY、4.5–10 KG 20 CNY），MAX 计费重，体积系数除数 5000（立方厘米 → 千克，体积重进位 0.1 KG），
// 计费重进位 0.5 KG，合计 HALF_UP 到 0.01，绑汇率序列。序列：两期（2026 上半年 0.19、此后 0.18765），口径引用指名那一版
// 销售价格政策。
func registerCardTermsPricing(t *testing.T, db *bentopg.DB) {
	t.Helper()
	ctx := t.Context()
	cny := mustValue(t, pricingdomain.NewCurrency, "CNY")
	kg := func(raw string) pricingdomain.Weight {
		weight, err := pricingdomain.NewWeightFromString(raw, pricingdomain.WeightUnitKilogram)
		if err != nil {
			t.Fatalf("重量 %s：%v", raw, err)
		}
		return weight
	}
	money := func(raw string) pricingdomain.Money {
		amount, err := pricingdomain.NewMoneyFromString(raw, cny)
		if err != nil {
			t.Fatalf("金额 %s：%v", raw, err)
		}
		return amount
	}
	period, err := pricingdomain.NewEffectivePeriod(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("适用期：%v", err)
	}
	light, err := pricingdomain.NewRateEntry(mustValue(t, pricingdomain.NewRateEntryID, "SYN-API-CARD-TERMS-LIGHT"), "Z1", kg("0"), kg("4.5"), money("10"))
	if err != nil {
		t.Fatalf("费率段：%v", err)
	}
	heavy, err := pricingdomain.NewRateEntry(mustValue(t, pricingdomain.NewRateEntryID, "SYN-API-CARD-TERMS-HEAVY"), "Z1", kg("4.5"), kg("10"), money("20"))
	if err != nil {
		t.Fatalf("费率段：%v", err)
	}
	table, err := pricingdomain.NewRateTableVersion(
		syntheticPricingReference(t, pricingdomain.ArtifactRateTable, "SYN-API-CARD-TERMS-TABLE", "v1"),
		pricingdomain.RateTableFamilyWeightZone, cny, pricingdomain.WeightUnitKilogram, period,
		[]pricingdomain.RateEntry{light, heavy})
	if err != nil {
		t.Fatalf("价表：%v", err)
	}
	volumetricRounding, err := pricingdomain.NewWeightRoundingPolicy(pricingdomain.RoundingCeiling, kg("0.1"))
	if err != nil {
		t.Fatalf("体积重进位：%v", err)
	}
	factor, err := pricingdomain.NewVolumetricFactor(mustValue(t, pricingdomain.ParseDecimal, "5000"), pricingdomain.LengthUnitCentimeter, volumetricRounding)
	if err != nil {
		t.Fatalf("体积系数：%v", err)
	}
	weightRounding, err := pricingdomain.NewWeightRoundingPolicy(pricingdomain.RoundingCeiling, kg("0.5"))
	if err != nil {
		t.Fatalf("计费重进位：%v", err)
	}
	weightPolicy, err := pricingdomain.NewPricingWeightPolicy(
		syntheticPricingReference(t, pricingdomain.ArtifactWeightPolicy, "SYN-API-CARD-TERMS-WEIGHT", "v1"),
		pricingdomain.PricingWeightMax, weightRounding, &factor)
	if err != nil {
		t.Fatalf("计价重策略：%v", err)
	}
	binding, err := pricingdomain.NewReferenceSeriesBinding(pricingdomain.ReferenceSeriesExchangeRate, cardTermsSeries)
	if err != nil {
		t.Fatalf("序列绑定：%v", err)
	}
	structures, err := pricingdomain.NewPricingPlanStructures(nil, nil, []pricingdomain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("方案结构：%v", err)
	}
	rounding, err := pricingdomain.NewAmountRoundingPolicy(pricingdomain.RoundingHalfUp, money("0.01"),
		[]pricingdomain.AmountRoundingPoint{pricingdomain.AmountRoundingTotal})
	if err != nil {
		t.Fatalf("金额取整：%v", err)
	}
	if structures, err = structures.WithAmountRounding(rounding); err != nil {
		t.Fatalf("挂金额取整：%v", err)
	}
	plan, err := pricingdomain.NewPricingPlanVersion(
		syntheticPricingReference(t, pricingdomain.ArtifactPricingPlan, cardTermsPlan, "v1"),
		mustValue(t, pricingdomain.NewPricingScopeID, cardTermsScope),
		pricingdomain.PricingDirectionSell, pricingdomain.PricingPurposeCustomerCharge,
		mustValue(t, pricingdomain.NewChargeCode, "BASE_FREIGHT"),
		period, table, weightPolicy, nil, structures,
	)
	if err != nil {
		t.Fatalf("价卡：%v", err)
	}
	source, err := pricingdomain.NewSourceFileIdentity("SYN-API-CARD-TERMS-CARD.xlsx",
		"0b1e4f5c6a7d8e9f00112233445566778899aabbccddeeff0011223344556677")
	if err != nil {
		t.Fatalf("源文件身份：%v", err)
	}
	card, err := pricingdomain.NewPriceCardRegistration(
		mustValue(t, pricingdomain.NewTenantID, cardTermsTenant), plan, source,
		syntheticPricingReference(t, pricingdomain.ArtifactCommercialAuthorization, "SYN-API-CARD-TERMS-GRANT", "v1"),
		"SYN-PRC-GOVERNANCE")
	if err != nil {
		t.Fatalf("价卡登记：%v", err)
	}
	cards, err := buildPriceCardRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配价卡登记编排：%v", err)
	}
	if outcome, err := cards.Handle(ctx, pricingapp.RegisterPriceCardCommand{Registration: card}); err != nil || outcome != pricingapp.PriceCardRecorded {
		t.Fatalf("价卡首登 outcome = %s err = %v", outcome, err)
	}

	periods := make([]pricingdomain.SeriesPeriodValue, 0, 2)
	for _, value := range []struct {
		startsAt, endsAt time.Time
		rate, evidence   string
	}{
		{time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), "0.19", "SYN-EVID-CARD-TERMS-2601"},
		{time.Date(2026, 7, 1, 0, 0, 0, 0, time.UTC), time.Time{}, "0.18765", "SYN-EVID-CARD-TERMS-2607"},
	} {
		period, err := pricingdomain.NewSeriesPeriodValue(value.startsAt, value.endsAt,
			mustValue(t, pricingdomain.ParseDecimal, value.rate), value.evidence)
		if err != nil {
			t.Fatalf("期次：%v", err)
		}
		periods = append(periods, period)
	}
	series, err := pricingdomain.NewReferenceSeriesRegistration(pricingdomain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, pricingdomain.NewTenantID, cardTermsTenant),
		Kind:             pricingdomain.ReferenceSeriesExchangeRate,
		Reference:        syntheticPricingReference(t, pricingdomain.ArtifactReferenceSeries, cardTermsSeries, "v1"),
		SourceIdentifier: "SYN-TREASURY-FX-DESK",
		Registrant:       "SYN-PRICING-OPS-01",
		QuoteBasis:       syntheticPricingReference(t, pricingdomain.ArtifactCommercialPolicy, cardTermsPolicy, "v1"),
		Periods:          periods,
	})
	if err != nil {
		t.Fatalf("汇率序列登记：%v", err)
	}
	register, err := buildReferenceSeriesRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配序列登记编排：%v", err)
	}
	if outcome, err := register.Handle(ctx, pricingapp.RegisterReferenceSeriesCommand{Registration: series}); err != nil || outcome != pricingapp.ReferenceSeriesRecorded {
		t.Fatalf("序列首登 outcome = %s err = %v", outcome, err)
	}
	// 人工复核按租户的参考复核职责规则判（ADR-0262 决定一），没登即未配置：登一条要求不同主体的，与演示租户同形。
	registerReferenceReviewDutyRule(t, db, cardTermsTenant)
	review, err := buildReferenceSeriesReviewOrchestration(db)
	if err != nil {
		t.Fatalf("装配序列复核编排：%v", err)
	}
	if outcome, err := review.Handle(ctx, pricingapp.ReviewReferenceSeriesCommand{
		Tenant:        mustValue(t, pricingdomain.NewTenantID, cardTermsTenant),
		SeriesID:      cardTermsSeries,
		SeriesVersion: "v1",
		Reviewer:      "SYN-PRICING-REVIEW-01",
		Decision:      pricingdomain.SeriesReviewApproved,
		Basis:         "SYN-REVIEW/合成序列逐期对照（S 级）",
		ReviewedAt:    time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC),
	}); err != nil || outcome != pricingapp.SeriesReviewRecorded {
		t.Fatalf("序列复核 outcome = %s err = %v", outcome, err)
	}
}
