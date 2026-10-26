package postgres_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/migrations"
)

// TestTheMigrationBackfillsEarlierEpisodesAsRecordedBeforeCauses 证 `AT-VE-203` 的迁移半边：执行迁移 0029 原文（不是手写回填后的
// 字面量）——先把库退回 0029 之前的表形、照那时的形状写两行存量发作期（一行活跃、一行已结束），再跑那份迁移；存量行回填为
// 「登记于因果条件之前」、依据空数组，命中、结束与规则原样，经适配器照旧读得回、不认领带原因的命中；迁移之后的新发作期照常写得进。
func TestTheMigrationBackfillsEarlierEpisodesAsRecordedBeforeCauses(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	assets, err := migrations.VisibilityException()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	var migration string
	for _, asset := range assets {
		if strings.HasPrefix(asset.Name, "0029_") {
			migration = asset.SQL
		}
	}
	if !strings.Contains(migration, "RECORDED_BEFORE_CAUSAL_CONDITION") {
		t.Fatal("没有迁移 0029，或它不再回填存量那一格")
	}

	for _, statement := range []string{
		`DROP INDEX visibility_exception.signal_episode_latest_for_cause`,
		`ALTER TABLE visibility_exception.signal_episode
		    DROP CONSTRAINT signal_episode_fact_basis_shape,
		    DROP CONSTRAINT signal_episode_causal_condition_not_blank,
		    DROP COLUMN causal_condition,
		    DROP COLUMN fact_basis`,
		`INSERT INTO visibility_exception.signal_episode
		     (tenant_id, episode_id, parcel_ref, kind_ref, rule_ref, confidence_ref, hits, started_at, last_hit_at,
		      release_basis, ended_at)
		 VALUES ('tenant-a', 'ep-legacy-active', 'parcel-1', 'DEPARTURE_GATE_BLOCKED', 'signal-rule/v0', 'confidence/high', 3,
		         '2026-10-01T08:00:00Z', '2026-10-03T08:00:00Z', NULL, NULL),
		        ('tenant-a', 'ep-legacy-ended', 'parcel-2', 'FACT_CONFLICT', 'signal-rule/v0', 'confidence/high', 1,
		         '2026-10-01T08:00:00Z', '2026-10-01T08:00:00Z', 'recovered', '2026-10-02T08:00:00Z')`,
		migration,
	} {
		if _, err := fixture.pool.Exec(ctx, statement); err != nil {
			t.Fatalf("执行 %q：%v", firstLine(statement), err)
		}
	}

	tenant := factValue(t, domain.NewTenantID, "tenant-a")
	for _, legacy := range []struct {
		episode, parcel, kind string
		hits                  int
		ended                 bool
	}{
		{"ep-legacy-active", "parcel-1", "DEPARTURE_GATE_BLOCKED", 3, false},
		{"ep-legacy-ended", "parcel-2", "FACT_CONFLICT", 1, true},
	} {
		parcel := factValue(t, domain.NewTrackedParcelReference, legacy.parcel)
		kind := factValue(t, domain.NewExceptionSignalKindReference, legacy.kind)
		found, exists, err := fixture.episodes.FindLatest(ctx, tenant, parcel, kind)
		if err != nil || !exists {
			t.Fatalf("%s 回填后读不回：err=%v exists=%v", legacy.episode, err, exists)
		}
		snapshot := found.Snapshot()
		if snapshot.ID.String() != legacy.episode || snapshot.Cause.String() != domain.CausalConditionRecordedBeforeCauses ||
			len(snapshot.Basis) != 0 || snapshot.Hits != legacy.hits || snapshot.Rule.String() != "signal-rule/v0" ||
			found.Active() == legacy.ended {
			t.Fatalf("%s 回填走样：%+v", legacy.episode, snapshot)
		}
		hit := episodeHit(t, legacy.parcel, legacy.kind, "fact/"+legacy.parcel, time.Date(2026, 10, 27, 8, 0, 0, 0, time.UTC))
		if _, exists, err := fixture.episodes.FindLatestForCause(ctx, tenant, parcel, kind, hit.Cause); err != nil || exists {
			t.Fatalf("%s 被带原因的命中认领了：err=%v exists=%v", legacy.episode, err, exists)
		}
	}

	fresh := openedEpisodeFor(t, "ep-after", "parcel-1", "DEPARTURE_GATE_BLOCKED", "movement-fact/parcel-1/MF-9", factBaseAt)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, fresh, "tenant-a", "parcel-1", "DEPARTURE_GATE_BLOCKED", domain.ManualReviewRequired))
	})
	if _, exists, err := fixture.episodes.FindLatestForCause(ctx, tenant,
		factValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		factValue(t, domain.NewExceptionSignalKindReference, "DEPARTURE_GATE_BLOCKED"), fresh.Cause()); err != nil || !exists {
		t.Fatalf("迁移之后的新发作期写不进或读不回：err=%v exists=%v", err, exists)
	}
}

func firstLine(statement string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(statement), "\n")
	return line
}
