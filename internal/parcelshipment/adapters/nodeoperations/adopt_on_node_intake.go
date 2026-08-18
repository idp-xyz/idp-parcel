package nodeoperations

import (
	"context"
	"errors"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/adoptconsume"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

var (
	// ErrReceptionNotVisible 表示按信封引用还读不回形成收寄，或读到的不是可采认的
	// 形成格。可见性滞后是续办，重投会改变结果；不当毒丸拒收。
	ErrReceptionNotVisible = errors.New(
		"parcel shipment nodeoperations adapter: node intake reception is not yet visible")
	// ErrParcelTargetNotFound 表示这个租户下当前没有可采认的已接受委托声明了该包裹。
	// 委托可能还没落到已接受，重投会改变结果。
	ErrParcelTargetNotFound = errors.New(
		"parcel shipment nodeoperations adapter: current accepted parcel target not found")
	// ErrAdoptionUndecided 与 ErrAdoptionHandoffPending 是 adoptconsume 同名哨兵的别名。
	// 两条采用消费链共用同一个错误值，生产名单与既有测试因此都指得住同一格；此处只保留
	// 本包的暴露口，语义与判断都在 adoptconsume。
	ErrAdoptionUndecided      = adoptconsume.ErrAdoptionUndecided
	ErrAdoptionHandoffPending = adoptconsume.ErrAdoptionHandoffPending
)

// ReceptionFinder 按收寄幂等键取回判断记录。由 NO 的 ReceptionStore 满足。
type ReceptionFinder interface {
	FindByKey(ctx context.Context, key noports.ReceptionKey) (noports.ReceptionRecord, bool, error)
}

// NodeIntakeAdopter 把已识别的节点收寄交给采用翻译。真实装配用 NodeIntakeAdapter。
type NodeIntakeAdopter interface {
	AdoptFromNodeIntake(
		ctx context.Context,
		intake nodomain.NodeIntake,
		target TargetShipment,
	) (psapplication.AdoptNetworkIntakeResult, error)
}

// AdoptOnNodeIntakeAdapter 是 psinbox.NodeIntakeConsumer 的真实处理方：按引用重读
// NO 收寄记录，用当前已接受投影反查目标委托，再交给 NodeIntakeAdapter。
//
// 跨上下文翻译留在消费方（ADR-0025）。本层不猜 latest，歧义原样上抛。
type AdoptOnNodeIntakeAdapter struct {
	receptions ReceptionFinder
	targets    psports.CurrentAcceptedParcelTargetView
	adopt      NodeIntakeAdopter
}

func NewAdoptOnNodeIntakeAdapter(
	receptions ReceptionFinder,
	targets psports.CurrentAcceptedParcelTargetView,
	adopt NodeIntakeAdopter,
) (*AdoptOnNodeIntakeAdapter, error) {
	if receptions == nil {
		return nil, fmt.Errorf("parcel shipment nodeoperations adapter: reception finder is nil")
	}
	if targets == nil {
		return nil, fmt.Errorf("parcel shipment nodeoperations adapter: parcel target view is nil")
	}
	if adopt == nil {
		return nil, fmt.Errorf("parcel shipment nodeoperations adapter: node intake adopter is nil")
	}
	return &AdoptOnNodeIntakeAdapter{receptions: receptions, targets: targets, adopt: adopt}, nil
}

var _ psinbox.FormedNodeIntakeHandler = (*AdoptOnNodeIntakeAdapter)(nil)

// HandleFormedNodeIntake 按信封引用取回收寄、反查当前已接受目标、转交采用。
//
// 租户从信封带到两次查询：NO FindByKey 与 PS 包裹反查用同一个租户字符串，避免信封
// 租户与记录租户各说各话。
func (adapter *AdoptOnNodeIntakeAdapter) HandleFormedNodeIntake(
	ctx context.Context,
	formed psinbox.FormedNodeIntake,
) error {
	noTenant, err := nodomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	record, found, err := adapter.receptions.FindByKey(ctx, noports.ReceptionKey{
		TenantID: noTenant,
		SourceID: formed.SourceID,
	})
	if err != nil {
		return fmt.Errorf("%w: %v", ErrReceptionNotVisible, err)
	}
	if !found || record.Kind != noports.RecordIntakeFormed {
		return fmt.Errorf("%w: source %q", ErrReceptionNotVisible, formed.SourceID)
	}

	association, identified := record.Intake.Association()
	if !identified {
		return ErrUnidentifiedHandlingUnit
	}
	psTenant, err := psdomain.NewTenantID(formed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	parcel, err := psdomain.NewDeclaredParcelID(association.String())
	if err != nil {
		return fmt.Errorf("%w: parcel association: %v", ErrUntranslatableAnswer, err)
	}

	target, found, err := adapter.targets.FindCurrentAcceptedByParcel(ctx, psTenant, parcel)
	if err != nil {
		return err
	}
	if !found {
		return fmt.Errorf("%w: parcel %q", ErrParcelTargetNotFound, parcel)
	}

	result, err := adapter.adopt.AdoptFromNodeIntake(ctx, record.Intake, TargetShipment{
		Identity:          target.Identity(),
		ShipmentRequestID: target.ShipmentRequestID(),
		SubmissionVersion: target.SubmissionVersion(),
	})
	if err != nil {
		return err
	}
	return adoptconsume.Consumption(result)
}
