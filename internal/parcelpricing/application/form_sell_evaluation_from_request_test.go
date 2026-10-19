package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/ports"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/pptest"
)

// 本文件钉 ADR-0190 决定四在应用入口上的那一半：SELL 请求点名包裹而不点名发生项，复用同一只「由请求形成评价」编排——
// 选卡按（租户、计价范围、SELL、客户费用、计价基准时点），按回指幂等、入册、交付都照旧；结算币种随查询交给输入读口。
// 选卡口径与受理前试算同一条（ADR-0179 决定三），所以未配置、适用冲突各自停，不挑卡。

var sellBasisAt = time.Date(2026, 8, 9, 6, 30, 0, 0, time.UTC)

// sellPlan 造一张 SELL·CUSTOMER_CHARGE 的合成价卡（S 级），卡币种与请求的结算币种同为 CNY，评价不必换算。
func sellPlan(t *testing.T) domain.PricingPlanVersion {
	t.Helper()
	return pptest.Plan(t, pptest.PlanSpec{
		Reference:             pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-SELL-PLAN", "v1"),
		TableReference:        pptest.IdentityReference(t, domain.ArtifactRateTable, "SYN-SELL-TABLE", "v1"),
		WeightPolicyReference: pptest.IdentityReference(t, domain.ArtifactWeightPolicy, "SYN-SELL-WEIGHT", "v1"),
		Scope:                 "scope-1",
		Direction:             domain.PricingDirectionSell,
		Purpose:               domain.PricingPurposeCustomerCharge,
		BaseChargeCode:        "BASE_FREIGHT",
		Currency:              "CNY",
		Period: pptest.Period{
			StartsAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
			EndsAt:   time.Date(2027, 1, 1, 0, 0, 0, 0, time.UTC),
		},
		RateEntryID:         "SYN-SELL-ENTRY",
		RateZone:            "Z1",
		MinimumKilograms:    "0",
		MaximumKilograms:    "10",
		RateAmount:          "55",
		WeightRounding:      domain.RoundingCeiling,
		WeightStepKilograms: "0.5",
	})
}

func sellInput(t *testing.T) domain.PricingInputSnapshot {
	t.Helper()
	input := pptest.Input(t, pptest.InputSpec{
		Tenant:     "tenant-1",
		Scope:      "scope-1",
		PackageID:  "SYN-PARCEL-1",
		Zone:       "Z1",
		Kilograms:  "1.2",
		BusinessAt: sellBasisAt,
	})
	input, err := input.WithSettlementCurrency(mustValue(t, domain.NewCurrency, "CNY"))
	if err != nil {
		t.Fatalf("settlement currency: %v", err)
	}
	return input
}

func sellCommand(t *testing.T) application.FormEvaluationFromRequestCommand {
	t.Helper()
	return application.FormEvaluationFromRequestCommand{
		Tenant:     mustValue(t, domain.NewTenantID, "tenant-1"),
		Request:    mustValue(t, domain.NewEvaluationRequestReference, "SELLREQ-SYN-1"),
		Scope:      mustValue(t, domain.NewPricingScopeID, "scope-1"),
		Direction:  domain.PricingDirectionSell,
		Purpose:    domain.PricingPurposeCustomerCharge,
		BasisAt:    sellBasisAt,
		Sources:    ports.EligibleSourceReferences{Parcel: "SYN-PARCEL-1"},
		Settlement: mustValue(t, domain.NewCurrency, "CNY"),
		Evidence:   domain.EvidenceSynthetic,
	}
}

// selectionRecordingCards 记下每次选卡问的（方向、目的、时点）：SELL 请求必须按 SELL·客户费用选卡，选错一对会让人
// 去登一张本不该存在的卡。
type selectionRecordingCards struct {
	inner      *priceCardResolverDouble
	directions []domain.PricingDirection
	purposes   []domain.PricingPurpose
}

func (double *selectionRecordingCards) ResolveInForce(
	ctx context.Context,
	tenant domain.TenantID,
	scope domain.PricingScopeID,
	direction domain.PricingDirection,
	purpose domain.PricingPurpose,
	at time.Time,
) (ports.PriceCardInForceResolution, error) {
	double.directions = append(double.directions, direction)
	double.purposes = append(double.purposes, purpose)
	return double.inner.ResolveInForce(ctx, tenant, scope, direction, purpose, at)
}

type sellFixture struct {
	*formFixture
	cards *selectionRecordingCards
}

// newSellFixture 装一套入口：价卡唯一命中那张 SELL 卡、输入读口答出带结算币种的快照。
func newSellFixture(t *testing.T) *sellFixture {
	t.Helper()
	base := newFormFixture(t, &inputResolverDouble{resolution: ports.PricingInputResolution{
		Outcome: ports.PricingInputResolved,
		Input:   sellInput(t),
	}})
	base.priceCards.resolution = ports.PriceCardInForceResolution{Outcome: ports.PriceCardVersionInForce, Plan: sellPlan(t)}
	cards := &selectionRecordingCards{inner: base.priceCards}
	evaluate := application.NewEvaluatePricingHandler(application.EvaluatePricingDeps{
		Store:      base.store,
		Downstream: base.downstream,
		Clock:      fixedClock{at: sellBasisAt.Add(time.Minute)},
	})
	handler, err := application.NewFormEvaluationFromRequestHandler(application.FormEvaluationFromRequestDeps{
		Evaluations: base.byRequest,
		PriceCards:  cards,
		Identity:    base.identity,
		Evaluate:    evaluate,
		Inputs:      base.inputs,
	})
	if err != nil {
		t.Fatalf("construct handler: %v", err)
	}
	base.handler = handler
	return &sellFixture{formFixture: base, cards: cards}
}

// Covers: 一份 SELL 请求形成一份评价并回指请求——卡按 SELL·客户费用在计价基准时点选、输入读口按包裹问且带上结算币种、
// 评价按结算币种成形、入册一份交出一封意图；同一请求再来一封按回指答已存在，不选卡、不造快照、不铸第二个标识。
func TestASellRequestFormsOneEvaluationThatBackReferencesTheRequest(t *testing.T) {
	fixture := newSellFixture(t)

	result, err := fixture.handler.Handle(context.Background(), sellCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome != application.RequestEvaluationFormed || !result.HasEvaluation {
		t.Fatalf("outcome = %s hasEvaluation=%v, want FORMED", result.Outcome, result.HasEvaluation)
	}
	evaluation := result.Evaluation
	if reference, has := evaluation.RequestReference(); !has || reference.String() != "SELLREQ-SYN-1" {
		t.Fatalf("evaluation back-reference = %q present=%v, want the SELL request", reference, has)
	}
	if evaluation.Direction() != domain.PricingDirectionSell || evaluation.Purpose() != domain.PricingPurposeCustomerCharge {
		t.Fatalf("evaluation = %s/%s, want SELL/CUSTOMER_CHARGE", evaluation.Direction(), evaluation.Purpose())
	}
	if evaluation.Status() != domain.EvaluationCompleted {
		t.Fatalf("status = %s issues=%v", evaluation.Status(), evaluation.Issues())
	}
	if total, ok := evaluation.Total(); !ok || total.Currency().String() != "CNY" {
		t.Fatalf("total = %v present=%v, want a CNY amount", total, ok)
	}
	if len(fixture.cards.directions) != 1 || fixture.cards.directions[0] != domain.PricingDirectionSell ||
		fixture.cards.purposes[0] != domain.PricingPurposeCustomerCharge || !fixture.priceCards.asked[0].Equal(sellBasisAt) {
		t.Fatalf("card selection = %v/%v at %v, want SELL/CUSTOMER_CHARGE at the pricing basis time",
			fixture.cards.directions, fixture.cards.purposes, fixture.priceCards.asked)
	}
	if len(fixture.inputs.queries) != 1 {
		t.Fatalf("input queries = %d, want 1", len(fixture.inputs.queries))
	}
	query := fixture.inputs.queries[0]
	settlement, declared := query.SettlementCurrency()
	if query.Sources.Parcel != "SYN-PARCEL-1" || query.Sources.OccurrenceReferenced() || !declared || settlement.String() != "CNY" ||
		!query.BasisAt.Equal(sellBasisAt) || query.Purpose != domain.PricingPurposeCustomerCharge {
		t.Fatalf("input query = %+v, want the parcel, the snapshotted settlement currency and the basis time", query)
	}
	if fixture.store.saved != 1 || len(fixture.downstream.intents) != 1 || fixture.identity.minted != 1 {
		t.Fatalf("saved=%d intents=%d minted=%d, want one of each", fixture.store.saved, len(fixture.downstream.intents), fixture.identity.minted)
	}

	fixture.byRequest.byReference["tenant-1/SELLREQ-SYN-1"] = evaluation
	again, err := fixture.handler.Handle(context.Background(), sellCommand(t))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if again.Outcome != application.RequestEvaluationExisting || again.Evaluation.ID() != evaluation.ID() {
		t.Fatalf("replay outcome = %s evaluation=%s, want EXISTING with the first evaluation", again.Outcome, again.Evaluation.ID())
	}
	if fixture.store.saved != 1 || fixture.identity.minted != 1 || len(fixture.cards.directions) != 1 || len(fixture.inputs.queries) != 1 {
		t.Fatalf("replay did work again: saved=%d minted=%d selections=%d inputs=%d",
			fixture.store.saved, fixture.identity.minted, len(fixture.cards.directions), len(fixture.inputs.queries))
	}
}

// Covers: SELL 请求停在形成之前的三格，各自零写入、零铸造——未配置（这一时点没有在用的 SELL·客户费用卡）、适用冲突
// （候选交人裁，不挑）、输入不可得（读口点名缺的事实，照转述）。
func TestASellRequestStopsBeforeFormationWhenNoCardManyCardsOrNoInput(t *testing.T) {
	conflicting := []domain.VersionReference{
		pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-SELL-PLAN", "v1"),
		pptest.IdentityReference(t, domain.ArtifactPricingPlan, "SYN-SELL-PLAN-B", "v1"),
	}
	for name, tc := range map[string]struct {
		arrange func(*sellFixture)
		outcome application.FormEvaluationFromRequestOutcome
		check   func(*testing.T, application.FormEvaluationFromRequestResult)
	}{
		"未配置": {
			arrange: func(fixture *sellFixture) {
				fixture.priceCards.resolution = ports.PriceCardInForceResolution{Outcome: ports.PriceCardNotConfigured}
			},
			outcome: application.RequestPriceCardNotConfigured,
		},
		"适用冲突": {
			arrange: func(fixture *sellFixture) {
				fixture.priceCards.resolution = ports.PriceCardInForceResolution{
					Outcome:    ports.PriceCardApplicabilityConflict,
					Candidates: conflicting,
				}
			},
			outcome: application.RequestPriceCardApplicabilityConflict,
			check: func(t *testing.T, result application.FormEvaluationFromRequestResult) {
				if len(result.Candidates) != len(conflicting) || !result.Candidates[1].SameIdentity(conflicting[1]) {
					t.Fatalf("candidates = %v", result.Candidates)
				}
			},
		},
		"输入不可得": {
			arrange: func(fixture *sellFixture) {
				fixture.inputs.resolution = ports.PricingInputResolution{
					Outcome: ports.PricingInputUnavailable,
					Missing: []string{"parcel-shipment: destination postal code is absent"},
				}
			},
			outcome: application.RequestPricingInputUnavailable,
			check: func(t *testing.T, result application.FormEvaluationFromRequestResult) {
				if len(result.Missing) != 1 || result.Missing[0] != "parcel-shipment: destination postal code is absent" {
					t.Fatalf("missing = %v", result.Missing)
				}
			},
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newSellFixture(t)
			tc.arrange(fixture)

			result, err := fixture.handler.Handle(context.Background(), sellCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != tc.outcome || result.HasEvaluation {
				t.Fatalf("outcome = %s hasEvaluation=%v, want %s", result.Outcome, result.HasEvaluation, tc.outcome)
			}
			if tc.check != nil {
				tc.check(t, result)
			}
			if fixture.identity.minted != 0 || fixture.store.saved != 0 || len(fixture.downstream.intents) != 0 {
				t.Fatalf("stopped outcome did work: minted=%d saved=%d intents=%d",
					fixture.identity.minted, fixture.store.saved, len(fixture.downstream.intents))
			}
		})
	}
}

// Covers: 受理门要发生项或包裹二者有一（ADR-0190 决定四）——只点名包裹的请求过门走到选卡；两者都没有、或包裹引用带
// 首尾空白，未受理且零依赖调用。
func TestTheGateTakesEitherAnOccurrenceOrAParcel(t *testing.T) {
	fixture := newSellFixture(t)
	if result, err := fixture.handler.Handle(context.Background(), sellCommand(t)); err != nil || result.Outcome == application.RequestEvaluationNotAccepted {
		t.Fatalf("a parcel-only request was refused: outcome=%s err=%v", result.Outcome, err)
	}

	for name, sources := range map[string]ports.EligibleSourceReferences{
		"两者都没有":  {},
		"包裹带空白":  {Parcel: " SYN-PARCEL-1"},
		"发生项缺版本": {Occurrence: "SYN-OCC-1"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newSellFixture(t)
			command := sellCommand(t)
			command.Sources = sources

			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome != application.RequestEvaluationNotAccepted {
				t.Fatalf("outcome = %s, want NOT_ACCEPTED", result.Outcome)
			}
			if fixture.byRequest.calls != 0 || len(fixture.cards.directions) != 0 || len(fixture.inputs.queries) != 0 {
				t.Fatal("a not-accepted command reached a dependency")
			}
		})
	}
}
