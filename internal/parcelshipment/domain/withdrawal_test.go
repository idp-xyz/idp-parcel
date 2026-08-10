package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var withdrawnAt = time.Date(2026, 8, 8, 9, 0, 0, 0, time.UTC)

func withdrawalSpec(t *testing.T) domain.WithdrawalSpec {
	t.Helper()
	return domain.WithdrawalSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Authority:  mustValue(t, domain.NewWithdrawalAuthorityReference, "PC-WITHDRAW-ROLE-1"),
		Requester:  mustValue(t, domain.NewWithdrawalRequesterReference, "CUSTOMER-CONTACT-1"),
		Reason:     mustValue(t, domain.NewWithdrawalReasonReference, "CUSTOMER_NO_LONGER_REQUIRES_SERVICE"),
		DecidedAt:  withdrawnAt,
	}
}

// Covers: CONTEXT 委托生命周期「已提交 → 已撤回」与 UC-PS-005 `AT-PS-067`「已提交且尚无决定，
// 客户授权有效 → 形成撤回和任务停止，不形成拒绝」。
func TestACustomerWithdrawsAStillUndecidedRequest(t *testing.T) {
	withdrawn, err := submitted(t).WithdrawByCustomer(withdrawalSpec(t))
	if err != nil {
		t.Fatalf("withdraw by customer: %v", err)
	}

	if withdrawn.State() != domain.ShipmentRequestWithdrawn {
		t.Fatalf("state = %q, want WITHDRAWN", withdrawn.State())
	}
	if withdrawn.State() == domain.ShipmentRequestRejected {
		t.Fatal("a customer withdrawal was recorded as an operator rejection")
	}
	if _, present := withdrawn.AcceptanceDecision(); present {
		t.Fatal("a withdrawal formed an acceptance decision; CONTEXT forbids it masquerading as accept or reject")
	}
	record, present := withdrawn.Withdrawal()
	if !present {
		t.Fatal("a withdrawn request kept no withdrawal record")
	}
	if record.Authority().String() == "" || record.Requester().String() == "" ||
		record.Reason().String() == "" || record.DecidedAt().IsZero() {
		t.Fatalf("withdrawal lost part of its audit trail: %#v", record)
	}
}

// Covers: CONTEXT「委托撤回决定成立 → 判断任务已停止」与「判断任务 → 已完成：只有接受或拒绝
// 决定已经越过提交边界时完成」— 停止与完成是两个终态，撤回只到停止。
func TestAWithdrawalStopsTheJudgmentTaskWithoutCompletingIt(t *testing.T) {
	withdrawn, err := submitted(t).WithdrawByCustomer(withdrawalSpec(t))
	if err != nil {
		t.Fatalf("withdraw by customer: %v", err)
	}

	task := withdrawn.AcceptanceDecisionTask()
	if !task.IsStopped() {
		t.Fatal("a withdrawal left the acceptance judgment task running")
	}
	if task.IsComplete() {
		t.Fatal("a withdrawal completed the task; only an accept or reject crossing the boundary completes it")
	}
}

// Covers: CONTEXT「撤回、接受和拒绝竞争同一个不可覆盖决定边界；已经合法提交的首个决定获胜」
// 与 UC-PS-005 `AT-PS-071`/`AT-PS-072`——决定先到时后到的撤回只能读既有结果。
func TestAWithdrawalCannotOverwriteADecisionThatAlreadyCrossedTheBoundary(t *testing.T) {
	rejected, err := submitted(t).RejectByAuthority(activeRejectionSpec(t))
	if err != nil {
		t.Fatalf("reject by authority: %v", err)
	}

	if _, err := rejected.WithdrawByCustomer(withdrawalSpec(t)); !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		t.Fatalf("err = %v, want ErrDecisionAlreadyFormed; a late withdrawal overwrote a decision that already won", err)
	}
}

// Covers: CONTEXT「撤回、接受和拒绝竞争同一个不可覆盖决定边界」的另一侧——撤回先到时，
// 自动接受与主动拒绝都不得再成立。
func TestADecisionCannotFormAfterAWithdrawalWon(t *testing.T) {
	withdrawn, err := submitted(t).WithdrawByCustomer(withdrawalSpec(t))
	if err != nil {
		t.Fatalf("withdraw by customer: %v", err)
	}

	if _, err := withdrawn.RejectByAuthority(activeRejectionSpec(t)); !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		t.Fatalf("reject err = %v, want ErrDecisionAlreadyFormed", err)
	}
	if _, err := withdrawn.Decide(decisionSpec(t, allGroupsPassing(t))); !errors.Is(err, domain.ErrDecisionAlreadyFormed) {
		t.Fatalf("decide err = %v, want ErrDecisionAlreadyFormed", err)
	}
}

// Covers: UC-PS-005「客户备注或连接中断不构成撤回」与输入语义表要求的请求方、授权、原因——
// 三项留痕缺一不可，少了就与一句「算了吧」分不开。
func TestAWithdrawalWithoutItsAuditTrailCannotBeFormed(t *testing.T) {
	missing := map[string]func(domain.WithdrawalSpec) domain.WithdrawalSpec{
		"authority": func(s domain.WithdrawalSpec) domain.WithdrawalSpec {
			s.Authority = domain.WithdrawalAuthorityReference{}
			return s
		},
		"requester": func(s domain.WithdrawalSpec) domain.WithdrawalSpec {
			s.Requester = domain.WithdrawalRequesterReference{}
			return s
		},
		"reason": func(s domain.WithdrawalSpec) domain.WithdrawalSpec {
			s.Reason = domain.WithdrawalReasonReference{}
			return s
		},
	}

	for name, drop := range missing {
		t.Run(name, func(t *testing.T) {
			if _, err := submitted(t).WithdrawByCustomer(drop(withdrawalSpec(t))); !errors.Is(err, domain.ErrInvalidWithdrawal) {
				t.Fatalf("err = %v, want ErrInvalidWithdrawal when %s is absent", err, name)
			}
		})
	}
}
