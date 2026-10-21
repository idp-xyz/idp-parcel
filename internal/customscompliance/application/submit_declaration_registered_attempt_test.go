package application_test

import (
	"context"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

// Covers: AT-CC-441（编排半边）——首次尝试可以记成`已登记待发送`：不带发送时间就成版、交出意图，尝试读回仍是待发送；发送时间
// 与结果错配（待发送带时间、已发出没时间）在受理处就拒，不落单元也不签版本号。
func TestASubmissionCanRegisterItsFirstAttemptWithoutSendingIt(t *testing.T) {
	fixture := newDeclarationFixture(t)
	command := declarationCommand(t)
	command.InitialResult = domain.AttemptRegisteredPendingSend
	command.SentAt = time.Time{}

	result, err := fixture.handler.Handle(context.Background(), command)
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	record, present := result.Record()
	if result.Outcome() != application.DeclarationSubmitted || !present {
		t.Fatalf("outcome = %s", result.Outcome())
	}
	if record.Attempt.Result() != domain.AttemptRegisteredPendingSend || !record.Attempt.SentAt().IsZero() {
		t.Fatalf("attempt = %s @ %s", record.Attempt.Result(), record.Attempt.SentAt())
	}
	if len(fixture.handoff.intents) != 1 {
		t.Fatalf("intents = %d", len(fixture.handoff.intents))
	}

	for _, mismatched := range []application.SubmitDeclarationCommand{
		func() application.SubmitDeclarationCommand {
			stamped := declarationCommand(t)
			stamped.UnitID = "declaration-unit-2"
			stamped.InitialResult = domain.AttemptRegisteredPendingSend
			return stamped
		}(),
		func() application.SubmitDeclarationCommand {
			unstamped := declarationCommand(t)
			unstamped.UnitID = "declaration-unit-3"
			unstamped.SentAt = time.Time{}
			return unstamped
		}(),
	} {
		refused, err := fixture.handler.Handle(context.Background(), mismatched)
		if err != nil || refused.Outcome() != application.DeclarationNotAccepted {
			t.Fatalf("%s @ %s: outcome = %s err = %v", mismatched.InitialResult, mismatched.SentAt, refused.Outcome(), err)
		}
	}
	if fixture.versions.minted != 1 || len(fixture.units.units) != 1 {
		t.Fatalf("minted = %d units = %d；错配的请求落了单元或签了版本号", fixture.versions.minted, len(fixture.units.units))
	}
}
