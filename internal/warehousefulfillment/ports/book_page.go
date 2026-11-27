package ports

// Book 是按页码交回的一页。页码从 1 起。
type Book struct {
	Page     int
	PageSize int
	Total    int
}

// CutBook 从已经排好的整份里切出一页，并带回这一页的页码、页大小与总行数。
// page 小于 1 或 size 小于 1 时不切，原样交回，Book 的页码为 0。越过末页交回空切片，总数仍是整份的行数。
func CutBook[T any](rows []T, page, size int) ([]T, Book) {
	total := len(rows)
	if page < 1 || size < 1 {
		return rows, Book{}
	}
	start := (page - 1) * size
	if start > total {
		start = total
	}
	end := start + size
	if end > total {
		end = total
	}
	window := rows[start:end]
	if window == nil {
		window = []T{}
	}
	return window, Book{Page: page, PageSize: size, Total: total}
}
