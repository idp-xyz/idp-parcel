package nodeoperations

import (
	"context"
	"fmt"
	"time"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ConsolidationMembershipLookup 是本适配器向 node-operations 问「这一件那一刻是不是集运单元」的窄口。真实装配交给
// nodeoperations/adapters/postgres.ConsolidationMembershipsAt，理由同 ParcelContainmentLookup。
type ConsolidationMembershipLookup interface {
	ReadConsolidationMembershipAt(
		ctx context.Context,
		tenant nodomain.TenantID,
		object string,
		at time.Time,
	) (noports.ConsolidationMembershipAt, error)
}

// ConsolidationUnitView 实现 psports.ConsolidationUnitView：只翻译节点作业单元册对「这一件是不是单元」的答复，不另立判据。
// 单元册按实例存在判单元，同串的包裹号也按单元认（ADR-0214 决定二、ADR-0225 决定四）；成员是谁、嵌套几层，这里不关心，
// 本上下文不把任何东西投到成员上。
type ConsolidationUnitView struct {
	memberships ConsolidationMembershipLookup
}

func NewConsolidationUnitView(memberships ConsolidationMembershipLookup) (ConsolidationUnitView, error) {
	if memberships == nil {
		return ConsolidationUnitView{}, fmt.Errorf("parcel shipment nodeoperations adapter: consolidation membership lookup is nil")
	}
	return ConsolidationUnitView{memberships: memberships}, nil
}

var _ psports.ConsolidationUnitView = ConsolidationUnitView{}

// IsConsolidationUnitAt 把（租户，包裹号，时点）译成节点作业的问法再译回一个是否。
//
// 节点作业的答复里「不是单元」时成员恒空（ConsolidationMembershipAt 的契约）；答出不是单元却带着成员，是册上或翻译自相矛盾，
// 重问还是同一个结果，包住 ErrConsolidationUnitAnswerUntranslatable。读不动原样交回，让调用方分到续办一格。
func (view ConsolidationUnitView) IsConsolidationUnitAt(
	ctx context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
	at time.Time,
) (bool, error) {
	noTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return false, fmt.Errorf("%w: tenant: %v", psports.ErrConsolidationUnitAnswerUntranslatable, err)
	}
	membership, err := view.memberships.ReadConsolidationMembershipAt(ctx, noTenant, parcel.String(), at)
	if err != nil {
		return false, fmt.Errorf("read consolidation unit: node operations: %w", err)
	}
	if !membership.Unit && len(membership.Members) > 0 {
		return false, fmt.Errorf("%w: object %q is answered as no unit yet carries %d members",
			psports.ErrConsolidationUnitAnswerUntranslatable, parcel, len(membership.Members))
	}
	return membership.Unit, nil
}
