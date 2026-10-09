package accessidentity

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrIntegrationClientNotGranted 表示令牌有效，但客户端不在册、或此刻没有所交那一类事实的生效授予（区间外、已撤销
// 同此）。几种情形同答这一格、不说哪一半不对，理由同 ErrOperatorNotGranted；恢复动作是同一个——管授予的人去登记。
var ErrIntegrationClientNotGranted = errors.New("access identity: integration client holds no effective grant for this fact type")

// ErrIntegrationClientRegistryUnavailable 表示集成客户端册读不动。它不折进 ErrIntegrationClientNotGranted，理由同
// ErrOperatorRegistryUnavailable。
var ErrIntegrationClientRegistryUnavailable = errors.New("access identity: integration client registry is unavailable")

// IntegrationClientRequest 是一次集成客户端出示所要的：交哪一类事实。不带租户——外部系统不知道也不该指名租户，租户
// 只取自册上的绑定。
type IntegrationClientRequest struct {
	FactType ExternalFactType
	// Admission 非空表示这一口的生产写要判准入范围（ADR-0149 决定四第二条）。外部结果与资金事实都形成生产事实，各口
	// 装配时都该带；留成指针而不写死，是因为能力与事实类型的取值随口定。
	Admission *AdmissionRequirement
}

// IntegrationClientEnvelope 是经铸造的集成客户端身份：租户、客户端主体、来源身份，与铸造那一刻生效的授予集
// （ADR-0149 决定三；票 operator-channel/11 做什么第 3 条）。
//
// 它与 OperatorEnvelope、SourceEnvelope 分型：外部系统的身份不是操作者也不是客户委托的来源，拿它去铸客户委托或
// 换操作者口，编译期就走不通。字段不导出、包外没有构造函数；零值不可用——Minted 为假、租户与来源为空、不持有任何
// 一类授予。
type IntegrationClientEnvelope struct {
	tenantID       string
	client         IntegrationClientSubject
	sourceIdentity string
	factTypes      []ExternalFactType
}

func (envelope IntegrationClientEnvelope) TenantID() string                 { return envelope.tenantID }
func (envelope IntegrationClientEnvelope) Client() IntegrationClientSubject { return envelope.client }

// SourceIdentity 是这个客户端代表的外部方。消费方按各自命令的来源格取用它，不从载荷收（ADR-0100 决定二「不采信
// 自报」同一条纪律）。
func (envelope IntegrationClientEnvelope) SourceIdentity() string { return envelope.sourceIdentity }

// Holds 答信封里有没有 fact 这一类事实的生效授予。
func (envelope IntegrationClientEnvelope) Holds(fact ExternalFactType) bool {
	return slices.Contains(envelope.factTypes, fact)
}

// Minted 答这个信封是不是铸出来的。
func (envelope IntegrationClientEnvelope) Minted() bool { return envelope.client.issuer != "" }

// IntegrationClientMinter 铸造集成客户端信封。
type IntegrationClientMinter struct {
	verifier  IntegrationClientCredentialVerifier
	registry  IntegrationClientRegistry
	admission AdmissionScope
	now       func() time.Time
}

// NewIntegrationClientMinter 的 admission 不许缺，理由同 NewOperatorMinter：没登对照时传 UnconfiguredAdmissionScope。
func NewIntegrationClientMinter(
	verifier IntegrationClientCredentialVerifier,
	registry IntegrationClientRegistry,
	admission AdmissionScope,
	now func() time.Time,
) (*IntegrationClientMinter, error) {
	if verifier == nil || registry == nil || admission == nil || now == nil {
		return nil, ErrNilDependency
	}
	return &IntegrationClientMinter{verifier: verifier, registry: registry, admission: admission, now: now}, nil
}

// MintIntegrationClient 铸造一次集成客户端出示的信封：校验令牌 → 查集成客户端册 → 核证书绑定要求 → 按所交的事实
// 类型核授予 → 判准入范围。
//
// 核验方的各格（未配置、令牌不过、取不回公钥集）原样交回；册读不动答依赖故障；不在册或无此类生效授予答 ErrIntegrationClientNotGranted；册上要求证书
// 绑定而令牌没绑答 ErrCredentialRejected；准入范围不覆盖答 ErrOutsideAdmissionScope、读不动答
// ErrAdmissionScopeUnavailable。租户与来源身份只取自册上的绑定。
func (minter *IntegrationClientMinter) MintIntegrationClient(
	ctx context.Context,
	credential IntegrationClientCredential,
	request IntegrationClientRequest,
) (IntegrationClientEnvelope, error) {
	if _, err := ParseExternalFactType(string(request.FactType)); err != nil {
		return IntegrationClientEnvelope{}, fmt.Errorf("mint integration client envelope: request names no fact type: %w", err)
	}
	verified, err := minter.verifier.VerifyIntegrationClientCredential(ctx, credential)
	if err != nil {
		return IntegrationClientEnvelope{}, err
	}
	standing, found, err := minter.registry.FindIntegrationClient(ctx, verified.subject)
	if err != nil {
		return IntegrationClientEnvelope{}, fmt.Errorf("%w: %w", ErrIntegrationClientRegistryUnavailable, err)
	}
	if !found {
		return IntegrationClientEnvelope{}, ErrIntegrationClientNotGranted
	}
	// 证书绑定判在授予之前：持一枚未绑定令牌的人不该从答复里看出这个客户端授了哪几类事实。答令牌不过而不答未授予，
	// 是因为恢复动作在持令牌的一方（去换一枚绑定在证书上的令牌）；答未授予会让管授予的人去登一笔早已登好的授予。
	// 这一格让出示者看得出「这个客户端在册」，而出示者已经证明自己持有以它为主体的有效令牌。
	if standing.binding.certificateBoundTokenRequired && !verified.certificateBound {
		return IntegrationClientEnvelope{}, fmt.Errorf("%w: this client requires a certificate-bound token", ErrCredentialRejected)
	}
	at := minter.now()
	if !standing.HoldsAt(request.FactType, at) {
		return IntegrationClientEnvelope{}, ErrIntegrationClientNotGranted
	}
	if request.Admission != nil {
		admitted, err := minter.admission.Admits(ctx, standing.binding.tenantID, *request.Admission, at)
		if err != nil {
			return IntegrationClientEnvelope{}, fmt.Errorf("%w: %w", ErrAdmissionScopeUnavailable, err)
		}
		if !admitted {
			return IntegrationClientEnvelope{}, ErrOutsideAdmissionScope
		}
	}
	var facts []ExternalFactType
	for _, recorded := range standing.grants {
		if recorded.EffectiveAt(at) && !slices.Contains(facts, recorded.grant.factType) {
			facts = append(facts, recorded.grant.factType)
		}
	}
	return IntegrationClientEnvelope{
		tenantID:       standing.binding.tenantID,
		client:         verified.subject,
		sourceIdentity: standing.binding.sourceIdentity,
		factTypes:      facts,
	}, nil
}
