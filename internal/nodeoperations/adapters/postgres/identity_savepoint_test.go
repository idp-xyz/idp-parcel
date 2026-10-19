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

// 本文件对真实 PostgreSQL 证身份核对读口的 savepoint 隔离（ADR-0199 决定五）：内层在事务里报 SQL 级错误时，收寄那一笔事务
// 照样能写下未核对的待识别、能提交。做法同服务结果读口那一份（service_result_savepoint_test）：替身读口与生产的 PS 读口一样
// 经 ReadExecutor 在调用方的事务里发 SQL。断言一律在事务闭包外。

// sqlIdentityView 在调用方的事务里发一条 SQL；query 出错时把错误原样交回，像一张被挪走的 PS 外部标识关联册那样。
type sqlIdentityView struct {
	db    *bentopg.DB
	query string
	asked int
}

func (view *sqlIdentityView) ResolveParcelIdentity(
	ctx context.Context,
	_ domain.TenantID,
	_ ports.ExternalMarkObservation,
) (ports.ParcelIdentityAnswer, error) {
	view.asked++
	querier, err := view.db.ReadExecutor(ctx)
	if err != nil {
		return ports.ParcelIdentityAnswer{}, err
	}
	var parcel string
	if err := querier.QueryRow(ctx, view.query).Scan(&parcel); err != nil {
		return ports.ParcelIdentityAnswer{}, err
	}
	reference, err := domain.NewParcelAssociationReference(parcel)
	if err != nil {
		return ports.ParcelIdentityAnswer{}, err
	}
	return ports.ParcelIdentityAnswer{Candidates: []domain.ParcelAssociationReference{reference}}, nil
}

func identitySavepointFixture(t *testing.T, query string) (*bentopg.DB, *adapter.Receptions, *sqlIdentityView, *adapter.SavepointedParcelIdentityView) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	receptions, err := adapter.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	inner := &sqlIdentityView{db: db, query: query}
	view, err := adapter.NewSavepointedParcelIdentityView(db, inner)
	if err != nil {
		t.Fatalf("构造装饰器：%v", err)
	}
	return db, receptions, inner, view
}

// Covers: ADR-0199 决定五「身份读口在翻转那一笔改经 savepoint 隔离……SQL 级读错误不得把收寄那笔事务打成中止态」——内层在事务
// 里执行 SELECT 1/0，装饰器把错误交回；同一事务随后写下的未核对待识别照样提交、读得回。没有这层隔离，事务被打成中止态，提交
// 答 rollback，读不动就连带丢了物理事实，翻转在库里不成立。
func TestASavepointKeepsAFailedIdentityReadFromAbortingTheReception(t *testing.T) {
	db, receptions, _, view := identitySavepointFixture(t, `SELECT (1/0)::text`)
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-NRS05")
	record := uncheckedPending(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-SAVEPOINT-1", "SYN-INTAKEV-NRS05-SAVEPOINT-1")

	var readErr error
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		_, readErr = view.ResolveParcelIdentity(txCtx, tenant, ports.ExternalMarkObservation{Mark: "SYN-MARK-NRS05"})
		_, saveErr := receptions.Save(txCtx, record)
		return saveErr
	})
	if readErr == nil {
		t.Fatal("内层 SELECT 1/0 却没有交回错误")
	}
	if err != nil {
		t.Fatalf("身份读口失败连带了收寄的写：%v", err)
	}
	found, exists, err := receptions.FindByKey(t.Context(), record.Key)
	if err != nil || !exists || found.IdentityCheck != ports.IdentityUnchecked {
		t.Fatalf("提交后读回：exists=%v err=%v 核对态 %q", exists, err, found.IdentityCheck)
	}
}

// Covers: 装饰器的可读一支——RELEASE 之后答复原样交回，同一事务随后的写入照常提交。
func TestASavepointReleasesAReadableIdentityUnchanged(t *testing.T) {
	db, receptions, _, view := identitySavepointFixture(t, `SELECT 'SYN-PARCEL-NRS05'`)
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-NRS05")
	record := formedRecord(t, "SYN-TENANT-NRS05", "SYN-SCAN-NRS05-SAVEPOINT-2")

	var answer ports.ParcelIdentityAnswer
	var readErr error
	err := db.Transactor().WithinTransaction(t.Context(), func(txCtx context.Context) error {
		answer, readErr = view.ResolveParcelIdentity(txCtx, tenant, ports.ExternalMarkObservation{Mark: "SYN-MARK-NRS05"})
		_, saveErr := receptions.Save(txCtx, record)
		return saveErr
	})
	if parcel, unique := answer.Unique(); readErr != nil || !unique || parcel.String() != "SYN-PARCEL-NRS05" {
		t.Fatalf("读回 %+v err=%v, want 唯一的 SYN-PARCEL-NRS05", answer, readErr)
	}
	if err != nil {
		t.Fatalf("提交：%v", err)
	}
	if _, exists, err := receptions.FindByKey(t.Context(), record.Key); err != nil || !exists {
		t.Fatalf("提交后读回：exists=%v err=%v", exists, err)
	}
}

// Covers: 装饰器的无事务一支——ctx 不在事务里时直通内层，不发 SAVEPOINT（事务块外发它本身就报错），答复与错误都原样交回。
func TestAnIdentitySavepointPassesThroughOutsideATransaction(t *testing.T) {
	tenant := ref(t, domain.NewTenantID, "SYN-TENANT-NRS05")
	mark := ports.ExternalMarkObservation{Mark: "SYN-MARK-NRS05"}

	db, _, inner, view := identitySavepointFixture(t, `SELECT 'SYN-PARCEL-NRS05'`)
	if _, err := db.RequireExecutor(t.Context()); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("前提不成立：ctx 应在事务外，RequireExecutor 却答 %v", err)
	}
	answer, err := view.ResolveParcelIdentity(t.Context(), tenant, mark)
	if _, unique := answer.Unique(); err != nil || !unique || inner.asked != 1 {
		t.Fatalf("读回 %+v err=%v 问内层 %d 次, want 直通一次", answer, err, inner.asked)
	}

	failing := errors.New("parcel shipment unreachable")
	wrapped, err := adapter.NewSavepointedParcelIdentityView(db, failingIdentity{err: failing})
	if err != nil {
		t.Fatalf("构造装饰器：%v", err)
	}
	if _, err := wrapped.ResolveParcelIdentity(t.Context(), tenant, mark); !errors.Is(err, failing) {
		t.Fatalf("err = %v, want 内层错误原样交回", err)
	}
}

type failingIdentity struct{ err error }

func (identity failingIdentity) ResolveParcelIdentity(context.Context, domain.TenantID, ports.ExternalMarkObservation) (ports.ParcelIdentityAnswer, error) {
	return ports.ParcelIdentityAnswer{}, identity.err
}
