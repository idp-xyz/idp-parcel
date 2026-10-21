package main

import (
	"fmt"
	"log/slog"
	"os"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
)

// actualFulfillmentCatchUpOnAdoptionUndecidedSentinels 只给「采用门铃 → TF 实际履约补形成」这一路（ADR-0231 决定一）。补形成停住的是自动
// 形成的同一个后半，可续办的那一格同是 ErrActualFulfillmentFormationUndecided；名单仍跟着路分开列，日后一路多一格未决不会顺带宣布到别的路上。
//
// 确定性错误都不进名单（tfinbox.ErrActualFulfillmentFormationDefect 与编排上抛的编程缺陷）——重投不会变。编排认得的数据缺陷不上抛，
// 记进未形成册、那一件入账，也轮不到名单。
var actualFulfillmentCatchUpOnAdoptionUndecidedSentinels = []error{
	tfinbox.ErrActualFulfillmentFormationUndecided,
}

// actualFulfillmentCatchUpOnRevisionUndecidedSentinels 只给「换版门铃 → TF 实际履约补形成」这一路（ADR-0231 决定五）。不在名单里的同上。
var actualFulfillmentCatchUpOnRevisionUndecidedSentinels = []error{
	tfinbox.ErrActualFulfillmentFormationUndecided,
}

// catchUpActualFulfillmentOnAdoptionConsumer 接 TF 采用门铃 → TF 实际履约补形成。补形成与自动形成是同一只编排（productionActualFulfillmentFormer）：
// 同一个登记用例、同一组登记册、同一本未形成册。
func catchUpActualFulfillmentOnAdoptionConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	former, err := productionActualFulfillmentFormer(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	consumer, err := tfinbox.NewActualFulfillmentCatchUpOnAdoptionConsumer(db.Transactor(), inboxStore, former, catchUpDefectLogger())
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: actual fulfillment catch-up on adoption consumer: %w", err)
	}
	return consumer, nil
}

// catchUpActualFulfillmentOnRevisionConsumer 接 TF 换版门铃 → TF 实际履约补形成。
func catchUpActualFulfillmentOnRevisionConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	former, err := productionActualFulfillmentFormer(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	consumer, err := tfinbox.NewActualFulfillmentCatchUpOnRevisionConsumer(db.Transactor(), inboxStore, former, catchUpDefectLogger())
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: actual fulfillment catch-up on revision consumer: %w", err)
	}
	return consumer, nil
}

// catchUpDefectLogger 是补形成消费门写数据缺陷日志的那一只。dispatchSources 不带 logger，路由登记处又不归特性文件改（ADR-0222），
// 所以照进程日志同一形状（main 里构造的那一只：JSON 写到标准输出）另建一只，写进同一条流。main 日后改了形状这里不会跟着
// （ADR-0231 越权风险点 6）。
func catchUpDefectLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}
