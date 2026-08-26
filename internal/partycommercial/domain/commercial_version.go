// Package domain 承载参与方与商业的领域模型：参与方身份、商业关系，以及供其他上下文
// 解析的版本化商业定义。它不拥有可执行价卡、客户委托或结算事实。
package domain

import (
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrBlankValue                      = errors.New("party commercial: blank value")
	ErrInvalidCommercialVersion        = errors.New("party commercial: invalid commercial version")
	ErrInvalidEffectiveInterval        = errors.New("party commercial: invalid effective interval")
	ErrIncompleteCommercialPublication = errors.New("party commercial: incomplete commercial publication")
	// ErrApprovalRoleNotConfirmed 是 AT-PC-010：依据字段可以齐全，但批准角色尚未确认。
	// 它绝不是 Incomplete——补字段推不动，要等角色确认；草稿与来源必须原样保留（ADR-0035）。
	ErrApprovalRoleNotConfirmed = errors.New("party commercial: approval role is not confirmed for publication")
	// ErrNamedReferenceNotPublished 是 AT-PC-005：正文指名的引用尚未发布。恢复动作是等被引
	// 对象发布，草稿来源必须保留；不得压成 Incomplete 或角色未确认（ADR-0036）。
	ErrNamedReferenceNotPublished  = errors.New("party commercial: a named reference is not published")
	ErrCommercialContentIsFixed    = errors.New("party commercial: published commercial content is fixed")
	ErrInvalidCommercialTransition = errors.New("party commercial: invalid commercial version transition")
)

type requiredValue struct {
	value string
}

func newRequiredValue(name, value string) (requiredValue, error) {
	if strings.TrimSpace(value) == "" {
		return requiredValue{}, fmt.Errorf("%w: %s", ErrBlankValue, name)
	}
	return requiredValue{value: value}, nil
}

func (value requiredValue) String() string {
	return value.value
}

func (value requiredValue) valid() bool {
	return strings.TrimSpace(value.value) != ""
}

type CommercialObjectID struct{ requiredValue }

func NewCommercialObjectID(value string) (CommercialObjectID, error) {
	required, err := newRequiredValue("commercial object ID", value)
	return CommercialObjectID{required}, err
}

type CommercialVersionLabel struct{ requiredValue }

func NewCommercialVersionLabel(value string) (CommercialVersionLabel, error) {
	required, err := newRequiredValue("commercial version label", value)
	return CommercialVersionLabel{required}, err
}

type CommercialScopeReference struct{ requiredValue }

func NewCommercialScopeReference(value string) (CommercialScopeReference, error) {
	required, err := newRequiredValue("commercial scope reference", value)
	return CommercialScopeReference{required}, err
}

type CommercialContentDigest struct{ requiredValue }

func NewCommercialContentDigest(value string) (CommercialContentDigest, error) {
	required, err := newRequiredValue("commercial content digest", value)
	return CommercialContentDigest{required}, err
}

type ApprovalReference struct{ requiredValue }

func NewApprovalReference(value string) (ApprovalReference, error) {
	required, err := newRequiredValue("approval reference", value)
	return ApprovalReference{required}, err
}

type CommercialSourceReference struct{ requiredValue }

func NewCommercialSourceReference(value string) (CommercialSourceReference, error) {
	required, err := newRequiredValue("commercial source reference", value)
	return CommercialSourceReference{required}, err
}

type RetirementReference struct{ requiredValue }

func NewRetirementReference(value string) (RetirementReference, error) {
	required, err := newRequiredValue("retirement reference", value)
	return RetirementReference{required}, err
}

// CommercialObjectKind 是遵循商业版本共同不变量的对象封闭集合。货主客户账户与责任
// 法人刻意不在其中：它们是参与方身份、走自己的生命周期，不是商业版本。
type CommercialObjectKind uint8

const (
	CommercialObjectKindInvalid CommercialObjectKind = iota
	ServiceProductObject
	CustomerContractObject
	SupplierAgreementObject
	AcceptanceRulePackageObject
	PreAcceptanceFinancialControlPolicyObject
	PriceRuleObject
	SettlementPolicyObject
	CreditPolicyObject
	AuthorizationRuleObject
)

func (kind CommercialObjectKind) valid() bool {
	return kind >= ServiceProductObject && kind <= AuthorizationRuleObject
}

func (kind CommercialObjectKind) String() string {
	switch kind {
	case ServiceProductObject:
		return "SERVICE_PRODUCT"
	case CustomerContractObject:
		return "CUSTOMER_CONTRACT"
	case SupplierAgreementObject:
		return "SUPPLIER_AGREEMENT"
	case AcceptanceRulePackageObject:
		return "ACCEPTANCE_RULE_PACKAGE"
	case PreAcceptanceFinancialControlPolicyObject:
		return "PRE_ACCEPTANCE_FINANCIAL_CONTROL_POLICY"
	case PriceRuleObject:
		return "PRICE_RULE"
	case SettlementPolicyObject:
		return "SETTLEMENT_POLICY"
	case CreditPolicyObject:
		return "CREDIT_POLICY"
	case AuthorizationRuleObject:
		return "AUTHORIZATION_RULE"
	default:
		return ""
	}
}

// CommercialVersionStatus 遵循 CONTEXT 的生命周期。批准是发布的完备性条件而非独立
// 状态，所以草稿与已发布之间没有 `APPROVED`。
type CommercialVersionStatus uint8

const (
	CommercialVersionStatusInvalid CommercialVersionStatus = iota
	CommercialVersionDraft
	CommercialVersionPublished
	CommercialVersionEffective
	CommercialVersionExpired
	CommercialVersionRetired
	CommercialVersionSuperseded
)

func (status CommercialVersionStatus) String() string {
	switch status {
	case CommercialVersionDraft:
		return "DRAFT"
	case CommercialVersionPublished:
		return "PUBLISHED"
	case CommercialVersionEffective:
		return "EFFECTIVE"
	case CommercialVersionExpired:
		return "EXPIRED"
	case CommercialVersionRetired:
		return "RETIRED"
	case CommercialVersionSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

type EffectiveInterval struct {
	startsAt time.Time
	endsAt   time.Time
}

// NewEffectiveInterval 接受开放结束：一个版本可以一直适用到被到期、退役或替代，而不
// 必在发布时就定死一个终止日期。
func NewEffectiveInterval(startsAt, endsAt time.Time) (EffectiveInterval, error) {
	if startsAt.IsZero() || (!endsAt.IsZero() && !endsAt.After(startsAt)) {
		return EffectiveInterval{}, ErrInvalidEffectiveInterval
	}
	return EffectiveInterval{startsAt: startsAt.UTC(), endsAt: endsAt.UTC()}, nil
}

func (interval EffectiveInterval) StartsAt() time.Time {
	return interval.startsAt
}

func (interval EffectiveInterval) EndsAt() (time.Time, bool) {
	if interval.endsAt.IsZero() {
		return time.Time{}, false
	}
	return interval.endsAt, true
}

func (interval EffectiveInterval) Contains(at time.Time) bool {
	if interval.startsAt.IsZero() || at.Before(interval.startsAt) {
		return false
	}
	return interval.endsAt.IsZero() || at.Before(interval.endsAt)
}

func (interval EffectiveInterval) valid() bool {
	return !interval.startsAt.IsZero() &&
		(interval.endsAt.IsZero() || interval.endsAt.After(interval.startsAt))
}

// ApprovalBasis 是一次发布必须携带的批准与来源依据。它是一个整体值对象：批准引用缺了
// 来源、或两者缺了时间，都支撑不起它本该证明的那次发布。
type ApprovalBasis struct {
	reference  ApprovalReference
	source     CommercialSourceReference
	approvedAt time.Time
}

func NewApprovalBasis(
	reference ApprovalReference,
	source CommercialSourceReference,
	approvedAt time.Time,
) (ApprovalBasis, error) {
	if !reference.valid() || !source.valid() || approvedAt.IsZero() {
		return ApprovalBasis{}, ErrIncompleteCommercialPublication
	}
	return ApprovalBasis{reference: reference, source: source, approvedAt: approvedAt.UTC()}, nil
}

func (basis ApprovalBasis) Reference() ApprovalReference {
	return basis.reference
}

func (basis ApprovalBasis) Source() CommercialSourceReference {
	return basis.source
}

func (basis ApprovalBasis) ApprovedAt() time.Time {
	return basis.approvedAt
}

func (basis ApprovalBasis) valid() bool {
	return basis.reference.valid() && basis.source.valid() && !basis.approvedAt.IsZero()
}

// ApprovalRoleStanding 是「这次发布所要求的批准角色是否已确认」的答复。
// 零值 = 未确认：忘了作答的调用点停在 AT-PC-010 那一格，而不是默认发布成功（ADR-0035）。
type ApprovalRoleStanding uint8

const (
	ApprovalRoleStandingInvalid ApprovalRoleStanding = iota
	ApprovalRoleUnconfirmed
	ApprovalRoleConfirmed
)

func (standing ApprovalRoleStanding) String() string {
	switch standing {
	case ApprovalRoleUnconfirmed:
		return "UNCONFIRMED"
	case ApprovalRoleConfirmed:
		return "CONFIRMED"
	default:
		return ""
	}
}

// NamedReferenceStanding 是「正文指名的某个对象此刻是否已发布」的答复。
// 零值 = 未确认：忘了作答不得默认放行（ADR-0036）。
type NamedReferenceStanding uint8

const (
	NamedReferenceStandingInvalid NamedReferenceStanding = iota
	NamedReferenceUnpublished
	NamedReferencePublished
)

func (standing NamedReferenceStanding) String() string {
	switch standing {
	case NamedReferenceUnpublished:
		return "UNPUBLISHED"
	case NamedReferencePublished:
		return "PUBLISHED"
	default:
		return ""
	}
}

// NamedReferenceStandingLookup 由调用方（通常据登记册）回答正文指名引用的发布存续。
type NamedReferenceStandingLookup func(kind CommercialObjectKind, objectID CommercialObjectID) NamedReferenceStanding

type CommercialVersionSpec struct {
	// TenantID 是版本身份的最高隔离边界（ADR-0040 / ADR-0003）。跨租户可以合法共用
	// 同一 objectID+version，二者不是同一次发布。
	TenantID      TenantID
	Kind          CommercialObjectKind
	ObjectID      CommercialObjectID
	Version       CommercialVersionLabel
	Scope         CommercialScopeReference
	ContentDigest CommercialContentDigest
	Effective     EffectiveInterval
	// References 是本版本正文里指名的对外引用，按被引对象类型归档。它随规格给出而不是
	// 登记之后再挂上去：一份合同指名哪个接单规则包，与它的有效区间一样属于那次受控发布
	// 固定下来的内容，事后可改的引用会让「发布后正文不可覆盖」出现一个缺口。
	References map[CommercialObjectKind]CommercialObjectID
}

// CommercialVersion 是一次受控发布形成的商业定义。它是值类型：每次转换返回新值、不改
// 接收者——这让「发布后正文不可覆盖」成为结构性事实，而不是一条需要有人记住的规则。
type CommercialVersion struct {
	tenant        TenantID
	kind          CommercialObjectKind
	objectID      CommercialObjectID
	version       CommercialVersionLabel
	scope         CommercialScopeReference
	contentDigest CommercialContentDigest
	effective     EffectiveInterval
	status        CommercialVersionStatus
	approval      ApprovalBasis
	publishedAt   time.Time
	effectiveAt   time.Time
	closedAt      time.Time
	retirementRef RetirementReference
	successor     CommercialVersionLabel
	// references 存成有序切片而不是 map，是为了让引用集合有稳定的逐项比对顺序：它参与
	// sameReleasedContent 判定「是不是同一次发布」，而声明顺序不构成不同的内容。
	//
	// 代价是 CommercialVersion 不再可比较，并且连带把它当字段持有的类型一并拖走——本包
	// 12 个（实测于 9b097c9）。这一条是对着那 12 个的尺寸接受的，不是对着一行编译错：全仓
	// 真拿 `==` 比过两个版本的只有一处且在测试里（同上时点；编译器会把每一处都报出来，
	// 那次只报了它）。何况 `==` 本来也答不了「是不是同一个版本」——同一次发布先后处在
	// 草稿、生效、已退役几个位置上，逐字段相等会把它们判成三个。要问身份用 SameVersionAs。
	//
	// 放弃可比较性不会静默出错——这一条靠三条支撑，其中两条永久成立：想 `==` 或拿它当 map
	// 键都是编译错，吵得见。第三条会过期：唯一能静默改变语义的是 reflect.DeepEqual（nil 切片
	// 与空切片不等），本包当时一处都没有（同上时点）。
	//
	// 所以第三条写成入口条件而不是现状描述：一旦有人对本类型、或对持有它的那 12 个类型之一
	// 用上 reflect.DeepEqual，「代价为零」就不再成立，届时该改的是这条立场的论证，而不是把它
	// 悄悄留着。这里有意不加门禁——扫 reflect.DeepEqual 会误伤一大片正当用法。
	references []DeclaredReference
}

// DeclaredReference 是一个版本在正文里指名的一条对外引用。它只说「指向谁」，不说那个
// 对象此刻可不可用——后者要去问引用的所有方，而这正是解析必须跟随引用的原因。
type DeclaredReference struct {
	kind     CommercialObjectKind
	objectID CommercialObjectID
}

func (reference DeclaredReference) Kind() CommercialObjectKind {
	return reference.kind
}

func (reference DeclaredReference) ObjectID() CommercialObjectID {
	return reference.objectID
}

func NewCommercialDraft(spec CommercialVersionSpec) (CommercialVersion, error) {
	if !spec.TenantID.valid() ||
		!spec.Kind.valid() ||
		!spec.ObjectID.valid() ||
		!spec.Version.valid() ||
		!spec.Scope.valid() ||
		!spec.ContentDigest.valid() ||
		!spec.Effective.valid() {
		return CommercialVersion{}, ErrInvalidCommercialVersion
	}
	references, err := declaredReferences(spec.Kind, spec.ObjectID, spec.References)
	if err != nil {
		return CommercialVersion{}, err
	}
	return CommercialVersion{
		tenant:        spec.TenantID,
		kind:          spec.Kind,
		objectID:      spec.ObjectID,
		version:       spec.Version,
		scope:         spec.Scope,
		contentDigest: spec.ContentDigest,
		effective:     spec.Effective,
		status:        CommercialVersionDraft,
		references:    references,
	}, nil
}

// declaredReferences 把指名引用整理成稳定顺序。顺序稳定是必需的：引用集合参与「同一次
// 发布」的判定，而声明顺序不构成不同的内容。
//
// 自引用被拒：一个版本指名自己所属的对象，会让「跟随引用」变成一个绕不出去的圈。
func declaredReferences(
	kind CommercialObjectKind,
	objectID CommercialObjectID,
	declared map[CommercialObjectKind]CommercialObjectID,
) ([]DeclaredReference, error) {
	if len(declared) == 0 {
		return nil, nil
	}
	references := make([]DeclaredReference, 0, len(declared))
	for referencedKind, referencedID := range declared {
		if !referencedKind.valid() || !referencedID.valid() {
			return nil, ErrInvalidCommercialVersion
		}
		if referencedKind == kind && referencedID == objectID {
			return nil, ErrInvalidCommercialVersion
		}
		references = append(references, DeclaredReference{kind: referencedKind, objectID: referencedID})
	}
	sort.Slice(references, func(left, right int) bool {
		return references[left].kind < references[right].kind
	})
	return references, nil
}

// DeclaredReferences 报出本版本正文指名的全部对外引用。
func (version CommercialVersion) DeclaredReferences() []DeclaredReference {
	return append([]DeclaredReference(nil), version.references...)
}

// ReferenceTo 报出本版本指名的某一类对象。缺席与指名是两件事：缺席说明这份正文没有
// 对该类对象的约定，而不是「随便哪一个都行」。
func (version CommercialVersion) ReferenceTo(kind CommercialObjectKind) (CommercialObjectID, bool) {
	for _, reference := range version.references {
		if reference.kind == kind {
			return reference.objectID, true
		}
	}
	return CommercialObjectID{}, false
}

// SameVersionAs 判断两个值指的是不是同一个对象版本。生命周期位置刻意不参与，理由与
// sameReleasedContent 相同：一个后来生效或已退役的版本仍是同一次发布。
//
// 内容摘要参与，因为它正是同版本号被改了正文时唯一会变的那一项——登记册把那种情况判为
// 需要商业责任方修正的冲突，此处若放过它，一份冒名的同号版本就能冒充被采用的那一个。
func (version CommercialVersion) SameVersionAs(other CommercialVersion) bool {
	return version.tenant == other.tenant &&
		version.kind == other.kind &&
		version.objectID == other.objectID &&
		version.version == other.version &&
		version.contentDigest == other.contentDigest
}

// Revise 修订草稿内容。已发布版本会拒绝：它的正文已经固定，变化必须形成新的版本。
func (version CommercialVersion) Revise(digest CommercialContentDigest) (CommercialVersion, error) {
	if version.status != CommercialVersionDraft {
		return CommercialVersion{}, ErrCommercialContentIsFixed
	}
	if !digest.valid() {
		return CommercialVersion{}, ErrInvalidCommercialVersion
	}
	version.contentDigest = digest
	return version, nil
}

// Publish 固定正文。批准与来源必须完备，批准角色必须已确认，正文指名引用必须已发布，
// 且发布不得早于为它背书的那次批准。
//
// 角色未确认、字段不全、指名引用未发布三者分格（ADR-0035 / ADR-0036）：恢复动作不同，
// 草稿与导入来源在后两格与角色未确认时一律原样保留。
func (version CommercialVersion) Publish(
	basis ApprovalBasis,
	roleStanding ApprovalRoleStanding,
	publishedAt time.Time,
	referenceStanding NamedReferenceStandingLookup,
) (CommercialVersion, error) {
	if version.status != CommercialVersionDraft {
		return CommercialVersion{}, ErrCommercialContentIsFixed
	}
	if !basis.valid() || publishedAt.IsZero() || publishedAt.Before(basis.ApprovedAt()) {
		return CommercialVersion{}, ErrIncompleteCommercialPublication
	}
	if roleStanding != ApprovalRoleConfirmed {
		return CommercialVersion{}, ErrApprovalRoleNotConfirmed
	}
	if err := confirmNamedReferencesPublished(version.references, referenceStanding); err != nil {
		return CommercialVersion{}, err
	}
	version.status = CommercialVersionPublished
	version.approval = basis
	version.publishedAt = publishedAt.UTC()
	return version, nil
}

// confirmNamedReferencesPublished 拒绝悬空生产引用：正文指名了谁，发布前谁就必须已发布。
func confirmNamedReferencesPublished(
	references []DeclaredReference,
	standingOf NamedReferenceStandingLookup,
) error {
	if len(references) == 0 {
		return nil
	}
	for _, reference := range references {
		standing := NamedReferenceStandingInvalid
		if standingOf != nil {
			standing = standingOf(reference.kind, reference.objectID)
		}
		if standing != NamedReferencePublished {
			return ErrNamedReferenceNotPublished
		}
	}
	return nil
}

// TakeEffect 在版本自己的生效边界到达后使其投入使用。发布不等于生效：提前发布的版本
// 在其有效区间打开之前不得参与解析。
func (version CommercialVersion) TakeEffect(at time.Time) (CommercialVersion, error) {
	if version.status != CommercialVersionPublished || at.IsZero() || at.Before(version.effective.StartsAt()) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	version.status = CommercialVersionEffective
	version.effectiveAt = at.UTC()
	return version, nil
}

// Expire 在版本自己声明的边界处收尾。开放结束的版本没有这样的边界，让它到期等于凭空
// 造一个，因此只能退役或替代。
func (version CommercialVersion) Expire(at time.Time) (CommercialVersion, error) {
	endsAt, bounded := version.effective.EndsAt()
	if version.status != CommercialVersionEffective || !bounded || at.IsZero() || at.Before(endsAt) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	return version.close(CommercialVersionExpired, at), nil
}

// Retire 按明确决定而非按有效区间收尾，因此要记录该决定出自哪个引用。
func (version CommercialVersion) Retire(reference RetirementReference, at time.Time) (CommercialVersion, error) {
	if version.status != CommercialVersionEffective || !reference.valid() || at.IsZero() || at.Before(version.effectiveAt) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	closed := version.close(CommercialVersionRetired, at)
	closed.retirementRef = reference
	return closed, nil
}

// SupersededBy 以一个指名的后继替代本版本。后继必须是同一对象的另一版本：跨对象或跨
// 类型替代会毁掉 CONTEXT 要求商业版本保有的「与前后版本的关系」。
func (version CommercialVersion) SupersededBy(successor CommercialVersion, at time.Time) (CommercialVersion, error) {
	sameObject := successor.kind == version.kind && successor.objectID == version.objectID
	if version.status != CommercialVersionEffective ||
		!sameObject ||
		successor.version == version.version ||
		successor.status == CommercialVersionDraft ||
		at.IsZero() ||
		at.Before(version.effectiveAt) {
		return CommercialVersion{}, ErrInvalidCommercialTransition
	}
	closed := version.close(CommercialVersionSuperseded, at)
	closed.successor = successor.version
	return closed, nil
}

// close 停止参与新的解析，同时原样保留已固定的正文、批准依据与发布时间——既有委托、
// 交易与结算仍在引用它们。
func (version CommercialVersion) close(status CommercialVersionStatus, at time.Time) CommercialVersion {
	version.status = status
	version.closedAt = at.UTC()
	return version
}

// AppliesAt 回答本版本是否仍可被新的解析选中。只有生效中且落在有效区间内的版本可以；
// 已收尾的版本仍可作为历史读取，但不再适用。
func (version CommercialVersion) AppliesAt(at time.Time) bool {
	return version.status == CommercialVersionEffective && version.effective.Contains(at)
}

func (version CommercialVersion) Tenant() TenantID {
	return version.tenant
}

func (version CommercialVersion) Kind() CommercialObjectKind {
	return version.kind
}

func (version CommercialVersion) ObjectID() CommercialObjectID {
	return version.objectID
}

func (version CommercialVersion) Version() CommercialVersionLabel {
	return version.version
}

// QualifiedLabel 是「对象/版本」两段式的版本指称，本上下文内引用某一版正文时用它。
//
// 它单列一处，是因为**两边必须写出同一个串才对得上**：结算政策的适用范围里那一维记的是
// 「本约定属于哪一版客户合同」，而闭包解出合同之后要拿这个串去命中它（ADR-0080）。两处
// 各自拼一遍字符串，日后谁改了分隔符，命中就会静静失败——看起来像「这个范围没有结算
// 政策」，而其实是两串对不上。
//
// 版本身份不成立时交回零值：把一个残缺版本拼成 "/" 之类的串，等于凭空造出一个能被匹配的
// 指称。
func (version CommercialVersion) QualifiedLabel() CommercialVersionLabel {
	if !version.objectID.valid() || !version.version.valid() {
		return CommercialVersionLabel{}
	}
	label, err := NewCommercialVersionLabel(version.objectID.String() + "/" + version.version.String())
	if err != nil {
		return CommercialVersionLabel{}
	}
	return label
}

func (version CommercialVersion) Scope() CommercialScopeReference {
	return version.scope
}

func (version CommercialVersion) ContentDigest() CommercialContentDigest {
	return version.contentDigest
}

func (version CommercialVersion) Effective() EffectiveInterval {
	return version.effective
}

func (version CommercialVersion) Status() CommercialVersionStatus {
	return version.status
}

func (version CommercialVersion) ApprovalBasis() (ApprovalBasis, bool) {
	if !version.approval.valid() {
		return ApprovalBasis{}, false
	}
	return version.approval, true
}

func (version CommercialVersion) PublishedAt() (time.Time, bool) {
	if version.publishedAt.IsZero() {
		return time.Time{}, false
	}
	return version.publishedAt, true
}

func (version CommercialVersion) EffectiveAt() (time.Time, bool) {
	if version.effectiveAt.IsZero() {
		return time.Time{}, false
	}
	return version.effectiveAt, true
}

func (version CommercialVersion) ClosedAt() (time.Time, bool) {
	if version.closedAt.IsZero() {
		return time.Time{}, false
	}
	return version.closedAt, true
}

func (version CommercialVersion) RetirementReference() (RetirementReference, bool) {
	if !version.retirementRef.valid() {
		return RetirementReference{}, false
	}
	return version.retirementRef, true
}

func (version CommercialVersion) Successor() (CommercialVersionLabel, bool) {
	if !version.successor.valid() {
		return CommercialVersionLabel{}, false
	}
	return version.successor, true
}
