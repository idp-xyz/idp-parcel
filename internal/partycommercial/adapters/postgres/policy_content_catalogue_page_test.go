package postgres_test

import (
	"context"
	"strings"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证商业策略目录里以正文为行的各册按 ADR-0144 分页（票 catalogue-read-pagination-rollout/11）：
// 接单规则包、接受前财务控制声明、价格政策、结算政策、信用政策与时点锚声明。翻页中途登记新正文不重不漏、空册答空页；筛选按
// 领域原词比，q 覆盖各册声明的那几格，% 按字面。以版本壳为行的那几种 kind 在 version_shell_catalogue_page_test.go。夹具租户与
// 范围沿用本包的 tenant-1 / scope-1，其余取值一律 SYN-。

// contentCatalogueCase 是正文一册在本文件里的样子。行抹成「拥有对象/版本」一格（时点锚声明再接判断类型），各册同一张表断言。
// row 登一行最简的正文，连同它挂靠的已生效版本；seed 登筛选与 q 用例的那几行，cases 按读面次序列出期望。夹具的对象号随登记
// 次序升：登记或声明时刻是库里的 now()，万一同值，按行标识同向收尾也不改次序。
type contentCatalogueCase struct {
	name   string
	prefix string
	pages  *cataloguepage.Catalogue
	list   pageLister[string]
	row    func(t *testing.T, fixture cutFixture, objectID string)
	seed   func(t *testing.T, fixture cutFixture)
	cases  []pageCase
}

func contentCatalogueCases(t *testing.T, catalogue *adapter.OperationsCatalogue, tenant domain.TenantID) []contentCatalogueCase {
	t.Helper()
	ctx := t.Context()
	return []contentCatalogueCase{
		{
			name: "接单规则包", prefix: "SYN-RULES-PAGE-", pages: ports.AcceptanceRulePackagePages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.AcceptanceRulePackageRow], error) {
				return catalogue.ListAcceptanceRulePackages(ctx, tenant, limit, query)
			}, func(row ports.AcceptanceRulePackageRow) string { return row.ObjectID + "/" + row.VersionLabel }),
			row: func(t *testing.T, fixture cutFixture, objectID string) {
				saveRulePackageBody(t, fixture, objectID, "SYN-PRODUCT-1", "SYN-CONTRACT-1", "SYN-LE-1")
			},
			seed: func(t *testing.T, fixture cutFixture) {
				saveRulePackageBody(t, fixture, "SYN-RULES-A", "SYN-PRODUCT-EXPRESS", "SYN-CONTRACT-1", "SYN-LE-1")
				saveRulePackageBody(t, fixture, "SYN-RULES-B", "SYN-PRODUCT-FREIGHT", "SYN-CONTRACT-2", "SYN-LE-2")
				saveRulePackageBody(t, fixture, "SYN-RULES-C", "SYN-PRODUCT-EXPRESS", "SYN-CONTRACT-3", "SYN-LE-1")
			},
			cases: []pageCase{
				{"", "SYN-RULES-C/v1,SYN-RULES-B/v1,SYN-RULES-A/v1"},
				{"q=syn-product-express", "SYN-RULES-C/v1,SYN-RULES-A/v1"},
				{"q=SYN-CONTRACT-3", "SYN-RULES-C/v1"},
				{"q=SYN-LE-2", "SYN-RULES-B/v1"},
				{"sort=objectId", "SYN-RULES-A/v1,SYN-RULES-B/v1,SYN-RULES-C/v1"},
				// 三版生效起点相同，按起点排即全由行标识收尾。
				{"sort=effectiveStartsAt", "SYN-RULES-A/v1,SYN-RULES-B/v1,SYN-RULES-C/v1"},
				{"sort=-effectiveStartsAt&q=SYN-LE-1", "SYN-RULES-C/v1,SYN-RULES-A/v1"},
				{"q=scope-1&sort=objectId", "SYN-RULES-A/v1,SYN-RULES-B/v1,SYN-RULES-C/v1"},
				{"q=%25", ""},
			},
		},
		{
			name: "接受前财务控制声明", prefix: "SYN-CONTROL-CONTRACT-", pages: ports.PreAcceptanceControlPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.PreAcceptanceControlRow], error) {
				return catalogue.ListPreAcceptanceControls(ctx, tenant, limit, query)
			}, func(row ports.PreAcceptanceControlRow) string {
				return row.ContractObjectID + "/" + row.ContractVersion
			}),
			row: func(t *testing.T, fixture cutFixture, objectID string) {
				saveControlDeclaration(t, fixture, objectID, "")
			},
			seed: func(t *testing.T, fixture cutFixture) {
				saveControlDeclaration(t, fixture, "SYN-CONTROL-A", "")
				saveControlDeclaration(t, fixture, "SYN-CONTROL-B", "SYN-CLAUSE-PREPAID-ONLY")
				saveControlDeclaration(t, fixture, "SYN-CONTROL-C", "")
			},
			cases: []pageCase{
				{"", "SYN-CONTROL-C/v1,SYN-CONTROL-B/v1,SYN-CONTROL-A/v1"},
				{"requirement=REQUIRED", "SYN-CONTROL-C/v1,SYN-CONTROL-A/v1"},
				{"requirement=NOT_APPLICABLE", "SYN-CONTROL-B/v1"},
				{"requirement=NOT_APPLICABLE&requirement=REQUIRED", "SYN-CONTROL-C/v1,SYN-CONTROL-B/v1,SYN-CONTROL-A/v1"},
				{"q=prepaid-only", "SYN-CONTROL-B/v1"},
				{"sort=contractObjectId&requirement=REQUIRED", "SYN-CONTROL-A/v1,SYN-CONTROL-C/v1"},
				{"q=%25", ""},
			},
		},
		{
			name: "价格政策", prefix: "SYN-PRICE-PAGE-", pages: ports.PricePolicyPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.PricePolicyRow], error) {
				return catalogue.ListPricePolicies(ctx, tenant, limit, query)
			}, func(row ports.PricePolicyRow) string { return row.ObjectID + "/" + row.VersionLabel }),
			row: func(t *testing.T, fixture cutFixture, objectID string) {
				savePricePolicyBody(t, fixture, objectID, domain.SellDirection, "SYN-PLAN-SELL-1")
			},
			seed: func(t *testing.T, fixture cutFixture) {
				savePricePolicyBody(t, fixture, "SYN-PRICE-A", domain.SellDirection, "SYN-PLAN-SELL-1")
				savePricePolicyBody(t, fixture, "SYN-PRICE-B", domain.BuyDirection, "SYN-PLAN-BUY-1")
				savePricePolicyBody(t, fixture, "SYN-PRICE-C", domain.InternalDirection, "SYN-PLAN-INTERNAL-1")
			},
			cases: []pageCase{
				{"", "SYN-PRICE-C/v1,SYN-PRICE-B/v1,SYN-PRICE-A/v1"},
				{"direction=SELL", "SYN-PRICE-A/v1"},
				{"direction=BUY&direction=INTERNAL", "SYN-PRICE-C/v1,SYN-PRICE-B/v1"},
				{"q=syn-plan-buy", "SYN-PRICE-B/v1"},
				{"sort=objectId&direction=SELL&direction=BUY", "SYN-PRICE-A/v1,SYN-PRICE-B/v1"},
				{"q=SCOPE-1&direction=INTERNAL", "SYN-PRICE-C/v1"},
				{"q=%25", ""},
			},
		},
		{
			name: "结算政策", prefix: "SYN-SETTLEMENT-PAGE-", pages: ports.SettlementPolicyPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.SettlementPolicyRow], error) {
				return catalogue.ListSettlementPolicies(ctx, tenant, limit, query)
			}, func(row ports.SettlementPolicyRow) string { return row.ObjectID + "/" + row.VersionLabel }),
			row: func(t *testing.T, fixture cutFixture, objectID string) {
				saveSettlementPolicyBody(t, fixture, objectID, domain.TermsMethod, "SYN-CUSTOMER-1", "SYN-CHARGE-EXPRESS")
			},
			seed: func(t *testing.T, fixture cutFixture) {
				saveSettlementPolicyBody(t, fixture, "SYN-SETTLEMENT-A", domain.PrepaidMethod, "SYN-CUSTOMER-1", "SYN-CHARGE-EXPRESS")
				saveSettlementPolicyBody(t, fixture, "SYN-SETTLEMENT-B", domain.TermsMethod, "SYN-CUSTOMER-2", "SYN-CHARGE-FREIGHT")
				saveSettlementPolicyBody(t, fixture, "SYN-SETTLEMENT-C", domain.TermsMethod, "SYN-CUSTOMER-1", "SYN-CHARGE-EXPRESS")
			},
			cases: []pageCase{
				{"", "SYN-SETTLEMENT-C/v1,SYN-SETTLEMENT-B/v1,SYN-SETTLEMENT-A/v1"},
				{"method=TERMS", "SYN-SETTLEMENT-C/v1,SYN-SETTLEMENT-B/v1"},
				{"method=PREPAID", "SYN-SETTLEMENT-A/v1"},
				{"q=syn-customer-1", "SYN-SETTLEMENT-C/v1,SYN-SETTLEMENT-A/v1"},
				{"q=SYN-CHARGE-FREIGHT&method=TERMS", "SYN-SETTLEMENT-B/v1"},
				{"q=SYN-CONTRACT-1/v1&sort=objectId", "SYN-SETTLEMENT-A/v1,SYN-SETTLEMENT-B/v1,SYN-SETTLEMENT-C/v1"},
				{"sort=-effectiveStartsAt&method=TERMS", "SYN-SETTLEMENT-C/v1,SYN-SETTLEMENT-B/v1"},
				{"q=%25", ""},
			},
		},
		{
			name: "信用政策", prefix: "SYN-CREDIT-PAGE-", pages: ports.CreditPolicyPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.CreditPolicyRow], error) {
				return catalogue.ListCreditPolicies(ctx, tenant, limit, query)
			}, func(row ports.CreditPolicyRow) string { return row.ObjectID + "/" + row.VersionLabel }),
			row: func(t *testing.T, fixture cutFixture, objectID string) {
				saveCreditPolicyBody(t, fixture, objectID, "SYN-LE-1", "SYN-LEVEL-COMMERCIAL", "SYN-CHARGE-FREIGHT", creditAmountLimit(t, 100000))
			},
			seed: func(t *testing.T, fixture cutFixture) {
				saveCreditPolicyBody(t, fixture, "SYN-CREDIT-A", "SYN-LE-1", "SYN-LEVEL-COMMERCIAL", "SYN-CHARGE-EXPRESS", creditAmountLimit(t, 100000))
				saveCreditPolicyBody(t, fixture, "SYN-CREDIT-B", "SYN-LE-2", "SYN-LEVEL-FINANCE", "SYN-CHARGE-FREIGHT", creditRatioLimit(t, 500))
				saveCreditPolicyBody(t, fixture, "SYN-CREDIT-C", "SYN-LE-1", "SYN-LEVEL-FINANCE", "SYN-CHARGE-EXPRESS", creditAmountLimit(t, 50000))
			},
			cases: []pageCase{
				{"", "SYN-CREDIT-C/v1,SYN-CREDIT-B/v1,SYN-CREDIT-A/v1"},
				{"q=syn-level-finance", "SYN-CREDIT-C/v1,SYN-CREDIT-B/v1"},
				{"q=SYN-LE-2", "SYN-CREDIT-B/v1"},
				{"sort=objectId&q=SYN-CHARGE-EXPRESS", "SYN-CREDIT-A/v1,SYN-CREDIT-C/v1"},
				{"sort=-effectiveStartsAt", "SYN-CREDIT-C/v1,SYN-CREDIT-B/v1,SYN-CREDIT-A/v1"},
				{"q=%25", ""},
			},
		},
		{
			name: "时点锚声明", prefix: "SYN-ASOF-PAGE-", pages: ports.AsOfPolicyPages,
			list: shellLister(func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.AsOfPolicyRow], error) {
				return catalogue.ListAsOfPolicyDeclarations(ctx, tenant, limit, query)
			}, func(row ports.AsOfPolicyRow) string {
				return row.RulePackageObjectID + "/" + row.RulePackageVersion + "/" + row.JudgmentType
			}),
			row: func(t *testing.T, fixture cutFixture, objectID string) {
				saveAsOfDeclarationBody(t, fixture, objectID,
					pcAsOfPolicy(t, domain.NetworkReachabilityJudgment, "SYN-AT-ACCEPTANCE", "SYN-ASOF-REACH-1"))
			},
			seed: func(t *testing.T, fixture cutFixture) {
				saveAsOfDeclarationBody(t, fixture, "SYN-ASOF-A",
					pcAsOfPolicy(t, domain.NetworkReachabilityJudgment, "SYN-AT-ACCEPTANCE", "SYN-ASOF-REACH-1"),
					pcAsOfPolicy(t, domain.PreAcceptanceFinancialControlJudgment, "SYN-AT-SUBMISSION", "SYN-ASOF-CONTROL-1"))
				saveAsOfDeclarationBody(t, fixture, "SYN-ASOF-B",
					pcAsOfPolicy(t, domain.NetworkReachabilityJudgment, "SYN-AT-ACCEPTANCE", "SYN-ASOF-REACH-2"))
			},
			// SYN-ASOF-A 的两类判断同笔声明、声明时刻相同，缺省序上由判断类型按行标识同向收尾。
			cases: []pageCase{
				{"", "SYN-ASOF-B/v1/NETWORK_REACHABILITY,SYN-ASOF-A/v1/PRE_ACCEPTANCE_FINANCIAL_CONTROL,SYN-ASOF-A/v1/NETWORK_REACHABILITY"},
				{"judgmentType=NETWORK_REACHABILITY", "SYN-ASOF-B/v1/NETWORK_REACHABILITY,SYN-ASOF-A/v1/NETWORK_REACHABILITY"},
				{"judgmentType=PRE_ACCEPTANCE_FINANCIAL_CONTROL", "SYN-ASOF-A/v1/PRE_ACCEPTANCE_FINANCIAL_CONTROL"},
				{"q=syn-asof-reach", "SYN-ASOF-B/v1/NETWORK_REACHABILITY,SYN-ASOF-A/v1/NETWORK_REACHABILITY"},
				{"q=at-submission", "SYN-ASOF-A/v1/PRE_ACCEPTANCE_FINANCIAL_CONTROL"},
				{"sort=rulePackageObjectId", "SYN-ASOF-A/v1/NETWORK_REACHABILITY,SYN-ASOF-A/v1/PRE_ACCEPTANCE_FINANCIAL_CONTROL,SYN-ASOF-B/v1/NETWORK_REACHABILITY"},
				{"sort=-rulePackageObjectId&judgmentType=NETWORK_REACHABILITY", "SYN-ASOF-B/v1/NETWORK_REACHABILITY,SYN-ASOF-A/v1/NETWORK_REACHABILITY"},
				{"q=%25", ""},
			},
		},
	}
}

// policyContentOwner 登一份 tenant-1 的 kind 类已生效版本 v1，充当正文的拥有对象。
func policyContentOwner(t *testing.T, fixture cutFixture, kind domain.CommercialObjectKind, objectID string) domain.CommercialVersion {
	t.Helper()
	version := shellVersionFixture(t, kind, objectID, "v1", publishedAtRow, domain.CommercialVersionEffective)
	fixture.save(t, version)
	return version
}

func saveRulePackageBody(t *testing.T, fixture cutFixture, objectID, product, contract, legalEntity string) {
	t.Helper()
	owner := policyContentOwner(t, fixture, domain.AcceptanceRulePackageObject, objectID)
	applicability, err := domain.NewRulePackageApplicability(
		pcValue(t, domain.NewCommercialObjectID, product),
		pcValue(t, domain.NewCommercialObjectID, contract),
		pcValue(t, domain.NewLegalEntityReference, legalEntity),
		pcScope(t),
		owner.Effective(),
	)
	if err != nil {
		t.Fatalf("五维适用性：%v", err)
	}
	pack, err := domain.NewAcceptanceRulePackage(owner, applicability,
		[]domain.AssembledRule{pcAssembledRule(t, domain.MinimumIngressIdentityRules, "SYN-RULE-INGRESS-1")})
	if err != nil {
		t.Fatalf("规则包正文：%v", err)
	}
	mustSaveDeclaration(t, fixture.transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return fixture.publications.SaveAcceptanceRulePackage(txCtx, pack)
	}, ports.DeclarationSaved)
}

// saveControlDeclaration 给客户合同 objectID 登一份接受前财务控制声明：basis 为空即需要控制，否则是不适用及其依据。
func saveControlDeclaration(t *testing.T, fixture cutFixture, objectID, basis string) {
	t.Helper()
	contract := policyContentOwner(t, fixture, domain.CustomerContractObject, objectID)
	requirement, notApplicable := domain.PreAcceptanceControlRequired, domain.ControlNotApplicableBasis{}
	if basis != "" {
		requirement = domain.PreAcceptanceControlNotApplicable
		notApplicable = pcValue(t, domain.NewControlNotApplicableBasis, basis)
	}
	declaration, err := domain.DeclarePreAcceptanceControl(contract, requirement, notApplicable)
	if err != nil {
		t.Fatalf("接受前财务控制声明：%v", err)
	}
	mustSaveDeclaration(t, fixture.transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return fixture.publications.SavePreAcceptanceControl(txCtx, declaration)
	}, ports.DeclarationSaved)
}

// savePricePolicyBody 给 objectID 登一版价格政策，方案绑定与政策同向、不换算。
func savePricePolicyBody(t *testing.T, fixture cutFixture, objectID string, direction domain.PriceDirection, plan string) {
	t.Helper()
	version := policyContentOwner(t, fixture, domain.PriceRuleObject, objectID)
	mustSavePricePolicy(t, fixture.transactor, t.Context(), fixture.publications,
		pricePolicyOn(t, version, direction, direction, domain.PlanBindingConversionNone, plan),
		direction, domain.PlanBindingConversionNone)
}

func saveSettlementPolicyBody(
	t *testing.T,
	fixture cutFixture,
	objectID string,
	method domain.SettlementMethod,
	counterparty, chargeScope string,
) {
	t.Helper()
	version := policyContentOwner(t, fixture, domain.SettlementPolicyObject, objectID)
	applicability, err := domain.NewSettlementApplicability(
		pcValue(t, domain.NewLegalEntityReference, "SYN-LE-1"),
		pcValue(t, domain.NewCounterpartyReference, counterparty),
		pcValue(t, domain.NewCommercialVersionLabel, "SYN-CONTRACT-1/v1"),
		pcValue(t, domain.NewChargeScopeReference, chargeScope),
		pcValue(t, domain.NewCurrencyCode, "SYN"),
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("结算适用范围：%v", err)
	}
	policy, err := domain.NewSettlementPolicy(version, method, applicability)
	if err != nil {
		t.Fatalf("结算政策：%v", err)
	}
	mustSaveSettlementPolicy(t, fixture.transactor, t.Context(), fixture.publications, policy)
}

func saveCreditPolicyBody(
	t *testing.T,
	fixture cutFixture,
	objectID, legalEntity, level, chargeType string,
	limit domain.CreditLimit,
) {
	t.Helper()
	version := policyContentOwner(t, fixture, domain.CreditPolicyObject, objectID)
	policy, err := domain.NewCreditPolicy(version,
		pcValue(t, domain.NewLegalEntityReference, legalEntity),
		pcValue(t, domain.NewAuthorityLevel, level),
		pcValue(t, domain.NewChargeTypeReference, chargeType),
		limit,
		version.Effective(),
	)
	if err != nil {
		t.Fatalf("信用政策：%v", err)
	}
	mustSaveCreditPolicy(t, fixture.transactor, t.Context(), fixture.publications, policy)
}

// saveAsOfDeclarationBody 给接单规则包 objectID 同笔声明 policies 里的各类判断。
func saveAsOfDeclarationBody(t *testing.T, fixture cutFixture, objectID string, policies ...domain.AsOfPolicy) {
	t.Helper()
	owner := policyContentOwner(t, fixture, domain.AcceptanceRulePackageObject, objectID)
	declaration, err := domain.DeclareAsOfPolicies(owner, policies)
	if err != nil {
		t.Fatalf("时点锚声明：%v", err)
	}
	mustSaveDeclaration(t, fixture.transactor, func(txCtx context.Context) (ports.DeclarationSaveOutcome, error) {
		return fixture.publications.SaveAsOfPolicies(txCtx, declaration)
	}, ports.DeclarationSaved)
}

// Covers: 完成判据「翻页期间插入新行不重不漏」与「空册答空页」，各册各一遍。缺省序是登记或声明时刻降序，后登的排前；各册共
// 一个租户，各读各的正文表。
func TestPolicyContentCataloguesPageWithoutRepeatsOrGapsWhileContentArrives(t *testing.T) {
	fixture := newCutFixture(t)
	catalogue := shellCatalogueOf(t, fixture)
	ownerOf := func(label string) string {
		objectID, _, _ := strings.Cut(label, "/")
		return objectID
	}
	for _, content := range contentCatalogueCases(t, catalogue, pcTenant(t, "tenant-1")) {
		assertPagesWithoutRepeatsOrGaps(t, content.pages, content.list, func(id string) {
			content.row(t, fixture, id)
		}, ownerOf, content.prefix)
	}
	for _, content := range contentCatalogueCases(t, catalogue, pcTenant(t, "SYN-TENANT-EMPTY")) {
		assertEmptyCatalogue(t, content.name, content.pages, content.list)
	}
}

// Covers: 完成判据「筛选与 q 收窄、total 随之变；q 里的 % 按字面」，各册按自己声明的筛选维、排序维与 q 列各证一遍。筛选比的
// 是落库原词（各表 CHECK 收的就是领域那组词）；q 不分大小写，覆盖的正文格各册不同。
func TestPolicyContentCataloguesFilterAndSearch(t *testing.T) {
	fixture := newCutFixture(t)
	catalogue := shellCatalogueOf(t, fixture)
	for _, content := range contentCatalogueCases(t, catalogue, pcTenant(t, "tenant-1")) {
		content.seed(t, fixture)
		assertFilteredPages(t, content.name, content.pages, content.list, func(row string) string { return row }, content.cases)
	}
}
