package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	commercialhttp "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/http"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// 商业上下文八类配置登记的生产装配（ADR-0085，票 admin-write-faces/02 切片 02c）。
// 三族各一个包装类型，与传输层的三个 Registrar 契约一一对应：发布族、参与方身份族
// （四类登记加停用）、产品渠道族（形态与映射）。运营操作者面发布路径（预览 + 载体三口，
// ADR-0126，票 admin-write-faces/08）是第五族，形同前四族。
//
// 一族一个包装而不是一格一个：同族的方法各自具名（不是同名 `Handle`），一个类型装得下
// 全族，且传输层本就按族收一个 Registrar——拆成八个只会把同一段事务壳抄八遍。这与关务
// 那四格拆四个类型不矛盾：那边四个 Registrar 契约的方法同名，一个类型实现不了四个。
//
// 事务边界与 transactionalPriceCardRegistration 同源：登记册写口按框架合同无事务即拒，
// 一次调用一笔事务——登记与它的修订连续性读回因此看同一份快照（形照受控 CLI 的 execute，
// 那里也是逐命令各起一笔事务，批不是聚合，AT-PC-011）。用例交回业务答案（含重放与治理
// 答案）时提交，返回错误时整笔回滚，端点按 ADR-0022 答「没形成答案」。

// commercialInTransaction 是三族共用的事务壳。泛型按结果类型实例化——三族的答案代数
// 互不相同（发布多出未决与声明落点，身份多出已停用与未找到），共用的只是「开事务、
// 把答案带出来、出错整笔回滚」这一段。
func commercialInTransaction[Result any](
	ctx context.Context,
	transactor bentoapp.Transactor,
	register func(context.Context) (Result, error),
) (Result, error) {
	var result Result
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		registered, registerErr := register(txCtx)
		if registerErr != nil {
			return registerErr
		}
		result = registered
		return nil
	})
	if err != nil {
		var none Result
		return none, err
	}
	return result, nil
}

type transactionalCommercialPublication struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.PublishCommercialAuthorityHandler
}

var _ commercialhttp.CommercialAuthorityPublisher = transactionalCommercialPublication{}

func (publication transactionalCommercialPublication) Handle(
	ctx context.Context,
	command commercialapp.PublishCommercialAuthorityCommand,
) (commercialapp.PublishCommercialAuthorityResult, error) {
	return commercialInTransaction(ctx, publication.transactor,
		func(txCtx context.Context) (commercialapp.PublishCommercialAuthorityResult, error) {
			return publication.inner.Handle(txCtx, command)
		})
}

type transactionalPartyIdentityRegistration struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.RegisterPartyIdentityHandler
}

var _ commercialhttp.PartyIdentityRegistrar = transactionalPartyIdentityRegistration{}

func (registration transactionalPartyIdentityRegistration) RegisterBusinessParty(
	ctx context.Context,
	command commercialapp.RegisterBusinessPartyCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return registration.inner.RegisterBusinessParty(txCtx, command)
		})
}

func (registration transactionalPartyIdentityRegistration) RegisterLegalEntity(
	ctx context.Context,
	command commercialapp.RegisterLegalEntityCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return registration.inner.RegisterLegalEntity(txCtx, command)
		})
}

func (registration transactionalPartyIdentityRegistration) RegisterCustomerAccount(
	ctx context.Context,
	command commercialapp.RegisterCustomerAccountCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return registration.inner.RegisterCustomerAccount(txCtx, command)
		})
}

func (registration transactionalPartyIdentityRegistration) RegisterRelationship(
	ctx context.Context,
	command commercialapp.RegisterPartyRelationshipCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return registration.inner.RegisterRelationship(txCtx, command)
		})
}

func (registration transactionalPartyIdentityRegistration) Deactivate(
	ctx context.Context,
	command commercialapp.DeactivatePartyIdentityCommand,
) (commercialapp.PartyRegistryResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.PartyRegistryResult, error) {
			return registration.inner.Deactivate(txCtx, command)
		})
}

type transactionalProductChannelRegistration struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.RegisterProductChannelHandler
}

var _ commercialhttp.ProductChannelRegistrar = transactionalProductChannelRegistration{}

func (registration transactionalProductChannelRegistration) RegisterServiceProductForm(
	ctx context.Context,
	command commercialapp.RegisterServiceProductFormCommand,
) (commercialapp.ProductChannelResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.ProductChannelResult, error) {
			return registration.inner.RegisterServiceProductForm(txCtx, command)
		})
}

func (registration transactionalProductChannelRegistration) RegisterMapping(
	ctx context.Context,
	command commercialapp.RegisterProductChannelMappingCommand,
) (commercialapp.ProductChannelResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.ProductChannelResult, error) {
			return registration.inner.RegisterMapping(txCtx, command)
		})
}

// transactionalRegistrationNumberTypeRegistration 是注册号类型目录族（票 legal-entity-profile/01）的
// 事务壳：登记修订与停用各一笔事务，登记与它的修订连续性读回看同一份快照。
type transactionalRegistrationNumberTypeRegistration struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.RegisterRegistrationNumberTypeHandler
}

var _ commercialhttp.RegistrationNumberTypeRegistrar = transactionalRegistrationNumberTypeRegistration{}

func (registration transactionalRegistrationNumberTypeRegistration) Register(
	ctx context.Context,
	command commercialapp.RegisterRegistrationNumberTypeCommand,
) (commercialapp.RegistrationNumberTypeResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.RegistrationNumberTypeResult, error) {
			return registration.inner.Register(txCtx, command)
		})
}

func (registration transactionalRegistrationNumberTypeRegistration) Deactivate(
	ctx context.Context,
	command commercialapp.DeactivateRegistrationNumberTypeCommand,
) (commercialapp.RegistrationNumberTypeResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.RegistrationNumberTypeResult, error) {
			return registration.inner.Deactivate(txCtx, command)
		})
}

// commercialRegistrationOrchestration 收拢三族，供装配点一次取回。
type transactionalChannelAccountUse struct {
	transactor bentoapp.Transactor
	inner      *commercialapp.RegisterChannelAccountUseHandler
}

var _ commercialhttp.ChannelAccountUseRegistrar = transactionalChannelAccountUse{}

func (registration transactionalChannelAccountUse) Register(
	ctx context.Context,
	command commercialapp.RegisterChannelAccountUseCommand,
) (commercialapp.ChannelAccountUseResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.ChannelAccountUseResult, error) {
			return registration.inner.Register(txCtx, command)
		})
}

// Revoke 与 Register 同样包一层事务：撤销走的是登记册的同一个只增写口，把前一修订读回来再
// 追加一条，读与写必须在同一个事务里——否则两次并发撤销会各自读到同一个最新修订，各自追加，
// 而登记册的主键只挡得住同修订号的第二条，挡不住两条号相同内容不同的竞态在应用层就已分岔。
func (registration transactionalChannelAccountUse) Revoke(
	ctx context.Context,
	command commercialapp.RevokeChannelAccountUseCommand,
) (commercialapp.ChannelAccountUseResult, error) {
	return commercialInTransaction(ctx, registration.transactor,
		func(txCtx context.Context) (commercialapp.ChannelAccountUseResult, error) {
			return registration.inner.Revoke(txCtx, command)
		})
}

// transactionalPublicationDrafts 是运营操作者面发布路径三口的事务壳（ADR-0126 Decision 三）：录入、批准各一笔；
// 发布那一笔把载体用例与它转交的受控发布用例包在同一事务里——版本、声明与载体状态同笔落库，不留「版本发了、
// 载体还写着已批准」的半份。三个用例各自具名，一个类型装得下全族，与 PublicationDraftOperator 契约一一对应。
type transactionalPublicationDrafts struct {
	transactor bentoapp.Transactor
	submit     *commercialapp.SubmitPublicationDraftHandler
	approve    *commercialapp.ApprovePublicationDraftHandler
	publish    *commercialapp.PublishPublicationDraftHandler
}

var _ commercialhttp.PublicationDraftOperator = transactionalPublicationDrafts{}

func (drafts transactionalPublicationDrafts) Submit(
	ctx context.Context,
	command commercialapp.SubmitPublicationDraftCommand,
) (commercialapp.SubmitPublicationDraftResult, error) {
	return commercialInTransaction(ctx, drafts.transactor,
		func(txCtx context.Context) (commercialapp.SubmitPublicationDraftResult, error) {
			return drafts.submit.Handle(txCtx, command)
		})
}

func (drafts transactionalPublicationDrafts) Approve(
	ctx context.Context,
	command commercialapp.ApprovePublicationDraftCommand,
) (commercialapp.ApprovePublicationDraftResult, error) {
	return commercialInTransaction(ctx, drafts.transactor,
		func(txCtx context.Context) (commercialapp.ApprovePublicationDraftResult, error) {
			return drafts.approve.Handle(txCtx, command)
		})
}

func (drafts transactionalPublicationDrafts) Publish(
	ctx context.Context,
	command commercialapp.PublishPublicationDraftCommand,
) (commercialapp.PublishPublicationDraftResult, error) {
	return commercialInTransaction(ctx, drafts.transactor,
		func(txCtx context.Context) (commercialapp.PublishPublicationDraftResult, error) {
			return drafts.publish.Handle(txCtx, command)
		})
}

type commercialRegistrationOrchestration struct {
	publication       transactionalCommercialPublication
	partyIdentity     transactionalPartyIdentityRegistration
	productChannel    transactionalProductChannelRegistration
	channelAccountUse transactionalChannelAccountUse
	// 注册号类型目录族（ADR-0145 决定一）：登记修订与停用两口共用这一个事务壳。
	registrationNumberType transactionalRegistrationNumberTypeRegistration
	// publicationPreview 没有事务壳：预览不读也不写（ADR-0126 Decision 四）。
	publicationPreview *commercialapp.PreviewCommercialPublicationHandler
	publicationDrafts  transactionalPublicationDrafts
}

// buildCommercialRegistrationOrchestration 装配八个 `/commercial-*` 写面的真编排。
// 接真不等墙降，判据同价卡首切片；形照 `parcel-commercial` 的同一份装配——发布登记册
// 一只（形态册挂在它的形态切面上，ADR-0050），映射登记册一只，身份登记册一只。
func buildCommercialRegistrationOrchestration(db *bentopg.DB) (commercialRegistrationOrchestration, error) {
	none := commercialRegistrationOrchestration{}

	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: commercial publication registry: %w", err)
	}
	mappings, err := pcpostgres.NewProductChannelMappings(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: product channel mapping registry: %w", err)
	}
	identities, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: party identity registry: %w", err)
	}
	channelAccountUse, err := pcpostgres.NewChannelAccountUseAuthorizations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: channel account use authorization registry: %w", err)
	}
	registrationNumberTypes, err := pcpostgres.NewRegistrationNumberTypes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: registration number type registry: %w", err)
	}
	// 「参数已登记」续办信封（ADR-0094 决定四，票 first-tenant-runway/07 D4）：发布编排在声明落库的
	// 同一事务里经 Outbox 发出，parcel-shipment 的消费门凭它重驱停在`等待运营登记`的委托。挂在
	// 发布处理器上而不是另立一层边界壳：时点策略是随发布同事务写的，交接跟着那一次写走。
	store, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: commercial outbox store: %w", err)
	}
	registrationHandoff, err := pcpostgres.NewOutboxOperatorRegistrationCompletedHandoff(db, store, systemClock{})
	if err != nil {
		return none, fmt.Errorf("parcel-api: operator registration completed handoff: %w", err)
	}

	// 运营操作者面发布路径（ADR-0126 Decision 三、四）：载体册与审批职责规则册各一只适配器；发布用例是**同一个**
	// PublishCommercialAuthorityHandler 实例——受控批文口与载体发布口消费的是同一份编排，不各造一份。
	drafts, err := pcpostgres.NewPublicationDrafts(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: publication draft registry: %w", err)
	}
	approvalDutyRules, err := pcpostgres.NewApprovalDutyRules(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: approval duty rule registry: %w", err)
	}

	transactor := db.Transactor()
	publisher := commercialapp.NewPublishCommercialAuthorityHandler(publications, systemClock{}, registrationHandoff)
	return commercialRegistrationOrchestration{
		publication: transactionalCommercialPublication{
			transactor: transactor,
			inner:      publisher,
		},
		publicationPreview: commercialapp.NewPreviewCommercialPublicationHandler(),
		publicationDrafts: transactionalPublicationDrafts{
			transactor: transactor,
			submit:     commercialapp.NewSubmitPublicationDraftHandler(drafts, systemClock{}),
			approve:    commercialapp.NewApprovePublicationDraftHandler(drafts, approvalDutyRules, systemClock{}),
			publish:    commercialapp.NewPublishPublicationDraftHandler(drafts, publisher, systemClock{}),
		},
		partyIdentity: transactionalPartyIdentityRegistration{
			transactor: transactor,
			inner:      commercialapp.NewRegisterPartyIdentityHandler(identities, registrationNumberTypes),
		},
		productChannel: transactionalProductChannelRegistration{
			transactor: transactor,
			inner:      commercialapp.NewRegisterProductChannelHandler(publications, mappings),
		},
		channelAccountUse: transactionalChannelAccountUse{
			transactor: transactor,
			inner:      commercialapp.NewRegisterChannelAccountUseHandler(channelAccountUse),
		},
		registrationNumberType: transactionalRegistrationNumberTypeRegistration{
			transactor: transactor,
			inner:      commercialapp.NewRegisterRegistrationNumberTypeHandler(registrationNumberTypes),
		},
	}, nil
}
