package application_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
)

// 本文件证「按委托提交版本形成受理前试算」的入口（票 product-strategy-boundary/25「PP 受理前试算入口」；ADR-0179 决定三、四）：
// 按（租户、计价范围、SELL、客户费用、判断时点）解析唯一在用价卡，逐件用提交版本上的申报造声明，交给定价卡的试算入口；
// 未配置、适用冲突、缺件各自成格，不挑卡、不顶值。

// preAcceptanceJudgedAt 是控制回显的判断时点，刻意与补齐读数的时钟不同：选卡与计价基准都按它，不按时钟。
var preAcceptanceJudgedAt = time.Date(2026, 8, 7, 10, 0, 0, 0, time.UTC)

type preAcceptanceCardDouble struct {
	resolution    ports.PriceCardInForceResolution
	err           error
	calls         int
	lastTenant    domain.TenantID
	lastScope     domain.PricingScopeID
	lastDirection domain.PricingDirection
	lastPurpose   domain.PricingPurpose
	lastAt        time.Time
}

func (double *preAcceptanceCardDouble) ResolveInForce(
	_ context.Context,
	tenant domain.TenantID,
	scope domain.PricingScopeID,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
	at time.Time,
) (ports.PriceCardInForceResolution, error) {
	double.calls++
	double.lastTenant, double.lastScope, double.lastDirection, double.lastPurpose, double.lastAt = tenant, scope, direction, purpose, at
	if double.err != nil {
		return ports.PriceCardInForceResolution{}, double.err
	}
	return double.resolution, nil
}

type submissionDeclarationDouble struct {
	declaration ports.SubmissionDeclaration
	found       bool
	err         error
	calls       int
	lastTenant  domain.TenantID
	lastRequest string
	lastVersion string
}

func (double *submissionDeclarationDouble) LoadSubmissionDeclaration(
	_ context.Context,
	tenant domain.TenantID,
	shipmentRequest string,
	submissionVersion string,
) (ports.SubmissionDeclaration, bool, error) {
	double.calls++
	double.lastTenant, double.lastRequest, double.lastVersion = tenant, shipmentRequest, submissionVersion
	if double.err != nil {
		return ports.SubmissionDeclaration{}, false, double.err
	}
	return double.declaration, double.found, nil
}

func inForceCard(plan domain.PricingPlanVersion) ports.PriceCardInForceResolution {
	return ports.PriceCardInForceResolution{Outcome: ports.PriceCardVersionInForce, Plan: plan}
}

// measuredParcel 是一件 1 千克、20 × 15 × 10 厘米的申报。
func measuredParcel(t *testing.T, parcel string) ports.DeclaredParcel {
	t.Helper()
	weight := mustWeight(t, "1")
	sides := planGivenSides(t)
	return ports.DeclaredParcel{Parcel: parcel, Weight: &weight, Dimensions: &sides}
}

func routedDeclaration(t *testing.T, parcels ...ports.DeclaredParcel) ports.SubmissionDeclaration {
	t.Helper()
	route, err := domain.NewPostalRoute("200000", "90210")
	if err != nil {
		t.Fatalf("route: %v", err)
	}
	return ports.SubmissionDeclaration{Parcels: parcels, Route: &route}
}

func preAcceptanceCommand(t *testing.T) application.FormPreAcceptanceEstimatesCommand {
	t.Helper()
	return application.FormPreAcceptanceEstimatesCommand{
		Tenant:            mustValue(t, domain.NewTenantID, "tenant-1"),
		Scope:             mustValue(t, domain.NewPricingScopeID, "scope-1"),
		ShipmentRequest:   "SHR-SYN-01",
		SubmissionVersion: "SV-SYN-01",
		At:                preAcceptanceJudgedAt,
		Settlement:        mustValue(t, domain.NewCurrency, "USD"),
	}
}

type preAcceptanceFixture struct {
	handler      *application.FormPreAcceptanceEstimatesHandler
	cards        *preAcceptanceCardDouble
	declarations *submissionDeclarationDouble
}

func newPreAcceptanceFixture(t *testing.T, card ports.PriceCardInForceResolution, declaration ports.SubmissionDeclaration) *preAcceptanceFixture {
	t.Helper()
	fixture := &preAcceptanceFixture{
		cards:        &preAcceptanceCardDouble{resolution: card},
		declarations: &submissionDeclarationDouble{declaration: declaration, found: true},
	}
	estimates := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{
		CatalogueInForce: &catalogueInForceDouble{reference: catalogueVersion(t, "v2"), outcome: ports.CatalogueVersionInForce},
		Catalogues:       &catalogueReadingDouble{registrations: map[string]domain.ReferenceCatalogueRegistration{"v2": zoneChartVersion(t, "v2")}},
	})
	handler, err := application.NewFormPreAcceptanceEstimatesHandler(application.FormPreAcceptanceEstimatesDeps{
		PriceCards:   fixture.cards,
		Declarations: fixture.declarations,
		Estimates:    estimates,
		Evidence:     domain.EvidenceSynthetic,
	})
	if err != nil {
		t.Fatalf("handler: %v", err)
	}
	fixture.handler = handler
	return fixture
}

// 选卡与正式 SELL·客户费用评价同口径（决定三）：按（租户、范围、SELL、客户费用、判断时点）问在用解析口一次；读申报按（租户、委托、
// 提交版本）；绑分区目录的卡由邮编路线解出分区，逐件形成试算评价、与申报同序，交回所用价卡的版本引用。证据层级随装配、结算币种随
// 命令进每一份评价。
func TestPreAcceptanceEstimatesResolveTheSellCustomerChargeCardAtTheJudgmentTimeAndEstimateEachParcel(t *testing.T) {
	plan := catalogueBoundPlan(t)
	fixture := newPreAcceptanceFixture(t, inForceCard(plan),
		routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1"), measuredParcel(t, "SHR-SYN-01/2")))

	result, err := fixture.handler.Handle(context.Background(), preAcceptanceCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.PreAcceptanceEstimatesFormed || len(result.Estimates) != 2 {
		t.Fatalf("result = %+v, 想要已形成且两件", result)
	}
	if result.Plan != plan.Reference() {
		t.Fatalf("plan = %+v, 想要交回所用价卡 %+v", result.Plan, plan.Reference())
	}
	cards := fixture.cards
	if cards.calls != 1 || cards.lastTenant.String() != "tenant-1" || cards.lastScope.String() != "scope-1" ||
		cards.lastDirection != domain.PricingDirectionSell || cards.lastPurpose != domain.PricingPurposeCustomerCharge ||
		!cards.lastAt.Equal(preAcceptanceJudgedAt) {
		t.Fatalf("在用解析被问 %d 次，问的是 %s %s %s %s @%s，想要按判断时点问一次 SELL·客户费用", cards.calls,
			cards.lastTenant, cards.lastScope, cards.lastDirection, cards.lastPurpose, cards.lastAt)
	}
	declarations := fixture.declarations
	if declarations.calls != 1 || declarations.lastTenant.String() != "tenant-1" ||
		declarations.lastRequest != "SHR-SYN-01" || declarations.lastVersion != "SV-SYN-01" {
		t.Fatalf("申报读口被问 %d 次（%s %s %s），想要按租户、委托与提交版本问一次", declarations.calls,
			declarations.lastTenant, declarations.lastRequest, declarations.lastVersion)
	}
	subjects := map[string]struct{}{}
	for index, want := range []string{"SHR-SYN-01/1", "SHR-SYN-01/2"} {
		estimate := result.Estimates[index]
		evaluation := estimate.Evaluation
		if estimate.Parcel != want || estimate.Answer != application.EstimateCandidateEvaluated ||
			evaluation.Status() != domain.EvaluationCompleted {
			t.Fatalf("第 %d 件 = %+v issues = %+v, 想要 %s 经目录解出分区后完成", index+1, estimate, evaluation.Issues(), want)
		}
		subject, present := evaluation.Input().Subject()
		if !present || subject.Kind() != domain.SubjectEstimate {
			t.Fatalf("第 %d 件的评价对象 = %+v, 想要试算对象", index+1, subject)
		}
		subjects[subject.Reference()] = struct{}{}
		if settlement, declared := evaluation.Input().SettlementCurrency(); !declared || settlement.String() != "USD" {
			t.Fatalf("第 %d 件的结算币种 = %s（声明 %t）, 想要命令交入的 USD", index+1, settlement, declared)
		}
		if evaluation.Evidence() != domain.EvidenceSynthetic {
			t.Fatalf("第 %d 件的证据层级 = %s, 想要装配声明的 S", index+1, evaluation.Evidence())
		}
		if !evaluation.Input().BusinessAt().Equal(preAcceptanceJudgedAt) {
			t.Fatalf("第 %d 件的计价基准时点 = %s, 想要判断时点", index+1, evaluation.Input().BusinessAt())
		}
	}
	if len(subjects) != 2 {
		t.Fatalf("试算对象 = %v, 想要两件各一个", subjects)
	}
}

// 同一委托同一版本重算得同一组评价标识；换一版本，标识随之不同——留痕按标识与版本清单复算（决定六）。
func TestPreAcceptanceEstimatesAreReproducibleForTheSameSubmissionVersion(t *testing.T) {
	declaration := routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1"))
	identities := func(version string) string {
		fixture := newPreAcceptanceFixture(t, inForceCard(catalogueBoundPlan(t)), declaration)
		command := preAcceptanceCommand(t)
		command.SubmissionVersion = version
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || result.Outcome != application.PreAcceptanceEstimatesFormed || len(result.Estimates) != 1 {
			t.Fatalf("result = %+v err = %v", result, err)
		}
		return result.Estimates[0].Evaluation.ID().String()
	}
	first, again, other := identities("SV-SYN-01"), identities("SV-SYN-01"), identities("SV-SYN-02")
	if first == "" || first != again {
		t.Fatalf("同一版本两次得 %q 与 %q, 想要同一标识", first, again)
	}
	if other == first {
		t.Fatalf("换版本仍得 %q, 想要另一标识", other)
	}
}

// 选卡那一步的三格各自停、不挑卡（决定三）：未配置与适用冲突照答，冲突交回候选；解析口读不回是未决。三格都不读客户的申报：卡都
// 没有，读了也是白读。
func TestPreAcceptanceEstimatesStopAtEachCardCellWithoutReadingTheDeclaration(t *testing.T) {
	conflicting := []domain.VersionReference{minimalPlan(t).Reference(), catalogueBoundPlan(t).Reference()}
	for name, tc := range map[string]struct {
		resolution ports.PriceCardInForceResolution
		err        error
		outcome    application.FormPreAcceptanceEstimatesOutcome
		reason     application.FormPreAcceptanceEstimatesUndecidedReason
		conflict   []domain.VersionReference
	}{
		"未配置": {
			resolution: ports.PriceCardInForceResolution{Outcome: ports.PriceCardNotConfigured},
			outcome:    application.PreAcceptancePriceCardNotConfigured,
		},
		"适用冲突": {
			resolution: ports.PriceCardInForceResolution{Outcome: ports.PriceCardApplicabilityConflict, Candidates: conflicting},
			outcome:    application.PreAcceptancePriceCardApplicabilityConflict,
			conflict:   conflicting,
		},
		"读不回": {
			err:     errors.New("price card register unreachable"),
			outcome: application.PreAcceptanceEstimatesUndecided,
			reason:  application.PreAcceptancePriceCardResolutionUnavailable,
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPreAcceptanceFixture(t, tc.resolution, routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1")))
			fixture.cards.err = tc.err

			result, err := fixture.handler.Handle(context.Background(), preAcceptanceCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != tc.outcome || result.Reason != tc.reason || len(result.Estimates) != 0 ||
				!slices.Equal(result.Conflict, tc.conflict) {
				t.Fatalf("result = %+v, 想要 %s / %s、候选 %+v、一件不算", result, tc.outcome, tc.reason, tc.conflict)
			}
			if fixture.declarations.calls != 0 {
				t.Fatalf("申报读口被问 %d 次, 想要选卡停下时一次都不问", fixture.declarations.calls)
			}
		})
	}
}

// 缺件自成一格（决定二、四）：没报测量的那一件点名测量、不拿同委托别件的测量顶，其余照算；没报邮编路线时绑分区目录的卡逐件点名
// 邮编路线；没绑分区目录的卡在本形态下没有调用方给的分区，逐件点名分区。逐件同序交回，整份答缺件而不是已形成。
func TestPreAcceptanceEstimatesNameEachMissingCellWithoutFillingIt(t *testing.T) {
	unmeasured := ports.DeclaredParcel{Parcel: "SHR-SYN-01/2"}
	unrouted := ports.SubmissionDeclaration{Parcels: []ports.DeclaredParcel{measuredParcel(t, "SHR-SYN-01/1")}}
	for name, tc := range map[string]struct {
		plan        domain.PricingPlanVersion
		declaration ports.SubmissionDeclaration
		want        [][]string
	}{
		"一件没报测量": {
			plan:        catalogueBoundPlan(t),
			declaration: routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1"), unmeasured),
			want:        [][]string{nil, {application.EstimateMissingMeasurement}},
		},
		"没报邮编": {
			plan:        catalogueBoundPlan(t),
			declaration: unrouted,
			want:        [][]string{{application.EstimateMissingPostalRoute}},
		},
		"卡没绑分区目录": {
			plan:        minimalPlan(t),
			declaration: routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1")),
			want:        [][]string{{application.EstimateMissingZone}},
		},
		"全都没报测量": {
			plan:        catalogueBoundPlan(t),
			declaration: routedDeclaration(t, ports.DeclaredParcel{Parcel: "SHR-SYN-01/1"}),
			want:        [][]string{{application.EstimateMissingMeasurement}},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPreAcceptanceFixture(t, inForceCard(tc.plan), tc.declaration)

			result, err := fixture.handler.Handle(context.Background(), preAcceptanceCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != application.PreAcceptanceDeclarationIncomplete || len(result.Estimates) != len(tc.want) {
				t.Fatalf("result = %+v, 想要缺件且 %d 件", result, len(tc.want))
			}
			for index, missing := range tc.want {
				estimate := result.Estimates[index]
				if estimate.Parcel != tc.declaration.Parcels[index].Parcel {
					t.Fatalf("第 %d 件 = %s, 想要与申报同序", index+1, estimate.Parcel)
				}
				if missing == nil {
					if estimate.Answer != application.EstimateCandidateEvaluated {
						t.Fatalf("第 %d 件 = %+v, 想要照算", index+1, estimate)
					}
					continue
				}
				if estimate.Answer != application.EstimateCandidateInputIncomplete || !slices.Equal(estimate.Missing, missing) {
					t.Fatalf("第 %d 件 = %+v, 想要输入不全、点名 %v", index+1, estimate, missing)
				}
			}
		})
	}
}

// 委托或版本不可见是自己的一格，不是一份零件的已形成；读口读不回是未决。
func TestPreAcceptanceEstimatesAnswerAnInvisibleSubmissionAndAnUnreachableReadSeparately(t *testing.T) {
	invisible := newPreAcceptanceFixture(t, inForceCard(catalogueBoundPlan(t)), ports.SubmissionDeclaration{})
	invisible.declarations.found = false
	result, err := invisible.handler.Handle(context.Background(), preAcceptanceCommand(t))
	if err != nil || result.Outcome != application.PreAcceptanceSubmissionNotVisible || len(result.Estimates) != 0 {
		t.Fatalf("不可见：result = %+v err = %v, 想要委托不可见", result, err)
	}

	unreachable := newPreAcceptanceFixture(t, inForceCard(catalogueBoundPlan(t)), ports.SubmissionDeclaration{})
	unreachable.declarations.err = errors.New("shipment read unreachable")
	result, err = unreachable.handler.Handle(context.Background(), preAcceptanceCommand(t))
	if err != nil || result.Outcome != application.PreAcceptanceEstimatesUndecided ||
		result.Reason != application.PreAcceptanceSubmissionDeclarationUnavailable {
		t.Fatalf("读不回：result = %+v err = %v, 想要未决、点名申报读口", result, err)
	}
}

// 受理门：租户、范围、委托、提交版本、判断时点与结算币种缺一即拒，一口都不问；结算币种由调用方交入、不给默认。
func TestPreAcceptanceEstimatesRefuseAnIncompleteCommandWithoutAsking(t *testing.T) {
	for name, mutate := range map[string]func(*application.FormPreAcceptanceEstimatesCommand){
		"缺租户":   func(command *application.FormPreAcceptanceEstimatesCommand) { command.Tenant = domain.TenantID{} },
		"缺范围":   func(command *application.FormPreAcceptanceEstimatesCommand) { command.Scope = domain.PricingScopeID{} },
		"缺委托":   func(command *application.FormPreAcceptanceEstimatesCommand) { command.ShipmentRequest = "" },
		"缺提交版本": func(command *application.FormPreAcceptanceEstimatesCommand) { command.SubmissionVersion = "" },
		"缺判断时点": func(command *application.FormPreAcceptanceEstimatesCommand) { command.At = time.Time{} },
		"缺结算币种": func(command *application.FormPreAcceptanceEstimatesCommand) { command.Settlement = domain.Currency{} },
		"委托带空白": func(command *application.FormPreAcceptanceEstimatesCommand) { command.ShipmentRequest = " SHR-SYN-01" },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPreAcceptanceFixture(t, inForceCard(catalogueBoundPlan(t)), routedDeclaration(t, measuredParcel(t, "SHR-SYN-01/1")))
			command := preAcceptanceCommand(t)
			mutate(&command)

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil || result.Outcome != application.PreAcceptanceEstimatesNotAccepted {
				t.Fatalf("result = %+v err = %v, 想要未受理", result, err)
			}
			if fixture.cards.calls != 0 || fixture.declarations.calls != 0 {
				t.Fatalf("在用解析 %d 次、申报读口 %d 次, 想要一口都不问", fixture.cards.calls, fixture.declarations.calls)
			}
		})
	}
}

// 构造门拒缺件与未声明的证据层级：证据层级随装配声明（决定四），不逐次给、不给默认。
func TestPreAcceptanceEstimatesHandlerRefusesMissingDependencies(t *testing.T) {
	estimates := newPlanGivenHandler(t, application.FormPlanGivenEstimateEvaluationsDeps{})
	for name, deps := range map[string]application.FormPreAcceptanceEstimatesDeps{
		"缺在用解析": {Declarations: &submissionDeclarationDouble{}, Estimates: estimates, Evidence: domain.EvidenceSynthetic},
		"缺申报读口": {PriceCards: &preAcceptanceCardDouble{}, Estimates: estimates, Evidence: domain.EvidenceSynthetic},
		"缺试算入口": {PriceCards: &preAcceptanceCardDouble{}, Declarations: &submissionDeclarationDouble{}, Evidence: domain.EvidenceSynthetic},
		"缺证据层级": {PriceCards: &preAcceptanceCardDouble{}, Declarations: &submissionDeclarationDouble{}, Estimates: estimates},
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := application.NewFormPreAcceptanceEstimatesHandler(deps); !errors.Is(err, application.ErrNilDependency) {
				t.Fatalf("err = %v, 想要 ErrNilDependency", err)
			}
		})
	}
}
