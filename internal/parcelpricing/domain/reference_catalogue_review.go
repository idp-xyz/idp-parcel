package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidCatalogueReview 表示目录复核记录立不住：指不到一版已登记目录，或复核责任方、时刻、结论、
	// 依据四件有缺。
	ErrInvalidCatalogueReview = errors.New("parcel pricing: invalid reference catalogue review")
	// ErrCatalogueReviewerIsRegistrant 表示复核责任方就是登记责任方——第二双眼睛的门与序列同一条。
	ErrCatalogueReviewerIsRegistrant = errors.New("parcel pricing: catalogue review needs a reviewer other than the registrant")
)

// CatalogueReview 是对一版已登记目录的转录正确性确认（ADR-0109 Decision 二：复核照 ADR-0099 给序列立的
// 那一套）。结论封闭集与序列复核共用一套（通过 / 退回）——词只定一次。它是追加在版本之旁的独立事实，
// 不改登记行一个字节。
type CatalogueReview struct {
	tenant     TenantID
	reference  VersionReference
	reviewer   string
	reviewedAt time.Time
	decision   SeriesReviewDecision
	basis      string
}

// NewCatalogueReview 以被复核的登记为入参：四眼门要拿到登记责任方，调用方拿不到登记就造不出复核。
func NewCatalogueReview(
	registration ReferenceCatalogueRegistration,
	reviewer string,
	reviewedAt time.Time,
	decision SeriesReviewDecision,
	basis string,
) (CatalogueReview, error) {
	if !registration.valid() || !trimmed(reviewer) || reviewedAt.IsZero() || !decision.valid() || !trimmed(basis) {
		return CatalogueReview{}, ErrInvalidCatalogueReview
	}
	if reviewer == registration.registrant {
		return CatalogueReview{}, ErrCatalogueReviewerIsRegistrant
	}
	return CatalogueReview{
		tenant:     registration.tenant,
		reference:  registration.reference,
		reviewer:   reviewer,
		reviewedAt: reviewedAt.UTC(),
		decision:   decision,
		basis:      basis,
	}, nil
}

func (review CatalogueReview) Tenant() TenantID               { return review.tenant }
func (review CatalogueReview) Reference() VersionReference    { return review.reference }
func (review CatalogueReview) Reviewer() string               { return review.reviewer }
func (review CatalogueReview) ReviewedAt() time.Time          { return review.reviewedAt }
func (review CatalogueReview) Decision() SeriesReviewDecision { return review.decision }
func (review CatalogueReview) Basis() string                  { return review.basis }

// ReviewedCatalogueVersion 是目录在用选择的候选：一版目录的引用、登记时刻，以及一条复核的时刻与结论。
type ReviewedCatalogueVersion struct {
	reference    VersionReference
	registeredAt time.Time
	reviewedAt   time.Time
	decision     SeriesReviewDecision
}

func NewReviewedCatalogueVersion(
	reference VersionReference,
	registeredAt time.Time,
	reviewedAt time.Time,
	decision SeriesReviewDecision,
) (ReviewedCatalogueVersion, error) {
	if reference.kind != ArtifactReferenceCatalogue || !reference.valid() ||
		registeredAt.IsZero() || reviewedAt.IsZero() || !decision.valid() {
		return ReviewedCatalogueVersion{}, ErrInvalidCatalogueReview
	}
	return ReviewedCatalogueVersion{
		reference:    reference,
		registeredAt: registeredAt.UTC(),
		reviewedAt:   reviewedAt.UTC(),
		decision:     decision,
	}, nil
}

func (candidate ReviewedCatalogueVersion) Reference() VersionReference    { return candidate.reference }
func (candidate ReviewedCatalogueVersion) RegisteredAt() time.Time        { return candidate.registeredAt }
func (candidate ReviewedCatalogueVersion) ReviewedAt() time.Time          { return candidate.reviewedAt }
func (candidate ReviewedCatalogueVersion) Decision() SeriesReviewDecision { return candidate.decision }

// SelectInForceCatalogueVersion 按评价基准时点从复核记录派生在用目录版本——判定与 SelectInForceSeriesVersion
// 是同一条：该时刻之前复核通过的最新版本，退回不进在用，没有候选不给答案。
func SelectInForceCatalogueVersion(candidates []ReviewedCatalogueVersion, at time.Time) (VersionReference, bool) {
	reviewed := make([]reviewedVersion, 0, len(candidates))
	for _, candidate := range candidates {
		reviewed = append(reviewed, reviewedVersion(candidate))
	}
	return selectInForceVersion(reviewed, at)
}
