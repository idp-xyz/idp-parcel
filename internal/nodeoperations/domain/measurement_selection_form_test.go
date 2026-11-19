package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// Covers: 形态词表外的词被拒，拒绝说的是形态词不对，不冒充成「实际测量报不全」；内置那一种按词名来回译得回。
func TestAMeasurementSelectionFormOutsideTheVocabularyIsRefusedByName(t *testing.T) {
	form, err := domain.MeasurementSelectionFormFromName(domain.LatestStillValidByOccurredAt.String())
	if err != nil || form != domain.LatestStillValidByOccurredAt {
		t.Fatalf("内置形态 = %v，err = %v，want 译得回", form, err)
	}
	for _, name := range []string{"", "latest_still_valid_by_occurred_at", "EARLIEST_STILL_VALID_BY_OCCURRED_AT"} {
		form, err := domain.MeasurementSelectionFormFromName(name)
		if !errors.Is(err, domain.ErrInvalidMeasurementSelectionForm) || errors.Is(err, domain.ErrInvalidActualMeasurement) || form != domain.MeasurementSelectionFormInvalid {
			t.Fatalf("词 %q：form = %v，err = %v，want 形态词不对的那个错", name, form, err)
		}
	}
}
