package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PriceCardWorkbookCarrier 是读回的两种载体。结构化工作簿给编辑器再提交，电子表格给下载。
// 判定只有一条：两种载体都从同一份投影来，不各写一套表与列。
type PriceCardWorkbookCarrier uint8

const (
	PriceCardWorkbookCarrierInvalid PriceCardWorkbookCarrier = iota
	// PriceCardWorkbookStructured 是页面提交的那一种 JSON 工作簿。
	PriceCardWorkbookStructured
	// PriceCardWorkbookSpreadsheet 是电子表格文件。
	PriceCardWorkbookSpreadsheet
)

func (carrier PriceCardWorkbookCarrier) String() string {
	switch carrier {
	case PriceCardWorkbookStructured:
		return "STRUCTURED"
	case PriceCardWorkbookSpreadsheet:
		return "SPREADSHEET"
	default:
		return ""
	}
}

// PriceCardWorkbookSubject 是读回要的那一张卡：过了构造门的方案，加上登记要的方向授权引用。
// 授权不在方案文档里，模板的 card 表却必填它，所以两样一起交。
type PriceCardWorkbookSubject struct {
	Plan                   domain.PricingPlanVersion
	DirectionAuthorization domain.VersionReference
}

// RegisteredPriceCardRead 按（租户 + 方案版本）读回已登记的那一版，供反推成工作簿。
// 不在册答 false。快照按本构建重建不了时交回包着 domain.ErrCanonicalizationVersionUnsupported 的错误。
type RegisteredPriceCardRead interface {
	ReadRegisteredPriceCard(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.VersionReference,
	) (PriceCardWorkbookSubject, bool, error)
}

// PriceCardWorkbookDocument 是投影交回的一种载体。Body 在结构化时是工作簿 JSON，在电子表格时是文件字节。
type PriceCardWorkbookDocument struct {
	TemplateVersion string
	Body            []byte
	MediaType       string
	FileName        string
}

// PriceCardWorkbookGap 是模板表达不了的那一处。Where 点名位置，Conclusion 是业务结论。
// 有它就没有 Document：不交一份丢掉内容的工作簿。
type PriceCardWorkbookGap struct {
	Where      string
	Conclusion string
}

// PriceCardWorkbookProjection 要么是一份工作簿，要么是一处表达不了。二者互斥。
type PriceCardWorkbookProjection struct {
	Document PriceCardWorkbookDocument
	Gap      PriceCardWorkbookGap
}

// PriceCardWorkbookProjector 把一版价卡写成导入模板的工作簿。表、列与封闭取值只用模板那一份定义。
type PriceCardWorkbookProjector interface {
	ProjectPriceCardWorkbook(
		plan domain.PricingPlanVersion,
		authorization domain.VersionReference,
		carrier PriceCardWorkbookCarrier,
	) (PriceCardWorkbookProjection, error)
}
