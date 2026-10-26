package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证目录复核用例的编排（ADR-0109 决定二、ADR-0262 决定一）：与序列复核同读那一条参考复核职责规则——没登答未配置，
// 要求不同主体时登记者本人答须换人，要求授予而复核者没有答复核者不合格，三格都不到复核册；装配漏了规则读口是未决带错误。

type catalogueLoaderDouble struct {
	registration domain.ReferenceCatalogueRegistration
}

func (double catalogueLoaderDouble) LoadVersion(_ context.Context, _ domain.TenantID, catalogueID, catalogueVersion string) (domain.ReferenceCatalogueRegistration, bool, error) {
	if catalogueID != double.registration.Reference().ID() || catalogueVersion != double.registration.Reference().Version() {
		return domain.ReferenceCatalogueRegistration{}, false, nil
	}
	return double.registration, true, nil
}

type catalogueReviewRegisterDouble struct {
	recorded []domain.CatalogueReview
}

func (double *catalogueReviewRegisterDouble) Record(_ context.Context, review domain.CatalogueReview) (ports.ReferenceCatalogueReviewOutcome, error) {
	double.recorded = append(double.recorded, review)
	return ports.ReferenceCatalogueReviewRecorded, nil
}

func zoneCatalogueForReview(t *testing.T) domain.ReferenceCatalogueRegistration {
	t.Helper()
	period, err := domain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("生效区间：%v", err)
	}
	origin, err := domain.NewPostalPrefixCatalogueOrigin([]string{"200"})
	if err != nil {
		t.Fatalf("始发维度：%v", err)
	}
	entry, err := domain.NewCatalogueEntry("018", domain.CategoryValue("Z1"))
	if err != nil {
		t.Fatalf("条目：%v", err)
	}
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE", "v1")
	if err != nil {
		t.Fatalf("引用：%v", err)
	}
	registration, err := domain.NewReferenceCatalogueRegistration(domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.CatalogueKindZone,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/zone-bulletin",
		Registrant:       "SYN-REGISTRAR",
		Origin:           origin,
		PrefixLength:     3,
		Entries:          []domain.CatalogueEntry{entry},
		Period:           period,
	})
	if err != nil {
		t.Fatalf("目录登记：%v", err)
	}
	return registration
}

type catalogueReviewFixture struct {
	handler    *application.ReviewReferenceCatalogueHandler
	reviews    *catalogueReviewRegisterDouble
	registered *pricingParameterDouble
}

func newCatalogueReviewFixture(t *testing.T, rules ports.ReferenceReviewDutyRuleView) catalogueReviewFixture {
	t.Helper()
	fixture := catalogueReviewFixture{reviews: &catalogueReviewRegisterDouble{}, registered: &pricingParameterDouble{}}
	fixture.handler = application.NewReviewReferenceCatalogueHandler(application.ReviewReferenceCatalogueDeps{
		Versions:     catalogueLoaderDouble{registration: zoneCatalogueForReview(t)},
		Reviews:      fixture.reviews,
		DutyRules:    rules,
		Clock:        fixedClock{at: formedAt},
		Registered:   fixture.registered,
		PricedScopes: &pricedScopesDouble{scopes: boundScopes(t)},
	})
	return fixture
}

func catalogueReviewCommand(t *testing.T, reviewer string, grants ...string) application.ReviewReferenceCatalogueCommand {
	t.Helper()
	command := application.ReviewReferenceCatalogueCommand{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		CatalogueID:      "SYN-CAT-ZONE",
		CatalogueVersion: "v1",
		Reviewer:         reviewer,
		Decision:         domain.SeriesReviewApproved,
		Basis:            "SYN-REVIEW/逐条对照公布表",
	}
	for _, name := range grants {
		command.ReviewerGrants = append(command.ReviewerGrants, mustValue(t, domain.NewOperatorGrant, name))
	}
	return command
}

// Covers: AT-PP-014、AT-PP-015 的编排半边（目录）——同一条规则管目录：没登答未配置；要求不同主体时登记者本人答须换人，另一个
// 人照落并交参数已登记；不要求时登记者本人照落；要求授予而复核者没有答复核者不合格，持有才落。拒下的三格都不到复核册。
func TestAManualCatalogueReviewIsJudgedByTheTenantRule(t *testing.T) {
	const grant = "SYN-GRANT-REFERENCE-REVIEW"
	cases := []struct {
		name     string
		rules    *dutyRuleViewDouble
		command  application.ReviewReferenceCatalogueCommand
		want     application.ReviewReferenceCatalogueOutcome
		recorded int
	}{
		{"规则未登记", &dutyRuleViewDouble{}, catalogueReviewCommand(t, "SYN-REVIEWER"), application.CatalogueReviewNotConfigured, 0},
		{"要求不同主体、登记者本人", dutyRuleFor(t, "tenant-1", true, ""), catalogueReviewCommand(t, "SYN-REGISTRAR"), application.CatalogueReviewNeedsAnotherReviewer, 0},
		{"要求不同主体、另一个人", dutyRuleFor(t, "tenant-1", true, ""), catalogueReviewCommand(t, "SYN-REVIEWER"), application.CatalogueReviewRecorded, 1},
		{"不要求不同主体、登记者本人", dutyRuleFor(t, "tenant-1", false, ""), catalogueReviewCommand(t, "SYN-REGISTRAR"), application.CatalogueReviewRecorded, 1},
		{"要求授予、复核者没有", dutyRuleFor(t, "tenant-1", true, grant), catalogueReviewCommand(t, "SYN-REVIEWER"), application.CatalogueReviewReviewerNotQualified, 0},
		{"要求授予、复核者持有", dutyRuleFor(t, "tenant-1", true, grant), catalogueReviewCommand(t, "SYN-REVIEWER", grant), application.CatalogueReviewRecorded, 1},
	}
	for _, testCase := range cases {
		fixture := newCatalogueReviewFixture(t, testCase.rules)
		outcome, err := fixture.handler.Handle(context.Background(), testCase.command)
		if err != nil || outcome != testCase.want {
			t.Fatalf("%s：outcome = %s err = %v，want %s", testCase.name, outcome, err, testCase.want)
		}
		if len(fixture.reviews.recorded) != testCase.recorded {
			t.Fatalf("%s：到了复核册 %d 次，want %d", testCase.name, len(fixture.reviews.recorded), testCase.recorded)
		}
		if (testCase.recorded == 0) != (len(fixture.registered.intents) == 0) {
			t.Fatalf("%s：参数已登记 %d 封与落没落复核对不上", testCase.name, len(fixture.registered.intents))
		}
	}
	for outcome, name := range map[application.ReviewReferenceCatalogueOutcome]string{
		application.CatalogueReviewNotConfigured:        "NOT_CONFIGURED",
		application.CatalogueReviewReviewerNotQualified: "REVIEWER_NOT_QUALIFIED",
	} {
		if outcome.String() != name {
			t.Fatalf("%d 的名字 = %q，want %s", outcome, outcome, name)
		}
	}

	if outcome, err := newCatalogueReviewFixture(t, &dutyRuleViewDouble{err: errors.New("rule register down")}).handler.Handle(context.Background(),
		catalogueReviewCommand(t, "SYN-REVIEWER")); err == nil || outcome != application.CatalogueReviewUndecided {
		t.Fatalf("规则读不动：outcome = %s err = %v", outcome, err)
	}
	if outcome, err := newCatalogueReviewFixture(t, nil).handler.Handle(context.Background(),
		catalogueReviewCommand(t, "SYN-REVIEWER")); !errors.Is(err, application.ErrReferenceReviewDutyRuleViewMissing) || outcome != application.CatalogueReviewUndecided {
		t.Fatalf("漏装规则读口：outcome = %s err = %v", outcome, err)
	}
}
