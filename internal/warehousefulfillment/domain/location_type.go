package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidLocationTypeContent 是一份库位类型内容缺了必填的格；具体缺哪一格由 LocationTypeContentFieldError 交回。
	ErrInvalidLocationTypeContent = errors.New("warehouse fulfillment: invalid location type content")
	// ErrInvalidLocationTypeRevision 是一笔库位类型修订缺了身份、登记责任方或登记时刻。
	ErrInvalidLocationTypeRevision = errors.New("warehouse fulfillment: invalid location type revision")
)

// LocationTypeField 指名库位类型修订上的一格，登记口答「内容不合」时用它指出差在哪里。库位类型没有登记后不改的格，也没有指名
// 别的对象的格，所以这里只有内容里的名称一格。
type LocationTypeField uint8

const (
	LocationTypeFieldInvalid LocationTypeField = iota
	LocationTypeFieldName
)

func (field LocationTypeField) String() string {
	switch field {
	case LocationTypeFieldName:
		return "name"
	default:
		return ""
	}
}

// LocationTypeContentFieldError 指出一份库位类型内容里哪一格不成立。
type LocationTypeContentFieldError struct {
	Field LocationTypeField
}

func (err *LocationTypeContentFieldError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidLocationTypeContent, err.Field)
}

func (err *LocationTypeContentFieldError) Is(target error) bool {
	return target == ErrInvalidLocationTypeContent
}

// LocationTypeCode 是库位类型的编码，与租户合成库位类型的身份，登记后不改。逐字保存：库位类型表按租户登记、各仓共用，产品不内置
// 任何库位类型（CONTEXT「主数据登记」），所以编码的写法完全是租户的。
type LocationTypeCode struct{ requiredValue }

func NewLocationTypeCode(value string) (LocationTypeCode, error) {
	required, err := newRequiredValue("location type code", value)
	return LocationTypeCode{required}, err
}

type LocationTypeName struct{ requiredValue }

func NewLocationTypeName(value string) (LocationTypeName, error) {
	required, err := newRequiredValue("location type name", value)
	return LocationTypeName{required}, err
}

// LocationTypeContentSpec 是一份库位类型内容的各格。是否停用缺省就是不停用。
type LocationTypeContentSpec struct {
	Name        LocationTypeName
	Deactivated bool
}

// LocationTypeContent 是一笔库位类型修订里随修订号变的那份完整内容。它的每一格都可比较，内容相同的判定直接用 ==：往里加一格
// 不属于「内容」的东西（登记责任方、登记时刻之类）会让重放被误判成冲突，所以那些格放在修订上，不放在这里。
type LocationTypeContent struct {
	name        LocationTypeName
	deactivated bool
}

// NewLocationTypeContent 要求名称给齐；缺就指出缺的那一格。
func NewLocationTypeContent(spec LocationTypeContentSpec) (LocationTypeContent, error) {
	content := LocationTypeContent{name: spec.Name, deactivated: spec.Deactivated}
	if field := content.firstMissingField(); field != LocationTypeFieldInvalid {
		return LocationTypeContent{}, &LocationTypeContentFieldError{Field: field}
	}
	return content, nil
}

func (content LocationTypeContent) firstMissingField() LocationTypeField {
	if !content.name.valid() {
		return LocationTypeFieldName
	}
	return LocationTypeFieldInvalid
}

func (content LocationTypeContent) Name() LocationTypeName {
	return content.name
}

func (content LocationTypeContent) Deactivated() bool {
	return content.deactivated
}

// LocationTypeRevision 是库位类型册上的一笔修订：身份（租户，库位类型编码）、修订号、完整内容，另记登记责任方与登记时刻。
// 修订从 1 起连续，只追加；停用与再启用都是新修订（CONTEXT「主数据登记」）。
type LocationTypeRevision struct {
	tenant       TenantID
	code         LocationTypeCode
	revision     int
	content      LocationTypeContent
	registrant   Registrant
	registeredAt time.Time
}

// NewLocationTypeRevision 判一笔修订的各部分是不是齐的。登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，登记口回显的
// 时刻与读回的差在纳秒上。
func NewLocationTypeRevision(
	tenant TenantID,
	code LocationTypeCode,
	revision int,
	content LocationTypeContent,
	registrant Registrant,
	registeredAt time.Time,
) (LocationTypeRevision, error) {
	if revision < 1 {
		return LocationTypeRevision{}, fmt.Errorf("%w: %d", ErrInvalidRevisionNumber, revision)
	}
	if field := content.firstMissingField(); field != LocationTypeFieldInvalid {
		return LocationTypeRevision{}, &LocationTypeContentFieldError{Field: field}
	}
	if !tenant.valid() || !code.valid() || !registrant.valid() || registeredAt.IsZero() {
		return LocationTypeRevision{}, ErrInvalidLocationTypeRevision
	}
	return LocationTypeRevision{
		tenant:       tenant,
		code:         code,
		revision:     revision,
		content:      content,
		registrant:   registrant,
		registeredAt: registeredAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func (revision LocationTypeRevision) Tenant() TenantID {
	return revision.tenant
}

func (revision LocationTypeRevision) Code() LocationTypeCode {
	return revision.code
}

func (revision LocationTypeRevision) Revision() int {
	return revision.revision
}

func (revision LocationTypeRevision) Content() LocationTypeContent {
	return revision.content
}

func (revision LocationTypeRevision) Registrant() Registrant {
	return revision.registrant
}

func (revision LocationTypeRevision) RegisteredAt() time.Time {
	return revision.registeredAt
}

// SameContentAs 回答两笔修订是不是同一份内容：比租户、库位类型编码、修订号与完整内容，不比登记责任方与登记时刻。
// 重放（原样重投，可能换了操作者）与冲突（同一修订号换了内容）都据此判（UC-WF-001「一致性、幂等与并发」）。
func (revision LocationTypeRevision) SameContentAs(other LocationTypeRevision) bool {
	return revision.tenant == other.tenant &&
		revision.code == other.code &&
		revision.revision == other.revision &&
		revision.content == other.content
}
