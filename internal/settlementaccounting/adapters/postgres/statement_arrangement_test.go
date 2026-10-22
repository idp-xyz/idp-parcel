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

func TestStatementArrangementsRefuseToRunOutsideATransaction(t *testing.T) {
	arrangements, err := adapter.NewStatementArrangements(catalogueDB(t))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := arrangements.SaveStatementArrangement(t.Context(), registerTenant(t), statementArrangementOf(t, "ACCT-1", "UTC", 18*60), time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("事务外 Save 应答 ErrTransactionRequired，实得：%v", err)
	}
}

// Covers: ADR-0269 决定一——对账安排按（租户、结算账户）登记、可追加新一版、读最新一版；与最新一版同内容是重放；
// 换回更早一版的内容也落新一版；没登记答 found=false；账户不在账户册上由外键拒、行不落。
func TestAStatementArrangementAppendsRevisionsAndReadsTheLatest(t *testing.T) {
	db := catalogueDB(t)
	arrangements, err := adapter.NewStatementArrangements(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := registerTenant(t)
	account := catalogueRef(t, domain.NewSettlementAccountID, "ACCT-ARR-1")
	at := time.Date(2026, 10, 21, 9, 0, 0, 0, time.UTC)
	save := func(arrangement domain.StatementArrangement) (ports.CatalogueRegistrationEffect, error) {
		var effect ports.CatalogueRegistrationEffect
		err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			var saveErr error
			effect, saveErr = arrangements.SaveStatementArrangement(ctx, tenant, arrangement, at)
			return saveErr
		})
		return effect, err
	}
	shanghai := statementArrangementOf(t, "ACCT-ARR-1", "Asia/Shanghai", 18*60)
	midnightUTC := statementArrangementOf(t, "ACCT-ARR-1", "UTC", 0)

	if _, err := save(shanghai); !errors.Is(err, domain.ErrCatalogueTargetMissing) {
		t.Fatalf("账户不在册上 err = %v", err)
	}
	if _, found, err := arrangements.LoadStatementArrangement(t.Context(), tenant, account); err != nil || found {
		t.Fatalf("空册 found=%v err=%v", found, err)
	}
	seedSettlementAccount(t, db, tenant, "ACCT-ARR-1")

	for _, step := range []struct {
		name        string
		arrangement domain.StatementArrangement
		want        ports.CatalogueRegistrationEffect
		latest      domain.StatementArrangement
	}{
		{"first revision", shanghai, ports.CatalogueRegistered, shanghai},
		{"replay of the latest", shanghai, ports.CatalogueReplay, shanghai},
		{"a new revision", midnightUTC, ports.CatalogueRegistered, midnightUTC},
		{"back to the earlier content is another revision", shanghai, ports.CatalogueRegistered, shanghai},
	} {
		effect, err := save(step.arrangement)
		if err != nil || effect != step.want {
			t.Fatalf("%s：effect = %d err = %v, want %d", step.name, effect, err, step.want)
		}
		latest, found, err := arrangements.LoadStatementArrangement(t.Context(), tenant, account)
		if err != nil || !found || !latest.SameArrangement(step.latest) {
			t.Fatalf("%s：最新一版 = %s/%d found=%v err=%v", step.name, latest.TimeZone(), latest.CutoffMinute(), found, err)
		}
	}
	var revisions int
	if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		executor, err := db.RequireExecutor(ctx)
		if err != nil {
			return err
		}
		return executor.QueryRow(ctx, `SELECT count(*) FROM settlement_accounting.statement_arrangement
			WHERE tenant_id = $1 AND account_id = $2`, tenant.String(), account.String()).Scan(&revisions)
	}); err != nil {
		t.Fatal(err)
	}
	if revisions != 3 {
		t.Fatalf("册上 %d 版，want 3（旧版留作历史）", revisions)
	}
	other := catalogueRef(t, domain.NewTenantID, "tenant-b")
	if _, found, err := arrangements.LoadStatementArrangement(t.Context(), other, account); err != nil || found {
		t.Fatalf("别的租户读到了这一户的对账安排 found=%v err=%v", found, err)
	}
}

func statementArrangementOf(t *testing.T, account, zone string, cutoff int) domain.StatementArrangement {
	t.Helper()
	arrangement, err := domain.NewStatementArrangement(catalogueRef(t, domain.NewSettlementAccountID, account), zone, cutoff)
	if err != nil {
		t.Fatal(err)
	}
	return arrangement
}
