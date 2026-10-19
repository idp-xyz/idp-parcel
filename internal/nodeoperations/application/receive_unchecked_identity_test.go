package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证身份核对读不动翻成`待识别实物已建立`加身份核对态「未核对」（ADR-0199 决定五，票 no-reception-source/05）：收寄与
// 控制照常、来源保全、候选为空、不问服务结果、不交意图，同一笔自投一封重核门铃；以及重核接上未核对的那几格。

// Covers: `AT-NO-051`「身份核对读不动——建立待识别实物并记未核对，接收、控制与来源照常，不交发布意图，与查无此号分得开」的
// 编排面（ADR-0199 决定五）：读口交回错误时落一条待识别，核对态未核对、候选为空、不记身份冲突、不带关联；控制以节点收寄
// 为成立依据；来源文档照存；不问服务结果、不交意图；同一笔自投一封门铃，只指名这一版。
func TestAnUnreadableIdentityCheckFormsAPendingUnitMarkedUnchecked(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.identity.err = errors.New("parcel shipment unreachable")
	command := receiveCommand(t)

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v——身份核对读不动不再不形成答案", err)
	}
	if result.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("outcome = %q, want PENDING_IDENTIFICATION", result.Outcome())
	}
	record, recorded := result.Record()
	_, associated := record.Intake.Association()
	switch {
	case !recorded || record.Kind != ports.RecordPendingIdentification:
		t.Fatalf("记录 = %+v, want 一条待识别", record)
	case record.IdentityCheck != ports.IdentityUnchecked:
		t.Fatalf("身份核对态 = %q, want UNCHECKED", record.IdentityCheck)
	case len(record.Candidates) != 0 || record.IdentityConflict || associated:
		t.Fatalf("候选 %v 冲突 %v 关联 %v, want 都没有——没问到不拿空候选顶替，也不猜", record.Candidates, record.IdentityConflict, associated)
	case !record.Control.Active() || record.Control.EstablishmentKind() != domain.EstablishedByNodeIntake:
		t.Fatalf("控制 = %+v, want 以节点收寄为成立依据——明确接收已经取得控制", record.Control)
	case !record.Intake.ReceivedAt().Equal(deliveredAt) || record.JudgmentVersion != record.Intake.Version():
		t.Fatalf("收寄 %s 版本 %s / %s", record.Intake.ReceivedAt(), record.JudgmentVersion, record.Intake.Version())
	case !record.Source.Preserved() || record.Source.Mark != command.Mark.Mark:
		t.Fatalf("来源文档 = %+v, want 照存、号码原样", record.Source)
	case record.ServiceResultCheck != ports.ServiceResultCheckAbsent || len(record.ServiceMarkers) != 0:
		t.Fatalf("服务结果核对态 %q 标记 %v, want 不带", record.ServiceResultCheck, record.ServiceMarkers)
	}
	if len(fixture.serviceResults.asked) != 0 || len(fixture.downstream.intents) != 0 {
		t.Fatalf("问服务结果 %v、意图 %d 封, want 都没有", fixture.serviceResults.asked, len(fixture.downstream.intents))
	}
	if fixture.receptions.saved != 1 {
		t.Fatalf("落册 %d 行, want 1", fixture.receptions.saved)
	}
	if len(fixture.doorbell.rung) != 1 || fixture.doorbell.rung[0].Record.JudgmentVersion != record.JudgmentVersion ||
		fixture.doorbell.rung[0].Record.Key != record.Key {
		t.Fatalf("门铃 %+v, want 恰一封、指名这一版 %s", fixture.doorbell.rung, record.JudgmentVersion)
	}
}

// Covers: ADR-0199 决定五「身份核对态只在待识别记录上，封闭两值」——读口答复了的待识别（零候选、多候选）记已核对，不响门铃；
// 关联收寄、拒收与仅扫描不带身份核对态。少了这一格，零候选与未核对两条待识别逐字相同。
func TestAnAnsweredIdentityCheckMarksThePendingUnitChecked(t *testing.T) {
	cases := map[string]struct {
		candidates []string
		claim      application.ReceptionClaimKind
		want       ports.IdentityCheck
	}{
		"零候选":  {claim: application.ExplicitReception, want: ports.IdentityChecked},
		"多候选":  {candidates: []string{"parcel-1", "parcel-2"}, claim: application.ExplicitReception, want: ports.IdentityChecked},
		"唯一命中": {candidates: []string{"parcel-1"}, claim: application.ExplicitReception, want: ports.IdentityCheckAbsent},
		"明确拒收": {candidates: []string{"parcel-1"}, claim: application.ExplicitRefusal, want: ports.IdentityCheckAbsent},
		"仅扫描":  {candidates: []string{"parcel-1"}, claim: application.ScanOnlyObservation, want: ports.IdentityCheckAbsent},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			fixture := newReceptionFixture(t)
			fixture.identity.candidates = nil
			for _, raw := range testCase.candidates {
				fixture.identity.candidates = append(fixture.identity.candidates, mustValue(t, domain.NewParcelAssociationReference, raw))
			}
			command := receiveCommand(t)
			command.Claim = testCase.claim
			if testCase.claim != application.ExplicitReception {
				command.Evidence = domain.ReceptionEvidenceReference{}
			}
			if testCase.claim == application.ExplicitRefusal {
				command.Refusal = "破损拒收"
			}
			result, err := fixture.handler.Handle(context.Background(), command)
			if err != nil {
				t.Fatalf("handle: %v", err)
			}
			record, _ := result.Record()
			if record.IdentityCheck != testCase.want {
				t.Fatalf("身份核对态 = %q, want %q（outcome %q）", record.IdentityCheck, testCase.want, result.Outcome())
			}
			if len(fixture.doorbell.rung) != 0 {
				t.Fatalf("读口答复了却响了 %d 封门铃", len(fixture.doorbell.rung))
			}
		})
	}
}

// Covers: `AT-NO-016` 与 ADR-0199 决定五「核对态是信息格……首答与重放都带」——未核对那条落册之后，PS 恢复了，同一来源重投答
// `已有结果`：原判断种类是待识别、核对态照读回仍是未核对；不再问身份核对读口、不另落一行、不再响门铃（续上靠门铃重核，不靠
// 重投：翻转之后设备收到 2xx 就出队）。
func TestReplayingAnUncheckedReceptionAnswersTheRecordedCheck(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.identity.err = errors.New("parcel shipment unreachable")
	if _, err := fixture.handler.Handle(context.Background(), receiveCommand(t)); err != nil {
		t.Fatalf("首投：%v", err)
	}
	fixture.identity.err = nil
	counting := &countingIdentity{identityViewDouble: fixture.identity}
	deps := fixture.deps()
	deps.Identity = counting
	handler, err := application.NewReceiveDeliveredUnitHandler(deps)
	if err != nil {
		t.Fatalf("构造收寄编排：%v", err)
	}

	replay, err := handler.Handle(context.Background(), receiveCommand(t))
	if err != nil {
		t.Fatalf("重投：%v", err)
	}
	record, _ := replay.Record()
	if replay.Outcome() != application.ReceptionExistingResult || replay.RecordedOutcome() != application.UnitPendingIdentification {
		t.Fatalf("重投 outcome %q 原判断 %q, want EXISTING_RESULT / PENDING_IDENTIFICATION", replay.Outcome(), replay.RecordedOutcome())
	}
	if record.IdentityCheck != ports.IdentityUnchecked || replay.CurrentVersion() != record.JudgmentVersion {
		t.Fatalf("重投读回核对态 %q 当前版本 %s, want UNCHECKED、仍是那一版", record.IdentityCheck, replay.CurrentVersion())
	}
	if len(counting.asked) != 0 || fixture.receptions.saved != 1 || len(fixture.doorbell.rung) != 1 {
		t.Fatalf("问身份 %d 次、落册 %d、门铃 %d, want 0、1、1", len(counting.asked), fixture.receptions.saved, len(fixture.doorbell.rung))
	}
}

// Covers: ADR-0199 决定五前提「挡住同一作业实物重复形成控制起点」在身份读不动这一格上成立——同一件实物换来源再送到同一节点，
// 并入那条未核对的待识别，答已有结果、核对态是那一版的未核对；不再问 PS、不建控制、不另响门铃。
func TestTheSameUnitAgainIsMergedIntoItsUncheckedPendingUnit(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.identity.err = errors.New("parcel shipment unreachable")
	first := receiveCommand(t)
	formed, err := fixture.handler.Handle(context.Background(), first)
	if err != nil {
		t.Fatalf("首投：%v", err)
	}
	pending, _ := formed.Record()

	fixture.identity.err = nil
	again := first
	again.SourceID = "delivery-again"
	again.OccurredAt = first.OccurredAt.Add(30 * time.Minute)
	result, err := fixture.handler.Handle(context.Background(), again)
	if err != nil {
		t.Fatalf("再来：%v", err)
	}
	record, _ := result.Record()
	if result.Outcome() != application.ReceptionExistingResult || result.MergedInto() != pending.JudgmentVersion {
		t.Fatalf("再来 outcome %q 并入 %s, want EXISTING_RESULT 并入 %s", result.Outcome(), result.MergedInto(), pending.JudgmentVersion)
	}
	if record.IdentityCheck != ports.IdentityUnchecked || result.RecordedOutcome() != application.UnitPendingIdentification {
		t.Fatalf("并入的那一版核对态 %q 原判断 %q, want UNCHECKED / PENDING_IDENTIFICATION", record.IdentityCheck, result.RecordedOutcome())
	}
	if starts := controlStartsOf(fixture, "unit-1"); starts != 1 {
		t.Fatalf("控制起点 %d 个, want 1", starts)
	}
	if len(fixture.doorbell.rung) != 1 || len(fixture.downstream.intents) != 0 {
		t.Fatalf("门铃 %d 封、意图 %d 封, want 1、0", len(fixture.doorbell.rung), len(fixture.downstream.intents))
	}
}

// Covers: ADR-0199 决定三「落下未核对行的同一笔事务给本上下文自投一封门铃」——门铃入不了队就不形成答案：一条没有门铃的未核对行
// 只能靠人工续上，比翻转前倒退。交回的不是被拒类错误，装配据此整笔回滚、答 5xx，设备留队重投。
func TestAnUncheckedReceptionWhoseDoorbellCannotBeRungFormsNoAnswer(t *testing.T) {
	fixture := newReceptionFixture(t)
	fixture.identity.err = errors.New("parcel shipment unreachable")
	fixture.doorbell.err = errors.New("outbox down")

	result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err == nil {
		t.Fatalf("outcome = %q, want error", result.Outcome())
	}
	if errors.Is(err, application.ErrReceptionCommandRejected) {
		t.Fatalf("门铃入不了队被当成了输入缺陷：%v", err)
	}
}

// Covers: ADR-0199 决定三「零候选 → 判断不变、不追加；被重核那一版若是『未核对』，追加一版『待识别 · 已核对』」——门铃重核时
// PS 答零候选：追加一版待识别，核对态已核对、候选为空、不记冲突、不带关联；前版是未核对那一版，来源标识由它派生，控制、接收
// 时刻与来源文档照原判断；不交意图、不问服务结果。这一版不是未核对，再重核它判断不变、不追加；再重核原来那一版答已有结果。
func TestRecheckingAnUncheckedPendingUnitWithNoCandidateAppendsACheckedVersion(t *testing.T) {
	fixture := newRecheckFixture(t)
	unchecked := fixture.receiveUnchecked(t)
	fixture.answer(t, false)

	result := recheckNamed(t, fixture, unchecked.JudgmentVersion)
	if result.Outcome() != application.RecheckIdentityNowChecked {
		t.Fatalf("outcome = %v, want IDENTITY_NOW_CHECKED", result.Outcome())
	}
	checked, present := result.Record()
	_, associated := checked.Intake.Association()
	switch {
	case !present || checked.Kind != ports.RecordPendingIdentification || checked.IdentityCheck != ports.IdentityChecked:
		t.Fatalf("追加版 = %+v, want 待识别、已核对", checked)
	case len(checked.Candidates) != 0 || checked.IdentityConflict || associated:
		t.Fatalf("追加版候选 %v 冲突 %v 关联 %v, want 都没有", checked.Candidates, checked.IdentityConflict, associated)
	case checked.Predecessor != unchecked.JudgmentVersion || checked.Key.SourceID != "IDENTITY-RECHECK/"+unchecked.JudgmentVersion.String():
		t.Fatalf("追加版前版 %s 键 %s, want 接续未核对那一版、派生键", checked.Predecessor, checked.Key.SourceID)
	case checked.Control != unchecked.Control || !checked.Intake.ReceivedAt().Equal(deliveredAt) || checked.Source != unchecked.Source:
		t.Fatal("追加版的控制、接收时刻或来源文档不是原判断那一份")
	}
	if len(fixture.downstream.intents) != 0 || len(fixture.serviceResults.asked) != 0 {
		t.Fatal("零候选的追加版交了意图或问了服务结果")
	}

	rows := fixture.rows()
	if again := recheckNamed(t, fixture, checked.JudgmentVersion); again.Outcome() != application.RecheckStillUnidentified {
		t.Fatalf("重核已核对那一版 = %v, want STILL_UNIDENTIFIED", again.Outcome())
	}
	if again := recheckNamed(t, fixture, unchecked.JudgmentVersion); again.Outcome() != application.RecheckExistingResult {
		t.Fatalf("再重核未核对那一版 = %v, want EXISTING_RESULT", again.Outcome())
	}
	if fixture.rows() != rows {
		t.Fatalf("册上 %d → %d 行, want 不再追加", rows, fixture.rows())
	}
}

// Covers: `AT-NO-026` 接上未核对（ADR-0199 决定三「重核对象」含不带关联、不记冲突的待识别）——门铃重核时 PS 已恢复、号码唯一
// 命中：追加一版`节点收寄已形成`，业务时间是原收寄时刻、控制照原判断，意图一封；按号重核同样找得到未核对那一件。
func TestRecheckingAnUncheckedPendingUnitOnceReadableFormsTheIntake(t *testing.T) {
	for name, recheck := range map[string]func(*testing.T, *recheckFixture, ports.ReceptionRecord) application.RecheckPendingIdentityResult{
		"门铃指名": func(t *testing.T, fixture *recheckFixture, unchecked ports.ReceptionRecord) application.RecheckPendingIdentityResult {
			return recheckNamed(t, fixture, unchecked.JudgmentVersion)
		},
		"按号": func(t *testing.T, fixture *recheckFixture, _ ports.ReceptionRecord) application.RecheckPendingIdentityResult {
			results := recheckByMark(t, fixture)
			if len(results) != 1 {
				t.Fatalf("按号重核找到 %d 件, want 1", len(results))
			}
			return results[0]
		},
	} {
		t.Run(name, func(t *testing.T) {
			fixture := newRecheckFixture(t)
			unchecked := fixture.receiveUnchecked(t)
			fixture.answer(t, false, "parcel-1")

			result := recheck(t, fixture, unchecked)
			formed, _ := result.Record()
			association, associated := formed.Intake.Association()
			if result.Outcome() != application.RecheckIntakeFormed || !associated || association.String() != "parcel-1" {
				t.Fatalf("outcome %v 追加版 %+v, want INTAKE_FORMED 关联 parcel-1", result.Outcome(), formed)
			}
			if formed.Predecessor != unchecked.JudgmentVersion || !formed.Intake.ReceivedAt().Equal(deliveredAt) || formed.Control != unchecked.Control {
				t.Fatal("追加版没有接续未核对那一版，或另起了控制起点")
			}
			if formed.IdentityCheck != ports.IdentityCheckAbsent {
				t.Fatalf("关联收寄带了身份核对态 %q", formed.IdentityCheck)
			}
			if len(fixture.downstream.intents) != 1 || fixture.downstream.intents[0].Record.Key != formed.Key {
				t.Fatalf("意图 %d 封, want 恰一封、指回追加版", len(fixture.downstream.intents))
			}
		})
	}
}

// Covers: ADR-0199 决定三「身份核对仍读不动 → 不追加，交回 error」在门铃这一路——未核对那一件重核时 PS 仍读不动：交回带
// ErrParcelIdentityUnreadable 的错，册上不追加，门铃留在 inbox 退避重投。
func TestRecheckingAnUncheckedPendingUnitThatIsStillUnreadableAppendsNothing(t *testing.T) {
	fixture := newRecheckFixture(t)
	unchecked := fixture.receiveUnchecked(t)
	fixture.identity.err = errors.New("parcel shipment still unreachable")
	rows := fixture.rows()

	_, err := fixture.recheck.Recheck(context.Background(), application.RecheckPendingIdentityCommand{
		TenantID: unchecked.Key.TenantID, Version: unchecked.JudgmentVersion,
	})
	if !errors.Is(err, application.ErrParcelIdentityUnreadable) {
		t.Fatalf("err = %v, want ErrParcelIdentityUnreadable", err)
	}
	if fixture.rows() != rows || len(fixture.downstream.intents) != 0 {
		t.Fatal("仍读不动的重核追加了判断或交了意图")
	}
}

// Covers: ADR-0199 决定五「已核对」的另两处来路——重核追加的冲突版与 ADR-0196 给先到那件追加的冲突版都是读口答复了之后落的，
// 核对态记已核对。
func TestAppendedConflictVersionsAreMarkedChecked(t *testing.T) {
	fixture := newRecheckFixture(t)
	pending := fixture.receivePending(t, "delivery-late", "unit-late")
	if pending.IdentityCheck != ports.IdentityChecked {
		t.Fatalf("零候选的待识别核对态 = %q, want CHECKED", pending.IdentityCheck)
	}
	fixture.answer(t, false, "parcel-1")
	early := receiveCommand(t)
	early.SourceID, early.Unit = "delivery-early", mustValue(t, domain.NewHandlingUnitID, "unit-early")
	formed, err := fixture.handler.Handle(context.Background(), early)
	if err != nil || formed.Outcome() != application.NodeIntakeFormed {
		t.Fatalf("先到那件 outcome = %v err = %v", formed.Outcome(), err)
	}
	earlyRecord, _ := formed.Record()

	result := recheckNamed(t, fixture, pending.JudgmentVersion)
	late, _ := result.Record()
	if result.Outcome() != application.RecheckIdentityConflict || late.IdentityCheck != ports.IdentityChecked {
		t.Fatalf("被重核那件的冲突版 outcome %v 核对态 %q, want IDENTITY_CONFLICT / CHECKED", result.Outcome(), late.IdentityCheck)
	}
	current, err := fixture.receptions.CurrentVersion(context.Background(), earlyRecord.Key.TenantID, earlyRecord.JudgmentVersion)
	if err != nil {
		t.Fatalf("先到那件的当前版本：%v", err)
	}
	earlyConflict, found, _ := fixture.receptions.FindByJudgmentVersion(context.Background(), earlyRecord.Key.TenantID, current)
	if !found || !earlyConflict.IdentityConflict || earlyConflict.IdentityCheck != ports.IdentityChecked {
		t.Fatalf("先到那件的冲突版 = %+v, want 记身份冲突、已核对", earlyConflict)
	}
}

// receiveUnchecked 让一件实物在身份核对读不动时经收寄编排落成未核对的待识别，交回那一条记录；之后读口恢复成零候选。
func (fixture *recheckFixture) receiveUnchecked(t *testing.T) ports.ReceptionRecord {
	t.Helper()
	fixture.identity.err = errors.New("parcel shipment unreachable")
	result, err := fixture.handler.Handle(context.Background(), receiveCommand(t))
	if err != nil || result.Outcome() != application.UnitPendingIdentification {
		t.Fatalf("收寄 outcome = %v err = %v, want PENDING_IDENTIFICATION", result.Outcome(), err)
	}
	record, _ := result.Record()
	if record.IdentityCheck != ports.IdentityUnchecked {
		t.Fatalf("收寄核对态 = %q, want UNCHECKED", record.IdentityCheck)
	}
	fixture.identity.err = nil
	return record
}
