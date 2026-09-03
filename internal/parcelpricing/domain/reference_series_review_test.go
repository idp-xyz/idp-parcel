package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证 ADR-0099 决定二与三的领域半边：序列版本复核是追加在版本之旁的独立事实，复核
// 责任方不得与登记责任方相同，复核不改证据等级；在用版本按（评价形成时刻）从复核记录派生
// ——该时刻之前复核通过的最新版本，退回与未来的复核都不算，判定确定不留任选。

var (
	reviewMoment      = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	reviewMomentLater = time.Date(2026, 8, 21, 9, 0, 0, 0, time.UTC)
)

func approvedReview(t *testing.T, registration domain.ReferenceSeriesRegistration) domain.SeriesReview {
	t.Helper()
	review, err := domain.NewSeriesReview(registration, "SYN-PRC-SERIES-REVIEWER", reviewMoment, domain.SeriesReviewApproved, "SYN-REVIEW/勾稽公布记录逐期核对")
	if err != nil {
		t.Fatalf("构造复核：%v", err)
	}
	return review
}

// TestSeriesReviewCarriesItsOwnFacts 证复核记录带齐四件（责任方、时刻、结论、依据）并指回
// 被复核的那一版；它不改登记行——登记对象在复核前后一字不变。
func TestSeriesReviewCarriesItsOwnFacts(t *testing.T) {
	registration := fuelSeries(t)
	before, _ := domain.MarshalReferenceSeriesRegistration(registration)

	review := approvedReview(t, registration)

	if review.Tenant() != registration.Tenant() || review.Reference() != registration.Reference() {
		t.Fatalf("复核指错版本：%v / %v", review.Tenant(), review.Reference())
	}
	if review.Reviewer() != "SYN-PRC-SERIES-REVIEWER" || !review.ReviewedAt().Equal(reviewMoment) ||
		review.Decision() != domain.SeriesReviewApproved || review.Basis() == "" {
		t.Fatalf("复核四件不齐：%+v", review)
	}
	after, _ := domain.MarshalReferenceSeriesRegistration(registration)
	if string(before) != string(after) {
		t.Fatal("复核改动了登记快照")
	}
}

// TestSeriesReviewNeedsASecondPairOfEyes 证复核责任方 ≠ 登记责任方（PAR-SET-11 分列两种
// 责任的机制落点）：同一个人登记又复核，转录错误没有第二双眼睛能拦。
func TestSeriesReviewNeedsASecondPairOfEyes(t *testing.T) {
	registration := fuelSeries(t)
	_, err := domain.NewSeriesReview(registration, registration.Registrant(), reviewMoment, domain.SeriesReviewApproved, "自己复核自己")
	if !errors.Is(err, domain.ErrSeriesReviewerIsRegistrant) {
		t.Fatalf("err = %v，想要 ErrSeriesReviewerIsRegistrant", err)
	}
}

// TestSeriesReviewRefusesIncompleteFacts 证缺任一件的复核立不住：没有依据的通过与没有时刻
// 的退回都指不出「谁在何时凭什么」。结论必须在封闭集内。
func TestSeriesReviewRefusesIncompleteFacts(t *testing.T) {
	registration := fuelSeries(t)
	cases := map[string]func() (domain.SeriesReview, error){
		"空复核人": func() (domain.SeriesReview, error) {
			return domain.NewSeriesReview(registration, " ", reviewMoment, domain.SeriesReviewApproved, "依据")
		},
		"零时刻": func() (domain.SeriesReview, error) {
			return domain.NewSeriesReview(registration, "reviewer", time.Time{}, domain.SeriesReviewApproved, "依据")
		},
		"空依据": func() (domain.SeriesReview, error) {
			return domain.NewSeriesReview(registration, "reviewer", reviewMoment, domain.SeriesReviewReturned, "")
		},
		"未知结论": func() (domain.SeriesReview, error) {
			return domain.NewSeriesReview(registration, "reviewer", reviewMoment, domain.SeriesReviewDecision("MAYBE"), "依据")
		},
		"空登记": func() (domain.SeriesReview, error) {
			return domain.NewSeriesReview(domain.ReferenceSeriesRegistration{}, "reviewer", reviewMoment, domain.SeriesReviewApproved, "依据")
		},
	}
	for label, build := range cases {
		if _, err := build(); !errors.Is(err, domain.ErrInvalidSeriesReview) {
			t.Fatalf("%s：err = %v，想要 ErrInvalidSeriesReview", label, err)
		}
	}
}

// TestSeriesReviewDoesNotChangeTheEvidenceGrade 证复核确认的是转录，不是凭证：一版 ASSERTED
// 复核通过之后仍是 ASSERTED——等级是取值凭证的属性，第二双眼睛看的是抄得对不对。
func TestSeriesReviewDoesNotChangeTheEvidenceGrade(t *testing.T) {
	registration := fuelSeries(t)
	if registration.Verifiable() {
		t.Fatal("夹具应有一期缺凭证")
	}
	approvedReview(t, registration)
	if registration.Verifiable() {
		t.Fatal("复核把断言强度抬成了可复核")
	}
}

func reviewed(t *testing.T, version string, registeredAt, reviewedAt time.Time, decision domain.SeriesReviewDecision) domain.ReviewedSeriesVersion {
	t.Helper()
	candidate, err := domain.NewReviewedSeriesVersion(
		versionReference(t, domain.ArtifactReferenceSeries, "SYN-PRC-FUEL-WEEKLY", version),
		registeredAt, reviewedAt, decision,
	)
	if err != nil {
		t.Fatalf("构造候选 %s：%v", version, err)
	}
	return candidate
}

// TestInForceIsTheLatestApprovedBeforeTheMoment 证在用版本 = 该时刻之前复核通过的最新版本：
// 按复核时刻取最新，之后才通过的那一版对这一刻不算。
func TestInForceIsTheLatestApprovedBeforeTheMoment(t *testing.T) {
	registeredAt := seriesWeekOne
	candidates := []domain.ReviewedSeriesVersion{
		reviewed(t, "v1", registeredAt, reviewMoment, domain.SeriesReviewApproved),
		reviewed(t, "v2", registeredAt.Add(time.Hour), reviewMomentLater, domain.SeriesReviewApproved),
	}
	at := reviewMoment.Add(time.Hour)
	inForce, found := domain.SelectInForceSeriesVersion(candidates, at)
	if !found || inForce.Version() != "v1" {
		t.Fatalf("在用 = %v/%v，想要 v1（v2 在此刻之后才通过）", inForce, found)
	}
	inForce, found = domain.SelectInForceSeriesVersion(candidates, reviewMomentLater)
	if !found || inForce.Version() != "v2" {
		t.Fatalf("在用 = %v/%v，想要 v2（复核时刻等于评价时刻即已在用）", inForce, found)
	}
}

// TestReturnedAndUnreviewedVersionsAreNeverInForce 证退回不进在用；只有退回记录时无在用版本，
// 评价据以挂起而不是退到任何未复核版本。
func TestReturnedAndUnreviewedVersionsAreNeverInForce(t *testing.T) {
	candidates := []domain.ReviewedSeriesVersion{
		reviewed(t, "v1", seriesWeekOne, reviewMoment, domain.SeriesReviewReturned),
	}
	if _, found := domain.SelectInForceSeriesVersion(candidates, reviewMomentLater); found {
		t.Fatal("退回的版本被选为在用")
	}
	if _, found := domain.SelectInForceSeriesVersion(nil, reviewMomentLater); found {
		t.Fatal("没有候选却选出了在用版本")
	}
	if _, found := domain.SelectInForceSeriesVersion(candidates, time.Time{}); found {
		t.Fatal("零时刻选出了在用版本")
	}
}

// TestInForceTieBreakIsDeterministic 证同一复核时刻按登记时刻、再按版本号字典序取最新——
// 判定不留任选，两次问同一时刻答同一版。
func TestInForceTieBreakIsDeterministic(t *testing.T) {
	sameMoment := reviewMoment
	byRegistration := []domain.ReviewedSeriesVersion{
		reviewed(t, "v1", seriesWeekOne, sameMoment, domain.SeriesReviewApproved),
		reviewed(t, "v2", seriesWeekTwo, sameMoment, domain.SeriesReviewApproved),
	}
	if inForce, _ := domain.SelectInForceSeriesVersion(byRegistration, reviewMomentLater); inForce.Version() != "v2" {
		t.Fatalf("同刻复核应按登记时刻取最新，得 %s", inForce.Version())
	}
	byVersion := []domain.ReviewedSeriesVersion{
		reviewed(t, "v10", seriesWeekOne, sameMoment, domain.SeriesReviewApproved),
		reviewed(t, "v9", seriesWeekOne, sameMoment, domain.SeriesReviewApproved),
	}
	first, _ := domain.SelectInForceSeriesVersion(byVersion, reviewMomentLater)
	second, _ := domain.SelectInForceSeriesVersion([]domain.ReviewedSeriesVersion{byVersion[1], byVersion[0]}, reviewMomentLater)
	if first != second || first.Version() != "v9" {
		t.Fatalf("字典序取最新应稳定为 v9，得 %s / %s", first.Version(), second.Version())
	}
}

// TestALaterReturnDoesNotRevokeAnApproval 证复核不撤销：通过之后再来一条退回，不把已在用的
// 版本拉下来——取值错误以更正版本处理（CONTEXT 生命周期）。
func TestALaterReturnDoesNotRevokeAnApproval(t *testing.T) {
	candidates := []domain.ReviewedSeriesVersion{
		reviewed(t, "v1", seriesWeekOne, reviewMoment, domain.SeriesReviewApproved),
		reviewed(t, "v1", seriesWeekOne, reviewMomentLater, domain.SeriesReviewReturned),
	}
	inForce, found := domain.SelectInForceSeriesVersion(candidates, reviewMomentLater.Add(time.Hour))
	if !found || inForce.Version() != "v1" {
		t.Fatalf("在用 = %v/%v，想要 v1 仍在用", inForce, found)
	}
}

// TestReviewedSeriesVersionRefusesIncompleteCandidates 证候选载体自己把门：不是序列引用、
// 缺时刻、结论不在封闭集，都不成候选。
func TestReviewedSeriesVersionRefusesIncompleteCandidates(t *testing.T) {
	if _, err := domain.NewReviewedSeriesVersion(
		versionReference(t, domain.ArtifactPricingPlan, "plan", "v1"), seriesWeekOne, reviewMoment, domain.SeriesReviewApproved,
	); !errors.Is(err, domain.ErrInvalidSeriesReview) {
		t.Fatalf("非序列引用被接受：%v", err)
	}
	if _, err := domain.NewReviewedSeriesVersion(
		versionReference(t, domain.ArtifactReferenceSeries, "s", "v1"), time.Time{}, reviewMoment, domain.SeriesReviewApproved,
	); !errors.Is(err, domain.ErrInvalidSeriesReview) {
		t.Fatalf("零登记时刻被接受：%v", err)
	}
	if _, err := domain.NewReviewedSeriesVersion(
		versionReference(t, domain.ArtifactReferenceSeries, "s", "v1"), seriesWeekOne, reviewMoment, domain.SeriesReviewDecision(""),
	); !errors.Is(err, domain.ErrInvalidSeriesReview) {
		t.Fatalf("空结论被接受：%v", err)
	}
}
