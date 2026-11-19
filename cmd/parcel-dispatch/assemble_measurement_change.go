package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	saidentity "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/identity"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sanodeops "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/nodeoperations"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	satransport "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/transportfulfillment"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// reevaluateOnMeasurementChangeConsumer 装出实测变化的收件箱消费者：销售与采购两路重评都挂在它转交的处理方上，
// 先后次序见 measurementChangeProcessing。
func reevaluateOnMeasurementChangeConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	associations, err := nopostgres.NewParcelAssociations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: parcel associations for measurement change: %w", err)
	}
	requests, err := sapostgres.NewSellEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation requests for measurement change: %w", err)
	}
	identities, err := saidentity.NewSellEvaluationRequestIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation request identities for measurement change: %w", err)
	}
	handoff, err := sapostgres.NewOutboxSellEvaluationRequestHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation request handoff for measurement change: %w", err)
	}
	sell, err := saapplication.NewReevaluateSellOnMeasurementChangeHandler(
		requests, requests, requests, identities, handoff, clock,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reevaluate sell on measurement change: %w", err)
	}
	sellProcessing, err := sanodeops.NewReevaluateOnMeasurementChange(associations, sell)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: measurement change adapter: %w", err)
	}
	buyRequests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation requests for measurement change: %w", err)
	}
	buyIdentities, err := saidentity.NewEvaluationRequestIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation request identities for measurement change: %w", err)
	}
	buyHandoff, err := sapostgres.NewOutboxEvaluationRequestHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation request handoff for measurement change: %w", err)
	}
	buy, err := saapplication.NewReevaluateBuyOnMeasurementChangeHandler(
		buyRequests, buyRequests, buyRequests, buyRequests, buyIdentities, buyHandoff, clock,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reevaluate buy on measurement change: %w", err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge occurrences for measurement change: %w", err)
	}
	buyProcessing, err := satransport.NewReevaluateBuyForMeasuredParcel(associations, occurrences, buy)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy measurement change adapter: %w", err)
	}
	consumer, err := sainbox.NewMeasurementChangedConsumer(db.Transactor(), inboxStore, measurementChangeProcessing{
		sell: sellProcessing, buy: buyProcessing,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: measurement changed consumer: %w", err)
	}
	return consumer, nil
}

// measurementChangeProcessing 让同一封实测变化先另起销售评价，再为载有这件包裹的发生项另起采购评价。
type measurementChangeProcessing struct {
	sell *sanodeops.ReevaluateOnMeasurementChange
	buy  *satransport.ReevaluateBuyForMeasuredParcel
}

func (processing measurementChangeProcessing) HandleChangedMeasurement(
	ctx context.Context,
	changed sainbox.ChangedMeasurement,
) error {
	if err := processing.sell.HandleChangedMeasurement(ctx, changed); err != nil {
		return err
	}
	return processing.buy.HandleChangedMeasurement(ctx, changed)
}
