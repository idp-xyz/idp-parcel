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

// 本文件证库位类型册的查阅用例：列表与修订史。读口用内存替身，分页、筛选与排序落在列上的行为在 adapters/postgres 的真库用例里证。

type fakeLocationTypeReads struct {
	listed    []ports.LocationTypeRow
	history   []ports.LocationTypeRow
	err       error
	gotTenant string
	gotLimit  int
	gotQuery  cataloguepage.Query
	gotCode   string
}

func (fake *fakeLocationTypeReads) ListLocationTypes(
	_ context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.LocationTypeRow], error) {
	fake.gotTenant, fake.gotLimit, fake.gotQuery = tenant.String(), limit, query
	if fake.err != nil {
		return cataloguepage.Result[ports.LocationTypeRow]{}, fake.err
	}
	return cataloguepage.Result[ports.LocationTypeRow]{Rows: fake.listed, Total: int64(len(fake.listed))}, nil
}

func (fake *fakeLocationTypeReads) ListLocationTypeRevisions(
	_ context.Context,
	tenant domain.TenantID,
	code domain.LocationTypeCode,
) ([]ports.LocationTypeRow, error) {
	fake.gotTenant, fake.gotCode = tenant.String(), code.String()
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.history, nil
}

func locationTypeFirstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.LocationTypePages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// Covers: UC-WF-001 查阅（库位类型的部分）——列表与修订史把读口给的行原样交出，租户、页大小、查询与库位类型编码照原样转交。
func TestQueryLocationTypesAnswersTheRowsThePortGivesForTheTenantAndQuery(t *testing.T) {
	reads := &fakeLocationTypeReads{
		listed: []ports.LocationTypeRow{
			{LocationTypeCode: "SYN-LT-01", Revision: 2},
			{LocationTypeCode: "SYN-LT-02", Revision: 1},
		},
		history: []ports.LocationTypeRow{
			{LocationTypeCode: "SYN-LT-01", Revision: 1},
			{LocationTypeCode: "SYN-LT-01", Revision: 2},
		},
	}
	handler := application.NewQueryLocationTypesHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	query := locationTypeFirstPage(t, url.Values{"deactivated": {"false"}})

	page, err := handler.ListLocationTypes(t.Context(), tenant, 25, query)
	if err != nil || len(page.Rows) != 2 || page.Total != 2 {
		t.Fatalf("ListLocationTypes = (%d 行, 总数 %d, %v)，want 2 行", len(page.Rows), page.Total, err)
	}
	if reads.gotTenant != "SYN-TENANT-01" || reads.gotLimit != 25 || reads.gotQuery.Filters["deactivated"][0] != "false" {
		t.Fatalf("交给读口的 = 租户 %q、页大小 %d、查询 %+v，want 原样转交", reads.gotTenant, reads.gotLimit, reads.gotQuery)
	}

	revisions, err := handler.LocationTypeRevisions(t.Context(), tenant, valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"))
	if err != nil || len(revisions) != 2 || revisions[0].Revision != 1 || revisions[1].Revision != 2 {
		t.Fatalf("LocationTypeRevisions = (%+v, %v)，want 修订 1、2", revisions, err)
	}
	if reads.gotCode != "SYN-LT-01" {
		t.Fatalf("交给读口的库位类型编码 %q", reads.gotCode)
	}
}

// Covers: AT-WF-009（库位类型的部分）——一个仓都没登记的租户，运营查阅库位类型答空列表、修订史答空，不答未配置，也不出现默认库位类型。
func TestQueryLocationTypesAnswersAnEmptyRegisterWithEmptyListsAndNoError(t *testing.T) {
	reads := &fakeLocationTypeReads{}
	handler := application.NewQueryLocationTypesHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-EMPTY")

	page, err := handler.ListLocationTypes(t.Context(), tenant, 25, locationTypeFirstPage(t, url.Values{}))
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册的列表 = (%d 行, 总数 %d, next %q, %v)，want 空页", len(page.Rows), page.Total, page.Next, err)
	}
	revisions, err := handler.LocationTypeRevisions(t.Context(), tenant, valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01"))
	if err != nil || len(revisions) != 0 {
		t.Fatalf("没登记过的库位类型的修订史 = (%+v, %v)，want 空", revisions, err)
	}
}

// 读不动不折成空册：空册是「没登记」，读不动是「不知道」，混了就会让查阅方以为这个租户一个库位类型都没有。
func TestQueryLocationTypesReturnsReadFailuresInsteadOfAnEmptyList(t *testing.T) {
	boom := errors.New("database unavailable")
	reads := &fakeLocationTypeReads{err: boom}
	handler := application.NewQueryLocationTypesHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")

	if _, err := handler.ListLocationTypes(t.Context(), tenant, 25, locationTypeFirstPage(t, url.Values{})); !errors.Is(err, boom) {
		t.Errorf("列表读不动的错误 = %v，want 带上原因上抛", err)
	}
	if _, err := handler.LocationTypeRevisions(t.Context(), tenant, valueOf(t, domain.NewLocationTypeCode, "SYN-LT-01")); !errors.Is(err, boom) {
		t.Errorf("修订史读不动的错误 = %v，want 带上原因上抛", err)
	}
}
