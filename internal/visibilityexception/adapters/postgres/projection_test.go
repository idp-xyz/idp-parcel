package postgres_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	adapter "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
)

var projectionBaseAt = time.Date(2026, 8, 14, 16, 0, 0, 0, time.UTC)

type projectionFixture struct {
	projections *adapter.Projections
	transactor  bentoapp.Transactor
	pool        *pgxpool.Pool
}

func newProjectionFixture(t *testing.T) *projectionFixture {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	projections, err := adapter.NewProjections(db)
	if err != nil {
		t.Fatalf("构造投影库：%v", err)
	}
	return &projectionFixture{projections: projections, transactor: db.Transactor(), pool: pool}
}

func (fixture *projectionFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func projectionValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

func classifiedEntry(t *testing.T, factRef, milestone string) domain.MilestoneClassification {
	t.Helper()
	fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      domain.SourceNodeOperations,
		Parcel:      projectionValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		Fact:        projectionValue(t, domain.NewSourceFactReference, factRef),
		Kind:        projectionValue(t, domain.NewSourceFactKind, "node-intake"),
		Version:     projectionValue(t, domain.NewSourceFactVersion, "v1"),
		OccurredAt:  projectionBaseAt,
		EffectiveAt: projectionBaseAt.Add(time.Hour),
		ReceivedAt:  projectionBaseAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造事实：%v", err)
	}
	mapping := projectionValue(t, domain.NewMappingVersionReference, "milestone-map/v1")
	if milestone == "" {
		classification, err := domain.LeaveUnclassified(fact, mapping)
		if err != nil {
			t.Fatalf("未归类：%v", err)
		}
		return classification
	}
	classification, err := domain.ClassifyMilestone(fact,
		projectionValue(t, domain.NewMilestoneReference, milestone), mapping)
	if err != nil {
		t.Fatalf("归类：%v", err)
	}
	return classification
}

func derivedProjection(t *testing.T, version string, entries []domain.MilestoneClassification) domain.TrackingProjection {
	t.Helper()
	projection, err := domain.DeriveTrackingProjection(
		projectionValue(t, domain.NewProjectionVersionID, version),
		projectionValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		entries,
		projectionBaseAt.Add(3*time.Hour),
	)
	if err != nil {
		t.Fatalf("派生投影：%v", err)
	}
	return projection
}

func TestProjectionRoundTripsAndRederiveReplacesTheRow(t *testing.T) {
	fixture := newProjectionFixture(t)
	ctx := t.Context()
	tenant := projectionValue(t, domain.NewTenantID, "tenant-a")

	first := derivedProjection(t, "projection-1", []domain.MilestoneClassification{
		classifiedEntry(t, "scan/origin", "PICKED_UP"),
		classifiedEntry(t, "scan/unknown", ""),
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenant, first)
	})

	loaded, found, err := fixture.projections.FindCurrent(ctx, tenant, first.Parcel())
	if err != nil || !found || loaded.Version().String() != "projection-1" {
		t.Fatalf("首版往返：err=%v found=%v version=%s", err, found, loaded.Version())
	}
	if _, prior := loaded.PriorVersion(); prior {
		t.Fatal("首版带了指回")
	}
	if milestone, ok := loaded.Entries()[0].Milestone(); !ok || milestone.String() != "PICKED_UP" {
		t.Fatal("已归类条目丢了")
	}
	if _, ok := loaded.Entries()[1].Milestone(); ok {
		t.Fatal("未归类条目被强行映射了")
	}

	rederived, err := loaded.Rederive(
		projectionValue(t, domain.NewProjectionVersionID, "projection-2"),
		[]domain.MilestoneClassification{classifiedEntry(t, "scan/origin", "PICKED_UP")},
		projectionBaseAt.Add(4*time.Hour),
	)
	if err != nil {
		t.Fatalf("重派生：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, tenant, rederived)
	})

	current, found, err := fixture.projections.FindCurrent(ctx, tenant, first.Parcel())
	if err != nil || !found || current.Version().String() != "projection-2" {
		t.Fatalf("当前版 = %s found=%v err=%v", current.Version(), found, err)
	}
	prior, present := current.PriorVersion()
	if !present || prior.String() != "projection-1" {
		t.Fatalf("指回 = %s present=%v", prior, present)
	}

	var rows int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM visibility_exception.tracking_projection
		  WHERE tenant_id = 'tenant-a'`).Scan(&rows); err != nil {
		t.Fatalf("数行：%v", err)
	}
	if rows != 1 {
		t.Fatalf("投影行数 = %d，库应只管当前版", rows)
	}
}

func TestProjectionsOfAnotherTenantAreInvisible(t *testing.T) {
	fixture := newProjectionFixture(t)
	ctx := t.Context()
	projection := derivedProjection(t, "projection-1", []domain.MilestoneClassification{
		classifiedEntry(t, "scan/origin", "PICKED_UP"),
	})
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, projectionValue(t, domain.NewTenantID, "tenant-a"), projection)
	})
	if _, found, err := fixture.projections.FindCurrent(ctx,
		projectionValue(t, domain.NewTenantID, "tenant-b"), projection.Parcel()); err != nil || found {
		t.Fatalf("跨租户可见：err=%v found=%v", err, found)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.projections.Save(txCtx, projectionValue(t, domain.NewTenantID, "tenant-b"), projection)
	})
}

func TestProjectionWritesRequireTransaction(t *testing.T) {
	fixture := newProjectionFixture(t)
	projection := derivedProjection(t, "projection-1", []domain.MilestoneClassification{
		classifiedEntry(t, "scan/origin", "PICKED_UP"),
	})
	if err := fixture.projections.Save(t.Context(),
		projectionValue(t, domain.NewTenantID, "tenant-a"), projection); err == nil {
		t.Fatal("无事务 Save 被接受了")
	}
}

func TestProjectionChecksRejectEmptyEntries(t *testing.T) {
	fixture := newProjectionFixture(t)
	if _, err := fixture.pool.Exec(t.Context(),
		`INSERT INTO visibility_exception.tracking_projection
			(tenant_id, parcel_ref, version_id, derived_at, entries)
		 VALUES ('t', 'p', 'v', now(), '[]')`); err == nil {
		t.Fatal("空条目的投影被库接受了")
	}
}

func TestProjectionRebuildRejectsMissingKind(t *testing.T) {
	fixture := newProjectionFixture(t)
	ctx := t.Context()
	if _, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.tracking_projection
			(tenant_id, parcel_ref, version_id, derived_at, entries)
		 VALUES ('tenant-a', 'parcel-1', 'projection-1', $1, $2)`,
		projectionBaseAt.Add(3*time.Hour),
		`[{"source":"NODE_OPERATIONS","parcel":"parcel-1","fact":"scan/origin","version":"v1",`+
			`"occurredAt":"2026-08-14T16:00:00Z","effectiveAt":"2026-08-14T17:00:00Z",`+
			`"receivedAt":"2026-08-14T18:00:00Z","mapping":"milestone-map/v1","milestone":"PICKED_UP"}]`,
	); err != nil {
		t.Fatalf("写入缺类型条目：%v", err)
	}
	parcel := projectionValue(t, domain.NewTrackedParcelReference, "parcel-1")
	_, _, err := fixture.projections.FindCurrent(ctx,
		projectionValue(t, domain.NewTenantID, "tenant-a"), parcel)
	if err == nil {
		t.Fatal("缺事实类型的投影条目被默契补上了")
	}
}
