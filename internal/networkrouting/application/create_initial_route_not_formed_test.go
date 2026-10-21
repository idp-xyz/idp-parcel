package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件钉初始路由按恢复动作分格（ADR-0273 决定二、ADR-0029）：重投推不动的各格落`路由未形成`、原因与对象落未形成登记，
// 同一交接里判成的包裹照常提交；重投推得动的照旧未决；取数侧答用不了响亮报错。

// costsAnswering 交回固定的事实或错误，不看证据。
type costsAnswering struct {
	facts []domain.CandidateCostFact
	err   error
}

func (costs costsAnswering) LoadCandidateCosts(
	context.Context, domain.InitialRouteJudgmentKey, ports.InitialRouteEvidence, ports.RequestCarriedContent,
) (ports.RouteCandidateCosts, error) {
	return ports.RouteCandidateCosts{Facts: costs.facts}, costs.err
}

func withCosts(fixture *routeFixture, costs ports.RouteCandidateCostSource) {
	fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: fixture.applicability,
		Evidence:      fixture.evidence,
		Costs:         costs,
		Store:         fixture.store,
		NotFormed:     fixture.notFormed,
		Log:           fixture.log,
		Downstream:    fixture.downstream,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: routeJudgedAt},
	})
}

func pendingCandidateCost(t *testing.T, candidate string) domain.CandidateCostFact {
	t.Helper()
	fact, err := domain.NewPendingCandidateCost(value(t, domain.NewCandidateID, candidate))
	if err != nil {
		t.Fatalf("new pending candidate cost: %v", err)
	}
	return fact
}

func unpriceableCandidateCost(t *testing.T, candidate string) domain.CandidateCostFact {
	t.Helper()
	fact, err := domain.NewUnpriceableCandidateCost(value(t, domain.NewCandidateID, candidate))
	if err != nil {
		t.Fatalf("new unpriceable candidate cost: %v", err)
	}
	return fact
}

// Covers: `AT-NR-088`——重投推不动的各格落`路由未形成`：原因落未形成登记、不落判断库、不交意图；同一交接里判成的那件
// 照常形成计划、交意图，不被它连坐。各格逐一走一遍，原因码对照 ports.InitialRouteNotFormedReason。
func TestEachSettledCellIsRecordedAndSparesItsSibling(t *testing.T) {
	cells := map[ports.InitialRouteNotFormedReason]func(t *testing.T, fixture *routeFixture){
		ports.NotFormedEvidenceNotConfigured: func(t *testing.T, fixture *routeFixture) {
			fixture.evidence.unconfigured["parcel-2"] = true
		},
		ports.NotFormedCandidateSpaceNotEstablished: func(t *testing.T, fixture *routeFixture) {
			evidence := routableEvidence(t)
			evidence.ServiceAreas = nil
			fixture.evidence.byParcel["parcel-2"] = evidence
		},
		ports.NotFormedRankingFormNotConfigured: func(t *testing.T, fixture *routeFixture) {
			evidence := routableEvidence(t)
			evidence.RankingForm = domain.RankingFormUndeclared
			fixture.evidence.byParcel["parcel-2"] = evidence
		},
		ports.NotFormedCandidateCostsPending: func(t *testing.T, fixture *routeFixture) {
			evidence := routableEvidence(t)
			evidence.CandidateCosts = []domain.CandidateCostFact{pendingCandidateCost(t, "candidate-1")}
			fixture.evidence.byParcel["parcel-2"] = evidence
		},
		ports.NotFormedCandidateCostsUnpriceable: func(t *testing.T, fixture *routeFixture) {
			evidence := routableEvidence(t)
			evidence.CandidateCosts = []domain.CandidateCostFact{unpriceableCandidateCost(t, "candidate-1")}
			fixture.evidence.byParcel["parcel-2"] = evidence
		},
		ports.NotFormedCandidateCostCurrenciesDiffer: func(t *testing.T, fixture *routeFixture) {
			evidence := cheaperAlternative(t, 400)
			evidence.CandidateCosts = []domain.CandidateCostFact{
				pricedCandidateCost(t, "candidate-1", 1000, "CNY"),
				pricedCandidateCost(t, "candidate-2", 400, "SGD"),
			}
			fixture.evidence.byParcel["parcel-2"] = evidence
		},
		ports.NotFormedCandidatesTied: func(t *testing.T, fixture *routeFixture) {
			fixture.evidence.byParcel["parcel-2"] = tiedEvidence(t)
		},
	}
	for reason, arrange := range cells {
		t.Run(reason.String(), func(t *testing.T) {
			fixture := newRouteFixture(t)
			fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
			arrange(t, fixture)

			result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1", "parcel-2"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			parcels := result.Parcels()
			if result.Outcome() != application.RouteHandoffProcessed || len(parcels) != 2 {
				t.Fatalf("outcome = %q、包裹 %d 件, want PROCESSED、两件", result.Outcome(), len(parcels))
			}
			if parcels[0].Outcome() != application.ParcelRouteFormed {
				t.Fatalf("parcel-1 = %q, want ROUTE_FORMED——兄弟包裹不连坐", parcels[0].Outcome())
			}
			if parcels[1].Outcome() != application.ParcelRouteNotFormed || parcels[1].NotFormedReason() != reason {
				t.Fatalf("parcel-2 = %q/%q, want NOT_FORMED/%s", parcels[1].Outcome(), parcels[1].NotFormedReason(), reason)
			}
			if parcels[1].ContinuationReference().String() == "" {
				t.Fatal("未形成没有续办引用")
			}
			if record := fixture.notFormed.only(t); record.Reason != reason ||
				record.Key.DeclaredParcelID.String() != "parcel-2" {
				t.Fatalf("未形成登记 = %+v, want parcel-2/%s", record, reason)
			}
			if fixture.store.saved != 1 || len(fixture.downstream.intents) != 1 {
				t.Fatalf("判断库写 %d 次、意图 %d 份, want 各 1（只有判成的那件）", fixture.store.saved, len(fixture.downstream.intents))
			}
		})
	}
}

// Covers: `AT-NR-088` 的「取数侧没接」一格——装配没给取数侧与取数侧答整条取数路径没接同落`COST_SOURCE_NOT_CONFIGURED`，
// 原因落登记。
func TestAMissingCostSourceIsRecordedAsNotFormed(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: fixture.applicability,
		Evidence:      fixture.evidence,
		Store:         fixture.store,
		NotFormed:     fixture.notFormed,
		Log:           fixture.log,
		Downstream:    fixture.downstream,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: routeJudgedAt},
	})
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)

	result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if parcel := result.Parcels()[0]; parcel.Outcome() != application.ParcelRouteNotFormed ||
		parcel.NotFormedReason() != ports.NotFormedCostSourceNotConfigured {
		t.Fatalf("包裹 = %q/%q, want NOT_FORMED/COST_SOURCE_NOT_CONFIGURED", parcel.Outcome(), parcel.NotFormedReason())
	}
	if record := fixture.notFormed.only(t); record.Reason != ports.NotFormedCostSourceNotConfigured {
		t.Fatalf("未形成登记原因 = %s", record.Reason)
	}
}

// Covers: `AT-NR-091`——重投推得动的照旧是未决、未形成登记不留行：证据读不回、取号依赖故障、成本读不回、证据连续换代。
// 原因写不进未形成登记时同样改答未决（原因没落库不得入账）。
func TestDependencyFailuresStayUndecidedAndLeaveNoNotFormedRow(t *testing.T) {
	cases := map[application.RouteUndecidedReason]func(t *testing.T, fixture *routeFixture){
		application.RouteEvidenceUnavailable: func(t *testing.T, fixture *routeFixture) {
			fixture.evidence.errs["parcel-1"] = errors.New("evidence down")
		},
		application.RouteCostSourceUnavailable: func(t *testing.T, fixture *routeFixture) {
			fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
			withCosts(fixture, costsAnswering{err: errors.New("pricing connection reset")})
		},
		application.RouteEvidenceSuperseded: func(t *testing.T, fixture *routeFixture) {
			second := routableEvidence(t)
			second.ViewRevision = value(t, domain.NewNetworkViewRevision, "net-view-rev-2")
			third := routableEvidence(t)
			third.ViewRevision = value(t, domain.NewNetworkViewRevision, "net-view-rev-3")
			fixture.evidence.sequence = map[string][]ports.InitialRouteEvidence{
				"parcel-1": {routableEvidence(t), second, third},
			}
		},
		application.RouteNotFormedUnrecorded: func(t *testing.T, fixture *routeFixture) {
			fixture.evidence.unconfigured["parcel-1"] = true
			fixture.notFormed.err = errors.New("register write failed")
		},
	}
	for reason, arrange := range cases {
		t.Run(reason.String(), func(t *testing.T) {
			fixture := newRouteFixture(t)
			arrange(t, fixture)

			result, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if parcel := result.Parcels()[0]; parcel.Outcome() != application.ParcelRouteUndecided ||
				parcel.UndecidedReason() != reason {
				t.Fatalf("包裹 = %q/%q, want UNDECIDED/%s", parcel.Outcome(), parcel.UndecidedReason(), reason)
			}
			if fixture.notFormed.writes != 0 || len(fixture.notFormed.records) != 0 {
				t.Fatalf("未形成登记写了 %d 次, want 0——依赖类未决整封重投，不留原因", fixture.notFormed.writes)
			}
		})
	}
}

// Covers: `AT-NR-092` 初始路由那一半——取数侧答用不了（ports.ErrRouteCostAnswerUnusable）重投多少次都一样，又不是等登记的
// 缺口：响亮报错，不答 COST_SOURCE_UNAVAILABLE 让消费门回滚重投，也不落未形成登记。
func TestAnUnusableCostAnswerIsLoud(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.evidence.byParcel["parcel-1"] = routableEvidence(t)
	withCosts(fixture, costsAnswering{err: fmt.Errorf("%w: candidate carries no line reference", ports.ErrRouteCostAnswerUnusable)})

	_, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1"))
	if !errors.Is(err, ports.ErrRouteCostAnswerUnusable) {
		t.Fatalf("err = %v, want ErrRouteCostAnswerUnusable 响亮上抛", err)
	}
	if fixture.store.saved != 0 || len(fixture.notFormed.records) != 0 || len(fixture.downstream.intents) != 0 {
		t.Fatalf("响亮报错却写了判断库 %d 次、未形成 %d 行、意图 %d 份",
			fixture.store.saved, len(fixture.notFormed.records), len(fixture.downstream.intents))
	}
}

// Covers: 包裹停在重投推不动的一格、装配却没给未形成登记——只有装配缺漏到得了，响亮报错，不折成未决占着分区重投。
func TestAMissingNotFormedRegisterIsLoud(t *testing.T) {
	fixture := newRouteFixture(t)
	fixture.handler = application.NewCreateInitialRouteHandler(application.CreateInitialRouteDeps{
		Applicability: fixture.applicability,
		Evidence:      fixture.evidence,
		Costs:         passthroughCosts{},
		Store:         fixture.store,
		Log:           fixture.log,
		Downstream:    fixture.downstream,
		Identities:    &routeIdentityDouble{},
		Clock:         fixedClock{at: routeJudgedAt},
	})
	fixture.evidence.unconfigured["parcel-1"] = true

	if _, err := fixture.handler.Handle(context.Background(), routeCommand(t, "parcel-1")); !errors.Is(err, application.ErrNotFormedRegisterMissing) {
		t.Fatalf("err = %v, want ErrNotFormedRegisterMissing", err)
	}
}
