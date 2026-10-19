package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 证服务结果读口的 savepoint 隔离（ADR-0186）：内层在事务里报 SQL 级错误时，收寄那一笔事务
// 照样能写、能提交。替身读口与生产的 PS 读口一样经 ReadExecutor 在调用方的事务里发 SQL。断言一律在事务闭包外。

// sqlServiceResultView 在调用方的事务里发一条 SQL；query 出错时把错误原样交回，像一张被挪走的 PS 表那样。
type sqlServiceResultView struct {
	db    *bentopg.DB
	query string
	asked int
}

func (view *sqlServiceResultView) FindParcelServiceResults(
	ctx context.Context,
	_ domain.TenantID,
	_ domain.ParcelAssociationReference,
) (ports.ParcelServiceResults, error) {
	view.asked++
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ParcelServiceResults{}, err
	}
	var cancellation string
	if err := querier.QueryRow(ctx, view.query).Scan(&cancellation); err != nil {
		return ports.ParcelServiceResults{}, err
	}
	return ports.ParcelServiceResults{Cancellation: cancellation}, nil
}

func savepointFixture(t *testing.T, query string) (*bentopg.DB, *adapter.Receptions, *sqlServiceResultView, *adapter.SavepointedServiceResultView) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	inner := &sqlServiceResultView{db: db, query: query}
	view, err := adapter.NewSavepointedServiceResultView(db, inner)
	if err != nil {
		t.Fatalf("构造装饰器：%v", err)
	}
	return db, receptions, inner, view
}

// Covers: ADR-0186「服务结果读口经 savepoint 隔离」——内层在事务里执行 SELECT 1/0，装饰器把错误交回；同一事务随后的
// 收寄写入照样提交、读得回。没有这层隔离，事务被打成中止态，提交答 rollback，读不动就连带丢了物理事实。
func TestASavepointKeepsAFailedServiceResultReadFromAbortingTheReception(t *testing.T) {
	db, receptions, _, view := savepointFixture(t, `SELECT (1/0)::text`)
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-01")
	parcel := ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-1")
	record := formedRecord(t, "SYN-TENANT-01", "SYN-SCAN-SAVEPOINT-1")
	record.ServiceMarkers = nil
	record.ServiceResultCheck = ports.ServiceResultsUnreadable

	var readErr error
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, readErr = view.FindParcelServiceResults(txCtx, tenant, parcel)
		_, saveErr := receptions.Save(txCtx, record)
		return saveErr
	})
	if readErr == nil {
		t.Fatal("内层 SELECT 1/0 却没有交回错误")
	}
	if err != nil {
		t.Fatalf("读口失败连带了收寄的写：%v", err)
	}
	if _, found, err := receptions.FindByKey(t.Context(), record.Key); err != nil || !found {
		t.Fatalf("提交后读回：found=%v err=%v", found, err)
	}
}

// Covers: ADR-0186 装饰器的可读一支——RELEASE 之后读数照常交回，同一事务随后的写入照常提交。
func TestASavepointReleasesAReadableServiceResultUnchanged(t *testing.T) {
	db, receptions, _, view := savepointFixture(t, `SELECT 'SYN-CANCEL-1'`)
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-01")
	parcel := ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-1")
	record := formedRecord(t, "SYN-TENANT-01", "SYN-SCAN-SAVEPOINT-2")

	var results ports.ParcelServiceResults
	var readErr error
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		results, readErr = view.FindParcelServiceResults(txCtx, tenant, parcel)
		_, saveErr := receptions.Save(txCtx, record)
		return saveErr
	})
	if readErr != nil || results.Cancellation != "SYN-CANCEL-1" {
		t.Fatalf("读回 %+v err=%v, want SYN-CANCEL-1", results, readErr)
	}
	if err != nil {
		t.Fatalf("提交：%v", err)
	}
	if _, found, err := receptions.FindByKey(t.Context(), record.Key); err != nil || !found {
		t.Fatalf("提交后读回：found=%v err=%v", found, err)
	}
}

// Covers: ADR-0186 装饰器的无事务一支——ctx 不在事务里（RequireExecutor 答 ErrTransactionRequired）时直通内层，不发
// SAVEPOINT（事务块外发它本身就报错），读数与错误都原样交回。读口不是写口：无事务时不拒，这一点与各写口的
// *RefuseToRunOutsideATransaction 用例相反。
func TestASavepointPassesThroughOutsideATransaction(t *testing.T) {
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-01")
	parcel := ref(t, domain.NewParcelAssociationReference, "SYN-PARCEL-1")

	db, _, inner, view := savepointFixture(t, `SELECT 'SYN-CANCEL-1'`)
	if _, err := db.RequireExecutor(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("前提不成立：ctx 应在事务外，RequireExecutor 却答 %v", err)
	}
	results, err := view.FindParcelServiceResults(t.Context(), tenant, parcel)
	if err != nil || results.Cancellation != "SYN-CANCEL-1" || inner.asked != 1 {
		t.Fatalf("读回 %+v err=%v 问内层 %d 次, want 直通一次", results, err, inner.asked)
	}

	failing := errors.New("parcel shipment unreachable")
	wrapped, err := adapter.NewSavepointedServiceResultView(db, failingResults{err: failing})
	if err != nil {
		t.Fatalf("构造装饰器：%v", err)
	}
	if _, err := wrapped.FindParcelServiceResults(t.Context(), tenant, parcel); !errors.Is(err, failing) {
		t.Fatalf("err = %v, want 内层错误原样交回", err)
	}
}

type failingResults struct{ err error }

func (results failingResults) FindParcelServiceResults(context.Context, domain.TenantID, domain.ParcelAssociationReference) (ports.ParcelServiceResults, error) {
	return ports.ParcelServiceResults{}, results.err
}
