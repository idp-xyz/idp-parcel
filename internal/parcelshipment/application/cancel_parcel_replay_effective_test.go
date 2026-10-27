package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// anotherCancellation 是同一件包裹上的另一份取消（标识不同），替身的有效取消读答它，说明被重放的那份已不是有效取消。
func anotherCancellation(t *testing.T) domain.ParcelCancellation {
	t.Helper()
	cancellation, err := domain.DecideParcelCancellation(domain.ParcelCancellationSpec{
		ID:          mustValue(t, domain.NewParcelCancellationID, "cancel-9"),
		Parcel:      mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		Requester:   mustValue(t, domain.NewCancellationRequesterReference, "customer-1"),
		Authority:   mustValue(t, domain.NewCancellationAuthorityReference, "CANCEL-RULE/PC-17"),
		Reason:      mustValue(t, domain.NewCancellationReasonReference, "CUSTOMER_CHANGED_MIND"),
		RequestedAt: cancelRequestedAt,
	}, domain.CurrentIntakeFact{})
	if err != nil {
		t.Fatalf("decide parcel cancellation: %v", err)
	}
	return cancellation
}

// Covers: AT-PS-168；ADR-0295 决定三——同一请求键重放一份取消：答复永远是历史答复（EXISTING_RESULT 加原记录 PARCEL_CANCELLED），但下游释放意图只在这份取消
// 此刻仍是有效取消时才重发。取消形成时意图交失败（留了续办引用），随后这份取消被认定依据不成立：重放不把释放意图交出去，也不留续办引用；
// 有效取消读答的是同一包裹上的另一份取消，同样不重发；有效取消读读不动，答未决带续办引用，不假装交了或没交。
func TestAReplayOfACancellationThatIsNoLongerEffectiveHandsOffNothing(t *testing.T) {
	formWithFailedHandoff := func(t *testing.T) (*cancelFixture, application.CancelParcelResult) {
		t.Helper()
		fixture := newCancelFixture(t)
		fixture.downstream.err = errors.New("outbox down")
		first, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("first handle: %v", err)
		}
		if first.Outcome() != application.ParcelCancelled || first.CancellationHandoffReference().String() == "" {
			t.Fatalf("first = %q handoff-ref = %q, want 取消成立且留了续办引用", first.Outcome(), first.CancellationHandoffReference().String())
		}
		fixture.downstream.err = nil
		return fixture, first
	}
	assertHistoricalAnswerWithoutHandoff := func(t *testing.T, fixture *cancelFixture, replay application.CancelParcelResult) {
		t.Helper()
		record, has := replay.Record()
		if replay.Outcome() != application.CancellationExistingResult || !has || record.Kind != ports.RecordParcelCancelled {
			t.Fatalf("replay = %q record-kind = %v, want EXISTING_RESULT 加原记录 PARCEL_CANCELLED", replay.Outcome(), record.Kind)
		}
		if len(fixture.downstream.intents) != 0 {
			t.Errorf("取消已不是有效取消，重放仍把释放意图交了出去（%d 份）", len(fixture.downstream.intents))
		}
		if replay.CancellationHandoffReference().String() != "" {
			t.Errorf("重放留了续办引用 %q——没有要交的意图", replay.CancellationHandoffReference().String())
		}
	}

	t.Run("set aside: no effective cancellation", func(t *testing.T) {
		fixture, _ := formWithFailedHandoff(t)
		// 有效取消读缺省答没有，即这份取消已被认定依据不成立。
		replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		assertHistoricalAnswerWithoutHandoff(t, fixture, replay)
		if fixture.store.saved != 1 {
			t.Errorf("saved = %d, want 1——重放不重复决定", fixture.store.saved)
		}
	})

	t.Run("set aside: another cancellation is the effective one", func(t *testing.T) {
		fixture, _ := formWithFailedHandoff(t)
		fixture.cancellations.cancellation = anotherCancellation(t)
		fixture.cancellations.cancelled = true
		replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		assertHistoricalAnswerWithoutHandoff(t, fixture, replay)
	})

	t.Run("still effective: the replay re-hands the same intent", func(t *testing.T) {
		fixture, _ := formWithFailedHandoff(t)
		fixture.cancellations.cancellation = cancelledAt(t, cancelRequestedAt)
		fixture.cancellations.cancelled = true
		replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		if replay.Outcome() != application.CancellationExistingResult || len(fixture.downstream.intents) != 1 ||
			replay.CancellationHandoffReference().String() != "" {
			t.Fatalf("replay = %q intents = %d handoff-ref = %q, want 已有结果、重发一份、无续办引用",
				replay.Outcome(), len(fixture.downstream.intents), replay.CancellationHandoffReference().String())
		}
	})

	t.Run("effective read unavailable: undecided with a continuation", func(t *testing.T) {
		fixture, _ := formWithFailedHandoff(t)
		fixture.cancellations.err = errors.New("cancellation view down")
		replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		// 幂等读之后的第一道读就是有效取消读：读不动时历史答复之外的意图交不交说不准，答未决。
		if replay.Outcome() != application.CancellationUndecided || replay.UndecidedReason() != application.CancelStoreUnavailable ||
			replay.ContinuationReference().String() == "" {
			t.Fatalf("replay = %q reason = %q continuation = %q, want 未决 CANCELLATION_STORE_UNAVAILABLE 带续办引用",
				replay.Outcome(), replay.UndecidedReason(), replay.ContinuationReference().String())
		}
		if len(fixture.downstream.intents) != 0 {
			t.Errorf("读不动有效取消却交了意图 %d 份", len(fixture.downstream.intents))
		}
	})
}

// Covers: AT-PS-168；ADR-0295 决定三——有效取消只管`包裹已取消`的记录：拒绝与待处置的记录没有下游释放意图，重放它们不问有效取消。即便有效取消读此刻读不动，
// 重放仍答已有结果（历史答复），不被拖进未决。
func TestAReplayOfARefusalOrAPendingDispositionDoesNotAskForTheEffectiveCancellation(t *testing.T) {
	for name, arrange := range map[string]struct {
		prepare func(t *testing.T, fixture *cancelFixture)
		want    ports.CancellationRecordKind
	}{
		"拒绝": {
			prepare: func(t *testing.T, fixture *cancelFixture) {
				fixture.authority.judgment = ports.CancellationAuthorityJudgment{
					Granted: false, Basis: mustValue(t, domain.NewCheckReason, "CANCEL-RULE/DENY"),
				}
			},
			want: ports.RecordCancellationRefused,
		},
		"待处置": {
			prepare: func(t *testing.T, fixture *cancelFixture) {
				adoptParcel(t, fixture.adoptions, "parcel-1", cancelRequestedAt.Add(-time.Hour))
			},
			want: ports.RecordDispositionPending,
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newCancelFixture(t)
			arrange.prepare(t, fixture)
			first, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("first handle: %v", err)
			}
			if record, has := first.Record(); !has || record.Kind != arrange.want {
				t.Fatalf("first = %q record-kind = %v, want %v", first.Outcome(), record.Kind, arrange.want)
			}

			fixture.cancellations.err = errors.New("cancellation view down")
			asked := fixture.cancellations.asked
			replay, err := fixture.handler.Handle(context.Background(), cancelCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("replay handle: %v", err)
			}
			record, has := replay.Record()
			if replay.Outcome() != application.CancellationExistingResult || !has || record.Kind != arrange.want {
				t.Fatalf("replay = %q record-kind = %v, want EXISTING_RESULT 加原记录 %v——读不动有效取消不该影响不涉及它的记录",
					replay.Outcome(), record.Kind, arrange.want)
			}
			if fixture.cancellations.asked != asked {
				t.Errorf("重放问了有效取消读 %d 次, want 0", fixture.cancellations.asked-asked)
			}
			if len(fixture.downstream.intents) != 0 {
				t.Errorf("拒绝与待处置没有释放意图，却交了 %d 份", len(fixture.downstream.intents))
			}
		})
	}
}
