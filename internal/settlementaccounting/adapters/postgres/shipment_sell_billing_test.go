package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

func TestShipmentBillingMomentAdoptionRefusesAWriteOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	adoptions, err := adapter.NewShipmentBillingMomentAdoptions(db)
	if err != nil {
		t.Fatalf("构造采用册：%v", err)
	}
	adoption, err := domain.NewShipmentBillingMomentAdoption(
		mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1"),
		domain.ShipmentMembersIntakeAdopted,
		domain.EstimatePricingScope{},
	)
	if err != nil {
		t.Fatalf("采用行：%v", err)
	}
	if _, err := adoptions.SaveShipmentBillingMomentAdoption(t.Context(), settlementTenant(t), adoption, time.Now().UTC()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外写入 = %v，想要 ErrTransactionRequired", err)
	}
	_, found, err := adoptions.FindShipmentBillingMomentAdoption(t.Context(), settlementTenant(t), adoption.Account())
	if err != nil || found {
		t.Fatalf("空册 found=%v err=%v，读口应答没有这一行", found, err)
	}
}

func TestShipmentSellRequestRefusesAWriteOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewShipmentSellEvaluationRequests(db)
	if err != nil {
		t.Fatalf("构造请求册：%v", err)
	}
	request := shipmentSellRequest(t)
	record := ports.ShipmentSellEvaluationRequestRecord{
		Key: ports.ShipmentSellEvaluationRequestKey{
			TenantID: settlementTenant(t), Request: request.ID(),
		},
		Request: request, RecordedAt: time.Now().UTC(), State: domain.SellRequestSubmitted,
	}
	if _, err := requests.Save(t.Context(), record); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save = %v，想要 ErrTransactionRequired", err)
	}
	if _, err := requests.RecordConclusion(t.Context(), record.Key, domain.NewZeroTotalSellEvaluationRequestConclusion()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 RecordConclusion = %v，想要 ErrTransactionRequired", err)
	}
}

func shipmentSellRequest(t *testing.T) domain.ShipmentSellEvaluationRequest {
	t.Helper()
	request, err := domain.SubmitShipmentSellEvaluationRequest(domain.ShipmentSellEvaluationRequestSpec{
		ID:                 mustAccountRef(t, domain.NewShipmentSellEvaluationRequestID, "SHPREQ-SYN-1"),
		Shipment:           mustAccountRef(t, domain.NewShipmentReference, "SYN-SHIP-1"),
		Members:            []domain.ParcelReference{mustAccountRef(t, domain.NewParcelReference, "SYN-PARCEL-1")},
		Account:            mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1"),
		SettlementCurrency: mustAccountRef(t, domain.NewCurrencyCode, "XTS"),
		PricingScope:       mustAccountRef(t, domain.NewEstimatePricingScope, "SYN-SCOPE-1"),
		Moment:             domain.ShipmentMembersIntakeAdopted,
		Source:             mustAccountRef(t, domain.NewSourceFactReference, "shipment-members-intake/SYN-SHIP-1"),
		PricingBasisAt:     time.Date(2026, 10, 18, 7, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("请求：%v", err)
	}
	return request
}

func TestShipmentSellRequestRoundTripsInsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	requests, err := adapter.NewShipmentSellEvaluationRequests(db)
	if err != nil {
		t.Fatalf("请求册：%v", err)
	}
	tenant := settlementTenant(t)
	at := time.Date(2026, 10, 19, 9, 0, 0, 0, time.UTC)
	request := shipmentSellRequest(t)
	record := ports.ShipmentSellEvaluationRequestRecord{
		Key:        ports.ShipmentSellEvaluationRequestKey{TenantID: tenant, Request: request.ID()},
		Request:    request,
		RecordedAt: at,
		State:      domain.SellRequestSubmitted,
	}
	var saved ports.EvaluationRequestSaveOutcome
	if err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		var err error
		saved, err = requests.Save(txCtx, record)
		return err
	}); err != nil || saved != ports.EvaluationRequestSaved {
		t.Fatalf("保存 = %v %v", saved, err)
	}
	loaded, found, err := requests.FindByNaturalKey(t.Context(), tenant, domain.ShipmentSellEvaluationRequestNaturalKey{
		Shipment: request.Shipment(), Moment: request.Moment(),
	})
	if err != nil || !found || !loaded.Request.SameQuestion(request) || loaded.State != domain.SellRequestSubmitted {
		t.Fatalf("读回 found=%v err=%v state=%s", found, err, loaded.State)
	}
}
