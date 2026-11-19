package postgres_test

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

// 本文件对真实 PostgreSQL 证测量与作废共用的来源身份由库自己守：不经应用、不取咨询锁，直接 SQL 也让一个（租户，来源身份）
// 只属于一类。夹具全为合成（S 级）。

const (
	sourceIdentityTenant = "SYN-TENANT-01"

	claimSourceIdentitySQL = `INSERT INTO node_operations.measurement_source_identity (tenant_id, source_id, kind) VALUES ($1, $2, $3)`

	rawMeasurementSQL = `INSERT INTO node_operations.actual_measurement
		(tenant_id, source_id, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
		 weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
		 performed_by, evidence, occurred_at, recorded_at)
		VALUES ($1, $2, 'syn-digest', 'HANDLING_UNIT', 'SYN-HU-1', 'SYN-NODE-1', 'SYN-BENCH-1', 'SYN-BASIS-1',
		        '2.10', 'KG', '20', '15', '10', 'CM', 'SYN-SCALE-1', 'WEIGH/1', now(), now())`

	rawInvalidationSQL = `INSERT INTO node_operations.measurement_invalidation
		(tenant_id, source_id, content_digest, target_source_id, reason, performed_by, evidence, occurred_at, recorded_at)
		VALUES ($1, $2, 'syn-digest', $3, '称错件', 'SYN-SCALE-1', 'WEIGH/2', now(), now())`
)

// sqlExecutor 是连接池与事务共有的最小写口，让一句 SQL 既能单句自提交也能放进事务里。
type sqlExecutor interface {
	Exec(ctx context.Context, sql string, arguments ...any) (pgconn.CommandTag, error)
}

// insertFact 往对应种类的事实表直接插一行；作废的 target 是它指回的那条测量。
func insertFact(ctx context.Context, executor sqlExecutor, kind, sourceID, target string) error {
	if kind == "MEASUREMENT" {
		_, err := executor.Exec(ctx, rawMeasurementSQL, sourceIdentityTenant, sourceID)
		return err
	}
	_, err := executor.Exec(ctx, rawInvalidationSQL, sourceIdentityTenant, sourceID, target)
	return err
}

// insertClaimed 是守规矩的直接写入：先在登记表占住来源身份，再写事实行，同一事务。
func insertClaimed(ctx context.Context, pool *pgxpool.Pool, kind, sourceID, target string) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := tx.Exec(ctx, claimSourceIdentitySQL, sourceIdentityTenant, sourceID, kind); err != nil {
		return err
	}
	if err := insertFact(ctx, tx, kind, sourceID, target); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func mustInsertClaimed(t *testing.T, pool *pgxpool.Pool, kind, sourceID, target string) {
	t.Helper()
	if err := insertClaimed(t.Context(), pool, kind, sourceID, target); err != nil {
		t.Fatalf("直接写 %s %s：%v", kind, sourceID, err)
	}
}

func wantPgError(t *testing.T, what string, err error, code, constraint string) {
	t.Helper()
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("%s：库没有拒（err = %v），want %s %s", what, err, code, constraint)
	}
	if pgErr.Code != code || pgErr.ConstraintName != constraint {
		t.Fatalf("%s：被拒了但不是想要的那道约束：%s %s（%s），want %s %s", what, pgErr.Code, pgErr.ConstraintName, pgErr.Message, code, constraint)
	}
}

func factConstraint(kind string) string {
	if kind == "MEASUREMENT" {
		return "actual_measurement_source_identity_fk"
	}
	return "measurement_invalidation_source_identity_fk"
}

// holders 数一个来源身份在两张事实表里各有几行：合在一起不得多于一行。
func holders(t *testing.T, pool *pgxpool.Pool, sourceID string) (measurements, invalidations, claims int) {
	t.Helper()
	if err := pool.QueryRow(t.Context(),
		`SELECT (SELECT count(*) FROM node_operations.actual_measurement WHERE tenant_id = $1 AND source_id = $2),
		        (SELECT count(*) FROM node_operations.measurement_invalidation WHERE tenant_id = $1 AND source_id = $2),
		        (SELECT count(*) FROM node_operations.measurement_source_identity WHERE tenant_id = $1 AND source_id = $2)`,
		sourceIdentityTenant, sourceID,
	).Scan(&measurements, &invalidations, &claims); err != nil {
		t.Fatalf("数来源身份 %s：%v", sourceID, err)
	}
	return measurements, invalidations, claims
}

// Covers: 同一（租户，来源身份）既是测量又是作废，库拒第二行，先写哪一类都一样：占登记表撞主键；绕开登记表直接写事实行，撞
// 指向登记表的外键。第一类留在册上，来源身份不被抢走。
func TestTheDatabaseRefusesASourceIdentityThatIsBothAMeasurementAndAnInvalidation(t *testing.T) {
	const (
		shared = "SYN-DEVICE-01/SHARED"
		target = "SYN-DEVICE-01/TARGET"
	)
	for name, order := range map[string]struct{ first, second string }{
		"先测量后作废": {first: "MEASUREMENT", second: "INVALIDATION"},
		"先作废后测量": {first: "INVALIDATION", second: "MEASUREMENT"},
	} {
		t.Run(name, func(t *testing.T) {
			pool := pgtest.Pool(t)
			ctx := t.Context()
			mustInsertClaimed(t, pool, "MEASUREMENT", target, "")
			mustInsertClaimed(t, pool, order.first, shared, target)

			wantPgError(t, "第二类占登记表", insertClaimed(ctx, pool, order.second, shared, target),
				"23505", "measurement_source_identity_pkey")
			wantPgError(t, "第二类绕开登记表直接写事实行", insertFact(ctx, pool, order.second, shared, target),
				"23503", factConstraint(order.second))

			measurements, invalidations, claims := holders(t, pool, shared)
			wantMeasurements, wantInvalidations := 1, 0
			if order.first == "INVALIDATION" {
				wantMeasurements, wantInvalidations = 0, 1
			}
			if measurements != wantMeasurements || invalidations != wantInvalidations || claims != 1 {
				t.Fatalf("来源身份在册 = 测量 %d 作废 %d 登记 %d，want 测量 %d 作废 %d 登记 1",
					measurements, invalidations, claims, wantMeasurements, wantInvalidations)
			}
		})
	}
}

// Covers: 事实行没有先在登记表占位就写不进去，哪一类都一样；种类列恒为自己那一种，写成另一类的种类被检查约束拒。
func TestTheDatabaseRefusesAFactRowThatDidNotClaimItsSourceIdentity(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	mustInsertClaimed(t, pool, "MEASUREMENT", "SYN-DEVICE-02/TARGET", "")

	wantPgError(t, "没占位的测量", insertFact(ctx, pool, "MEASUREMENT", "SYN-DEVICE-02/BARE-M", ""),
		"23503", "actual_measurement_source_identity_fk")
	wantPgError(t, "没占位的作废", insertFact(ctx, pool, "INVALIDATION", "SYN-DEVICE-02/BARE-V", "SYN-DEVICE-02/TARGET"),
		"23503", "measurement_invalidation_source_identity_fk")

	if _, err := pool.Exec(ctx, claimSourceIdentitySQL, sourceIdentityTenant, "SYN-DEVICE-02/LIE-M", "INVALIDATION"); err != nil {
		t.Fatalf("占位：%v", err)
	}
	_, err := pool.Exec(ctx, `INSERT INTO node_operations.actual_measurement
		(tenant_id, source_id, source_kind, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
		 weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
		 performed_by, evidence, occurred_at, recorded_at)
		VALUES ($1, 'SYN-DEVICE-02/LIE-M', 'INVALIDATION', 'syn-digest', 'HANDLING_UNIT', 'SYN-HU-1', 'SYN-NODE-1',
		        'SYN-BENCH-1', 'SYN-BASIS-1', '2.10', 'KG', '20', '15', '10', 'CM', 'SYN-SCALE-1', 'WEIGH/1', now(), now())`,
		sourceIdentityTenant)
	wantPgError(t, "测量行写成作废的种类", err, "23514", "actual_measurement_source_kind_is_measurement")

	if _, err := pool.Exec(ctx, claimSourceIdentitySQL, sourceIdentityTenant, "SYN-DEVICE-02/LIE-V", "MEASUREMENT"); err != nil {
		t.Fatalf("占位：%v", err)
	}
	_, err = pool.Exec(ctx, `INSERT INTO node_operations.measurement_invalidation
		(tenant_id, source_id, source_kind, content_digest, target_source_id, reason, performed_by, evidence, occurred_at, recorded_at)
		VALUES ($1, 'SYN-DEVICE-02/LIE-V', 'MEASUREMENT', 'syn-digest', 'SYN-DEVICE-02/TARGET', '称错件', 'SYN-SCALE-1',
		        'WEIGH/2', now(), now())`, sourceIdentityTenant)
	wantPgError(t, "作废行写成测量的种类", err, "23514", "measurement_invalidation_source_kind_is_invalidation")

	if _, err := pool.Exec(ctx, claimSourceIdentitySQL, sourceIdentityTenant, "SYN-DEVICE-02/KIND", "ELSE"); err == nil {
		t.Fatal("登记表收下了封闭集之外的种类")
	}
}

// Covers: 登记表按租户分隔：别的租户可以用同一个来源身份，不被撞。
func TestTheSourceIdentityRegisterSeparatesTenants(t *testing.T) {
	pool := pgtest.Pool(t)
	mustInsertClaimed(t, pool, "MEASUREMENT", "SYN-DEVICE-03/SAME", "")
	if _, err := pool.Exec(t.Context(), claimSourceIdentitySQL, "SYN-TENANT-02", "SYN-DEVICE-03/SAME", "INVALIDATION"); err != nil {
		t.Fatalf("别的租户占同一个来源身份被拒：%v", err)
	}
}

// Covers: 两个事务不取咨询锁、各自直接写同一来源身份的不同种类，恰好一个提交成功：后到的那一方卡在登记表的主键上，等先到的
// 提交之后以唯一冲突失败。先到的事务未提交时后到的不得先完成。
func TestASourceIdentityClaimWaitsForTheOpenClaimAndThenLosesToIt(t *testing.T) {
	pool := pgtest.Pool(t)
	ctx := t.Context()
	const (
		shared = "SYN-DEVICE-04/SHARED"
		target = "SYN-DEVICE-04/TARGET"
	)
	mustInsertClaimed(t, pool, "MEASUREMENT", target, "")

	first, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = first.Rollback(context.WithoutCancel(ctx)) }()
	if _, err := first.Exec(ctx, claimSourceIdentitySQL, sourceIdentityTenant, shared, "MEASUREMENT"); err != nil {
		t.Fatalf("先到的占位：%v", err)
	}
	if err := insertFact(ctx, first, "MEASUREMENT", shared, ""); err != nil {
		t.Fatalf("先到的写事实行：%v", err)
	}

	second := make(chan error, 1)
	go func() { second <- insertClaimed(ctx, pool, "INVALIDATION", shared, target) }()
	select {
	case err := <-second:
		t.Fatalf("先到的未提交，后到的就完成了：%v", err)
	case <-time.After(300 * time.Millisecond):
	}

	if err := first.Commit(ctx); err != nil {
		t.Fatalf("先到的提交：%v", err)
	}
	wantPgError(t, "后到的占位", <-second, "23505", "measurement_source_identity_pkey")
	if measurements, invalidations, claims := holders(t, pool, shared); measurements != 1 || invalidations != 0 || claims != 1 {
		t.Fatalf("来源身份在册 = 测量 %d 作废 %d 登记 %d，want 测量 1 作废 0 登记 1", measurements, invalidations, claims)
	}
}

// Covers: 同上，不排先后：一条测量与一条作废同时各自直接写同一来源身份，每一轮恰好一个提交成功，另一个唯一冲突失败，册上
// 这个来源身份只在一张事实表里有行。每轮用新的来源身份，免得一次侥幸。
func TestTwoRawWritersOfDifferentKindsLeaveExactlyOneSourceIdentityHolder(t *testing.T) {
	pool := pgtest.Pool(t)
	const rounds = 12
	for round := 0; round < rounds; round++ {
		shared := fmt.Sprintf("SYN-DEVICE-05/SHARED-%02d", round)
		target := fmt.Sprintf("SYN-DEVICE-05/TARGET-%02d", round)
		mustInsertClaimed(t, pool, "MEASUREMENT", target, "")

		var (
			start    = make(chan struct{})
			group    sync.WaitGroup
			outcomes [2]error
		)
		for index, kind := range []string{"MEASUREMENT", "INVALIDATION"} {
			group.Add(1)
			go func() {
				defer group.Done()
				<-start
				outcomes[index] = insertClaimed(t.Context(), pool, kind, shared, target)
			}()
		}
		close(start)
		group.Wait()

		if (outcomes[0] == nil) == (outcomes[1] == nil) {
			t.Fatalf("第 %d 轮：测量写入 %v，作废写入 %v，want 恰好一个成功", round, outcomes[0], outcomes[1])
		}
		loser := outcomes[0]
		if loser == nil {
			loser = outcomes[1]
		}
		wantPgError(t, fmt.Sprintf("第 %d 轮输掉的一方", round), loser, "23505", "measurement_source_identity_pkey")
		if measurements, invalidations, claims := holders(t, pool, shared); measurements+invalidations != 1 || claims != 1 {
			t.Fatalf("第 %d 轮：来源身份在册 = 测量 %d 作废 %d 登记 %d，want 事实行合计 1、登记 1", round, measurements, invalidations, claims)
		}
	}
}

// Covers: 登记册的写口不替库约束圆场：来源身份已被另一类占着时，SaveMeasurement 与 SaveInvalidation 交回错误，而不是记成功或
// 当成重放；两边都没有写进多余的行。编排的先锁后读盖不住的那一步，到这里是错误，不是静默。
func TestTheRegisterWritersFailWhenTheOtherKindHoldsTheSourceIdentity(t *testing.T) {
	measurements, transactor := newActualMeasurements(t)
	ctx := t.Context()
	occurred := time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)
	target := savedWeight(t, "SYN-TARGET", "2.10")
	other := savedWeight(t, "SYN-OTHER", "2.20")
	void := savedInvalidation(t, "SYN-VOID", "SYN-TARGET", occurred)
	if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		for _, record := range []ports.ActualMeasurementRecord{target, other} {
			if _, err := measurements.SaveMeasurement(txCtx, record); err != nil {
				return err
			}
		}
		_, err := measurements.SaveInvalidation(txCtx, void)
		return err
	}); err != nil {
		t.Fatalf("备料：%v", err)
	}

	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := measurements.SaveMeasurement(txCtx, savedWeight(t, "SYN-VOID", "2.40"))
		if err == nil {
			return fmt.Errorf("测量借用作废的来源身份被当成 %s", outcome)
		}
		return err
	})
	wantPgError(t, "测量借用作废的来源身份", err, "23503", "actual_measurement_source_identity_fk")

	err = transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		outcome, err := measurements.SaveInvalidation(txCtx, savedInvalidation(t, "SYN-TARGET", "SYN-OTHER", occurred))
		if err == nil {
			return fmt.Errorf("作废借用测量的来源身份被当成 %s", outcome)
		}
		return err
	})
	wantPgError(t, "作废借用测量的来源身份", err, "23503", "measurement_invalidation_source_identity_fk")

	tenant := ref(t, domain.NewTenantID, "tenant-a")
	if _, found, err := measurements.FindMeasurement(ctx, ports.ActualMeasurementKey{TenantID: tenant, SourceID: "SYN-VOID"}); err != nil || found {
		t.Fatalf("借用来源身份的测量落了册：found %v err %v", found, err)
	}
	if _, found, err := measurements.FindInvalidation(ctx, ports.ActualMeasurementKey{TenantID: tenant, SourceID: "SYN-TARGET"}); err != nil || found {
		t.Fatalf("借用来源身份的作废落了册：found %v err %v", found, err)
	}
}

// Covers: 同类重放仍是幂等的：登记册上同一条测量、同一条作废各写第二遍答已登记，不报错，登记表上仍是一行。
func TestTheRegisterWritersStayIdempotentForTheSameKind(t *testing.T) {
	measurements, transactor := newActualMeasurements(t)
	ctx := t.Context()
	occurred := time.Date(2026, 11, 17, 4, 0, 0, 0, time.UTC)
	for round := 1; round <= 2; round++ {
		if err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			measured, err := measurements.SaveMeasurement(txCtx, savedWeight(t, "SYN-TARGET", "2.10"))
			if err != nil {
				return err
			}
			voided, err := measurements.SaveInvalidation(txCtx, savedInvalidation(t, "SYN-VOID", "SYN-TARGET", occurred))
			if err != nil {
				return err
			}
			wantMeasured, wantVoided := ports.ActualMeasurementSaved, ports.MeasurementInvalidationSaved
			if round == 2 {
				wantMeasured, wantVoided = ports.ActualMeasurementAlreadyRecorded, ports.MeasurementInvalidationAlreadyRecorded
			}
			if measured != wantMeasured || voided != wantVoided {
				return fmt.Errorf("第 %d 遍：测量 %s，作废 %s，want %s、%s", round, measured, voided, wantMeasured, wantVoided)
			}
			return nil
		}); err != nil {
			t.Fatalf("重放：%v", err)
		}
	}
}

// 迁移回填的夹具：从空库按序施加本上下文的迁移，停在目标那一份之前，由用例落旧形状的行，再施加目标。
func nodeOperationsConnectionBefore(t *testing.T, target string) (*pgx.Conn, migrations.Asset) {
	t.Helper()
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.WithoutCancel(ctx)) })
	if _, err := conn.Exec(ctx, `CREATE SCHEMA node_operations`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.NodeOperations()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	for _, asset := range assets {
		if asset.Name == target {
			return conn, asset
		}
		if _, err := conn.Exec(ctx, asset.SQL); err != nil {
			t.Fatalf("施加 %s：%v", asset.Name, err)
		}
	}
	t.Fatalf("没有找到迁移 %s", target)
	return nil, migrations.Asset{}
}

const (
	registerMigration = "0031_measurement_source_identity_register"

	// 旧形状：事实表上还没有种类列，也没有登记表。
	oldShapeMeasurement = `INSERT INTO node_operations.actual_measurement
		(tenant_id, source_id, content_digest, object_kind, object_id, node_ref, location_ref, basis_ref,
		 weight_value, weight_unit, length_value, width_value, height_value, dimension_unit,
		 performed_by, evidence, occurred_at, recorded_at)
		VALUES ($1, $2, 'syn-digest', 'HANDLING_UNIT', 'SYN-HU-1', 'SYN-NODE-1', 'SYN-BENCH-1', 'SYN-BASIS-1',
		        '2.10', 'KG', '20', '15', '10', 'CM', 'SYN-SCALE-1', 'WEIGH/1', now(), now())`
	oldShapeInvalidation = `INSERT INTO node_operations.measurement_invalidation
		(tenant_id, source_id, content_digest, target_source_id, reason, performed_by, evidence, occurred_at, recorded_at)
		VALUES ($1, $2, 'syn-digest', $3, '称错件', 'SYN-SCALE-1', 'WEIGH/2', now(), now())`
)

// Covers: 迁移把已有的测量与作废回填进登记表（各带自己的种类），事实行的种类列落成自己那一种；回填之后登记表立刻守得住：
// 已在册的作废身份再被占成测量，库拒。回填只来自已有的事实行，不种任何租户的行。
func TestTheSourceIdentityRegisterMigrationBackfillsTheExistingFacts(t *testing.T) {
	ctx := t.Context()
	conn, migration := nodeOperationsConnectionBefore(t, registerMigration)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{oldShapeMeasurement, []any{"SYN-TENANT-01", "SYN-DEVICE-06/M1"}},
		{oldShapeMeasurement, []any{"SYN-TENANT-01", "SYN-DEVICE-06/M2"}},
		{oldShapeMeasurement, []any{"SYN-TENANT-02", "SYN-DEVICE-06/M1"}},
		{oldShapeInvalidation, []any{"SYN-TENANT-01", "SYN-DEVICE-06/V1", "SYN-DEVICE-06/M1"}},
	} {
		if _, err := conn.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("落旧形状的行：%v", err)
		}
	}
	if _, err := conn.Exec(ctx, migration.SQL); err != nil {
		t.Fatalf("施加 %s：%v", registerMigration, err)
	}

	got := map[string]string{}
	rows, err := conn.Query(ctx, `SELECT tenant_id, source_id, kind FROM node_operations.measurement_source_identity`)
	if err != nil {
		t.Fatalf("读登记表：%v", err)
	}
	for rows.Next() {
		var tenant, source, kind string
		if err := rows.Scan(&tenant, &source, &kind); err != nil {
			t.Fatalf("扫描：%v", err)
		}
		got[tenant+"/"+source] = kind
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读登记表：%v", err)
	}
	want := map[string]string{
		"SYN-TENANT-01/SYN-DEVICE-06/M1": "MEASUREMENT",
		"SYN-TENANT-01/SYN-DEVICE-06/M2": "MEASUREMENT",
		"SYN-TENANT-02/SYN-DEVICE-06/M1": "MEASUREMENT",
		"SYN-TENANT-01/SYN-DEVICE-06/V1": "INVALIDATION",
	}
	if len(got) != len(want) {
		t.Fatalf("登记表 = %v，want %v", got, want)
	}
	for key, kind := range want {
		if got[key] != kind {
			t.Fatalf("登记表 = %v，want %v", got, want)
		}
	}

	var measurementKinds, invalidationKinds int
	if err := conn.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM node_operations.actual_measurement WHERE source_kind = 'MEASUREMENT'),
		        (SELECT count(*) FROM node_operations.measurement_invalidation WHERE source_kind = 'INVALIDATION')`,
	).Scan(&measurementKinds, &invalidationKinds); err != nil {
		t.Fatalf("读种类列：%v", err)
	}
	if measurementKinds != 3 || invalidationKinds != 1 {
		t.Fatalf("事实行的种类 = 测量 %d 作废 %d，want 3、1", measurementKinds, invalidationKinds)
	}

	if _, err := conn.Exec(ctx, claimSourceIdentitySQL, "SYN-TENANT-01", "SYN-DEVICE-06/V1", "MEASUREMENT"); err == nil {
		t.Fatal("回填之后，已在册的作废身份还能被占成测量")
	}
}

// Covers: 已有数据里同一（租户，来源身份）既是测量又是作废（迁移之前只有编排守这条规则）：回填撞主键，整条迁移回滚，不替人挑一类；
// 登记表不留半截。
func TestTheSourceIdentityRegisterMigrationRefusesAnExistingCollision(t *testing.T) {
	ctx := t.Context()
	conn, migration := nodeOperationsConnectionBefore(t, registerMigration)
	for _, statement := range []struct {
		sql  string
		args []any
	}{
		{oldShapeMeasurement, []any{"SYN-TENANT-01", "SYN-DEVICE-07/TARGET"}},
		{oldShapeMeasurement, []any{"SYN-TENANT-01", "SYN-DEVICE-07/BOTH"}},
		{oldShapeInvalidation, []any{"SYN-TENANT-01", "SYN-DEVICE-07/BOTH", "SYN-DEVICE-07/TARGET"}},
	} {
		if _, err := conn.Exec(ctx, statement.sql, statement.args...); err != nil {
			t.Fatalf("落旧形状的行：%v", err)
		}
	}
	_, err := conn.Exec(ctx, migration.SQL)
	wantPgError(t, "回填撞上两类共用的来源身份", err, "23505", "measurement_source_identity_pkey")

	var present bool
	if err := conn.QueryRow(ctx, `SELECT to_regclass('node_operations.measurement_source_identity') IS NOT NULL`).Scan(&present); err != nil {
		t.Fatalf("查登记表：%v", err)
	}
	if present {
		t.Fatal("迁移失败之后登记表还在")
	}
}
