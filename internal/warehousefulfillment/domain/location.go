package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidLocationContent 是一份库位内容缺了必填的格；具体缺哪一格由 LocationContentFieldError 交回。
	ErrInvalidLocationContent = errors.New("warehouse fulfillment: invalid location content")
	// ErrInvalidLocationRevision 是一笔库位修订缺了身份、登记责任方或登记时刻。
	ErrInvalidLocationRevision = errors.New("warehouse fulfillment: invalid location revision")
)

// LocationField 指名库位修订上的一格，登记口答「不可改的格」「查无所指」与「所指已停用」时用它指出差在哪里。仓编码这一格不在库位的
// 内容里，它是库位身份的一段、指名的是仓：库位登记口答「指名的仓查不到」时指出它。库位类型的编码是内容里可缺的一格。
type LocationField uint8

const (
	LocationFieldInvalid LocationField = iota
	LocationFieldWarehouseCode
	LocationFieldZoneCode
	LocationFieldLocationTypeCode
)

func (field LocationField) String() string {
	switch field {
	case LocationFieldWarehouseCode:
		return "warehouseCode"
	case LocationFieldZoneCode:
		return "zoneCode"
	case LocationFieldLocationTypeCode:
		return "locationTypeCode"
	default:
		return ""
	}
}

// LocationContentFieldError 指出一份库位内容里哪一格不成立。
type LocationContentFieldError struct {
	Field LocationField
}

func (err *LocationContentFieldError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidLocationContent, err.Field)
}

func (err *LocationContentFieldError) Is(target error) bool {
	return target == ErrInvalidLocationContent
}

// LocationCode 是库位的编码，与租户、仓编码合成库位的身份，登记后不改。逐字保存：库位编码在仓内才唯一，同一个编码在不同的仓下
// 是不同的库位（CONTEXT「主数据登记」）。
type LocationCode struct{ requiredValue }

func NewLocationCode(value string) (LocationCode, error) {
	required, err := newRequiredValue("location code", value)
	return LocationCode{required}, err
}

// LocationPriority 是库位的优先级，用于分配，数字越大越优先（CONTEXT「库位」）。可缺，所以零值就是「没给」：不给就是没有，
// 不取任何默认值，也不当成 0——给了 0 是另一份内容。取值是 32 位有符号整数，可以为负；范围由类型本身守住，超出范围的报文
// 在接入侧就是坏载荷。
type LocationPriority struct {
	value int32
	given bool
}

func NewLocationPriority(value int32) LocationPriority {
	return LocationPriority{value: value, given: true}
}

// Given 回答这一格有没有给。
func (priority LocationPriority) Given() bool {
	return priority.given
}

// Value 是给了的取值；没给时是 0，调用方须先看 Given，不可把它当成「优先级为 0」。
func (priority LocationPriority) Value() int32 {
	return priority.value
}

// LocationContentSpec 是一份库位内容的各格。库位类型与优先级可缺，缺省就是没有；是否停用缺省就是不停用。
type LocationContentSpec struct {
	ZoneCode     ZoneCode
	LocationType LocationTypeCode
	Priority     LocationPriority
	Deactivated  bool
}

// LocationContent 是一笔库位修订里随修订号变的那份完整内容。它的每一格都可比较，内容相同的判定直接用 ==：往里加一格不属于
// 「内容」的东西（登记责任方、登记时刻之类）会让重放被误判成冲突，所以那些格放在修订上，不放在这里。库位类型与优先级「没有」
// 与「有」是不同的内容，上一笔带类型、这一笔不带，是一次内容变化。
type LocationContent struct {
	zone         ZoneCode
	locationType LocationTypeCode
	priority     LocationPriority
	deactivated  bool
}

// NewLocationContent 要求所属库区给齐；缺就指出缺的那一格。
func NewLocationContent(spec LocationContentSpec) (LocationContent, error) {
	content := LocationContent{
		zone: spec.ZoneCode, locationType: spec.LocationType, priority: spec.Priority, deactivated: spec.Deactivated,
	}
	if field := content.firstMissingField(); field != LocationFieldInvalid {
		return LocationContent{}, &LocationContentFieldError{Field: field}
	}
	return content, nil
}

func (content LocationContent) firstMissingField() LocationField {
	if !content.zone.valid() {
		return LocationFieldZoneCode
	}
	return LocationFieldInvalid
}

func (content LocationContent) ZoneCode() ZoneCode {
	return content.zone
}

// LocationType 交回库位类型的编码；这份内容没带库位类型时第二个返回值为假。
func (content LocationContent) LocationType() (LocationTypeCode, bool) {
	return content.locationType, content.locationType.valid()
}

func (content LocationContent) Priority() LocationPriority {
	return content.priority
}

func (content LocationContent) Deactivated() bool {
	return content.deactivated
}

// NamesLocationTypeNewly 回答这份内容是不是「新指名」了一个库位类型：带了库位类型，并且这是首笔修订，或在册最新那一笔没带库位类型，
// 或带的是另一个类型编码。沿用在册最新那一笔里的同一个类型编码不算新指名——那个类型后来被停用了，库位照常登记新修订（停用只
// 作用于此后的新指名，CONTEXT「主数据登记」）。编码按字面比：只差大小写的是另一个类型。
func (content LocationContent) NamesLocationTypeNewly(latest LocationContent, hasLatest bool) bool {
	if !content.locationType.valid() {
		return false
	}
	return !hasLatest || latest.locationType != content.locationType
}

// LocationRevision 是库位册上的一笔修订：身份（租户，仓编码，库位编码）、修订号、完整内容，另记登记责任方与登记时刻。
// 修订从 1 起连续，只追加；停用与再启用都是新修订（CONTEXT「主数据登记」）。
type LocationRevision struct {
	tenant       TenantID
	warehouse    WarehouseCode
	code         LocationCode
	revision     int
	content      LocationContent
	registrant   Registrant
	registeredAt time.Time
}

// NewLocationRevision 判一笔修订的各部分是不是齐的。登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，登记口回显的
// 时刻与读回的差在纳秒上。
func NewLocationRevision(
	tenant TenantID,
	warehouse WarehouseCode,
	code LocationCode,
	revision int,
	content LocationContent,
	registrant Registrant,
	registeredAt time.Time,
) (LocationRevision, error) {
	if revision < 1 {
		return LocationRevision{}, fmt.Errorf("%w: %d", ErrInvalidRevisionNumber, revision)
	}
	if field := content.firstMissingField(); field != LocationFieldInvalid {
		return LocationRevision{}, &LocationContentFieldError{Field: field}
	}
	if !tenant.valid() || !warehouse.valid() || !code.valid() || !registrant.valid() || registeredAt.IsZero() {
		return LocationRevision{}, ErrInvalidLocationRevision
	}
	return LocationRevision{
		tenant:       tenant,
		warehouse:    warehouse,
		code:         code,
		revision:     revision,
		content:      content,
		registrant:   registrant,
		registeredAt: registeredAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func (revision LocationRevision) Tenant() TenantID {
	return revision.tenant
}

func (revision LocationRevision) Warehouse() WarehouseCode {
	return revision.warehouse
}

func (revision LocationRevision) Code() LocationCode {
	return revision.code
}

func (revision LocationRevision) Revision() int {
	return revision.revision
}

func (revision LocationRevision) Content() LocationContent {
	return revision.content
}

func (revision LocationRevision) Registrant() Registrant {
	return revision.registrant
}

func (revision LocationRevision) RegisteredAt() time.Time {
	return revision.registeredAt
}

// SameContentAs 回答两笔修订是不是同一份内容：比租户、仓编码、库位编码、修订号与完整内容，不比登记责任方与登记时刻。
// 重放（原样重投，可能换了操作者）与冲突（同一修订号换了内容）都据此判（UC-WF-001「一致性、幂等与并发」）。
func (revision LocationRevision) SameContentAs(other LocationRevision) bool {
	return revision.tenant == other.tenant &&
		revision.warehouse == other.warehouse &&
		revision.code == other.code &&
		revision.revision == other.revision &&
		revision.content == other.content
}
