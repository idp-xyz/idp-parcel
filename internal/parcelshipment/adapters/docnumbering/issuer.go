// Package docnumbering 是 parcel-shipment 对平台编号组件的消费侧适配器（ADR-0025；ADR-0355 决定四）：把「给这份委托取
// 一个`SHIPMENT`的号」翻译成取号编排的一次请求，再把编排的答案翻译回本上下文的三格。它只翻译，不判断——撞号要不要回滚、
// 时区未配置记什么，都由提交编排定。
package docnumbering

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
)

// ErrUntranslatableAssignment 表示取号编排交回了一格本适配器没有落点的答案。上抛而不落进任何一格：封闭集合外的取值没有
// 恢复动作可派（ADR-0025「翻译必须是全函数」）。
var ErrUntranslatableAssignment = errors.New("parcel shipment document numbering: untranslatable assignment outcome")

// assigner 是取号编排那一个口，生产实现是 docnumber.Assigner。
type assigner interface {
	Assign(ctx context.Context, request docnumber.IssueRequest) (docnumber.Assignment, error)
}

// usedNumbers 是判撞要的那一个查询，生产实现是 pspostgres.ShipmentRequests：在开立事务里查本租户的委托是否已用这个号。
type usedNumbers interface {
	ShipmentDocumentNumberUsed(ctx context.Context, tenant domain.TenantID, number string) (bool, error)
}

// Issuer 实现 ports.ShipmentDocumentNumberIssuer。
type Issuer struct {
	assigner assigner
	used     usedNumbers
}

var _ ports.ShipmentDocumentNumberIssuer = Issuer{}

// NewIssuer 装配委托的取号适配器。取号编排与判撞查询必须接在开立事务所用的同一个库上，这一点由装配点保证，本函数只拒缺件。
func NewIssuer(assigner assigner, used usedNumbers) (Issuer, error) {
	if assigner == nil || used == nil {
		return Issuer{}, errors.New("parcel shipment document numbering: assigner and used-number query are required")
	}
	return Issuer{assigner: assigner, used: used}, nil
}

// IssueShipmentDocumentNumber 在调用方的开立事务里取一个委托单据号。不交开立方业务时区：委托的类型声明为「开立方没有业务
// 时区」（domain.ShipmentDocumentNumberType），日期按租户的单据编号时区。
func (issuer Issuer) IssueShipmentDocumentNumber(ctx context.Context, tenant domain.TenantID) (ports.ShipmentDocumentNumberIssue, error) {
	assignment, err := issuer.assigner.Assign(ctx, docnumber.IssueRequest{
		Tenant: tenant.String(),
		Type:   domain.ShipmentDocumentNumberType.Code,
		IsUsed: func(ctx context.Context, number string) (bool, error) {
			return issuer.used.ShipmentDocumentNumberUsed(ctx, tenant, number)
		},
	})
	if err != nil {
		return ports.ShipmentDocumentNumberIssue{}, err
	}
	switch assignment.Outcome {
	case docnumber.NumberAssigned:
		number, err := domain.NewShipmentDocumentNumber(assignment.Number)
		if err != nil {
			return ports.ShipmentDocumentNumberIssue{}, fmt.Errorf("parcel shipment document numbering: %w", err)
		}
		return ports.ShipmentDocumentNumberIssue{Outcome: ports.ShipmentDocumentNumberAssigned, Number: number}, nil
	case docnumber.NumberingTimeZoneNotConfigured:
		return ports.ShipmentDocumentNumberIssue{Outcome: ports.ShipmentDocumentNumberingTimeZoneNotConfigured}, nil
	case docnumber.NumberingRuleCollision:
		return ports.ShipmentDocumentNumberIssue{Outcome: ports.ShipmentDocumentNumberRuleCollision}, nil
	}
	return ports.ShipmentDocumentNumberIssue{}, fmt.Errorf("%w: %q", ErrUntranslatableAssignment, assignment.Outcome)
}
