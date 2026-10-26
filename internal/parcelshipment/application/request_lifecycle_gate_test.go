package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// rejectedRequest 是 submittedRequest 经授权角色主动拒绝形成的委托：假状态挡不住领域转移，也证明不了闸门读的是真终态。
func rejectedRequest(t *testing.T) domain.ShipmentRequest {
	t.Helper()
	rejected, err := submittedRequest(t).RejectByAuthority(domain.ActiveRejectionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "rejection-1"),
		Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-1"),
		Decider:    mustValue(t, domain.NewDeciderReference, "OPERATOR-1"),
		Reason:     mustValue(t, domain.NewRejectionReasonReference, "SERVICE_NOT_UNDERTAKEN"),
		Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "EVID-1"),
		DecidedAt:  time.Date(2026, 8, 7, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatalf("reject by authority: %v", err)
	}
	if rejected.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", rejected.State())
	}
	return rejected
}

// lifecycleCases 是三个非`已接受`的状态各自该答的确定格；取消与终局形成共用一套词（REQUEST_ENDED、
// REQUEST_AWAITING_DECISION），所以两张表同源。
type lifecycleCase struct {
	name    string
	request func(*testing.T) domain.ShipmentRequest
	ended   bool
}

func lifecycleCases() []lifecycleCase {
	return []lifecycleCase{
		{name: "withdrawn", request: withdrawnRequest, ended: true},
		{name: "rejected", request: rejectedRequest, ended: true},
		{name: "submitted", request: submittedRequest, ended: false},
	}
}

func holdCancelRequest(t *testing.T, fixture *cancelFixture, request domain.ShipmentRequest) {
	t.Helper()
	fixture.requests.records[sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")] = request
}

func holdFinalRequest(t *testing.T, fixture *finalFixture, request domain.ShipmentRequest) {
	t.Helper()
	fixture.requests.records[sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-1")] = request
}

// touchedByCancellation 数一次取消编排在提交边界上碰过的东西：取消库的读写、锁、边界上每一道读、下游意图。
func touchedByCancellation(fixture *cancelFixture) int {
	return fixture.store.lookups + fixture.store.saved + len(fixture.lock.locked) + fixture.finals.asked +
		fixture.cancellations.asked + fixture.adoptions.startsAsked + fixture.pickups.asked +
		len(fixture.downstream.intents)
}

// Covers: PS CONTEXT「委托」生命周期与 UC-PS-006 结果语义「委托已终结」「委托待决定」——只有`已接受`的委托才能取消其包裹。
// 已撤回、已拒绝答 REQUEST_ENDED（客户无事可做），只是已提交的答 REQUEST_AWAITING_DECISION（客户改用撤回）；都是确定的业务
// 答复，不是未决、不是统一不可见、不是字段缺。闸门在授权、取消库的幂等读、取边界锁与边界上每一道读之前：被拦时什么都不碰。
func TestACancellationIsClosedToRequestsThatAreNotAccepted(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newCancelFixture(t)
			holdCancelRequest(t, fixture, tc.request(t))
			// 授权读不动：闸门若落在授权之后，答的会是未决，不是这一格。
			fixture.authority.err = errors.New("authority unreachable")

			result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			want := application.CancellationRequestAwaitingDecision
			if tc.ended {
				want = application.CancellationRequestEnded
			}
			if result.Outcome() != want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), want)
			}
			if _, present := result.Record(); present ||
				result.UndecidedReason() != application.CancelUndecidedReasonNone ||
				result.ContinuationReference().String() != "" ||
				result.CancellationHandoffReference().String() != "" {
				t.Fatalf("result = %#v; 确定格不带记录、未决原因、续办引用或发布引用", result)
			}
			if n := touchedByCancellation(fixture); n != 0 {
				t.Fatalf("被拦的取消碰了 %d 处（取消库、锁、边界读、下游）", n)
			}
		})
	}

	t.Run("an accepted request still cancels", func(t *testing.T) {
		fixture := newCancelFixture(t)
		result, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ParcelCancelled || fixture.store.saved != 1 {
			t.Fatalf("outcome = %q saved = %d, want PARCEL_CANCELLED", result.Outcome(), fixture.store.saved)
		}
	})

	t.Run("a request accepted after the refusal is judged on its own boundary", func(t *testing.T) {
		fixture := newCancelFixture(t)
		holdCancelRequest(t, fixture, submittedRequest(t))
		first, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil || first.Outcome() != application.CancellationRequestAwaitingDecision {
			t.Fatalf("first = %q err = %v", first.Outcome(), err)
		}

		holdCancelRequest(t, fixture, acceptedRequest(t))
		second, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("second handle: %v", err)
		}
		if second.Outcome() != application.ParcelCancelled {
			t.Fatalf("second = %q; 早先的拦截不留记录，委托被接受后同一请求按接受后的边界判", second.Outcome())
		}
	})
}

// Covers: `AT-PS-090` 的隔离面落到应用层——目标委托不属命令里的信封账户，与查无、编号不符、成员出界同答统一不可见错误
// （逐字相同）；账户核在生命周期闸门之前，别人的已撤回委托不因此露出自己的状态；账户字段缺席答 REQUEST_NOT_ACCEPTED，
// 不是跳过账户核放行。
func TestACancellationChecksTheTargetBelongsToTheEnvelopeAccount(t *testing.T) {
	absent := func(t *testing.T) error {
		t.Helper()
		fixture := newCancelFixture(t)
		miss := cancelCommand(t, "parcel-1")
		miss.Identity = sourceIdentity(t, "tenant-1", "customer-1", "source-a", "key-unknown")
		_, err := fixture.handler.Handle(context.Background(), miss)
		if !errors.Is(err, application.ErrShipmentRequestNotVisible) {
			t.Fatalf("absent: err = %v", err)
		}
		return err
	}

	t.Run("another account's target answers like an absent one", func(t *testing.T) {
		fixture := newCancelFixture(t)
		probe := cancelCommand(t, "parcel-1")
		probe.CustomerAccount = mustValue(t, domain.NewCustomerAccountID, "customer-2")

		result, err := fixture.handler.Handle(context.Background(), probe)
		if !errors.Is(err, application.ErrShipmentRequestNotVisible) {
			t.Fatalf("err = %v, want ErrShipmentRequestNotVisible", err)
		}
		if err.Error() != absent(t).Error() {
			t.Fatalf("err = %q; 跨账户必须与查无逐字同答", err)
		}
		if result != (application.CancelParcelResult{}) {
			t.Fatalf("统一不可见不带结果：%#v", result)
		}
		if n := touchedByCancellation(fixture); n != 0 {
			t.Fatalf("看不见的委托上碰了 %d 处", n)
		}
	})

	t.Run("a withdrawn target of another account does not show its state", func(t *testing.T) {
		fixture := newCancelFixture(t)
		holdCancelRequest(t, fixture, withdrawnRequest(t))
		probe := cancelCommand(t, "parcel-1")
		probe.CustomerAccount = mustValue(t, domain.NewCustomerAccountID, "customer-2")

		_, err := fixture.handler.Handle(context.Background(), probe)
		if !errors.Is(err, application.ErrShipmentRequestNotVisible) || err.Error() != absent(t).Error() {
			t.Fatalf("err = %v; 别人的已撤回委托也只答统一不可见", err)
		}
	})

	t.Run("a missing account is not accepted and the check is not skipped", func(t *testing.T) {
		fixture := newCancelFixture(t)
		missing := cancelCommand(t, "parcel-1")
		missing.CustomerAccount = domain.CustomerAccountID{}

		result, err := fixture.handler.Handle(context.Background(), missing)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.CancellationNotAccepted {
			t.Fatalf("outcome = %q, want REQUEST_NOT_ACCEPTED——缺账户不得放行", result.Outcome())
		}
		if n := touchedByCancellation(fixture); n != 0 {
			t.Fatalf("请求立不起来，却碰了 %d 处", n)
		}
	})
}

// Covers: PS CONTEXT「委托」生命周期与 UC-PS-004 结果语义——终局只为`已接受`委托的包裹形成。已撤回、已拒绝答
// REQUEST_ENDED，只是已提交的答 REQUEST_AWAITING_DECISION；确定格，不落采用记录、不交发布意图、不派生完成摘要。
// 先过成员核验：别的委托的包裹仍答 REQUEST_NOT_ACCEPTED，不露生命周期状态。闸门不依赖取消机制是否装配。
func TestAFinalIsNotFormedForRequestsThatAreNotAccepted(t *testing.T) {
	for _, tc := range lifecycleCases() {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newFinalFixture(t)
			holdFinalRequest(t, fixture, tc.request(t))

			result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			want := application.FinalRequestAwaitingDecision
			if tc.ended {
				want = application.FinalRequestEnded
			}
			if result.Outcome() != want {
				t.Fatalf("outcome = %q, want %q", result.Outcome(), want)
			}
			if _, present := result.Record(); present || fixture.finals.saved != 0 || len(fixture.downstream.intents) != 0 {
				t.Fatalf("saved = %d intents = %d；被拦的终局什么都不落、不交意图", fixture.finals.saved, len(fixture.downstream.intents))
			}
			if _, present := result.Completion(); present {
				t.Fatal("被拦的终局派生了完成摘要")
			}
			if result.UndecidedReason() != application.FinalUndecidedReasonNone || result.ContinuationReference().String() != "" {
				t.Fatalf("确定格不带未决原因或续办引用：%#v", result)
			}
		})
	}

	t.Run("the gate does not depend on the cancellation machinery being wired", func(t *testing.T) {
		for _, tc := range lifecycleCases() {
			fixture := newFinalFixture(t)
			holdFinalRequest(t, fixture, tc.request(t))
			fixture.rebuild(func(deps *application.FormParcelFinalDeps) {
				deps.Cancellations = nil
				deps.BoundaryLock = nil
			})
			result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("%s: handle: %v", tc.name, err)
			}
			if result.Outcome() != application.FinalRequestEnded && result.Outcome() != application.FinalRequestAwaitingDecision {
				t.Fatalf("%s: outcome = %q; 取消机制没装配时闸门照拦", tc.name, result.Outcome())
			}
			if fixture.finals.saved != 0 {
				t.Fatalf("%s: 落了 %d 行", tc.name, fixture.finals.saved)
			}
		}
	})

	t.Run("an accepted request still forms the final, with or without the cancellation machinery", func(t *testing.T) {
		fixture := newFinalFixture(t)
		formed, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil || formed.Outcome() != application.ParcelFinalFormed {
			t.Fatalf("outcome = %q err = %v, want FINAL_FORMED", formed.Outcome(), err)
		}

		bare := newFinalFixture(t)
		bare.rebuild(func(deps *application.FormParcelFinalDeps) {
			deps.Cancellations = nil
			deps.BoundaryLock = nil
		})
		formed, err = bare.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil || formed.Outcome() != application.ParcelFinalFormed {
			t.Fatalf("未装配取消机制：outcome = %q err = %v, want FINAL_FORMED", formed.Outcome(), err)
		}
	})

	t.Run("membership is checked before the lifecycle", func(t *testing.T) {
		fixture := newFinalFixture(t)
		holdFinalRequest(t, fixture, withdrawnRequest(t))
		result, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-9"))
		if err != nil || result.Outcome() != application.FinalRequestNotAccepted {
			t.Fatalf("outcome = %q err = %v, want REQUEST_NOT_ACCEPTED", result.Outcome(), err)
		}
	})

	t.Run("a request accepted after the refusal forms its final on a later arrival", func(t *testing.T) {
		fixture := newFinalFixture(t)
		holdFinalRequest(t, fixture, submittedRequest(t))
		first, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil || first.Outcome() != application.FinalRequestAwaitingDecision {
			t.Fatalf("first = %q err = %v", first.Outcome(), err)
		}

		holdFinalRequest(t, fixture, acceptedRequest(t))
		second, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
		if err != nil || second.Outcome() != application.ParcelFinalFormed {
			t.Fatalf("second = %q err = %v; 早先的拦截不留采用记录，不占住同一来源", second.Outcome(), err)
		}
	})
}
