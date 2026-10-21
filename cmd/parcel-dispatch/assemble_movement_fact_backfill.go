package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
)

// backfillMovementFacts 装移动事实补派生的处理方（ADR-0224）：按信封键重读交接或对象级揽收，在它让对象进的那一段上按段列出
// 声明了这一段的移动事实，凡事实那一刻对象在控的就补派生——交接册、揽收登记册、段册与移动事实册都只读，权威事实留在
// transport-fulfillment。派生借投影那一份租户现绑的编排，与移动事实那一路同一个写入方。交接那一路与揽收那一路共用这一个处理方。
func backfillMovementFacts(db *bentopg.DB, derive *tenantBoundProjectionDerive) (*vetf.BackfillMovementFactsAdapter, error) {
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill transport handovers: %w", err)
	}
	pickups, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill pickup registrations: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill segments: %w", err)
	}
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts: %w", err)
	}
	processing, err := vetf.NewBackfillMovementFactsAdapter(handovers, pickups, segments, facts, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts: %w", err)
	}
	return processing, nil
}

// backfillMovementFactsOnHandoverConsumer 接 TF 权威交接登记 → VE 移动事实补派生。
func backfillMovementFactsOnHandoverConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	processing, err := backfillMovementFacts(db, derive)
	if err != nil {
		return nil, err
	}
	consumer, err := veinbox.NewMovementFactBackfillOnHandoverConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts on handover consumer: %w", err)
	}
	return consumer, nil
}

// backfillMovementFactsOnPickupConsumer 接 TF 对象级场外揽收登记 → VE 移动事实补派生。
func backfillMovementFactsOnPickupConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	processing, err := backfillMovementFacts(db, derive)
	if err != nil {
		return nil, err
	}
	consumer, err := veinbox.NewMovementFactBackfillOnOffsitePickupConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts on pickup consumer: %w", err)
	}
	return consumer, nil
}
