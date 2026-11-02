// Package docnumber 是运输履约取单据号的消费侧适配器（ADR-0025）：把本上下文的 ports.DocumentNumberIssuer 接到编号组件的
// 取号编排上（ADR-0355 决定四）。它只做两端的形状翻译，不判是否已用、不读规则册——那两件分别归本上下文的单据表与编号组件。
package docnumber

import (
	"context"
	"errors"
	"fmt"

	platform "go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// ErrUnexpectedAssignment 说明编号组件交回了本适配器认不出的答案，或说取到了号而号立不起来。
var ErrUnexpectedAssignment = errors.New("transport fulfillment docnumber: unexpected assignment")

// Assigner 是编号组件取号编排里本适配器用到的那一口（platform.Assigner 满足它）。
type Assigner interface {
	Assign(ctx context.Context, request platform.IssueRequest) (platform.Assignment, error)
}

// Issuer 实现 ports.DocumentNumberIssuer。
type Issuer struct {
	assigner Assigner
}

var _ ports.DocumentNumberIssuer = (*Issuer)(nil)

func NewIssuer(assigner Assigner) (*Issuer, error) {
	if assigner == nil {
		return nil, errors.New("transport fulfillment docnumber: assigner is nil")
	}
	return &Issuer{assigner: assigner}, nil
}

// IssueDocumentNumber 不传开立方业务时区：委托与订舱的类型声明都是「开立方没有业务时区」，传了就是编程错误
// （platform.ErrIssuerTimeZoneNotDeclared）。「是否已用」那一问原样转给调用方在同一事务里查自己的表；编号组件渲染出的号
// 立不成本上下文的单据号（例如超过字节界）时交错误，开立事务随之回滚——不答「已用」：那会把一个存不下的号说成撞号，
// 再烧掉一整段流水。
func (issuer *Issuer) IssueDocumentNumber(ctx context.Context, request ports.DocumentNumberRequest) (ports.DocumentNumberIssue, error) {
	if request.IsUsed == nil {
		return ports.DocumentNumberIssue{}, fmt.Errorf("%w: the used-number query is required", platform.ErrInvalidIssueRequest)
	}
	assignment, err := issuer.assigner.Assign(ctx, platform.IssueRequest{
		Tenant: request.TenantID.String(),
		Type:   request.Type,
		IsUsed: func(ctx context.Context, rendered string) (bool, error) {
			number, err := domain.NewDocumentNumber(rendered)
			if err != nil {
				return false, fmt.Errorf("%w: %w", ErrUnexpectedAssignment, err)
			}
			return request.IsUsed(ctx, number)
		},
	})
	if err != nil {
		return ports.DocumentNumberIssue{}, err
	}
	switch assignment.Outcome {
	case platform.NumberAssigned:
		number, err := domain.NewDocumentNumber(assignment.Number)
		if err != nil {
			return ports.DocumentNumberIssue{}, fmt.Errorf("%w: %w", ErrUnexpectedAssignment, err)
		}
		return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberIssued, Number: number}, nil
	case platform.NumberingTimeZoneNotConfigured:
		return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberTimeZoneNotConfigured}, nil
	case platform.NumberingRuleCollision:
		return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberRuleCollision}, nil
	default:
		return ports.DocumentNumberIssue{}, fmt.Errorf("%w: outcome %q", ErrUnexpectedAssignment, assignment.Outcome)
	}
}
