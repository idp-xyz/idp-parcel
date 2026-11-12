package main

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	pppostgres "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	ppdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	ppports "go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证票 declared-goods-lines/23 的闭环（ADR-0380 决定一至四）：受理前估价按登记的计价范围选不到在用的
// 售价卡时，接受前财务控制答端口的`未配置`，链落`等待运营登记`，提交门入账；处理尝试落库，带结算的原因码、这一版申报的服务产品与
// 版本，委托详情的「上次处理」从那里读出四格。计价登卡时经真交接适配器铸出「参数已登记」信封，计价参数那一份登记续办门按租户重驱：
// BUY 那一封不重驱；只在判断时点之后才生效的 SELL 卡叫醒委托、它照旧停、多一条尝试；判断时点在用的 SELL 卡登上之后委托被接受。
//
// 装配与估价采用那条闭环同一份（newOperatorRegistrationLoopFixtureWith、生产的 PS 适配器与作用域源、生产的 SA 施加编排与两本账），
// 时点照「提交接收」折成这一版的系统接收时间（生产的 SubmissionReceiptAsOf）。试算那一口的选卡走计价真价卡册的在用判断（按 SA 交来的
// 估价时点、登记的计价范围、SELL·客户费用与申报产品），选中之后的金额由替身给——估价怎么算不是本票的事。证据只记 S。

const (
	pricingResumeConsumer    = "parcel-shipment/advance-acceptance-chain-on-pricing-parameter-registration"
	pricingResumeProduct     = "SYN-PROD-CN-SG-EXPRESS"
	pricingResumeScope       = "SYN-PRICING-SCOPE-R1"
	pricingResumeFreezeMinor = int64(7300)
)

// Covers: 票 declared-goods-lines/23 完成判据——真库：没有在用价卡 → 委托停在等运营登记，处理尝试落库、带原因码与服务产品；BUY
// 方向的信封不重驱；判断时点之后才生效的售价卡叫醒委托但解不开、新的一条尝试落库；判断时点在用的售价卡登上之后委托被重驱、
// 解开后接受。委托详情的「上次处理」在等运营登记时有值并带两格与续办方。
func TestAPriceCardWaitIsResumedBySellPricingParameterRegistration(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	fixture.commercial.asOfAt = submissionReceiptAsOf(t, fixture.requests)
	ctx := t.Context()

	settlement := wirePricedSettlement(t, fixture, control)
	settlement.trials.answer = cardBackedTrials(t, fixture)

	received := fixture.submitDeclaringProduct(t, ctx, pricingResumeProduct)
	request := fixture.mustLoadRequest(t, ctx)
	version := request.CurrentSubmissionVersion().VersionID()
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：选不到在用的售价卡——价卡那一格等运营登记，等待态先落库，消费门入账。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("没有在用价卡必须按处理完毕入账：%v", err)
	}
	fixture.markPublished(t, submittedDelivery)
	if settlement.trials.asked != 1 || !settlement.trials.query.At.Equal(received) {
		t.Fatalf("试算被问 %d 次、估价时点 %s, want 问一次、时点是这一版的接收时间 %s",
			settlement.trials.asked, settlement.trials.query.At, received)
	}
	fixture.assertWaitingOnRegistration(t, ctx)
	const attemptsOnThePriceCard = `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'FINANCIAL_CONTROL_NOT_CONFIGURED' AND resume_path = 'OPERATOR_REGISTRATION'
		  AND settlement_reason = 'ESTIMATE_PRICE_CARD_NOT_CONFIGURED' AND service_product = '` + pricingResumeProduct + `'`
	if n := fixture.countSQL(t, attemptsOnThePriceCard+` AND submission_version = '`+version.String()+`'`); n != 1 {
		t.Fatalf("带结算原因、服务产品与版本的处理尝试 = %d 行, want 1", n)
	}
	fixture.assertDetailShowsThePriceCardWait(t, ctx)

	// BUY 卡登上：计价同事务铸「参数已登记」，方向 BUY——续办门入账，不问队列、不重驱。
	registerPriceCardForTest(t, fixture.db, pricingResumeBuyCard(t))
	buyDelivery := fixture.claimDeliveryOfType(t, string(psinbox.PricingParameterRegisteredEventType))
	if err := fixture.pricingParameterGate.Consume(ctx, buyDelivery.Envelope); err != nil {
		t.Fatalf("BUY 那一封应入账：%v", err)
	}
	fixture.markPublished(t, buyDelivery)
	if n := fixture.countInbox(t, pricingResumeConsumer, string(buyDelivery.Envelope.ID)); n != 1 {
		t.Fatalf("BUY 那一封 inbox 行数 = %d, want 1", n)
	}
	if settlement.trials.asked != 1 || fixture.countSQL(t, attemptsOnThePriceCard) != 1 {
		t.Fatalf("BUY 那一封重驱了委托：试算被问 %d 次、价卡停因的尝试 %d 行", settlement.trials.asked, fixture.countSQL(t, attemptsOnThePriceCard))
	}

	// 登卡这一刻才生效的售价卡：晚于这一版的接收时间、早于重驱那一刻。SELL 那一封叫醒委托，判断时点（提交接收）上它不在用，委托
	// 照旧停，新的一条尝试落库，本封入账——估价若错拿重驱那一刻当时点，这张卡就会被选中、委托被接受。
	registerPriceCardForTest(t, fixture.db, pricingResumeSellCard(t, "SYN-PLAN-SELL-FROM-NOW", time.Now().UTC()))
	laterDelivery := fixture.claimDeliveryOfType(t, string(psinbox.PricingParameterRegisteredEventType))
	if err := fixture.pricingParameterGate.Consume(ctx, laterDelivery.Envelope); err != nil {
		t.Fatalf("判断时点之后才生效的卡解不开，本封应按处理完毕入账：%v", err)
	}
	fixture.markPublished(t, laterDelivery)
	if settlement.trials.asked != 2 {
		t.Fatalf("SELL 那一封之后试算被问 %d 次, want 2——委托没被叫醒", settlement.trials.asked)
	}
	fixture.assertWaitingOnRegistration(t, ctx)
	if n := fixture.countSQL(t, attemptsOnThePriceCard); n != 2 {
		t.Fatalf("价卡停因的处理尝试 = %d 行, want 2——叫醒之后照旧停的那一轮要留一条", n)
	}

	// 判断时点在用的售价卡：SELL 那一封重驱，估价形成、冻结占下，形成决定成立接受，队列出列。
	registerPriceCardForTest(t, fixture.db,
		pricingResumeSellCard(t, "SYN-PLAN-SELL-IN-FORCE", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))
	inForceDelivery := fixture.claimDeliveryOfType(t, string(psinbox.PricingParameterRegisteredEventType))
	if err := fixture.pricingParameterGate.Consume(ctx, inForceDelivery.Envelope); err != nil {
		t.Fatalf("在用的售价卡登上之后的一拍：%v", err)
	}
	accepted := fixture.mustLoadRequest(t, ctx)
	if accepted.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("续办后状态 = %q, want ACCEPTED", accepted.State())
	}
	if remaining := fixture.listRegistrationQueue(t, ctx); len(remaining) != 0 {
		t.Fatalf("接受之后等待运营登记队列仍列出 %d 行", len(remaining))
	}
	if n := fixture.countOutboxOfType(t, string(nrinbox.AcceptedDecisionEventType)); n != 1 {
		t.Fatalf("接受信封 = %d 封, want 恰好 1", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.funds_freeze`); n != 1 {
		t.Fatalf("冻结账本 %d 行, want 1——控制没有按选中的卡占下这一笔", n)
	}
}

// Covers: ADR-0380 决定一「依赖读不回照旧内部重试」——试算那一口读不回（SA-ESTIMATE_TRIALS_UNAVAILABLE）时，链停在`未形成`，提交门
// 交回未决哨兵、整笔回滚：等待态不落、处理尝试一行不留，信封等重投。
func TestAnUnavailableEstimateStillRollsBack(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	ctx := t.Context()

	settlement := wirePricedSettlement(t, fixture, control)
	settlement.trials.err = saports.ErrPreAcceptanceTrialsUnavailable

	fixture.submitDeclaringProduct(t, ctx, pricingResumeProduct)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope)
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) || !strings.Contains(err.Error(), "FINANCIAL_CONTROL_NOT_FORMED") {
		t.Fatalf("试算读不回应交回未决哨兵、停在`未形成`，实际 err = %v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submittedDelivery.Envelope.ID)); n != 0 {
		t.Fatalf("内部重试必须让账本无痕等重投，实际 inbox 行数 = %d", n)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt`); n != 0 {
		t.Fatalf("回滚之后处理尝试 = %d 行, want 0", n)
	}
	if _, present := fixture.mustLoadRequest(t, ctx).AcceptanceDecisionTask().WaitingOn(); present {
		t.Fatal("内部重试那一轮落了等待态")
	}
}

// Covers: 路由表「计价参数已登记」那一份登记续办门（ADR-0380 决定二），且它在生产依赖图上接得起来：信封由 parcel-pricing 真交接
// 适配器随登卡铸出，类型与小包托运、结算两个消费方各写的字面对上；一拍扇出给两路，两路各记各的 inbox，该租户没有委托在等、
// 没有评价停着，本封定稿。漏挂的话，PS 这一路收不到；排位撞了，装配即拒。
func TestARegisteredPricingParameterReachesBothResumeGatesOnTheProductionGraph(t *testing.T) {
	beat, db, _ := wiredBeat(t)
	registerPriceCardForTest(t, db, pricingResumeSellCard(t, "SYN-PLAN-SELL-PARITY", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)))

	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var eventID, eventType string
	if err := querier.QueryRow(t.Context(),
		`SELECT event_id, event_type FROM `+migrate.SchemaBento+`.outbox WHERE event_type LIKE 'parcel-pricing.%'`,
	).Scan(&eventID, &eventType); err != nil {
		t.Fatalf("读回登卡铸的信封：%v", err)
	}
	if eventType != string(psinbox.PricingParameterRegisteredEventType) {
		t.Fatalf("发布侧写 %q，小包托运认 %q", eventType, psinbox.PricingParameterRegisteredEventType)
	}
	if eventType != string(sainbox.PricingParameterRegisteredResumption.EventType) {
		t.Fatalf("发布侧写 %q，结算认 %q", eventType, sainbox.PricingParameterRegisteredResumption.EventType)
	}

	published, err := beat.DispatchOnce(t.Context())
	if err != nil {
		t.Fatalf("一拍：%v", err)
	}
	if published != 1 {
		t.Fatalf("published = %d, want 1；失败码 = %q——路由表没把计价参数已登记投给两路续办门",
			published, recordedFailureCode(t, db, eventID))
	}
	for _, consumer := range []string{pricingResumeConsumer, sainbox.PricingParameterRegisteredResumption.ConsumerName} {
		var count int
		if err := querier.QueryRow(t.Context(),
			`SELECT count(*) FROM `+migrate.SchemaBento+`.inbox WHERE consumer = $1 AND event_id = $2`, consumer, eventID,
		).Scan(&count); err != nil {
			t.Fatalf("数 inbox：%v", err)
		}
		if count != 1 {
			t.Fatalf("%s 名下 inbox 行数 = %d, want 1", consumer, count)
		}
	}
}

// wirePricedSettlement 登好 SA 那一侧并把生产的 PS→SA 适配器装进控制那一口：结算账户、受理前估价采用（计价范围 pricingResumeScope）、
// 恰好够冻一笔估价的入账余额。试算那一口留给用例拨。
func wirePricedSettlement(
	t *testing.T,
	fixture *operatorRegistrationLoopFixture,
	control *synRLateBoundControl,
) *synRSettlementControl {
	t.Helper()
	ctx := t.Context()
	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	adoptions, err := sapostgres.NewPreAcceptanceEstimateAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("构造采用册：%v", err)
	}
	freezes, err := sapostgres.NewFreezeLedgers(fixture.db)
	if err != nil {
		t.Fatalf("构造冻结账本：%v", err)
	}
	balances, err := sapostgres.NewOperationalBalances(fixture.db, freezes)
	if err != nil {
		t.Fatalf("构造运营余额：%v", err)
	}
	account := synRReceivableAccount(t)
	adoption, err := sadomain.NewPreAcceptanceEstimateAdoption(account.ID(), sadomain.PerParcelSellTrialSum,
		mustPS(t, sadomain.NewEstimatePricingScope, pricingResumeScope))
	if err != nil {
		t.Fatalf("采用行：%v", err)
	}
	scope, err := sadomain.NewSettlementScope(
		mustPS(t, sadomain.NewLegalEntityReference, synRLegalEntity), account.ID(), mustPS(t, sadomain.NewCurrencyCode, synRCurrency))
	if err != nil {
		t.Fatalf("结算作用域：%v", err)
	}
	registeredAt := time.Date(2026, 10, 13, 9, 0, 0, 0, time.UTC)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		if _, err := accounts.Save(txCtx, tenant, account, registeredAt); err != nil {
			return err
		}
		if _, err := adoptions.SaveAdoption(txCtx, tenant, adoption, registeredAt); err != nil {
			return err
		}
		_, err := balances.RecordBalance(txCtx, tenant, scope, sapostgres.BalancePosting{
			PostedMinor: pricingResumeFreezeMinor,
			AsOf:        registeredAt,
		})
		return err
	})
	directory, err := pssettlement.NewRegisteredAccountDirectory(accounts)
	if err != nil {
		t.Fatalf("构造账户目录：%v", err)
	}
	settlement := newSynRSettlementControl(t, fixture.db)
	control.inner = pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Apply: settlement.apply,
		Scopes: pssettlement.NewPolicyBackedControlScopeSource(
			&synRSettledCommercialBasis{synRSwitchableAsOfCommercialBasis: fixture.commercial}, directory),
	})
	return settlement
}

// cardBackedTrials 让试算那一口按计价真价卡册的在用判断选卡：（租户、登记的计价范围、SELL、客户费用、估价时点、申报产品）。没有在用
// 的卡、适用冲突各答各的格；选中一张时答一份带合计的试算（一件、pricingResumeFreezeMinor），价卡字面取选中的那一版。
//
// 解析在试算那一口收到的 ctx 上做：重驱那一轮在自己的顶层事务里，测试库的连接池只有两条，另开一条会等不到。
func cardBackedTrials(
	t *testing.T,
	fixture *operatorRegistrationLoopFixture,
) func(context.Context, saports.PreAcceptanceTrialQuery) saports.PreAcceptanceTrialAnswer {
	t.Helper()
	cards, err := pppostgres.NewPriceCards(fixture.db)
	if err != nil {
		t.Fatalf("构造价卡库：%v", err)
	}
	product := pptest.Value(t, ppdomain.NewServiceProductReference, pricingResumeProduct)
	return func(ctx context.Context, query saports.PreAcceptanceTrialQuery) saports.PreAcceptanceTrialAnswer {
		resolution, err := cards.ResolveInForce(ctx,
			pptest.Value(t, ppdomain.NewTenantID, query.Tenant.String()),
			pptest.Value(t, ppdomain.NewPricingScopeID, query.PricingScope.String()),
			ppdomain.PricingDirectionSell, ppdomain.PricingPurposeCustomerCharge, query.At, product)
		if err != nil {
			t.Errorf("在用价卡解析：%v", err)
			return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialPriceCardNotConfigured}
		}
		switch resolution.Outcome {
		case ppports.PriceCardVersionInForce:
		case ppports.PriceCardApplicabilityConflict:
			return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialPriceCardConflict}
		default:
			return saports.PreAcceptanceTrialAnswer{Outcome: saports.PreAcceptanceTrialPriceCardNotConfigured}
		}
		reference := resolution.Plan.Reference()
		card := sadomain.VersionReferenceLiteral(sadomain.VersionReferenceParts{
			Kind: string(reference.Kind()), ID: reference.ID(), Version: reference.Version(), Fingerprint: reference.Fingerprint(),
		})
		trial, err := sadomain.NewPreAcceptanceTrial("SYN-PARCEL-01", "SYN-TRIAL-"+query.SubmissionVersion,
			[]string{card}, pricingResumeFreezeMinor, query.Currency)
		if err != nil {
			t.Errorf("试算：%v", err)
		}
		return saports.PreAcceptanceTrialAnswer{
			Outcome:   saports.PreAcceptanceTrialsFormed,
			PriceCard: card,
			Trials:    []sadomain.PreAcceptanceTrial{trial},
		}
	}
}

// submissionReceiptAsOf 照「提交接收」折时点：用生产的 SubmissionReceiptAsOf，把替身声明的语义换成产品发布的那一版引用再问。
func submissionReceiptAsOf(
	t *testing.T,
	requests *pspostgres.ShipmentRequests,
) func(context.Context, psports.JudgmentAsOfQuery) (time.Time, error) {
	t.Helper()
	citation, err := pspartycommercial.SubmissionReceiptCitation()
	if err != nil {
		t.Fatalf("提交接收的产品引用：%v", err)
	}
	source, err := pspartycommercial.NewSubmissionReceiptAsOf(requests)
	if err != nil {
		t.Fatalf("构造提交接收时点源：%v", err)
	}
	semantics := mustPS(t, psdomain.NewAsOfSemanticsReference, citation)
	return func(ctx context.Context, query psports.JudgmentAsOfQuery) (time.Time, error) {
		declared, err := psdomain.NewDeclaredAsOf(query.Declared.Kind(), semantics, query.Declared.PolicyVersion())
		if err != nil {
			return time.Time{}, err
		}
		query.Declared = declared
		at, found, err := source.FormAsOfValue(ctx, query)
		if err != nil {
			return time.Time{}, err
		}
		if !found {
			return time.Time{}, errors.New("提交接收没有折出时点")
		}
		return at, nil
	}
}

// submitDeclaringProduct 照 SYN 纵切的提交配方建单，另申报一个服务产品；交回这一版的接收时间。
func (fixture *operatorRegistrationLoopFixture) submitDeclaringProduct(t *testing.T, ctx context.Context, product string) time.Time {
	t.Helper()
	entry, err := psdomain.NewCanonicalContentEntry(psdomain.RequestedServiceProductEntryName, product)
	if err != nil {
		t.Fatalf("服务产品条目：%v", err)
	}
	command := fixture.submitCommand(t)
	command.RequestedServiceProduct = psdomain.RequestedServiceProductOf([]psdomain.CanonicalContentEntry{entry})
	var outcome psapplication.SubmitOutcome
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		result, err := fixture.submitHandler.Handle(txCtx, command)
		if err != nil {
			return err
		}
		outcome = result.Outcome()
		return nil
	})
	if outcome != psapplication.OutcomeSubmitted {
		t.Fatalf("submit outcome = %q, want SUBMITTED", outcome)
	}
	return fixture.mustLoadRequest(t, ctx).CurrentSubmissionVersion().SourceSubmission().ReceivedAt()
}

func (fixture *operatorRegistrationLoopFixture) assertWaitingOnRegistration(t *testing.T, ctx context.Context) {
	t.Helper()
	waiting := fixture.mustLoadRequest(t, ctx)
	if waitingOn, present := waiting.AcceptanceDecisionTask().WaitingOn(); waiting.State() != psdomain.ShipmentRequestSubmitted ||
		!present || waitingOn != psdomain.ResumeByOperatorRegistration {
		t.Fatalf("状态 = %q、等待 %v（present=%v）, want SUBMITTED、等运营登记", waiting.State(), waitingOn, present)
	}
	if queue := fixture.listRegistrationQueue(t, ctx); len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("等待运营登记队列 = %+v, want 恰好这一份委托", queue)
	}
}

// assertDetailShowsThePriceCardWait 经生产的查阅读面读委托详情：「上次处理」在等运营登记时有值，带续办方、结算原因与服务产品。
func (fixture *operatorRegistrationLoopFixture) assertDetailShowsThePriceCardWait(t *testing.T, ctx context.Context) {
	t.Helper()
	views, err := pspostgres.NewShipmentRequestViews(fixture.db)
	if err != nil {
		t.Fatalf("构造查阅读面：%v", err)
	}
	scope, err := psdomain.NewAuthorizedQueryScope(
		mustPS(t, psdomain.NewQueryScopeReference, "SYN-SCOPE-GRANT-01"),
		fixture.identity.TenantID(),
		[]psdomain.CustomerAccountID{fixture.identity.CustomerAccountID()},
	)
	if err != nil {
		t.Fatalf("查询作用域：%v", err)
	}
	detail, found, err := views.FindVisibleByID(ctx, scope, fixture.requestID)
	if err != nil || !found {
		t.Fatalf("查阅详情：found=%v err=%v", found, err)
	}
	task := detail.Task
	if !task.HasAttempt || task.LastAttemptReason != "FINANCIAL_CONTROL_NOT_CONFIGURED" ||
		task.LastAttemptResumePath != psdomain.ResumeByOperatorRegistration ||
		!strings.HasPrefix(task.LastAttemptContinuation, "CONT-") ||
		task.LastAttemptSettlementReason != "ESTIMATE_PRICE_CARD_NOT_CONFIGURED" ||
		task.LastAttemptServiceProduct != pricingResumeProduct {
		t.Fatalf("委托详情的上次处理 = %+v, want 等运营登记、带结算原因与服务产品", task)
	}
}

// pricingResumeSellCard 造一张演示租户的 SELL·客户费用合成卡，放在采用登记的计价范围上，从 startsAt 起生效、不设止期。卡上的价
// 不进试算（金额由替身给），要证的是它在估价时点在不在用。
func pricingResumeSellCard(t *testing.T, planID string, startsAt time.Time) ppdomain.PriceCardRegistration {
	t.Helper()
	plan := pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, ppdomain.ArtifactPricingPlan, planID, "v1"),
		TableReference:        pptest.IdentityReference(t, ppdomain.ArtifactRateTable, planID+"-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, ppdomain.ArtifactWeightPolicy, planID+"-WEIGHT", "v1"),
		Scope:                 pricingResumeScope,
		Direction:             ppdomain.PricingDirectionSell,
		Purpose:               ppdomain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period:                pptest.Period{StartsAt: startsAt},
		RateEntryID:           planID + "-Z1",
		RateZone:              "Z1",
		MinimumKilograms:      "0",
		MaximumKilograms:      "30",
		RateAmount:            "73",
		WeightRounding:        ppdomain.RoundingNone,
		WeightStepKilograms:   "1",
	})
	return pricingResumeRegistration(t, planID, plan)
}

// pricingResumeBuyCard 造一张 BUY·供应商成本卡，放在同一个计价范围上：它的「参数已登记」方向是 BUY。
func pricingResumeBuyCard(t *testing.T) ppdomain.PriceCardRegistration {
	t.Helper()
	structures, err := ppdomain.NewPricingPlanStructures(nil, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	return syntheticBuyCard(t, "SYN-PLAN-BUY-PRICING-RESUME", pricingResumeScope, "CNY", structures)
}

func pricingResumeRegistration(t *testing.T, planID string, plan ppdomain.PricingPlanVersion) ppdomain.PriceCardRegistration {
	t.Helper()
	source, err := ppdomain.NewSourceFileIdentity("SYN-"+planID+".xlsx", strings.Repeat("cd", 32))
	if err != nil {
		t.Fatal(err)
	}
	authorization, err := ppdomain.NewVersionReferenceIdentity(ppdomain.ArtifactCommercialAuthorization, "SYN-AUTH-"+planID, "v1")
	if err != nil {
		t.Fatal(err)
	}
	registration, err := ppdomain.NewPriceCardRegistration(ppValue(t, ppdomain.NewTenantID, demoTenant), plan, source,
		authorization, "SYN-PRICING-GOVERNOR-01")
	if err != nil {
		t.Fatalf("价卡登记：%v", err)
	}
	return registration
}
