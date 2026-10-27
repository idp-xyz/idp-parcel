package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfnode "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/nodeoperations"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 随袋走过出发的成员后来才识别成功时的出发门禁补问的派发装配（ADR-0331）：TF 消费节点作业的节点收寄形成信封，按信封键重读收寄，认出了
// 包裹就问节点作业这件实物曾被装进哪几件单元，在那些单元有参与的段上按段列出在册出发，凡出发那一刻这件包裹正随单元走的，按原出发时点只替它
// 再问一次关务、追加一版读数并交一封补问信封；全程追踪消费那封补问信封（raiseDepartureGateSignalOnReaskConsumer），为补问读数里阻断的
// 那一件形成出发门禁阻断信号。
//
// 读数那一半与交接、揽收上的补问是同一份接法（departureGateReader），也就是 cmd/parcel-api 的 buildDepartureGateReader 那一份。

// departureGateMemberReasks 把成员补问编排接到给定的关务那一口上。这一口之外的接线只有这一处：生产装配交真关务，真库用例交按对象答格的替身。
func departureGateMemberReasks(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
	customs tfports.DepartureGateView,
) (*tfapplication.ReaskDepartureGatesOnIdentifiedMemberHandler, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask receptions: %w", err)
	}
	holders, err := nopostgres.NewConsolidationUnitsHoldingMember(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask unit holders: %w", err)
	}
	identified, err := tfnode.NewIdentifiedMemberView(receptions, holders)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask identified member view: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask segments: %w", err)
	}
	facts, err := tfpostgres.NewMovementFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask movement facts: %w", err)
	}
	reasks, err := tfpostgres.NewDepartureGateReasks(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask registry: %w", err)
	}
	handoff, err := tfpostgres.NewOutboxDepartureGateReaskHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask handoff: %w", err)
	}
	reader, err := departureGateReader(db, customs)
	if err != nil {
		return nil, err
	}
	handler, err := tfapplication.NewReaskDepartureGatesOnIdentifiedMemberHandler(tfapplication.ReaskDepartureGatesOnIdentifiedMemberDeps{
		Identified: identified,
		Segments:   segments,
		Facts:      facts,
		Reasks:     reasks,
		Gate:       reader,
		Handoff:    handoff,
		Clock:      clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask departure gates: %w", err)
	}
	return handler, nil
}

// departureGateReaskOnNodeIntakeConsumerOver 装节点收寄形成信封上的成员补问那一扇门，接到给定的关务那一口上。
func departureGateReaskOnNodeIntakeConsumerOver(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	outboxStore *outbox.Store,
	clock systemClock,
	customs tfports.DepartureGateView,
) (dispatch.Consumer, error) {
	handler, err := departureGateMemberReasks(db, outboxStore, clock, customs)
	if err != nil {
		return nil, err
	}
	consumer, err := tfinbox.NewDepartureGateReaskOnNodeIntakeConsumer(db.Transactor(), inboxStore, handler)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: member reask departure gates on node intake consumer: %w", err)
	}
	return consumer, nil
}

// departureGateReaskOnNodeIntakeConsumer 是生产装配：关务那一口接真判断。
func departureGateReaskOnNodeIntakeConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	outboxStore *outbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	customs, err := departureGateCustomsView(db, clock)
	if err != nil {
		return nil, err
	}
	return departureGateReaskOnNodeIntakeConsumerOver(db, inboxStore, outboxStore, clock, customs)
}
