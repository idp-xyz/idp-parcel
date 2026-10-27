package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

const etaVersionsMigration = "0032_eta_prediction_versions"

// legacyETAConnection 从空库按序施加本上下文的迁移，停在预测版本表那一份之前，交回连接与那一份迁移。
func legacyETAConnection(t *testing.T) (*pgx.Conn, migrations.Asset) {
	t.Helper()
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.WithoutCancel(ctx)) })
	if _, err := conn.Exec(ctx, `CREATE SCHEMA visibility_exception`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.VisibilityException()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	for _, asset := range assets {
		if asset.Name == etaVersionsMigration {
			return conn, asset
		}
		if _, err := conn.Exec(ctx, asset.SQL); err != nil {
			t.Fatalf("施加 %s：%v", asset.Name, err)
		}
	}
	t.Fatalf("没有找到迁移 %s", etaVersionsMigration)
	return nil, migrations.Asset{}
}

// Covers: AT-VE-222 的存量半边——ETA 预测版本表迁移——迁移之前落下的预测行各自成为版本表里一条线的一版并立为当前，来源、输入、模型、区间、
// 可信度、预测时点原样；带前身指回的行把指回原样带过来，不抹成首版；同名包裹在不同租户各是各的线；旧表随之退场，
// 当前标记只能指它所在那条线名下存下来的版本。
func TestTheETAVersionMigrationCarriesExistingPredictionsAsTheirCurrentVersions(t *testing.T) {
	ctx := t.Context()
	conn, migration := legacyETAConnection(t)

	if _, err := conn.Exec(ctx, `INSERT INTO visibility_exception.eta_prediction
		(tenant_id, parcel_ref, milestone_ref, version_id, source, inputs_ref, model_ref,
		 range_from, range_to, confidence_ref, predicted_at, prior_version)
		VALUES ('SYN-TENANT-E32', 'SYN-PARCEL-A', 'DELIVERED', 'SYN-ETA-OP-2', 'OPERATOR_DERIVED', 'SYN-INPUTS-2', 'SYN-MODEL',
		        '2026-10-02T00:00:00Z', '2026-10-03T00:00:00Z', 'MEDIUM/history', '2026-10-01T08:00:00Z', 'SYN-ETA-OP-1-OVERWRITTEN'),
		       ('SYN-TENANT-E32', 'SYN-PARCEL-B', 'DELIVERED', 'SYN-ETA-CARRIER-1', 'CARRIER_PROVIDED', 'SYN-INPUTS-C', 'SYN-CARRIER-MODEL',
		        '2026-10-04T00:00:00Z', '2026-10-05T00:00:00Z', 'HIGH/carrier-confirmed', '2026-10-01T09:00:00Z', NULL),
		       ('SYN-TENANT-E32-OTHER', 'SYN-PARCEL-A', 'DELIVERED', 'SYN-ETA-OTHER-1', 'OPERATOR_DERIVED', 'SYN-INPUTS-O', 'SYN-MODEL',
		        '2026-10-06T00:00:00Z', '2026-10-07T00:00:00Z', 'LOW/sparse', '2026-10-01T10:00:00Z', NULL)`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	if _, err := conn.Exec(ctx, migration.SQL); err != nil {
		t.Fatalf("施加 %s：%v", etaVersionsMigration, err)
	}

	for _, want := range []struct {
		tenant, parcel, source, version, inputs, model, confidence string
		from, to, predictedAt                                      string
		prior                                                      *string
	}{
		{"SYN-TENANT-E32", "SYN-PARCEL-A", "OPERATOR_DERIVED", "SYN-ETA-OP-2", "SYN-INPUTS-2", "SYN-MODEL", "MEDIUM/history",
			"2026-10-02T00:00:00Z", "2026-10-03T00:00:00Z", "2026-10-01T08:00:00Z", stringRef("SYN-ETA-OP-1-OVERWRITTEN")},
		{"SYN-TENANT-E32", "SYN-PARCEL-B", "CARRIER_PROVIDED", "SYN-ETA-CARRIER-1", "SYN-INPUTS-C", "SYN-CARRIER-MODEL", "HIGH/carrier-confirmed",
			"2026-10-04T00:00:00Z", "2026-10-05T00:00:00Z", "2026-10-01T09:00:00Z", nil},
		{"SYN-TENANT-E32-OTHER", "SYN-PARCEL-A", "OPERATOR_DERIVED", "SYN-ETA-OTHER-1", "SYN-INPUTS-O", "SYN-MODEL", "LOW/sparse",
			"2026-10-06T00:00:00Z", "2026-10-07T00:00:00Z", "2026-10-01T10:00:00Z", nil},
	} {
		var version, inputs, model, confidence, from, to, predictedAt string
		var prior *string
		if err := conn.QueryRow(ctx,
			`SELECT v.version_id, v.inputs_ref, v.model_ref, v.confidence_ref,
			        to_char(v.range_from AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			        to_char(v.range_to AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'),
			        to_char(v.predicted_at AT TIME ZONE 'UTC', 'YYYY-MM-DD"T"HH24:MI:SS"Z"'), v.prior_version
			   FROM visibility_exception.eta_prediction_current AS c
			   JOIN visibility_exception.eta_prediction_version AS v
			     ON v.tenant_id = c.tenant_id AND v.parcel_ref = c.parcel_ref AND v.milestone_ref = c.milestone_ref
			    AND v.source = c.source AND v.version_id = c.version_id
			  WHERE c.tenant_id = $1 AND c.parcel_ref = $2 AND c.milestone_ref = 'DELIVERED' AND c.source = $3`,
			want.tenant, want.parcel, want.source,
		).Scan(&version, &inputs, &model, &confidence, &from, &to, &predictedAt, &prior); err != nil {
			t.Fatalf("%s/%s：读回迁入的当前版：%v", want.tenant, want.parcel, err)
		}
		if version != want.version || inputs != want.inputs || model != want.model || confidence != want.confidence ||
			from != want.from || to != want.to || predictedAt != want.predictedAt || !sameText(prior, want.prior) {
			t.Errorf("%s/%s：迁入变形：%s %s %s %s %s %s %s %v，want %+v", want.tenant, want.parcel, version, inputs, model,
				confidence, from, to, predictedAt, prior, want)
		}
	}

	// 标记只能指它所在那条线名下存下来的版本：别的来源线的版本被外键拒绝。
	_, err := conn.Exec(ctx, `INSERT INTO visibility_exception.eta_prediction_current
		(tenant_id, parcel_ref, milestone_ref, source, version_id)
		VALUES ('SYN-TENANT-E32', 'SYN-PARCEL-A', 'DELIVERED', 'CARRIER_PROVIDED', 'SYN-ETA-OP-2')`)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "23503" || pgErr.ConstraintName != "eta_prediction_current_names_stored_version" {
		t.Errorf("跨来源线的标记应被版本外键拒绝，实得：%v", err)
	}

	var legacy *string
	if err := conn.QueryRow(ctx, `SELECT to_regclass('visibility_exception.eta_prediction')::text`).Scan(&legacy); err != nil {
		t.Fatalf("查旧表：%v", err)
	}
	if legacy != nil {
		t.Errorf("旧表 %s 仍在：当前版只能由一处指名", *legacy)
	}
}

// Covers: AT-VE-222 的存量半边，存量数据迁移的预检——旧表对版本号没有租户内唯一的约束；同租户下两行共用一个版本号时迁移停下，给出指明租户与版本号的
// 可读报错，而不是丢出主键冲突的原始报错；停下时什么都没动（旧表原样、新表未建）。
func TestTheETAVersionMigrationStopsWithAReadableErrorWhenVersionIDsCollide(t *testing.T) {
	ctx := t.Context()
	conn, migration := legacyETAConnection(t)

	if _, err := conn.Exec(ctx, `INSERT INTO visibility_exception.eta_prediction
		(tenant_id, parcel_ref, milestone_ref, version_id, source, inputs_ref, model_ref,
		 range_from, range_to, confidence_ref, predicted_at)
		VALUES ('SYN-TENANT-E32', 'SYN-PARCEL-A', 'DELIVERED', 'SYN-ETA-DUP', 'OPERATOR_DERIVED', 'i', 'm',
		        '2026-10-02T00:00:00Z', '2026-10-03T00:00:00Z', 'c', '2026-10-01T08:00:00Z'),
		       ('SYN-TENANT-E32', 'SYN-PARCEL-B', 'DELIVERED', 'SYN-ETA-DUP', 'CARRIER_PROVIDED', 'i', 'm',
		        '2026-10-02T00:00:00Z', '2026-10-03T00:00:00Z', 'c', '2026-10-01T08:00:00Z')`); err != nil {
		t.Fatalf("落旧形状的行：%v", err)
	}
	_, err := conn.Exec(ctx, migration.SQL)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) {
		t.Fatalf("重复的版本号应让迁移停下，实得：%v", err)
	}
	for _, mention := range []string{"SYN-TENANT-E32", "SYN-ETA-DUP", "版本号"} {
		if !strings.Contains(pgErr.Message, mention) {
			t.Errorf("报错 %q 里没有指明 %q", pgErr.Message, mention)
		}
	}
	if pgErr.Code == "23505" {
		t.Errorf("报错是主键冲突的原始报错，不是预检给的可读说明：%v", pgErr)
	}

	var kept int
	var created *string
	if err := conn.QueryRow(ctx,
		`SELECT (SELECT count(*) FROM visibility_exception.eta_prediction),
		        to_regclass('visibility_exception.eta_prediction_version')::text`).Scan(&kept, &created); err != nil {
		t.Fatalf("查迁移停下后的状态：%v", err)
	}
	if kept != 2 || created != nil {
		t.Errorf("迁移停下后旧表 %d 行、新表 %v，want 旧表原样 2 行、新表未建", kept, created)
	}
}
