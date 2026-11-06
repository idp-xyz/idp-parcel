package postgres_test

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/domain"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/ports"
)

// codSubledgerKeys 把一页行写成「客户/责任法人/币种/渠道」四维键，比次序时一眼看得出是哪一段走了样。
func codSubledgerKeys(rows []ports.CodSubledgerCatalogueRow) []string {
	keys := make([]string, len(rows))
	for index, row := range rows {
		keys[index] = row.Customer + "/" + row.LegalEntity + "/" + row.Currency + "/" + row.Channel
	}
	return keys
}

// walkCodSubledgers 在 values 这组条件下按 size 一页页翻到末页，交回拼起来的行；每一页的 total 都须是 want。
func walkCodSubledgers(
	t *testing.T,
	catalogue *adapter.CodSubledgerCatalogue,
	tenant domain.TenantID,
	size int,
	values url.Values,
	want int64,
) []ports.CodSubledgerCatalogueRow {
	t.Helper()
	query := pageQuery(t, ports.CodSubledgerPages, values)
	var walked []ports.CodSubledgerCatalogueRow
	for turned := 0; turned <= int(want); turned++ {
		page, err := catalogue.ListCodSubledgers(t.Context(), tenant, size, query)
		if err != nil {
			t.Fatalf("第 %d 页：%v", turned+1, err)
		}
		if page.Total != want {
			t.Fatalf("第 %d 页 total = %d，want %d", turned+1, page.Total, want)
		}
		walked = append(walked, page.Rows...)
		if page.Next == "" {
			return walked
		}
		query = nextPage(t, ports.CodSubledgerPages, values, page.Next)
	}
	t.Fatalf("翻过 %d 页还没到末页", want+1)
	return nil
}

// Covers: ADR-0144 决定一——按「客户 + 四维键」的游标翻页。翻完第一页后新开立一本排在已翻过那头的账：第二页不重出第一页的
// 行、后面各页也不漏行（偏移分页会在第二页开头重出 SYN-CUST-02）；total 随之 5 → 6，末页 next 为空。余额与批次随本页行
// 现算：第二页上那两本账的派生余额、记账笔数与批次照实。
func TestCodSubledgersPageByCursorWithoutRepeatsOrGapsWhileLedgersOpen(t *testing.T) {
	ctx := t.Context()
	catalogue, fixture := newCodSubledgerCatalogue(t)
	tenant := catalogueValue(t, domain.NewTenantID, "SYN-TENANT-01")
	for index := 1; index <= 5; index++ {
		fixture.seedSubledger(t, "SYN-TENANT-01", fmt.Sprintf("SYN-CUST-%02d", index), "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	}
	fixture.seedPosting(t, "SYN-TENANT-01", "SYN-POST-01",
		"SYN-CUST-03", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01",
		"EXTERNAL_SOURCE", "IN_TRANSIT_AT_CHANNEL", 88000, "COLLECTION_FACT", "SYN-FACT-01")
	fixture.seed(t,
		`INSERT INTO collection_remittance.remittance_batch
			(tenant_id, batch_ref, customer_ref, legal_entity_ref, currency, channel_ref,
			 collected_through, state, formed_at)
		 VALUES ('SYN-TENANT-01', 'SYN-BATCH-01', 'SYN-CUST-04', 'SYN-LE-01', 'SYN-CUR-01', 'SYN-CHAN-01',
			 $1, 'COLLECTED', $1)`,
		catalogueBaseAt)

	first, err := catalogue.ListCodSubledgers(ctx, tenant, 2, firstPage(t, ports.CodSubledgerPages))
	if err != nil {
		t.Fatal(err)
	}
	if got := codSubledgerKeys(first.Rows); !reflect.DeepEqual(got, []string{
		"SYN-CUST-01/SYN-LE-01/SYN-CUR-01/SYN-CHAN-01", "SYN-CUST-02/SYN-LE-01/SYN-CUR-01/SYN-CHAN-01",
	}) || first.Total != 5 {
		t.Fatalf("第一页：得 %v / total %d", got, first.Total)
	}

	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-00", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	second, err := catalogue.ListCodSubledgers(ctx, tenant, 2, nextPage(t, ports.CodSubledgerPages, url.Values{}, first.Next))
	if err != nil {
		t.Fatal(err)
	}
	if got := codSubledgerKeys(second.Rows); !reflect.DeepEqual(got, []string{
		"SYN-CUST-03/SYN-LE-01/SYN-CUR-01/SYN-CHAN-01", "SYN-CUST-04/SYN-LE-01/SYN-CUR-01/SYN-CHAN-01",
	}) || second.Total != 6 {
		t.Fatalf("第二页：得 %v / total %d", got, second.Total)
	}
	if posted := second.Rows[0]; posted.Balances.InTransitAtChannelMinor != 88000 || posted.PostingCount != 1 {
		t.Fatalf("第二页上的余额没有随本页行算对：%+v", posted)
	}
	if batched := second.Rows[1]; len(batched.Batches) != 1 || batched.Batches[0].Batch != "SYN-BATCH-01" ||
		batched.PostingCount != 0 {
		t.Fatalf("第二页上的批次没有随本页行带出：%+v", batched)
	}

	last, err := catalogue.ListCodSubledgers(ctx, tenant, 2, nextPage(t, ports.CodSubledgerPages, url.Values{}, second.Next))
	if err != nil {
		t.Fatal(err)
	}
	if got := codSubledgerKeys(last.Rows); !reflect.DeepEqual(got, []string{
		"SYN-CUST-05/SYN-LE-01/SYN-CUR-01/SYN-CHAN-01",
	}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q", got, last.Next)
	}
}

// Covers: ADR-0144 决定三——行标识是四维键、与排序同向作决胜键：同一客户名下几本账跨页时按（责任法人、币种、渠道）续上，
// 正序与倒序各走一遍都恰是全集、次序即四维同向。
func TestCodSubledgersBreakTiesOnTheFourDimensionKey(t *testing.T) {
	catalogue, fixture := newCodSubledgerCatalogue(t)
	tenant := catalogueValue(t, domain.NewTenantID, "SYN-TENANT-01")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-02", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-01", "SYN-LE-02", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-02", "SYN-CHAN-01")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-02")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")

	ascending := []string{
		"SYN-CUST-01/SYN-LE-01/SYN-CUR-01/SYN-CHAN-01",
		"SYN-CUST-01/SYN-LE-01/SYN-CUR-01/SYN-CHAN-02",
		"SYN-CUST-01/SYN-LE-01/SYN-CUR-02/SYN-CHAN-01",
		"SYN-CUST-01/SYN-LE-02/SYN-CUR-01/SYN-CHAN-01",
		"SYN-CUST-02/SYN-LE-01/SYN-CUR-01/SYN-CHAN-01",
	}
	descending := make([]string, len(ascending))
	for index, key := range ascending {
		descending[len(ascending)-1-index] = key
	}
	cases := map[string]struct {
		values url.Values
		want   []string
	}{
		"缺省序":  {url.Values{}, ascending},
		"客户倒序": {url.Values{"sort": {"-customer"}}, descending},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			if got := codSubledgerKeys(walkCodSubledgers(t, catalogue, tenant, 2, test.values, 5)); !reflect.DeepEqual(got, test.want) {
				t.Fatalf("走完全册：得 %v\nwant %v", got, test.want)
			}
		})
	}
}

// Covers: ADR-0144 决定四、五——q 在四维键与保管依据上做不分大小写的字面包含（% 与 _ 按字面、零命中）；total 与本页出自同一
// 组条件。空册答空页且 total 为 0（ADR-0077 Decision 四）。
func TestCodSubledgersTotalFollowsTheKeyword(t *testing.T) {
	catalogue, fixture := newCodSubledgerCatalogue(t)
	tenant := catalogueValue(t, domain.NewTenantID, "SYN-TENANT-01")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-01", "SYN-LE-01", "SYN-CUR-01", "SYN-CHAN-01")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-02", "SYN-LE-02", "SYN-CUR-01", "SYN-CHAN-02")
	fixture.seedSubledger(t, "SYN-TENANT-01", "SYN-CUST-03", "SYN-LE-01", "SYN-CUR-02", "SYN-CHAN-01")
	fixture.seedSubledger(t, "SYN-TENANT-02", "SYN-CUST-09", "SYN-LE-09", "SYN-CUR-09", "SYN-CHAN-09")

	cases := map[string]struct {
		keyword string
		want    []string
	}{
		"命中客户（不分大小写）": {"syn-cust-02", []string{"SYN-CUST-02"}},
		"命中责任法人":      {"SYN-LE-02", []string{"SYN-CUST-02"}},
		"命中币种":        {"SYN-CUR-02", []string{"SYN-CUST-03"}},
		"命中渠道":        {"SYN-CHAN-01", []string{"SYN-CUST-01", "SYN-CUST-03"}},
		"命中保管依据":      {"custody", []string{"SYN-CUST-01", "SYN-CUST-02", "SYN-CUST-03"}},
		"别的租户的值不命中":   {"SYN-CUST-09", []string{}},
		"% 按字面":       {"%", []string{}},
		"_ 按字面":       {"SYN_CUST", []string{}},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			page, err := catalogue.ListCodSubledgers(t.Context(), tenant, 10,
				pageQuery(t, ports.CodSubledgerPages, url.Values{"q": {test.keyword}}))
			if err != nil {
				t.Fatal(err)
			}
			customers := make([]string, 0, len(page.Rows))
			for _, row := range page.Rows {
				customers = append(customers, row.Customer)
			}
			if !reflect.DeepEqual(customers, test.want) || page.Total != int64(len(test.want)) || page.Next != "" {
				t.Fatalf("得 %v / total %d / next %q，应为 %v", customers, page.Total, page.Next, test.want)
			}
		})
	}

	empty, err := catalogue.ListCodSubledgers(t.Context(), catalogueValue(t, domain.NewTenantID, "SYN-TENANT-EMPTY"), 10,
		firstPage(t, ports.CodSubledgerPages))
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Rows) != 0 || empty.Total != 0 || empty.Next != "" {
		t.Fatalf("空册：得 %+v", empty)
	}
}
