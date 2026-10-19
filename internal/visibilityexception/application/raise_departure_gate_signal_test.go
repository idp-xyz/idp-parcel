package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// 本文件证阻断读数派生异常信号的编排半边（票 demo-chain-gaps/33，ADR-0205 决定五）：信号三样全取登记，未配置如实不形成；
// 规则视图调不通与分诊未决都是搁置，交整封重放；命中时刻取判断时刻。分诊一侧用真的 RaiseSignalHandler 接替身端口。

type departureGateRuleDouble struct {
	rule       ports.DepartureGateSignalRule
	configured bool
	err        error
}

func (double *departureGateRuleDouble) DepartureGateSignalRule(context.Context) (ports.DepartureGateSignalRule, bool, error) {
	return double.rule, double.configured, double.err
}

var departureGateJudgedAt = time.Date(2026, 10, 21, 15, 0, 0, 0, time.UTC)

func newDepartureGateSignalFixture(t *testing.T) (*application.RaiseDepartureGateSignalHandler, *departureGateRuleDouble, *raiseFixture) {
	t.Helper()
	raise := newRaiseFixture(t)
	rules := &departureGateRuleDouble{
		rule: ports.DepartureGateSignalRule{
			Kind:       mustValue(t, domain.NewExceptionSignalKindReference, "DEPARTURE_GATE_BLOCKED"),
			Rule:       mustValue(t, domain.NewSignalRuleVersionReference, "departure-gate-signal/v1"),
			Confidence: mustValue(t, domain.NewConfidenceReference, "SOURCE_CONFIRMED"),
		},
		configured: true,
	}
	handler, err := application.NewRaiseDepartureGateSignalHandler(application.RaiseDepartureGateSignalDeps{
		Rules:   rules,
		Signals: raise.handler,
		Clock:   fixedClock{at: departureGateJudgedAt},
	})
	if err != nil {
		t.Fatalf("构造：%v", err)
	}
	return handler, rules, raise
}

func departureGateSignalCommand(t *testing.T) application.RaiseDepartureGateSignalCommand {
	t.Helper()
	return application.RaiseDepartureGateSignalCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Parcel:   mustValue(t, domain.NewTrackedParcelReference, "parcel-1"),
	}
}

// Covers: AT-TF-158 的信号半边——登记了规则：形成一个发作期，类型、规则版本、可信度取登记那一份，开启时刻是判断时刻；分诊结论
// 随它落库、意图交出。
func TestABlockedDepartureRaisesTheRegisteredSignal(t *testing.T) {
	handler, _, raise := newDepartureGateSignalFixture(t)

	result, err := handler.Handle(context.Background(), departureGateSignalCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DepartureGateSignalRaised || result.HandoffReference() != "" {
		t.Fatalf("outcome = %q handoff=%q, want SIGNAL_RAISED", result.Outcome(), result.HandoffReference())
	}
	if raise.episodes.raisedSaves != 1 || len(raise.downstream.intents) != 1 {
		t.Fatalf("发作期 %d 次落库、分诊意图 %d 份, want 各 1", raise.episodes.raisedSaves, len(raise.downstream.intents))
	}
	for _, episode := range raise.episodes.latest {
		snapshot := episode.Snapshot()
		if snapshot.Kind.String() != "DEPARTURE_GATE_BLOCKED" || snapshot.Rule.String() != "departure-gate-signal/v1" ||
			snapshot.Confidence.String() != "SOURCE_CONFIRMED" || !snapshot.StartedAt.Equal(departureGateJudgedAt) {
			t.Fatalf("发作期 = %s %s %s %s，三样要取登记那一份、时刻取判断时刻",
				snapshot.Kind, snapshot.Rule, snapshot.Confidence, snapshot.StartedAt)
		}
	}

	again, err := handler.Handle(context.Background(), departureGateSignalCommand(t))
	if err != nil || again.Outcome() != application.DepartureGateSignalRaised {
		t.Fatalf("再命中：outcome=%q err=%v", again.Outcome(), err)
	}
	if raise.episodes.raisedSaves != 1 || raise.episodes.hitSaves != 1 {
		t.Fatalf("同一件再被阻断：新建 %d、命中 %d，want 同一发作期记一次命中", raise.episodes.raisedSaves, raise.episodes.hitSaves)
	}
}

// Covers: AT-TF-158 的未配置格——没登记规则：不形成信号、不问分诊，答规则未配置（入账，不重投）。
func TestAnUnregisteredDepartureGateRuleRaisesNothing(t *testing.T) {
	handler, rules, raise := newDepartureGateSignalFixture(t)
	rules.configured = false

	result, err := handler.Handle(context.Background(), departureGateSignalCommand(t))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DepartureGateSignalRuleNotConfigured {
		t.Fatalf("outcome = %q, want SIGNAL_RULE_NOT_CONFIGURED", result.Outcome())
	}
	if raise.triage.calls != 0 || raise.episodes.raisedSaves != 0 {
		t.Fatalf("未配置却问了分诊 %d 次、落了 %d 个发作期", raise.triage.calls, raise.episodes.raisedSaves)
	}
}

// 规则视图调不通、分诊那一侧停在未决，都是搁置：调用方据此整封回滚重放，不把「这次没交进分诊」吞成入账。
func TestADepartureGateSignalIsDeferredWhenTheRuleOrTriageCannotBeRead(t *testing.T) {
	handler, rules, raise := newDepartureGateSignalFixture(t)
	rules.err = errors.New("规则册不可用")
	if result, err := handler.Handle(context.Background(), departureGateSignalCommand(t)); err != nil ||
		result.Outcome() != application.DepartureGateSignalDeferred {
		t.Fatalf("规则读不动：outcome=%q err=%v, want SIGNAL_DEFERRED", result.Outcome(), err)
	}

	rules.err = nil
	raise.episodes.findErr = errors.New("发作期库不可用")
	if result, err := handler.Handle(context.Background(), departureGateSignalCommand(t)); err != nil ||
		result.Outcome() != application.DepartureGateSignalDeferred {
		t.Fatalf("分诊未决：outcome=%q err=%v, want SIGNAL_DEFERRED", result.Outcome(), err)
	}
}

func TestTheDepartureGateSignalHandlerRefusesMissingDependencies(t *testing.T) {
	if _, err := application.NewRaiseDepartureGateSignalHandler(application.RaiseDepartureGateSignalDeps{}); err == nil {
		t.Fatal("缺件构造成功了")
	}
}
