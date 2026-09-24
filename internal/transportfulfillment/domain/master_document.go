package domain

import (
	"errors"
	"fmt"
	"sort"
	"time"
)

var (
	ErrInvalidMasterDocument = errors.New("transport fulfillment: invalid master document")
	// ErrMasterDocumentNoLongerInForce 与形状错误分格：对一份已撤销或已替代的总单再形成新版本，
	// 恢复动作不是改输入，而是去看它最后一版是谁、什么时候收掉的（CONTEXT：已撤销或已替代的总单
	// 不再接受新版本）。
	ErrMasterDocumentNoLongerInForce = errors.New("transport fulfillment: master document is no longer in force")
)

// MasterDocumentReference 是总单的身份：由登记方声明，本上下文不铸、不解析（ADR-0113 决定二）。它通常
// 就是签发方给的总单号，但号码格式属实例半边，这里只当一个非空引用。
type MasterDocumentReference struct{ requiredValue }

func NewMasterDocumentReference(value string) (MasterDocumentReference, error) {
	required, err := newRequiredValue("master document reference", value)
	return MasterDocumentReference{required}, err
}

// MasterDocumentVersion 是总单的版本。撤销、替代、关联重述都形成新版本回指前版，原版本原样保留——
// CONTEXT 规则节「每个版本回指前版，原版本一字不动」在类型上的落点。
type MasterDocumentVersion struct{ requiredValue }

func NewMasterDocumentVersion(value string) (MasterDocumentVersion, error) {
	required, err := newRequiredValue("master document version", value)
	return MasterDocumentVersion{required}, err
}

// MasterDocumentIssuerReference 是总单的签发方——外部运输服务提供方。它引用 party-commercial 已登记
// 的参与方身份，本上下文不铸；签发方不推导实际承运商、签约服务商或任何其他角色（CONTEXT 规则节）。
type MasterDocumentIssuerReference struct{ requiredValue }

func NewMasterDocumentIssuerReference(value string) (MasterDocumentIssuerReference, error) {
	required, err := newRequiredValue("master document issuer reference", value)
	return MasterDocumentIssuerReference{required}, err
}

// TransportScopeReference 是总单表达的主运输凭证范围：登记方声明的范围引用。它不从 network-routing
// 的线路或计划履约段推导（ADR-0004），也不在这里结构化成起讫地——那是实例半边的事。
type TransportScopeReference struct{ requiredValue }

func NewTransportScopeReference(value string) (TransportScopeReference, error) {
	required, err := newRequiredValue("transport scope reference", value)
	return TransportScopeReference{required}, err
}

// AssociatedObjectKind 是总单关联的对象属哪一类。GLOSSARY「总单」词条列的三类，封闭：集运单元、包裹、
// 运输履约范围（在本上下文里就是实际履约段）。类别是登记出来的，不从引用字符串的格式推断。
type AssociatedObjectKind uint8

const (
	AssociatedObjectKindInvalid AssociatedObjectKind = iota
	AssociatesConsolidationUnit
	AssociatesParcel
	AssociatesFulfillmentSegment
)

func (kind AssociatedObjectKind) String() string {
	switch kind {
	case AssociatesConsolidationUnit:
		return "CONSOLIDATION_UNIT"
	case AssociatesParcel:
		return "PARCEL"
	case AssociatesFulfillmentSegment:
		return "FULFILLMENT_SEGMENT"
	default:
		return ""
	}
}

func (kind AssociatedObjectKind) valid() bool {
	return kind >= AssociatesConsolidationUnit && kind <= AssociatesFulfillmentSegment
}

// ParseAssociatedObjectKind 把库面或登记输入里的类别词认回封闭集合；词不在集合内即拒，不猜。
func ParseAssociatedObjectKind(raw string) (AssociatedObjectKind, error) {
	for kind := AssociatesConsolidationUnit; kind <= AssociatesFulfillmentSegment; kind++ {
		if kind.String() == raw {
			return kind, nil
		}
	}
	return AssociatedObjectKindInvalid, fmt.Errorf("%w: unknown associated object kind %q", ErrInvalidMasterDocument, raw)
}

// MasterDocumentAssociation 是总单与一个对象的关联：类别加引用。关联只说「这一版总单列入了它」，
// 不证明装载、交接或运输（CONTEXT 硬句）；被关联对象的身份分属 node-operations / parcel-shipment /
// 本上下文，这里只引用。
type MasterDocumentAssociation struct {
	kind      AssociatedObjectKind
	reference requiredValue
}

func NewMasterDocumentAssociation(kind AssociatedObjectKind, reference string) (MasterDocumentAssociation, error) {
	if !kind.valid() {
		return MasterDocumentAssociation{}, fmt.Errorf("%w: associated object kind", ErrInvalidMasterDocument)
	}
	required, err := newRequiredValue("associated object reference", reference)
	if err != nil {
		return MasterDocumentAssociation{}, fmt.Errorf("%w: %v", ErrInvalidMasterDocument, err)
	}
	return MasterDocumentAssociation{kind: kind, reference: required}, nil
}

func (association MasterDocumentAssociation) Kind() AssociatedObjectKind { return association.kind }
func (association MasterDocumentAssociation) Reference() string {
	return association.reference.String()
}

func (association MasterDocumentAssociation) valid() bool {
	return association.kind.valid() && association.reference.valid()
}

// canonicalAssociations 把关联集整理成可比较的形状：按（类别，引用）排序、拒绝重复。关联是集合不是
// 列表，登记方给的顺序不是内容；同一关联给两遍是输入错误而不是两条关联。
func canonicalAssociations(associations []MasterDocumentAssociation) ([]MasterDocumentAssociation, error) {
	sorted := make([]MasterDocumentAssociation, 0, len(associations))
	for _, association := range associations {
		if !association.valid() {
			return nil, fmt.Errorf("%w: association", ErrInvalidMasterDocument)
		}
		sorted = append(sorted, association)
	}
	sort.Slice(sorted, func(left, right int) bool {
		if sorted[left].kind != sorted[right].kind {
			return sorted[left].kind < sorted[right].kind
		}
		return sorted[left].reference.value < sorted[right].reference.value
	})
	for index := 1; index < len(sorted); index++ {
		if sorted[index] == sorted[index-1] {
			return nil, fmt.Errorf("%w: duplicate association %s %q", ErrInvalidMasterDocument, sorted[index].kind, sorted[index].Reference())
		}
	}
	return sorted, nil
}

func sameAssociations(left, right []MasterDocumentAssociation) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

// MasterDocumentStanding 是总单此刻的适用状态。撤销与替代两格分开：续办不同——撤销要问签发方为什么
// 收回，替代要去看替代它的那一份。关联重述不改变状态，所以这里没有它的格。
type MasterDocumentStanding uint8

const (
	MasterDocumentStandingInvalid MasterDocumentStanding = iota
	MasterDocumentInForce
	MasterDocumentRevoked
	MasterDocumentSuperseded
)

func (standing MasterDocumentStanding) String() string {
	switch standing {
	case MasterDocumentInForce:
		return "IN_FORCE"
	case MasterDocumentRevoked:
		return "REVOKED"
	case MasterDocumentSuperseded:
		return "SUPERSEDED"
	default:
		return ""
	}
}

func (standing MasterDocumentStanding) valid() bool {
	return standing >= MasterDocumentInForce && standing <= MasterDocumentSuperseded
}

// ParseMasterDocumentStanding 把库面里的状态词认回封闭集合。
func ParseMasterDocumentStanding(raw string) (MasterDocumentStanding, error) {
	for standing := MasterDocumentInForce; standing <= MasterDocumentSuperseded; standing++ {
		if standing.String() == raw {
			return standing, nil
		}
	}
	return MasterDocumentStandingInvalid, fmt.Errorf("%w: unknown master document standing %q", ErrInvalidMasterDocument, raw)
}

// MasterDocumentRevision 是一份总单形成新版本的三种方式，封闭（CONTEXT 规则节与 ADR-0113 决定三）。
// 它是登记输入里的词，不是状态：撤销与替代落成状态，关联重述只换关联集。它不落库；ParseMasterDocumentRevision
// 只供操作者渠道的登记口把请求体里的词认回封闭集合。
type MasterDocumentRevision uint8

const (
	MasterDocumentRevisionInvalid MasterDocumentRevision = iota
	MasterDocumentRevocation
	MasterDocumentSupersession
	MasterDocumentAssociationRestatement
)

func (revision MasterDocumentRevision) String() string {
	switch revision {
	case MasterDocumentRevocation:
		return "REVOKE"
	case MasterDocumentSupersession:
		return "SUPERSEDE"
	case MasterDocumentAssociationRestatement:
		return "RESTATE_ASSOCIATIONS"
	default:
		return ""
	}
}

// ParseMasterDocumentRevision 把登记输入里的新版本方式词认回封闭集合。
func ParseMasterDocumentRevision(raw string) (MasterDocumentRevision, error) {
	for revision := MasterDocumentRevocation; revision <= MasterDocumentAssociationRestatement; revision++ {
		if revision.String() == raw {
			return revision, nil
		}
	}
	return MasterDocumentRevisionInvalid, fmt.Errorf("%w: unknown master document revision %q", ErrInvalidMasterDocument, raw)
}

// MasterDocumentSpec 是登记一份总单首版所需的全部输入：CONTEXT 词条点名的签发方、主运输凭证范围、
// 关联对象集，加可缺的运输委托 / 订舱引用、总单身份、版本与租户。
type MasterDocumentSpec struct {
	TenantID TenantID
	Document MasterDocumentReference
	Version  MasterDocumentVersion
	Issuer   MasterDocumentIssuerReference
	Scope    TransportScopeReference
	// Commission 与 Booking 零值即缺席：CONTEXT「总单可以引用运输委托或订舱关系」是「可以」。
	Commission   TransportCommissionReference
	Booking      BookingReference
	Associations []MasterDocumentAssociation
}

// MasterDocument 是运营企业与外部运输服务提供方之间针对明确运输范围形成的主运输凭证的一个版本
// （CONTEXT「总单」）。值语义：任何改变都交回新版本，原值不动。
type MasterDocument struct {
	tenantID     TenantID
	document     MasterDocumentReference
	version      MasterDocumentVersion
	issuer       MasterDocumentIssuerReference
	scope        TransportScopeReference
	commission   TransportCommissionReference
	booking      BookingReference
	associations []MasterDocumentAssociation
	standing     MasterDocumentStanding
	// changedAt 是本版本改变前版的业务时间，首版没有。它与登记落库的时刻是两个时间。
	changedAt  time.Time
	supersedes MasterDocumentVersion
	replacedBy MasterDocumentReference
}

// RegisterMasterDocument 是登记的构造门：首版必定有效、不回指任何前版。首版允许零关联——总单先签、
// 集运单元后列是常态，关联随后以关联重述补入。
func RegisterMasterDocument(spec MasterDocumentSpec) (MasterDocument, error) {
	if !spec.TenantID.valid() || !spec.Document.valid() || !spec.Version.valid() ||
		!spec.Issuer.valid() || !spec.Scope.valid() {
		return MasterDocument{}, ErrInvalidMasterDocument
	}
	associations, err := canonicalAssociations(spec.Associations)
	if err != nil {
		return MasterDocument{}, err
	}
	return MasterDocument{
		tenantID:     spec.TenantID,
		document:     spec.Document,
		version:      spec.Version,
		issuer:       spec.Issuer,
		scope:        spec.Scope,
		commission:   spec.Commission,
		booking:      spec.Booking,
		associations: associations,
		standing:     MasterDocumentInForce,
	}, nil
}

func (document MasterDocument) TenantID() TenantID                    { return document.tenantID }
func (document MasterDocument) Document() MasterDocumentReference     { return document.document }
func (document MasterDocument) Version() MasterDocumentVersion        { return document.version }
func (document MasterDocument) Issuer() MasterDocumentIssuerReference { return document.issuer }
func (document MasterDocument) Scope() TransportScopeReference        { return document.scope }
func (document MasterDocument) Standing() MasterDocumentStanding      { return document.standing }
func (document MasterDocument) InForce() bool                         { return document.standing == MasterDocumentInForce }

// Commission 只在总单引用了运输委托时交回它。
func (document MasterDocument) Commission() (TransportCommissionReference, bool) {
	return document.commission, document.commission.valid()
}

// Booking 只在总单引用了订舱时交回它。
func (document MasterDocument) Booking() (BookingReference, bool) {
	return document.booking, document.booking.valid()
}

// Associations 交回这一版关联的对象集（按类别、引用排序的副本）。
func (document MasterDocument) Associations() []MasterDocumentAssociation {
	copied := make([]MasterDocumentAssociation, len(document.associations))
	copy(copied, document.associations)
	return copied
}

// ChangedAt 是本版本改变前版的业务时间；首版第二个返回值为 false。
func (document MasterDocument) ChangedAt() (time.Time, bool) {
	return document.changedAt, !document.changedAt.IsZero()
}

// Supersedes 交回本版本回指的前版；首版第二个返回值为 false。
func (document MasterDocument) Supersedes() (MasterDocumentVersion, bool) {
	return document.supersedes, document.supersedes.valid()
}

// ReplacedBy 只在已替代时交回替代它的总单。
func (document MasterDocument) ReplacedBy() (MasterDocumentReference, bool) {
	return document.replacedBy, document.replacedBy.valid()
}

// Revoke 撤销：签发方收回了总单。状态落为已撤销，关联集原样带过去。
func (document MasterDocument) Revoke(at time.Time, version MasterDocumentVersion) (MasterDocument, error) {
	return document.nextVersion(MasterDocumentRevoked, at, version, MasterDocumentReference{}, document.associations)
}

// Supersede 替代：另一份总单接替了它。替代者必备且不能是自己（CONTEXT「替代凭证必须建立显式替代关系」）。
func (document MasterDocument) Supersede(
	at time.Time,
	replacement MasterDocumentReference,
	version MasterDocumentVersion,
) (MasterDocument, error) {
	if !replacement.valid() || replacement == document.document {
		return MasterDocument{}, ErrInvalidMasterDocument
	}
	return document.nextVersion(MasterDocumentSuperseded, at, version, replacement, document.associations)
}

// RestateAssociations 关联重述：总单仍有效，关联的对象集变了。状态不动，身份不动——不为同一份真实凭证
// 另立第二个总单身份（ADR-0113 决定三）。关联集与当前版相同就没有东西可重述，拒。
func (document MasterDocument) RestateAssociations(
	at time.Time,
	associations []MasterDocumentAssociation,
	version MasterDocumentVersion,
) (MasterDocument, error) {
	canonical, err := canonicalAssociations(associations)
	if err != nil {
		return MasterDocument{}, err
	}
	if sameAssociations(canonical, document.associations) {
		return MasterDocument{}, fmt.Errorf("%w: associations are unchanged", ErrInvalidMasterDocument)
	}
	return document.nextVersion(MasterDocumentInForce, at, version, MasterDocumentReference{}, canonical)
}

// nextVersion 是三种改变共用的一扇门：只对有效的版本开放，新版本回指本版、带改变的业务时间，签发方、
// 范围与两个可缺引用原样带过去。沿用原版本号就是覆盖，拒。
func (document MasterDocument) nextVersion(
	standing MasterDocumentStanding,
	at time.Time,
	version MasterDocumentVersion,
	replacement MasterDocumentReference,
	associations []MasterDocumentAssociation,
) (MasterDocument, error) {
	if !document.InForce() {
		return MasterDocument{}, ErrMasterDocumentNoLongerInForce
	}
	if !version.valid() || version == document.version || at.IsZero() {
		return MasterDocument{}, ErrInvalidMasterDocument
	}
	changed := document
	changed.version = version
	changed.associations = associations
	changed.standing = standing
	changed.changedAt = at.UTC()
	changed.supersedes = document.version
	changed.replacedBy = replacement
	return changed, nil
}

// Equal 按业务内容比较两个版本——同键异内容要答`内容冲突`而不是`已登记`（ADR-0031），比的是这里的
// 字段而不是结构体相等：time.Time 的 == 会被单调时钟读数搅掉，切片也比不了。
func (document MasterDocument) Equal(other MasterDocument) bool {
	return document.tenantID == other.tenantID &&
		document.document == other.document &&
		document.version == other.version &&
		document.issuer == other.issuer &&
		document.scope == other.scope &&
		document.commission == other.commission &&
		document.booking == other.booking &&
		sameAssociations(document.associations, other.associations) &&
		document.standing == other.standing &&
		document.changedAt.Equal(other.changedAt) &&
		document.supersedes == other.supersedes &&
		document.replacedBy == other.replacedBy
}
