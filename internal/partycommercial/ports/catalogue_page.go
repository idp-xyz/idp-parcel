package ports

// CataloguePage 是目录上列读回的一页（ADR-0144 决定六）：本页行、下一页游标（空串即已到末页），以及与本页
// 同一组条件下的总数。迁到 ADR-0144 的各册读端口都答这一形。
type CataloguePage[Row any] struct {
	Rows  []Row
	Next  string
	Total int64
}
