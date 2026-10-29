package ports

import (
	"context"
	"encoding/json"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// PriceCardContent 是一版已登记价卡的内容查阅结果：登记册上那一版方案的规范化文档，加上登记元数据。
//
// Document 是登记快照里 `plan` 那一块的原样 JSON——键、值、嵌套都不经服务端翻译，更不换成面向某个页面的形状：
// 读面只搬运，翻成人话是呈现面的事。它在交出之前已经走过领域整图重验（含按规范化版本重算内容摘要自校），所以
// 读到的文档就是评价会用的那一份，不是一份可能漂离了列面的快照。
type PriceCardContent struct {
	PlanID              string
	PlanVersion         string
	Canonicalization    string
	ContentDigest       string
	PublicationApprover string
	RegisteredAt        time.Time
	Document            json.RawMessage
}

// PriceCardContentRead 按（租户 + 方案版本引用）读回一版价卡的登记内容。与目录列面（PriceCardCatalogueRead）分开：
// 目录上列检索列、不读快照，这一口反过来只读一行、必读快照。
//
// 不在册答 false 不答 error，别的租户的卡同答 false（越权探针与真不存在不可分辨，ADR-0029）。快照按另一套规范化
// 版本折装、本构建重建不了时交回包着 domain.ErrCanonicalizationVersionUnsupported 的错误——「册上有、本构建读不了」
// 与「册上没有」续办不同，调用方按 errors.Is 分格。引用的种类不是 pricing-plan 是调用方的错，答 error。
type PriceCardContentRead interface {
	ReadPriceCardContent(ctx context.Context, tenant domain.TenantID, reference domain.VersionReference) (PriceCardContent, bool, error)
}
