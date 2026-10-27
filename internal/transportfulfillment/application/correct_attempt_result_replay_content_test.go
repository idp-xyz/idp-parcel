package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证更正重放比对的每一格都在比（票 docs-consistency/code/90 评审）：比对锚覆盖结果、依据、来源、控制依据、业务时间与更正时刻，换掉其中任一格再交都是
// 另一种说法，答冲突；不换则答已有更正。依据与更正时刻是两格最容易没人守的：揽收到手与妥投不带依据，换依据的重放要用失败那一格才看得出来。

// Covers: `AT-TF-360`（编排半边）——揽收一侧：同一格被更正成客户不在之后，换了依据或换了更正时刻再交，答冲突，在册的更正不被顶替；原样再交答已有更正。
func TestAPickupCorrectionReplayWithAnotherBasisOrCorrectionTimeConflicts(t *testing.T) {
	for name, change := range map[string]func(*application.CorrectPickupAttemptResultCommand){
		"another basis": func(command *application.CorrectPickupAttemptResultCommand) {
			command.Basis = value(t, domain.NewAttemptResultBasisReference, "reason/other-basis")
		},
		"another correction time": func(command *application.CorrectPickupAttemptResultCommand) {
			command.CorrectedAt = command.CorrectedAt.Add(time.Minute)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newPickupCorrectionFixture(t)
			if first := fixture.correct(t, fixture.command(t)); first.Outcome() != application.AttemptResultCorrected {
				t.Fatalf("first = %q", first.Outcome())
			}
			if same := fixture.correct(t, fixture.command(t)); same.Outcome() != application.AttemptResultCorrectionExisting {
				t.Fatalf("the same statement = %q, want EXISTING_CORRECTION", same.Outcome())
			}
			changed := fixture.command(t)
			change(&changed)
			if conflict := fixture.correct(t, changed); conflict.Outcome() != application.AttemptResultCorrectionConflict {
				t.Fatalf("outcome = %q, want SOURCE_CONFLICT", conflict.Outcome())
			}
			if fixture.corrections.saves != 1 {
				t.Fatalf("correction saves = %d, want 1；冲突的重放落了册", fixture.corrections.saves)
			}
		})
	}
}

// Covers: `AT-TF-360`（编排半边）——派送一侧同上：被更正成地址错误之后，换了依据或换了更正时刻再交，答冲突；原样再交答已有更正。
func TestADeliveryCorrectionReplayWithAnotherBasisOrCorrectionTimeConflicts(t *testing.T) {
	for name, change := range map[string]func(*application.CorrectDeliveryAttemptResultCommand){
		"another basis": func(command *application.CorrectDeliveryAttemptResultCommand) {
			command.Basis = value(t, domain.NewAttemptResultBasisReference, "reason/other-basis")
		},
		"another correction time": func(command *application.CorrectDeliveryAttemptResultCommand) {
			command.CorrectedAt = command.CorrectedAt.Add(time.Minute)
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newDeliveryCorrectionFixture(t)
			wrongAddress := fixture.command(t)
			wrongAddress.Outcome = domain.WrongAddress
			wrongAddress.Basis = value(t, domain.NewAttemptResultBasisReference, "reason/address-review-says-elsewhere")
			if first := fixture.correct(t, wrongAddress); first.Outcome() != application.AttemptResultCorrected {
				t.Fatalf("first = %q", first.Outcome())
			}
			if same := fixture.correct(t, wrongAddress); same.Outcome() != application.AttemptResultCorrectionExisting {
				t.Fatalf("the same statement = %q, want EXISTING_CORRECTION", same.Outcome())
			}
			changed := wrongAddress
			change(&changed)
			if conflict := fixture.correct(t, changed); conflict.Outcome() != application.AttemptResultCorrectionConflict {
				t.Fatalf("outcome = %q, want SOURCE_CONFLICT", conflict.Outcome())
			}
			if fixture.corrections.saves != 1 {
				t.Fatalf("correction saves = %d, want 1；冲突的重放落了册", fixture.corrections.saves)
			}
		})
	}
}

// Covers: `AT-TF-360`（编排半边）——派送一侧：更正不早于它所更正的待确认那一版（待确认记在到场后 6 分钟）：更正所说的那件事业务上可以更早（伙伴迟到的签收），可更正这一判断
// 本身早于待确认那一版答`来源未受理`，什么都不写；不早于它（对照）则成立。
func TestADeliveryCorrectionEarlierThanThePendingVersionIsNotAccepted(t *testing.T) {
	fixture := newDeliveryCorrectionFixture(t)
	command := fixture.command(t)
	command.OccurredAt = arrivedAt.Add(2 * time.Minute)  // 早于待确认那一版的业务时间：允许
	command.CorrectedAt = arrivedAt.Add(3 * time.Minute) // 但更正这一判断早于它所更正的待确认那一版：不接

	if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultCorrectionNotAccepted {
		t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
	}
	if fixture.corrections.saves != 0 {
		t.Fatal("早于待确认那一版的更正落了册")
	}

	command.CorrectedAt = arrivedAt.Add(7 * time.Minute)
	if result := fixture.correct(t, command); result.Outcome() != application.AttemptResultCorrected {
		t.Fatalf("outcome = %q, want RESULT_CORRECTED（更正不早于待确认那一版，业务时间早于它是允许的）", result.Outcome())
	}
}
