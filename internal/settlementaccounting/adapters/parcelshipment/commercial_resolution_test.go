package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/parcelshipment"
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
)

// 钉「包裹 → 结算账户」链的第一段（ADR-0190 决定三）：小包托运的三格原样带过来——回指逐字、没有就是没有、读坏了
// 是 error；答了「有」却给空回指是提供方违约，不折成「没有」。

type resolutionReferenceDouble struct {
	reference psdomain.CommercialResolutionID
	present   bool
	err       error
	asked     psdomain.DeclaredParcelID
}

func (double *resolutionReferenceDouble) LoadCommercialResolutionReference(
	_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CommercialResolutionID, bool, error) {
	double.asked = parcel
	return double.reference, double.present, double.err
}

func TestTheParcelCommercialResolutionCarriesThePSAnswerAcross(t *testing.T) {
	tenant := must(t, sadomain.NewTenantID, "SYN-TENANT-1")
	parcel := must(t, sadomain.NewParcelReference, "SYN-PARCEL-1")
	load := func(t *testing.T, double *resolutionReferenceDouble) (sadomain.CommercialResolutionReference, bool, error) {
		t.Helper()
		view, err := adapter.NewParcelCommercialResolutions(double)
		if err != nil {
			t.Fatalf("构造：%v", err)
		}
		return view.LoadParcelCommercialResolution(t.Context(), tenant, parcel)
	}

	t.Run("回指", func(t *testing.T) {
		double := &resolutionReferenceDouble{reference: must(t, psdomain.NewCommercialResolutionID, "SYN-RES-1"), present: true}
		reference, found, err := load(t, double)
		if err != nil || !found || reference.String() != "SYN-RES-1" || double.asked.String() != "SYN-PARCEL-1" {
			t.Fatalf("reference = %q found = %v err = %v asked = %q", reference, found, err, double.asked)
		}
	})
	t.Run("没有", func(t *testing.T) {
		if _, found, err := load(t, &resolutionReferenceDouble{}); err != nil || found {
			t.Fatalf("found = %v err = %v, want 没有且不报错", found, err)
		}
	})
	t.Run("读坏了", func(t *testing.T) {
		broken := errors.New("syn: read down")
		if _, found, err := load(t, &resolutionReferenceDouble{err: broken}); !errors.Is(err, broken) || found {
			t.Fatalf("found = %v err = %v, want 原样上抛", found, err)
		}
	})
	t.Run("有却是空回指", func(t *testing.T) {
		if _, found, err := load(t, &resolutionReferenceDouble{present: true}); !errors.Is(err, adapter.ErrUntranslatableAnswer) || found {
			t.Fatalf("found = %v err = %v, want ErrUntranslatableAnswer", found, err)
		}
	})
}

func must[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

func TestTheParcelCommercialResolutionViewRefusesAMissingSource(t *testing.T) {
	if _, err := adapter.NewParcelCommercialResolutions(nil); err == nil {
		t.Fatal("缺小包托运读口仍装配成功")
	}
}
