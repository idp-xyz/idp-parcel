package main

import (
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	satransport "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/transportfulfillment"
)

// TF 运输收费发生项已形成 → SA 撤下依失效那一版形成的供应商预期成本（ADR-0232 决定七）。与发起 BUY 评价那一路消费同一类信封，名次排在
// 它之后：那一路对失效版本只入账、不发起，两路互不依赖，排先后只为扇出次序不由文件名定。
func init() {
	registerRoutes(func(sources dispatchSources) ([]routeLeg, error) {
		withdrawals, err := withdrawSupplierCostOnChargeOccurrenceInvalidatedConsumer(sources.db, sources.inbox, sources.clock)
		if err != nil {
			return nil, err
		}
		return []routeLeg{
			{
				event: sainbox.ChargeOccurrenceFormedEventType, position: positionBilling + 10, consumer: withdrawals,
				sentinels: chargeOccurrenceInvalidatedUndecidedSentinels, label: "supplier cost withdrawal on charge occurrence invalidated",
			},
		}, nil
	})
}

// chargeOccurrenceInvalidatedUndecidedSentinels 是这一路的未决哨兵，恢复动作都是等依赖、重投：
//   - satransport.ErrChargeOccurrenceNotVisible——信封所指的发生项还读不回，或登记册自己答不出。
//   - satransport.ErrSupplierCostWithdrawalUndecided——撤下编排停在预期成本库或失效记录上。
//
// 不在名单里的：入账定局的各格（撤下了、重放、没有依那一版形成的成本、不是失效版本）处理方答 nil；回读与键对不上、词汇表外、本包译出的
// 命令被拒、派生的失效纠错版本上已有别的内容，重投不自愈，落 publish_failed 等人看。
var chargeOccurrenceInvalidatedUndecidedSentinels = []error{
	satransport.ErrChargeOccurrenceNotVisible,
	satransport.ErrSupplierCostWithdrawalUndecided,
}
