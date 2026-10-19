package accessidentity

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrCustomerClientNotGranted 表示令牌有效，但客户端不在客户渠道册，或此刻没有这一项客户面能力的生效授予（撤销、出区间
// 同此）。几种情形同答这一格、不说哪一半不对，理由同 ErrIntegrationClientNotGranted（ADR-0139 决定四「令牌有效，但客户端
// 不在客户渠道册……」那一格）。
//
// ErrAccessChannelNotConfigured 不承载这一格：在首方族上它只留给发行方参数未设，恢复动作是配部署参数的人；这一格的恢复
// 动作是租户运营去登记或恢复授予。
var ErrCustomerClientNotGranted = errors.New("access identity: customer client holds no effective grant for this capability")

// ErrCustomerClientRegistryUnavailable 表示客户渠道册读不动。它不折进 ErrCustomerClientNotGranted，理由同
// ErrOperatorRegistryUnavailable。
var ErrCustomerClientRegistryUnavailable = errors.New("access identity: customer client registry is unavailable")

// AccessRequestIdentifier 是首方族交给推导口的 ChannelRequest 里「接入请求标识」那一格的名字（ADR-0139 决定五；UC-PS-001
// 输入语义契约来源信封行的原词）。它在载荷里，身份却是信封元数据：推导成来源请求键，不进规范化内容摘要。
const AccessRequestIdentifier = "accessRequestId"

// VerifiedCustomerClientToken 是核验方对一次首方出示的结论：令牌代表的客户端主体，以及令牌是否绑定在这次连接出示的证书
// 上且对得上。「绑没绑」由核验方答、「要不要绑」由册答，理由同 VerifiedClientToken。
type VerifiedCustomerClientToken struct {
	subject          CustomerClientSubject
	certificateBound bool
}

func NewVerifiedCustomerClientToken(subject CustomerClientSubject, certificateBound bool) VerifiedCustomerClientToken {
	return VerifiedCustomerClientToken{subject: subject, certificateBound: certificateBound}
}

func (verified VerifiedCustomerClientToken) Subject() CustomerClientSubject { return verified.subject }
func (verified VerifiedCustomerClientToken) CertificateBound() bool         { return verified.certificateBound }

// CustomerClientCredentialVerifier 核验一次首方出示，交回经核验的客户端主体与证书绑定情况。
//
// 出示物沿用 IntegrationClientCredential：首方族复用集成客户端族的凭据机制（ADR-0139 决定二），令牌加本次连接的客户端
// 证书，不另立一个同形的类型。核验方却不复用 IntegrationClientCredentialVerifier：首方族与集成客户端族的信任锚是两组部署
// 参数、受众不同，共用一个接口，装配时把集成客户端族那一只接到首方铸造器上也编得过，令牌就能跨族通过校验期。失败分三格，
// 同操作者族、集成客户端族：
// ErrAccessChannelNotConfigured、ErrCredentialRejected、ErrCredentialVerifierUnavailable。
type CustomerClientCredentialVerifier interface {
	VerifyCustomerClientCredential(ctx context.Context, credential IntegrationClientCredential) (VerifiedCustomerClientToken, error)
}

// UnconfiguredCustomerClientCredentialVerifier 是首方族发行方参数未设时的核验方：对任何出示都答
// ErrAccessChannelNotConfigured，缺令牌也一样。
type UnconfiguredCustomerClientCredentialVerifier struct{}

func (UnconfiguredCustomerClientCredentialVerifier) VerifyCustomerClientCredential(
	context.Context,
	IntegrationClientCredential,
) (VerifiedCustomerClientToken, error) {
	return VerifiedCustomerClientToken{}, ErrAccessChannelNotConfigured
}

// customerClientRequest 是一次首方出示要过的那几道：哪一项客户面能力，以及这一口的生产写要不要判准入范围。不带租户与
// 客户账户——客户系统不指名它们，两者只取自册上的绑定（ADR-0003）。
//
// 不导出：哪一口要哪一项能力、判不判准入，由按命令分型的铸造方法自己定，调用方递不进一个与方法不配的能力。
type customerClientRequest struct {
	capability CustomerCapability
	// admission 非空表示这一口的生产写在铸造侧判准入范围（ADR-0139 决定八）。提交口为空：它的生产归属由 UC-PS-001 编排
	// 自己判，铸造侧不判第二遍；撤回、取消、补充、修订与索赔各口换上首方 Intake 时各带自己那一格。
	admission *AdmissionRequirement
}

// CustomerClientMinter 铸造首方客户族的来源信封（ADR-0139 决定二、三）。
//
// 它铸的是 SourceEnvelope（经 SubmissionEnvelope 等按命令分型），与 OperatorEnvelope、IntegrationClientEnvelope 分型：
// 拿别族的信封去铸客户委托，编译期就走不通。信封仍只经 seal 产生。
type CustomerClientMinter struct {
	verifier  CustomerClientCredentialVerifier
	registry  CustomerClientRegistry
	admission AdmissionScope
	now       func() time.Time
}

// NewCustomerClientMinter 的 admission 不许缺，取法同 NewIntegrationClientMinter：提交口不在铸造侧判准入，余下客户面各口
// 换上首方 Intake 时判，装配点只给一只。
func NewCustomerClientMinter(
	verifier CustomerClientCredentialVerifier,
	registry CustomerClientRegistry,
	admission AdmissionScope,
	now func() time.Time,
) (*CustomerClientMinter, error) {
	if verifier == nil || registry == nil || admission == nil || now == nil {
		return nil, ErrNilDependency
	}
	return &CustomerClientMinter{verifier: verifier, registry: registry, admission: admission, now: now}, nil
}

// MintSubmission 铸一次委托提交的来源信封：先过 admit 那几道，再读请求、推导来源请求键，最后经 seal 封出。
//
// 请求经 readRequest 在认证之后才读：令牌不过、不在册或没有授予的调用方看不到载荷校验的结果（同集成客户端族 Intake 的
// 先认证后读载荷）。readRequest 交回的错原样交回，调用方据它自己的格答复。
func (minter *CustomerClientMinter) MintSubmission(
	ctx context.Context,
	credential IntegrationClientCredential,
	readRequest func() (ChannelRequest, error),
) (SubmissionEnvelope, error) {
	if readRequest == nil {
		return SubmissionEnvelope{}, ErrNilDependency
	}
	registration, err := minter.admit(ctx, credential, customerClientRequest{capability: CustomerCapabilityShipmentRequestSubmission})
	if err != nil {
		return SubmissionEnvelope{}, err
	}
	request, err := readRequest()
	if err != nil {
		return SubmissionEnvelope{}, err
	}
	requestKey, err := registration.derivation.DeriveSubmissionRequestKey(ctx, request)
	if err != nil {
		return SubmissionEnvelope{}, fmt.Errorf("mint customer submission envelope: %w", err)
	}
	envelope, err := seal(registration, requestKey)
	if err != nil {
		return SubmissionEnvelope{}, err
	}
	return SubmissionEnvelope{envelope: envelope}, nil
}

// admit 校验令牌 → 以已核验主体查客户渠道册 → 核证书绑定要求 → 核授予 → 判准入范围（若这一口要判），交回由册上那一行
// 拼出的 ChannelRegistration。顺序照 MintIntegrationClient，不可倒（ADR-0139 决定二）。
//
// 令牌那一步在查册之前、不读任何册：签名校验若排到查册之后，一个未经任何校验的调用方拿伪造令牌只换主体，在册与不在册
// 就得两种答复，册的成员由此可被枚举。查到即绑定成立，不另核「令牌主体与行所登客户端是否同一个」——行就是按这个主体
// 查出来的。
func (minter *CustomerClientMinter) admit(
	ctx context.Context,
	credential IntegrationClientCredential,
	request customerClientRequest,
) (ChannelRegistration, error) {
	verified, err := minter.verifier.VerifyCustomerClientCredential(ctx, credential)
	if err != nil {
		return ChannelRegistration{}, err
	}
	standing, found, err := minter.registry.FindCustomerClient(ctx, verified.subject)
	if err != nil {
		return ChannelRegistration{}, fmt.Errorf("%w: %w", ErrCustomerClientRegistryUnavailable, err)
	}
	if !found {
		return ChannelRegistration{}, ErrCustomerClientNotGranted
	}
	// 证书绑定判在授予之前、答令牌不过，理由同 MintIntegrationClient 里同一处。
	if standing.binding.certificateBoundTokenRequired && !verified.certificateBound {
		return ChannelRegistration{}, fmt.Errorf("%w: this client requires a certificate-bound token", ErrCredentialRejected)
	}
	at := minter.now()
	if !standing.HoldsAt(request.capability, at) {
		return ChannelRegistration{}, ErrCustomerClientNotGranted
	}
	if request.admission != nil {
		admitted, err := minter.admission.Admits(ctx, standing.binding.tenantID, *request.admission, at)
		if err != nil {
			return ChannelRegistration{}, fmt.Errorf("%w: %w", ErrAdmissionScopeUnavailable, err)
		}
		if !admitted {
			return ChannelRegistration{}, ErrOutsideAdmissionScope
		}
	}
	return NewChannelRegistration(
		standing.binding.tenantID,
		standing.binding.customerAccountID,
		verified.subject.channelSource(),
		standing.binding.credential,
		customerAPIRequestKeys{},
	)
}

// customerAPIRequestKeys 是首方族的 RequestKeyDerivation（ADR-0139 决定五）：提交的来源请求键就是客户系统在载荷里给的
// 接入请求标识，去两端空白，此外一字不动。标识按契约逐请求唯一（它标识的是一次请求，不是一份业务对象）：同一份内容换
// 标识重发是两次来源请求，同标识重发是重放。
type customerAPIRequestKeys struct{}

func (customerAPIRequestKeys) DeriveSubmissionRequestKey(_ context.Context, request ChannelRequest) (string, error) {
	return accessRequestIdentifierOf(request)
}

// DeriveWithdrawalRequestKey 答推导不出：首方族还没有撤回口，撤回的来源请求键怎么推、与提交那一串怎么区分，留给撤回口
// 换上首方 Intake 的那一票按 ADR-0139 越权风险点「撤回、取消、补充、资料修订、索赔提交各命令的来源请求键推导」的复核
// 建议裁。先拿提交那一格顶上，等于替它裁了「同一标识先提交后撤回」答什么，而那件事 RequestKeyDerivation 的头注正是
// 不许含糊的。
func (customerAPIRequestKeys) DeriveWithdrawalRequestKey(context.Context, ChannelRequest) (string, error) {
	return "", fmt.Errorf("%w: the first-party customer family has no withdrawal derivation yet", ErrRequestKeyNotDerived)
}

func accessRequestIdentifierOf(request ChannelRequest) (string, error) {
	value, _ := request.Value(AccessRequestIdentifier)
	trimmed := strings.TrimSpace(value)
	if trimmed == "" {
		return "", fmt.Errorf("%w: %s is absent", ErrRequestKeyNotDerived, AccessRequestIdentifier)
	}
	return trimmed, nil
}
