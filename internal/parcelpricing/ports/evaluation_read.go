package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是计价评价登记册的伴生列表读端口（票 admin-skeleton-closure-batch/03，读面
// 形状照 ADR-0077 通例逐字成立）：管理台 pricing-evaluation 页的供数面。不拓宽既有
// 写口（EvaluationStore）——扩写侧接口会拆全部写侧测试替身，伴生读端口另立，理由与
// 本包 catalogue_read 两口同句。
//
// 上列的是**检索列面**的照实转写：评价的权威内容在领域折装的快照里（snapshot 列），
// 读回要经领域整图重验含摘要自校——那是 EvaluationStore.FindByID 装载的纪律；目录
// 上列不重建领域对象、不形成判断，列面与登记字段对齐，不发明列，快照不透出。评价
// 对象、方向、金额等语义细节都住在快照内，属详情读法（届时按装载纪律另立），本口
// 不从 jsonb 里抠字段冒充列。
//
// 租户在方法签名上（ADR-0077 Decision 五）；Limit 必须为正，每页多大由接入面按渠道
// 契约裁决，读口只拒绝无意义的取值；空登记册如实交回空列表（Decision 四：空册本身
// 就是内容——评价的写入方是评价编排，编排在接入渠道墙后面，册空是墙拦不是缺陷）。

// EvaluationCatalogueRow 是计价评价登记册上列的一行：一次已落册评价的检索列面。
// 状态封闭五格（COMPLETED/PENDING/CONFLICT/FAILED/UNRATABLE，迁移 CHECK 钉住）；
// 语义摘要供冒名比对、方案内容摘要与规范化版本供重放可比性判断（ADR-0014），
// 三样都是登记册汇总好的检索列，照登转写。
type EvaluationCatalogueRow struct {
	EvaluationID      string
	Status            string
	SemanticDigest    string
	PlanContentDigest string
	Canonicalization  string
	RecordedAt        time.Time
}

// EvaluationCatalogueRead 是计价评价登记册的伴生列表读端口。
type EvaluationCatalogueRead interface {
	ListEvaluations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]EvaluationCatalogueRow, error)
}
