package postgres_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证派送待形成册（迁移 0045，ADR-0230 决定一、二）：立与刷新、了结不改指、只作罢待形成的、按租户与
// 最近一次答复时刻列队、无事务拒，以及库内 CHECK 挡住领域给不出的行。夹具全部合成（S 级）。

var pendingAnsweredAt = time.Date(2026, 10, 21, 3, 0, 0, 0, time.UTC)

func newDeliveryDispatchPendings(t *testing.T) (*adapter.DeliveryDispatchPendings, bentoapp.Transactor, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewDeliveryDispatchPendings(db)
	if err != nil {
		t.Fatalf("构造派送待形成册：%v", err)
	}
	return register, db.Transactor(), db
}

func pendingKey(t *testing.T, tenant, segment, object string) ports.DeliveryDispatchPendingKey {
	t.Helper()
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		t.Fatal(err)
	}
	segmentRef, err := domain.NewFulfillmentSegmentReference(segment)
	if err != nil {
		t.Fatal(err)
	}
	objectRef, err := domain.NewCarriedObjectReference(object)
	if err != nil {
		t.Fatal(err)
	}
	return ports.DeliveryDispatchPendingKey{TenantID: tenantID, Segment: segmentRef, Object: objectRef}
}

func pendingBasis(t *testing.T, raw string) domain.ParticipationBasisReference {
	t.Helper()
	basis, err := domain.NewParticipationBasisReference(raw)
	if err != nil {
		t.Fatal(err)
	}
	return basis
}

func withinPendingTransaction(t *testing.T, transactor bentoapp.Transactor, fn func(context.Context) error) {
	t.Helper()
	if err := transactor.WithinTransaction(t.Context(), fn); err != nil {
		t.Fatalf("事务内写册：%v", err)
	}
}

func mustFindPending(t *testing.T, register *adapter.DeliveryDispatchPendings, key ports.DeliveryDispatchPendingKey) ports.DeliveryDispatchPendingRecord {
	t.Helper()
	record, found, err := register.FindByKey(t.Context(), key)
	if err != nil || !found {
		t.Fatalf("读回 %v：found=%v err=%v", key, found, err)
	}
	return record
}

// Covers: AT-TF-281（ADR-0230 决定一）——立一条带齐段、对象、入场依据、缺件与答复时刻；再记一次仍是一条，入场依据、缺件、最近一次
// 答复时刻换成这一次，首次答复时刻不动。
func TestAPendingEntryIsRecordedOnceAndRefreshed(t *testing.T) {
	register, transactor, _ := newDeliveryDispatchPendings(t)
	key := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-A")
	withinPendingTransaction(t, transactor, func(ctx context.Context) error {
		return register.RecordMissing(ctx, key, pendingBasis(t, "basis/v1"), []string{"DELIVERY_PLACE", "DELIVERY_CONDITION"}, pendingAnsweredAt)
	})
	later := pendingAnsweredAt.Add(2 * time.Hour)
	withinPendingTransaction(t, transactor, func(ctx context.Context) error {
		return register.RecordMissing(ctx, key, pendingBasis(t, "basis/v2"), []string{"DELIVERY_CONDITION"}, later)
	})
	record := mustFindPending(t, register, key)
	if record.State != ports.DeliveryDispatchPending || record.EntryBasis.String() != "basis/v2" ||
		!slices.Equal(record.Missing, []string{"DELIVERY_CONDITION"}) ||
		!record.FirstAnsweredAt.Equal(pendingAnsweredAt) || !record.LastAnsweredAt.Equal(later) {
		t.Fatalf("刷新之后 = %+v", record)
	}
	listed, err := register.ListPending(t.Context(), key.TenantID, time.Time{}, 10)
	if err != nil || len(listed) != 1 {
		t.Fatalf("列队 = %v err=%v, want 一条", listed, err)
	}
}

// Covers: AT-TF-282、AT-TF-283（ADR-0230 决定一）——了结与作罢：了结不改指先落下的那一项；作罢只动待形成的那一条；了结或作罢的
// 不再列队；之后又答要求缺失（参与重新在场）改回待形成。册上没有的那一件了结什么也不写。
func TestSettlementAndLapseFollowTheLatestAnswer(t *testing.T) {
	register, transactor, _ := newDeliveryDispatchPendings(t)
	formed := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-B")
	lapsed := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-C")
	absent := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-D")
	task := func(raw string) domain.DispatchTaskReference {
		reference, err := domain.NewDispatchTaskReference(raw)
		if err != nil {
			t.Fatal(err)
		}
		return reference
	}
	withinPendingTransaction(t, transactor, func(ctx context.Context) error {
		for _, key := range []ports.DeliveryDispatchPendingKey{formed, lapsed} {
			if err := register.RecordMissing(ctx, key, pendingBasis(t, "basis/v1"), []string{"DELIVERY_WINDOW"}, pendingAnsweredAt); err != nil {
				return err
			}
		}
		if err := register.SettleFormed(ctx, formed, task("DELIVERY-DISPATCH/task-1"), pendingAnsweredAt.Add(time.Hour)); err != nil {
			return err
		}
		if err := register.SettleFormed(ctx, formed, task("DELIVERY-DISPATCH/task-2"), pendingAnsweredAt.Add(2*time.Hour)); err != nil {
			return err
		}
		if err := register.SettleLapsed(ctx, formed, "PARTICIPATION_NOT_ACTIVE", pendingAnsweredAt.Add(3*time.Hour)); err != nil {
			return err
		}
		if err := register.SettleLapsed(ctx, lapsed, "PARTICIPATION_NOT_ACTIVE", pendingAnsweredAt.Add(time.Hour)); err != nil {
			return err
		}
		return register.SettleFormed(ctx, absent, task("DELIVERY-DISPATCH/task-3"), pendingAnsweredAt)
	})
	if record := mustFindPending(t, register, formed); record.State != ports.DeliveryDispatchPendingFormed ||
		record.Task != "DELIVERY-DISPATCH/task-1" || !record.SettledAt.Equal(pendingAnsweredAt.Add(time.Hour)) {
		t.Fatalf("已形成那一条 = %+v, want 指名先落下的 task-1、作罢不动它", record)
	}
	if record := mustFindPending(t, register, lapsed); record.State != ports.DeliveryDispatchPendingLapsed ||
		record.LapsedBy != "PARTICIPATION_NOT_ACTIVE" {
		t.Fatalf("已作罢那一条 = %+v", record)
	}
	if _, found, err := register.FindByKey(t.Context(), absent); err != nil || found {
		t.Fatalf("册上没有的那一件被了结写进了册：found=%v err=%v", found, err)
	}
	if listed, err := register.ListPending(t.Context(), formed.TenantID, time.Time{}, 10); err != nil || len(listed) != 0 {
		t.Fatalf("了结与作罢之后列队 = %v err=%v, want 空", listed, err)
	}

	withinPendingTransaction(t, transactor, func(ctx context.Context) error {
		return register.RecordMissing(ctx, lapsed, pendingBasis(t, "basis/v3"), []string{"DELIVERY_PLACE"}, pendingAnsweredAt.Add(4*time.Hour))
	})
	if record := mustFindPending(t, register, lapsed); record.State != ports.DeliveryDispatchPending ||
		record.LapsedBy != "" || !record.SettledAt.IsZero() || !record.FirstAnsweredAt.Equal(pendingAnsweredAt) {
		t.Fatalf("重新待形成 = %+v, want 待形成、作罢三件清空、首次答复时刻不动", record)
	}
}

// Covers: AT-TF-284、AT-TF-286 的队列那一半（ADR-0230 决定三、五）——列队按租户隔离、最近一次答复最早的在前；给了截止时刻只列
// 不晚于它的；limit 截断。
func TestThePendingQueueIsPerTenantOldestFirstAndFiltersByDueTime(t *testing.T) {
	register, transactor, _ := newDeliveryDispatchPendings(t)
	withinPendingTransaction(t, transactor, func(ctx context.Context) error {
		for index, object := range []string{"SYN-PARCEL-70-E", "SYN-PARCEL-70-F", "SYN-PARCEL-70-G"} {
			at := pendingAnsweredAt.Add(time.Duration(2-index) * time.Hour)
			if err := register.RecordMissing(ctx, pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", object),
				pendingBasis(t, "basis/v1"), []string{"DELIVERY_PLACE"}, at); err != nil {
				return err
			}
		}
		return register.RecordMissing(ctx, pendingKey(t, "SYN-TENANT-02", "SYN-SEG-70", "SYN-PARCEL-70-E"),
			pendingBasis(t, "basis/v1"), []string{"DELIVERY_PLACE"}, pendingAnsweredAt)
	})
	tenant := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-E").TenantID
	listed, err := register.ListPending(t.Context(), tenant, time.Time{}, 10)
	if err != nil {
		t.Fatal(err)
	}
	var objects []string
	for _, record := range listed {
		objects = append(objects, record.Key.Object.String())
	}
	if !slices.Equal(objects, []string{"SYN-PARCEL-70-G", "SYN-PARCEL-70-F", "SYN-PARCEL-70-E"}) {
		t.Fatalf("列队 = %v, want 本租户三件、最近一次答复最早的在前", objects)
	}
	if due, err := register.ListPending(t.Context(), tenant, pendingAnsweredAt.Add(time.Hour), 10); err != nil || len(due) != 2 {
		t.Fatalf("截止 +1h 列队 = %v err=%v, want 两件", due, err)
	}
	if limited, err := register.ListPending(t.Context(), tenant, time.Time{}, 1); err != nil || len(limited) != 1 ||
		limited[0].Key.Object.String() != "SYN-PARCEL-70-G" {
		t.Fatalf("limit 1 列队 = %v err=%v", limited, err)
	}
}

// TestThePendingRegisterWritesRefuseToRunOutsideATransaction 证三个写口无环境事务即拒：册上那一行要与执行器那一拍同笔，事务外
// 写进去就可能留下一笔没人答过的待形成。断言指名 ErrTransactionRequired，不把「因为别的原因也失败了」混进来。
func TestThePendingRegisterWritesRefuseToRunOutsideATransaction(t *testing.T) {
	register, _, _ := newDeliveryDispatchPendings(t)
	key := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-H")
	task, err := domain.NewDispatchTaskReference("DELIVERY-DISPATCH/task-h")
	if err != nil {
		t.Fatal(err)
	}
	for name, write := range map[string]func() error{
		"RecordMissing": func() error {
			return register.RecordMissing(t.Context(), key, pendingBasis(t, "basis/v1"), []string{"DELIVERY_PLACE"}, pendingAnsweredAt)
		},
		"SettleFormed": func() error { return register.SettleFormed(t.Context(), key, task, pendingAnsweredAt) },
		"SettleLapsed": func() error {
			return register.SettleLapsed(t.Context(), key, "PARTICIPATION_NOT_ACTIVE", pendingAnsweredAt)
		},
	} {
		if err := write(); !errors.Is(err, bentopg.ErrTransactionRequired) {
			t.Errorf("%s 无事务应返回 ErrTransactionRequired，实得：%v", name, err)
		}
	}
}

// 库内 CHECK：缺件不在封闭三格里、缺件为空、状态与了结三件不成组，都写不进去。
func TestThePendingTableRefusesRowsTheRegisterCannotGive(t *testing.T) {
	_, transactor, db := newDeliveryDispatchPendings(t)
	for name, statement := range map[string]string{
		"缺件不在封闭集": `INSERT INTO transport_fulfillment.delivery_dispatch_pending
			(tenant_id, segment_ref, object_ref, entry_basis, missing, first_answered_at, last_answered_at, state)
			VALUES ('t', 's', 'o', 'b', ARRAY['DELIVERY_ADDRESS'], now(), now(), 'PENDING')`,
		"缺件为空": `INSERT INTO transport_fulfillment.delivery_dispatch_pending
			(tenant_id, segment_ref, object_ref, entry_basis, missing, first_answered_at, last_answered_at, state)
			VALUES ('t', 's', 'o', 'b', ARRAY[]::text[], now(), now(), 'PENDING')`,
		"已形成没有任务": `INSERT INTO transport_fulfillment.delivery_dispatch_pending
			(tenant_id, segment_ref, object_ref, entry_basis, missing, first_answered_at, last_answered_at, state, settled_at)
			VALUES ('t', 's', 'o', 'b', ARRAY['DELIVERY_PLACE'], now(), now(), 'FORMED', now())`,
		"作罢格不在封闭集": `INSERT INTO transport_fulfillment.delivery_dispatch_pending
			(tenant_id, segment_ref, object_ref, entry_basis, missing, first_answered_at, last_answered_at, state, lapsed_by, settled_at)
			VALUES ('t', 's', 'o', 'b', ARRAY['DELIVERY_PLACE'], now(), now(), 'LAPSED', 'SEGMENT_NOT_FOUND', now())`,
	} {
		t.Run(name, func(t *testing.T) {
			err := transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
				executor, err := db.RequireExecutor(ctx)
				if err != nil {
					return err
				}
				_, err = executor.Exec(ctx, statement)
				return err
			})
			if err == nil {
				t.Fatal("CHECK 没挡住")
			}
		})
	}
}

// Covers: AT-TF-284（ADR-0230 决定三）——查阅页那一格读的就是待形成队列：只列本租户待形成的，别的租户同段同对象那一条不出现，
// 了结的不出现。
func TestTheReviewCatalogueListsOnlyThisTenantsPendingDeliveries(t *testing.T) {
	register, transactor, db := newDeliveryDispatchPendings(t)
	catalogue, err := adapter.NewReviewCatalogue(db)
	if err != nil {
		t.Fatal(err)
	}
	mine := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-J")
	settled := pendingKey(t, "SYN-TENANT-01", "SYN-SEG-70", "SYN-PARCEL-70-K")
	theirs := pendingKey(t, "SYN-TENANT-02", "SYN-SEG-70", "SYN-PARCEL-70-J")
	task, err := domain.NewDispatchTaskReference("DELIVERY-DISPATCH/task-k")
	if err != nil {
		t.Fatal(err)
	}
	withinPendingTransaction(t, transactor, func(ctx context.Context) error {
		for _, key := range []ports.DeliveryDispatchPendingKey{mine, settled, theirs} {
			if err := register.RecordMissing(ctx, key, pendingBasis(t, "basis/v1"), []string{"DELIVERY_CONDITION"}, pendingAnsweredAt); err != nil {
				return err
			}
		}
		return register.SettleFormed(ctx, settled, task, pendingAnsweredAt.Add(time.Hour))
	})
	rows, err := catalogue.ListDeliveryDispatchPendings(t.Context(), mine.TenantID, 25)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 1 || rows[0].Key != mine {
		t.Fatalf("查阅页列出 %+v, want 只有本租户待形成的 %v", rows, mine)
	}
}
