package accessidentity

import (
	"context"
	"errors"
	"strings"
	"time"
)

// ErrInvalidOperatorSubject 表示操作者主体缺发行方或 sub。
var ErrInvalidOperatorSubject = errors.New("access identity: operator subject needs both issuer and subject")

// ErrIncompleteOperatorRegistration 表示操作者册的一行缺件，建不成。
var ErrIncompleteOperatorRegistration = errors.New("access identity: operator registration is incomplete")

// OperatorSubject 是操作者主体：发行方 + `sub`（ADR-0100 决定二第三条）。
//
// 两件合起来才是身份——`sub` 只在它的发行方之内唯一，单拿 `sub` 当键，换一个发行方就会
// 撞上另一个人。库里登的只是这两个标识，凭据本体不落库（ADR-0100 决定二第二条）。
type OperatorSubject struct {
	issuer  string
	subject string
}

func NewOperatorSubject(issuer, subject string) (OperatorSubject, error) {
	trimmedIssuer := strings.TrimSpace(issuer)
	trimmedSubject := strings.TrimSpace(subject)
	if trimmedIssuer == "" || trimmedSubject == "" {
		return OperatorSubject{}, ErrInvalidOperatorSubject
	}
	return OperatorSubject{issuer: trimmedIssuer, subject: trimmedSubject}, nil
}

func (subject OperatorSubject) Issuer() string  { return subject.issuer }
func (subject OperatorSubject) Subject() string { return subject.subject }

// OperatorBinding 是操作者册里「一个操作者主体绑定唯一租户」那一行。
//
// 绑定没有生效区间也不可撤销：ADR-0100 给区间与撤销的是授予，不是绑定。一个操作者离开
// 就撤掉他名下的授予，绑定作为「这个主体属于哪个租户」的事实留在册上；让绑定能改指，
// 等于让同一个主体先后属于两个租户，而那正是 ADR-0100 说不存在的跨租户主体。
type OperatorBinding struct {
	subject  OperatorSubject
	tenantID string
	basis    string
}

// NewOperatorBinding 要求登记依据非空：册上每一行都要答得出凭什么登的，行是租户取值，
// 依据指向该租户的受控证据（参数登记册 PAR-INT-08）。
func NewOperatorBinding(subject OperatorSubject, tenantID, basis string) (OperatorBinding, error) {
	tenant := strings.TrimSpace(tenantID)
	reference := strings.TrimSpace(basis)
	if subject.issuer == "" || tenant == "" || reference == "" {
		return OperatorBinding{}, ErrIncompleteOperatorRegistration
	}
	return OperatorBinding{subject: subject, tenantID: tenant, basis: reference}, nil
}

func (binding OperatorBinding) Subject() OperatorSubject { return binding.subject }
func (binding OperatorBinding) TenantID() string         { return binding.tenantID }
func (binding OperatorBinding) Basis() string            { return binding.basis }

// ErrCapabilityFaceUnknown 表示给出的能力面不在 ADR-0100 定下的那几格里。
var ErrCapabilityFaceUnknown = errors.New("access identity: capability face is unknown")

// ErrCapabilityFaceReserved 表示这一格能力面只预留、授予模型未裁，此刻登不进册。
//
// 它与 ErrCapabilityFaceUnknown 分开：前者是「产品认得这一格、还没定怎么授」，恢复动作是等
// 那份裁决；后者是批文写错了，恢复动作是改批文。
var ErrCapabilityFaceReserved = errors.New("access identity: capability face is reserved and not grantable yet")

// CapabilityFace 是授予的单位：操作者能在哪一族端点上做什么（ADR-0100 决定二第三条）。
//
// 这几格是册的**列**，由产品的授权模型定死；某个租户给谁授了哪一格才是行。
type CapabilityFace string

const (
	// CapabilityRegistryConfigurationWrite 是登记册配置写：ADR-0085 决定一那一族登记端点。
	CapabilityRegistryConfigurationWrite CapabilityFace = "REGISTRY_CONFIGURATION_WRITE"
	// CapabilityMasterDataAndOperationsRead 是主数据与运营查阅读：ADR-0077 那一族目录查阅端点。
	CapabilityMasterDataAndOperationsRead CapabilityFace = "MASTER_DATA_AND_OPERATIONS_READ"
)

// reservedGovernanceRegistration 是治理登记那一格：ADR-0100 只预留它，授予模型按 ADR-0085
// 决定四另裁。不导出成 CapabilityFace 常量，是为了让包外没有一个现成的值可以拿去授；
// 裁决落地时它连同迁移里的 CHECK 一起放开。
const reservedGovernanceRegistration = "GOVERNANCE_REGISTRATION"

// ParseCapabilityFace 按字面取值认能力面，大小写不折叠：册里存的就是这些字面量。
func ParseCapabilityFace(value string) (CapabilityFace, error) {
	face := CapabilityFace(value)
	if err := face.checkGrantable(); err != nil {
		return "", err
	}
	return face, nil
}

// checkGrantable 是能力面唯一的入口闸：解析与构造授予都经它，CapabilityFace 是字符串底型，
// 包外转型递进来的值也要在这里被拦下。
func (face CapabilityFace) checkGrantable() error {
	switch face {
	case CapabilityRegistryConfigurationWrite, CapabilityMasterDataAndOperationsRead:
		return nil
	case reservedGovernanceRegistration:
		return ErrCapabilityFaceReserved
	}
	return ErrCapabilityFaceUnknown
}

func (face CapabilityFace) String() string { return string(face) }

// ErrInvalidEffectiveInterval 表示生效区间缺起点，或终点不晚于起点。
var ErrInvalidEffectiveInterval = errors.New("access identity: effective interval needs a start and an end after it")

// EffectiveInterval 是授予的生效区间：含起点、不含终点；终点缺席即不设终点。
//
// 「区间外不生效」由区间对时点导出，不存成状态：存一个「已到期」就是存一份会过期的推导。
type EffectiveInterval struct {
	startsAt time.Time
	endsAt   time.Time
}

// NewEffectiveInterval 的 endsAt 取零值表示不设终点。起点不代填：没有「从现在起」这种缺省，
// 缺省的现在取自哪台机器的钟、哪一刻，事后都答不出。
func NewEffectiveInterval(startsAt, endsAt time.Time) (EffectiveInterval, error) {
	if startsAt.IsZero() || (!endsAt.IsZero() && !endsAt.After(startsAt)) {
		return EffectiveInterval{}, ErrInvalidEffectiveInterval
	}
	return EffectiveInterval{startsAt: startsAt, endsAt: endsAt}, nil
}

func (interval EffectiveInterval) StartsAt() time.Time { return interval.startsAt }

func (interval EffectiveInterval) EndsAt() (time.Time, bool) {
	return interval.endsAt, !interval.endsAt.IsZero()
}

func (interval EffectiveInterval) Contains(at time.Time) bool {
	if at.Before(interval.startsAt) {
		return false
	}
	return interval.endsAt.IsZero() || at.Before(interval.endsAt)
}

// OperatorGrant 是按能力面显式登记的一笔授予（ADR-0100 决定二第三条）。
//
// 授予带自己的登记标识而不以（主体、能力面）为键：同一个操作者可以在不同期间先后被授同一格，
// 撤了再授是两笔授予，不是一笔的两次修改——拿（主体、能力面）当键，后一笔就得覆盖前一笔，
// 而前一笔正是那段时间里他确实有权的证据。
type OperatorGrant struct {
	tenantID string
	grantID  string
	subject  OperatorSubject
	face     CapabilityFace
	interval EffectiveInterval
	basis    string
}

func NewOperatorGrant(
	tenantID string,
	grantID string,
	subject OperatorSubject,
	face CapabilityFace,
	interval EffectiveInterval,
	basis string,
) (OperatorGrant, error) {
	if err := face.checkGrantable(); err != nil {
		return OperatorGrant{}, err
	}
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || subject.issuer == "" || interval.startsAt.IsZero() || reference == "" {
		return OperatorGrant{}, ErrIncompleteOperatorRegistration
	}
	return OperatorGrant{
		tenantID: tenant,
		grantID:  id,
		subject:  subject,
		face:     face,
		interval: interval,
		basis:    reference,
	}, nil
}

func (grant OperatorGrant) TenantID() string            { return grant.tenantID }
func (grant OperatorGrant) GrantID() string             { return grant.grantID }
func (grant OperatorGrant) Subject() OperatorSubject    { return grant.subject }
func (grant OperatorGrant) Face() CapabilityFace        { return grant.face }
func (grant OperatorGrant) Interval() EffectiveInterval { return grant.interval }
func (grant OperatorGrant) Basis() string               { return grant.basis }

// GrantRevocation 是对一笔授予的撤销：自撤销时刻起（含该时刻）那笔授予不再生效。
//
// 时刻与依据缺一不成立：只记时刻答不出凭什么撤，只记依据答不出从哪一刻起不能用，而此后
// 每一次授予核查要的正是那一刻。撤销是一次外部决定，无处可导，所以它是登记，不是推导。
type GrantRevocation struct {
	tenantID  string
	grantID   string
	revokedAt time.Time
	basis     string
}

func NewGrantRevocation(tenantID, grantID string, revokedAt time.Time, basis string) (GrantRevocation, error) {
	tenant := strings.TrimSpace(tenantID)
	id := strings.TrimSpace(grantID)
	reference := strings.TrimSpace(basis)
	if tenant == "" || id == "" || revokedAt.IsZero() || reference == "" {
		return GrantRevocation{}, ErrIncompleteOperatorRegistration
	}
	return GrantRevocation{tenantID: tenant, grantID: id, revokedAt: revokedAt, basis: reference}, nil
}

func (revocation GrantRevocation) TenantID() string     { return revocation.tenantID }
func (revocation GrantRevocation) GrantID() string      { return revocation.grantID }
func (revocation GrantRevocation) RevokedAt() time.Time { return revocation.revokedAt }
func (revocation GrantRevocation) Basis() string        { return revocation.basis }

// ErrRevocationDoesNotMatchGrant 表示一笔撤销被挂到了它没撤的那笔授予上。
var ErrRevocationDoesNotMatchGrant = errors.New("access identity: revocation belongs to another grant")

// RecordedGrant 是册上的一笔授予，连同它的撤销（若有）。
type RecordedGrant struct {
	grant      OperatorGrant
	revocation GrantRevocation
	revoked    bool
}

// NewRecordedGrant 的 revocation 取 nil 表示未撤销。
func NewRecordedGrant(grant OperatorGrant, revocation *GrantRevocation) (RecordedGrant, error) {
	if revocation == nil {
		return RecordedGrant{grant: grant}, nil
	}
	if revocation.tenantID != grant.tenantID || revocation.grantID != grant.grantID {
		return RecordedGrant{}, ErrRevocationDoesNotMatchGrant
	}
	return RecordedGrant{grant: grant, revocation: *revocation, revoked: true}, nil
}

func (recorded RecordedGrant) Grant() OperatorGrant { return recorded.grant }

func (recorded RecordedGrant) Revocation() (GrantRevocation, bool) {
	return recorded.revocation, recorded.revoked
}

// EffectiveAt 答这笔授予在 at 那一刻生不生效：在区间内，且还没到撤销时刻。
func (recorded RecordedGrant) EffectiveAt(at time.Time) bool {
	if !recorded.grant.interval.Contains(at) {
		return false
	}
	return !recorded.revoked || at.Before(recorded.revocation.revokedAt)
}

// ErrGrantOutsideBinding 表示现状里混进了别的主体或别的租户名下的授予。
var ErrGrantOutsideBinding = errors.New("access identity: grant does not belong to the operator binding")

// OperatorStanding 是一个操作者主体在册上的全部：它绑定的租户，与它名下每一笔授予（连同撤销）。
//
// 它只答「册上怎么写」，不答「这次请求准不准」：后者还要比对请求所在的租户与所要的能力面，
// 那是铸造信封那一步的事（ADR-0100 决定三、四）。
type OperatorStanding struct {
	binding OperatorBinding
	grants  []RecordedGrant
}

func NewOperatorStanding(binding OperatorBinding, grants []RecordedGrant) (OperatorStanding, error) {
	copied := make([]RecordedGrant, 0, len(grants))
	for _, recorded := range grants {
		if recorded.grant.subject != binding.subject || recorded.grant.tenantID != binding.tenantID {
			return OperatorStanding{}, ErrGrantOutsideBinding
		}
		copied = append(copied, recorded)
	}
	return OperatorStanding{binding: binding, grants: copied}, nil
}

func (standing OperatorStanding) Binding() OperatorBinding { return standing.binding }

func (standing OperatorStanding) Grants() []RecordedGrant {
	return append([]RecordedGrant(nil), standing.grants...)
}

// HoldsAt 答这个操作者在 at 那一刻是否持有 face：名下有任何一笔该格的授予此刻生效即是。
func (standing OperatorStanding) HoldsAt(face CapabilityFace, at time.Time) bool {
	for _, recorded := range standing.grants {
		if recorded.grant.face == face && recorded.EffectiveAt(at) {
			return true
		}
	}
	return false
}

// OperatorRegistry 是操作者册的装载口：按主体取它在册上的全部。
//
// 主体不在册时答 found=false 而**不返回 error**，理由与 ChannelRegistry 同款：空册与读不动
// 是两件事，恢复动作一个是去登记、一个是去救依赖。
type OperatorRegistry interface {
	FindOperator(ctx context.Context, subject OperatorSubject) (OperatorStanding, bool, error)
}

// OperatorRegistrationOutcome 是操作者册登记口的答复。撞键从不覆盖：同内容重放答原结果，
// 异内容答冲突，册上已有的那一行原样不动。
type OperatorRegistrationOutcome string

const (
	OperatorRegistrationRecorded          OperatorRegistrationOutcome = "RECORDED"
	OperatorRegistrationAlreadyRegistered OperatorRegistrationOutcome = "ALREADY_REGISTERED"
	OperatorRegistrationContentConflict   OperatorRegistrationOutcome = "CONTENT_CONFLICT"
	// OperatorSubjectBoundToAnotherTenant 只由 RegisterOperator 答：这个主体已绑在别的租户上。
	// 它不说是哪个租户——登记口背后的人换一个租户上下文就能看见，而那正是 ADR-0003 的最高
	// 隔离边界要挡的。
	OperatorSubjectBoundToAnotherTenant OperatorRegistrationOutcome = "SUBJECT_BOUND_TO_ANOTHER_TENANT"
	// OperatorNotRegistered 只由 RegisterGrant 答：本租户册上没有这个主体。主体绑在别的租户
	// 上也答这一格而不是上一格——从本租户看，它就是不在册。
	OperatorNotRegistered OperatorRegistrationOutcome = "OPERATOR_NOT_REGISTERED"
	// OperatorGrantNotRegistered 只由 RegisterRevocation 答：本租户册上没有这笔授予。
	OperatorGrantNotRegistered OperatorRegistrationOutcome = "GRANT_NOT_REGISTERED"
)

func (outcome OperatorRegistrationOutcome) String() string { return string(outcome) }

// OperatorRegistrar 是操作者册的登记口。受控批量口（ADR-0085 决定一、ADR-0100 决定六）与将来
// 的在线口消费它，答复代数一致。
//
// 写入要落在调用方开的框架事务里：本仓的写一律经框架事务而不是裸连接，实现方拿不到事务
// 句柄时应报错，不退回连接池。
type OperatorRegistrar interface {
	RegisterOperator(ctx context.Context, binding OperatorBinding) (OperatorRegistrationOutcome, error)
	RegisterGrant(ctx context.Context, grant OperatorGrant) (OperatorRegistrationOutcome, error)
	RegisterRevocation(ctx context.Context, revocation GrantRevocation) (OperatorRegistrationOutcome, error)
}
