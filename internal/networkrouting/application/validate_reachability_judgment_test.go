package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

func recordedJudgment(t *testing.T, parcel, revision string) ports.ReachabilityJudgmentRecord {
	t.Helper()
	finding, err := domain.ConcludeReachability([]domain.RouteCandidate{qualifiedCandidate(t, "candidate-1")}, nil)
	if err != nil {
		t.Fatalf("conclude reachability: %v", err)
	}
	return ports.ReachabilityJudgmentRecord{
		Key:          judgmentKey(t, parcel),
		Finding:      finding,
		JudgedAt:     judgedAt,
		ViewRevision: value(t, domain.NewNetworkViewRevision, revision),
	}
}

func validateCommand(t *testing.T, parcel string) application.ValidateReachabilityJudgmentCommand {
	t.Helper()
	return application.ValidateReachabilityJudgmentCommand{
		Correlation: value(t, domain.NewRequestCorrelationID, "correlation-1"),
		Key:         judgmentKey(t, parcel),
	}
}

// Covers: `AT-PS-037`「可达性或其他关键判断形成后、接受提交前被有效新版本或限制推翻 →
// 原结果不再用于接受并重新判断」的 NR 半边——视图没换代时确认原判断（携带原结论与原判断
// 时间），换代即`已换代`且不携带结论：交回一份，调用方会以为可以继续用它，而 NR CONTEXT
// 要它「请求新的判断版本」。
func TestAValidationConfirmsOrSupersedesByTheViewRevision(t *testing.T) {
	t.Run("same revision confirms the original judgment", func(t *testing.T) {
		store := &storeDouble{found: true, existing: recordedJudgment(t, "parcel-1", "net-view-rev-1")}
		evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-2")}
		handler := application.NewValidateReachabilityJudgmentHandler(evidence, store)

		result, err := handler.Handle(context.Background(), validateCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}

		if result.Outcome() != application.JudgmentStillCurrent {
			t.Fatalf("outcome = %q, want JUDGMENT_STILL_CURRENT", result.Outcome())
		}
		finding, present := result.Finding()
		if !present || finding.Value() != domain.Reachable {
			t.Fatalf("finding = %#v present = %v, want the original judgment carried back", finding, present)
		}
		if !result.JudgedAt().Equal(judgedAt) {
			t.Fatalf("judged at = %s, want the original %s——确认不是一次新判断", result.JudgedAt(), judgedAt)
		}
	})

	t.Run("a changed revision supersedes without carrying the finding", func(t *testing.T) {
		store := &storeDouble{found: true, existing: recordedJudgment(t, "parcel-1", "net-view-rev-0")}
		evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-2")}
		handler := application.NewValidateReachabilityJudgmentHandler(evidence, store)

		result, err := handler.Handle(context.Background(), validateCommand(t, "parcel-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}

		if result.Outcome() != application.JudgmentSuperseded {
			t.Fatalf("outcome = %q, want JUDGMENT_SUPERSEDED", result.Outcome())
		}
		if _, present := result.Finding(); present {
			t.Fatal("已换代的答复携带了原结论——调用方会拿它继续往下走")
		}
	})
}

// Covers: 「查无此关联」与「范围不符」同一个答案——可区分即可枚举同租户下别人的判断
// （ADR-0029 的合并理由在重校取回这一步同样成立）。
func TestAMissingOrForeignJudgmentIsUniformlyNotFound(t *testing.T) {
	cases := map[string]*storeDouble{
		"never formed":  {},
		"another scope": {found: true, existing: recordedJudgment(t, "parcel-9", "net-view-rev-1")},
	}
	for name, store := range cases {
		t.Run(name, func(t *testing.T) {
			evidence := &evidenceDouble{areas: coveringAreas(t, "candidate-2")}
			handler := application.NewValidateReachabilityJudgmentHandler(evidence, store)

			result, err := handler.Handle(context.Background(), validateCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.JudgmentNotFound {
				t.Fatalf("outcome = %q, want JUDGMENT_NOT_FOUND", result.Outcome())
			}
			if evidence.assembled != 0 {
				t.Fatal("判断都没找回却去装配了证据")
			}
		})
	}
}

// Covers: 权威读不到既不确认也不断言换代——判成换代会让调用方重判一份好好的判断，判成
// 仍然当前是免检放行；两个读不回各有原因与续办引用。库里没有比对锚的旧记录同样未决。
func TestAnUnreadableAuthorityKeepsTheValidationUnformed(t *testing.T) {
	cases := map[string]struct {
		store    *storeDouble
		evidence *evidenceDouble
		want     application.NotFormedReason
	}{
		"store unreadable": {
			store:    &storeDouble{findErr: errors.New("store down")},
			evidence: &evidenceDouble{areas: coveringAreas(t, "candidate-2")},
			want:     application.JudgmentStoreUnavailable,
		},
		"evidence unreadable": {
			store:    &storeDouble{found: true, existing: recordedJudgment(t, "parcel-1", "net-view-rev-1")},
			evidence: &evidenceDouble{err: errors.New("evidence down")},
			want:     application.NetworkEvidenceUnavailable,
		},
		"record without an anchor": {
			store: &storeDouble{found: true, existing: ports.ReachabilityJudgmentRecord{
				Key:      judgmentKey(t, "parcel-1"),
				Finding:  recordedJudgment(t, "parcel-1", "net-view-rev-1").Finding,
				JudgedAt: judgedAt,
			}},
			evidence: &evidenceDouble{areas: coveringAreas(t, "candidate-2")},
			want:     application.JudgmentStoreUnavailable,
		},
	}

	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			handler := application.NewValidateReachabilityJudgmentHandler(testCase.evidence, testCase.store)

			result, err := handler.Handle(context.Background(), validateCommand(t, "parcel-1"))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}

			if result.Outcome() != application.ValidationNotFormed {
				t.Fatalf("outcome = %q, want VALIDATION_NOT_FORMED", result.Outcome())
			}
			if result.NotFormedReason() != testCase.want {
				t.Fatalf("reason = %q, want %q", result.NotFormedReason(), testCase.want)
			}
			if result.ContinuationReference().String() == "" {
				t.Fatal("未形成的重校无法安全续办")
			}
		})
	}
}

// Covers: 与形成判断同一条受理前提——最小身份不成立时不读任何权威。
func TestAnIncompleteValidationRequestIsNotAccepted(t *testing.T) {
	store := &storeDouble{found: true, existing: recordedJudgment(t, "parcel-1", "net-view-rev-1")}
	handler := application.NewValidateReachabilityJudgmentHandler(&evidenceDouble{}, store)

	command := validateCommand(t, "parcel-1")
	command.Correlation = domain.RequestCorrelationID{}

	result, err := handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}

	if result.Outcome() != application.ValidationNotAccepted {
		t.Fatalf("outcome = %q, want VALIDATION_NOT_ACCEPTED", result.Outcome())
	}
	if store.findCalled != 0 {
		t.Fatal("身份不成立却按关联查询了判断库")
	}
}
