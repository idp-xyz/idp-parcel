package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	saidentity "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/identity"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapricing "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelpricing"
	sapartycommercial "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

// evaluationResumptionPageSize 是续办门每次向续办队列要多少件。它不是租户取值：一页多大只影响一封「参数已登记」分几次
// 取完队列，不影响续办哪几件；取值照登记续办门那一页（operatorRegistrationRedrivePageSize 头注）。
const evaluationResumptionPageSize = 200

// buyEvaluationOnOccurrenceTrigger 装 ADR-0208 的 BUY 发起编排：触发册取费用项目、PC 取协议的商业范围、请求编排登记请求并交
// 信封，答`未配置`时记进 BUY 发起停办册（ADR-0229 决定四）。发生项已形成那一路与续办重跑发起停办那一路各调一次、各得
// 一只：同一种装法只写在这里，两路问的是同一组册。
func buyEvaluationOnOccurrenceTrigger(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
) (*saapplication.TriggerBuyEvaluationOnOccurrenceFormedHandler, error) {
	registry, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: evaluation requests for charge occurrences: %w", err)
	}
	identities, err := saidentity.NewEvaluationRequestIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: evaluation request identities for charge occurrences: %w", err)
	}
	handoff, err := sapostgres.NewOutboxEvaluationRequestHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: evaluation request handoff for charge occurrences: %w", err)
	}
	requests, err := saapplication.NewRequestBuyEvaluationHandler(saapplication.RequestBuyEvaluationDeps{
		Registry:   registry,
		Identity:   identities,
		Downstream: handoff,
		Clock:      clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: request buy evaluation for charge occurrences: %w", err)
	}
	triggers, err := sapostgres.NewBuyEvaluationTriggers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation triggers: %w", err)
	}
	agreementScopes, err := pcpostgres.NewSupplierAgreementScopes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: supplier agreement scopes: %w", err)
	}
	scopes, err := sapartycommercial.NewSupplierAgreementScope(agreementScopes)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: supplier agreement scope for buy evaluation: %w", err)
	}
	stalls, err := sapostgres.NewEvaluationResumptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation initiation stalls: %w", err)
	}
	trigger, err := saapplication.NewTriggerBuyEvaluationOnOccurrenceFormedHandler(saapplication.TriggerBuyEvaluationOnOccurrenceFormedDeps{
		Triggers:   triggers,
		Agreements: scopes,
		Requests:   requests,
		Stalls:     stalls,
		Clock:      clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: trigger buy evaluation on charge occurrence formed: %w", err)
	}
	return trigger, nil
}

// recordEvaluationUnformedConsumer 接 ADR-0229 决定三那条线：PP 对一份评价请求答不出评价时交出的
// `parcel-pricing.evaluation-request.unformed` → SA 消费门 → 消费侧适配器把 PP 的方向与停因词译成本上下文的词 → 在 BUY 或
// SELL 请求册上记停因。跨上下文翻译只在 SA 的 `adapters/parcelpricing`。
func recordEvaluationUnformedConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	sellRequests, err := sapostgres.NewSellEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation requests for unformed evaluations: %w", err)
	}
	buyRequests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation requests for unformed evaluations: %w", err)
	}
	recording, err := saapplication.NewRecordEvaluationUnformedHandler(saapplication.RecordEvaluationUnformedDeps{
		SellRequests:    sellRequests,
		SellConclusions: sellRequests,
		BuyRequests:     buyRequests,
		BuyStops:        buyRequests,
		Clock:           clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: record evaluation unformed: %w", err)
	}
	processing, err := sapricing.NewRecordOnEvaluationRequestUnformedAdapter(recording)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: record on evaluation request unformed: %w", err)
	}
	consumer, err := sainbox.NewEvaluationRequestUnformedConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: evaluation request unformed consumer: %w", err)
	}
	return consumer, nil
}

// stalledEvaluationResumer 装 ADR-0229 决定二、四的续办编排：两只发起编排（重跑发起停办）与两本请求册的整套写口（起下一代）。
// 发起编排与计费时点、发生项已形成两路同一种装法（sellEvaluationRequester、buyEvaluationOnOccurrenceTrigger），续办重跑问的
// 是同一组册。
func stalledEvaluationResumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
) (*saapplication.ResumeStalledEvaluationHandler, error) {
	sellMoments, err := sellEvaluationRequester(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	buyOccurrences, err := buyEvaluationOnOccurrenceTrigger(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	stalls, err := sapostgres.NewEvaluationResumptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: evaluation initiation stalls: %w", err)
	}
	sellRequests, err := sapostgres.NewSellEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation requests for resumption: %w", err)
	}
	sellIdentities, err := saidentity.NewSellEvaluationRequestIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation request identities for resumption: %w", err)
	}
	sellHandoff, err := sapostgres.NewOutboxSellEvaluationRequestHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: sell evaluation request handoff for resumption: %w", err)
	}
	buyRequests, err := sapostgres.NewEvaluationRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation requests for resumption: %w", err)
	}
	buyIdentities, err := saidentity.NewEvaluationRequestIdentities()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation request identities for resumption: %w", err)
	}
	buyHandoff, err := sapostgres.NewOutboxEvaluationRequestHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: buy evaluation request handoff for resumption: %w", err)
	}
	resumer, err := saapplication.NewResumeStalledEvaluationHandler(saapplication.ResumeStalledEvaluationDeps{
		SellStalls:     stalls,
		BuyStalls:      stalls,
		SellMoments:    sellMoments,
		BuyOccurrences: buyOccurrences,
		SellRequests:   sellRequests,
		SellSuccessors: sellRequests,
		SellRegistry:   sellRequests,
		SellIdentity:   sellIdentities,
		SellHandoff:    sellHandoff,
		BuyRequests:    buyRequests,
		BuySuccessors:  buyRequests,
		BuyRegistry:    buyRequests,
		BuyIdentity:    buyIdentities,
		BuyHandoff:     buyHandoff,
		Clock:          clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: resume stalled evaluation: %w", err)
	}
	return resumer, nil
}

// evaluationResumptionConsumer 装一扇续办门：一类「参数已登记」信封到来，按租户取续办队列逐件续办。每一类信封各装一扇，
// 各记各的 inbox，续办编排各得一只。
func evaluationResumptionConsumer(
	registration sainbox.EvaluationParameterRegistration,
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	resumer, err := stalledEvaluationResumer(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}
	queue, err := sapostgres.NewEvaluationResumptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: stalled evaluation queue: %w", err)
	}
	consumer, err := sainbox.NewEvaluationParameterRegisteredConsumer(
		registration, db.Transactor(), inboxStore, queue, resumer, evaluationResumptionPageSize)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: evaluation resumption consumer %s: %w", registration.EventType, err)
	}
	return consumer, nil
}
