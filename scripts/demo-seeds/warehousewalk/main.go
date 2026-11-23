// 演示租户走出库前的仓储收费。金额、账户与方案都是 SYN-，不是生产默认。
// 再跑同一条是重放：方案、采用与两笔确认都认已有的那一笔，不另起。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pricingstore "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func main() {
	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IDP_PARCEL_POSTGRES_DSN 未设置——仓储收费种子不猜连接串")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库：%v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造框架 DB：%v\n", err)
		os.Exit(1)
	}
	if err := walk(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "仓储收费种子：%v\n", err)
		os.Exit(1)
	}
	fmt.Println("== 仓储收费：方案已登记，采用已记下，入库与仓储各确认一笔 ==")
}

func walk(ctx context.Context, db *bentopg.DB) error {
	plans, err := pricingstore.NewWarehousePlans(db)
	if err != nil {
		return err
	}
	store, err := adapter.NewWarehouseCharges(db)
	if err != nil {
		return err
	}
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		return err
	}
	mappings, err := adapter.NewFeeItemMappings(db)
	if err != nil {
		return err
	}
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		return err
	}
	conditions, err := adapter.NewChargeConfirmationConditions(db)
	if err != nil {
		return err
	}
	moments, err := adapter.NewSettlementMoments(db)
	if err != nil {
		return err
	}
	supersessions, err := adapter.NewChargeSupersessions(db)
	if err != nil {
		return err
	}
	momentHandler, err := application.NewAdmitSettlementMomentHandler(moments)
	if err != nil {
		return err
	}
	when := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	pricing := seedPricing{handler: pricingapp.NewWarehousePlanHandler(plans, true, &countingIDs{prefix: "WEV-SEED"})}
	handler := application.NewWarehouseChargeHandler(
		store, pricing, accounts, mappings, charges, conditions, conditions, momentHandler, supersessions,
		&countingIDs{prefix: "CHG-SEED"}, seedClock{at: when},
	)
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return err
	}
	account, err := seedAccount()
	if err != nil {
		return err
	}
	return db.Transactor().WithinTransaction(ctx, func(ctx context.Context) error {
		if effect, err := accounts.Save(ctx, tenant, account, when); err != nil {
			return err
		} else if effect != ports.SettlementAccountRegistered && effect != ports.SettlementAccountReplay {
			return fmt.Errorf("结算账户 %v", effect)
		}
		if outcome, err := pricing.handler.Register(ctx, tenant.String(), seedPlan()); err != nil {
			return err
		} else if outcome != pricingapp.WarehousePlanRecorded && outcome != pricingapp.WarehousePlanExisting {
			return fmt.Errorf("方案 %s", outcome)
		}
		adopted, err := handler.Adopt(ctx, tenant, application.WarehouseAdoption{
			Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01", Revision: 1,
			Effective: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
			Account:   account.ID().String(), Scope: "SYN-SCOPE-WH", Basis: "SYN-BASIS", Registrant: "operator-1",
		})
		if err != nil {
			return err
		}
		if adopted != application.WarehouseChargeFormed && adopted != application.WarehouseChargeExisting {
			return fmt.Errorf("采用 %s", adopted)
		}
		if err := saveMapping(ctx, mappings, tenant, "INBOUND_PIECE", when); err != nil {
			return err
		}
		if err := saveMapping(ctx, mappings, tenant, "STORAGE_PIECE", when); err != nil {
			return err
		}
		feeItem, err := domain.NewFeeItemReference("SYN-FEE-WH")
		if err != nil {
			return err
		}
		if _, err := conditions.RegisterCondition(ctx, tenant, feeItem, "WAREHOUSE_WORK_OCCURRED", when); err != nil {
			return err
		}
		if effect, err := moments.SaveSettlementMoment(ctx, tenant, domain.SettlementMomentConfirm, when); err != nil {
			return err
		} else if effect != ports.CatalogueRegistered && effect != ports.CatalogueReplay {
			return fmt.Errorf("确认时点 %v", effect)
		}
		if err := settleConfirmed(ctx, handler, charges, tenant, seedInbound(), 400); err != nil {
			return err
		}
		return settleConfirmed(ctx, handler, charges, tenant, seedStorage(), 100)
	})
}

func settleConfirmed(
	ctx context.Context,
	handler *application.WarehouseChargeHandler,
	charges interface {
		FindByID(context.Context, domain.TenantID, domain.CustomerChargeID) (domain.CustomerCharge, bool, error)
	},
	tenant domain.TenantID,
	fact application.WarehouseOccurrenceFact,
	wantMinor int64,
) error {
	outcome, chargeID, err := handler.Settle(ctx, tenant, fact)
	if err != nil {
		return err
	}
	if outcome != application.WarehouseChargeConfirmed && outcome != application.WarehouseChargeExisting {
		return fmt.Errorf("%s %s", fact.ID, outcome)
	}
	id, err := domain.NewCustomerChargeID(chargeID)
	if err != nil {
		return err
	}
	stored, found, err := charges.FindByID(ctx, tenant, id)
	if err != nil {
		return err
	}
	if !found || stored.Stage() != domain.ChargeConfirmed {
		return fmt.Errorf("%s 未确认", fact.ID)
	}
	if _, minor := stored.OriginalAmount(); minor != wantMinor {
		return fmt.Errorf("%s 金额 %d", fact.ID, minor)
	}
	return nil
}

func saveMapping(ctx context.Context, mappings *adapter.FeeItemMappings, tenant domain.TenantID, code string, at time.Time) error {
	mapping, err := domain.NewFeeItemMapping(
		mustRef(domain.NewPricingChargeCode, code),
		mustRef(domain.NewFeeItemReference, "SYN-FEE-WH"),
		mustRef(domain.NewFeeItemMappingBasisReference, "SYN-BASIS"),
	)
	if err != nil {
		return err
	}
	effect, err := mappings.SaveFeeItemMapping(ctx, tenant, mapping, at)
	if err != nil {
		return err
	}
	if effect != ports.CatalogueRegistered && effect != ports.CatalogueReplay {
		return fmt.Errorf("映射 %s %v", code, effect)
	}
	return nil
}

func seedAccount() (domain.SettlementAccount, error) {
	key, err := domain.NewSettlementAccountKey(
		mustRef(domain.NewLegalEntityReference, "SYN-LE-WH"),
		mustRef(domain.NewSettlementCounterpartyReference, "SYN-OWNER-01"),
		domain.ChargeReceivable,
		mustRef(domain.NewCurrencyCode, "CNY"),
		mustRef(domain.NewSettlementPolicyReference, "SYN-POL-WH"),
	)
	if err != nil {
		return domain.SettlementAccount{}, err
	}
	return domain.NewSettlementAccount(
		mustRef(domain.NewSettlementAccountID, "SYN-ACCT-WH"),
		key, domain.SettlementCounterpartyReference{}, false,
		mustRef(domain.NewResponsibilityBasis, "SYN-CONTRACT-WH"),
	)
}

func seedPlan() pricingdomain.WarehousePlan {
	return pricingdomain.WarehousePlan{
		ID: "SYN-PLAN-WH-01", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []pricingdomain.WarehouseChargeGroup{
			{Name: "INBOUND", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", OnMiss: "PENDING"},
			{Name: "STORAGE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", OnMiss: "PENDING"},
		},
		Rules: []pricingdomain.WarehouseChargeRule{
			{Code: "INBOUND_PIECE", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", Group: "INBOUND", Priority: 1, Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT"},
			{Code: "STORAGE_FREE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 1, Method: "WAIVE", When: []pricingdomain.WarehouseCondition{
				{MeasureUnit: "DAY", Operator: "LT", Threshold: "30"},
			}},
			{Code: "STORAGE_PIECE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 1, Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 50, Rounding: "EXACT", When: []pricingdomain.WarehouseCondition{
				{MeasureUnit: "DAY", Operator: "GE", Threshold: "30"},
				{MeasureClass: "VOLUME", MeasureUnit: "M3", Operator: "GT", Threshold: "0"},
			}},
		},
	}
}

func seedInbound() application.WarehouseOccurrenceFact {
	return application.WarehouseOccurrenceFact{
		ID: "CHO-SEED-IN", Version: 1, Kind: "INBOUND_HANDLING", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01",
		Occurred: time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC), Correction: "ORIGINAL",
		Measures: []application.WarehouseFactMeasure{{Unit: "PIECE", Quantity: "4"}},
	}
}

func seedStorage() application.WarehouseOccurrenceFact {
	return application.WarehouseOccurrenceFact{
		ID: "CHO-SEED-STORE", Version: 1, Kind: "STORAGE", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01",
		Occurred: time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC), Correction: "ORIGINAL",
		Lines: []application.WarehouseFactLine{
			{Kind: "INBOUND_BATCH", Measures: []application.WarehouseFactMeasure{
				{Unit: "PIECE", Quantity: "9"},
				{Class: "VOLUME", Unit: "M3", Quantity: "1.2"},
				{Unit: "DAY", Quantity: "10"},
			}},
			{Kind: "INBOUND_BATCH", Measures: []application.WarehouseFactMeasure{
				{Unit: "PIECE", Quantity: "2"},
				{Class: "VOLUME", Unit: "M3", Quantity: "0.4"},
				{Unit: "DAY", Quantity: "30"},
			}},
		},
	}
}

type seedPricing struct {
	handler *pricingapp.WarehousePlanHandler
}

func (bridge seedPricing) Price(ctx context.Context, tenant, scope string, at time.Time, fact application.WarehouseOccurrenceFact) (application.WarehouseChargePrice, error) {
	snapshot := pricingdomain.WarehouseOccurrenceSnapshot{
		ID: fact.ID, Version: fact.Version, Kind: fact.Kind, Owner: fact.Owner, Warehouse: fact.Warehouse, Occurred: fact.Occurred,
		Correction: fact.Correction,
	}
	for _, measure := range fact.Measures {
		snapshot.Measures = append(snapshot.Measures, seedMeasure(measure))
	}
	for _, line := range fact.Lines {
		mapped := pricingdomain.WarehouseOccurrenceLine{Kind: line.Kind}
		for _, measure := range line.Measures {
			mapped.Measures = append(mapped.Measures, seedMeasure(measure))
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

func seedMeasure(measure application.WarehouseFactMeasure) pricingdomain.WarehouseMeasure {
	return pricingdomain.WarehouseMeasure{
		Unit: measure.Unit, Quantity: measure.Quantity, Unmeasured: measure.Unmeasured,
		Declared: measure.Declared, DeclaredUnit: measure.DeclaredUnit, Class: measure.Class,
	}
}

type countingIDs struct {
	prefix string
	n      int
}

func (ids *countingIDs) Next() (string, error) {
	ids.n++
	return fmt.Sprintf("%s-%d", ids.prefix, ids.n), nil
}

type seedClock struct{ at time.Time }

func (clock seedClock) Now() time.Time { return clock.at }

func mustRef[T any](parse func(string) (T, error), value string) T {
	parsed, err := parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}
