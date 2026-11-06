package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// 本文件是治理登记册四册的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/12）：可排维按答复体的
// JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序（该票 spec 取舍 1）。
//
// 四册同在 GET /governance-registers 下，按 register 选择器分派，各是一册、各有册名；register 登为选择器（端点自己读、保持
// 原义、取值进游标摘要），换一册就得从第一页重取。治理登记册没有租户维（ADR-0083 决定一），取页条件里也就没有租户那一段。
// 四册都不放筛选维：登记列全是脱敏引用、没有状态或封闭词表（spec 取舍 3）；找行走 q。可排维只放缺省那一维。

// registerSelector 是四册共用的选择器键，词同端点的 register 查询参数。
var registerSelector = []string{"register"}

// AuthorityIntervalRegistryPages 是生产权威区间册的声明。缺省序是登记时间倒序，即决定三的缺省；行标识是库面代理键
// interval_id——它是主键，也是迁移前同刻收尾的那一列，只进游标、不作答复字段（AuthorityIntervalRegistryRow 头注）。
// q 在区间四维（对象范围、能力、事实类型、权威方）上做字面包含。
var AuthorityIntervalRegistryPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "governance-registers/authority-interval",
	Sorts:       []cataloguepage.SortDimension{{Name: "insertedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "insertedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Integer},
	Selectors:   registerSelector,
})

// SuspensionRegistryPages 是暂停决定册的声明。缺省序是发生时刻倒序：暂停决定没有另记登记时间，发生时刻就是它进册的
// 那一刻。行标识是暂停标识（主键）。q 在暂停标识、触发来源、暂停范围与执行人上做字面包含。
var SuspensionRegistryPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "governance-registers/suspension",
	Sorts:       []cataloguepage.SortDimension{{Name: "occurredAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "occurredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   registerSelector,
})

// ResumptionRegistryPages 是恢复决定册的声明。缺省序是决定时刻倒序，理由同暂停册。行标识是被恢复的暂停标识（主键，一个
// 暂停至多一次恢复）。q 在暂停标识与决定人上做字面包含。
var ResumptionRegistryPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "governance-registers/resumption",
	Sorts:       []cataloguepage.SortDimension{{Name: "decidedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "decidedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   registerSelector,
})

// TakeoverRegistryPages 是对象级接管册的声明。缺省序是生效时刻倒序，理由同暂停册。
//
// 接管登记表没有主键，身份是唯一约束 takeover_record_interval_identity：区间四维、起点与可为空的终点（NULLS NOT DISTINCT）。
// 行标识因此是六段——四维、起点，再加读面把终点导出成的非空一列「区间终点，开放区间取起点」（spec 取舍 6）。导出不撞值：
// 约束 takeover_record_bounds 要求闭区间的终点晚于起点，开放区间同维同起点至多一行。迁移前收尾只到起点，前五段相同而终点
// 不同的两行今天次序不定。q 在区间四维上做字面包含。
var TakeoverRegistryPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "governance-registers/takeover",
	Sorts:       []cataloguepage.SortDimension{{Name: "effectiveAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "effectiveAt", Descending: true},
	Identity: []cataloguepage.ValueKind{
		cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Text,
		cataloguepage.Instant, cataloguepage.Instant,
	},
	Selectors: registerSelector,
})
