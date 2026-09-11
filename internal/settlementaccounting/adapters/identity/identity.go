// Package identity 为 settlement-accounting 的标识签发端口提供生产实现。
//
// 签发内核共用 internal/platform/identity；本包只决定两件属本上下文的事：标识空间取什么前缀，以及签出的
// 字符串交给哪个领域构造函数。内核里已经写下的取舍不在这里复述（为什么取随机不取库序列、为什么用 base32、
// 为什么不收 ctx、为什么熵源必须可注入）。
//
// 评价请求的 ID 取随机铸造而不取库序列，还多一层本上下文自己的理由：编排在登记之前先铸 ID（application 的
// RequestBuyEvaluationHandler 先按自然键找先到者、再铸、再登），铸造不落库，不依赖事务也不占连接；一次
// 未能提交的请求也不会烧掉一个可被另一租户推算业务量的连续号。
package identity

import (
	"context"
	"fmt"

	platformidentity "go.idp.xyz/idp-parcel/internal/platform/identity"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// evaluationRequestPrefix 只为让评价请求 ID 在日志与工单里一眼分得开，不参与任何判断。不含分隔符、不含
// 上下文名——领域侧的 EvaluationRequestID 已是独立 Go 类型，张冠李戴编译期就拦得住。全仓前缀唯一由
// internal/architecture 的标识前缀门禁守。
const evaluationRequestPrefix = "EVREQ"

// EvaluationRequestIdentities 实现 ports.EvaluationRequestIdentityFactory（票 sa-cc/08 裁决 2：评价请求的身份
// 是铸造的 ID，自然键只守幂等）。
type EvaluationRequestIdentities struct {
	minter platformidentity.Minter
}

func NewEvaluationRequestIdentities(options ...platformidentity.Option) (*EvaluationRequestIdentities, error) {
	minter, err := platformidentity.NewMinter(evaluationRequestPrefix, options...)
	if err != nil {
		return nil, fmt.Errorf("settlement accounting identity: %w", err)
	}
	return &EvaluationRequestIdentities{minter: minter}, nil
}

var _ ports.EvaluationRequestIdentityFactory = (*EvaluationRequestIdentities)(nil)

// MintEvaluationRequestID 签发一个新的评价请求 ID。签不出来时如实上抛：编排把它答成`未决`并指名铸造口，
// 拿一个可预测的替代值顶上会撞号。
func (factory *EvaluationRequestIdentities) MintEvaluationRequestID(_ context.Context) (domain.EvaluationRequestID, error) {
	minted, err := factory.minter.Next()
	if err != nil {
		return domain.EvaluationRequestID{}, fmt.Errorf("mint evaluation request ID: %w", err)
	}
	return domain.NewEvaluationRequestID(minted)
}
