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

// 运输履约的场外揽收、有效交付、外部承运轨迹与权威交接各一路 VE 投影的消费者构造。各路在派生之前先问节点作业对象那一刻是不是
// 集运单元，是就入账不派生；问的那一口由 projectionConsolidationMembers 装，与移动事实那一路同一种。

// derivePickupConsumer 接对象级揽收登记 → VE 投影。与 PS 采用分 inbox 名。
func derivePickupConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	registrations, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection pickup registrations: %w", err)
	}
	members, err := projectionConsolidationMembers(db)
	if err != nil {
		return nil, err
	}
	processing, err := vetf.NewDeriveOnOffsitePickupAdapter(registrations, members, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on offsite pickup: %w", err)
	}
	consumer, err := veinbox.NewOffsitePickupConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive pickup consumer: %w", err)
	}
	return consumer, nil
}

// deriveDeliveryConsumer 接有效交付登记 → VE 投影。与 PS 终局分 inbox 名。
func deriveDeliveryConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection deliveries: %w", err)
	}
	members, err := projectionConsolidationMembers(db)
	if err != nil {
		return nil, err
	}
	processing, err := vetf.NewDeriveOnEffectiveDeliveryAdapter(deliveries, members, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on effective delivery: %w", err)
	}
	consumer, err := veinbox.NewEffectiveDeliveryConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive delivery consumer: %w", err)
	}
	return consumer, nil
}

// deriveExternalTrackingConsumer 接 TF 外部承运轨迹事实（有效时间已判断的版本）→ VE 投影。
// 本路不接 PS：外部轨迹是来源事实，不是有效交付，也不构成终局（TF CONTEXT「外部承运轨迹事实」）。
func deriveExternalTrackingConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	facts, err := tfpostgres.NewExternalTrackingFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection external tracking facts: %w", err)
	}
	members, err := projectionConsolidationMembers(db)
	if err != nil {
		return nil, err
	}
	processing, err := vetf.NewDeriveOnExternalCarrierTrackingAdapter(facts, members, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on external carrier tracking: %w", err)
	}
	consumer, err := veinbox.NewExternalTrackingConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive external tracking consumer: %w", err)
	}
	return consumer, nil
}

// deriveHandoverConsumer 接 TF 权威交接登记 → VE 投影。本路不接 PS：终局只认有效交付。
func deriveHandoverConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection transport handovers: %w", err)
	}
	members, err := projectionConsolidationMembers(db)
	if err != nil {
		return nil, err
	}
	processing, err := vetf.NewDeriveOnTransportHandoverAdapter(handovers, members, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on transport handover: %w", err)
	}
	consumer, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive handover consumer: %w", err)
	}
	return consumer, nil
}
