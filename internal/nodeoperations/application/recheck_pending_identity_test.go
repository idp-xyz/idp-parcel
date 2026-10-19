package application_test

import (
	"context"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证重核编排（ADR-0199 决定三，票 no-reception-source/03）：按存下的号再问一次 PS，结果唯一即追加关联版，其余各格
// 照决定三分格。收寄库替身沿用收寄编排那一只，在这里补上按判断版本与按号码找待识别的两问。

// FindByJudgmentVersion 照真库在内存里找：同租户、判断版本相等的那一行。
func (double *receptionStoreDouble) FindByJudgmentVersion(
	_ context.Context,
	tenant domain.TenantID,
	version domain.IntakeResultVersion,
) (ports.ReceptionRecord, bool, error) {
	for key, record := range double.byKey {
		if key.TenantID == tenant && record.JudgmentVersion == version {
			return record, true, nil
		}
	}
	return ports.ReceptionRecord{}, false, nil
}

// FindRecheckableByMark 照真库的筛法在内存里找：当前有效版本是待识别、不带关联、不记身份冲突、来源文档的号逐字相等。
func (double *receptionStoreDouble) FindRecheckableByMark(
	_ context.Context,
	tenant domain.TenantID,
	mark string,
) ([]ports.ReceptionRecord, error) {
	found := []ports.ReceptionRecord{}
	for key, record := range double.byKey {
		if key.TenantID != tenant || record.Kind != ports.RecordPendingIdentification || record.IdentityConflict ||
			!record.Source.Preserved() || record.Source.Mark != mark || record.JudgmentVersion.String() == "" {
			continue
		}
		if _, associated := record.Intake.Association(); associated {
			continue
		}
		if _, succeeded := double.successorOf(tenant, record.JudgmentVersion); succeeded {
			continue
		}
		found = append(found, record)
	}
	slices.SortFunc(found, func(left, right ports.ReceptionRecord) int {
		return strings.Compare(left.Key.SourceID, right.Key.SourceID)
	})
	return found, nil
}

// countingIdentity 记下身份核对读口被问了几次，答复照内层替身。
type countingIdentity struct {
	*identityViewDouble
	asked []string
}

func (double *countingIdentity) ResolveParcelIdentity(
	ctx context.Context,
	tenant domain.TenantID,
	observation ports.ExternalMarkObservation,
) (ports.ParcelIdentityAnswer, error) {
	double.asked = append(double.asked, observation.Mark)
	return double.identityViewDouble.ResolveParcelIdentity(ctx, tenant, observation)
}

type recheckFixture struct {
	*receptionFixture
	identityAsked *countingIdentity
	recheck       *application.RecheckPendingIdentityHandler
}

func newRecheckFixture(t *testing.T) *recheckFixture {
	t.Helper()
	reception := newReceptionFixture(t)
	reception.identity.candidates = nil
	counting := &countingIdentity{identityViewDouble: reception.identity}
	handler, err := application.NewRecheckPendingIdentityHandler(application.RecheckPendingIdentityDeps{
		Identity:       counting,
		ServiceResults: reception.serviceResults,
		Receptions:     reception.receptions,
		Pending:        reception.receptions,
		Versions:       reception.versions,
		Downstream:     reception.downstream,
		Clock:          fixedClock{at: deliveredAt.Add(time.Hour)},
	})
	if err != nil {
		t.Fatalf("构造重核编排：%v", err)
	}
	return &recheckFixture{receptionFixture: reception, identityAsked: counting, recheck: handler}
}

// receivePending 让一件实物经收寄编排落成零候选的待识别，交回那一条记录。
func (fixture *recheckFixture) receivePending(t *testing.T, sourceID, unit string) ports.ReceptionRecord {
	t.Helper()
	saved := fixture.identity.candidates
	fixture.identity.candidates = nil
	command := receiveCommand(t)
	command.SourceID = sourceID
	command.Unit = mustValue(t, domain.NewHandlingUnitID, unit)
	result, err := fixture.handler.Handle(context.Background(), command)
	fixture.identity.candidates = saved
	if err != nil || result.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("收寄 %s outcome = %v err = %v, want PENDING_IDENTIFICATION", sourceID, result.Outcome(), err)
	}
	record, _ := result.Record()
	if record.IdentityConflict {
		t.Fatalf("收寄 %s 落成了身份冲突，want 零候选的待识别", sourceID)
	}
	return record
}

func (fixture *recheckFixture) answer(t *testing.T, ambiguous bool, parcels ...string) {
	t.Helper()
	fixture.identity.candidates = nil
	for _, parcel := range parcels {
		fixture.identity.candidates = append(fixture.identity.candidates, mustValue(t, domain.NewParcelAssociationReference, parcel))
	}
	fixture.identity.ambiguous = ambiguous
}

func (fixture *recheckFixture) rows() int {
	return len(fixture.receptions.byKey)
}

func recheckNamed(t *testing.T, fixture *recheckFixture, version domain.IntakeResultVersion) application.RecheckPendingIdentityResult {
	t.Helper()
	result, err := fixture.recheck.Recheck(context.Background(), application.RecheckPendingIdentityCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Version:  version,
	})
	if err != nil {
		t.Fatalf("指名重核 %s：%v", version, err)
	}
	return result
}

func recheckByMark(t *testing.T, fixture *recheckFixture) []application.RecheckPendingIdentityResult {
	t.Helper()
	results, err := fixture.recheck.RecheckByMark(context.Background(), mustValue(t, domain.NewTenantID, "tenant-1"), "BARCODE-1")
	if err != nil {
		t.Fatalf("按号重核：%v", err)
	}
	return results
}

// Covers: `AT-NO-026`「身份后来被 `parcel-shipment` 确认——建立版本化关联，原待识别记录和既有事实保留；业务时间取原接收
// 时刻，不另起控制起点，发布意图一封」的编排面（ADR-0199 决定三）：零候选的待识别落册后，PS 形成了这个号的关联，按号重核
// 追加一版`节点收寄已形成`——带关联、接收时刻与控制照原判断、前版是原待识别、来源标识由原判断版本派生、意图一封；原待识别
// 原样留着。同一信封重投、同一件再人工重核都不再追加。
func TestAPendingUnitIsRecheckedIntoAnIntakeOnceItsMarkIsRelated(t *testing.T) {
	fixture := newRecheckFixture(t)
	pending := fixture.receivePending(t, "delivery-1", "unit-1")
	fixture.answer(t, false, "parcel-1")

	results := recheckByMark(t, fixture)
	if len(results) != 1 || results[0].Outcome() != application.RecheckIntakeFormed {
		t.Fatalf("按号重核 = %v, want 恰一件 INTAKE_FORMED", outcomesOf(results))
	}
	if results[0].Rechecked() != pending.JudgmentVersion {
		t.Fatalf("重核的是 %s, want 原待识别 %s", results[0].Rechecked(), pending.JudgmentVersion)
	}
	formed, present := results[0].Record()
	if !present {
		t.Fatal("追加的那一版没随结果交回")
	}
	association, associated := formed.Intake.Association()
	switch {
	case formed.Kind != ports.RecordIntakeFormed || !associated || association.String() != "parcel-1":
		t.Fatalf("追加版 = %+v, want 节点收寄已形成、关联 parcel-1", formed)
	case formed.Key.SourceID != "IDENTITY-RECHECK/"+pending.JudgmentVersion.String():
		t.Fatalf("追加版来源标识 = %q, want 由原判断版本派生", formed.Key.SourceID)
	case formed.Predecessor != pending.JudgmentVersion || formed.JudgmentVersion == pending.JudgmentVersion:
		t.Fatalf("追加版判断版本 %s 前版 %s, want 新版本、前版是原待识别 %s", formed.JudgmentVersion, formed.Predecessor, pending.JudgmentVersion)
	case formed.Intake.Version() != formed.JudgmentVersion:
		t.Fatalf("收寄结果版本 %s 不是判断版本 %s", formed.Intake.Version(), formed.JudgmentVersion)
	case !formed.Intake.ReceivedAt().Equal(deliveredAt) || formed.Intake.Unit() != pending.Intake.Unit() || formed.Intake.Node() != pending.Intake.Node():
		t.Fatalf("追加版接收 %s / %s @ %s, want 照原收寄 %s @ %s", formed.Intake.Unit(), formed.Intake.Node(), formed.Intake.ReceivedAt(), pending.Intake.Unit(), deliveredAt)
	case formed.Control != pending.Control:
		t.Fatalf("追加版控制 = %+v, want 照原判断的控制（不另起控制起点）%+v", formed.Control, pending.Control)
	case formed.Source != pending.Source || formed.ContentDigest != pending.ContentDigest:
		t.Fatal("追加版的来源文档或摘要不是原待识别那一份")
	case formed.ServiceResultCheck != ports.ServiceResultsChecked:
		t.Fatalf("追加版核对态 = %q, want CHECKED——照 ADR-0186 向 PS 取服务结果", formed.ServiceResultCheck)
	}
	if len(fixture.downstream.intents) != 1 || fixture.downstream.intents[0].Record.Key != formed.Key {
		t.Fatalf("意图 %d 封, want 恰一封、指回追加版", len(fixture.downstream.intents))
	}
	kept := fixture.receptions.byKey[pending.Key]
	if kept.Kind != ports.RecordPendingIdentification || kept.JudgmentVersion != pending.JudgmentVersion {
		t.Fatalf("原待识别被改了：%+v", kept)
	}
	rows := fixture.rows()

	if again := recheckByMark(t, fixture); len(again) != 0 {
		t.Fatalf("同一信封重投又重核了 %v——原待识别已有后继，不再是待重核的对象", outcomesOf(again))
	}
	manual := recheckNamed(t, fixture, pending.JudgmentVersion)
	if manual.Outcome() != application.RecheckExistingResult {
		t.Fatalf("同一件再人工重核 outcome = %v, want EXISTING_RESULT", manual.Outcome())
	}
	if current, _ := manual.Record(); current.JudgmentVersion != formed.JudgmentVersion {
		t.Fatalf("已有结果指的是 %s, want 当前有效版本 %s", current.JudgmentVersion, formed.JudgmentVersion)
	}
	if fixture.rows() != rows || len(fixture.downstream.intents) != 1 {
		t.Fatalf("重投与再重核之后 %d 行、%d 封意图, want 不再追加（%d 行、1 封）", fixture.rows(), len(fixture.downstream.intents), rows)
	}
}

// Covers: ADR-0199 决定三「追加版的来源标识……撞键即重放」——并发里另一方先追加了这一版（派生键已在册、前版还没连上链，
// 正是那一刻的样子）：落册答已有记录，读回占着派生键的那一行作答，不再交意图。
func TestARecheckWhoseDerivedKeyIsTakenAnswersTheWinner(t *testing.T) {
	fixture := newRecheckFixture(t)
	pending := fixture.receivePending(t, "delivery-1", "unit-1")
	fixture.answer(t, false, "parcel-1")
	winner := pending
	winner.Key.SourceID = "IDENTITY-RECHECK/" + pending.JudgmentVersion.String()
	winner.JudgmentVersion = mustValue(t, domain.NewIntakeResultVersion, "winner-version")
	fixture.receptions.byKey[winner.Key] = winner
	intents := len(fixture.downstream.intents)

	result := recheckNamed(t, fixture, pending.JudgmentVersion)
	if result.Outcome() != application.RecheckExistingResult {
		t.Fatalf("outcome = %v, want EXISTING_RESULT——派生键撞上即重放", result.Outcome())
	}
	if record, _ := result.Record(); record.JudgmentVersion != winner.JudgmentVersion {
		t.Fatalf("答的是 %s, want 占着派生键的那一版 %s", record.JudgmentVersion, winner.JudgmentVersion)
	}
	if len(fixture.downstream.intents) != intents {
		t.Fatal("重放又交了一封意图")
	}
}

// Covers: ADR-0199 决定三非唯一各格之「零候选 → 判断不变、不追加」：重核仍查无此号，与零候选带歧义信号（ADR-0195 落在零候选
// 那一格）都不追加、不交意图、不问服务结果。
func TestARecheckThatStillFindsNoCandidateAppendsNothing(t *testing.T) {
	for name, ambiguous := range map[string]bool{"查无此号": false, "零候选带歧义信号": true} {
		t.Run(name, func(t *testing.T) {
			fixture := newRecheckFixture(t)
			pending := fixture.receivePending(t, "delivery-1", "unit-1")
			fixture.answer(t, ambiguous)
			rows, asked := fixture.rows(), len(fixture.serviceResults.asked)

			results := recheckByMark(t, fixture)
			if len(results) != 1 || results[0].Outcome() != application.RecheckStillUnidentified {
				t.Fatalf("按号重核 = %v, want 恰一件 STILL_UNIDENTIFIED", outcomesOf(results))
			}
			if fixture.rows() != rows || len(fixture.downstream.intents) != 0 || len(fixture.serviceResults.asked) != asked {
				t.Fatal("零候选的重核追加了判断、交了意图或问了服务结果")
			}
			if current, _ := fixture.receptions.CurrentVersion(context.Background(), pending.Key.TenantID, pending.JudgmentVersion); current != pending.JudgmentVersion {
				t.Fatalf("当前有效版本变成了 %s", current)
			}
		})
	}
}

// Covers: ADR-0199 决定三非唯一各格之「多候选 → 追加一版待识别，记全部候选与身份冲突」，以及 ADR-0195 决定四的继承：恰一个
// 候选却带歧义信号落在多候选那一格，候选记那一件。追加版前版是被重核那一版、控制照原判断，不交意图。
func TestARecheckWithSeveralCandidatesAppendsAnIdentityConflict(t *testing.T) {
	cases := map[string]struct {
		ambiguous bool
		parcels   []string
	}{
		"两个候选":      {parcels: []string{"parcel-1", "parcel-2"}},
		"一个候选带歧义信号": {ambiguous: true, parcels: []string{"parcel-1"}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newRecheckFixture(t)
			pending := fixture.receivePending(t, "delivery-1", "unit-1")
			fixture.answer(t, testCase.ambiguous, testCase.parcels...)

			result := recheckNamed(t, fixture, pending.JudgmentVersion)
			if result.Outcome() != application.RecheckIdentityConflict {
				t.Fatalf("outcome = %v, want IDENTITY_CONFLICT", result.Outcome())
			}
			conflict, _ := result.Record()
			if conflict.Kind != ports.RecordPendingIdentification || !conflict.IdentityConflict ||
				!slices.Equal(candidatesOfRecord(conflict), testCase.parcels) {
				t.Fatalf("追加版 = %+v, want 待识别、记身份冲突、候选 %v", conflict, testCase.parcels)
			}
			if _, associated := conflict.Intake.Association(); associated {
				t.Fatal("冲突版带了关联")
			}
			if conflict.Predecessor != pending.JudgmentVersion || conflict.Control != pending.Control ||
				conflict.Key.SourceID != "IDENTITY-RECHECK/"+pending.JudgmentVersion.String() {
				t.Fatalf("冲突版前版 %s 键 %s, want 接续原待识别、照原控制、派生键", conflict.Predecessor, conflict.Key.SourceID)
			}
			if len(fixture.downstream.intents) != 0 || len(fixture.serviceResults.asked) != 0 {
				t.Fatal("冲突版交了意图或问了服务结果")
			}
		})
	}
}

// Covers: ADR-0199 决定三「唯一，但包裹已关联到别的作业实物 → 照 ADR-0196 追加冲突版，先到那件也追加」：号码后来只标识
// 一件包裹，而那件包裹已经关联到另一件先到的实物。被重核那件追加冲突版（候选是那件包裹），先到那件也追加一版冲突，各自
// 接续自己的链；不交新的意图。
func TestARecheckOfAMarkAlreadyOnAnotherUnitAppendsConflictsOnBothSides(t *testing.T) {
	fixture := newRecheckFixture(t)
	pending := fixture.receivePending(t, "delivery-late", "unit-late")
	fixture.answer(t, false, "parcel-1")
	early := receiveCommand(t)
	early.SourceID, early.Unit = "delivery-early", mustValue(t, domain.NewHandlingUnitID, "unit-early")
	formed, err := fixture.handler.Handle(context.Background(), early)
	if err != nil || formed.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("先到那件 outcome = %v err = %v", formed.Outcome(), err)
	}
	earlyRecord, _ := formed.Record()
	intents := len(fixture.downstream.intents)

	result := recheckNamed(t, fixture, pending.JudgmentVersion)
	if result.Outcome() != application.RecheckIdentityConflict {
		t.Fatalf("outcome = %v, want IDENTITY_CONFLICT——号码唯一指向的包裹已关联到别的实物", result.Outcome())
	}
	late, _ := result.Record()
	if !late.IdentityConflict || late.Predecessor != pending.JudgmentVersion || !slices.Equal(candidatesOfRecord(late), []string{"parcel-1"}) {
		t.Fatalf("被重核那件的追加版 = %+v", late)
	}
	earlyCurrent, err := fixture.receptions.CurrentVersion(context.Background(), earlyRecord.Key.TenantID, earlyRecord.JudgmentVersion)
	if err != nil || earlyCurrent == earlyRecord.JudgmentVersion {
		t.Fatalf("先到那件的链没有接续（当前 %s，err %v）", earlyCurrent, err)
	}
	earlyConflict, found, _ := fixture.receptions.FindByJudgmentVersion(context.Background(), earlyRecord.Key.TenantID, earlyCurrent)
	if !found || !earlyConflict.IdentityConflict || earlyConflict.Intake.Unit().String() != "unit-early" || earlyConflict.Predecessor != earlyRecord.JudgmentVersion {
		t.Fatalf("先到那件的追加版 = %+v, want 记身份冲突、接续原判断", earlyConflict)
	}
	if len(fixture.downstream.intents) != intents {
		t.Fatal("冲突两边交了新的意图")
	}
}

// Covers: ADR-0199 决定三「身份核对仍读不动 → 不追加，交回 error」：信封留在 inbox 重投、人工发起答 5xx。错误带
// ErrParcelIdentityUnreadable，装配据此把它记成未决而不是发布失败。
func TestARecheckThatCannotReadTheIdentityAppendsNothingAndFails(t *testing.T) {
	fixture := newRecheckFixture(t)
	pending := fixture.receivePending(t, "delivery-1", "unit-1")
	fixture.identity.err = errors.New("parcel shipment unreachable")
	rows := fixture.rows()

	_, err := fixture.recheck.Recheck(context.Background(), application.RecheckPendingIdentityCommand{
		TenantID: pending.Key.TenantID, Version: pending.JudgmentVersion,
	})
	if !errors.Is(err, application.ErrParcelIdentityUnreadable) {
		t.Fatalf("指名重核 err = %v, want ErrParcelIdentityUnreadable", err)
	}
	if _, err := fixture.recheck.RecheckByMark(context.Background(), pending.Key.TenantID, "BARCODE-1"); !errors.Is(err, application.ErrParcelIdentityUnreadable) {
		t.Fatalf("按号重核 err = %v, want ErrParcelIdentityUnreadable", err)
	}
	if fixture.rows() != rows || len(fixture.downstream.intents) != 0 {
		t.Fatal("读不动的重核追加了判断或交了意图")
	}
}

// Covers: ADR-0199 决定三的重核对象——记身份冲突的待识别（多候选、一码两物）不重核，来源文档缺席的旧行不重核，指名的不是
// 待识别也不重核：三格各答各的业务结果，不问身份核对读口、不追加。按号找也找不到前两种。指名本租户册上没有的判断版本，
// 答 ErrRecheckSubjectNotFound。
func TestConflictedSourcelessAndFormedJudgmentsAreNotRechecked(t *testing.T) {
	fixture := newRecheckFixture(t)
	fixture.answer(t, false, "parcel-1", "parcel-2")
	conflicted := receiveCommand(t)
	conflicted.SourceID, conflicted.Unit = "delivery-conflict", mustValue(t, domain.NewHandlingUnitID, "unit-conflict")
	answered, err := fixture.handler.Handle(context.Background(), conflicted)
	if err != nil || answered.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("多候选收寄 outcome = %v err = %v", answered.Outcome(), err)
	}
	conflictRecord, _ := answered.Record()

	sourceless := fixture.receivePending(t, "delivery-old", "unit-old")
	sourceless.Source = domain.DeliverySource{}
	fixture.receptions.byKey[sourceless.Key] = sourceless

	fixture.answer(t, false, "parcel-9")
	identified := receiveCommand(t)
	identified.SourceID, identified.Unit = "delivery-formed", mustValue(t, domain.NewHandlingUnitID, "unit-formed")
	formed, err := fixture.handler.Handle(context.Background(), identified)
	if err != nil || formed.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("唯一收寄 outcome = %v err = %v", formed.Outcome(), err)
	}
	formedRecord, _ := formed.Record()

	fixture.answer(t, false, "parcel-1")
	fixture.identityAsked.asked = nil
	rows, intents := fixture.rows(), len(fixture.downstream.intents)
	cases := map[string]struct {
		version domain.IntakeResultVersion
		want    application.RecheckOutcome
	}{
		"记身份冲突的待识别": {conflictRecord.JudgmentVersion, application.RecheckIdentityConflictRecorded},
		"来源未保全的旧行":  {sourceless.JudgmentVersion, application.RecheckSourceNotPreserved},
		"节点收寄已形成":   {formedRecord.JudgmentVersion, application.RecheckNotPendingIdentification},
	}
	for name, testCase := range cases {
		if got := recheckNamed(t, fixture, testCase.version).Outcome(); got != testCase.want {
			t.Fatalf("%s：outcome = %v, want %v", name, got, testCase.want)
		}
	}
	if results := recheckByMark(t, fixture); len(results) != 0 {
		t.Fatalf("按号找到了不该重核的行：%v", outcomesOf(results))
	}
	if len(fixture.identityAsked.asked) != 0 || fixture.rows() != rows || len(fixture.downstream.intents) != intents {
		t.Fatalf("不重核的几格问了身份 %d 次、册上 %d 行（原 %d）", len(fixture.identityAsked.asked), fixture.rows(), rows)
	}

	_, err = fixture.recheck.Recheck(context.Background(), application.RecheckPendingIdentityCommand{
		TenantID: mustValue(t, domain.NewTenantID, "tenant-1"),
		Version:  mustValue(t, domain.NewIntakeResultVersion, "no-such-version"),
	})
	if !errors.Is(err, application.ErrRecheckSubjectNotFound) {
		t.Fatalf("指名册上没有的版本 err = %v, want ErrRecheckSubjectNotFound", err)
	}
}

// Covers: ADR-0199 决定三「照 ADR-0186 向 PS 取服务结果并记核对态」——服务结果读不动时照常追加`节点收寄已形成`，核对态记
// 读不动、标记留空，意图照交。
func TestARecheckRecordsAnUnreadableServiceResultLikeTheReception(t *testing.T) {
	fixture := newRecheckFixture(t)
	pending := fixture.receivePending(t, "delivery-1", "unit-1")
	fixture.answer(t, false, "parcel-1")
	fixture.serviceResults.err = errors.New("parcel shipment unreachable")

	result := recheckNamed(t, fixture, pending.JudgmentVersion)
	formed, _ := result.Record()
	if result.Outcome() != application.RecheckIntakeFormed || formed.ServiceResultCheck != ports.ServiceResultsUnreadable || len(formed.ServiceMarkers) != 0 {
		t.Fatalf("outcome %v 核对态 %q 标记 %v, want INTAKE_FORMED、UNREADABLE、无标记", result.Outcome(), formed.ServiceResultCheck, formed.ServiceMarkers)
	}
	if len(fixture.downstream.intents) != 1 {
		t.Fatalf("意图 %d 封, want 1", len(fixture.downstream.intents))
	}
}

// Covers: 重核没形成答案的几格——意图交不出去、版本签不出、落册失败，都交回 error，由调用方整笔回滚：设备那头没有续办引用可读，
// 留着一版没交出意图的关联收寄，PS 就永远等不到它。
func TestRecheckFailuresFormNoAnswer(t *testing.T) {
	failure := errors.New("dependency down")
	cases := map[string]func(*recheckFixture){
		"意图交不出去": func(fixture *recheckFixture) { fixture.downstream.err = failure },
		"版本签不出":  func(fixture *recheckFixture) { fixture.versions.err = failure },
		"落册失败":   func(fixture *recheckFixture) { fixture.receptions.saveErr = failure },
	}
	for name, arrange := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newRecheckFixture(t)
			pending := fixture.receivePending(t, "delivery-1", "unit-1")
			fixture.answer(t, false, "parcel-1")
			arrange(fixture)
			if _, err := fixture.recheck.Recheck(context.Background(), application.RecheckPendingIdentityCommand{
				TenantID: pending.Key.TenantID, Version: pending.JudgmentVersion,
			}); err == nil {
				t.Fatal("没形成答案却没交回 error")
			}
		})
	}
}

// Covers: 重核编排在构造期拒缺依赖的装配，同收寄编排。
func TestARecheckWithAMissingDependencyIsRefusedAtConstruction(t *testing.T) {
	cases := map[string]func(*application.RecheckPendingIdentityDeps){
		"身份核对读口": func(deps *application.RecheckPendingIdentityDeps) { deps.Identity = nil },
		"服务结果读口": func(deps *application.RecheckPendingIdentityDeps) { deps.ServiceResults = nil },
		"收寄库":    func(deps *application.RecheckPendingIdentityDeps) { deps.Receptions = nil },
		"待识别查找":  func(deps *application.RecheckPendingIdentityDeps) { deps.Pending = nil },
		"版本签发":   func(deps *application.RecheckPendingIdentityDeps) { deps.Versions = nil },
		"意图交付":   func(deps *application.RecheckPendingIdentityDeps) { deps.Downstream = nil },
		"时钟":     func(deps *application.RecheckPendingIdentityDeps) { deps.Clock = nil },
	}
	for name, drop := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			deps := application.RecheckPendingIdentityDeps{
				Identity: fixture.identity, ServiceResults: fixture.serviceResults, Receptions: fixture.receptions,
				Pending: fixture.receptions, Versions: fixture.versions, Downstream: fixture.downstream,
				Clock: fixedClock{at: deliveredAt},
			}
			drop(&deps)
			if handler, err := application.NewRecheckPendingIdentityHandler(deps); err == nil || handler != nil {
				t.Fatalf("缺%s却构造成功", name)
			}
		})
	}
}

func outcomesOf(results []application.RecheckPendingIdentityResult) []string {
	names := make([]string, 0, len(results))
	for _, result := range results {
		names = append(names, result.Outcome().String())
	}
	return names
}

func candidatesOfRecord(record ports.ReceptionRecord) []string {
	names := make([]string, 0, len(record.Candidates))
	for _, candidate := range record.Candidates {
		names = append(names, candidate.String())
	}
	return names
}
