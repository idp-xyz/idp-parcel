package postgres_test

import (
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// 本文件对真实 PostgreSQL 证渠道产品目录按 ADR-0144 分页（票 catalogue-read-pagination-rollout/11）：翻页中途登记新映射
// 不重不漏、q 收窄的总数与本页同一组条件（命中服务产品、版本与渠道引用；显式“未配置”的绑定不命中任何渠道）、空册答空页。

const mappingPageTenant = "SYN-TENANT-01"

// productMappingFixture 同 mappingRegistrationFixture，另可给服务产品与版本：q 要分得出不同产品。
func productMappingFixture(t *testing.T, id, product, version string, channels ...string) domain.ProductChannelMappingRegistration {
	t.Helper()
	binding := domain.UnconfiguredChannelBinding()
	if len(channels) > 0 {
		refs := make([]domain.ChannelProductReference, 0, len(channels))
		for _, channel := range channels {
			refs = append(refs, pcValue(t, domain.NewChannelProductReference, channel))
		}
		var err error
		binding, err = domain.NewConfiguredChannelBinding(refs)
		if err != nil {
			t.Fatalf("new configured binding: %v", err)
		}
	}
	interval, err := domain.NewEffectiveInterval(mappingStartsAt, time.Time{})
	if err != nil {
		t.Fatalf("new interval: %v", err)
	}
	registration, err := domain.NewProductChannelMappingRegistration(
		pcTenant(t, mappingPageTenant),
		pcValue(t, domain.NewProductChannelMappingID, id),
		1,
		domain.ProductChannelMappingSpec{
			Product:        pcValue(t, domain.NewCommercialObjectID, product),
			ProductVersion: pcValue(t, domain.NewCommercialVersionLabel, version),
			Binding:        binding,
			Effective:      interval,
			Basis:          pcValue(t, domain.NewMappingBasisReference, "SYN-BASIS-"+id),
		},
	)
	if err != nil {
		t.Fatalf("new mapping registration: %v", err)
	}
	return registration
}

// Covers: 完成判据「翻页期间插入新行不重不漏」与「空册答空页」。
func TestProductChannelMappingCataloguePagesWithoutRepeatsOrGapsWhileRowsArrive(t *testing.T) {
	mappings, catalogue, transactor := newProductChannelMappings(t)
	ctx := t.Context()
	list := func(tenant domain.TenantID) pageLister[ports.ProductChannelMappingRow] {
		return func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.ProductChannelMappingRow], error) {
			return catalogue.ListProductChannelMappings(ctx, tenant, limit, query)
		}
	}

	assertPagesWithoutRepeatsOrGaps(t, ports.ProductChannelMappingPages, list(pcTenant(t, mappingPageTenant)), func(id string) {
		mustSaveMapping(t, transactor, mappings, productMappingFixture(t, id, "SYN-PRODUCT-1", "v1", "SYN-CH-1"))
	}, func(row ports.ProductChannelMappingRow) string { return row.MappingID }, "SYN-MAP-")
	assertEmptyCatalogue(t, "渠道产品映射", ports.ProductChannelMappingPages, list(pcTenant(t, "SYN-TENANT-EMPTY")))
}

// Covers: 完成判据「total 随 q 变；q 里的 % 按字面」。本册不设筛选维（映射没有状态代数），收窄只靠 q。渠道引用存在 JSON
// 数组里，q 只看各引用的值：引号与逗号不命中，两个相邻引用用空格拼起来也不命中。
func TestProductChannelMappingCatalogueSearches(t *testing.T) {
	mappings, catalogue, transactor := newProductChannelMappings(t)
	ctx := t.Context()
	mustSaveMapping(t, transactor, mappings, productMappingFixture(t, "SYN-MAP-A", "SYN-PRODUCT-EXPRESS", "v1", "SYN-CH-POST", "SYN-CH-AGG"))
	mustSaveMapping(t, transactor, mappings, productMappingFixture(t, "SYN-MAP-B", "SYN-PRODUCT-ECONOMY", "v2"))
	mustSaveMapping(t, transactor, mappings, productMappingFixture(t, "SYN-MAP-C", "SYN-PRODUCT-EXPRESS", "v2", "SYN-CH-AGG"))

	tenant := pcTenant(t, mappingPageTenant)
	assertFilteredPages(t, "渠道产品映射", ports.ProductChannelMappingPages,
		func(query cataloguepage.Query, limit int) (cataloguepage.Result[ports.ProductChannelMappingRow], error) {
			return catalogue.ListProductChannelMappings(ctx, tenant, limit, query)
		},
		func(row ports.ProductChannelMappingRow) string { return row.MappingID },
		[]pageCase{
			{"", "SYN-MAP-C,SYN-MAP-B,SYN-MAP-A"},
			{"sort=registeredAt", "SYN-MAP-A,SYN-MAP-B,SYN-MAP-C"},
			{"q=express", "SYN-MAP-C,SYN-MAP-A"},
			{"q=SYN-CH-AGG", "SYN-MAP-C,SYN-MAP-A"},
			{"q=syn-ch-post", "SYN-MAP-A"},
			{"q=SYN-CH-POST%20SYN-CH-AGG", ""},
			{"q=%22", ""},
			{"q=%2C", ""},
			{"q=economy", "SYN-MAP-B"},
			{"q=v2", "SYN-MAP-C,SYN-MAP-B"},
			{"q=SYN-BASIS-SYN-MAP-B", "SYN-MAP-B"},
			{"q=%25", ""},
		})
}
