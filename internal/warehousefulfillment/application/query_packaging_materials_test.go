package application_test

import (
	"context"
	"errors"
	"net/url"
	"testing"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// 本文件证包材册的查阅用例：列表与修订史。读口用内存替身，分页、筛选与排序落在列上的行为在 adapters/postgres 的真库用例里证。

type fakePackagingMaterialReads struct {
	listed    []ports.PackagingMaterialRow
	history   []ports.PackagingMaterialRow
	err       error
	gotTenant string
	gotLimit  int
	gotQuery  cataloguepage.Query
	gotCode   string
}

func (fake *fakePackagingMaterialReads) ListPackagingMaterials(
	_ context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.PackagingMaterialRow], error) {
	fake.gotTenant, fake.gotLimit, fake.gotQuery = tenant.String(), limit, query
	if fake.err != nil {
		return cataloguepage.Result[ports.PackagingMaterialRow]{}, fake.err
	}
	return cataloguepage.Result[ports.PackagingMaterialRow]{Rows: fake.listed, Total: int64(len(fake.listed))}, nil
}

func (fake *fakePackagingMaterialReads) ListPackagingMaterialRevisions(
	_ context.Context,
	tenant domain.TenantID,
	code domain.PackagingMaterialCode,
) ([]ports.PackagingMaterialRow, error) {
	fake.gotTenant, fake.gotCode = tenant.String(), code.String()
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.history, nil
}

func packagingMaterialFirstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.PackagingMaterialPages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// Covers: UC-WF-001 查阅（包材的部分）——列表与修订史把读口给的行原样交出，租户、页大小、查询与包材编码照原样转交。
func TestQueryPackagingMaterialsAnswersTheRowsThePortGivesForTheTenantAndQuery(t *testing.T) {
	reads := &fakePackagingMaterialReads{
		listed: []ports.PackagingMaterialRow{
			{PackagingMaterialCode: "SYN-BOX-01", Revision: 2},
			{PackagingMaterialCode: "SYN-BOX-02", Revision: 1},
		},
		history: []ports.PackagingMaterialRow{
			{PackagingMaterialCode: "SYN-BOX-01", Revision: 1},
			{PackagingMaterialCode: "SYN-BOX-01", Revision: 2},
		},
	}
	handler := application.NewQueryPackagingMaterialsHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	query := packagingMaterialFirstPage(t, url.Values{"deactivated": {"false"}})

	page, err := handler.ListPackagingMaterials(t.Context(), tenant, 25, query)
	if err != nil || len(page.Rows) != 2 || page.Total != 2 {
		t.Fatalf("ListPackagingMaterials = (%d 行, 总数 %d, %v)，want 2 行", len(page.Rows), page.Total, err)
	}
	if reads.gotTenant != "SYN-TENANT-01" || reads.gotLimit != 25 || reads.gotQuery.Filters["deactivated"][0] != "false" {
		t.Fatalf("交给读口的 = 租户 %q、页大小 %d、查询 %+v，want 原样转交", reads.gotTenant, reads.gotLimit, reads.gotQuery)
	}

	revisions, err := handler.PackagingMaterialRevisions(t.Context(), tenant, valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"))
	if err != nil || len(revisions) != 2 || revisions[0].Revision != 1 || revisions[1].Revision != 2 {
		t.Fatalf("PackagingMaterialRevisions = (%+v, %v)，want 修订 1、2", revisions, err)
	}
	if reads.gotCode != "SYN-BOX-01" {
		t.Fatalf("交给读口的包材编码 %q", reads.gotCode)
	}
}

// Covers: AT-WF-009（包材的部分）——一个仓都没登记的租户，运营查阅包材答空列表、修订史答空，不答未配置，也不出现默认包材。
func TestQueryPackagingMaterialsAnswersAnEmptyRegisterWithEmptyListsAndNoError(t *testing.T) {
	reads := &fakePackagingMaterialReads{}
	handler := application.NewQueryPackagingMaterialsHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-EMPTY")

	page, err := handler.ListPackagingMaterials(t.Context(), tenant, 25, packagingMaterialFirstPage(t, url.Values{}))
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册的列表 = (%d 行, 总数 %d, next %q, %v)，want 空页", len(page.Rows), page.Total, page.Next, err)
	}
	revisions, err := handler.PackagingMaterialRevisions(t.Context(), tenant, valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01"))
	if err != nil || len(revisions) != 0 {
		t.Fatalf("没登记过的包材的修订史 = (%+v, %v)，want 空", revisions, err)
	}
}

// 读不动不折成空册：空册是「没登记」，读不动是「不知道」，混了就会让查阅方以为这个租户一个包材都没有。
func TestQueryPackagingMaterialsReturnsReadFailuresInsteadOfAnEmptyList(t *testing.T) {
	boom := errors.New("database unavailable")
	reads := &fakePackagingMaterialReads{err: boom}
	handler := application.NewQueryPackagingMaterialsHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")

	if _, err := handler.ListPackagingMaterials(t.Context(), tenant, 25, packagingMaterialFirstPage(t, url.Values{})); !errors.Is(err, boom) {
		t.Errorf("列表读不动的错误 = %v，want 带上原因上抛", err)
	}
	if _, err := handler.PackagingMaterialRevisions(t.Context(), tenant, valueOf(t, domain.NewPackagingMaterialCode, "SYN-BOX-01")); !errors.Is(err, boom) {
		t.Errorf("修订史读不动的错误 = %v，want 带上原因上抛", err)
	}
}
