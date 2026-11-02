// Package documentnumber 把编号组件的取号编排接到本上下文的消费侧端口 ports.DocumentNumberAssigner 上（ADR-0025；
// ADR-0355 决定四）。只翻译，不判断：编号组件三格答案逐格落到端口的三格，取号与判撞的规则都在编号组件那一处。
package documentnumber

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// Issuer 是本适配器对编号组件的最小依赖，*docnumber.Assigner 满足它。
type Issuer interface {
	Assign(ctx context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error)
}

// Assigner 实现 ports.DocumentNumberAssigner。
type Assigner struct {
	issuer Issuer
}

var _ ports.DocumentNumberAssigner = (*Assigner)(nil)

func NewAssigner(issuer Issuer) (*Assigner, error) {
	if issuer == nil {
		return nil, errors.New("settlement documentnumber: assigner needs the document numbering issuer")
	}
	return &Assigner{issuer: issuer}, nil
}

// AssignDocumentNumber 在调用方的开立事务里取号。编号组件交回错误照原样上抛：事务外、密钥被撤、序列写不进都是要回滚
// 开立事务的故障，不译成业务答案。
func (assigner *Assigner) AssignDocumentNumber(
	ctx context.Context,
	request ports.DocumentNumberRequest,
) (ports.DocumentNumberAssignment, error) {
	assigned, err := assigner.issuer.Assign(ctx, docnumber.IssueRequest{
		Tenant:         request.Tenant.String(),
		Type:           request.Type,
		IssuerTimeZone: request.IssuerTimeZone,
		IsUsed:         request.IsUsed,
	})
	if err != nil {
		return ports.DocumentNumberAssignment{}, err
	}
	switch assigned.Outcome {
	case docnumber.NumberAssigned:
		return ports.DocumentNumberAssignment{Outcome: ports.DocumentNumberAssigned, Number: assigned.Number}, nil
	case docnumber.NumberingTimeZoneNotConfigured:
		return ports.DocumentNumberAssignment{Outcome: ports.DocumentNumberTimeZoneUnconfigured}, nil
	case docnumber.NumberingRuleCollision:
		return ports.DocumentNumberAssignment{Outcome: ports.DocumentNumberRuleCollision}, nil
	}
	// 编号组件将来多出一格，这里不悄悄落到哪一格上：响亮报错，由改它的人给新格定落点（ADR-0025 全函数翻译）。
	return ports.DocumentNumberAssignment{}, fmt.Errorf("settlement documentnumber: unknown assignment outcome %q", assigned.Outcome)
}
