package domain_test

import (
	"testing"

	"go.idp.xyz/idp-parcel/internal/customscompliance/domain"
)

func routeValue[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	built, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return built
}

// Covers: 三个引用词形的空值门——空白引用构造不出，缺格在进目录之前就被指名拒绝。
func TestPortsPathsReferencesRejectBlankValues(t *testing.T) {
	if _, err := domain.NewCustomsPortReference("  "); err == nil {
		t.Fatal("空白口岸标识被构造出来了")
	}
	if _, err := domain.NewDeclarationPathReference(""); err == nil {
		t.Fatal("空白申报路径标识被构造出来了")
	}
	if _, err := domain.NewDeclarationModeReference("\t"); err == nil {
		t.Fatal("空白申报模式引用被构造出来了")
	}
}

// Covers: DeclarationPathRoute 三维缺一构造不出——零值口岸、集合外方向、零值模式
// 各自被指名拒绝，不会静默落成一条没人登记过的路径事实。
func TestDeclarationPathRouteRequiresAllThreeDimensions(t *testing.T) {
	port := routeValue(t, domain.NewCustomsPortReference, "SYN-PORT-01")
	mode := routeValue(t, domain.NewDeclarationModeReference, "SYN-MODE-GENERAL")

	if _, err := domain.NewDeclarationPathRoute(
		domain.CustomsPortReference{}, domain.ExportManifest, mode); err == nil {
		t.Fatal("零值口岸被接受了")
	}
	if _, err := domain.NewDeclarationPathRoute(
		port, domain.ManifestDirectionInvalid, mode); err == nil {
		t.Fatal("集合外方向被接受了")
	}
	if _, err := domain.NewDeclarationPathRoute(
		port, domain.ExportManifest, domain.DeclarationModeReference{}); err == nil {
		t.Fatal("零值申报模式被接受了")
	}
}

// Covers: 合法三维原样读回；路由是纯值对象，同输入可比较相等（应用层冲突判定靠它）。
func TestDeclarationPathRouteCarriesItsFacts(t *testing.T) {
	port := routeValue(t, domain.NewCustomsPortReference, "SYN-PORT-01")
	mode := routeValue(t, domain.NewDeclarationModeReference, "SYN-MODE-GENERAL")

	route, err := domain.NewDeclarationPathRoute(port, domain.ImportManifest, mode)
	if err != nil {
		t.Fatalf("构造路径三维：%v", err)
	}
	if route.Port() != port || route.Direction() != domain.ImportManifest || route.Mode() != mode {
		t.Fatalf("三维没有原样读回：%+v", route)
	}

	same, err := domain.NewDeclarationPathRoute(port, domain.ImportManifest, mode)
	if err != nil {
		t.Fatalf("构造同输入路径三维：%v", err)
	}
	if route != same {
		t.Fatal("同输入的路径三维不相等——冲突判定的逐字段比对将失真")
	}
}
