package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/adoptconsume"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/cancellationredrive"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
)

// cancellationNotUpheldRedriveUndecidedSentinels 是「取消被认定不成立」续办这条链登记的未决哨兵：目标委托还没落到已接受、收寄采用停在自己的未决、
// 终局采用停在自己的未决、面单终局判断的目标反查与判断编排停在依赖上——都是「等一个依赖」，重投会改变结果。面单终局判断的几格与面单交易那一路是同一张名单
// （同一只处理方核），不抄第二份。
//
// 不在名单里的几格，恢复动作各不相同，保持 publish_failed：
//   - adoptconsume.ErrAdoptionHandoffPending / finalconsume.ErrFinalHandoffPending——采用或终局已提交而发布意图没交出去，要运维查的是 outbox 下游。
//   - cancellationredrive.ErrRedriveUntranslatable——信封里译不出的维或被库面改坏的原料，重投不自愈，现场要查那一行为什么长成这样。
//   - adoptconsume.ErrUnexpectedAdoptionOutcome / finalconsume.ErrUnexpectedFinalOutcome——封闭集合外，静默入账等于替编排作判断。
var cancellationNotUpheldRedriveUndecidedSentinels = append(
	[]error{
		cancellationredrive.ErrRedriveTargetNotFound,
		adoptconsume.ErrAdoptionUndecided,
	},
	labelTransactionJudgmentUndecidedSentinels...,
)

// cancellationNotUpheldRedriveConsumer 接 code/76 那条线（ADR-0295 决定四）：运营对一份取消先后冲突决定「认定取消依据不成立」后，同笔交出的
// `parcel-shipment.parcel-cancellation.not-upheld` 信封 → 消费门 → 把被这份取消挡住的来源重新交回原来的编排。
//
// 三条被挡住的链各用自己已装好的那一只处理方，不另装第二份：收寄采用与终局采用用与它们各自消费门同形的真编排；面单终局判断用同一只处理方核；
// 目标委托用同一张当前已接受投影；首次有效收寄链尾用取消编排边界核验读的同一只 TF 适配器。锁用同一把包裹取消边界锁，进来先取一次，各条链再取是重入。
func cancellationNotUpheldRedriveConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	intake, err := networkIntakeAdoption(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	finals, err := parcelFinalAdoptionChain(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	label, err := labelFinalJudgmentCore(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	replays, err := pspostgres.NewCancellationBlockedReplays(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: cancellation blocked replays: %w", err)
	}
	pickupRegistry, err := tfpostgres.NewCarrierFirstEffectivePickups(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: carrier first effective pickups: %w", err)
	}
	pickups, err := pstf.NewCarrierPickupBoundaryAdapter(pickupRegistry)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: carrier pickup boundary: %w", err)
	}
	boundaryLocks, err := pspostgres.NewCancellationBoundaryLocks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: cancellation boundary locks: %w", err)
	}
	redriver, err := cancellationredrive.NewRedriver(replays, intake.requests, pickups, intake.handler, finals.handler, label, boundaryLocks)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: cancellation redriver: %w", err)
	}
	consumer, err := psinbox.NewCancellationNotUpheldConsumer(db.Transactor(), inboxStore, redriver)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: cancellation not upheld consumer: %w", err)
	}
	return consumer, nil
}
