package cataloguepage

import "fmt"

// Result 是读端口答出的一页：本页行、下一页的游标与同一组条件下的总数（ADR-0144 决定六）。
type Result[Row any] struct {
	Rows []Row
	// Next 为空串即已到末页。
	Next string
	// Total 是与本页同一组条件下的精确总数。
	Total int64
	// Facets 是本册点名的各筛选维的分面计数（ADR-0378 决定五）；没点名的册为 nil，答复里没有这一格。
	Facets Facets
}

// FacetCount 是分面里的一格：筛选维词表上的一个值，与它在「本次的 q 与其余各维、不含这一维自己」之下的行数。
type FacetCount struct {
	Value string `json:"value"`
	Count int64  `json:"count"`
}

// Facets 是答复体在 page 之外多出的 facets 一格：按筛选维名给各值的行数，每一维照词表次序列出词表上的每个值、为零的
// 也列。各册的答复体把它标成 omitempty：没点名分面的册答复里没有这一格，不认识它的调用方不受影响。
type Facets map[string][]FacetCount

// Page 拼出答复里的 page 一格；size 是本次使用的页大小，不是本页实际行数。
func (result Result[Row]) Page(size int) Page {
	return NewPage(size, result.Next, result.Total)
}

// Finish 把读面照 Plan 取回的至多 limit+1 行切成本页；还有下一页时，用本页末行的位置编出游标。position 给出一行的
// 位置：Value 取 sortField 那一维的值（按声明的 ValueKind 规整，见 FormatInstant / FormatInteger），Identity 按声明的段序。
//
// 位置的值必须取自读面 ORDER BY 用的那几列，不取领域对象或快照文档里的副本：副本的精度或写法与列不一致时（快照里的
// 时刻带纳秒、列只到微秒），键集比较会把本页末行再取回来，或漏掉与它同值的行。所以 Row 常是读面自己的扫描行。
func Finish[Row any](
	query Query,
	fetched []Row,
	limit int,
	total int64,
	position func(row Row, sortField string) Position,
) (Result[Row], error) {
	if limit < 1 {
		return Result[Row]{}, fmt.Errorf("cataloguepage: 页大小须为正，得 %d", limit)
	}
	rows, more := Trim(fetched, limit)
	result := Result[Row]{Rows: rows, Total: total}
	if more {
		next, err := query.CursorAfter(position(rows[len(rows)-1], query.Sort.Field))
		if err != nil {
			return Result[Row]{}, err
		}
		result.Next = next
	}
	return result, nil
}
