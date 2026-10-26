package domain_test

import (
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func baselineGeoSpec(t *testing.T) domain.AcceptanceBaselineGeoProjectionSpec {
	t.Helper()
	return domain.AcceptanceBaselineGeoProjectionSpec{
		ShipmentRequestID: mustValue(t, domain.NewShipmentRequestID, "request-1"),
		CustomerAccountID: mustValue(t, domain.NewCustomerAccountID, "customer-1"),
		BaselineVersion:   mustValue(t, domain.NewSubmissionVersionID, "version-1"),
		FixedAt:           time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC),
		Geo: domain.NewCarriedGeoProjection(
			domain.NewCarriedGeoSide("", false, "SYN-200000", true),
			domain.NewCarriedGeoSide("SYN-CC", true, "SYN-100115", true),
		),
	}
}

// 受理基线地理投影缺一格就不成一份：三格引用、固定时刻，以及「带了」的投影。答「不是」由读口的 found=false 表达，
// 不靠一份零值的投影。
func TestAnAcceptanceBaselineGeoProjectionNeedsEveryPart(t *testing.T) {
	built, err := domain.NewAcceptanceBaselineGeoProjection(baselineGeoSpec(t))
	if err != nil {
		t.Fatalf("完整的一份被拒：%v", err)
	}
	if built.BaselineVersion().String() != "version-1" || built.Geo() != baselineGeoSpec(t).Geo {
		t.Fatalf("读回 = %+v", built)
	}

	cases := map[string]func(*domain.AcceptanceBaselineGeoProjectionSpec){
		"缺委托": func(spec *domain.AcceptanceBaselineGeoProjectionSpec) {
			spec.ShipmentRequestID = domain.ShipmentRequestID{}
		},
		"缺客户账户": func(spec *domain.AcceptanceBaselineGeoProjectionSpec) {
			spec.CustomerAccountID = domain.CustomerAccountID{}
		},
		"缺基线版本": func(spec *domain.AcceptanceBaselineGeoProjectionSpec) {
			spec.BaselineVersion = domain.SubmissionVersionID{}
		},
		"固定时刻为零": func(spec *domain.AcceptanceBaselineGeoProjectionSpec) { spec.FixedAt = time.Time{} },
		"投影没带":   func(spec *domain.AcceptanceBaselineGeoProjectionSpec) { spec.Geo = domain.CarriedGeoProjection{} },
	}
	for name, mutate := range cases {
		t.Run(name, func(t *testing.T) {
			spec := baselineGeoSpec(t)
			mutate(&spec)
			if _, err := domain.NewAcceptanceBaselineGeoProjection(spec); !errors.Is(err, domain.ErrInvalidAcceptanceBaselineGeoProjection) {
				t.Fatalf("err = %v, want ErrInvalidAcceptanceBaselineGeoProjection", err)
			}
		})
	}
}
