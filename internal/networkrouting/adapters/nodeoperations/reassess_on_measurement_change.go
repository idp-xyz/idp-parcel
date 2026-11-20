// Package nodeoperations 是 network-routing 消费 node-operations 实际测量变化的适配器。只翻译不判断。
// 称重不是控制依据：本包不从测量里取位置，复核在最近一次已判控制触发的位置与业务时间上再判一次。
package nodeoperations

import (
	"context"
	"errors"
	"fmt"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nrports "go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

var (
	// ErrUntranslatableAnswer 是本包自己组出的触发过不了复核的门，或一侧交出了词汇表之外的内容。编程错误，不折成「不复核」。
	ErrUntranslatableAnswer = errors.New(
		"network routing nodeoperations adapter: untranslatable answer")
	// ErrMeasurementReassessmentUndecided 说明这次复核停在未决。与收寄采用、权威交接两路的同类哨兵分设：
	// 各条触发等的依赖不同，运维读到失败要分得出是哪一路。
	ErrMeasurementReassessmentUndecided = errors.New(
		"network routing nodeoperations adapter: route reassessment on measurement change is undecided")
)

// AcceptedCarriedGeoLookup 按（租户，包裹）答这件包裹是不是当前已接受包裹，并交回判断键各维与受理基线那一版的地理投影。
// 真实装配是 nrparcelshipment.AcceptedCarriedGeoSource。
type AcceptedCarriedGeoLookup interface {
	LoadAcceptedCarriedGeo(
		ctx context.Context,
		tenant nrdomain.TenantID,
		parcel nrdomain.DeclaredParcelID,
	) (nrparcelshipment.AcceptedCarriedGeo, bool, error)
}

// ReassessOnMeasurementChangeAdapter 是实测变化的处理方。只认作业实物上此刻恰好一件包裹的测量；
// 集运单元、零件与多件入账收工。口径与结算的 MeasuredParcel 相同，不引用结算的包。
type ReassessOnMeasurementChangeAdapter struct {
	associations noports.CurrentParcelAssociation
	accepted     AcceptedCarriedGeoLookup
	reassess     *nrapplication.ReassessRouteHandler
	purpose      nrdomain.ServicePurpose
}

func NewReassessOnMeasurementChangeAdapter(
	associations noports.CurrentParcelAssociation,
	accepted AcceptedCarriedGeoLookup,
	reassess *nrapplication.ReassessRouteHandler,
	purpose nrdomain.ServicePurpose,
) (*ReassessOnMeasurementChangeAdapter, error) {
	if associations == nil || accepted == nil || reassess == nil {
		return nil, fmt.Errorf("network routing nodeoperations adapter: measurement change adapter is missing a dependency")
	}
	return &ReassessOnMeasurementChangeAdapter{
		associations: associations,
		accepted:     accepted,
		reassess:     reassess,
		purpose:      purpose,
	}, nil
}

var _ nrinbox.ChangedActualMeasurementHandler = (*ReassessOnMeasurementChangeAdapter)(nil)

func (adapter *ReassessOnMeasurementChangeAdapter) HandleChangedActualMeasurement(
	ctx context.Context,
	changed nrinbox.ChangedActualMeasurement,
) error {
	if !adapter.purpose.Valid() {
		return fmt.Errorf("%w: service purpose is not configured", ErrUntranslatableAnswer)
	}
	parcel, proceed, err := measuredParcel(ctx, adapter.associations, changed)
	if err != nil {
		return err
	}
	if !proceed {
		return nil
	}
	tenant, err := nrdomain.NewTenantID(changed.TenantID)
	if err != nil {
		return fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	declared, err := nrdomain.NewDeclaredParcelID(parcel.String())
	if err != nil {
		return fmt.Errorf("%w: parcel: %v", ErrUntranslatableAnswer, err)
	}
	accepted, isAccepted, err := adapter.accepted.LoadAcceptedCarriedGeo(ctx, tenant, declared)
	if err != nil {
		return err
	}
	if !isAccepted {
		return nil
	}
	correlation, err := nrdomain.NewMeasurementChangeCorrelation(tenant, declared, changed.SourceID)
	if err != nil {
		return fmt.Errorf("%w: correlation: %v", ErrUntranslatableAnswer, err)
	}
	if _, err := nrdomain.NewRerouteTriggerReference(correlation.String()); err != nil {
		return fmt.Errorf("%w: reroute trigger reference: %v", ErrUntranslatableAnswer, err)
	}
	source, err := nrdomain.NewSourceFactVersionReference(changed.SourceID)
	if err != nil {
		return fmt.Errorf("%w: source: %v", ErrUntranslatableAnswer, err)
	}
	result, err := adapter.reassess.HandleMeasurementChange(ctx, nrapplication.RejudgeOnMeasurementChangeCommand{
		Correlation: correlation,
		Key: nrdomain.InitialRouteJudgmentKey{
			TenantID:           tenant,
			CustomerAccountID:  accepted.CustomerAccountID,
			ShipmentRequestID:  accepted.ShipmentRequestID,
			AcceptanceBaseline: accepted.AcceptanceBaseline,
			DeclaredParcelID:   declared,
			ServicePurpose:     adapter.purpose,
		},
		SourceVersion: source,
		OccurredAt:    changed.OccurredAt,
		AcceptedAt:    accepted.AcceptedAt,
		Carried:       nrports.RequestCarriedContent{Geo: accepted.Geo},
	})
	if err != nil {
		return fmt.Errorf("reassess route on measurement change: %w", err)
	}
	return reassessmentToConsumption(result)
}

// measuredParcel 从一次实测变化解出它对上的那一件包裹。只认作业实物；集运单元上的测量不对应哪一件包裹。
// 作业实物此刻关联着零件或多件包裹同样对不上——不在多件里挑一件猜。与结算的 MeasuredParcel 同一口径。
func measuredParcel(
	ctx context.Context,
	associations noports.CurrentParcelAssociation,
	changed nrinbox.ChangedActualMeasurement,
) (nodomain.ParcelAssociationReference, bool, error) {
	if changed.ObjectKind != nodomain.HandlingUnitObject.String() {
		return nodomain.ParcelAssociationReference{}, false, nil
	}
	tenant, err := nodomain.NewTenantID(changed.TenantID)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, fmt.Errorf("%w: tenant: %v", ErrUntranslatableAnswer, err)
	}
	unit, err := nodomain.NewHandlingUnitID(changed.ObjectID)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, fmt.Errorf("%w: handling unit: %v", ErrUntranslatableAnswer, err)
	}
	parcels, err := associations.ListParcels(ctx, tenant, unit)
	if err != nil {
		return nodomain.ParcelAssociationReference{}, false, fmt.Errorf("list parcels of handling unit %q: %w", changed.ObjectID, err)
	}
	if len(parcels) != 1 {
		return nodomain.ParcelAssociationReference{}, false, nil
	}
	return parcels[0], true, nil
}

func reassessmentToConsumption(result nrapplication.ReassessRouteResult) error {
	switch result.Outcome() {
	case nrapplication.ReassessedStillApplicable,
		nrapplication.ReassessedPlanLapsed,
		nrapplication.ReassessedFirstPlanFormed,
		nrapplication.ReassessedRerouted,
		nrapplication.ReassessExistingResult,
		nrapplication.ReassessTriggerConflict,
		nrapplication.ReassessTriggerNotJudged:
		return nil
	case nrapplication.ReassessUndecided:
		return fmt.Errorf("%w: %s", ErrMeasurementReassessmentUndecided, result.UndecidedReason())
	case nrapplication.ReassessTriggerNotAccepted:
		return fmt.Errorf("%w: the trigger built from the measurement change was refused", ErrUntranslatableAnswer)
	default:
		return fmt.Errorf("%w: unexpected reassessment outcome %q", ErrUntranslatableAnswer, result.Outcome())
	}
}
