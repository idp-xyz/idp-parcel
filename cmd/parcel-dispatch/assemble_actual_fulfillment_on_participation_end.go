package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	tfinbox "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/inbox"
	tfpartycommercial "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/partycommercial"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapplication "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// actualFulfillmentOnHandoverUndecidedSentinels 只给「交接 → TF 实际履约发生项自动形成」这一路（ADR-0218）。与同一封信上
// VE 投影、派送发起两路的名单分开列：各包各的哨兵再 FanOut，本路会停在采用连线、旅程册、委托、协议核对口与发生项登记册上，
// 那几格别路没有。
//
// 不在名单里：tfinbox.ErrActualFulfillmentFormationDefect（自家键过不了自家门、登记用例答出本路不该见的格）、
// tfapplication.ErrParticipationEndedInSeveralSegments（库面不一致）、domain.ErrAmbiguousJourney（旅程册不一致）、
// tfapplication.ErrAdoptionWithoutCommission（连线指名的委托不在册）——重投都不会变。未配置、不计费、缺采购上下文都不在任何
// 名单里：它们入账不重投，不是错误。
var actualFulfillmentOnHandoverUndecidedSentinels = []error{
	tfinbox.ErrActualFulfillmentFormationUndecided,
}

// actualFulfillmentOnDeliveryUndecidedSentinels 只给「有效交付 → TF 实际履约发生项自动形成」这一路。与交接那一路内容相同
// 也分开列：两路挂在两张扇出上，名单跟着路走，日后一路多一格未决不会顺带宣布到另一路上。不在名单里的同上。
var actualFulfillmentOnDeliveryUndecidedSentinels = []error{
	tfinbox.ErrActualFulfillmentFormationUndecided,
}

// formActualFulfillmentOnHandoverConsumer 接 TF 权威交接登记 → TF 实际履约发生项自动形成（ADR-0218）。
func formActualFulfillmentOnHandoverConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	former, err := productionActualFulfillmentFormer(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	consumer, err := tfinbox.NewActualFulfillmentOnHandoverConsumer(db.Transactor(), inboxStore, former)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: actual fulfillment on handover consumer: %w", err)
	}
	return consumer, nil
}

// formActualFulfillmentOnDeliveryConsumer 接 TF 有效交付登记 → TF 实际履约发生项自动形成（ADR-0218）。
func formActualFulfillmentOnDeliveryConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	former, err := productionActualFulfillmentFormer(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	consumer, err := tfinbox.NewActualFulfillmentOnDeliveryConsumer(db.Transactor(), inboxStore, former)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: actual fulfillment on delivery consumer: %w", err)
	}
	return consumer, nil
}

// productionActualFulfillmentFormer 给协议核对口接 PC 的真点读口，其余交 actualFulfillmentFormerOver。
func productionActualFulfillmentFormer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
) (*tfapplication.FormActualFulfillmentOnParticipationEndHandler, error) {
	agreementVersions, err := pcpostgres.NewSupplierAgreementContents(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: supplier agreement versions for actual fulfillment: %w", err)
	}
	agreements, err := tfpartycommercial.NewSupplierAgreementSource(agreementVersions)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: supplier agreement check for actual fulfillment: %w", err)
	}
	return actualFulfillmentFormerOver(db, outboxStore, clock, agreements)
}

// actualFulfillmentFormerOver 把自动形成编排连同它调的登记用例接到给定的协议核对口上。协议核对口之外的接线只有这一处：生产
// 装配交真适配器，真库用例交替身，两边的段、采用、委托、旅程、发生项登记册与 Outbox 交出是同一份接法。
//
// 登记用例照 cmd/parcel-api 的 chargeOccurrenceDeps 另接一份，不跨 main 包复用（理由同 deliveryDispatchOnHandoverConsumer
// 头注）。两份接法要一致：人工登记与自动形成调的是同一个用例、读同一组登记册，同一身份才互相答`已存在`。揽收尝试、派送尝试、
// 订舱与承运答复四口本路用不到（只登实际履约），照接，免得登记用例在本路上出现与人工那一路不同的缺口。
//
// 不另包事务：消费门已在 inbox 那一笔事务里调处理方，发生项、已形成信封与 inbox 的已处理同笔提交；同一 ctx 带着同一笔。
func actualFulfillmentFormerOver(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock tfports.Clock,
	agreements tfports.SupplierAgreementSource,
) (*tfapplication.FormActualFulfillmentOnParticipationEndHandler, error) {
	pickups, err := tfpostgres.NewPickupAttempts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: pickup attempts for actual fulfillment: %w", err)
	}
	deliveries, err := tfpostgres.NewDeliveryAttempts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery attempts for actual fulfillment: %w", err)
	}
	segments, err := tfpostgres.NewFulfillmentSegments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: fulfillment segments for actual fulfillment: %w", err)
	}
	judgments, err := tfpostgres.NewActualCarrierJudgments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: actual carrier judgments for actual fulfillment: %w", err)
	}
	adoptions, err := tfpostgres.NewSegmentCommissionAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: segment commission adoptions for actual fulfillment: %w", err)
	}
	commissions, err := tfpostgres.NewTransportCommissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: transport commissions for actual fulfillment: %w", err)
	}
	bookings, err := tfpostgres.NewBookingRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: booking requests for actual fulfillment: %w", err)
	}
	answers, err := tfpostgres.NewBookingAnswers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: booking answers for actual fulfillment: %w", err)
	}
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge occurrences for actual fulfillment: %w", err)
	}
	handoff, err := tfpostgres.NewOutboxChargeOccurrenceHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge occurrence handoff for actual fulfillment: %w", err)
	}
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: transport handovers for actual fulfillment: %w", err)
	}
	journeys, err := tfpostgres.NewAlternateJourneys(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: alternate journeys for actual fulfillment: %w", err)
	}
	registration := tfapplication.NewRegisterChargeOccurrenceHandler(tfapplication.RegisterChargeOccurrenceDeps{
		PickupAttempts:   pickups,
		DeliveryAttempts: deliveries,
		Segments:         segments,
		CarrierJudgments: judgments,
		Adoptions:        adoptions,
		Commissions:      commissions,
		Bookings:         bookings,
		BookingAnswers:   answers,
		Agreements:       agreements,
		Occurrences:      occurrences,
		Handoff:          handoff,
		Clock:            clock,
	})
	return tfapplication.NewFormActualFulfillmentOnParticipationEndHandler(tfapplication.FormActualFulfillmentOnParticipationEndDeps{
		Handovers:    handovers,
		Segments:     segments,
		Adoptions:    adoptions,
		Journeys:     journeys,
		Registration: registration,
	}), nil
}
