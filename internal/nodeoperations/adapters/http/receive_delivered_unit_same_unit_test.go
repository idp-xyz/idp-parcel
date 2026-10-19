package nodeopshttp_test

import (
	"context"
	"errors"
	"net/http"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证同一作业实物在同一节点并入已有收寄的 HTTP 面（ADR-0199 决定四，票 no-reception-source/04）。收寄库替身沿用本包
// 那一只，在这里补上收寄编排新问的两问；重核命令口的替身不收寄，那两问不该被问到。

// FindCurrentJudgmentsAtNode 在内存里找这件实物在这个节点上各链的当前有效版本：本文件只送带收寄的观察，按收寄上的实物与节点认。
func (double *receptionStoreDouble) FindCurrentJudgmentsAtNode(
	_ context.Context,
	tenant domain.TenantID,
	unit domain.HandlingUnitID,
	node domain.NodeReference,
) ([]ports.ReceptionRecord, error) {
	found := []ports.ReceptionRecord{}
	for key, record := range double.byKey {
		if key.TenantID != tenant || record.JudgmentVersion.String() == "" || record.Kind == ports.RecordMergedIntoExistingIntake {
			continue
		}
		recordUnit, recordNode := record.Source.Unit, record.Source.Node
		if !record.Intake.ReceivedAt().IsZero() {
			recordUnit, recordNode = record.Intake.Unit(), record.Intake.Node()
		}
		if recordUnit == unit && recordNode == node {
			found = append(found, record)
		}
	}
	return found, nil
}

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

func (store *recheckStore) FindCurrentJudgmentsAtNode(
	context.Context, domain.TenantID, domain.HandlingUnitID, domain.NodeReference,
) ([]ports.ReceptionRecord, error) {
	return nil, errors.New("重核命令口不收寄")
}

// Covers: `AT-NO-049`「同一作业实物换来源，再次交到已有当前收寄的同一节点——并入那一条收寄并返回它的结果」的 HTTP 面（ADR-0199
// 决定四）：同一件实物换来源再来明确接收或明确拒收，都答 200 EXISTING_RESULT，响应体带的是并入的那一条收寄——收寄版本、关联、
// 控制依据照它，`recordedOutcome` 是它的种类、`currentVersion` 是它的版本；不带拒收原因，线上不另立一格。意图只有首投那一份。
func TestTheSameUnitAgainAnswersTheIntakeItWasMergedInto(t *testing.T) {
	refusal := defaultBody()
	refusal.Claim, refusal.Refusal, refusal.Evidence = "REFUSAL", "PACKAGING_UNSAFE", ""
	for name, again := range map[string]deviceBody{"明确接收": defaultBody(), "明确拒收": refusal} {
		t.Run(name, func(t *testing.T) {
			fixture := newAdapterFixture(t)
			first := decodeReception(t, fixture.post(t, defaultBody()))
			if first.Outcome != "INTAKE_FORMED" {
				t.Fatalf("首投 outcome = %q", first.Outcome)
			}
			again.SourceID = "device-scan-8"
			again.OccurredAt = "2026-09-05T09:30:00Z"
			response := fixture.post(t, again)
			merged := decodeReception(t, response)
			if response.Code != http.StatusOK || merged.Outcome != "EXISTING_RESULT" {
				t.Fatalf("同一件实物再来 %d %q, want 200 EXISTING_RESULT", response.Code, merged.Outcome)
			}
			if merged.IntakeVersion != first.IntakeVersion || merged.Association != first.Association || merged.ControlBasis != first.ControlBasis {
				t.Fatalf("响应体 = %+v, want 并入的那一条收寄 %+v", merged, first)
			}
			if merged.RecordedOutcome == nil || *merged.RecordedOutcome != "INTAKE_FORMED" ||
				merged.CurrentVersion == nil || *merged.CurrentVersion != first.IntakeVersion {
				t.Fatalf("recordedOutcome = %v currentVersion = %v, want INTAKE_FORMED %q", merged.RecordedOutcome, merged.CurrentVersion, first.IntakeVersion)
			}
			if merged.RefusalReason != "" {
				t.Fatalf("并入的答复带了拒收原因 %q——那是这次送站的来源，答复说的是并入的那一条", merged.RefusalReason)
			}
			stored := fixture.store.byKey[ports.ReceptionKey{TenantID: fixture.intake.tenant, SourceID: "device-scan-8"}]
			if stored.Kind != ports.RecordMergedIntoExistingIntake || stored.MergedInto.String() != first.IntakeVersion {
				t.Fatalf("在册 种类 = %s 并入版本 = %q, want 并入行指名 %q", stored.Kind, stored.MergedInto, first.IntakeVersion)
			}
			if len(fixture.handoff.intents) != 1 {
				t.Fatalf("意图 %d 份, want 只有首投那一份", len(fixture.handoff.intents))
			}
		})
	}
}
