package application_test

import (
	"context"
	"slices"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证身份冲突的来由，与只因歧义信号落下的冲突经重核解开（ADR-0211，票 no-reception-identity/07）。收寄、重核与处置三只编排
// 各用既有的替身夹具；夹具全为合成引用（S 级）。

// Covers: ADR-0211 决定一——收寄落身份冲突时记封闭的冲突来由：恰一个候选带歧义信号记只因歧义信号；多候选不论带不带信号都记
// 多候选（ADR-0195「多候选，有无信号都一样」）；号码唯一指向的包裹已关联到先到那件时，后到那件与给先到那件追加的冲突版都记
// 一码两物。不记冲突的各格（零候选带信号、唯一命中）不带来由。
func TestAReceptionConflictRecordsItsCause(t *testing.T) {
	parcel1 := mustValue(t, domain.NewParcelAssociationReference, "parcel-1")
	parcel2 := mustValue(t, domain.NewParcelAssociationReference, "parcel-2")
	cases := map[string]struct {
		ambiguous  bool
		candidates []domain.ParcelAssociationReference
		want       ports.IdentityConflictCause
	}{
		"唯一候选带歧义信号": {true, []domain.ParcelAssociationReference{parcel1}, ports.ConflictByAmbiguityOnly},
		"两个候选":      {false, []domain.ParcelAssociationReference{parcel1, parcel2}, ports.ConflictByMultipleCandidates},
		"两个候选带歧义信号": {true, []domain.ParcelAssociationReference{parcel1, parcel2}, ports.ConflictByMultipleCandidates},
		"零候选带歧义信号":  {true, nil, ports.IdentityConflictCauseAbsent},
		"唯一命中":      {false, []domain.ParcelAssociationReference{parcel1}, ports.IdentityConflictCauseAbsent},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.identity.candidates, fixture.identity.ambiguous = testCase.candidates, testCase.ambiguous
			result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			record, _ := result.Record()
			if record.IdentityConflictCause != testCase.want {
				t.Fatalf("冲突来由 = %q, want %q", record.IdentityConflictCause, testCase.want)
			}
			if record.IdentityConflict != (testCase.want != ports.IdentityConflictCauseAbsent) {
				t.Fatalf("记身份冲突 = %v, 与来由 %q 对不上", record.IdentityConflict, record.IdentityConflictCause)
			}
		})
	}

	t.Run("一码两物", func(t *testing.T) {
		fixture, _ := oneMarkFixture(t)
		if _, err := fixture.handler.Handle(context.Background(), oneMarkCommand(t, "SYN-DELIVERY-NRI07-A", "SYN-UNIT-NRI07-A")); err != nil {
			t.Fatalf("先到那件：%v", err)
		}
		result, err := fixture.handler.Handle(context.Background(), oneMarkCommand(t, "SYN-DELIVERY-NRI07-B", "SYN-UNIT-NRI07-B"))
		if err != nil {
			t.Fatalf("后到那件：%v", err)
		}
		late, _ := result.Record()
		if !late.IdentityConflict || late.IdentityConflictCause != ports.ConflictByOneMarkOnTwoUnits {
			t.Fatalf("后到那件冲突 %v 来由 %q, want 一码两物", late.IdentityConflict, late.IdentityConflictCause)
		}
		var appended []ports.ReceptionRecord
		for _, row := range fixture.receptions.recordsOfUnit("SYN-UNIT-NRI07-A") {
			if row.IdentityConflict {
				appended = append(appended, row)
			}
		}
		if len(appended) != 1 || appended[0].IdentityConflictCause != ports.ConflictByOneMarkOnTwoUnits {
			t.Fatalf("先到那件追加的冲突版 = %+v, want 一版、来由一码两物", appended)
		}
	})
}

// receiveContested 让一件实物经收寄编排落成只因歧义信号的身份冲突：号码在册里只标识 parcel，同时另有一份`已提交`委托也报了它。
func (fixture *recheckFixture) receiveContested(t *testing.T, sourceID, unit, parcel string) ports.ReceptionRecord {
	t.Helper()
	saved, savedAmbiguous := fixture.identity.candidates, fixture.identity.ambiguous
	fixture.answer(t, true, parcel)
	command := receiveCommand(t)
	command.SourceID = sourceID
	command.Unit = mustValue(t, domain.NewHandlingUnitID, unit)
	result, err := fixture.handler.Handle(context.Background(), command)
	fixture.identity.candidates, fixture.identity.ambiguous = saved, savedAmbiguous
	if err != nil || result.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("收寄 %s outcome = %v err = %v, want PENDING_IDENTIFICATION", sourceID, result.Outcome(), err)
	}
	record, _ := result.Record()
	if !record.IdentityConflict || record.IdentityConflictCause != ports.ConflictByAmbiguityOnly {
		t.Fatalf("收寄 %s 冲突 %v 来由 %q, want 只因歧义信号的身份冲突", sourceID, record.IdentityConflict, record.IdentityConflictCause)
	}
	return record
}

// Covers: `AT-NO-062` 的编排面（ADR-0211 决定二）：只因歧义信号落下的身份冲突，声明它的委托被拒绝或撤回之后，按号重核答唯一命中、
// 不带信号——追加一版`节点收寄已形成`：关联那件包裹，前版是冲突那一版，业务时间与控制照原收寄（不另起控制起点），来源文档与摘要
// 沿用，意图一封；这一版不记冲突、不带来由，冲突随链头解开，身份一格判成立。冲突那一版原样留着。同一封门铃重投、同一件再人工
// 重核都不再追加。
func TestAnAmbiguityOnlyConflictIsReleasedOnceTheDeclarationIsGone(t *testing.T) {
	fixture := newRecheckFixture(t)
	contested := fixture.receiveContested(t, "delivery-1", "unit-1", "parcel-1")
	fixture.answer(t, false, "parcel-1")

	results := recheckByMark(t, fixture)
	if len(results) != 1 || results[0].Outcome() != application.RecheckIntakeFormed || results[0].Rechecked() != contested.JudgmentVersion {
		t.Fatalf("按号重核 = %v, want 恰一件、重核冲突那一版、INTAKE_FORMED", outcomesOf(results))
	}
	formed, _ := results[0].Record()
	association, associated := formed.Intake.Association()
	switch {
	case formed.Kind != ports.RecordIntakeFormed || !associated || association.String() != "parcel-1":
		t.Fatalf("追加版 = %+v, want 节点收寄已形成、关联 parcel-1", formed)
	case formed.IdentityConflict || formed.IdentityConflictCause != ports.IdentityConflictCauseAbsent || len(formed.Candidates) != 1:
		t.Fatalf("追加版冲突 %v 来由 %q 候选 %v, want 不记冲突、不带来由、候选照答复", formed.IdentityConflict, formed.IdentityConflictCause, formed.Candidates)
	case formed.Predecessor != contested.JudgmentVersion || formed.Key.SourceID != "IDENTITY-RECHECK/"+contested.JudgmentVersion.String():
		t.Fatalf("追加版前版 %s 键 %s, want 接续冲突那一版、派生键", formed.Predecessor, formed.Key.SourceID)
	case !formed.Intake.ReceivedAt().Equal(deliveredAt) || formed.Control != contested.Control:
		t.Fatalf("追加版接收 %s 控制 %+v, want 照原收寄（不另起控制起点）", formed.Intake.ReceivedAt(), formed.Control)
	case formed.Source != contested.Source || formed.ContentDigest != contested.ContentDigest:
		t.Fatal("追加版的来源文档或摘要不是冲突那一版的")
	}
	if len(fixture.downstream.intents) != 1 || fixture.downstream.intents[0].Record.Key != formed.Key {
		t.Fatalf("意图 %d 封, want 恰一封、指回追加版", len(fixture.downstream.intents))
	}
	if reasons := identityReasonsOf(t, fixture.receptions.recordsOfUnit("unit-1")); len(reasons) != 0 {
		t.Fatalf("身份一格缺少 %v, want 成立——冲突随链头解开", reasons)
	}
	if kept := fixture.receptions.byKey[contested.Key]; !kept.IdentityConflict || kept.IdentityConflictCause != ports.ConflictByAmbiguityOnly {
		t.Fatalf("冲突那一版被改了：%+v", kept)
	}
	rows := fixture.rows()

	if again := recheckByMark(t, fixture); len(again) != 0 {
		t.Fatalf("门铃重投又重核了 %v——冲突那一版已有后继", outcomesOf(again))
	}
	if manual := recheckNamed(t, fixture, contested.JudgmentVersion); manual.Outcome() != application.RecheckExistingResult {
		t.Fatalf("再人工重核 outcome = %v, want EXISTING_RESULT", manual.Outcome())
	}
	if fixture.rows() != rows || len(fixture.downstream.intents) != 1 {
		t.Fatalf("重投与再重核之后 %d 行、%d 封意图, want 不再追加", fixture.rows(), len(fixture.downstream.intents))
	}
}

// Covers: `AT-NO-062` 反向（ADR-0211 决定二「信号仍在 → 不追加」）：声明它的那一份没了，号码却还被另一份`已提交`委托报着——
// 重核仍答恰一个候选带信号。指名重核与按号重核都答仍带歧义信号，不追加、不交意图、不问服务结果，冲突那一版仍是当前有效版本。
func TestAnAmbiguityOnlyConflictStaysWhileADeclarationRemains(t *testing.T) {
	fixture := newRecheckFixture(t)
	contested := fixture.receiveContested(t, "delivery-1", "unit-1", "parcel-1")
	fixture.answer(t, true, "parcel-1")
	rows, asked := fixture.rows(), len(fixture.serviceResults.asked)

	named := recheckNamed(t, fixture, contested.JudgmentVersion)
	if named.Outcome() != application.RecheckStillAmbiguous {
		t.Fatalf("指名重核 outcome = %v, want STILL_AMBIGUOUS", named.Outcome())
	}
	if record, _ := named.Record(); record.JudgmentVersion != contested.JudgmentVersion {
		t.Fatalf("答的是 %s, want 被重核那一版自己 %s", record.JudgmentVersion, contested.JudgmentVersion)
	}
	byMark := recheckByMark(t, fixture)
	if len(byMark) != 1 || byMark[0].Outcome() != application.RecheckStillAmbiguous {
		t.Fatalf("按号重核 = %v, want 恰一件 STILL_AMBIGUOUS", outcomesOf(byMark))
	}
	if fixture.rows() != rows || len(fixture.downstream.intents) != 0 || len(fixture.serviceResults.asked) != asked {
		t.Fatal("信号仍在却追加了判断、交了意图或问了服务结果")
	}
	if current, _ := fixture.receptions.CurrentVersion(context.Background(), contested.Key.TenantID, contested.JudgmentVersion); current != contested.JudgmentVersion {
		t.Fatalf("当前有效版本变成了 %s", current)
	}
}

// Covers: ADR-0211 决定二其余各格：重核答多候选，追加冲突版、来由改记多候选；答唯一而那件包裹已关联到先到的另一件实物，照
// ADR-0196 两边各追加冲突版、来由记一码两物；答零候选，判断不变、不追加（ADR-0199 决定三零候选那一格），冲突那一版仍是链头。
func TestAnAmbiguityOnlyConflictFollowsTheNewAnswer(t *testing.T) {
	t.Run("多候选", func(t *testing.T) {
		fixture := newRecheckFixture(t)
		contested := fixture.receiveContested(t, "delivery-1", "unit-1", "parcel-1")
		fixture.answer(t, false, "parcel-1", "parcel-2")
		result := recheckNamed(t, fixture, contested.JudgmentVersion)
		appended, _ := result.Record()
		if result.Outcome() != application.RecheckIdentityConflict || !appended.IdentityConflict ||
			appended.IdentityConflictCause != ports.ConflictByMultipleCandidates ||
			!slices.Equal(candidatesOfRecord(appended), []string{"parcel-1", "parcel-2"}) || appended.Predecessor != contested.JudgmentVersion {
			t.Fatalf("outcome %v 追加版 %+v, want IDENTITY_CONFLICT、来由多候选、两个候选、接续冲突那一版", result.Outcome(), appended)
		}
		if len(fixture.downstream.intents) != 0 {
			t.Fatal("冲突版交了意图")
		}
	})

	t.Run("一码两物", func(t *testing.T) {
		fixture := newRecheckFixture(t)
		fixture.answer(t, false, "parcel-1")
		early := receiveCommand(t)
		early.SourceID, early.Unit = "delivery-early", mustValue(t, domain.NewHandlingUnitID, "unit-early")
		formed, err := fixture.handler.Handle(context.Background(), early)
		if err != nil || formed.Outcome() != application.NodeIntakeFormed {
			t.Fatalf("先到那件 outcome = %v err = %v", formed.Outcome(), err)
		}
		earlyRecord, _ := formed.Record()
		contested := fixture.receiveContested(t, "delivery-late", "unit-late", "parcel-1")
		if rows := fixture.receptions.recordsOfUnit("unit-early"); len(rows) != 1 {
			t.Fatalf("带信号的唯一命中给先到那件追加了 %d 行——它不走一码两物那一支", len(rows)-1)
		}

		fixture.answer(t, false, "parcel-1")
		result := recheckNamed(t, fixture, contested.JudgmentVersion)
		late, _ := result.Record()
		if result.Outcome() != application.RecheckIdentityConflict || late.IdentityConflictCause != ports.ConflictByOneMarkOnTwoUnits {
			t.Fatalf("outcome %v 来由 %q, want IDENTITY_CONFLICT、一码两物", result.Outcome(), late.IdentityConflictCause)
		}
		current, err := fixture.receptions.CurrentVersion(context.Background(), earlyRecord.Key.TenantID, earlyRecord.JudgmentVersion)
		if err != nil {
			t.Fatalf("先到那件的链头：%v", err)
		}
		earlyConflict, found, _ := fixture.receptions.FindByJudgmentVersion(context.Background(), earlyRecord.Key.TenantID, current)
		if !found || !earlyConflict.IdentityConflict || earlyConflict.IdentityConflictCause != ports.ConflictByOneMarkOnTwoUnits {
			t.Fatalf("先到那件追加的冲突版 = %+v, want 来由一码两物", earlyConflict)
		}
	})

	t.Run("零候选", func(t *testing.T) {
		fixture := newRecheckFixture(t)
		contested := fixture.receiveContested(t, "delivery-1", "unit-1", "parcel-1")
		fixture.answer(t, false)
		rows := fixture.rows()
		if result := recheckNamed(t, fixture, contested.JudgmentVersion); result.Outcome() != application.RecheckStillUnidentified {
			t.Fatalf("outcome = %v, want STILL_UNIDENTIFIED", result.Outcome())
		}
		if fixture.rows() != rows {
			t.Fatal("零候选的重核追加了判断")
		}
	})
}

// Covers: ADR-0211 决定一、二——重核对象只放宽到来由是只因歧义信号的冲突：多候选、一码两物（后到那件与先到那件的冲突版）与
// 来由缺席的旧冲突行（本票迁移之前落下、不回填）照旧答记着身份冲突、不问身份核对读口、不追加，按号也找不到它们，只经处置解开。
func TestConflictsOfOtherCausesStillWaitForADisposition(t *testing.T) {
	fixture := newRecheckFixture(t)
	fixture.answer(t, false, "parcel-1", "parcel-2")
	many := receiveCommand(t)
	many.SourceID, many.Unit = "delivery-many", mustValue(t, domain.NewHandlingUnitID, "unit-many")
	manyResult, err := fixture.handler.Handle(context.Background(), many)
	if err != nil {
		t.Fatalf("多候选收寄：%v", err)
	}
	manyRecord, _ := manyResult.Record()

	fixture.answer(t, false, "parcel-9")
	early := receiveCommand(t)
	early.SourceID, early.Unit = "delivery-early", mustValue(t, domain.NewHandlingUnitID, "unit-early")
	if _, err := fixture.handler.Handle(context.Background(), early); err != nil {
		t.Fatalf("先到那件：%v", err)
	}
	late := receiveCommand(t)
	late.SourceID, late.Unit = "delivery-late", mustValue(t, domain.NewHandlingUnitID, "unit-late")
	lateResult, err := fixture.handler.Handle(context.Background(), late)
	if err != nil {
		t.Fatalf("后到那件：%v", err)
	}
	lateRecord, _ := lateResult.Record()
	var earlyConflict ports.ReceptionRecord
	for _, row := range fixture.receptions.recordsOfUnit("unit-early") {
		if row.IdentityConflict {
			earlyConflict = row
		}
	}

	legacy := fixture.receiveContested(t, "delivery-legacy", "unit-legacy", "parcel-1")
	legacy.IdentityConflictCause = ports.IdentityConflictCauseAbsent
	fixture.receptions.byKey[legacy.Key] = legacy

	fixture.answer(t, false, "parcel-1")
	fixture.identityAsked.asked = nil
	rows, intents := fixture.rows(), len(fixture.downstream.intents)
	for name, record := range map[string]ports.ReceptionRecord{
		"多候选":       manyRecord,
		"一码两物后到那件":  lateRecord,
		"一码两物先到那件":  earlyConflict,
		"来由缺席的旧冲突行": legacy,
	} {
		if !record.IdentityConflict {
			t.Fatalf("%s 没落成身份冲突", name)
		}
		if got := recheckNamed(t, fixture, record.JudgmentVersion).Outcome(); got != application.RecheckIdentityConflictRecorded {
			t.Fatalf("%s：outcome = %v, want IDENTITY_CONFLICT_RECORDED", name, got)
		}
	}
	if results := recheckByMark(t, fixture); len(results) != 0 {
		t.Fatalf("按号找到了不该重核的冲突行：%v", outcomesOf(results))
	}
	if len(fixture.identityAsked.asked) != 0 || fixture.rows() != rows || len(fixture.downstream.intents) != intents {
		t.Fatalf("不重核的冲突行问了身份 %d 次、册上 %d 行（原 %d）", len(fixture.identityAsked.asked), fixture.rows(), rows)
	}
}

// Covers: ADR-0211 决定一「处置版不记冲突，也就不带来由」与决定四「已经人工处置过的链，链头不再记冲突，不在重核对象里；自动解开
// 不撤销任何处置」：只因歧义信号的冲突被人指定为那件包裹之后，处置版带关联、不带来由；信号随后消失、门铃按号重核，那一版不是
// 重核对象，册上不多一行。
func TestADispositionVersionCarriesNoConflictCause(t *testing.T) {
	fixture := newDispositionFixture(t)
	contested := fixture.arrive(t, "SYN-DELIVERY-NRI07-AMB", "SYN-UNIT-NRI07-AMB", true, fixture.parcel)
	if !contested.IdentityConflict || contested.IdentityConflictCause != ports.ConflictByAmbiguityOnly {
		t.Fatalf("冲突 %v 来由 %q, want 只因歧义信号", contested.IdentityConflict, contested.IdentityConflictCause)
	}
	if got := fixture.disposeOK(t, fixture.command(t, "SYN-UNIT-NRI07-AMB", contested, domain.DispositionDesignatesParcel)); got.Outcome() != application.IdentityConflictDisposed {
		t.Fatalf("处置 outcome = %v", got.Outcome())
	}
	designated := fixture.versionOn(t, contested)
	if designated.Kind != ports.RecordIntakeFormed || designated.IdentityConflict || designated.IdentityConflictCause != ports.IdentityConflictCauseAbsent {
		t.Fatalf("处置版 = %+v, want 节点收寄已形成、不记冲突、不带来由", designated)
	}

	fixture.identity.candidates, fixture.identity.ambiguous = []domain.ParcelAssociationReference{fixture.parcel}, false
	rows := len(fixture.store.byKey)
	results, err := fixture.recheck.RecheckByMark(context.Background(), contested.Key.TenantID, "SYN-BARCODE-NRI01")
	if err != nil || len(results) != 0 || len(fixture.store.byKey) != rows {
		t.Fatalf("处置之后按号重核 = %v err = %v、册上多了 %d 行, want 什么都不做", outcomesOf(results), err, len(fixture.store.byKey)-rows)
	}
}
