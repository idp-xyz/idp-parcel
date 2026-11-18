package main

import (
	"context"
	"fmt"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
)

// transactionalMeasurement 把测量登记包进一笔事务。登记册的写口无事务即拒，事务边界归装配点。
type transactionalMeasurement struct {
	transactor bentoapp.Transactor
	inner      *nodeopsapp.RecordActualMeasurementHandler
}

func (measurement transactionalMeasurement) Handle(
	ctx context.Context,
	command nodeopsapp.RecordActualMeasurementCommand,
) (nodeopsapp.RecordActualMeasurementResult, error) {
	var result nodeopsapp.RecordActualMeasurementResult
	err := measurement.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := measurement.inner.Handle(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.RecordActualMeasurementResult{}, err
	}
	return result, nil
}

func (measurement transactionalMeasurement) Invalidate(
	ctx context.Context,
	command nodeopsapp.InvalidateActualMeasurementCommand,
) (nodeopsapp.InvalidateActualMeasurementResult, error) {
	var result nodeopsapp.InvalidateActualMeasurementResult
	err := measurement.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, handleErr := measurement.inner.Invalidate(txCtx, command)
		if handleErr != nil {
			return handleErr
		}
		result = handled
		return nil
	})
	if err != nil {
		return nodeopsapp.InvalidateActualMeasurementResult{}, err
	}
	return result, nil
}

func buildActualMeasurementOrchestration(db *bentopg.DB) (nodeopshttp.ActualMeasurementHandler, error) {
	store, err := nopostgres.NewActualMeasurements(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: actual measurements: %w", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: measurement change outbox: %w", err)
	}
	notice, err := nopostgres.NewOutboxMeasurementChangeNotice(db, outboxStore, systemClock{})
	if err != nil {
		return nil, fmt.Errorf("parcel-api: measurement change notice: %w", err)
	}
	handler, err := nodeopsapp.NewRecordActualMeasurementHandler(store, systemClock{}, notice)
	if err != nil {
		return nil, fmt.Errorf("parcel-api: record actual measurement: %w", err)
	}
	return transactionalMeasurement{
		transactor: db.Transactor(),
		inner:      handler,
	}, nil
}
