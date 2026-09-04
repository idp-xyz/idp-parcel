package domain

import (
	"errors"
	"sort"
	"time"
)

var (
	// ErrInvalidSeriesReview 表示复核记录立不住：指不到一版已登记序列，或复核责任方、
	// 时刻、结论、依据四件有缺，或结论不在封闭集内。
	ErrInvalidSeriesReview = errors.New("parcel pricing: invalid reference series review")
	// ErrSeriesReviewerIsRegistrant 表示复核责任方就是登记责任方。PAR-SET-11 把两种责任
	// 分列，机制上的含义就是第二双眼睛：同一个人登记又复核，抄错的数没人能拦。
	ErrSeriesReviewerIsRegistrant = errors.New("parcel pricing: series review needs a reviewer other than the registrant")
)

// SeriesReviewDecision 是复核结论的封闭集。
type SeriesReviewDecision string

const (
	// SeriesReviewApproved：转录确认无误，自复核时刻起该版本可被新评价采用。
	SeriesReviewApproved SeriesReviewDecision = "APPROVED"
	// SeriesReviewReturned：退回。版本留在册上、不进在用，可再次复核；退回不删除。
	SeriesReviewReturned SeriesReviewDecision = "RETURNED"
)

func (decision SeriesReviewDecision) valid() bool {
	switch decision {
	case SeriesReviewApproved, SeriesReviewReturned:
		return true
	default:
		return false
	}
}

func (decision SeriesReviewDecision) String() string { return string(decision) }

// SeriesReview 是对一版已登记序列取值的转录正确性确认（CONTEXT「序列版本复核」）。它是
// 追加在版本之旁的独立事实，不改登记行一个字节；复核确认的是转录，不是凭证——证据等级
// 是取值凭证的属性，一版 ASSERTED 复核通过之后照样只有断言强度。
type SeriesReview struct {
	tenant     TenantID
	reference  VersionReference
	reviewer   string
	reviewedAt time.Time
	decision   SeriesReviewDecision
	basis      string
}

// NewSeriesReview 以被复核的登记为入参而不是以版本引用为入参：四眼门要拿到登记责任方，
// 做成这样门才在结构上关得住——调用方拿不到登记就造不出复核。
func NewSeriesReview(
	registration ReferenceSeriesRegistration,
	reviewer string,
	reviewedAt time.Time,
	decision SeriesReviewDecision,
	basis string,
) (SeriesReview, error) {
	if !registration.valid() || !trimmed(reviewer) || reviewedAt.IsZero() || !decision.valid() || !trimmed(basis) {
		return SeriesReview{}, ErrInvalidSeriesReview
	}
	if reviewer == registration.registrant {
		return SeriesReview{}, ErrSeriesReviewerIsRegistrant
	}
	return SeriesReview{
		tenant:     registration.tenant,
		reference:  registration.reference,
		reviewer:   reviewer,
		reviewedAt: reviewedAt.UTC(),
		decision:   decision,
		basis:      basis,
	}, nil
}

func (review SeriesReview) Tenant() TenantID               { return review.tenant }
func (review SeriesReview) Reference() VersionReference    { return review.reference }
func (review SeriesReview) Reviewer() string               { return review.reviewer }
func (review SeriesReview) ReviewedAt() time.Time          { return review.reviewedAt }
func (review SeriesReview) Decision() SeriesReviewDecision { return review.decision }
func (review SeriesReview) Basis() string                  { return review.basis }

// ReviewedSeriesVersion 是在用选择的最小候选载体：一版序列的引用、登记时刻，以及一条
// 复核的时刻与结论。同一版本有几条复核就是几个候选——选择规则看的是复核事实，不是
// 版本行上的某个状态列。
type ReviewedSeriesVersion struct {
	reference    VersionReference
	registeredAt time.Time
	reviewedAt   time.Time
	decision     SeriesReviewDecision
}

func NewReviewedSeriesVersion(
	reference VersionReference,
	registeredAt time.Time,
	reviewedAt time.Time,
	decision SeriesReviewDecision,
) (ReviewedSeriesVersion, error) {
	if reference.kind != ArtifactReferenceSeries || !reference.valid() ||
		registeredAt.IsZero() || reviewedAt.IsZero() || !decision.valid() {
		return ReviewedSeriesVersion{}, ErrInvalidSeriesReview
	}
	return ReviewedSeriesVersion{
		reference:    reference,
		registeredAt: registeredAt.UTC(),
		reviewedAt:   reviewedAt.UTC(),
		decision:     decision,
	}, nil
}

func (candidate ReviewedSeriesVersion) Reference() VersionReference    { return candidate.reference }
func (candidate ReviewedSeriesVersion) RegisteredAt() time.Time        { return candidate.registeredAt }
func (candidate ReviewedSeriesVersion) ReviewedAt() time.Time          { return candidate.reviewedAt }
func (candidate ReviewedSeriesVersion) Decision() SeriesReviewDecision { return candidate.decision }

// SelectInForceSeriesVersion 按评价形成时刻从复核记录派生在用版本（ADR-0099 决定三）：
// 该时刻之前（含同刻）复核通过的最新版本。「最新」按复核时刻，同刻按登记时刻，再同按
// 版本号字典序——判定必须确定，两次问同一时刻答同一版。退回不进在用，之后的退回也不
// 撤销已通过的复核（取值错误以更正版本处理）。没有候选时不给答案：评价据以挂起，不退到
// 任何未复核版本。
func SelectInForceSeriesVersion(candidates []ReviewedSeriesVersion, at time.Time) (VersionReference, bool) {
	reviewed := make([]reviewedVersion, 0, len(candidates))
	for _, candidate := range candidates {
		reviewed = append(reviewed, reviewedVersion(candidate))
	}
	return selectInForceVersion(reviewed, at)
}

// reviewedVersion 是在用选择的最小载体，序列与目录共用同一条判定（ADR-0109 Decision 二「复核与在用的门
// 照 ADR-0099 给序列立的那一套」）：两处各排一遍就是两处口径。
type reviewedVersion struct {
	reference    VersionReference
	registeredAt time.Time
	reviewedAt   time.Time
	decision     SeriesReviewDecision
}

func selectInForceVersion(candidates []reviewedVersion, at time.Time) (VersionReference, bool) {
	if at.IsZero() {
		return VersionReference{}, false
	}
	moment := at.UTC()
	eligible := make([]reviewedVersion, 0, len(candidates))
	for _, candidate := range candidates {
		if candidate.decision != SeriesReviewApproved || candidate.reviewedAt.After(moment) {
			continue
		}
		eligible = append(eligible, candidate)
	}
	if len(eligible) == 0 {
		return VersionReference{}, false
	}
	sort.SliceStable(eligible, func(left, right int) bool {
		if !eligible[left].reviewedAt.Equal(eligible[right].reviewedAt) {
			return eligible[left].reviewedAt.After(eligible[right].reviewedAt)
		}
		if !eligible[left].registeredAt.Equal(eligible[right].registeredAt) {
			return eligible[left].registeredAt.After(eligible[right].registeredAt)
		}
		return eligible[left].reference.version > eligible[right].reference.version
	})
	return eligible[0].reference, true
}
