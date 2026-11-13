package visibilityhttp_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	visibilityhttp "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/http"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 披露两口的转写测试用真编排、内存替身：DecideDisclosureResult 的各格不导出，带决定的答复只有编排造得出来。

type disclosureEpisodes struct {
	ports.SignalEpisodeStore
	episode *domain.SignalEpisode
}

func (store disclosureEpisodes) FindLatest(_ context.Context, _ domain.TenantID, parcel domain.TrackedParcelReference, kind domain.ExceptionSignalKindReference) (*domain.SignalEpisode, bool, error) {
	snapshot := store.episode.Snapshot()
	if snapshot.Parcel != parcel || snapshot.Kind != kind {
		return nil, false, nil
	}
	return store.episode, true, nil
}

type disclosureCustomers struct{ found bool }

func (customers *disclosureCustomers) FindCustomerAccount(context.Context, domain.TenantID, domain.TrackedParcelReference) (domain.CustomerAccountReference, bool, error) {
	account, err := domain.NewCustomerAccountReference("customer-1")
	return account, customers.found, err
}

type disclosureRules struct {
	rule       ports.ExceptionDisclosureRule
	configured bool
	err        error
}

func (rules *disclosureRules) RuleForSignal(context.Context, domain.CustomerAccountReference, domain.ExceptionSignalKindReference, domain.ConfidenceReference) (ports.ExceptionDisclosureRule, bool, error) {
	return rules.rule, rules.configured, rules.err
}

// disclosureBook 是决定册的内存替身：按规则形成的至多一份，授权确认追加一份并成为当前。
type disclosureBook struct{ decisions []domain.DisclosureDecision }

func (book *disclosureBook) FindCurrent(context.Context, domain.TenantID, domain.EpisodeID, domain.CustomerAccountReference) (domain.DisclosureDecision, bool, error) {
	if len(book.decisions) == 0 {
		return domain.DisclosureDecision{}, false, nil
	}
	return book.decisions[len(book.decisions)-1], true, nil
}

func (book *disclosureBook) Save(_ context.Context, _ domain.TenantID, decision domain.DisclosureDecision) (ports.DisclosureDecisionSaveOutcome, error) {
	if len(book.decisions) != 0 {
		return ports.DisclosureDecisionAlreadyRecorded, nil
	}
	book.decisions = append(book.decisions, decision)
	return ports.DisclosureDecisionSaved, nil
}

func (book *disclosureBook) RecordDecider(context.Context, domain.TenantID, domain.DisclosureDecision, string) error {
	return nil
}

func (book *disclosureBook) SaveAuthorization(_ context.Context, authorization ports.DisclosureAuthorization) (ports.DisclosureDecisionSaveOutcome, error) {
	book.decisions = append(book.decisions, authorization.Decision)
	return ports.DisclosureDecisionSaved, nil
}

type disclosureClock struct{ at time.Time }

func (clock *disclosureClock) Now() time.Time { return clock.at }

type disclosureOrchestration struct {
	decide, authorize http.Handler
	rules             *disclosureRules
	customers         *disclosureCustomers
	clock             *disclosureClock
}

func must[T any](t *testing.T, build func(string) (T, error), raw string) T {
	t.Helper()
	value, err := build(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
}

// newDisclosureOrchestration 预置 parcel-1 上一期 ETA_BREACH_RISK 信号、可反查的客户，以及一条「披露但未批准自动
// 发布」的规则——首发默认口径是逐条由授权角色确认。
func newDisclosureOrchestration(t *testing.T) *disclosureOrchestration {
	t.Helper()
	fact := must(t, domain.NewSourceFactReference, "movement-fact/parcel-1/MF-1")
	cause, err := domain.SourceFactCausalCondition(domain.SourceTransportFulfillment, fact)
	if err != nil {
		t.Fatal(err)
	}
	episode, err := domain.OpenEpisode(must(t, domain.NewEpisodeID, "episode-1"), domain.SignalHit{
		Kind:       must(t, domain.NewExceptionSignalKindReference, "ETA_BREACH_RISK"),
		Parcel:     must(t, domain.NewTrackedParcelReference, "parcel-1"),
		Cause:      cause,
		Rule:       must(t, domain.NewSignalRuleVersionReference, "signal-rules/v3"),
		Confidence: must(t, domain.NewConfidenceReference, "HIGH/route-deviation"),
		Basis: []domain.SignalFactBasis{{
			Source: domain.SourceTransportFulfillment, Fact: fact, Version: must(t, domain.NewSourceFactVersion, "v1"),
		}},
		At: time.Date(2026, 11, 13, 1, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &disclosureOrchestration{
		rules: &disclosureRules{configured: true, rule: ports.ExceptionDisclosureRule{
			Policy:      must(t, domain.NewDisclosurePolicyReference, "exception-disclosure/v1"),
			Disclosable: true,
			Content:     must(t, domain.NewDisclosureContentReference, "disclosure-content/eta-breach/v1"),
		}},
		customers: &disclosureCustomers{found: true},
		clock:     &disclosureClock{at: time.Date(2026, 11, 13, 2, 0, 0, 0, time.UTC)},
	}
	book := &disclosureBook{}
	handler := application.NewDecideDisclosureHandler(application.DecideDisclosureDeps{
		Episodes: disclosureEpisodes{episode: episode}, Customers: fixture.customers, Rules: fixture.rules,
		Decisions: book, Ledger: book, Clock: fixture.clock,
	})
	intake, err := visibilityhttp.NewOperatorDecisionIntake(caseDecisionAuthenticator{})
	if err != nil {
		t.Fatal(err)
	}
	fixture.decide = visibilityhttp.NewDecideCustomerDisclosureEndpoint(intake, handler)
	fixture.authorize = visibilityhttp.NewAuthorizeCustomerDisclosureEndpoint(intake, handler)
	return fixture
}
