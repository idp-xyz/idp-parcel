package customscompliance

import (
	"context"
	"fmt"

	ccapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	ccdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// HandoverGateView 实现 tfports.HandoverGateView：交接登记时以交出意图问一件对象（ADR-0213 决定四）。与 DepartureGateView
// 共用同一口判断与同一份答复对照，只有意图不同。
type HandoverGateView struct {
	judge ActionAdmissionJudge
}

// NewHandoverGateView 判断口必须在，理由同 NewDepartureGateView。
func NewHandoverGateView(judge ActionAdmissionJudge) (*HandoverGateView, error) {
	if judge == nil {
		return nil, fmt.Errorf("transport fulfillment customscompliance adapter: action admission judge is nil")
	}
	return &HandoverGateView{judge: judge}, nil
}

var _ tfports.HandoverGateView = (*HandoverGateView)(nil)

// ReadHandoverGate 以交出意图问一件对象：at 取交出节点，onward 取去向节点，没声明去向就不带——关务据此不列跨关务区域
// 那一道（ADR-0206 决定二），本包不替登记方补一个去向。对象引用的译法与出发同一条（袋或集运单元由关务按自己的规则作答）。
func (view *HandoverGateView) ReadHandoverGate(
	ctx context.Context,
	query tfports.HandoverGateQuery,
) (tfdomain.GateFinding, error) {
	command, err := handoverAdmissionCommandFor(query)
	if err != nil {
		return tfdomain.GateFinding{}, err
	}
	result, err := view.judge.Handle(ctx, command)
	if err != nil {
		return tfdomain.GateFinding{}, fmt.Errorf("judge action admission: customs compliance: %w", err)
	}
	return findingFromResult(query.Object, result)
}

// handoverAdmissionCommandFor 把交出问询译成关务的命令。登记方声明的节点在关务那边立不起来都归译不回来：那一件记读不动、
// 交接照登，依据种类指向两边的口径，而不是关务进程。去向与交出节点相同的那一版编排不来问
// （domain.TransportHandover.UnaskedGateReading）；仍问到这里，关务的交出意图拒它，同归译不回来。
func handoverAdmissionCommandFor(query tfports.HandoverGateQuery) (ccapp.JudgeActionAdmissionCommand, error) {
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
		return none, fmt.Errorf("%w: releasing node: %v", ErrUntranslatableAnswer, err)
	}
	var onward ccdomain.NetworkNodeReference
	if query.Onward.String() != "" {
		if onward, err = ccdomain.NewNetworkNodeReference(query.Onward.String()); err != nil {
			return none, fmt.Errorf("%w: onward node: %v", ErrUntranslatableAnswer, err)
		}
	}
	intent, err := ccdomain.NewHandoverIntent(at, onward)
	if err != nil {
		return none, fmt.Errorf("%w: handover intent: %v", ErrUntranslatableAnswer, err)
	}
	return ccapp.JudgeActionAdmissionCommand{TenantID: tenant, Object: object, Intent: intent, At: query.At}, nil
}
