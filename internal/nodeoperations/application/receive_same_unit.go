package application

import (
	"context"
	"errors"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件是收寄编排里同一作业实物在同一节点的那几步（ADR-0199 决定四）：从各链的当前有效版本里认当前收寄与要续办的待确认，
// 并入与答并入。

// currentIntakeAmong 从这件实物在这个节点上各链的当前有效版本里挑「当前收寄」：形成或待识别、控制没有转出（ADR-0199 决定四）。
// 并入之前同一件实物在同一节点各自形成过收寄时不止一条，取接收时间最晚的那一条，同刻取来源标识在前的（各链按来源标识定序
// 交来）——与 ADR-0196 冲突版依哪一行追加、方向依据取所依收寄同一个口径。
func currentIntakeAmong(judgments []ports.ReceptionRecord) (ports.ReceptionRecord, bool) {
	var current ports.ReceptionRecord
	found := false
	for _, judgment := range judgments {
		intake := judgment.Kind == ports.RecordIntakeFormed || judgment.Kind == ports.RecordPendingIdentification
		if !intake || !judgment.Control.Active() {
			continue
		}
		if !found || judgment.Intake.ReceivedAt().After(current.Intake.ReceivedAt()) {
			current, found = judgment, true
		}
	}
	return current, found
}

// undecidedToContinue 挑后来的观察要续办的那条待确认，交回它的判断版本作新判断的前版；没有交回空（ADR-0199 决定四）。续办之前
// 同一件实物在同一节点仅扫描过多次、各成一链时，取发生时刻最晚的那一条，同刻取来源标识在前的，其余照留。`节点收寄未形成`
// 不在其中：拒收之后的再次交付是另一次交付尝试，不续办那次拒收。
//
// 来源保全之前落下的待确认没有判断版本，也没存作业实物与节点，收寄库认不出它是这件实物的；后来的观察照首判办、不指名前版，
// 原行照留。不为让它能被指名而补签版本，迁移 0007 头注说过补签的后果：
// 「就是在册上造出一版当时并不存在的判断」。
func undecidedToContinue(judgments []ports.ReceptionRecord) domain.IntakeResultVersion {
	var continued ports.ReceptionRecord
	found := false
	for _, judgment := range judgments {
		if judgment.Kind != ports.RecordReceptionUndecided {
			continue
		}
		if !found || judgment.Source.OccurredAt.After(continued.Source.OccurredAt) {
			continued, found = judgment, true
		}
	}
	return continued.JudgmentVersion
}

// merge 把这次送站并入那条当前收寄（ADR-0199 决定四）：落一行「并入已有收寄」，来源文档照存、指名并入的那一版；不形成新判断、
// 不签版本、不建控制、不交意图。三种观察都这样：已在节点控制下的实物，再扫一次、再签收一次、再说一次拒收，都不改变它此刻
// 在节点的事实，只是又一条要留下的来源。
//
// 撞上同一来源已在册（并发里另一方先落了同一次送站）就读回赢家作答，与 commit 同一个口径。
func (handler *ReceiveDeliveredUnitHandler) merge(
	ctx context.Context,
	command ReceiveDeliveredUnitCommand,
	key ports.ReceptionKey,
	digest string,
	current ports.ReceptionRecord,
) (ReceiveDeliveredUnitResult, error) {
	saved, err := handler.deps.Receptions.Save(ctx, ports.ReceptionRecord{
		Key:           key,
		ContentDigest: digest,
		Kind:          ports.RecordMergedIntoExistingIntake,
		MergedInto:    current.JudgmentVersion,
		Source:        deliverySourceOf(command),
		RecordedAt:    handler.deps.Clock.Now(),
	})
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("record the merge into the current intake: %w", err)
	}
	switch saved {
	case ports.ReceptionSaved:
		return handler.mergedResult(ctx, current)
	case ports.ReceptionAlreadyRecorded:
		winner, found, err := handler.deps.Receptions.FindByKey(ctx, key)
		if err != nil {
			return ReceiveDeliveredUnitResult{}, fmt.Errorf("read back reception winner: %w", err)
		}
		if !found {
			return ReceiveDeliveredUnitResult{}, errors.New("node operations: reception already recorded but not readable")
		}
		return handler.existingResult(ctx, winner)
	default:
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("%w: %d", ErrUnexpectedReceptionSave, saved)
	}
}

// mergedResult 答并入：已有结果，带回并入的那一版判断，原判断种类与当前有效版本都是那一版的（ADR-0199 决定四「信息格照决定二」）。
// 不交意图：并入没有形成可供 PS 采用的东西；那一版若是形成格，它的意图由它自己那一来源重投时重发。
func (handler *ReceiveDeliveredUnitHandler) mergedResult(
	ctx context.Context,
	merged ports.ReceptionRecord,
) (ReceiveDeliveredUnitResult, error) {
	current, err := handler.deps.Receptions.CurrentVersion(ctx, merged.Key.TenantID, merged.JudgmentVersion)
	if err != nil {
		return ReceiveDeliveredUnitResult{}, fmt.Errorf("current version of the merged judgment: %w", err)
	}
	result := resultForRecord(merged)
	result.outcome = ReceptionExistingResult
	result.currentVersion = current
	result.mergedInto = merged.JudgmentVersion
	return result, nil
}
