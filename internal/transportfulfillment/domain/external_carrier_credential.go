package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	ErrInvalidExternalCarrierCredential = errors.New("transport fulfillment: invalid external carrier credential")
	// ErrCredentialNoLongerApplicable 与形状错误分格：对一份已作废、已失效或已替代的凭证再改
	// 适用关系，恢复动作不是改输入，而是去看它最后一版是谁、什么时候收掉的。
	ErrCredentialNoLongerApplicable = errors.New("transport fulfillment: external carrier credential is no longer applicable")
)

// CredentialAssignerReference 是凭证的分配方——外部运输服务提供方。它引用 party-commercial
// 已登记的参与方身份，本上下文不铸；渠道服务方、签约服务商、底层承运商与实际承运商各是各的
// 角色，分配方不推导其中任何一个（CONTEXT 规则节）。
type CredentialAssignerReference struct{ requiredValue }

func NewCredentialAssignerReference(value string) (CredentialAssignerReference, error) {
	required, err := newRequiredValue("credential assigner reference", value)
	return CredentialAssignerReference{required}, err
}

// ExternalCarrierCredentialVersion 是凭证的版本。作废、失效、替代都形成新版本回指前版，原版本
// 原样保留——CONTEXT 规则节「不得删除历史凭证或其关联履约事实」在类型上的落点。
type ExternalCarrierCredentialVersion struct{ requiredValue }

func NewExternalCarrierCredentialVersion(value string) (ExternalCarrierCredentialVersion, error) {
	required, err := newRequiredValue("external carrier credential version", value)
	return ExternalCarrierCredentialVersion{required}, err
}

// IdentifiedObjectKind 是凭证真实标识的对象属哪一类。CONTEXT「外部承运凭证」词条列的五类，封闭：
// 运输委托、订舱、班次、载运对象、实际履约段。类别是登记出来的，不从凭证字符串的格式推断。
type IdentifiedObjectKind uint8

const (
	IdentifiedObjectKindInvalid IdentifiedObjectKind = iota
	IdentifiesTransportCommission
	IdentifiesBooking
	IdentifiesTransportSchedule
	IdentifiesCarriedObject
	IdentifiesFulfillmentSegment
)

func (kind IdentifiedObjectKind) String() string {
	switch kind {
	case IdentifiesTransportCommission:
		return "TRANSPORT_COMMISSION"
	case IdentifiesBooking:
		return "BOOKING"
	case IdentifiesTransportSchedule:
		return "TRANSPORT_SCHEDULE"
	case IdentifiesCarriedObject:
		return "CARRIED_OBJECT"
	case IdentifiesFulfillmentSegment:
		return "FULFILLMENT_SEGMENT"
	default:
		return ""
	}
}

func (kind IdentifiedObjectKind) valid() bool {
	return kind >= IdentifiesTransportCommission && kind <= IdentifiesFulfillmentSegment
}

// ParseIdentifiedObjectKind 把库面或登记输入里的类别词认回封闭集合；词不在集合内即拒，不猜。
func ParseIdentifiedObjectKind(raw string) (IdentifiedObjectKind, error) {
	for kind := IdentifiesTransportCommission; kind <= IdentifiesFulfillmentSegment; kind++ {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return IdentifiedObjectKindInvalid, fmt.Errorf("%w: unknown identified object kind %q", ErrInvalidExternalCarrierCredential, raw)
}

// IdentifiedObject 是凭证的真实标识对象：类别加引用。CONTEXT 词条的原话是「不能全部解释为包裹
// 的当前运单号」——标识运输委托或班次的凭证不指向任何一个载运对象，CarriedObject 因此只对
// 载运对象那一类交回引用。
type IdentifiedObject struct {
	kind      IdentifiedObjectKind
	reference requiredValue
}

func NewIdentifiedObject(kind IdentifiedObjectKind, reference string) (IdentifiedObject, error) {
	if !kind.valid() {
		return IdentifiedObject{}, fmt.Errorf("%w: identified object kind", ErrInvalidExternalCarrierCredential)
	}
	required, err := newRequiredValue("identified object reference", reference)
	if err != nil {
		return IdentifiedObject{}, fmt.Errorf("%w: %v", ErrInvalidExternalCarrierCredential, err)
	}
	return IdentifiedObject{kind: kind, reference: required}, nil
}

func (object IdentifiedObject) Kind() IdentifiedObjectKind { return object.kind }
func (object IdentifiedObject) Reference() string          { return object.reference.String() }

// CarriedObject 只在凭证标识的就是一个载运对象时交回它。
func (object IdentifiedObject) CarriedObject() (CarriedObjectReference, bool) {
	if object.kind != IdentifiesCarriedObject {
		return CarriedObjectReference{}, false
	}
	return CarriedObjectReference{object.reference}, true
}

func (object IdentifiedObject) valid() bool {
	return object.kind.valid() && object.reference.valid()
}

// CredentialStanding 是凭证适用关系此刻的状态。作废、失效、替代三格分开：续办不同——作废要问
// 分配方为什么收回，失效是适用期间自然走完，替代要去看替代它的那一份。
type CredentialStanding uint8

const (
	CredentialStandingInvalid CredentialStanding = iota
	CredentialApplicable
	CredentialRevoked
	CredentialExpired
	CredentialSuperseded
)

func (standing CredentialStanding) String() string {
	switch standing {
	case CredentialApplicable:
		return "APPLICABLE"
	case CredentialRevoked:
		return "REVOKED"
	case CredentialExpired:
		return "EXPIRED"
	case CredentialSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

func (standing CredentialStanding) valid() bool {
	return standing >= CredentialApplicable && standing <= CredentialSuperseded
}

// ParseCredentialStanding 把库面或登记输入里的状态词认回封闭集合。
func ParseCredentialStanding(raw string) (CredentialStanding, error) {
	for standing := CredentialApplicable; standing <= CredentialSuperseded; standing++ {
		if standing.String() == raw {
			return standing, nil
		}
	}
	return CredentialStandingInvalid, fmt.Errorf("%w: unknown credential standing %q", ErrInvalidExternalCarrierCredential, raw)
}

// CredentialApplicability 是凭证的适用范围：左闭右开的业务时间区间，终点可开放。作废、失效、
// 替代只做一件事——把终点落定——这就是「只改变其适用关系」在形状上的意思。
type CredentialApplicability struct {
	from  time.Time
	until time.Time
}

// NewCredentialApplicability 起点必备；until 为零值表示开放，非零时必须晚于起点。
func NewCredentialApplicability(from, until time.Time) (CredentialApplicability, error) {
	if from.IsZero() {
		return CredentialApplicability{}, fmt.Errorf("%w: applicability start", ErrInvalidExternalCarrierCredential)
	}
	if !until.IsZero() && !until.After(from) {
		return CredentialApplicability{}, fmt.Errorf("%w: applicability end must be after its start", ErrInvalidExternalCarrierCredential)
	}
	return CredentialApplicability{from: from.UTC(), until: until.UTC()}, nil
}

func (applicability CredentialApplicability) From() time.Time { return applicability.from }

// Until 在终点已落定时交回它；开放的范围第二个返回值为 false。
func (applicability CredentialApplicability) Until() (time.Time, bool) {
	return applicability.until, !applicability.until.IsZero()
}

// Covers 判一个业务时刻落不落在范围里。
func (applicability CredentialApplicability) Covers(at time.Time) bool {
	if at.Before(applicability.from) {
		return false
	}
	return applicability.until.IsZero() || at.Before(applicability.until)
}

func (applicability CredentialApplicability) valid() bool {
	return !applicability.from.IsZero() && (applicability.until.IsZero() || applicability.until.After(applicability.from))
}

// closedAt 收掉终点。既定终点已过就不能再收——那是改写一段已经结束的区间。
func (applicability CredentialApplicability) closedAt(at time.Time) (CredentialApplicability, bool) {
	if at.IsZero() || at.Before(applicability.from) {
		return CredentialApplicability{}, false
	}
	if !applicability.until.IsZero() && at.After(applicability.until) {
		return CredentialApplicability{}, false
	}
	return CredentialApplicability{from: applicability.from, until: at.UTC()}, true
}

// ExternalCarrierCredentialSpec 是登记一份凭证首版所需的全部输入：CONTEXT 词条点名的分配方、
// 真实标识对象、适用范围、版本，加凭证身份与租户。
type ExternalCarrierCredentialSpec struct {
	TenantID      TenantID
	Credential    ExternalCarrierCredentialReference
	Version       ExternalCarrierCredentialVersion
	Assigner      CredentialAssignerReference
	Identifies    IdentifiedObject
	Applicability CredentialApplicability
}

// ExternalCarrierCredential 是外部运输服务提供方为运输委托、订舱、班次、载运对象或实际履约段
// 分配的业务凭证的一个版本（CONTEXT「外部承运凭证」）。值语义：任何改变都交回新版本，原值不动。
type ExternalCarrierCredential struct {
	tenantID      TenantID
	credential    ExternalCarrierCredentialReference
	version       ExternalCarrierCredentialVersion
	assigner      CredentialAssignerReference
	identifies    IdentifiedObject
	applicability CredentialApplicability
	standing      CredentialStanding
	// changedAt 是适用关系改变的业务时间，首版没有。它与登记落库的时刻是两个时间。
	changedAt  time.Time
	supersedes ExternalCarrierCredentialVersion
	replacedBy ExternalCarrierCredentialReference
}

// RegisterExternalCarrierCredential 是登记的构造门：首版必定适用中、不回指任何前版。
func RegisterExternalCarrierCredential(spec ExternalCarrierCredentialSpec) (ExternalCarrierCredential, error) {
	if !spec.TenantID.valid() || !spec.Credential.valid() || !spec.Version.valid() ||
		!spec.Assigner.valid() || !spec.Identifies.valid() || !spec.Applicability.valid() {
		return ExternalCarrierCredential{}, ErrInvalidExternalCarrierCredential
	}
	return ExternalCarrierCredential{
		tenantID:      spec.TenantID,
		credential:    spec.Credential,
		version:       spec.Version,
		assigner:      spec.Assigner,
		identifies:    spec.Identifies,
		applicability: spec.Applicability,
		standing:      CredentialApplicable,
	}, nil
}

func (credential ExternalCarrierCredential) TenantID() TenantID { return credential.tenantID }
func (credential ExternalCarrierCredential) Credential() ExternalCarrierCredentialReference {
	return credential.credential
}
func (credential ExternalCarrierCredential) Version() ExternalCarrierCredentialVersion {
	return credential.version
}
func (credential ExternalCarrierCredential) Assigner() CredentialAssignerReference {
	return credential.assigner
}
func (credential ExternalCarrierCredential) Identifies() IdentifiedObject {
	return credential.identifies
}
func (credential ExternalCarrierCredential) Applicability() CredentialApplicability {
	return credential.applicability
}
func (credential ExternalCarrierCredential) Standing() CredentialStanding { return credential.standing }
func (credential ExternalCarrierCredential) Applicable() bool {
	return credential.standing == CredentialApplicable
}

// ChangedAt 是本版本改变适用关系的业务时间；首版第二个返回值为 false。
func (credential ExternalCarrierCredential) ChangedAt() (time.Time, bool) {
	return credential.changedAt, !credential.changedAt.IsZero()
}

// Supersedes 交回本版本回指的前版；首版第二个返回值为 false。
func (credential ExternalCarrierCredential) Supersedes() (ExternalCarrierCredentialVersion, bool) {
	return credential.supersedes, credential.supersedes.valid()
}

// ReplacedBy 只在已替代时交回替代它的凭证。
func (credential ExternalCarrierCredential) ReplacedBy() (ExternalCarrierCredentialReference, bool) {
	return credential.replacedBy, credential.replacedBy.valid()
}

// Revoke 作废：分配方收回了凭证。适用范围的终点落在 at，其余一字不动。
func (credential ExternalCarrierCredential) Revoke(
	at time.Time,
	version ExternalCarrierCredentialVersion,
) (ExternalCarrierCredential, error) {
	return credential.changeApplicability(CredentialRevoked, at, version, ExternalCarrierCredentialReference{})
}

// Expire 失效：适用期间走完。
func (credential ExternalCarrierCredential) Expire(
	at time.Time,
	version ExternalCarrierCredentialVersion,
) (ExternalCarrierCredential, error) {
	return credential.changeApplicability(CredentialExpired, at, version, ExternalCarrierCredentialReference{})
}

// Supersede 替代：另一份凭证接替了它。替代者必备且不能是自己。
func (credential ExternalCarrierCredential) Supersede(
	at time.Time,
	replacement ExternalCarrierCredentialReference,
	version ExternalCarrierCredentialVersion,
) (ExternalCarrierCredential, error) {
	if !replacement.valid() || replacement == credential.credential {
		return ExternalCarrierCredential{}, ErrInvalidExternalCarrierCredential
	}
	return credential.changeApplicability(CredentialSuperseded, at, version, replacement)
}

// changeApplicability 是三种改变共用的一扇门：只对适用中的版本开放，新版本回指本版、终点落定，
// 分配方与标识对象原样带过去。沿用原版本号就是覆盖，拒。
func (credential ExternalCarrierCredential) changeApplicability(
	standing CredentialStanding,
	at time.Time,
	version ExternalCarrierCredentialVersion,
	replacement ExternalCarrierCredentialReference,
) (ExternalCarrierCredential, error) {
	if !credential.Applicable() {
		return ExternalCarrierCredential{}, ErrCredentialNoLongerApplicable
	}
	if !version.valid() || version == credential.version {
		return ExternalCarrierCredential{}, ErrInvalidExternalCarrierCredential
	}
	closed, ok := credential.applicability.closedAt(at)
	if !ok {
		return ExternalCarrierCredential{}, ErrInvalidExternalCarrierCredential
	}
	changed := credential
	changed.version = version
	changed.applicability = closed
	changed.standing = standing
	changed.changedAt = at.UTC()
	changed.supersedes = credential.version
	changed.replacedBy = replacement
	return changed, nil
}

// Equal 按业务内容比较两个版本——同键异内容要答`内容冲突`而不是`已登记`（ADR-0031），比的是
// 这里的字段而不是结构体相等：time.Time 的 == 会被单调时钟读数搅掉。
func (credential ExternalCarrierCredential) Equal(other ExternalCarrierCredential) bool {
	return credential.tenantID == other.tenantID &&
		credential.credential == other.credential &&
		credential.version == other.version &&
		credential.assigner == other.assigner &&
		credential.identifies == other.identifies &&
		credential.applicability.from.Equal(other.applicability.from) &&
		credential.applicability.until.Equal(other.applicability.until) &&
		credential.standing == other.standing &&
		credential.changedAt.Equal(other.changedAt) &&
		credential.supersedes == other.supersedes &&
		credential.replacedBy == other.replacedBy
}
