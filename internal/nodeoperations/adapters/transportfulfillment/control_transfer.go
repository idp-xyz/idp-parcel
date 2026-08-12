// Package transportfulfillment 是 node-operations 消费 transport-fulfillment 权威
// 运输交接结果的适配器（ADR-0025 消费方侧）。它只翻译不判断：`已交接`译成控制转出、
// 已拒收与待确认译不出任何东西——两边的硬句在这里对上（TF：拒收/待确认不转出控制；
// NO：转出唯一依据是已交接结果）。
package transportfulfillment

import (
	"errors"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致。
var ErrUntranslatableAnswer = errors.New("node operations transportfulfillment adapter: untranslatable answer")

// ErrHandoverDoesNotTransfer 说明这份交接结果是已拒收或待确认：控制不转出，节点继续
// 保持控制责任。它与翻译坏了分开——不转出是提供方的业务答案，不是本适配器的故障。
var ErrHandoverDoesNotTransfer = errors.New("node operations transportfulfillment adapter: this handover does not transfer control")

// ControlTransferAdapter 把权威交接的`已交接`结果应用到节点实物控制上。
type ControlTransferAdapter struct{}

func NewControlTransferAdapter() *ControlTransferAdapter {
	return &ControlTransferAdapter{}
}

// TransferOut 依据一份权威交接结果转出一件实物的节点控制。转出时刻取交接判断的业务
// 时间；已转出的控制不可逆与时刻不得早于成立由 NO 领域承担，这里不复述。
func (adapter *ControlTransferAdapter) TransferOut(
	control nodomain.PhysicalControl,
	handover tfdomain.TransportHandover,
) (nodomain.PhysicalControl, error) {
	basis, transfers := handover.TransferOutBasis()
	if !transfers {
		return nodomain.PhysicalControl{}, ErrHandoverDoesNotTransfer
	}
	if handover.Object().String() != control.Unit().String() ||
		handover.TenantID().String() != control.TenantID().String() {
		// 交接对象与控制对象不是同一件实物：拿别的对象的交接转出这件的控制，部分
		// 交接的逐对象纪律就破了。
		return nodomain.PhysicalControl{}, fmt.Errorf(
			"%w: handover object %q does not match controlled unit %q",
			ErrUntranslatableAnswer, handover.Object(), control.Unit())
	}
	reference, err := nodomain.NewTransferOutReference(basis)
	if err != nil {
		return nodomain.PhysicalControl{}, fmt.Errorf("%w: transfer basis: %v", ErrUntranslatableAnswer, err)
	}
	transferred, err := control.TransferOut(reference, handover.JudgedAt())
	if err != nil {
		return nodomain.PhysicalControl{}, fmt.Errorf("transfer out node control: %w", err)
	}
	return transferred, nil
}
