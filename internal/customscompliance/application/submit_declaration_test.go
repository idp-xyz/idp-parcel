package application_test

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/customscompliance/ports"
)

var (
	declarationJudgedAt = time.Date(2026, 8, 13, 8, 0, 0, 0, time.UTC)
	declarationSentAt   = time.Date(2026, 8, 13, 9, 0, 0, 0, time.UTC)
	declarationFixedAt  = time.Date(2026, 8, 13, 8, 30, 0, 0, time.UTC)
)

type submissionStoreDouble struct {
	records map[string]ports.DeclarationSubmissionRecord
	findErr error
	saveErr error
	// forceResult/saveResult 强制 Save 的答案；forceCorrection 强制 SaveCorrection 答
	// `当前版已被换`（模拟并发更正赢家先落，见 correct_declaration_test.go）。
	saveResult      ports.DeclarationSubmissionSaveOutcome
	forceResult     bool
	forceCorrection bool
	saves           int
}

func newSubmissionStore() *submissionStoreDouble {
	return &submissionStoreDouble{records: map[string]ports.DeclarationSubmissionRecord{}}
}

func submissionKey(key ports.DeclarationSubmissionKey) string {
	return key.TenantID.String() + "|" + key.Unit.String() + "|" + key.Procedure.String()
}

func (double *submissionStoreDouble) FindByKey(
	_ context.Context,
	key ports.DeclarationSubmissionKey,
) (ports.DeclarationSubmissionRecord, bool, error) {
	if double.findErr != nil {
		return ports.DeclarationSubmissionRecord{}, false, double.findErr
	}
	record, found := double.records[submissionKey(key)]
	return record, found, nil
}

func (double *submissionStoreDouble) Save(
	_ context.Context,
	record ports.DeclarationSubmissionRecord,
) (ports.DeclarationSubmissionSaveOutcome, error) {
	double.saves++
	if double.saveErr != nil {
		return ports.DeclarationSubmissionSaveOutcomeInvalid, double.saveErr
	}
	if double.forceResult {
		return double.saveResult, nil
	}
	if _, exists := double.records[submissionKey(record.Key)]; exists {
		return ports.DeclarationSubmissionAlreadyRecorded, nil
	}
	double.records[submissionKey(record.Key)] = record
	return ports.DeclarationSubmissionSaved, nil
}

// submissionCaseAuthorityDouble 是提交链案件反查的替身：只按标识答在册与否——用例
// 只消费 found 位（ADR-0073 决定五核存在），案件内容不进提交判断。
type submissionCaseAuthorityDouble struct {
	ids map[string]bool
	err error
}

func (double *submissionCaseAuthorityDouble) FindByID(
	_ context.Context,
	_ domain.TenantID,
	id domain.CustomsCaseID,
) (domain.CustomsCase, bool, error) {
	if double.err != nil {
		return domain.CustomsCase{}, false, double.err
	}
	return domain.CustomsCase{}, double.ids[id.String()], nil
}

func (double *submissionCaseAuthorityDouble) FindByKey(
	_ context.Context,
	_ ports.CustomsCaseKey,
) (domain.CustomsCase, bool, error) {
	return domain.CustomsCase{}, false, nil
}

func (double *submissionCaseAuthorityDouble) Save(
	_ context.Context,
	_ ports.CustomsCaseKey,
	_ domain.CustomsCase,
) (ports.CustomsCaseSaveOutcome, error) {
	return ports.CustomsCaseSaved, nil
}

// unitStoreDouble 照真库代数：同键只答`已有记录`绝不顶替，比对归编排。
type unitStoreDouble struct {
	units   map[string]domain.DeclarationUnit
	saveErr error
	findErr error
	saves   int
}

func newUnitStore() *unitStoreDouble {
	return &unitStoreDouble{units: map[string]domain.DeclarationUnit{}}
}

func unitKey(tenant domain.TenantID, unit domain.DeclarationUnitID) string {
	return tenant.String() + "|" + unit.String()
}

func (double *unitStoreDouble) Save(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnit,
	_ time.Time,
) (ports.DeclarationUnitSaveOutcome, error) {
	if double.saveErr != nil {
		return ports.DeclarationUnitSaveOutcomeInvalid, double.saveErr
	}
	double.saves++
	key := unitKey(tenant, unit.ID())
	if _, exists := double.units[key]; exists {
		return ports.DeclarationUnitAlreadyRecorded, nil
	}
	double.units[key] = unit
	return ports.DeclarationUnitSaved, nil
}

func (double *unitStoreDouble) FindByID(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.DeclarationUnit, bool, error) {
	if double.findErr != nil {
		return domain.DeclarationUnit{}, false, double.findErr
	}
	stored, found := double.units[unitKey(tenant, unit)]
	return stored, found, nil
}

type readinessViewDouble struct {
	configured bool
	revoked    bool
	err        error
}

func (double *readinessViewDouble) LoadReadiness(
	_ context.Context,
	_ domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.ReadinessJudgment, bool, error) {
	if double.err != nil {
		return domain.ReadinessJudgment{}, false, double.err
	}
	if !double.configured {
		return domain.ReadinessJudgment{}, false, nil
	}
	basis, err := domain.NewReadinessBasisReference("readiness-basis-1")
	if err != nil {
		return domain.ReadinessJudgment{}, false, err
	}
	judgment, err := domain.JudgeReady(unit, basis, declarationJudgedAt)
	if err != nil {
		return domain.ReadinessJudgment{}, false, err
	}
	if double.revoked {
		judgment, err = judgment.Revoke("dossier-changed", declarationJudgedAt.Add(time.Minute))
		if err != nil {
			return domain.ReadinessJudgment{}, false, err
		}
	}
	return judgment, true, nil
}

type authorityViewDouble struct {
	granted bool
	revoked bool
	err     error
}

func (double *authorityViewDouble) LoadSubmissionAuthority(
	_ context.Context,
	_ domain.TenantID,
	unit domain.DeclarationUnitID,
) (domain.SubmissionAuthorization, bool, error) {
	if double.err != nil {
		return domain.SubmissionAuthorization{}, false, double.err
	}
	if !double.granted {
		return domain.SubmissionAuthorization{}, false, nil
	}
	authority, err := domain.NewSubmissionAuthorityReference("submission-authority-1")
	if err != nil {
		return domain.SubmissionAuthorization{}, false, err
	}
	authorization, err := domain.GrantSubmissionAuthority(unit, authority, declarationJudgedAt)
	if err != nil {
		return domain.SubmissionAuthorization{}, false, err
	}
	if double.revoked {
		authorization, err = authorization.Revoke("mandate-withdrawn", declarationJudgedAt.Add(time.Minute))
		if err != nil {
			return domain.SubmissionAuthorization{}, false, err
		}
	}
	return authorization, true, nil
}

type versionFactoryDouble struct {
	minted int
	err    error
}

func (double *versionFactoryDouble) NextSubmissionVersion(context.Context) (domain.SubmissionVersionID, error) {
	if double.err != nil {
		return domain.SubmissionVersionID{}, double.err
	}
	double.minted++
	return domain.NewSubmissionVersionID(fmt.Sprintf("submission/v%d", double.minted))
}

type declarationHandoffDouble struct {
	intents []ports.DeclarationSubmissionHandoffIntent
	err     error
}

func (double *declarationHandoffDouble) HandOffDeclarationSubmission(
	_ context.Context,
	intent ports.DeclarationSubmissionHandoffIntent,
) error {
	if double.err != nil {
		return double.err
	}
	double.intents = append(double.intents, intent)
	return nil
}

type declarationClock struct{ at time.Time }

func (clock declarationClock) Now() time.Time { return clock.at }

type declarationFixture struct {
	store     *submissionStoreDouble
	cases     *submissionCaseAuthorityDouble
	units     *unitStoreDouble
	readiness *readinessViewDouble
	authority *authorityViewDouble
	versions  *versionFactoryDouble
	handoff   *declarationHandoffDouble
	handler   *application.SubmitDeclarationHandler
}

func newDeclarationFixture(t *testing.T) *declarationFixture {
	t.Helper()
	fixture := &declarationFixture{
		store:     newSubmissionStore(),
		cases:     &submissionCaseAuthorityDouble{ids: map[string]bool{"case-1": true}},
		units:     newUnitStore(),
		readiness: &readinessViewDouble{configured: true},
		authority: &authorityViewDouble{granted: true},
		versions:  &versionFactoryDouble{},
		handoff:   &declarationHandoffDouble{},
	}
	fixture.handler = application.NewSubmitDeclarationHandler(application.SubmitDeclarationDeps{
		Submissions: fixture.store,
		Cases:       fixture.cases,
		Units:       fixture.units,
		Readiness:   fixture.readiness,
		Authority:   fixture.authority,
		Versions:    fixture.versions,
		Downstream:  fixture.handoff,
		Clock:       declarationClock{at: declarationFixedAt},
	})
	return fixture
}

func declarationCommand(t *testing.T) application.SubmitDeclarationCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.SubmitDeclarationCommand{
		TenantID:      tenant,
		UnitID:        "declaration-unit-1",
		CaseID:        "case-1",
		Procedure:     "US-IMPORT-T86",
		Members:       []string{"parcel-1", "parcel-2"},
		Dossier:       "dossier-snapshot-1",
		Roles:         "role-snapshot-1",
		Target:        "customs-channel-1",
		InitialResult: domain.AttemptPendingConfirmation,
		SentAt:        declarationSentAt,
	}
}

// 双有效（就绪+授权）才成版：版本固定组成快照、首次尝试序号 1、意图交出一份。
func TestAReadyAuthorizedUnitFixesAVersionWithItsFirstAttempt(t *testing.T) {
	fixture := newDeclarationFixture(t)
	command := declarationCommand(t)
	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if result.Outcome() != application.DeclarationSubmitted {
		t.Fatalf("outcome = %q, want DECLARATION_SUBMITTED", result.Outcome())
	}
	record, present := result.Record()
	if !present {
		t.Fatal("no record returned")
	}
	// 计数锚定本夹具：两个成员（parcel-1/parcel-2，固定于 2026-08-13T08:30Z）。
	if len(record.Version.Members()) != 2 {
		t.Fatalf("members = %d, want 2（组成在成版时快照固定）", len(record.Version.Members()))
	}
	if record.Attempt.Sequence() != 1 {
		t.Fatalf("attempt sequence = %d, want 1", record.Attempt.Sequence())
	}
	if record.Attempt.Result() != domain.AttemptPendingConfirmation {
		t.Fatalf("attempt result = %q", record.Attempt.Result())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d, want 1", len(fixture.handoff.intents))
	}
	// 意图带案件维（ADR-0073 决定五）：载荷从这里取，缺了适配器会响亮拒。
	if fixture.handoff.intents[0].Case.String() != "case-1" {
		t.Fatalf("intent case = %q, want case-1", fixture.handoff.intents[0].Case)
	}
	// 单元本体随提交落册（决定一）：身份、案件与组成一字不差。
	storedUnit, unitFound, err := fixture.units.FindByID(
		context.Background(), command.TenantID, record.Key.Unit)
	if err != nil || !unitFound {
		t.Fatalf("单元没落册：err=%v found=%v", err, unitFound)
	}
	if storedUnit.Case().String() != "case-1" {
		t.Fatalf("册上单元的案件维 = %q", storedUnit.Case())
	}
}

// Covers: CONTEXT「首次实际对外发送前都必须形成不可覆盖的提交版本」——重放返回原版本且不重
// 形成（版本厂只签一次）；同目标不同内容不顶替已固定版本。
func TestAReplayReturnsTheOriginalVersionWithoutReforming(t *testing.T) {
	fixture := newDeclarationFixture(t)
	command := declarationCommand(t)

	first, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("first handle: %v", err)
	}
	firstRecord, _ := first.Record()

	replay, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("replay handle: %v", err)
	}
	if replay.Outcome() != application.DeclarationExistingVersion {
		t.Fatalf("outcome = %q, want EXISTING_VERSION", replay.Outcome())
	}
	replayRecord, _ := replay.Record()
	if replayRecord.Version.ID() != firstRecord.Version.ID() {
		t.Fatal("重放换了版本——原版本被顶替")
	}
	if fixture.versions.minted != 1 {
		t.Fatalf("versions minted = %d, want 1（重放不重形成）", fixture.versions.minted)
	}
	if fixture.store.saves != 1 {
		t.Fatalf("saves = %d, want 1", fixture.store.saves)
	}
	if len(fixture.handoff.intents) != 2 {
		t.Fatalf("intents = %d, want 2（重放重发同一份）", len(fixture.handoff.intents))
	}

	t.Run("a different content under the same target is a conflict", func(t *testing.T) {
		flipped := declarationCommand(t)
		flipped.Dossier = "dossier-snapshot-2"
		result, err := fixture.handler.Handle(context.Background(), flipped)
		if err != nil {
			t.Fatalf("conflict handle: %v", err)
		}
		if result.Outcome() != application.DeclarationSourceConflict {
			t.Fatalf("outcome = %q, want SOURCE_CONFLICT（已固定版本不可覆盖，修订走撤销重报）", result.Outcome())
		}
		if fixture.versions.minted != 1 {
			t.Fatal("冲突还签了新版本")
		}
	})
}

// Covers: CONTEXT 244「就绪判断与提交授权分别形成和失效，双有效才成版」——就绪未配置
// 与授权未配置各占一格未决（互不顶替）；不再就绪是业务负向不是未决。
func TestReadinessAndAuthorityAreTwoSeparateTracks(t *testing.T) {
	t.Run("unconfigured readiness is undecided", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.readiness.configured = false
		result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationUndecided ||
			result.UndecidedReason() != application.ReadinessUnconfigured {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
		if fixture.versions.minted != 0 || len(fixture.store.records) != 0 {
			t.Fatal("未决还签版或落库")
		}
	})

	t.Run("no-longer-ready is a business negative, not undecided", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.readiness.revoked = true
		result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationNotReady {
			t.Fatalf("outcome = %q, want NOT_READY（原判断保留但不得继续支持实际提交）", result.Outcome())
		}
		if result.UndecidedReason() != application.DeclarationUndecidedReasonNone {
			t.Fatalf("reason = %q；业务负向不指名依赖", result.UndecidedReason())
		}
		if len(fixture.store.records) != 0 {
			t.Fatal("不再就绪还落了库")
		}
	})

	t.Run("readiness alone cannot stand in for authority", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.authority.granted = false
		result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationUndecided ||
			result.UndecidedReason() != application.AuthorityUnconfigured {
			t.Fatalf("outcome = %q reason = %q（就绪在场也顶替不了授权）", result.Outcome(), result.UndecidedReason())
		}
		if fixture.versions.minted != 0 {
			t.Fatal("授权缺席还签了版本")
		}
	})

	t.Run("a revoked authorization is a business negative, not unconfigured", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.authority.revoked = true
		result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationNotAuthorized {
			t.Fatalf("outcome = %q, want NOT_AUTHORIZED（失效不是未配置——恢复动作是重新取得授权，不是等实例参数）", result.Outcome())
		}
		if result.UndecidedReason() != application.DeclarationUndecidedReasonNone {
			t.Fatalf("reason = %q；业务负向不指名依赖", result.UndecidedReason())
		}
		if fixture.versions.minted != 0 || len(fixture.store.records) != 0 {
			t.Fatal("失效授权还签版或落库")
		}
	})

	t.Run("the undecided reason set is closed", func(t *testing.T) {
		labels := map[string]struct{}{}
		for _, reason := range []application.DeclarationUndecidedReason{
			application.SubmissionStoreUnavailable, application.ReadinessUnavailable,
			application.ReadinessUnconfigured, application.AuthorityUnavailable,
			application.AuthorityUnconfigured, application.VersionIdentityUnavailable,
			application.CaseAuthorityUnavailable, application.UnitStoreUnavailable,
			application.FollowUpStoreUnavailable,
		} {
			label := reason.String()
			if label == "" {
				t.Fatalf("reason %d has no label", reason)
			}
			labels[label] = struct{}{}
		}
		if len(labels) != 9 {
			t.Fatalf("labels collapsed into %d", len(labels))
		}
		if application.DeclarationUndecidedReason(len(labels)+1).String() != "" {
			t.Fatal("第十个未决原因带了标签——封闭集合被悄悄放开")
		}
	})
}

// Covers: ADR-0073 决定一/二/五——案件先于申报存在（悬空引用拒、反查故障未决）；单元
// 本体成立即定（同单元换案件/换程序是身份冲突，重放与新提交两条路都拦）；单元库故障
// 未决。
func TestTheUnitCaseDimensionIsFixedAtFormation(t *testing.T) {
	t.Run("an unknown case is refused before anything lands", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		command := declarationCommand(t)
		command.CaseID = "case-never-established"
		result, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationCaseUnknown {
			t.Fatalf("outcome = %q, want CASE_UNKNOWN（案件先于申报存在，建案后重来）", result.Outcome())
		}
		if len(fixture.store.records) != 0 || len(fixture.units.units) != 0 || fixture.versions.minted != 0 {
			t.Fatal("悬空案件引用还落了库")
		}
	})

	t.Run("a case lookup failure is undecided", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.cases.err = errors.New("case authority down")
		result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationUndecided ||
			result.UndecidedReason() != application.CaseAuthorityUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("a unit store failure is undecided", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.units.saveErr = errors.New("unit store down")
		result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationUndecided ||
			result.UndecidedReason() != application.UnitStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("replaying with a different case is a unit conflict, not a replay", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.cases.ids["case-2"] = true
		if _, err := fixture.handler.Handle(context.Background(), declarationCommand(t)); err != nil {
			t.Fatalf("first handle: %v", err)
		}
		flipped := declarationCommand(t)
		flipped.CaseID = "case-2"
		result, err := fixture.handler.Handle(context.Background(), flipped)
		if err != nil {
			t.Fatalf("replay handle: %v", err)
		}
		if result.Outcome() != application.DeclarationUnitConflict {
			t.Fatalf("outcome = %q, want UNIT_CONFLICT（换案件即替代单元，不是重放）", result.Outcome())
		}
		if stored := fixture.units.units["tenant-1|declaration-unit-1"]; stored.Case().String() != "case-1" {
			t.Fatalf("册上单元的案件维被顶成 %q", stored.Case())
		}
	})

	t.Run("a fresh submission against a unit bound to another shape is a unit conflict", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), declarationCommand(t)); err != nil {
			t.Fatalf("first handle: %v", err)
		}
		// 换程序换出新的提交键（不走重放路），单元身份却还是同一个——身份上程序已定。
		changed := declarationCommand(t)
		changed.Procedure = "US-EXPORT-STANDARD"
		result, err := fixture.handler.Handle(context.Background(), changed)
		if err != nil {
			t.Fatalf("changed handle: %v", err)
		}
		if result.Outcome() != application.DeclarationUnitConflict {
			t.Fatalf("outcome = %q, want UNIT_CONFLICT（单元身份成立即定）", result.Outcome())
		}
		if fixture.versions.minted != 1 {
			t.Fatalf("versions minted = %d, want 1（输给身份的请求不签版本）", fixture.versions.minted)
		}
	})
}

// 未受理与恢复纪律：单元形状坏/缺快照/缺目标 → 未受理不落库；库故障未决；投递失败不翻
// 结果、重放重发；并发落败读回赢家；写入代数外是编程错误。
func TestMalformedInputsAndRecoveryDiscipline(t *testing.T) {
	broken := map[string]func(*application.SubmitDeclarationCommand){
		"no members": func(command *application.SubmitDeclarationCommand) { command.Members = nil },
		"duplicate member": func(command *application.SubmitDeclarationCommand) {
			command.Members = []string{"parcel-1", "parcel-1"}
		},
		"no case":    func(command *application.SubmitDeclarationCommand) { command.CaseID = " " },
		"no dossier": func(command *application.SubmitDeclarationCommand) { command.Dossier = " " },
		"no roles":   func(command *application.SubmitDeclarationCommand) { command.Roles = " " },
		"no target":  func(command *application.SubmitDeclarationCommand) { command.Target = " " },
		"invalid attempt result": func(command *application.SubmitDeclarationCommand) {
			command.InitialResult = domain.AttemptResultInvalid
		},
	}
	for name, breakCommand := range broken {
		t.Run(name, func(t *testing.T) {
			fixture := newDeclarationFixture(t)
			command := declarationCommand(t)
			breakCommand(&command)
			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			if result.Outcome() != application.DeclarationNotAccepted {
				t.Fatalf("outcome = %q, want SOURCE_NOT_ACCEPTED", result.Outcome())
			}
			if len(fixture.store.records) != 0 || len(fixture.handoff.intents) != 0 {
				t.Fatal("未受理的提交落了库或交了意图")
			}
		})
	}

	t.Run("a handoff failure keeps the outcome and is resent on replay", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.handoff.err = errors.New("downstream unavailable")
		command := declarationCommand(t)
		first, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("first handle: %v", err)
		}
		if first.Outcome() != application.DeclarationSubmitted || first.SubmissionHandoffReference() == "" {
			t.Fatalf("outcome = %q handoff = %q（投递失败不翻结果）", first.Outcome(), first.SubmissionHandoffReference())
		}
		fixture.handoff.err = nil
		replay, err := fixture.handler.Handle(context.Background(), command)
		if err != nil {
			t.Fatalf("replay: %v", err)
		}
		if replay.SubmissionHandoffReference() != "" || len(fixture.handoff.intents) != 1 {
			t.Fatalf("intents = %d handoff = %q（重放重发同一份）", len(fixture.handoff.intents), replay.SubmissionHandoffReference())
		}
	})

	t.Run("a store failure is undecided with its reason", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.store.findErr = errors.New("store down")
		result, err := fixture.handler.Handle(context.Background(), declarationCommand(t))
		if err != nil {
			t.Fatalf("handle: %v", err)
		}
		if result.Outcome() != application.DeclarationUndecided ||
			result.UndecidedReason() != application.SubmissionStoreUnavailable {
			t.Fatalf("outcome = %q reason = %q", result.Outcome(), result.UndecidedReason())
		}
	})

	t.Run("an unexpected save outcome is a programming error", func(t *testing.T) {
		fixture := newDeclarationFixture(t)
		fixture.store.forceResult = true
		fixture.store.saveResult = ports.DeclarationSubmissionSaveOutcome(99)
		if _, err := fixture.handler.Handle(context.Background(), declarationCommand(t)); !errors.Is(err, application.ErrUnexpectedSubmissionSave) {
			t.Fatalf("error = %v, want ErrUnexpectedSubmissionSave", err)
		}
	})
}
