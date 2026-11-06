package cataloguepage

import "fmt"

// Result 是读端口答出的一页：本页行、下一页的游标与同一组条件下的总数（ADR-0144 决定六）。
type Result[Row any] struct {
	Rows []Row
	// Next 为空串即已到末页。
	Next string
	// Total 是与本页同一组条件下的精确总数。
	Total int64
}

// Page 拼出答复里的 page 一格；size 是本次使用的页大小，不是本页实际行数。
func (result Result[Row]) Page(size int) Page {
	return NewPage(size, result.Next, result.Total)
}

// Finish 把读面照 Plan 取回的至多 limit+1 行切成本页；还有下一页时，用本页末行的位置编出游标。position 给出一行的
// 位置：Value 取 sortField 那一维的值（按声明的 ValueKind 规整，见 FormatInstant / FormatInteger），Identity 按声明的段序。
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
