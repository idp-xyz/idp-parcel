package ports

import (
	"net/url"
	"strings"
	"testing"
)

func TestParseReturnsQueryTreatsAnEmptyQueryAsTheWholeOverview(t *testing.T) {
	query, err := ParseReturnsQuery(nil)
	if err != nil || query.View != "" || query.Paged() || query.Keyed() {
		t.Fatalf("空查询 = %+v %v", query, err)
	}
}

func TestParseReturnsQueryPagesAdvicesWithTheDefaultPageSize(t *testing.T) {
	query, err := ParseReturnsQuery(url.Values{"view": {"advices"}, "status": {"ARRIVED"}, "sort": {"oldest"}})
	if err != nil {
		t.Fatal(err)
	}
	if query.Page != 1 || query.PageSize != ReturnsDefaultPageSize || query.Status != "ARRIVED" || query.Sort != "oldest" {
		t.Fatalf("%+v", query)
	}
}

func TestParseReturnsQueryRefusesAPageSizeOutsideTheSet(t *testing.T) {
	for _, size := range []string{"0", "1", "15", "201", "50x"} {
		_, err := ParseReturnsQuery(url.Values{"view": {"receipts"}, "pageSize": {size}})
		if err == nil || !strings.Contains(err.Error(), "pageSize") {
			t.Fatalf("pageSize=%s 应拒，得 %v", size, err)
		}
		if strings.Contains(size, "x") && strings.Contains(err.Error(), size) {
			t.Fatalf("理由回显了取值 %s：%v", size, err)
		}
	}
}

func TestParseReturnsQueryRefusesPagingOnTheWholeOverview(t *testing.T) {
	_, err := ParseReturnsQuery(url.Values{"pageSize": {"50"}})
	if err == nil {
		t.Fatal("整份总览不该收下 pageSize")
	}
}

func TestParseReturnsQueryReadsOneAdviceByKey(t *testing.T) {
	query, err := ParseReturnsQuery(url.Values{"view": {"advice"}, "key": {"RT-1"}})
	if err != nil || !query.Keyed() || query.Key != "RT-1" {
		t.Fatalf("%+v %v", query, err)
	}
}
