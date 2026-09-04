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

// ReferenceCatalogueReviews 实现 ports.ReferenceCatalogueReviewRegister 与 ports.ReferenceCatalogueInForceResolver
// （ADR-0109 Decision 二：复核与在用照序列那一套）。复核行只增不改；在用版本不是列，是按时刻从复核记录派生
// 的结论——本适配器只取候选，选择规则在领域一处。
type ReferenceCatalogueReviews struct {
	db *bentopg.DB
}

func NewReferenceCatalogueReviews(db *bentopg.DB) (*ReferenceCatalogueReviews, error) {
	if db == nil {
		return nil, fmt.Errorf("parcel pricing postgres: db is nil")
	}
	return &ReferenceCatalogueReviews{db: db}, nil
}

// Record 追加一条复核。同键第二份由主键拦住，再按（结论 + 依据）比对译成结果代数；版本不在册先问再写，
// 理由与序列复核同：撞外键会让整个事务进入 aborted 态，把一格「先去登记」的答案变成技术失败。
func (register *ReferenceCatalogueReviews) Record(
	ctx context.Context,
	review domain.CatalogueReview,
) (ports.ReferenceCatalogueReviewOutcome, error) {
	executor, err := register.db.RequireExecutor(ctx)
	if err != nil {
		return ports.ReferenceCatalogueReviewOutcomeInvalid, fmt.Errorf("record catalogue review: %w", err)
	}
	if review.Tenant().String() == "" || review.Reference().ID() == "" {
		return ports.ReferenceCatalogueReviewOutcomeInvalid, fmt.Errorf("record catalogue review: review is not constructed")
	}

	var exists bool
	err = executor.QueryRow(ctx,
		`SELECT EXISTS (
		   SELECT 1 FROM parcel_pricing.reference_catalogue_version
		    WHERE tenant_id = $1 AND catalogue_id = $2 AND catalogue_version = $3)`,
		review.Tenant().String(), review.Reference().ID(), review.Reference().Version(),
	).Scan(&exists)
	if err != nil {
		return ports.ReferenceCatalogueReviewOutcomeInvalid, fmt.Errorf("record catalogue review: %w", err)
	}
	if !exists {
		return ports.ReferenceCatalogueReviewVersionUnknown, nil
	}

	tag, err := executor.Exec(ctx,
		`INSERT INTO parcel_pricing.reference_catalogue_review
			(tenant_id, catalogue_id, catalogue_version, reviewer, reviewed_at, decision, basis)
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
			return ports.ReferenceCatalogueReviewOutcomeInvalid, fmt.Errorf(
				"record catalogue review: version %s/%s vanished between existence check and insert: %w",
				review.Reference().ID(), review.Reference().Version(), err)
		}
		return ports.ReferenceCatalogueReviewOutcomeInvalid, fmt.Errorf("record catalogue review: %w", err)
	}
	if tag.RowsAffected() == 1 {
		return ports.ReferenceCatalogueReviewRecorded, nil
	}

	var decision, basis string
	err = executor.QueryRow(ctx,
		`SELECT decision, basis
		   FROM parcel_pricing.reference_catalogue_review
		  WHERE tenant_id = $1 AND catalogue_id = $2 AND catalogue_version = $3
		    AND reviewed_at = $4 AND reviewer = $5`,
		review.Tenant().String(),
		review.Reference().ID(),
		review.Reference().Version(),
		review.ReviewedAt().UTC(),
		review.Reviewer(),
	).Scan(&decision, &basis)
	if err != nil {
		return ports.ReferenceCatalogueReviewOutcomeInvalid, fmt.Errorf("record catalogue review: 比对在册行：%w", err)
	}
	if decision != review.Decision().String() || basis != review.Basis() {
		return ports.ReferenceCatalogueReviewConflict, nil
	}
	return ports.ReferenceCatalogueReviewAlreadyRecorded, nil
}

// ResolveInForce 按（租户、种类、目录标识、时刻）解析在用版本。取这本目录的全部版本与其每一条复核作候选，
// 交 domain.SelectInForceCatalogueVersion 挑；SQL 不排序裁决。
func (register *ReferenceCatalogueReviews) ResolveInForce(
	ctx context.Context,
	tenant domain.TenantID,
	kind domain.CatalogueKind,
	catalogueID string,
	at time.Time,
) (domain.VersionReference, ports.CatalogueInForceOutcome, error) {
	if tenant.String() == "" || kind.String() == "" || catalogueID == "" || at.IsZero() {
		return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf(
			"resolve in-force catalogue version: tenant, kind, catalogue and moment are required")
	}
	querier, err := register.db.ReadExecutor(ctx)
	if err != nil {
		return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf("resolve in-force catalogue version: %w", err)
	}

	rows, err := querier.Query(ctx,
		`SELECT v.catalogue_version, v.kind, v.registered_at, v.snapshot, r.reviewed_at, r.decision
		   FROM parcel_pricing.reference_catalogue_version v
		   LEFT JOIN parcel_pricing.reference_catalogue_review r
		     ON r.tenant_id = v.tenant_id AND r.catalogue_id = v.catalogue_id AND r.catalogue_version = v.catalogue_version
		  WHERE v.tenant_id = $1 AND v.catalogue_id = $2`,
		tenant.String(), catalogueID,
	)
	if err != nil {
		return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf("resolve in-force catalogue version: %w", err)
	}
	defer rows.Close()

	references := make(map[string]domain.VersionReference)
	candidates := make([]domain.ReviewedCatalogueVersion, 0)
	registered := false
	for rows.Next() {
		var version, rowKind string
		var registeredAt time.Time
		var snapshot []byte
		var reviewedAt *time.Time
		var rowDecision *string
		if err := rows.Scan(&version, &rowKind, &registeredAt, &snapshot, &reviewedAt, &rowDecision); err != nil {
			return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf("resolve in-force catalogue version: %w", err)
		}
		registered = true
		if rowKind != kind.String() {
			return domain.VersionReference{}, ports.CatalogueKindDisagrees, nil
		}
		reference, cached := references[version]
		if !cached {
			// 只读引用，不整版重建：万行级映射逐版重建的代价随版本数乘条目数增长，而挑选只需要引用。
			reference, err = domain.PeekReferenceCatalogueRegistrationReference(snapshot)
			if err != nil {
				return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf(
					"resolve in-force catalogue version: %s/%s：%w", catalogueID, version, err)
			}
			if reference.ID() != catalogueID || reference.Version() != version {
				return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf(
					"resolve in-force catalogue version: key columns disagree with the snapshot for %s/%s", catalogueID, version)
			}
			references[version] = reference
		}
		if reviewedAt == nil || rowDecision == nil {
			continue
		}
		candidate, err := domain.NewReviewedCatalogueVersion(reference, registeredAt, *reviewedAt, domain.SeriesReviewDecision(*rowDecision))
		if err != nil {
			return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf(
				"resolve in-force catalogue version: review row for %s/%s：%w", catalogueID, version, err)
		}
		candidates = append(candidates, candidate)
	}
	if err := rows.Err(); err != nil {
		return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, fmt.Errorf("resolve in-force catalogue version: %w", err)
	}
	if !registered {
		return domain.VersionReference{}, ports.CatalogueHasNoRegisteredVersion, nil
	}
	inForce, found := domain.SelectInForceCatalogueVersion(candidates, at)
	if !found {
		return domain.VersionReference{}, ports.CatalogueHasNoApprovedVersion, nil
	}
	return inForce, ports.CatalogueVersionInForce, nil
}

var (
	_ ports.ReferenceCatalogueReviewRegister  = (*ReferenceCatalogueReviews)(nil)
	_ ports.ReferenceCatalogueInForceResolver = (*ReferenceCatalogueReviews)(nil)
)
