package postgres_test

import (
	"fmt"
	"net/url"
	"reflect"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
	"go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
)

func labelTransactionIDs(records []ports.LabelTransactionRecord) []string {
	ids := make([]string, len(records))
	for index, record := range records {
		ids[index] = record.TransactionID.String()
	}
	return ids
}

// Covers: ADR-0144 决定一——按「建立时刻 + 交易标识」的游标翻页。翻完第一页后新建立一笔，它落在已翻过的那头：第二页不重出
// 第一页的行、后面各页也不漏行（偏移分页会在第二页开头重出 SYN-LT-4）；total 随之 5 → 6，末页 next 为空。
func TestLabelTransactionsPageByCursorWithoutRepeatsOrGapsWhileTransactionsArrive(t *testing.T) {
	ctx := t.Context()
	repository, views, transactor := newLabelTransactions(t)
	tenant := mustBuild(t, domain.NewTenantID, "SYN-TENANT-01")
	establish := func(index int) {
		mustInsertLabelTransaction(t, transactor, ctx, repository, establishedLabelTransactionFixtureAt(t,
			"SYN-TENANT-01", fmt.Sprintf("SYN-LT-%d", index),
			labelEstablishedAt.Add(time.Duration(index)*time.Minute), fmt.Sprintf("SYN-PARCEL-%d", index)))
	}
	for index := 1; index <= 5; index++ {
		establish(index)
	}

	first, err := views.ListLabelTransactions(ctx, tenant, 2, firstPage(t, ports.LabelTransactionPages))
	if err != nil {
		t.Fatal(err)
	}
	if got := labelTransactionIDs(first.Rows); !reflect.DeepEqual(got, []string{"SYN-LT-5", "SYN-LT-4"}) || first.Total != 5 {
		t.Fatalf("第一页：得 %v / total %d", got, first.Total)
	}

	establish(6)
	second, err := views.ListLabelTransactions(ctx, tenant, 2, nextPage(t, ports.LabelTransactionPages, url.Values{}, first.Next))
	if err != nil {
		t.Fatal(err)
	}
	if got := labelTransactionIDs(second.Rows); !reflect.DeepEqual(got, []string{"SYN-LT-3", "SYN-LT-2"}) || second.Total != 6 {
		t.Fatalf("第二页：得 %v / total %d", got, second.Total)
	}

	last, err := views.ListLabelTransactions(ctx, tenant, 2, nextPage(t, ports.LabelTransactionPages, url.Values{}, second.Next))
	if err != nil {
		t.Fatal(err)
	}
	if got := labelTransactionIDs(last.Rows); !reflect.DeepEqual(got, []string{"SYN-LT-1"}) || last.Next != "" {
		t.Fatalf("末页：得 %v / next %q", got, last.Next)
	}
}

// Covers: ADR-0144 决定四、五——transactionResult 落在存代码的 state 列上，维内为或；q 在交易标识、渠道账户与覆盖包裹上做
// 不分大小写的字面包含（% 按字面、零命中）；total 与本页出自同一组条件。空册答空页且 total 为 0（ADR-0077 Decision 四）。
func TestLabelTransactionsTotalFollowsTheResultFilterAndTheKeyword(t *testing.T) {
	ctx := t.Context()
	repository, views, transactor := newLabelTransactions(t)
	tenant := mustBuild(t, domain.NewTenantID, "SYN-TENANT-01")
	for index := 1; index <= 2; index++ {
		mustInsertLabelTransaction(t, transactor, ctx, repository, establishedLabelTransactionFixtureAt(t,
			"SYN-TENANT-01", fmt.Sprintf("SYN-LT-%d", index),
			labelEstablishedAt.Add(time.Duration(index)*time.Minute), fmt.Sprintf("SYN-PARCEL-%d", index)))
	}
	submitted, err := establishedLabelTransactionFixtureAt(t, "SYN-TENANT-01", "SYN-LT-3",
		labelEstablishedAt.Add(3*time.Minute), "SYN-PARCEL-3").SubmitToChannel(labelEstablishedAt.Add(4 * time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	mustInsertLabelTransaction(t, transactor, ctx, repository, submitted)

	cases := map[string]struct {
		values url.Values
		want   []string
	}{
		"一个结果":      {values: url.Values{"transactionResult": {"SUBMITTED_TO_CHANNEL"}}, want: []string{"SYN-LT-3"}},
		"维内为或":      {values: url.Values{"transactionResult": {"ESTABLISHED", "SUBMITTED_TO_CHANNEL"}}, want: []string{"SYN-LT-3", "SYN-LT-2", "SYN-LT-1"}},
		"q 命中覆盖包裹":  {values: url.Values{"q": {"syn-parcel-2"}}, want: []string{"SYN-LT-2"}},
		"q 命中渠道账户":  {values: url.Values{"q": {"CHANNEL-ACCOUNT"}}, want: []string{"SYN-LT-3", "SYN-LT-2", "SYN-LT-1"}},
		"维与 q 为与":   {values: url.Values{"transactionResult": {"ESTABLISHED"}, "q": {"SYN-LT-3"}}, want: []string{}},
		"q 的 % 按字面": {values: url.Values{"q": {"%"}}, want: []string{}},
	}
	for name, test := range cases {
		t.Run(name, func(t *testing.T) {
			page, err := views.ListLabelTransactions(ctx, tenant, 10, pageQuery(t, ports.LabelTransactionPages, test.values))
			if err != nil {
				t.Fatal(err)
			}
			if got := labelTransactionIDs(page.Rows); !reflect.DeepEqual(got, test.want) || page.Total != int64(len(test.want)) {
				t.Fatalf("得 %v / total %d，应为 %v", got, page.Total, test.want)
			}
		})
	}

	empty, err := views.ListLabelTransactions(ctx, mustBuild(t, domain.NewTenantID, "SYN-TENANT-EMPTY"), 10,
		firstPage(t, ports.LabelTransactionPages))
	if err != nil {
		t.Fatal(err)
	}
	if len(empty.Rows) != 0 || empty.Total != 0 || empty.Next != "" {
		t.Fatalf("空册：得 %+v", empty)
	}
}
