package domain

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"time"
)

var (
	// ErrInvalidGoodsContent 是一份商品资料缺了必填的格；具体缺哪一格由 GoodsContentFieldError 交回。
	ErrInvalidGoodsContent = errors.New("warehouse fulfillment: invalid goods content")
	// ErrInvalidGoodsDeclaration 是商品申报资料里申报单价与币种没有同在同缺。
	ErrInvalidGoodsDeclaration = errors.New("warehouse fulfillment: invalid goods declaration")
	// ErrInvalidDeclaredMeasurement 是一条按包装层级声明的尺寸重量里尺寸与重量两样都没给，或尺寸的某一格没给。
	ErrInvalidDeclaredMeasurement = errors.New("warehouse fulfillment: invalid declared measurement")
)

// GoodsField 指名商品资料里的一格，登记口答「资料内容不合」时逐格指出差在哪里。键路径与登记报文、答复体里的写法一致；列表里的格带 `[]`，
// 配上位置才是一个确切的格（见 GoodsCell）。位置按报文里交来的先后数，不按存储或规范化之后的次序。
type GoodsField uint8

const (
	GoodsFieldInvalid GoodsField = iota
	GoodsFieldName
	GoodsFieldBaseUnit
	GoodsFieldPackagingLevels
	GoodsFieldPackagingLevelName
	GoodsFieldPackagingLevelFactor
	GoodsFieldAliasCode
	GoodsFieldAliasType
	GoodsFieldAliasPlatform
	GoodsFieldAliasValidity
	GoodsFieldCompositionComponents
	GoodsFieldCompositionOutboundMode
	GoodsFieldComponentCode
	GoodsFieldComponentQuantity
	GoodsFieldDeclarationOriginCountry
	GoodsFieldDeclarationUnitPrice
	GoodsFieldDeclarationCurrency
	GoodsFieldMeasurementLevel
	GoodsFieldMeasurementLength
	GoodsFieldMeasurementWidth
	GoodsFieldMeasurementHeight
	GoodsFieldMeasurementDimensionsUnit
	GoodsFieldMeasurementWeightValue
	GoodsFieldMeasurementWeightUnit
)

func (field GoodsField) String() string {
	switch field {
	case GoodsFieldName:
		return "name"
	case GoodsFieldBaseUnit:
		return "baseUnit"
	case GoodsFieldPackagingLevels:
		return "packagingLevels"
	case GoodsFieldPackagingLevelName:
		return "packagingLevels[].name"
	case GoodsFieldPackagingLevelFactor:
		return "packagingLevels[].factor"
	case GoodsFieldAliasCode:
		return "aliases[].code"
	case GoodsFieldAliasType:
		return "aliases[].type"
	case GoodsFieldAliasPlatform:
		return "aliases[].platform"
	case GoodsFieldAliasValidity:
		return "aliases[].validity"
	case GoodsFieldCompositionComponents:
		return "composition.components"
	case GoodsFieldCompositionOutboundMode:
		return "composition.outboundMode"
	case GoodsFieldComponentCode:
		return "composition.components[].code"
	case GoodsFieldComponentQuantity:
		return "composition.components[].quantity"
	case GoodsFieldDeclarationOriginCountry:
		return "declaration.originCountry"
	case GoodsFieldDeclarationUnitPrice:
		return "declaration.unitPrice"
	case GoodsFieldDeclarationCurrency:
		return "declaration.currency"
	case GoodsFieldMeasurementLevel:
		return "declaredMeasurements[].level"
	case GoodsFieldMeasurementLength:
		return "declaredMeasurements[].dimensions.length"
	case GoodsFieldMeasurementWidth:
		return "declaredMeasurements[].dimensions.width"
	case GoodsFieldMeasurementHeight:
		return "declaredMeasurements[].dimensions.height"
	case GoodsFieldMeasurementDimensionsUnit:
		return "declaredMeasurements[].dimensions.unit"
	case GoodsFieldMeasurementWeightValue:
		return "declaredMeasurements[].weight.value"
	case GoodsFieldMeasurementWeightUnit:
		return "declaredMeasurements[].weight.unit"
	default:
		return ""
	}
}

// Indexed 回答这一格是不是列表里的一项：是的话，指出它要带位置。
func (field GoodsField) Indexed() bool {
	return strings.Contains(field.String(), "[]")
}

// GoodsCell 是商品资料里一个确切的格：格加上（列表项的）位置。位置只对列表里的格有意义，别的格上恒为 0 且不出现在写法里。
type GoodsCell struct {
	field GoodsField
	index int
}

func NewGoodsCell(field GoodsField, index int) GoodsCell {
	if !field.Indexed() {
		index = 0
	}
	return GoodsCell{field: field, index: index}
}

func (cell GoodsCell) Field() GoodsField {
	return cell.field
}

// Index 交回列表项的位置；这一格不在列表里时第二个返回值为假。
func (cell GoodsCell) Index() (int, bool) {
	return cell.index, cell.field.Indexed()
}

// String 是格在报文里的键路径：`aliases[1].validity`、`packagingLevels`。
func (cell GoodsCell) String() string {
	if !cell.field.Indexed() {
		return cell.field.String()
	}
	return strings.Replace(cell.field.String(), "[]", "["+strconv.Itoa(cell.index)+"]", 1)
}

// GoodsContentFieldError 指出一份商品资料里哪一格没给。
type GoodsContentFieldError struct {
	Cell GoodsCell
}

func (err *GoodsContentFieldError) Error() string {
	return fmt.Sprintf("%s: %s", ErrInvalidGoodsContent, err.Cell)
}

func (err *GoodsContentFieldError) Is(target error) bool {
	return target == ErrInvalidGoodsContent
}

// PackagingLevel 是商品声明的一个包装层级：一个单位名与折合多少个基本单位。折合数在这里就是一个整数，「是正整数」是登记口判的资料内容，
// 不拦在构造器里——不然负数与零在到登记口之前就成了坏载荷，登记方看不到「哪一层的折合数不成立」。
type PackagingLevel struct {
	name   GoodsUnitName
	factor int64
}

func NewPackagingLevel(name GoodsUnitName, factor int64) PackagingLevel {
	return PackagingLevel{name: name, factor: factor}
}

func (level PackagingLevel) Name() GoodsUnitName {
	return level.name
}

func (level PackagingLevel) Factor() int64 {
	return level.factor
}

// AliasValidity 是别名的有效区间，起含、止不含；没给的那一端按无界算（CONTEXT「商品别名」）。给出的时刻统一到 UTC 并收成库里存得下的精度
// （微秒），零时刻就是没给。
type AliasValidity struct {
	from time.Time
	to   time.Time
}

func NewAliasValidity(from, to time.Time) AliasValidity {
	return AliasValidity{from: normalizedInstant(from), to: normalizedInstant(to)}
}

func normalizedInstant(at time.Time) time.Time {
	if at.IsZero() {
		return time.Time{}
	}
	return at.UTC().Truncate(time.Microsecond)
}

// From 交回区间的起；没给起时第二个返回值为假。
func (validity AliasValidity) From() (time.Time, bool) {
	return validity.from, !validity.from.IsZero()
}

// To 交回区间的止；没给止时第二个返回值为假。
func (validity AliasValidity) To() (time.Time, bool) {
	return validity.to, !validity.to.IsZero()
}

// Inverted 回答两端都给了、而止不晚于起：这样的区间不成立，是资料内容不合。
func (validity AliasValidity) Inverted() bool {
	return !validity.from.IsZero() && !validity.to.IsZero() && !validity.to.After(validity.from)
}

// Contains 回答 at 这一刻落不落在区间里：起含、止不含，没给的那一端无界。
func (validity AliasValidity) Contains(at time.Time) bool {
	if !validity.from.IsZero() && at.Before(validity.from) {
		return false
	}
	return validity.to.IsZero() || at.Before(validity.to)
}

// Overlaps 回答两个区间有没有交叠。两个半开区间交叠，当且仅当各自的起都早于对方的止；没给的起当负无穷、没给的止当正无穷。
// 不成立的区间（止不晚于起）是空的，不占任何时刻，与谁都不交叠——它之后会被判资料内容不合，不该先抢走一个码。
func (validity AliasValidity) Overlaps(other AliasValidity) bool {
	if validity.Inverted() || other.Inverted() {
		return false
	}
	return startsBeforeEnd(validity.from, other.to) && startsBeforeEnd(other.from, validity.to)
}

func startsBeforeEnd(start, end time.Time) bool {
	return start.IsZero() || end.IsZero() || start.Before(end)
}

// GoodsAlias 是同一货主内只指向一个商品的另一个识别码：码、类型、可选的平台或渠道名、可选的有效区间。平台或渠道名只有平台商品码可以带，
// 条码带了是资料内容不合，登记口判。
type GoodsAlias struct {
	code     GoodsAliasCode
	kind     GoodsAliasKind
	platform PlatformName
	validity AliasValidity
}

func NewGoodsAlias(code GoodsAliasCode, kind GoodsAliasKind, platform PlatformName, validity AliasValidity) GoodsAlias {
	return GoodsAlias{code: code, kind: kind, platform: platform, validity: validity}
}

func (alias GoodsAlias) Code() GoodsAliasCode {
	return alias.code
}

func (alias GoodsAlias) Kind() GoodsAliasKind {
	return alias.kind
}

// Platform 交回平台或渠道名；没带时第二个返回值为假。
func (alias GoodsAlias) Platform() (PlatformName, bool) {
	return alias.platform, alias.platform.valid()
}

func (alias GoodsAlias) Validity() AliasValidity {
	return alias.validity
}

// GoodsComponent 是组合商品的一个组件：组件的商品编码与数量，数量按组件的基本单位计。「数量是正整数」同折合数，由登记口判。
type GoodsComponent struct {
	code     GoodsCode
	quantity int64
}

func NewGoodsComponent(code GoodsCode, quantity int64) GoodsComponent {
	return GoodsComponent{code: code, quantity: quantity}
}

func (component GoodsComponent) Code() GoodsCode {
	return component.code
}

func (component GoodsComponent) Quantity() int64 {
	return component.quantity
}

// GoodsDeclarationSpec 是商品申报资料的各格，每一格都可缺；零值就是没给。
type GoodsDeclarationSpec struct {
	Name          DeclaredGoodsName
	HSCode        HSCode
	OriginCountry CountryCode
	UnitPrice     Measure
	Currency      CurrencyCode
}

// GoodsDeclaration 是货主为商品声明的申报资料：申报品名、HS 编码、原产国、申报单价与币种。整份不带与各格都没给是同一回事。它是货主的声明性
// 资料，本上下文只保存，不判归类与合规。
type GoodsDeclaration struct {
	name          DeclaredGoodsName
	hsCode        HSCode
	originCountry CountryCode
	unitPrice     Measure
	currency      CurrencyCode
}

// NewGoodsDeclaration 要求申报单价与币种同在同缺：只给一个的价没有意义，是报文形状不合，不是资料内容不合。单价的数值写法已过 Measure 的构造门；
// 「为正」与币种「在目录里」由登记口判。
func NewGoodsDeclaration(spec GoodsDeclarationSpec) (GoodsDeclaration, error) {
	if spec.UnitPrice.valid() != spec.Currency.valid() {
		return GoodsDeclaration{}, ErrInvalidGoodsDeclaration
	}
	return GoodsDeclaration{
		name: spec.Name, hsCode: spec.HSCode, originCountry: spec.OriginCountry, unitPrice: spec.UnitPrice, currency: spec.Currency,
	}, nil
}

func (declaration GoodsDeclaration) Name() (DeclaredGoodsName, bool) {
	return declaration.name, declaration.name.valid()
}

func (declaration GoodsDeclaration) HSCode() (HSCode, bool) {
	return declaration.hsCode, declaration.hsCode.valid()
}

func (declaration GoodsDeclaration) OriginCountry() (CountryCode, bool) {
	return declaration.originCountry, declaration.originCountry.valid()
}

// UnitPrice 交回申报单价与它的币种；没给时第三个返回值为假。
func (declaration GoodsDeclaration) UnitPrice() (Measure, CurrencyCode, bool) {
	return declaration.unitPrice, declaration.currency, declaration.unitPrice.valid()
}

// DeclaredDimensions 是货主声明的一个包装层级的尺寸：长、宽、高与一个长度单位。四格一起给，缺一格是报文形状不合。
type DeclaredDimensions struct {
	length Measure
	width  Measure
	height Measure
	unit   UnitCode
}

func NewDeclaredDimensions(length, width, height Measure, unit UnitCode) (DeclaredDimensions, error) {
	dimensions := DeclaredDimensions{length: length, width: width, height: height, unit: unit}
	if !length.valid() || !width.valid() || !height.valid() || !unit.valid() {
		return DeclaredDimensions{}, ErrInvalidDeclaredMeasurement
	}
	return dimensions, nil
}

func (dimensions DeclaredDimensions) Length() Measure {
	return dimensions.length
}

func (dimensions DeclaredDimensions) Width() Measure {
	return dimensions.width
}

func (dimensions DeclaredDimensions) Height() Measure {
	return dimensions.height
}

func (dimensions DeclaredDimensions) Unit() UnitCode {
	return dimensions.unit
}

// DeclaredWeight 是货主声明的一个包装层级的重量：一个数值与一个重量单位，两格一起给。
type DeclaredWeight struct {
	value Measure
	unit  UnitCode
}

func NewDeclaredWeight(value Measure, unit UnitCode) (DeclaredWeight, error) {
	if !value.valid() || !unit.valid() {
		return DeclaredWeight{}, ErrInvalidDeclaredMeasurement
	}
	return DeclaredWeight{value: value, unit: unit}, nil
}

func (weight DeclaredWeight) Value() Measure {
	return weight.value
}

func (weight DeclaredWeight) Unit() UnitCode {
	return weight.unit
}

// DeclaredMeasurementSpec 是一条按包装层级声明的尺寸重量的各格：尺寸与重量各自可缺，但至少给一样。
type DeclaredMeasurementSpec struct {
	Level      GoodsUnitName
	Dimensions *DeclaredDimensions
	Weight     *DeclaredWeight
}

// DeclaredMeasurement 是货主按包装层级声明的尺寸与重量。它是货主的声明，不是仓测：仓测是仓内作业另登记的，读口把两样分开标明来源。
type DeclaredMeasurement struct {
	level         GoodsUnitName
	dimensions    DeclaredDimensions
	hasDimensions bool
	weight        DeclaredWeight
	hasWeight     bool
}

// NewDeclaredMeasurement 要求尺寸与重量至少给一样：两样都没给的一条声明什么也没声明。层级是不是本商品声明过的层级、数值为正、单位在目录里
// 且类别对，都是登记口判的资料内容。
func NewDeclaredMeasurement(spec DeclaredMeasurementSpec) (DeclaredMeasurement, error) {
	measurement := DeclaredMeasurement{level: spec.Level}
	if spec.Dimensions != nil {
		measurement.dimensions, measurement.hasDimensions = *spec.Dimensions, true
	}
	if spec.Weight != nil {
		measurement.weight, measurement.hasWeight = *spec.Weight, true
	}
	if !measurement.hasDimensions && !measurement.hasWeight {
		return DeclaredMeasurement{}, ErrInvalidDeclaredMeasurement
	}
	return measurement, nil
}

func (measurement DeclaredMeasurement) Level() GoodsUnitName {
	return measurement.level
}

// Dimensions 交回声明的尺寸；没声明时第二个返回值为假。
func (measurement DeclaredMeasurement) Dimensions() (DeclaredDimensions, bool) {
	return measurement.dimensions, measurement.hasDimensions
}

// Weight 交回声明的重量；没声明时第二个返回值为假。
func (measurement DeclaredMeasurement) Weight() (DeclaredWeight, bool) {
	return measurement.weight, measurement.hasWeight
}

// GoodsContentSpec 是一份商品资料的各格。是否跟踪批号与效期缺省就是不跟踪；非组合商品不带出库方式与组件。
type GoodsContentSpec struct {
	Name                 GoodsName
	BaseUnit             GoodsUnitName
	PackagingLevels      []PackagingLevel
	TrackBatch           bool
	TrackExpiry          bool
	TrackSerial          bool
	Aliases              []GoodsAlias
	Composite            bool
	OutboundMode         GoodsOutboundMode
	Components           []GoodsComponent
	Declaration          GoodsDeclaration
	DeclaredMeasurements []DeclaredMeasurement
}

// GoodsContent 是一笔商品修订里随修订号变的那份完整资料，不是增量。包装层级、别名、组件与声明的尺寸重量这几个列表保持报文里交来的先后，
// 读回也是这个次序；可是「内容相同」不看先后（见 SameAs），同一份资料换个次序重投仍是重放。
//
// 构造器只要求必填的格给齐。折合数为正、层名不重复、基本单位是折合为一的那一层、目录里的码、数值为正、区间先后、组成成立，都是只拦新修订的
// 登记门，在登记用例里判（同包材那一册对目录与「为正」的做法）：在册的修订读回时不经它们，不会被后来的目录改版牵连。
type GoodsContent struct {
	name         GoodsName
	baseUnit     GoodsUnitName
	levels       []PackagingLevel
	trackBatch   bool
	trackExpiry  bool
	trackSerial  bool
	aliases      []GoodsAlias
	composite    bool
	outboundMode GoodsOutboundMode
	components   []GoodsComponent
	declaration  GoodsDeclaration
	measurements []DeclaredMeasurement
}

// NewGoodsContent 要求名称、基本单位与各列表项里的必填格都给齐；缺哪一格就指出哪一格（缺了几处时指出先遇到的那一处）。各列表复制一份，
// 之后调用方改自己的切片不动已构造的资料。
func NewGoodsContent(spec GoodsContentSpec) (GoodsContent, error) {
	content := GoodsContent{
		name:         spec.Name,
		baseUnit:     spec.BaseUnit,
		levels:       slices.Clone(spec.PackagingLevels),
		trackBatch:   spec.TrackBatch,
		trackExpiry:  spec.TrackExpiry,
		trackSerial:  spec.TrackSerial,
		aliases:      slices.Clone(spec.Aliases),
		composite:    spec.Composite,
		outboundMode: spec.OutboundMode,
		components:   slices.Clone(spec.Components),
		declaration:  spec.Declaration,
		measurements: slices.Clone(spec.DeclaredMeasurements),
	}
	if missing := content.MissingCells(); len(missing) > 0 {
		return GoodsContent{}, &GoodsContentFieldError{Cell: missing[0]}
	}
	return content, nil
}

// MissingCells 逐格列出没给的必填格，次序固定：名称、基本单位，再是各列表项（层名；别名的码与类型；组件的商品编码；声明的尺寸重量的层级）。
// 经构造器得来的资料恒为空；零值的资料缺名称与基本单位。
func (content GoodsContent) MissingCells() []GoodsCell {
	var cells []GoodsCell
	if !content.name.valid() {
		cells = append(cells, NewGoodsCell(GoodsFieldName, 0))
	}
	if !content.baseUnit.valid() {
		cells = append(cells, NewGoodsCell(GoodsFieldBaseUnit, 0))
	}
	for index, level := range content.levels {
		if !level.name.valid() {
			cells = append(cells, NewGoodsCell(GoodsFieldPackagingLevelName, index))
		}
	}
	for index, alias := range content.aliases {
		if !alias.code.valid() {
			cells = append(cells, NewGoodsCell(GoodsFieldAliasCode, index))
		}
		if alias.kind.String() == "" {
			cells = append(cells, NewGoodsCell(GoodsFieldAliasType, index))
		}
	}
	for index, component := range content.components {
		if !component.code.valid() {
			cells = append(cells, NewGoodsCell(GoodsFieldComponentCode, index))
		}
	}
	for index, measurement := range content.measurements {
		if !measurement.level.valid() {
			cells = append(cells, NewGoodsCell(GoodsFieldMeasurementLevel, index))
		}
	}
	return cells
}

func (content GoodsContent) Name() GoodsName {
	return content.name
}

func (content GoodsContent) BaseUnit() GoodsUnitName {
	return content.baseUnit
}

// PackagingLevels 按报文里交来的先后交回各包装层级的副本。
func (content GoodsContent) PackagingLevels() []PackagingLevel {
	return slices.Clone(content.levels)
}

func (content GoodsContent) TrackBatch() bool {
	return content.trackBatch
}

func (content GoodsContent) TrackExpiry() bool {
	return content.trackExpiry
}

func (content GoodsContent) TrackSerial() bool {
	return content.trackSerial
}

func (content GoodsContent) Aliases() []GoodsAlias {
	return slices.Clone(content.aliases)
}

func (content GoodsContent) Composite() bool {
	return content.composite
}

// OutboundMode 交回组合商品的出库方式；没给时第二个返回值为假。
func (content GoodsContent) OutboundMode() (GoodsOutboundMode, bool) {
	return content.outboundMode, content.outboundMode.String() != ""
}

func (content GoodsContent) Components() []GoodsComponent {
	return slices.Clone(content.components)
}

func (content GoodsContent) Declaration() GoodsDeclaration {
	return content.declaration
}

func (content GoodsContent) DeclaredMeasurements() []DeclaredMeasurement {
	return slices.Clone(content.measurements)
}

// SameAs 回答两份资料是不是同一份内容。标量格逐格比；几个列表按集合比（各项规范成一个键、排序后逐项比），所以别名、包装层级、组件与声明的
// 尺寸重量在报文里的先后不影响结论。数值按数值比（12.5 与 12.50 同一内容），码与名逐字比。
//
// 不比的是资料之外的东西：登记责任方与登记时刻不在资料里。
func (content GoodsContent) SameAs(other GoodsContent) bool {
	return content.name == other.name &&
		content.baseUnit == other.baseUnit &&
		content.trackBatch == other.trackBatch &&
		content.trackExpiry == other.trackExpiry &&
		content.trackSerial == other.trackSerial &&
		content.composite == other.composite &&
		content.outboundMode == other.outboundMode &&
		content.declaration == other.declaration &&
		slices.Equal(canonicalKeys(content.levels, packagingLevelKey), canonicalKeys(other.levels, packagingLevelKey)) &&
		slices.Equal(canonicalKeys(content.aliases, goodsAliasKey), canonicalKeys(other.aliases, goodsAliasKey)) &&
		slices.Equal(canonicalKeys(content.components, goodsComponentKey), canonicalKeys(other.components, goodsComponentKey)) &&
		slices.Equal(canonicalKeys(content.measurements, declaredMeasurementKey), canonicalKeys(other.measurements, declaredMeasurementKey))
}

// canonicalKeys 把一个列表的每一项规范成一个键并排序。键里用 NUL 作分隔：逐字保存的取值不含 NUL（构造门拒收），分隔不会与取值混淆。
func canonicalKeys[Item any](items []Item, key func(Item) string) []string {
	keys := make([]string, len(items))
	for index, item := range items {
		keys[index] = key(item)
	}
	slices.Sort(keys)
	return keys
}

func joinKey(parts ...string) string {
	return strings.Join(parts, "\x00")
}

func instantKey(at time.Time) string {
	if at.IsZero() {
		return ""
	}
	return strconv.FormatInt(at.UnixMicro(), 10)
}

func packagingLevelKey(level PackagingLevel) string {
	return joinKey(level.name.String(), strconv.FormatInt(level.factor, 10))
}

func goodsAliasKey(alias GoodsAlias) string {
	return joinKey(
		alias.code.String(), alias.kind.String(), alias.platform.String(),
		instantKey(alias.validity.from), instantKey(alias.validity.to),
	)
}

func goodsComponentKey(component GoodsComponent) string {
	return joinKey(component.code.String(), strconv.FormatInt(component.quantity, 10))
}

func declaredMeasurementKey(measurement DeclaredMeasurement) string {
	dimensions, weight := "-", "-"
	if measurement.hasDimensions {
		dimensions = joinKey(
			measurement.dimensions.length.String(), measurement.dimensions.width.String(),
			measurement.dimensions.height.String(), measurement.dimensions.unit.String(),
		)
	}
	if measurement.hasWeight {
		weight = joinKey(measurement.weight.value.String(), measurement.weight.unit.String())
	}
	return joinKey(measurement.level.String(), "D", dimensions, "W", weight)
}

// GoodsImmutableField 指名登记后不改的那一格。商品编码是身份，不在这里：它本来就改不了，换个编码就是另一个商品。
type GoodsImmutableField uint8

const (
	GoodsImmutableFieldInvalid GoodsImmutableField = iota
	GoodsImmutableBaseUnit
	GoodsImmutableComposite
	GoodsImmutablePackagingLevelRemoved
	GoodsImmutablePackagingLevelFactor
	GoodsImmutableTrackBatch
	GoodsImmutableTrackExpiry
)

func (field GoodsImmutableField) String() string {
	switch field {
	case GoodsImmutableBaseUnit:
		return "baseUnit"
	case GoodsImmutableComposite:
		return "composite"
	case GoodsImmutablePackagingLevelRemoved:
		return "packagingLevels"
	case GoodsImmutablePackagingLevelFactor:
		return "packagingLevels.factor"
	case GoodsImmutableTrackBatch:
		return "trackBatch"
	case GoodsImmutableTrackExpiry:
		return "trackExpiry"
	default:
		return ""
	}
}

// GoodsImmutableCell 是「不可改的格」答复指出的那一格；包装层级的两种（删掉、改折合数）另带是哪一层。
type GoodsImmutableCell struct {
	field GoodsImmutableField
	level GoodsUnitName
}

func (cell GoodsImmutableCell) Field() GoodsImmutableField {
	return cell.field
}

// Level 交回被删或被改折合数的那一层的单位名；别的格上第二个返回值为假。
func (cell GoodsImmutableCell) Level() (GoodsUnitName, bool) {
	return cell.level, cell.level.valid()
}

func (cell GoodsImmutableCell) String() string {
	return cell.field.String()
}

// StructuralChangeFrom 找新资料相对在册最新那一笔改了哪个「登记后不改」的结构格：基本单位；是否组合商品；已声明的包装层级不删、折合数不改
// （可以追加）。次序固定，指出先遇到的那一格：基本单位，是否组合，再按在册那一笔里各层的先后逐层看（先看删没删、再看折合数）。包装层级按单位名认。
func (content GoodsContent) StructuralChangeFrom(previous GoodsContent) (GoodsImmutableCell, bool) {
	if content.baseUnit != previous.baseUnit {
		return GoodsImmutableCell{field: GoodsImmutableBaseUnit}, true
	}
	if content.composite != previous.composite {
		return GoodsImmutableCell{field: GoodsImmutableComposite}, true
	}
	for _, declared := range previous.levels {
		index := slices.IndexFunc(content.levels, func(level PackagingLevel) bool { return level.name == declared.name })
		switch {
		case index < 0:
			return GoodsImmutableCell{field: GoodsImmutablePackagingLevelRemoved, level: declared.name}, true
		case content.levels[index].factor != declared.factor:
			return GoodsImmutableCell{field: GoodsImmutablePackagingLevelFactor, level: declared.name}, true
		}
	}
	return GoodsImmutableCell{}, false
}

// TrackingChangeFrom 找新资料相对在册最新那一笔改了是否跟踪批号或效期（先看批号）。这两格只在商品没有在库库存时可改，所以它们与结构格分开：
// 判不判得先问库存账。
func (content GoodsContent) TrackingChangeFrom(previous GoodsContent) (GoodsImmutableCell, bool) {
	if content.trackBatch != previous.trackBatch {
		return GoodsImmutableCell{field: GoodsImmutableTrackBatch}, true
	}
	if content.trackExpiry != previous.trackExpiry {
		return GoodsImmutableCell{field: GoodsImmutableTrackExpiry}, true
	}
	return GoodsImmutableCell{}, false
}

// IdentifierClaim 是一份资料对同一货主识别空间里一个码的占用：商品编码占它那个码、两端无界；别名占它声明的有效区间。
type IdentifierClaim struct {
	code     IdentifierCode
	validity AliasValidity
}

func (claim IdentifierClaim) Code() IdentifierCode {
	return claim.code
}

func (claim IdentifierClaim) Validity() AliasValidity {
	return claim.validity
}

// Collides 回答两处占用是不是同一个码、区间有交叠——在同一货主内属于不同商品时，这就是识别码冲突（CONTEXT「主数据登记」）。
func (claim IdentifierClaim) Collides(other IdentifierClaim) bool {
	return claim.code == other.code && claim.validity.Overlaps(other.validity)
}

// IdentifierClaims 列出这份资料占用的码：商品编码在先、两端无界，各别名按报文里的先后随后。
func (content GoodsContent) IdentifierClaims(code GoodsCode) []IdentifierClaim {
	claims := make([]IdentifierClaim, 0, 1+len(content.aliases))
	claims = append(claims, IdentifierClaim{code: IdentifierCode{code.requiredValue}})
	for _, alias := range content.aliases {
		claims = append(claims, IdentifierClaim{code: IdentifierCode{alias.code.requiredValue}, validity: alias.validity})
	}
	return claims
}

// RepeatedIdentifier 找同一份资料里先出现两次的那个码：商品编码与各别名里的码，不论区间。别名等于自己的商品编码也在内。
// 按 IdentifierClaims 的次序，交回第二次出现的那个码。
func (content GoodsContent) RepeatedIdentifier(code GoodsCode) (IdentifierCode, bool) {
	seen := make(map[IdentifierCode]bool)
	for _, claim := range content.IdentifierClaims(code) {
		if seen[claim.code] {
			return claim.code, true
		}
		seen[claim.code] = true
	}
	return IdentifierCode{}, false
}
