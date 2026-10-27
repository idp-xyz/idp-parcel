package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件证释放未受理册上一笔被拒收的占用怎样被人处理（ADR-0293 决定二、四）：
//   - 标记已处理：不去问提供方，只留痕；处理留痕与续办信封同笔交出。
//   - 核对后在结算侧释放：按原关联、存下的作用域再请求一次释放；提供方答已收口才算处理，可重试未形成与确定未受理都不留痕、不交信封，
//     把确定的格答给处理人。取作用域锁先于释放。
//   - 先到的处理不被后到的覆盖：已处理的一笔、与并发里输掉的一方，都读回赢家、不再放、不交第二封信封。
//   - 册上没有这一笔答不可见；续办信封交不出来整笔失败（由调用方的事务回滚处理留痕）。

type refusalHandlingsDouble struct {
	handled map[ports.ControlReleaseRefusalKey]ports.ControlReleaseRefusalHandling
	// raceWinner 在场时，Find 仍答没处理过、Handle 却读回这一份赢家：并发里输掉的一方。
	raceWinner *ports.ControlReleaseRefusalHandling
	// notRegistered 让 Handle 答册上无此笔。
	notRegistered bool
	saved         []ports.ControlReleaseRefusalHandling
	findErr       error
	handleErr     error
}

func (double *refusalHandlingsDouble) FindControlReleaseRefusalHandling(
	_ context.Context,
	key ports.ControlReleaseRefusalKey,
) (ports.ControlReleaseRefusalHandling, bool, error) {
	if double.findErr != nil {
		return ports.ControlReleaseRefusalHandling{}, false, double.findErr
	}
	handling, found := double.handled[key]
	return handling, found, nil
}

func (double *refusalHandlingsDouble) HandleControlReleaseRefusal(
	_ context.Context,
	handling ports.ControlReleaseRefusalHandling,
) (ports.ControlReleaseRefusalHandlingSaveOutcome, ports.ControlReleaseRefusalHandling, error) {
	if double.handleErr != nil {
		return ports.ControlReleaseRefusalHandlingSaveOutcomeInvalid, ports.ControlReleaseRefusalHandling{}, double.handleErr
	}
	if double.notRegistered {
		return ports.ControlReleaseRefusalNotRegistered, ports.ControlReleaseRefusalHandling{}, nil
	}
	if double.raceWinner != nil {
		return ports.ControlReleaseRefusalAlreadyHandled, *double.raceWinner, nil
	}
	double.saved = append(double.saved, handling)
	return ports.ControlReleaseRefusalHandlingRecorded, handling, nil
}

var _ ports.ControlReleaseRefusalHandlings = (*refusalHandlingsDouble)(nil)

type refusalHandoffDouble struct {
	intents []ports.ControlReleaseRefusalHandledHandoffIntent
	err     error
}

func (double *refusalHandoffDouble) HandOffControlReleaseRefusalHandled(
	_ context.Context,
	intent ports.ControlReleaseRefusalHandledHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

var _ ports.ControlReleaseRefusalHandledHandoff = (*refusalHandoffDouble)(nil)

type refusalHandlingFixture struct {
	handler   *application.HandleControlReleaseRefusalHandler
	requests  *disposableRequestStore
	refusals  *controlReleaseRefusalDouble
	handlings *refusalHandlingsDouble
	judgments *recordedJudgmentsDouble
	release   *controlReleaseDouble
	scopeLock *scopeLockDouble
	handoff   *refusalHandoffDouble
	// calls 按序记下"取锁""释放""留痕""交信封"，断言次序。
	calls []string
}

func newRefusalHandlingFixture(t *testing.T) *refusalHandlingFixture {
	t.Helper()
	control := heldThenRestrictedControl(t)
	fixture := &refusalHandlingFixture{
		requests:  &disposableRequestStore{t: t, request: submittedRequest(t)},
		refusals:  &controlReleaseRefusalDouble{},
		handlings: &refusalHandlingsDouble{handled: map[ports.ControlReleaseRefusalKey]ports.ControlReleaseRefusalHandling{}},
		judgments: &recordedJudgmentsDouble{t: t, control: &control},
		release:   &controlReleaseDouble{},
		scopeLock: &scopeLockDouble{},
		handoff:   &refusalHandoffDouble{},
	}
	fixture.refusals.recorded = []ports.ControlReleaseRefusal{{
		TenantID:          mustValue(t, domain.NewTenantID, "tenant-1"),
		CustomerAccountID: mustValue(t, domain.NewCustomerAccountID, "customer-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		ControlResultID:   mustValue(t, domain.NewFinancialControlResultID, "SAC-1"),
		Scope:             control.Scope(),
		Reason:            checkReasonFor("SA-RELEASE_REQUEST_NOT_ACCEPTED"),
		RecordedAt:        handlerClockAt,
	}}
	fixture.release.record = func(call string) { fixture.calls = append(fixture.calls, call) }
	fixture.scopeLock.onLock = func(domain.OccupationScope) { fixture.calls = append(fixture.calls, "lock-scope") }
	fixture.handler = application.NewHandleControlReleaseRefusalHandler(application.HandleControlReleaseRefusalDeps{
		Requests:  fixture.requests,
		Refusals:  fixture.refusals,
		Handlings: fixture.handlings,
		Judgments: fixture.judgments,
		Release:   fixture.release,
		ScopeLock: fixture.scopeLock,
		Handoff:   &recordingHandoff{inner: fixture.handoff, record: func() { fixture.calls = append(fixture.calls, "hand-off") }},
		Clock:     fixedClock{at: handlerClockAt},
	})
	return fixture
}

// recordingHandoff 把"交信封"记进夹具的调用序。
type recordingHandoff struct {
	inner  *refusalHandoffDouble
	record func()
}

func (handoff *recordingHandoff) HandOffControlReleaseRefusalHandled(ctx context.Context, intent ports.ControlReleaseRefusalHandledHandoffIntent) error {
	handoff.record()
	return handoff.inner.HandOffControlReleaseRefusalHandled(ctx, intent)
}

func (fixture *refusalHandlingFixture) command(t *testing.T, action application.ControlReleaseRefusalAction) application.HandleControlReleaseRefusalCommand {
	t.Helper()
	return application.HandleControlReleaseRefusalCommand{
		Identity:          sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1"),
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		SubmissionVersion: mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		ControlResultID:   mustValue(t, domain.NewFinancialControlResultID, "SAC-1"),
		Action:            action,
		Handler:           mustValue(t, domain.NewControlReleaseRefusalHandlerReference, "https://id.syn.example#alice"),
		Basis:             mustValue(t, domain.NewControlReleaseRefusalHandlingBasis, "SYN-LEDGER-CHECKED"),
	}
}

func (fixture *refusalHandlingFixture) assertNothingWritten(t *testing.T) {
	t.Helper()
	if len(fixture.handlings.saved) != 0 {
		t.Fatalf("不该留痕，却留了 %+v", fixture.handlings.saved)
	}
	if len(fixture.handoff.intents) != 0 {
		t.Fatalf("不该交续办信封，却交了 %d 封", len(fixture.handoff.intents))
	}
}

// Covers: AT-PS-163；ADR-0293 决定二——标记已处理：不问提供方（释放一次也不发、不取作用域锁），只留痕：处理人、依据与编排时钟的处理时刻；同笔交恰一封续办信封，
// 信封带处理后读回的委托与这一次的留痕。
func TestMarkingARefusalHandledRecordsItAndHandsOffOneResumeWithoutAskingTheProvider(t *testing.T) {
	fixture := newRefusalHandlingFixture(t)

	command := fixture.command(t, application.MarkControlReleaseRefusalHandled)
	result, err := fixture.handler.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ControlReleaseRefusalHandled {
		t.Fatalf("outcome = %q, want HANDLED", result.Outcome())
	}
	if fixture.release.calls != 0 || len(fixture.scopeLock.scopes) != 0 {
		t.Fatalf("标记已处理去问了提供方（释放 %d 次、取锁 %d 把）", fixture.release.calls, len(fixture.scopeLock.scopes))
	}
	if len(fixture.handlings.saved) != 1 {
		t.Fatalf("留痕 %d 行, want 1", len(fixture.handlings.saved))
	}
	saved := fixture.handlings.saved[0]
	if saved.Kind != ports.ControlReleaseRefusalMarkedHandled || saved.Handler.String() != "https://id.syn.example#alice" ||
		saved.Basis.String() != "SYN-LEDGER-CHECKED" || !saved.HandledAt.Equal(handlerClockAt) {
		t.Fatalf("留痕 = %+v, want 标记已处理、处理人、依据、编排时钟", saved)
	}
	wantKey := ports.KeyOfRefusal(fixture.refusals.recorded[0])
	if saved.Key != wantKey {
		t.Fatalf("留痕的键 = %+v, want 册行的键 %+v", saved.Key, wantKey)
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("续办信封 %d 封, want 恰好 1——处理与续办必须同笔", len(fixture.handoff.intents))
	}
	intent := fixture.handoff.intents[0]
	if intent.Handling != saved || intent.Identity != command.Identity {
		t.Fatalf("信封意图 = %+v, want 带这一次的留痕与委托的来源身份", intent)
	}
	if intent.Request.ShipmentRequestID() != fixture.requests.request.ShipmentRequestID() {
		t.Fatalf("信封意图的委托 = %s, want 读回的那一份", intent.Request.ShipmentRequestID())
	}
}

// Covers: AT-PS-162；ADR-0293 决定二、四——核对后在结算侧释放：先取作用域锁，再按原关联与存下的作用域请求释放，答已收口才留痕并同笔交信封。
func TestReleasingAtSettlementRecordsAndHandsOffOnlyAfterTheProviderSettlesIt(t *testing.T) {
	fixture := newRefusalHandlingFixture(t)

	result, err := fixture.handler.Handle(t.Context(), fixture.command(t, application.ReleaseControlReleaseRefusalAtSettlement))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.ControlReleaseRefusalHandled {
		t.Fatalf("outcome = %q, want HANDLED", result.Outcome())
	}
	if fixture.release.calls != 1 || fixture.release.controlResultID != "SAC-1" || fixture.release.versions[0].String() != "version-1" {
		t.Fatalf("释放 %d 次（控制关联 %q）, want 按原关联 SAC-1 释放恰一次", fixture.release.calls, fixture.release.controlResultID)
	}
	if fixture.release.scopes[0] != fixture.refusals.recorded[0].Scope || !fixture.release.scopes[0].Recorded() {
		t.Fatalf("释放交出的作用域 = %+v, want 控制结果上存下的那一份 %+v", fixture.release.scopes[0], fixture.refusals.recorded[0].Scope)
	}
	order := []string{"lock-scope", "release-control", "hand-off"}
	if len(fixture.calls) != len(order) {
		t.Fatalf("调用序 = %v, want %v", fixture.calls, order)
	}
	for index := range order {
		if fixture.calls[index] != order[index] {
			t.Fatalf("调用序 = %v, want %v——取锁先于释放，交信封在留痕之后", fixture.calls, order)
		}
	}
	if len(fixture.handlings.saved) != 1 || fixture.handlings.saved[0].Kind != ports.ControlReleaseRefusalReleasedAtSettlement {
		t.Fatalf("留痕 = %+v, want 一行「已在结算侧释放」", fixture.handlings.saved)
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("续办信封 %d 封, want 1", len(fixture.handoff.intents))
	}
}

// Covers: AT-PS-162；ADR-0293 决定二——释放没收口不算处理：可重试未形成与确定未受理（含形不成三格之一）都不留痕、不交信封，把确定的格答给处理人；
// 确定未受理带提供方的原因，形不成三格之一带本上下文自有的原因。
func TestAReleaseThatDoesNotSettleLeavesTheRefusalUnhandledAndSaysWhy(t *testing.T) {
	for name, test := range map[string]struct {
		arrange func(*controlReleaseDouble)
		outcome application.ControlReleaseRefusalHandlingOutcome
		reason  string
	}{
		"可重试未形成": {
			arrange: func(release *controlReleaseDouble) { release.outcome = ports.ControlReleaseNotFormed },
			outcome: application.ControlReleaseNotSettled,
		},
		"确定未受理": {
			arrange: func(release *controlReleaseDouble) { release.outcome = ports.ControlReleaseNotAccepted },
			outcome: application.ControlReleaseRefusedAgain,
			reason:  "SA-RELEASE_REQUEST_NOT_ACCEPTED",
		},
		"形不成三格之一": {
			arrange: func(release *controlReleaseDouble) {
				release.err = errors.Join(ports.ErrUntranslatableProviderAnswer, errors.New("release outcome 9"))
			},
			outcome: application.ControlReleaseRefusedAgain,
			reason:  "CONTROL_RELEASE_ANSWER_UNTRANSLATABLE",
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRefusalHandlingFixture(t)
			test.arrange(fixture.release)

			result, err := fixture.handler.Handle(t.Context(), fixture.command(t, application.ReleaseControlReleaseRefusalAtSettlement))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != test.outcome {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), test.outcome)
			}
			if result.Reason().String() != test.reason {
				t.Fatalf("reason = %q, want %q", result.Reason(), test.reason)
			}
			fixture.assertNothingWritten(t)
		})
	}
}

// Covers: AT-PS-163；ADR-0293 决定二——先到的处理不被覆盖：已处理的一笔读回先到的那一份，不再问提供方、不再留痕、不交第二封信封；并发里输掉的一方
// （查时没处理过、写时读回赢家）同答。
func TestASecondHandlerReadsTheFirstAndTriggersNothing(t *testing.T) {
	for name, arrange := range map[string]func(*testing.T, *refusalHandlingFixture, ports.ControlReleaseRefusalHandling){
		"已处理": func(_ *testing.T, fixture *refusalHandlingFixture, winner ports.ControlReleaseRefusalHandling) {
			fixture.handlings.handled[winner.Key] = winner
		},
		"并发输掉": func(_ *testing.T, fixture *refusalHandlingFixture, winner ports.ControlReleaseRefusalHandling) {
			fixture.handlings.raceWinner = &winner
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRefusalHandlingFixture(t)
			winner := ports.ControlReleaseRefusalHandling{
				Key:       ports.KeyOfRefusal(fixture.refusals.recorded[0]),
				Kind:      ports.ControlReleaseRefusalMarkedHandled,
				Handler:   mustValue(t, domain.NewControlReleaseRefusalHandlerReference, "https://id.syn.example#bob"),
				Basis:     mustValue(t, domain.NewControlReleaseRefusalHandlingBasis, "SYN-EARLIER-BASIS"),
				HandledAt: handlerClockAt.Add(-1),
			}
			arrange(t, fixture, winner)

			result, err := fixture.handler.Handle(t.Context(), fixture.command(t, application.MarkControlReleaseRefusalHandled))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			got, present := result.Handling()
			if result.Outcome() != application.ControlReleaseRefusalAlreadyHandled || !present || got != winner {
				t.Fatalf("outcome = %q handling = %+v（在场 %v）, want ALREADY_HANDLED 与先到那一份", result.Outcome(), got, present)
			}
			fixture.assertNothingWritten(t)
		})
	}

	// 已处理的一笔选「释放」也不再去问提供方。
	fixture := newRefusalHandlingFixture(t)
	key := ports.KeyOfRefusal(fixture.refusals.recorded[0])
	fixture.handlings.handled[key] = ports.ControlReleaseRefusalHandling{Key: key, Kind: ports.ControlReleaseRefusalMarkedHandled,
		Handler: mustValue(t, domain.NewControlReleaseRefusalHandlerReference, "h"), Basis: mustValue(t, domain.NewControlReleaseRefusalHandlingBasis, "b"),
		HandledAt: handlerClockAt}
	if result, err := fixture.handler.Handle(t.Context(), fixture.command(t, application.ReleaseControlReleaseRefusalAtSettlement)); err != nil ||
		result.Outcome() != application.ControlReleaseRefusalAlreadyHandled || fixture.release.calls != 0 {
		t.Fatalf("已处理的一笔选释放：outcome=%q err=%v 释放 %d 次, want ALREADY_HANDLED 且一次不发", result.Outcome(), err, fixture.release.calls)
	}
}

// Covers: AT-PS-163；ADR-0293 决定二、ADR-0003——册上没有这一笔（不存在，或属于别的租户）答「统一不可见」，什么也不问、不写：释放一次不发、不留痕、不交信封。
// 留痕口说册上无此笔（册读到之后又没了）同答。
func TestARefusalThatIsNotOnTheTenantsRegisterIsNotVisible(t *testing.T) {
	for name, arrange := range map[string]func(*refusalHandlingFixture){
		"册上没有": func(fixture *refusalHandlingFixture) { fixture.refusals.recorded = nil },
		"另一版": func(fixture *refusalHandlingFixture) {
			fixture.refusals.recorded[0].SubmissionVersion = checkVersion(fixture, "version-9")
		},
		"留痕口说无此笔": func(fixture *refusalHandlingFixture) { fixture.handlings.notRegistered = true },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRefusalHandlingFixture(t)
			arrange(fixture)
			_, err := fixture.handler.Handle(t.Context(), fixture.command(t, application.MarkControlReleaseRefusalHandled))
			if !errors.Is(err, application.ErrShipmentRequestNotVisible) {
				t.Fatalf("err = %v, want ErrShipmentRequestNotVisible", err)
			}
			if fixture.release.calls != 0 {
				t.Fatalf("释放了 %d 次", fixture.release.calls)
			}
			fixture.assertNothingWritten(t)
		})
	}
}

func checkVersion(_ *refusalHandlingFixture, raw string) domain.SubmissionVersionID {
	version, err := domain.NewSubmissionVersionID(raw)
	if err != nil {
		panic(err)
	}
	return version
}

// Covers: AT-PS-163；ADR-0094 决定四、ADR-0293 决定二——续办信封交不出来整笔失败：错误原样上抛让调用方的事务回滚处理留痕，不答「已处理」。
// 读册、读留痕、读判断、取锁、读委托任一步读不动同样上抛，不当成没处理过或没拒收过。
func TestAFailureAnywhereOnThePathSurfacesAsAnErrorNotAnAnswer(t *testing.T) {
	boom := errors.New("dependency failed")
	for name, arrange := range map[string]func(*refusalHandlingFixture){
		"交不出信封": func(fixture *refusalHandlingFixture) { fixture.handoff.err = boom },
		"留痕读不动": func(fixture *refusalHandlingFixture) { fixture.handlings.findErr = boom },
		"留痕写不动": func(fixture *refusalHandlingFixture) { fixture.handlings.handleErr = boom },
		"判断读不动": func(fixture *refusalHandlingFixture) { fixture.judgments.err = boom },
		"取不到锁":  func(fixture *refusalHandlingFixture) { fixture.scopeLock.err = boom },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRefusalHandlingFixture(t)
			arrange(fixture)
			result, err := fixture.handler.Handle(t.Context(), fixture.command(t, application.ReleaseControlReleaseRefusalAtSettlement))
			if err == nil {
				t.Fatalf("outcome = %q, want 错误上抛", result.Outcome())
			}
			if fixture.release.calls != 0 && (name == "判断读不动" || name == "取不到锁") {
				t.Fatalf("%s 之后还发了释放", name)
			}
		})
	}
}

// Covers: 装配缺件响亮：少接任何一个必需件的编排不当作未决，也不悄悄跳过交信封。
func TestTheHandlerRefusesToRunWithAMissingPart(t *testing.T) {
	deps := func(mutate func(*application.HandleControlReleaseRefusalDeps)) application.HandleControlReleaseRefusalDeps {
		fixture := newRefusalHandlingFixture(t)
		built := application.HandleControlReleaseRefusalDeps{
			Requests: fixture.requests, Refusals: fixture.refusals, Handlings: fixture.handlings, Judgments: fixture.judgments,
			Release: fixture.release, ScopeLock: fixture.scopeLock, Handoff: fixture.handoff, Clock: fixedClock{at: handlerClockAt},
		}
		mutate(&built)
		return built
	}
	for name, mutate := range map[string]func(*application.HandleControlReleaseRefusalDeps){
		"没接续办信封": func(d *application.HandleControlReleaseRefusalDeps) { d.Handoff = nil },
		"没接处理留痕": func(d *application.HandleControlReleaseRefusalDeps) { d.Handlings = nil },
		"没接册":    func(d *application.HandleControlReleaseRefusalDeps) { d.Refusals = nil },
		"没接委托仓储": func(d *application.HandleControlReleaseRefusalDeps) { d.Requests = nil },
		"没接时钟":   func(d *application.HandleControlReleaseRefusalDeps) { d.Clock = nil },
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRefusalHandlingFixture(t)
			handler := application.NewHandleControlReleaseRefusalHandler(deps(mutate))
			_, err := handler.Handle(t.Context(), fixture.command(t, application.MarkControlReleaseRefusalHandled))
			if !errors.Is(err, application.ErrControlReleaseRefusalHandlingNotAssembled) {
				t.Fatalf("err = %v, want ErrControlReleaseRefusalHandlingNotAssembled", err)
			}
		})
	}

	// 释放那一支另需判断读口、释放口与取锁口；标记已处理用不到它们，缺了也照样能标。
	fixture := newRefusalHandlingFixture(t)
	handler := application.NewHandleControlReleaseRefusalHandler(deps(func(d *application.HandleControlReleaseRefusalDeps) {
		d.Judgments, d.Release, d.ScopeLock = nil, nil, nil
	}))
	if _, err := handler.Handle(t.Context(), fixture.command(t, application.MarkControlReleaseRefusalHandled)); err != nil {
		t.Fatalf("标记已处理缺释放件：%v", err)
	}
	_, err := handler.Handle(t.Context(), fixture.command(t, application.ReleaseControlReleaseRefusalAtSettlement))
	if !errors.Is(err, application.ErrControlReleaseRefusalHandlingNotAssembled) {
		t.Fatalf("释放缺释放件 err = %v, want ErrControlReleaseRefusalHandlingNotAssembled", err)
	}
}

// Covers: 命令的动作是封闭两值，认不出的动作不当成任何一种。
func TestAnUnknownActionIsRefusedBeforeAnythingIsRead(t *testing.T) {
	fixture := newRefusalHandlingFixture(t)
	command := fixture.command(t, application.ControlReleaseRefusalActionInvalid)
	if _, err := fixture.handler.Handle(t.Context(), command); !errors.Is(err, application.ErrControlReleaseRefusalActionUnknown) {
		t.Fatalf("err = %v, want ErrControlReleaseRefusalActionUnknown", err)
	}
	fixture.assertNothingWritten(t)
}
