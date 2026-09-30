package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
)

func concurrencyBoundary(t *testing.T, at time.Time) domain.RerouteEffectBoundary {
	t.Helper()
	return domain.RerouteEffectBoundary{
		EffectiveAt: at,
		Plan:        mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		PlanNodes:   []string{"origin", "hub", "dest"},
	}
}

func concurrencyLoad(t *testing.T, at time.Time, node string) domain.LoadingFact {
	t.Helper()
	return domain.LoadingFact{
		OccurredAt: at,
		Node:       node,
		Plan:       mustValue(t, domain.NewRoutePlanVersionID, "plan-1/v1"),
		Causality:  domain.CausalityComparable,
	}
}

func TestLoadBeforeTheRerouteTakesEffectAtTheNextControllableNode(t *testing.T) {
	effective := time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)
	load := concurrencyLoad(t, effective.Add(-time.Hour), "origin")
	judgment, err := domain.ArbitrateLoadingConcurrency(concurrencyBoundary(t, effective), load)
	if err != nil {
		t.Fatalf("arbitrate: %v", err)
	}
	if judgment.Outcome() != domain.LoadOccurredFirst {
		t.Fatalf("outcome = %q", judgment.Outcome())
	}
	next, hasNext := judgment.NextControllableNode()
	if !hasNext || next != "hub" {
		t.Fatalf("next = %q has = %v", next, hasNext)
	}
	if judgment.RouteDeviationIndicated() {
		t.Fatal("装载先发生不应形成路由偏离")
	}
}

func TestRerouteEffectiveFirstKeepsTheLoadAndIndicatesDeviation(t *testing.T) {
	effective := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	load := concurrencyLoad(t, effective.Add(time.Hour), "hub")
	judgment, err := domain.ArbitrateLoadingConcurrency(concurrencyBoundary(t, effective), load)
	if err != nil {
		t.Fatalf("arbitrate: %v", err)
	}
	if judgment.Outcome() != domain.RerouteEffectiveFirst || !judgment.RouteDeviationIndicated() {
		t.Fatalf("outcome = %q deviation = %v", judgment.Outcome(), judgment.RouteDeviationIndicated())
	}
	if _, hasNext := judgment.NextControllableNode(); hasNext {
		t.Fatal("改路先有效不应改到下一个可控节点")
	}
}

func TestTheSameInstantDoesNotCollapseIntoEitherOrderedCell(t *testing.T) {
	at := time.Date(2026, 9, 30, 10, 0, 0, 0, time.UTC)
	judgment, err := domain.ArbitrateLoadingConcurrency(concurrencyBoundary(t, at), concurrencyLoad(t, at, "origin"))
	if err != nil {
		t.Fatalf("arbitrate: %v", err)
	}
	if judgment.Outcome() != domain.SameInstant {
		t.Fatalf("outcome = %q, want SAME_INSTANT", judgment.Outcome())
	}
	if _, hasNext := judgment.NextControllableNode(); hasNext || judgment.RouteDeviationIndicated() {
		t.Fatal("同一时刻并进了装载先或改路先")
	}
}

func TestUnknownCausalityStaysUnclearEvenWhenTheLoadTimeIsEarlier(t *testing.T) {
	effective := time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)
	load := concurrencyLoad(t, effective.Add(-time.Hour), "origin")
	load.Causality = domain.CausalityUnknown
	judgment, err := domain.ArbitrateLoadingConcurrency(concurrencyBoundary(t, effective), load)
	if err != nil {
		t.Fatalf("arbitrate: %v", err)
	}
	if judgment.Outcome() != domain.CausalityUnclear {
		t.Fatalf("outcome = %q, want CAUSALITY_UNCLEAR", judgment.Outcome())
	}
	if _, hasNext := judgment.NextControllableNode(); hasNext || judgment.RouteDeviationIndicated() {
		t.Fatal("因果不明并进了装载先或改路先")
	}

	missing := concurrencyLoad(t, time.Time{}, "origin")
	judgment, err = domain.ArbitrateLoadingConcurrency(concurrencyBoundary(t, effective), missing)
	if err != nil {
		t.Fatalf("missing time: %v", err)
	}
	if judgment.Outcome() != domain.CausalityUnclear {
		t.Fatalf("zero time outcome = %q", judgment.Outcome())
	}
}

func TestAnUnstatedCausalityIsNotTreatedAsUnknown(t *testing.T) {
	effective := time.Date(2026, 9, 30, 11, 0, 0, 0, time.UTC)
	load := concurrencyLoad(t, effective.Add(-time.Hour), "origin")
	load.Causality = domain.CausalityStatementInvalid
	_, err := domain.ArbitrateLoadingConcurrency(concurrencyBoundary(t, effective), load)
	if !errors.Is(err, domain.ErrInvalidLoadingConcurrency) {
		t.Fatalf("err = %v", err)
	}
}
