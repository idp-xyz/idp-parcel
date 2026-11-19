package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidZoneContent 是一份库区内容缺了必填的格；具体缺哪一格由 ZoneContentFieldError 交回。
	ErrInvalidZoneContent = errors.New("warehouse fulfillment: invalid zone content")
	// ErrInvalidZoneRevision 是一笔库区修订缺了身份、登记责任方或登记时刻。
	ErrInvalidZoneRevision = errors.New("warehouse fulfillment: invalid zone revision")
)

// ZoneField 指名库区修订上的一格，登记口答「不可改的格」与「查无所指」时用它指出差在哪里。仓编码这一格不在库区的内容里，
// 它是库区身份的一段、指名的是仓：库区登记口答「指名的仓查不到」时指出它。
type ZoneField uint8

const (
	ZoneFieldInvalid ZoneField = iota
	ZoneFieldWarehouseCode
	ZoneFieldName
	ZoneFieldUsage
)

func (field ZoneField) String() string {
	switch field {
	case ZoneFieldWarehouseCode:
		return "warehouseCode"
	case ZoneFieldName:
		return "name"
	case ZoneFieldUsage:
		return "usage"
	default:
		return ""
	}
}

// ZoneContentFieldError 指出一份库区内容里哪一格不成立。
type ZoneContentFieldError struct {
	Field ZoneField
}

func (err *ZoneContentFieldError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidZoneContent, err.Field)
}

func (err *ZoneContentFieldError) Is(target error) bool {
	return target == ErrInvalidZoneContent
}

// ZoneCode 是库区的编码，与租户、仓编码合成库区的身份，登记后不改。逐字保存：同一个编码在不同的仓下是不同的库区。
type ZoneCode struct{ requiredValue }

func NewZoneCode(value string) (ZoneCode, error) {
	required, err := newRequiredValue("zone code", value)
	return ZoneCode{required}, err
}

type ZoneName struct{ requiredValue }

func NewZoneName(value string) (ZoneName, error) {
	required, err := newRequiredValue("zone name", value)
	return ZoneName{required}, err
}

// ZoneUsage 是库区用途，产品内置的封闭六种（CONTEXT「库区」）。登记后不改。库存状态与库区用途的相容关系、哪些用途不可分配
// 是库内用例的规则，不在登记这一册里。
type ZoneUsage uint8

const (
	ZoneUsageInvalid ZoneUsage = iota
	ZoneUsageReceivingStaging
	ZoneUsageStorage
	ZoneUsagePicking
	ZoneUsageDefective
	ZoneUsageReturnStaging
	ZoneUsageOutboundStaging
)

func (usage ZoneUsage) String() string {
	switch usage {
	case ZoneUsageReceivingStaging:
		return "RECEIVING_STAGING"
	case ZoneUsageStorage:
		return "STORAGE"
	case ZoneUsagePicking:
		return "PICKING"
	case ZoneUsageDefective:
		return "DEFECTIVE"
	case ZoneUsageReturnStaging:
		return "RETURN_STAGING"
	case ZoneUsageOutboundStaging:
		return "OUTBOUND_STAGING"
	default:
		return ""
	}
}

// ZoneUsages 按字面量的写法顺序交回封闭集里的六种，每次是一份新切片。接入侧据它写「用途须是其中之一」的理由，持久化面的封闭集
// 检查据它逐种核对；封闭集本身只此一处。
func ZoneUsages() []ZoneUsage {
	return []ZoneUsage{
		ZoneUsageReceivingStaging, ZoneUsageStorage, ZoneUsagePicking,
		ZoneUsageDefective, ZoneUsageReturnStaging, ZoneUsageOutboundStaging,
	}
}

// ZoneUsageNamed 把字面量反查回封闭集；空串与集外的词都答不认识，由接入侧按坏载荷答，不是未受理。
func ZoneUsageNamed(name string) (ZoneUsage, bool) {
	for _, usage := range ZoneUsages() {
		if usage.String() == name {
			return usage, true
		}
	}
	return ZoneUsageInvalid, false
}

// ZoneContentSpec 是一份库区内容的各格。是否停用缺省就是不停用。
type ZoneContentSpec struct {
	Name        ZoneName
	Usage       ZoneUsage
	Deactivated bool
}

// ZoneContent 是一笔库区修订里随修订号变的那份完整内容。它的每一格都可比较，内容相同的判定直接用 ==：往里加一格不属于
// 「内容」的东西（登记责任方、登记时刻之类）会让重放被误判成冲突，所以那些格放在修订上，不放在这里。
type ZoneContent struct {
	name        ZoneName
	usage       ZoneUsage
	deactivated bool
}

// NewZoneContent 要求名称与用途都给齐；缺哪一格就指出哪一格。
func NewZoneContent(spec ZoneContentSpec) (ZoneContent, error) {
	content := ZoneContent{name: spec.Name, usage: spec.Usage, deactivated: spec.Deactivated}
	if field := content.firstMissingField(); field != ZoneFieldInvalid {
		return ZoneContent{}, &ZoneContentFieldError{Field: field}
	}
	return content, nil
}

func (content ZoneContent) firstMissingField() ZoneField {
	switch {
	case !content.name.valid():
		return ZoneFieldName
	case content.usage.String() == "":
		return ZoneFieldUsage
	default:
		return ZoneFieldInvalid
	}
}

func (content ZoneContent) Name() ZoneName {
	return content.name
}

func (content ZoneContent) Usage() ZoneUsage {
	return content.usage
}

func (content ZoneContent) Deactivated() bool {
	return content.deactivated
}

// ZoneRevision 是库区册上的一笔修订：身份（租户，仓编码，库区编码）、修订号、完整内容，另记登记责任方与登记时刻。
// 修订从 1 起连续，只追加；停用与再启用都是新修订（CONTEXT「主数据登记」）。
type ZoneRevision struct {
	tenant       TenantID
	warehouse    WarehouseCode
	code         ZoneCode
	revision     int
	content      ZoneContent
	registrant   Registrant
	registeredAt time.Time
}

// NewZoneRevision 判一笔修订的各部分是不是齐的。登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，登记口回显的
// 时刻与读回的差在纳秒上。
func NewZoneRevision(
	tenant TenantID,
	warehouse WarehouseCode,
	code ZoneCode,
	revision int,
	content ZoneContent,
	registrant Registrant,
	registeredAt time.Time,
) (ZoneRevision, error) {
	if revision < 1 {
		return ZoneRevision{}, fmt.Errorf("%w: %d", ErrInvalidRevisionNumber, revision)
	}
	if field := content.firstMissingField(); field != ZoneFieldInvalid {
		return ZoneRevision{}, &ZoneContentFieldError{Field: field}
	}
	if !tenant.valid() || !warehouse.valid() || !code.valid() || !registrant.valid() || registeredAt.IsZero() {
		return ZoneRevision{}, ErrInvalidZoneRevision
	}
	return ZoneRevision{
		tenant:       tenant,
		warehouse:    warehouse,
		code:         code,
		revision:     revision,
		content:      content,
		registrant:   registrant,
		registeredAt: registeredAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func (revision ZoneRevision) Tenant() TenantID {
	return revision.tenant
}

func (revision ZoneRevision) Warehouse() WarehouseCode {
	return revision.warehouse
}

func (revision ZoneRevision) Code() ZoneCode {
	return revision.code
}

func (revision ZoneRevision) Revision() int {
	return revision.revision
}

func (revision ZoneRevision) Content() ZoneContent {
	return revision.content
}

func (revision ZoneRevision) Registrant() Registrant {
	return revision.registrant
}

func (revision ZoneRevision) RegisteredAt() time.Time {
	return revision.registeredAt
}

// SameContentAs 回答两笔修订是不是同一份内容：比租户、仓编码、库区编码、修订号与完整内容，不比登记责任方与登记时刻。
// 重放（原样重投，可能换了操作者）与冲突（同一修订号换了内容）都据此判（UC-WF-001「一致性、幂等与并发」）。
func (revision ZoneRevision) SameContentAs(other ZoneRevision) bool {
	return revision.tenant == other.tenant &&
		revision.warehouse == other.warehouse &&
		revision.code == other.code &&
		revision.revision == other.revision &&
		revision.content == other.content
}
