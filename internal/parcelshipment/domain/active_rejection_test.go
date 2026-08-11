package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

var rejectedByAuthorityAt = time.Date(2026, 8, 7, 13, 0, 0, 0, time.UTC)

func activeRejectionSpec(t *testing.T) domain.ActiveRejectionSpec {
	t.Helper()
	return domain.ActiveRejectionSpec{
		DecisionID: mustValue(t, domain.NewAcceptanceDecisionID, "decision-1"),
		Authority:  mustValue(t, domain.NewRejectionAuthorityReference, "PC-REJECT-ROLE-1"),
		Decider:    mustValue(t, domain.NewDeciderReference, "OPERATOR-1"),
		Reason:     mustValue(t, domain.NewRejectionReasonReference, "OPERATOR_DECLINED_SERVICE"),
		Evidence:   mustValue(t, domain.NewRejectionEvidenceReference, "EVID-1"),
		DecidedAt:  rejectedByAuthorityAt,
	}
}

// Covers: UC-PS-001 步骤 8「授权角色可以依据结构化原因主动拒绝」与 9B「拒绝时固定原因、
// 适用规则、证据和决定依据」— 主动拒绝形成的是拒绝决定，且四项留痕全部读得回来。
func TestAnAuthorizedActiveRejectionFormsARejection(t *testing.T) {
	rejected, err := submitted(t).RejectByAuthority(activeRejectionSpec(t))
	if err != nil {
		t.Fatalf("reject by authority: %v", err)
	}

	if rejected.State() != domain.ShipmentRequestRejected {
		t.Fatalf("state = %q, want REJECTED", rejected.State())
	}
	decision, present := rejected.AcceptanceDecision()
	if !present || decision.Accepted() {
		t.Fatalf("decision = %#v present = %v", decision, present)
	}
	active, isActive := decision.ActiveRejection()
	if !isActive {
		t.Fatal("an operator rejection was recorded as if the rules had produced it")
	}
	if active.Authority().String() == "" || active.Decider().String() == "" ||
		active.Reason().String() == "" || active.Evidence().String() == "" {
		t.Fatalf("active rejection lost part of its audit trail: %#v", active)
	}
	// 主动拒绝同样让接受判断任务收工：决定已经越过提交边界，任务再续办就是在判一份已决委托。
	if !rejected.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("an active rejection left the acceptance judgment task open")
	}
}

// Covers: CONTEXT「普通备注、口头意见或未经授权的操作不能形成拒绝事实」— 授权依据、实际
// 决定方、结构化原因与证据四项缺一不可。少任一项，这次拒绝就与一句「不做了」分不开。
func TestAnActiveRejectionWithoutItsAuditTrailCannotBeFormed(t *testing.T) {
	missing := map[string]func(domain.ActiveRejectionSpec) domain.ActiveRejectionSpec{
		"authority": func(s domain.ActiveRejectionSpec) domain.ActiveRejectionSpec {
			s.Authority = domain.RejectionAuthorityReference{}
			return s
		},
		"decider": func(s domain.ActiveRejectionSpec) domain.ActiveRejectionSpec {
			s.Decider = domain.DeciderReference{}
			return s
		},
		"reason": func(s domain.ActiveRejectionSpec) domain.ActiveRejectionSpec {
			s.Reason = domain.RejectionReasonReference{}
			return s
		},
		"evidence": func(s domain.ActiveRejectionSpec) domain.ActiveRejectionSpec {
			s.Evidence = domain.RejectionEvidenceReference{}
			return s
		},
	}

	for name, drop := range missing {
		t.Run(name, func(t *testing.T) {
			if _, err := submitted(t).RejectByAuthority(drop(activeRejectionSpec(t))); !errors.Is(
				err, domain.ErrInvalidActiveRejection,
			) {
				t.Fatalf("error = %v, want ErrInvalidActiveRejection", err)
			}
		})
	}
}

// Covers: CONTEXT「主动拒绝……只能在委托仍为`已提交`时与自动接受竞争同一个决定提交边界。
// 当前提交版本只能有一个合法决定；接受或拒绝提交后，另一方只能读取既有结果，不能追加相反
// 决定」— 两个方向都要挡：已接受后不能补一个主动拒绝，主动拒绝后也不能再自动接受。
func TestAnActiveRejectionCompetesForTheSingleDecisionBoundary(t *testing.T) {
	t.Run("cannot follow an acceptance", func(t *testing.T) {
		accepted, err := submitted(t).Decide(decisionSpec(t, allGroupsPassing(t)))
		if err != nil {
			t.Fatalf("decide: %v", err)
		}
		if _, err := accepted.RejectByAuthority(activeRejectionSpec(t)); !errors.Is(
			err, domain.ErrDecisionAlreadyFormed,
		) {
			t.Fatalf("error = %v, want ErrDecisionAlreadyFormed", err)
		}
	})

	t.Run("automatic acceptance cannot follow it", func(t *testing.T) {
		rejected, err := submitted(t).RejectByAuthority(activeRejectionSpec(t))
		if err != nil {
			t.Fatalf("reject by authority: %v", err)
		}
		if _, err := rejected.Decide(decisionSpec(t, allGroupsPassing(t))); !errors.Is(
			err, domain.ErrDecisionAlreadyFormed,
		) {
			t.Fatalf("error = %v, want ErrDecisionAlreadyFormed", err)
		}
	})

	t.Run("a second active rejection is refused", func(t *testing.T) {
		rejected, err := submitted(t).RejectByAuthority(activeRejectionSpec(t))
		if err != nil {
			t.Fatalf("reject by authority: %v", err)
		}
		if _, err := rejected.RejectByAuthority(activeRejectionSpec(t)); !errors.Is(
			err, domain.ErrDecisionAlreadyFormed,
		) {
			t.Fatalf("error = %v, want ErrDecisionAlreadyFormed", err)
		}
	})
}

// Covers: CONTEXT「委托接受基线冻结的是客户声明的服务范围」的反面 — 拒绝不形成接受基线与
// 预计承诺。拒绝没有承诺可作，凭空造一个会让下游按一份不存在的承诺办事。
func TestAnActiveRejectionFormsNoBaselineOrCommitment(t *testing.T) {
	rejected, err := submitted(t).RejectByAuthority(activeRejectionSpec(t))
	if err != nil {
		t.Fatalf("reject by authority: %v", err)
	}

	if _, present := rejected.AcceptanceBaseline(); present {
		t.Fatal("a rejection fixed an acceptance baseline")
	}
	if _, present := rejected.ExpectedCommitment(); present {
		t.Fatal("a rejection formed an expected commitment")
	}
}

// Covers: `WaitingOn` 自身的契约「本轮形成了决定或任务已完成时报告缺席」，以及 ADR-0028
// 列为非法的跨字段组合「`waitingOn` 有值而决定已形成」。
//
// 主动拒绝可以落在一轮未决之后，而那一轮留下了续办路径。另两个终态转移都自己清零，只有
// 这里没清，于是一份已拒绝、任务已完成的委托仍会交回一条活的续办路径——编排照它续办，
// 就是在续办一份已决委托。
func TestAnActiveRejectionClearsTheResumePathLeftByAnUndeterminedRound(t *testing.T) {
	undecided, err := submitted(t).Decide(decisionSpec(t, []domain.AcceptanceCheck{
		undeterminedCheck(t, domain.CustomerRelationshipCheck, "DEPENDENCY_TIMEOUT", domain.ResumeByInternalRetry),
	}))
	if err != nil {
		t.Fatalf("decide: %v", err)
	}
	if _, waiting := undecided.AcceptanceDecisionTask().WaitingOn(); !waiting {
		t.Fatal("前置不成立：未决的一轮没有留下续办路径")
	}

	rejected, err := undecided.RejectByAuthority(activeRejectionSpec(t))
	if err != nil {
		t.Fatalf("reject by authority: %v", err)
	}

	if !rejected.AcceptanceDecisionTask().IsComplete() {
		t.Fatal("an active rejection left the acceptance judgment task open")
	}
	if path, waiting := rejected.AcceptanceDecisionTask().WaitingOn(); waiting {
		t.Fatalf("waiting on %q; 决定已形成、任务已完成，续办路径却仍然活着", path)
	}
}
