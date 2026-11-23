package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

func TestPieceMeasurementOriginNamesEachSource(t *testing.T) {
	if got := domain.PieceMeasurementOrigin("").String(); got != "" {
		t.Fatalf("空来源 = %q", got)
	}
	for _, origin := range []domain.PieceMeasurementOrigin{domain.PieceMeasurementMeasured, domain.PieceMeasurementDeclared} {
		if origin.String() == "" {
			t.Fatalf("来源 %q 没有词", origin)
		}
	}
}
