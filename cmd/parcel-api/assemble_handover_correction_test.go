package main

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 票 operator-channel/41 交接更正那一半在生产装配的交接编排与真库上成立——同一新版本原样重投答已有版本，
// 换内容答冲突且在册那一版不动。测试输入是隔离合成，只记 `S`。
func TestTheWiredHandoverCorrectionTellsReplayFromConflictAgainstARealDatabase(t *testing.T) {
	orchestrations, pool := wiredHandoverOrchestrations(t)
	registerPlainHandover(t, orchestrations, "SYN-PARCEL-41-H", "SYN-HANDOVER-41/v1", "SYN-EVIDENCE-RELEASE-1")
	command := refusingHandoverCorrection(t, "SYN-PARCEL-41-H", "SYN-HANDOVER-41/v1", "SYN-HANDOVER-41/v2", "SYN-BASIS-41-A")

	first, err := orchestrations.handover.Correct(t.Context(), command)
	if err != nil || first.Outcome() != tfapp.HandoverCorrected {
		t.Fatalf("更正 outcome = %v err = %v, want HANDOVER_CORRECTED", first.Outcome(), err)
	}
	replay, err := orchestrations.handover.Correct(t.Context(), command)
	if err != nil || replay.Outcome() != tfapp.HandoverExistingVersion {
		t.Fatalf("原样重投 outcome = %v err = %v, want EXISTING_VERSION", replay.Outcome(), err)
	}
	changed := command
	changed.Basis = "SYN-BASIS-41-B"
	conflict, err := orchestrations.handover.Correct(t.Context(), changed)
	if err != nil || conflict.Outcome() != tfapp.HandoverRegistrationConflict {
		t.Fatalf("同一新版本换依据 outcome = %v err = %v, want SOURCE_CONFLICT", conflict.Outcome(), err)
	}
	var basis string
	if err := pool.QueryRow(t.Context(),
		`SELECT basis_ref FROM transport_fulfillment.transport_handover
		  WHERE tenant_id = 'SYN-TENANT-1' AND object_ref = 'SYN-PARCEL-41-H' AND handover_version = 'SYN-HANDOVER-41/v2'`).Scan(&basis); err != nil {
		t.Fatalf("读回 v2：%v", err)
	}
	if basis != "SYN-BASIS-41-A" {
		t.Fatalf("在册 v2 依据 = %q，want SYN-BASIS-41-A（冲突不顶替）", basis)
	}
}

// Covers: 票 operator-channel/41 附录 F-7「交接首登并发输家同版本异内容也答已有版本」——同一版本、异放行证据的两份首登
// 并发，多轮里每一轮恰好一份登上，另一份答冲突。
func TestConcurrentHandoverRegistrationsOfOneVersionWithDifferentContentConflictAgainstARealDatabase(t *testing.T) {
	orchestrations, _ := wiredHandoverOrchestrations(t)
	for round := range 6 {
		object := fmt.Sprintf("SYN-PARCEL-41-HR%d", round)
		outcomes := make([]tfapp.HandoverRegistrationOutcome, 2)
		var wait sync.WaitGroup
		for racer := range outcomes {
			wait.Add(1)
			go func() {
				defer wait.Done()
				command := plainHandoverCommand(t, object, "SYN-HANDOVER-41/v1", fmt.Sprintf("SYN-EVIDENCE-RELEASE-RACE-%d", racer))
				result, err := orchestrations.handover.Register(context.Background(), command)
				if err != nil {
					t.Errorf("%s 第 %d 份首登：%v", object, racer, err)
					return
				}
				outcomes[racer] = result.Outcome()
			}()
		}
		wait.Wait()
		registered, conflicts := 0, 0
		for _, outcome := range outcomes {
			switch outcome {
			case tfapp.HandoverRegistered:
				registered++
			case tfapp.HandoverRegistrationConflict:
				conflicts++
			}
		}
		if registered != 1 || conflicts != 1 {
			t.Fatalf("%s 两份并发首登答 %v，want 恰好一份 HANDOVER_REGISTERED、一份 SOURCE_CONFLICT", object, outcomes)
		}
	}
}

func wiredHandoverOrchestrations(t *testing.T) (controlFactOrchestrations, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	orchestrations, err := buildControlFactOrchestrations(db)
	if err != nil {
		t.Fatalf("装配控制事实编排：%v", err)
	}
	return orchestrations, pool
}

// plainHandoverCommand 是一份不带段引用的`已交接`首登：本文件只看交接登记册的代数，不看进段。
func plainHandoverCommand(t *testing.T, object, version, releasingEvidence string) tfapp.RegisterTransportHandoverCommand {
	command := registerHandoverCommand(t, object, "", "")
	command.Version = version
	command.ReleasingEvidence = releasingEvidence
	return command
}

func registerPlainHandover(t *testing.T, orchestrations controlFactOrchestrations, object, version, releasingEvidence string) {
	t.Helper()
	result, err := orchestrations.handover.Register(t.Context(), plainHandoverCommand(t, object, version, releasingEvidence))
	if err != nil || result.Outcome() != tfapp.HandoverRegistered {
		t.Fatalf("首登 %s outcome = %v err = %v", object, result.Outcome(), err)
	}
}

func refusingHandoverCorrection(t *testing.T, object, predecessor, newVersion, basis string) tfapp.CorrectTransportHandoverCommand {
	t.Helper()
	return tfapp.CorrectTransportHandoverCommand{
		TenantID:           mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Object:             object,
		Scope:              "SYN-HANDOVER-SCOPE-1",
		PredecessorVersion: predecessor,
		Verdict:            tfdomain.HandoverRefused,
		Basis:              basis,
		NewVersion:         newVersion,
		CorrectedAt:        controlFactJudgedAt.Add(2 * time.Hour),
	}
}
