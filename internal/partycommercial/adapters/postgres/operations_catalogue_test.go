package postgres_test

import (
	"context"
	"testing"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
)

// 本文件对真实 PostgreSQL 16 证主数据目录读面(ADR-0077,票 master-data-wiring/05):
// 服务产品上列版本壳且形态可缺席、跨租户不可见、空租户答空列表、limit 生效且非正拒、
// 五种策略册各答各的不串。

func newCatalogue(t *testing.T) (*adapter.OperationsCatalogue, *adapter.CommercialPublications, bentoapp.Transactor) {
	t.Helper()
	repository, transactor, db := newDeclarationFixture(t)
	catalogue, err := adapter.NewOperationsCatalogue(db)
	if err != nil {
		t.Fatalf("构造目录读面:%v", err)
	}
	return catalogue, repository, transactor
}

// Covers: 上列对象是版本壳——未登形态的版本照样在列(ADR-0050 缺席合法),已发布
// 未生效的版本也在列(目录答「有哪些版本」,不答「哪些可选」);合同版本(kind 2)
// 不进服务产品目录。
func TestServiceProductCatalogueListsVersionShellsWithOptionalForms(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	published := publishedProductVersion(t, "product-0", "v1", "digest-p0")
	withForm := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-p1")
	bare := productVersionInTenant(t, "tenant-1", "product-2", "v1", "digest-p2")
	contract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	mustSaveVersion(t, transactor, ctx, repository, published)
	mustSaveVersion(t, transactor, ctx, repository, withForm)
	mustSaveVersion(t, transactor, ctx, repository, bare)
	mustSaveVersion(t, transactor, ctx, repository, contract)
	mustSaveServiceProduct(t, transactor, ctx, repository, serviceProductOf(t, withForm))

	rows, err := catalogue.ListServiceProducts(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列服务产品:%v", err)
	}
	if len(rows) != 3 {
		t.Fatalf("上列 %d 行,want 3(合同版本不得进服务产品目录)", len(rows))
	}
	// 夹具的 published_at 同刻,次序由对象标识收尾。
	if rows[0].ObjectID != "product-0" || rows[1].ObjectID != "product-1" || rows[2].ObjectID != "product-2" {
		t.Fatalf("次序 = %q/%q/%q", rows[0].ObjectID, rows[1].ObjectID, rows[2].ObjectID)
	}

	if rows[0].Status != "PUBLISHED" {
		t.Fatalf("已发布未生效的版本 status = %q, want PUBLISHED", rows[0].Status)
	}

	formed := rows[1]
	if !formed.HasForm || formed.Form != "NETWORK_SERVICE" {
		t.Fatalf("已登形态未透出:hasForm=%v form=%q", formed.HasForm, formed.Form)
	}
	if formed.Status != "EFFECTIVE" || formed.Scope != "scope-1" || formed.VersionLabel != "v1" {
		t.Fatalf("版本壳字段变形:status=%q scope=%q version=%q", formed.Status, formed.Scope, formed.VersionLabel)
	}
	if !formed.EffectiveStartsAt.Equal(effectiveAtRow) || !formed.PublishedAt.Equal(publishedAtRow) {
		t.Fatalf("时间字段变形:startsAt=%v publishedAt=%v", formed.EffectiveStartsAt, formed.PublishedAt)
	}
	if !formed.HasEffectiveEnd {
		t.Fatal("有界区间读回成了开放结束")
	}

	if rows[2].HasForm || rows[2].Form != "" {
		t.Fatalf("未登形态的版本长出了形态:hasForm=%v form=%q", rows[2].HasForm, rows[2].Form)
	}
}

// Covers: ADR-0003——租户是最高数据隔离边界,双向不可见;从未登记过的租户答空列表
// 而不是错误(ADR-0077 Decision 四,空目录是内容)。
func TestServiceProductCatalogueIsTenantIsolated(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()

	mine := productVersionInTenant(t, "tenant-1", "product-1", "v1", "digest-mine")
	theirs := productVersionInTenant(t, "tenant-2", "product-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, mine)
	mustSaveVersion(t, transactor, ctx, repository, theirs)
	mustSaveServiceProduct(t, transactor, ctx, repository, serviceProductOf(t, theirs))

	rows, err := catalogue.ListServiceProducts(ctx, pcTenant(t, "tenant-1"), 10)
	if err != nil {
		t.Fatalf("上列本租户:%v", err)
	}
	if len(rows) != 1 || rows[0].HasForm {
		t.Fatalf("本租户看见了他租的行或形态:len=%d", len(rows))
	}

	other, err := catalogue.ListServiceProducts(ctx, pcTenant(t, "tenant-2"), 10)
	if err != nil {
		t.Fatalf("上列他租户:%v", err)
	}
	if len(other) != 1 || !other[0].HasForm {
		t.Fatalf("他租户自己的行不完整:len=%d", len(other))
	}

	empty, err := catalogue.ListServiceProducts(ctx, pcTenant(t, "tenant-9"), 10)
	if err != nil {
		t.Fatalf("空租户上列:%v", err)
	}
	if len(empty) != 0 {
		t.Fatalf("空租户答了 %d 行,want 0", len(empty))
	}
}

// Covers: ADR-0077 Decision 五——limit 生效,非正拒(静默答一页会把缺参变成没人
// 决定过的页大小)。
func TestServiceProductCatalogueAppliesTheLimitAndRejectsNonPositive(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	for _, objectID := range []string{"product-1", "product-2", "product-3"} {
		version := productVersionInTenant(t, "tenant-1", objectID, "v1", "digest-"+objectID)
		mustSaveVersion(t, transactor, ctx, repository, version)
	}

	rows, err := catalogue.ListServiceProducts(ctx, tenant, 2)
	if err != nil {
		t.Fatalf("带 limit 上列:%v", err)
	}
	if len(rows) != 2 {
		t.Fatalf("limit 2 交回 %d 行", len(rows))
	}

	if _, err := catalogue.ListServiceProducts(ctx, tenant, 0); err == nil {
		t.Fatal("limit 0 未被拒")
	}
	if _, err := catalogue.ListPricePolicies(ctx, tenant, -1); err == nil {
		t.Fatal("策略册 limit -1 未被拒")
	}
}

// Covers: 五种策略册各答各的、内容逐字段不变形、策略种类间不串;他租的同类登记不进
// 本租户的列表;从未登记过的租户五册全部答空。
func TestCommercialPolicyCatalogueKindsAnswerSeparately(t *testing.T) {
	catalogue, repository, transactor := newCatalogue(t)
	ctx := t.Context()
	tenant := pcTenant(t, "tenant-1")

	// 接单规则包正文(0014)与它的时点锚声明(0005)同挂 rules-1/v1。
	rulesOwner := effectiveDeclarationOwner(t, domain.AcceptanceRulePackageObject, "rules-1", "v1")
	mustSaveVersion(t, transactor, ctx, repository, rulesOwner)
	pack := rulePackageBodyOf(t, rulesOwner,
		pcAssembledRule(t, domain.MinimumIngressIdentityRules, "RULE/ingress-1"),
		pcAssembledRule(t, domain.ShipmentInvariantRules, "RULE/invariant-1"),
	)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveAcceptanceRulePackage(txCtx, pack)
	}, ports.DeclarationSaved)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SaveAsOfPolicies(txCtx, asOfDeclarationOf(t, rulesOwner, "asof-policy/reach-v1"))
	}, ports.DeclarationSaved)

	// 接受前财务控制声明(0007):一份`要求控制`、一份带依据的`不适用`。
	requiredContract := effectiveContract(t, "contract-1", "v1", "digest-c1")
	mustSaveVersion(t, transactor, ctx, repository, requiredContract)
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SavePreAcceptanceControl(txCtx, preAcceptanceControlOf(t, requiredContract))
	}, ports.DeclarationSaved)

	waivedContract := effectiveContract(t, "contract-2", "v1", "digest-c2")
	mustSaveVersion(t, transactor, ctx, repository, waivedContract)
	waived, err := domain.DeclarePreAcceptanceControl(waivedContract,
		domain.PreAcceptanceControlNotApplicable,
		pcValue(t, domain.NewControlNotApplicableBasis, "CONTRACT-CLAUSE/NO-CONTROL"))
	if err != nil {
		t.Fatalf("组不适用声明:%v", err)
	}
	mustSaveDeclaration(t, transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return repository.SavePreAcceptanceControl(txCtx, waived)
	}, ports.DeclarationSaved)

	// 价格政策(0010),本租一份、他租一份同名——他租那份不得进本租户的列表。
	priceVersion := effectiveVersionOfKind(t, domain.PriceRuleObject, "price-1", "v1", "digest-price-1")
	mustSaveVersion(t, transactor, ctx, repository, priceVersion)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, priceVersion, domain.SellDirection, domain.SellDirection, domain.PlanBindingConversionNone, "plan-sell-1"),
		domain.SellDirection, domain.PlanBindingConversionNone)

	theirPriceVersion := policyVersionInTenant(t, "tenant-2", domain.PriceRuleObject, "price-1", "v1", "digest-theirs")
	mustSaveVersion(t, transactor, ctx, repository, theirPriceVersion)
	mustSavePricePolicy(t, transactor, ctx, repository,
		pricePolicyOn(t, theirPriceVersion, domain.BuyDirection, domain.BuyDirection, domain.PlanBindingConversionNone, "plan-buy-9"),
		domain.BuyDirection, domain.PlanBindingConversionNone)

	// 结算政策(0011)。
	settleVersion := effectiveVersionOfKind(t, domain.SettlementPolicyObject, "settle-1", "v1", "digest-settle-1")
	mustSaveVersion(t, transactor, ctx, repository, settleVersion)
	mustSaveSettlementPolicy(t, transactor, ctx, repository,
		settlementPolicyOn(t, settleVersion, domain.TermsMethod, "charge-express"))

	packages, err := catalogue.ListAcceptanceRulePackages(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列规则包:%v", err)
	}
	if len(packages) != 1 {
		t.Fatalf("规则包 %d 行,want 1(种类间不得串)", len(packages))
	}
	ruled := packages[0]
	if ruled.ObjectID != "rules-1" || ruled.ServiceProduct != "product-1" ||
		ruled.Contract != "contract-1" || ruled.LegalEntity != "legal-1" || ruled.Scope != "scope-1" {
		t.Fatalf("五维变形:%+v", ruled)
	}
	if !ruled.HasEffectiveEnd || !ruled.EffectiveStartsAt.Equal(effectiveAtRow) {
		t.Fatalf("适用期间变形:%+v", ruled)
	}
	if len(ruled.Rules) != 2 ||
		ruled.Rules[0] != (ports.AssembledRuleRow{Category: "MINIMUM_INGRESS_IDENTITY", Reference: "RULE/ingress-1"}) ||
		ruled.Rules[1] != (ports.AssembledRuleRow{Category: "SHIPMENT_INVARIANT", Reference: "RULE/invariant-1"}) {
		t.Fatalf("规则引用变形:%+v", ruled.Rules)
	}

	controls, err := catalogue.ListPreAcceptanceControls(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列控制声明:%v", err)
	}
	if len(controls) != 2 {
		t.Fatalf("控制声明 %d 行,want 2", len(controls))
	}
	byContract := map[string]ports.PreAcceptanceControlRow{}
	for _, row := range controls {
		byContract[row.ContractObjectID] = row
	}
	if row := byContract["contract-1"]; row.Requirement != "REQUIRED" || row.NotApplicableBasis != "" {
		t.Fatalf("要求控制的声明变形:%+v", row)
	}
	if row := byContract["contract-2"]; row.Requirement != "NOT_APPLICABLE" ||
		row.NotApplicableBasis != "CONTRACT-CLAUSE/NO-CONTROL" {
		t.Fatalf("不适用声明变形:%+v", row)
	}

	prices, err := catalogue.ListPricePolicies(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列价格政策:%v", err)
	}
	if len(prices) != 1 {
		t.Fatalf("价格政策 %d 行,want 1(他租的登记进了本租户)", len(prices))
	}
	price := prices[0]
	if price.ObjectID != "price-1" || price.Direction != "SELL" || price.PlanRef != "plan-sell-1" ||
		price.PlanDirection != "SELL" || price.BindingConversion != "NONE" || price.PolicyScope != "scope-1" {
		t.Fatalf("价格政策变形:%+v", price)
	}

	theirPrices, err := catalogue.ListPricePolicies(ctx, pcTenant(t, "tenant-2"), 10)
	if err != nil {
		t.Fatalf("上列他租价格政策:%v", err)
	}
	if len(theirPrices) != 1 || theirPrices[0].Direction != "BUY" {
		t.Fatalf("他租自己的列表不完整:%+v", theirPrices)
	}

	settlements, err := catalogue.ListSettlementPolicies(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列结算政策:%v", err)
	}
	if len(settlements) != 1 {
		t.Fatalf("结算政策 %d 行,want 1", len(settlements))
	}
	settlement := settlements[0]
	if settlement.ObjectID != "settle-1" || settlement.Method != "TERMS" ||
		settlement.LegalEntity != "legal-1" || settlement.Counterparty != "customer-1" ||
		settlement.ContractLabel != "contract-1/v1" || settlement.ChargeScope != "charge-express" ||
		settlement.Currency != "SYN" {
		t.Fatalf("结算政策变形:%+v", settlement)
	}

	asOf, err := catalogue.ListAsOfPolicyDeclarations(ctx, tenant, 10)
	if err != nil {
		t.Fatalf("上列时点锚声明:%v", err)
	}
	if len(asOf) != 2 {
		t.Fatalf("时点锚 %d 行,want 2", len(asOf))
	}
	byJudgment := map[string]ports.AsOfPolicyRow{}
	for _, row := range asOf {
		if row.RulePackageObjectID != "rules-1" || row.RulePackageVersion != "v1" {
			t.Fatalf("时点锚挂错了规则包:%+v", row)
		}
		byJudgment[row.JudgmentType] = row
	}
	if row := byJudgment["NETWORK_REACHABILITY"]; row.SemanticsRef != "AT_ACCEPTANCE" ||
		row.PolicyVersion != "asof-policy/reach-v1" {
		t.Fatalf("可达性时点锚变形:%+v", row)
	}
	if row := byJudgment["PRE_ACCEPTANCE_FINANCIAL_CONTROL"]; row.SemanticsRef != "AT_SUBMISSION" ||
		row.PolicyVersion != "asof-policy/ctrl-v1" {
		t.Fatalf("财务控制时点锚变形:%+v", row)
	}

	// 从未登记过的租户:五册全部如实答空。
	emptyTenant := pcTenant(t, "tenant-9")
	if rows, err := catalogue.ListAcceptanceRulePackages(ctx, emptyTenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("空租户规则包:len=%d err=%v", len(rows), err)
	}
	if rows, err := catalogue.ListPreAcceptanceControls(ctx, emptyTenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("空租户控制声明:len=%d err=%v", len(rows), err)
	}
	if rows, err := catalogue.ListPricePolicies(ctx, emptyTenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("空租户价格政策:len=%d err=%v", len(rows), err)
	}
	if rows, err := catalogue.ListSettlementPolicies(ctx, emptyTenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("空租户结算政策:len=%d err=%v", len(rows), err)
	}
	if rows, err := catalogue.ListAsOfPolicyDeclarations(ctx, emptyTenant, 10); err != nil || len(rows) != 0 {
		t.Fatalf("空租户时点锚:len=%d err=%v", len(rows), err)
	}
}
