package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	satransport "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/transportfulfillment"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// withdrawSupplierCostOnChargeOccurrenceInvalidatedConsumer 接 ADR-0232 决定七：TF 发生项已形成信封 → SA 消费门 → 按键回读 TF 发生项 →
// 是失效版本就在发生项锁下记下那一版已失效、撤下依它形成的各条成本链头。跨上下文翻译只在 SA 的 `adapters/transportfulfillment`。各口都接
// 真册：预期成本库同时是读口、登记面与失效记录。
func withdrawSupplierCostOnChargeOccurrenceInvalidatedConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: supplier expected costs for withdrawal: %w", err)
	}
	withdrawal, err := saapplication.NewWithdrawSupplierExpectedCostHandler(saapplication.WithdrawSupplierExpectedCostDeps{
		Registry:      costs,
		Costs:         costs,
		Invalidations: costs,
		Clock:         clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: withdraw supplier expected cost: %w", err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge occurrences for supplier cost withdrawal: %w", err)
	}
	processing, err := satransport.NewWithdrawOnChargeOccurrenceInvalidatedAdapter(occurrences, withdrawal)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: withdraw on charge occurrence invalidated: %w", err)
	}
	consumer, err := sainbox.NewChargeOccurrenceInvalidatedConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: supplier cost withdrawal consumer: %w", err)
	}
	return consumer, nil
}
