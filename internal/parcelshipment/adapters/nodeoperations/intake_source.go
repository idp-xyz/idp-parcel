// Package nodeoperations 是 parcel-shipment 消费 node-operations 节点收寄结果的适配器
// （ADR-0025 消费方侧）。它只翻译不判断：收寄结果译成来源采用命令，采不采用由 PS 的
// 采用编排回答。
package nodeoperations

import (
	"context"
	"errors"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致。
var ErrUntranslatableAnswer = errors.New("parcel shipment nodeoperations adapter: untranslatable answer")

// ErrUnidentifiedHandlingUnit 说明这份收寄还挂在待识别实物上：没有版本化包裹关联的
// 收寄真实存在、控制真实成立，但采用判断没有包裹身份可锚——识别成功形成新版本后再来。
// 它与翻译坏了分开：等识别是业务续办，不是修适配器。
var ErrUnidentifiedHandlingUnit = errors.New("parcel shipment nodeoperations adapter: the handling unit is not yet identified")

// TargetShipment 指名收寄要采用到的目标委托（来源身份 + 编号 + 基线版本，与采用编排的
// 双重指名一致）。定位映射（作业实物→委托）属接入编排的实例配置，本适配器只转交。
type TargetShipment struct {
	Identity          psdomain.SourceIdentity
	ShipmentRequestID psdomain.ShipmentRequestID
	SubmissionVersion psdomain.SubmissionVersionID
}

// NodeIntakeAdapter 把节点收寄结果交给 parcel-shipment 的采用编排。
type NodeIntakeAdapter struct {
	adopt *psapplication.AdoptNetworkIntakeHandler
}

func NewNodeIntakeAdapter(adopt *psapplication.AdoptNetworkIntakeHandler) *NodeIntakeAdapter {
	return &NodeIntakeAdapter{adopt: adopt}
}

// AdoptFromNodeIntake 翻译并转交一份节点收寄。待识别实物拒绝：采用判断锚在包裹身份上，
// 而正式包裹身份只能来自版本化关联——不猜、不拿作业实物标识冒充。
func (adapter *NodeIntakeAdapter) AdoptFromNodeIntake(
	ctx context.Context,
	intake nodomain.NodeIntake,
	target TargetShipment,
) (psapplication.AdoptNetworkIntakeResult, error) {
	spec, err := intakeSourceFor(intake)
	if err != nil {
		return psapplication.AdoptNetworkIntakeResult{}, err
	}
	return adapter.adopt.Handle(ctx, psapplication.AdoptNetworkIntakeCommand{
		Identity:          target.Identity,
		ShipmentRequestID: target.ShipmentRequestID,
		SubmissionVersion: target.SubmissionVersion,
		Source:            spec,
	})
}

// intakeSourceFor 逐维翻译：作业实物→来源对象、节点→收寄地点、接收证据→控制依据
// （节点收寄本身就是控制成立的业务结果，接收证据正是它的依据）、结果版本与实际接收
// 时间原样带过。包裹身份取自版本化关联。
func intakeSourceFor(intake nodomain.NodeIntake) (psdomain.IntakeSourceSpec, error) {
	association, identified := intake.Association()
	if !identified {
		return psdomain.IntakeSourceSpec{}, ErrUnidentifiedHandlingUnit
	}
	parcel, err := psdomain.NewDeclaredParcelID(association.String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: parcel association: %v", ErrUntranslatableAnswer, err)
	}
	object, err := psdomain.NewSourceObjectReference(intake.Unit().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: handling unit: %v", ErrUntranslatableAnswer, err)
	}
	place, err := psdomain.NewIntakePlaceReference(intake.Node().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: node: %v", ErrUntranslatableAnswer, err)
	}
	control, err := psdomain.NewIntakeControlReference("NODE-INTAKE/" + intake.Evidence().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: reception evidence: %v", ErrUntranslatableAnswer, err)
	}
	version, err := psdomain.NewSourceResultVersion(intake.Version().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: result version: %v", ErrUntranslatableAnswer, err)
	}
	return psdomain.IntakeSourceSpec{
		Kind:       psdomain.NodeIntakeSource,
		Object:     object,
		Parcel:     parcel,
		Place:      place,
		Control:    control,
		Version:    version,
		OccurredAt: intake.ReceivedAt(),
	}, nil
}
