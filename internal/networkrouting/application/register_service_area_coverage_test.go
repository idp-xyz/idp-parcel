package application_test

import (
	"reflect"
	"testing"

	"go.idp.xyz/idp-parcel/internal/networkrouting/application"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
)

// 本文件证服务区域覆盖与节点角色的受理门（ADR-0148 决定二、五）：覆盖形态经领域构造门收，节点角色逐个
// 成形，缺覆盖时不收覆盖内容；齐备的登记按领域规整后到达写入口。取值一律 SYN- 合成（隔离合成只记 S）。

func coveredArea() ports.ServiceAreaDefinitionVersion {
	row := validArea()
	row.HasCoverage = true
	row.CoverageCountry = "XA"
	row.PostalPrefixes = []string{"20", "10"}
	row.OriginNodes = []string{"SYN-NODE-A"}
	row.DestinationNodes = []string{"SYN-NODE-B", "SYN-NODE-C"}
	return row
}

// Covers: 覆盖在登记时就要成形，节点角色不得空白或重复，没登覆盖的版本不得带覆盖内容——每一格指名拒绝，
// 写入口零调用。
func TestServiceAreaCoverageRegistrationRefusesMalformedContentBySlot(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(row *ports.ServiceAreaDefinitionVersion)
		want   application.CatalogRefusalReason
	}{
		{"国家码小写", func(row *ports.ServiceAreaDefinitionVersion) { row.CoverageCountry = "xa" },
			application.CatalogCoverageMalformed},
		{"覆盖缺国家码", func(row *ports.ServiceAreaDefinitionVersion) { row.CoverageCountry = "" },
			application.CatalogCoverageMalformed},
		{"邮编前缀重复", func(row *ports.ServiceAreaDefinitionVersion) { row.PostalPrefixes = []string{"10", "10"} },
			application.CatalogCoverageMalformed},
		{"没登覆盖却带邮编前缀", func(row *ports.ServiceAreaDefinitionVersion) {
			row.HasCoverage, row.CoverageCountry, row.OriginNodes, row.DestinationNodes = false, "", nil, nil
		}, application.CatalogCoverageMalformed},
		{"始发节点空白", func(row *ports.ServiceAreaDefinitionVersion) { row.OriginNodes = []string{" "} },
			application.CatalogNodeRoleBlank},
		{"交付节点重复", func(row *ports.ServiceAreaDefinitionVersion) {
			row.DestinationNodes = []string{"SYN-NODE-B", "SYN-NODE-B"}
		}, application.CatalogNodeRoleDuplicated},
		{"没登覆盖却带节点角色", func(row *ports.ServiceAreaDefinitionVersion) {
			row.HasCoverage, row.CoverageCountry, row.PostalPrefixes = false, "", nil
		}, application.CatalogNodeRolesWithoutCoverage},
	}
	for _, test := range cases {
		t.Run(test.name, func(t *testing.T) {
			double := &catalogRegistryDouble{}
			row := coveredArea()
			test.mutate(&row)

			result, err := newCatalogRegistration(t, double).RegisterServiceAreaVersion(t.Context(),
				application.RegisterServiceAreaVersionCommand{TenantID: catalogTenant(t), Area: row})
			if err != nil {
				t.Fatalf("受理门拒绝是业务答案不是错误，实得：%v", err)
			}
			if result.Outcome() != application.CatalogRegistrationRefused || result.RefusalReason() != test.want {
				t.Fatalf("结果 = %s/%s，想要 REFUSED/%s", result.Outcome(), result.RefusalReason(), test.want)
			}
			if double.calls != 0 {
				t.Fatal("被拒的登记到达了写入口")
			}
		})
	}
}

// Covers: 齐备的覆盖登记到达写入口时前缀已按领域规整（同一组前缀只有一种写法），其余各格原样；整国家覆盖
// 可以只登一侧节点角色——区域只收寄或只交付是正当的目录内容。
func TestServiceAreaCoverageRegistrationPassesNormalizedCoverageThrough(t *testing.T) {
	double := &catalogRegistryDouble{}
	service := newCatalogRegistration(t, double)

	result, err := service.RegisterServiceAreaVersion(t.Context(),
		application.RegisterServiceAreaVersionCommand{TenantID: catalogTenant(t), Area: coveredArea()})
	if err != nil || result.Outcome() != application.CatalogRegistered {
		t.Fatalf("登记齐备的覆盖：outcome=%s refusal=%s err=%v", result.Outcome(), result.RefusalReason(), err)
	}
	want := coveredArea()
	want.PostalPrefixes = []string{"10", "20"}
	if !reflect.DeepEqual(double.area, want) {
		t.Fatalf("写入口收到 %+v，想要 %+v", double.area, want)
	}

	wholeCountry := validArea()
	wholeCountry.HasCoverage, wholeCountry.CoverageCountry = true, "XB"
	wholeCountry.DestinationNodes = []string{"SYN-NODE-LM"}
	result, err = service.RegisterServiceAreaVersion(t.Context(),
		application.RegisterServiceAreaVersionCommand{TenantID: catalogTenant(t), Area: wholeCountry})
	if err != nil || result.Outcome() != application.CatalogRegistered {
		t.Fatalf("登记整国家覆盖：outcome=%s refusal=%s err=%v", result.Outcome(), result.RefusalReason(), err)
	}
	if !reflect.DeepEqual(double.area, wholeCountry) {
		t.Fatalf("写入口收到 %+v，想要原样 %+v", double.area, wholeCountry)
	}
}
