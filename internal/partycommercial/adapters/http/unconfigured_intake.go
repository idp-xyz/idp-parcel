package commercialhttp

import (
	"context"
	"errors"
	"net/http"

	"go.idp.xyz/idp-parcel/internal/partycommercial/application"
)

// ErrAccessChannelNotConfigured 表示当前没有任何已启用的接入渠道:运营接入认证属
// 接入渠道实例半边、`PAR-INT-01` 未登记,装配点上还没有一行真渠道 Intake
// (ADR-0055、ADR-0077 Decision 三)。
//
// 恢复动作判据同 ADR-0029:这一格要接入方去提供并配置渠道参数,改请求或重试都不会
// 好。哨兵只此一个而不随端点分设:未配置是渠道这一层的状态,按端点分设哨兵会让装配
// 点看起来能只配一半。本包据以回 403 + ACCESS_CHANNEL_NOT_CONFIGURED;折进 404 会与
// 「产品没有这个能力」不可分辨,折进 INTAKE_FAILED(5xx)会让客户端把一件人不来配就
// 永远不会好的事留队重发。
var ErrAccessChannelNotConfigured = errors.New("party commercial http: access channel is not configured")

// codeAccessChannelNotConfigured 命名状态,不命名参数(ADR-0055):这里等的是哪个
// 登记册行由参数登记册说,错误码只说「渠道未配置」。
const codeAccessChannelNotConfigured = "ACCESS_CHANNEL_NOT_CONFIGURED"

// UnconfiguredIntake 是「接入渠道未配置」的如实答复:对每个请求不读业务内容、不采信
// 任何自报身份、不构造查询,一律交回 ErrAccessChannelNotConfigured。
//
// 它不是被禁的「开发用」采信实现——那条红线禁的是采信自报租户(穿透 ADR-0003 的
// 隔离);本类型恰是其反面,分界同 ADR-0052:「读一个空登记册并如实答未配置不是默认
// 实现,恰恰是它想保护的东西」。这里的空登记册就是装配点本身。真渠道就位时在装配点
// 替换,本类型随之退场。
type UnconfiguredIntake struct{}

var _ CommercialCatalogueIntake = UnconfiguredIntake{}

// IntakeCatalogueQuery 不读请求。参数刻意匿名:连签名都不给「读一眼再决定」留位置。
func (UnconfiguredIntake) IntakeCatalogueQuery(context.Context, *http.Request) (CatalogueQuery, error) {
	return CatalogueQuery{}, ErrAccessChannelNotConfigured
}

// 下列登记命令口的未配置实现（ADR-0085）：与查阅口同一分界——不读业务内容、不采信
// 自报身份、不构造命令。隔离读放行（ADR-0078）只实现 CommercialCatalogueIntake，
// 下列接口它一个也不实现，因此启用隔离读换得了查阅行、换不了写行。
//
// 这段注释此前写着一个数（「八个」），已改成指代整份名单：名单每增一类就要有人记得回来改
// 那个数，而漏改不会有任何东西变红——本仓已因同形的计数吃过几次亏。
var (
	_ CommercialPublicationIntake              = UnconfiguredIntake{}
	_ CommercialPublicationPreviewIntake       = UnconfiguredIntake{}
	_ PublicationDraftSubmissionIntake         = UnconfiguredIntake{}
	_ PublicationDraftApprovalIntake           = UnconfiguredIntake{}
	_ PublicationDraftPublicationIntake        = UnconfiguredIntake{}
	_ PublicationVocabularyIntake              = UnconfiguredIntake{}
	_ BusinessPartyRegistrationIntake          = UnconfiguredIntake{}
	_ LegalEntityRegistrationIntake            = UnconfiguredIntake{}
	_ CustomerAccountRegistrationIntake        = UnconfiguredIntake{}
	_ PartyRelationshipRegistrationIntake      = UnconfiguredIntake{}
	_ PartyIdentityDeactivationIntake          = UnconfiguredIntake{}
	_ ServiceProductFormRegistrationIntake     = UnconfiguredIntake{}
	_ ProductChannelMappingRegistrationIntake  = UnconfiguredIntake{}
	_ ChannelAccountUseRegistrationIntake      = UnconfiguredIntake{}
	_ ChannelAccountUseRevocationIntake        = UnconfiguredIntake{}
	_ RegistrationNumberTypeRegistrationIntake = UnconfiguredIntake{}
	_ RegistrationNumberTypeDeactivationIntake = UnconfiguredIntake{}
)

// 方法逐个写出而不借一个泛型助手：Go 的方法不能泛型化，而这里要的恰是「每类各有
// 一个具名方法」——某类将来换上真 Intake 时，替换的是装配点那一行，本类型不动。

func (UnconfiguredIntake) IntakeCommercialPublication(
	context.Context, *http.Request,
) (application.PublishCommercialAuthorityCommand, error) {
	return application.PublishCommercialAuthorityCommand{}, ErrAccessChannelNotConfigured
}

// 运营操作者面发布路径的四口（ADR-0126 Decision 三、四）：预览虽不落库，拟录的壳也要信封里的租户才立得住，
// 与三个命令口同一分界。
func (UnconfiguredIntake) IntakeCommercialPublicationPreview(
	context.Context, *http.Request,
) (application.PreviewCommercialPublicationCommand, error) {
	return application.PreviewCommercialPublicationCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePublicationDraftSubmission(
	context.Context, *http.Request,
) (application.SubmitPublicationDraftCommand, error) {
	return application.SubmitPublicationDraftCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePublicationDraftApproval(
	context.Context, *http.Request,
) (application.ApprovePublicationDraftCommand, error) {
	return application.ApprovePublicationDraftCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePublicationDraftPublication(
	context.Context, *http.Request,
) (application.PublishPublicationDraftCommand, error) {
	return application.PublishPublicationDraftCommand{}, ErrAccessChannelNotConfigured
}

// 词表读口（票 admin-write-faces/20）只答准入、不交出作用域，未配置时同样一律拒；它跟着四口而不跟着查阅行的
// 理由在 PublicationVocabularyIntake 的注释。
func (UnconfiguredIntake) IntakePublicationVocabularyQuery(context.Context, *http.Request) error {
	return ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeBusinessPartyRegistration(
	context.Context, *http.Request,
) (application.RegisterBusinessPartyCommand, error) {
	return application.RegisterBusinessPartyCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeLegalEntityRegistration(
	context.Context, *http.Request,
) (application.RegisterLegalEntityCommand, error) {
	return application.RegisterLegalEntityCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeCustomerAccountRegistration(
	context.Context, *http.Request,
) (application.RegisterCustomerAccountCommand, error) {
	return application.RegisterCustomerAccountCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePartyRelationshipRegistration(
	context.Context, *http.Request,
) (application.RegisterPartyRelationshipCommand, error) {
	return application.RegisterPartyRelationshipCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakePartyIdentityDeactivation(
	context.Context, *http.Request,
) (application.DeactivatePartyIdentityCommand, error) {
	return application.DeactivatePartyIdentityCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeServiceProductFormRegistration(
	context.Context, *http.Request,
) (application.RegisterServiceProductFormCommand, error) {
	return application.RegisterServiceProductFormCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeProductChannelMappingRegistration(
	context.Context, *http.Request,
) (application.RegisterProductChannelMappingCommand, error) {
	return application.RegisterProductChannelMappingCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeChannelAccountUseRegistration(
	context.Context, *http.Request,
) (application.RegisterChannelAccountUseCommand, error) {
	return application.RegisterChannelAccountUseCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeChannelAccountUseRevocation(
	context.Context, *http.Request,
) (application.RevokeChannelAccountUseCommand, error) {
	return application.RevokeChannelAccountUseCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeRegistrationNumberTypeRegistration(
	context.Context, *http.Request,
) (application.RegisterRegistrationNumberTypeCommand, error) {
	return application.RegisterRegistrationNumberTypeCommand{}, ErrAccessChannelNotConfigured
}

func (UnconfiguredIntake) IntakeRegistrationNumberTypeDeactivation(
	context.Context, *http.Request,
) (application.DeactivateRegistrationNumberTypeCommand, error) {
	return application.DeactivateRegistrationNumberTypeCommand{}, ErrAccessChannelNotConfigured
}
