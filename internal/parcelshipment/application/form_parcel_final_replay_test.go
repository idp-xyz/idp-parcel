package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证终局采用把依据一份取消的不采用记录连同回放原料一起落库（ADR-0295 决定三、四）：取消日后被认定不成立时，作废这一行、凭原料把来源重新交回本
// 编排。依据别的东西（已有终局）的不采用行、已形成的终局不带。

// Covers: AT-PS-169, AT-PS-173；依据取消的不采用记录带回放原料（责任结果规格原样、指名那份取消）。
func TestADeliveryRefusedOverACancellationCarriesItsReplayMaterial(t *testing.T) {
	fixture := newFinalFixture(t)
	fixture.cancellations.cancelled = true
	fixture.cancellations.cancellation = cancelledAt(t, finalOccurredAt.Add(-2*time.Hour))
	fixture.cancellations.onlyFor = mustValue(t, domain.NewDeclaredParcelID, "parcel-1")
	command := finalCommand(t, "parcel-1")

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	record, held := result.Record()
	if !held || record.Finalized || record.CancellationReplay == nil {
		t.Fatalf("record = %+v held = %v; 依据取消的不采用记录必须带回放原料", record, held)
	}
	replay := record.CancellationReplay
	if replay.Cancellation.String() != "cancel-1" || replay.Final == nil || replay.Intake != nil || *replay.Final != command.Outcome {
		t.Fatalf("回放原料 = %+v, want 指名 cancel-1 且带命令里那份责任结果规格原样", replay)
	}
}

// Covers: AT-PS-173；不是依据取消的不采用行（已有终局）与已形成的终局都不带回放原料：带上就是让作废或重驱去碰不该碰的东西。
func TestOnlyARefusalGroundedOnACancellationCarriesReplayMaterial(t *testing.T) {
	fixture := newFinalFixture(t)
	formed, err := fixture.handler.Handle(context.Background(), finalCommand(t, "parcel-1"))
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if record, held := formed.Record(); !held || !record.Finalized || record.CancellationReplay != nil {
		t.Fatalf("形成的终局 = %+v held = %v, want 终局且不带原料", record, held)
	}

	other := finalCommand(t, "parcel-1")
	other.Outcome.Kind = domain.ReturnCompletedOutcome
	other.Outcome.Version = mustValue(t, domain.NewResponsibilityOutcomeVersion, "return-result/v1")
	refused, err := fixture.handler.Handle(context.Background(), other)
	if err != nil {
		t.Fatalf("handle other: %v", err)
	}
	record, held := refused.Record()
	if refused.Outcome() != application.FinalSourceNotAdopted || !held || record.CancellationReplay != nil {
		t.Fatalf("outcome = %q record = %+v, want 依据已有终局的不采用行且不带原料", refused.Outcome(), record)
	}
}
