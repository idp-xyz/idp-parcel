package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// 本文件是关务读客户原始资料里货物明细的口（ADR-0373 决定九；UC-CC-002 输入「客户原始资料」一行；票 declared-goods-lines/07）。
// 货物明细归小包托运，关务只读、只按（资料版本锚，委托，行号）引用：正式申报资料由关务按自己的形成规则从这些行里形成，并项、
// 补算、折算与选哪种语言的品名都在关务那一侧，正式资料不写回客户原始资料。
//
// 这里的记录是关务自己的词：来源标记、内件性质与编码口径都译成本包的常量，提供方改了线上的词，适配器的对应表就对不上、
// 答读不动，不会静默换了关务的数据；各格的值与装箱分配照客户给的原样交来，不在适配器里解释或换算。

// CustomerSourceDataAnchor 是关务记下的「按客户原始资料的哪一版形成」：要么在接受基线上（委托接受时那一版提交），要么指名一版
// 资料修订。版本不可覆盖，锚记下之后按它读回的永远是同一份行。
//
// 锚只从 FindCurrentCustomerGoodsDetails 的答复里来，不自己拼：接受基线只有 OnAcceptanceBaseline 一种写法，建案引用
// （AcceptedParcelSource.SourceRef）里的提交版本不是 SourceDataVersion——拿它当版本锚读，答的是不可见。
type CustomerSourceDataAnchor struct {
	OnAcceptanceBaseline bool
	SourceDataVersion    string
}

// CustomerContentCategory 是内件性质（UPU CN23 的勾选格）；各申报口怎样从它映射出口原因是关务的形成规则。
type CustomerContentCategory string

const (
	CustomerContentNotDeclared      CustomerContentCategory = ""
	CustomerContentGift             CustomerContentCategory = "GIFT"
	CustomerContentDocuments        CustomerContentCategory = "DOCUMENTS"
	CustomerContentCommercialSample CustomerContentCategory = "COMMERCIAL_SAMPLE"
	CustomerContentReturnedGoods    CustomerContentCategory = "RETURNED_GOODS"
	CustomerContentSaleOfGoods      CustomerContentCategory = "SALE_OF_GOODS"
	CustomerContentOther            CustomerContentCategory = "OTHER"
)

// CustomerCommodityCodeSide 是申报商品编码的口径：出口国还是进口国的编码。取哪一口径是关务的形成规则。
type CustomerCommodityCodeSide string

const (
	CustomerCommodityCodeExport CustomerCommodityCodeSide = "EXPORT"
	CustomerCommodityCodeImport CustomerCommodityCodeSide = "IMPORT"
)

// CustomerGoodsCellSource 是一格申报内容出自哪里，与小包托运的来源标记逐格对应；关务形成正式资料时按它判这一格可不可以
// 直接用（例如箱单上的品名与商品申报资料里的编码，可信度不同）。
type CustomerGoodsCellSource uint8

const (
	CustomerGoodsCellSourceUnknown CustomerGoodsCellSource = iota
	// CustomerGoodsCellFromThisDeclaration：客户在这一单上申报的。
	CustomerGoodsCellFromThisDeclaration
	// CustomerGoodsCellFromBoxList：仓配从箱单取的。
	CustomerGoodsCellFromBoxList
	// CustomerGoodsCellFromCommodityDeclaration：仓配从商品申报资料取的。
	CustomerGoodsCellFromCommodityDeclaration
)

// CustomerGoodsCell 是一格申报内容：值原样，带出处。
type CustomerGoodsCell struct {
	Value  string
	Source CustomerGoodsCellSource
}

type CustomerGoodsDescription struct {
	Language string
	Text     CustomerGoodsCell
}

type CustomerCommodityCode struct {
	Side CustomerCommodityCodeSide
	Code CustomerGoodsCell
}

type CustomerSalesOrderReference struct {
	Platform    *CustomerGoodsCell
	OrderNumber *CustomerGoodsCell
	OrderLine   *CustomerGoodsCell
}

// CustomerGoodsLine 是货物明细的一行。Line 是版本内的行号，与锚一起才是稳定引用。没报的格为 nil。
type CustomerGoodsLine struct {
	Line           int
	Descriptions   []CustomerGoodsDescription
	Quantity       *CustomerGoodsCell
	Unit           *CustomerGoodsCell
	UnitValue      *CustomerGoodsCell
	LineValue      *CustomerGoodsCell
	Currency       *CustomerGoodsCell
	NetWeight      *CustomerGoodsCell
	NetWeightUnit  *CustomerGoodsCell
	CommodityCodes []CustomerCommodityCode
	OriginCountry  *CustomerGoodsCell
	OwnerSKU       *CustomerGoodsCell
	SalesOrder     *CustomerSalesOrderReference
}

// CustomerGoodsPacking 是一条装箱分配：这件包裹装了第几行多少。按分配把行推到包裹、算逐包裹的价值，都是关务的形成规则。
type CustomerGoodsPacking struct {
	Parcel   string
	Line     int
	Quantity string
}

// CustomerGoodsDetails 是一版客户原始资料上的货物明细。Declared 为假即那一版上没有货物明细（首次提交没报，或显式清空被
// 采用）——缺席是真话，关务该停在「输入不可得」，不拿别的版本顶。
//
// Parcels 是这份委托接受时的全部包裹，锚换了也不变。没报装箱分配时要靠它分两种情形：只有一件，那一件装的就是全部行；不止
// 一件，就是整票清单，说不出哪件装了什么（ADR-0373 决定三）——逐包裹申报的口拿不到逐件内容，是资料缺口，不是零。
type CustomerGoodsDetails struct {
	Anchor                CustomerSourceDataAnchor
	Declared              bool
	ContentCategory       CustomerContentCategory
	ContentCategorySource CustomerGoodsCellSource
	ContentCategoryNote   *CustomerGoodsCell
	Lines                 []CustomerGoodsLine
	Packing               []CustomerGoodsPacking
	Parcels               []string
}

// CustomerGoodsDetailsView 是关务读货物明细的口。found=false 是统一不可见结果：这个租户下没有这份委托、委托还没接受、或锚
// 指的版本不在这份委托的货物明细范围上——不区分哪一种。error 只表示读不动。
type CustomerGoodsDetailsView interface {
	// FindCurrentCustomerGoodsDetails 答此刻该按哪一版与那一版的货物明细。undetermined 为真即两条修订分叉待复核，此刻说不出
	// 该按哪一版：关务不该挑一版形成。
	FindCurrentCustomerGoodsDetails(
		ctx context.Context,
		tenant domain.TenantID,
		shipmentRequest string,
	) (details CustomerGoodsDetails, undetermined bool, found bool, err error)

	// FindCustomerGoodsDetails 按记下的锚读回那一版的货物明细。
	FindCustomerGoodsDetails(
		ctx context.Context,
		tenant domain.TenantID,
		shipmentRequest string,
		anchor CustomerSourceDataAnchor,
	) (CustomerGoodsDetails, bool, error)
}
