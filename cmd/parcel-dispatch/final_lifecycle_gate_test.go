package main

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

// 本文件对真实 PostgreSQL 证终局采用编排的生命周期闸门经生产装配（parcelFinalAdoptionChain）起作用（票
// docs-consistency/code/101）：委托经领域真的越过各个生命周期状态再经生产仓储落库，编排读生产委托仓储。终局只为`已接受`
// 的委托形成——其余三态答确定格，终局库与发布意图一行都不落；已接受的委托走过闸门，停在生产的诚实停点
// FINAL_RULE_UNCONFIGURED（PAR-COM-17 实例半边，本用例不种终局声明）。

func finalCommandForSYN(t *testing.T, fixture *synVerticalFixture) psapplication.FormParcelFinalCommand {
	t.Helper()
	return psapplication.FormParcelFinalCommand{
		Identity:          fixture.identity,
		ShipmentRequestID: fixture.requestID,
		Outcome: psdomain.ResponsibilityOutcomeSpec{
			Kind:       psdomain.EffectiveDeliveryOutcome,
			Parcel:     mustPS(t, psdomain.NewDeclaredParcelID, effectiveDeliveryObject),
			Decision:   mustPS(t, psdomain.NewResponsibilityDecisionReference, "SYN-DELIVERY-JUDGMENT/1"),
			Execution:  mustPS(t, psdomain.NewExecutionEvidenceReference, "SYN-POD/1"),
			Version:    mustPS(t, psdomain.NewResponsibilityOutcomeVersion, effectiveDeliveryVersion),
			OccurredAt: time.Date(2026, 8, 24, 9, 0, 0, 0, time.UTC),
		},
	}
}

// moveRequestTo 把已提交的委托经领域转移推到目标状态，再经生产仓储保存。
func moveRequestTo(t *testing.T, fixture *synVerticalFixture, state string) {
	t.Helper()
	ctx := t.Context()
	decidedAt := time.Date(2026, 8, 24, 8, 0, 0, 0, time.UTC)
	switch state {
	case "submitted":
		return
	case "accepted":
		fixture.recordPassingJudgments(t, ctx)
		if result := fixture.formDecision(t, ctx); result.State() != psdomain.ShipmentRequestAccepted {
			t.Fatalf("state = %q, want ACCEPTED；pending = %q", result.State(), result.PendingReason())
		}
		return
	}

	request := fixture.mustLoadRequest(t, ctx)
	var err error
	switch state {
	case "withdrawn":
		request, err = request.WithdrawByCustomer(psdomain.WithdrawalSpec{
			DecisionID: mustPS(t, psdomain.NewAcceptanceDecisionID, "SYN-WITHDRAWAL-1"),
			Authority:  mustPS(t, psdomain.NewWithdrawalAuthorityReference, "SYN-WITHDRAW-ROLE-1"),
			Requester:  mustPS(t, psdomain.NewWithdrawalRequesterReference, "SYN-CUSTOMER-1"),
			Reason:     mustPS(t, psdomain.NewWithdrawalReasonReference, "SYN-NO-LONGER-REQUIRED"),
			DecidedAt:  decidedAt,
		})
	case "rejected":
		request, err = request.RejectByAuthority(psdomain.ActiveRejectionSpec{
			DecisionID: mustPS(t, psdomain.NewAcceptanceDecisionID, "SYN-REJECTION-1"),
			Authority:  mustPS(t, psdomain.NewRejectionAuthorityReference, "SYN-REJECT-ROLE-1"),
			Decider:    mustPS(t, psdomain.NewDeciderReference, "SYN-OPERATOR-1"),
			Reason:     mustPS(t, psdomain.NewRejectionReasonReference, "SYN-NOT-UNDERTAKEN"),
			Evidence:   mustPS(t, psdomain.NewRejectionEvidenceReference, "SYN-EVIDENCE-1"),
			DecidedAt:  decidedAt,
		})
	default:
		t.Fatalf("未知的委托状态 %q", state)
	}
	if err != nil {
		t.Fatalf("领域转移到 %s：%v", state, err)
	}
	mustWithinTX(t, fixture.transactor, ctx, func(txCtx context.Context) error {
		saved, saveErr := fixture.requests.Save(txCtx, fixture.identity, request)
		if saveErr != nil {
			return saveErr
		}
		if saved != psports.ShipmentRequestSaved {
			return fmt.Errorf("save outcome = %v", saved)
		}
		return nil
	})
}

// Covers: PS CONTEXT「委托」生命周期与 UC-PS-004 结果语义「委托已终结」「委托待决定」经生产装配对真库——已撤回、已拒绝的委托
// 为其包裹形成终局答 `REQUEST_ENDED`，只是已提交的答 `REQUEST_AWAITING_DECISION`；终局库与发布意图一行都不落，规则读口
// 之前就被拦。已接受的委托走过闸门，停在终局规则未配置。
func TestTheWiredFinalAdoptionFormsNoFinalForRequestsThatAreNotAccepted(t *testing.T) {
	cases := []struct {
		state   string
		outcome psapplication.ParcelFinalAdoptionOutcome
		reason  psapplication.FinalUndecidedReason
	}{
		{"withdrawn", psapplication.FinalRequestEnded, psapplication.FinalUndecidedReasonNone},
		{"rejected", psapplication.FinalRequestEnded, psapplication.FinalUndecidedReasonNone},
		{"submitted", psapplication.FinalRequestAwaitingDecision, psapplication.FinalUndecidedReasonNone},
		{"accepted", psapplication.ParcelFinalUndecided, psapplication.FinalRuleUnconfigured},
	}
	for _, tc := range cases {
		t.Run(tc.state, func(t *testing.T) {
			fixture := newSYNVerticalFixture(t)
			fixture.submit(t, t.Context())
			moveRequestTo(t, fixture, tc.state)

			store, err := outbox.NewStore(fixture.db)
			if err != nil {
				t.Fatalf("构造 Outbox Store：%v", err)
			}
			chain, err := parcelFinalAdoptionChain(fixture.db, store, systemClock{})
			if err != nil {
				t.Fatalf("装配终局采用编排：%v", err)
			}

			var result psapplication.FormParcelFinalResult
			mustWithinTX(t, fixture.transactor, t.Context(), func(txCtx context.Context) error {
				var handleErr error
				result, handleErr = chain.handler.Handle(txCtx, finalCommandForSYN(t, fixture))
				return handleErr
			})

			if result.Outcome() != tc.outcome || result.UndecidedReason() != tc.reason {
				t.Fatalf("outcome = %q/%q, want %q/%q", result.Outcome(), result.UndecidedReason(), tc.outcome, tc.reason)
			}
			// 消费门的落点：确定格入账收工（重投换不来另一个答案），规则未配置仍是未决哨兵。
			consumed := finalconsume.Consumption(result)
			if tc.outcome == psapplication.ParcelFinalUndecided {
				if !errors.Is(consumed, finalconsume.ErrFinalUndecided) {
					t.Fatalf("消费门 = %v, want ErrFinalUndecided", consumed)
				}
			} else if consumed != nil {
				t.Fatalf("消费门 = %v; 被拦的终局是确定答复，应入账收工", consumed)
			}
			if n := fixture.countSQL(t, `SELECT count(*) FROM parcel_shipment.final_outcome`); n != 0 {
				t.Fatalf("final_outcome 行数 = %d, want 0", n)
			}
			if n := fixture.countOutboxOfType(t, finalOutcomeFormedType); n != 0 {
				t.Fatalf("发出了 %d 封 %s", n, finalOutcomeFormedType)
			}
		})
	}
}
