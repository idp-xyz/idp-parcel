package application_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证接受链开头放被取代各版时怎样对待「人已处理」的拒收（ADR-0293 决定四）：提供方仍先问（人在结算侧释放过的，提供方会答已收口，用不着看留痕），
// 只有提供方又确定不受理、而那一笔（版本、控制关联）在处理留痕里有行时，链才凭留痕认出这一笔并放行、接着判新版本；没处理过的、别的版本的拒收上抛；
// 读不动留痕不当成处理过，也不当成没处理过——是依赖读不回，停在本步内部重试。

type refusalHandlingViewDouble struct {
	handled map[ports.ControlReleaseRefusalKey]bool
	err     error
	asked   []ports.ControlReleaseRefusalKey
}

func (double *refusalHandlingViewDouble) FindControlReleaseRefusalHandling(
	_ context.Context,
	key ports.ControlReleaseRefusalKey,
) (ports.ControlReleaseRefusalHandling, bool, error) {
	double.asked = append(double.asked, key)
	if double.err != nil {
		return ports.ControlReleaseRefusalHandling{}, false, double.err
	}
	return ports.ControlReleaseRefusalHandling{Key: key}, double.handled[key], nil
}

var _ ports.ControlReleaseRefusalHandlingView = (*refusalHandlingViewDouble)(nil)

func (double *refusalHandlingViewDouble) markHandled(t *testing.T, version, control string) {
	t.Helper()
	if double.handled == nil {
		double.handled = map[ports.ControlReleaseRefusalKey]bool{}
	}
	double.handled[ports.ControlReleaseRefusalKey{
		TenantID:          mustValue(t, domain.NewTenantID, "tenant-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, version),
		ControlResultID:   mustValue(t, domain.NewFinancialControlResultID, control),
	}] = true
}

// withHandlingView 用同一套替身重装链，只在形成决定的依赖里加上处理留痕读口。
func withHandlingView(t *testing.T, fixture *chainFixture, view ports.ControlReleaseRefusalHandlingView) {
	t.Helper()
	recorder := &judgmentRequestStore{}
	clock := fixedClock{at: handlerClockAt}
	fixture.deps.Decision = application.NewFormAcceptanceDecisionHandler(application.FormAcceptanceDecisionDeps{
		Requests:         fixture.requests,
		Commercial:       fixture.commercial,
		Judgments:        fixture.judgments,
		Reachability:     &reachabilityRevalidatorDouble{outcome: ports.ReachabilityJudgmentStillCurrent},
		Recorder:         recorder,
		Release:          fixture.release,
		ScopeLock:        &scopeLockDouble{},
		Downstream:       fixture.downstream,
		Identities:       &decisionIdentityFactory{t: t},
		Clock:            clock,
		RefusalHandlings: view,
	})
	fixture.handler = application.NewAdvanceAcceptanceChainHandler(fixture.deps)
}

// Covers: AT-PS-165；ADR-0293 决定四——被取代那一版的释放被提供方确定不受理、而那一笔人已处理（留痕在册）：链凭留痕认出这一笔并放行，接着判新版本；提供方先被问了一次。
func TestAHandledRefusalOfASupersededVersionNoLongerBlocksTheNewOne(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
	fixture.release.outcome = ports.ControlReleaseNotAccepted
	view := &refusalHandlingViewDouble{}
	view.markHandled(t, "version-1", "SAC-version-1")
	withHandlingView(t, fixture, view)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v（outcome=%q）——人已处理的拒收挡住了新版本", err, result.Outcome())
	}
	if result.Outcome() != application.AcceptanceChainDecided || result.State() != domain.ShipmentRequestAccepted {
		t.Fatalf("outcome = %q state = %q reason = %q, want DECIDED / ACCEPTED", result.Outcome(), result.State(), result.PendingReason())
	}
	if fixture.release.calls != 1 || fixture.release.versions[0].String() != "version-1" {
		t.Fatalf("释放 %d 次（%v）, want 仍先问提供方一次被取代的 version-1", fixture.release.calls, fixture.release.versions)
	}
	if fixture.controller.calls != 1 {
		t.Fatalf("financial control calls = %d, want 1——新版本照常进入接受前财务控制", fixture.controller.calls)
	}
	if len(view.asked) != 1 || view.asked[0].ControlResultID.String() != "SAC-version-1" {
		t.Fatalf("留痕被问了 %v, want 只问被拒收的那一笔", view.asked)
	}
}

// Covers: AT-PS-165；ADR-0282 决定三——没处理过的拒收响亮上抛 ErrControlReleaseNotAccepted、新版本一步不判；留痕读口没接上（nil）同样上抛。
func TestAnUnhandledRefusalStillFailsTheChainLoudly(t *testing.T) {
	for name, view := range map[string]ports.ControlReleaseRefusalHandlingView{
		"留痕里没有这一笔": &refusalHandlingViewDouble{},
		"留痕读口没接上":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
			fixture.release.outcome = ports.ControlReleaseNotAccepted
			withHandlingView(t, fixture, view)

			_, err := fixture.handler.Handle(context.Background(), command)
			if !errors.Is(err, application.ErrControlReleaseNotAccepted) {
				t.Fatalf("err = %v, want ErrControlReleaseNotAccepted", err)
			}
			if fixture.reachability.calls != 0 || fixture.controller.calls != 0 {
				t.Fatalf("拒收没人处理，链却判了 %d 个成员的可达性、发起了 %d 次资金占用", fixture.reachability.calls, fixture.controller.calls)
			}
		})
	}
}

// Covers: AT-PS-165；处理只认它处理的那一笔——两版被取代、两笔都被拒收，只有一笔人已处理：另一笔响亮上抛，错误点名没处理的那一版。
func TestHandlingOneRefusalDoesNotClearAnotherSupersededVersionsRefusal(t *testing.T) {
	fixture := newChainFixture(t)
	twice := twiceSupersededRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &twice, true
	fixture.judgments.controlByVersion = map[string]domain.FinancialControlOutcome{
		"version-1": domain.FinancialControlHeld, "version-2": domain.FinancialControlHeld, "version-3": domain.FinancialControlHeld,
	}
	fixture.release.outcome = ports.ControlReleaseNotAccepted
	view := &refusalHandlingViewDouble{}
	view.markHandled(t, "version-1", "SAC-version-1")
	withHandlingView(t, fixture, view)
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-3")

	_, err := fixture.handler.Handle(context.Background(), command)
	if !errors.Is(err, application.ErrControlReleaseNotAccepted) {
		t.Fatalf("err = %v, want ErrControlReleaseNotAccepted", err)
	}
	if want := "version-2"; !strings.Contains(err.Error(), want) || strings.Contains(err.Error(), "superseded version version-1") {
		t.Fatalf("err = %v, want 点名没处理的 %s", err, want)
	}
}

// Covers: AT-PS-165；ADR-0029——留痕读不动是依赖读不回，不是「没处理过」也不是「处理过」：链停在放被取代各版这一步内部重试（未决 ControlReleasePending），
// 新版本一步不判；不上抛成响亮失败，也不放行。
func TestAnUnreadableHandlingStopsTheChainForRetryInsteadOfSkippingOrFailingLoudly(t *testing.T) {
	fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
	fixture.release.outcome = ports.ControlReleaseNotAccepted
	view := &refusalHandlingViewDouble{err: errors.New("handling table unreadable")}
	view.markHandled(t, "version-1", "SAC-version-1")
	withHandlingView(t, fixture, view)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v, want 未决而不是错误", err)
	}
	if result.Outcome() != application.AcceptanceChainUndecided || result.Stage() != application.AcceptanceChainSupersededReleaseStage ||
		result.PendingReason() != application.ControlReleasePending {
		t.Fatalf("outcome = %q stage = %q reason = %q, want UNDECIDED / SUPERSEDED_CONTROL_RELEASE / CONTROL_RELEASE_PENDING",
			result.Outcome(), result.Stage(), result.PendingReason())
	}
	if fixture.reachability.calls != 0 || fixture.controller.calls != 0 {
		t.Fatalf("留痕读不动，链却判了新版本（可达性 %d、占用 %d）", fixture.reachability.calls, fixture.controller.calls)
	}
}

// Covers: AT-PS-165；处理留痕不遮住「放不掉」：一版人已处理的拒收、另一版可重试未形成——后者停在本步内部重试。
func TestAHandledRefusalDoesNotHideAPendingReleaseOfAnotherVersion(t *testing.T) {
	fixture := newChainFixture(t)
	twice := twiceSupersededRequest(t)
	fixture.requests.saved, fixture.requests.sticky = &twice, true
	fixture.judgments.controlByVersion = map[string]domain.FinancialControlOutcome{
		"version-1": domain.FinancialControlHeld, "version-2": domain.FinancialControlHeld, "version-3": domain.FinancialControlHeld,
	}
	fixture.release.outcomeFor = map[string]ports.ControlReleaseOutcome{
		"version-1": ports.ControlReleaseNotAccepted, "version-2": ports.ControlReleaseNotFormed,
	}
	view := &refusalHandlingViewDouble{}
	view.markHandled(t, "version-1", "SAC-version-1")
	withHandlingView(t, fixture, view)
	command := fixture.command(t)
	command.SubmissionVersion = mustValue(t, domain.NewSubmissionVersionID, "version-3")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.AcceptanceChainUndecided || result.PendingReason() != application.ControlReleasePending {
		t.Fatalf("outcome = %q reason = %q, want UNDECIDED / CONTROL_RELEASE_PENDING", result.Outcome(), result.PendingReason())
	}
}

// Covers: AT-PS-165；处理只认逐段对得上的那一笔——同一版本不同控制关联、同一控制关联不同版本的处理留痕，都不放行这一笔拒收。
func TestAHandlingOfAnotherVersionOrControlResultDoesNotClearTheRefusal(t *testing.T) {
	for name, handled := range map[string][2]string{
		"同版本另一笔控制": {"version-1", "SAC-another"},
		"同控制另一版本":  {"version-9", "SAC-version-1"},
	} {
		t.Run(name, func(t *testing.T) {
			fixture, command := supersededChainFixture(t, domain.FinancialControlHeld)
			fixture.release.outcome = ports.ControlReleaseNotAccepted
			view := &refusalHandlingViewDouble{}
			view.markHandled(t, handled[0], handled[1])
			withHandlingView(t, fixture, view)

			if _, err := fixture.handler.Handle(context.Background(), command); !errors.Is(err, application.ErrControlReleaseNotAccepted) {
				t.Fatalf("err = %v, want ErrControlReleaseNotAccepted", err)
			}
		})
	}
}
