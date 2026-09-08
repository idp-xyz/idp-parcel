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

// 本文件证 ADR-0112 决定二、三在编排一侧：两条来源的更正共用一道重派生门，与更正同事务、作派生一侧
// ——登记册故障只留续办引用不翻更正，领域正当拒绝单开答格；段由登记册按对象反查，更正命令不带段号；
// 段已关闭照样重派生且段不重开。
//
// 用例写在读对方在途代码之前（parallel-sessions「先写自己第一片 red」），判据取自 ADR-0112 与领域读面。

func objectRef(t *testing.T, raw string) domain.CarriedObjectReference {
	t.Helper()
	object, err := domain.NewCarriedObjectReference(raw)
	if err != nil {
		t.Fatalf("对象引用：%v", err)
	}
	return object
}

func correctHandoverStillHandedOver(t *testing.T, predecessor, next string) application.CorrectTransportHandoverCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("tenant: %v", err)
	}
	return application.CorrectTransportHandoverCommand{
		TenantID:           tenant,
		Object:             "parcel-1",
		Scope:              "handover-scope-1",
		PredecessorVersion: predecessor,
		Verdict:            domain.ObjectHandedOver,
		ReleasingEvidence:  "evidence-release-1-recheck",
		ReceivingEvidence:  "evidence-receive-1-recheck",
		Rule:               "handover-rule/v1",
		NewVersion:         next,
		CorrectedAt:        handoverJudgedTime.Add(2 * time.Hour),
	}
}

// Covers: ADR-0112 决定一、二在揽收一路——更正落新版本之后同事务重派生：段里该对象的当前参与换成回指前版
// 的替代版本，起点随更正后的发生时刻，原参与一字不动；段不另立、不另 Join；命令不带段号，段由登记册按
// 对象反查；成功时段那一半不留引用、不答拒绝。
func TestAPickupCorrectionRederivesTheParticipationOnTheSameSegment(t *testing.T) {
	fixture := newPickupSegmentFixture(t)
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"
	command.PlannedSegment = "planned-segment-1"
	if _, err := fixture.handler.Register(context.Background(), command); err != nil {
		t.Fatalf("seed: %v", err)
	}
	savesBefore, joinsBefore := fixture.segments.saves, fixture.segments.joins

	correction := pickupCorrectionCommand(t, "pickup-result/v1")
	result, err := fixture.handler.Correct(context.Background(), correction)
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.PickupCorrected {
		t.Fatalf("outcome = %q, want PICKUP_CORRECTED", result.Outcome())
	}
	if result.SegmentContinuationReference() != "" || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("重派生成功却答了欠账或拒绝：%q %q", result.SegmentContinuationReference(), result.SegmentEntryRefusal())
	}
	if fixture.segments.saves != savesBefore || fixture.segments.joins != joinsBefore {
		t.Fatalf("更正另立了段或另 Join 了：saves %d→%d joins %d→%d", savesBefore, fixture.segments.saves, joinsBefore, fixture.segments.joins)
	}
	if fixture.segments.supersedes != 1 {
		t.Fatalf("替代窄口被调 %d 次, want 1", fixture.segments.supersedes)
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	parcel := objectRef(t, "parcel-1")
	current, present := record.Segment.ParticipationFor(parcel)
	if !present || current.EntryBasis().String() != "OFFSITE-PICKUP/pickup-result/v2" {
		t.Fatalf("当前参与 = %+v present=%v, want 入场依据 OFFSITE-PICKUP/pickup-result/v2", current, present)
	}
	if supersedes, chained := current.Supersedes(); !chained || supersedes.String() != "OFFSITE-PICKUP/pickup-result/v1" {
		t.Fatalf("替代版本没回指原参与：%v %v", supersedes, chained)
	}
	if !current.EnteredAt().Equal(correction.OccurredAt) {
		t.Fatalf("替代版本起点 = %s, want 更正后的发生时刻 %s", current.EnteredAt(), correction.OccurredAt)
	}
	if planned, has := current.PlannedSegment(); !has || planned.String() != "planned-segment-1" {
		t.Fatal("替代版本没沿用计划段")
	}
	history := record.Segment.ParticipationHistory(parcel)
	if len(history) != 2 || !history[0].Superseded() || !history[0].EnteredAt().Equal(pickupOccurredAt) {
		t.Fatalf("原参与不在历史里或被改写：%+v", history)
	}
	if record.Segment.ActiveParticipations() != 1 {
		t.Fatalf("在场参与 = %d, want 1（只数链尾）", record.Segment.ActiveParticipations())
	}

	t.Run("a second correction chains onto the tail", func(t *testing.T) {
		second := pickupCorrectionCommand(t, "pickup-result/v2")
		second.OccurredAt = correction.OccurredAt.Add(-30 * time.Minute)
		result, err := fixture.handler.Correct(context.Background(), second)
		if err != nil || result.Outcome() != application.PickupCorrected {
			t.Fatalf("second correction: %v outcome=%q", err, result.Outcome())
		}
		record := fixture.segments.saved(t, "tenant-1", "segment-1")
		current, _ := record.Segment.ParticipationFor(parcel)
		if supersedes, _ := current.Supersedes(); current.EntryBasis().String() != "OFFSITE-PICKUP/pickup-result/v3" || supersedes.String() != "OFFSITE-PICKUP/pickup-result/v2" {
			t.Fatalf("链没接上：%+v", current)
		}
		if len(record.Segment.ParticipationHistory(parcel)) != 3 || record.Segment.ActiveParticipations() != 1 {
			t.Fatal("三版链或在场计数走样")
		}
	})
}

// Covers: ADR-0112 决定二「领域正当拒绝单开答格」——对象从未进段（首登没给段号）时更正照落、意图照交，
// 段那一半答 NO_PARTICIPATION_TO_REDERIVE，不留续办引用（重试一万次都一样）。
func TestAPickupCorrectionWithNothingToRederiveAnswersTheRefusal(t *testing.T) {
	fixture := newPickupSegmentFixture(t)
	if _, err := fixture.handler.Register(context.Background(), pickupRegistrationCommand(t)); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.PickupCorrected {
		t.Fatalf("outcome = %q, want PICKUP_CORRECTED——段那一半拒了抹不掉已落的更正", result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusedNoParticipationToRederive {
		t.Fatalf("refusal = %q, want NO_PARTICIPATION_TO_REDERIVE", result.SegmentEntryRefusal())
	}
	if result.SegmentEntryRefusal().String() != "NO_PARTICIPATION_TO_REDERIVE" {
		t.Fatalf("refusal label = %q", result.SegmentEntryRefusal().String())
	}
	if result.SegmentContinuationReference() != "" {
		t.Fatal("领域正当拒绝不该留续办引用")
	}
	if fixture.segments.supersedes != 0 {
		t.Fatal("没有可替代的参与却调了替代窄口")
	}
}

// Covers: ADR-0112 决定二「派生一侧的失败不得回滚来源更正」——段登记册读不到时更正照落、意图照交，段那
// 一半只留续办引用、不答拒绝；段登记册缺席（装配没交入）时两格都空。
func TestASegmentRegistryFailureDuringRederivationKeepsTheCorrection(t *testing.T) {
	fixture := newPickupSegmentFixture(t)
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"
	if _, err := fixture.handler.Register(context.Background(), command); err != nil {
		t.Fatalf("seed: %v", err)
	}
	fixture.segments.findErr = errors.New("段登记册不可用")

	result, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.PickupCorrected {
		t.Fatalf("outcome = %q, want PICKUP_CORRECTED", result.Outcome())
	}
	if result.SegmentContinuationReference() == "" {
		t.Fatal("登记册故障却没有续办引用——调用方无从续办这一半")
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("登记册故障被答成了领域拒绝 %q", result.SegmentEntryRefusal())
	}
	if len(fixture.pickups.chainOf(t, keyOfFirstPickup(t, fixture))) != 2 {
		t.Fatal("段那一半失败翻掉了更正版本")
	}

	t.Run("without a segment registry the correction answers nothing about segments", func(t *testing.T) {
		bare := newPickupRegFixture(t)
		seedRegisteredPickup(t, bare)
		result, err := bare.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
		if err != nil || result.Outcome() != application.PickupCorrected {
			t.Fatalf("correct: %v outcome=%q", err, result.Outcome())
		}
		if result.SegmentContinuationReference() != "" || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
			t.Fatalf("派生一侧缺席却答了段那一半：%q %q", result.SegmentContinuationReference(), result.SegmentEntryRefusal())
		}
	})
}

// Covers: ADR-0112 决定三「段已关闭仍重派生」——对象已按交付离场、段已关闭，来源更正照样在同段长出替代
// 版本：继承原参与的离场三件，段仍关闭、不重开。
func TestAPickupCorrectionRederivesIntoAClosedSegmentWithoutReopeningIt(t *testing.T) {
	fixture := newPickupSegmentFixture(t)
	command := pickupRegistrationCommand(t)
	command.Segment = "segment-1"
	if _, err := fixture.handler.Register(context.Background(), command); err != nil {
		t.Fatalf("seed: %v", err)
	}
	// 直接在替身的库面上让对象离场、段关闭：结束与关段各有自己的编排，这里只要那个库面状态。
	deliveredAt := pickupOccurredAt.Add(8 * time.Hour)
	closedAt := deliveredAt.Add(time.Hour)
	for _, rows := range fixture.segments.rows {
		for index := range rows.participations {
			row := &rows.participations[index]
			row.EndKind = domain.EndedByEffectiveDelivery
			row.EndBasis = mustBasis(t, "EFFECTIVE-DELIVERY/delivery-result/parcel-1/v1")
			row.EndedAt = deliveredAt
		}
		rows.closed, rows.closedAt = true, closedAt
	}

	result, err := fixture.handler.Correct(context.Background(), pickupCorrectionCommand(t, "pickup-result/v1"))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.PickupCorrected || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
		t.Fatalf("已关闭段上的重派生被拒或欠账：%q %q %q", result.Outcome(), result.SegmentEntryRefusal(), result.SegmentContinuationReference())
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	if !record.Segment.Closed() {
		t.Fatal("重派生把段重开了")
	}
	if at, _ := record.Segment.ClosedAt(); !at.Equal(closedAt) {
		t.Fatalf("关闭时刻被动了：%v", at)
	}
	tail, _ := record.Segment.ParticipationFor(objectRef(t, "parcel-1"))
	kind, basis, at, ended := tail.End()
	if tail.EntryBasis().String() != "OFFSITE-PICKUP/pickup-result/v2" || !ended ||
		kind != domain.EndedByEffectiveDelivery || basis.String() != "EFFECTIVE-DELIVERY/delivery-result/parcel-1/v1" || !at.Equal(deliveredAt) {
		t.Fatalf("替代版本没继承离场三件：%+v", tail)
	}
	if record.Segment.ActiveParticipations() != 0 {
		t.Fatal("已关闭段上凭空多了在场参与")
	}
}

// Covers: ADR-0112 决定一、二在交接一路——仍`已交接`的更正形成替代参与版本（入场依据换成新版本的转出引用，
// 起点沿用裁决业务时间）；两条来源共用同一道门，所以行为与揽收一路逐字一致。
func TestAHandoverCorrectionThatStillTransfersControlRederivesTheParticipation(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	command := registerHandoverCommand(t)
	command.Segment = "segment-1"
	if _, err := fixture.handler.Register(context.Background(), command); err != nil {
		t.Fatalf("seed: %v", err)
	}

	result, err := fixture.handler.Correct(context.Background(), correctHandoverStillHandedOver(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2"))
	if err != nil {
		t.Fatalf("correct: %v", err)
	}
	if result.Outcome() != application.HandoverCorrected {
		t.Fatalf("outcome = %q, want HANDOVER_CORRECTED", result.Outcome())
	}
	if result.SegmentContinuationReference() != "" || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
		t.Fatalf("重派生成功却答了欠账或拒绝：%q %q", result.SegmentContinuationReference(), result.SegmentEntryRefusal())
	}
	if fixture.segments.supersedes != 1 || fixture.segments.saves != 1 || fixture.segments.joins != 0 {
		t.Fatalf("窄口调用走样：supersedes=%d saves=%d joins=%d", fixture.segments.supersedes, fixture.segments.saves, fixture.segments.joins)
	}

	record := fixture.segments.saved(t, "tenant-1", "segment-1")
	parcel := objectRef(t, "parcel-1")
	current, _ := record.Segment.ParticipationFor(parcel)
	if current.EntryBasis().String() != "TRANSPORT-HANDOVER/handover-result/parcel-1/v2" || current.EntryKind() != domain.EnteredByTransportHandover {
		t.Fatalf("当前参与 = %+v, want 新版本的转出引用", current)
	}
	if supersedes, chained := current.Supersedes(); !chained || supersedes.String() != "TRANSPORT-HANDOVER/handover-result/parcel-1/v1" {
		t.Fatalf("替代版本没回指原参与：%v %v", supersedes, chained)
	}
	if !current.EnteredAt().Equal(handoverJudgedTime) {
		t.Fatalf("交接更正不改业务时间，起点该沿用 %s，得到 %s", handoverJudgedTime, current.EnteredAt())
	}
	if len(record.Segment.ParticipationHistory(parcel)) != 2 || record.Segment.ActiveParticipations() != 1 {
		t.Fatal("链或在场计数走样")
	}
}

// Covers: ADR-0112 决定四与票 tf-segment-lifecycle-closure/11 裁决 1、2、5 在编排一侧——`已交接`被更正为拒收/待确认
// 走的是同一道重派生门：更正照落，段里凭前版入场的参与经 `Supersede` 长出失效版本（回指前版、Voided 为是、不在场），
// 原参与一字不动；成功时两格都空，与替代版本同形——CORRECTION_WITHDRAWS_CONTROL 那一格已退场，撤回控制不再是段
// 那一半的拒绝。此后该对象在本段当前无有效参与：在场计数为零，按对象反查在场也找不到它。
func TestAHandoverCorrectionThatWithdrawsControlVoidsTheParticipation(t *testing.T) {
	for _, verdict := range []domain.HandoverVerdict{domain.HandoverRefused, domain.HandoverPendingConfirmation} {
		t.Run(verdict.String(), func(t *testing.T) {
			fixture := newHandoverSegmentFixture(t)
			command := registerHandoverCommand(t)
			command.Segment = "segment-1"
			if _, err := fixture.handler.Register(context.Background(), command); err != nil {
				t.Fatalf("seed: %v", err)
			}

			withdrawing := correctHandoverStillHandedOver(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2")
			withdrawing.Verdict = verdict
			withdrawing.ReleasingEvidence, withdrawing.ReceivingEvidence, withdrawing.Rule = "", "", ""
			withdrawing.Basis = "handover-basis/withdrawn-1"
			result, err := fixture.handler.Correct(context.Background(), withdrawing)
			if err != nil {
				t.Fatalf("correct: %v", err)
			}
			if result.Outcome() != application.HandoverCorrected {
				t.Fatalf("outcome = %q, want HANDOVER_CORRECTED", result.Outcome())
			}
			if result.SegmentContinuationReference() != "" || result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone {
				t.Fatalf("失效版本落地成功却答了欠账或拒绝：%q %q", result.SegmentContinuationReference(), result.SegmentEntryRefusal())
			}
			if fixture.segments.supersedes != 1 || fixture.segments.saves != 1 || fixture.segments.joins != 0 {
				t.Fatalf("窄口调用走样：supersedes=%d saves=%d joins=%d", fixture.segments.supersedes, fixture.segments.saves, fixture.segments.joins)
			}

			parcel := objectRef(t, "parcel-1")
			record := fixture.segments.saved(t, "tenant-1", "segment-1")
			tail, present := record.Segment.ParticipationFor(parcel)
			if !present || !tail.Voided() || tail.Active() {
				t.Fatalf("链尾不是失效版本：present=%v voided=%v active=%v", present, tail.Voided(), tail.Active())
			}
			if tail.EntryBasis().String() != "TRANSPORT-HANDOVER/handover-result/parcel-1/v2" || tail.EntryKind() != domain.EnteredByTransportHandover {
				t.Fatalf("失效版本的入场依据或种类走样：%+v", tail)
			}
			if supersedes, chained := tail.Supersedes(); !chained || supersedes.String() != "TRANSPORT-HANDOVER/handover-result/parcel-1/v1" {
				t.Fatalf("失效版本没回指原参与：%v %v", supersedes, chained)
			}
			if !tail.EnteredAt().Equal(handoverJudgedTime) {
				t.Fatalf("失效版本的起点该沿用被失效那一版 %s，得到 %s", handoverJudgedTime, tail.EnteredAt())
			}
			history := record.Segment.ParticipationHistory(parcel)
			if len(history) != 2 || history[0].EntryBasis().String() != "TRANSPORT-HANDOVER/handover-result/parcel-1/v1" ||
				!history[0].Superseded() || history[0].Voided() || !history[0].EnteredAt().Equal(handoverJudgedTime) {
				t.Fatalf("原参与被动了或没被标为已被替代：%+v", history)
			}
			if record.Segment.ActiveParticipations() != 0 {
				t.Fatalf("在场参与 = %d，失效版本不算在场", record.Segment.ActiveParticipations())
			}
			tenant, _ := domain.NewTenantID("tenant-1")
			active, err := fixture.segments.FindActiveSegments(context.Background(), tenant, parcel)
			if err != nil || len(active) != 0 {
				t.Fatalf("按对象反查在场仍找得到已失效的参与：%v err=%v", active, err)
			}
		})
	}
}

// Covers: 段由登记册按对象反查——对象先后在两段（前一段已按下一次交接离场、后一段在场），更正前一段的入场
// 来源只替代前一段里那一条，后一段一字不动。
func TestARederivationLandsOnTheSegmentWhoseCurrentParticipationCarriesTheCorrectedVersion(t *testing.T) {
	fixture := newHandoverSegmentFixture(t)
	first := registerHandoverCommand(t)
	first.Segment = "segment-1"
	if _, err := fixture.handler.Register(context.Background(), first); err != nil {
		t.Fatalf("seed first: %v", err)
	}
	// 前一段离场、进后一段：直接摆库面，结束与进段各有自己的编排。
	for _, rows := range fixture.segments.rows {
		row := &rows.participations[0]
		row.EndKind = domain.EndedByNextHandover
		row.EndBasis = mustBasis(t, "TRANSPORT-HANDOVER/handover-result/parcel-1/next")
		row.EndedAt = handoverJudgedTime.Add(6 * time.Hour)
	}
	second := registerHandoverCommand(t)
	second.Version = "handover-result/parcel-1/next"
	second.Scope = "handover-scope-2"
	second.JudgedAt = handoverJudgedTime.Add(6 * time.Hour)
	second.Segment = "segment-2"
	if _, err := fixture.handler.Register(context.Background(), second); err != nil {
		t.Fatalf("seed second: %v", err)
	}

	result, err := fixture.handler.Correct(context.Background(), correctHandoverStillHandedOver(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2"))
	if err != nil || result.Outcome() != application.HandoverCorrected {
		t.Fatalf("correct: %v outcome=%q", err, result.Outcome())
	}
	if result.SegmentEntryRefusal() != application.SegmentEntryRefusalNone || result.SegmentContinuationReference() != "" {
		t.Fatalf("已离场的前一段照样要重派生：%q %q", result.SegmentEntryRefusal(), result.SegmentContinuationReference())
	}
	parcel := objectRef(t, "parcel-1")
	previous := fixture.segments.saved(t, "tenant-1", "segment-1")
	tail, _ := previous.Segment.ParticipationFor(parcel)
	if tail.EntryBasis().String() != "TRANSPORT-HANDOVER/handover-result/parcel-1/v2" || tail.Active() {
		t.Fatalf("前一段的替代版本走样（该回指 v1 且继承离场）：%+v", tail)
	}
	next := fixture.segments.saved(t, "tenant-1", "segment-2")
	if len(next.Segment.ParticipationHistory(parcel)) != 1 {
		t.Fatal("后一段被牵连")
	}
}

func mustBasis(t *testing.T, raw string) domain.ParticipationBasisReference {
	t.Helper()
	basis, err := domain.NewParticipationBasisReference(raw)
	if err != nil {
		t.Fatalf("basis: %v", err)
	}
	return basis
}

func keyOfFirstPickup(t *testing.T, fixture *pickupSegmentFixture) ports.OffsitePickupKey {
	t.Helper()
	tenant, _ := domain.NewTenantID("tenant-1")
	return ports.OffsitePickupKey{
		TenantID: tenant,
		Object:   objectRef(t, "parcel-1"),
		Attempt:  mustAttempt(t, "attempt-1"),
	}
}

func mustAttempt(t *testing.T, raw string) domain.AttemptReference {
	t.Helper()
	attempt, err := domain.NewAttemptReference(raw)
	if err != nil {
		t.Fatalf("attempt: %v", err)
	}
	return attempt
}
