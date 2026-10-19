package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// correctHandoverCommand 把 registerHandoverCommand 登下的那一版改判为拒收。
func correctHandoverCommand(t *testing.T, predecessor, newVersion string) application.CorrectTransportHandoverCommand {
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
		Verdict:            domain.HandoverRefused,
		Basis:              "late-correction-1",
		NewVersion:         newVersion,
		CorrectedAt:        handoverJudgedTime.Add(24 * time.Hour),
	}
}

// Covers: 票 operator-channel/41 交接更正那一半（ADR-0149 决定四「摘要作同身份重放与内容冲突的判据」）——同一新版本
// 原样重投答已有版本；换证据、规则、更正时刻、改判、换前版，或新版本撞上同对象同范围另一份首登，都答冲突，在册那一版不动。
func TestAHandoverCorrectionOnAnOccupiedVersionTellsReplayFromConflict(t *testing.T) {
	v1, v2 := "handover-result/parcel-1/v1", "handover-result/parcel-1/v2"
	other := "handover-result/parcel-1/other"
	variants := map[string]func(*application.CorrectTransportHandoverCommand){
		"another basis": func(command *application.CorrectTransportHandoverCommand) { command.Basis = "late-correction-2" },
		"another correction time": func(command *application.CorrectTransportHandoverCommand) {
			command.CorrectedAt = command.CorrectedAt.Add(time.Minute)
		},
		"another verdict": func(command *application.CorrectTransportHandoverCommand) {
			command.Verdict = domain.HandoverPendingConfirmation
		},
		"handed over with evidence": func(command *application.CorrectTransportHandoverCommand) {
			command.Verdict = domain.ObjectHandedOver
			command.ReleasingEvidence = "evidence-release-9"
			command.ReceivingEvidence = "evidence-receive-9"
			command.Rule = "handover-rule/v9"
			command.Basis = ""
		},
		"another predecessor": func(command *application.CorrectTransportHandoverCommand) { command.PredecessorVersion = other },
		"a first registration's version": func(command *application.CorrectTransportHandoverCommand) {
			command.PredecessorVersion = v2
			command.NewVersion = other
		},
	}
	for name, change := range variants {
		t.Run(name, func(t *testing.T) {
			fixture := newHandoverFixture(t)
			for _, version := range []string{v1, other} {
				first := registerHandoverCommand(t)
				first.Version = version
				result, err := fixture.handler.Register(context.Background(), first)
				wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
			}
			command := correctHandoverCommand(t, v1, v2)
			result, err := fixture.handler.Correct(context.Background(), command)
			wantOutcome(t, result.Outcome(), err, application.HandoverCorrected)

			replay, err := fixture.handler.Correct(context.Background(), command)
			wantOutcome(t, replay.Outcome(), err, application.HandoverExistingVersion)

			changed := command
			change(&changed)
			before := len(fixture.handoff.intents)
			conflict, err := fixture.handler.Correct(context.Background(), changed)
			wantOutcome(t, conflict.Outcome(), err, application.HandoverRegistrationConflict)
			if len(fixture.handoff.intents) != before {
				t.Fatal("冲突还交了意图")
			}
			if len(fixture.registry.records) != 3 {
				t.Fatalf("登记册里 %d 版，want 3（冲突不落新版、不顶替）", len(fixture.registry.records))
			}
		})
	}
}

// Covers: 票 operator-channel/41——撞键读回赢家之后照样比摘要：读时查无、落库时另一方已先登了同一键（并发输家），
// 首登与更正两口都按赢家的摘要分重放与冲突，不一律答已有版本。
func TestAHandoverThatLostTheRaceIsComparedWithTheWinner(t *testing.T) {
	winnerOf := func(t *testing.T, fixture *handoverFixture, register application.RegisterTransportHandoverCommand) func() {
		return func() {
			winner := newHandoverFixture(t)
			result, err := winner.handler.Register(context.Background(), register)
			wantOutcome(t, result.Outcome(), err, application.HandoverRegistered)
			record, _ := result.Record()
			fixture.registry.records[handoverRegistryKey(record.Key)] = record
		}
	}

	t.Run("first registration", func(t *testing.T) {
		for name, want := range map[string]application.HandoverRegistrationOutcome{
			"evidence-release-1": application.HandoverExistingVersion,
			"evidence-release-2": application.HandoverRegistrationConflict,
		} {
			t.Run(name, func(t *testing.T) {
				fixture := newHandoverFixture(t)
				winner := registerHandoverCommand(t)
				winner.ReleasingEvidence = name
				fixture.registry.beforeSave = winnerOf(t, fixture, winner)
				result, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
				wantOutcome(t, result.Outcome(), err, want)
			})
		}
	})

	t.Run("correction", func(t *testing.T) {
		fixture := newHandoverFixture(t)
		registered, err := fixture.handler.Register(context.Background(), registerHandoverCommand(t))
		wantOutcome(t, registered.Outcome(), err, application.HandoverRegistered)
		collider := registerHandoverCommand(t)
		collider.Version = "handover-result/parcel-1/v2"
		fixture.registry.beforeSave = winnerOf(t, fixture, collider)

		result, err := fixture.handler.Correct(context.Background(), correctHandoverCommand(t, "handover-result/parcel-1/v1", "handover-result/parcel-1/v2"))
		wantOutcome(t, result.Outcome(), err, application.HandoverRegistrationConflict)
	})
}

// handoverRecordAt 读回登记册替身里指名版本的那一行。
func handoverRecordAt(t *testing.T, fixture *handoverFixture, version string) ports.TransportHandoverRecord {
	t.Helper()
	for _, record := range fixture.registry.records {
		if record.Key.Version.String() == version {
			return record
		}
	}
	t.Fatalf("登记册里没有 %s", version)
	return ports.TransportHandoverRecord{}
}
