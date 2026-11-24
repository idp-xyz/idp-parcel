package ports

import (
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// OutboundRequestPages 是运营查阅出库请求列表的查询声明（ADR-0144）。
// 筛选维覆盖货主、仓、发运方式与状态；q 覆盖请求标识、单据号、货主、仓、收件人与来源键。
var OutboundRequestPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-outbound-request-list",
	Sorts: []cataloguepage.SortDimension{
		{Name: "receivedAt", Kind: cataloguepage.Instant},
		{Name: "requestId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "receivedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "ownerAccount"},
		{Name: "warehouseCode"},
		{Name: "dispatch", Vocabulary: []string{"PRODUCT_NETWORK", "OWNER_LABEL", "PICKUP"}},
		{Name: "status", Vocabulary: []string{
			"RESERVED", "PARTIALLY_RESERVED", "REJECTED", "PENDING_POLICY", "HANDED_OFF",
			"FORWARDED", "SPLIT", "ALLOCATED", "PICKED", "CHECKED", "PACKED",
			"SHIPPED", "CANCELLED", "AWAITING_RETURN", "PARTIALLY_CANCELLED",
		}},
	},
})

// OutboundRequestLineRow 是出库请求列表项带出的一条商品明细。
type OutboundRequestLineRow struct {
	GoodsCode  string
	Unit       string
	StockState string
	Quantity   int64
}

// OutboundRequestRow 是运营查阅出库请求列表的一行。
type OutboundRequestRow struct {
	RequestID                string
	DocumentNumber           string
	NumberingAbsence         string
	OwnerAccount             string
	WarehouseCode            string
	Origin                   string
	RequestKey               string
	Dispatch                 string
	Service                  string
	Recipient                string
	Address                  string
	Basis                    string
	Operator                 string
	Status                   string
	ParentRequestID          string
	PlatformStocking         bool
	PrefetchShipmentID       string
	PrefetchDeclaredParcelID string
	ReceivedAt               time.Time
	Lines                    []OutboundRequestLineRow
	ReservedQuantity         int64
	WantedQuantity           int64
}

// WavePages 是运营查阅波次列表的查询声明（ADR-0144）。
// 筛选维覆盖仓与是否预取；q 覆盖波次标识、仓与操作者。
var WavePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-wave-list",
	Sorts: []cataloguepage.SortDimension{
		{Name: "releasedAt", Kind: cataloguepage.Instant},
		{Name: "waveId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "releasedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
		{Name: "prefetch", Vocabulary: cataloguepage.BooleanVocabulary()},
	},
})

// WaveTaskRow 是波次列表项带出的一条拣货任务。
type WaveTaskRow struct {
	TaskID          string
	RequestID       string
	LocationCode    string
	StagingLocation string
	GoodsCode       string
	Unit            string
	StockState      string
	Quantity        int64
	Picked          int64
	Status          string
}

// WaveRow 是运营查阅波次列表的一行。
type WaveRow struct {
	WaveID        string
	WarehouseCode string
	Operator      string
	Prefetch      bool
	ReleasedAt    time.Time
	Requests      []string
	Tasks         []WaveTaskRow
}

// ChargeOccurrencePages 是运营查阅仓储作业收费发生项列表的查询声明（ADR-0144）。
// 一份发生项一行，取最新一版（head_version）。不带金额。
var ChargeOccurrencePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-charge-occurrences",
	Sorts: []cataloguepage.SortDimension{
		{Name: "businessTime", Kind: cataloguepage.Instant},
		{Name: "occurrenceId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "businessTime", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "ownerAccount"},
		{Name: "warehouseCode"},
		{Name: "kind", Vocabulary: []string{
			"INBOUND_HANDLING", "OUTBOUND_HANDLING", "STORAGE", "RETURN_HANDLING",
			"VALUE_ADDED", "PROCESSING", "DESTRUCTION",
		}},
	},
})

// ChargeOccurrenceRow 是仓储作业收费发生项列表的一行（最新一版）。
type ChargeOccurrenceRow struct {
	OccurrenceID string
	Version      int
	Supersedes   int
	Correction   string
	Kind         string
	OwnerAccount string
	Warehouse    string
	SourceFact   string
	BusinessTime time.Time
	WeightUnit   string
	VolumeUnit   string
	Measures     []ChargeMeasure
	Attributes   []ChargeAttribute
	LineCount    int
}
