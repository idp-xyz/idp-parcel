package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// 本文件证受理之后、不由 PS 信封触发的复核怎样取所携地理投影（ADR-0215 决定二）：按（租户，包裹）反查当前已接受委托，
// 读受理基线那一版提交的投影，只到地理维。

type parcelTargetDouble struct {
	target psdomain.CurrentAcceptedParcelTarget
	found  bool
	err    error
	asked  []psdomain.DeclaredParcelID
}

func (double *parcelTargetDouble) FindCurrentAcceptedByParcel(
	_ context.Context, _ psdomain.TenantID, parcel psdomain.DeclaredParcelID,
) (psdomain.CurrentAcceptedParcelTarget, bool, error) {
	double.asked = append(double.asked, parcel)
	return double.target, double.found, double.err
}

type countingRequestSource struct {
	request psdomain.ShipmentRequest
	found   bool
	err     error
	calls   int
}

func (double *countingRequestSource) FindBySourceIdentity(
	_ context.Context, _ psdomain.SourceIdentity,
) (psdomain.ShipmentRequest, bool, error) {
	double.calls++
	return double.request, double.found, double.err
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

func targetFor(t *testing.T, request psdomain.ShipmentRequest, requestID string) psdomain.CurrentAcceptedParcelTarget {
	t.Helper()
	identity, err := psdomain.NewSourceIdentity(
		value(t, psdomain.NewTenantID, "tenant-1"),
		value(t, psdomain.NewCustomerAccountID, "customer-1"),
		value(t, psdomain.NewSource, "source-a"),
		value(t, psdomain.NewSourceRequestKey, "key-1"),
	)
	if err != nil {
		t.Fatalf("identity: %v", err)
	}
	target, err := psdomain.NewCurrentAcceptedParcelTarget(identity,
		value(t, psdomain.NewShipmentRequestID, requestID), request.CurrentSubmissionVersion().VersionID())
	if err != nil {
		t.Fatalf("target: %v", err)
	}
	return target
}

func geoSourceOver(t *testing.T, targets *parcelTargetDouble, requests *countingRequestSource) *adapter.AcceptedCarriedGeoSource {
	t.Helper()
	source, err := adapter.NewAcceptedCarriedGeoSource(targets, requests)
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
// 同源），投影是基线那一版提交的地理维，摘要与按同一份取值组成的投影相同。
func TestAnAcceptedParcelAnswersItsJudgmentKeyAndTheBaselineVersionsGeo(t *testing.T) {
	request := acceptedShipmentRequestDeclaring(t, declaredCNToSG(t))
	targets := &parcelTargetDouble{target: targetFor(t, request, "request-1"), found: true}
	source := geoSourceOver(t, targets, &countingRequestSource{request: request, found: true})

	got, found, err := loadGeo(t, source, "parcel-1")
	if err != nil || !found {
		t.Fatalf("found=%v err=%v, want 当前已接受包裹", found, err)
	}
	if got.CustomerAccountID.String() != "customer-1" || got.ShipmentRequestID.String() != "request-1" ||
		got.AcceptanceBaseline.String() != "version-1" {
		t.Fatalf("判断键各维 = %+v", got)
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
	request := acceptedShipmentRequest(t)
	targets := &parcelTargetDouble{target: targetFor(t, request, "request-1"), found: true}
	got, found, err := loadGeo(t, geoSourceOver(t, targets, &countingRequestSource{request: request, found: true}), "parcel-1")
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

// Covers: AT-NR-066（取投影那一半）——没有当前已接受委托声明它，或声明它的委托的受理基线里没有它：都答「不是」，不是错误，
// 也不再读委托聚合。
func TestAnObjectThatIsNotACurrentAcceptedParcelAnswersNotFound(t *testing.T) {
	t.Run("反查不中", func(t *testing.T) {
		requests := &countingRequestSource{}
		_, found, err := loadGeo(t, geoSourceOver(t, &parcelTargetDouble{}, requests), "SYN-UNIT-01")
		if err != nil || found {
			t.Fatalf("found=%v err=%v, want 不是、无错", found, err)
		}
		if requests.calls != 0 {
			t.Fatalf("反查不中还读了 %d 次委托", requests.calls)
		}
	})
	t.Run("基线里没有它", func(t *testing.T) {
		request := acceptedShipmentRequest(t)
		targets := &parcelTargetDouble{target: targetFor(t, request, "request-1"), found: true}
		_, found, err := loadGeo(t, geoSourceOver(t, targets, &countingRequestSource{request: request, found: true}), "parcel-9")
		if err != nil || found {
			t.Fatalf("found=%v err=%v, want 不是、无错", found, err)
		}
	})
}

// Covers: AT-NR-067（取投影那一半）——PS 读不动答可重投的哨兵；反查歧义与聚合自相矛盾响亮上抛，不进那一格。
func TestAnUnreadableShipmentAnswerIsRetryableAndAContradictionIsLoud(t *testing.T) {
	request := acceptedShipmentRequest(t)
	hit := func() *parcelTargetDouble {
		return &parcelTargetDouble{target: targetFor(t, request, "request-1"), found: true}
	}
	cases := map[string]struct {
		targets   *parcelTargetDouble
		requests  *countingRequestSource
		retryable bool
		sentinel  error
	}{
		"反查读口报错":     {&parcelTargetDouble{err: errors.New("pool closed")}, &countingRequestSource{}, true, adapter.ErrAcceptedParcelUnreadable},
		"聚合读口报错":     {hit(), &countingRequestSource{err: errors.New("pool closed")}, true, adapter.ErrAcceptedParcelUnreadable},
		"反查命中而聚合读不回": {hit(), &countingRequestSource{}, true, adapter.ErrAcceptedParcelUnreadable},
		"反查歧义": {
			&parcelTargetDouble{err: psdomain.ErrAmbiguousParcelTarget}, &countingRequestSource{}, false, psdomain.ErrAmbiguousParcelTarget,
		},
		"反查与聚合指着两份委托": {
			&parcelTargetDouble{target: targetFor(t, request, "request-2"), found: true},
			&countingRequestSource{request: request, found: true}, false, adapter.ErrAcceptedRequestInconsistent,
		},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			_, found, err := loadGeo(t, geoSourceOver(t, tc.targets, tc.requests), "parcel-1")
			if found || !errors.Is(err, tc.sentinel) {
				t.Fatalf("found=%v err=%v, want %v", found, err, tc.sentinel)
			}
			if retryable := errors.Is(err, adapter.ErrAcceptedParcelUnreadable); retryable != tc.retryable {
				t.Fatalf("可重投 = %v, want %v：%v", retryable, tc.retryable, err)
			}
		})
	}
}
