package ports

import (
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件是本上下文其余各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/11）：可排维与
// 筛选维按答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列、q 覆盖哪几列在 postgres 读面里。缺省序取各册
// 分页之前的首列与方向，不带 sort 的第一页次序不因分页而变。决胜键是行标识、与排序同向（决定三），只在首列完全相同的行
// 之间起作用。客户账户那一册的声明在 CustomerAccountCatalogue。
//
// 缺省维之外只为管理台确有换序需要的维度开可排维、只为页面要收窄的格开筛选维，不为「以后可能要」开：每开一维就多一个要
// 索引、要校验的面。各册的依据写在各自声明上；注册号类型、渠道产品映射与开放中的载体这几页管理台只按缺省次序列，不开别的
// 可排维。

// 参与方身份三册（业务参与方、集团法人、参与方关系）与客户账户读同一份登记册（0015），按修订折叠到最新一笔再排，缺省按
// 登记时刻倒序。其余可排维的依据是管理台这几页的表头排序：业务参与方与法人页可按标识、生效时点、登记时间排，关系页可按
// 关系号与生效起排。表头上的名称列与关系双方的标识列不开：名称在法人册与关系册是左连接来的、可缺，可排维的列须非空；按号
// 找有本册的标识那一维就够。业务参与方与法人的状态随时钟变，不作可排维（理由同 CustomerAccountCatalogue）。

// identityStatuses 是业务参与方与法人两册 status 的词表：两册与客户账户的 status 出自同一个 domain.IdentityLifecycle 的
// 时点导出，词与 CustomerAccountCatalogue 的那一维逐字相同。
var identityStatuses = cataloguepage.Words[domain.PartyIdentityStatus]()

// BusinessPartyPages 是 /commercial-business-parties 的查询声明。
var BusinessPartyPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-business-parties",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "effectiveFrom", Kind: cataloguepage.Instant},
		{Name: "partyId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters:     []cataloguepage.FilterDimension{{Name: "status", Vocabulary: identityStatuses}},
})

// GroupLegalEntityPages 是 /commercial-group-legal-entities 的查询声明。registrationCountry 是开放码（ISO 3166 两位字母形状，
// 码表不内置，判据同 0033），不设词表；最新修订未登身份层的法人不匹配任何国家。
var GroupLegalEntityPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-group-legal-entities",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "effectiveFrom", Kind: cataloguepage.Instant},
		{Name: "legalEntityId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: identityStatuses},
		{Name: "registrationCountry"},
	},
})

// PartyRelationshipPages 是 /commercial-party-relationships 的查询声明。status 是登记的关系状态事实，不随时钟走（与身份
// 两册的时点导出不同，理由在 postgres 读面 ListPartyRelationships 的头注），词表取自 domain.RelationshipStatus；role 取自
// domain.PartyRole。
var PartyRelationshipPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-party-relationships",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
		{Name: "effectiveStartsAt", Kind: cataloguepage.Instant},
		{Name: "relationshipId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: cataloguepage.Words[domain.RelationshipStatus]()},
		{Name: "role", Vocabulary: cataloguepage.Words[domain.PartyRole]()},
	},
})

// RegistrationNumberTypePages 是 /commercial-registration-number-types 的查询声明。行标识是国家 / 地区与类型代码两段（登记册
// 的键去掉修订号）。status 与身份两册出自同一条时点导出，词表取自 domain.RegistrationNumberTypeStatus；layer 取自
// domain.RegistrationNumberLayer；countryCode 是开放码，判据同 GroupLegalEntityPages 的 registrationCountry。layer 与
// countryCode 的依据是法人表单挑类型候选的查法（身份层与资料层各按国家 / 地区挑）：分页之后表单要让服务端按这两格收窄，
// 不能只在第一页里挑。
var RegistrationNumberTypePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-registration-number-types",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "status", Vocabulary: cataloguepage.Words[domain.RegistrationNumberTypeStatus]()},
		{Name: "layer", Vocabulary: cataloguepage.Words[domain.RegistrationNumberLayer]()},
		{Name: "countryCode"},
	},
})

// 版本壳各册（服务产品、客户合同、供应商协议，与策略目录里的授权规则、客户服务规则、接受前财务控制策略）由
// commercial_version 驱动，缺省按发布时刻倒序（-publishedAt 而不是 -registeredAt）：版本壳上没有登记时刻这一格，发布时刻
// 就是它进入发布登记册的那一刻。行标识是对象与版本（commercial_version 的主键去掉租户与类别）。另可按对象号排：这几页把
// 同一对象的各版本归成一组展示，服务端按对象号排，一组才不被页界拆开。

// versionStatuses 是版本壳各册 status 的词表，即目录行上那一格的取值：发布登记册只收发布之后的位置（发布重建拒草稿），
// DRAFT 因此不在词表里，筛它答 400 而不是永远答空页。
var versionStatuses = []string{
	domain.CommercialVersionPublished.String(),
	domain.CommercialVersionEffective.String(),
	domain.CommercialVersionExpired.String(),
	domain.CommercialVersionRetired.String(),
	domain.CommercialVersionSuperseded.String(),
}

// versionShellPages 是版本壳各册共用的声明形状，selectors 只有策略目录里以版本壳为行的那几种用（kind）。
func versionShellPages(name string, selectors ...string) *cataloguepage.Catalogue {
	return cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name: name,
		Sorts: []cataloguepage.SortDimension{
			{Name: "publishedAt", Kind: cataloguepage.Instant},
			{Name: "objectId", Kind: cataloguepage.Text},
		},
		DefaultSort: cataloguepage.Sort{Field: "publishedAt", Descending: true},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
		Filters:     []cataloguepage.FilterDimension{{Name: "status", Vocabulary: versionStatuses}},
		Selectors:   selectors,
	})
}

// ServiceProductPages 是 /commercial-service-products 的查询声明。
var ServiceProductPages = versionShellPages("commercial-service-products")

// CustomerContractPages 是 /commercial-customer-contracts 的查询声明。
var CustomerContractPages = versionShellPages("commercial-customer-contracts")

// SupplierAgreementPages 是 /commercial-supplier-agreements 的查询声明。
var SupplierAgreementPages = versionShellPages("commercial-supplier-agreements")

// 策略目录 /commercial-policies 按 kind 分派册子，每种各是一册（ADR-0144 决定四）：kind 保持册子选择器原义、登为
// Selectors，取值进游标摘要，拿一种的游标去翻另一种答游标不符。各册缺省序取分页之前的首列与方向：正文册按登记或声明时刻
// 倒序，以版本壳为行的那几种按发布时刻倒序（版本壳上没有登记时刻这一格）。各册另可按对象号排（声明册是它挂靠的合同或
// 规则包对象）：策略页同样按对象归组展示，判据同版本壳各册。筛选维只放有封闭词表的那一格；信用政策的权限等级与费用类型
// 是开放引用，不设筛选维。

// policyContentPages 是策略目录里自带版本与生效区间的正文册共用的声明形状：缺省按 at 倒序，另可按对象号排；行标识是对象
// 与版本（各表的 object_kind 由 CHECK 钉成一格，主键去掉租户与类别即是）。
func policyContentPages(kind, at string, filters ...cataloguepage.FilterDimension) *cataloguepage.Catalogue {
	return cataloguepage.MustCatalogue(cataloguepage.Spec{
		Name: "commercial-policies/" + kind,
		Sorts: []cataloguepage.SortDimension{
			{Name: at, Kind: cataloguepage.Instant},
			{Name: "objectId", Kind: cataloguepage.Text},
		},
		DefaultSort: cataloguepage.Sort{Field: at, Descending: true},
		Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
		Filters:     filters,
		Selectors:   []string{"kind"},
	})
}

// AcceptanceRulePackagePages 是 kind=ACCEPTANCE_RULE_PACKAGE 的查询声明。
var AcceptanceRulePackagePages = policyContentPages("ACCEPTANCE_RULE_PACKAGE", "declaredAt")

// PricePolicyPages 是 kind=PRICE_POLICY 的查询声明；direction 取自 domain.PriceDirection。
var PricePolicyPages = policyContentPages("PRICE_POLICY", "registeredAt",
	cataloguepage.FilterDimension{Name: "direction", Vocabulary: cataloguepage.Words[domain.PriceDirection]()})

// SettlementPolicyPages 是 kind=SETTLEMENT_POLICY 的查询声明；method 取自 domain.SettlementMethod。
var SettlementPolicyPages = policyContentPages("SETTLEMENT_POLICY", "registeredAt",
	cataloguepage.FilterDimension{Name: "method", Vocabulary: cataloguepage.Words[domain.SettlementMethod]()})

// CreditPolicyPages 是 kind=CREDIT_POLICY 的查询声明。
var CreditPolicyPages = policyContentPages("CREDIT_POLICY", "registeredAt")

// PreAcceptanceControlPages 是 kind=PRE_ACCEPTANCE_CONTROL 的查询声明。声明挂在客户合同版本下，没有自己的版本与生效区间：
// 行标识是合同对象与合同版本两段，另可按合同对象排；requirement 取自 domain.PreAcceptanceControlRequirement。
var PreAcceptanceControlPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-policies/PRE_ACCEPTANCE_CONTROL",
	Sorts: []cataloguepage.SortDimension{
		{Name: "declaredAt", Kind: cataloguepage.Instant},
		{Name: "contractObjectId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "declaredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "requirement", Vocabulary: cataloguepage.Words[domain.PreAcceptanceControlRequirement]()},
	},
	Selectors: []string{"kind"},
})

// AsOfPolicyPages 是 kind=AS_OF_POLICY 的查询声明。声明挂在接单规则包版本下、一版规则包每类判断一行：行标识是规则包对象、
// 规则包版本与判断类型三段，另可按规则包对象排；judgmentType 取自 domain.JudgmentType。
var AsOfPolicyPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-policies/AS_OF_POLICY",
	Sorts: []cataloguepage.SortDimension{
		{Name: "declaredAt", Kind: cataloguepage.Instant},
		{Name: "rulePackageObjectId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "declaredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "judgmentType", Vocabulary: cataloguepage.Words[domain.JudgmentType]()},
	},
	Selectors: []string{"kind"},
})

// AuthorizationRulePages 是 kind=AUTHORIZATION_RULE 的查询声明。
var AuthorizationRulePages = versionShellPages("commercial-policies/AUTHORIZATION_RULE", "kind")

// CustomerServiceRulePages 是 kind=CUSTOMER_SERVICE_RULE 的查询声明。
var CustomerServiceRulePages = versionShellPages("commercial-policies/CUSTOMER_SERVICE_RULE", "kind")

// PreAcceptanceFinancialControlPolicyPages 是 kind=PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY 的查询声明。
var PreAcceptanceFinancialControlPolicyPages = versionShellPages(
	"commercial-policies/PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY", "kind")

// ProductChannelMappingPages 是 /commercial-product-channel-mappings 的查询声明。映射没有独立状态代数（行上不导出状态，裁决在
// ProductChannelMappingRow），本册因此不设筛选维，收窄只靠 q。
var ProductChannelMappingPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-product-channel-mappings",
	Sorts: []cataloguepage.SortDimension{
		{Name: "registeredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "registeredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
})

// openDraftStatuses 是开放中载体 status 的词表：读面只列`待批准`与`已批准`，已发布的不在册，PUBLISHED 进词表只会让筛它恒答
// 空页，判据同 versionStatuses 不收 DRAFT。
var openDraftStatuses = []string{
	domain.PublicationDraftPendingApproval.String(),
	domain.PublicationDraftApproved.String(),
}

// OpenPublicationDraftPages 是 /commercial-open-publication-drafts 的查询声明。缺省按录入时刻倒序；行标识是版本身份去掉
// 租户的类别、对象与版本，类别在库上存领域码、按整数比。筛选维只有 status：待批准页按状态分看，不按类别收窄。本构建读不回
// 的载体照样在册（ADR-0344），可读与否是读面经重建门之后才知道的事、库上没有这一列，不作筛选维。
var OpenPublicationDraftPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "commercial-open-publication-drafts",
	Sorts: []cataloguepage.SortDimension{
		{Name: "submittedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "submittedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Integer, cataloguepage.Text, cataloguepage.Text},
	Filters:     []cataloguepage.FilterDimension{{Name: "status", Vocabulary: openDraftStatuses}},
})
