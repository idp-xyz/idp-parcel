package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// OperatorDecisionTarget 是运营操作者对一份委托作决定时，服务端按租户与委托标识查出的那份委托：来源身份
// 与当前提交版本（ADR-0151 决定二「委托寻址」）。操作者信封里只有租户与提交操作者，客户账户、来源与来源请求键
// 都不从请求体收。
type OperatorDecisionTarget struct {
	Identity       domain.SourceIdentity
	CurrentVersion domain.SubmissionVersionID
}

// OperatorDecisionTargets 按（租户、委托标识）找一份委托。
//
// 它不复用 ShipmentRequestViews：那一口的作用域按可见客户账户切（授权查询作用域至少一个账户），答的是
// 「客户侧这个人能看哪些委托」；运营决定的可见范围是整个租户，拿一组账户去凑等于造一个作用域。
// 查不到答 found=false 而不返回 error：空与读不动是两件事，调用方据此分别答「与越权探针同答」与依赖故障。
type OperatorDecisionTargets interface {
	FindOperatorDecisionTarget(ctx context.Context, tenant domain.TenantID, requestID domain.ShipmentRequestID) (OperatorDecisionTarget, bool, error)
}
