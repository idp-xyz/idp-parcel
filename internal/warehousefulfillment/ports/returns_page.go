package ports

import (
	"fmt"
	"net/url"
	"strconv"
	"unicode/utf8"
)

// 退货列表的页大小。目录册不收调用方的页大小；这一口是用户点名要能改每页多少条，所以只开这几档。
var ReturnsPageSizes = []int{20, 50, 100, 200}

const ReturnsDefaultPageSize = 50

const (
	ReturnsViewAdvices  = "advices"
	ReturnsViewReceipts = "receipts"
	ReturnsViewAdvice   = "advice"
	ReturnsViewReceipt  = "receipt"
)

// ReturnsQuery 是退货查阅口的查询。View 空着表示要整份总览，不带页。
type ReturnsQuery struct {
	View      string
	Page      int
	PageSize  int
	Status    string
	Keyword   string
	Owner     string
	Warehouse string
	// Sort 是 newest 或 oldest。列表缺省 newest。
	Sort string
	Key  string
}

func (query ReturnsQuery) Paged() bool {
	return query.View == ReturnsViewAdvices || query.View == ReturnsViewReceipts
}

func (query ReturnsQuery) Keyed() bool {
	return query.View == ReturnsViewAdvice || query.View == ReturnsViewReceipt
}

// ReturnsPage 是按页或按一张取回的退货。整份总览不走这里。
type ReturnsPage struct {
	View       string
	PreAdvices []ReturnPreAdviceRow
	Receipts   []ReturnReceiptRow
	Page       int
	PageSize   int
	Total      int
	// Counts 是检索词、货主、仓之下各状态有几条，不含状态这一维的筛选。按一张取时为空。
	Counts map[string]int
}

// ParseReturnsQuery 解退货查阅的查询串。没有参数是整份总览。带了 view 才分页或按一张取。
// 拒绝理由不回显取值。
func ParseReturnsQuery(values url.Values) (ReturnsQuery, error) {
	if len(values) == 0 {
		return ReturnsQuery{}, nil
	}
	for key := range values {
		switch key {
		case "view", "page", "pageSize", "status", "q", "owner", "warehouse", "sort", "key":
		default:
			return ReturnsQuery{}, fmt.Errorf("不认识的参数 %s", safeKey(key))
		}
		if len(values[key]) != 1 {
			return ReturnsQuery{}, fmt.Errorf("参数 %s 只能给一个值", safeKey(key))
		}
	}
	query := ReturnsQuery{View: values.Get("view"), Status: values.Get("status"), Keyword: values.Get("q"), Owner: values.Get("owner"), Warehouse: values.Get("warehouse"), Sort: values.Get("sort"), Key: values.Get("key")}
	if utf8.RuneCountInString(query.Keyword) > 80 || utf8.RuneCountInString(query.Owner) > 80 || utf8.RuneCountInString(query.Warehouse) > 80 || utf8.RuneCountInString(query.Key) > 80 {
		return ReturnsQuery{}, fmt.Errorf("检索或标识过长")
	}
	switch query.View {
	case "":
		if query.pagedOrKeyedKeys(values) {
			return ReturnsQuery{}, fmt.Errorf("整份总览不收分页或筛选参数")
		}
		return ReturnsQuery{}, nil
	case ReturnsViewAdvices, ReturnsViewReceipts:
		if query.Key != "" {
			return ReturnsQuery{}, fmt.Errorf("列表不收 key")
		}
		if err := query.decodePage(values); err != nil {
			return ReturnsQuery{}, err
		}
		if query.Sort == "" {
			query.Sort = "newest"
		}
		if query.Sort != "newest" && query.Sort != "oldest" {
			return ReturnsQuery{}, fmt.Errorf("sort 只接受 newest 或 oldest")
		}
		if !statusAllowed(query.View, query.Status) {
			return ReturnsQuery{}, fmt.Errorf("status 不在这张表的状态里")
		}
		return query, nil
	case ReturnsViewAdvice, ReturnsViewReceipt:
		if query.Key == "" {
			return ReturnsQuery{}, fmt.Errorf("按一张取必须带 key")
		}
		for key := range values {
			if key != "view" && key != "key" {
				return ReturnsQuery{}, fmt.Errorf("按一张取只收 view 和 key")
			}
		}
		return query, nil
	default:
		return ReturnsQuery{}, fmt.Errorf("view 只接受 advices、receipts、advice 或 receipt")
	}
}

func (query ReturnsQuery) pagedOrKeyedKeys(values url.Values) bool {
	for _, key := range []string{"page", "pageSize", "status", "q", "owner", "warehouse", "sort", "key"} {
		if values.Get(key) != "" {
			return true
		}
	}
	return false
}

func (query *ReturnsQuery) decodePage(values url.Values) error {
	query.Page = 1
	if raw := values.Get("page"); raw != "" {
		page, err := strconv.Atoi(raw)
		if err != nil || page < 1 || page > 1_000_000 {
			return fmt.Errorf("page 须是从 1 起的整数")
		}
		query.Page = page
	}
	query.PageSize = ReturnsDefaultPageSize
	if raw := values.Get("pageSize"); raw != "" {
		size, err := strconv.Atoi(raw)
		if err != nil || !pageSizeAllowed(size) {
			return fmt.Errorf("pageSize 只接受 20、50、100 或 200")
		}
		query.PageSize = size
	}
	return nil
}

func pageSizeAllowed(size int) bool {
	for _, allowed := range ReturnsPageSizes {
		if size == allowed {
			return true
		}
	}
	return false
}

func statusAllowed(view, status string) bool {
	if status == "" {
		return true
	}
	switch view {
	case ReturnsViewAdvices:
		return status == "AWAITING_ARRIVAL" || status == "ARRIVED" || status == "DISPOSED"
	case ReturnsViewReceipts:
		return status == "AWAITING_DISPOSITION" || status == "DISPOSED" || status == "DIVERTED"
	default:
		return false
	}
}

func safeKey(key string) string {
	if len(key) > 40 {
		return "（名字过长）"
	}
	for _, r := range key {
		if r > 127 || r == ' ' {
			return "（名字含不接受的字符）"
		}
	}
	return key
}
