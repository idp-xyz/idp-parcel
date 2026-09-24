package ports

import "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"

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
