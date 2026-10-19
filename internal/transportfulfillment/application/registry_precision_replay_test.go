package application_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件证票 operator-channel/30 裁定 2 在编排一侧的落点：替身里预置的是登记册上的样子（截到微秒），命令带纳秒原样
// 重放，答重放那一格；跨一微秒才答冲突、签新版或来源冲突。替身只模拟读回，真库那一半在 adapters/postgres 的
// registry_precision_replay_test.go。夹具全为合成值（S 级）。

var (
	precisionFrom    = time.Date(2026, 9, 1, 9, 0, 0, 123456789, time.UTC)
	precisionUntil   = time.Date(2026, 12, 1, 9, 0, 0, 987654321, time.UTC)
	precisionChanged = time.Date(2026, 9, 3, 9, 0, 0, 555555555, time.UTC)
)

// onRegistry 是登记册读回的样子：测试里独立按微秒截，不调被测函数，免得用例同义反复。
func onRegistry(at time.Time) time.Time { return at.UTC().Truncate(time.Microsecond) }

func nanosecondCredentialCommand(t *testing.T) application.RegisterExternalCarrierCredentialCommand {
	t.Helper()
	command := registerCredentialCommand(t, "carrier-x/1Z-NS", "v1")
	command.EffectiveFrom, command.EffectiveUntil = precisionFrom, precisionUntil
	return command
}

// credentialOnRegistry 交回一本只放了「这条命令在登记册上的样子」的替身。
func credentialOnRegistry(t *testing.T, command application.RegisterExternalCarrierCredentialCommand) *credentialRegistryDouble {
	t.Helper()
	stored := command
	stored.EffectiveFrom, stored.EffectiveUntil = onRegistry(command.EffectiveFrom), onRegistry(command.EffectiveUntil)
	registry := &credentialRegistryDouble{}
	if result, err := newCredentialHandler(registry).Register(t.Context(), stored); err != nil || result.Outcome() != application.CredentialRegistered {
		t.Fatalf("预置在册记录：%v %s", err, result.Outcome())
	}
	return registry
}

// Covers: 凭证首登——适用起止带纳秒的命令原样重放答`已有版本`；起点或终点跨一微秒答`内容冲突`。
func TestACredentialRegistrationReplayedWithNanosecondsIsTheExistingVersion(t *testing.T) {
	command := nanosecondCredentialCommand(t)
	registry := credentialOnRegistry(t, command)
	handler := newCredentialHandler(registry)

	replay, err := handler.Register(t.Context(), command)
	if err != nil || replay.Outcome() != application.CredentialExistingVersion {
		t.Fatalf("带纳秒原样重放 = %s %v，期望 EXISTING_VERSION", replay.Outcome(), err)
	}
	for name, shift := range map[string]func(*application.RegisterExternalCarrierCredentialCommand){
		"from": func(c *application.RegisterExternalCarrierCredentialCommand) {
			c.EffectiveFrom = c.EffectiveFrom.Add(time.Microsecond)
		},
		"until": func(c *application.RegisterExternalCarrierCredentialCommand) {
			c.EffectiveUntil = c.EffectiveUntil.Add(time.Microsecond)
		},
	} {
		apart := command
		shift(&apart)
		if result, err := handler.Register(t.Context(), apart); err != nil || result.Outcome() != application.CredentialContentConflict {
			t.Fatalf("%s 跨一微秒 = %s %v，期望 CONTENT_CONFLICT", name, result.Outcome(), err)
		}
	}
	if len(registry.records) != 1 {
		t.Fatalf("重放与冲突都不落版本：%d", len(registry.records))
	}
}

// Covers: 凭证首登的并发输家——本方查时还没有、存时赢家已先落了它在登记册上的样子；读回赢家后带纳秒的本次命令答
// `已有版本`，跨一微秒答`内容冲突`。
func TestACredentialRaceLoserWithNanosecondsReadsBackTheWinnerAsTheExistingVersion(t *testing.T) {
	command := nanosecondCredentialCommand(t)
	for name, tc := range map[string]struct {
		shift time.Duration
		want  application.CredentialRegistrationOutcome
	}{
		"same command": {0, application.CredentialExistingVersion},
		"one µs apart": {time.Microsecond, application.CredentialContentConflict},
	} {
		t.Run(name, func(t *testing.T) {
			winner := credentialOnRegistry(t, command).records[0]
			registry := &credentialRegistryDouble{raceWinner: &winner}
			loser := command
			loser.EffectiveFrom = loser.EffectiveFrom.Add(tc.shift)
			result, err := newCredentialHandler(registry).Register(t.Context(), loser)
			if err != nil || result.Outcome() != tc.want {
				t.Fatalf("并发输家 = %s %v，期望 %s", result.Outcome(), err, tc.want)
			}
		})
	}
}

// Covers: 凭证改变适用关系——改变时间带纳秒的命令原样重放答`已有版本`；跨一微秒答`内容冲突`。
func TestACredentialApplicabilityChangeReplayedWithNanosecondsIsTheExistingVersion(t *testing.T) {
	registry := credentialOnRegistry(t, nanosecondCredentialCommand(t))
	handler := newCredentialHandler(registry)
	revoke := changeCredentialCommand(t, "carrier-x/1Z-NS", domain.CredentialRevoked, "v2")
	stored := revoke
	stored.At = onRegistry(precisionChanged)
	if result, err := handler.ChangeApplicability(t.Context(), stored); err != nil || result.Outcome() != application.CredentialApplicabilityChanged {
		t.Fatalf("预置改变：%v %s", err, result.Outcome())
	}

	revoke.At = precisionChanged
	replay, err := handler.ChangeApplicability(t.Context(), revoke)
	if err != nil || replay.Outcome() != application.CredentialExistingVersion {
		t.Fatalf("带纳秒原样重放 = %s %v，期望 EXISTING_VERSION", replay.Outcome(), err)
	}
	apart := revoke
	apart.At = precisionChanged.Add(time.Microsecond)
	if result, err := handler.ChangeApplicability(t.Context(), apart); err != nil || result.Outcome() != application.CredentialContentConflict {
		t.Fatalf("跨一微秒 = %s %v，期望 CONTENT_CONFLICT", result.Outcome(), err)
	}
	if len(registry.records) != 2 {
		t.Fatalf("重放与冲突都不落版本：%d", len(registry.records))
	}
}

// Covers: 总单形成新版本——改变时间带纳秒的命令原样重放答`已有版本`；跨一微秒答`内容冲突`。
func TestAMasterDocumentRevisionReplayedWithNanosecondsIsTheExistingVersion(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-NS", "MDV-1"))
	revoke := reviseMasterDocumentCommand(t, "SYN-MAWB-NS", domain.MasterDocumentRevocation, "MDV-2")
	stored := revoke
	stored.At = onRegistry(precisionChanged)
	if result, err := handler.Revise(t.Context(), stored); err != nil || result.Outcome() != application.MasterDocumentRevised {
		t.Fatalf("预置撤销：%v %s", err, result.Outcome())
	}

	revoke.At = precisionChanged
	if result, err := handler.Revise(t.Context(), revoke); err != nil || result.Outcome() != application.MasterDocumentExistingVersion {
		t.Fatalf("带纳秒原样重放 = %s %v，期望 EXISTING_VERSION", result.Outcome(), err)
	}
	apart := revoke
	apart.At = onRegistry(precisionChanged).Add(time.Microsecond)
	if result, err := handler.Revise(t.Context(), apart); err != nil || result.Outcome() != application.MasterDocumentContentConflict {
		t.Fatalf("跨一微秒 = %s %v，期望 CONTENT_CONFLICT", result.Outcome(), err)
	}
	if len(registry.records) != 2 {
		t.Fatalf("重放与冲突都不落版本：%d", len(registry.records))
	}
}

// Covers: 有效时间显式判断——有效时间带纳秒的命令原样重放答`已按所给判断`，不签新版，重发的意图仍指原版本
// （去重在交接一侧按含版本的事件 ID）；跨一微秒是改判，签新版。
func TestAnExplicitJudgmentReplayedWithNanosecondsIsAlreadyJudgedAsGiven(t *testing.T) {
	fixture := newAdoptionFixture(knownCredentials())
	factRef := adoptPending(t, fixture)
	handler := newJudgeHandler(fixture)
	command := application.JudgeEffectiveTimeCommand{
		TenantID: value(t, domain.NewTenantID, "tenant-1"), Fact: factRef.String(), EffectiveAt: precisionFrom,
	}
	stored := command
	stored.EffectiveAt = onRegistry(precisionFrom)
	first, err := handler.Judge(t.Context(), stored)
	if err != nil || first.Outcome() != application.EffectiveTimeJudged {
		t.Fatalf("预置判断：%v %s", err, first.Outcome())
	}
	judged, _ := first.Record()

	replay, err := handler.Judge(t.Context(), command)
	if err != nil || replay.Outcome() != application.EffectiveTimeAlreadyJudgedAsGiven {
		t.Fatalf("带纳秒原样重放 = %s %v，期望 ALREADY_JUDGED_AS_GIVEN", replay.Outcome(), err)
	}
	if len(fixture.registry.records) != 2 {
		t.Fatalf("重放不得签新版：%d", len(fixture.registry.records))
	}
	if intents := fixture.handoff.intents; len(intents) == 0 || intents[len(intents)-1].Record.Key != judged.Key {
		t.Fatal("重放应重发意图，且仍指原版本")
	}

	apart := command
	apart.EffectiveAt = onRegistry(precisionFrom).Add(time.Microsecond)
	if result, err := handler.Judge(t.Context(), apart); err != nil || result.Outcome() != application.EffectiveTimeJudged {
		t.Fatalf("跨一微秒 = %s %v，期望改判签新版", result.Outcome(), err)
	}
}

// nanosecondDeliveryCommand 交回一条各时刻格都带纳秒尾巴的派送尝试命令。
func nanosecondDeliveryCommand(fixture *deliveryAttemptFixture) application.RecordDeliveryAttemptCommand {
	delivered := fixture.delivered("parcel-1")
	delivered.OccurredAt = delivered.OccurredAt.Add(111111111)
	command := fixture.command(delivered)
	command.PlannedFrom = command.PlannedFrom.Add(123456789)
	command.PlannedTo = command.PlannedTo.Add(987654321)
	command.ArrivedAt = command.ArrivedAt.Add(555555555)
	return command
}

func deliveryOnRegistry(command application.RecordDeliveryAttemptCommand) application.RecordDeliveryAttemptCommand {
	stored := command
	stored.PlannedFrom, stored.PlannedTo, stored.ArrivedAt = onRegistry(command.PlannedFrom), onRegistry(command.PlannedTo), onRegistry(command.ArrivedAt)
	stored.Objects = append([]application.ObjectDeliverySubmission(nil), command.Objects...)
	for index := range stored.Objects {
		stored.Objects[index].OccurredAt = onRegistry(stored.Objects[index].OccurredAt)
	}
	return stored
}

// deliveryShifts 把派送尝试的每个时刻格各推后一微秒（自登记册上的值起算）。
var deliveryShifts = map[string]func(*application.RecordDeliveryAttemptCommand){
	"planned from": func(c *application.RecordDeliveryAttemptCommand) {
		c.PlannedFrom = onRegistry(c.PlannedFrom).Add(time.Microsecond)
	},
	"planned to": func(c *application.RecordDeliveryAttemptCommand) {
		c.PlannedTo = onRegistry(c.PlannedTo).Add(time.Microsecond)
	},
	"arrived at": func(c *application.RecordDeliveryAttemptCommand) {
		c.ArrivedAt = onRegistry(c.ArrivedAt).Add(time.Microsecond)
	},
	"occurred at": func(c *application.RecordDeliveryAttemptCommand) {
		c.Objects = append([]application.ObjectDeliverySubmission(nil), c.Objects...)
		c.Objects[0].OccurredAt = onRegistry(c.Objects[0].OccurredAt).Add(time.Microsecond)
	},
}

// Covers: 派送尝试——计划窗两端、到场时刻与逐对象发生时刻都带纳秒的命令原样重放答`原结果`；任一格跨一微秒答`来源冲突`。
func TestADeliveryAttemptReplayedWithNanosecondsReturnsTheExistingResult(t *testing.T) {
	fixture := newDeliveryAttemptFixture(t)
	command := nanosecondDeliveryCommand(fixture)
	if got := fixture.handle(deliveryOnRegistry(command)).Outcome().String(); got != "ATTEMPT_RECORDED" {
		t.Fatalf("预置尝试 = %s", got)
	}

	if got := fixture.handle(command).Outcome().String(); got != "EXISTING_RESULT" {
		t.Fatalf("带纳秒原样重放 = %s，期望 EXISTING_RESULT", got)
	}
	for name, shift := range deliveryShifts {
		apart := command
		shift(&apart)
		if got := fixture.handle(apart).Outcome().String(); got != "SOURCE_CONFLICT" {
			t.Fatalf("%s 跨一微秒 = %s，期望 SOURCE_CONFLICT", name, got)
		}
	}
	if len(fixture.store.records) != 1 {
		t.Fatalf("重放与冲突都不落记录：%d", len(fixture.store.records))
	}
}
