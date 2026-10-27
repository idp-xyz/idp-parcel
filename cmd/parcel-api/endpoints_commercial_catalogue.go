package main

import (
	"net/http"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type commercialCatalogueWiring struct {
	serviceProducts            commercialhttp.ServiceProductCatalogueReader
	commercialPolicies         commercialhttp.CommercialPolicyCatalogueReader
	commercialRelations        commercialhttp.CommercialRelationCatalogueReader
	partyIdentities            commercialhttp.PartyIdentityCatalogueReader
	customerAccounts           commercialhttp.CustomerAccountCatalogueReader
	productChannelMappings     commercialhttp.ProductChannelCatalogueReader
	registrationNumberTypes    commercialhttp.RegistrationNumberTypeCatalogueReader
	publicationDrafts          commercialhttp.PublicationDraftReader
	legalEntityProfiles        commercialhttp.LegalEntityProfileRevisionHistoryReader
	legalEntityProfileResolver commercialhttp.LegalEntityProfileResolver
}

func init() {
	registerEndpoints(endpointFeature[commercialCatalogueWiring]{
		lines: map[string]endpointLine{
			"/commercial-service-products":    {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-policies":            {method: http.MethodGet, face: operationsReadFace(), probe: "/commercial-policies?kind=ACCEPTANCE_RULE_PACKAGE"},
			"/commercial-customer-contracts":  {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-supplier-agreements": {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-business-parties":    {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-business-parties/{partyId}/revisions": {
				method: http.MethodGet, face: operationsReadFace(), probe: "/commercial-business-parties/SYN-PARTY-01/revisions",
			},
			"/commercial-group-legal-entities": {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-group-legal-entities/{legalEntityId}/revisions": {
				method: http.MethodGet, face: operationsReadFace(), probe: "/commercial-group-legal-entities/SYN-LE-01/revisions",
			},
			"/commercial-group-legal-entities/{legalEntityId}/profile-revisions": {
				method: http.MethodGet, face: operationsReadFace(), probe: "/commercial-group-legal-entities/SYN-LE-01/profile-revisions",
			},
			"/commercial-group-legal-entities/{legalEntityId}/profile-resolution": {
				method: http.MethodGet, face: operationsReadFace(),
				probe: "/commercial-group-legal-entities/SYN-LE-01/profile-resolution?at=2026-09-24T00:00:00Z",
			},
			"/commercial-party-relationships":      {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-customer-accounts":        {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-product-channel-mappings": {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-open-publication-drafts":  {method: http.MethodGet, face: operationsReadFace()},
			"/commercial-publication-draft-readbacks/{kind}/{objectId}/{version}": {
				method: http.MethodGet, face: operationsReadFace(), probe: "/commercial-publication-draft-readbacks/CREDIT_POLICY/SYN-CREDIT-01/v1",
			},
			"/commercial-product-channel-mappings/{mappingId}/revisions": {
				method: http.MethodGet, face: operationsReadFace(), probe: "/commercial-product-channel-mappings/SYN-MAP-01/revisions",
			},
			"/commercial-registration-number-types": {method: http.MethodGet, face: operationsReadFace()},
		},
		build: func(sources endpointSources) (commercialCatalogueWiring, error) {
			none := commercialCatalogueWiring{}
			commercialCatalog, err := pcpostgres.NewOperationsCatalogue(sources.db)
			if err != nil {
				return none, err
			}
			// 法人资料修订历史读口在资料登记册适配器上，不在商业目录适配器上（LegalEntityProfiles 同时实现写口与两个读口）。
			// 写编排另构造自己的一只：适配器只包着 db、不带状态，两只读写同一张表。
			legalEntityProfiles, err := pcpostgres.NewLegalEntityProfiles(sources.db)
			if err != nil {
				return none, err
			}
			// 法人资料按时点解析（票 legal-entity-profile/05）交入的是应用读用例：它读法人最新修订与整条资料修订链，交领域判。
			// 法人那一侧读身份登记册——资料的写入与解析都经窄口 LegalEntityRegistrationLookup 看法人，同一只适配器。
			partyIdentityRegistrations, err := pcpostgres.NewPartyIdentityRegistrations(sources.db)
			if err != nil {
				return none, err
			}
			publicationDrafts, err := pcpostgres.NewPublicationDrafts(sources.db)
			if err != nil {
				return none, err
			}
			return commercialCatalogueWiring{
				publicationDrafts:          publicationDrafts,
				serviceProducts:            commercialCatalog,
				commercialPolicies:         commercialCatalog,
				commercialRelations:        commercialCatalog,
				partyIdentities:            commercialCatalog,
				customerAccounts:           commercialCatalog,
				productChannelMappings:     commercialCatalog,
				registrationNumberTypes:    commercialCatalog,
				legalEntityProfiles:        legalEntityProfiles,
				legalEntityProfileResolver: commercialapp.NewResolveLegalEntityProfileHandler(legalEntityProfiles, partyIdentityRegistrations),
			}, nil
		},
		mount: func(channels endpointChannels, wiring commercialCatalogueWiring) []httpapi.BusinessEndpoint {
			commercialCatalogueIntake := channels.commercialCatalogue()
			return []httpapi.BusinessEndpoint{
				{Pattern: "/commercial-service-products", Handler: commercialhttp.NewQueryServiceProductsEndpoint(commercialCatalogueIntake, wiring.serviceProducts)},
				{Pattern: "/commercial-policies", Handler: commercialhttp.NewQueryCommercialPoliciesEndpoint(commercialCatalogueIntake, wiring.commercialPolicies)},
				{Pattern: "/commercial-customer-contracts", Handler: commercialhttp.NewQueryCustomerContractsEndpoint(commercialCatalogueIntake, wiring.commercialRelations)},
				{Pattern: "/commercial-supplier-agreements", Handler: commercialhttp.NewQuerySupplierAgreementsEndpoint(commercialCatalogueIntake, wiring.commercialRelations)},
				// 参与方身份两册（票 admin-remainder-mechanism-batch/01）各立入口，不并进上面
				// 载体目录的读口：载体是版本化商业对象，身份与关系是登记→生效→停用的生命周期，
				// 行形状与状态代数不同（裁决在 ports.PartyIdentityCatalogueRead 注释）。
				// 身份本体那一册与法人、关系两册各立入口（票 admin-remainder-mechanism-batch/01
				// 的补格裁定）：一个参与方既可以不是法人、也可以不在任何关系里，被停用的那种恰恰
				// 如此；不给它自己的入口，身份生命周期的`已登记`与`已停用`两格在管理台就没有实例
				// 可显，而那正是本票标题那个生命周期。三口共用同一个读口参数与同一个 Intake 变量。
				{Pattern: "/commercial-business-parties", Handler: commercialhttp.NewQueryBusinessPartiesEndpoint(commercialCatalogueIntake, wiring.partyIdentities)},
				// 业务参与方修订历史（票 admin-web-group-legal-entities/12）：参与方册那一页详情抽屉的供数面，按路径里的参与方展开
				// 整条修订链。同读口参数、同 Intake 变量，裁决与下面法人册那一行一字不改（在 ports.BusinessPartyRevisionHistoryRead）；
				// {partyId} 由 chi 填进 PathValue，端点按同名取。
				{Pattern: "/commercial-business-parties/{partyId}/revisions", Handler: commercialhttp.NewQueryBusinessPartyRevisionsEndpoint(commercialCatalogueIntake, wiring.partyIdentities)},
				{Pattern: "/commercial-group-legal-entities", Handler: commercialhttp.NewQueryGroupLegalEntitiesEndpoint(commercialCatalogueIntake, wiring.partyIdentities)},
				// 责任法人修订历史（票 admin-web-group-legal-entities/03）：法人册那一页详情抽屉的供数面，按路径里的法人展开
				// 整条修订链。同读口参数、同 Intake 变量（读同一张表、同一租户作用域、供同一页——裁决在 ports.LegalEntityRevisionHistoryRead）；
				// 挂操作者查阅读（票 operator-channel/05）。{legalEntityId} 由 chi 填进 PathValue，端点按同名取。
				{Pattern: "/commercial-group-legal-entities/{legalEntityId}/revisions", Handler: commercialhttp.NewQueryLegalEntityRevisionsEndpoint(commercialCatalogueIntake, wiring.partyIdentities)},
				// 法人资料修订历史（ADR-0145 决定三，票 legal-entity-profile/03）：法人资料页的供数面，按路径里的法人展开资料修订链。
				// 同 Intake 变量，挂操作者查阅读（票 operator-channel/05）；读口参数另立——资料修订在自己的登记册上，
				// 生产装配交入的是资料登记册适配器而不是商业目录适配器。本口只交修订事实，此刻有效的是哪一笔由按时点解析回答。
				{Pattern: "/commercial-group-legal-entities/{legalEntityId}/profile-revisions", Handler: commercialhttp.NewQueryLegalEntityProfileRevisionsEndpoint(commercialCatalogueIntake, wiring.legalEntityProfiles)},
				// 法人资料按时点解析（ADR-0145 决定五、六，票 legal-entity-profile/05）：给定时点答那一刻有效的是哪一笔、能不能开立。
				// 同 Intake 变量，挂操作者查阅读（票 operator-channel/05）；第二参是应用读用例而不是读口——哪一笔有效只能由
				// 领域对时点判出，在装配点预先挑一遍就等于为同一判断立第二个口径。
				{Pattern: "/commercial-group-legal-entities/{legalEntityId}/profile-resolution", Handler: commercialhttp.NewQueryLegalEntityProfileResolutionEndpoint(commercialCatalogueIntake, wiring.legalEntityProfileResolver)},
				{Pattern: "/commercial-party-relationships", Handler: commercialhttp.NewQueryPartyRelationshipsEndpoint(commercialCatalogueIntake, wiring.partyIdentities)},
				// 货主客户账户册（票 admin-write-faces/04）——身份三级里的第三级，独立入口且独立读口
				// 参数：它按 CONTEXT 落在客户与合同页而不是上面两册所在的页（一页一入口），读口跟着
				// 页走；不并进 /commercial-customer-contracts，合同是商业版本、账户是身份登记修订，
				// 状态代数不同（裁决在 ports.CustomerAccountCatalogueRead 注释）。生产装配交入的仍是
				// 同一只商业目录适配器。
				{Pattern: "/commercial-customer-accounts", Handler: commercialhttp.NewQueryCustomerAccountsEndpoint(commercialCatalogueIntake, wiring.customerAccounts)},
				// 产品—渠道映射册（票 admin-remainder-mechanism-batch/02）独立入口，不并进
				// /commercial-service-products：那边上列版本壳，这边上列登记册信封（产品×渠道
				// ×区间的修订），行形状与修订轴不同（裁决在 ports.ProductChannelMappingCatalogueRead）。
				{Pattern: "/commercial-product-channel-mappings", Handler: commercialhttp.NewQueryProductChannelMappingsEndpoint(commercialCatalogueIntake, wiring.productChannelMappings)},
				// 待批准发布载体的读回（ADR-0314 决定一）：列表与单份读回各立入口，挂操作者查阅读，
				// 与批准、发布两个写口各走各的授予面。读的是载体册适配器（同一张表的写口在登记写面上另装配）。
				{Pattern: "/commercial-open-publication-drafts", Handler: commercialhttp.NewQueryPublicationDraftsEndpoint(commercialCatalogueIntake, wiring.publicationDrafts)},
				{Pattern: "/commercial-publication-draft-readbacks/{kind}/{objectId}/{version}", Handler: commercialhttp.NewQueryPublicationDraftEndpoint(commercialCatalogueIntake, wiring.publicationDrafts)},
				// 映射修订历史（票 party-commercial-uiux-backend/02）：渠道产品目录页详情抽屉「历史」栏的供数面，按路径里的映射展开整条修订链。
				// 同读口参数、同 Intake 变量，裁决与业务参与方修订历史那一行一字不改（在 ports.ProductChannelMappingRevisionHistoryRead）；
				// {mappingId} 由路由填进 PathValue，端点按同名取。
				{Pattern: "/commercial-product-channel-mappings/{mappingId}/revisions", Handler: commercialhttp.NewQueryProductChannelMappingRevisionsEndpoint(commercialCatalogueIntake, wiring.productChannelMappings)},
				// 注册号类型目录（ADR-0145 决定一，票 legal-entity-profile/01）：按注册国家 / 地区登记的
				// 类型、格式与所属层的最新修订。判号不走本口，走 ports.RegistrationNumberTypeLookup。
				{Pattern: "/commercial-registration-number-types", Handler: commercialhttp.NewQueryRegistrationNumberTypesEndpoint(commercialCatalogueIntake, wiring.registrationNumberTypes)},
			}
		},
	})
}
