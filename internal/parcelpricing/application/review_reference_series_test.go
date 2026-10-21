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

// 本文件证复核用例的编排（ADR-0099 决定二）：取登记 → 领域四眼门 → 复核册结果代数 → 答案
// 翻译。四眼不满足与请求不合法分格（恢复动作不同）；版本不在册是一格答案不是 error；依赖
// 故障未决不吞。复核时刻缺省取时钟。

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

type reviewFixture struct {
	handler    *application.ReviewReferenceSeriesHandler
	loader     *versionLoaderDouble
	reviews    *reviewRegisterDouble
	registered *pricingParameterDouble
	scopes     *pricedScopesDouble
}

func newReviewFixture(t *testing.T) *reviewFixture {
	t.Helper()
	registration := fuelRegistrationForReview(t)
	fixture := &reviewFixture{
		loader:     &versionLoaderDouble{registrations: map[string]domain.ReferenceSeriesRegistration{"SYN-FUEL@v1": registration}},
		reviews:    &reviewRegisterDouble{outcome: ports.ReferenceSeriesReviewRecorded},
		registered: &pricingParameterDouble{},
		scopes:     &pricedScopesDouble{scopes: boundScopes(t)},
	}
	fixture.handler = application.NewReviewReferenceSeriesHandler(application.ReviewReferenceSeriesDeps{
		Versions:     fixture.loader,
		Reviews:      fixture.reviews,
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
		"交接口":  {Versions: fixture.loader, Reviews: fixture.reviews, Clock: fixedClock{at: formedAt}, PricedScopes: fixture.scopes},
		"绑定读口": {Versions: fixture.loader, Reviews: fixture.reviews, Clock: fixedClock{at: formedAt}, Registered: fixture.registered},
	} {
		unwired := application.NewReviewReferenceSeriesHandler(deps)
		if _, err := unwired.Handle(context.Background(), reviewCommand("SYN-REVIEWER", "v1", domain.SeriesReviewApproved)); !errors.Is(err, application.ErrSeriesReviewHandoffMissing) {
			t.Fatalf("漏装%s err = %v", label, err)
		}
	}
}
