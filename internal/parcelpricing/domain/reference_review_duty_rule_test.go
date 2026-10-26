package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件证参考复核职责规则的领域半边（ADR-0262 决定一）：人工复核的门按租户登记的这一条判——要求不同主体时登记者本人
// 复核答须换人，不要求时照落；要求某一格授予时复核者没有它答复核者不合格，不论他是谁；规则只裁本租户的复核。免复核声明
// 下的系统代写不收规则，只守写死的那一道「复核者不得是登记者」。

func reviewDutyRule(t *testing.T, tenant string, distinct bool, grant string) domain.ReferenceReviewDutyRule {
	t.Helper()
	var required domain.OperatorGrant
	if grant != "" {
		required = mustValue(t, domain.NewOperatorGrant, grant)
	}
	rule, err := domain.NewReferenceReviewDutyRule(mustValue(t, domain.NewTenantID, tenant), distinct, required)
	if err != nil {
		t.Fatalf("构造参考复核职责规则：%v", err)
	}
	return rule
}

func reviewerSubject(t *testing.T, reference string, grants ...string) domain.OperatorSubject {
	t.Helper()
	held := make([]domain.OperatorGrant, 0, len(grants))
	for _, name := range grants {
		held = append(held, mustValue(t, domain.NewOperatorGrant, name))
	}
	subject, err := domain.NewOperatorSubject(reference, held)
	if err != nil {
		t.Fatalf("构造复核者：%v", err)
	}
	return subject
}

func TestAReferenceReviewDutyRuleStatesBothCellsForOneTenant(t *testing.T) {
	if _, err := domain.NewReferenceReviewDutyRule(domain.TenantID{}, true, domain.OperatorGrant{}); !errors.Is(err, domain.ErrInvalidReferenceReviewDutyRule) {
		t.Fatalf("无租户的规则：err = %v，want ErrInvalidReferenceReviewDutyRule", err)
	}
	strict := reviewDutyRule(t, "tenant-1", true, "SYN-GRANT-REFERENCE-REVIEW")
	if strict.Tenant().String() != "tenant-1" || !strict.RequiresDistinctSubjects() {
		t.Fatalf("规则读回走样：%+v", strict)
	}
	if grant, required := strict.RequiredGrant(); !required || grant.String() != "SYN-GRANT-REFERENCE-REVIEW" {
		t.Fatalf("要求授予读回走样：%v %v", grant, required)
	}
	// 两格都不要求也是一条合法的租户声明：单人可复核由租户说出，不是系统缺省。
	loose := reviewDutyRule(t, "tenant-1", false, "")
	if loose.RequiresDistinctSubjects() {
		t.Fatal("不要求不同主体的规则读回成了要求")
	}
	if _, required := loose.RequiredGrant(); required {
		t.Fatal("不要求授予的规则读回成了要求")
	}
}

// Covers: AT-PP-015 的领域半边（序列）——要求不同主体时登记者本人复核答须换人；不要求时登记者本人复核照落；要求某一格授予
// 而复核者没有答复核者不合格，复核者是不是登记者都一样（受控 CLI 带不出授予集，空授予集就是它的形状）；持有那一格且是另一个
// 人才落。规则只裁本租户，零值规则立不住。
func TestAManualSeriesReviewIsJudgedByTheTenantReviewDutyRule(t *testing.T) {
	registration := fuelSeries(t)
	registrant := registration.Registrant()
	const grant = "SYN-GRANT-REFERENCE-REVIEW"
	cases := []struct {
		name     string
		rule     domain.ReferenceReviewDutyRule
		reviewer domain.OperatorSubject
		want     error
	}{
		{"要求不同主体、登记者本人", reviewDutyRule(t, "tenant-1", true, ""), reviewerSubject(t, registrant), domain.ErrSeriesReviewerIsRegistrant},
		{"要求不同主体、另一个人", reviewDutyRule(t, "tenant-1", true, ""), reviewerSubject(t, "SYN-PRC-SERIES-REVIEWER"), nil},
		{"不要求不同主体、登记者本人", reviewDutyRule(t, "tenant-1", false, ""), reviewerSubject(t, registrant), nil},
		{"要求授予、复核者没有", reviewDutyRule(t, "tenant-1", false, grant), reviewerSubject(t, "SYN-PRC-SERIES-REVIEWER", "SYN-GRANT-OTHER"), domain.ErrReferenceReviewerLacksRequiredGrant},
		{"要求授予、复核者持有", reviewDutyRule(t, "tenant-1", false, grant), reviewerSubject(t, "SYN-PRC-SERIES-REVIEWER", grant), nil},
		{"两格都要求、登记者本人且空授予集", reviewDutyRule(t, "tenant-1", true, grant), reviewerSubject(t, registrant), domain.ErrReferenceReviewerLacksRequiredGrant},
		{"两格都要求、另一个人且空授予集", reviewDutyRule(t, "tenant-1", true, grant), reviewerSubject(t, "SYN-PRC-SERIES-REVIEWER"), domain.ErrReferenceReviewerLacksRequiredGrant},
		{"两格都要求、登记者本人持有授予", reviewDutyRule(t, "tenant-1", true, grant), reviewerSubject(t, registrant, grant), domain.ErrSeriesReviewerIsRegistrant},
		{"他租户的规则", reviewDutyRule(t, "tenant-2", false, ""), reviewerSubject(t, "SYN-PRC-SERIES-REVIEWER"), domain.ErrReferenceReviewDutyRuleTenantMismatch},
		{"零值规则", domain.ReferenceReviewDutyRule{}, reviewerSubject(t, "SYN-PRC-SERIES-REVIEWER"), domain.ErrInvalidReferenceReviewDutyRule},
	}
	for _, testCase := range cases {
		review, err := domain.NewSeriesReview(registration, testCase.reviewer, testCase.rule, reviewMoment, domain.SeriesReviewApproved, "SYN-REVIEW/逐期核对")
		if testCase.want == nil {
			if err != nil {
				t.Fatalf("%s：err = %v，want 立得住", testCase.name, err)
			}
			if review.Reviewer() != testCase.reviewer.Reference() {
				t.Fatalf("%s：复核者 = %q", testCase.name, review.Reviewer())
			}
			continue
		}
		if !errors.Is(err, testCase.want) {
			t.Fatalf("%s：err = %v，want %v", testCase.name, err, testCase.want)
		}
	}
}

// Covers: AT-PP-015 的领域半边（目录）——目录复核与序列同用这一条规则，判法逐格相同。
func TestAManualCatalogueReviewIsJudgedByTheSameReviewDutyRule(t *testing.T) {
	registration := zoneChart(t)
	registrant := "SYN-CAT-REGISTRAR"
	const grant = "SYN-GRANT-REFERENCE-REVIEW"
	reviewedAt := catalogueYearStart.Add(24 * time.Hour)
	cases := []struct {
		name     string
		rule     domain.ReferenceReviewDutyRule
		reviewer domain.OperatorSubject
		want     error
	}{
		{"要求不同主体、登记者本人", reviewDutyRule(t, "tenant-1", true, ""), reviewerSubject(t, registrant), domain.ErrCatalogueReviewerIsRegistrant},
		{"不要求不同主体、登记者本人", reviewDutyRule(t, "tenant-1", false, ""), reviewerSubject(t, registrant), nil},
		{"要求授予、复核者没有", reviewDutyRule(t, "tenant-1", true, grant), reviewerSubject(t, "SYN-CAT-REVIEWER"), domain.ErrReferenceReviewerLacksRequiredGrant},
		{"要求授予、复核者持有", reviewDutyRule(t, "tenant-1", true, grant), reviewerSubject(t, "SYN-CAT-REVIEWER", grant), nil},
		{"他租户的规则", reviewDutyRule(t, "tenant-2", false, ""), reviewerSubject(t, "SYN-CAT-REVIEWER"), domain.ErrReferenceReviewDutyRuleTenantMismatch},
	}
	for _, testCase := range cases {
		_, err := domain.NewCatalogueReview(registration, testCase.reviewer, testCase.rule, reviewedAt, domain.SeriesReviewApproved, "SYN-REVIEW/zone-chart-v1")
		if testCase.want == nil {
			if err != nil {
				t.Fatalf("%s：err = %v，want 立得住", testCase.name, err)
			}
			continue
		}
		if !errors.Is(err, testCase.want) {
			t.Fatalf("%s：err = %v，want %v", testCase.name, err, testCase.want)
		}
	}
}

// Covers: AT-PP-016 的领域半边——免复核声明下的系统代写不收规则，写死的那一道比较照做：连接器身份就是登记者答须换人，
// 否则立得住；四件缺一同样立不住。
func TestAnExemptionDelegatedSeriesReviewKeepsOnlyTheFixedRegistrantComparison(t *testing.T) {
	registration := fuelSeries(t)
	review, err := domain.NewExemptionDelegatedSeriesReview(registration, "connector:FILE", reviewMoment, domain.SeriesReviewApproved, "免人工复核声明 ← 来源连接器绑定")
	if err != nil {
		t.Fatalf("代写复核：%v", err)
	}
	if review.Reviewer() != "connector:FILE" || review.Reference() != registration.Reference() {
		t.Fatalf("代写复核走样：%+v", review)
	}
	if _, err := domain.NewExemptionDelegatedSeriesReview(registration, registration.Registrant(), reviewMoment, domain.SeriesReviewApproved, "依据"); !errors.Is(err, domain.ErrSeriesReviewerIsRegistrant) {
		t.Fatalf("连接器身份就是登记者：err = %v，want ErrSeriesReviewerIsRegistrant", err)
	}
	if _, err := domain.NewExemptionDelegatedSeriesReview(registration, "connector:FILE", reviewMoment, domain.SeriesReviewApproved, ""); !errors.Is(err, domain.ErrInvalidSeriesReview) {
		t.Fatalf("缺依据：err = %v，want ErrInvalidSeriesReview", err)
	}
}
