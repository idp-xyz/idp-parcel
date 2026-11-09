package ports

import (
	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是本上下文各列表读口的查询声明（ADR-0144 决定三、四、七；票 catalogue-read-pagination-rollout/05）：可排维与筛选维按
// 答复体的 JSON 字段名点名，查询对象由端点按它解出，各维落到哪一列在 postgres 读面里。缺省序保迁移前的次序
// （该票 spec「缺省序保今天的次序」）；行标识与排序同向作决胜键，取登记表主键去掉租户那几段
// （spec「行标识取登记表主键去掉租户那几段」），待确认复核清单是例外，理由在 PendingAttemptResultPages。
//
// GET /transport-fulfillment-records 按 registry 分派，每一种各是一册、各有名字。registry 登为选择器：它照旧由端点读、
// 用来选册，不并入筛选维；取值进游标摘要，拿这一册的游标去翻另一册答 400。

// RecordsRegistrySelector 是 GET /transport-fulfillment-records 选册的查询键。
const RecordsRegistrySelector = "registry"

// TransportSchedulePages 是班次册（registry=transport-schedule）的声明。
//
// 只列每个班次的当前版，行标识是（班次，版本）。缺省序是出发时刻倒序，与迁移前同一口径：班次的登记时刻是提交簿记，操作者
// 按出发找班次。origin 放出作筛选维（物化来的还是临时班次），词表取自领域 ScheduleOriginKind。
var TransportSchedulePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/transport-schedule",
	Sorts: []cataloguepage.SortDimension{
		{Name: "departsAt", Kind: cataloguepage.Instant},
		{Name: "scheduleId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "departsAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Integer},
	Filters: []cataloguepage.FilterDimension{
		{Name: "origin", Vocabulary: cataloguepage.Words[domain.ScheduleOriginKind]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// ScheduleTemplatePages 是周期安排册（registry=transport-schedule-template）的声明。
//
// 只列每份周期安排的当前版，行标识是（代码，版本）。缺省序是代码升序，与迁移前同一口径：周期安排没有业务时刻可排，登记时刻
// 另作一维可排。周期安排上没有状态一格，不放筛选维。
var ScheduleTemplatePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/transport-schedule-template",
	Sorts: []cataloguepage.SortDimension{
		{Name: "template", Kind: cataloguepage.Text},
		{Name: "recordedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "template"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Integer},
	Selectors:   []string{RecordsRegistrySelector},
})

// CapacityPoolPages 是容量池册（registry=capacity-pool）的声明。
//
// 行标识是池标识。缺省序是池标识升序，与迁移前同一口径：池上没有业务时刻（登记时刻是提交簿记），池标识由班次与维度派生，
// 按它排同一班次的各维挨在一起；也只放这一维可排。dimension 放出作筛选维，词表取自领域 CapacityDimension。
var CapacityPoolPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/capacity-pool",
	Sorts: []cataloguepage.SortDimension{
		{Name: "poolId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "poolId"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "dimension", Vocabulary: cataloguepage.Words[domain.CapacityDimension]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// TransportHandoverPages 是权威交接判断册（registry=transport-handover）的声明。
//
// 一行一版本，行标识是（对象，范围，版本）。缺省序是裁决时刻倒序，与迁移前同一口径。交接判断按载运对象逐个成立，是按包裹
// 计量的大册，total 照给精确值（该票 spec「total 一律给精确值」）。verdict 放出作筛选维，词表取自领域 HandoverVerdict。
var TransportHandoverPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/transport-handover",
	Sorts: []cataloguepage.SortDimension{
		{Name: "judgedAt", Kind: cataloguepage.Instant},
		{Name: "object", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "judgedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "verdict", Vocabulary: cataloguepage.Words[domain.HandoverVerdict]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// EffectiveDeliveryPages 是有效交付结果册（registry=effective-delivery）的声明。
//
// 只列当前版，行标识是（对象，尝试，版本）。缺省序是交付发生时刻倒序，与迁移前同一口径；按包裹计量的大册。gateReading 放出作
// 筛选维：交付门禁读数整格单成一列（ADR-0213 决定五），就是为了按格查「哪些交付被阻断」；词表取自领域 GateAnswer。
var EffectiveDeliveryPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/effective-delivery",
	Sorts: []cataloguepage.SortDimension{
		{Name: "occurredAt", Kind: cataloguepage.Instant},
		{Name: "object", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "occurredAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "gateReading", Vocabulary: cataloguepage.Words[domain.GateAnswer]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// CarrierMasterDocumentPages 是总单册（registry=carrier-master-document）的声明。
//
// 一行一版本，行标识是（总单，版本）。缺省序是登记时刻倒序，与迁移前同一口径。standing 放出作筛选维，词表取自领域
// MasterDocumentStanding。
var CarrierMasterDocumentPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/carrier-master-document",
	Sorts: []cataloguepage.SortDimension{
		{Name: "recordedAt", Kind: cataloguepage.Instant},
		{Name: "document", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "standing", Vocabulary: cataloguepage.Words[domain.MasterDocumentStanding]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// TransportPreparationPages 是运输准备结果册（registry=transport-preparation）的声明。
//
// 行标识是（段，对象，规则版本）。缺省序是判断时刻倒序，与迁移前同一口径。准备结果按载运对象逐个判，是按包裹计量的大册。
// outcome 放出作筛选维，词表取自领域 PreparationOutcome。
var TransportPreparationPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/transport-preparation",
	Sorts: []cataloguepage.SortDimension{
		{Name: "judgedAt", Kind: cataloguepage.Instant},
		{Name: "object", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "judgedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "outcome", Vocabulary: cataloguepage.Words[domain.PreparationOutcome]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// DeliveryDispatchPendingPages 是派送待形成册（registry=delivery-dispatch-pending）的声明。
//
// 只列待形成的，行标识是（段，对象）。缺省序是最近一次问的时刻升序，与迁移前同一口径：这是要人去催所有者的队列，最久没问的
// 先看（spec「缺省序保今天的次序」：待办队列照旧最早的在前）。册上只有待形成一种状态，不放筛选维。
var DeliveryDispatchPendingPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/delivery-dispatch-pending",
	Sorts: []cataloguepage.SortDimension{
		{Name: "lastAskedAt", Kind: cataloguepage.Instant},
		{Name: "firstAnsweredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "lastAskedAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Selectors:   []string{RecordsRegistrySelector},
})

// DeliveryDispatchReaskCadencePages 是重问间隔册（registry=delivery-dispatch-reask-cadence）的声明。
//
// 一行一版，行标识是版本。缺省序是登记时刻倒序，与迁移前同一口径：第一行就是生效的那一版。只有登记时刻一维可排，不放筛选维。
var DeliveryDispatchReaskCadencePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/delivery-dispatch-reask-cadence",
	Sorts: []cataloguepage.SortDimension{
		{Name: "recordedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "recordedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{RecordsRegistrySelector},
})

// ActualFulfillmentUnformedPages 是实际履约未形成册（registry=actual-fulfillment-unformed）的声明。
//
// 只列还未形成的，行标识是（段，对象）。缺省序是参与终点升序，与迁移前同一口径：这是要人去补的清单，最早结束的先看。委托另作
// 一维可排：续办常是给一份委托换版，同一委托的几件挨在一起。reason 放出作筛选维（各格续办不同），词表取自
// ActualFulfillmentUnformedReason。
var ActualFulfillmentUnformedPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/actual-fulfillment-unformed",
	Sorts: []cataloguepage.SortDimension{
		{Name: "endedAt", Kind: cataloguepage.Instant},
		{Name: "commission", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "endedAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "reason", Vocabulary: cataloguepage.Words[ActualFulfillmentUnformedReason]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// BookingRequestPages 是订舱册（registry=booking-request）的声明。
//
// 一次订舱一行，行标识是订舱标识。申请量记在 quantity 一格，订多少都只占一行，不按包裹计量，不是该票 spec「total 一律给精确值」
// 点名的那类大册。缺省序是申请时刻倒序，与迁移前同一口径。不放状态维：状态词是读口从名下各份承运答复里选出当前那一份、再比
// 接受量与申请量派生的，选法只在 domain.CurrentCarrierAcceptance 一处定义，SQL 里再选一遍就是第二处；先读后筛又让 total 不再
// 是真数。这一类的取舍在 ADR-0372：要服务端按它筛，先在写面把领域判出的状态词随写入落列。
var BookingRequestPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/booking-request",
	Sorts: []cataloguepage.SortDimension{
		{Name: "requestedAt", Kind: cataloguepage.Instant},
		{Name: "booking", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "requestedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Selectors:   []string{RecordsRegistrySelector},
})

// BookingAnswerBounceBookingFilter 与 BookingAnswerBounceSourceFilter 是挡回册无词表的订舱、答复方两维的名字，也是它们的
// 查询键；声明、端点判据与 postgres 落列都引这两个常量。落列写错了名 MustBind 当场 panic，端点判据却只是按名从查询对象里取值：
// 名字对不上就什么也取不到，写坏的筛选不再被拒，静默放宽成全册。
const (
	BookingAnswerBounceBookingFilter = "booking"
	BookingAnswerBounceSourceFilter  = "source"
)

// BookingAnswerBouncePages 是承运答复挡回册（registry=booking-answer-bounce）的声明。
//
// 行标识是（答复方，订舱，答复标识，依据）。缺省序是最近一次挡回时刻倒序，与迁移前同一口径；次数另作一维可排，看谁在反复答。
// booking 与 source 是迁移前就有的筛选参数，照旧收下作筛选维：订舱一维让订舱册的一行链得到它名下的挡回，答复方一维让运营
// 看一个承运商反复在答什么。它们没有词表，取值立不立得起来由端点照迁移前的判据判。basis 放出作筛选维，词表取自
// BookingAnswerBounceBasis。
var BookingAnswerBouncePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/booking-answer-bounce",
	Sorts: []cataloguepage.SortDimension{
		{Name: "lastBouncedAt", Kind: cataloguepage.Instant},
		{Name: "count", Kind: cataloguepage.Integer},
	},
	DefaultSort: cataloguepage.Sort{Field: "lastBouncedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Integer},
	Filters: []cataloguepage.FilterDimension{
		{Name: BookingAnswerBounceBookingFilter},
		{Name: BookingAnswerBounceSourceFilter},
		{Name: "basis", Vocabulary: cataloguepage.Words[BookingAnswerBounceBasis]()},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// PendingAttemptResultPages 是待确认复核清单（registry=pending-attempt-result）的声明。
//
// 揽收与派送两侧合并成一册，行标识是（一侧，尝试，对象，来源身份）。它不是哪一张登记表的主键去掉租户：揽收一侧的登记键是
// （来源身份，对象），派送一侧的是（尝试，对象）、来源身份恒为空串；单取哪一侧的键，另一侧都会撞。加上一侧一段之后两侧的键
// 合起来唯一。缺省序是发生时刻升序，与迁移前同一口径：复核清单最早的先看。side 放出作筛选维。
var PendingAttemptResultPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-records/pending-attempt-result",
	Sorts: []cataloguepage.SortDimension{
		{Name: "occurredAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "occurredAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text, cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "side", Vocabulary: []string{PendingAttemptSidePickup, PendingAttemptSideDelivery}},
	},
	Selectors: []string{RecordsRegistrySelector},
})

// ExternalTrackingFactPages 是外部承运轨迹事实当前版（GET /transport-fulfillment-external-tracking-facts）的声明。
//
// source 与 view 是迁移前就有的必给参数，登为选择器，照旧由端点在准入之后读、判：源是这本册子的分组维，视图选待判断的当前版还是
// 全部当前版。行标识是（事实，版本）。缺省序是发生时刻升序，与迁移前同一口径：判断人按事实发生的先后一条条判；接收时刻另作
// 一维可排。effectiveBasis 放出作筛选维，词表取自领域 EffectiveTimeBasis。一条外部轨迹事件一行，是按事件计量的大册。
var ExternalTrackingFactPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "transport-fulfillment-external-tracking-facts",
	Sorts: []cataloguepage.SortDimension{
		{Name: "occurredAt", Kind: cataloguepage.Instant},
		{Name: "receivedAt", Kind: cataloguepage.Instant},
	},
	DefaultSort: cataloguepage.Sort{Field: "occurredAt"},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text, cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "effectiveBasis", Vocabulary: cataloguepage.Words[domain.EffectiveTimeBasis]()},
	},
	Selectors: []string{"source", "view"},
})
