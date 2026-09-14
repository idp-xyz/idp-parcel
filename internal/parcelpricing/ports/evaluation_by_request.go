package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// EvaluationByRequestView 按回指取评价：（租户、SA 评价请求引用）→ 为它形成的那一份评价（票 sa-cc/11 裁决 2
// 「随评价入册、读口可按回指取」）。
//
// 另立读口而不扩 EvaluationStore：扩写侧接口会拆全部写侧替身（理由同 catalogue_read 与 evaluation_read 两口）；
// 而且它回答的是另一问——EvaluationStore.FindByID 按评价标识守幂等，这一口按请求守「同一请求不形成第二份评价」。
// 同租户同请求至多一份由库上的部分唯一索引守（迁移 0010），所以答案是单份不是列表。
type EvaluationByRequestView interface {
	FindByRequestReference(
		ctx context.Context,
		tenant domain.TenantID,
		reference domain.EvaluationRequestReference,
	) (domain.PricingEvaluation, bool, error)
}
