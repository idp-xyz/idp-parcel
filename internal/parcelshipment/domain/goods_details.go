package domain

import (
	"errors"
	"fmt"
	"math/big"

	"go.idp.xyz/idp-parcel/internal/platform/storable"
)

// 本文件是 PS CONTEXT「货物明细」「货物明细行」「装箱分配」「内件性质」「申报格来源标记」「销售订单引用」的值对象
// （ADR-0373）。货物明细是客户的声明：在场的格原样保存客户给的串，本上下文只认形状，不校验格式、不换算、不交叉核对。

// ErrInvalidGoodsDetails 说明一份货物明细在形状上立不起来。它与 ErrInvalidDeclaredMeasurement 同格：客户输入的问题，
// 接入口答请求形状不对，重发同样内容不会好。
var ErrInvalidGoodsDetails = errors.New("parcel shipment: invalid goods details")

// ErrInvalidPacking 是货物明细里装箱分配那一部分的形状错，同时是 ErrInvalidGoodsDetails：接入口据它把出错位置指到包裹的
// packing，而不是笼统的货物明细。
var ErrInvalidPacking = fmt.Errorf("%w: invalid packing", ErrInvalidGoodsDetails)

// maxGoodsLines 是产品机制，防一份声明无限增长；不是任何渠道或申报口的取值，它们更严的上限归各自（ADR-0373 决定二）。
const maxGoodsLines = 999

// DeclarationSourceMark 是申报格来源标记的封闭三格（ADR-0373 决定七，承接 ADR-0370 决定五）。它只说这一格是谁说的，
// 不说这一格对不对。
type DeclarationSourceMark uint8

const (
	DeclarationSourceMarkInvalid DeclarationSourceMark = iota
	// SourceMarkThisDeclaration 是本单申报：客户渠道交来的一律是这一格，由接入口记。
	SourceMarkThisDeclaration
	// SourceMarkBoxList 是箱单：仓配整箱出时取自货主的箱单。
	SourceMarkBoxList
	// SourceMarkCommodityDeclaration 是商品申报资料：本单与箱单都没给时，仓配取自商品的申报资料。
	SourceMarkCommodityDeclaration
)

func (mark DeclarationSourceMark) String() string {
	switch mark {
	case SourceMarkThisDeclaration:
		return "THIS_DECLARATION"
	case SourceMarkBoxList:
		return "BOX_LIST"
	case SourceMarkCommodityDeclaration:
		return "COMMODITY_DECLARATION"
	default:
		return ""
	}
}

// GoodsCellSpec 是一格申报内容的输入。值为空串即这一格没报：空串不是一个品名或数量，交出去无处可用；全是空白的值
// 不是空串，原样在场——与「地址要素」同一条口径，为判在场而 trim 也是一次规范化。
type GoodsCellSpec struct {
	Value string
	Mark  DeclarationSourceMark
}

func (spec GoodsCellSpec) present() bool {
	return spec.Value != ""
}

// GoodsCell 是一格在场的申报内容与它的来源标记。
type GoodsCell struct {
	value string
	mark  DeclarationSourceMark
}

func (cell GoodsCell) Value() string {
	return cell.value
}

func (cell GoodsCell) Mark() DeclarationSourceMark {
	return cell.mark
}

func (cell GoodsCell) present() bool {
	return cell.value != ""
}

// optional 把一格交给读方：第二个返回值区分「没报」与「报了」，不拿零值冒充一格内容。
func (cell GoodsCell) optional() (GoodsCell, bool) {
	return cell, cell.present()
}

func goodsCellOf(spec GoodsCellSpec) GoodsCell {
	return GoodsCell{value: spec.Value, mark: spec.Mark}
}

// CommodityCodeSide 是申报商品编码的口径，封闭两格（ADR-0373 决定六）：同一行货在出口国与进口国常用不同的编码。
type CommodityCodeSide uint8

const (
	CommodityCodeSideInvalid CommodityCodeSide = iota
	CommodityCodeExportSide
	CommodityCodeImportSide
)

func (side CommodityCodeSide) String() string {
	switch side {
	case CommodityCodeExportSide:
		return "EXPORT"
	case CommodityCodeImportSide:
		return "IMPORT"
	default:
		return ""
	}
}

type GoodsDescriptionSpec struct {
	Language string
	Text     GoodsCellSpec
}

type CommodityCodeSpec struct {
	Side CommodityCodeSide
	Code GoodsCellSpec
}

type SalesOrderReferenceSpec struct {
	Platform    GoodsCellSpec
	OrderNumber GoodsCellSpec
	OrderLine   GoodsCellSpec
}

func (spec SalesOrderReferenceSpec) present() bool {
	return spec.Platform.present() || spec.OrderNumber.present() || spec.OrderLine.present()
}

// GoodsLineSpec 是一条货物明细行的输入，每格可缺。
type GoodsLineSpec struct {
	Descriptions   []GoodsDescriptionSpec
	Quantity       GoodsCellSpec
	Unit           GoodsCellSpec
	UnitValue      GoodsCellSpec
	LineValue      GoodsCellSpec
	Currency       GoodsCellSpec
	NetWeight      GoodsCellSpec
	NetWeightUnit  GoodsCellSpec
	CommodityCodes []CommodityCodeSpec
	OriginCountry  GoodsCellSpec
	OwnerSKU       GoodsCellSpec
	SalesOrder     SalesOrderReferenceSpec
}

func (spec GoodsLineSpec) anyCell() bool {
	if len(spec.Descriptions) > 0 || len(spec.CommodityCodes) > 0 {
		return true
	}
	for _, cell := range spec.plainCells() {
		if cell.present() {
			return true
		}
	}
	return false
}

// checkShape 只核形状（ADR-0373 决定六）。数与数之间、行与包裹测量或保价之间一概不核：平台折扣后行总价常常不等于
// 单价乘数量，按算术拒单会拒掉真实订单；那些判断归接单规则包与关务。
func (spec GoodsLineSpec) checkShape() error {
	for _, cell := range spec.plainCells() {
		if err := cell.checkPresentCell(); err != nil {
			return err
		}
	}
	// 数量与金额要读得出数，下游才谈得上换算；形状与声明测量同一把尺（decimalShape），不要求大于零。
	for _, cell := range []GoodsCellSpec{spec.Quantity, spec.UnitValue, spec.LineValue, spec.NetWeight} {
		if cell.present() && !decimalShape(cell.Value) {
			return fmt.Errorf("%q is not a number", cell.Value)
		}
	}
	languages := make(map[string]struct{}, len(spec.Descriptions))
	for _, description := range spec.Descriptions {
		if description.Language == "" || !storable.Text(description.Language) || !description.Text.present() {
			return errors.New("a description needs a language and a text")
		}
		if err := description.Text.checkPresentCell(); err != nil {
			return err
		}
		// 语言标签逐字比较：大小写折算也是一次规范化，本上下文不做。
		if _, twice := languages[description.Language]; twice {
			return fmt.Errorf("two descriptions in language %q", description.Language)
		}
		languages[description.Language] = struct{}{}
	}
	sides := make(map[CommodityCodeSide]struct{}, len(spec.CommodityCodes))
	for _, code := range spec.CommodityCodes {
		if code.Side.String() == "" || !code.Code.present() {
			return errors.New("a commodity code needs a side and a code")
		}
		if err := code.Code.checkPresentCell(); err != nil {
			return err
		}
		if _, twice := sides[code.Side]; twice {
			return fmt.Errorf("two %s commodity codes", code.Side)
		}
		sides[code.Side] = struct{}{}
	}
	return nil
}

// plainCells 交回一行里不带结构的各格（品名与编码另有语言、口径两格，单独核）。加一格只改这里，在场判断与标记核对一起覆盖到。
func (spec GoodsLineSpec) plainCells() []GoodsCellSpec {
	return []GoodsCellSpec{
		spec.Quantity, spec.Unit, spec.UnitValue, spec.LineValue, spec.Currency,
		spec.NetWeight, spec.NetWeightUnit, spec.OriginCountry, spec.OwnerSKU,
		spec.SalesOrder.Platform, spec.SalesOrder.OrderNumber, spec.SalesOrder.OrderLine,
	}
}

// checkPresentCell 核一格在场的申报内容：要带来源标记，要是库存得下的文本（storable.Text：合法 UTF-8、不含 NUL）。存不下的
// 留到写库时就成了依赖故障，而重发同样内容不会好；坏字节还会被 JSON 编码成同一个替换字符，让两份不同的内容摘要相等。
func (spec GoodsCellSpec) checkPresentCell() error {
	if !spec.present() {
		return nil
	}
	if spec.Mark.String() == "" {
		return errors.New("a declared cell carries no source mark")
	}
	if !storable.Text(spec.Value) {
		return errors.New("a declared cell is not storable text")
	}
	return nil
}

// ContentCategory 是内件性质的封闭六格，取自 UPU CN23（ADR-0373 决定二）。它是客户的声明；据它判减免或数据要求归关务。
type ContentCategory uint8

const (
	ContentCategoryInvalid ContentCategory = iota
	ContentCategoryGift
	ContentCategoryDocuments
	ContentCategoryCommercialSample
	ContentCategoryReturnedGoods
	ContentCategorySaleOfGoods
	ContentCategoryOther
)

func (category ContentCategory) String() string {
	switch category {
	case ContentCategoryGift:
		return "GIFT"
	case ContentCategoryDocuments:
		return "DOCUMENTS"
	case ContentCategoryCommercialSample:
		return "COMMERCIAL_SAMPLE"
	case ContentCategoryReturnedGoods:
		return "RETURNED_GOODS"
	case ContentCategorySaleOfGoods:
		return "SALE_OF_GOODS"
	case ContentCategoryOther:
		return "OTHER"
	default:
		return ""
	}
}

// PackingAllocationSpec 是装箱分配的一条输入：这个包裹装了这一行多少。数量随该行的计量单位，不换算。
type PackingAllocationSpec struct {
	Parcel   DeclaredParcelID
	Line     int
	Quantity string
}

// GoodsDetailsSpec 是一份委托的货物明细输入。Category 零值即没报内件性质。
type GoodsDetailsSpec struct {
	Category     ContentCategory
	CategoryMark DeclarationSourceMark
	CategoryNote GoodsCellSpec
	Lines        []GoodsLineSpec
	Packing      []PackingAllocationSpec
}

// PackingAllocation 是装箱分配的一条。它说货在哪一箱，不是申报内容，所以不带申报格来源标记（ADR-0373 决定七）。
type PackingAllocation struct {
	parcel   DeclaredParcelID
	line     int
	quantity string
}

func (allocation PackingAllocation) Parcel() DeclaredParcelID {
	return allocation.parcel
}

func (allocation PackingAllocation) Line() int {
	return allocation.line
}

func (allocation PackingAllocation) Quantity() string {
	return allocation.quantity
}

// GoodsDescription 是一条品名：语言标签原样保存，同一语言至多一条。
type GoodsDescription struct {
	language string
	text     GoodsCell
}

func (description GoodsDescription) Language() string {
	return description.language
}

func (description GoodsDescription) Text() GoodsCell {
	return description.text
}

type CommodityCode struct {
	side CommodityCodeSide
	code GoodsCell
}

func (code CommodityCode) Side() CommodityCodeSide {
	return code.side
}

func (code CommodityCode) Code() GoodsCell {
	return code.code
}

// SalesOrderReference 说一行货出自哪张销售订单。它是引用不是订单：本上下文不拥有、不核对、不据它找任何对象。
type SalesOrderReference struct {
	platform    GoodsCell
	orderNumber GoodsCell
	orderLine   GoodsCell
}

func (reference SalesOrderReference) Platform() (GoodsCell, bool) {
	return reference.platform.optional()
}

func (reference SalesOrderReference) OrderNumber() (GoodsCell, bool) {
	return reference.orderNumber.optional()
}

func (reference SalesOrderReference) OrderLine() (GoodsCell, bool) {
	return reference.orderLine.optional()
}

func (reference SalesOrderReference) present() bool {
	return reference.platform.present() || reference.orderNumber.present() || reference.orderLine.present()
}

// GoodsLine 是货物明细里的一条，说一种货。行号是版本内的序号，不跨版本延续，也不另立行键（ADR-0373 决定四）。
type GoodsLine struct {
	number         int
	descriptions   []GoodsDescription
	quantity       GoodsCell
	unit           GoodsCell
	unitValue      GoodsCell
	lineValue      GoodsCell
	currency       GoodsCell
	netWeight      GoodsCell
	netWeightUnit  GoodsCell
	commodityCodes []CommodityCode
	originCountry  GoodsCell
	ownerSKU       GoodsCell
	salesOrder     SalesOrderReference
}

func (line GoodsLine) Number() int {
	return line.number
}

func (line GoodsLine) Descriptions() []GoodsDescription {
	return append([]GoodsDescription(nil), line.descriptions...)
}

func (line GoodsLine) Quantity() (GoodsCell, bool)      { return line.quantity.optional() }
func (line GoodsLine) Unit() (GoodsCell, bool)          { return line.unit.optional() }
func (line GoodsLine) UnitValue() (GoodsCell, bool)     { return line.unitValue.optional() }
func (line GoodsLine) LineValue() (GoodsCell, bool)     { return line.lineValue.optional() }
func (line GoodsLine) Currency() (GoodsCell, bool)      { return line.currency.optional() }
func (line GoodsLine) NetWeight() (GoodsCell, bool)     { return line.netWeight.optional() }
func (line GoodsLine) NetWeightUnit() (GoodsCell, bool) { return line.netWeightUnit.optional() }
func (line GoodsLine) OriginCountry() (GoodsCell, bool) { return line.originCountry.optional() }
func (line GoodsLine) OwnerSKU() (GoodsCell, bool)      { return line.ownerSKU.optional() }

func (line GoodsLine) CommodityCodes() []CommodityCode {
	return append([]CommodityCode(nil), line.commodityCodes...)
}

func (line GoodsLine) SalesOrder() (SalesOrderReference, bool) {
	return line.salesOrder, line.salesOrder.present()
}

// GoodsDetails 是一份委托的货物明细。零值是「没报」：整份缺席合法，必填与否由所采用接单规则包说。
type GoodsDetails struct {
	category     ContentCategory
	categoryMark DeclarationSourceMark
	categoryNote GoodsCell
	lines        []GoodsLine
	packing      []PackingAllocation
}

// Declared 报告这份委托报没报货物明细。只报了内件性质「文件」也算报了。
func (details GoodsDetails) Declared() bool {
	return details.category != ContentCategoryInvalid || len(details.lines) > 0
}

// Category 交回内件性质与它的来源标记；第三个返回值为假即没报。
func (details GoodsDetails) Category() (ContentCategory, DeclarationSourceMark, bool) {
	return details.category, details.categoryMark, details.category != ContentCategoryInvalid
}

func (details GoodsDetails) CategoryNote() (GoodsCell, bool) {
	return details.categoryNote.optional()
}

func (details GoodsDetails) Lines() []GoodsLine {
	return append([]GoodsLine(nil), details.lines...)
}

// Packing 交回装箱分配；空即没报（单件委托，或多件委托只报整票清单）。
func (details GoodsDetails) Packing() []PackingAllocation {
	return append([]PackingAllocation(nil), details.packing...)
}

// NewGoodsDetails 是货物明细唯一的构造门，只认形状。parcels 是这份委托声明的成员集合，装箱分配只能指它们。
func NewGoodsDetails(spec GoodsDetailsSpec, parcels []DeclaredParcelID) (GoodsDetails, error) {
	// 文件件可以不逐项报货（ADR-0373 决定二）；其余情形至少一行，否则就是什么都没说。
	documentsOnly := len(spec.Lines) == 0 && spec.Category == ContentCategoryDocuments
	if (len(spec.Lines) == 0 && !documentsOnly) || len(spec.Lines) > maxGoodsLines {
		return GoodsDetails{}, fmt.Errorf("%w: a declaration carries 1 to %d lines (none only for documents), got %d",
			ErrInvalidGoodsDetails, maxGoodsLines, len(spec.Lines))
	}
	if err := checkContentCategory(spec); err != nil {
		return GoodsDetails{}, err
	}
	lines := make([]GoodsLine, 0, len(spec.Lines))
	for index, lineSpec := range spec.Lines {
		line, err := goodsLineOf(index+1, lineSpec)
		if err != nil {
			return GoodsDetails{}, err
		}
		lines = append(lines, line)
	}
	packing, err := packingOf(spec.Packing, spec.Lines, parcels)
	if err != nil {
		return GoodsDetails{}, err
	}
	return GoodsDetails{
		category:     spec.Category,
		categoryMark: spec.CategoryMark,
		categoryNote: goodsCellOf(spec.CategoryNote),
		lines:        lines,
		packing:      packing,
	}, nil
}

func checkContentCategory(spec GoodsDetailsSpec) error {
	if spec.Category == ContentCategoryInvalid {
		// 没报内件性质时标记与说明都不该在：留着它们，读口答没报、摘要却不同，同一请求重发会被判冲突。
		if spec.CategoryNote.present() || spec.CategoryMark != DeclarationSourceMarkInvalid {
			return fmt.Errorf("%w: a content category mark or note without a category", ErrInvalidGoodsDetails)
		}
		return nil
	}
	if spec.Category.String() == "" || spec.CategoryMark.String() == "" {
		return fmt.Errorf("%w: content category %d needs to be one of the six and carry a source mark", ErrInvalidGoodsDetails, spec.Category)
	}
	// 说明只给「其他」：CN23 的说明栏就是为「其他」补一句是什么，别的五格本身已经说清。
	if spec.CategoryNote.present() && spec.Category != ContentCategoryOther {
		return fmt.Errorf("%w: only OTHER carries a note", ErrInvalidGoodsDetails)
	}
	if err := spec.CategoryNote.checkPresentCell(); err != nil {
		return fmt.Errorf("%w: content category note: %v", ErrInvalidGoodsDetails, err)
	}
	return nil
}

// packingOf 核装箱分配要么不报、要么报全（ADR-0373 决定三）。这是同一份声明内部的结构——行写多少、各箱分了多少——
// 不是跨来源核对，所以与「不交叉核对」不冲突。数量按十进制精确相加，不经浮点。
func packingOf(specs []PackingAllocationSpec, lines []GoodsLineSpec, parcels []DeclaredParcelID) ([]PackingAllocation, error) {
	if len(specs) == 0 {
		return nil, nil
	}
	members := make(map[DeclaredParcelID]struct{}, len(parcels))
	for _, parcel := range parcels {
		members[parcel] = struct{}{}
	}
	// 同一包裹同一行至多一条，所以条数到不了行数乘包裹数之上；先拦住，不为一份过大的声明逐条解析数量。
	if len(specs) > len(lines)*len(members) {
		return nil, fmt.Errorf("%w: packing has more entries than lines times parcels", ErrInvalidPacking)
	}
	type cellKey struct {
		parcel DeclaredParcelID
		line   int
	}
	seen := make(map[cellKey]struct{}, len(specs))
	sums := make([]*big.Rat, len(lines))
	allocations := make([]PackingAllocation, 0, len(specs))
	for _, spec := range specs {
		if _, member := members[spec.Parcel]; !member {
			return nil, fmt.Errorf("%w: packing names parcel %q, which is outside the request", ErrInvalidPacking, spec.Parcel)
		}
		if spec.Line < 1 || spec.Line > len(lines) {
			return nil, fmt.Errorf("%w: parcel %q names line %d, the declaration has %d", ErrInvalidPacking, spec.Parcel, spec.Line, len(lines))
		}
		key := cellKey{parcel: spec.Parcel, line: spec.Line}
		if _, twice := seen[key]; twice {
			return nil, fmt.Errorf("%w: parcel %q names line %d twice", ErrInvalidPacking, spec.Parcel, spec.Line)
		}
		seen[key] = struct{}{}
		// 分配一条就是说这一箱装了这一行若干：零件说不出任何事，所以要正数。
		if !decimalShape(spec.Quantity) || !positiveDecimal(spec.Quantity) {
			return nil, fmt.Errorf("%w: parcel %q packs %q of line %d, which is not a positive number", ErrInvalidPacking, spec.Parcel, spec.Quantity, spec.Line)
		}
		quantity, _ := new(big.Rat).SetString(spec.Quantity)
		if sums[spec.Line-1] == nil {
			sums[spec.Line-1] = new(big.Rat)
		}
		sums[spec.Line-1].Add(sums[spec.Line-1], quantity)
		allocations = append(allocations, PackingAllocation{parcel: spec.Parcel, line: spec.Line, quantity: spec.Quantity})
	}
	if err := checkPackingCoversMembers(allocations, members); err != nil {
		return nil, err
	}
	for index, line := range lines {
		if !line.Quantity.present() {
			return nil, fmt.Errorf("%w: line %d has packing but no quantity", ErrInvalidPacking, index+1)
		}
		total, _ := new(big.Rat).SetString(line.Quantity.Value)
		allocated := sums[index]
		if allocated == nil {
			allocated = new(big.Rat)
		}
		if total.Cmp(allocated) != 0 {
			return nil, fmt.Errorf("%w: line %d declares %s but packing adds up to %s",
				ErrInvalidPacking, index+1, line.Quantity.Value, allocated.FloatString(6))
		}
	}
	return allocations, nil
}

// goodsFitMembers 对着一个提交版本的成员集合核货物明细（checkPackingCoversMembers）。建单、补充与重建各核一次：货物明细与
// 成员是分开交进来的，聚合不收两边对不上的一份。
func goodsFitMembers(goods GoodsDetails, parcels []DeclaredParcelID) error {
	members := make(map[DeclaredParcelID]struct{}, len(parcels))
	for _, parcel := range parcels {
		members[parcel] = struct{}{}
	}
	return checkPackingCoversMembers(goods.packing, members)
}

// checkPackingCoversMembers 核装箱分配只指成员、每个成员至少一条。构造时与算摘要时各核一次，后者对着这份提交的成员集合：
// 货物明细与成员是分开交进来的，两边的成员集合对不上时，摘要不收一份委托管线会拒的输入。
func checkPackingCoversMembers(packing []PackingAllocation, members map[DeclaredParcelID]struct{}) error {
	if len(packing) == 0 {
		return nil
	}
	covered := make(map[DeclaredParcelID]struct{}, len(members))
	for _, allocation := range packing {
		if _, member := members[allocation.parcel]; !member {
			return fmt.Errorf("%w: packing names parcel %q, which is outside the request", ErrInvalidPacking, allocation.parcel)
		}
		covered[allocation.parcel] = struct{}{}
	}
	for member := range members {
		if _, packed := covered[member]; !packed {
			return fmt.Errorf("%w: packing leaves parcel %q out; declare it for every parcel or not at all", ErrInvalidPacking, member)
		}
	}
	return nil
}

// WithPackingParcels 交回装箱分配换了包裹标识的一份拷贝，其余不动。接入口用它把对着客户包裹参考造好、校验过的货物明细
// 挪到现签或回执里的成员上：两边是同一份内容，不必为第二套标识再解析、再求和一遍。renamed 要覆盖分配指到的每个包裹。
func (details GoodsDetails) WithPackingParcels(renamed map[DeclaredParcelID]DeclaredParcelID) (GoodsDetails, error) {
	// 换名要一一对应：两个来源包裹换成同一个成员，同一个成员同一行就会有两条分配，破坏「至多一条」。
	targets := make(map[DeclaredParcelID]DeclaredParcelID, len(renamed))
	for source, target := range renamed {
		if earlier, taken := targets[target]; taken && earlier != source {
			return GoodsDetails{}, fmt.Errorf("%w: parcels %q and %q both become %q", ErrInvalidPacking, earlier, source, target)
		}
		targets[target] = source
	}
	copied := details
	copied.packing = make([]PackingAllocation, 0, len(details.packing))
	for _, allocation := range details.packing {
		parcel, known := renamed[allocation.parcel]
		if !known || !parcel.valid() {
			return GoodsDetails{}, fmt.Errorf("%w: no member for packed parcel %q", ErrInvalidPacking, allocation.parcel)
		}
		allocation.parcel = parcel
		copied.packing = append(copied.packing, allocation)
	}
	copied.lines = append([]GoodsLine(nil), details.lines...)
	return copied, nil
}

func goodsLineOf(number int, spec GoodsLineSpec) (GoodsLine, error) {
	if !spec.anyCell() {
		return GoodsLine{}, fmt.Errorf("%w: line %d declares no cell", ErrInvalidGoodsDetails, number)
	}
	if err := spec.checkShape(); err != nil {
		return GoodsLine{}, fmt.Errorf("%w: line %d: %v", ErrInvalidGoodsDetails, number, err)
	}
	return goodsLineValueOf(number, spec), nil
}

// goodsLineValueOf 只把输入搬成值，不核形状；接入走 goodsLineOf 先核，读回走 RehydrateGoodsDetails 不核。
func goodsLineValueOf(number int, spec GoodsLineSpec) GoodsLine {
	descriptions := make([]GoodsDescription, 0, len(spec.Descriptions))
	for _, description := range spec.Descriptions {
		descriptions = append(descriptions, GoodsDescription{language: description.Language, text: goodsCellOf(description.Text)})
	}
	codes := make([]CommodityCode, 0, len(spec.CommodityCodes))
	for _, code := range spec.CommodityCodes {
		codes = append(codes, CommodityCode{side: code.Side, code: goodsCellOf(code.Code)})
	}
	return GoodsLine{
		number:         number,
		descriptions:   descriptions,
		quantity:       goodsCellOf(spec.Quantity),
		unit:           goodsCellOf(spec.Unit),
		unitValue:      goodsCellOf(spec.UnitValue),
		lineValue:      goodsCellOf(spec.LineValue),
		currency:       goodsCellOf(spec.Currency),
		netWeight:      goodsCellOf(spec.NetWeight),
		netWeightUnit:  goodsCellOf(spec.NetWeightUnit),
		commodityCodes: codes,
		originCountry:  goodsCellOf(spec.OriginCountry),
		ownerSKU:       goodsCellOf(spec.OwnerSKU),
		salesOrder: SalesOrderReference{
			platform:    goodsCellOf(spec.SalesOrder.Platform),
			orderNumber: goodsCellOf(spec.SalesOrder.OrderNumber),
			orderLine:   goodsCellOf(spec.SalesOrder.OrderLine),
		},
	}
}

// RehydrateGoodsDetails 还原库里已经存下的货物明细，不再过接入的门（行数上限、可存文本、十进制形状、分配求和）：门守的是进
// 来，不守读回——门日后收紧，已存的委托照样要读得回来（与 RehydrateExternalIdentifierNumber 同一个理由）。装箱分配是否贴合
// 成员，由聚合重建时对着版本的成员核一次（goodsFitMembers）。只给仓储还原用，新内容一律走 NewGoodsDetails。
func RehydrateGoodsDetails(spec GoodsDetailsSpec) GoodsDetails {
	details := GoodsDetails{
		category:     spec.Category,
		categoryMark: spec.CategoryMark,
		categoryNote: goodsCellOf(spec.CategoryNote),
	}
	for index, line := range spec.Lines {
		details.lines = append(details.lines, goodsLineValueOf(index+1, line))
	}
	for _, allocation := range spec.Packing {
		details.packing = append(details.packing, PackingAllocation{parcel: allocation.Parcel, line: allocation.Line, quantity: allocation.Quantity})
	}
	return details
}

// ParseDeclarationSourceMark、ParseContentCategory 与 ParseCommodityCodeSide 把封闭集合的词译回取值，给接入口与仓储共用：
// 词表只在各自的 String 一处，解析按它逐格比对，不另写一份。第二个返回值为假即这个词不在集合里。
func ParseDeclarationSourceMark(word string) (DeclarationSourceMark, bool) {
	for _, mark := range []DeclarationSourceMark{SourceMarkThisDeclaration, SourceMarkBoxList, SourceMarkCommodityDeclaration} {
		if mark.String() == word {
			return mark, true
		}
	}
	return DeclarationSourceMarkInvalid, false
}

func ParseContentCategory(word string) (ContentCategory, bool) {
	for category := ContentCategoryGift; category <= ContentCategoryOther; category++ {
		if category.String() == word {
			return category, true
		}
	}
	return ContentCategoryInvalid, false
}

func ParseCommodityCodeSide(word string) (CommodityCodeSide, bool) {
	for _, side := range []CommodityCodeSide{CommodityCodeExportSide, CommodityCodeImportSide} {
		if side.String() == word {
			return side, true
		}
	}
	return CommodityCodeSideInvalid, false
}
