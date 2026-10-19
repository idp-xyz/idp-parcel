package customscompliance

import (
	"context"
	"fmt"

	ccapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// DeliveryGateView 实现 tfports.DeliveryGateView：有效交付登记时以交付意图问一件对象（ADR-0213 决定五）。与出发、交出
// 共用同一口判断与同一份答复对照，只有意图不同。
type DeliveryGateView struct {
	judge ActionAdmissionJudge
}

// NewDeliveryGateView 判断口必须在，理由同 NewDepartureGateView。
func NewDeliveryGateView(judge ActionAdmissionJudge) (*DeliveryGateView, error) {
	if judge == nil {
		return nil, fmt.Errorf("transport fulfillment customscompliance adapter: action admission judge is nil")
	}
	return &DeliveryGateView{judge: judge}, nil
}

var _ tfports.DeliveryGateView = (*DeliveryGateView)(nil)

// ReadDeliveryGate 以交付意图问一件对象：at 取编排交来的节点（对象进入这次交付所在那一段时那次交接的交出节点）。交付意图
// 没有去向，关务据此不列跨关务区域那一道（ADR-0206 决定二）。对象引用的译法与出发同一条。
func (view *DeliveryGateView) ReadDeliveryGate(
	ctx context.Context,
	query tfports.DeliveryGateQuery,
) (tfdomain.GateFinding, error) {
	command, err := deliveryAdmissionCommandFor(query)
	if err != nil {
		return tfdomain.GateFinding{}, err
	}
	result, err := view.judge.Handle(ctx, command)
	if err != nil {
		return tfdomain.GateFinding{}, fmt.Errorf("judge action admission: customs compliance: %w", err)
	}
	return findingFromResult(query.Object, result)
}

// deliveryAdmissionCommandFor 把交付问询译成关务的命令。交接登记方声明的节点在关务那边立不起来归译不回来：那一件记读
// 不动、交付照登，依据种类指向两边的口径，而不是关务进程。
func deliveryAdmissionCommandFor(query tfports.DeliveryGateQuery) (ccapp.JudgeActionAdmissionCommand, error) {
	none := ccapp.JudgeActionAdmissionCommand{}
	tenant, err := ccdomain.NewTenantID(query.TenantID.String())
	if err != nil {
		return none, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	object, err := ccdomain.NewDeclaredParcelReference(query.Object.String())
	if err != nil {
		return none, fmt.Errorf("%w: object: %v", ErrUntranslatableAnswer, err)
	}
	at, err := ccdomain.NewNetworkNodeReference(query.Node.String())
	if err != nil {
		return none, fmt.Errorf("%w: delivery node: %v", ErrUntranslatableAnswer, err)
	}
	intent, err := ccdomain.NewDeliveryIntent(at)
	if err != nil {
		return none, fmt.Errorf("%w: delivery intent: %v", ErrUntranslatableAnswer, err)
	}
	return ccapp.JudgeActionAdmissionCommand{TenantID: tenant, Object: object, Intent: intent, At: query.At}, nil
}
