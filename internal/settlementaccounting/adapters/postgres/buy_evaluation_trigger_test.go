package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestBuyEvaluationTriggersRefuseToRunOutsideATransaction(t *testing.T) {
	triggers := newBuyEvaluationTriggers(t)
	registration := buyEvaluationTriggerRegistration(t, "BOOKING")

	if _, err := triggers.SaveBuyEvaluationTrigger(t.Context(), registerTenant(t), registration, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

func TestBuyEvaluationTriggerReplaysConflictsAndMissesAsUnconfigured(t *testing.T) {
	db := catalogueDB(t)
	triggers := mustBuyEvaluationTriggers(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 9, 30, 12, 0, 0, 0, time.UTC)
	booking := buyEvaluationTriggerRegistration(t, "BOOKING")
	reason := booking.Reason()

	if _, found, err := triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, reason); err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if effect, err := triggers.SaveBuyEvaluationTrigger(ctx, tenant, booking, at); err != nil || effect != ports.CatalogueRegistered {
			return errOrEffect(err, effect, ports.CatalogueRegistered)
		}
		if effect, err := triggers.SaveBuyEvaluationTrigger(ctx, tenant, booking, at); err != nil || effect != ports.CatalogueReplay {
			return errOrEffect(err, effect, ports.CatalogueReplay)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, reason)
	if err != nil || !found || loaded.Moment() != domain.BuyEvaluationTriggerOnOccurrenceFormed {
		t.Fatalf("读回 moment=%s found=%v err=%v", loaded.Moment(), found, err)
	}
	if _, registered := loaded.FeeItem(); registered {
		t.Fatal("没登费用项目的行读回来带了费用项目")
	}
	other, err := domain.NewOccurrenceReasonReference("CANCEL")
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, other); err != nil || found {
		t.Fatalf("未登记原因 found=%v err=%v", found, err)
	}
}

// Covers: ADR-0208——费用项目随行落库读回；同原因换一个已登的费用项目是异内容，答冲突、原行不动。ADR-0229 决定七
// （AT-SA-197）：先登了没有费用项目的那一版，再来补登一格是追加下一版，读口取最新一版、旧版留在册上；任一版再交一次
// 是重放；补上之后再换一个费用项目仍答冲突。
func TestBuyEvaluationTriggerCarriesItsFeeItemAndOnlyRevisesAMissingOne(t *testing.T) {
	db := catalogueDB(t)
	triggers := mustBuyEvaluationTriggers(t, db)
	tenant := registerTenant(t)
	at := time.Date(2026, 10, 19, 12, 0, 0, 0, time.UTC)
	fulfillment := withFeeItem(t, buyEvaluationTriggerRegistration(t, "ACTUAL_FULFILLMENT"), "SYN-FEE-ITEM-TRUNK-COST-01")
	otherFeeItem := withFeeItem(t, buyEvaluationTriggerRegistration(t, "ACTUAL_FULFILLMENT"), "SYN-FEE-ITEM-OTHER")
	bare := buyEvaluationTriggerRegistration(t, "FAILED_ATTEMPT")
	bareThenFeeItem := withFeeItem(t, buyEvaluationTriggerRegistration(t, "FAILED_ATTEMPT"), "SYN-FEE-ITEM-ATTEMPT")
	revisedAgain := withFeeItem(t, buyEvaluationTriggerRegistration(t, "FAILED_ATTEMPT"), "SYN-FEE-ITEM-OTHER")

	err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		for _, step := range []struct {
			registration domain.BuyEvaluationTriggerRegistration
			want         ports.CatalogueRegistrationEffect
		}{
			{fulfillment, ports.CatalogueRegistered},
			{fulfillment, ports.CatalogueReplay},
			{otherFeeItem, ports.CatalogueConflict},
			{bare, ports.CatalogueRegistered},
			{bareThenFeeItem, ports.CatalogueRegistered},
			{bareThenFeeItem, ports.CatalogueReplay},
			{bare, ports.CatalogueReplay},
			{revisedAgain, ports.CatalogueConflict},
		} {
			effect, err := triggers.SaveBuyEvaluationTrigger(ctx, tenant, step.registration, at)
			if err != nil || effect != step.want {
				return errOrEffect(err, effect, step.want)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	loaded, found, err := triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, fulfillment.Reason())
	if err != nil || !found {
		t.Fatalf("读回 found=%v err=%v", found, err)
	}
	if feeItem, registered := loaded.FeeItem(); !registered || feeItem.String() != "SYN-FEE-ITEM-TRUNK-COST-01" {
		t.Fatalf("费用项目 = %q registered=%v, want 先登的那一个", feeItem, registered)
	}
	loaded, found, err = triggers.LoadBuyEvaluationTrigger(t.Context(), tenant, bare.Reason())
	if err != nil || !found {
		t.Fatalf("读回 found=%v err=%v", found, err)
	}
	if feeItem, registered := loaded.FeeItem(); !registered || feeItem.String() != "SYN-FEE-ITEM-ATTEMPT" {
		t.Fatalf("费用项目 = %q registered=%v, want 追加的那一版", feeItem, registered)
	}
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var revisions int
	var first *string
	if err := querier.QueryRow(t.Context(),
		`SELECT count(*), (SELECT fee_item_ref FROM settlement_accounting.buy_evaluation_trigger
		                    WHERE tenant_id = $1 AND occurrence_reason = 'FAILED_ATTEMPT' AND revision = 1)
		   FROM settlement_accounting.buy_evaluation_trigger WHERE tenant_id = $1 AND occurrence_reason = 'FAILED_ATTEMPT'`,
		tenant.String()).Scan(&revisions, &first); err != nil {
		t.Fatal(err)
	}
	if revisions != 2 || first != nil {
		t.Fatalf("版次 %d 行、第一版费用项目 %v，want 两版且第一版原样没登", revisions, first)
	}
}

func withFeeItem(t *testing.T, registration domain.BuyEvaluationTriggerRegistration, feeItem string) domain.BuyEvaluationTriggerRegistration {
	t.Helper()
	reference, err := domain.NewFeeItemReference(feeItem)
	if err != nil {
		t.Fatal(err)
	}
	registered, err := registration.WithFeeItem(reference)
	if err != nil {
		t.Fatal(err)
	}
	return registered
}

func newBuyEvaluationTriggers(t *testing.T) *adapter.BuyEvaluationTriggers {
	t.Helper()
	return mustBuyEvaluationTriggers(t, catalogueDB(t))
}

func mustBuyEvaluationTriggers(t *testing.T, db *bentopg.DB) *adapter.BuyEvaluationTriggers {
	t.Helper()
	triggers, err := adapter.NewBuyEvaluationTriggers(db)
	if err != nil {
		t.Fatal(err)
	}
	return triggers
}

func buyEvaluationTriggerRegistration(t *testing.T, reason string) domain.BuyEvaluationTriggerRegistration {
	t.Helper()
	reference, err := domain.NewOccurrenceReasonReference(reason)
	if err != nil {
		t.Fatal(err)
	}
	registration, err := domain.NewBuyEvaluationTriggerRegistration(reference, domain.BuyEvaluationTriggerOnOccurrenceFormed)
	if err != nil {
		t.Fatal(err)
	}
	return registration
}
