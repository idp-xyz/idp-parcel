package ports

import (
	"errors"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 价卡导入模板的读口（ADR-0101 决定二至四）。模板怎么读、问题码各指什么，由设计文档
// docs/design/pp-price-card-import-template-and-validation-spec.md 定；这里只立预览与录入
// 两条编排共用的形状。两条编排拿同一个读口、同一份读法，交出的内容摘要才逐字节相同——
// 否则会长出「预览通过、录入却不同」那一格（ADR-0101 决定四）。

// PriceCardTemplateReader 把一份上传文件的原始字节读成三种读法之一：整份不收、带问题、已校验。
// 读法本身就是答案，所以没有错误返回——读不通是一种读法，不是故障。
type PriceCardTemplateReader interface {
	ReadPriceCardTemplate(raw []byte) PriceCardTemplateReading
}

// PriceCardTemplateReading 是一次读取的结果。
type PriceCardTemplateReading struct {
	// TemplateVersion 是读口认的模板版本，给答复与完成记录看。
	TemplateVersion string
	// Accepted 为假即整份不收：Problems 恰有一条整份文件层面的问题，Plan 与 Content 都没有。
	Accepted bool
	// Plan 是读出的方案版本引用；Accepted 为真时必有，草稿按它一版一行。
	Plan domain.VersionReference
	// Content 只在没有任何问题时有：方案已过构造门、内容摘要已算出，即生命周期的「已校验」。
	Content *PriceCardTemplateContent
	// Problems 按模板的表序、行号、列序排好。
	Problems []PriceCardTemplateProblem
}

// PriceCardTemplateContent 是读通了的一张卡：方案与登记要的方向授权引用。租户、录入者、
// 批准者与源文件身份不在模板里，由编排从信封与上传字节补上。
type PriceCardTemplateContent struct {
	Plan                   domain.PricingPlanVersion
	DirectionAuthorization domain.VersionReference
}

// PriceCardTemplateProblem 是一条带坐标的问题。Sheet 为空是整份文件层面的；Row 为 0 是表
// 层面的；Column 是列键（card 表写字段名），指不到列键时写电子表格的列名。
type PriceCardTemplateProblem struct {
	Sheet   string
	Row     int
	Column  string
	Code    PriceCardTemplateProblemCode
	Message string
}

// PriceCardTemplateProblemCode 是问题码的封闭集合，各码的情形见设计文档第六节。
type PriceCardTemplateProblemCode string

// 整份不收：不落草稿行，录入口答未受理。
const (
	TemplateFileTooLarge           PriceCardTemplateProblemCode = "FILE_TOO_LARGE"
	TemplateFileNotWorkbook        PriceCardTemplateProblemCode = "FILE_NOT_WORKBOOK"
	TemplateVersionUnsupported     PriceCardTemplateProblemCode = "TEMPLATE_VERSION_UNSUPPORTED"
	TemplatePlanIdentityUnreadable PriceCardTemplateProblemCode = "PLAN_IDENTITY_UNREADABLE"
)

// 进草稿带问题：方案身份读得出来，其余某一格有问题。
const (
	TemplateSheetMissing         PriceCardTemplateProblemCode = "SHEET_MISSING"
	TemplateSheetUnknown         PriceCardTemplateProblemCode = "SHEET_UNKNOWN"
	TemplateColumnMissing        PriceCardTemplateProblemCode = "COLUMN_MISSING"
	TemplateColumnUnknown        PriceCardTemplateProblemCode = "COLUMN_UNKNOWN"
	TemplateColumnDuplicated     PriceCardTemplateProblemCode = "COLUMN_DUPLICATED"
	TemplateFieldMissing         PriceCardTemplateProblemCode = "FIELD_MISSING"
	TemplateFieldUnknown         PriceCardTemplateProblemCode = "FIELD_UNKNOWN"
	TemplateFieldDuplicated      PriceCardTemplateProblemCode = "FIELD_DUPLICATED"
	TemplateCellRequired         PriceCardTemplateProblemCode = "CELL_REQUIRED"
	TemplateCellNotApplicable    PriceCardTemplateProblemCode = "CELL_NOT_APPLICABLE"
	TemplateCellFormula          PriceCardTemplateProblemCode = "CELL_FORMULA"
	TemplateCellErrorValue       PriceCardTemplateProblemCode = "CELL_ERROR_VALUE"
	TemplateCellNotText          PriceCardTemplateProblemCode = "CELL_NOT_TEXT"
	TemplateCellNumericNotExact  PriceCardTemplateProblemCode = "CELL_NUMERIC_NOT_EXACT"
	TemplateCellInvalid          PriceCardTemplateProblemCode = "CELL_INVALID"
	TemplateIDDuplicated         PriceCardTemplateProblemCode = "ID_DUPLICATED"
	TemplateReferenceUnresolved  PriceCardTemplateProblemCode = "REFERENCE_UNRESOLVED"
	TemplateReferenceUnused      PriceCardTemplateProblemCode = "REFERENCE_UNUSED"
	TemplateTreeInvalid          PriceCardTemplateProblemCode = "TREE_INVALID"
	TemplateConstructionRejected PriceCardTemplateProblemCode = "CONSTRUCTION_REJECTED"
)

// 价卡导入的第二种载体：页面表单提交的结构化工作簿（JSON）。它与电子表格文件是同一个模板的两种写法，读出来走同一套
// 译码与校验，所以读法的三种形状、问题码与坐标都与读文件一模一样；变的只有字节从哪来、源文件身份怎么记。

var (
	// ErrPriceCardWorkbookTooLarge：提交的工作簿 JSON 超过技术上限（与文件同一组上限）。
	ErrPriceCardWorkbookTooLarge = errors.New("parcel pricing: price card workbook exceeds the read limits")
	// ErrPriceCardWorkbookMalformed：提交的不是一份形状对的工作簿 JSON（不是 JSON、顶层缺键或多键、表不是数组、格不是
	// 字符串 / 数字 / null、同一对象里同名键出现两次……）。
	ErrPriceCardWorkbookMalformed = errors.New("parcel pricing: price card workbook is malformed")
)

// PriceCardWorkbookRejection 是工作簿 JSON 的形状没过：Detail 是说人话的说明（哪里不对），errors.Is 认上面两个哨兵。
// 它存在是为了让答复里的话是 Detail 本身，而不是带英文哨兵前缀的错误串。
type PriceCardWorkbookRejection struct {
	TooLarge bool
	Detail   string
}

func (rejection PriceCardWorkbookRejection) Error() string {
	if rejection.TooLarge {
		return ErrPriceCardWorkbookTooLarge.Error() + ": " + rejection.Detail
	}
	return ErrPriceCardWorkbookMalformed.Error() + ": " + rejection.Detail
}

func (rejection PriceCardWorkbookRejection) Is(target error) bool {
	if rejection.TooLarge {
		return target == ErrPriceCardWorkbookTooLarge
	}
	return target == ErrPriceCardWorkbookMalformed
}

// PriceCardWorkbookReader 读页面表单提交的结构化工作簿。分两步，是为了让「源文件身份」与「被读的字节」是同一份：先规范化
// （键排序、去空白，数值字面量原样保留），身份取规范化字节的散列；再读规范化字节。同一份内容无论空白与键序怎么写，
// 得到同一个身份与同一份读法。
type PriceCardWorkbookReader interface {
	// CanonicalizePriceCardWorkbook 校验工作簿 JSON 的形状并给出规范化字节；形状不对或超限答 PriceCardWorkbookRejection。
	// 形状之外的一切（表、列、格的内容）不在这一步判。
	CanonicalizePriceCardWorkbook(raw []byte) ([]byte, error)
	// ReadPriceCardWorkbook 读规范化字节。读法本身就是答案，没有错误返回。
	ReadPriceCardWorkbook(canonical []byte) PriceCardTemplateReading
}

// PriceCardTemplateSpec 是模板规格：版本、表清单与每一列的定义。它由读取模板的同一张列定义生成，增删一列规格自动跟着变，
// 编辑器照它画表单而不是在前端抄一份列表。
type PriceCardTemplateSpec struct {
	// TemplateVersion 是本构建写出的版本（最新的一版）；AcceptedTemplateVersions 是本构建读得了的全部版本。
	TemplateVersion          string
	AcceptedTemplateVersions []string
	Sheets                   []PriceCardTemplateSheetSpec
}

// PriceCardTemplateSheetSpec 是一张表。Layout 为 ROWS 的表一行一条记录、列就是 Columns；为 FIELDS 的表（card）每个字段
// 一行，Columns 列的是字段，工作簿里写成 {field, value} 行。
type PriceCardTemplateSheetSpec struct {
	Name       string
	Title      string
	Purpose    string
	Layout     string
	Repeatable bool
	// Since 非空：这张表自该版起才有。
	Since   string
	Columns []PriceCardTemplateColumnSpec
}

// PriceCardTemplateColumnSpec 是一列（或 card 表的一个字段）。Requirement 取 always、conditional、optional。AllowedValues 只有
// 封闭取值的列才有；MultiValued 为真时那一格是以英文分号分隔的列表，AllowedValues 是每个元素的可取值。
type PriceCardTemplateColumnSpec struct {
	Key           string
	Title         string
	Requirement   string
	Description   string
	AllowedValues []string
	MultiValued   bool
}

// PriceCardTemplateSpecSource 交出模板规格。
type PriceCardTemplateSpecSource interface {
	PriceCardTemplateSpec() PriceCardTemplateSpec
}
