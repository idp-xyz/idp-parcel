package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证评价用例的在用解析半边（ADR-0099 决定四）：输入缺序列取值时，编排在形成评价前
// 按（租户、种类、序列标识、评价形成时刻）解析在用版本、按计价基准时点解析期次并补齐；解析
// 不到不编造，留说明进解释，评价照旧待判断；同标识重发借原评价冻结的取值比对，不重新解析；
// 输入自带取值的请求与重放一律不解析；半套装配在受理时拒。

type inForceDouble struct {
	reference domain.VersionReference
	outcome   ports.InForceOutcome
	err       error
	calls     int
	lastAt    time.Time
	lastKind  domain.ReferenceSeriesKind
	lastID    string
}

func (double *inForceDouble) ResolveInForce(
	_ context.Context,
	_ domain.TenantID,
	kind domain.ReferenceSeriesKind,
	seriesID string,
	at time.Time,
) (domain.VersionReference, ports.InForceOutcome, error) {
	double.calls++
	double.lastAt, double.lastKind, double.lastID = at, kind, seriesID
	if double.err != nil {
		return domain.VersionReference{}, ports.InForceOutcomeInvalid, double.err
	}
	return double.reference, double.outcome, nil
}

type seriesReadingDouble struct {
	readings map[string]domain.ResolvedSeriesReading
	lastAsOf time.Time
	err      error
}

func (double *seriesReadingDouble) Register(context.Context, domain.ReferenceSeriesRegistration) (ports.ReferenceSeriesRegistrationOutcome, error) {
	return ports.ReferenceSeriesRegistrationOutcomeInvalid, errors.New("not used")
}

func (double *seriesReadingDouble) ResolveAt(
	_ context.Context,
	_ domain.TenantID,
	reference domain.VersionReference,
	asOf time.Time,
) (domain.ResolvedSeriesReading, bool, error) {
	double.lastAsOf = asOf
	if double.err != nil {
		return domain.ResolvedSeriesReading{}, false, double.err
	}
	reading, found := double.readings[reference.Version()]
	return reading, found, nil
}

var (
	formedAt = time.Date(2026, 9, 3, 15, 0, 0, 0, time.UTC)
	fxPolicy = "SYN-FX-POLICY"
)

func fxReference(t *testing.T, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, "SYN-FX-USD-CNY", version, "sha256:syn-fx-"+version)
	if err != nil {
		t.Fatalf("fx reference: %v", err)
	}
	return reference
}

// fxReading 造一期已解析的汇率取值：登记一版单期序列再按期内时点解析，取值随口径进来。
func fxReading(t *testing.T, version, rate string) domain.ResolvedSeriesReading {
	t.Helper()
	policy, err := domain.NewVersionReference(domain.ArtifactCommercialPolicy, fxPolicy, "v1", "sha256:syn-policy")
	if err != nil {
		t.Fatalf("policy reference: %v", err)
	}
	period, err := domain.NewSeriesPeriodValue(
		time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{},
		mustValue(t, domain.ParseDecimal, rate), "SYN-EVIDENCE/fx")
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	registration, err := domain.NewReferenceSeriesRegistration(domain.ReferenceSeriesRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.ReferenceSeriesExchangeRate,
		Reference:        fxReference(t, version),
		SourceIdentifier: "SYN-TREASURY",
		Registrant:       "SYN-OPS",
		QuoteBasis:       policy,
		Periods:          []domain.SeriesPeriodValue{period},
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	reading, found := registration.ResolveAt(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC))
	if !found {
		t.Fatal("fixture period does not cover the pricing basis time")
	}
	return reading
}

// fxBoundPlan 是 minimalPlan 加一条汇率序列绑定：卡按 USD 定价，输入要求 CNY 结算时评价
// 必须拿到汇率才能完成。
func fxBoundPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	base := minimalPlan(t)
	binding, err := domain.NewReferenceSeriesBinding(domain.ReferenceSeriesExchangeRate, "SYN-FX-USD-CNY")
	if err != nil {
		t.Fatalf("binding: %v", err)
	}
	structures, err := domain.NewPricingPlanStructures(nil, nil, []domain.ReferenceSeriesBinding{binding})
	if err != nil {
		t.Fatalf("structures: %v", err)
	}
	plan, err := domain.NewPricingPlanVersion(
		base.Reference(), base.Scope(), base.Direction(), base.Purpose(), base.BaseChargeCode(),
		base.EffectivePeriod(), base.RateTable(), base.WeightPolicy(), nil, structures,
	)
	if err != nil {
		t.Fatalf("plan: %v", err)
	}
	return plan
}

func fxCommand(t *testing.T, id string) application.EvaluatePricingCommand {
	t.Helper()
	input, err := minimalInput(t, "Z1").WithSettlementCurrency(mustValue(t, domain.NewCurrency, "CNY"))
	if err != nil {
		t.Fatalf("settlement currency: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), fxBoundPlan(t), input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return application.EvaluatePricingCommand{Request: request}
}

type seriesFixture struct {
	handler  *application.EvaluatePricingHandler
	store    *evaluationStoreDouble
	inForce  *inForceDouble
	register *seriesReadingDouble
}

func newSeriesFixture(t *testing.T) *seriesFixture {
	t.Helper()
	fixture := &seriesFixture{
		store:    &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		inForce:  &inForceDouble{reference: fxReference(t, "v2"), outcome: ports.SeriesVersionInForce},
		register: &seriesReadingDouble{readings: map[string]domain.ResolvedSeriesReading{"v2": fxReading(t, "v2", "7.2"), "v3": fxReading(t, "v3", "7.5")}},
	}
	fixture.handler = application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:          fixture.store,
		Downstream:     &evaluationDownstreamDouble{},
		Clock:          fixedClock{at: formedAt},
		InForce:        fixture.inForce,
		SeriesVersions: fixture.register,
	})
	return fixture
}

// Covers: ADR-0099 决定四——形成时刻定版本、基准时点定期次；解析到的版本冻结进评价清单，
// 评价完成且按结算币种输出。
func TestEvaluationResolvesTheInForceSeriesVersionBeforeForming(t *testing.T) {
	fixture := newSeriesFixture(t)

	result, err := fixture.handler.Handle(context.Background(), fxCommand(t, "eval-fx-1"))
	if err != nil || result.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
	}
	evaluation, _ := result.Evaluation()
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %q issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	total, _ := evaluation.Total()
	if total.Currency().String() != "CNY" || total.Amount().String() != "72" {
		t.Fatalf("total = %s %s, want 72 CNY (10 USD × 7.2)", total.Amount(), total.Currency())
	}
	if fixture.inForce.calls != 1 || !fixture.inForce.lastAt.Equal(formedAt) || fixture.inForce.lastKind != domain.ReferenceSeriesExchangeRate || fixture.inForce.lastID != "SYN-FX-USD-CNY" {
		t.Fatalf("in-force asked wrongly: calls=%d at=%s kind=%s id=%s", fixture.inForce.calls, fixture.inForce.lastAt, fixture.inForce.lastKind, fixture.inForce.lastID)
	}
	if !fixture.register.lastAsOf.Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) {
		t.Fatalf("period resolved at %s, want the pricing basis time, not the forming moment", fixture.register.lastAsOf)
	}
	frozen := false
	for _, reference := range evaluation.Manifest().References() {
		if reference.Kind() == domain.ArtifactReferenceSeries && reference.Version() == "v2" {
			frozen = true
		}
	}
	if !frozen {
		t.Fatalf("manifest = %#v, want SYN-FX-USD-CNY@v2 frozen in", evaluation.Manifest().References())
	}
	if len(result.SeriesResolutionNotes()) != 0 {
		t.Fatalf("notes = %#v, want none when everything resolved", result.SeriesResolutionNotes())
	}
}

// Covers: 同标识重发借原评价冻结的取值比对——在用版本此后换成 v3，重发同一请求仍是重放
// 而不是冲突，且不重新解析。
func TestResendingBorrowsTheFrozenSeriesReadingInsteadOfResolvingAgain(t *testing.T) {
	fixture := newSeriesFixture(t)
	if _, err := fixture.handler.Handle(context.Background(), fxCommand(t, "eval-fx-2")); err != nil {
		t.Fatalf("first handle: %v", err)
	}

	fixture.inForce.reference = fxReference(t, "v3")
	callsBefore := fixture.inForce.calls
	resent, err := fixture.handler.Handle(context.Background(), fxCommand(t, "eval-fx-2"))
	if err != nil || resent.Outcome() != application.EvaluationExistingResult {
		t.Fatalf("resend outcome = %q err = %v, want EXISTING_RESULT", resent.Outcome(), err)
	}
	if fixture.inForce.calls != callsBefore {
		t.Fatal("resending a recorded evaluation resolved the in-force version again")
	}
	if fixture.store.saved != 1 {
		t.Fatalf("saved = %d", fixture.store.saved)
	}
}

// Covers: 解析不到不编造——评价待判断，原因文字按恢复动作分格进解释与结果。
func TestUnresolvedSeriesLeavesTheEvaluationPendingWithAReason(t *testing.T) {
	cases := map[string]struct {
		outcome ports.InForceOutcome
		wants   string
	}{
		"无已登记版本": {ports.SeriesHasNoRegisteredVersion, "no registered version"},
		"有版本未复核": {ports.SeriesHasNoApprovedVersion, "none approved by review"},
		"种类不合":   {ports.SeriesKindDisagrees, "another kind"},
	}
	for label, tc := range cases {
		fixture := newSeriesFixture(t)
		fixture.inForce.outcome = tc.outcome
		result, err := fixture.handler.Handle(context.Background(), fxCommand(t, "eval-fx-"+label))
		if err != nil || result.Outcome() != application.EvaluationRecorded {
			t.Fatalf("%s: outcome = %q err = %v", label, result.Outcome(), err)
		}
		evaluation, _ := result.Evaluation()
		if evaluation.Status() != domain.EvaluationPending {
			t.Fatalf("%s: status = %q, want PENDING", label, evaluation.Status())
		}
		if len(result.SeriesResolutionNotes()) != 1 || !strings.Contains(result.SeriesResolutionNotes()[0], tc.wants) {
			t.Fatalf("%s: notes = %#v, want one mentioning %q", label, result.SeriesResolutionNotes(), tc.wants)
		}
		if !strings.Contains(strings.Join(evaluation.Explanation(), "\n"), tc.wants) {
			t.Fatalf("%s: explanation = %#v lacks the reason", label, evaluation.Explanation())
		}
	}

	gap := newSeriesFixture(t)
	gap.register.readings = map[string]domain.ResolvedSeriesReading{}
	result, err := gap.handler.Handle(context.Background(), fxCommand(t, "eval-fx-gap"))
	if err != nil {
		t.Fatalf("gap handle: %v", err)
	}
	evaluation, _ := result.Evaluation()
	if evaluation.Status() != domain.EvaluationPending || len(result.SeriesResolutionNotes()) != 1 || !strings.Contains(result.SeriesResolutionNotes()[0], "no period covering") {
		t.Fatalf("gap: status = %q notes = %#v", evaluation.Status(), result.SeriesResolutionNotes())
	}
}

// Covers: 输入自带取值不解析；依赖故障是未决不是待判断；半套装配在受理时拒。
func TestSeriesResolutionBoundaries(t *testing.T) {
	supplied := newSeriesFixture(t)
	command := fxCommand(t, "eval-fx-supplied")
	input, err := command.Request.Input().WithReferenceSeries(fxReading(t, "v9", "8.0").Value())
	if err != nil {
		t.Fatalf("attach reading: %v", err)
	}
	command.Request, err = domain.NewEvaluationRequest(command.Request.ID(), command.Request.Plan(), input, command.Request.Evidence())
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	result, err := supplied.handler.Handle(context.Background(), command)
	if err != nil || result.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
	}
	if supplied.inForce.calls != 0 {
		t.Fatal("a request that already carries the reading was resolved again")
	}
	evaluation, _ := result.Evaluation()
	if total, _ := evaluation.Total(); total.Amount().String() != "80" {
		t.Fatalf("total = %s, want the supplied 8.0 rate to be used", total.Amount())
	}

	broken := newSeriesFixture(t)
	broken.inForce.err = errors.New("register unreachable")
	result, err = broken.handler.Handle(context.Background(), fxCommand(t, "eval-fx-broken"))
	if err != nil || result.Outcome() != application.EvaluationUndecided {
		t.Fatalf("outcome = %q err = %v, want UNDECIDED when the register is down", result.Outcome(), err)
	}
	if broken.store.saved != 0 {
		t.Fatal("a dependency failure was recorded as a pending evaluation")
	}

	half := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:      &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		Downstream: &evaluationDownstreamDouble{},
		Clock:      fixedClock{at: formedAt},
		InForce:    &inForceDouble{},
	})
	if _, err := half.Handle(context.Background(), fxCommand(t, "eval-fx-half")); !errors.Is(err, application.ErrSeriesResolutionHalfWired) {
		t.Fatalf("half-wired handler err = %v, want ErrSeriesResolutionHalfWired", err)
	}
}
