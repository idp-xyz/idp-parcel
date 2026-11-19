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

// 本文件证库区册的查阅用例：列表与修订史。读口用内存替身，分页、筛选与排序落在列上的行为在 adapters/postgres 的真库用例里证。

type fakeZoneReads struct {
	listed       []ports.ZoneRow
	history      []ports.ZoneRow
	err          error
	gotTenant    string
	gotLimit     int
	gotQuery     cataloguepage.Query
	gotWarehouse string
	gotCode      string
}

func (fake *fakeZoneReads) ListZones(
	_ context.Context,
	tenant domain.TenantID,
	limit int,
	query cataloguepage.Query,
) (cataloguepage.Result[ports.ZoneRow], error) {
	fake.gotTenant, fake.gotLimit, fake.gotQuery = tenant.String(), limit, query
	if fake.err != nil {
		return cataloguepage.Result[ports.ZoneRow]{}, fake.err
	}
	return cataloguepage.Result[ports.ZoneRow]{Rows: fake.listed, Total: int64(len(fake.listed))}, nil
}

func (fake *fakeZoneReads) ListZoneRevisions(
	_ context.Context,
	tenant domain.TenantID,
	warehouse domain.WarehouseCode,
	code domain.ZoneCode,
) ([]ports.ZoneRow, error) {
	fake.gotTenant, fake.gotWarehouse, fake.gotCode = tenant.String(), warehouse.String(), code.String()
	if fake.err != nil {
		return nil, fake.err
	}
	return fake.history, nil
}

func zoneFirstPage(t *testing.T, values url.Values) cataloguepage.Query {
	t.Helper()
	query, err := ports.ZonePages.Decode(values)
	if err != nil {
		t.Fatalf("解出查询 %v：%v", values, err)
	}
	return query
}

// Covers: UC-WF-001 查阅（库区的部分）——列表与修订史把读口给的行原样交出，租户、页大小、查询与仓编码、库区编码照原样转交。
func TestQueryZonesAnswersTheRowsThePortGivesForTheTenantAndQuery(t *testing.T) {
	reads := &fakeZoneReads{
		listed: []ports.ZoneRow{
			{WarehouseCode: "SYN-WH-01", ZoneCode: "SYN-ZONE-01", Revision: 2},
			{WarehouseCode: "SYN-WH-01", ZoneCode: "SYN-ZONE-02", Revision: 1},
		},
		history: []ports.ZoneRow{
			{WarehouseCode: "SYN-WH-01", ZoneCode: "SYN-ZONE-01", Revision: 1},
			{WarehouseCode: "SYN-WH-01", ZoneCode: "SYN-ZONE-01", Revision: 2},
		},
	}
	handler := application.NewQueryZonesHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	query := zoneFirstPage(t, url.Values{"warehouseCode": {"SYN-WH-01"}, "deactivated": {"false"}})

	page, err := handler.ListZones(t.Context(), tenant, 25, query)
	if err != nil || len(page.Rows) != 2 || page.Total != 2 {
		t.Fatalf("ListZones = (%d 行, 总数 %d, %v)，want 2 行", len(page.Rows), page.Total, err)
	}
	if reads.gotTenant != "SYN-TENANT-01" || reads.gotLimit != 25 ||
		reads.gotQuery.Filters["deactivated"][0] != "false" || reads.gotQuery.Filters["warehouseCode"][0] != "SYN-WH-01" {
		t.Fatalf("交给读口的 = 租户 %q、页大小 %d、查询 %+v，want 原样转交", reads.gotTenant, reads.gotLimit, reads.gotQuery)
	}

	revisions, err := handler.ZoneRevisions(t.Context(), tenant,
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"))
	if err != nil || len(revisions) != 2 || revisions[0].Revision != 1 || revisions[1].Revision != 2 {
		t.Fatalf("ZoneRevisions = (%+v, %v)，want 修订 1、2", revisions, err)
	}
	if reads.gotWarehouse != "SYN-WH-01" || reads.gotCode != "SYN-ZONE-01" {
		t.Fatalf("交给读口的仓编码 %q、库区编码 %q", reads.gotWarehouse, reads.gotCode)
	}
}

// Covers: AT-WF-009（库区的部分）——一个仓都没登记的租户，运营查阅库区答空列表、修订史答空，不答未配置，也不出现默认库区。
func TestQueryZonesAnswersAnEmptyRegisterWithEmptyListsAndNoError(t *testing.T) {
	reads := &fakeZoneReads{}
	handler := application.NewQueryZonesHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-EMPTY")

	page, err := handler.ListZones(t.Context(), tenant, 25, zoneFirstPage(t, url.Values{}))
	if err != nil || len(page.Rows) != 0 || page.Total != 0 || page.Next != "" {
		t.Fatalf("空册的列表 = (%d 行, 总数 %d, next %q, %v)，want 空页", len(page.Rows), page.Total, page.Next, err)
	}
	revisions, err := handler.ZoneRevisions(t.Context(), tenant,
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), valueOf(t, domain.NewZoneCode, "SYN-ZONE-01"))
	if err != nil || len(revisions) != 0 {
		t.Fatalf("没登记过的库区的修订史 = (%+v, %v)，want 空", revisions, err)
	}
}

// 读不动不折成空册：空册是「没登记」，读不动是「不知道」，混了就会让查阅方以为这个租户一个库区都没有。
func TestQueryZonesReturnsReadFailuresInsteadOfAnEmptyList(t *testing.T) {
	boom := errors.New("database unavailable")
	reads := &fakeZoneReads{err: boom}
	handler := application.NewQueryZonesHandler(reads, reads)
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")

	if _, err := handler.ListZones(t.Context(), tenant, 25, zoneFirstPage(t, url.Values{})); !errors.Is(err, boom) {
		t.Errorf("列表读不动的错误 = %v，want 带上原因上抛", err)
	}
	if _, err := handler.ZoneRevisions(t.Context(), tenant,
		valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), valueOf(t, domain.NewZoneCode, "SYN-ZONE-01")); !errors.Is(err, boom) {
		t.Errorf("修订史读不动的错误 = %v，want 带上原因上抛", err)
	}
}
