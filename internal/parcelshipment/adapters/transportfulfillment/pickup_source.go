// Package transportfulfillment 是 parcel-shipment 消费 transport-fulfillment 场外揽收
// 结果的适配器（ADR-0025 消费方侧）。它只翻译不判断：揽收结果译成来源采用命令，采不
// 采用由 PS 的采用编排回答。
package transportfulfillment

import (
	"context"
	"errors"
	"fmt"

	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// ErrUntranslatableAnswer 语义与其他消费方适配器的同名哨兵一致。
var ErrUntranslatableAnswer = errors.New("parcel shipment transportfulfillment adapter: untranslatable answer")

// TargetShipment 指名揽收要采用到的目标委托，与节点收寄适配器同形。
type TargetShipment struct {
	Identity          psdomain.SourceIdentity
	ShipmentRequestID psdomain.ShipmentRequestID
	SubmissionVersion psdomain.SubmissionVersionID
}

// NetworkIntakeCommandHandler 是采用编排在本适配器侧的窄口。真实装配交给
// *application.AdoptNetworkIntakeHandler；测试用替身只记命令，不把应用层接口扩宽。
type NetworkIntakeCommandHandler interface {
	Handle(
		ctx context.Context,
		command psapplication.AdoptNetworkIntakeCommand,
	) (psapplication.AdoptNetworkIntakeResult, error)
}

// OffsitePickupAdapter 把场外揽收结果交给 parcel-shipment 的采用编排。
type OffsitePickupAdapter struct {
	adopt NetworkIntakeCommandHandler
}

func NewOffsitePickupAdapter(adopt NetworkIntakeCommandHandler) *OffsitePickupAdapter {
	return &OffsitePickupAdapter{adopt: adopt}
}

// AdoptFromOffsitePickup 翻译并转交一份场外揽收。TF 的揽收结果本就按对象逐一成立，
// 这里不虚构节点到站，收寄地点是实际接货位置。
//
// 前置条件：载运对象已被调用方确认为包裹身份。TF 的载运对象引用可能指集运单元，本层
// 无从分辨，因此调用方（AdoptOnOffsitePickupAdapter）先用包裹投影唯一命中，命不中的
// 对象停在它那一格；这里不再猜集运单元的成员。
func (adapter *OffsitePickupAdapter) AdoptFromOffsitePickup(
	ctx context.Context,
	pickup tfdomain.OffsitePickup,
	target TargetShipment,
) (psapplication.AdoptNetworkIntakeResult, error) {
	spec, err := pickupSourceFor(pickup)
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

// pickupSourceFor 逐维翻译：载运对象→包裹与来源对象（前置条件见 AdoptFromOffsitePickup）、
// 实际接货位置→收寄地点、运输控制依据→控制依据、结果版本与实际接货时间原样带过。
func pickupSourceFor(pickup tfdomain.OffsitePickup) (psdomain.IntakeSourceSpec, error) {
	parcel, err := psdomain.NewDeclaredParcelID(pickup.Object().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	object, err := psdomain.NewSourceObjectReference(pickup.Object().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: carried object: %v", ErrUntranslatableAnswer, err)
	}
	place, err := psdomain.NewIntakePlaceReference(pickup.Place().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: pickup place: %v", ErrUntranslatableAnswer, err)
	}
	control, err := psdomain.NewIntakeControlReference("OFFSITE-PICKUP/" + pickup.Control().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: transport control: %v", ErrUntranslatableAnswer, err)
	}
	version, err := psdomain.NewSourceResultVersion(pickup.Version().String())
	if err != nil {
		return psdomain.IntakeSourceSpec{}, fmt.Errorf("%w: result version: %v", ErrUntranslatableAnswer, err)
	}
	return psdomain.IntakeSourceSpec{
		Kind:       psdomain.OffsitePickupSource,
		Object:     object,
		Parcel:     parcel,
		Place:      place,
		Control:    control,
		Version:    version,
		OccurredAt: pickup.OccurredAt(),
	}, nil
}
