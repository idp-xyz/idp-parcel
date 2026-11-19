package nodeoperations_test

import (
	"context"
	"errors"
	"testing"

	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	sainbox "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/inbox"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/nodeoperations"
)

// parcelAssociationDouble 按作业实物答它此刻关联着的包裹，并记下被问过几次。
type parcelAssociationDouble struct {
	parcels []string
	err     error
	asked   int
	unit    string
}

func (double *parcelAssociationDouble) ListHandlingUnits(
	context.Context, nodomain.TenantID, nodomain.ParcelAssociationReference,
) ([]nodomain.HandlingUnitID, error) {
	return nil, errors.New("本用例不按包裹列作业实物")
}

func (double *parcelAssociationDouble) ListParcels(
	_ context.Context, _ nodomain.TenantID, unit nodomain.HandlingUnitID,
) ([]nodomain.ParcelAssociationReference, error) {
	double.asked++
	double.unit = unit.String()
	if double.err != nil {
		return nil, double.err
	}
	references := make([]nodomain.ParcelAssociationReference, 0, len(double.parcels))
	for _, raw := range double.parcels {
		reference, err := nodomain.NewParcelAssociationReference(raw)
		if err != nil {
			return nil, err
		}
		references = append(references, reference)
	}
	return references, nil
}

func handlingUnitChange() sainbox.ChangedMeasurement {
	return sainbox.ChangedMeasurement{
		TenantID:   "SYN-TENANT-01",
		SourceID:   "measure-1",
		ObjectKind: nodomain.HandlingUnitObject.String(),
		ObjectID:   "SYN-HU-1",
	}
}

// Covers: 一次测量变化只在恰好对上一件包裹时解出那一件；集运单元上的测量、零件与多件包裹都对不上，答 false 而不是错误。
// 集运单元上的测量连关联都不去问。
func TestMeasuredParcelResolvesOnlyAnUnambiguousSingleParcel(t *testing.T) {
	consolidation := handlingUnitChange()
	consolidation.ObjectKind = nodomain.ConsolidationUnitObject.String()

	cases := []struct {
		name       string
		changed    sainbox.ChangedMeasurement
		parcels    []string
		wantParcel string
		wantFound  bool
		wantAsked  int
	}{
		{"集运单元上的测量", consolidation, []string{"SYN-PARCEL-1"}, "", false, 0},
		{"作业实物没有关联包裹", handlingUnitChange(), nil, "", false, 1},
		{"作业实物关联着一件包裹", handlingUnitChange(), []string{"SYN-PARCEL-1"}, "SYN-PARCEL-1", true, 1},
		{"作业实物关联着多件包裹", handlingUnitChange(), []string{"SYN-PARCEL-1", "SYN-PARCEL-2"}, "", false, 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			double := &parcelAssociationDouble{parcels: tc.parcels}
			parcel, found, err := nodeoperations.MeasuredParcel(context.Background(), double, tc.changed)
			if err != nil || found != tc.wantFound || parcel.String() != tc.wantParcel || double.asked != tc.wantAsked {
				t.Fatalf("包裹 %q found=%t 问了 %d 次 err=%v，want %q found=%t 问了 %d 次",
					parcel.String(), found, double.asked, err, tc.wantParcel, tc.wantFound, tc.wantAsked)
			}
			if tc.wantAsked > 0 && double.unit != "SYN-HU-1" {
				t.Fatalf("问的作业实物 = %q，want SYN-HU-1", double.unit)
			}
		})
	}
}

// Covers: 租户或实物标识写不成合法值、关联读不回，都是错误交回而不是当成对不上——对不上是「这次不另起请求」，
// 错误是「这封事件该重投」。
func TestMeasuredParcelReturnsTheErrorOfAnUnreadableChange(t *testing.T) {
	blankTenant := handlingUnitChange()
	blankTenant.TenantID = " "
	blankUnit := handlingUnitChange()
	blankUnit.ObjectID = ""
	readFailure := errors.New("关联读不回")

	t.Run("租户为空", func(t *testing.T) {
		double := &parcelAssociationDouble{parcels: []string{"SYN-PARCEL-1"}}
		if _, found, err := nodeoperations.MeasuredParcel(context.Background(), double, blankTenant); err == nil || found || double.asked != 0 {
			t.Fatalf("found=%t asked=%d err=%v，want 错误且没去读关联", found, double.asked, err)
		}
	})
	t.Run("实物标识为空", func(t *testing.T) {
		double := &parcelAssociationDouble{parcels: []string{"SYN-PARCEL-1"}}
		if _, found, err := nodeoperations.MeasuredParcel(context.Background(), double, blankUnit); err == nil || found || double.asked != 0 {
			t.Fatalf("found=%t asked=%d err=%v，want 错误且没去读关联", found, double.asked, err)
		}
	})
	t.Run("关联读不回", func(t *testing.T) {
		double := &parcelAssociationDouble{err: readFailure}
		if _, found, err := nodeoperations.MeasuredParcel(context.Background(), double, handlingUnitChange()); !errors.Is(err, readFailure) || found {
			t.Fatalf("found=%t err=%v，want 原样交回读不回的错误", found, err)
		}
	})
	t.Run("集运单元上的测量不看租户与标识", func(t *testing.T) {
		consolidation := blankTenant
		consolidation.ObjectKind = nodomain.ConsolidationUnitObject.String()
		double := &parcelAssociationDouble{}
		if _, found, err := nodeoperations.MeasuredParcel(context.Background(), double, consolidation); err != nil || found {
			t.Fatalf("found=%t err=%v，want 直接对不上", found, err)
		}
	})
}
