package main

import (
	"net/http"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	"go.idp.xyz/idp-parcel/internal/platform/httpapi"
)

type commercialRegistrationWiring struct {
	commercialPublication              commercialhttp.CommercialAuthorityPublisher
	commercialPublicationPreview       commercialhttp.CommercialPublicationPreviewer
	commercialPublicationDrafts        commercialhttp.PublicationDraftOperator
	partyIdentityRegistration          commercialhttp.PartyIdentityRegistrar
	productChannelRegistration         commercialhttp.ProductChannelRegistrar
	registrationNumberTypeRegistration commercialhttp.RegistrationNumberTypeRegistrar
	legalEntityProfileRegistration     commercialhttp.LegalEntityProfileRegistrar
	channelAccountUseRegistration      commercialhttp.ChannelAccountUseRegistrar
}

func init() {
	registerEndpoints(endpointFeature[commercialRegistrationWiring]{
		lines: map[string]endpointLine{
			"/commercial-publications":                   {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-publication-previews":           {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-publication-drafts":             {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-publication-draft-approvals":    {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-publication-draft-publications": {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-publication-vocabularies": {
				method: http.MethodGet, face: registryWriteFace(), probe: "/commercial-publication-vocabularies?kind=ACCEPTANCE_RULE_PACKAGE",
			},
			"/commercial-business-party-registrations":           {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-legal-entity-registrations":             {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-customer-account-registrations":         {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-party-relationship-registrations":       {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-party-identity-deactivations":           {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-service-product-form-registrations":     {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-product-channel-mapping-registrations":  {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-registration-number-type-registrations": {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-registration-number-type-deactivations": {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-legal-entity-profile-registrations":     {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-channel-account-use-registrations":      {method: http.MethodPost, face: registryWriteFace()},
			"/commercial-channel-account-use-revocations":        {method: http.MethodPost, face: registryWriteFace()},
		},
		build: func(sources endpointSources) (commercialRegistrationWiring, error) {
			commercialRegistration, err := buildCommercialRegistrationOrchestration(sources.db)
			return commercialRegistrationWiring{
				commercialPublication:              commercialRegistration.publication,
				commercialPublicationPreview:       commercialRegistration.publicationPreview,
				commercialPublicationDrafts:        commercialRegistration.publicationDrafts,
				partyIdentityRegistration:          commercialRegistration.partyIdentity,
				productChannelRegistration:         commercialRegistration.productChannel,
				registrationNumberTypeRegistration: commercialRegistration.registrationNumberType,
				legalEntityProfileRegistration:     commercialRegistration.legalEntityProfile,
				channelAccountUseRegistration:      commercialRegistration.channelAccountUse,
			}, err
		},
		mount: func(channels endpointChannels, wiring commercialRegistrationWiring) []httpapi.BusinessEndpoint {
			return []httpapi.BusinessEndpoint{
				// 商业八类配置写面（ADR-0085，票 admin-write-faces/02 切片 02c）：写准入不另立形，
				// 判据同网络目录登记写面——挂操作者渠道的登记册 Intake，查阅行的 Intake 变量换不了写行。
				//
				// 发布口不带 `-registrations` 后缀：本上下文这一格的动词是发布，答案代数说的也是
				// 发布（`已发布已生效`/`已计划生效`），叫成登记会与身份、映射两族的登记答案混为
				// 一谈（裁决在端点文件头）。它也只有一个端点——服务产品、规则包、合同、协议与各类
				// 策略是同一个发布用例的输入，类别在版本规格里，不是另一种命令。
				//
				// 停用口叫 `-deactivations` 而不是 `-registrations`：它是往修订链上插一笔新修订的
				// 状态推进，不是登记一个新身份。名字照实说，是因为「登记」与「停用」在这本册上的
				// 续办动作不同，路径是登记方看见的第一样东西。
				//
				// 它收受控批文的在线镜像，挂操作者渠道的登记册 Intake（票 operator-channel/18）：外壳与逐项翻译就是受控 CLI
				// publish 那一份（registrationjson），租户取认证出的身份、项上的 tenantId 在场即拒。批准三格与角色确认照旧从批文
				// 取，不从信封来——操作者自己的批准路径是下面的载体（ADR-0126 决定三）。
				{Pattern: "/commercial-publications", Handler: commercialhttp.NewPublishCommercialAuthorityEndpoint(channels.operatorRegistries.commercial, wiring.commercialPublication)},
				// 运营操作者面发布路径四口（ADR-0126 Decision 三、四；票 admin-write-faces/08）：预览 + 载体录入 / 批准 / 发布。
				// 四口挂操作者渠道的登记册 Intake（同一 ADR 决定五，票 operator-channel/06）：租户、录入者与批准者（主体引用 +
				// 授予集）都取认证出的身份。预览不落库，但拟录的壳要信封里的租户才立得住，所以与三个命令口同一个 Intake、不走
				// 查阅行的 Intake 变量。**批准那一口比其余更不能松**：批准者是审批
				// 职责规则的一半，任何采信自报身份的 Intake 都等于把那道门拆了。发布口不并进 `/commercial-publications`：那一口
				// 收受控批文（声明摘要 + 批准三格），这一口收载体引用，两口消费的用例不同。
				{Pattern: "/commercial-publication-previews", Handler: commercialhttp.NewPreviewCommercialPublicationEndpoint(channels.operatorRegistries.commercial, wiring.commercialPublicationPreview)},
				{Pattern: "/commercial-publication-drafts", Handler: commercialhttp.NewSubmitPublicationDraftEndpoint(channels.operatorRegistries.commercial, wiring.commercialPublicationDrafts)},
				{Pattern: "/commercial-publication-draft-approvals", Handler: commercialhttp.NewApprovePublicationDraftEndpoint(channels.operatorRegistries.commercial, wiring.commercialPublicationDrafts)},
				{Pattern: "/commercial-publication-draft-publications", Handler: commercialhttp.NewPublishPublicationDraftEndpoint(channels.operatorRegistries.commercial, wiring.commercialPublicationDrafts)},
				// 商业发布词表读口（票 admin-write-faces/20）：按 kind 答该册正文各封闭集的码，表单据此供下拉。它不要租户，
				// 却与四口同挂操作者渠道的登记册 Intake（ADR-0151 决定五），理由不是预览口那条：它唯一的消费者是四口喂的表单，
				// 四口开不了时它单独开只让一张提交不了的表单多几行下拉；且它不读任何存储读面，不是运营查阅，不挂查阅行的
				// commercialCatalogueIntake。
				{Pattern: "/commercial-publication-vocabularies", Handler: commercialhttp.NewQueryPublicationVocabularyEndpoint(channels.operatorRegistries.commercial)},
				// 身份族四类登记加停用挂操作者渠道的登记册 Intake（ADR-0100 决定四，票 operator-channel/04）：租户取认证出的身份，
				// 写开关换不了这几行（ADR-0150 决定三）。
				{Pattern: "/commercial-business-party-registrations", Handler: commercialhttp.NewRegisterBusinessPartyEndpoint(channels.operatorRegistries.commercial, wiring.partyIdentityRegistration)},
				{Pattern: "/commercial-legal-entity-registrations", Handler: commercialhttp.NewRegisterLegalEntityEndpoint(channels.operatorRegistries.commercial, wiring.partyIdentityRegistration)},
				{Pattern: "/commercial-customer-account-registrations", Handler: commercialhttp.NewRegisterCustomerAccountEndpoint(channels.operatorRegistries.commercial, wiring.partyIdentityRegistration)},
				{Pattern: "/commercial-party-relationship-registrations", Handler: commercialhttp.NewRegisterPartyRelationshipEndpoint(channels.operatorRegistries.commercial, wiring.partyIdentityRegistration)},
				{Pattern: "/commercial-party-identity-deactivations", Handler: commercialhttp.NewDeactivatePartyIdentityEndpoint(channels.operatorRegistries.commercial, wiring.partyIdentityRegistration)},
				{Pattern: "/commercial-service-product-form-registrations", Handler: commercialhttp.NewRegisterServiceProductFormEndpoint(channels.operatorRegistries.commercial, wiring.productChannelRegistration)},
				{Pattern: "/commercial-product-channel-mapping-registrations", Handler: commercialhttp.NewRegisterProductChannelMappingEndpoint(channels.operatorRegistries.commercial, wiring.productChannelRegistration)},
				// 注册号类型目录两口（票 legal-entity-profile/01）：登记修订与停用，停用是修订链上的新一笔，
				// 同参与方身份族取 `-deactivations`。
				{Pattern: "/commercial-registration-number-type-registrations", Handler: commercialhttp.NewRegisterRegistrationNumberTypeEndpoint(channels.operatorRegistries.commercial, wiring.registrationNumberTypeRegistration)},
				{Pattern: "/commercial-registration-number-type-deactivations", Handler: commercialhttp.NewDeactivateRegistrationNumberTypeEndpoint(channels.operatorRegistries.commercial, wiring.registrationNumberTypeRegistration)},
				// 法人资料登记（ADR-0145 决定三，票 legal-entity-profile/03）只有登记修订一口：资料没有停用，新修订是往修订链上插
				// 一笔。与身份族挂同一个操作者渠道 Intake（票 operator-channel/04）；写开关换不了它（ADR-0150 决定三）。
				{Pattern: "/commercial-legal-entity-profile-registrations", Handler: commercialhttp.NewRegisterLegalEntityProfileEndpoint(channels.operatorRegistries.commercial, wiring.legalEntityProfileRegistration)},
				// 渠道账号使用授权两口（ADR-0093）。撤销不叫 `-deactivations` 也不走 DELETE：它是往
				// 修订链上追加一条终止事实，册上那一行不会消失，而那两个名字都会让登记方以为会。
				// 分两口而不带动作字段的理由在端点族注释里：合一口之后载荷里会同时躺着动作与授权
				// 正文，等于把「后继修订不得改换账号或双方」那道门要挡的机会又递回给调用方。
				{Pattern: "/commercial-channel-account-use-registrations", Handler: commercialhttp.NewRegisterChannelAccountUseEndpoint(channels.operatorRegistries.commercial, wiring.channelAccountUseRegistration)},
				{Pattern: "/commercial-channel-account-use-revocations", Handler: commercialhttp.NewRevokeChannelAccountUseEndpoint(channels.operatorRegistries.commercial, wiring.channelAccountUseRegistration)},
			}
		},
	})
}
