package application_test

import (
	"context"
	"testing"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 tf-face-algebra/07——交接更正链线性，一版最多被更正一次（照揽收更正的先例）。前版已有后继时，以另一个新
// 版本再更正它：同内容是那次更正的重放，交回已落下的后继；异内容是冲突，不在前版上再分一枝。
func TestAPredecessorThatAlreadyHasASuccessorIsNotCorrectedAgain(t *testing.T) {
	v1, v2, retry := "handover-result/parcel-1/v1", "handover-result/parcel-1/v2", "handover-result/parcel-1/v2-retry"
	cases := []struct {
		name   string
		change func(*application.CorrectTransportHandoverCommand)
		want   application.HandoverRegistrationOutcome
	}{
		{"the same correction under another new version", func(*application.CorrectTransportHandoverCommand) {}, application.HandoverExistingVersion},
		{"another correction of the same predecessor", func(command *application.CorrectTransportHandoverCommand) {
			command.Verdict = domain.ObjectHandedOver
			command.ReleasingEvidence = "evidence-release-retry"
			command.ReceivingEvidence = "evidence-receive-1"
			command.Rule = "handover-rule/v1"
			command.Basis = ""
		}, application.HandoverRegistrationConflict},
	}
	for _, testCase := range cases {
		t.Run(testCase.name, func(t *testing.T) {
			fixture := newHandoverFixture(t)
			registered, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
			wantOutcome(t, registered.Outcome(), err, application.HandoverRegistered)
			first, err := fixture.handler.Correct(context.Background(), correctHandoverCommand(t, v1, v2))
			wantOutcome(t, first.Outcome(), err, application.HandoverCorrected)

			again := correctHandoverCommand(t, v1, retry)
			testCase.change(&again)
			result, err := fixture.handler.Correct(context.Background(), again)
			wantOutcome(t, result.Outcome(), err, testCase.want)
			if len(fixture.registry.records) != 2 {
				t.Fatalf("登记册里 %d 版，want 2（v1 只有一个后继）", len(fixture.registry.records))
			}
			if record, has := result.Record(); testCase.want == application.HandoverExistingVersion &&
				(!has || record.Key.Version.String() != v2) {
				t.Fatalf("重放交回 %q，want 已落下的后继 %s", record.Key.Version, v2)
			}
		})
	}
}

// Covers: 票 tf-face-algebra/07 修复方向「所以要改成按（前版）读回那份后继，再用〈交接更正冲突〉那套比对分格」——读时前版
// 还没有后继，落库时另一方的更正先占了它（库面部分唯一索引撞上，按新键读回是空的）：按前版读回那份后继再分格，不答未决、
// 不一律答已有版本。
func TestACorrectionThatLostTheRaceForThePredecessorIsComparedWithTheWinner(t *testing.T) {
	v1 := "handover-result/parcel-1/v1"
	for name, want := range map[string]application.HandoverRegistrationOutcome{
		"late-correction-1": application.HandoverExistingVersion,
		"late-correction-9": application.HandoverRegistrationConflict,
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newHandoverFixture(t)
			registered, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
			wantOutcome(t, registered.Outcome(), err, application.HandoverRegistered)
			fixture.registry.beforeSave = func() {
				winner := correctHandoverCommand(t, v1, "handover-result/parcel-1/winner")
				winner.Basis = name
				predecessor := handoverRecordAt(t, fixture, v1)
				correction, err := predecessor.Handover.Correct(domain.HandoverCorrection{
					Verdict:     winner.Verdict,
					Basis:       mustHandover(t, domain.NewHandoverBasisReference, winner.Basis),
					Version:     mustHandover(t, domain.NewHandoverResultVersion, winner.NewVersion),
					CorrectedAt: winner.CorrectedAt,
				})
				if err != nil {
					t.Fatalf("赢家更正：%v", err)
				}
				_, digest, err := domain.CanonicalizeHandoverCorrectionPayload(winner.Verdict, winner.PredecessorVersion,
					winner.ReleasingEvidence, winner.ReceivingEvidence, winner.Rule, winner.Basis, winner.CorrectedAt)
				if err != nil {
					t.Fatalf("赢家摘要：%v", err)
				}
				record := predecessor
				record.Key.Version = correction.Version()
				record.Handover = correction
				record.ContentDigest = digest
				fixture.registry.records[handoverRegistryKey(record.Key)] = record
			}

			result, err := fixture.handler.Correct(context.Background(), correctHandoverCommand(t, v1, "handover-result/parcel-1/v2"))
			wantOutcome(t, result.Outcome(), err, want)
			if len(fixture.registry.records) != 2 {
				t.Fatalf("登记册里 %d 版，want 2", len(fixture.registry.records))
			}
		})
	}
}

func mustHandover[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}
