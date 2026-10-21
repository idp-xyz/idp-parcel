package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

// 同层先后到达的结果在编排面上怎么落（CONTEXT「同层后续事件不是同层冲突」，ADR-0226）：替身结果库把
// 已保存的记录交回给下一次接收，先后几次接收在这里真的看得见彼此。

var (
	earlierOccurredAt = resultOccurredAt.Add(-2 * time.Hour)
	laterOccurredAt   = resultOccurredAt.Add(-time.Hour)
)

func successiveRelease(t *testing.T, sourceID string, kind domain.ReleaseKind, occurredAt time.Time) application.ReceiveExternalResultCommand {
	t.Helper()
	condition := ""
	if kind == domain.ConditionalRelease {
		condition = "post-release inspection within 7 days"
	}
	command := releaseCommand(t, sourceID, kind, condition)
	command.RawSemantics = "RELEASE/" + kind.String()
	command.OccurredAt = occurredAt
	return command
}

func processDecision(t *testing.T, sourceID, semantics string, occurredAt time.Time) application.ReceiveExternalResultCommand {
	t.Helper()
	command := resultCommand(t, sourceID)
	command.Layer = domain.ProcessDecisionLayer
	command.RawSemantics = semantics
	command.OccurredAt = occurredAt
	return command
}

func handleInOrder(
	t *testing.T,
	fixture *resultFixture,
	commands ...application.ReceiveExternalResultCommand,
) []application.ReceiveExternalResultResult {
	t.Helper()
	var results []application.ReceiveExternalResultResult
	for _, command := range commands {
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle %s: %v", command.SourceID, err)
		}
		results = append(results, result)
	}
	return results
}

func requireOutcomes(t *testing.T, results []application.ReceiveExternalResultResult, want ...application.ExternalResultOutcome) {
	t.Helper()
	for index, result := range results {
		if result.Outcome() != want[index] {
			t.Fatalf("第 %d 份 outcome = %q, want %q", index+1, result.Outcome(), want[index])
		}
		record, present := result.Record()
		if !present || record.LayerConflict != (want[index] == application.ResultLayerConflict) {
			t.Fatalf("第 %d 份记录的冲突标记与结果格不符：%+v", index+1, record)
		}
	}
}

// Covers: AT-CC-431、AT-CC-432（编排半边）——先部分放行、后全部放行，两份都照常记下、都交给下游；全部放行
// 先到、业务时间更早的部分放行后到，照样两份都记下，没有一份被当作冲突吞掉。
func TestAFullReleaseAfterAPartialReleaseIsRecordedInEitherArrivalOrder(t *testing.T) {
	for name, order := range map[string][]application.ReceiveExternalResultCommand{
		"先部分、后全部":      {successiveRelease(t, "source-partial", domain.PartialRelease, earlierOccurredAt), successiveRelease(t, "source-full", domain.FullRelease, laterOccurredAt)},
		"全部先到、更早的部分后到": {successiveRelease(t, "source-full", domain.FullRelease, laterOccurredAt), successiveRelease(t, "source-partial", domain.PartialRelease, earlierOccurredAt)},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newResultFixture(t)
			requireOutcomes(t, handleInOrder(t, fixture, order...), application.ResultRecorded, application.ResultRecorded)
			if len(fixture.handoff.intents) != 2 {
				t.Fatalf("intents = %d, want 2——后到的那份没交给下游，成员到不了清关", len(fixture.handoff.intents))
			}
		})
	}
}

// Covers: AT-CC-433（编排半边）——同一层先查验、后补资料，两项监管过程决定按业务时间成序列，都记下；
// 乱序到达也一样。
func TestSuccessiveProcessDecisionsAreAllRecorded(t *testing.T) {
	inspection := processDecision(t, "source-inspection", "INSPECTION_ORDERED", earlierOccurredAt)
	documents := processDecision(t, "source-documents", "DOCUMENTS_REQUESTED", laterOccurredAt)
	for name, order := range map[string][]application.ReceiveExternalResultCommand{
		"先查验、后补资料":      {inspection, documents},
		"补资料先到、更早的查验后到": {documents, inspection},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newResultFixture(t)
			requireOutcomes(t, handleInOrder(t, fixture, order...), application.ResultRecorded, application.ResultRecorded)
		})
	}
}

// Covers: AT-CC-434、AT-CC-435（编排半边）——同一时刻同一范围两种结论、全部放行之后回退成部分放行：后到的
// 那份记同层冲突、照样留存并交出，先到的那份一字不动。
func TestContradictionsAreStillConflictsOnTheLaterArrival(t *testing.T) {
	for name, order := range map[string][]application.ReceiveExternalResultCommand{
		"同一时刻全部与部分":  {successiveRelease(t, "source-full", domain.FullRelease, laterOccurredAt), successiveRelease(t, "source-partial", domain.PartialRelease, laterOccurredAt)},
		"同一时刻两种过程决定": {processDecision(t, "source-inspection", "INSPECTION_ORDERED", laterOccurredAt), processDecision(t, "source-documents", "DOCUMENTS_REQUESTED", laterOccurredAt)},
		"全部之后回退成部分":  {successiveRelease(t, "source-full", domain.FullRelease, earlierOccurredAt), successiveRelease(t, "source-partial", domain.PartialRelease, laterOccurredAt)},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newResultFixture(t)
			results := handleInOrder(t, fixture, order...)
			requireOutcomes(t, results, application.ResultRecorded, application.ResultLayerConflict)
			first, _ := results[0].Record()
			if kept := fixture.store.records[resultKey(first.Key)]; kept.LayerConflict {
				t.Fatal("先到的那份被改了标记——冲突只标在后到的一方")
			}
			if len(fixture.handoff.intents) != 2 {
				t.Fatalf("intents = %d, want 2（冲突记录照样交出，下游据标记入账不派生）", len(fixture.handoff.intents))
			}
		})
	}
}

// Covers: AT-CC-436（编排半边）——同一来源标识同内容重投答已有记录，不再存一份；本判法之前已记了冲突的存量
// 重投时照旧带着冲突标记，不在重放里重判。
func TestReplaysAnswerTheBookedRecordAndStockConflictsAreNotRejudged(t *testing.T) {
	fixture := newResultFixture(t)
	partial := successiveRelease(t, "source-partial", domain.PartialRelease, earlierOccurredAt)
	full := successiveRelease(t, "source-full", domain.FullRelease, laterOccurredAt)
	handleInOrder(t, fixture, partial, full)

	replayed := handleInOrder(t, fixture, full)[0]
	if replayed.Outcome() != application.ResultExistingResult || fixture.store.saves != 2 {
		t.Fatalf("outcome = %q saves = %d；重投该答已有记录、不再存", replayed.Outcome(), fixture.store.saves)
	}

	stock := newResultFixture(t)
	handleInOrder(t, stock, partial)
	flagged := ports.ExternalResultRecord{}
	for _, record := range fixture.store.records {
		if record.Key.SourceID == "source-full" {
			flagged = record
		}
	}
	flagged.LayerConflict = true
	stock.store.records[resultKey(flagged.Key)] = flagged
	stock.store.saved = append(stock.store.saved, resultKey(flagged.Key))

	again := handleInOrder(t, stock, full)[0]
	record, _ := again.Record()
	if again.Outcome() != application.ResultExistingResult || !record.LayerConflict {
		t.Fatalf("outcome = %q conflict = %v；存量冲突标记是接收那一刻的判断，重放不改写它",
			again.Outcome(), record.LayerConflict)
	}
}

// 在册事实的放行事实与层次配不上，是仓储不变量已破：整笔报错不作答，不答未受理（那是来源的错）、
// 不记下新到的这份。
func TestAMismatchedBookedFactFailsLoudly(t *testing.T) {
	fixture := newResultFixture(t)
	booked := handleInOrder(t, newResultFixture(t), successiveRelease(t, "source-booked", domain.PartialRelease, earlierOccurredAt))[0]
	record, _ := booked.Record()
	smuggled := existingLayerFact(t, "RECEIVED")
	fixture.store.layerFacts = []domain.LayerFact{{Result: smuggled, Release: record.Release}}

	_, err := fixture.handler.Handle(context.Background(), successiveRelease(t, "source-full", domain.FullRelease, laterOccurredAt))
	if !errors.Is(err, domain.ErrInvalidExternalResult) {
		t.Fatalf("err = %v；在册事实配不上层次该响亮报错", err)
	}
	if fixture.store.saves != 0 || len(fixture.handoff.intents) != 0 {
		t.Fatalf("saves = %d intents = %d；报错就不该落记录或交意图", fixture.store.saves, len(fixture.handoff.intents))
	}
}
