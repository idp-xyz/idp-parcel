package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricingstore "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 入库作业与仓租各走到预估客户费用；确认时点与确认条件都登记后才确认。已确认上的更正答待人处理。
func TestWarehouseChargeWritersRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	store, err := adapter.NewWarehouseCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustAccountRef(t, domain.NewTenantID, "SYN-TENANT-01")
	ctx := context.Background()
	if err := store.SaveAdoption(ctx, tenant, application.WarehouseAdoption{Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01", Revision: 1, Effective: time.Now(), Account: "SYN-ACCT", Scope: "SYN-SCOPE", Basis: "SYN", Registrant: "operator-1"}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveAdoption：%v", err)
	}
	if err := store.SaveRequest(ctx, tenant, application.WarehouseChargeRequest{Occurrence: "CHO-1", Version: 1, Status: "PENDING"}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveRequest：%v", err)
	}
}

func TestWarehouseChargesFollowTheSyntheticOccurrence(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	plans, err := pricingstore.NewWarehousePlans(db)
	if err != nil {
		t.Fatal(err)
	}
	pricing := applicationBridge{handler: pricingapp.NewWarehousePlanHandler(plans, true, &countingIDs{prefix: "WEV"})}
	store, err := adapter.NewWarehouseCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		t.Fatal(err)
	}
	mappings, err := adapter.NewFeeItemMappings(db)
	if err != nil {
		t.Fatal(err)
	}
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := adapter.NewChargeConfirmationConditions(db)
	if err != nil {
		t.Fatal(err)
	}
	moments, err := adapter.NewSettlementMoments(db)
	if err != nil {
		t.Fatal(err)
	}
	supersessions, err := adapter.NewChargeSupersessions(db)
	if err != nil {
		t.Fatal(err)
	}
	momentHandler, err := application.NewAdmitSettlementMomentHandler(moments)
	if err != nil {
		t.Fatal(err)
	}
	clock := warehouseClock{at: time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)}
	handler := application.NewWarehouseChargeHandler(store, pricing, accounts, mappings, charges, conditions, conditions, momentHandler, supersessions, &countingIDs{prefix: "CHG"}, clock)
	tenant := mustAccountRef(t, domain.NewTenantID, "SYN-TENANT-01")
	account := warehouseAccount(t)

	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := accounts.Save(ctx, tenant, account, clock.Now()); err != nil {
			return err
		}
		if outcome, err := pricing.handler.Register(ctx, tenant.String(), warehouseSettlementPlan()); err != nil || outcome != pricingapp.WarehousePlanRecorded {
			return fmt.Errorf("方案 %s %v", outcome, err)
		}
		adopted, err := handler.Adopt(ctx, tenant, application.WarehouseAdoption{
			Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01", Revision: 1, Effective: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			Account: account.ID().String(), Scope: "SYN-SCOPE-WH", Basis: "SYN-BASIS", Registrant: "operator-1",
		})
		if err != nil || adopted != application.WarehouseChargeFormed {
			return fmt.Errorf("采用 %s %v", adopted, err)
		}
		if outcome, _, err := handler.Settle(ctx, tenant, inboundFact("CHO-MISS", 1)); err != nil || outcome != application.WarehouseChargePending {
			return fmt.Errorf("没映射 %s %v", outcome, err)
		}
		mapping, err := domain.NewFeeItemMapping(
			mustAccountRef(t, domain.NewPricingChargeCode, "INBOUND_PIECE"),
			mustAccountRef(t, domain.NewFeeItemReference, "SYN-FEE-WH"),
			mustAccountRef(t, domain.NewFeeItemMappingBasisReference, "SYN-BASIS"),
		)
		if err != nil {
			return err
		}
		if _, err := mappings.SaveFeeItemMapping(ctx, tenant, mapping, clock.Now()); err != nil {
			return err
		}
		formed, chargeID, err := handler.Settle(ctx, tenant, inboundFact("CHO-IN", 1))
		if err != nil || formed != application.WarehouseChargeFormed || chargeID == "" {
			return fmt.Errorf("没确认时点 %s %s %v", formed, chargeID, err)
		}
		if _, err := conditions.RegisterCondition(ctx, tenant, mustAccountRef(t, domain.NewFeeItemReference, "SYN-FEE-WH"), "WAREHOUSE_WORK_OCCURRED", clock.Now()); err != nil {
			return err
		}
		if _, err := moments.SaveSettlementMoment(ctx, tenant, domain.SettlementMomentConfirm, clock.Now()); err != nil {
			return err
		}
		confirmed, confirmedID, err := handler.Settle(ctx, tenant, inboundFact("CHO-IN-2", 1))
		if err != nil || confirmed != application.WarehouseChargeConfirmed {
			return fmt.Errorf("确认 %s %v", confirmed, err)
		}
		again, _, err := handler.Settle(ctx, tenant, inboundFact("CHO-IN-2", 1))
		if err != nil || again != application.WarehouseChargeExisting {
			return fmt.Errorf("重放 %s %v", again, err)
		}
		storageMapping, err := domain.NewFeeItemMapping(
			mustAccountRef(t, domain.NewPricingChargeCode, "STORAGE_DAY"),
			mustAccountRef(t, domain.NewFeeItemReference, "SYN-FEE-WH"),
			mustAccountRef(t, domain.NewFeeItemMappingBasisReference, "SYN-BASIS"),
		)
		if err != nil {
			return err
		}
		if _, err := mappings.SaveFeeItemMapping(ctx, tenant, storageMapping, clock.Now()); err != nil {
			return err
		}
		storedRent, rentID, err := handler.Settle(ctx, tenant, storageFact())
		if err != nil || storedRent != application.WarehouseChargeConfirmed {
			return fmt.Errorf("仓租 %s %v", storedRent, err)
		}
		rent, foundRent, err := charges.FindByID(ctx, tenant, mustAccountRef(t, domain.NewCustomerChargeID, rentID))
		if err != nil || !foundRent {
			return err
		}
		if _, minor := rent.OriginalAmount(); minor != 100 {
			return fmt.Errorf("仓租金额 %d", minor)
		}
		waiting, _, err := handler.Settle(ctx, tenant, inboundFact("CHO-IN-2", 2))
		if err != nil || waiting != application.WarehouseChargeAwaitingPerson {
			return fmt.Errorf("已确认上的更正 %s %v", waiting, err)
		}
		stored, found, err := charges.FindByID(ctx, tenant, mustAccountRef(t, domain.NewCustomerChargeID, confirmedID))
		if err != nil || !found || stored.Stage() != domain.ChargeConfirmed {
			return fmt.Errorf("费用 %v %s", err, stored.Stage())
		}
		_, minor := stored.OriginalAmount()
		if minor != 400 {
			return fmt.Errorf("金额 %d", minor)
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func storageFact() application.WarehouseOccurrenceFact {
	return application.WarehouseOccurrenceFact{
		ID: "CHO-STORE", Version: 1, Kind: "STORAGE", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01",
		Occurred: time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC), Correction: "ORIGINAL",
		Lines: []application.WarehouseFactLine{{
			Kind: "STORAGE_CELL", Measures: []application.WarehouseFactMeasure{{Unit: "PIECE", Quantity: "2"}},
		}},
	}
}

func inboundFact(id string, version int) application.WarehouseOccurrenceFact {
	return application.WarehouseOccurrenceFact{
		ID: id, Version: version, Kind: "INBOUND_HANDLING", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01",
		Occurred: time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC), Correction: "ORIGINAL",
		Measures: []application.WarehouseFactMeasure{{Unit: "PIECE", Quantity: "4"}},
	}
}

func warehouseSettlementPlan() pricingdomain.WarehousePlan {
	return pricingdomain.WarehousePlan{
		ID: "SYN-PLAN-WH-01", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []pricingdomain.WarehouseChargeGroup{
			{Name: "INBOUND", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", OnMiss: "PENDING"},
			{Name: "STORAGE", Kind: "STORAGE", Level: "LINE", Line: "STORAGE_CELL", OnMiss: "PENDING"},
		},
		Rules: []pricingdomain.WarehouseChargeRule{
			{Code: "INBOUND_PIECE", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", Group: "INBOUND", Priority: 1, Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT"},
			{Code: "STORAGE_DAY", Kind: "STORAGE", Level: "LINE", Line: "STORAGE_CELL", Group: "STORAGE", Priority: 1, Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 50, Rounding: "EXACT"},
		},
	}
}

func warehouseAccount(t *testing.T) domain.SettlementAccount {
	t.Helper()
	key, err := domain.NewSettlementAccountKey(
		mustAccountRef(t, domain.NewLegalEntityReference, "SYN-LE"),
		mustAccountRef(t, domain.NewSettlementCounterpartyReference, "SYN-OWNER-01"),
		domain.ChargeReceivable,
		mustAccountRef(t, domain.NewCurrencyCode, "CNY"),
		mustAccountRef(t, domain.NewSettlementPolicyReference, "SYN-POL"),
	)
	if err != nil {
		t.Fatal(err)
	}
	account, err := domain.NewSettlementAccount(
		mustAccountRef(t, domain.NewSettlementAccountID, "SYN-ACCT-WH"),
		key, domain.SettlementCounterpartyReference{}, false,
		mustAccountRef(t, domain.NewResponsibilityBasis, "SYN-CONTRACT"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return account
}

type applicationBridge struct {
	handler *pricingapp.WarehousePlanHandler
}

func (bridge applicationBridge) Price(ctx context.Context, tenant, scope string, at time.Time, fact application.WarehouseOccurrenceFact) (application.WarehouseChargePrice, error) {
	snapshot := pricingdomain.WarehouseOccurrenceSnapshot{
		ID: fact.ID, Version: fact.Version, Kind: fact.Kind, Owner: fact.Owner, Warehouse: fact.Warehouse, Occurred: fact.Occurred,
		Correction: fact.Correction,
	}
	for _, measure := range fact.Measures {
		snapshot.Measures = append(snapshot.Measures, pricingdomain.WarehouseMeasure{Unit: measure.Unit, Quantity: measure.Quantity, Unmeasured: measure.Unmeasured, Declared: measure.Declared})
	}
	for _, line := range fact.Lines {
		mapped := pricingdomain.WarehouseOccurrenceLine{Kind: line.Kind}
		for _, measure := range line.Measures {
			mapped.Measures = append(mapped.Measures, pricingdomain.WarehouseMeasure{Unit: measure.Unit, Quantity: measure.Quantity})
		}
		snapshot.Lines = append(snapshot.Lines, mapped)
	}
	outcome, evaluation, id, err := bridge.handler.Price(ctx, tenant, scope, at, snapshot)
	if err != nil {
		return application.WarehouseChargePrice{}, err
	}
	status := string(outcome)
	switch outcome {
	case pricingapp.WarehousePlanNotConfigured:
		status = "NOT_CONFIGURED"
	case pricingapp.WarehousePlanPending:
		status = "PENDING"
	case pricingapp.WarehousePlanConflict:
		status = "CONFLICT"
	case pricingapp.WarehousePlanZero:
		status = "ZERO"
	case pricingapp.WarehousePlanPriced:
		status = "COMPLETED"
	}
	return application.WarehouseChargePrice{
		EvaluationID: id, Status: status, TotalMinor: evaluation.TotalMinor, Currency: evaluation.Currency, Codes: evaluation.Codes,
	}, nil
}

type countingIDs struct {
	prefix string
	n      int
}

func (ids *countingIDs) Next() (string, error) {
	ids.n++
	return fmt.Sprintf("%s-%d", ids.prefix, ids.n), nil
}

type warehouseClock struct{ at time.Time }

func (clock warehouseClock) Now() time.Time { return clock.at }
