package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// SignalEpisodes 实现 ports.SignalEpisodeStore。发作期与它的分诊结论分两表、
// SaveRaised 同一事务写入——只落发作期不落结论，重试会走进「已有活跃发作期」那一支
// 去记命中，结论就永远补不上了（端口注释钉的正是这半句）。
type SignalEpisodes struct {
	db *bentopg.DB
}

func NewSignalEpisodes(db *bentopg.DB) (*SignalEpisodes, error) {
	if db == nil {
		return nil, fmt.Errorf("visibility exception postgres: db is nil")
	}
	return &SignalEpisodes{db: db}, nil
}

// FindLatest 按对象+类型取回最近一次发作期，含已结束的——「已结束+再命中」要据它
// 建立关联的新发作期，只查活跃会把重开误判成首启。重开的首命中允许与前期结束同刻，
// started_at 并列时由到达序（seq）裁决。读回经 RehydrateSignalEpisode 重验生命周期
// 形状。
func (repository *SignalEpisodes) FindLatest(
	ctx context.Context,
	parcel domain.TrackedParcelReference,
	kind domain.ExceptionSignalKindReference,
) (*domain.SignalEpisode, bool, error) {
	querier, err := repository.db.ReadExecutor(ctx)
	if err != nil {
		return nil, false, fmt.Errorf("find latest episode: %w", err)
	}

	var (
		episodeID, ruleRef, confidenceRef string
		hits                              int
		startedAt, lastHitAt              time.Time
		releaseBasis, priorEpisode        *string
		endedAt                           *time.Time
	)
	err = querier.QueryRow(ctx,
		`SELECT episode_id, rule_ref, confidence_ref, hits, started_at, last_hit_at,
		        release_basis, ended_at, prior_episode
		   FROM visibility_exception.signal_episode
		  WHERE parcel_ref = $1 AND kind_ref = $2
		  ORDER BY started_at DESC, seq DESC
		  LIMIT 1`,
		parcel.String(), kind.String(),
	).Scan(&episodeID, &ruleRef, &confidenceRef, &hits, &startedAt, &lastHitAt,
		&releaseBasis, &endedAt, &priorEpisode)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("find latest episode: %w", err)
	}

	snapshot := domain.SignalEpisodeSnapshot{
		Kind:      kind,
		Parcel:    parcel,
		Hits:      hits,
		StartedAt: startedAt,
		LastHitAt: lastHitAt,
	}
	if snapshot.ID, err = domain.NewEpisodeID(episodeID); err != nil {
		return nil, false, fmt.Errorf("rebuild episode: %w", err)
	}
	if snapshot.Rule, err = domain.NewSignalRuleVersionReference(ruleRef); err != nil {
		return nil, false, fmt.Errorf("rebuild episode: %w", err)
	}
	if snapshot.Confidence, err = domain.NewConfidenceReference(confidenceRef); err != nil {
		return nil, false, fmt.Errorf("rebuild episode: %w", err)
	}
	if releaseBasis != nil {
		snapshot.ReleaseBasis = *releaseBasis
	}
	if endedAt != nil {
		snapshot.EndedAt = *endedAt
	}
	if priorEpisode != nil {
		if snapshot.PriorEpisode, err = domain.NewEpisodeID(*priorEpisode); err != nil {
			return nil, false, fmt.Errorf("rebuild episode: %w", err)
		}
	}

	episode, err := domain.RehydrateSignalEpisode(snapshot)
	if err != nil {
		return nil, false, fmt.Errorf("rebuild episode: %w", err)
	}
	return episode, true, nil
}

// SaveRaised 把新开或重开的发作期与它的分诊结论同一事务写入。发作期是新键新行，
// 并发撞键不是业务答案而是身份签发失守——不译 ON CONFLICT，让唯一约束如实报错，
// 事务整体回退后重试会在 FindLatest 看到赢家。
//
// 走 RequireExecutor：两条 INSERT 必须同生共死，这正是本方法存在的理由。
func (repository *SignalEpisodes) SaveRaised(
	ctx context.Context,
	record ports.RaisedSignalRecord,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save raised signal: %w", err)
	}
	if record.Episode == nil {
		return fmt.Errorf("save raised signal: episode is nil")
	}
	snapshot := record.Episode.Snapshot()
	if record.Parcel != snapshot.Parcel || record.Kind != snapshot.Kind {
		return fmt.Errorf("save raised signal: record scope disagrees with the episode")
	}
	if record.Conclusion.Episode() != snapshot.ID {
		return fmt.Errorf("save raised signal: conclusion belongs to another episode")
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.signal_episode
			(episode_id, parcel_ref, kind_ref, rule_ref, confidence_ref,
			 hits, started_at, last_hit_at, release_basis, ended_at, prior_episode)
		 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
		snapshot.ID.String(),
		snapshot.Parcel.String(),
		snapshot.Kind.String(),
		snapshot.Rule.String(),
		snapshot.Confidence.String(),
		snapshot.Hits,
		snapshot.StartedAt,
		snapshot.LastHitAt,
		nullIfBlank(snapshot.ReleaseBasis),
		nullIfZeroTime(snapshot.EndedAt),
		nullIfBlank(snapshot.PriorEpisode.String()),
	); err != nil {
		return fmt.Errorf("save raised signal: episode: %w", err)
	}

	if _, err := executor.Exec(ctx,
		`INSERT INTO visibility_exception.triage_conclusion
			(episode_id, outcome, rule_ref, triaged_at)
		 VALUES ($1, $2, $3, $4)`,
		record.Conclusion.Episode().String(),
		record.Conclusion.Outcome().String(),
		record.Conclusion.Rule().String(),
		record.Conclusion.TriagedAt(),
	); err != nil {
		return fmt.Errorf("save raised signal: conclusion: %w", err)
	}
	return nil
}

// SaveHit 落同一发作期内的判断历史推进（命中数、末次命中，或结束时的解除依据）。
// 行不存在如实报错——命中只可能落在 FindLatest 刚交回的发作期上。
func (repository *SignalEpisodes) SaveHit(
	ctx context.Context,
	episode *domain.SignalEpisode,
) error {
	executor, err := repository.db.RequireExecutor(ctx)
	if err != nil {
		return fmt.Errorf("save signal hit: %w", err)
	}
	if episode == nil {
		return fmt.Errorf("save signal hit: episode is nil")
	}
	snapshot := episode.Snapshot()

	tag, err := executor.Exec(ctx,
		`UPDATE visibility_exception.signal_episode
		    SET hits = $2, last_hit_at = $3, release_basis = $4, ended_at = $5
		  WHERE episode_id = $1`,
		snapshot.ID.String(),
		snapshot.Hits,
		snapshot.LastHitAt,
		nullIfBlank(snapshot.ReleaseBasis),
		nullIfZeroTime(snapshot.EndedAt),
	)
	if err != nil {
		return fmt.Errorf("save signal hit: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("save signal hit: episode %s not found", snapshot.ID)
	}
	return nil
}

func nullIfBlank(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func nullIfZeroTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	return &value
}
