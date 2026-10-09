package accessidentity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrInvalidIntegrationClientSubject 表示集成客户端主体缺发行方或 sub。
var ErrInvalidIntegrationClientSubject = errors.New("access identity: integration client subject needs both issuer and subject")

// ErrIncompleteIntegrationClientRegistration 表示集成客户端册的一行缺件，建不成。
var ErrIncompleteIntegrationClientRegistration = errors.New("access identity: integration client registration is incomplete")

// IntegrationClientSubject 是集成客户端主体：签发客户端凭据令牌的发行方 + 令牌的 `sub`（ADR-0149 决定三）。
//
// 取 `sub` 而不取 `client_id` 或 `azp`：RFC 9068 第 2.2 节对客户端凭据许可写的是 `sub` 应当对应授权服务器用来指称
// 该客户端的标识，而 `client_id` 并非每个授权服务器都签；册上登的就是租户的授权服务器在 `sub` 里放的那个值。
// 两件合起来才是身份，理由同 OperatorSubject。它与 OperatorSubject 分型：同一个发行方同时签操作者与客户端的令牌时，
// 一个人的 `sub` 不该在类型上就能拿去查客户端册。
type IntegrationClientSubject struct {
	issuer  string
	subject string
}

func NewIntegrationClientSubject(issuer, subject string) (IntegrationClientSubject, error) {
	trimmedIssuer := strings.TrimSpace(issuer)
	trimmedSubject := strings.TrimSpace(subject)
	if trimmedIssuer == "" || trimmedSubject == "" {
		return IntegrationClientSubject{}, ErrInvalidIntegrationClientSubject
	}
	return IntegrationClientSubject{issuer: trimmedIssuer, subject: trimmedSubject}, nil
}

func (subject IntegrationClientSubject) Issuer() string  { return subject.issuer }
func (subject IntegrationClientSubject) Subject() string { return subject.subject }

// IntegrationClientBinding 是集成客户端册里「一个客户端主体绑定唯一租户与一个来源身份」那一行（ADR-0149 决定三）。
//
// 来源身份是这个客户端代表的外部方（某报关服务商、某承运商、某银行），它随绑定一次登定：同一个客户端先后代表两个
// 来源，它交进来的事实就说不清出自谁。绑定不可撤销、不可改指，理由同 OperatorBinding；停用一个客户端就撤它名下的
// 授予。
//
// 凭据引用指向租户约定的受限存储里这个客户端凭据的出处，供轮换与审计追溯；凭据本体不入库（ADR-0149 越权风险点 4）。
// 核验令牌不读它——令牌按发行方的公钥集验。
//
// 要求证书绑定令牌那一格是租户对这个客户端的取值（ADR-0149 决定三「高保证场景可要求 mTLS 绑定令牌」）：为真时，
// 只认绑定在这次 TLS 连接所出示证书上的令牌（RFC 8705），偷到的持有者令牌换个连接用不了。
type IntegrationClientBinding struct {
	subject                       IntegrationClientSubject
	tenantID                      string
	sourceIdentity                string
	credential                    CredentialReference
	certificateBoundTokenRequired bool
	basis                         string
}

// NewIntegrationClientBinding 要求登记依据非空：册上每一行都要答得出凭什么登的，行是租户取值。
func NewIntegrationClientBinding(
	subject IntegrationClientSubject,
	tenantID string,
	sourceIdentity string,
	credential CredentialReference,
	certificateBoundTokenRequired bool,
	basis string,
) (IntegrationClientBinding, error) {
	tenant := strings.TrimSpace(tenantID)
	source := strings.TrimSpace(sourceIdentity)
	reference := strings.TrimSpace(basis)
	if subject.issuer == "" || tenant == "" || source == "" || credential.value == "" || reference == "" {
		return IntegrationClientBinding{}, ErrIncompleteIntegrationClientRegistration
	}
	return IntegrationClientBinding{
		subject:                       subject,
		tenantID:                      tenant,
		sourceIdentity:                source,
		credential:                    credential,
		certificateBoundTokenRequired: certificateBoundTokenRequired,
		basis:                         reference,
	}, nil
}

func (binding IntegrationClientBinding) Subject() IntegrationClientSubject { return binding.subject }
func (binding IntegrationClientBinding) TenantID() string                  { return binding.tenantID }
func (binding IntegrationClientBinding) SourceIdentity() string            { return binding.sourceIdentity }
func (binding IntegrationClientBinding) Basis() string                     { return binding.basis }

func (binding IntegrationClientBinding) CredentialReference() CredentialReference {
	return binding.credential
}

func (binding IntegrationClientBinding) CertificateBoundTokenRequired() bool {
	return binding.certificateBoundTokenRequired
}

// ErrExternalFactTypeUnknown 表示给出的事实类型不在 ADR-0149 决定一列给集成客户端族的那几类里。
var ErrExternalFactTypeUnknown = errors.New("access identity: external fact type is unknown")

// ExternalFactType 是集成客户端授予的单位：外部系统能向本产品交哪一类事实（ADR-0149 决定一、三）。
//
// 这几类是册的列，由产品定；某个租户给哪个客户端授了哪一类才是行。更正口不另立一类：更正与首登的提交方是同一方，
// 认证与授予也是同一份（ADR-0151 决定四），另立一类就会出现「能登不能改」或「能改不能登」。
type ExternalFactType string

const (
	FactCustomsExternalResult               ExternalFactType = "CUSTOMS_EXTERNAL_RESULT"
	FactRegulatoryCredential                ExternalFactType = "REGULATORY_CREDENTIAL"
	FactCarrierTracking                     ExternalFactType = "CARRIER_TRACKING"
	FactCarrierFirstEffectivePickupEvidence ExternalFactType = "CARRIER_FIRST_EFFECTIVE_PICKUP_EVIDENCE"
	FactExternalFunds                       ExternalFactType = "EXTERNAL_FUNDS_FACT"
)

var externalFactTypes = []ExternalFactType{
	FactCustomsExternalResult, FactRegulatoryCredential, FactCarrierTracking,
	FactCarrierFirstEffectivePickupEvidence, FactExternalFunds,
}

// ParseExternalFactType 按字面取值认事实类型，大小写不折叠：册里存的就是这些字面量。
func ParseExternalFactType(value string) (ExternalFactType, error) {
	for _, fact := range externalFactTypes {
		if string(fact) == value {
			return fact, nil
		}
	}
	return "", ErrExternalFactTypeUnknown
}

func (fact ExternalFactType) String() string { return string(fact) }

// IntegrationClientGrant 是按事实类型显式登记的一笔授予。授予带自己的登记标识，撤了再授是两笔，理由同 OperatorGrant。
type IntegrationClientGrant struct {
	tenantID string
	grantID  string
	subject  IntegrationClientSubject
	factType ExternalFactType
	interval EffectiveInterval
	basis    string
}

func NewIntegrationClientGrant(
	tenantID string,
	grantID string,
	subject IntegrationClientSubject,
	factType ExternalFactType,
	interval EffectiveInterval,
	basis string,
) (IntegrationClientGrant, error) {
	// ExternalFactType 是字符串底型，包外转型递进来的值也要在这里被拦下。
	if _, err := ParseExternalFactType(string(factType)); err != nil {
		return IntegrationClientGrant{}, err
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || subject.issuer == "" || interval.startsAt.IsZero() || reference == "" {
		return IntegrationClientGrant{}, ErrIncompleteIntegrationClientRegistration
	}
	return IntegrationClientGrant{
		tenantID: tenant,
		grantID:  id,
		subject:  subject,
		factType: factType,
		interval: interval,
		basis:    reference,
	}, nil
}

func (grant IntegrationClientGrant) TenantID() string                  { return grant.tenantID }
func (grant IntegrationClientGrant) GrantID() string                   { return grant.grantID }
func (grant IntegrationClientGrant) Subject() IntegrationClientSubject { return grant.subject }
func (grant IntegrationClientGrant) FactType() ExternalFactType        { return grant.factType }
func (grant IntegrationClientGrant) Interval() EffectiveInterval       { return grant.interval }
func (grant IntegrationClientGrant) Basis() string                     { return grant.basis }

// RecordedClientGrant 是册上的一笔集成客户端授予，连同它的撤销（若有）。撤销沿用 GrantRevocation：撤一笔授予
// 要的就是（租户、授予标识、时刻、依据）四件，与它撤的是哪一族的授予无关。
type RecordedClientGrant struct {
	grant      IntegrationClientGrant
	revocation GrantRevocation
	revoked    bool
}

// NewRecordedClientGrant 的 revocation 取 nil 表示未撤销。
func NewRecordedClientGrant(grant IntegrationClientGrant, revocation *GrantRevocation) (RecordedClientGrant, error) {
	if revocation == nil {
		return RecordedClientGrant{grant: grant}, nil
	}
	if revocation.tenantID != grant.tenantID || revocation.grantID != grant.grantID {
		return RecordedClientGrant{}, ErrRevocationDoesNotMatchGrant
	}
	return RecordedClientGrant{grant: grant, revocation: *revocation, revoked: true}, nil
}

func (recorded RecordedClientGrant) Grant() IntegrationClientGrant { return recorded.grant }

func (recorded RecordedClientGrant) Revocation() (GrantRevocation, bool) {
	return recorded.revocation, recorded.revoked
}

// EffectiveAt 答这笔授予在 at 那一刻生不生效：在区间内，且还没到撤销时刻。
func (recorded RecordedClientGrant) EffectiveAt(at time.Time) bool {
	return effectiveAt(recorded.grant.interval, recorded.revocation, recorded.revoked, at)
}

// ErrIntegrationClientGrantOutsideBinding 表示现状里混进了别的客户端主体或别的租户名下的授予。
var ErrIntegrationClientGrantOutsideBinding = errors.New("access identity: grant does not belong to the integration client binding")

// IntegrationClientStanding 是一个客户端主体在册上的全部：它的绑定，与它名下每一笔授予（连同撤销）。
//
// 它只答「册上怎么写」，不答「这次出示准不准」：后者还要看令牌是否绑在证书上、所交的是哪一类事实，那是铸造那一步
// 的事。
type IntegrationClientStanding struct {
	binding IntegrationClientBinding
	grants  []RecordedClientGrant
}

func NewIntegrationClientStanding(binding IntegrationClientBinding, grants []RecordedClientGrant) (IntegrationClientStanding, error) {
	copied := make([]RecordedClientGrant, 0, len(grants))
	for _, recorded := range grants {
		if recorded.grant.subject != binding.subject || recorded.grant.tenantID != binding.tenantID {
			return IntegrationClientStanding{}, ErrIntegrationClientGrantOutsideBinding
		}
		copied = append(copied, recorded)
	}
	return IntegrationClientStanding{binding: binding, grants: copied}, nil
}

func (standing IntegrationClientStanding) Binding() IntegrationClientBinding { return standing.binding }

func (standing IntegrationClientStanding) Grants() []RecordedClientGrant {
	return append([]RecordedClientGrant(nil), standing.grants...)
}

// HoldsAt 答这个客户端在 at 那一刻是否持有 fact 这一类事实的生效授予。
func (standing IntegrationClientStanding) HoldsAt(fact ExternalFactType, at time.Time) bool {
	for _, recorded := range standing.grants {
		if recorded.grant.factType == fact && recorded.EffectiveAt(at) {
			return true
		}
	}
	return false
}

// IntegrationClientRegistry 是集成客户端册的装载口：按主体取它在册上的全部。主体不在册时答 found=false 而不返回
// error，理由同 OperatorRegistry。
type IntegrationClientRegistry interface {
	FindIntegrationClient(ctx context.Context, subject IntegrationClientSubject) (IntegrationClientStanding, bool, error)
}

// IntegrationClientRegistrationOutcome 是集成客户端册登记口的答复。撞键从不覆盖：同内容重放答原结果，异内容答冲突，
// 册上已有的那一行原样不动。它不复用 OperatorRegistrationOutcome：两本册的「未登记」指的不是同一种主体，共用一个
// 类型，批文翻错了册在答复上也看不出来。
type IntegrationClientRegistrationOutcome string

const (
	IntegrationClientRegistrationRecorded          IntegrationClientRegistrationOutcome = "RECORDED"
	IntegrationClientRegistrationAlreadyRegistered IntegrationClientRegistrationOutcome = "ALREADY_REGISTERED"
	IntegrationClientRegistrationContentConflict   IntegrationClientRegistrationOutcome = "CONTENT_CONFLICT"
	// IntegrationClientBoundToAnotherTenant 只由 RegisterIntegrationClient 答：这个主体已绑在别的租户上。不说是哪个
	// 租户，理由同 OperatorSubjectBoundToAnotherTenant。
	IntegrationClientBoundToAnotherTenant IntegrationClientRegistrationOutcome = "CLIENT_BOUND_TO_ANOTHER_TENANT"
	// IntegrationClientNotRegistered 只由 RegisterIntegrationClientGrant 答：本租户册上没有这个主体；绑在别的租户上
	// 也答这一格。
	IntegrationClientNotRegistered IntegrationClientRegistrationOutcome = "CLIENT_NOT_REGISTERED"
	// IntegrationClientGrantNotRegistered 只由 RegisterIntegrationClientRevocation 答：本租户册上没有这笔授予。
	IntegrationClientGrantNotRegistered IntegrationClientRegistrationOutcome = "GRANT_NOT_REGISTERED"
)

func (outcome IntegrationClientRegistrationOutcome) String() string { return string(outcome) }

// IntegrationClientRegistrar 是集成客户端册的登记口，受控批量口与将来的在线口消费它，答复代数一致。写入要落在调用方
// 开的框架事务里，理由同 OperatorRegistrar。
type IntegrationClientRegistrar interface {
	RegisterIntegrationClient(ctx context.Context, binding IntegrationClientBinding) (IntegrationClientRegistrationOutcome, error)
	RegisterIntegrationClientGrant(ctx context.Context, grant IntegrationClientGrant) (IntegrationClientRegistrationOutcome, error)
	RegisterIntegrationClientRevocation(ctx context.Context, revocation GrantRevocation) (IntegrationClientRegistrationOutcome, error)
}
