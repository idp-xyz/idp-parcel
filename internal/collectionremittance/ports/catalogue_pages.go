package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// 本文件是本上下文列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/10）：可排维按答复体的
// JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序（该票 spec 取舍 1）。

// CodSubledgerPages 是代收分户账册（GET /collection-subledgers）的声明。
//
// 缺省序是货主客户正序，不是决定三的登记时刻倒序：迁移前按分户账四维键升序上列，同一客户名下的几本账（分责任法人、币种、
// 代收渠道）挨在一起，第一页与迁移前屏上看到的那一页同口径。openedAt（开立时刻，即分户账进册的时刻）另放作可排维，看新开立
// 的账。行标识是四维键（客户、责任法人、币种、代收渠道），即开立面主键去掉租户；按客户排时第一段与排序维重合，次序仍是迁移
// 前的四维升序。
//
// 分户账没有状态，四维又都是租户实例值、没有封闭词表，不放筛选维（spec 取舍 3「从小放」）；按客户、渠道、币种找账走 q，q 在
// 四维键与保管依据上做字面包含。各位置余额是按记账现算的派生值，不作可排维也不作筛选维：拿它排序或筛选，就得为翻不到的行
// 也扫一遍记账。
var CodSubledgerPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "collection-subledgers",
	Sorts: []cataloguepage.SortDimension{
		{Name: "customer", Kind: cataloguepage.Text},
		{Name: "openedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "customer"},
	Identity: []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Text,
	},
})
