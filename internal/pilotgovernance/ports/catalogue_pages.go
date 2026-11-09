package ports

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// 本文件是治理登记册四册的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/12）：可排维按答复体的
// JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序（该票 spec「缺省序保今天的
// 次序」）。
//
// 四册同在 GET /governance-registers 下，按 register 选择器分派，各是一册、各有册名；register 登为选择器（端点自己读、保持
// 原义、取值进游标摘要），换一册就得从第一页重取。治理登记册没有租户维（ADR-0083 决定一），取页条件里也就没有租户那一段。
// 登记列全是脱敏引用、没有封闭词表；有状态的两册各放出状态一维（决定四）：权威区间的开闭、暂停有没有恢复决定，都由本册（与以
// 暂停标识为键的恢复册）上的行直接读出。其余两册不放筛选维，找行走 q。可排维只放缺省那一维。

// presenceWords 是两册状态维的词表：答复体里 open、resumed 那个布尔的两种写法。
var presenceWords = []string{"true", "false"}

// registerSelector 是四册共用的选择器键，词同端点的 register 查询参数。
var registerSelector = []string{"register"}

// AuthorityIntervalRegistryPages 是生产权威区间册的声明。缺省序是登记时间倒序，即决定三的缺省；行标识是库面代理键
// interval_id——它是主键，只进游标、不作答复字段（AuthorityIntervalRegistryRow 头注）。open 是状态维：区间还没有终点即开放
// （domain.AuthorityInterval 的 To 为零值），读面由本行导出。它不说区间此刻在不在效——终点可以落在将来，那是读者拿区间判读的事。
// q 在区间四维（对象范围、能力、事实类型、权威方）上做字面包含。
var AuthorityIntervalRegistryPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "governance-registers/authority-interval",
	Sorts:       []cataloguepage.SortDimension{{Name: "insertedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "insertedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Integer},
	Filters:     []cataloguepage.FilterDimension{{Name: "open", Vocabulary: presenceWords}},
	Selectors:   registerSelector,
})

// SuspensionRegistryPages 是暂停决定册的声明。缺省序是发生时刻倒序：暂停决定没有另记登记时间，发生时刻就是它进册的
// 那一刻。行标识是暂停标识（主键）。resumed 是状态维：这条暂停名下有没有恢复决定（恢复册以暂停标识为主键，一个暂停至多一次
// 恢复；重复恢复按已恢复作答，与 GovernIncidentHandler.Resume 同一个读法）。它不说范围此刻还拦不拦新准入——恢复与暂停各有生效
// 时点，那一问按时点现算（Suspensions.FindUnresumedSuspension），不在本维。q 在暂停标识、触发来源、暂停范围与执行人上做字面包含。
var SuspensionRegistryPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "governance-registers/suspension",
	Sorts:       []cataloguepage.SortDimension{{Name: "occurredAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "occurredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters:     []cataloguepage.FilterDimension{{Name: "resumed", Vocabulary: presenceWords}},
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
// 行标识因此是六段——四维、起点，再加读面把终点导出成的非空一列「区间终点，开放区间取起点」（spec「可排维与行标识的列必须
// 非空」）。导出不撞值：约束 takeover_record_bounds 要求闭区间的终点晚于起点，开放区间同维同起点至多一行。收尾只到起点的话，
// 前五段相同而终点不同的两行次序不定，键集翻页会漏掉其中一行。q 在区间四维上做字面包含。
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
