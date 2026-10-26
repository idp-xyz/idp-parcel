package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证释放未受理册（ADR-0282 决定二）：一笔整行往返、同一笔再登记答已有记录、另一版另起一行、
// 没存作用域的存量那一笔照样记得下、租户隔离、无事务拒；库内 CHECK 钉住三段作用域同进同出与原因必填。

var refusalRecordedAt = time.Date(2026, 10, 26, 9, 0, 0, 0, time.UTC)

func refusalOf(t *testing.T, tenant, version, result string, scope domain.OccupationScope) ports.ControlReleaseRefusal {
	t.Helper()
	return ports.ControlReleaseRefusal{
		TenantID:          psTenant(t, tenant),
		CustomerAccountID: mustBuild(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID: mustBuild(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustBuild(t, domain.NewSubmissionVersionID, version),
		ControlResultID:   mustBuild(t, domain.NewFinancialControlResultID, result),
		Scope:             scope,
		Reason:            mustBuild(t, domain.NewCheckReason, "SA-RELEASE_REQUEST_NOT_ACCEPTED"),
		RecordedAt:        refusalRecordedAt,
	}
}

func TestAControlReleaseRefusalIsRecordedOnceAndListedByShipmentRequest(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewControlReleaseRefusals(db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	ctx := t.Context()
	record := func(refusal ports.ControlReleaseRefusal) ports.ControlReleaseRefusalSaveOutcome {
		t.Helper()
		var outcome ports.ControlReleaseRefusalSaveOutcome
		mustWithinTransaction(t, db.Transactor(), ctx, func(txCtx context.Context) error {
			var err error
			outcome, err = register.Record(txCtx, refusal)
			return err
		})
		return outcome
	}
	scope, err := domain.NewOccupationScope("SYN-LEGAL-ENTITY-1", "SYN-ACCOUNT-1", "CNY")
	if err != nil {
		t.Fatalf("new occupation scope: %v", err)
	}

	first := refusalOf(t, "tenant-1", "version-2", "request-1/version-2", scope)
	if got := record(first); got != ports.ControlReleaseRefusalRecorded {
		t.Fatalf("首记 = %d, want 已记下", got)
	}
	if got := record(first); got != ports.ControlReleaseRefusalAlreadyRecorded {
		t.Fatalf("重放 = %d, want 已有记录——同一笔占用不是第二份", got)
	}
	legacy := refusalOf(t, "tenant-1", "version-1", "request-1/version-1", domain.OccupationScope{})
	if got := record(legacy); got != ports.ControlReleaseRefusalRecorded {
		t.Fatalf("没存作用域的存量那一笔 = %d, want 照样记下", got)
	}

	refusals, err := register.ListByShipmentRequest(ctx, psTenant(t, "tenant-1"), first.ShipmentRequestID)
	if err != nil {
		t.Fatalf("按委托列：%v", err)
	}
	if len(refusals) != 2 {
		t.Fatalf("行数 = %d, want 2", len(refusals))
	}
	if refusals[0] != legacy || refusals[1] != first {
		t.Fatalf("往返变形：%+v, want %+v 与 %+v", refusals, legacy, first)
	}
	if foreign, err := register.ListByShipmentRequest(ctx, psTenant(t, "tenant-2"), first.ShipmentRequestID); err != nil ||
		len(foreign) != 0 {
		t.Fatalf("另一租户读到了 %d 行（err=%v）", len(foreign), err)
	}

	if _, err := register.Record(ctx, first); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Errorf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}

	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.control_release_refusal
			(tenant_id, shipment_request_id, submission_version, control_result_id, customer_account_id,
			 occupation_legal_entity, occupation_settlement_account, occupation_currency, reason_ref, recorded_at)
		 VALUES ('tenant-1', 'request-x', 'version-1', 'request-x/version-1', 'customer-1',
		         'SYN-LEGAL-ENTITY-1', NULL, 'CNY', 'SA-RELEASE_REQUEST_NOT_ACCEPTED', now())`); err == nil {
		t.Fatal("一行只存了一半作用域的进了册——缺一段的作用域认领不到任何一笔")
	}
	if _, err := pool.Exec(ctx,
		`INSERT INTO parcel_shipment.control_release_refusal
			(tenant_id, shipment_request_id, submission_version, control_result_id, customer_account_id,
			 reason_ref, recorded_at)
		 VALUES ('tenant-1', 'request-x', 'version-1', 'request-x/version-1', 'customer-1', ' ', now())`); err == nil {
		t.Fatal("一行没有原因的进了册——交到人手里却说不出为什么")
	}
}

// failRefusalWrites 在释放未受理册上挂一个 BEFORE INSERT 触发器让每一次登记报 SQL 错误，像一张写不进的表那样。
func failRefusalWrites(t *testing.T, ctx context.Context, exec func(context.Context, string, ...any) error) {
	t.Helper()
	if err := exec(ctx, `
CREATE FUNCTION parcel_shipment.refusal_write_fails() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN RAISE EXCEPTION 'control release refusal register is not writable'; END $$;
CREATE TRIGGER refusal_write_fails BEFORE INSERT ON parcel_shipment.control_release_refusal
FOR EACH ROW EXECUTE FUNCTION parcel_shipment.refusal_write_fails();`); err != nil {
		t.Fatalf("挂写入失败触发器：%v", err)
	}
}

// Covers: ADR-0282 决定二「记不下册（写不进）时那一笔改留补偿续办、业务事实照常成立」——登记那一句在库里报 SQL 错误时
// 交回错误，并退回自己的 savepoint：调用方那一笔事务随后照样能写、能提交，不会被打进中止态、整笔回滚。
func TestARefusalThatCannotBeWrittenLeavesTheCallerTransactionUsable(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewControlReleaseRefusals(db)
	if err != nil {
		t.Fatalf("构造释放未受理册：%v", err)
	}
	ctx := t.Context()
	failRefusalWrites(t, ctx, func(ctx context.Context, sql string, args ...any) error {
		_, err := pool.Exec(ctx, sql, args...)
		return err
	})
	scope, err := domain.NewOccupationScope("SYN-LEGAL-ENTITY-1", "SYN-ACCOUNT-1", "CNY")
	if err != nil {
		t.Fatalf("new occupation scope: %v", err)
	}

	var recordErr error
	err = db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, recordErr = register.Record(txCtx, refusalOf(t, "tenant-1", "version-1", "request-1/version-1", scope))
		return writeAfterRelease(txCtx, db)
	})
	if recordErr == nil {
		t.Fatal("触发器拒了这一行，登记却答成功")
	}
	if err != nil {
		t.Fatalf("登记写不进连带了调用方事务随后的写：%v", err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM parcel_shipment.control_release_refusal`).Scan(&rows); err != nil {
		t.Fatalf("数册：%v", err)
	}
	if rows != 0 {
		t.Fatalf("册上 %d 行, want 0", rows)
	}
}
