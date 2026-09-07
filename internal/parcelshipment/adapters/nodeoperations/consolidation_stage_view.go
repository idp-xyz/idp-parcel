package nodeoperations

import (
	"context"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ParcelContainmentLookup 是本适配器向 node-operations 取「该包裹此刻在不在集运单元里」的窄口。
// 真实装配交给 nodeoperations/adapters/postgres.ParcelContainmentView；理由同 ExecutionFactLookup。
type ParcelContainmentLookup interface {
	LoadParcelContainment(
		ctx context.Context,
		tenant nodomain.TenantID,
		parcel nodomain.ParcelAssociationReference,
	) (noports.ParcelContainment, error)
}

// ConsolidationStageView 读节点作业按包裹键交出的容纳三值，译成装袋一格的三态（票
// ps-port-remainder/05 接线的那一只；它替下的 UnconnectedConsolidationStageView 已随读面立起
// 退场）。
//
// 翻译是全函数：节点作业的封闭三值每个都有落点——`在`译`在`、`不在`译`不在`、`不可归属`译
// `不知道`。最后那一格是节点作业自己的解释（候选不是归属，它说不出袋里那件是不是这个包裹），
// 本适配器只把它搬进 PS 的三态，不替它判成任何一边。集外取值上抛：那是提供方长了新取值而本
// 处没跟上，静默映射成任何一格都会让提供方替消费方作了业务判断（ADR-0025）。
type ConsolidationStageView struct {
	containment ParcelContainmentLookup
}

func NewConsolidationStageView(containment ParcelContainmentLookup) (ConsolidationStageView, error) {
	if containment == nil {
		return ConsolidationStageView{}, fmt.Errorf("parcel shipment nodeoperations adapter: parcel containment lookup is nil")
	}
	return ConsolidationStageView{containment: containment}, nil
}

var _ psports.ConsolidationStageView = ConsolidationStageView{}

// LoadBaggingFact 把（租户 + 正式包裹）译成节点作业的键——正式包裹在节点作业侧就是那条版本化
// 关联的引用，与 adopt_on_node_intake 反向把关联引用当包裹标识用的是同一条约定——读三值再译。
func (view ConsolidationStageView) LoadBaggingFact(
	ctx context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psdomain.StageFact, error) {
	noTenant, err := nodomain.NewTenantID(tenant.String())
	if err != nil {
		return psdomain.StageFactUnknown, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	association, err := nodomain.NewParcelAssociationReference(parcel.String())
	if err != nil {
		return psdomain.StageFactUnknown, fmt.Errorf("%w: parcel association: %v", ErrUntranslatableAnswer, err)
	}
	containment, err := view.containment.LoadParcelContainment(ctx, noTenant, association)
	if err != nil {
		return psdomain.StageFactUnknown, fmt.Errorf("load bagging fact: %w", err)
	}
	switch containment {
	case noports.ParcelContained:
		return psdomain.StageFactPresent, nil
	case noports.ParcelNotContained:
		return psdomain.StageFactAbsent, nil
	case noports.ParcelContainmentUnattributable:
		return psdomain.StageFactUnknown, nil
	default:
		return psdomain.StageFactUnknown, fmt.Errorf("%w: parcel containment %d", ErrUntranslatableAnswer, containment)
	}
}
