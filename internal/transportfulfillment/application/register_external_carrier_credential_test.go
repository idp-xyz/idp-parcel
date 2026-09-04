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

// 本文件证凭证登记编排（label-channel/18）的结果代数：首登、重放、冲突、已有版本链、三种改变
// 适用关系、未登记、已不适用、未受理、未决，以及并发撞键读回赢家。登记册用内存替身，真库那一半
// 在 adapters/postgres 的用例里。

var (
	credentialFromAt = time.Date(2026, 9, 1, 0, 0, 0, 0, time.UTC)
	credentialNowAt  = time.Date(2026, 9, 4, 12, 0, 0, 0, time.UTC)
)

type credentialClock struct{ at time.Time }

func (clock credentialClock) Now() time.Time { return clock.at }

// credentialRegistryDouble 是只插不改的内存登记册；`当前版`同真实现一样按回指派生。
// raceWinner 非空时模拟并发：本方 Save 那一刻另一方先落了它，写口答已登记。
type credentialRegistryDouble struct {
	records    []ports.ExternalCarrierCredentialRecord
	failFind   error
	failSave   error
	raceWinner *ports.ExternalCarrierCredentialRecord
}

func (double *credentialRegistryDouble) FindByKey(
	_ context.Context,
	key ports.ExternalCarrierCredentialKey,
) (ports.ExternalCarrierCredentialRecord, bool, error) {
	if double.failFind != nil {
		return ports.ExternalCarrierCredentialRecord{}, false, double.failFind
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.ExternalCarrierCredentialRecord{}, false, nil
}

func (double *credentialRegistryDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	credential domain.ExternalCarrierCredentialReference,
) (ports.ExternalCarrierCredentialRecord, bool, error) {
	if double.failFind != nil {
		return ports.ExternalCarrierCredentialRecord{}, false, double.failFind
	}
	superseded := map[domain.ExternalCarrierCredentialVersion]bool{}
	for _, record := range double.records {
		if record.Key.TenantID != tenant || record.Key.Credential != credential {
			continue
		}
		if prior, has := record.Credential.Supersedes(); has {
			superseded[prior] = true
		}
	}
	var current ports.ExternalCarrierCredentialRecord
	found := false
	for _, record := range double.records {
		if record.Key.TenantID != tenant || record.Key.Credential != credential || superseded[record.Key.Version] {
			continue
		}
		if !found || record.RecordedAt.After(current.RecordedAt) {
			current, found = record, true
		}
	}
	return current, found, nil
}

func (double *credentialRegistryDouble) ListVersions(
	_ context.Context,
	tenant domain.TenantID,
	credential domain.ExternalCarrierCredentialReference,
) ([]ports.ExternalCarrierCredentialRecord, error) {
	var versions []ports.ExternalCarrierCredentialRecord
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Credential == credential {
			versions = append(versions, record)
		}
	}
	return versions, nil
}

func (double *credentialRegistryDouble) Save(
	_ context.Context,
	record ports.ExternalCarrierCredentialRecord,
) (ports.ExternalCarrierCredentialSaveOutcome, error) {
	if double.failSave != nil {
		return ports.ExternalCarrierCredentialSaveOutcomeInvalid, double.failSave
	}
	if double.raceWinner != nil {
		double.records = append(double.records, *double.raceWinner)
		double.raceWinner = nil
		return ports.ExternalCarrierCredentialAlreadyRegistered, nil
	}
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.ExternalCarrierCredentialAlreadyRegistered, nil
		}
	}
	double.records = append(double.records, record)
	return ports.ExternalCarrierCredentialSaved, nil
}

func newCredentialHandler(registry *credentialRegistryDouble) *application.RegisterExternalCarrierCredentialHandler {
	return application.NewRegisterExternalCarrierCredentialHandler(application.RegisterExternalCarrierCredentialDeps{
		Credentials: registry,
		Clock:       credentialClock{at: credentialNowAt},
	})
}

func registerCredentialCommand(t *testing.T, credential, version string) application.RegisterExternalCarrierCredentialCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	return application.RegisterExternalCarrierCredentialCommand{
		TenantID:       tenant,
		Credential:     credential,
		Version:        version,
		Assigner:       "carrier-x",
		IdentifiedKind: "CARRIED_OBJECT",
		IdentifiedRef:  "PCL-1",
		EffectiveFrom:  credentialFromAt,
	}
}

func changeCredentialCommand(t *testing.T, credential string, change domain.CredentialStanding, version string) application.ChangeCredentialApplicabilityCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	return application.ChangeCredentialApplicabilityCommand{
		TenantID:   tenant,
		Credential: credential,
		Change:     change,
		At:         credentialFromAt.Add(48 * time.Hour),
		NewVersion: version,
	}
}

func mustRegisterCredential(
	t *testing.T,
	handler *application.RegisterExternalCarrierCredentialHandler,
	command application.RegisterExternalCarrierCredentialCommand,
) application.RegisterExternalCarrierCredentialResult {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil {
		t.Fatalf("登记：%v", err)
	}
	return result
}

func TestFirstVersionRegistersOnceAndReplaysAsExistingVersion(t *testing.T) {
	registry := &credentialRegistryDouble{}
	handler := newCredentialHandler(registry)
	command := registerCredentialCommand(t, "carrier-x/1Z001", "ECV-1")

	first := mustRegisterCredential(t, handler, command)
	if first.Outcome() != application.CredentialRegistered {
		t.Fatalf("outcome = %s, want CREDENTIAL_REGISTERED", first.Outcome())
	}
	record, has := first.Record()
	if !has || !record.RecordedAt.Equal(credentialNowAt) || !record.Credential.Applicable() {
		t.Fatalf("首登应带回适用中的记录且登记时刻取时钟：%+v has=%v", record, has)
	}
	if record.Credential.Identifies().Kind() != domain.IdentifiesCarriedObject {
		t.Fatalf("类别词没有认回封闭集合：%s", record.Credential.Identifies().Kind())
	}

	replay := mustRegisterCredential(t, handler, command)
	if replay.Outcome() != application.CredentialExistingVersion {
		t.Fatalf("重放 outcome = %s, want EXISTING_VERSION", replay.Outcome())
	}
	if len(registry.records) != 1 {
		t.Fatalf("重放不该再落一版：%d", len(registry.records))
	}

	command.IdentifiedRef = "PCL-2"
	conflict := mustRegisterCredential(t, handler, command)
	if conflict.Outcome() != application.CredentialContentConflict {
		t.Fatalf("同版本异内容 outcome = %s, want CONTENT_CONFLICT", conflict.Outcome())
	}
	if kept, _ := conflict.Record(); kept.Credential.Identifies().Reference() != "PCL-1" {
		t.Fatal("冲突应保留原版本，不按最后到达顶替")
	}
}

func TestASecondFirstVersionForTheSameCredentialIsRefused(t *testing.T) {
	registry := &credentialRegistryDouble{}
	handler := newCredentialHandler(registry)
	mustRegisterCredential(t, handler, registerCredentialCommand(t, "carrier-x/1Z002", "ECV-1"))

	result := mustRegisterCredential(t, handler, registerCredentialCommand(t, "carrier-x/1Z002", "ECV-2"))
	if result.Outcome() != application.CredentialAlreadyChained {
		t.Fatalf("outcome = %s, want CREDENTIAL_ALREADY_REGISTERED", result.Outcome())
	}
	if current, has := result.Record(); !has || current.Key.Version.String() != "ECV-1" {
		t.Fatalf("应带回撞上的当前版：%+v has=%v", current, has)
	}
	if len(registry.records) != 1 {
		t.Fatal("第二个首版落库了——当前版从此有两个答案")
	}
}

func TestRegistrationRefusesInputsTheDomainCannotForm(t *testing.T) {
	registry := &credentialRegistryDouble{}
	handler := newCredentialHandler(registry)
	for name, mutate := range map[string]func(*application.RegisterExternalCarrierCredentialCommand){
		"类别词不在封闭集合内": func(command *application.RegisterExternalCarrierCredentialCommand) {
			command.IdentifiedKind = "TRACKING_NUMBER"
		},
		"分配方缺失":    func(command *application.RegisterExternalCarrierCredentialCommand) { command.Assigner = "  " },
		"版本缺失":     func(command *application.RegisterExternalCarrierCredentialCommand) { command.Version = "" },
		"标识对象引用缺失": func(command *application.RegisterExternalCarrierCredentialCommand) { command.IdentifiedRef = "" },
		"适用起点缺失": func(command *application.RegisterExternalCarrierCredentialCommand) {
			command.EffectiveFrom = time.Time{}
		},
		"终点不晚于起点": func(command *application.RegisterExternalCarrierCredentialCommand) {
			command.EffectiveUntil = command.EffectiveFrom
		},
		"租户缺失": func(command *application.RegisterExternalCarrierCredentialCommand) {
			command.TenantID = domain.TenantID{}
		},
	} {
		t.Run(name, func(t *testing.T) {
			command := registerCredentialCommand(t, "carrier-x/1Z003", "ECV-1")
			mutate(&command)
			result := mustRegisterCredential(t, handler, command)
			if result.Outcome() != application.CredentialRegistrationNotAccepted {
				t.Fatalf("outcome = %s, want INPUT_NOT_ACCEPTED", result.Outcome())
			}
		})
	}
	if len(registry.records) != 0 {
		t.Fatalf("未受理的输入落库了：%d", len(registry.records))
	}
}

func TestRevocationFormsANewVersionThatBecomesCurrent(t *testing.T) {
	registry := &credentialRegistryDouble{}
	handler := newCredentialHandler(registry)
	mustRegisterCredential(t, handler, registerCredentialCommand(t, "carrier-x/1Z004", "ECV-1"))

	command := changeCredentialCommand(t, "carrier-x/1Z004", domain.CredentialRevoked, "ECV-2")
	result, err := handler.ChangeApplicability(t.Context(), command)
	if err != nil {
		t.Fatalf("作废：%v", err)
	}
	if result.Outcome() != application.CredentialApplicabilityChanged {
		t.Fatalf("outcome = %s, want APPLICABILITY_CHANGED", result.Outcome())
	}
	record, _ := result.Record()
	if record.Credential.Standing() != domain.CredentialRevoked || record.Key.Version.String() != "ECV-2" {
		t.Fatalf("新版本应是作废版 ECV-2：%s %s", record.Credential.Standing(), record.Key.Version)
	}
	if prior, has := record.Credential.Supersedes(); !has || prior.String() != "ECV-1" {
		t.Fatalf("新版本应回指首版：%q has=%v", prior, has)
	}
	current, found, _ := registry.FindCurrent(t.Context(), command.TenantID, record.Key.Credential)
	if !found || current.Key.Version.String() != "ECV-2" {
		t.Fatalf("当前版应移到作废版：%+v found=%v", current.Key, found)
	}

	replay, _ := handler.ChangeApplicability(t.Context(), command)
	if replay.Outcome() != application.CredentialExistingVersion {
		t.Fatalf("同一次改变重放 outcome = %s, want EXISTING_VERSION", replay.Outcome())
	}
	command.At = command.At.Add(time.Hour)
	conflict, _ := handler.ChangeApplicability(t.Context(), command)
	if conflict.Outcome() != application.CredentialContentConflict {
		t.Fatalf("同版本不同改变时间 outcome = %s, want CONTENT_CONFLICT", conflict.Outcome())
	}

	again, _ := handler.ChangeApplicability(t.Context(), changeCredentialCommand(t, "carrier-x/1Z004", domain.CredentialExpired, "ECV-3"))
	if again.Outcome() != application.CredentialNoLongerApplicable {
		t.Fatalf("对已作废的凭证再改 outcome = %s, want NO_LONGER_APPLICABLE", again.Outcome())
	}
	if len(registry.records) != 2 {
		t.Fatalf("只该有首版与作废版两条：%d", len(registry.records))
	}
}

func TestSupersessionNeedsAReplacementAndTheOthersRefuseOne(t *testing.T) {
	registry := &credentialRegistryDouble{}
	handler := newCredentialHandler(registry)
	mustRegisterCredential(t, handler, registerCredentialCommand(t, "carrier-x/1Z005", "ECV-1"))

	withoutReplacement := changeCredentialCommand(t, "carrier-x/1Z005", domain.CredentialSuperseded, "ECV-2")
	result, _ := handler.ChangeApplicability(t.Context(), withoutReplacement)
	if result.Outcome() != application.CredentialRegistrationNotAccepted {
		t.Fatalf("无替代者的替代 outcome = %s, want INPUT_NOT_ACCEPTED", result.Outcome())
	}

	selfReplacement := withoutReplacement
	selfReplacement.Replacement = "carrier-x/1Z005"
	result, _ = handler.ChangeApplicability(t.Context(), selfReplacement)
	if result.Outcome() != application.CredentialRegistrationNotAccepted {
		t.Fatalf("替代者是自己 outcome = %s, want INPUT_NOT_ACCEPTED", result.Outcome())
	}

	revokeWithReplacement := changeCredentialCommand(t, "carrier-x/1Z005", domain.CredentialRevoked, "ECV-2")
	revokeWithReplacement.Replacement = "carrier-x/1Z005-B"
	result, _ = handler.ChangeApplicability(t.Context(), revokeWithReplacement)
	if result.Outcome() != application.CredentialRegistrationNotAccepted {
		t.Fatalf("作废带替代者 outcome = %s, want INPUT_NOT_ACCEPTED", result.Outcome())
	}

	applicableChange := changeCredentialCommand(t, "carrier-x/1Z005", domain.CredentialApplicable, "ECV-2")
	result, _ = handler.ChangeApplicability(t.Context(), applicableChange)
	if result.Outcome() != application.CredentialRegistrationNotAccepted {
		t.Fatalf("改成`适用中` outcome = %s, want INPUT_NOT_ACCEPTED——那不是一种改变", result.Outcome())
	}

	supersede := withoutReplacement
	supersede.Replacement = "carrier-x/1Z005-B"
	result, err := handler.ChangeApplicability(t.Context(), supersede)
	if err != nil || result.Outcome() != application.CredentialApplicabilityChanged {
		t.Fatalf("替代：%v %s", err, result.Outcome())
	}
	record, _ := result.Record()
	if replacedBy, has := record.Credential.ReplacedBy(); !has || replacedBy.String() != "carrier-x/1Z005-B" {
		t.Fatalf("替代者没有落在新版本上：%q has=%v", replacedBy, has)
	}
}

func TestChangingANeverRegisteredCredentialAnswersNotRegistered(t *testing.T) {
	handler := newCredentialHandler(&credentialRegistryDouble{})
	result, err := handler.ChangeApplicability(t.Context(), changeCredentialCommand(t, "carrier-x/never", domain.CredentialRevoked, "ECV-2"))
	if err != nil || result.Outcome() != application.CredentialNotRegistered {
		t.Fatalf("outcome = %s (%v), want CREDENTIAL_NOT_REGISTERED", result.Outcome(), err)
	}
}

func TestReusingTheCurrentVersionNumberForAChangeIsAnOverwriteAndIsRefused(t *testing.T) {
	registry := &credentialRegistryDouble{}
	handler := newCredentialHandler(registry)
	mustRegisterCredential(t, handler, registerCredentialCommand(t, "carrier-x/1Z006", "ECV-1"))

	result, _ := handler.ChangeApplicability(t.Context(), changeCredentialCommand(t, "carrier-x/1Z006", domain.CredentialRevoked, "ECV-1"))
	if result.Outcome() != application.CredentialRegistrationNotAccepted {
		t.Fatalf("outcome = %s, want INPUT_NOT_ACCEPTED", result.Outcome())
	}
	if registry.records[0].Credential.Standing() != domain.CredentialApplicable {
		t.Fatal("首版被覆盖成不适用")
	}
}

func TestRegistryFailuresLeaveTheRegistrationUndecided(t *testing.T) {
	failing := &credentialRegistryDouble{failFind: errors.New("registry down")}
	handler := newCredentialHandler(failing)
	command := registerCredentialCommand(t, "carrier-x/1Z007", "ECV-1")

	result := mustRegisterCredential(t, handler, command)
	if result.Outcome() != application.CredentialRegistrationUndecided || result.UndecidedReason() != application.CredentialRegistryUnavailable {
		t.Fatalf("outcome = %s reason = %s, want REGISTRATION_UNDECIDED / CREDENTIAL_REGISTRY_UNAVAILABLE", result.Outcome(), result.UndecidedReason())
	}
	if result.ContinuationReference() == "" {
		t.Fatal("未决必须带续办引用")
	}

	change, _ := handler.ChangeApplicability(t.Context(), changeCredentialCommand(t, "carrier-x/1Z007", domain.CredentialRevoked, "ECV-2"))
	if change.Outcome() != application.CredentialRegistrationUndecided {
		t.Fatalf("改变入口 outcome = %s, want REGISTRATION_UNDECIDED", change.Outcome())
	}

	saveFailing := &credentialRegistryDouble{failSave: errors.New("write refused")}
	result = mustRegisterCredential(t, newCredentialHandler(saveFailing), command)
	if result.Outcome() != application.CredentialRegistrationUndecided {
		t.Fatalf("写失败 outcome = %s, want REGISTRATION_UNDECIDED", result.Outcome())
	}
}

func TestALostRaceReadsBackTheWinner(t *testing.T) {
	// 先在一本干净的册子上形成赢家那一版，拿它当另一方先落的记录。
	seed := &credentialRegistryDouble{}
	command := registerCredentialCommand(t, "carrier-x/1Z008", "ECV-1")
	mustRegisterCredential(t, newCredentialHandler(seed), command)
	winner := seed.records[0]

	// 另一方在本方 FindByKey 之后、Save 之前落了同一版：写口答已登记，编排读回赢家比内容。
	sameContent := &credentialRegistryDouble{raceWinner: &winner}
	result := mustRegisterCredential(t, newCredentialHandler(sameContent), command)
	if result.Outcome() != application.CredentialExistingVersion {
		t.Fatalf("赢家内容相同应答 EXISTING_VERSION：%s", result.Outcome())
	}

	otherContent := &credentialRegistryDouble{raceWinner: &winner}
	command.IdentifiedRef = "PCL-2"
	result = mustRegisterCredential(t, newCredentialHandler(otherContent), command)
	if result.Outcome() != application.CredentialContentConflict {
		t.Fatalf("赢家内容不同应答 CONTENT_CONFLICT：%s", result.Outcome())
	}
	if kept, _ := result.Record(); kept.Credential.Identifies().Reference() != "PCL-1" {
		t.Fatal("冲突应带回赢家那一版")
	}
}
