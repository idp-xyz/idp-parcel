package postgres

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// pgForeignKeyViolation 是 PostgreSQL 的 foreign_key_violation 代码。0004 的复合外键是
// 「复核一个不在册的版本」的最后一道墙；消费方能续办的那一格答案在 INSERT 之前就问出来了。
const pgForeignKeyViolation = "23503"

// ReferenceSeriesReviews 实现 ports.ReferenceSeriesReviewRegister 与
// ports.ReferenceSeriesInForceResolver（ADR-0099 决定二、三）。复核行只增不改；在用版本
// 不是本表或版本表上的列，是按评价形成时刻从复核记录派生的结论——本适配器只取候选，
// 选择规则在领域一处。
type ReferenceSeriesReviews struct {
	db *bentopg.DB
}

func NewReferenceSeriesReviews(db *bentopg.DB) (*ReferenceSeriesReviews, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &ReferenceSeriesReviews{db: db}, nil
}

// Record 追加一条复核。同键（版本 + 复核时刻 + 复核责任方）第二份由主键拦住，再按
// （结论 + 依据）比对译成结果代数：同即幂等重放，异即冲突，原行不顶替。版本不在册由
// 外键拦住，译成 VersionUnknown——先登记再复核。
//
// 走 RequireExecutor：复核与将来同一步的治理记录必须同生共死。
func (register *ReferenceSeriesReviews) Record(
	ctx context.Context,
	review domain.SeriesReview,
) (ports.ReferenceSeriesReviewOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReferenceSeriesReviewOutcomeInvalid, fmt.Errorf("record series review: %w", err)
	}
	if review.Tenant().String() == "" || review.Reference().ID() == "" {
		return ports.ReferenceSeriesReviewOutcomeInvalid, fmt.Errorf("record series review: review is not constructed")
	}

	// 先问版本在不在册，再写。外键照样守着，但撞外键会让整个事务进入 aborted 态，调用方
	// 的 commit 变成 rollback——那把一格「先去登记」的答案变成了一次技术失败。版本行只增
	// 不改，问的时候在、写的时候就还在；问的时候不在，就不写。
	var exists bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM parcel_pricing.reference_series_version
		    WHERE tenant_id = $1 AND series_id = $2 AND series_version = $3)`,
		review.Tenant().String(), review.Reference().ID(), review.Reference().Version(),
	).Scan(&exists)
	if err != nil {
		return ports.ReferenceSeriesReviewOutcomeInvalid, fmt.Errorf("record series review: %w", err)
	}
	if !exists {
		return ports.ReferenceSeriesReviewVersionUnknown, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_series_review
			(tenant_id, series_id, series_version, reviewer, reviewed_at, decision, basis)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)
		 ON CONFLICT DO NOTHING`,
		review.Tenant().String(),
		review.Reference().ID(),
		review.Reference().Version(),
		review.Reviewer(),
		review.ReviewedAt().UTC(),
		review.Decision().String(),
		review.Basis(),
	)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == pgForeignKeyViolation {
			// 上面刚问过在册；走到这里只可能是版本行被删——而那张表的纪律是只增不改，
			// 所以这是一条要人看的技术失败，不是「先去登记」。
			return ports.ReferenceSeriesReviewOutcomeInvalid, fmt.Errorf(
				"record series review: version %s/%s vanished between existence check and insert: %w",
				review.Reference().ID(), review.Reference().Version(), err)
		}
		return ports.ReferenceSeriesReviewOutcomeInvalid, fmt.Errorf("record series review: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.ReferenceSeriesReviewRecorded, nil
	}

	var decision, basis string
	err = executor.QueryRow(ctx,
		`SELECT decision, basis
		   FROM parcel_pricing.reference_series_review
		  WHERE tenant_id = $1 AND series_id = $2 AND series_version = $3
		    AND reviewed_at = $4 AND reviewer = $5`,
		review.Tenant().String(),
		review.Reference().ID(),
		review.Reference().Version(),
		review.ReviewedAt().UTC(),
		review.Reviewer(),
	).Scan(&decision, &basis)
	if err != nil {
		return ports.ReferenceSeriesReviewOutcomeInvalid, fmt.Errorf("record series review: 比对在册行：%w", err)
	}
	if decision != review.Decision().String() || basis != review.Basis() {
		return ports.ReferenceSeriesReviewConflict, nil
	}
	return ports.ReferenceSeriesReviewAlreadyRecorded, nil
}

// ResolveInForce 按（租户、种类、序列标识、评价形成时刻）解析在用版本。取这条序列的全部
// 版本与其每一条复核作候选，交 domain.SelectInForceSeriesVersion 挑；SQL 不排序裁决。
// 三个「没有」按恢复动作分格：无版本 / 有版本无通过复核 / 种类不合。
func (register *ReferenceSeriesReviews) ResolveInForce(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.ReferenceSeriesKind,
	seriesID string,
	at time.Time,
) (domain.VersionReference, ports.InForceOutcome, error) {
	if tenant.String() == "" || kind.String() == "" || seriesID == "" || at.IsZero() {
		return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf(
			"resolve in-force series version: tenant, kind, series and moment are required")
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf("resolve in-force series version: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT v.series_version, v.kind, v.registered_at, v.snapshot, r.reviewed_at, r.decision
		   FROM parcel_pricing.reference_series_version v
		   LEFT JOIN parcel_pricing.reference_series_review r
		     ON r.tenant_id = v.tenant_id AND r.series_id = v.series_id AND r.series_version = v.series_version
		  WHERE v.tenant_id = $1 AND v.series_id = $2`,
		tenant.String(), seriesID,
	)
	if err != nil {
		return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf("resolve in-force series version: %w", err)
	}
	defer rows.Close()

	references := make(map[string]domain.VersionReference)
	candidates := make([]domain.ReviewedSeriesVersion, 0)
	registered := false
	for rows.Next() {
		var version, rowKind, decision string
		var registeredAt time.Time
		var snapshot []byte
		var reviewedAt *time.Time
		var rowDecision *string
		if err := rows.Scan(&version, &rowKind, &registeredAt, &snapshot, &reviewedAt, &rowDecision); err != nil {
			return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf("resolve in-force series version: %w", err)
		}
		registered = true
		if rowKind != kind.String() {
			return domain.VersionReference{}, ports.SeriesKindDisagrees, nil
		}
		reference, cached := references[version]
		if !cached {
			reference, err = domain.PeekReferenceSeriesRegistrationReference(snapshot)
			if err != nil {
				return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf(
					"resolve in-force series version: %s/%s：%w", seriesID, version, err)
			}
			if reference.ID() != seriesID || reference.Version() != version {
				return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf(
					"resolve in-force series version: key columns disagree with the snapshot for %s/%s", seriesID, version)
			}
			references[version] = reference
		}
		if reviewedAt == nil || rowDecision == nil {
			continue
		}
		decision = *rowDecision
		candidate, err := domain.NewReviewedSeriesVersion(reference, registeredAt, *reviewedAt, domain.SeriesReviewDecision(decision))
		if err != nil {
			return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf(
				"resolve in-force series version: review row for %s/%s：%w", seriesID, version, err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return domain.VersionReference{}, ports.InForceOutcomeInvalid, fmt.Errorf("resolve in-force series version: %w", err)
	}
	if !registered {
		return domain.VersionReference{}, ports.SeriesHasNoRegisteredVersion, nil
	}
	inForce, found := domain.SelectInForceSeriesVersion(candidates, at)
	if !found {
		return domain.VersionReference{}, ports.SeriesHasNoApprovedVersion, nil
	}
	return inForce, ports.SeriesVersionInForce, nil
}

var (
	_ ports.ReferenceSeriesReviewRegister  = (*ReferenceSeriesReviews)(nil)
	_ ports.ReferenceSeriesInForceResolver = (*ReferenceSeriesReviews)(nil)
)
