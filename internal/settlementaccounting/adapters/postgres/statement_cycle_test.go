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

// 钉对账周期册与截止记录的库面（迁移 0049，ADR-0339 决定三）：一户一份周期，同内容重放、异内容冲突不覆盖；截止记录读回最后一个
// 截止日；各写口在事务外按框架合同拒。真库，合成串。

func dailyCycle(t *testing.T, since string) domain.StatementCycle {
	t.Helper()
	date, err := domain.ParseAttributionDate(since)
	if err != nil {
		t.Fatal(err)
	}
	cycle, err := domain.NewStatementCycle(mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1"), domain.DailyCycle, nil, nil, date)
	if err != nil {
		t.Fatal(err)
	}
	return cycle
}

func TestTheStatementCycleWritesRefuseToRunOutsideATransaction(t *testing.T) {
	cycles, err := adapter.NewStatementCycles(catalogueDB(t))
	if err != nil {
		t.Fatal(err)
	}
	tenant := settlementTenant(t)
	account := mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1")
	cny := mustAccountRef(t, domain.NewCurrencyCode, "CNY")
	closing, _ := domain.ParseAttributionDate("2026-10-31")
	if _, err := cycles.SaveStatementCycle(t.Context(), tenant, dailyCycle(t, "2026-10-01"), time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveStatementCycle err = %v", err)
	}
	if _, _, err := cycles.LastCycleCut(t.Context(), tenant, account); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("LastCycleCut err = %v", err)
	}
	if err := cycles.RecordCycleCut(t.Context(), tenant, account, ports.StatementCycleCut{Closing: closing, Outcome: "NO_CHARGE", CutAt: time.Now()}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("RecordCycleCut err = %v", err)
	}
	if _, err := cycles.ListCycleCutCandidates(t.Context(), tenant, account, cny, closing); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ListCycleCutCandidates err = %v", err)
	}
	if _, err := cycles.ListUnattributedConfirmedCharges(t.Context(), tenant, account); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("ListUnattributedConfirmedCharges err = %v", err)
	}
}

// Covers: AT-SA-224——一户一份周期：同内容重登答重放，异内容答冲突、册上那一份不动；截止记录读回最后一个截止日。
func TestAStatementCycleRegistersOnceAndTheLedgerReadsTheLastClosing(t *testing.T) {
	db := catalogueDB(t)
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		t.Fatal(err)
	}
	cycles, err := adapter.NewStatementCycles(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := settlementTenant(t)
	account := mustAccountRef(t, domain.NewSettlementAccountID, "ACCT-1")
	first, _ := domain.ParseAttributionDate("2026-10-30")
	second, _ := domain.ParseAttributionDate("2026-10-31")
	err = db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
		if _, err := accounts.Save(ctx, tenant, settlementAccountOutsideATransaction(t), time.Now()); err != nil {
			return err
		}
		for _, step := range []struct {
			since string
			want  ports.CatalogueRegistrationEffect
		}{{"2026-10-01", ports.CatalogueRegistered}, {"2026-10-01", ports.CatalogueReplay}, {"2026-10-15", ports.CatalogueConflict}} {
			if effect, err := cycles.SaveStatementCycle(ctx, tenant, dailyCycle(t, step.since), time.Now()); err != nil || effect != step.want {
				return errOrEffect(err, effect, step.want)
			}
		}
		if _, found, err := cycles.LastCycleCut(ctx, tenant, account); err != nil || found {
			return errors.Join(err, errors.New("还没截过却读到了截止日"))
		}
		for _, closing := range []domain.AttributionDate{first, second} {
			if err := cycles.RecordCycleCut(ctx, tenant, account, ports.StatementCycleCut{Closing: closing, Outcome: "NO_CHARGE", CutAt: time.Now()}); err != nil {
				return err
			}
		}
		last, found, err := cycles.LastCycleCut(ctx, tenant, account)
		if err != nil || !found || last != second {
			return errors.Join(err, errors.New("最后截止日读错了："+last.String()))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	records, err := cycles.ListStatementCycles(t.Context())
	if err != nil || len(records) != 1 || records[0].Cycle.Since().String() != "2026-10-01" || records[0].Tenant != tenant {
		t.Fatalf("周期册 = %+v err = %v，want 一份、起算日仍是先登的那一天", records, err)
	}
}
