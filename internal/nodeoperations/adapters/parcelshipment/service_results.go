// Package parcelshipment 是 node-operations 消费 parcel-shipment 的适配器（ADR-0025 消费方侧），两件：按外部标识核对
// 正式包裹身份（ADR-0184，见 ParcelIdentityView），与读当前服务结果（票 operator-channel/25，见 ServiceResultView）。
// 它只翻译不判断：哪些包裹被这个号码标识、有没有已成立的取消决定、有没有当前有效终局，都由 PS 自己的读口答；收寄
// 编排再据候选数分走向、据服务结果落标记（UC-NO-002 第 4、7 步）。
//
// 包裹按既有约定译：NO 的 ParcelAssociationReference 的串就是 PS 的声明包裹标识——parcel-shipment 那一侧消费节点收寄的
// 适配器（adapters/nodeoperations）用的是同一约定，两边一个口径。
package parcelshipment

import (
	"context"
	"errors"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致：输入在提供方那边立不起来。
var ErrUntranslatableAnswer = errors.New("node operations parcelshipment adapter: untranslatable answer")

// ServiceResultView 读 PS 的取消决定与当前有效终局。
type ServiceResultView struct {
	cancellations psports.ParcelCancellationView
	finals        psports.CurrentFinalView
}

var _ noports.ParcelServiceResultView = (*ServiceResultView)(nil)

// NewServiceResultView 构造期拒缺件：漏装的读口到第一次身份唯一的收寄才暴露，会被当成一次依赖故障。
func NewServiceResultView(cancellations psports.ParcelCancellationView, finals psports.CurrentFinalView) (*ServiceResultView, error) {
	if cancellations == nil || finals == nil {
		return nil, errors.New("node operations parcelshipment adapter: service result view needs a cancellation view and a current final view")
	}
	return &ServiceResultView{cancellations: cancellations, finals: finals}, nil
}

// FindParcelServiceResults 交回这件包裹此刻的取消与终局。终局只认已定案的那一份：在册而未定案的采用判断（被拒的那一份）
// 不是终局。任一读口读不动原样交回——「没查到」不能译成「没有限制」。
func (view *ServiceResultView) FindParcelServiceResults(
	ctx context.Context,
	tenant nodomain.TenantID,
	parcel nodomain.ParcelAssociationReference,
) (noports.ParcelServiceResults, error) {
	none := noports.ParcelServiceResults{}
	psTenant, err := psdomain.NewTenantID(tenant.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	psParcel, err := psdomain.NewDeclaredParcelID(parcel.String())
	if err != nil {
		return none, fmt.Errorf("%w: parcel association: %v", ErrUntranslatableAnswer, err)
	}
	cancellation, cancelled, err := view.cancellations.FindCancellation(ctx, psTenant, psParcel)
	if err != nil {
		return none, fmt.Errorf("node operations parcelshipment adapter: find cancellation: %w", err)
	}
	final, found, err := view.finals.FindCurrentFinal(ctx, psTenant, psParcel)
	if err != nil {
		return none, fmt.Errorf("node operations parcelshipment adapter: find current final: %w", err)
	}
	results := none
	if cancelled {
		results.Cancellation = cancellation.ID().String()
	}
	if found && final.Finalized {
		results.FinalKind = final.Final.Kind().String()
		results.FinalVersion = final.Final.Version().String()
	}
	return results, nil
}
