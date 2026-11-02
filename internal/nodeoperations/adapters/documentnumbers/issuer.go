// Package documentnumbers 是 node-operations 消费编号技术组件取号编排的一侧（ADR-0025 消费方侧；ADR-0355 决定四）：把本上下文的
// 取号请求译成 docnumber.IssueRequest，把取号答案译回 ports.DocumentNumberIssue。只翻译不判断：哪个时区、撞没撞、跳几个都是取号
// 编排的事，本包一样都不替它定。
package documentnumbers

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

// Assigner 是取号编排的那一个方法，按 *docnumber.Assigner 原形取。只声明它而不收具体类型：测试与别的装配可以换一只时钟可控的
// 编排进来，生产上就是组合根装出的那一只（cmd/internal/docnumbering.Components.Assigner）。
type Assigner interface {
	Assign(ctx context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error)
}

// Issuer 实现 ports.DocumentNumberIssuer。
type Issuer struct {
	assigner Assigner
}

var _ ports.DocumentNumberIssuer = (*Issuer)(nil)

// NewIssuer 拒缺件：漏装的编排到第一张单据才暴露，会被当成一次依赖故障。
func NewIssuer(assigner Assigner) (*Issuer, error) {
	if assigner == nil {
		return nil, errors.New("node operations document numbers: assigner is nil")
	}
	return &Issuer{assigner: assigner}, nil
}

// IssueDocumentNumber 取一个号。编排交回 error 原样交回（事务外、类型没声明、时区读不动……），调用方回滚开立事务。
func (issuer *Issuer) IssueDocumentNumber(ctx context.Context, request ports.DocumentNumberRequest) (ports.DocumentNumberIssue, error) {
	if request.IsUsed == nil {
		return ports.DocumentNumberIssue{}, errors.New("node operations document numbers: the used-number query is required")
	}
	assignment, err := issuer.assigner.Assign(ctx, docnumber.IssueRequest{
		Tenant:         request.Tenant.String(),
		Type:           request.Type,
		IssuerTimeZone: request.IssuerTimeZone,
		IsUsed:         request.IsUsed,
	})
	if err != nil {
		return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s document number: %w", request.Type, err)
	}
	switch assignment.Outcome {
	case docnumber.NumberAssigned:
		number, err := domain.NewDocumentNumber(assignment.Number)
		if err != nil {
			return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s document number: %w", request.Type, err)
		}
		return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberIssued, Number: number}, nil
	case docnumber.NumberingTimeZoneNotConfigured:
		return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberTimeZoneNotConfigured}, nil
	case docnumber.NumberingRuleCollision:
		return ports.DocumentNumberIssue{Outcome: ports.DocumentNumberRuleCollision}, nil
	default:
		// 编排只该答上面三格；答别的格是实现坏了，不是业务答案。
		return ports.DocumentNumberIssue{}, fmt.Errorf("issue %s document number: unexpected outcome %q", request.Type, assignment.Outcome)
	}
}
