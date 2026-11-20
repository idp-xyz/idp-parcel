package nodeoperations

import (
	"context"
	"errors"
	"fmt"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// ErrRePreparationUndecided 说明这次按实测再准备停在未决，或还列不出作业实物上的包裹。过一会儿可能就好，进本路自己的
// 未决名单。与初始路由那一路的哨兵分开：两路等的依赖不同。
var ErrRePreparationUndecided = errors.New(
	"transport fulfillment nodeoperations adapter: re-preparation on measurement change is undecided")

// MeasurementChangePreparation 是实测变化入口。真实装配接 PrepareOnPlannedLegsHandler。
type MeasurementChangePreparation interface {
	RePrepareOnMeasurementChange(
		ctx context.Context,
		command tfapplication.RePrepareOnMeasurementChangeCommand,
	) (tfapplication.RePrepareOnMeasurementChangeResult, error)
}

// RePrepareOnMeasurementChange 把一次实测变化解成一件包裹，交给再准备。只认作业实物上此刻恰好一件包裹；
// 集运单元、零件与多件入账收工。口径与路由那一路的 measuredParcel 相同，不引用路由的包。
type RePrepareOnMeasurementChange struct {
	associations noports.CurrentParcelAssociation
	prepare      MeasurementChangePreparation
}

func NewRePrepareOnMeasurementChange(
	associations noports.CurrentParcelAssociation,
	prepare MeasurementChangePreparation,
) (*RePrepareOnMeasurementChange, error) {
	if associations == nil || prepare == nil {
		return nil, fmt.Errorf("transport fulfillment nodeoperations adapter: measurement change re-preparation is missing a dependency")
	}
	return &RePrepareOnMeasurementChange{associations: associations, prepare: prepare}, nil
}

var _ tfinbox.ChangedMeasurementHandler = (*RePrepareOnMeasurementChange)(nil)

func (adapter *RePrepareOnMeasurementChange) HandleChangedMeasurement(
	ctx context.Context,
	changed tfinbox.ChangedMeasurement,
) error {
	parcel, proceed, err := parcelOnMeasuredUnit(ctx, adapter.associations, changed)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	tenant, err := tfdomain.NewTenantID(changed.TenantID)
	if err != nil {
		return fmt.Errorf("transport fulfillment: measurement change tenant: %w", err)
	}
	result, err := adapter.prepare.RePrepareOnMeasurementChange(ctx, tfapplication.RePrepareOnMeasurementChangeCommand{
		TenantID: tenant, Object: parcel.String(),
	})
	if err != nil {
		return err
	}
	return reprepareConsumption(result)
}

// parcelOnMeasuredUnit 从一次实测变化解出它对上的那一件包裹。集运单元上的测量对不上哪一件包裹。
// 作业实物此刻没有关联、或关联着不止一件，同样不在其中挑一件猜。
func parcelOnMeasuredUnit(
	ctx context.Context,
	associations noports.CurrentParcelAssociation,
	changed tfinbox.ChangedMeasurement,
) (nodomain.ParcelAssociationReference, bool, error) {
	if changed.ObjectKind != nodomain.HandlingUnitObject.String() {
		return nodomain.ParcelAssociationReference{}, false, nil
	}
	tenant, err := nodomain.NewTenantID(changed.TenantID)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, fmt.Errorf("transport fulfillment: measurement change tenant: %w", err)
	}
	unit, err := nodomain.NewHandlingUnitID(changed.ObjectID)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, fmt.Errorf("transport fulfillment: measurement change handling unit: %w", err)
	}
	parcels, err := associations.ListParcels(ctx, tenant, unit)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, fmt.Errorf("%w: list parcels of handling unit %q: %v",
			ErrRePreparationUndecided, changed.ObjectID, err)
	}
	if len(parcels) != 1 {
		return nodomain.ParcelAssociationReference{}, false, nil
	}
	return parcels[0], true, nil
}

func reprepareConsumption(result tfapplication.RePrepareOnMeasurementChangeResult) error {
	switch result.Outcome() {
	case tfapplication.RePrepareRunAnswered,
		tfapplication.RePrepareRunNoMeasurement,
		tfapplication.RePrepareRunMeasurementUndetermined,
		tfapplication.RePrepareRunNoDeclaration,
		tfapplication.RePrepareRunWindowClosed:
		return nil
	case tfapplication.RePrepareRunUndecided:
		return fmt.Errorf("%w: %s (%s)", ErrRePreparationUndecided, result.UndecidedReason(), result.ContinuationReference())
	case tfapplication.RePrepareRunNotAccepted:
		return fmt.Errorf("transport fulfillment: the parcel resolved from the measurement change was refused")
	default:
		return fmt.Errorf("transport fulfillment: unexpected re-preparation outcome %q", result.Outcome())
	}
}
