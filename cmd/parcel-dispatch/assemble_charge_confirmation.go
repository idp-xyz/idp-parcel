package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saparcelshipment "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelshipment"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// confirmationBasisOnFinalOutcomeUndecidedSentinels 是 PS 终局信封 → SA 记确认依据并按时点确认这一路的未决哨兵
// （ADR-0336 决定二）。终局读不回是可见性滞后；记依据或确认停在依赖上是续办。确认条件与确认事实没登是答案，处理方入账，
// 到不了这里。
var confirmationBasisOnFinalOutcomeUndecidedSentinels = []error{
	saparcelshipment.ErrFinalOutcomeNotVisible,
	saparcelshipment.ErrConfirmOnBasisUndecided,
}

// chargeConfirmationOnBasis 装「记依据并按时点确认」：确认编排本体与 parcel-api 的 buildSettlementOrchestrations 接的是
// 同一组册与同一只确认信封交接，时点门也是同一道——两个进程各装一份，答案不因从哪个进程进来而不同。
func chargeConfirmationOnBasis(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
) (*saapplication.ConfirmChargeOnBasisHandler, error) {
	charges, err := sapostgres.NewCustomerCharges(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer charges for confirmation: %w", err)
	}
	conditions, err := sapostgres.NewChargeConfirmationConditions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: confirmation conditions: %w", err)
	}
	catalogues, err := sapostgres.NewSettlementCatalogues(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: settlement catalogues for confirmation: %w", err)
	}
	handoff, err := sapostgres.NewOutboxChargeConfirmationHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge confirmation handoff: %w", err)
	}
	attributionForms, err := sapostgres.NewChargeAttributions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: charge attributions: %w", err)
	}
	arrangements, err := sapostgres.NewStatementArrangements(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: statement arrangements: %w", err)
	}
	attributions, err := saapplication.NewJudgeChargeAttributionHandler(attributionForms, arrangements)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: judge charge attribution: %w", err)
	}
	confirm, err := saapplication.NewConfirmChargeHandler(saapplication.ConfirmChargeDeps{
		Charges: charges, Conditions: conditions, Facts: catalogues, Attributions: attributions,
		Downstream: handoff, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: confirm charge: %w", err)
	}
	momentStore, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: settlement moments: %w", err)
	}
	moments, err := saapplication.NewAdmitSettlementMomentHandler(momentStore)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: admit settlement moment: %w", err)
	}
	requests, err := sapostgres.NewSellEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation requests for confirmation: %w", err)
	}
	handler, err := saapplication.NewConfirmChargeOnBasisHandler(saapplication.ConfirmChargeOnBasisDeps{
		Requests: requests, Charges: charges, Bases: conditions, Moments: moments, Confirm: confirm, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: confirm charge on basis: %w", err)
	}
	return handler, nil
}

// formedChargeConfirmationFollowUp 装预估客户费用形成之后的跟进（ADR-0336 决定一、二）：登确认事实，已终局的补记依据。
func formedChargeConfirmationFollowUp(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
) (*saapplication.FormedChargeConfirmationFollowUp, error) {
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: settlement accounts for confirmation facts: %w", err)
	}
	facts, err := sapostgres.NewSettlementCatalogues(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: confirmation fact register: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: final outcomes for confirmation basis: %w", err)
	}
	effectiveFinals, err := saparcelshipment.NewEffectiveDeliveryFinalAdapter(finals)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: effective delivery final view: %w", err)
	}
	confirmation, err := chargeConfirmationOnBasis(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	followUp, err := saapplication.NewFormedChargeConfirmationFollowUp(saapplication.FormedChargeConfirmationDeps{
		Accounts: accounts, Facts: facts, Finals: effectiveFinals, Confirmation: confirmation, Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: formed charge confirmation follow-up: %w", err)
	}
	return followUp, nil
}

// confirmationBasisOnFinalOutcomeConsumer 接终局那一头：PS 终局信封 → SA 消费门 → 按终局采用键回读 PS 终局记录（只认
// 已终局的有效交付）→ 给这件包裹已形成的客户费用记依据并按时点确认。
func confirmationBasisOnFinalOutcomeConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	confirmation, err := chargeConfirmationOnBasis(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: final outcomes for confirmation basis: %w", err)
	}
	processing, err := saparcelshipment.NewConfirmOnFinalOutcomeAdapter(finals, confirmation)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: confirm on final outcome: %w", err)
	}
	consumer, err := sainbox.NewConfirmationBasisOnFinalOutcomeConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: confirmation basis on final outcome consumer: %w", err)
	}
	return consumer, nil
}
