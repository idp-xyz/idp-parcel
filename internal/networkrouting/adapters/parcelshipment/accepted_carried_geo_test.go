package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证受理之后、不由 PS 信封触发的复核怎样取所携地理投影（ADR-0215 决定二，经 ADR-0301 改走 PS 的受理基线地理投影
// 读口）：按（租户，包裹）问 PS 读口，译成判断键各维、接受时刻与地理维。PS 读口自己的答格（基线成员、基线那一版、自相
// 矛盾的几格）由 PS 真库用例钉，这里钉的是本适配器怎样译、怎样把 PS 的答格分进重投与响亮失败。

// baselineGeoViewDouble 是 PS 受理基线地理投影读口的替身，记下被问的包裹。
type baselineGeoViewDouble struct {
	projection psdomain.AcceptanceBaselineGeoProjection
	found      bool
	err        error
	asked      []psdomain.DeclaredParcelID
}

func (double *baselineGeoViewDouble) LoadAcceptanceBaselineGeoProjection(
	_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.AcceptanceBaselineGeoProjection, bool, error) {
	double.asked = append(double.asked, parcel)
	return double.projection, double.found, double.err
}

// declaredCNToSG 是两端各报国家与邮编的地址要素，随首个提交版本申报，受理基线就钉在那一版上。
func declaredCNToSG(t *testing.T) psdomain.DeclaredAddressElements {
	t.Helper()
	entry := func(group psdomain.SourceDataGroupReference, element psdomain.AddressElementName, raw string) psdomain.CanonicalContentEntry {
		built, err := psdomain.NewCanonicalContentEntry(psdomain.AddressElementEntryName(group, element), raw)
		if err != nil {
			t.Fatalf("entry: %v", err)
		}
		return built
	}
	sender := psdomain.SenderPlaceDataGroup()
	delivery := psdomain.DeliveryPlaceDataGroup()
	return psdomain.NewDeclaredAddressElements(
		psdomain.AddressElementsOf(sender, []psdomain.CanonicalContentEntry{
			entry(sender, psdomain.CountryCodeElement, "CN"),
			entry(sender, psdomain.PostalCodeElement, "200000"),
		}),
		psdomain.AddressElementsOf(delivery, []psdomain.CanonicalContentEntry{
			entry(delivery, psdomain.CountryCodeElement, "SG"),
			entry(delivery, psdomain.PostalCodeElement, "018956"),
		}),
	)
}

// baselineGeoOf 按 PS 读口的定义从一份真接受的委托上取受理基线地理投影：基线所指那一版、基线固定的时刻、那一版的投影。
func baselineGeoOf(t *testing.T, request psdomain.ShipmentRequest) psdomain.AcceptanceBaselineGeoProjection {
	t.Helper()
	baseline, present := request.AcceptanceBaseline()
	if !present {
		t.Fatal("夹具委托没有受理基线")
	}
	submission, present := request.SubmissionVersionByID(baseline.SubmissionVersionID())
	if !present {
		t.Fatal("夹具委托找不到基线那一版")
	}
	projection, err := psdomain.NewAcceptanceBaselineGeoProjection(psdomain.AcceptanceBaselineGeoProjectionSpec{
		ShipmentRequestID: request.ShipmentRequestID(),
		CustomerAccountID: value(t, psdomain.NewCustomerAccountID, "customer-1"),
		BaselineVersion:   baseline.SubmissionVersionID(),
		FixedAt:           baseline.FixedAt(),
		Geo:               submission.GeoProjection(),
	})
	if err != nil {
		t.Fatalf("受理基线地理投影：%v", err)
	}
	return projection
}

func geoSourceOver(t *testing.T, view *baselineGeoViewDouble) *adapter.AcceptedCarriedGeoSource {
	t.Helper()
	source, err := adapter.NewAcceptedCarriedGeoSource(view)
	if err != nil {
		t.Fatalf("construct: %v", err)
	}
	return source
}

func loadGeo(t *testing.T, source *adapter.AcceptedCarriedGeoSource, parcel string) (adapter.AcceptedCarriedGeo, bool, error) {
	t.Helper()
	return source.LoadAcceptedCarriedGeo(context.Background(),
		value(t, nrdomain.NewTenantID, "tenant-1"), value(t, nrdomain.NewDeclaredParcelID, parcel))
}

// Covers: AT-NR-064（取投影那一半）——当前已接受包裹交回判断键各维，基线引用是基线的提交版本标识（与初始路由、收寄复核
// 同源），投影是基线那一版提交的地理维，摘要与按同一份取值组成的投影相同。AT-NR-087（取接受时刻那一半）：接受时刻是接受基线
// 固定的那一刻，与初始路由交接带来的同源。
func TestAnAcceptedParcelAnswersItsJudgmentKeyAndTheBaselineVersionsGeo(t *testing.T) {
	request := acceptedShipmentRequestDeclaring(t, declaredCNToSG(t))
	view := &baselineGeoViewDouble{projection: baselineGeoOf(t, request), found: true}

	got, found, err := loadGeo(t, geoSourceOver(t, view), "parcel-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v, want 当前已接受包裹", found, err)
	}
	if len(view.asked) != 1 || view.asked[0].String() != "parcel-1" {
		t.Fatalf("问 PS 读口的包裹 = %v", view.asked)
	}
	if got.CustomerAccountID.String() != "customer-1" || got.ShipmentRequestID.String() != "request-1" ||
		got.AcceptanceBaseline.String() != "version-1" {
		t.Fatalf("判断键各维 = %+v", got)
	}
	if baseline, _ := request.AcceptanceBaseline(); got.AcceptedAt.IsZero() || !got.AcceptedAt.Equal(baseline.FixedAt()) {
		t.Fatalf("接受时刻 = %s, want 接受基线固定的 %s", got.AcceptedAt, baseline.FixedAt())
	}
	want := nrdomain.NewGeoResolutionProjection(
		nrdomain.NewGeoResolutionSide("CN", true, "200000", true),
		nrdomain.NewGeoResolutionSide("SG", true, "018956", true),
	)
	if got.Geo.ContentDigest() != want.ContentDigest() {
		t.Fatalf("投影摘要 = %s, want 基线那一版的 %s", got.Geo.ContentDigest(), want.ContentDigest())
	}
}

// 基线那一版一格地址要素都没报，投影照样是「带了」、各格缺席：与 PS 信封交出的同一形状，不是「没带」。
func TestABaselineWithoutAddressElementsStillCarriesAnEmptyProjection(t *testing.T) {
	view := &baselineGeoViewDouble{projection: baselineGeoOf(t, acceptedShipmentRequest(t)), found: true}
	got, found, err := loadGeo(t, geoSourceOver(t, view), "parcel-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v", found, err)
	}
	if !got.Geo.Carried() {
		t.Fatal("没报地址要素被读成了「没带投影」")
	}
	if _, has := got.Geo.Delivery().Country(); has {
		t.Fatal("没报的收件国家冒了出来")
	}
}

// Covers: AT-NR-066（取投影那一半）——PS 读口答「不是」（没有当前已接受委托声明它，或受理基线里没有它）原样答「不是」，
// 不是错误。
func TestAnObjectThatIsNotACurrentAcceptedParcelAnswersNotFound(t *testing.T) {
	got, found, err := loadGeo(t, geoSourceOver(t, &baselineGeoViewDouble{}), "SYN-UNIT-01")
	if err != nil || found {
		t.Fatalf("found=%v err=%v, want 不是、无错", found, err)
	}
	if got != (adapter.AcceptedCarriedGeo{}) {
		t.Fatalf("答「不是」还交回了 %+v", got)
	}
}

// Covers: AT-NR-067（取投影那一半）——PS 读不动答可重投的哨兵；反查歧义与受理基线自相矛盾响亮上抛，不进那一格。
func TestAnUnreadableShipmentAnswerIsRetryableAndAContradictionIsLoud(t *testing.T) {
	cases := map[string]struct {
		err       error
		retryable bool
		sentinel  error
	}{
		"读口报错":     {errors.New("pool closed"), true, adapter.ErrAcceptedParcelUnreadable},
		"反查歧义":     {psdomain.ErrAmbiguousParcelTarget, false, psdomain.ErrAmbiguousParcelTarget},
		"受理基线自相矛盾": {psdomain.ErrAcceptanceBaselineInconsistent, false, adapter.ErrAcceptedRequestInconsistent},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, found, err := loadGeo(t, geoSourceOver(t, &baselineGeoViewDouble{err: tc.err}), "parcel-1")
			if found || !errors.Is(err, tc.sentinel) {
				t.Fatalf("found=%v err=%v, want %v", found, err, tc.sentinel)
			}
			if retryable := errors.Is(err, adapter.ErrAcceptedParcelUnreadable); retryable != tc.retryable {
				t.Fatalf("可重投 = %v, want %v：%v", retryable, tc.retryable, err)
			}
		})
	}
}

func TestTheGeoSourceNeedsThePSView(t *testing.T) {
	if _, err := adapter.NewAcceptedCarriedGeoSource(nil); err == nil {
		t.Fatal("nil 读口应在构造期被拒")
	}
}
