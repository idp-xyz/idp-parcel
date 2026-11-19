package domain

import (
	"errors"
	"fmt"
	"time"
)

var (
	// ErrInvalidPackagingMaterialContent 是一份包材内容缺了必填的格；具体缺哪一格由 PackagingMaterialContentFieldError 交回。
	ErrInvalidPackagingMaterialContent = errors.New("warehouse fulfillment: invalid packaging material content")
	// ErrInvalidPackagingMaterialRevision 是一笔包材修订缺了身份、登记责任方或登记时刻。
	ErrInvalidPackagingMaterialRevision = errors.New("warehouse fulfillment: invalid packaging material revision")
)

// PackagingMaterialField 指名包材修订上的一格，登记口答「内容不合」时用它指出差在哪里。键路径与登记报文、答复体里的写法一致：
// 外廓尺寸的三条边与单位在 `dimensions` 下，重量的数值与单位在 `weight` 下。包材没有登记后不改的格，也没有指名别的对象的格。
type PackagingMaterialField uint8

const (
	PackagingMaterialFieldInvalid PackagingMaterialField = iota
	PackagingMaterialFieldName
	PackagingMaterialFieldDimensionsLength
	PackagingMaterialFieldDimensionsWidth
	PackagingMaterialFieldDimensionsHeight
	PackagingMaterialFieldDimensionsUnit
	PackagingMaterialFieldWeightValue
	PackagingMaterialFieldWeightUnit
)

func (field PackagingMaterialField) String() string {
	switch field {
	case PackagingMaterialFieldName:
		return "name"
	case PackagingMaterialFieldDimensionsLength:
		return "dimensions.length"
	case PackagingMaterialFieldDimensionsWidth:
		return "dimensions.width"
	case PackagingMaterialFieldDimensionsHeight:
		return "dimensions.height"
	case PackagingMaterialFieldDimensionsUnit:
		return "dimensions.unit"
	case PackagingMaterialFieldWeightValue:
		return "weight.value"
	case PackagingMaterialFieldWeightUnit:
		return "weight.unit"
	default:
		return ""
	}
}

// PackagingMaterialContentFieldError 指出一份包材内容里哪一格不成立。
type PackagingMaterialContentFieldError struct {
	Field PackagingMaterialField
}

func (err *PackagingMaterialContentFieldError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidPackagingMaterialContent, err.Field)
}

func (err *PackagingMaterialContentFieldError) Is(target error) bool {
	return target == ErrInvalidPackagingMaterialContent
}

// PackagingMaterialCode 是包材的编码，与租户合成包材的身份，登记后不改。逐字保存：包材表是租户取值，各仓共用，编码的写法完全是租户的
// （CONTEXT「包材」）。
type PackagingMaterialCode struct{ requiredValue }

func NewPackagingMaterialCode(value string) (PackagingMaterialCode, error) {
	required, err := newRequiredValue("packaging material code", value)
	return PackagingMaterialCode{required}, err
}

type PackagingMaterialName struct{ requiredValue }

func NewPackagingMaterialName(value string) (PackagingMaterialName, error) {
	required, err := newRequiredValue("packaging material name", value)
	return PackagingMaterialName{required}, err
}

// UnitCode 是计量单位的码，按码逐字保存。构造器只管「给了且存得下」；是不是产品内置计量单位目录里的码、是长度类还是重量类，由登记口在
// 判过重放、冲突与修订连续之后另判（ADR-0335，同国家码的做法），读回的在册行因此不受目录更新牵连。登记时不折算单位：`30 CM` 与 `0.3 M`
// 是两份不同的内容。
type UnitCode struct{ requiredValue }

func NewUnitCode(value string) (UnitCode, error) {
	required, err := newRequiredValue("unit code", value)
	return UnitCode{required}, err
}

// PackagingMaterialContentSpec 是一份包材内容的各格：外廓尺寸（长、宽、高三个数值与一个长度单位）、重量（一个数值与一个重量单位）。
// 是否停用缺省就是不停用。
type PackagingMaterialContentSpec struct {
	Name           PackagingMaterialName
	Length         Measure
	Width          Measure
	Height         Measure
	DimensionsUnit UnitCode
	Weight         Measure
	WeightUnit     UnitCode
	Deactivated    bool
}

// PackagingMaterialContent 是一笔包材修订里随修订号变的那份完整内容。它的每一格都可比较，内容相同的判定直接用 ==：数值按数值比，
// 不按写法比（Measure 只存规范形），单位按码逐字比；往里加一格不属于「内容」的东西（登记责任方、登记时刻之类）会让重放被误判成冲突，
// 所以那些格放在修订上，不放在这里。
//
// 构造器只要求各格都给齐、数值写法合规。数值「为正」与单位「在目录里且类别对」是只拦新修订的登记门，在登记用例里判（见 Measure）。
type PackagingMaterialContent struct {
	name           PackagingMaterialName
	length         Measure
	width          Measure
	height         Measure
	dimensionsUnit UnitCode
	weight         Measure
	weightUnit     UnitCode
	deactivated    bool
}

// NewPackagingMaterialContent 要求名称、四个数值与两个单位都给齐；缺哪一格就指出哪一格。
func NewPackagingMaterialContent(spec PackagingMaterialContentSpec) (PackagingMaterialContent, error) {
	content := PackagingMaterialContent{
		name:           spec.Name,
		length:         spec.Length,
		width:          spec.Width,
		height:         spec.Height,
		dimensionsUnit: spec.DimensionsUnit,
		weight:         spec.Weight,
		weightUnit:     spec.WeightUnit,
		deactivated:    spec.Deactivated,
	}
	if field := content.firstMissingField(); field != PackagingMaterialFieldInvalid {
		return PackagingMaterialContent{}, &PackagingMaterialContentFieldError{Field: field}
	}
	return content, nil
}

func (content PackagingMaterialContent) firstMissingField() PackagingMaterialField {
	switch {
	case !content.name.valid():
		return PackagingMaterialFieldName
	case !content.length.valid():
		return PackagingMaterialFieldDimensionsLength
	case !content.width.valid():
		return PackagingMaterialFieldDimensionsWidth
	case !content.height.valid():
		return PackagingMaterialFieldDimensionsHeight
	case !content.dimensionsUnit.valid():
		return PackagingMaterialFieldDimensionsUnit
	case !content.weight.valid():
		return PackagingMaterialFieldWeightValue
	case !content.weightUnit.valid():
		return PackagingMaterialFieldWeightUnit
	default:
		return PackagingMaterialFieldInvalid
	}
}

func (content PackagingMaterialContent) Name() PackagingMaterialName {
	return content.name
}

func (content PackagingMaterialContent) Length() Measure {
	return content.length
}

func (content PackagingMaterialContent) Width() Measure {
	return content.width
}

func (content PackagingMaterialContent) Height() Measure {
	return content.height
}

func (content PackagingMaterialContent) DimensionsUnit() UnitCode {
	return content.dimensionsUnit
}

func (content PackagingMaterialContent) Weight() Measure {
	return content.weight
}

func (content PackagingMaterialContent) WeightUnit() UnitCode {
	return content.weightUnit
}

func (content PackagingMaterialContent) Deactivated() bool {
	return content.deactivated
}

// PackagingMaterialRevision 是包材册上的一笔修订：身份（租户，包材编码）、修订号、完整内容，另记登记责任方与登记时刻。
// 修订从 1 起连续，只追加；停用与再启用都是新修订（CONTEXT「主数据登记」）。
type PackagingMaterialRevision struct {
	tenant       TenantID
	code         PackagingMaterialCode
	revision     int
	content      PackagingMaterialContent
	registrant   Registrant
	registeredAt time.Time
}

// NewPackagingMaterialRevision 判一笔修订的各部分是不是齐的。登记时刻收成库里存得下的精度（微秒）并统一到 UTC：不收的话，登记口回显的
// 时刻与读回的差在纳秒上。
func NewPackagingMaterialRevision(
	tenant TenantID,
	code PackagingMaterialCode,
	revision int,
	content PackagingMaterialContent,
	registrant Registrant,
	registeredAt time.Time,
) (PackagingMaterialRevision, error) {
	if revision < 1 {
		return PackagingMaterialRevision{}, fmt.Errorf("%w: %d", ErrInvalidRevisionNumber, revision)
	}
	if field := content.firstMissingField(); field != PackagingMaterialFieldInvalid {
		return PackagingMaterialRevision{}, &PackagingMaterialContentFieldError{Field: field}
	}
	if !tenant.valid() || !code.valid() || !registrant.valid() || registeredAt.IsZero() {
		return PackagingMaterialRevision{}, ErrInvalidPackagingMaterialRevision
	}
	return PackagingMaterialRevision{
		tenant:       tenant,
		code:         code,
		revision:     revision,
		content:      content,
		registrant:   registrant,
		registeredAt: registeredAt.UTC().Truncate(time.Microsecond),
	}, nil
}

func (revision PackagingMaterialRevision) Tenant() TenantID {
	return revision.tenant
}

func (revision PackagingMaterialRevision) Code() PackagingMaterialCode {
	return revision.code
}

func (revision PackagingMaterialRevision) Revision() int {
	return revision.revision
}

func (revision PackagingMaterialRevision) Content() PackagingMaterialContent {
	return revision.content
}

func (revision PackagingMaterialRevision) Registrant() Registrant {
	return revision.registrant
}

func (revision PackagingMaterialRevision) RegisteredAt() time.Time {
	return revision.registeredAt
}

// SameContentAs 回答两笔修订是不是同一份内容：比租户、包材编码、修订号与完整内容，不比登记责任方与登记时刻。数值按数值比（12.5 与 12.50
// 同一内容），单位按码逐字比（30 CM 与 0.3 M 不是同一内容）。重放（原样重投，可能换了操作者）与冲突（同一修订号换了内容）都据此判
// （UC-WF-001「一致性、幂等与并发」）。
func (revision PackagingMaterialRevision) SameContentAs(other PackagingMaterialRevision) bool {
	return revision.tenant == other.tenant &&
		revision.code == other.code &&
		revision.revision == other.revision &&
		revision.content == other.content
}
