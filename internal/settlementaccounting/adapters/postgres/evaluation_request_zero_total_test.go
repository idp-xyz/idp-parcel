package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

// 钉 ADR-0306 决定二、三在库上的半边（票 demo-chain-gaps/133）：两本评价请求册上的`负向结束（零额）`——SELL 册的状态格
// ZERO_TOTAL 不带费用标识与停因，BUY 册的 negative_end 一列与停因互斥；两种去向互不覆盖，重放认得出；不带停因，续办队列
// 取不到它。夹具全是合成串。

// Covers: AT-SA-215 的 SELL 册——`已提交` → `负向结束（零额）`写下，读回不带费用标识与停因；同一份再记是重放，换成停下的
// 一格不覆盖；库上的 CHECK 拒零额带停因或带费用标识。
func TestASellRequestEndsAtZeroTotalOnceWithoutChargeOrStopCause(t *testing.T) {
	fixture := newConclusionFixture(t)
	zero := domain.NewZeroTotalSellEvaluationRequestConclusion()

	if effect, err := fixture.conclude(t, fixture.record.Key, zero, ""); err != nil || effect != ports.SellEvaluationRequestConcluded {
		t.Fatalf("首记 = %s err = %v，want CONCLUDED", effect, err)
	}
	stored := fixture.stored(t)
	if stored.State != domain.SellRequestZeroTotal || stored.FormedCharge.String() != "" || stored.StopCause != domain.EvaluationStopCauseInvalid {
		t.Fatalf("读回 = %s / %q / %s，want ZERO_TOTAL 不带费用与停因", stored.State, stored.FormedCharge, stored.StopCause)
	}
	if effect, err := fixture.conclude(t, fixture.record.Key, zero, ""); err != nil || effect != ports.SellEvaluationRequestConclusionReplayed {
		t.Fatalf("重放 = %s err = %v，want REPLAYED", effect, err)
	}
	pending := conclusionOf(t, domain.StopEvaluationPendingJudgment, "")
	if effect, err := fixture.conclude(t, fixture.record.Key, pending, ""); err != nil || effect != ports.SellEvaluationRequestConcludedOtherwise {
		t.Fatalf("零额之后再记待判断 = %s err = %v，want CONCLUDED_OTHERWISE", effect, err)
	}
}

// Covers: 库上的 CHECK 与领域构造守同一条——绕过适配器给 SELL 册的零额补停因、给 BUY 册已停下的请求补零额、往 negative_end
// 写零额以外的词，都被拒。
func TestTheRegistersRefuseAZeroTotalPairedWithAStopCause(t *testing.T) {
	fixture := newResumptionFixture(t)
	sell := sellEvaluationRequestRecord(t, "tenant-a", "SELLREQ-SYN-Z", "SYN-PARCEL-Z",
		domain.BillingMomentNetworkIntakeAdopted, sellRequestRecordedAt)
	saveSellEvaluationRequest(t, fixture.db.Transactor(), fixture.sellRequests, sell)
	fixture.within(t, func(ctx context.Context) error {
		_, err := fixture.sellRequests.RecordConclusion(ctx, sell.Key, domain.NewZeroTotalSellEvaluationRequestConclusion())
		return err
	})
	buy := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-Z", "syn-fee-z")
	saveEvaluationRequest(t, fixture.db.Transactor(), fixture.buyRequests, buy)
	fixture.within(t, func(ctx context.Context) error {
		_, err := fixture.buyRequests.RecordStop(ctx, buy.Key, domain.StopEvaluationConflict, resumptionAt)
		return err
	})

	for name, statement := range map[string]string{
		"SELL 零额补停因": `UPDATE settlement_accounting.sell_evaluation_request
			SET stop_cause = 'EVALUATION_FAILED', stopped_at = now() WHERE tenant_id = $1 AND request_id = 'SELLREQ-SYN-Z'`,
		"BUY 停下补零额": `UPDATE settlement_accounting.evaluation_request
			SET negative_end = 'ZERO_TOTAL' WHERE tenant_id = $1 AND request_id = 'EVREQ-SYN-Z'`,
		"BUY 零额以外的词": `UPDATE settlement_accounting.evaluation_request
			SET negative_end = 'NOTHING', stop_cause = NULL, stopped_at = NULL WHERE tenant_id = $1 AND request_id = 'EVREQ-SYN-Z'`,
	} {
		err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			executor, err := fixture.db.RequireExecutor(ctx)
			if err != nil {
				return err
			}
			_, err = executor.Exec(ctx, statement, "tenant-a")
			return err
		})
		if err == nil {
			t.Fatalf("%s：库上收下了", name)
		}
	}
}

// Covers: AT-SA-215 的 BUY 册——记零额只从「既无停因也无零额」往外走：首记写下、再记重放；已有停因的请求答已停下、不覆盖；
// 已记零额的请求再记停因答另有停因；写口无事务拒；登记口不收带零额的记录；续办队列取不到零额的请求。
func TestABuyRequestEndsAtZeroTotalExclusiveOfAStopCause(t *testing.T) {
	fixture := newResumptionFixture(t)
	zero := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-Z", "syn-fee-z")
	stopped := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-S", "syn-fee-s")
	for _, record := range []ports.EvaluationRequestRecord{zero, stopped} {
		if outcome := saveEvaluationRequest(t, fixture.db.Transactor(), fixture.buyRequests, record); outcome != ports.EvaluationRequestSaved {
			t.Fatalf("登 %s = %s", record.Key.Request, outcome)
		}
	}
	if _, err := fixture.buyRequests.RecordZeroTotal(t.Context(), zero.Key); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 RecordZeroTotal 应答 ErrTransactionRequired，实得：%v", err)
	}
	for _, want := range []ports.EvaluationRequestZeroTotalEffect{ports.EvaluationRequestZeroTotalRecorded, ports.EvaluationRequestZeroTotalReplayed} {
		fixture.within(t, func(ctx context.Context) error {
			effect, err := fixture.buyRequests.RecordZeroTotal(ctx, zero.Key)
			if err == nil && effect != want {
				return errors.New("effect " + effect.String() + " want " + want.String())
			}
			return err
		})
	}
	fixture.within(t, func(ctx context.Context) error {
		if effect, err := fixture.buyRequests.RecordStop(ctx, zero.Key, domain.StopEvaluationConflict, resumptionAt); err != nil ||
			effect != ports.EvaluationRequestStoppedOtherwise {
			return errors.New("零额之后记停因 = " + effect.String())
		}
		if _, err := fixture.buyRequests.RecordStop(ctx, stopped.Key, domain.StopEvaluationConflict, resumptionAt); err != nil {
			return err
		}
		if effect, err := fixture.buyRequests.RecordZeroTotal(ctx, stopped.Key); err != nil || effect != ports.EvaluationRequestAlreadyStopped {
			return errors.New("停下之后记零额 = " + effect.String())
		}
		return nil
	})
	read, found, err := fixture.buyRequests.FindByID(t.Context(), zero.Key)
	if err != nil || !found || !read.ZeroTotal || read.StopCause != domain.EvaluationStopCauseInvalid {
		t.Fatalf("读回零额的请求 = 零额 %v 停因 %s found=%v err=%v", read.ZeroTotal, read.StopCause, found, err)
	}
	if read, _, err := fixture.buyRequests.FindByID(t.Context(), stopped.Key); err != nil || read.ZeroTotal || read.StopCause != domain.StopEvaluationConflict {
		t.Fatalf("读回停下的请求 = 零额 %v 停因 %s err=%v", read.ZeroTotal, read.StopCause, err)
	}

	carrying := evaluationRequestRecord(t, "tenant-a", "EVREQ-SYN-C", "syn-fee-c")
	carrying.ZeroTotal = true
	if err := fixture.db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		_, err := fixture.buyRequests.Save(ctx, carrying)
		return err
	}); err == nil {
		t.Fatal("登记口收下了带零额的记录")
	}

	for _, item := range fixture.list(t, ports.StalledEvaluationQuery{Causes: domain.AllEvaluationStopCauses()}) {
		if item.Kind == ports.StalledBuyRequest && item.BuyRequest == zero.Key.Request {
			t.Fatalf("续办队列取到了零额的请求：%+v", item)
		}
	}
}
