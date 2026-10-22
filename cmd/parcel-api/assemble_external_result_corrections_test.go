package main

import (
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	customshttp "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/http"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	ccports "go.idp.xyz/idp-parcel/internal/customscompliance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/outboxintent"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件在真实 PostgreSQL 上经 `/customs/external-results` 的生产装配（buildExternalResultsOrchestration 与其
// 事务壳）证层内更正关系（UC-CC-006「同一层次和业务事项的后续明确更正形成新结果及更正关系」、AT-CC-156）：
// 来源明确的更正形成新结果与更正关系、不判冲突、进当前判断；不带更正指向的矛盾照旧判冲突；重投幂等。测试输入
// 是隔离合成，只记 `S`。

type correctionChain struct {
	pool    *pgxpool.Pool
	db      *bentopg.DB
	results customshttp.ResultHandler
}

func newCorrectionChain(t *testing.T, layers ...customsdomain.ResultLayer) *correctionChain {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	results, err := buildExternalResultsOrchestration(db)
	if err != nil {
		t.Fatalf("装配外部结果编排：%v", err)
	}
	seedExternalResultChain(t, pool, "SYN-TENANT-1", "SYN-CC-UNIT-1", "SYN-CC-CASE-1", "SYN-CC-VERSION-1")
	for _, layer := range layers {
		seedInterpretationRule(t, pool, "SYN-TENANT-1", layer, "SYN-JURISDICTION-1", externalResultOccurredAt.Add(-48*time.Hour))
	}
	return &correctionChain{pool: pool, db: db, results: results}
}

func (chain *correctionChain) receive(t *testing.T, command customsapp.ReceiveExternalResultCommand) customsapp.ReceiveExternalResultResult {
	t.Helper()
	result, err := chain.results.Handle(t.Context(), command)
	if err != nil {
		t.Fatalf("接收 %s：%v", command.SourceID, err)
	}
	return result
}

func layeredCommand(
	t *testing.T,
	sourceID string,
	layer customsdomain.ResultLayer,
	semantics string,
	at time.Time,
	corrects string,
) customsapp.ReceiveExternalResultCommand {
	t.Helper()
	command := externalResultCommand(t, sourceID, "SYN-CC-VERSION-1")
	command.Layer = layer
	command.RawSemantics = semantics
	command.OccurredAt = at
	command.ReceivedAt = at.Add(time.Hour)
	command.Corrects = corrects
	return command
}

func releaseResultCommand(t *testing.T, sourceID string, kind customsdomain.ReleaseKind, at time.Time, corrects string) customsapp.ReceiveExternalResultCommand {
	t.Helper()
	command := layeredCommand(t, sourceID, customsdomain.ReleaseResultLayer, "SYN-RELEASE/"+kind.String(), at, corrects)
	command.Release = &customsapp.ReleaseContent{
		Kind:      kind,
		Authority: mustValue(t, customsdomain.NewRegulatoryAuthorityReference, "SYN-AUTHORITY/customs-1"),
	}
	return command
}

func (chain *correctionChain) stored(t *testing.T, sourceID string) ccports.ExternalResultRecord {
	t.Helper()
	store, err := ccpostgres.NewExternalResults(chain.db)
	if err != nil {
		t.Fatalf("构造登记册：%v", err)
	}
	record, found, err := store.FindByKey(t.Context(), ccports.ExternalResultKey{
		TenantID: mustValue(t, customsdomain.NewTenantID, "SYN-TENANT-1"), SourceID: sourceID,
	})
	if err != nil || !found {
		t.Fatalf("读回 %s：found=%v err=%v", sourceID, found, err)
	}
	return record
}

func requireOutcome(t *testing.T, result customsapp.ReceiveExternalResultResult, want customsapp.ExternalResultOutcome) {
	t.Helper()
	if result.Outcome() != want {
		t.Fatalf("outcome = %q（原因 %q），want %q", result.Outcome(), result.UndecidedReason(), want)
	}
}

// Covers: AT-CC-156——监管核定税费换金额、监管处置决定换种类，来源明确指名更正哪一份时照常记下并带生效的更正
// 关系，不判冲突，原结果一行不动；对照：不带更正指向的另一份金额照判同层冲突。
func TestTheWiredExternalResultsFormACorrectionRelationForAnExplicitCorrection(t *testing.T) {
	for name, layer := range map[string]struct {
		layer             customsdomain.ResultLayer
		original, correct string
		stray             string
	}{
		"监管核定税费换金额": {customsdomain.AssessedDutyLayer, "SYN-DUTY/USD-120.00", "SYN-DUTY/USD-95.50", "SYN-DUTY/USD-80.00"},
		"监管处置决定换种类": {customsdomain.DispositionDecisionLayer, "SYN-DISPOSITION/DESTROY", "SYN-DISPOSITION/RE-EXPORT", "SYN-DISPOSITION/RETURN"},
	} {
		t.Run(name, func(t *testing.T) {
			chain := newCorrectionChain(t, layer.layer)
			earlier := externalResultOccurredAt.Add(-2 * time.Hour)
			requireOutcome(t, chain.receive(t, layeredCommand(t, "SYN-RESULT-1", layer.layer, layer.original, earlier, "")), customsapp.ResultRecorded)

			corrected := chain.receive(t, layeredCommand(t, "SYN-RESULT-2", layer.layer, layer.correct, externalResultOccurredAt, "SYN-RESULT-1"))
			requireOutcome(t, corrected, customsapp.ResultRecorded)
			if ref := corrected.ResultHandoffReference(); ref != "" {
				t.Fatalf("更正的意图没交出去：%q", ref)
			}
			record := chain.stored(t, "SYN-RESULT-2")
			if target, has := record.Result.Corrects(); !has || target != "SYN-RESULT-1" || !record.CorrectionInForce || record.LayerConflict {
				t.Fatalf("库里的更正关系不对：corrects=%q in force=%v conflict=%v", target, record.CorrectionInForce, record.LayerConflict)
			}
			if original := chain.stored(t, "SYN-RESULT-1"); original.Result.RawSemantics() != layer.original || original.LayerConflict {
				t.Fatalf("原结果被改写：%+v", original)
			}
			if got := countOutboxRows(t, chain.pool, string(outboxintent.FingerprintEventID("external-result", "SYN-TENANT-1", "SYN-RESULT-2"))); got != 1 {
				t.Fatalf("更正的 outbox 行数 = %d，want 1", got)
			}

			stray := chain.receive(t, layeredCommand(t, "SYN-RESULT-3", layer.layer, layer.stray, externalResultOccurredAt.Add(time.Minute), ""))
			requireOutcome(t, stray, customsapp.ResultLayerConflict)
			if record := chain.stored(t, "SYN-RESULT-3"); record.CorrectionInForce {
				t.Fatal("不带更正指向的那份带上了更正关系")
			}
		})
	}
}

// Covers: AT-CC-156 与 CC CONTEXT「后续监管更正形成新的外部事实及当前判断，不覆盖原放行结果」——全部放行之后
// 来源明确收窄成部分放行：更正照常记下、不判冲突；放行读口（动作准入与离港门禁读的那一口）交出原全部放行并带上
// 更正发生的时点作取代时点，更正那份按它自己的业务发生时间另作一份——更正之前的时点仍由原放行答，之后由更正答
// （领域按时点取，见 domain 的准入用例）。原全部放行留在库里。对照：同样的收窄不带更正指向，是放行回退，照判
// 冲突，放行读口只交出原全部放行、不带取代时点。
func TestTheWiredExternalResultsLetANarrowingReleaseCorrectionIntoTheCurrentJudgement(t *testing.T) {
	earlier := externalResultOccurredAt.Add(-2 * time.Hour)
	for name, test := range map[string]struct {
		corrects   string
		want       customsapp.ExternalResultOutcome
		superseded time.Time
		sources    int
	}{
		"来源指明更正": {"SYN-RELEASE-1", customsapp.ResultRecorded, externalResultOccurredAt, 2},
		"不带更正指向": {"", customsapp.ResultLayerConflict, time.Time{}, 1},
	} {
		t.Run(name, func(t *testing.T) {
			chain := newCorrectionChain(t, customsdomain.ReleaseResultLayer)
			requireOutcome(t, chain.receive(t, releaseResultCommand(t, "SYN-RELEASE-1", customsdomain.FullRelease, earlier, "")), customsapp.ResultRecorded)
			requireOutcome(t, chain.receive(t, releaseResultCommand(t, "SYN-RELEASE-2", customsdomain.PartialRelease, externalResultOccurredAt, test.corrects)), test.want)

			view, err := ccpostgres.NewReleaseOutcomeView(chain.db)
			if err != nil {
				t.Fatalf("构造放行读口：%v", err)
			}
			readings, err := view.ListReleaseOutcomes(t.Context(),
				mustValue(t, customsdomain.NewTenantID, "SYN-TENANT-1"),
				mustValue(t, customsdomain.NewDecisionScopeReference, "SYN-CC-UNIT-1"))
			if err != nil {
				t.Fatalf("放行读口：%v", err)
			}
			if len(readings) != test.sources {
				t.Fatalf("放行读口交出 %+v，want %d 份", readings, test.sources)
			}
			for _, reading := range readings {
				switch reading.Source {
				case "SYN-RELEASE-1":
					if !reading.OccurredAt.Equal(earlier) || !reading.SupersededAt.Equal(test.superseded) {
						t.Fatalf("原全部放行 occurred=%s superseded=%s，want %s / %s",
							reading.OccurredAt, reading.SupersededAt, earlier, test.superseded)
					}
				case "SYN-RELEASE-2":
					if !reading.OccurredAt.Equal(externalResultOccurredAt) || !reading.SupersededAt.IsZero() {
						t.Fatalf("更正 occurred=%s superseded=%s——更正不继承原结果的时间", reading.OccurredAt, reading.SupersededAt)
					}
				default:
					t.Fatalf("放行读口交出意外的 %s", reading.Source)
				}
			}
			if got := countExternalResultRows(t, chain.pool, "SYN-TENANT-1", "SYN-RELEASE-1"); got != 1 {
				t.Fatalf("原全部放行行数 = %d，want 1——原结果保持历史", got)
			}
		})
	}
}

// Covers: 重投幂等——同一份更正重放答已有记录，不形成第二行、不多入一封信；同一来源标识换了更正指向是来源响应
// 冲突，不改写原记录。所更正那份还没在册时停在指名的未决，什么也不落；原结果到了再重投，形成更正关系。
func TestTheWiredExternalResultsReplayACorrectionIdempotently(t *testing.T) {
	chain := newCorrectionChain(t, customsdomain.AssessedDutyLayer)
	correction := layeredCommand(t, "SYN-RESULT-2", customsdomain.AssessedDutyLayer, "SYN-DUTY/USD-95.50", externalResultOccurredAt, "SYN-RESULT-1")

	early := chain.receive(t, correction)
	requireOutcome(t, early, customsapp.ResultUndecided)
	if early.UndecidedReason() != customsapp.CorrectedResultNotRecorded || early.ContinuationReference() == "" {
		t.Fatalf("原结果未到时 reason = %q continuation = %q", early.UndecidedReason(), early.ContinuationReference())
	}
	if got := countExternalResultRows(t, chain.pool, "SYN-TENANT-1", "SYN-RESULT-2"); got != 0 {
		t.Fatalf("未决落了 %d 行", got)
	}

	requireOutcome(t, chain.receive(t, layeredCommand(t, "SYN-RESULT-1", customsdomain.AssessedDutyLayer, "SYN-DUTY/USD-120.00",
		externalResultOccurredAt.Add(-2*time.Hour), "")), customsapp.ResultRecorded)
	requireOutcome(t, chain.receive(t, correction), customsapp.ResultRecorded)

	replay := chain.receive(t, correction)
	requireOutcome(t, replay, customsapp.ResultExistingResult)
	if record, _ := replay.Record(); !record.CorrectionInForce {
		t.Fatal("重放读回的记录丢了更正关系")
	}
	if got := countExternalResultRows(t, chain.pool, "SYN-TENANT-1", "SYN-RESULT-2"); got != 1 {
		t.Fatalf("重放后行数 = %d，want 1", got)
	}
	if got := countOutboxRows(t, chain.pool, string(outboxintent.FingerprintEventID("external-result", "SYN-TENANT-1", "SYN-RESULT-2"))); got != 1 {
		t.Fatalf("重放后 outbox 行数 = %d，want 1——重发的是同一封", got)
	}

	repointed := correction
	repointed.Corrects = "SYN-RESULT-0"
	requireOutcome(t, chain.receive(t, repointed), customsapp.ResultSourceConflict)
	if target, _ := chain.stored(t, "SYN-RESULT-2").Result.Corrects(); target != "SYN-RESULT-1" {
		t.Fatalf("来源响应冲突改写了原记录的更正指向：%q", target)
	}
}
