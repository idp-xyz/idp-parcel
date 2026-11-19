package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
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
	if err := supersessions.SaveSupersessionWithoutSuccessor(context.Background(), tenant, charge, time.Now()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务的没有后继的替代应返回 ErrTransactionRequired，实得：%v", err)
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

// Covers: 没有后继的替代（新评价合计为零）存得进，Superseded 答是，客户费用册读回的行 superseded 为真、supersededBy 为空；
// 之后一代形成新预估，给出后继就补上；已有后继的不被再改写——再来的没有后继的替代不动它（另一个后继的那一格见
// TestASupersessionNamingAnotherSuccessorIsRefusedAndLeavesTheRowAsItWas）。
func TestASupersessionWithoutASuccessorIsStoredAndTheSuccessorLaterFillsItIn(t *testing.T) {
	db := catalogueDB(t)
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	supersessions, err := adapter.NewChargeSupersessions(db)
	if err != nil {
		t.Fatal(err)
	}
	catalogue, err := adapter.NewChargeCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := saTenant(t, "tenant-a")
	zeroed, successor := estimatedCharge(t, "charge-zero-1"), estimatedCharge(t, "charge-zero-2")
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		for _, charge := range []domain.CustomerCharge{zeroed, successor} {
			if _, err := charges.SaveEstimated(ctx, tenant, charge); err != nil {
				return err
			}
		}
		return nil
	})
	// read 读回被替代那笔在客户费用册上的行，同时问确认口用的 Superseded。
	read := func() (supersededBy string, superseded bool, supersededFlag bool) {
		t.Helper()
		rows, err := rowsOf(catalogue.ListCustomerCharges(t.Context(), tenant, 10, firstPage(t, ports.CustomerChargePages)))
		if err != nil {
			t.Fatalf("上列客户费用：%v", err)
		}
		for _, row := range rows {
			if row.Charge == zeroed.ID().String() {
				supersededBy, supersededFlag = row.SupersededBy, row.Superseded
			}
		}
		superseded, err = supersessions.Superseded(t.Context(), tenant, zeroed.ID())
		if err != nil {
			t.Fatal(err)
		}
		return supersededBy, superseded, supersededFlag
	}
	if by, superseded, flag := read(); by != "" || superseded || flag {
		t.Fatalf("没记替代时读到 supersededBy=%q Superseded=%v superseded=%v", by, superseded, flag)
	}

	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		return supersessions.SaveSupersessionWithoutSuccessor(ctx, tenant, zeroed.ID(), stageLockAt)
	})
	if by, superseded, flag := read(); by != "" || !superseded || !flag {
		t.Fatalf("没有后继的替代读到 supersededBy=%q Superseded=%v superseded=%v，want 空、是、真", by, superseded, flag)
	}

	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		return supersessions.SaveSupersession(ctx, tenant, zeroed.ID(), successor.ID(), stageLockAt.Add(time.Hour))
	})
	if by, superseded, flag := read(); by != successor.ID().String() || !superseded || !flag {
		t.Fatalf("补上后继之后读到 supersededBy=%q Superseded=%v superseded=%v，want %s", by, superseded, flag, successor.ID())
	}

	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		return supersessions.SaveSupersessionWithoutSuccessor(ctx, tenant, zeroed.ID(), stageLockAt.Add(3*time.Hour))
	})
	if by, superseded, _ := read(); by != successor.ID().String() || !superseded {
		t.Fatalf("已有后继的替代被改写：supersededBy=%q Superseded=%v，want 仍是 %s", by, superseded, successor.ID())
	}
}

// supersessionOf 读替代册上被替代那一笔的原样：后继（空串即没有后继）与最初记下的时刻；行不在则 found 为假。
func supersessionOf(
	t *testing.T,
	db *bentopg.DB,
	tenant domain.TenantID,
	charge domain.CustomerChargeID,
) (successor string, recordedAt time.Time, found bool) {
	t.Helper()
	querier, err := db.ReadExecutor(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var stored *string
	err = querier.QueryRow(t.Context(), `
		SELECT superseded_by_charge_id, recorded_at
		  FROM settlement_accounting.customer_charge_supersession
		 WHERE tenant_id = $1 AND charge_id = $2`,
		tenant.String(), charge.String(),
	).Scan(&stored, &recordedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", time.Time{}, false
	}
	if err != nil {
		t.Fatal(err)
	}
	if stored != nil {
		successor = *stored
	}
	return successor, recordedAt, true
}

// Covers: 记替代撞上「已有另一个后继」不能静默不写——同一后继记两次都成功（重放，行不变、recorded_at 仍是最初那一刻）；
// 已有行带着另一个后继，答 ports.ErrChargeSupersededByAnother，行一字不改；已有行的后继为空时给出后继就补上。
func TestASupersessionNamingAnotherSuccessorIsRefusedAndLeavesTheRowAsItWas(t *testing.T) {
	db := catalogueDB(t)
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		t.Fatal(err)
	}
	supersessions, err := adapter.NewChargeSupersessions(db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := saTenant(t, "tenant-a")
	replaced, successor, other := estimatedCharge(t, "charge-dup-1"), estimatedCharge(t, "charge-dup-2"), estimatedCharge(t, "charge-dup-3")
	hollow := estimatedCharge(t, "charge-dup-4")
	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		for _, charge := range []domain.CustomerCharge{replaced, successor, other, hollow} {
			if _, err := charges.SaveEstimated(ctx, tenant, charge); err != nil {
				return err
			}
		}
		return nil
	})
	save := func(charge, by domain.CustomerChargeID, at time.Time) error {
		return db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			return supersessions.SaveSupersession(ctx, tenant, charge, by, at)
		})
	}

	if err := save(replaced.ID(), successor.ID(), stageLockAt); err != nil {
		t.Fatalf("首次记替代：%v", err)
	}
	if err := save(replaced.ID(), successor.ID(), stageLockAt.Add(time.Hour)); err != nil {
		t.Fatalf("同一后继重放应算成功，实得：%v", err)
	}
	if by, at, found := supersessionOf(t, db, tenant, replaced.ID()); !found || by != successor.ID().String() || !at.Equal(stageLockAt) {
		t.Fatalf("重放之后替代行 found=%v 后继=%q 时刻=%s，want 后继 %s、时刻 %s", found, by, at, successor.ID(), stageLockAt)
	}

	err = save(replaced.ID(), other.ID(), stageLockAt.Add(2*time.Hour))
	if !errors.Is(err, ports.ErrChargeSupersededByAnother) {
		t.Fatalf("另一个后继应答 ErrChargeSupersededByAnother，实得：%v", err)
	}
	if by, at, found := supersessionOf(t, db, tenant, replaced.ID()); !found || by != successor.ID().String() || !at.Equal(stageLockAt) {
		t.Fatalf("被拒之后替代行 found=%v 后继=%q 时刻=%s，want 一字未改：后继 %s、时刻 %s", found, by, at, successor.ID(), stageLockAt)
	}

	saWithin(t, db.Transactor(), t.Context(), func(ctx context.Context) error {
		return supersessions.SaveSupersessionWithoutSuccessor(ctx, tenant, hollow.ID(), stageLockAt)
	})
	if by, _, found := supersessionOf(t, db, tenant, hollow.ID()); !found || by != "" {
		t.Fatalf("没有后继的替代 found=%v 后继=%q，want 行在、后继为空", found, by)
	}
	if err := save(hollow.ID(), successor.ID(), stageLockAt.Add(time.Hour)); err != nil {
		t.Fatalf("空后继补上后继：%v", err)
	}
	if by, at, found := supersessionOf(t, db, tenant, hollow.ID()); !found || by != successor.ID().String() || !at.Equal(stageLockAt) {
		t.Fatalf("补上之后替代行 found=%v 后继=%q 时刻=%s，want 后继 %s、时刻仍是 %s", found, by, at, successor.ID(), stageLockAt)
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
