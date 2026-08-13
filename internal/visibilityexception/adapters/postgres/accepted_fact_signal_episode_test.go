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
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件对真实 PostgreSQL 16 证事实登记册与发作期库的行为：三时间分存原样读回、
// 源上下文封闭五值由迁移 CHECK 把关、发作期与分诊结论同一事务越过提交边界、
// FindLatest 的并列裁决。断言一律在事务闭包外（Goexit 会挂死连接）。

var factBaseAt = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)

type factFixture struct {
	facts      *adapter.AcceptedFacts
	episodes   *adapter.SignalEpisodes
	transactor bentoapp.Transactor
	pool       *pgxpool.Pool
}

func newFactFixture(t *testing.T) *factFixture {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	facts, err := adapter.NewAcceptedFacts(db)
	if err != nil {
		t.Fatalf("构造事实登记册：%v", err)
	}
	episodes, err := adapter.NewSignalEpisodes(db)
	if err != nil {
		t.Fatalf("构造发作期库：%v", err)
	}
	return &factFixture{facts: facts, episodes: episodes, transactor: db.Transactor(), pool: pool}
}

func (fixture *factFixture) inTx(t *testing.T, ctx context.Context, fn func(context.Context) error) {
	t.Helper()
	if err := fixture.transactor.WithinTransaction(ctx, fn); err != nil {
		t.Fatalf("事务内写入失败：%v", err)
	}
}

func factValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// factRecord 造一份三时间刻意错开的事实：发生、生效与接收各差一小时，往返断言据此
// 证「三时间分存」不是三列存同一个值。
func factRecord(t *testing.T, source domain.SourceContext, parcel, factRef, version string) ports.FactRecord {
	t.Helper()
	fact, err := domain.NewAcceptedSourceFact(domain.AcceptedSourceFactSpec{
		Source:      source,
		Parcel:      factValue(t, domain.NewTrackedParcelReference, parcel),
		Fact:        factValue(t, domain.NewSourceFactReference, factRef),
		Version:     factValue(t, domain.NewSourceFactVersion, version),
		OccurredAt:  factBaseAt,
		EffectiveAt: factBaseAt.Add(time.Hour),
		ReceivedAt:  factBaseAt.Add(2 * time.Hour),
	})
	if err != nil {
		t.Fatalf("构造事实：%v", err)
	}
	return ports.FactRecord{
		Key: ports.FactKey{
			Source:  source,
			Fact:    fact.Fact(),
			Version: fact.Version(),
		},
		ContentDigest: "digest-" + factRef + "-" + version,
		Fact:          fact,
	}
}

// TestFactsAreReadBackWithThreeTimesApart 证事实往返：三时间分存原样读回，
// FindByParcel 交回全部且次序确定。
func TestFactsAreReadBackWithThreeTimesApart(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	shipment := factRecord(t, domain.SourceParcelShipment, "parcel-1", "fact-a", "v1")
	customs := factRecord(t, domain.SourceCustomsCompliance, "parcel-1", "fact-b", "v1")
	elsewhere := factRecord(t, domain.SourceNodeOperations, "parcel-2", "fact-c", "v1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		for _, record := range []ports.FactRecord{shipment, customs, elsewhere} {
			if _, err := fixture.facts.Save(txCtx, record); err != nil {
				return err
			}
		}
		return nil
	})

	found, exists, err := fixture.facts.FindByKey(ctx, shipment.Key)
	if err != nil || !exists {
		t.Fatalf("读回事实：%v exists=%v", err, exists)
	}
	if found.Fact.OccurredAt() != factBaseAt ||
		found.Fact.EffectiveAt() != factBaseAt.Add(time.Hour) ||
		found.Fact.ReceivedAt() != factBaseAt.Add(2*time.Hour) {
		t.Fatalf("三时间没分开读回：occurred=%v effective=%v received=%v",
			found.Fact.OccurredAt(), found.Fact.EffectiveAt(), found.Fact.ReceivedAt())
	}
	if found.ContentDigest != shipment.ContentDigest ||
		found.Fact.Source() != domain.SourceParcelShipment ||
		found.Fact.Parcel().String() != "parcel-1" {
		t.Fatalf("事实没原样读回：%+v", found)
	}

	all, err := fixture.facts.FindByParcel(ctx, factValue(t, domain.NewTrackedParcelReference, "parcel-1"))
	if err != nil {
		t.Fatalf("按包裹读回：%v", err)
	}
	if len(all) != 2 {
		t.Fatalf("按包裹读回 %d 份，想要 2（parcel-2 的事实不该混进来）", len(all))
	}
}

// TestSecondFactWriterGetsAlreadyRecorded 证写入代数：同键第二份答`已有记录`，原
// 内容不被顶替，事务保持可用（同事务读回作答）。
func TestSecondFactWriterGetsAlreadyRecorded(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	original := factRecord(t, domain.SourceNetworkRouting, "parcel-1", "fact-a", "v1")
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		_, err := fixture.facts.Save(txCtx, original)
		return err
	})

	// 同键异内容：来源冲突的判定在应用层拿指纹比，库只答`已有记录`不顶替。
	impostor := original
	impostor.ContentDigest = "digest-imposter"

	var outcome ports.FactSaveOutcome
	var foundInTx ports.FactRecord
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.facts.Save(txCtx, impostor)
		if err != nil {
			return err
		}
		outcome = saved
		found, _, err := fixture.facts.FindByKey(txCtx, original.Key)
		foundInTx = found
		return err
	})
	if outcome != ports.FactAlreadyRecorded {
		t.Fatalf("重写 outcome = %d, 想要 FactAlreadyRecorded", outcome)
	}
	if foundInTx.ContentDigest != original.ContentDigest {
		t.Fatalf("原内容被顶替：%q", foundInTx.ContentDigest)
	}

	// 新版本是新键：来源更正不撞幂等。
	var corrected ports.FactSaveOutcome
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		saved, err := fixture.facts.Save(txCtx,
			factRecord(t, domain.SourceNetworkRouting, "parcel-1", "fact-a", "v2"))
		corrected = saved
		return err
	})
	if corrected != ports.FactSaved {
		t.Fatalf("新版本 outcome = %d, 想要 FactSaved", corrected)
	}
}

// TestSourceContextOutsideClosedSetIsRejectedByCheck 证封闭五值在库内 CHECK 把关：
// 适配器在类型上就到不了这一步，绕过适配器的裸写同样进不来。
func TestSourceContextOutsideClosedSetIsRejectedByCheck(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	_, err := fixture.pool.Exec(ctx,
		`INSERT INTO visibility_exception.accepted_fact
			(source_context, fact_ref, fact_version, parcel_ref, content_digest,
			 occurred_at, effective_at, received_at)
		 VALUES ('RAW_SCAN', 'fact-x', 'v1', 'parcel-1', 'digest-x', now(), now(), now())`)
	if err == nil {
		t.Fatalf("集合外源上下文被库接受了")
	}
}

// TestFactWritesRequireTransactionAndRollBack 证事务纪律：无事务写一律拒；事务失败
// 后库里没有半份事实。
func TestFactWritesRequireTransactionAndRollBack(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	record := factRecord(t, domain.SourceTransportFulfillment, "parcel-1", "fact-a", "v1")
	if _, err := fixture.facts.Save(ctx, record); err == nil {
		t.Fatalf("无事务写入被接受了")
	}

	rollback := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if _, err := fixture.facts.Save(txCtx, record); err != nil {
			return err
		}
		return context.Canceled
	})
	if rollback == nil {
		t.Fatalf("事务该失败没失败")
	}
	if _, exists, err := fixture.facts.FindByKey(ctx, record.Key); err != nil || exists {
		t.Fatalf("回滚后仍有残留：err=%v exists=%v", err, exists)
	}
}

func raisedRecord(t *testing.T, episode *domain.SignalEpisode, parcel, kind string, outcome domain.TriageOutcome) ports.RaisedSignalRecord {
	t.Helper()
	conclusion, err := domain.ConcludeTriage(
		episode.ID(),
		outcome,
		factValue(t, domain.NewSignalRuleVersionReference, "triage-rule/v1"),
		factBaseAt.Add(5*time.Minute),
	)
	if err != nil {
		t.Fatalf("构造分诊结论：%v", err)
	}
	return ports.RaisedSignalRecord{
		Parcel:     factValue(t, domain.NewTrackedParcelReference, parcel),
		Kind:       factValue(t, domain.NewExceptionSignalKindReference, kind),
		Episode:    episode,
		Conclusion: conclusion,
	}
}

func openedEpisode(t *testing.T, id, parcel, kind string, firstHitAt time.Time) *domain.SignalEpisode {
	t.Helper()
	episode, err := domain.OpenEpisode(
		factValue(t, domain.NewEpisodeID, id),
		factValue(t, domain.NewExceptionSignalKindReference, kind),
		factValue(t, domain.NewTrackedParcelReference, parcel),
		factValue(t, domain.NewSignalRuleVersionReference, "signal-rule/v1"),
		factValue(t, domain.NewConfidenceReference, "confidence/high"),
		firstHitAt,
	)
	if err != nil {
		t.Fatalf("开启发作期：%v", err)
	}
	return episode
}

func (fixture *factFixture) conclusionCount(t *testing.T, ctx context.Context, episodeID string) int {
	t.Helper()
	var count int
	if err := fixture.pool.QueryRow(ctx,
		`SELECT count(*) FROM visibility_exception.triage_conclusion WHERE episode_id = $1`,
		episodeID,
	).Scan(&count); err != nil {
		t.Fatalf("数结论行：%v", err)
	}
	return count
}

// TestRaisedEpisodeRoundTripsAndAbsorbsHits 证发作期往返与命中推进：SaveRaised 落
// 发作期+结论，FindLatest 重建出的发作期能继续 RecordHit，SaveHit 后的判断历史
// 原样读回。
func TestRaisedEpisodeRoundTripsAndAbsorbsHits(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	opened := openedEpisode(t, "ep-1", "parcel-1", "STALLED", factBaseAt)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, opened, "parcel-1", "STALLED", domain.AutoEstablishCase))
	})
	if fixture.conclusionCount(t, ctx, "ep-1") != 1 {
		t.Fatalf("分诊结论没随发作期落库")
	}

	found, exists, err := fixture.episodes.FindLatest(ctx,
		factValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		factValue(t, domain.NewExceptionSignalKindReference, "STALLED"))
	if err != nil || !exists {
		t.Fatalf("读回发作期：%v exists=%v", err, exists)
	}
	if found.ID().String() != "ep-1" || found.Hits() != 1 || !found.Active() {
		t.Fatalf("发作期没原样读回：id=%s hits=%d active=%v", found.ID(), found.Hits(), found.Active())
	}

	// 重建出的发作期继续吸收命中——快照重建不重演，但生命周期方法照常工作。
	if err := found.RecordHit(factBaseAt.Add(10 * time.Minute)); err != nil {
		t.Fatalf("重建后记命中：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveHit(txCtx, found)
	})

	again, _, err := fixture.episodes.FindLatest(ctx,
		factValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		factValue(t, domain.NewExceptionSignalKindReference, "STALLED"))
	if err != nil {
		t.Fatalf("再读发作期：%v", err)
	}
	if again.Hits() != 2 {
		t.Fatalf("命中推进没落库：hits=%d", again.Hits())
	}
}

// TestRaisedPairCrossesCommitBoundaryTogether 证同笔提交：事务失败后发作期与结论
// 都不存在——只落一半，重试会走进命中支，结论永远补不上。
func TestRaisedPairCrossesCommitBoundaryTogether(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	opened := openedEpisode(t, "ep-half", "parcel-1", "STALLED", factBaseAt)
	rollback := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		if err := fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, opened, "parcel-1", "STALLED", domain.ManualReviewRequired)); err != nil {
			return err
		}
		return context.Canceled
	})
	if rollback == nil {
		t.Fatalf("事务该失败没失败")
	}

	if _, exists, err := fixture.episodes.FindLatest(ctx,
		factValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		factValue(t, domain.NewExceptionSignalKindReference, "STALLED")); err != nil || exists {
		t.Fatalf("回滚后发作期仍在：err=%v exists=%v", err, exists)
	}
	if fixture.conclusionCount(t, ctx, "ep-half") != 0 {
		t.Fatalf("回滚后结论仍在")
	}

	if err := fixture.episodes.SaveRaised(ctx,
		raisedRecord(t, opened, "parcel-1", "STALLED", domain.ManualReviewRequired)); err == nil {
		t.Fatalf("无事务 SaveRaised 被接受了")
	}
}

// TestReopenedEpisodeWinsFindLatest 证重开链与并列裁决：前期结束后重开的新发作期
// 指回前期；即便首命中与前期开启同刻（started_at 并列），FindLatest 也交回后到的
// 那一期——到达序（seq）裁决。
func TestReopenedEpisodeWinsFindLatest(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	first := openedEpisode(t, "ep-first", "parcel-1", "STALLED", factBaseAt)
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, first, "parcel-1", "STALLED", domain.NoCaseNeeded))
	})

	// 同刻结束再同刻重开：End 允许与末次命中同刻，重开允许与结束同刻——两期的
	// started_at 完全相等，是并列裁决的最刁钻形态。
	if err := first.End("recovered", factBaseAt); err != nil {
		t.Fatalf("结束前期：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveHit(txCtx, first)
	})

	reopened, err := first.ReopenAsLinked(factValue(t, domain.NewEpisodeID, "ep-second"), factBaseAt)
	if err != nil {
		t.Fatalf("重开发作期：%v", err)
	}
	fixture.inTx(t, ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveRaised(txCtx,
			raisedRecord(t, reopened, "parcel-1", "STALLED", domain.AttachToExistingCase))
	})

	latest, exists, err := fixture.episodes.FindLatest(ctx,
		factValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		factValue(t, domain.NewExceptionSignalKindReference, "STALLED"))
	if err != nil || !exists {
		t.Fatalf("读回最近发作期：%v exists=%v", err, exists)
	}
	if latest.ID().String() != "ep-second" {
		t.Fatalf("FindLatest 交回 %s，想要重开的 ep-second", latest.ID())
	}
	prior, has := latest.PriorEpisode()
	if !has || prior.String() != "ep-first" {
		t.Fatalf("重开没指回前期：prior=%s has=%v", prior, has)
	}

	// 另一对象与另一类型都不可见——键的两维各自隔离。
	if _, exists, err := fixture.episodes.FindLatest(ctx,
		factValue(t, domain.NewTrackedParcelReference, "parcel-2"),
		factValue(t, domain.NewExceptionSignalKindReference, "STALLED")); err != nil || exists {
		t.Fatalf("跨对象可见：err=%v exists=%v", err, exists)
	}
	if _, exists, err := fixture.episodes.FindLatest(ctx,
		factValue(t, domain.NewTrackedParcelReference, "parcel-1"),
		factValue(t, domain.NewExceptionSignalKindReference, "DELAYED")); err != nil || exists {
		t.Fatalf("跨类型可见：err=%v exists=%v", err, exists)
	}
}

// TestSaveHitOnMissingEpisodeFails 证命中只能落在已存在的发作期上。
func TestSaveHitOnMissingEpisodeFails(t *testing.T) {
	fixture := newFactFixture(t)
	ctx := t.Context()

	ghost := openedEpisode(t, "ep-ghost", "parcel-1", "STALLED", factBaseAt)
	err := fixture.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return fixture.episodes.SaveHit(txCtx, ghost)
	})
	if err == nil {
		t.Fatalf("不存在的发作期被 SaveHit 接受了")
	}
}
