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

// 本文件证复核用例的编排（ADR-0099 决定二、ADR-0262 决定一）：取登记 → 读租户的参考复核职责
// 规则 → 领域按规则判 → 复核册结果代数 → 答案翻译。未配置、须换人、复核者不合格与请求不合法
// 各分一格（恢复动作不同），三格拒下的都不到复核册；版本不在册是一格答案不是 error；依赖故障
// 未决不吞。复核时刻缺省取时钟。

type versionLoaderDouble struct {
	registrations map[string]domain.ReferenceSeriesRegistration
	err           error
}

func (double *versionLoaderDouble) LoadVersion(
	_ context.Context,
	_ domain.TenantID,
	seriesID string,
	seriesVersion string,
) (domain.ReferenceSeriesRegistration, bool, error) {
	if double.err != nil {
		return domain.ReferenceSeriesRegistration{}, false, double.err
	}
	registration, found := double.registrations[seriesID+"@"+seriesVersion]
	return registration, found, nil
}

type reviewRegisterDouble struct {
	outcome  ports.ReferenceSeriesReviewOutcome
	err      error
	recorded []domain.SeriesReview
}

func (double *reviewRegisterDouble) Record(_ context.Context, review domain.SeriesReview) (ports.ReferenceSeriesReviewOutcome, error) {
	if double.err != nil {
		return ports.ReferenceSeriesReviewOutcomeInvalid, double.err
	}
	double.recorded = append(double.recorded, review)
	return double.outcome, nil
}

func fuelRegistrationForReview(t *testing.T) domain.ReferenceSeriesRegistration {
	t.Helper()
	period, err := domain.NewSeriesPeriodValue(
		time.Date(2026, 8, 3, 0, 0, 0, 0, time.UTC), time.Date(2026, 8, 10, 0, 0, 0, 0, time.UTC),
		mustValue(t, domain.ParseDecimal, "0.22"), "SYN-EVIDENCE/fuel")
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	reference, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, "SYN-FUEL", "v1", "sha256:syn-fuel-v1")
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesFuelRate,
		Reference:        reference,
		SourceIdentifier: "SYN-CARRIER/fuel",
		Registrant:       "SYN-REGISTRAR",
		Periods:          []domain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}

// dutyRuleViewDouble 是参考复核职责规则读口的替身：rule 为零值即答未登记。
type dutyRuleViewDouble struct {
	rule  domain.ReferenceReviewDutyRule
	found bool
	err   error
	asked int
}

func (double *dutyRuleViewDouble) LoadReferenceReviewDutyRule(_ context.Context, _ domain.TenantID) (domain.ReferenceReviewDutyRule, bool, error) {
	double.asked++
	return double.rule, double.found, double.err
}

func dutyRuleFor(t *testing.T, tenant string, distinct bool, grant string) *dutyRuleViewDouble {
	t.Helper()
	var required domain.OperatorGrant
	if grant != "" {
		required = mustValue(t, domain.NewOperatorGrant, grant)
	}
	rule, err := domain.NewReferenceReviewDutyRule(mustValue(t, domain.NewTenantID, tenant), distinct, required)
	if err != nil {
		t.Fatalf("构造规则：%v", err)
	}
	return &dutyRuleViewDouble{rule: rule, found: true}
}

type reviewFixture struct {
	handler    *application.ReviewReferenceSeriesHandler
	loader     *versionLoaderDouble
	reviews    *reviewRegisterDouble
	rules      *dutyRuleViewDouble
	registered *pricingParameterDouble
	scopes     *pricedScopesDouble
}

// newReviewFixture 装的是要求不同主体、不要求授予的那一条规则——与演示租户登的同形。
func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	return newReviewFixtureWith(t, dutyRuleFor(t, "tenant-1", true, ""))
}

func newReviewFixtureWith(t *testing.T, rules *dutyRuleViewDouble) *reviewFixture {
	t.Helper()
	registration := fuelRegistrationForReview(t)
	fixture := &reviewFixture{
		loader:     &versionLoaderDouble{registrations: map[string]domain.ReferenceSeriesRegistration{"SYN-FUEL@v1": registration}},
		reviews:    &reviewRegisterDouble{outcome: ports.ReferenceSeriesReviewRecorded},
		rules:      rules,
		registered: &pricingParameterDouble{},
		scopes:     &pricedScopesDouble{scopes: boundScopes(t)},
	}
	fixture.handler = application.NewReviewReferenceSeriesHandler(application.ReviewReferenceSeriesDeps{
		Versions:     fixture.loader,
		Reviews:      fixture.reviews,
		DutyRules:    fixture.rules,
		Clock:        fixedClock{at: formedAt},
		Registered:   fixture.registered,
		PricedScopes: fixture.scopes,
	})
	return fixture
}

func reviewCommand(reviewer, version string, decision domain.SeriesReviewDecision) application.ReviewReferenceSeriesCommand {
	tenant, _ := domain.NewTenantID("tenant-1")
	return application.ReviewReferenceSeriesCommand{
		Tenant:        tenant,
		SeriesID:      "SYN-FUEL",
		SeriesVersion: version,
		Reviewer:      reviewer,
		Decision:      decision,
		Basis:         "SYN-REVIEW/逐期核对",
	}
}

func TestReviewIsRecordedWithTheClockMomentWhenNoneGiven(t *testing.T) {
	fixture := newReviewFixture(t)
	outcome, err := fixture.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved))
	if err != nil || outcome != application.SeriesReviewRecorded {
		t.Fatalf("outcome = %q err = %v", outcome, err)
	}
	if len(fixture.reviews.recorded) != 1 || !fixture.reviews.recorded[0].ReviewedAt().Equal(formedAt) || fixture.reviews.recorded[0].Reviewer() != "SYN-REVIEWER" {
		t.Fatalf("recorded = %#v", fixture.reviews.recorded)
	}

	explicit := reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewReturned)
	explicit.ReviewedAt = time.Date(2026, 8, 20, 9, 0, 0, 0, time.UTC)
	if _, err := fixture.handler.Handle(context.Background(), explicit); err != nil {
		t.Fatalf("explicit handle: %v", err)
	}
	if got := fixture.reviews.recorded[1].ReviewedAt(); !got.Equal(explicit.ReviewedAt) {
		t.Fatalf("explicit moment lost: %s", got)
	}
}

func TestReviewOutcomesFollowRecoveryActions(t *testing.T) {
	fixture := newReviewFixture(t)

	if outcome, err := fixture.handler.Handle(context.Background(), reviewCommand("SYN-REGISTRAR", "v1", domain.SeriesReviewApproved)); err != nil || outcome != application.SeriesReviewNeedsAnotherReviewer {
		t.Fatalf("registrant reviewing own version: outcome = %q err = %v", outcome, err)
	}
	if len(fixture.reviews.recorded) != 0 {
		t.Fatal("a refused review reached the register")
	}

	if outcome, err := fixture.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v7", domain.SeriesReviewApproved)); err != nil || outcome != application.SeriesReviewVersionUnknown {
		t.Fatalf("unknown version: outcome = %q err = %v", outcome, err)
	}

	malformed := reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewDecision("MAYBE"))
	if outcome, err := fixture.handler.Handle(context.Background(), malformed); err != nil || outcome != application.SeriesReviewNotAccepted {
		t.Fatalf("unknown decision: outcome = %q err = %v", outcome, err)
	}
	if outcome, err := fixture.handler.Handle(context.Background(), application.ReviewReferenceSeriesCommand{}); err != nil || outcome != application.SeriesReviewNotAccepted {
		t.Fatalf("zero command: outcome = %q err = %v", outcome, err)
	}

	for label, registered := range map[string]struct {
		outcome ports.ReferenceSeriesReviewOutcome
		want    application.ReviewReferenceSeriesOutcome
	}{
		"幂等重放":  {ports.ReferenceSeriesReviewAlreadyRecorded, application.SeriesReviewAlreadyOnRegister},
		"同键异内容": {ports.ReferenceSeriesReviewConflict, application.SeriesReviewConflict},
	} {
		fixture.reviews.outcome = registered.outcome
		if outcome, err := fixture.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err != nil || outcome != registered.want {
			t.Fatalf("%s: outcome = %q err = %v", label, outcome, err)
		}
	}

	fixture.reviews.outcome = ports.ReferenceSeriesReviewRecorded
	fixture.reviews.err = errors.New("review register down")
	if outcome, err := fixture.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err == nil || outcome != application.SeriesReviewUndecided {
		t.Fatalf("register down: outcome = %q err = %v", outcome, err)
	}
	fixture.reviews.err = nil
	fixture.loader.err = errors.New("version store down")
	if outcome, err := fixture.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err == nil || outcome != application.SeriesReviewUndecided {
		t.Fatalf("loader down: outcome = %q err = %v", outcome, err)
	}
}

// Covers: AT-PP-004 一版序列通过复核才交「参数已登记」：先问绑了这条序列的卡在哪些范围上，按 BUY 与 SELL 的那几面各交
// 一封（种类 REFERENCE_SERIES、那一版的引用、那一面的范围与方向），内部方向的面不交，没有卡绑它就一封不交；退回的那一版
// 不在用，不交、也不问；问不出或交不出整笔报错让登记口回滚；装配漏了交接口或绑定读口就不受理。
func TestAnApprovedReviewAnnouncesTheSeriesOnEveryScopeThatBindsIt(t *testing.T) {
	fixture := newReviewFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err != nil {
		t.Fatalf("复核：%v", err)
	}
	if len(fixture.scopes.asked) != 1 || fixture.scopes.asked[0] != "REFERENCE_SERIES/SYN-FUEL" {
		t.Fatalf("问绑定 = %v", fixture.scopes.asked)
	}
	if len(fixture.registered.intents) != 2 {
		t.Fatalf("交了 %d 封参数已登记, want BUY 与 SELL 各一封", len(fixture.registered.intents))
	}
	for i, want := range []struct{ scope, direction string }{{"SYN-SCOPE-BUY", "BUY"}, {"SYN-SCOPE-SELL", "SELL"}} {
		intent := fixture.registered.intents[i]
		if intent.Kind != ports.PricingParameterReferenceSeries || intent.Registration.ID() != "SYN-FUEL" ||
			intent.Registration.Version() != "v1" || intent.Scope.String() != want.scope ||
			intent.Direction.String() != want.direction || intent.Tenant.String() != "tenant-1" {
			t.Fatalf("第 %d 封 intent = %+v", i, intent)
		}
	}

	unbound := newReviewFixture(t)
	unbound.scopes.scopes = nil
	if outcome, err := unbound.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err != nil ||
		outcome != application.SeriesReviewRecorded || len(unbound.registered.intents) != 0 {
		t.Fatalf("没有卡绑：outcome = %s err = %v 交了 %d 封", outcome, err, len(unbound.registered.intents))
	}

	returned := newReviewFixture(t)
	if _, err := returned.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewReturned)); err != nil {
		t.Fatalf("退回：%v", err)
	}
	if len(returned.registered.intents) != 0 || len(returned.scopes.asked) != 0 {
		t.Fatal("退回的那一版交了参数已登记或问了绑定")
	}

	failing := newReviewFixture(t)
	failing.registered.err = errors.New("outbox down")
	if outcome, err := failing.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err == nil ||
		outcome != application.SeriesReviewUndecided {
		t.Fatalf("交不出 outcome = %s err = %v, want 未决与错误", outcome, err)
	}
	blind := newReviewFixture(t)
	blind.scopes.err = errors.New("card register down")
	if outcome, err := blind.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err == nil ||
		outcome != application.SeriesReviewUndecided || len(blind.registered.intents) != 0 {
		t.Fatalf("问不出绑定 outcome = %s err = %v, want 未决与错误、一封不交", outcome, err)
	}

	for label, deps := range map[string]application.ReviewReferenceSeriesDeps{
		"交接口":  {Versions: fixture.loader, Reviews: fixture.reviews, DutyRules: fixture.rules, Clock: fixedClock{at: formedAt}, PricedScopes: fixture.scopes},
		"绑定读口": {Versions: fixture.loader, Reviews: fixture.reviews, DutyRules: fixture.rules, Clock: fixedClock{at: formedAt}, Registered: fixture.registered},
	} {
		unwired := application.NewReviewReferenceSeriesHandler(deps)
		if _, err := unwired.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); !errors.Is(err, application.ErrSeriesReviewHandoffMissing) {
			t.Fatalf("漏装%s err = %v", label, err)
		}
	}
}

// Covers: AT-PP-014 的编排半边（序列）——租户没登参考复核职责规则时，人工复核答未配置、不到复核册、不交参数已登记；登记者
// 本人与另一个人答的是同一格，不按默认双人也不按默认单人放行。装配漏了规则读口不是未配置，是未决带错误。
func TestAManualSeriesReviewWithoutATenantRuleIsNotConfigured(t *testing.T) {
	fixture := newReviewFixtureWith(t, &dutyRuleViewDouble{})
	for _, reviewer := range []string{"SYN-REGISTRAR", "SYN-REVIEWER"} {
		if outcome, err := fixture.handler.Handle(context.Background(), reviewCommand(reviewer, "v1", domain.SeriesReviewApproved)); err != nil || outcome != application.SeriesReviewNotConfigured {
			t.Fatalf("%s 复核：outcome = %s err = %v，want NOT_CONFIGURED", reviewer, outcome, err)
		}
	}
	if len(fixture.reviews.recorded) != 0 || len(fixture.registered.intents) != 0 {
		t.Fatalf("未配置到了复核册或交了信封：recorded=%d intents=%d", len(fixture.reviews.recorded), len(fixture.registered.intents))
	}
	if application.SeriesReviewNotConfigured.String() != "NOT_CONFIGURED" {
		t.Fatalf("未配置的名字 = %q", application.SeriesReviewNotConfigured)
	}

	down := newReviewFixtureWith(t, &dutyRuleViewDouble{err: errors.New("rule register down")})
	if outcome, err := down.handler.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); err == nil || outcome != application.SeriesReviewUndecided {
		t.Fatalf("规则读不动：outcome = %s err = %v，want 未决与错误", outcome, err)
	}

	unwired := application.NewReviewReferenceSeriesHandler(application.ReviewReferenceSeriesDeps{
		Versions: fixture.loader, Reviews: fixture.reviews, Clock: fixedClock{at: formedAt},
		Registered: fixture.registered, PricedScopes: fixture.scopes,
	})
	if outcome, err := unwired.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); !errors.Is(err, application.ErrReferenceReviewDutyRuleViewMissing) || outcome != application.SeriesReviewUndecided {
		t.Fatalf("漏装规则读口：outcome = %s err = %v", outcome, err)
	}
}

// Covers: AT-PP-015 的编排半边（序列）——规则要求不同主体时登记者本人答须换人、不到复核册；不要求时登记者本人复核照落、
// 交参数已登记；要求某一格授予时复核者的授予集（随命令从操作者信封来）里没有它答复核者不合格，持有它才落；空授予集（受控
// 批量口的形状）在要求授予时一律不合格，复核者是不是登记者都一样。
func TestAManualSeriesReviewFollowsTheTenantRule(t *testing.T) {
	const grant = "SYN-GRANT-REFERENCE-REVIEW"
	withGrants := func(reviewer string, grants ...string) application.ReviewReferenceSeriesCommand {
		command := reviewCommand(reviewer, "v1", domain.SeriesReviewApproved)
		for _, name := range grants {
			command.ReviewerGrants = append(command.ReviewerGrants, mustValue(t, domain.NewOperatorGrant, name))
		}
		return command
	}
	cases := []struct {
		name     string
		rules    *dutyRuleViewDouble
		command  application.ReviewReferenceSeriesCommand
		want     application.ReviewReferenceSeriesOutcome
		recorded int
	}{
		{"要求不同主体、登记者本人", dutyRuleFor(t, "tenant-1", true, ""), withGrants("SYN-REGISTRAR"), application.SeriesReviewNeedsAnotherReviewer, 0},
		{"不要求不同主体、登记者本人", dutyRuleFor(t, "tenant-1", false, ""), withGrants("SYN-REGISTRAR"), application.SeriesReviewRecorded, 1},
		{"要求授予、复核者没有", dutyRuleFor(t, "tenant-1", false, grant), withGrants("SYN-REVIEWER", "SYN-GRANT-OTHER"), application.SeriesReviewReviewerNotQualified, 0},
		{"要求授予、复核者持有", dutyRuleFor(t, "tenant-1", false, grant), withGrants("SYN-REVIEWER", grant), application.SeriesReviewRecorded, 1},
		{"两格都要求、登记者本人空授予集", dutyRuleFor(t, "tenant-1", true, grant), withGrants("SYN-REGISTRAR"), application.SeriesReviewReviewerNotQualified, 0},
		{"两格都要求、另一个人空授予集", dutyRuleFor(t, "tenant-1", true, grant), withGrants("SYN-REVIEWER"), application.SeriesReviewReviewerNotQualified, 0},
	}
	for _, testCase := range cases {
		fixture := newReviewFixtureWith(t, testCase.rules)
		outcome, err := fixture.handler.Handle(context.Background(), testCase.command)
		if err != nil || outcome != testCase.want {
			t.Fatalf("%s：outcome = %s err = %v，want %s", testCase.name, outcome, err, testCase.want)
		}
		if len(fixture.reviews.recorded) != testCase.recorded {
			t.Fatalf("%s：到了复核册 %d 次，want %d", testCase.name, len(fixture.reviews.recorded), testCase.recorded)
		}
		if testCase.recorded == 0 && len(fixture.registered.intents) != 0 {
			t.Fatalf("%s：拒下的复核交了参数已登记", testCase.name)
		}
	}
	if application.SeriesReviewReviewerNotQualified.String() != "REVIEWER_NOT_QUALIFIED" {
		t.Fatalf("复核者不合格的名字 = %q", application.SeriesReviewReviewerNotQualified)
	}

	foreign := newReviewFixtureWith(t, dutyRuleFor(t, "tenant-2", false, ""))
	if outcome, err := foreign.handler.Handle(context.Background(), withGrants("SYN-REVIEWER")); !errors.Is(err, domain.ErrReferenceReviewDutyRuleTenantMismatch) || outcome != application.SeriesReviewUndecided {
		t.Fatalf("读口交回他租户的规则：outcome = %s err = %v，want 未决与错误", outcome, err)
	}
}
