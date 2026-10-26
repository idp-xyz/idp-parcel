package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// withdrawalReplayStore 是一只会记住保存结果的委托仓储替身：重放要跨两次调用才谈得上，第一次形成的撤回要被第二次读到。
// 保存失败与并发抢先是一次性的拨动——停下的那一轮之后，条件恢复，重放走的是同一只仓储。
type withdrawalReplayStore struct {
	t            *testing.T
	current      domain.ShipmentRequest
	readErr      error
	saveErr      error
	conflictNext bool
	saves        int
}

func (store *withdrawalReplayStore) FindBySourceIdentity(
	_ context.Context,
	_ domain.SourceIdentity,
) (domain.ShipmentRequest, bool, error) {
	store.t.Helper()
	if store.readErr != nil {
		return domain.ShipmentRequest{}, false, store.readErr
	}
	return store.current, true, nil
}

func (store *withdrawalReplayStore) Insert(
	context.Context,
	domain.SourceIdentity,
	domain.ShipmentRequest,
) (ports.ShipmentRequestInsertOutcome, error) {
	return ports.ShipmentRequestInserted, nil
}

func (store *withdrawalReplayStore) Save(
	_ context.Context,
	_ domain.SourceIdentity,
	request domain.ShipmentRequest,
) (ports.ShipmentRequestSaveOutcome, error) {
	if store.saveErr != nil {
		return ports.ShipmentRequestSaveOutcomeInvalid, store.saveErr
	}
	if store.conflictNext {
		// 抢先那一方已经落库，本方这一份不留。
		return ports.ShipmentRequestRevisionConflict, nil
	}
	store.current = request
	store.saves++
	return ports.ShipmentRequestSaved, nil
}

// replayDecisionIdentity 是决定标识工厂替身：能拨成签发不出，签出的每个标识都不同——重放若又形成一次撤回，
// 两份撤回的决定标识不同，断言才分得出「读回原撤回」与「又撤了一次」。
type replayDecisionIdentity struct {
	t      *testing.T
	err    error
	issued int
}

func (factory *replayDecisionIdentity) NextAcceptanceDecisionID(
	_ context.Context,
) (domain.AcceptanceDecisionID, error) {
	factory.t.Helper()
	if factory.err != nil {
		return domain.AcceptanceDecisionID{}, factory.err
	}
	factory.issued++
	return mustValue(factory.t, domain.NewAcceptanceDecisionID, "decision-"+string(rune('0'+factory.issued))), nil
}

var (
	_ ports.ShipmentRequestRepository  = (*withdrawalReplayStore)(nil)
	_ ports.AcceptanceDecisionIdentity = (*replayDecisionIdentity)(nil)
)

// newWithdrawalReplayFixture 在既有撤回夹具上换一只有记忆的委托仓储与一只能拨的标识工厂，其余替身沿用。
func newWithdrawalReplayFixture(t *testing.T) (*withdrawalFixture, *withdrawalReplayStore, *replayDecisionIdentity) {
	t.Helper()
	fixture := newWithdrawalFixture(t)
	store := &withdrawalReplayStore{t: t, current: submittedRequest(t)}
	identities := &replayDecisionIdentity{t: t}
	fixture.handler = application.NewWithdrawShipmentRequestHandler(application.WithdrawShipmentRequestDeps{
		Sources:    fixture.sources,
		Requests:   store,
		Authorizer: fixture.authorizer,
		Judgments:  fixture.judgments,
		Recorder:   &judgmentRequestStore{},
		Release:    fixture.release,
		Refusals:   fixture.refusals,
		ScopeLock:  fixture.scopeLocks,
		Identities: identities,
		Clock:      fixedClock{at: handlerClockAt},
	})
	return fixture, store, identities
}

// Covers: ADR-0256 决定一第 2 格「重放须接着办」、UC-PS-005「撤回未决」一行——撤回的装配事务在编排交回业务答案时
// 照样提交来源保全，所以停在授权、标识、读不回或并发抢先的撤回，来源已保全而委托仍是`已提交`。重放同一请求要按首次
// 请求走授权与决定提交，形成撤回；再重放才是已有决定，带那份撤回，不再形成第二次。
func TestAReplayedWithdrawalIsCarriedOnAfterEveryRoundThatFormedNoWithdrawal(t *testing.T) {
	cases := []struct {
		name   string
		stop   func(*withdrawalFixture, *withdrawalReplayStore, *replayDecisionIdentity)
		reason application.JudgmentPendingReason
		resume func(*withdrawalFixture, *withdrawalReplayStore, *replayDecisionIdentity)
	}{
		{
			name: "委托读不回",
			stop: func(_ *withdrawalFixture, store *withdrawalReplayStore, _ *replayDecisionIdentity) {
				store.readErr = errors.New("storage unreachable")
			},
			reason: application.ShipmentRequestUnavailable,
			resume: func(_ *withdrawalFixture, store *withdrawalReplayStore, _ *replayDecisionIdentity) {
				store.readErr = nil
			},
		},
		{
			name: "授权答不出",
			stop: func(f *withdrawalFixture, _ *withdrawalReplayStore, _ *replayDecisionIdentity) {
				f.authorizer.err = errors.New("authority unreachable")
			},
			reason: application.WithdrawalAuthorityUnavailable,
			resume: func(f *withdrawalFixture, _ *withdrawalReplayStore, _ *replayDecisionIdentity) {
				f.authorizer.err = nil
			},
		},
		{
			name: "授权规则未登记",
			stop: func(f *withdrawalFixture, _ *withdrawalReplayStore, _ *replayDecisionIdentity) {
				f.authorizer.rulesNotConfigured = true
			},
			reason: application.WithdrawalAuthorityRulesNotConfigured,
			resume: func(f *withdrawalFixture, _ *withdrawalReplayStore, _ *replayDecisionIdentity) {
				f.authorizer.rulesNotConfigured = false
			},
		},
		{
			name: "授权适用冲突",
			stop: func(f *withdrawalFixture, _ *withdrawalReplayStore, _ *replayDecisionIdentity) {
				f.authorizer.applicabilityConflict = true
			},
			reason: application.WithdrawalAuthorityApplicabilityConflict,
			resume: func(f *withdrawalFixture, _ *withdrawalReplayStore, _ *replayDecisionIdentity) {
				f.authorizer.applicabilityConflict = false
			},
		},
		{
			name: "决定标识签发不出",
			stop: func(_ *withdrawalFixture, _ *withdrawalReplayStore, identities *replayDecisionIdentity) {
				identities.err = errors.New("identity factory unreachable")
			},
			reason: application.DecisionIdentityUnavailable,
			resume: func(_ *withdrawalFixture, _ *withdrawalReplayStore, identities *replayDecisionIdentity) {
				identities.err = nil
			},
		},
		{
			name: "并发写入抢先",
			stop: func(_ *withdrawalFixture, store *withdrawalReplayStore, _ *replayDecisionIdentity) {
				store.conflictNext = true
			},
			reason: application.StaleShipmentRequestRevision,
			resume: func(_ *withdrawalFixture, store *withdrawalReplayStore, _ *replayDecisionIdentity) {
				store.conflictNext = false
			},
		},
		{
			name: "保存失败",
			stop: func(_ *withdrawalFixture, store *withdrawalReplayStore, _ *replayDecisionIdentity) {
				store.saveErr = errors.New("storage unreachable")
			},
			reason: application.DecisionNotRecorded,
			resume: func(_ *withdrawalFixture, store *withdrawalReplayStore, _ *replayDecisionIdentity) {
				store.saveErr = nil
			},
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, store, identities := newWithdrawalReplayFixture(t)
			command := fixture.command(t)

			tc.stop(fixture, store, identities)
			stopped, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle the round that stops: %v", err)
			}
			if stopped.Outcome() != application.WithdrawalUndecided || stopped.PendingReason() != tc.reason {
				t.Fatalf("outcome = %q reason = %q, want UNDECIDED / %q", stopped.Outcome(), stopped.PendingReason(), tc.reason)
			}
			if _, preserved := fixture.sources.stored(t, withdrawalSourceIdentity(t)); !preserved {
				t.Fatal("the stopped round did not preserve its source, so there is nothing for a replay to recognise")
			}
			if store.current.State() != domain.ShipmentRequestSubmitted {
				t.Fatalf("state = %q; the stopped round must leave the request submitted", store.current.State())
			}

			tc.resume(fixture, store, identities)
			replayed, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("replay the stopped request: %v", err)
			}
			if replayed.Outcome() != application.WithdrawalFormed {
				t.Fatalf("replay outcome = %q reason = %q, want FORMED——a replay must carry on, not report a decision that was never formed",
					replayed.Outcome(), replayed.PendingReason())
			}
			formed, present := replayed.Withdrawal()
			if !present {
				t.Fatal("the replay that carried on returned no withdrawal")
			}
			if store.current.State() != domain.ShipmentRequestWithdrawn || store.saves != 1 {
				t.Fatalf("state = %q saves = %d, want withdrawn after exactly one save", store.current.State(), store.saves)
			}
			if fixture.sources.preserveCount != 1 {
				t.Fatalf("preserved %d times; a replay recognises the preserved source and must not preserve a second one", fixture.sources.preserveCount)
			}

			authorizations := fixture.authorizer.calls
			issued := identities.issued
			again, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("replay the formed withdrawal: %v", err)
			}
			if again.Outcome() != application.WithdrawalDecisionAlreadyFormed {
				t.Fatalf("second replay outcome = %q, want DECISION_ALREADY_FORMED", again.Outcome())
			}
			record, present := again.Withdrawal()
			if !present || record.DecisionID() != formed.DecisionID() {
				t.Fatalf("second replay withdrawal = %+v (present=%v), want the one the first replay formed (%+v)", record, present, formed)
			}
			if fixture.authorizer.calls != authorizations || identities.issued != issued || store.saves != 1 {
				t.Fatal("a replay of a request that already formed its withdrawal asked for authorization, a decision identity or a second save")
			}
		})
	}
}

// Covers: ADR-0256 决定三「不得答成已形成结果」——仍为`已提交`的委托，重放不得答 DECISION_ALREADY_FORMED：那一格要带
// 一份决定，而这份委托没有。授权仍未决时重放答的还是未决，带续办引用。
func TestAReplayedWithdrawalOfASubmittedRequestIsNeverAnsweredAsADecisionAlreadyFormed(t *testing.T) {
	fixture, store, _ := newWithdrawalReplayFixture(t)
	command := fixture.command(t)
	fixture.authorizer.err = errors.New("authority unreachable")

	for round := 0; round < 3; round++ {
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("round %d: %v", round, err)
		}
		if result.Outcome() != application.WithdrawalUndecided || result.PendingReason() != application.WithdrawalAuthorityUnavailable {
			t.Fatalf("round %d: outcome = %q reason = %q, want UNDECIDED / WITHDRAWAL_AUTHORITY_UNAVAILABLE", round, result.Outcome(), result.PendingReason())
		}
		if result.State() != domain.ShipmentRequestSubmitted || result.ContinuationReference().String() == "" {
			t.Fatalf("round %d: state = %q continuation = %q, want the read state and a continuation", round, result.State(), result.ContinuationReference())
		}
	}
	if store.saves != 0 {
		t.Fatalf("saves = %d, want none", store.saves)
	}
}

// Covers: 确定的业务答案重放不被改写成已有决定——未获授权的撤回重放，授权条件没变就还是未获授权，不是答一个并不存在的
// 已有决定；授权补上了才接着办形成撤回。
func TestAReplayedWithdrawalThatWasRefusedIsAnsweredAgain(t *testing.T) {
	fixture, store, _ := newWithdrawalReplayFixture(t)
	command := fixture.command(t)
	fixture.authorizer.granted = false

	for _, round := range []string{"first", "replay"} {
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil || result.Outcome() != application.WithdrawalNotAuthorized {
			t.Fatalf("%s: outcome = %q err = %v, want NOT_AUTHORIZED", round, result.Outcome(), err)
		}
	}

	fixture.authorizer.granted = true
	carried, err := fixture.handler.Handle(context.Background(), command)
	if err != nil || carried.Outcome() != application.WithdrawalFormed {
		t.Fatalf("replay after the grant: outcome = %q err = %v, want FORMED", carried.Outcome(), err)
	}
	if store.saves != 1 {
		t.Fatalf("saves = %d, want one", store.saves)
	}
}

// Covers: AT-PS-071、AT-PS-072、AT-PS-068 与 ADR-0256 决定一第 2 格——委托在上一轮之后越过了提交边界（别人先接受、先拒绝，
// 或本请求自己的撤回已成立）：重放答已有决定并带上那份决定，不问授权、不耗决定标识、不写；已撤回的另带原撤回。
// 接受先成立时不释放合法冻结。
func TestAReplayedWithdrawalOfADecidedRequestAnswersTheExistingDecisionAndCarriesIt(t *testing.T) {
	cases := []struct {
		name     string
		decided  func(*testing.T) domain.ShipmentRequest
		state    domain.ShipmentRequestState
		decision string
		withdraw bool
	}{
		{
			name:     "已接受",
			decided:  acceptedRequest,
			state:    domain.ShipmentRequestAccepted,
			decision: "decision-1",
		},
		{
			name: "已拒绝",
			decided: func(t *testing.T) domain.ShipmentRequest {
				rejected, err := submittedRequest(t).RejectByAuthority(domain.ActiveRejectionSpec{
					DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-0"),
					Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-0"),
					Decider:    mustValue(t, domain.NewDeciderReference, "OPERATOR-0"),
					Reason:     mustValue(t, domain.NewRejectionReasonReference, "EARLIER_DECISION"),
					Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "EVID-0"),
					DecidedAt:  handlerClockAt,
				})
				if err != nil {
					t.Fatalf("form the earlier rejection: %v", err)
				}
				return rejected
			},
			state:    domain.ShipmentRequestRejected,
			decision: "decision-0",
		},
		{
			name: "已撤回",
			decided: func(t *testing.T) domain.ShipmentRequest {
				withdrawn, err := submittedRequest(t).WithdrawByCustomer(domain.WithdrawalSpec{
					DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-0"),
					Authority:  mustValue(t, domain.NewWithdrawalAuthorityReference, "PC-WITHDRAW-ROLE-0"),
					Requester:  mustValue(t, domain.NewWithdrawalRequesterReference, "CUSTOMER-CONTACT-0"),
					Reason:     mustValue(t, domain.NewWithdrawalReasonReference, "EARLIER_WITHDRAWAL"),
					DecidedAt:  handlerClockAt,
				})
				if err != nil {
					t.Fatalf("form the earlier withdrawal: %v", err)
				}
				return withdrawn
			},
			state:    domain.ShipmentRequestWithdrawn,
			decision: "decision-0",
			withdraw: true,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fixture, store, identities := newWithdrawalReplayFixture(t)
			command := fixture.command(t)
			fixture.authorizer.err = errors.New("authority unreachable")
			if _, err := fixture.handler.Handle(context.Background(), command); err != nil {
				t.Fatalf("handle the round that stops: %v", err)
			}

			store.current = tc.decided(t)
			authorizations := fixture.authorizer.calls
			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("replay: %v", err)
			}

			if result.Outcome() != application.WithdrawalDecisionAlreadyFormed {
				t.Fatalf("outcome = %q reason = %q, want DECISION_ALREADY_FORMED", result.Outcome(), result.PendingReason())
			}
			if result.State() != tc.state {
				t.Fatalf("state = %q, want %q", result.State(), tc.state)
			}
			if tc.withdraw {
				record, present := result.Withdrawal()
				if !present || record.DecisionID().String() != tc.decision {
					t.Fatalf("withdrawal = %+v (present=%v), want the earlier one (%q)", record, present, tc.decision)
				}
				if _, carried := result.AcceptanceDecision(); carried {
					t.Fatal("a withdrawn request carried an acceptance decision it never had")
				}
			} else {
				decision, present := result.AcceptanceDecision()
				if !present || decision.DecisionID().String() != tc.decision {
					t.Fatalf("decision = %+v (present=%v), want the earlier one (%q)", decision, present, tc.decision)
				}
				if _, formed := result.Withdrawal(); formed {
					t.Fatal("a request that was not withdrawn carried a withdrawal")
				}
			}
			if fixture.authorizer.calls != authorizations || identities.issued != 0 || store.saves != 0 {
				t.Fatal("a replay over an already decided request asked for authorization, a decision identity or a write")
			}
			if tc.state != domain.ShipmentRequestWithdrawn && fixture.release.calls != 0 {
				t.Fatalf("released %d times over a lawful %s decision", fixture.release.calls, tc.state)
			}
		})
	}
}

// Covers: AT-PS-069「同一撤回请求身份携带不同内容 → 来源冲突，不覆盖原请求」——停在未决之后，同身份不同内容
// 仍答冲突，不被重放逻辑吞成接着办；原请求的内容摘要不变，也不形成撤回。
func TestAWithdrawalWithDifferentContentAfterAStoppedRoundStillConflicts(t *testing.T) {
	fixture, store, identities := newWithdrawalReplayFixture(t)
	fixture.authorizer.err = errors.New("authority unreachable")
	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle the round that stops: %v", err)
	}

	fixture.authorizer.err = nil
	conflicting := fixture.command(t)
	conflicting.PayloadDigest = mustValue(t, domain.NewPayloadDigest, "withdrawal-digest-2")
	authorizations := fixture.authorizer.calls

	result, err := fixture.handler.Handle(context.Background(), conflicting)
	if err != nil {
		t.Fatalf("handle the conflicting request: %v", err)
	}
	if result.Outcome() != application.WithdrawalSourceConflict {
		t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
	}
	if fixture.authorizer.calls != authorizations || identities.issued != 0 || store.saves != 0 {
		t.Fatal("a conflicting request was carried on instead of refused")
	}
	if preserved, _ := fixture.sources.stored(t, withdrawalSourceIdentity(t)); preserved.Digest().String() != "withdrawal-digest-1" {
		t.Fatalf("preserved digest = %q; the conflicting request overwrote the original source", preserved.Digest())
	}
}

// Covers: 指名查不到的委托是调用方的错——重放也答统一不可见，不是已有结果。
func TestAReplayedWithdrawalForAnUnknownRequestIsStillInvisible(t *testing.T) {
	fixture := newWithdrawalFixture(t)
	fixture.authorizer.err = errors.New("authority unreachable")
	if _, err := fixture.handler.Handle(context.Background(), fixture.command(t)); err != nil {
		t.Fatalf("handle the round that stops: %v", err)
	}

	fixture.requests.missing = true
	_, err := fixture.handler.Handle(context.Background(), fixture.command(t))
	if !errors.Is(err, application.ErrShipmentRequestNotVisible) {
		t.Fatalf("err = %v, want ErrShipmentRequestNotVisible", err)
	}
}
