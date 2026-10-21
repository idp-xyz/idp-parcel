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

// backfillMovementFactsOnHandoverConsumer 接 TF 权威交接登记 → VE 移动事实补派生（ADR-0224）。处理适配器按信封键重读交接，
// 在它让对象进的那一段上按段列出声明了这一段的移动事实，凡事实那一刻对象在控的就补派生——三本册都只读，权威事实留在
// transport-fulfillment。派生借投影那一份租户现绑的编排，与移动事实那一路同一个写入方。
func backfillMovementFactsOnHandoverConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill transport handovers: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill segments: %w", err)
	}
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts: %w", err)
	}
	processing, err := vetf.NewBackfillMovementFactsOnHandoverAdapter(handovers, segments, facts, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts on handover: %w", err)
	}
	consumer, err := veinbox.NewMovementFactBackfillOnHandoverConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: backfill movement facts consumer: %w", err)
	}
	return consumer, nil
}
