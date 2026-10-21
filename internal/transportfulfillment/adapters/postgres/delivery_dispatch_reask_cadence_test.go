package postgres_test

import (
	"context"
	"errors"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件对真实 PostgreSQL 16 证派送重问间隔册（迁移 0046，ADR-0230 决定五）：按版本首登、撞键答 false、读回过构造门、此刻生效的是
// 最近登记的那一版、按租户隔离、无事务拒。夹具全部合成（S 级）。

func newReaskCadences(t *testing.T) (*adapter.DeliveryDispatchReaskCadences, *bentopg.DB) {
	t.Helper()
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	register, err := adapter.NewDeliveryDispatchReaskCadences(db)
	if err != nil {
		t.Fatalf("构造重问间隔册：%v", err)
	}
	return register, db
}

func reaskCadenceRecord(t *testing.T, tenant, version string, interval time.Duration, recordedAt time.Time) ports.DeliveryDispatchReaskCadenceRecord {
	t.Helper()
	tenantID, err := domain.NewTenantID(tenant)
	if err != nil {
		t.Fatal(err)
	}
	versionRef, err := domain.NewDeliveryDispatchReaskCadenceVersion(version)
	if err != nil {
		t.Fatal(err)
	}
	cadence, err := domain.NewDeliveryDispatchReaskCadence(tenantID, versionRef, interval, "SYN-BASIS/"+version)
	if err != nil {
		t.Fatal(err)
	}
	return ports.DeliveryDispatchReaskCadenceRecord{Cadence: cadence, RecordedAt: recordedAt}
}

// Covers: AT-TF-287（ADR-0230 决定五）——一版一行：首登 true、同版本再登 false；此刻生效的是最近登记的那一版；没登过的租户答 false
// （未配置），别的租户登的不算。
func TestReaskCadencesAreVersionedAndTheLatestIsCurrent(t *testing.T) {
	register, db := newReaskCadences(t)
	at := time.Date(2026, 10, 21, 8, 0, 0, 0, time.UTC)
	save := func(record ports.DeliveryDispatchReaskCadenceRecord) bool {
		t.Helper()
		var saved bool
		if err := db.Transactor().WithinTransaction(t.Context(), func(ctx context.Context) error {
			var err error
			saved, err = register.Save(ctx, record)
			return err
		}); err != nil {
			t.Fatalf("登记：%v", err)
		}
		return saved
	}
	if !save(reaskCadenceRecord(t, "SYN-TENANT-01", "SYN-REASK/v1", 6*time.Hour, at)) {
		t.Fatal("首登答 false")
	}
	if save(reaskCadenceRecord(t, "SYN-TENANT-01", "SYN-REASK/v1", time.Hour, at.Add(time.Minute))) {
		t.Fatal("同版本再登答 true")
	}
	save(reaskCadenceRecord(t, "SYN-TENANT-01", "SYN-REASK/v2", 30*time.Minute, at.Add(time.Hour)))
	save(reaskCadenceRecord(t, "SYN-TENANT-02", "SYN-REASK/v9", 5*time.Minute, at.Add(2*time.Hour)))

	tenant := reaskCadenceRecord(t, "SYN-TENANT-01", "x", time.Minute, at).Cadence.TenantID()
	current, found, err := register.CurrentCadence(t.Context(), tenant)
	if err != nil || !found || current.Cadence.Version().String() != "SYN-REASK/v2" || current.Cadence.Interval() != 30*time.Minute {
		t.Fatalf("此刻生效 = %+v found=%v err=%v, want 最近登记的 v2", current, found, err)
	}
	held, found, err := register.FindByVersion(t.Context(), tenant, current.Cadence.Version())
	if err != nil || !found || !held.RecordedAt.Equal(at.Add(time.Hour)) || held.Cadence.Basis() != "SYN-BASIS/SYN-REASK/v2" {
		t.Fatalf("按版本读回 = %+v found=%v err=%v", held, found, err)
	}
	unregistered := reaskCadenceRecord(t, "SYN-TENANT-03", "x", time.Minute, at).Cadence.TenantID()
	if _, found, err := register.CurrentCadence(t.Context(), unregistered); err != nil || found {
		t.Fatalf("没登过的租户 found=%v err=%v, want false——那是未配置", found, err)
	}
}

// TestTheReaskCadenceRegisterRefusesToWriteOutsideATransaction 证首登无环境事务即拒：登记写面的事务边界归装配点。
func TestTheReaskCadenceRegisterRefusesToWriteOutsideATransaction(t *testing.T) {
	register, _ := newReaskCadences(t)
	_, err := register.Save(t.Context(), reaskCadenceRecord(t, "SYN-TENANT-01", "SYN-REASK/v1", time.Hour, time.Now()))
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("无事务登记应返回 ErrTransactionRequired，实得：%v", err)
	}
}
