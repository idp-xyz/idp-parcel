package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfparcelshipment "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
)

// postIntakeDispositionJourneyUndecidedSentinels 只有一格：旅程册读不动或意图没交出去，册子恢复后重投会落旅程。不在名单里的是依据种类不对、
// 译出的命令被编排拒收或来源冲突——重投不会变，保持 publish_failed 让现场去查那份信封。
var postIntakeDispositionJourneyUndecidedSentinels = []error{
	tfparcelshipment.ErrPostIntakeJourneyUndecided,
}

// startJourneyOnPostIntakeDispositionConsumer 接 PS 处置决定 → TF 另起旅程。处理方与运营决定口（parcel-api）是同一只 StartAlternateJourneyHandler，
// 缝全接真：旅程册、监管来路的处置执行意图、异常链意图。消费门已开事务，旅程与意图同笔落地，这里不再包事务。
func startJourneyOnPostIntakeDispositionConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	journeys, err := tfpostgres.NewAlternateJourneys(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: alternate journeys: %w", err)
	}
	disposition, err := tfpostgres.NewOutboxDispositionExecutionHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: disposition execution handoff: %w", err)
	}
	exception, err := tfpostgres.NewOutboxExceptionJourneyHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: exception journey handoff: %w", err)
	}
	starter := tfapplication.NewStartAlternateJourneyHandler(tfapplication.StartAlternateJourneyDeps{
		Journeys:    journeys,
		Disposition: disposition,
		Exception:   exception,
		Clock:       clock,
	})
	processing, err := tfparcelshipment.NewStartJourneyOnPostIntakeDisposition(starter, journeys)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: start journey on post-intake disposition: %w", err)
	}
	consumer, err := tfinbox.NewPostIntakeDispositionConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: post-intake disposition consumer: %w", err)
	}
	return consumer, nil
}
