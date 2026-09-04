package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证有效时间规则登记编排（label-channel/19）的结果代数：首登、换版回指当前版、重放、冲突、
// 未受理、未决，以及并发撞键读回赢家。目录用内存替身，真库那一半在 adapters/postgres 的用例里。

var ruleNowAt = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)

// ruleRegistryDouble 是只插不改的内存目录；`当前版`同真实现一样按回指派生。raceWinner 非空时模拟
// 并发：本方 Save 那一刻另一方先落了它，写口答已登记。
type ruleRegistryDouble struct {
	records    []ports.EffectiveTimeRuleRecord
	failFind   error
	failSave   error
	raceWinner *ports.EffectiveTimeRuleRecord
}

func (double *ruleRegistryDouble) FindByKey(
	_ context.Context,
	key ports.EffectiveTimeRuleKey,
) (ports.EffectiveTimeRuleRecord, bool, error) {
	if double.failFind != nil {
		return ports.EffectiveTimeRuleRecord{}, false, double.failFind
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.EffectiveTimeRuleRecord{}, false, nil
}

func (double *ruleRegistryDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
) (ports.EffectiveTimeRuleRecord, bool, error) {
	if double.failFind != nil {
		return ports.EffectiveTimeRuleRecord{}, false, double.failFind
	}
	superseded := map[domain.EffectiveTimeRuleVersion]bool{}
	for _, record := range double.records {
		if record.Key.TenantID != tenant || record.Key.Source != source {
			continue
		}
		if prior, has := record.Rule.Supersedes(); has {
			superseded[prior] = true
		}
	}
	var current ports.EffectiveTimeRuleRecord
	found := false
	for _, record := range double.records {
		if record.Key.TenantID != tenant || record.Key.Source != source || superseded[record.Key.Version] {
			continue
		}
		if !found || record.RecordedAt.After(current.RecordedAt) {
			current, found = record, true
		}
	}
	return current, found, nil
}

func (double *ruleRegistryDouble) ListVersions(
	_ context.Context,
	tenant domain.TenantID,
	source domain.TrackingSourceReference,
) ([]ports.EffectiveTimeRuleRecord, error) {
	var versions []ports.EffectiveTimeRuleRecord
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Source == source {
			versions = append(versions, record)
		}
	}
	return versions, nil
}

func (double *ruleRegistryDouble) Save(
	_ context.Context,
	record ports.EffectiveTimeRuleRecord,
) (ports.EffectiveTimeRuleSaveOutcome, error) {
	if double.failSave != nil {
		return ports.EffectiveTimeRuleSaveOutcomeInvalid, double.failSave
	}
	if double.raceWinner != nil {
		double.records = append(double.records, *double.raceWinner)
		double.raceWinner = nil
		return ports.EffectiveTimeRuleAlreadyRegistered, nil
	}
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.EffectiveTimeRuleAlreadyRegistered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.EffectiveTimeRuleSaved, nil
}

func newRuleHandler(registry *ruleRegistryDouble) *application.RegisterEffectiveTimeRuleHandler {
	return application.NewRegisterEffectiveTimeRuleHandler(application.RegisterEffectiveTimeRuleDeps{
		Rules: registry,
		Clock: credentialClock{at: ruleNowAt},
	})
}

func registerRuleCommand(t *testing.T, source, version string) application.RegisterEffectiveTimeRuleCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	return application.RegisterEffectiveTimeRuleCommand{
		TenantID:          tenant,
		Source:            source,
		Version:           version,
		SourceTimeMeaning: "EVENT_OCCURRENCE",
		Anchor:            "OCCURRED_AT",
	}
}

func TestRegisteringAFirstRuleVersionRecordsItWithoutAPredecessor(t *testing.T) {
	registry := &ruleRegistryDouble{}
	result, err := newRuleHandler(registry).Register(t.Context(), registerRuleCommand(t, "aggregator-a", "ETR-1"))
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if result.Outcome() != application.EffectiveTimeRuleRegistered {
		t.Fatalf("outcome = %s, want RULE_REGISTERED", result.Outcome())
	}
	record, has := result.Record()
	if !has || record.Key.Version.String() != "ETR-1" || record.Key.Source.String() != "aggregator-a" {
		t.Fatalf("record = %+v has=%v", record.Key, has)
	}
	if _, superseded := record.Rule.Supersedes(); superseded {
		t.Fatal("首版不该回指")
	}
	if record.Rule.Content().Anchor != domain.AnchoredAtOccurrence || record.Rule.Content().SourceTimeMeaning != domain.SourceTimeIsEventOccurrence {
		t.Fatalf("正文没有按命令登记：%+v", record.Rule.Content())
	}
	if !record.RecordedAt.Equal(ruleNowAt) {
		t.Fatalf("登记时刻应取时钟：%s", record.RecordedAt)
	}
	if len(registry.records) != 1 {
		t.Fatalf("目录里应恰好一条：%d", len(registry.records))
	}
}

// Covers: 一源一链——第二次登记不同版本不是「第二个首版」，是回指当前版的换版。
func TestRegisteringANewVersionForASourceWithARuleRevisesTheCurrentOne(t *testing.T) {
	registry := &ruleRegistryDouble{}
	handler := newRuleHandler(registry)
	if _, err := handler.Register(t.Context(), registerRuleCommand(t, "aggregator-a", "ETR-1")); err != nil {
		t.Fatalf("首登：%v", err)
	}

	revision := registerRuleCommand(t, "aggregator-a", "ETR-2")
	revision.SourceTimeMeaning = "SOURCE_PROCESSING"
	revision.Anchor = "RECEIVED_AT"
	revision.Offset = -30 * time.Minute
	result, err := handler.Register(t.Context(), revision)
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	if result.Outcome() != application.EffectiveTimeRuleRevised {
		t.Fatalf("outcome = %s, want RULE_REVISED", result.Outcome())
	}
	record, _ := result.Record()
	if prior, has := record.Rule.Supersedes(); !has || prior.String() != "ETR-1" {
		t.Fatalf("新版应回指 ETR-1：%q has=%v", prior, has)
	}
	if record.Rule.Content().Offset != -30*time.Minute || record.Rule.Content().Anchor != domain.AnchoredAtReception {
		t.Fatalf("新正文没有登上：%+v", record.Rule.Content())
	}

	current, found, _ := registry.FindCurrent(t.Context(), record.Key.TenantID, record.Key.Source)
	if !found || current.Key.Version.String() != "ETR-2" {
		t.Fatalf("当前版应是 ETR-2：%+v found=%v", current.Key, found)
	}
}

func TestReplayAndConflictAreToldApartByContent(t *testing.T) {
	registry := &ruleRegistryDouble{}
	handler := newRuleHandler(registry)
	command := registerRuleCommand(t, "aggregator-a", "ETR-1")
	if _, err := handler.Register(t.Context(), command); err != nil {
		t.Fatalf("首登：%v", err)
	}

	replay, err := handler.Register(t.Context(), command)
	if err != nil || replay.Outcome() != application.EffectiveTimeRuleExistingVersion {
		t.Fatalf("同键同内容应答 EXISTING_VERSION：%v %s", err, replay.Outcome())
	}

	conflicting := command
	conflicting.Offset = time.Hour
	conflict, err := handler.Register(t.Context(), conflicting)
	if err != nil || conflict.Outcome() != application.EffectiveTimeRuleContentConflict {
		t.Fatalf("同键异内容应答 CONTENT_CONFLICT：%v %s", err, conflict.Outcome())
	}
	record, has := conflict.Record()
	if !has || record.Rule.Content().Offset != 0 {
		t.Fatal("冲突应带回既有版本，原版本不被顶替")
	}
	if len(registry.records) != 1 {
		t.Fatalf("重放与冲突都不该多登一条：%d", len(registry.records))
	}
}

func TestRegistrationRefusesInputTheDomainCannotShape(t *testing.T) {
	cases := map[string]func(*application.RegisterEffectiveTimeRuleCommand){
		"缺源":       func(command *application.RegisterEffectiveTimeRuleCommand) { command.Source = " " },
		"缺版本":      func(command *application.RegisterEffectiveTimeRuleCommand) { command.Version = "" },
		"时间字段含义集外": func(command *application.RegisterEffectiveTimeRuleCommand) { command.SourceTimeMeaning = "GUESSED" },
		"锚点集外":     func(command *application.RegisterEffectiveTimeRuleCommand) { command.Anchor = "NOW" },
		"缺租户":      func(command *application.RegisterEffectiveTimeRuleCommand) { command.TenantID = domain.TenantID{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			registry := &ruleRegistryDouble{}
			command := registerRuleCommand(t, "aggregator-a", "ETR-1")
			mutate(&command)
			result, err := newRuleHandler(registry).Register(t.Context(), command)
			if err != nil || result.Outcome() != application.EffectiveTimeRuleRegistrationNotAccepted {
				t.Fatalf("outcome = %s err = %v, want INPUT_NOT_ACCEPTED", result.Outcome(), err)
			}
			if len(registry.records) != 0 {
				t.Fatal("未受理的输入不得到达目录")
			}
		})
	}
}

func TestRegistryFailuresAreUndecidedWithAContinuation(t *testing.T) {
	t.Run("读不回", func(t *testing.T) {
		registry := &ruleRegistryDouble{failFind: errors.New("catalogue down")}
		result, err := newRuleHandler(registry).Register(t.Context(), registerRuleCommand(t, "aggregator-a", "ETR-1"))
		if err != nil || result.Outcome() != application.EffectiveTimeRuleRegistrationUndecided {
			t.Fatalf("outcome = %s err = %v", result.Outcome(), err)
		}
		if result.UndecidedReason() != application.EffectiveTimeRuleRegistryUnavailable || result.ContinuationReference() == "" {
			t.Fatalf("未决应指名目录并带续办引用：%s %q", result.UndecidedReason(), result.ContinuationReference())
		}
	})
	t.Run("写不进", func(t *testing.T) {
		registry := &ruleRegistryDouble{failSave: errors.New("catalogue down")}
		result, err := newRuleHandler(registry).Register(t.Context(), registerRuleCommand(t, "aggregator-a", "ETR-1"))
		if err != nil || result.Outcome() != application.EffectiveTimeRuleRegistrationUndecided {
			t.Fatalf("outcome = %s err = %v", result.Outcome(), err)
		}
	})
}

// Covers: 并发撞键读回赢家按内容分重放/冲突；赢家不是自己那一版（索引拦下的第二个首版）则未决。
func TestALostRuleRaceReadsBackTheWinner(t *testing.T) {
	t.Run("赢家是同键同内容", func(t *testing.T) {
		registry := &ruleRegistryDouble{}
		command := registerRuleCommand(t, "aggregator-a", "ETR-1")
		winner := registerRuleCommand(t, "aggregator-a", "ETR-1")
		seeded := &ruleRegistryDouble{}
		seededResult, _ := newRuleHandler(seeded).Register(t.Context(), winner)
		winnerRecord, _ := seededResult.Record()
		registry.raceWinner = &winnerRecord
		result, err := newRuleHandler(registry).Register(t.Context(), command)
		if err != nil || result.Outcome() != application.EffectiveTimeRuleExistingVersion {
			t.Fatalf("outcome = %s err = %v, want EXISTING_VERSION", result.Outcome(), err)
		}
	})
	t.Run("赢家是另一版本", func(t *testing.T) {
		registry := &ruleRegistryDouble{}
		seeded := &ruleRegistryDouble{}
		seededResult, _ := newRuleHandler(seeded).Register(t.Context(), registerRuleCommand(t, "aggregator-a", "ETR-other"))
		winnerRecord, _ := seededResult.Record()
		registry.raceWinner = &winnerRecord
		result, err := newRuleHandler(registry).Register(t.Context(), registerRuleCommand(t, "aggregator-a", "ETR-1"))
		if err != nil || result.Outcome() != application.EffectiveTimeRuleRegistrationUndecided {
			t.Fatalf("撞键却读不回自己那一版应未决：%s err = %v", result.Outcome(), err)
		}
	})
}
