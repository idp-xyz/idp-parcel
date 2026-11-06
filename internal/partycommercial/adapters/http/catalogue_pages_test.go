package commercialhttp_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
	"time"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对票 catalogue-read-pagination-rollout/11 迁到 ADR-0144 的各列表端点证查询参数的传输面：答复带 page（size 是
// Intake 定的页大小，next 与 total 照读口）、读口收到的是按本册声明解出的查询；未知键、集外排序维、词表外筛选值、解不开
// 的游标各答 400 带理由散文，且不进读口；解码先于 Intake——渠道未配置时坏参数答 400、好参数答 403。各册同形，表驱动，
// 每册各跑一遍。

// tenantPage 是各册旧替身答页的共用一段：读失败照答错，他租答空页，本租答全部行、总数即行数。
func tenantPage[Row any](err error, mine bool, rows []Row) (cataloguepage.Result[Row], error) {
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	if !mine {
		return cataloguepage.Result[Row]{}, nil
	}
	return cataloguepage.Result[Row]{Rows: rows, Total: int64(len(rows))}, nil
}

var pageListedAt = time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)

// pageRecorder 记下读口收到的查询。recordPage 按那份查询给本页末行编下一页游标——游标只能由解出它的那份查询编，端点对它的
// 校验（摘要与本次条件是否相符）才证得到；identity 是那一行标识各段的取值，段数与各段的值类须合本册声明。
type pageRecorder struct {
	seen *cataloguepage.Query
}

func recordPage[Row any](recorder *pageRecorder, query cataloguepage.Query, row Row, identity ...string) (cataloguepage.Result[Row], error) {
	recorder.seen = &query
	next, err := query.CursorAfter(cataloguepage.Position{
		Value:    cataloguepage.FormatInstant(pageListedAt),
		Identity: identity,
	})
	if err != nil {
		return cataloguepage.Result[Row]{}, err
	}
	return cataloguepage.Result[Row]{Rows: []Row{row}, Next: next, Total: 3}, nil
}

type pagingPartyIdentityReader struct {
	pageRecorder
}

func (reader *pagingPartyIdentityReader) ListBusinessParties(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.BusinessPartyRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.BusinessPartyRow{
		TenantID: "tenant-1", PartyID: "SYN-PARTY-1", Status: "EFFECTIVE",
		EffectiveFrom: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-PARTY-1")
}

func (reader *pagingPartyIdentityReader) ListGroupLegalEntities(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.GroupLegalEntityRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.GroupLegalEntityRow{
		TenantID: "tenant-1", LegalEntityID: "SYN-LE-1", PartyID: "SYN-PARTY-1", Status: "EFFECTIVE",
		EffectiveFrom: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-LE-1")
}

func (reader *pagingPartyIdentityReader) ListPartyRelationships(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.PartyRelationshipRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.PartyRelationshipRow{
		TenantID: "tenant-1", RelationshipID: "SYN-REL-1", HolderID: "SYN-PARTY-1", CounterpartyID: "SYN-PARTY-2",
		Role: "CUSTOMER", Status: "EFFECTIVE", EffectiveStartsAt: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-REL-1")
}

func (reader *pagingPartyIdentityReader) ListLegalEntityRevisions(
	context.Context, domain.TenantID, domain.LegalEntityReference,
) ([]ports.LegalEntityRevisionRow, error) {
	return nil, nil
}

func (reader *pagingPartyIdentityReader) ListBusinessPartyRevisions(
	context.Context, domain.TenantID, domain.PartyID,
) ([]ports.BusinessPartyRevisionRow, error) {
	return nil, nil
}

type pagingRegistrationNumberTypeReader struct {
	pageRecorder
}

func (reader *pagingRegistrationNumberTypeReader) ListRegistrationNumberTypes(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.RegistrationNumberTypeRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.RegistrationNumberTypeRow{
		TenantID: "tenant-1", CountryCode: "XA", TypeCode: "SYN-LIFETIME", Revision: 1, TypeName: "合成终身注册号",
		Layer: "IDENTITY", FormatPattern: `SYN-[0-9]{6}`, Basis: "SYN-BASIS-1", Status: "EFFECTIVE",
		EffectiveFrom: pageListedAt, RegisteredAt: pageListedAt,
	}, "XA", "SYN-LIFETIME")
}

type pagingProductChannelMappingReader struct {
	pageRecorder
}

func (reader *pagingProductChannelMappingReader) ListProductChannelMappings(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ProductChannelMappingRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.ProductChannelMappingRow{
		TenantID: "tenant-1", MappingID: "SYN-MAP-1", Revision: 1, ProductObjectID: "SYN-PRODUCT-1",
		ProductVersionLabel: "v1", Channels: []string{"SYN-CH-1"}, Basis: "SYN-BASIS-1",
		EffectiveStartsAt: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-MAP-1")
}

func (reader *pagingProductChannelMappingReader) ListProductChannelMappingRevisions(
	context.Context, domain.TenantID, domain.ProductChannelMappingID,
) ([]ports.ProductChannelMappingRow, error) {
	return nil, nil
}

// pagingVersionShellReader 同时是服务产品与商业关系两个读口：三册都是版本壳，行标识都是对象与版本两段。
type pagingVersionShellReader struct {
	pageRecorder
}

func (reader *pagingVersionShellReader) ListServiceProducts(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.ServiceProductCatalogueRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.ServiceProductCatalogueRow{
		ObjectID: "SYN-PRODUCT-1", VersionLabel: "v1", Scope: "SYN-SCOPE-1", Status: "EFFECTIVE",
		EffectiveStartsAt: pageListedAt, PublishedAt: pageListedAt,
	}, "SYN-PRODUCT-1", "v1")
}

func (reader *pagingVersionShellReader) ListCustomerContracts(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerContractCatalogueRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.CustomerContractCatalogueRow{
		ObjectID: "SYN-CONTRACT-1", VersionLabel: "v1", Scope: "SYN-SCOPE-1", Status: "EFFECTIVE",
		EffectiveStartsAt: pageListedAt, PublishedAt: pageListedAt,
	}, "SYN-CONTRACT-1", "v1")
}

func (reader *pagingVersionShellReader) ListSupplierAgreements(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.SupplierAgreementCatalogueRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.SupplierAgreementCatalogueRow{
		ObjectID: "SYN-AGREEMENT-1", VersionLabel: "v1", Scope: "SYN-SCOPE-1", Status: "EFFECTIVE",
		EffectiveStartsAt: pageListedAt, PublishedAt: pageListedAt,
	}, "SYN-AGREEMENT-1", "v1")
}

func (*pagingVersionShellReader) FindServiceProduct(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.ServiceProductCatalogueRow, bool, error) {
	return ports.ServiceProductCatalogueRow{}, false, nil
}

func (*pagingVersionShellReader) FindCustomerContract(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.CustomerContractCatalogueRow, bool, error) {
	return ports.CustomerContractCatalogueRow{}, false, nil
}

func (*pagingVersionShellReader) FindSupplierAgreement(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.SupplierAgreementCatalogueRow, bool, error) {
	return ports.SupplierAgreementCatalogueRow{}, false, nil
}

// pagingPolicyReader 是策略目录九种册子的读口。各册行标识的段数照本册声明：接受前财务控制声明指名合同两段，时点锚声明多
// 判断类型一段。
type pagingPolicyReader struct {
	pageRecorder
}

func (reader *pagingPolicyReader) ListAcceptanceRulePackages(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.AcceptanceRulePackageRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.AcceptanceRulePackageRow{
		ObjectID: "SYN-RULES-1", VersionLabel: "v1", ServiceProduct: "SYN-PRODUCT-1", Contract: "SYN-CONTRACT-1",
		LegalEntity: "SYN-LE-1", Scope: "SYN-SCOPE-1", EffectiveStartsAt: pageListedAt, DeclaredAt: pageListedAt,
	}, "SYN-RULES-1", "v1")
}

func (reader *pagingPolicyReader) ListPreAcceptanceControls(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.PreAcceptanceControlRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.PreAcceptanceControlRow{
		ContractObjectID: "SYN-CONTRACT-1", ContractVersion: "v1", Requirement: "REQUIRED", DeclaredAt: pageListedAt,
	}, "SYN-CONTRACT-1", "v1")
}

func (reader *pagingPolicyReader) ListPricePolicies(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.PricePolicyRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.PricePolicyRow{
		ObjectID: "SYN-PRICE-1", VersionLabel: "v1", Direction: "SELL", PlanRef: "SYN-PLAN-1", PlanDirection: "SELL",
		PolicyScope: "SYN-SCOPE-1", EffectiveStartsAt: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-PRICE-1", "v1")
}

func (reader *pagingPolicyReader) ListSettlementPolicies(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.SettlementPolicyRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.SettlementPolicyRow{
		ObjectID: "SYN-SETTLEMENT-1", VersionLabel: "v1", Method: "TERMS", LegalEntity: "SYN-LE-1",
		Counterparty: "SYN-PARTY-1", ContractLabel: "v1", ChargeScope: "SYN-CHARGE-1", Currency: "XTS",
		EffectiveStartsAt: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-SETTLEMENT-1", "v1")
}

func (reader *pagingPolicyReader) ListAsOfPolicyDeclarations(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.AsOfPolicyRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.AsOfPolicyRow{
		RulePackageObjectID: "SYN-RULES-1", RulePackageVersion: "v1", JudgmentType: "NETWORK_REACHABILITY",
		SemanticsRef: "SYN-SEMANTICS-1", PolicyVersion: "SYN-POLICY-V1", DeclaredAt: pageListedAt,
	}, "SYN-RULES-1", "v1", "NETWORK_REACHABILITY")
}

func (reader *pagingPolicyReader) ListAuthorizationRules(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.AuthorizationRuleRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.AuthorizationRuleRow{
		ObjectID: "SYN-AUTHZ-1", VersionLabel: "v1", Scope: "SYN-SCOPE-1", Status: "EFFECTIVE",
		EffectiveStartsAt: pageListedAt, PublishedAt: pageListedAt,
	}, "SYN-AUTHZ-1", "v1")
}

func (reader *pagingPolicyReader) ListCreditPolicies(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CreditPolicyRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.CreditPolicyRow{
		ObjectID: "SYN-CREDIT-1", VersionLabel: "v1", LegalEntity: "SYN-LE-1", AuthorityLevel: "SYN-AUTHORITY-1",
		ChargeType: "SYN-CHARGE-1", HasAmount: true, LimitMinor: 100000,
		EffectiveStartsAt: pageListedAt, RegisteredAt: pageListedAt,
	}, "SYN-CREDIT-1", "v1")
}

func (reader *pagingPolicyReader) ListCustomerServiceRules(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.CustomerServiceRuleRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.CustomerServiceRuleRow{
		ObjectID: "SYN-SERVICE-RULE-1", VersionLabel: "v1", Scope: "SYN-SCOPE-1", Status: "EFFECTIVE",
		EffectiveStartsAt: pageListedAt, PublishedAt: pageListedAt,
	}, "SYN-SERVICE-RULE-1", "v1")
}

func (reader *pagingPolicyReader) ListPreAcceptanceFinancialControlPolicies(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.PreAcceptanceFinancialControlPolicyRow], error) {
	return recordPage(&reader.pageRecorder, query, ports.PreAcceptanceFinancialControlPolicyRow{
		ObjectID: "SYN-CONTROL-POLICY-1", VersionLabel: "v1", Scope: "SYN-SCOPE-1", Status: "EFFECTIVE",
		EffectiveStartsAt: pageListedAt, PublishedAt: pageListedAt,
	}, "SYN-CONTROL-POLICY-1", "v1")
}

func (*pagingPolicyReader) FindAcceptanceRulePackage(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.AcceptanceRulePackageRow, bool, error) {
	return ports.AcceptanceRulePackageRow{}, false, nil
}

func (*pagingPolicyReader) FindPricePolicy(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.PricePolicyRow, bool, error) {
	return ports.PricePolicyRow{}, false, nil
}

func (*pagingPolicyReader) FindSettlementPolicy(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.SettlementPolicyRow, bool, error) {
	return ports.SettlementPolicyRow{}, false, nil
}

func (*pagingPolicyReader) FindAuthorizationRule(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.AuthorizationRuleRow, bool, error) {
	return ports.AuthorizationRuleRow{}, false, nil
}

func (*pagingPolicyReader) FindCreditPolicy(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.CreditPolicyRow, bool, error) {
	return ports.CreditPolicyRow{}, false, nil
}

func (*pagingPolicyReader) FindCustomerServiceRule(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.CustomerServiceRuleRow, bool, error) {
	return ports.CustomerServiceRuleRow{}, false, nil
}

func (*pagingPolicyReader) FindPreAcceptanceFinancialControlPolicy(
	context.Context, domain.TenantID, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (ports.PreAcceptanceFinancialControlPolicyRow, bool, error) {
	return ports.PreAcceptanceFinancialControlPolicyRow{}, false, nil
}

// pagingDraftReader 是待批准载体列表的翻页替身。载体要经录入链才造得出，这张表只数行、不看行里的字段，交一行零值载体；
// 行标识按本册声明的段序与值类给（类别是整数码）。
type pagingDraftReader struct {
	pageRecorder
}

func (reader *pagingDraftReader) ListOpenDrafts(
	_ context.Context, _ domain.TenantID, _ int, query cataloguepage.Query,
) (cataloguepage.Result[ports.OpenPublicationDraft], error) {
	return recordPage(&reader.pageRecorder, query, ports.OpenPublicationDraft{},
		cataloguepage.FormatInteger(int64(domain.CreditPolicyObject)), "SYN-DRAFT-1", "v1")
}

func (*pagingDraftReader) LoadDraft(
	context.Context, domain.TenantID, domain.CommercialObjectKind, domain.CommercialObjectID, domain.CommercialVersionLabel,
) (domain.PublicationDraft, bool, error) {
	return domain.PublicationDraft{}, false, nil
}

// pagedEndpointCase 是一册在表里的一行。selector 是册子选择器（如 kind=PRICE_POLICY），每次请求都带上；badFilter 只在本册
// 有带词表的筛选维时给。goodQuery 是一组合本册声明的条件，翻页那一步换条件拿旧游标就是拿它与 changedQuery 对照。
type pagedEndpointCase struct {
	name         string
	endpoint     func(commercialhttp.CommercialCatalogueIntake) http.Handler
	recorder     *pageRecorder
	collection   string
	selector     string
	defaultSort  string
	badSort      string
	badFilter    string
	goodQuery    string
	changedQuery string
}

func pagedEndpointCases() []pagedEndpointCase {
	identities := &pagingPartyIdentityReader{}
	numberTypes := &pagingRegistrationNumberTypeReader{}
	mappings := &pagingProductChannelMappingReader{}
	shells := &pagingVersionShellReader{}
	policies := &pagingPolicyReader{}
	policyEndpoint := func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
		return commercialhttp.NewQueryCommercialPoliciesEndpoint(intake, policies)
	}
	drafts := &pagingDraftReader{}
	return []pagedEndpointCase{
		{
			name: "业务参与方",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryBusinessPartiesEndpoint(intake, identities)
			},
			recorder: &identities.pageRecorder, collection: "parties",
			defaultSort: "-registeredAt", badSort: "status", badFilter: "status=ARCHIVED",
			goodQuery: "status=EFFECTIVE", changedQuery: "status=REGISTERED",
		},
		{
			name: "集团法人",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryGroupLegalEntitiesEndpoint(intake, identities)
			},
			recorder: &identities.pageRecorder, collection: "entities",
			defaultSort: "-registeredAt", badSort: "partyName", badFilter: "status=ACTIVE",
			goodQuery: "registrationCountry=CN&status=EFFECTIVE", changedQuery: "registrationCountry=SG&status=EFFECTIVE",
		},
		{
			name: "参与方关系",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryPartyRelationshipsEndpoint(intake, identities)
			},
			recorder: &identities.pageRecorder, collection: "relationships",
			defaultSort: "-registeredAt", badSort: "holderName", badFilter: "role=OWNER",
			goodQuery: "role=CUSTOMER&status=CANDIDATE&status=EFFECTIVE", changedQuery: "role=SUPPLIER&status=EFFECTIVE",
		},
		{
			name: "注册号类型",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryRegistrationNumberTypesEndpoint(intake, numberTypes)
			},
			recorder: &numberTypes.pageRecorder, collection: "registrationNumberTypes",
			defaultSort: "-registeredAt", badSort: "typeName", badFilter: "layer=CORPORATE",
			goodQuery: "layer=IDENTITY&countryCode=XA", changedQuery: "layer=PROFILE&countryCode=XA",
		},
		{
			name: "渠道产品映射",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryProductChannelMappingsEndpoint(intake, mappings)
			},
			recorder: &mappings.pageRecorder, collection: "mappings",
			defaultSort: "-registeredAt", badSort: "productObjectId",
			goodQuery: "q=SYN-PRODUCT", changedQuery: "q=SYN-CH",
		},
		{
			name: "服务产品",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryServiceProductsEndpoint(intake, shells)
			},
			recorder: &shells.pageRecorder, collection: "products",
			defaultSort: "-publishedAt", badSort: "registeredAt", badFilter: "status=DRAFT",
			goodQuery: "status=EFFECTIVE", changedQuery: "status=RETIRED",
		},
		{
			name: "客户合同",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryCustomerContractsEndpoint(intake, shells)
			},
			recorder: &shells.pageRecorder, collection: "contracts",
			defaultSort: "-publishedAt", badSort: "status", badFilter: "status=ACTIVE",
			goodQuery: "status=EFFECTIVE&status=PUBLISHED", changedQuery: "status=EFFECTIVE",
		},
		{
			name: "供应商协议",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQuerySupplierAgreementsEndpoint(intake, shells)
			},
			recorder: &shells.pageRecorder, collection: "agreements",
			defaultSort: "-publishedAt", badSort: "supplier", badFilter: "status=DRAFT",
			goodQuery: "q=SYN-SUPPLIER", changedQuery: "q=SYN-SUPPLIER&status=EFFECTIVE",
		},
		{
			name: "接单规则包", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=ACCEPTANCE_RULE_PACKAGE", defaultSort: "-declaredAt", badSort: "registeredAt",
			goodQuery: "q=SYN-RULES", changedQuery: "q=SYN-PRODUCT",
		},
		{
			name: "接受前财务控制声明", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=PRE_ACCEPTANCE_CONTROL", defaultSort: "-declaredAt", badSort: "objectId",
			badFilter: "requirement=OPTIONAL", goodQuery: "requirement=REQUIRED", changedQuery: "requirement=NOT_APPLICABLE",
		},
		{
			name: "价格政策", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=PRICE_POLICY", defaultSort: "-registeredAt", badSort: "declaredAt",
			badFilter: "direction=OUTBOUND", goodQuery: "direction=SELL", changedQuery: "direction=BUY&direction=SELL",
		},
		{
			name: "结算政策", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=SETTLEMENT_POLICY", defaultSort: "-registeredAt", badSort: "currency",
			badFilter: "method=CASH", goodQuery: "method=TERMS&q=SYN-LE", changedQuery: "method=TERMS",
		},
		{
			name: "时点锚声明", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=AS_OF_POLICY", defaultSort: "-declaredAt", badSort: "effectiveStartsAt",
			badFilter: "judgmentType=PRICING", goodQuery: "judgmentType=NETWORK_REACHABILITY",
			changedQuery: "judgmentType=PRE_ACCEPTANCE_FINANCIAL_CONTROL",
		},
		{
			name: "授权规则", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=AUTHORIZATION_RULE", defaultSort: "-publishedAt", badSort: "registeredAt",
			badFilter: "status=DRAFT", goodQuery: "status=EFFECTIVE", changedQuery: "status=RETIRED",
		},
		{
			name: "信用政策", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=CREDIT_POLICY", defaultSort: "-registeredAt", badSort: "chargeType",
			goodQuery: "q=SYN-LE", changedQuery: "q=SYN-CHARGE",
		},
		{
			name: "客户服务规则", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=CUSTOMER_SERVICE_RULE", defaultSort: "-publishedAt", badSort: "responsibleParty",
			badFilter: "status=ACTIVE", goodQuery: "status=EFFECTIVE&q=SYN-PARTY", changedQuery: "status=EFFECTIVE",
		},
		{
			name: "接受前财务控制策略", endpoint: policyEndpoint, recorder: &policies.pageRecorder, collection: "policies",
			selector: "kind=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", defaultSort: "-publishedAt", badSort: "jointPassCondition",
			badFilter: "status=DRAFT", goodQuery: "status=SUPERSEDED&status=EXPIRED", changedQuery: "status=EXPIRED",
		},
		{
			name: "待批准发布载体",
			endpoint: func(intake commercialhttp.CommercialCatalogueIntake) http.Handler {
				return commercialhttp.NewQueryPublicationDraftsEndpoint(intake, drafts)
			},
			recorder: &drafts.pageRecorder, collection: "drafts",
			defaultSort: "-submittedAt", badSort: "approvedAt", badFilter: "status=PUBLISHED",
			goodQuery: "status=PENDING_APPROVAL&kind=CREDIT_POLICY", changedQuery: "status=APPROVED&kind=CREDIT_POLICY",
		},
	}
}

// Covers: 票面「kind 保持册子选择器原义、登为 Selectors」——kind 的取值进游标摘要（ADR-0144 决定四）：价格政策与结算政策
// 缺省序与行标识同形，拿前者的游标翻后者仍答游标不符，一册的位置不交给另一册的读口。
func TestCommercialPolicyCursorDoesNotCrossKinds(t *testing.T) {
	reader := &pagingPolicyReader{}
	endpoint := commercialhttp.NewQueryCommercialPoliciesEndpoint(intakeDouble{query: catalogueQuery(t)}, reader)
	first, _ := getPaged(t, endpoint, "kind=PRICE_POLICY", "")
	var body struct {
		Page struct {
			Next string `json:"next"`
		} `json:"page"`
	}
	if err := json.Unmarshal(first.Body.Bytes(), &body); err != nil || first.Code != http.StatusOK || body.Page.Next == "" {
		t.Fatalf("价格政策第一页 status = %d, body = %s（%v），want 200 且带 next", first.Code, first.Body, err)
	}
	reader.seen = nil
	stale, problem := getPaged(t, endpoint, "kind=SETTLEMENT_POLICY", "after="+url.QueryEscape(body.Page.Next))
	if stale.Code != http.StatusBadRequest || problem.Error.Detail != "游标与本次的排序或筛选不符，请从第一页重取" {
		t.Fatalf("拿价格政策的游标翻结算政策 status = %d, problem = %+v，want 400 游标不符", stale.Code, problem)
	}
	if reader.seen != nil {
		t.Fatalf("跨种的游标进了读口：%+v", reader.seen)
	}
}

func getPaged(t *testing.T, endpoint http.Handler, selector, rawQuery string) (*httptest.ResponseRecorder, problemBody) {
	t.Helper()
	parts := make([]string, 0, 2)
	for _, part := range []string{selector, rawQuery} {
		if part != "" {
			parts = append(parts, part)
		}
	}
	recorder := httptest.NewRecorder()
	endpoint.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/?"+strings.Join(parts, "&"), nil))
	var problem problemBody
	if recorder.Code != http.StatusOK {
		if err := json.Unmarshal(recorder.Body.Bytes(), &problem); err != nil {
			t.Fatalf("decode problem %s: %v", recorder.Body.Bytes(), err)
		}
	}
	return recorder, problem
}

// Covers: 完成判据「答复带 page，size 等于 Intake 的页大小」与「换条件拿旧游标答 400」——page 三格齐在（ADR-0144 决定五），
// 读口收到的第一页查询是本册缺省序、没有游标；游标带着上一次的条件摘要，换了条件还拿它答 400，条件没变则把位置交给读口。
func TestMigratedCatalogueEndpointsCarryThePage(t *testing.T) {
	limit := catalogueQuery(t).Limit
	for _, tc := range pagedEndpointCases() {
		endpoint := tc.endpoint(intakeDouble{query: catalogueQuery(t)})
		tc.recorder.seen = nil
		recorder, _ := getPaged(t, endpoint, tc.selector, tc.goodQuery)
		if recorder.Code != http.StatusOK {
			t.Fatalf("%s 第一页 status = %d, body = %s", tc.name, recorder.Code, recorder.Body)
		}
		var body map[string]json.RawMessage
		if err := json.Unmarshal(recorder.Body.Bytes(), &body); err != nil {
			t.Fatalf("%s decode %s: %v", tc.name, recorder.Body.Bytes(), err)
		}
		var rows []json.RawMessage
		if err := json.Unmarshal(body[tc.collection], &rows); err != nil || len(rows) != 1 {
			t.Fatalf("%s 行集 %s = %s（%v），want 读口答的那一行", tc.name, tc.collection, body[tc.collection], err)
		}
		var page struct {
			Size  int     `json:"size"`
			Next  *string `json:"next"`
			Total *int64  `json:"total"`
		}
		if err := json.Unmarshal(body["page"], &page); err != nil {
			t.Fatalf("%s page = %s: %v", tc.name, body["page"], err)
		}
		if page.Size != limit || page.Next == nil || *page.Next == "" || page.Total == nil || *page.Total != 3 {
			t.Fatalf("%s page = %s，want size %d（Intake 的页大小）、next 在场、total 3", tc.name, body["page"], limit)
		}
		if seen := tc.recorder.seen; seen == nil || seen.Sort.String() != tc.defaultSort || seen.After != nil {
			t.Fatalf("%s 读口收到的第一页查询 = %+v，want 缺省序 %s 且无游标", tc.name, seen, tc.defaultSort)
		}

		cursor := "after=" + url.QueryEscape(*page.Next)
		stale, problem := getPaged(t, endpoint, tc.selector, tc.changedQuery+"&"+cursor)
		if stale.Code != http.StatusBadRequest || problem.Error.Detail != "游标与本次的排序或筛选不符，请从第一页重取" {
			t.Fatalf("%s 换条件拿旧游标 status = %d, problem = %+v", tc.name, stale.Code, problem)
		}
		next, _ := getPaged(t, endpoint, tc.selector, tc.goodQuery+"&"+cursor)
		if next.Code != http.StatusOK || tc.recorder.seen.After == nil {
			t.Fatalf("%s 同条件翻页 status = %d, 读口收到的位置 = %+v", tc.name, next.Code, tc.recorder.seen.After)
		}
	}
}

// Covers: 完成判据「未知键、集外排序维、词表外筛选值、解不开的游标各答 400 带理由」与「未配置渠道时坏参数 400、好参数 403」——
// 查询参数一律封闭集（ADR-0144 决定三、四）：多给的键若被静默忽略，页面以为筛了、服务端其实没筛。坏参数不进读口。
func TestMigratedCatalogueEndpointsRefuseQueriesOutsideTheDeclaration(t *testing.T) {
	for _, tc := range pagedEndpointCases() {
		endpoint := tc.endpoint(intakeDouble{query: catalogueQuery(t)})
		refusals := []struct {
			query  string
			reason string
		}{
			{"tenant=SYN-TENANT-02", "tenant"},
			{"sort=" + tc.badSort, tc.badSort},
			{"sort=-" + tc.badSort, tc.badSort},
			{"after=SYN-not-a-cursor", "请从第一页重取"},
		}
		if tc.badFilter != "" {
			_, value, _ := strings.Cut(tc.badFilter, "=")
			refusals = append(refusals, struct {
				query  string
				reason string
			}{tc.badFilter, value})
		}
		for _, refusal := range refusals {
			tc.recorder.seen = nil
			recorder, problem := getPaged(t, endpoint, tc.selector, refusal.query)
			if recorder.Code != http.StatusBadRequest || problem.Error.Code != "MALFORMED_REQUEST" ||
				!strings.Contains(problem.Error.Detail, refusal.reason) {
				t.Fatalf("%s %s: status = %d, problem = %+v；want 400 MALFORMED_REQUEST 且理由提到 %q",
					tc.name, refusal.query, recorder.Code, problem, refusal.reason)
			}
			if tc.recorder.seen != nil {
				t.Fatalf("%s %s: 坏参数进了读口", tc.name, refusal.query)
			}
		}

		unconfigured := tc.endpoint(intakeDouble{err: commercialhttp.ErrAccessChannelNotConfigured})
		if recorder, problem := getPaged(t, unconfigured, tc.selector, "sort="+tc.badSort); recorder.Code != http.StatusBadRequest ||
			!strings.Contains(problem.Error.Detail, tc.badSort) {
			t.Fatalf("%s 未配置渠道 + 坏参数 status = %d, problem = %+v，want 400 且理由点名参数（解码先于 Intake）",
				tc.name, recorder.Code, problem)
		}
		if recorder, _ := getPaged(t, unconfigured, tc.selector, tc.goodQuery); recorder.Code != http.StatusForbidden {
			t.Fatalf("%s 未配置渠道 + 好参数 status = %d，want 403", tc.name, recorder.Code)
		}
	}
}
