// Package identity 为 parcel-pricing 的标识签发端口提供生产实现。
//
// 签发内核共用 internal/platform/identity；本包只决定两件属本上下文的事：标识空间取什么前缀，以及签出的字符串
// 交给哪个领域构造函数。内核里已经写下的取舍不在这里复述。
//
// 评价标识此前一律由调用方带进来（HTTP 回放门、测试夹具、别的上下文的适配器）；「按评价请求形成评价」（票
// sa-cc/11）是 PP 第一条自己形成评价的生产路，标识归 PP 铸。取随机铸造而不取「从 SA 请求标识派生」：身份声明
// 不派生（ADR-0096 同一取向），幂等由回指那一格在库上的唯一约束守（迁移 0010），不由标识的拼法守；铸造不落库、
// 不占连接，编排在停于前几格时也不会烧掉一个可被推算业务量的连续号。
package identity

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
)

// evaluationPrefix 只为让评价标识在日志与工单里一眼分得开，不参与任何判断。不含分隔符、不含上下文名——领域侧的
// EvaluationID 已是独立 Go 类型，张冠李戴编译期就拦得住。全仓前缀唯一由 internal/architecture 的标识前缀门禁守。
const evaluationPrefix = "EVAL"

// EvaluationIdentities 实现 ports.EvaluationIdentityFactory。
type EvaluationIdentities struct {
	minter platformidentity.Minter
}

func NewEvaluationIdentities(options ...platformidentity.Option) (*EvaluationIdentities, error) {
	minter, err := platformidentity.NewMinter(evaluationPrefix, options...)
	if err != nil {
		return nil, fmt.Errorf("parcel pricing identity: %w", err)
	}
	return &EvaluationIdentities{minter: minter}, nil
}

var _ ports.EvaluationIdentityFactory = (*EvaluationIdentities)(nil)

// MintEvaluationID 签发一个新的评价标识。签不出来时如实上抛：编排把它答成 `未决` 并指名铸造口，拿一个可预测的
// 替代值顶上会撞号。
func (factory *EvaluationIdentities) MintEvaluationID(_ context.Context) (domain.EvaluationID, error) {
	minted, err := factory.minter.Next()
	if err != nil {
		return domain.EvaluationID{}, fmt.Errorf("mint evaluation ID: %w", err)
	}
	return domain.NewEvaluationID(minted)
}
