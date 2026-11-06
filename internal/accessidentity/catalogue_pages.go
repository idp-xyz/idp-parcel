package accessidentity

import "go.idp.xyz/idp-parcel/internal/platform/cataloguepage"

// 本文件是本上下文两册查阅的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/13）。本上下文没有
// ports 子包，查阅口 OperatorCatalogue 与 DeviceCatalogue 就在包根，声明随它们放。可排维按答复体的 JSON 字段名点名，查询
// 对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序（该票 spec「缺省序保今天的次序」）。

// OperatorCataloguePages 是操作者册查阅（GET /operators）的声明。
//
// 缺省序是登记时刻正序，不是决定三的登记时刻倒序：迁移前即按登记先后上列，管理台显的是「谁先登的」，第一页与迁移前屏上
// 那一页同口径。行标识是（发行方、主体），即操作者表的主键——它本就不含租户，一个主体至多绑一个租户。只放缺省那一维。
//
// 操作者没有状态：授予此刻生不生效是对时点的推导，不在行上存（理由见 OperatorCatalogueRow），不作筛选维。q 在主体标识与
// 绑定依据上做字面包含；授予的标识与能力面挂在各个操作者名下、不是这一册的列，不进 q。
var OperatorCataloguePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "operators",
	Sorts:       []cataloguepage.SortDimension{{Name: "recordedAt", Kind: cataloguepage.Instant}},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
})

// DeviceCataloguePages 是作业设备册查阅（GET /work-devices）的声明。
//
// 缺省序是设备标识正序，不是决定三的登记时刻倒序：迁移前即按设备标识上列（两次读回才对得上，不表示业务优先级），查阅行上
// 也没有登记时刻。行标识是设备标识，即设备表主键去掉租户，与排序维重合。状态由停用导出，放作筛选维，词表取
// DeviceCatalogueStatus 的词。q 在设备标识与作业范围引用上做字面包含。
var DeviceCataloguePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name:        "work-devices",
	Sorts:       []cataloguepage.SortDimension{{Name: "deviceId", Kind: cataloguepage.Text}},
	DefaultSort: cataloguepage.Sort{Field: "deviceId"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{{
		Name:       "status",
		Vocabulary: []string{DeviceCatalogueRegistered.String(), DeviceCatalogueDeactivated.String()},
	}},
})
