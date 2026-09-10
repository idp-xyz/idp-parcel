package parcelshipment_test

import (
	"context"
	"errors"
	"testing"

	psdomain "go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	adapter "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/parcelshipment"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	tfports "go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件钉住 DeliveryPlaceSource 对 PS 读口四格的全函数翻译（票 tf-segment-lifecycle-closure/12 完成判据 1）：
// 源侧四格各一例 + 读口 error 原样上抛 + 集外取值上抛 ErrUntranslatableAnswer。地点串断言与 PS 的 String() 逐字
// 相等——TF 侧不重拼、不改一字（ADR-0130 决定一「TF 只当不透明串」）。

// referenceLookupStub 是 PS 读口的替身：记下被问的键，按预设答一格或一个错。
type referenceLookupStub struct {
	resolution psdomain.DeliveryPlaceResolution
	err        error
	tenant     string
	parcel     string
	calls      int
}

func (stub *referenceLookupStub) LoadDeliveryPlaceReference(
	_ context.Context,
	tenant psdomain.TenantID,
	parcel psdomain.DeclaredParcelID,
) (psdomain.DeliveryPlaceResolution, error) {
	stub.calls++
	stub.tenant, stub.parcel = tenant.String(), parcel.String()
	if stub.err != nil {
		return psdomain.DeliveryPlaceResolution{}, stub.err
	}
	return stub.resolution, nil
}

func mustPS[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("construct %q: %v", raw, err)
	}
	return value
}

// deliveryPlaceReference 造一份 PS 的收件地点引用：委托级收件范围，锚由调用方给。
func deliveryPlaceReference(t *testing.T, anchor psdomain.SourceDataVersionAnchor) psdomain.DeliveryPlaceReference {
	t.Helper()
	scope, err := psdomain.NewShipmentScopedSourceData(
		mustPS(t, psdomain.NewShipmentRequestID, "request-1"), psdomain.DeliveryPlaceDataGroup(),
	)
	if err != nil {
		t.Fatalf("scope: %v", err)
	}
	reference, err := psdomain.NewDeliveryPlaceReference(mustPS(t, psdomain.NewTenantID, "tenant-1"), scope, anchor)
	if err != nil {
		t.Fatalf("reference: %v", err)
	}
	return reference
}

func referencedResolution(t *testing.T, reference psdomain.DeliveryPlaceReference) psdomain.DeliveryPlaceResolution {
	t.Helper()
	resolution, err := psdomain.DeliveryPlaceReferenced(reference)
	if err != nil {
		t.Fatalf("referenced resolution: %v", err)
	}
	return resolution
}

func loadThrough(t *testing.T, stub *referenceLookupStub, object string) (string, tfports.RequirementResolution, error) {
	t.Helper()
	source, err := adapter.NewDeliveryPlaceSource(stub)
	if err != nil {
		t.Fatalf("new delivery place source: %v", err)
	}
	return source.LoadDeliveryPlace(t.Context(),
		mustPS(t, tfdomain.NewTenantID, "tenant-1"), mustPS(t, tfdomain.NewCarriedObjectReference, object))
}

var _ tfports.DeliveryPlaceSource = (*adapter.DeliveryPlaceSource)(nil)

// Covers: 源侧「基线锚引用」与「已采用版本锚引用」两格 → RequirementResolved + 逐字同串；键的翻译是同一串字面
// （租户、包裹身份在两个上下文里是同一个词）。
func TestAnAnchoredReferenceIsHandedOverVerbatimAsResolved(t *testing.T) {
	versionAnchor, err := psdomain.NewSourceDataVersionAnchor(mustPS(t, psdomain.NewSourceDataVersionID, "dp-ver-2"))
	if err != nil {
		t.Fatalf("version anchor: %v", err)
	}
	cases := map[string]psdomain.DeliveryPlaceReference{
		"anchored on the acceptance baseline": deliveryPlaceReference(t, psdomain.NewAcceptanceBaselineAnchor()),
		"anchored on the adopted version":     deliveryPlaceReference(t, versionAnchor),
	}
	for name, reference := range cases {
		t.Run(name, func(t *testing.T) {
			stub := &referenceLookupStub{resolution: referencedResolution(t, reference)}
			place, resolution, err := loadThrough(t, stub, "parcel-1")
			if err != nil {
				t.Fatalf("load delivery place: %v", err)
			}
			if resolution != tfports.RequirementResolved {
				t.Fatalf("resolution = %q, want RESOLVED", resolution)
			}
			if place != reference.String() {
				t.Fatalf("place = %q, want PS String() verbatim %q", place, reference.String())
			}
			if stub.calls != 1 || stub.tenant != "tenant-1" || stub.parcel != "parcel-1" {
				t.Fatalf("PS asked %d times with tenant %q parcel %q, want once with tenant-1 / parcel-1", stub.calls, stub.tenant, stub.parcel)
			}
		})
	}
}

// Covers: 源侧「没有收件地点」→ RequirementMissing，照传不补默认（票面红线「只翻译不判断」）；集运单元引用在 PS
// 那一侧本就不属任何已接受委托的成员集合，走的是同一格。
func TestNoDeliveryPlaceIsHandedOverAsMissing(t *testing.T) {
	stub := &referenceLookupStub{resolution: psdomain.NoDeliveryPlaceResolution()}
	place, resolution, err := loadThrough(t, stub, "consolidation-unit-7")
	if err != nil {
		t.Fatalf("load delivery place: %v", err)
	}
	if resolution != tfports.RequirementMissing || place != "" {
		t.Fatalf("resolution = %q place = %q, want MISSING with an empty place", resolution, place)
	}
}

// Covers: 源侧「收件地点未定」→ 按做法第 2 步取甲：译成 RequirementMissing（所有者说未定，不是说没有——端口今天没有
// 「未定」那一格，任务同样待形成）。不给引用、不猜一版。
func TestAnUndeterminedDeliveryPlaceIsHandedOverAsMissingUnderRulingA(t *testing.T) {
	stub := &referenceLookupStub{resolution: psdomain.DeliveryPlaceUndeterminedResolution()}
	place, resolution, err := loadThrough(t, stub, "parcel-1")
	if err != nil {
		t.Fatalf("load delivery place: %v", err)
	}
	if resolution != tfports.RequirementMissing || place != "" {
		t.Fatalf("resolution = %q place = %q, want MISSING with an empty place", resolution, place)
	}
}

// Covers: 读口 error 原样上抛，让执行器落它既有的「读不到」格（DELIVERY_PLACE_SOURCE_UNAVAILABLE），不折成任何一格。
func TestAReadFaceFailureIsPropagatedNotTranslated(t *testing.T) {
	readFailure := errors.New("parcel shipment: read face down")
	stub := &referenceLookupStub{err: readFailure}
	place, resolution, err := loadThrough(t, stub, "parcel-1")
	if !errors.Is(err, readFailure) {
		t.Fatalf("err = %v, want the PS read failure wrapped", err)
	}
	if resolution != tfports.RequirementResolutionInvalid || place != "" {
		t.Fatalf("a failure still handed back resolution %q place %q", resolution, place)
	}
}

// Covers: PS 封闭集之外的取值（零值答复）上抛 ErrUntranslatableAnswer——是编程错误不是业务答案，不读成「没有」。
func TestAnAnswerOutsideTheClosedSetIsUntranslatable(t *testing.T) {
	stub := &referenceLookupStub{resolution: psdomain.DeliveryPlaceResolution{}}
	place, resolution, err := loadThrough(t, stub, "parcel-1")
	if !errors.Is(err, adapter.ErrUntranslatableAnswer) {
		t.Fatalf("err = %v, want ErrUntranslatableAnswer", err)
	}
	if resolution != tfports.RequirementResolutionInvalid || place != "" {
		t.Fatalf("an untranslatable answer still handed back resolution %q place %q", resolution, place)
	}
}

func TestADeliveryPlaceSourceNeedsItsLookup(t *testing.T) {
	if _, err := adapter.NewDeliveryPlaceSource(nil); err == nil {
		t.Fatal("a nil lookup was accepted")
	}
}
