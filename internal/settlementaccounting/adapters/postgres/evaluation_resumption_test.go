package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0229 在库上的半边：请求册的代次链（首代、下一代指名前一代、一份至多一个下一代、自然键读最新一代、旧代
// 不被改写）、BUY 请求册的停因写口、两本发起停办册，以及跨四本册的续办队列。夹具全是合成串。

var resumptionAt = time.Date(2026, 9, 20, 9, 0, 0, 0, time.UTC)

type resumptionFixture struct {
	db           *bentopg.DB
	sellRequests *adapter.SellEvaluationRequests
	buyRequests  *adapter.EvaluationRequests
	resumptions  *adapter.EvaluationResumptions
	tenant       domain.TenantID
}

func newResumptionFixture(t *testing.T) resumptionFixture {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	sell, err := adapter.NewSellEvaluationRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	buy, err := adapter.NewEvaluationRequests(db)
	if err != nil {
		t.Fatal(err)
	}
	resumptions, err := adapter.NewEvaluationResumptions(db)
	if err != nil {
		t.Fatal(err)
	}
	return resumptionFixture{db: db, sellRequests: sell, buyRequests: buy, resumptions: resumptions, tenant: saTenant(t, "tenant-a")}
}

func (fixture resumptionFixture) within(t *testing.T, fn func(context.Context) error) {
	t.Helper()
	saWithin(t, fixture.db.Transactor(), t.Context(), fn)
}

func basisOf(t *testing.T, kind domain.EvaluationParameterKind, reference string) domain.ResumeBasis {
	t.Helper()
	basis, err := domain.NewParameterResumeBasis(kind, reference)
	if err != nil {
		t.Fatal(err)
	}
	return basis
}

// stopSell 给一份 SELL 请求记停下的结论。
func (fixture resumptionFixture) stopSell(t *testing.T, key ports.SellEvaluationRequestKey, cause domain.EvaluationStopCause) {
	t.Helper()
	conclusion, err := domain.NewStoppedSellEvaluationRequestConclusion(cause, resumptionAt)
	if err != nil {
		t.Fatal(err)
	}
	fixture.within(t, func(ctx context.Context) error {
		effect, err := fixture.sellRequests.RecordConclusion(ctx, key, conclusion)
		if err == nil && effect != ports.SellEvaluationRequestConcluded {
			return errors.New("not concluded: " + effect.String())
		}
		return err
	})
}

func (fixture resumptionFixture) list(t *testing.T, query ports.StalledEvaluationQuery) []ports.StalledEvaluation {
	t.Helper()
	items, err := fixture.resumptions.ListStalled(t.Context(), fixture.tenant, query, 50)
	if err != nil {
		t.Fatalf("取续办队列：%v", err)
	}
	return items
}

// Covers: AT-SA-190 SELL 请求册的代次在库上——停下的首代起下一代：自然键读回最新一代，按前一代找得到它；同一份首代的
// 第二个下一代撞前一代上的唯一约束答`已存在`；首代的结论与停因一个字不变；读回的下一代带代次、前一代与依据。
func TestASellRequestLineageKeepsThePreviousGenerationAndAllowsOneSuccessor(t *testing.T) {
	fixture := newResumptionFixture(t)
	first := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-1", "SYN-PARCEL-1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	if outcome := saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, first); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("首代 = %s", outcome)
	}
	fixture.stopSell(t, first.Key, domain.StopPriceCardNotConfigured)

	basis := basisOf(t, domain.ParameterPriceCard, "pricing-parameter/1")
	nextRequest, err := first.Request.Resume(saValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-2"), basis)
	if err != nil {
		t.Fatal(err)
	}
	next := ports.SellEvaluationRequestRecord{
		Key: ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: nextRequest.ID()}, Request: nextRequest,
		RecordedAt: sellRequestRecordedAt.Add(time.Hour), State: domain.SellRequestSubmitted,
	}
	if outcome := saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, next); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("下一代 = %s", outcome)
	}
	rival, err := first.Request.Resume(saValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-3"), basisOf(t, domain.ParameterPriceCard, "pricing-parameter/2"))
	if err != nil {
		t.Fatal(err)
	}
	if outcome := saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, ports.SellEvaluationRequestRecord{
		Key: ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: rival.ID()}, Request: rival,
		RecordedAt: sellRequestRecordedAt.Add(2 * time.Hour), State: domain.SellRequestSubmitted,
	}); outcome != ports.EvaluationRequestAlreadyRequested {
		t.Fatalf("同一份首代的第二个下一代 = %s，want 已存在", outcome)
	}

	latest, found, err := fixture.sellRequests.FindByNaturalKey(t.Context(), fixture.tenant, first.Request.NaturalKey())
	if err != nil || !found || latest.Key.Request.String() != "SELLREQ-SYN-2" || latest.Request.Generation() != 2 ||
		latest.Request.ResumeBasis() != basis {
		t.Fatalf("自然键读回 = %s 第 %d 代 found=%v err=%v", latest.Key.Request, latest.Request.Generation(), found, err)
	}
	if previous, has := latest.Request.Previous(); !has || previous.String() != "SELLREQ-SYN-1" {
		t.Fatalf("下一代的前一代 = %q %v", previous, has)
	}
	successor, found, err := fixture.sellRequests.FindSuccessor(t.Context(), first.Key)
	if err != nil || !found || successor.Key.Request.String() != "SELLREQ-SYN-2" {
		t.Fatalf("按前一代找下一代 = %s found=%v err=%v", successor.Key.Request, found, err)
	}
	if _, found, err := fixture.sellRequests.FindSuccessor(t.Context(), next.Key); err != nil || found {
		t.Fatalf("下一代自己没有下一代：found=%v err=%v", found, err)
	}
	old, _, err := fixture.sellRequests.FindByID(t.Context(), first.Key)
	if err != nil || old.State != domain.SellRequestNotFormed || old.StopCause != domain.StopPriceCardNotConfigured || old.Request.Generation() != 1 {
		t.Fatalf("首代被改写：%s / %s 第 %d 代 err=%v", old.State, old.StopCause, old.Request.Generation(), err)
	}
}

// Covers: AT-SA-190 的 BUY 半边与 AT-SA-192 的写口——BUY 请求册记停因只从「没有」往外走（重放、另有停因都不覆盖），
// 下一代同自然键、指名前一代，第二个下一代答`已存在`；写口无事务拒。
func TestABuyRequestRecordsItsStopOnceAndResumesIntoOneSuccessor(t *testing.T) {
	fixture := newResumptionFixture(t)
	first := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-1", "syn-fee-1")
	if outcome := saveEvaluationRequest(t, fixture.db.Transactor(), fixture.buyRequests, first); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("首代 = %s", outcome)
	}
	if _, err := fixture.buyRequests.RecordStop(t.Context(), first.Key, domain.StopEvaluationPendingJudgment, resumptionAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 RecordStop 应答 ErrTransactionRequired，实得：%v", err)
	}
	for _, step := range []struct {
		cause domain.EvaluationStopCause
		want  ports.EvaluationRequestStopEffect
	}{
		{domain.StopAmountRoundingUndeclared, ports.EvaluationRequestStopped},
		{domain.StopAmountRoundingUndeclared, ports.EvaluationRequestStopReplayed},
		{domain.StopEvaluationConflict, ports.EvaluationRequestStoppedOtherwise},
	} {
		fixture.within(t, func(ctx context.Context) error {
			effect, err := fixture.buyRequests.RecordStop(ctx, first.Key, step.cause, resumptionAt)
			if err == nil && effect != step.want {
				return errors.New("effect " + effect.String() + " want " + step.want.String())
			}
			return err
		})
	}
	fixture.within(t, func(ctx context.Context) error {
		if _, err := fixture.buyRequests.RecordStop(ctx, first.Key, domain.StopSettlementAccountNotRegistered, resumptionAt); err == nil {
			return errors.New("发起之前的停因记进了 BUY 请求册")
		}
		return nil
	})

	basis := basisOf(t, domain.ParameterPriceCard, "pricing-parameter/1")
	requester := saValue(t, domain.NewRequesterReference, "settlement-accounting/evaluation-resumption")
	nextRequest, err := first.Request.Resume(saValue(t, domain.NewEvaluationRequestID, "EVREQ-SYN-2"), basis, resumptionAt, requester)
	if err != nil {
		t.Fatal(err)
	}
	next := ports.EvaluationRequestRecord{
		Key: ports.EvaluationRequestKey{TenantID: fixture.tenant, Request: nextRequest.ID()}, Request: nextRequest, RecordedAt: resumptionAt,
	}
	if outcome := saveEvaluationRequest(t, fixture.db.Transactor(), fixture.buyRequests, next); outcome != ports.EvaluationRequestSaved {
		t.Fatalf("下一代 = %s", outcome)
	}
	rivalRequest, err := first.Request.Resume(saValue(t, domain.NewEvaluationRequestID, "EVREQ-SYN-3"), basis, resumptionAt, requester)
	if err != nil {
		t.Fatal(err)
	}
	if outcome := saveEvaluationRequest(t, fixture.db.Transactor(), fixture.buyRequests, ports.EvaluationRequestRecord{
		Key: ports.EvaluationRequestKey{TenantID: fixture.tenant, Request: rivalRequest.ID()}, Request: rivalRequest, RecordedAt: resumptionAt,
	}); outcome != ports.EvaluationRequestAlreadyRequested {
		t.Fatalf("第二个下一代 = %s，want 已存在", outcome)
	}
	latest, found, err := fixture.buyRequests.FindByNaturalKey(t.Context(), fixture.tenant, first.Request.NaturalKey())
	if err != nil || !found || latest.Key.Request.String() != "EVREQ-SYN-2" || latest.StopCause != domain.EvaluationStopCauseInvalid {
		t.Fatalf("自然键读回 = %s / %s found=%v err=%v", latest.Key.Request, latest.StopCause, found, err)
	}
	old, _, err := fixture.buyRequests.FindByID(t.Context(), first.Key)
	if err != nil || old.StopCause != domain.StopAmountRoundingUndeclared {
		t.Fatalf("首代停因 = %s err=%v，want 原样", old.StopCause, err)
	}
	if successor, found, err := fixture.buyRequests.FindSuccessor(t.Context(), first.Key); err != nil || !found || successor.Key != next.Key {
		t.Fatalf("按前一代找下一代 found=%v err=%v", found, err)
	}
}

// Covers: AT-SA-191 两本发起停办册——落一行、按自然键读回原样；同一自然键再停一次换上这一次的停因、时刻、依据与户，来源
// 事实不换；只有停在计费时点没采用的那一拍记户；写口无事务拒。
func TestInitiationStallsRecordTheMomentAndTakeTheLatestStop(t *testing.T) {
	fixture := newResumptionFixture(t)
	key := domain.SellEvaluationRequestNaturalKey{
		Parcel: saValue(t, domain.NewParcelReference, "SYN-PARCEL-9"), Moment: domain.BillingMomentNetworkIntakeAdopted,
	}
	stall := func(cause domain.EvaluationStopCause, at time.Time, basis domain.ResumeBasis, account domain.SettlementAccountID) domain.SellEvaluationInitiationStall {
		built, err := domain.NewSellEvaluationInitiationStall(domain.SellEvaluationInitiationStallSpec{
			Parcel: key.Parcel, Moment: key.Moment, Source: saValue(t, domain.NewSourceFactReference, "SYN-INTAKE-9"),
			PricingBasisAt: sellRequestBasisAt, Cause: cause, StoppedAt: at, Basis: basis, Account: account,
		})
		if err != nil {
			t.Fatal(err)
		}
		return built
	}
	unregistered := stall(domain.StopSettlementAccountNotRegistered, resumptionAt, domain.ResumeBasis{}, domain.SettlementAccountID{})
	if err := fixture.resumptions.RecordSellInitiationStall(t.Context(), fixture.tenant, unregistered); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外记 SELL 发起停办应答 ErrTransactionRequired，实得：%v", err)
	}
	fixture.within(t, func(ctx context.Context) error {
		return fixture.resumptions.RecordSellInitiationStall(ctx, fixture.tenant, unregistered)
	})
	if loaded, _, err := fixture.resumptions.FindSellInitiationStall(t.Context(), fixture.tenant, key); err != nil {
		t.Fatal(err)
	} else if _, reached := loaded.Account(); reached {
		t.Fatalf("账户没登的那一拍记了户：%+v", loaded)
	}
	basis := basisOf(t, domain.ParameterSettlementAccount, "settlement-account/1")
	notAdopted := stall(domain.StopSellBillingMomentNotAdopted, resumptionAt.Add(time.Hour), basis,
		saValue(t, domain.NewSettlementAccountID, "SYN-ACCT-9"))
	fixture.within(t, func(ctx context.Context) error {
		return fixture.resumptions.RecordSellInitiationStall(ctx, fixture.tenant, notAdopted)
	})
	loaded, found, err := fixture.resumptions.FindSellInitiationStall(t.Context(), fixture.tenant, key)
	account, reached := loaded.Account()
	if err != nil || !found || loaded.Cause() != domain.StopSellBillingMomentNotAdopted || loaded.Basis() != basis ||
		!loaded.StoppedAt().Equal(resumptionAt.Add(time.Hour)) || loaded.Source().String() != "SYN-INTAKE-9" ||
		!loaded.PricingBasisAt().Equal(sellRequestBasisAt) || !reached || account.String() != "SYN-ACCT-9" {
		t.Fatalf("读回 = %+v found=%v err=%v", loaded, found, err)
	}

	occurrence, err := domain.NewTransportChargeOccurrence(
		saValue(t, domain.NewChargeOccurrenceID, "SYN-OCC-9"), saValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
		saValue(t, domain.NewOccurrenceVersion, "V1"), evaluationOccurredAt)
	if err != nil {
		t.Fatal(err)
	}
	buyStall, err := domain.NewBuyEvaluationInitiationStall(domain.BuyEvaluationInitiationStallSpec{
		Occurrence: occurrence, Agreement: saValue(t, domain.NewSupplierAgreementReference, "SYN-AGR/v1"),
		Cause: domain.StopTriggerFeeItemNotRegistered, StoppedAt: resumptionAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := fixture.resumptions.RecordBuyInitiationStall(t.Context(), fixture.tenant, buyStall); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外记 BUY 发起停办应答 ErrTransactionRequired，实得：%v", err)
	}
	fixture.within(t, func(ctx context.Context) error {
		return fixture.resumptions.RecordBuyInitiationStall(ctx, fixture.tenant, buyStall)
	})
	loadedBuy, found, err := fixture.resumptions.FindBuyInitiationStall(t.Context(), fixture.tenant, occurrence.ID(), occurrence.Version())
	if err != nil || !found || loadedBuy.Cause() != domain.StopTriggerFeeItemNotRegistered || loadedBuy.Occurrence() != occurrence ||
		loadedBuy.Agreement().String() != "SYN-AGR/v1" {
		t.Fatalf("BUY 读回 = %+v found=%v err=%v", loadedBuy, found, err)
	}
}

// Covers: AT-SA-193 续办队列的口径——发起停办在这一拍还没有请求时等着、有了请求就出队；停下的请求在还没有下一代时等着、
// 有了下一代就出队；只取停因在名单里的；凭被排除的依据续办过的不取；计价范围在场时只取问这个范围的请求、不取发起
// 停办；点名包裹只取 SELL 两支。另一个租户的不取。
func TestTheStalledQueueListsWhatStillWaits(t *testing.T) {
	fixture := newResumptionFixture(t)
	// SELL 发起停办：账户没登。
	sellStall, err := domain.NewSellEvaluationInitiationStall(domain.SellEvaluationInitiationStallSpec{
		Parcel: saValue(t, domain.NewParcelReference, "SYN-PARCEL-INIT"), Moment: domain.BillingMomentNetworkIntakeAdopted,
		Source: saValue(t, domain.NewSourceFactReference, "SYN-INTAKE-INIT"), PricingBasisAt: sellRequestBasisAt,
		Cause: domain.StopSettlementAccountNotRegistered, StoppedAt: resumptionAt,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.within(t, func(ctx context.Context) error {
		return fixture.resumptions.RecordSellInitiationStall(ctx, fixture.tenant, sellStall)
	})
	// SELL 请求停在没卡；BUY 请求停在取整未声明。
	sellRecord := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-Q", "SYN-PARCEL-Q",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, sellRecord)
	fixture.stopSell(t, sellRecord.Key, domain.StopPriceCardNotConfigured)
	buyRecord := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-Q", "syn-fee-q")
	saveEvaluationRequest(t, fixture.db.Transactor(), fixture.buyRequests, buyRecord)
	fixture.within(t, func(ctx context.Context) error {
		_, err := fixture.buyRequests.RecordStop(ctx, buyRecord.Key, domain.StopAmountRoundingUndeclared, resumptionAt.Add(time.Minute))
		return err
	})

	kinds := func(items []ports.StalledEvaluation) map[ports.StalledEvaluationKind]int {
		counts := map[ports.StalledEvaluationKind]int{}
		for _, item := range items {
			counts[item.Kind]++
		}
		return counts
	}
	all := fixture.list(t, ports.StalledEvaluationQuery{Causes: domain.AllEvaluationStopCauses()})
	if got := kinds(all); got[ports.StalledSellInitiation] != 1 || got[ports.StalledSellRequest] != 1 || got[ports.StalledBuyRequest] != 1 || len(all) != 3 {
		t.Fatalf("全集 = %+v", all)
	}
	if all[0].Kind != ports.StalledSellInitiation && all[0].Kind != ports.StalledSellRequest {
		t.Fatalf("队首 = %s，want 停得最早的那一件", all[0].Kind)
	}
	priceCard := fixture.list(t, ports.StalledEvaluationQuery{
		Causes: domain.EvaluationStopCausesResumedBy(domain.ParameterPriceCard), PricingScope: "SYN-SCOPE-SELL-1",
	})
	if len(priceCard) != 1 || priceCard[0].Kind != ports.StalledSellRequest || priceCard[0].SellRequest != sellRecord.Key.Request ||
		priceCard[0].Cause != domain.StopPriceCardNotConfigured {
		t.Fatalf("价卡、范围 SYN-SCOPE-SELL-1 = %+v", priceCard)
	}
	if got := fixture.list(t, ports.StalledEvaluationQuery{
		Causes: domain.EvaluationStopCausesResumedBy(domain.ParameterPriceCard), PricingScope: "syn-scope-1",
	}); len(got) != 1 || got[0].Kind != ports.StalledBuyRequest {
		t.Fatalf("价卡、范围 syn-scope-1 = %+v", got)
	}
	if got := fixture.list(t, ports.StalledEvaluationQuery{
		Causes: domain.EvaluationStopCausesResumedBy(domain.ParameterSettlementAccount),
	}); len(got) != 1 || got[0].Kind != ports.StalledSellInitiation || got[0].SellMoment != sellStall.NaturalKey() {
		t.Fatalf("账户登记 = %+v", got)
	}
	if got := fixture.list(t, ports.StalledEvaluationQuery{
		Causes: domain.AllEvaluationStopCauses(), Parcel: saValue(t, domain.NewParcelReference, "SYN-PARCEL-Q"),
	}); len(got) != 1 || got[0].Kind != ports.StalledSellRequest {
		t.Fatalf("点名包裹 = %+v", got)
	}
	if got, err := fixture.resumptions.ListStalled(t.Context(), saTenant(t, "tenant-b"),
		ports.StalledEvaluationQuery{Causes: domain.AllEvaluationStopCauses()}, 50); err != nil || len(got) != 0 {
		t.Fatalf("另一个租户 = %+v err=%v", got, err)
	}
	if _, err := fixture.resumptions.ListStalled(t.Context(), fixture.tenant, ports.StalledEvaluationQuery{}, 50); !errors.Is(err, ports.ErrStalledEvaluationQueryInvalid) {
		t.Fatalf("空停因名单 err = %v", err)
	}

	// 发起停办重跑之后又停下、换上了依据：凭同一个依据不再取，凭别的依据照取。
	basis := basisOf(t, domain.ParameterSettlementAccount, "settlement-account/9")
	restalled, err := domain.NewSellEvaluationInitiationStall(domain.SellEvaluationInitiationStallSpec{
		Parcel: sellStall.NaturalKey().Parcel, Moment: sellStall.NaturalKey().Moment, Source: sellStall.Source(),
		PricingBasisAt: sellStall.PricingBasisAt(), Cause: domain.StopSettlementAccountNotRegistered,
		StoppedAt: resumptionAt.Add(time.Hour), Basis: basis,
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture.within(t, func(ctx context.Context) error {
		return fixture.resumptions.RecordSellInitiationStall(ctx, fixture.tenant, restalled)
	})
	accountCauses := domain.EvaluationStopCausesResumedBy(domain.ParameterSettlementAccount)
	if got := fixture.list(t, ports.StalledEvaluationQuery{Causes: accountCauses, ExcludeBasis: basis}); len(got) != 0 {
		t.Fatalf("凭同一个依据又取到了 %+v", got)
	}
	if got := fixture.list(t, ports.StalledEvaluationQuery{
		Causes: accountCauses, ExcludeBasis: basisOf(t, domain.ParameterSettlementAccount, "settlement-account/10"),
	}); len(got) != 1 {
		t.Fatalf("凭别的依据 = %+v", got)
	}

	// 这一拍有了请求、停下的请求有了下一代：都出队。
	initRequest := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-INIT", "SYN-PARCEL-INIT",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt.Add(time.Hour))
	saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, initRequest)
	nextRequest, err := sellRecord.Request.Resume(saValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-Q2"),
		basisOf(t, domain.ParameterPriceCard, "pricing-parameter/9"))
	if err != nil {
		t.Fatal(err)
	}
	saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, ports.SellEvaluationRequestRecord{
		Key: ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: nextRequest.ID()}, Request: nextRequest,
		RecordedAt: sellRequestRecordedAt.Add(2 * time.Hour), State: domain.SellRequestSubmitted,
	})
	if got := kinds(fixture.list(t, ports.StalledEvaluationQuery{Causes: domain.AllEvaluationStopCauses()})); got[ports.StalledSellInitiation] != 0 ||
		got[ports.StalledSellRequest] != 0 || got[ports.StalledBuyRequest] != 1 {
		t.Fatalf("续办之后的队列 = %+v", got)
	}
}

// Covers: AT-SA-199 「评价参数已登记」交接口——同一行重放发同一封（EnqueueOnce 按 ID 吞掉第二封）、两行两封；种类不归本口拒；
// 收窄格随种类必带（触发册带发生项原因、计费时点采用带户、映射两格都空），缺了或带错拒；写口无事务拒（PBC-08）。
func TestTheEvaluationParameterHandoffEnqueuesOneEnvelopePerRegisteredRow(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := outbox.NewStore(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := adapter.NewOutboxEvaluationParameterRegisteredHandoff(db, store, saHandoffClock{at: resumptionAt})
	if err != nil {
		t.Fatal(err)
	}
	intent := ports.EvaluationParameterRegisteredIntent{
		Tenant: saTenant(t, "tenant-a"), Kind: domain.ParameterFeeItemMapping, Subject: "BASE_FREIGHT", RegisteredAt: resumptionAt,
	}
	if err := handoff.HandOffEvaluationParameterRegistered(t.Context(), intent); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外交信封应答 ErrTransactionRequired，实得：%v", err)
	}
	other := intent
	other.Subject = "REMOTE_AREA_SURCHARGE"
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		for _, each := range []ports.EvaluationParameterRegisteredIntent{intent, intent, other} {
			if err := handoff.HandOffEvaluationParameterRegistered(ctx, each); err != nil {
				return err
			}
		}
		return nil
	})
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var count int
	if err := querier.QueryRow(t.Context(), `SELECT count(*) FROM `+migrate.SchemaBento+`.outbox
		WHERE event_type = 'settlement-accounting.evaluation-parameter.registered' AND partition_key = 'tenant-a'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 2 {
		t.Fatalf("评价参数已登记 %d 封，want 两行两封、重放不翻倍", count)
	}
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		wrong := intent
		wrong.Kind = domain.ParameterPriceCard
		if err := handoff.HandOffEvaluationParameterRegistered(ctx, wrong); err == nil {
			return errors.New("计价的种类经结算的口交了出去")
		}
		trigger := ports.EvaluationParameterRegisteredIntent{
			Tenant: saTenant(t, "tenant-a"), Kind: domain.ParameterBuyEvaluationTrigger,
			Subject: "ACTUAL_FULFILLMENT@SYN-FEE", RegisteredAt: resumptionAt,
		}
		if err := handoff.HandOffEvaluationParameterRegistered(ctx, trigger); err == nil {
			return errors.New("不带发生项原因的触发册登记交了出去")
		}
		trigger.OccurrenceReason = saValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT")
		adoption := ports.EvaluationParameterRegisteredIntent{
			Tenant: saTenant(t, "tenant-a"), Kind: domain.ParameterSellBillingMomentAdoption,
			Subject: "SYN-ACCT-1", Account: saValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1"), RegisteredAt: resumptionAt,
		}
		mappingWithAccount := intent
		mappingWithAccount.Account = adoption.Account
		if err := handoff.HandOffEvaluationParameterRegistered(ctx, mappingWithAccount); err == nil {
			return errors.New("带户的映射登记交了出去")
		}
		for _, each := range []ports.EvaluationParameterRegisteredIntent{trigger, adoption} {
			if err := handoff.HandOffEvaluationParameterRegistered(ctx, each); err != nil {
				return err
			}
		}
		return nil
	})
	payloads := map[string]map[string]any{}
	rows, err := querier.Query(t.Context(), `SELECT payload FROM `+migrate.SchemaBento+`.outbox
		WHERE event_type = 'settlement-accounting.evaluation-parameter.registered'`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var raw []byte
		if err := rows.Scan(&raw); err != nil {
			t.Fatal(err)
		}
		body := map[string]any{}
		if err := json.Unmarshal(raw, &body); err != nil {
			t.Fatal(err)
		}
		payloads[fmt.Sprint(body["registrationKind"])] = body
	}
	if payloads["BUY_EVALUATION_TRIGGER"]["occurrenceReason"] != "ACTUAL_FULFILLMENT" ||
		payloads["SELL_BILLING_MOMENT_ADOPTION"]["settlementAccountId"] != "SYN-ACCT-1" {
		t.Fatalf("收窄格 = %+v", payloads)
	}
	if _, carried := payloads["FEE_ITEM_MAPPING"]["settlementAccountId"]; carried {
		t.Fatalf("映射带了户：%+v", payloads["FEE_ITEM_MAPPING"])
	}
}

// Covers: AT-SA-193 的翻页与整链排除——按位置往后取：同一组行第二页接着第一页、不重不漏；排除的依据查整条代次链：
// 一条链先凭 B 续办出第二代、再凭 C 续办出又停下的第三代，排除 B 时第三代不取，排除别的依据照取。
func TestTheStalledQueuePagesByPositionAndExcludesABasisAcrossTheWholeChain(t *testing.T) {
	fixture := newResumptionFixture(t)
	for index, parcel := range []string{"SYN-PARCEL-P1", "SYN-PARCEL-P2", "SYN-PARCEL-P3"} {
		record := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-P"+string(rune('1'+index)), parcel,
			domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
		saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, record)
		fixture.stopSell(t, record.Key, domain.StopEvaluationPendingJudgment)
	}
	causes := domain.EvaluationStopCausesResumedBy(domain.ParameterReferenceSeries)
	first, err := fixture.resumptions.ListStalled(t.Context(), fixture.tenant, ports.StalledEvaluationQuery{Causes: causes}, 2)
	if err != nil || len(first) != 2 {
		t.Fatalf("第一页 = %+v err=%v", first, err)
	}
	second, err := fixture.resumptions.ListStalled(t.Context(), fixture.tenant,
		ports.StalledEvaluationQuery{Causes: causes, After: first[1].Position}, 2)
	if err != nil || len(second) != 1 {
		t.Fatalf("第二页 = %+v err=%v", second, err)
	}
	seen := map[string]bool{}
	for _, item := range append(first, second...) {
		seen[item.SellRequest.String()] = true
	}
	if len(seen) != 3 {
		t.Fatalf("两页合起来 = %v，want 三件不重不漏", seen)
	}

	// 整链排除：P1 那条链 B → 第二代、C → 第三代，第三代又停下。
	chainHead := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-P1", "SYN-PARCEL-P1",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	b := basisOf(t, domain.ParameterPriceCard, "pricing-parameter/B")
	c := basisOf(t, domain.ParameterFeeItemMapping, "evaluation-parameter/C")
	second2, err := chainHead.Request.Resume(saValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-P1-2"), b)
	if err != nil {
		t.Fatal(err)
	}
	record2 := ports.SellEvaluationRequestRecord{Key: ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: second2.ID()},
		Request: second2, RecordedAt: sellRequestRecordedAt.Add(time.Hour), State: domain.SellRequestSubmitted}
	saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, record2)
	fixture.stopSell(t, record2.Key, domain.StopFeeItemMappingIncomplete)
	third, err := second2.Resume(saValue(t, domain.NewSellEvaluationRequestID, "SELLREQ-SYN-P1-3"), c)
	if err != nil {
		t.Fatal(err)
	}
	record3 := ports.SellEvaluationRequestRecord{Key: ports.SellEvaluationRequestKey{TenantID: fixture.tenant, Request: third.ID()},
		Request: third, RecordedAt: sellRequestRecordedAt.Add(2 * time.Hour), State: domain.SellRequestSubmitted}
	saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, record3)
	fixture.stopSell(t, record3.Key, domain.StopEvaluationFailed)

	listed := func(exclude domain.ResumeBasis) bool {
		items := fixture.list(t, ports.StalledEvaluationQuery{
			Causes: domain.AllEvaluationStopCauses(), ExcludeBasis: exclude, Parcel: saValue(t, domain.NewParcelReference, "SYN-PARCEL-P1"),
		})
		for _, item := range items {
			if item.SellRequest.String() == "SELLREQ-SYN-P1-3" {
				return true
			}
		}
		return false
	}
	if listed(b) {
		t.Fatal("链上第二代凭 B 续办过，排除 B 时第三代又被取到")
	}
	if listed(c) {
		t.Fatal("第三代自己凭 C 续办出来，排除 C 时又被取到")
	}
	if !listed(basisOf(t, domain.ParameterPriceCard, "pricing-parameter/D")) {
		t.Fatal("排除一个链上没用过的依据，第三代却不在队列里")
	}
}

// Covers: AT-SA-193 的收窄与终止——方向只取那一路；发生项原因只取 BUY 那一路里原因是它的；户只取 SELL 那一路里落在这一户
// 的；落了终止的 SELL 发起停办不再进队列、终止重放不改时刻、不在册的键答没收，再停一次又进队列；落终止要求环境事务。
func TestTheStalledQueueNarrowsByDirectionReasonAndAccountAndSkipsClosedStalls(t *testing.T) {
	fixture := newResumptionFixture(t)
	sellStall := func(parcel, account string) domain.SellEvaluationInitiationStall {
		built, err := domain.NewSellEvaluationInitiationStall(domain.SellEvaluationInitiationStallSpec{
			Parcel: saValue(t, domain.NewParcelReference, parcel), Moment: domain.BillingMomentNetworkIntakeAdopted,
			Source: saValue(t, domain.NewSourceFactReference, "SYN-INTAKE-"+parcel), PricingBasisAt: sellRequestBasisAt,
			Cause: domain.StopSellBillingMomentNotAdopted, StoppedAt: resumptionAt,
			Account: saValue(t, domain.NewSettlementAccountID, account),
		})
		if err != nil {
			t.Fatal(err)
		}
		return built
	}
	buyStall := func(occurrence, reason string) domain.BuyEvaluationInitiationStall {
		formed, err := domain.NewTransportChargeOccurrence(saValue(t, domain.NewChargeOccurrenceID, occurrence),
			saValue(t, domain.NewOccurrenceReasonReference, reason), saValue(t, domain.NewOccurrenceVersion, "V1"), evaluationOccurredAt)
		if err != nil {
			t.Fatal(err)
		}
		built, err := domain.NewBuyEvaluationInitiationStall(domain.BuyEvaluationInitiationStallSpec{
			Occurrence: formed, Agreement: saValue(t, domain.NewSupplierAgreementReference, "SYN-AGR/v1"),
			Cause: domain.StopOccurrenceReasonNotRegistered, StoppedAt: resumptionAt,
		})
		if err != nil {
			t.Fatal(err)
		}
		return built
	}
	parcelA, parcelB := sellStall("SYN-PARCEL-A", "SYN-ACCT-A"), sellStall("SYN-PARCEL-B", "SYN-ACCT-B")
	fixture.within(t, func(ctx context.Context) error {
		for _, stall := range []domain.SellEvaluationInitiationStall{parcelA, parcelB} {
			if err := fixture.resumptions.RecordSellInitiationStall(ctx, fixture.tenant, stall); err != nil {
				return err
			}
		}
		for _, stall := range []domain.BuyEvaluationInitiationStall{
			buyStall("SYN-OCC-FULFIL", "ACTUAL_FULFILLMENT"), buyStall("SYN-OCC-RETURN", "SYN_RETURN_HANDLING"),
		} {
			if err := fixture.resumptions.RecordBuyInitiationStall(ctx, fixture.tenant, stall); err != nil {
				return err
			}
		}
		return nil
	})
	sellRecord := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-N", "SYN-PARCEL-N",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, sellRecord)
	fixture.stopSell(t, sellRecord.Key, domain.StopPriceCardNotConfigured)
	buyRecord := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-N", "syn-fee-n")
	saveEvaluationRequest(t, fixture.db.Transactor(), fixture.buyRequests, buyRecord)
	fixture.within(t, func(ctx context.Context) error {
		_, err := fixture.buyRequests.RecordStop(ctx, buyRecord.Key, domain.StopPriceCardNotConfigured, resumptionAt)
		return err
	})

	describe := func(items []ports.StalledEvaluation) map[string]bool {
		got := map[string]bool{}
		for _, item := range items {
			got[item.Kind.String()+"/"+item.Position.FirstKey] = true
		}
		return got
	}
	expect := func(label string, query ports.StalledEvaluationQuery, want ...string) {
		t.Helper()
		got := describe(fixture.list(t, query))
		if len(got) != len(want) {
			t.Fatalf("%s = %v, want %v", label, got, want)
		}
		for _, each := range want {
			if !got[each] {
				t.Fatalf("%s = %v, want %v", label, got, want)
			}
		}
	}
	all := domain.AllEvaluationStopCauses()
	expect("BUY 一路", ports.StalledEvaluationQuery{Causes: all, Direction: domain.BuyEvaluationDirection},
		"BUY_INITIATION/SYN-OCC-FULFIL", "BUY_INITIATION/SYN-OCC-RETURN", "BUY_REQUEST/EVREQ-SYN-N")
	expect("SELL 一路", ports.StalledEvaluationQuery{Causes: all, Direction: domain.SellEvaluationDirection},
		"SELL_INITIATION/SYN-PARCEL-A", "SELL_INITIATION/SYN-PARCEL-B", "SELL_REQUEST/SELLREQ-SYN-N")
	priceCard := domain.EvaluationStopCausesResumedBy(domain.ParameterPriceCard)
	expect("BUY 卡、SELL 请求的范围", ports.StalledEvaluationQuery{
		Causes: priceCard, PricingScope: "SYN-SCOPE-SELL-1", Direction: domain.BuyEvaluationDirection,
	})
	expect("采用登记、户 A", ports.StalledEvaluationQuery{
		Causes:  domain.EvaluationStopCausesResumedBy(domain.ParameterSellBillingMomentAdoption),
		Account: saValue(t, domain.NewSettlementAccountID, "SYN-ACCT-A"),
	}, "SELL_INITIATION/SYN-PARCEL-A")
	expect("户 SYN-ACCT-1、不分停因", ports.StalledEvaluationQuery{
		Causes: all, Account: saValue(t, domain.NewSettlementAccountID, "SYN-ACCT-1"),
	}, "SELL_REQUEST/SELLREQ-SYN-N")
	expect("触发册登记、原因 ACTUAL_FULFILLMENT", ports.StalledEvaluationQuery{
		Causes:           domain.EvaluationStopCausesResumedBy(domain.ParameterBuyEvaluationTrigger),
		OccurrenceReason: saValue(t, domain.NewOccurrenceReasonReference, "ACTUAL_FULFILLMENT"),
	}, "BUY_INITIATION/SYN-OCC-FULFIL")
	expect("原因 BOOKING、不分停因", ports.StalledEvaluationQuery{
		Causes: all, OccurrenceReason: saValue(t, domain.NewOccurrenceReasonReference, "BOOKING"),
	}, "BUY_REQUEST/EVREQ-SYN-N")

	adoptionCauses := domain.EvaluationStopCausesResumedBy(domain.ParameterSellBillingMomentAdoption)
	closedAt := resumptionAt.Add(2 * time.Hour)
	if _, err := fixture.resumptions.CloseSellInitiationStall(t.Context(), fixture.tenant, parcelA.NaturalKey(), closedAt); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外落终止应答 ErrTransactionRequired，实得：%v", err)
	}
	closeStall := func(key domain.SellEvaluationRequestNaturalKey, at time.Time) bool {
		var closed bool
		fixture.within(t, func(ctx context.Context) error {
			var err error
			closed, err = fixture.resumptions.CloseSellInitiationStall(ctx, fixture.tenant, key, at)
			return err
		})
		return closed
	}
	if !closeStall(parcelA.NaturalKey(), closedAt) || !closeStall(parcelA.NaturalKey(), closedAt.Add(time.Hour)) {
		t.Fatal("在册的那一拍答没收")
	}
	if closeStall(domain.SellEvaluationRequestNaturalKey{
		Parcel: saValue(t, domain.NewParcelReference, "SYN-PARCEL-NONE"), Moment: domain.BillingMomentNetworkIntakeAdopted,
	}, closedAt) {
		t.Fatal("不在册的键答收了")
	}
	expect("户 A 落终止之后", ports.StalledEvaluationQuery{Causes: adoptionCauses}, "SELL_INITIATION/SYN-PARCEL-B")
	querier, err := fixture.db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var recordedClose time.Time
	if err := querier.QueryRow(t.Context(), `SELECT closed_at FROM settlement_accounting.sell_evaluation_initiation_stall
		WHERE tenant_id = 'tenant-a' AND parcel_id = 'SYN-PARCEL-A'`).Scan(&recordedClose); err != nil {
		t.Fatal(err)
	}
	if !recordedClose.Equal(closedAt) {
		t.Fatalf("终止时刻 = %s，want 第一次的 %s", recordedClose, closedAt)
	}
	fixture.within(t, func(ctx context.Context) error {
		return fixture.resumptions.RecordSellInitiationStall(ctx, fixture.tenant, parcelA)
	})
	expect("户 A 再停一次", ports.StalledEvaluationQuery{Causes: adoptionCauses},
		"SELL_INITIATION/SYN-PARCEL-A", "SELL_INITIATION/SYN-PARCEL-B")
}
