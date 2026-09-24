package cataloguepage

// Page 是答复体在各册既有集合键之外多出的 page 一格（ADR-0144 决定五）。
type Page struct {
	// Size 是本次使用的页大小，不是本页实际行数；客户端据它与 Total 算页数。
	Size int `json:"size"`
	// Next 为 null 即已到末页。
	Next *string `json:"next"`
	// Total 与本页出自同一组筛选条件。为 null 表示该读口不给总数，客户端不显示、也不去猜。
	Total *int64 `json:"total"`
}

// Trim 把读面照 size+1 取回的行切成本页：多出来的那一行只用来判断有没有下一页，不交给调用方。
// size 即读面的页大小，由它的 limit 门禁保证为正。
func Trim[T any](rows []T, size int) (page []T, more bool) {
	if len(rows) > size {
		return rows[:size], true
	}
	return rows, false
}

// NewPage 拼一格 page：next 为空串即已到末页，total 是与本页同一组条件下的精确总数。
func NewPage(size int, next string, total int64) Page {
	page := Page{Size: size, Total: &total}
	if next != "" {
		page.Next = &next
	}
	return page
}
