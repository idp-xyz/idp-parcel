package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidWarehouseContent 是一份内容缺了必填的格；具体缺哪一格由 ContentFieldError 交回。
	ErrInvalidWarehouseContent = errors.New("warehouse fulfillment: invalid warehouse content")
	// ErrInvalidRevisionNumber 是修订号不是从 1 起的正整数。
	ErrInvalidRevisionNumber = errors.New("warehouse fulfillment: invalid revision number")
	// ErrInvalidWarehouseRevision 是一笔修订缺了身份、登记责任方或登记时刻。
	ErrInvalidWarehouseRevision = errors.New("warehouse fulfillment: invalid warehouse revision")
)

// WarehouseField 指名仓修订上的一格，登记口答「不可改的格」与「内容不合」时用它指出差在哪里。
type WarehouseField uint8

const (
	WarehouseFieldInvalid WarehouseField = iota
	WarehouseFieldName
	WarehouseFieldKind
	WarehouseFieldAddressCountry
	WarehouseFieldAddressText
	WarehouseFieldBusinessTimeZone
)

func (field WarehouseField) String() string {
	switch field {
	case WarehouseFieldName:
		return "name"
	case WarehouseFieldKind:
		return "kind"
	case WarehouseFieldAddressCountry:
		return "address.country"
	case WarehouseFieldAddressText:
		return "address.text"
	case WarehouseFieldBusinessTimeZone:
		return "businessTimeZone"
	default:
		return ""
	}
}

// ContentFieldError 指出一份内容里哪一格不成立。
type ContentFieldError struct {
	Field WarehouseField
}

func (err *ContentFieldError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidWarehouseContent, err.Field)
}

func (err *ContentFieldError) Is(target error) bool {
	return target == ErrInvalidWarehouseContent
}

// WarehouseKind 是仓的种类，产品内置的封闭两格（CONTEXT「自营仓」「代理仓」）。登记后不改。
type WarehouseKind uint8

const (
	WarehouseKindInvalid WarehouseKind = iota
	WarehouseKindSelfOperated
	WarehouseKindAgent
)

func (kind WarehouseKind) String() string {
	switch kind {
	case WarehouseKindSelfOperated:
		return "SELF_OPERATED"
	case WarehouseKindAgent:
		return "AGENT"
	default:
		return ""
	}
}

// WarehouseKindNamed 把字面量反查回封闭集；空串与集外的词都答不认识，由接入侧按坏载荷答，不是未受理。
func WarehouseKindNamed(name string) (WarehouseKind, bool) {
	for _, kind := range []WarehouseKind{WarehouseKindSelfOperated, WarehouseKindAgent} {
		if kind.String() == name {
			return kind, true
		}
	}
	return WarehouseKindInvalid, false
}

// WarehouseAddress 是仓的经营场所地址：国家码与地址正文。
type WarehouseAddress struct {
	country CountryCode
	text    AddressText
}

func NewWarehouseAddress(country CountryCode, text AddressText) WarehouseAddress {
	return WarehouseAddress{country: country, text: text}
}

func (address WarehouseAddress) Country() CountryCode {
	return address.country
}

func (address WarehouseAddress) Text() AddressText {
	return address.text
}

// WarehouseContentSpec 是一份仓内容的各格。业务时区与同址物流节点可缺，缺就是零值。
type WarehouseContentSpec struct {
	Name             WarehouseName
	Kind             WarehouseKind
	Address          WarehouseAddress
	BusinessTimeZone BusinessTimeZone
	ColocatedNode    ColocatedNode
	Deactivated      bool
}

// WarehouseContent 是一笔仓修订里随修订号变的那份完整内容。它的每一格都可比较，内容相同的判定直接用 ==：
// 往里加一格不属于「内容」的东西（登记责任方、登记时刻之类）会让重放被误判成冲突，所以那些格放在修订上，不放在这里。
type WarehouseContent struct {
	name             WarehouseName
	kind             WarehouseKind
	address          WarehouseAddress
	businessTimeZone BusinessTimeZone
	colocatedNode    ColocatedNode
	deactivated      bool
}

// NewWarehouseContent 要求名称、种类、国家与地址正文都给齐；缺哪一格就指出哪一格。
func NewWarehouseContent(spec WarehouseContentSpec) (WarehouseContent, error) {
	content := WarehouseContent{
		name:             spec.Name,
		kind:             spec.Kind,
		address:          spec.Address,
		businessTimeZone: spec.BusinessTimeZone,
		colocatedNode:    spec.ColocatedNode,
		deactivated:      spec.Deactivated,
	}
	if field := content.firstMissingField(); field != WarehouseFieldInvalid {
		return WarehouseContent{}, &ContentFieldError{Field: field}
	}
	return content, nil
}

func (content WarehouseContent) firstMissingField() WarehouseField {
	switch {
	case !content.name.valid():
		return WarehouseFieldName
	case content.kind.String() == "":
		return WarehouseFieldKind
	case !content.address.country.valid():
		return WarehouseFieldAddressCountry
	case !content.address.text.valid():
		return WarehouseFieldAddressText
	default:
		return WarehouseFieldInvalid
	}
}

func (content WarehouseContent) Name() WarehouseName {
	return content.name
}

func (content WarehouseContent) Kind() WarehouseKind {
	return content.kind
}

func (content WarehouseContent) Address() WarehouseAddress {
	return content.address
}

func (content WarehouseContent) BusinessTimeZone() BusinessTimeZone {
	return content.businessTimeZone
}

func (content WarehouseContent) ColocatedNode() ColocatedNode {
	return content.colocatedNode
}

func (content WarehouseContent) Deactivated() bool {
	return content.deactivated
}

// WarehouseRevision 是仓册上的一笔修订：身份（租户，仓编码）、修订号、完整内容，另记登记责任方与登记时刻。
// 修订从 1 起连续，只追加；停用与再启用都是新修订（CONTEXT「主数据登记」）。
type WarehouseRevision struct {
	tenant       TenantID
	code         WarehouseCode
	revision     int
	content      WarehouseContent
	registrant   Registrant
	registeredAt time.Time
}

// NewWarehouseRevision 判一笔修订的各部分是不是齐的。登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，
// 登记口回显的时刻与读回的差在纳秒上。
func NewWarehouseRevision(
	tenant TenantID,
	code WarehouseCode,
	revision int,
	content WarehouseContent,
	registrant Registrant,
	registeredAt time.Time,
) (WarehouseRevision, error) {
	if revision < 1 {
		return WarehouseRevision{}, fmt.Errorf("%w: %d", ErrInvalidRevisionNumber, revision)
	}
	if field := content.firstMissingField(); field != WarehouseFieldInvalid {
		return WarehouseRevision{}, &ContentFieldError{Field: field}
	}
	if !tenant.valid() || !code.valid() || !registrant.valid() || registeredAt.IsZero() {
		return WarehouseRevision{}, ErrInvalidWarehouseRevision
	}
	return WarehouseRevision{
		tenant:       tenant,
		code:         code,
		revision:     revision,
		content:      content,
		registrant:   registrant,
		registeredAt: registeredAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func (revision WarehouseRevision) Tenant() TenantID {
	return revision.tenant
}

func (revision WarehouseRevision) Code() WarehouseCode {
	return revision.code
}

func (revision WarehouseRevision) Revision() int {
	return revision.revision
}

func (revision WarehouseRevision) Content() WarehouseContent {
	return revision.content
}

func (revision WarehouseRevision) Registrant() Registrant {
	return revision.registrant
}

func (revision WarehouseRevision) RegisteredAt() time.Time {
	return revision.registeredAt
}

// SameContentAs 回答两笔修订是不是同一份内容：比租户、仓编码、修订号与完整内容，不比登记责任方与登记时刻。
// 重放（原样重投，可能换了操作者）与冲突（同一修订号换了内容）都据此判（UC-WF-001「一致性、幂等与并发」）。
func (revision WarehouseRevision) SameContentAs(other WarehouseRevision) bool {
	return revision.tenant == other.tenant &&
		revision.code == other.code &&
		revision.revision == other.revision &&
		revision.content == other.content
}
