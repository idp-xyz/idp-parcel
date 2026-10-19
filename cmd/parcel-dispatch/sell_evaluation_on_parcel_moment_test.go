package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	pcports "go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	saparcelshipment "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelshipment"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	saports "go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 本文件对真实 PostgreSQL 16 证票 sell-customer-charge/02 的装配判据：一封真 PS 适配器入队的收寄采用信封，经生产
// wireDispatcher 的路由表扇出到 SA 的计费时点消费者，走真的「包裹 → 结算账户」链（PS 委托仓储里接受时固定的商业解析
// → PC 解析库里闭包采用的结算政策 → SA 账户册）与真的采用册：这一户采用了`有效网络收寄已采用`时落一份 SELL 评价
// 请求、交一封请求信封；这一户没登记采用时入账、零行零信封。
//
// 已接受委托只能由 PS 应用编排形成，夹具复用 SYN-V0。商业解析 SYN-RES-01 的闭包由本文件种一份带结算政策的（SYN-PC-SEED
// 那一份不带结算政策，两份不能同标识共存），政策四维与 synRReceivableAccount 逐维相符。采用记录与它的信封经 PS 真仓储与
// 真交接适配器在一个事务里落，不手搓信封：两串事件类型是否对得上由这里钉。
//
// 同一封信还投给 network-routing 的路由复核。那一路在本夹具上停在它自己的格子，本用例不看它：FanOut 每路都调到、
// 两本 inbox 互不隶属，SA 那一路的入账与落行不随它回滚。全部隔离合成，证据只记 `S`。

const (
	sellOnNetworkIntakeConsumer = "settlement-accounting/request-sell-evaluation-on-network-intake"
	sellEvaluationRequestType   = "settlement-accounting.sell-evaluation-request.submitted"
	sellChainIntakeVersion      = "SYN-INTAKE-V1"
	sellChainChargeScope        = "SYN-CHARGE-01"
	sellChainContract           = "SYN-CONTRACT-01"
	sellChainPricingScope       = "SYN-PRICING-SCOPE-SELL-01"
)

// Covers: 票 02 完成判据（真库装配）——一户采用了 NETWORK_INTAKE_ADOPTED，一封收寄采用信封落恰一份请求、交恰一封
// 请求信封；请求各格取自链、采用册与采用记录；重拍不翻倍。
func TestANetworkIntakeEnvelopeRequestsOneSellEvaluationForAnAdoptingAccount(t *testing.T) {
	fixture := newSellChainFixture(t, true)
	ctx := t.Context()
	eventID, occurredAt := recordAdoptedNetworkIntake(t, fixture)

	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	assertSellLaneProcessed(t, fixture, eventID)
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 1 {
		t.Fatalf("sell_evaluation_request 行数 = %d, want 1", n)
	}
	if n := fixture.countOutboxOfType(t, sellEvaluationRequestType); n != 1 {
		t.Fatalf("请求信封 %d 封, want 1", n)
	}

	requests, err := sapostgres.NewSellEvaluationRequests(fixture.db)
	if err != nil {
		t.Fatalf("构造请求册：%v", err)
	}
	record, found, err := requests.FindCurrentByParcel(ctx,
		mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String()),
		mustPS(t, sadomain.NewParcelReference, "SYN-PARCEL-01"))
	if err != nil || !found {
		t.Fatalf("按包裹读请求：found=%v err=%v", found, err)
	}
	request := record.Request
	if request.Moment() != sadomain.BillingMomentNetworkIntakeAdopted || request.Account().String() != "SYN-ACCT-R1" ||
		request.SettlementCurrency().String() != synRCurrency || request.PricingScope().String() != sellChainPricingScope ||
		request.Source().String() != "network-intake/SYN-PARCEL-01/NODE_INTAKE/"+sellChainIntakeVersion ||
		!request.PricingBasisAt().Equal(occurredAt) || record.State != sadomain.SellRequestSubmitted {
		t.Fatalf("请求 = %+v state = %q", request, record.State)
	}
	ids := fixture.outboxIDsOfType(t, sellEvaluationRequestType)
	want := string(outboxintent.FingerprintEventID("sell-evaluation-request",
		fixture.identity.TenantID().String(), record.Key.Request.String(), "submitted"))
	if len(ids) != 1 || ids[0] != want {
		t.Fatalf("请求信封 ID = %v, want [%s]——信封不是这份请求的", ids, want)
	}

	// 收寄采用信封若因复核那一路未决而重投，SA 那一路由 inbox 跳过：行与信封都不翻倍。
	if _, err := fixture.beat.DispatchOnce(ctx); err != nil {
		t.Fatalf("重拍：%v", err)
	}
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 1 {
		t.Fatalf("重拍后 sell_evaluation_request 行数 = %d, want 仍为 1", n)
	}
	if n := fixture.countOutboxOfType(t, sellEvaluationRequestType); n != 1 {
		t.Fatalf("重拍后请求信封 %d 封, want 仍为 1", n)
	}
}

// Covers: 票 02 完成判据（真库装配）——账户在册、链走得通，但这一户没登记计费时点采用：SA 那一路入账（不是毒丸、不重投），
// 请求册零行，零请求信封。
func TestANetworkIntakeEnvelopeForAnAccountWithoutAdoptionIsAcknowledgedWithoutARequest(t *testing.T) {
	fixture := newSellChainFixture(t, false)
	eventID, _ := recordAdoptedNetworkIntake(t, fixture)

	if _, err := fixture.beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("第一拍：%v", err)
	}
	assertSellLaneProcessed(t, fixture, eventID)
	if n := fixture.countSQL(t, `SELECT count(*) FROM settlement_accounting.sell_evaluation_request`); n != 0 {
		t.Fatalf("没登记采用却落了 %d 份请求", n)
	}
	if n := fixture.countOutboxOfType(t, sellEvaluationRequestType); n != 0 {
		t.Fatalf("没登记采用却交了 %d 封请求信封", n)
	}
}

// newSellChainFixture 铺一条走得通的链：已接受委托 SYN-REQ-01（商业解析 SYN-RES-01）、带结算政策的闭包、应收账户；
// adopt 为真时这一户再登记采用`有效网络收寄已采用`。
func newSellChainFixture(t *testing.T, adopt bool) *synVerticalFixture {
	t.Helper()
	fixture := newSYNVerticalFixture(t)
	ctx := t.Context()
	fixture.submit(t, ctx)
	fixture.recordPassingJudgments(t, ctx)
	if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
		t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
	}
	seedSettledClosure(t, fixture)

	tenant := mustPS(t, sadomain.NewTenantID, fixture.identity.TenantID().String())
	accounts, err := sapostgres.NewSettlementAccounts(fixture.db)
	if err != nil {
		t.Fatalf("构造账户册：%v", err)
	}
	account := synRReceivableAccount(t)
	registeredAt := time.Date(2026, 10, 19, 8, 0, 0, 0, time.UTC)
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := accounts.Save(txCtx, tenant, account, registeredAt)
		return err
	})
	if !adopt {
		return fixture
	}
	adoptions, err := sapostgres.NewSellBillingMomentAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("构造采用册：%v", err)
	}
	adoption, err := sadomain.NewSellBillingMomentAdoption(account.ID(), sadomain.BillingMomentNetworkIntakeAdopted,
		mustPS(t, sadomain.NewEstimatePricingScope, sellChainPricingScope))
	if err != nil {
		t.Fatalf("采用行：%v", err)
	}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		_, err := adoptions.SaveSellBillingMomentAdoption(txCtx, tenant, adoption, registeredAt)
		return err
	})
	return fixture
}

// seedSettledClosure 给已接受委托回指的 SYN-RES-01 固定一份唯一已解析闭包，采用的结算政策四维与 synRReceivableAccount
// 逐维相符：法人、相对方、币种取政策的适用范围，账户键上的政策取对象标识。
func seedSettledClosure(t *testing.T, fixture *synVerticalFixture) {
	t.Helper()
	tenant := fixture.identity.TenantID().String()
	contract := sellChainVersion(t, tenant, pcdomain.CustomerContractObject, sellChainContract)
	rules := sellChainVersion(t, tenant, pcdomain.AcceptanceRulePackageObject, synPCRuleObject)
	policyVersion := sellChainVersion(t, tenant, pcdomain.SettlementPolicyObject, synRSettlementPolicy)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	applicability, err := pcdomain.NewSettlementApplicability(
		mustPC(t, pcdomain.NewLegalEntityReference, synRLegalEntity),
		mustPC(t, pcdomain.NewCounterpartyReference, synRCounterparty),
		mustPC(t, pcdomain.NewCommercialVersionLabel, sellChainContract+"/v1"),
		mustPC(t, pcdomain.NewChargeScopeReference, sellChainChargeScope),
		mustPC(t, pcdomain.NewCurrencyCode, synRCurrency),
		interval,
	)
	if err != nil {
		t.Fatalf("结算适用范围：%v", err)
	}
	policy, err := pcdomain.NewSettlementPolicy(policyVersion, pcdomain.TermsMethod, applicability)
	if err != nil {
		t.Fatalf("结算政策：%v", err)
	}
	anchor, err := pcdomain.NewSelectionAnchor(time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
		mustPC(t, pcdomain.NewAnchorPolicyVersion, "SYN-ANCHOR-POLICY-1"))
	if err != nil {
		t.Fatalf("选用锚点：%v", err)
	}
	closure, err := pcdomain.RehydrateCommercialClosure(pcdomain.RehydrateCommercialClosureSpec{
		Outcome:      pcdomain.UniquelyResolved,
		ResolutionID: mustPC(t, pcdomain.NewResolutionID, synPCResolutionID),
		Key: pcdomain.ClosureResolutionKey{
			TenantID:             mustPC(t, pcdomain.NewTenantID, tenant),
			CustomerAccountID:    mustPC(t, pcdomain.NewCustomerAccountID, fixture.identity.CustomerAccountID().String()),
			LegalEntityCandidate: mustPC(t, pcdomain.NewLegalEntityReference, synRLegalEntity),
			Scope:                mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-01"),
			Purpose:              pcdomain.AcceptanceControlPurpose,
			Anchor:               anchor,
			Settlement: pcdomain.SettlementSelector{
				Counterparty: mustPC(t, pcdomain.NewCounterpartyReference, synRCounterparty),
				ChargeScope:  mustPC(t, pcdomain.NewChargeScopeReference, sellChainChargeScope),
				Currency:     mustPC(t, pcdomain.NewCurrencyCode, synRCurrency),
			},
			RequiredBases: []pcdomain.CommercialObjectKind{
				pcdomain.CustomerContractObject,
				pcdomain.AcceptanceRulePackageObject,
				pcdomain.SettlementPolicyObject,
			},
		},
		Anchor:       anchor,
		ViewRevision: mustPC(t, pcdomain.NewAuthorityViewRevision, synPCViewRevision),
		Adopted: []pcdomain.RehydrateAdoptedBasisSpec{
			{Kind: pcdomain.CustomerContractObject, Version: contract},
			{Kind: pcdomain.AcceptanceRulePackageObject, Version: rules},
			{
				Kind:                pcdomain.SettlementPolicyObject,
				Version:             policyVersion,
				SettlementPolicy:    policy,
				HasSettlementPolicy: true,
			},
		},
	})
	if err != nil {
		t.Fatalf("重建带结算政策的闭包：%v", err)
	}
	store, err := pcpostgres.NewCommercialResolutions(fixture.db)
	if err != nil {
		t.Fatalf("构造解析库：%v", err)
	}
	var outcome pcports.ResolutionSaveOutcome
	mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
		var saveErr error
		outcome, saveErr = store.Save(txCtx, closure)
		return saveErr
	})
	if outcome != pcports.ResolutionSaved {
		t.Fatalf("save outcome = %s, want 已写入", outcome)
	}
}

func sellChainVersion(t *testing.T, tenant string, kind pcdomain.CommercialObjectKind, object string) pcdomain.CommercialVersion {
	t.Helper()
	approvedAt := time.Date(2026, 1, 2, 0, 0, 0, 0, time.UTC)
	interval, err := pcdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("有效区间：%v", err)
	}
	approval, err := pcdomain.NewApprovalBasis(
		mustPC(t, pcdomain.NewApprovalReference, "SYN-APPROVAL-"+object),
		mustPC(t, pcdomain.NewCommercialSourceReference, "SYN-SOURCE-"+object),
		approvedAt,
	)
	if err != nil {
		t.Fatalf("批准依据：%v", err)
	}
	version, err := pcdomain.RehydrateCommercialVersion(pcdomain.RehydrateCommercialVersionSpec{
		TenantID:      mustPC(t, pcdomain.NewTenantID, tenant),
		Kind:          kind,
		ObjectID:      mustPC(t, pcdomain.NewCommercialObjectID, object),
		Version:       mustPC(t, pcdomain.NewCommercialVersionLabel, "v1"),
		Scope:         mustPC(t, pcdomain.NewCommercialScopeReference, "SYN-PC-SCOPE-01"),
		ContentDigest: mustPC(t, pcdomain.NewCommercialContentDigest, "sha256:"+object),
		Effective:     interval,
		Status:        pcdomain.CommercialVersionEffective,
		Approval:      approval,
		PublishedAt:   approvedAt,
		EffectiveAt:   approvedAt.Add(time.Hour),
	})
	if err != nil {
		t.Fatalf("重建已生效版本 %s：%v", object, err)
	}
	return version
}

// recordAdoptedNetworkIntake 落一份`采用`格的有效网络收寄采用记录，并在同一事务经 PS 真交接适配器交出信封；交回信封 ID
// 与这份收寄的责任起点。采用记录挂在已接受委托当前提交版本上，客户与委托两维取夹具同一份。
func recordAdoptedNetworkIntake(t *testing.T, fixture *synVerticalFixture) (string, time.Time) {
	t.Helper()
	ctx := t.Context()
	store, err := outbox.NewStore(fixture.db)
	if err != nil {
		t.Fatalf("构造 Outbox Store：%v", err)
	}
	adoptions, err := pspostgres.NewIntakeAdoptions(fixture.db)
	if err != nil {
		t.Fatalf("构造采用仓储：%v", err)
	}
	handoff, err := pspostgres.NewOutboxNetworkIntakeHandoff(fixture.db, store, systemClock{})
	if err != nil {
		t.Fatalf("构造收寄采用交接：%v", err)
	}

	occurredAt := time.Date(2026, 10, 18, 7, 15, 0, 0, time.UTC)
	key := psports.IntakeAdoptionKey{
		TenantID: fixture.identity.TenantID(),
		Parcel:   mustPS(t, psdomain.NewDeclaredParcelID, "SYN-PARCEL-01"),
		Kind:     psdomain.NodeIntakeSource,
		Version:  mustPS(t, psdomain.NewSourceResultVersion, sellChainIntakeVersion),
	}
	source, err := psdomain.NewIntakeSource(psdomain.IntakeSourceSpec{
		Kind:       key.Kind,
		Object:     mustPS(t, psdomain.NewSourceObjectReference, "SYN-UNIT-01"),
		Parcel:     key.Parcel,
		Place:      mustPS(t, psdomain.NewIntakePlaceReference, "SYN-NODE-01"),
		Control:    mustPS(t, psdomain.NewIntakeControlReference, "SYN-CONTROL-01"),
		Version:    key.Version,
		OccurredAt: occurredAt,
	})
	if err != nil {
		t.Fatalf("收寄来源：%v", err)
	}
	baseline := fixture.mustLoadRequest(t, ctx).CurrentSubmissionVersion().VersionID()
	intake, err := psdomain.AdoptNetworkIntake(source, baseline)
	if err != nil {
		t.Fatalf("采用收寄：%v", err)
	}
	commitment, err := psdomain.FormFormalCommitment(
		mustPS(t, psdomain.NewCommitmentVersionID, "SYN-CMT-01"),
		intake,
		mustPS(t, psdomain.NewExpectedCommitmentReference, "SYN-RES-01@2026-10-18T00:00:00Z"),
	)
	if err != nil {
		t.Fatalf("正式承诺：%v", err)
	}
	record := psports.IntakeAdoptionRecord{
		Key:               key,
		CustomerAccountID: fixture.identity.CustomerAccountID(),
		ShipmentRequestID: fixture.requestID,
		ContentDigest:     "SYN-ADOPTION-DIGEST-01",
		Adopted:           true,
		Intake:            intake,
		Commitment:        commitment,
		AdoptedAt:         occurredAt.Add(time.Minute),
	}
	var saved psports.IntakeAdoptionSaveOutcome
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		var saveErr error
		if saved, saveErr = adoptions.Save(txCtx, record); saveErr != nil {
			return saveErr
		}
		return handoff.HandOffNetworkIntake(txCtx, psports.NetworkIntakeHandoffIntent{Record: record})
	})
	if saved != psports.IntakeAdoptionSaved {
		t.Fatalf("save outcome = %v, want 已写入", saved)
	}
	eventID := string(outboxintent.FingerprintEventID("network-intake",
		key.TenantID.String(), key.Parcel.String(), key.Kind.String(), key.Version.String()))
	if ids := fixture.outboxIDsOfType(t, networkIntakeRecordedType); len(ids) != 1 || ids[0] != eventID {
		t.Fatalf("收寄采用信封 = %v, want [%s]", ids, eventID)
	}
	return eventID, occurredAt
}

// assertSellLaneProcessed 钉 SA 那一路这封信已入账且是处理完毕，不是毒丸拒收——拒收同样入账，只数行分不出两者。
func assertSellLaneProcessed(t *testing.T, fixture *synVerticalFixture, eventID string) {
	t.Helper()
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatalf("取读执行器：%v", err)
	}
	var status string
	query := `SELECT status FROM ` + migrate.SchemaBento + `.inbox WHERE consumer = $1 AND event_id = $2`
	if err := querier.QueryRow(t.Context(), query, sellOnNetworkIntakeConsumer, eventID).Scan(&status); err != nil {
		t.Fatalf("SA 那一路的 inbox 行读不回（没入账或入了别的名）：%v", err)
	}
	if status != "PROCESSED" {
		t.Fatalf("SA 那一路 inbox status = %q, want PROCESSED", status)
	}
}

// Covers: 两种计费时点信封都扇出到 SA 那一路，与既有那一路各记各的 inbox 账。手法同路由表那几条：毒丸载荷让两路
// 消费门都显式拒收入账，这一封因而定稿；漏挂 SA 那一路时 SA 名下没有 inbox 行。
func TestBothParcelMomentEnvelopesFanOutToTheSettlementLane(t *testing.T) {
	for name, lane := range map[string]struct {
		eventType eventing.EventType
		consumer  string
	}{
		"收寄采用": {sainbox.NetworkIntakeRecordedEventType, sellOnNetworkIntakeConsumer},
		"终局":   {sainbox.FinalOutcomeFormedEventType, "settlement-accounting/request-sell-evaluation-on-final-outcome"},
	} {
		t.Run(name, func(t *testing.T) {
			beat, db, store := wiredBeat(t)
			enqueueForBeat(t, db, store, "moment-1", lane.eventType, `{}`)

			published, err := beat.DispatchOnce(t.Context())
			if err != nil {
				t.Fatalf("一拍：%v", err)
			}
			if published != 1 {
				t.Fatalf("published = %d, want 1；失败码 = %q", published, recordedFailureCode(t, db, "moment-1"))
			}
			querier, err := db.ReadExecutor(t.Context())
			if err != nil {
				t.Fatalf("取读执行器：%v", err)
			}
			var lanes int
			query := `SELECT count(*) FROM ` + migrate.SchemaBento + `.inbox WHERE event_id = $1`
			if err := querier.QueryRow(t.Context(), query, "moment-1").Scan(&lanes); err != nil {
				t.Fatalf("数 inbox：%v", err)
			}
			var settlement int
			query = `SELECT count(*) FROM ` + migrate.SchemaBento + `.inbox WHERE event_id = $1 AND consumer = $2`
			if err := querier.QueryRow(t.Context(), query, "moment-1", lane.consumer).Scan(&settlement); err != nil {
				t.Fatalf("数 SA inbox：%v", err)
			}
			if lanes != 2 || settlement != 1 {
				t.Fatalf("inbox 行 = %d（SA 名下 %d）, want 两路各一行", lanes, settlement)
			}
		})
	}
}

// Covers: 两路的未决名单只收续办那几枚——可见性滞后与发起编排未决在册；硬失败那几枚（仓储不变量已破、词汇表外、
// 封闭集外、请求信封被确定性拒收）一枚都不在。名单取生产切片本身，测试里重列一份会让误登记的哨兵一起消失。
func TestTheSettlementMomentLanesRegisterOnlyResumableSentinels(t *testing.T) {
	hard := []error{
		saparcelshipment.ErrAdoptionRecordInconsistent,
		saparcelshipment.ErrUntranslatableAnswer,
		saparcelshipment.ErrUnexpectedSellEvaluationRequestOutcome,
		fmt.Errorf("request sell evaluation: %w", saports.ErrSellEvaluationRequestHandoffRejected),
	}
	for name, lane := range map[string]struct {
		sentinels  []error
		resumables []error
	}{
		"收寄采用": {sellEvaluationOnNetworkIntakeUndecidedSentinels, []error{
			saparcelshipment.ErrIntakeAdoptionNotVisible, saparcelshipment.ErrSellEvaluationRequestUndecided,
		}},
		"终局": {sellEvaluationOnFinalOutcomeUndecidedSentinels, []error{
			saparcelshipment.ErrFinalOutcomeNotVisible, saparcelshipment.ErrSellEvaluationRequestUndecided,
		}},
	} {
		t.Run(name, func(t *testing.T) {
			listed := func(err error) bool {
				for _, sentinel := range lane.sentinels {
					if errors.Is(err, sentinel) {
						return true
					}
				}
				return false
			}
			for _, err := range lane.resumables {
				if !listed(fmt.Errorf("%w: syn", err)) {
					t.Fatalf("续办哨兵 %v 不在名单里——它会塌成 publish_failed", err)
				}
			}
			for _, err := range hard {
				if listed(err) {
					t.Fatalf("硬失败 %v 被登成了未决——重投不自愈，只会以未决之名耗尽失败预算", err)
				}
			}
		})
	}
}
