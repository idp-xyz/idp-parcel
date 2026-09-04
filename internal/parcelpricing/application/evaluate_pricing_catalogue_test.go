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

// 本文件证评价用例的目录解析半边（ADR-0109 Decision 三、四）：输入缺目录读数时，编排在形成评价前按
// （租户、种类、目录标识、评价形成时刻）解析在用版本、按计价基准时点与邮编路线解读数并补齐；查不到不编
// 造——「查过没查到」的读数照样冻结，评价落待判断；无在用版本留说明；依赖故障是未决；半套装配受理时拒。

type catalogueInForceDouble struct {
	reference domain.VersionReference
	outcome   ports.CatalogueInForceOutcome
	err       error
	calls     int
	lastAt    time.Time
	lastKind  domain.CatalogueKind
	lastID    string
}

func (double *catalogueInForceDouble) ResolveInForce(
	_ context.Context,
	_ domain.TenantID,
	kind domain.CatalogueKind,
	catalogueID string,
	at time.Time,
) (domain.VersionReference, ports.CatalogueInForceOutcome, error) {
	double.calls++
	double.lastAt, double.lastKind, double.lastID = at, kind, catalogueID
	if double.err != nil {
		return domain.VersionReference{}, ports.CatalogueInForceOutcomeInvalid, double.err
	}
	return double.reference, double.outcome, nil
}

type catalogueReadingDouble struct {
	registrations map[string]domain.ReferenceCatalogueRegistration
	lastAsOf      time.Time
	lastRoute     domain.PostalRoute
	err           error
}

func (double *catalogueReadingDouble) Register(context.Context, domain.ReferenceCatalogueRegistration) (ports.ReferenceCatalogueRegistrationOutcome, error) {
	return ports.ReferenceCatalogueRegistrationOutcomeInvalid, errors.New("not used")
}

func (double *catalogueReadingDouble) ResolveAt(
	_ context.Context,
	_ domain.TenantID,
	reference domain.VersionReference,
	asOf time.Time,
	route domain.PostalRoute,
) (domain.ResolvedCatalogueValue, bool, error) {
	double.lastAsOf, double.lastRoute = asOf, route
	if double.err != nil {
		return domain.ResolvedCatalogueValue{}, false, double.err
	}
	registration, found := double.registrations[reference.Version()]
	if !found {
		return domain.ResolvedCatalogueValue{}, false, nil
	}
	reading, applicable := registration.ResolveAt(asOf, route)
	return reading, applicable, nil
}

func catalogueVersion(t *testing.T, version string) domain.VersionReference {
	t.Helper()
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactReferenceCatalogue, "SYN-CAT-ZONE", version)
	if err != nil {
		t.Fatalf("catalogue reference: %v", err)
	}
	return reference
}

// zoneChartVersion 造一版单表分区目录：目的前缀 902 → Z1（minimalPlan 的价表只有 Z1 一行）。
func zoneChartVersion(t *testing.T, version string) domain.ReferenceCatalogueRegistration {
	t.Helper()
	period, err := domain.NewEffectivePeriod(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("period: %v", err)
	}
	entry, err := domain.NewCatalogueEntry("902", "Z1")
	if err != nil {
		t.Fatalf("entry: %v", err)
	}
	registration, err := domain.NewReferenceCatalogueRegistration(domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           mustValue(t, domain.NewTenantID, "tenant-1"),
		Kind:             domain.CatalogueKindZone,
		Reference:        catalogueVersion(t, version),
		SourceIdentifier: "SYN-CARRIER/zone-chart",
		Registrant:       "SYN-OPS",
		Origin:           domain.NewIndependentCatalogueOrigin(),
		PrefixLength:     3,
		Entries:          []domain.CatalogueEntry{entry},
		Period:           period,
	})
	if err != nil {
		t.Fatalf("registration: %v", err)
	}
	return registration
}

// catalogueBoundPlan 是 minimalPlan 加一条分区目录绑定：卡不再读调用方给的分区。
func catalogueBoundPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	base := minimalPlan(t)
	link, err := domain.NewReferenceCatalogueLink(domain.CatalogueKindZone, "SYN-CAT-ZONE")
	if err != nil {
		t.Fatalf("link: %v", err)
	}
	structures, err := (domain.PricingPlanStructures{}).WithReferenceCatalogues(link)
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

func postalCommand(t *testing.T, id, destination string) application.EvaluatePricingCommand {
	t.Helper()
	subject, err := domain.NewAcceptedPackageSubject(mustValue(t, domain.NewPackageID, "package-1"))
	if err != nil {
		t.Fatalf("subject: %v", err)
	}
	actual, err := domain.NewWeight(mustValue(t, domain.ParseDecimal, "5"), domain.WeightUnitKilogram)
	if err != nil {
		t.Fatalf("actual weight: %v", err)
	}
	route, err := domain.NewPostalRoute("", destination)
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	input, err := domain.NewPostalPricingInputSnapshot(
		mustValue(t, domain.NewTenantID, "tenant-1"),
		mustValue(t, domain.NewPricingScopeID, "scope-1"),
		subject, route, actual, nil,
		time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC),
	)
	if err != nil {
		t.Fatalf("input: %v", err)
	}
	request, err := domain.NewEvaluationRequest(mustValue(t, domain.NewEvaluationID, id), catalogueBoundPlan(t), input, domain.EvidenceSynthetic)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	return application.EvaluatePricingCommand{Request: request}
}

type catalogueFixture struct {
	handler  *application.EvaluatePricingHandler
	store    *evaluationStoreDouble
	inForce  *catalogueInForceDouble
	register *catalogueReadingDouble
}

func newCatalogueFixture(t *testing.T) *catalogueFixture {
	t.Helper()
	fixture := &catalogueFixture{
		store:    &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		inForce:  &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce},
		register: &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{"v2": zoneChartVersion(t, "v2")}},
	}
	fixture.handler = application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:            fixture.store,
		Downstream:       &evaluationDownstreamDouble{},
		Clock:            fixedClock{at: formedAt},
		CatalogueInForce: fixture.inForce,
		Catalogues:       fixture.register,
	})
	return fixture
}

// Covers: ADR-0109 Decision 三——形成时刻定在用版本、基准时点与邮编路线定读数；版本冻进清单，评价完成。
func TestEvaluationResolvesTheZoneFromTheInForceCatalogueBeforeForming(t *testing.T) {
	fixture := newCatalogueFixture(t)

	result, err := fixture.handler.Handle(context.Background(), postalCommand(t, "eval-cat-1", "90210"))
	if err != nil || result.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
	}
	evaluation, _ := result.Evaluation()
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %q issues = %#v explanation = %#v", evaluation.Status(), evaluation.Issues(), evaluation.Explanation())
	}
	if fixture.inForce.calls != 1 || !fixture.inForce.lastAt.Equal(formedAt) || fixture.inForce.lastKind != domain.CatalogueKindZone || fixture.inForce.lastID != "SYN-CAT-ZONE" {
		t.Fatalf("in-force asked wrongly: calls=%d at=%s kind=%s id=%s", fixture.inForce.calls, fixture.inForce.lastAt, fixture.inForce.lastKind, fixture.inForce.lastID)
	}
	if !fixture.register.lastAsOf.Equal(time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)) || fixture.register.lastRoute.Destination() != "90210" {
		t.Fatalf("reading resolved at %s for %s, want the pricing basis time and the input's destination", fixture.register.lastAsOf, fixture.register.lastRoute.Destination())
	}
	frozen := false
	for _, reference := range evaluation.Manifest().References() {
		if reference.Kind() == domain.ArtifactReferenceCatalogue && reference.Version() == "v2" {
			frozen = true
		}
	}
	if !frozen {
		t.Fatalf("manifest = %#v, want SYN-CAT-ZONE@v2 frozen in", evaluation.Manifest().References())
	}
}

// Covers: ADR-0109 Decision 四「查不到即待判断，不给默认」——在用版本里没有该邮编，读数照样冻结（查过
// 哪一版），评价落 ZONE_UNRESOLVED；无在用版本时留说明，同样待判断。
func TestUnresolvableZoneLeavesTheEvaluationPending(t *testing.T) {
	notFound := newCatalogueFixture(t)
	result, err := notFound.handler.Handle(context.Background(), postalCommand(t, "eval-cat-miss", "33101"))
	if err != nil || result.Outcome() != application.EvaluationRecorded {
		t.Fatalf("outcome = %q err = %v", result.Outcome(), err)
	}
	evaluation, _ := result.Evaluation()
	if evaluation.Status() != domain.EvaluationPending || evaluation.Issues()[0].Code() != "ZONE_UNRESOLVED" {
		t.Fatalf("status = %q issues = %#v", evaluation.Status(), evaluation.Issues())
	}
	readings := evaluation.Input().CatalogueReadings()
	if len(readings) != 1 || readings[0].Reference().Version() != "v2" {
		t.Fatalf("the consulted version was not frozen: %#v", readings)
	}

	for label, tc := range map[string]struct {
		outcome ports.CatalogueInForceOutcome
		wants   string
	}{
		"无已登记版本": {ports.CatalogueHasNoRegisteredVersion, "no registered version"},
		"有版本未复核": {ports.CatalogueHasNoApprovedVersion, "none approved by review"},
		"种类不合":   {ports.CatalogueKindDisagrees, "another kind"},
	} {
		fixture := newCatalogueFixture(t)
		fixture.inForce.outcome = tc.outcome
		result, err := fixture.handler.Handle(context.Background(), postalCommand(t, "eval-cat-"+label, "90210"))
		if err != nil {
			t.Fatalf("%s: %v", label, err)
		}
		evaluation, _ := result.Evaluation()
		if evaluation.Status() != domain.EvaluationPending || evaluation.Issues()[0].Code() != "ZONE_UNRESOLVED" {
			t.Fatalf("%s: status = %q issues = %#v", label, evaluation.Status(), evaluation.Issues())
		}
		if len(result.SeriesResolutionNotes()) != 1 || !strings.Contains(result.SeriesResolutionNotes()[0], tc.wants) {
			t.Fatalf("%s: notes = %#v", label, result.SeriesResolutionNotes())
		}
	}
}

// Covers: 依赖故障是未决不是待判断；半套装配在受理时拒；输入自带读数不再解析。
func TestCatalogueResolutionBoundaries(t *testing.T) {
	broken := newCatalogueFixture(t)
	broken.inForce.err = errors.New("register unreachable")
	result, err := broken.handler.Handle(context.Background(), postalCommand(t, "eval-cat-broken", "90210"))
	if err != nil || result.Outcome() != application.EvaluationUndecided || broken.store.saved != 0 {
		t.Fatalf("outcome = %q err = %v saved = %d, want UNDECIDED and nothing saved", result.Outcome(), err, broken.store.saved)
	}

	half := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:            &evaluationStoreDouble{byID: map[string]domain.PricingEvaluation{}},
		Downstream:       &evaluationDownstreamDouble{},
		Clock:            fixedClock{at: formedAt},
		CatalogueInForce: &catalogueInForceDouble{},
	})
	if _, err := half.Handle(context.Background(), postalCommand(t, "eval-cat-half", "90210")); !errors.Is(err, application.ErrCatalogueResolutionHalfWired) {
		t.Fatalf("half-wired handler err = %v, want ErrCatalogueResolutionHalfWired", err)
	}

	supplied := newCatalogueFixture(t)
	command := postalCommand(t, "eval-cat-supplied", "90210")
	reading, err := domain.NewResolvedCatalogueValue(domain.CatalogueKindZone, catalogueVersion(t, "v9"), "Z1")
	if err != nil {
		t.Fatalf("reading: %v", err)
	}
	input, err := command.Request.Input().WithReferenceCatalogues(reading)
	if err != nil {
		t.Fatalf("attach reading: %v", err)
	}
	command.Request, err = domain.NewEvaluationRequest(command.Request.ID(), command.Request.Plan(), input, command.Request.Evidence())
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	if _, err := supplied.handler.Handle(context.Background(), command); err != nil {
		t.Fatalf("handle: %v", err)
	}
	if supplied.inForce.calls != 0 {
		t.Fatal("a request that already carries the reading was resolved again")
	}
}
