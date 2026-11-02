// Package docnumbering 把本上下文的建案取号端口（ports.CaseDocumentNumbers）接到编号技术组件的取号编排上
// （ADR-0025、ADR-0355 决定四）。编号组件只认类型代码、不读任何单据表；判撞要查的案件表由本上下文的案件库答，
// 在这里接到取号请求上。
package docnumbering

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

// Assigner 是编号组件的取号编排（docnumber.Assigner）。
type Assigner interface {
	Assign(ctx context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error)
}

// UsedCaseNumbers 答（租户，单据号）是否已被某个案件用过，由案件库在建案事务里答（postgres.CustomsCases）。
type UsedCaseNumbers interface {
	CaseNumberUsed(ctx context.Context, tenant domain.TenantID, number string) (bool, error)
}

// CaseNumbers 实现 ports.CaseDocumentNumbers。
type CaseNumbers struct {
	assigner Assigner
	used     UsedCaseNumbers
}

var _ ports.CaseDocumentNumbers = (*CaseNumbers)(nil)

func NewCaseNumbers(assigner Assigner, used UsedCaseNumbers) (*CaseNumbers, error) {
	if assigner == nil || used == nil {
		return nil, errors.New("customs compliance document numbering: assigner and used-number query are required")
	}
	return &CaseNumbers{assigner: assigner, used: used}, nil
}

// AssignCaseNumber 按 CUSTOMS_CASE 取一个号。不交开立方业务时区：类型声明说关务案件的开立方没有业务时区
// （domain.CustomsCaseDocumentNumberType），日期按租户的单据编号时区出；交了反倒是编程错误。
func (numbers *CaseNumbers) AssignCaseNumber(ctx context.Context, tenant domain.TenantID) (ports.CaseNumberAssignment, error) {
	assignment, err := numbers.assigner.Assign(ctx, docnumber.IssueRequest{
		Tenant: tenant.String(),
		Type:   domain.CustomsCaseDocumentNumberType.Code,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			return numbers.used.CaseNumberUsed(ctx, tenant, number)
		},
	})
	if err != nil {
		return ports.CaseNumberAssignment{}, fmt.Errorf("assign customs case number: %w", err)
	}
	switch assignment.Outcome {
	case docnumber.NumberAssigned:
		return ports.CaseNumberAssignment{Outcome: ports.CaseNumberAssigned, Number: assignment.Number}, nil
	case docnumber.NumberingTimeZoneNotConfigured:
		return ports.CaseNumberAssignment{Outcome: ports.CaseNumberingTimeZoneNotConfigured}, nil
	case docnumber.NumberingRuleCollision:
		return ports.CaseNumberAssignment{Outcome: ports.CaseNumberingRuleCollision}, nil
	default:
		return ports.CaseNumberAssignment{}, fmt.Errorf("assign customs case number: unknown outcome %q", assignment.Outcome)
	}
}
