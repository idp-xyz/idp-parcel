package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestLabelChannelParcelReadsBothAdoptedOperatingResults(t *testing.T) {
	ctx := t.Context()
	db, pool, handler, catalogue := directOperatingStack(t)
	tenant := "SYN-DIRECT-LABEL"
	parcel := "SYN-PARCEL-LABEL"
	seedServiceForm(t, pool, tenant, parcel, "SYN-PRODUCT-LABEL", "LABEL_CHANNEL_SERVICE")
	seedCustomerCharge(t, pool, tenant, parcel, "SYN-CHG-LABEL", "2026-11-20", true)
	seedExpectedCost(t, pool, tenant, parcel, "occ-label", "occ-v1", "cost-v1", "", 4000)
	seedPayable(t, pool, tenant, "claim-label", "2026-11-20", "cost-v1", "pay-label", 4500)
	adoptBoth(t, db, tenant)

	parcelRef := directRef(t, parcel)
	tenantID := directTenantID(t, tenant)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, tenantID, parcelRef)
	})

	adoption, err := catalogue.OperatingBasisAdoption(ctx, tenantID)
	if err != nil || !adoption.Configured || !adoption.Estimated || !adoption.Confirmed {
		t.Fatalf("采用 = %+v err=%v", adoption, err)
	}
	results := listDirectResults(t, catalogue, tenantID)
	if len(results) != 2 {
		t.Fatalf("两种口径应各有一份结果，got %d", len(results))
	}
	byBasis := map[string]ports.OperatingResultCatalogueRow{}
	for _, row := range results {
		byBasis[row.Basis] = row
		if len(row.Components) < 2 {
			t.Fatalf("%s 组成项不可见：%+v", row.Basis, row.Components)
		}
	}
	if byBasis["ESTIMATED"].MarginMinor != 6000 || byBasis["CONFIRMED"].MarginMinor != 5500 {
		t.Fatalf("毛利 = 预估 %d 已确认 %d", byBasis["ESTIMATED"].MarginMinor, byBasis["CONFIRMED"].MarginMinor)
	}

	seedExpectedCost(t, pool, tenant, parcel, "occ-label", "occ-v1", "cost-v2", "cost-v1", 7000)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterOccurrence(txCtx, tenantID, directOccurrence(t, "occ-label"), directOccurrenceVersion(t, "occ-v1"))
	})
	var versions int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1`, tenant).Scan(&versions); err != nil {
		t.Fatal(err)
	}
	if versions != 2 {
		t.Fatalf("纠错后原版本应还在，版本数 = %d", versions)
	}
	revised := listDirectResults(t, catalogue, tenantID)
	var estimated ports.OperatingResultCatalogueRow
	for _, row := range revised {
		if row.Basis == "ESTIMATED" {
			estimated = row
		}
	}
	if estimated.MarginMinor != 3000 || estimated.Corrects == "" || estimated.Corrects == estimated.Version {
		t.Fatalf("新版本应关联原截点：%+v", estimated)
	}
}

func TestOnlyCustomerSideIsIndeterminate(t *testing.T) {
	ctx := t.Context()
	db, pool, handler, catalogue := directOperatingStack(t)
	tenant := "SYN-DIRECT-CUSTOMER"
	parcel := "SYN-PARCEL-CUSTOMER"
	seedServiceForm(t, pool, tenant, parcel, "SYN-PRODUCT-CUSTOMER", "LABEL_CHANNEL_SERVICE")
	seedCustomerCharge(t, pool, tenant, parcel, "SYN-CHG-CUSTOMER", "2026-11-20", true)
	adoptBoth(t, db, tenant)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, directTenantID(t, tenant), directRef(t, parcel))
	})
	results := listDirectResults(t, catalogue, directTenantID(t, tenant))
	if len(results) != 0 {
		t.Fatalf("只有客户侧不应有毛利，got %+v", results)
	}
	dispositions, err := catalogue.ListDirectOperatingDispositions(ctx, directTenantID(t, tenant))
	if err != nil || len(dispositions) != 2 || dispositions[0].Disposition != "INDETERMINATE" {
		t.Fatalf("应判不出：%+v err=%v", dispositions, err)
	}
}

func TestNetworkServiceProductIsIncompleteCost(t *testing.T) {
	ctx := t.Context()
	db, pool, handler, catalogue := directOperatingStack(t)
	tenant := "SYN-DIRECT-NETWORK"
	parcel := "SYN-PARCEL-NETWORK"
	seedServiceForm(t, pool, tenant, parcel, "SYN-PRODUCT-NETWORK", "NETWORK_SERVICE")
	seedCustomerCharge(t, pool, tenant, parcel, "SYN-CHG-NETWORK", "2026-11-20", true)
	seedExpectedCost(t, pool, tenant, parcel, "occ-network", "occ-v1", "cost-network", "", 1000)
	adoptBoth(t, db, tenant)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, directTenantID(t, tenant), directRef(t, parcel))
	})
	results := listDirectResults(t, catalogue, directTenantID(t, tenant))
	if len(results) != 0 {
		t.Fatalf("网络服务产品不应有毛利率，got %+v", results)
	}
	dispositions, err := catalogue.ListDirectOperatingDispositions(ctx, directTenantID(t, tenant))
	if err != nil || len(dispositions) == 0 || dispositions[0].Disposition != "INCOMPLETE_COST" {
		t.Fatalf("应成本不全：%+v err=%v", dispositions, err)
	}
}

func TestSharedCostRemovesAnExistingMargin(t *testing.T) {
	ctx := t.Context()
	db, pool, handler, catalogue := directOperatingStack(t)
	tenant := "SYN-DIRECT-SHARED"
	parcel := "SYN-PARCEL-SHARED"
	seedServiceForm(t, pool, tenant, parcel, "SYN-PRODUCT-SHARED", "LABEL_CHANNEL_SERVICE")
	seedCustomerCharge(t, pool, tenant, parcel, "SYN-CHG-SHARED", "2026-11-20", true)
	seedExpectedCost(t, pool, tenant, parcel, "occ-shared", "occ-v1", "cost-shared", "", 4000)
	seedPayable(t, pool, tenant, "claim-shared", "2026-11-20", "cost-shared", "pay-shared", 4500)
	adoptBoth(t, db, tenant)
	tenantID := directTenantID(t, tenant)
	parcelRef := directRef(t, parcel)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, tenantID, parcelRef)
	})
	if got := countOperatingResults(t, pool, tenant); got != 2 {
		t.Fatalf("落进共享成本之前应有两份毛利，got %d", got)
	}

	mustExec(t, pool, ctx, `
		INSERT INTO node_operations.consolidation_unit
			(tenant_id, unit_id, asset_ref, phase, opened_source)
		VALUES ($1, 'SYN-UNIT-SHARED', 'asset', 'OPEN',
		        '{"sourceId":"src","performedBy":"op","evidence":"ev","occurredAt":"2026-11-20T00:00:00Z"}')`,
		tenant)
	mustExec(t, pool, ctx, `
		INSERT INTO node_operations.containment_current (tenant_id, member_id, unit_id)
		VALUES ($1, $2, 'SYN-UNIT-SHARED')`,
		tenant, parcel)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, tenantID, parcelRef)
	})
	if got := countOperatingResults(t, pool, tenant); got != 0 {
		t.Fatalf("挂进集运单元之后毛利行应撤下，还剩 %d", got)
	}
	assertDispositions(t, catalogue, tenantID, "INCOMPLETE_COST", 2)
}

func TestNonUniqueOccurrenceMembersAreIncompleteCost(t *testing.T) {
	ctx := t.Context()
	db, pool, handler, catalogue := directOperatingStack(t)
	tenant := "SYN-DIRECT-MULTI"
	parcel := "SYN-PARCEL-MULTI"
	seedServiceForm(t, pool, tenant, parcel, "SYN-PRODUCT-MULTI", "LABEL_CHANNEL_SERVICE")
	seedCustomerCharge(t, pool, tenant, parcel, "SYN-CHG-MULTI", "2026-11-20", true)
	seedExpectedCost(t, pool, tenant, parcel, "occ-multi", "occ-v1", "cost-multi", "", 4000)
	seedPayable(t, pool, tenant, "claim-multi", "2026-11-20", "cost-multi", "pay-multi", 4500)
	mustExec(t, pool, ctx, `
		INSERT INTO transport_fulfillment.transport_charge_occurrence_member
			(tenant_id, occurrence_ref, validity_version, object_ref)
		VALUES ($1, 'occ-multi', 'occ-v1', 'SYN-OTHER-OBJECT')`,
		tenant)
	adoptBoth(t, db, tenant)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, directTenantID(t, tenant), directRef(t, parcel))
	})
	if got := countOperatingResults(t, pool, tenant); got != 0 {
		t.Fatalf("发生项成员不止这一件时不应有毛利，got %d", got)
	}
	assertDispositions(t, catalogue, directTenantID(t, tenant), "INCOMPLETE_COST", 2)
}

func TestLosingTheSupplierSideRemovesTheMargin(t *testing.T) {
	ctx := t.Context()
	db, pool, handler, catalogue := directOperatingStack(t)
	tenant := "SYN-DIRECT-LOST-COST"
	parcel := "SYN-PARCEL-LOST-COST"
	seedServiceForm(t, pool, tenant, parcel, "SYN-PRODUCT-LOST", "LABEL_CHANNEL_SERVICE")
	seedCustomerCharge(t, pool, tenant, parcel, "SYN-CHG-LOST", "2026-11-20", true)
	seedExpectedCost(t, pool, tenant, parcel, "occ-lost", "occ-v1", "cost-lost", "", 4000)
	seedPayable(t, pool, tenant, "claim-lost", "2026-11-20", "cost-lost", "pay-lost", 4500)
	adoptBoth(t, db, tenant)
	tenantID := directTenantID(t, tenant)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, tenantID, directRef(t, parcel))
	})
	if got := countOperatingResults(t, pool, tenant); got != 2 {
		t.Fatalf("撤下供应商侧之前应有两份毛利，got %d", got)
	}

	mustExec(t, pool, ctx, `DELETE FROM settlement_accounting.audited_payable WHERE tenant_id = $1`, tenant)
	mustExec(t, pool, ctx, `DELETE FROM settlement_accounting.supplier_expected_cost WHERE tenant_id = $1`, tenant)
	saWithin(t, db.Transactor(), ctx, func(txCtx context.Context) error {
		return handler.AfterParcel(txCtx, tenantID, directRef(t, parcel))
	})
	if got := countOperatingResults(t, pool, tenant); got != 0 {
		t.Fatalf("只剩客户侧时旧毛利应撤下，还剩 %d", got)
	}
	assertDispositions(t, catalogue, tenantID, "INDETERMINATE", 2)
}

func TestMissingOperatingBasisAdoptionReadsUnconfigured(t *testing.T) {
	ctx := t.Context()
	_, _, _, catalogue := directOperatingStack(t)
	adoption, err := catalogue.OperatingBasisAdoption(ctx, directTenantID(t, "SYN-DIRECT-NONE"))
	if err != nil || adoption.Configured {
		t.Fatalf("没登记应未配置：%+v err=%v", adoption, err)
	}
	dispositions, err := catalogue.ListDirectOperatingDispositions(ctx, directTenantID(t, "SYN-DIRECT-NONE"))
	if err != nil || len(dispositions) != 0 {
		t.Fatalf("未配置不是空的判不出列表：%+v err=%v", dispositions, err)
	}
}

func TestOperatingBasisAdoptionSaveRefusesOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := adapter.NewOperatingBasisAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	adoption, err := domain.NewOperatingBasisAdoption(true, true)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adoptions.SaveOperatingBasisAdoption(t.Context(), directTenantID(t, "SYN-DIRECT-TX"), adoption, time.Now().UTC()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务保存采用应被拒，err=%v", err)
	}
	dispositions, err := adapter.NewDirectOperatingDispositions(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := directTenantID(t, "SYN-DIRECT-TX")
	if err := dispositions.SaveDirectOperatingDisposition(t.Context(), tenant, "parcel", domain.EstimatedBasis, "INDETERMINATE", time.Now().UTC()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务保存答案应被拒，err=%v", err)
	}
	if err := dispositions.ClearDirectOperatingDisposition(t.Context(), tenant, "parcel", domain.EstimatedBasis); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务撤下答案应被拒，err=%v", err)
	}
	if err := dispositions.ForgetDirectOperatingAnswer(t.Context(), tenant, "parcel", domain.EstimatedBasis); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务忘掉答案应被拒，err=%v", err)
	}
}

type quietOperatingHandoff struct{}

func (quietOperatingHandoff) HandOffOperating(context.Context, ports.OperatingIntent) error {
	return nil
}

type directStackClock struct{}

func (directStackClock) Now() time.Time { return time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC) }

func directOperatingStack(t *testing.T) (*bentopg.DB, *pgxpool.Pool, *application.DeriveDirectOperatingResultsHandler, *adapter.OperatingCatalogue) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	adoptions, err := adapter.NewOperatingBasisAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	facts, err := adapter.NewParcelDirectFacts(db)
	if err != nil {
		t.Fatal(err)
	}
	dispositions, err := adapter.NewDirectOperatingDispositions(db)
	if err != nil {
		t.Fatal(err)
	}
	results, err := adapter.NewOperatingResults(db)
	if err != nil {
		t.Fatal(err)
	}
	allocations, err := adapter.NewCostAllocations(db)
	if err != nil {
		t.Fatal(err)
	}
	rules, err := adapter.NewAllocationRuleApplicability(db)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := adapter.NewOperatingCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	handler, err := application.NewDeriveDirectOperatingResultsHandler(application.DeriveDirectOperatingResultsDeps{
		Adoptions: adoptions, Facts: facts, Dispositions: dispositions, Results: results,
		Allocate: application.NewAllocateCostsHandler(application.AllocateCostsDeps{
			Allocations: allocations, Results: results, Rules: rules, Downstream: quietOperatingHandoff{}, Clock: directStackClock{},
		}),
		Clock: directStackClock{},
	})
	if err != nil {
		t.Fatal(err)
	}
	return db, pool, handler, catalogue
}

func adoptBoth(t *testing.T, db *bentopg.DB, tenant string) {
	t.Helper()
	adoptions, err := adapter.NewOperatingBasisAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	adoption, err := domain.NewOperatingBasisAdoption(true, true)
	if err != nil {
		t.Fatal(err)
	}
	var effect ports.CatalogueRegistrationEffect
	saWithin(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		var err error
		effect, err = adoptions.SaveOperatingBasisAdoption(txCtx, directTenantID(t, tenant), adoption, time.Now().UTC())
		return err
	})
	if effect != ports.CatalogueRegistered {
		t.Fatalf("采用落点 = %d", effect)
	}
	saWithin(t, db.Transactor(), t.Context(), func(txCtx context.Context) error {
		again, err := adoptions.SaveOperatingBasisAdoption(txCtx, directTenantID(t, tenant), adoption, time.Now().UTC())
		if err != nil {
			return err
		}
		if again != ports.CatalogueReplay {
			return errors.New("同内容应已登记")
		}
		other, err := domain.NewOperatingBasisAdoption(true, false)
		if err != nil {
			return err
		}
		conflict, err := adoptions.SaveOperatingBasisAdoption(txCtx, directTenantID(t, tenant), other, time.Now().UTC())
		if err != nil {
			return err
		}
		if conflict != ports.CatalogueConflict {
			return errors.New("异内容应冲突")
		}
		return nil
	})
}

func countOperatingResults(t *testing.T, pool *pgxpool.Pool, tenant string) int {
	t.Helper()
	var count int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM settlement_accounting.operating_result WHERE tenant_id = $1`, tenant).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func assertDispositions(t *testing.T, catalogue *adapter.OperatingCatalogue, tenant domain.TenantID, want string, count int) {
	t.Helper()
	dispositions, err := catalogue.ListDirectOperatingDispositions(t.Context(), tenant)
	if err != nil {
		t.Fatal(err)
	}
	if len(dispositions) != count {
		t.Fatalf("答案条数 = %d，want %d：%+v", len(dispositions), count, dispositions)
	}
	for _, disposition := range dispositions {
		if disposition.Disposition != want {
			t.Fatalf("答案 = %s，want %s", disposition.Disposition, want)
		}
	}
}

func listDirectResults(t *testing.T, catalogue *adapter.OperatingCatalogue, tenant domain.TenantID) []ports.OperatingResultCatalogueRow {
	t.Helper()
	page, err := catalogue.ListOperatingResults(t.Context(), tenant, 20, firstPage(t, ports.OperatingResultPages))
	if err != nil {
		t.Fatal(err)
	}
	return page.Rows
}

func seedServiceForm(t *testing.T, pool *pgxpool.Pool, tenant, parcel, product, form string) {
	t.Helper()
	ctx := t.Context()
	at := time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)
	mustExec(t, pool, ctx, `
		INSERT INTO party_commercial.commercial_version
			(tenant_id, object_kind, object_id, version_label, scope_ref, content_digest,
			 effective_starts_at, status, snapshot, published_at)
		VALUES ($1, 1, $2, 'v1', 'SYN-SCOPE', 'digest', $3, 3, '{}', $3)`,
		tenant, product, at)
	mustExec(t, pool, ctx, `
		INSERT INTO party_commercial.service_product_form
			(tenant_id, object_kind, object_id, version_label, form)
		VALUES ($1, 1, $2, 'v1', $3)`,
		tenant, product, form)
	requestID := "request-" + parcel
	snapshot := fmt.Sprintf(
		`{"shipmentRequestId":%q,"currentVersion":{"versionId":"v1","requestedProduct":%q,"declaredParcelIds":[%q]},"baseline":{"submissionVersion":"v1","declaredParcelIds":[%q],"fixedAt":"2026-11-01T00:00:00Z"}}`,
		requestID, product, parcel, parcel)
	mustExec(t, pool, ctx, `
		INSERT INTO parcel_shipment.shipment_request
			(tenant_id, customer_account_id, source, source_request_key, shipment_request_id,
			 revision, state, snapshot, submitted_at, current_submission_version_id, declared_parcel_ids, task_waiting_on)
		VALUES ($1, 'SYN-ACCOUNT', 'SYN', $2, $2, 1, 2, $5::jsonb, $3, 'v1', ARRAY[$4], 0)`,
		tenant, requestID, at, parcel, snapshot)
}

func seedCustomerCharge(t *testing.T, pool *pgxpool.Pool, tenant, parcel, charge, period string, confirmed bool) {
	t.Helper()
	ctx := t.Context()
	at := time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC)
	if confirmed {
		mustExec(t, pool, ctx, `
			INSERT INTO settlement_accounting.customer_charge
				(tenant_id, charge_id, fee_item, evaluation_ref, settlement_currency, settlement_minor,
				 original_currency, original_minor, stage, confirmation_basis, formed_at, confirmed_at, recorded_at,
				 responsible_entity, counterparty_ref, charge_direction, settlement_account_id, contract_basis,
				 primary_charging_scope, source_fact_ref, attribution_date)
			VALUES ($1, $2, 'freight', 'eval-sell', 'CNY', 10000, 'CNY', 10000, 'CONFIRMED', 'basis',
			        $3, $3, $3, 'entity', 'shipper', 'RECEIVABLE', 'account', 'contract', 'scope', 'fact', $4::date)`,
			tenant, charge, at, period)
	} else {
		mustExec(t, pool, ctx, `
			INSERT INTO settlement_accounting.customer_charge
				(tenant_id, charge_id, fee_item, evaluation_ref, settlement_currency, settlement_minor,
				 original_currency, original_minor, stage, formed_at, recorded_at)
			VALUES ($1, $2, 'freight', 'eval-sell', 'CNY', 10000, 'CNY', 10000, 'ESTIMATED', $3, $3)`,
			tenant, charge, at)
	}
	mustExec(t, pool, ctx, `
		INSERT INTO settlement_accounting.sell_evaluation_request
			(tenant_id, request_id, parcel_id, account_id, settlement_currency, pricing_scope_id,
			 billing_moment, source_fact_ref, pricing_basis_at, recorded_at, state, customer_charge_id, generation)
		VALUES ($1, $2, $3, 'SYN-ACCOUNT', 'CNY', 'SYN-SCOPE', 'NETWORK_INTAKE_ADOPTED', 'fact', $4, $4, 'FORMED', $5, 1)`,
		tenant, "sell-"+charge, parcel, at, charge)
}

func seedExpectedCost(t *testing.T, pool *pgxpool.Pool, tenant, parcel, occurrence, occurrenceVersion, version, prior string, amount int64) {
	t.Helper()
	ctx := t.Context()
	at := time.Date(2026, 11, 20, 2, 0, 0, 0, time.UTC)
	if prior == "" {
		mustExec(t, pool, ctx, `
			INSERT INTO transport_fulfillment.transport_charge_occurrence
				(tenant_id, occurrence_ref, validity_version, journey_ref, legal_entity_ref, provider_ref, agreement_ref,
				 reason, fact_basis, scope_ref, quantity, unit_ref, occurred_at, recorded_at, condition_basis, registered_by)
			VALUES ($1, $2, $3, 'journey', 'entity', 'provider', 'agreement', 'ACTUAL_FULFILLMENT', 'fact', 'scope',
			        1, 'piece', $4, $4, 'AGREEMENT_DECLARED', 'registrar')`,
			tenant, occurrence, occurrenceVersion, at)
		mustExec(t, pool, ctx, `
			INSERT INTO transport_fulfillment.transport_charge_occurrence_member
				(tenant_id, occurrence_ref, validity_version, object_ref)
			VALUES ($1, $2, $3, $4)`,
			tenant, occurrence, occurrenceVersion, parcel)
	}
	var priorArg any
	var reason any
	if prior != "" {
		priorArg = prior
		reason = "repriced"
	}
	mustExec(t, pool, ctx, `
		INSERT INTO settlement_accounting.supplier_expected_cost
			(tenant_id, version, occurrence_id, occurrence_reason, occurrence_version, occurred_at, fee_item,
			 purchase_rule_version, agreement_ref, evaluation_ref, original_currency, original_minor,
			 settlement_currency, settlement_minor, prior_version, correction_reason, recorded_at)
		VALUES ($1, $2, $3, 'ACTUAL_FULFILLMENT', $4, $5, 'freight', 'rule-v1', 'agreement', 'eval-buy',
		        'CNY', $6, 'CNY', $6, $7, $8, $5)`,
		tenant, version, occurrence, occurrenceVersion, at, amount, priorArg, reason)
}

func seedPayable(t *testing.T, pool *pgxpool.Pool, tenant, claim, period, expectedVersion, payable string, amount int64) {
	t.Helper()
	ctx := t.Context()
	at := time.Date(2026, 11, 20, 3, 0, 0, 0, time.UTC)
	mustExec(t, pool, ctx, `
		INSERT INTO settlement_accounting.supplier_bill_reception
			(tenant_id, claim_id, claim_version, supplier_ref, legal_entity, period_ref, currency,
			 content_digest, claim, matches, audit_authority_configured, recorded_at)
		VALUES ($1, $2, 'v1', 'supplier', 'entity', $3, 'CNY', 'digest',
		        '{"lines":[{"line":"1"}],"received_at":"2026-11-20T03:00:00Z"}', '[]', false, $4)`,
		tenant, claim, period, at)
	mustExec(t, pool, ctx, `
		INSERT INTO settlement_accounting.audited_payable
			(tenant_id, payable_id, claim_id, line_ref, expected_version, legal_entity, account_id,
			 currency, amount_minor, auditor_ref, audited_at, content_digest, recorded_at)
		VALUES ($1, $2, $3, 'line-1', $4, 'entity', 'account', 'CNY', $5, 'auditor', $6, 'digest', $6)`,
		tenant, payable, claim, expectedVersion, amount, at)
}

func mustExec(t *testing.T, pool *pgxpool.Pool, ctx context.Context, sql string, args ...any) {
	t.Helper()
	if _, err := pool.Exec(ctx, sql, args...); err != nil {
		t.Fatalf("sql: %v\n%s", err, sql)
	}
}

func directTenantID(t *testing.T, value string) domain.TenantID {
	t.Helper()
	tenant, err := domain.NewTenantID(value)
	if err != nil {
		t.Fatal(err)
	}
	return tenant
}

func directRef(t *testing.T, value string) domain.ParcelReference {
	t.Helper()
	parcel, err := domain.NewParcelReference(value)
	if err != nil {
		t.Fatal(err)
	}
	return parcel
}

func directOccurrence(t *testing.T, value string) domain.ChargeOccurrenceID {
	t.Helper()
	occurrence, err := domain.NewChargeOccurrenceID(value)
	if err != nil {
		t.Fatal(err)
	}
	return occurrence
}

func directOccurrenceVersion(t *testing.T, value string) domain.OccurrenceVersion {
	t.Helper()
	version, err := domain.NewOccurrenceVersion(value)
	if err != nil {
		t.Fatal(err)
	}
	return version
}
