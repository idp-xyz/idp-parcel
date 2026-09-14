package ports

import (
	"context"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// EvaluationIdentityFactory 为本上下文自己形成的评价签发标识。
//
// 此前评价标识一律由调用方带进来（HTTP 回放门、测试夹具、别的上下文的适配器），PP 没有任何生产路径自己
// 形成评价，也就不需要签发。「按评价请求形成评价」（票 sa-cc/11）是第一条：评价是 PP 的事实，标识归 PP 铸，
// 取铸造而不取「从 SA 请求标识派生」——身份声明不派生（ADR-0096 同一取向），幂等由回指那一格的唯一约束守
// （迁移 0010），不由标识的拼法守。
type EvaluationIdentityFactory interface {
	MintEvaluationID(ctx context.Context) (domain.EvaluationID, error)
}
