package domain_test

import (
	"errors"
	"strings"
	"testing"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

func deliveryPlaceScope(t *testing.T, requestID string) domain.SourceDataScope {
	t.Helper()
	scope, err := domain.NewShipmentScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, requestID),
		domain.DeliveryPlaceDataGroup(),
	)
	if err != nil {
		t.Fatalf("new shipment scoped source data: %v", err)
	}
	return scope
}

func deliveryPlaceReference(
	t *testing.T,
	tenant, requestID string,
	anchor domain.SourceDataVersionAnchor,
) domain.DeliveryPlaceReference {
	t.Helper()
	reference, err := domain.NewDeliveryPlaceReference(
		mustValue(t, domain.NewTenantID, tenant),
		deliveryPlaceScope(t, requestID),
		anchor,
	)
	if err != nil {
		t.Fatalf("new delivery place reference: %v", err)
	}
	return reference
}

func versionAnchor(t *testing.T, versionID string) domain.SourceDataVersionAnchor {
	t.Helper()
	anchor, err := domain.NewSourceDataVersionAnchor(mustValue(t, domain.NewSourceDataVersionID, versionID))
	if err != nil {
		t.Fatalf("new source data version anchor: %v", err)
	}
	return anchor
}

// Covers: ADR-0130 决定一「引用是委托级……同一委托的两个包裹答出同一个引用」与「串自带形状版本……
// 四段之外不多一字，地址内容一个字不进串」。规范串只由四段决定，形状版本前缀 `DPR-1:` 让 TF 当
// 不透明串存下的东西日后仍能按记录时的形状解读（ADR-0014 先例）。
func TestADeliveryPlaceReferenceRendersItsFourSegmentsBehindTheShapeVersion(t *testing.T) {
	baseline := deliveryPlaceReference(t, "tenant-1", "request-1", domain.NewAcceptanceBaselineAnchor())
	if got, want := baseline.String(), "DPR-1:tenant-1/request-1/DELIVERY_PLACE/baseline"; got != want {
		t.Fatalf("baseline anchored reference = %q, want %q", got, want)
	}

	adopted := deliveryPlaceReference(t, "tenant-1", "request-1", versionAnchor(t, "data-version-2"))
	if got, want := adopted.String(), "DPR-1:tenant-1/request-1/DELIVERY_PLACE/version;data-version-2"; got != want {
		t.Fatalf("version anchored reference = %q, want %q", got, want)
	}

	if baseline.String() == adopted.String() {
		t.Fatal("a baseline anchor and a version anchor on the same scope rendered the same string")
	}
	if again := deliveryPlaceReference(t, "tenant-1", "request-1", domain.NewAcceptanceBaselineAnchor()); again != baseline || again.String() != baseline.String() {
		t.Fatalf("two references built from the same four segments differ: %q vs %q", again.String(), baseline.String())
	}
}

// Covers: ADR-0130 决定一「收件资料范围若按 SourceDataScope 形状指名了包裹，引用照样带上它」——
// 包裹那一段进范围段，不另开第五段。
func TestAParcelScopedDeliveryPlaceReferenceCarriesTheParcelInsideTheScopeSegment(t *testing.T) {
	scope, err := domain.NewParcelScopedSourceData(
		mustValue(t, domain.NewShipmentRequestID, "request-1"),
		mustValue(t, domain.NewDeclaredParcelID, "parcel-1"),
		domain.DeliveryPlaceDataGroup(),
	)
	if err != nil {
		t.Fatalf("new parcel scoped source data: %v", err)
	}
	reference, err := domain.NewDeliveryPlaceReference(
		mustValue(t, domain.NewTenantID, "tenant-1"), scope, domain.NewAcceptanceBaselineAnchor(),
	)
	if err != nil {
		t.Fatalf("new delivery place reference: %v", err)
	}
	if got, want := reference.String(), "DPR-1:tenant-1/request-1/DELIVERY_PLACE;parcel-1/baseline"; got != want {
		t.Fatalf("parcel scoped reference = %q, want %q", got, want)
	}
	rebuilt, err := domain.ParseDeliveryPlaceReference(reference.String())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if rebuilt != reference {
		t.Fatalf("round trip changed the reference: %#v vs %#v", rebuilt, reference)
	}

	// 范围段带包裹、锚段带版本时两个子段分隔符同时在场，往返要分得清哪个 `;` 属哪一段。
	onVersion, err := domain.NewDeliveryPlaceReference(
		mustValue(t, domain.NewTenantID, "tenant-1"), scope, versionAnchor(t, "data-version-4"),
	)
	if err != nil {
		t.Fatalf("new delivery place reference: %v", err)
	}
	if got, want := onVersion.String(), "DPR-1:tenant-1/request-1/DELIVERY_PLACE;parcel-1/version;data-version-4"; got != want {
		t.Fatalf("parcel scoped version anchored reference = %q, want %q", got, want)
	}
	if rebuilt, err = domain.ParseDeliveryPlaceReference(onVersion.String()); err != nil || rebuilt != onVersion {
		t.Fatalf("round trip of a parcel scoped version anchored reference: %#v, %v", rebuilt, err)
	}
}

// Covers: 完成判据 1「构造 / 重建 / String() 往返」。段里的分隔符与非 ASCII 都要能往返——标识是
// 自由文本引用（requiredValue 只拒空白），谁也不保证租户号里没有斜杠或分号。
func TestADeliveryPlaceReferenceRoundTripsThroughItsCanonicalString(t *testing.T) {
	cases := []struct {
		name   string
		tenant string
		req    string
		anchor domain.SourceDataVersionAnchor
	}{
		{"baseline anchor", "tenant-1", "request-1", domain.NewAcceptanceBaselineAnchor()},
		{"version anchor", "tenant-1", "request-1", versionAnchor(t, "data-version-7")},
		{"delimiters inside segments", "t/a;b", "req/1;x", versionAnchor(t, "v;1/2")},
		{"non ascii and spaces", "租户 甲", "委托 1", versionAnchor(t, "版本 2")},
		{"percent sign inside a segment", "100%", "r%2F", versionAnchor(t, "%41")},
	}
	for _, item := range cases {
		t.Run(item.name, func(t *testing.T) {
			reference := deliveryPlaceReference(t, item.tenant, item.req, item.anchor)
			canonical := reference.String()
			if !strings.HasPrefix(canonical, "DPR-1:") {
				t.Fatalf("canonical string %q lacks the shape version prefix", canonical)
			}
			rebuilt, err := domain.ParseDeliveryPlaceReference(canonical)
			if err != nil {
				t.Fatalf("parse %q: %v", canonical, err)
			}
			if rebuilt != reference {
				t.Fatalf("round trip changed the reference: %#v vs %#v", rebuilt, reference)
			}
			if rebuilt.String() != canonical {
				t.Fatalf("re-rendered %q, want %q", rebuilt.String(), canonical)
			}
			if rebuilt.TenantID().String() != item.tenant || rebuilt.ShipmentRequestID().String() != item.req {
				t.Fatalf("segments lost in the round trip: tenant %q request %q", rebuilt.TenantID(), rebuilt.ShipmentRequestID())
			}
		})
	}
}

// Covers: 票面「带重建门（ADR-0028），非规范串拒不规范化」。重建门只认本上下文自己渲染得出的那一
// 种写法：换个转义大小写、多一段、少一段、没有前缀，都是别人造的串，收下等于让第二种写法冒充同一个
// 引用——而 TF 拿两个引用比「地址变了没有」比的正是这个串。
func TestANonCanonicalDeliveryPlaceReferenceStringIsRefused(t *testing.T) {
	cases := map[string]string{
		"empty":                        "",
		"missing shape version":        "tenant-1/request-1/DELIVERY_PLACE/baseline",
		"unknown shape version":        "DPR-2:tenant-1/request-1/DELIVERY_PLACE/baseline",
		"too few segments":             "DPR-1:tenant-1/request-1/DELIVERY_PLACE",
		"too many segments":            "DPR-1:tenant-1/request-1/DELIVERY_PLACE/baseline/extra",
		"blank tenant":                 "DPR-1:%20/request-1/DELIVERY_PLACE/baseline",
		"blank request":                "DPR-1:tenant-1//DELIVERY_PLACE/baseline",
		"blank group":                  "DPR-1:tenant-1/request-1//baseline",
		"blank parcel in scope":        "DPR-1:tenant-1/request-1/DELIVERY_PLACE;/baseline",
		"three parts in scope":         "DPR-1:tenant-1/request-1/DELIVERY_PLACE;parcel-1;extra/baseline",
		"unknown anchor":               "DPR-1:tenant-1/request-1/DELIVERY_PLACE/latest",
		"version anchor without id":    "DPR-1:tenant-1/request-1/DELIVERY_PLACE/version;",
		"version anchor blank id":      "DPR-1:tenant-1/request-1/DELIVERY_PLACE/version;%20",
		"lowercase percent escape":     "DPR-1:t%2fa/request-1/DELIVERY_PLACE/baseline",
		"needless percent escape":      "DPR-1:%74enant-1/request-1/DELIVERY_PLACE/baseline",
		"raw space instead of escape":  "DPR-1:tenant 1/request-1/DELIVERY_PLACE/baseline",
		"surrounding whitespace":       " DPR-1:tenant-1/request-1/DELIVERY_PLACE/baseline",
		"truncated percent escape":     "DPR-1:tenant%2/request-1/DELIVERY_PLACE/baseline",
		"baseline anchor with payload": "DPR-1:tenant-1/request-1/DELIVERY_PLACE/baseline;x",
		// 尾随空白与换行落在锚段里：requiredValue 只拒全空白，版本号「data-version-2 」本身立得住，
		// 拦它的是重渲染逐字比——正是「拒不规范化」那道门，不是构造函数。
		"trailing whitespace":   "DPR-1:tenant-1/request-1/DELIVERY_PLACE/version;data-version-2 ",
		"trailing newline":      "DPR-1:tenant-1/request-1/DELIVERY_PLACE/version;data-version-2\n",
		"lower-case shape word": "dpr-1:tenant-1/request-1/DELIVERY_PLACE/baseline",
		"foreign shape version": "PSC-1:tenant-1/request-1/DELIVERY_PLACE/baseline",
	}
	for name, raw := range cases {
		t.Run(name, func(t *testing.T) {
			rebuilt, err := domain.ParseDeliveryPlaceReference(raw)
			if !errors.Is(err, domain.ErrInvalidDeliveryPlaceReference) {
				t.Fatalf("parse %q: err = %v, want ErrInvalidDeliveryPlaceReference", raw, err)
			}
			if rebuilt != (domain.DeliveryPlaceReference{}) {
				t.Fatalf("refusal still handed back a reference: %#v", rebuilt)
			}
		})
	}
}

// Covers: ADR-0130 决定一「资料版本锚两态，与 SourceDataBasis 同形」——零值不是锚，半截的引用立不起来。
func TestADeliveryPlaceReferenceRequiresAllFourSegments(t *testing.T) {
	tenant := mustValue(t, domain.NewTenantID, "tenant-1")
	scope := deliveryPlaceScope(t, "request-1")

	if _, err := domain.NewDeliveryPlaceReference(domain.TenantID{}, scope, domain.NewAcceptanceBaselineAnchor()); !errors.Is(err, domain.ErrInvalidDeliveryPlaceReference) {
		t.Fatalf("blank tenant: err = %v", err)
	}
	if _, err := domain.NewDeliveryPlaceReference(tenant, domain.SourceDataScope{}, domain.NewAcceptanceBaselineAnchor()); !errors.Is(err, domain.ErrInvalidDeliveryPlaceReference) {
		t.Fatalf("zero scope: err = %v", err)
	}
	if _, err := domain.NewDeliveryPlaceReference(tenant, scope, domain.SourceDataVersionAnchor{}); !errors.Is(err, domain.ErrInvalidDeliveryPlaceReference) {
		t.Fatalf("zero anchor: err = %v", err)
	}
	if _, err := domain.NewSourceDataVersionAnchor(domain.SourceDataVersionID{}); !errors.Is(err, domain.ErrInvalidSourceDataVersionAnchor) {
		t.Fatalf("blank version anchor: err = %v", err)
	}
	if (domain.DeliveryPlaceReference{}).String() != "" {
		t.Fatalf("a zero reference rendered %q, want the empty string", (domain.DeliveryPlaceReference{}).String())
	}
}

// Covers: 锚的两态各自可读——消费方比「地址内容变了没有」看的是锚，不是整串。
func TestASourceDataVersionAnchorExposesItsTwoStates(t *testing.T) {
	baseline := domain.NewAcceptanceBaselineAnchor()
	if !baseline.OnAcceptanceBaseline() {
		t.Fatal("the baseline anchor does not say it is on the acceptance baseline")
	}
	if _, adopted := baseline.AdoptedVersion(); adopted {
		t.Fatal("the baseline anchor names an adopted version")
	}

	version := versionAnchor(t, "data-version-3")
	if version.OnAcceptanceBaseline() {
		t.Fatal("a version anchor claims to be on the acceptance baseline")
	}
	if adopted, present := version.AdoptedVersion(); !present || adopted.String() != "data-version-3" {
		t.Fatalf("adopted version = %q present = %v", adopted, present)
	}
}
