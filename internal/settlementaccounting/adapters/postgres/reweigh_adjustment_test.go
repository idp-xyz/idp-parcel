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
)

func TestReweighWritesRefuseToRunOutsideATransaction(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatal(err)
	}
	supersessions, err := adapter.NewChargeSupersessions(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := mustSA(t, domain.NewTenantID, "SYN-TENANT-01")
	charge := mustSA(t, domain.NewCustomerChargeID, "SYN-CHARGE-1")
	other := mustSA(t, domain.NewCustomerChargeID, "SYN-CHARGE-2")
	if err := supersessions.SaveSupersession(context.Background(), tenant, charge, other, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务替代应返回 ErrTransactionRequired，实得：%v", err)
	}
	adoptions, err := adapter.NewReweighAdjustmentAdoptions(db)
	if err != nil {
		t.Fatal(err)
	}
	account := mustSA(t, domain.NewSettlementAccountID, "SYN-ACCT-1")
	if _, err := adoptions.SaveReweighAdjustmentAdoption(context.Background(), tenant, account, "basis-1", time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务采用应返回 ErrTransactionRequired，实得：%v", err)
	}
}

func mustSA[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatal(err)
	}
	return value
}
