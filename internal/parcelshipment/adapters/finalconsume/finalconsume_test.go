package finalconsume_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/finalconsume"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
)

func TestAnUnmappedFinalOutcomeIsNotSilentlyConsumed(t *testing.T) {
	err := finalconsume.Consumption(psapplication.FormParcelFinalResult{})
	if !errors.Is(err, finalconsume.ErrUnexpectedFinalOutcome) {
		t.Fatalf("err = %v, want ErrUnexpectedFinalOutcome", err)
	}
}
