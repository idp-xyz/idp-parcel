package application_test

import (
	"context"
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 层内更正关系在编排面上怎么落（UC-CC-006「同一层次和业务事项的后续明确更正形成新结果及更正关系」、
// AT-CC-156）：替身结果库把已保存的记录连同更正是否生效交回给下一次接收。

func assessedDuty(t *testing.T, sourceID, amount, corrects string) application.ReceiveExternalResultCommand {
	t.Helper()
	command := resultCommand(t, sourceID)
	command.Layer = domain.AssessedDutyLayer
	command.RawSemantics = "DUTY/" + amount
	command.Corrects = corrects
	return command
}

func recordOf(t *testing.T, result application.ReceiveExternalResultResult) ports.ExternalResultRecord {
	t.Helper()
	record, present := result.Record()
	if !present {
		t.Fatalf("outcome %q 没带记录", result.Outcome())
	}
	return record
}

// Covers: AT-CC-156 的编排半边——来源明确指名更正哪一份时，后一份核定税费照常记下、带着生效的更正关系交给
// 下游，不记冲突；原结果一行不动。对照：另一份金额不带更正指向时判同层冲突。
func TestAnExplicitCorrectionIsRecordedWithItsRelationAndWithoutAConflict(t *testing.T) {
	fixture := newResultFixture(t)
	results := handleInOrder(t, fixture,
		assessedDuty(t, "duty-1", "USD-120.00", ""),
		assessedDuty(t, "duty-2", "USD-95.50", "duty-1"),
		assessedDuty(t, "duty-3", "USD-80.00", ""),
	)
	requireOutcomes(t, results, application.ResultRecorded, application.ResultRecorded, application.ResultLayerConflict)

	correction := recordOf(t, results[1])
	if corrected, has := correction.Result.Corrects(); !has || corrected != "duty-1" || !correction.CorrectionInForce {
		t.Fatalf("更正关系没随记录落下：corrects=%q in force=%v", corrected, correction.CorrectionInForce)
	}
	original := fixture.store.records[resultKey(ports.ExternalResultKey{TenantID: mustTenant(t), SourceID: "duty-1"})]
	if original.Result.RawSemantics() != "DUTY/USD-120.00" || original.CorrectionInForce {
		t.Fatalf("原结果被改写了：%+v", original)
	}
	if len(fixture.handoff.intents) != 3 {
		t.Fatalf("intents = %d, want 3（更正也要交给下游）", len(fixture.handoff.intents))
	}
	if !fixture.handoff.intents[1].Record.CorrectionInForce {
		t.Fatal("交给下游的意图没带生效的更正关系，下游分不出谁取代了谁")
	}
	if conflicted := recordOf(t, results[2]); conflicted.CorrectionInForce {
		t.Fatal("不带更正指向的那份不该带更正关系")
	}
}

// Covers: 重投幂等——同一份更正重放答已有记录、不形成第二份；同一来源标识换了更正指向，或原来不带更正指向
// 的来源标识重投时带上了，都是来源响应冲突，不按最后到达改写原记录。同身份内容比对先于一切关联，所以
// 换上的那个指向在不在册无关紧要。
func TestACorrectionReplaysIdempotentlyAndAChangedPointerIsASourceConflict(t *testing.T) {
	fixture := newResultFixture(t)
	handleInOrder(t, fixture,
		assessedDuty(t, "duty-1", "USD-120.00", ""),
		assessedDuty(t, "duty-2", "USD-95.50", "duty-1"),
	)
	saves := fixture.store.saves

	replay, err := fixture.handler.Handle(context.Background(), assessedDuty(t, "duty-2", "USD-95.50", "duty-1"))
	if err != nil {
		t.Fatalf("replay: %v", err)
	}
	if replay.Outcome() != application.ResultExistingResult || !recordOf(t, replay).CorrectionInForce {
		t.Fatalf("重放 outcome = %q, want EXISTING_RESULT 且带原更正关系", replay.Outcome())
	}
	if fixture.store.saves != saves {
		t.Fatalf("重放又落了一份：saves %d → %d", saves, fixture.store.saves)
	}

	for name, command := range map[string]application.ReceiveExternalResultCommand{
		"同一来源标识换了更正指向":    assessedDuty(t, "duty-2", "USD-95.50", "duty-0"),
		"同一来源标识丢了更正指向":    assessedDuty(t, "duty-2", "USD-95.50", ""),
		"原来不带更正指向，重投时带上了": assessedDuty(t, "duty-1", "USD-120.00", "duty-0"),
	} {
		t.Run(name, func(t *testing.T) {
			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.ResultSourceConflict {
				t.Fatalf("outcome = %q, want SOURCE_CONFLICT", result.Outcome())
			}
		})
	}
}

// Covers: 票 docs-consistency/code/04 的代决——所指那份已被另一份生效的更正取代时只许更正链尾：第二份更正记
// 冲突，原有的更正关系不变；更正链尾照样生效。
func TestOnlyTheTailOfACorrectionChainCanBeCorrected(t *testing.T) {
	t.Run("所指那份已被更正过", func(t *testing.T) {
		fixture := newResultFixture(t)
		results := handleInOrder(t, fixture,
			assessedDuty(t, "duty-1", "USD-120.00", ""),
			assessedDuty(t, "duty-2", "USD-95.50", "duty-1"),
			assessedDuty(t, "duty-3", "USD-95.50", "duty-1"),
		)
		requireOutcomes(t, results, application.ResultRecorded, application.ResultRecorded, application.ResultLayerConflict)
		if recordOf(t, results[2]).CorrectionInForce {
			t.Fatal("记了冲突的更正不得生效——冲突的一方不改变当前判断")
		}
	})

	t.Run("更正链尾", func(t *testing.T) {
		fixture := newResultFixture(t)
		results := handleInOrder(t, fixture,
			assessedDuty(t, "duty-1", "USD-120.00", ""),
			assessedDuty(t, "duty-2", "USD-95.50", "duty-1"),
			assessedDuty(t, "duty-4", "USD-101.00", "duty-2"),
		)
		requireOutcomes(t, results, application.ResultRecorded, application.ResultRecorded, application.ResultRecorded)
		if !recordOf(t, results[2]).CorrectionInForce {
			t.Fatal("更正链尾没生效")
		}
	})
}

// Covers: 更正指向坏了的几格——所指那份还没在册是此刻判断不了（可能是乱序，原结果后到），停在指名的未决、
// 带续办引用、什么也不落；所指那份是归属不上的留存、在另一层、属另一版提交，指名自己，或指向全空白，都是
// 矛盾输入，不受理。读不动所指那份同读不动结果库。
func TestABrokenCorrectionPointerIsUndecidedOrNotAccepted(t *testing.T) {
	t.Run("所指那份还没在册", func(t *testing.T) {
		fixture := newResultFixture(t)
		result, err := fixture.handler.Handle(context.Background(), assessedDuty(t, "duty-2", "USD-95.50", "duty-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultUndecided || result.UndecidedReason() != application.CorrectedResultNotRecorded {
			t.Fatalf("outcome = %q reason = %q, want RESULT_UNDECIDED / CORRECTED_RESULT_NOT_RECORDED",
				result.Outcome(), result.UndecidedReason())
		}
		if result.ContinuationReference() == "" {
			t.Fatal("未决没带续办引用")
		}
		if fixture.store.saves != 0 || len(fixture.handoff.intents) != 0 {
			t.Fatalf("未决落了东西：saves=%d intents=%d", fixture.store.saves, len(fixture.handoff.intents))
		}
	})

	t.Run("读不动所指那份", func(t *testing.T) {
		fixture := newResultFixture(t)
		handleInOrder(t, fixture, assessedDuty(t, "duty-1", "USD-120.00", ""))
		fixture.store.findErr = errors.New("result store unavailable")
		result, err := fixture.handler.Handle(context.Background(), assessedDuty(t, "duty-2", "USD-95.50", "duty-1"))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultUndecided || result.UndecidedReason() != application.ResultStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	notAccepted := func(t *testing.T, fixture *resultFixture, command application.ReceiveExternalResultCommand) {
		t.Helper()
		saves := fixture.store.saves
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.ResultNotAccepted {
			t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
		}
		if fixture.store.saves != saves {
			t.Fatal("矛盾输入落了记录")
		}
	}

	t.Run("全空白的更正指向", func(t *testing.T) {
		notAccepted(t, newResultFixture(t), assessedDuty(t, "duty-1", "USD-120.00", "   "))
		unattributable := newResultFixture(t)
		unattributable.submissions.found = false
		notAccepted(t, unattributable, assessedDuty(t, "duty-1", "USD-120.00", "  "))
	})

	t.Run("指名自己", func(t *testing.T) {
		notAccepted(t, newResultFixture(t), assessedDuty(t, "duty-1", "USD-120.00", "duty-1"))
	})

	t.Run("所指那份是归属不上的留存", func(t *testing.T) {
		fixture := newResultFixture(t)
		fixture.submissions.found = false
		handleInOrder(t, fixture, assessedDuty(t, "duty-1", "USD-120.00", ""))
		fixture.submissions.found = true
		notAccepted(t, fixture, assessedDuty(t, "duty-2", "USD-95.50", "duty-1"))
	})

	t.Run("所指那份在另一层", func(t *testing.T) {
		fixture := newResultFixture(t)
		handleInOrder(t, fixture, resultCommand(t, "receipt-1"))
		notAccepted(t, fixture, assessedDuty(t, "duty-2", "USD-95.50", "receipt-1"))
	})

	t.Run("所指那份属另一版提交", func(t *testing.T) {
		fixture := newResultFixture(t)
		earlier := assessedDuty(t, "duty-1", "USD-120.00", "")
		earlier.ClaimedVersion = "submission/v0"
		handleInOrder(t, fixture, earlier)
		notAccepted(t, fixture, assessedDuty(t, "duty-2", "USD-95.50", "duty-1"))
	})
}
