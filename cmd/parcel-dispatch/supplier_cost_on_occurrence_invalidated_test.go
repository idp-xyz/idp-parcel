package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件在生产依赖图上钉「TF 发生项失效 → SA 撤下依它形成的供应商预期成本」一条（票 demo-chain-gaps/15，ADR-0232 决定七、八）：TF 用真
// 登记用例落发生项 V1 与信封，SA 那一侧照首版形状落一份依 V1 的预期成本；TF 经真失效编排长出 V2 并交信封；一拍之后撤下那一路长出失效
// 纠错版本、记下 V1 已失效，发起 BUY 评价那一路对失效版本只入账。失效之后同一件事实再登长出 V3，再一拍，发起那一路为 V3 发起新的请求。
// 证据只记 S。

const withdrawalConsumer = "settlement-accounting/withdraw-supplier-cost-on-occurrence-invalidated"

// seedFirstSupplierCost 照首版形状为发生项 record 的 V1 落一份预期成本：BUY 评价到形成那一段另有用例钉，这里只要一条可撤的链头。
func seedFirstSupplierCost(t *testing.T, db *bentopg.DB, tenant string, record tfports.ChargeOccurrenceRecord) sadomain.SupplierExpectedCost {
	t.Helper()
	value := func(construct func(string) (sadomain.SupplierCostVersionID, error), raw string) sadomain.SupplierCostVersionID {
		built, err := construct(raw)
		if err != nil {
			t.Fatal(err)
		}
		return built
	}
	id, _ := sadomain.NewChargeOccurrenceID(record.Key.Occurrence.String())
	reason, _ := sadomain.NewOccurrenceReasonReference(record.Occurrence.Reason().String())
	version, _ := sadomain.NewOccurrenceVersion(record.Key.Validity.String())
	occurrence, err := sadomain.NewTransportChargeOccurrence(id, reason, version, record.Occurrence.OccurredAt())
	if err != nil {
		t.Fatal(err)
	}
	feeItem, _ := sadomain.NewFeeItemReference(trunkFeeItem)
	rule, _ := sadomain.NewPurchaseRuleVersionReference("SYN-BUY-CARD/v1")
	agreement, _ := sadomain.NewSupplierAgreementReference(record.Occurrence.Agreement().String())
	evaluation, _ := sadomain.NewBuyEvaluationReference("SYN-BUY-EVAL-DCG15")
	currency, _ := sadomain.NewCurrencyCode("CNY")
	cost, err := sadomain.FormSupplierExpectedCost(sadomain.SupplierExpectedCostSpec{
		Version: value(sadomain.NewSupplierCostVersionID, "BUYSC-SYN-EVREQ-DCG15"), Occurrence: occurrence, FeeItem: feeItem,
		RuleVersion: rule, Agreement: agreement, Evaluation: evaluation,
		OriginalCurrency: currency, OriginalMinor: 30000, SettlementCurrency: currency, SettlementMinor: 30000,
	})
	if err != nil {
		t.Fatal(err)
	}
	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatal(err)
	}
	tenantID, _ := sadomain.NewTenantID(tenant)
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		_, err := costs.Save(txCtx, tenantID, cost, time.Now())
		return err
	})
	return cost
}

// invalidateTrunkOccurrence 走 TF 的真失效编排：失效版本与信封经真登记册与真交接口同一笔落库。
func invalidateTrunkOccurrence(t *testing.T, db *bentopg.DB, store *outbox.Store, record tfports.ChargeOccurrenceRecord) {
	t.Helper()
	occurrences, err := tfpostgres.NewChargeOccurrences(db)
	if err != nil {
		t.Fatal(err)
	}
	handoff, err := tfpostgres.NewOutboxChargeOccurrenceHandoff(db, store, systemClock{})
	if err != nil {
		t.Fatal(err)
	}
	handler := tfapp.NewInvalidateChargeOccurrenceHandler(tfapp.InvalidateChargeOccurrenceDeps{
		Occurrences: occurrences, Scopes: occurrences, Handoff: handoff, Clock: systemClock{},
	})
	var result tfapp.ChargeOccurrenceInvalidationResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		result, err = handler.Invalidate(txCtx, tfapp.InvalidateChargeOccurrenceCommand{
			TenantID: record.Key.TenantID, Operator: "https://dex.syn.example#SYN-OPERATOR-CORRECTION-01",
			Occurrence: record.Key.Occurrence.String(), Version: record.Key.Validity.String(), Basis: "SYN-EVIDENCE-DCG15",
		})
		return err
	})
	if result.Outcome() != tfapp.ChargeOccurrenceInvalidated {
		t.Fatalf("失效：%q", result.Outcome())
	}
}

func countRows(t *testing.T, db *bentopg.DB, query string, args ...any) int {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var rows int
	if err := querier.QueryRow(t.Context(), query, args...).Scan(&rows); err != nil {
		t.Fatalf("%s：%v", query, err)
	}
	return rows
}

// Covers: AT-SA-206、AT-SA-210 的生产装配半边——见文件头；同一版失效再交撤下编排答重放、不多一版。
func TestAnInvalidatedOccurrenceWithdrawsItsSupplierCostAndTheReestablishedOneIsEvaluatedAgain(t *testing.T) {
	const tenant = "tenant-a"
	beat, db, store := wiredBeat(t)
	registerTrunkAgreement(t, db, tenant)
	registerBuyTriggers(t, db, tenant)
	registerEndedTrunkSegment(t, db, tenant)
	record := registerTrunkOccurrence(t, db, store, tenant)
	first := seedFirstSupplierCost(t, db, tenant, record)
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("首版那一拍：%v", err)
	}

	invalidateTrunkOccurrence(t, db, store, record)
	var eventID string
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := querier.QueryRow(t.Context(),
		`SELECT event_id FROM `+migrate.SchemaBento+`.outbox WHERE event_type = $1 AND payload->>'version' = 'V2'`,
		tfpostgres.ChargeOccurrenceFormedEventType,
	).Scan(&eventID); err != nil {
		t.Fatalf("找失效版本那一封：%v", err)
	}
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("失效那一拍：%v", err)
	}
	if status := outboxStatus(t, db, eventID); status != "PUBLISHED" {
		t.Fatalf("失效信封 status = %q；失败码 = %q", status, recordedFailureCode(t, db, eventID))
	}
	if n := inboxRowsOf(t, db, withdrawalConsumer, eventID); n != 1 {
		t.Fatalf("撤下那一路 inbox 行数 = %d, want 1", n)
	}
	var withdrawnVersion, prior, reason, occurrenceVersion string
	var withdrawn bool
	if err := querier.QueryRow(t.Context(), `
		SELECT version, prior_version, correction_reason, occurrence_version, withdrawn
		  FROM settlement_accounting.supplier_expected_cost
		 WHERE tenant_id = $1 AND prior_version IS NOT NULL`, tenant,
	).Scan(&withdrawnVersion, &prior, &reason, &occurrenceVersion, &withdrawn); err != nil {
		t.Fatalf("读失效纠错版本（应恰一版）：%v", err)
	}
	if prior != first.Version().String() || reason != "OCCURRENCE_INVALIDATED" || occurrenceVersion != "V2" || !withdrawn {
		t.Fatalf("失效纠错版本 = %s / %s / %s / %s / %v", withdrawnVersion, prior, reason, occurrenceVersion, withdrawn)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.supplier_cost_occurrence_invalidation WHERE invalidated_version = 'V1'`); n != 1 {
		t.Fatalf("失效记录 %d 行，want 1", n)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.evaluation_request WHERE tenant_id = $1`, tenant); n != 1 {
		t.Fatalf("失效版本发起了 BUY 请求：%d 份，want 只有 V1 那一份", n)
	}

	// 重投不重复纠错：同一版失效再交撤下编排一次，答重放，不多一版。
	costs, err := sapostgres.NewSupplierExpectedCosts(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := saapplication.NewWithdrawSupplierExpectedCostHandler(saapplication.WithdrawSupplierExpectedCostDeps{
		Registry: costs, Costs: costs, Invalidations: costs, Clock: systemClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	tenantID, _ := sadomain.NewTenantID(tenant)
	invalidated, _ := sadomain.NewTransportChargeOccurrence(first.Occurrence().ID(), first.Occurrence().Reason(),
		mustSAVersion(t, "V2"), first.Occurrence().OccurredAt())
	var replay saapplication.SupplierCostWithdrawalResult
	mustWithinTX(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		replay, err = handler.Withdraw(txCtx, saapplication.WithdrawSupplierExpectedCostCommand{
			TenantID: tenantID, Occurrence: invalidated, Invalidates: mustSAVersion(t, "V1"),
		})
		return err
	})
	if replay.Outcome() != saapplication.SupplierCostWithdrawalExisting ||
		countRows(t, db, `SELECT count(*) FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1`, tenant) != 2 {
		t.Fatalf("重投 = %s", replay.Outcome())
	}

	reestablished := registerTrunkOccurrence(t, db, store, tenant)
	if reestablished.Key.Validity.String() != "V3" {
		t.Fatalf("重登 = %s, want V3", reestablished.Key.Validity)
	}
	if _, err := beat.DispatchOnce(t.Context()); err != nil {
		t.Fatalf("替代版本那一拍：%v", err)
	}
	if n := countRows(t, db, `SELECT count(*) FROM settlement_accounting.evaluation_request WHERE tenant_id = $1 AND occurrence_version = 'V3'`, tenant); n != 1 {
		t.Fatalf("替代版本没发起 BUY 请求：%d 份", n)
	}
}

func mustSAVersion(t *testing.T, raw string) sadomain.OccurrenceVersion {
	t.Helper()
	version, err := sadomain.NewOccurrenceVersion(raw)
	if err != nil {
		t.Fatal(err)
	}
	return version
}
