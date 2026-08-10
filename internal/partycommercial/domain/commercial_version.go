// Package domain 承载参与方与商业的领域模型：参与方身份、商业关系，以及供其他上下文
// 解析的版本化商业定义。它不拥有可执行价卡、客户委托或结算事实。
package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

var (
	ErrBlankValue                      = errors.New("party commercial: blank value")
	ErrInvalidCommercialVersion        = errors.New("party commercial: invalid commercial version")
	ErrInvalidEffectiveInterval        = errors.New("party commercial: invalid effective interval")
	ErrIncompleteCommercialPublication = errors.New("party commercial: incomplete commercial publication")
	ErrCommercialContentIsFixed        = errors.New("party commercial: published commercial content is fixed")
	ErrInvalidCommercialTransition     = errors.New("party commercial: invalid commercial version transition")
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

type CommercialVersionSpec struct {
	Kind          CommercialObjectKind
	ObjectID      CommercialObjectID
	Version       CommercialVersionLabel
	Scope         CommercialScopeReference
	ContentDigest CommercialContentDigest
	Effective     EffectiveInterval
}

// CommercialVersion 是一次受控发布形成的商业定义。它是值类型：每次转换返回新值、不改
// 接收者——这让「发布后正文不可覆盖」成为结构性事实，而不是一条需要有人记住的规则。
type CommercialVersion struct {
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
}

func NewCommercialDraft(spec CommercialVersionSpec) (CommercialVersion, error) {
	if !spec.Kind.valid() ||
		!spec.ObjectID.valid() ||
		!spec.Version.valid() ||
		!spec.Scope.valid() ||
		!spec.ContentDigest.valid() ||
		!spec.Effective.valid() {
		return CommercialVersion{}, ErrInvalidCommercialVersion
	}
	return CommercialVersion{
		kind:          spec.Kind,
		objectID:      spec.ObjectID,
		version:       spec.Version,
		scope:         spec.Scope,
		contentDigest: spec.ContentDigest,
		effective:     spec.Effective,
		status:        CommercialVersionDraft,
	}, nil
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

// Publish 固定正文。批准与来源必须完备，且发布不得早于为它背书的那次批准。
func (version CommercialVersion) Publish(basis ApprovalBasis, publishedAt time.Time) (CommercialVersion, error) {
	if version.status != CommercialVersionDraft {
		return CommercialVersion{}, ErrCommercialContentIsFixed
	}
	if !basis.valid() || publishedAt.IsZero() || publishedAt.Before(basis.ApprovedAt()) {
		return CommercialVersion{}, ErrIncompleteCommercialPublication
	}
	version.status = CommercialVersionPublished
	version.approval = basis
	version.publishedAt = publishedAt.UTC()
	return version, nil
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

func (version CommercialVersion) Kind() CommercialObjectKind {
	return version.kind
}

func (version CommercialVersion) ObjectID() CommercialObjectID {
	return version.objectID
}

func (version CommercialVersion) Version() CommercialVersionLabel {
	return version.version
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
