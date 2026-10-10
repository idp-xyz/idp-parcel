package domain_test

import (
	"errors"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// Covers: 目录登记依据的零值是「这一版登记时没给依据」，与给了依据分得开；给了就不能是空白。
func TestACatalogBasisIsEitherAbsentOrANonBlankReference(t *testing.T) {
	var absent domain.CatalogBasisReference
	if absent.Present() || absent.String() != "" {
		t.Fatalf("零值应答没给依据：Present=%v String=%q", absent.Present(), absent.String())
	}
	given, err := domain.NewCatalogBasisReference("SYN-NET-OPS/CHANGE-0001")
	if err != nil || !given.Present() || given.String() != "SYN-NET-OPS/CHANGE-0001" {
		t.Fatalf("租户自己的依据：%+v err=%v", given, err)
	}
	for _, blank := range []string{"", "   "} {
		if _, err := domain.NewCatalogBasisReference(blank); !errors.Is(err, domain.ErrBlankValue) {
			t.Fatalf("空白依据 %q：err=%v，想要 ErrBlankValue", blank, err)
		}
	}
}

// Covers: ADR-0157 同一条——带参考配置引用前缀的依据必须打得开已发布的那一版；形状坏了或该版没发布都立不起来，
// 不能退成一个不透明串。
func TestACatalogBasisThatCitesAReferenceMustOpenAReleasedVersion(t *testing.T) {
	released := referenceconfig.Released()
	if len(released) == 0 {
		t.Fatal("发布清单为空，没有可引的版本")
	}
	cited, err := domain.NewCatalogBasisReference(released[0].Citation())
	if err != nil || cited.String() != released[0].Citation() {
		t.Fatalf("已发布版本的引用串：%+v err=%v", cited, err)
	}
	unreleased, err := referenceconfig.NewReference(released[0].Identifier(), released[0].Version()+1000)
	if err != nil {
		t.Fatalf("构造未发布引用：%v", err)
	}
	if _, err := domain.NewCatalogBasisReference(unreleased.Citation()); !errors.Is(err, referenceconfig.ErrNotReleased) {
		t.Fatalf("未发布版本：err=%v，想要 ErrNotReleased", err)
	}
	if _, err := domain.NewCatalogBasisReference("REFCFG-1:not-a-reference"); !errors.Is(err, referenceconfig.ErrInvalidReference) {
		t.Fatalf("坏形状：err=%v，想要 ErrInvalidReference", err)
	}
}
