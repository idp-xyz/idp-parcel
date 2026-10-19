package accessidentity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrInvalidCustomerClientSubject 表示首方客户端主体缺发行方或 sub。
var ErrInvalidCustomerClientSubject = errors.New("access identity: customer client subject needs both issuer and subject")

// ErrIncompleteCustomerClientRegistration 表示客户渠道册的一行缺件，建不成。
var ErrIncompleteCustomerClientRegistration = errors.New("access identity: customer client registration is incomplete")

// CustomerClientSubject 是首方客户族的客户端主体：签发客户端凭据令牌的发行方 + 令牌的 `sub`（ADR-0139 决定二）。
//
// 取 `sub` 而不取 `client_id`，理由同 IntegrationClientSubject 的头注。它与 IntegrationClientSubject、OperatorSubject
// 分型：各族可以落在同一个发行方实例上（部署决定），一个外部系统的 `sub` 不该在类型上就能拿去查客户渠道册——首方族与
// 集成客户端族按 ADR-0149「按提交者分族」分开，「不得互相顶替」靠的就是这一层（ADR-0139 决定三）。
type CustomerClientSubject struct {
	issuer  string
	subject string
}

func NewCustomerClientSubject(issuer, subject string) (CustomerClientSubject, error) {
	trimmedIssuer := strings.TrimSpace(issuer)
	trimmedSubject := strings.TrimSpace(subject)
	if trimmedIssuer == "" || trimmedSubject == "" {
		return CustomerClientSubject{}, ErrInvalidCustomerClientSubject
	}
	return CustomerClientSubject{issuer: trimmedIssuer, subject: trimmedSubject}, nil
}

func (subject CustomerClientSubject) Issuer() string  { return subject.issuer }
func (subject CustomerClientSubject) Subject() string { return subject.subject }

// channelSource 是这个客户端交进来的委托在来源信封里的来源格。客户渠道册的行形（ADR-0139 决定二）没有来源一列：来源就是
// 这个客户端本身，于是由主体推出，不另登一格让它与主体各说各话。发行方要在里面——`sub` 只在它的发行方之内唯一，换一个
// 发行方，同一个 `sub` 是另一个客户端，它们的接入请求标识不该落进同一个判重空间。拼法与 PS 留痕里提交操作者的引用同一种
// （发行方 + `#` + `sub`）。
func (subject CustomerClientSubject) channelSource() string {
	return subject.issuer + "#" + subject.subject
}

// CustomerClientBinding 是客户渠道册里「一个客户端主体绑定唯一（租户、客户账户）」那一行（ADR-0139 决定二）。
//
// 绑定一次登定、不可撤销、不可改指：同一个客户端先后代表两个客户账户，它交进来的委托就说不清是谁的。停用一个客户端就
// 撤它名下的授予，理由同 OperatorBinding。首发一客户端一客户账户（ADR-0139 越权风险点「客户系统的令牌是否允许携带多个
// 客户账户」的复核建议；1:N 归票 first-party-api/01）。
//
// 凭据引用与要求证书绑定令牌两格的含义同 IntegrationClientBinding：前者指向受限存储里这个客户端凭据的出处、核验不读它；
// 后者是租户对这个客户端的取值，产品不给缺省。
type CustomerClientBinding struct {
	subject                       CustomerClientSubject
	tenantID                      string
	customerAccountID             string
	credential                    CredentialReference
	certificateBoundTokenRequired bool
	basis                         string
}

// NewCustomerClientBinding 要求登记依据非空：册上每一行都要答得出凭什么登的，行是租户取值（参数登记册 PAR-INT-13）。
func NewCustomerClientBinding(
	subject CustomerClientSubject,
	tenantID string,
	customerAccountID string,
	credential CredentialReference,
	certificateBoundTokenRequired bool,
	basis string,
) (CustomerClientBinding, error) {
	tenant := strings.TrimSpace(tenantID)
	account := strings.TrimSpace(customerAccountID)
	reference := strings.TrimSpace(basis)
	if subject.issuer == "" || tenant == "" || account == "" || credential.value == "" || reference == "" {
		return CustomerClientBinding{}, ErrIncompleteCustomerClientRegistration
	}
	return CustomerClientBinding{
		subject:                       subject,
		tenantID:                      tenant,
		customerAccountID:             account,
		credential:                    credential,
		certificateBoundTokenRequired: certificateBoundTokenRequired,
		basis:                         reference,
	}, nil
}

func (binding CustomerClientBinding) Subject() CustomerClientSubject { return binding.subject }
func (binding CustomerClientBinding) TenantID() string               { return binding.tenantID }
func (binding CustomerClientBinding) CustomerAccountID() string      { return binding.customerAccountID }
func (binding CustomerClientBinding) Basis() string                  { return binding.basis }

func (binding CustomerClientBinding) CredentialReference() CredentialReference {
	return binding.credential
}

func (binding CustomerClientBinding) CertificateBoundTokenRequired() bool {
	return binding.certificateBoundTokenRequired
}

// ErrCustomerCapabilityUnknown 表示给出的客户面能力不在产品定下的词表里。
var ErrCustomerCapabilityUnknown = errors.New("access identity: customer capability is unknown")

// CustomerCapability 是首方客户族授予的单位：一个客户系统能在哪一个客户面口上做什么（ADR-0139 决定二）。
//
// 词表是册的列，归产品策略；某个租户给哪个客户端授了哪几项才是行。一口一词，随那一口换上首方 Intake 的那一笔加：还没换口
// 的客户面各口不进词表——授一个用不上的能力，只会让人以为那一口已经通了。粒度（按口还是按用例归并）归票
// first-party-api/01——它若归并，改的是这张表与迁移里那条 CHECK，两处同笔。
type CustomerCapability string

const (
	// CustomerCapabilityShipmentRequestSubmission 是委托提交（`/shipment-requests`，UC-PS-001）。
	CustomerCapabilityShipmentRequestSubmission CustomerCapability = "SHIPMENT_REQUEST_SUBMISSION"
)

var customerCapabilities = []CustomerCapability{
	CustomerCapabilityShipmentRequestSubmission,
}

// ParseCustomerCapability 按字面取值认能力，大小写不折叠：册里存的就是这些字面量。
func ParseCustomerCapability(value string) (CustomerCapability, error) {
	for _, capability := range customerCapabilities {
		if string(capability) == value {
			return capability, nil
		}
	}
	return "", ErrCustomerCapabilityUnknown
}

func (capability CustomerCapability) String() string { return string(capability) }

// CustomerClientGrant 是按客户面能力显式登记的一笔授予。授予带自己的登记标识，撤了再授是两笔，理由同 OperatorGrant。
type CustomerClientGrant struct {
	tenantID   string
	grantID    string
	subject    CustomerClientSubject
	capability CustomerCapability
	interval   EffectiveInterval
	basis      string
}

func NewCustomerClientGrant(
	tenantID string,
	grantID string,
	subject CustomerClientSubject,
	capability CustomerCapability,
	interval EffectiveInterval,
	basis string,
) (CustomerClientGrant, error) {
	// CustomerCapability 是字符串底型，包外转型递进来的值也要在这里被拦下。
	if _, err := ParseCustomerCapability(string(capability)); err != nil {
		return CustomerClientGrant{}, err
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || subject.issuer == "" || interval.startsAt.IsZero() || reference == "" {
		return CustomerClientGrant{}, ErrIncompleteCustomerClientRegistration
	}
	return CustomerClientGrant{
		tenantID:   tenant,
		grantID:    id,
		subject:    subject,
		capability: capability,
		interval:   interval,
		basis:      reference,
	}, nil
}

func (grant CustomerClientGrant) TenantID() string               { return grant.tenantID }
func (grant CustomerClientGrant) GrantID() string                { return grant.grantID }
func (grant CustomerClientGrant) Subject() CustomerClientSubject { return grant.subject }
func (grant CustomerClientGrant) Capability() CustomerCapability { return grant.capability }
func (grant CustomerClientGrant) Interval() EffectiveInterval    { return grant.interval }
func (grant CustomerClientGrant) Basis() string                  { return grant.basis }

// RecordedCustomerClientGrant 是册上的一笔首方客户端授予，连同它的撤销（若有）。撤销沿用 GrantRevocation，理由同
// RecordedClientGrant。
type RecordedCustomerClientGrant struct {
	grant      CustomerClientGrant
	revocation GrantRevocation
	revoked    bool
}

// NewRecordedCustomerClientGrant 的 revocation 取 nil 表示未撤销。
func NewRecordedCustomerClientGrant(grant CustomerClientGrant, revocation *GrantRevocation) (RecordedCustomerClientGrant, error) {
	if revocation == nil {
		return RecordedCustomerClientGrant{grant: grant}, nil
	}
	if revocation.tenantID != grant.tenantID || revocation.grantID != grant.grantID {
		return RecordedCustomerClientGrant{}, ErrRevocationDoesNotMatchGrant
	}
	return RecordedCustomerClientGrant{grant: grant, revocation: *revocation, revoked: true}, nil
}

func (recorded RecordedCustomerClientGrant) Grant() CustomerClientGrant { return recorded.grant }

func (recorded RecordedCustomerClientGrant) Revocation() (GrantRevocation, bool) {
	return recorded.revocation, recorded.revoked
}

// EffectiveAt 答这笔授予在 at 那一刻生不生效：在区间内，且还没到撤销时刻。
func (recorded RecordedCustomerClientGrant) EffectiveAt(at time.Time) bool {
	return effectiveAt(recorded.grant.interval, recorded.revocation, recorded.revoked, at)
}

// ErrCustomerClientGrantOutsideBinding 表示现状里混进了别的客户端主体或别的租户名下的授予。
var ErrCustomerClientGrantOutsideBinding = errors.New("access identity: grant does not belong to the customer client binding")

// CustomerClientStanding 是一个首方客户端主体在册上的全部：它的绑定，与它名下每一笔授予（连同撤销）。它只答「册上怎么
// 写」，不答「这次出示准不准」，理由同 IntegrationClientStanding。
type CustomerClientStanding struct {
	binding CustomerClientBinding
	grants  []RecordedCustomerClientGrant
}

func NewCustomerClientStanding(binding CustomerClientBinding, grants []RecordedCustomerClientGrant) (CustomerClientStanding, error) {
	copied := make([]RecordedCustomerClientGrant, 0, len(grants))
	for _, recorded := range grants {
		if recorded.grant.subject != binding.subject || recorded.grant.tenantID != binding.tenantID {
			return CustomerClientStanding{}, ErrCustomerClientGrantOutsideBinding
		}
		copied = append(copied, recorded)
	}
	return CustomerClientStanding{binding: binding, grants: copied}, nil
}

func (standing CustomerClientStanding) Binding() CustomerClientBinding { return standing.binding }

func (standing CustomerClientStanding) Grants() []RecordedCustomerClientGrant {
	return append([]RecordedCustomerClientGrant(nil), standing.grants...)
}

// HoldsAt 答这个客户端在 at 那一刻是否持有 capability 这一项的生效授予。
func (standing CustomerClientStanding) HoldsAt(capability CustomerCapability, at time.Time) bool {
	for _, recorded := range standing.grants {
		if recorded.grant.capability == capability && recorded.EffectiveAt(at) {
			return true
		}
	}
	return false
}

// CustomerClientRegistry 是客户渠道册的装载口：按主体取它在册上的全部。主体不在册时答 found=false 而不返回 error，理由同
// OperatorRegistry。
type CustomerClientRegistry interface {
	FindCustomerClient(ctx context.Context, subject CustomerClientSubject) (CustomerClientStanding, bool, error)
}

// CustomerClientRegistrationOutcome 是客户渠道册登记口的答复。撞键从不覆盖：同内容重放答原结果，异内容答冲突，册上已有
// 的那一行原样不动。不复用集成客户端册的答复类型，理由同 IntegrationClientRegistrationOutcome 不复用操作者册的。
type CustomerClientRegistrationOutcome string

const (
	CustomerClientRegistrationRecorded          CustomerClientRegistrationOutcome = "RECORDED"
	CustomerClientRegistrationAlreadyRegistered CustomerClientRegistrationOutcome = "ALREADY_REGISTERED"
	// CustomerClientRegistrationContentConflict 也是「同一租户里改指到另一个客户账户」的答复：绑定不可改指。
	CustomerClientRegistrationContentConflict CustomerClientRegistrationOutcome = "CONTENT_CONFLICT"
	// CustomerClientBoundToAnotherTenant 只由 RegisterCustomerClient 答：这个主体已绑在别的租户上。不说是哪个租户，理由同
	// OperatorSubjectBoundToAnotherTenant。
	CustomerClientBoundToAnotherTenant CustomerClientRegistrationOutcome = "CLIENT_BOUND_TO_ANOTHER_TENANT"
	// CustomerClientNotRegistered 只由 RegisterCustomerClientGrant 答：本租户册上没有这个主体；绑在别的租户上也答这一格。
	CustomerClientNotRegistered CustomerClientRegistrationOutcome = "CLIENT_NOT_REGISTERED"
	// CustomerClientGrantNotRegistered 只由 RegisterCustomerClientRevocation 答：本租户册上没有这笔授予。
	CustomerClientGrantNotRegistered CustomerClientRegistrationOutcome = "GRANT_NOT_REGISTERED"
)

func (outcome CustomerClientRegistrationOutcome) String() string { return string(outcome) }

// CustomerClientRegistrar 是客户渠道册的登记口，受控批量口与将来的在线口消费它，答复代数一致。写入要落在调用方开的框架
// 事务里，理由同 OperatorRegistrar。
type CustomerClientRegistrar interface {
	RegisterCustomerClient(ctx context.Context, binding CustomerClientBinding) (CustomerClientRegistrationOutcome, error)
	RegisterCustomerClientGrant(ctx context.Context, grant CustomerClientGrant) (CustomerClientRegistrationOutcome, error)
	RegisterCustomerClientRevocation(ctx context.Context, revocation GrantRevocation) (CustomerClientRegistrationOutcome, error)
}
