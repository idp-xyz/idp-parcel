package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 本文件对真实 PostgreSQL 16 证票 routing-first-cut/19 结算账户那一笔的闭环：结算账户册上没有这一户时，接受前财务
// 控制在作用域那一步答端口的`未配置`格（CONTROL_SCOPE_NOT_CONFIGURED），链落`等待运营登记`，提交门入账而不回滚
// 重投，处理尝试带新未决原因与续办引用留在库里；settlement-accounting 侧登记这一户时经真交接适配器铸出「结算
// 账户已登记」信封，结算账户那一份登记续办门按租户重驱，同一条链越过作用域、停到金额那一格。
//
// 控制那一口装的是生产的 PS 适配器、生产的作用域源与生产的账户目录，账户目录读的是真 SA 账户册——登记那一行与
// 重驱时作用域问到的是同一张表；只把金额那一口换成计数替身（金额格不在本票，随 psb/25 挪），商业解析换成带结算
// 政策回显的替身。金额那一格照旧是`未形成`、回滚重投，所以重驱那一轮整封交回未决哨兵，账本无痕等重投。

// Covers: 票 routing-first-cut/19 完成判据一（作用域未配置：入账留痕、信封不落 ABANDONED；登记结算账户之后信封到达，
// 重驱同一委托越过作用域、停到金额）。
func TestAScopeWaitIsResumedByTheSettlementAccountRegistrationEnvelope(t *testing.T) {
	control := &synRLateBoundControl{}
	fixture := newOperatorRegistrationLoopFixtureWith(t, control)
	fixture.commercial.configured = true
	ctx := t.Context()
	tenant := fixture.identity.TenantID()

	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatalf("构造结算账户登记册：%v", err)
	}
	directory, err := pssettlement.NewRegisteredAccountDirectory(accounts)
	if err != nil {
		t.Fatalf("构造账户目录：%v", err)
	}
	amounts := &synRCountingControlAmounts{}
	control.inner = pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Scopes: pssettlement.NewPolicyBackedControlScopeSource(
			&synRSettledCommercialBasis{synRSwitchableAsOfCommercialBasis: fixture.commercial}, directory),
		Amounts: amounts,
	})

	fixture.submit(t, ctx)
	request := fixture.mustLoadRequest(t, ctx)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		return fixture.submittedHandoff.HandOffShipmentRequestSubmitted(txCtx,
			psports.ShipmentRequestSubmittedHandoffIntent{Identity: fixture.identity, Request: request})
	})

	// 第一扇门：作用域那一步在账户册上查不到这一户——`未配置`，等待态先落库，消费门入账。
	submittedDelivery := fixture.claimDeliveryOfType(t, string(psinbox.ShipmentRequestSubmittedEventType))
	if err := fixture.submittedGate.Consume(ctx, submittedDelivery.Envelope); err != nil {
		t.Fatalf("结算账户没登记必须按处理完毕入账：%v", err)
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain", string(submittedDelivery.Envelope.ID)); n != 1 {
		t.Fatalf("提交门 inbox 行数 = %d, want 1——等待态没有随本份投递入账", n)
	}
	if amounts.asked != 0 {
		t.Fatalf("作用域没形成却问了 %d 次金额——停顿不是本用例要的那一格", amounts.asked)
	}
	waiting := fixture.mustLoadRequest(t, ctx)
	if waitingOn, present := waiting.AcceptanceDecisionTask().WaitingOn(); !present || waitingOn != psdomain.ResumeByOperatorRegistration {
		t.Fatalf("等待态 = %v（present=%v）, want OPERATOR_REGISTRATION", waitingOn, present)
	}
	if queue := fixture.listRegistrationQueue(t, ctx); len(queue) != 1 || queue[0].ShipmentRequestID != fixture.requestID {
		t.Fatalf("等待运营登记队列 = %+v, want 恰好这一份委托", queue)
	}
	const attemptsOnTheNewReason = `SELECT count(*) FROM parcel_shipment.acceptance_processing_attempt
		WHERE reason_ref = 'FINANCIAL_CONTROL_NOT_CONFIGURED' AND resume_path = 'OPERATOR_REGISTRATION'
		  AND continuation_ref LIKE 'CONT-%'`
	if n := fixture.countSQL(t, attemptsOnTheNewReason); n != 1 {
		t.Fatalf("带新原因与续办引用的处理尝试 = %d 行, want 1——停在哪一格没有留在任务上", n)
	}
	fixture.markPublished(t, submittedDelivery)

	// 登记：settlement-accounting 的登记编排落账户行，同事务经真交接适配器铸「结算账户已登记」信封。
	handoff, err := sapostgres.NewOutboxSettlementAccountRegisteredHandoff(fixture.db, fixture.store, systemClock{})
	if err != nil {
		t.Fatalf("构造结算账户已登记交接：%v", err)
	}
	register, err := saapplication.NewRegisterSettlementAccountHandler(accounts, handoff, systemClock{})
	if err != nil {
		t.Fatalf("构造结算账户登记：%v", err)
	}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := register.Register(txCtx, saapplication.RegisterSettlementAccountCommand{
			Tenant:  mustPS(t, sadomain.NewTenantID, tenant.String()),
			Account: synRReceivableAccount(t),
		})
		return err
	})
	if n := fixture.countOutboxOfType(t, string(psinbox.SettlementAccountRegisteredEventType)); n != 1 {
		t.Fatalf("「结算账户已登记」信封 = %d 封, want 恰好 1——消费侧认的类型串与发布侧写的不是同一个", n)
	}

	// 结算账户那一份续办门：按租户取队列再驱一拍——作用域形成、问到金额，金额那一格`未形成`照旧回滚重投。
	accountDelivery := fixture.claimDeliveryOfType(t, string(psinbox.SettlementAccountRegisteredEventType))
	err = fixture.settlementAccountGate.Consume(ctx, accountDelivery.Envelope)
	if !errors.Is(err, psinbox.ErrAcceptanceChainUndecided) {
		t.Fatalf("越过作用域之后停在金额那一格，整封应交回未决哨兵等重投，实际 err = %v", err)
	}
	if !strings.Contains(err.Error(), "FINANCIAL_CONTROL_NOT_FORMED") {
		t.Fatalf("重驱停在的不是`未形成`那一格：%v", err)
	}
	if amounts.asked == 0 {
		t.Fatal("重驱没有问到金额——作用域那一格没有越过")
	}
	if n := fixture.countInbox(t, "parcel-shipment/advance-acceptance-chain-on-settlement-account-registration",
		string(accountDelivery.Envelope.ID)); n != 0 {
		t.Fatalf("未决必须让账本无痕等重投，实际 inbox 行数 = %d", n)
	}
	still := fixture.mustLoadRequest(t, ctx)
	if still.State() != psdomain.ShipmentRequestSubmitted {
		t.Fatalf("重驱后状态 = %q, want SUBMITTED", still.State())
	}
	if n := fixture.countSQL(t, attemptsOnTheNewReason); n != 1 {
		t.Fatalf("重驱那一轮回滚之后处理尝试 = %d 行, want 仍为 1", n)
	}
}

// synRLateBoundControl 让控制那一口在夹具建好之后再装：生产 PS 适配器要读的账户册挂在夹具自己的库上。
type synRLateBoundControl struct {
	inner psports.PreAcceptanceFinancialController
}

func (control *synRLateBoundControl) ApplyPreAcceptanceFinancialControl(
	ctx context.Context,
	request psports.FinancialControlRequest,
) (psports.PreAcceptanceControlAssessment, error) {
	return control.inner.ApplyPreAcceptanceFinancialControl(ctx, request)
}

var _ psports.PreAcceptanceFinancialController = (*synRLateBoundControl)(nil)

// synRCountingControlAmounts 是金额那一口的替身（隔离合成 `S`）：恒答未配置并计数。被问到即证作用域已形成——
// 适配器只在作用域形成之后才问金额。
type synRCountingControlAmounts struct{ asked int }

func (amounts *synRCountingControlAmounts) FormControlAmount(
	context.Context,
	psports.FinancialControlRequest,
) (int64, bool, error) {
	amounts.asked++
	return 0, false, nil
}

// synRSettledCommercialBasis 在时点可拨的商业依据替身之上，让解析带回已采用结算政策的回显——作用域源据它向
// 账户册问应收账户。其余两段照旧。
type synRSettledCommercialBasis struct {
	*synRSwitchableAsOfCommercialBasis
}

const (
	synRSettlementPolicy = "SYN-SETTLE-R1"
	synRLegalEntity      = "SYN-LE-R1"
	synRCounterparty     = "SYN-CP-R1"
	synRCurrency         = "XTS"
)

func (double *synRSettledCommercialBasis) ResolveCommercialBasis(
	_ context.Context,
	_ psports.CommercialBasisQuery,
) (psports.CommercialBasisResolution, error) {
	t := double.t
	t.Helper()
	policy, err := psdomain.NewSettlementPolicyEcho(synRSettlementPolicy, "v1")
	if err != nil {
		t.Fatalf("结算政策回显：%v", err)
	}
	terms, err := psdomain.NewAdoptedSettlementTerms(psdomain.AdoptedSettlementTermsSpec{
		Policy:       policy,
		Method:       mustPS(t, psdomain.NewSettlementMethodEcho, "TERMS"),
		LegalEntity:  mustPS(t, psdomain.NewSettlementLegalEntityEcho, synRLegalEntity),
		Counterparty: mustPS(t, psdomain.NewSettlementCounterpartyEcho, synRCounterparty),
		Currency:     mustPS(t, psdomain.NewSettlementCurrencyEcho, synRCurrency),
	})
	if err != nil {
		t.Fatalf("采用结算政策回显：%v", err)
	}
	groups, err := psdomain.NewApplicableCheckGroups(
		psdomain.PreAcceptanceFinancialControlCheck,
		psdomain.NetworkReachabilityCheck,
	)
	if err != nil {
		t.Fatalf("适用校验组：%v", err)
	}
	snapshot, err := psdomain.NewCommercialBasisSnapshot(psdomain.CommercialBasisSnapshotSpec{
		ResolutionID: mustPS(t, psdomain.NewCommercialResolutionID, "SYN-RES-R2"),
		RulePackage:  mustPS(t, psdomain.NewRulePackageReference, "SYN-RULES-R1/v1"),
		ViewRevision: mustPS(t, psdomain.NewCommercialViewRevision, "SYN-VIEW-R2"),
		DeclaredAsOf: []psdomain.DeclaredAsOf{
			synDeclaredAsOf(t, psdomain.ReachabilityJudgmentKind),
			synDeclaredAsOf(t, psdomain.FinancialControlJudgmentKind),
		},
		Applicable:      groups,
		ManualReview:    psdomain.ManualReviewNotRequiredByRules,
		SettlementTerms: terms,
	})
	if err != nil {
		t.Fatalf("商业依据快照：%v", err)
	}
	return psports.CommercialBasisResolution{Snapshot: snapshot, Applicability: psdomain.CommerciallyApplicable}, nil
}

var _ psports.CommercialBasisResolver = (*synRSettledCommercialBasis)(nil)

// synRReceivableAccount 是与上面那份回显逐维相符的应收账户（隔离合成 `S`）：账户目录按（法人、相对方、应收、币种、
// 结算政策对象）查它。
func synRReceivableAccount(t *testing.T) sadomain.SettlementAccount {
	t.Helper()
	key, err := sadomain.NewSettlementAccountKey(
		mustPS(t, sadomain.NewLegalEntityReference, synRLegalEntity),
		mustPS(t, sadomain.NewSettlementCounterpartyReference, synRCounterparty),
		sadomain.ChargeReceivable,
		mustPS(t, sadomain.NewCurrencyCode, synRCurrency),
		mustPS(t, sadomain.NewSettlementPolicyReference, synRSettlementPolicy),
	)
	if err != nil {
		t.Fatalf("结算账户键：%v", err)
	}
	account, err := sadomain.NewSettlementAccount(
		mustPS(t, sadomain.NewSettlementAccountID, "SYN-ACCT-R1"),
		key,
		sadomain.SettlementCounterpartyReference{},
		false,
		mustPS(t, sadomain.NewResponsibilityBasis, "SYN-CONTRACT-R1"),
	)
	if err != nil {
		t.Fatalf("结算账户：%v", err)
	}
	return account
}
