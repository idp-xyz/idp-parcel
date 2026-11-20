package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrnodeoperations "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/nodeoperations"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
)

// reassessOnMeasurementChangeUndecidedSentinels 只给「实测变化 → NR 路由复核」这一路。与同一封信上结算重评那一路分开列，
// 也与收寄采用、权威交接两路的未决哨兵分开：各路各记各的 inbox 账，运维要分得出是哪一路停住。
//
// 不在名单里：nrnodeoperations.ErrUntranslatableAnswer（编程错误）、nrparcelshipment.ErrAcceptedRequestInconsistent
// 与 psdomain.ErrAmbiguousParcelTarget（重投都不会变）。集运单元、零件、多件、不是当前已接受包裹，以及复核答`未判`，
// 都入账，不是错误。
var reassessOnMeasurementChangeUndecidedSentinels = []error{
	nrnodeoperations.ErrMeasurementReassessmentUndecided,
	nrparcelshipment.ErrAcceptedParcelUnreadable,
}

func reassessOnMeasurementChangeConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	purpose nrdomain.ServicePurpose,
	clock systemClock,
) (dispatch.Consumer, error) {
	reassess, err := reassessRouteHandler(db, clock)
	if err != nil {
		return nil, err
	}
	return reassessOnMeasurementChangeConsumerOver(db, inboxStore, reassess, purpose)
}

func reassessOnMeasurementChangeConsumerOver(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	reassess *nrapplication.ReassessRouteHandler,
	purpose nrdomain.ServicePurpose,
) (dispatch.Consumer, error) {
	associations, err := nopostgres.NewParcelAssociations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: parcel associations for route reassessment on measurement change: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: shipment requests for measurement change: %w", err)
	}
	accepted, err := nrparcelshipment.NewAcceptedCarriedGeoSource(requests)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: accepted carried geo for measurement change: %w", err)
	}
	adapter, err := nrnodeoperations.NewReassessOnMeasurementChangeAdapter(associations, accepted, reassess, purpose)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: measurement change reassessment adapter: %w", err)
	}
	consumer, err := nrinbox.NewMeasurementChangedConsumer(db.Transactor(), inboxStore, adapter)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: measurement change reassessment consumer: %w", err)
	}
	return consumer, nil
}
