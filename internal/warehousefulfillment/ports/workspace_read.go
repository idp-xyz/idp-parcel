package ports

import (
	"time"

	"go.idp.xyz/idp-parcel/internal/platform/cataloguepage"
)

// SettingsOverview 是本租户全部仓配规则的实时配置总览。未配置项如实返回 false 或空切片，不编造默认。
type SettingsOverview struct {
	AllocationStrategy *AllocationStrategyOverview
	ShortagePolicies   []ShortagePolicyOverview
	ApprovalDuty       *ApprovalDutyOverview
	DisposalRules      []DisposalRuleOverview
	ChargeUnits        []ChargeUnitsOverview
	ChargeAging        []ChargeAgingOverview
	ChargeProjects     []ChargeProjectOverview
	AgentConnectors    []AgentConnectorOverview
	// ToteRequirements 是登记过「拣货必须用周转箱」的仓各自最新的一版；没登记的仓不在里面，那是未配置。
	ToteRequirements []ToteRequirementOverview
	// WaveRules 是各仓登记过的成波规则各自最新的一版，停用的也在里面；没登记规则的仓不在里面。
	WaveRules []WaveRuleOverview
}

type ToteRequirementOverview struct {
	WarehouseCode string
	Revision      int
	Required      bool
	Operator      string
	RegisteredAt  time.Time
}

type AllocationStrategyOverview struct {
	Revision     int
	Operator     string
	RegisteredAt time.Time
}

// ShortagePolicyOverview 是一位货主当前的缺货处置。Kind 与 PartialShipment 各说各的，没说的那一格是空串。
type ShortagePolicyOverview struct {
	OwnerAccount    string
	Revision        int
	Kind            string
	PartialShipment string
	Operator        string
	RegisteredAt    time.Time
}

type ApprovalDutyOverview struct {
	Revision         int
	DistinctSubjects bool
	Operator         string
	RegisteredAt     time.Time
}

type DisposalRuleOverview struct {
	WarehouseCode      string
	Revision           int
	ClaimWindowSeconds int64
	DisposalMethods    []string
	Registrant         string
	RegisteredAt       time.Time
}

type ChargeUnitsOverview struct {
	WarehouseCode string
	Revision      int
	WeightUnit    string
	VolumeUnit    string
	RegisteredAt  time.Time
}

type ChargeAgingOverview struct {
	WarehouseCode string
	Basis         string
	RegisteredAt  time.Time
}

type ChargeProjectOverview struct {
	WarehouseCode string
	ProjectName   string
	RegisteredAt  time.Time
}

type AgentConnectorOverview struct {
	WarehouseCode string
	RegisteredAt  time.Time
}

// PartialShipmentDetail 是一张请求此刻适用的部分发货规则，连同三层各自写的是什么。
// Rule 与 Source 为空是哪一层都没写明。
type PartialShipmentDetail struct {
	Rule            string
	Source          string
	Order           string
	Owner           string
	Consent         string
	ConsentBasis    string
	ConsentOperator string
	ConsentAt       *time.Time
}

// ShortPickDisposalDetail 是这张请求拣不齐之后照部分发货规则办过的那一次；没办过就没有。
type ShortPickDisposalDetail struct {
	Rule             string
	Source           string
	Wanted           int64
	Shipped          int64
	RemainderRequest string
	Operator         string
	RecordedAt       time.Time
}

// OutboundToteDetail 是一张出库请求绑过的一个周转箱。
type OutboundToteDetail struct {
	ToteCode   string
	BoundAt    time.Time
	ReleasedAt *time.Time
}

// OutboundRequestDetail 是单张出库单的 360° 全链路详情，供管理台与作业台复核打包直接使用。
type OutboundRequestDetail struct {
	PartialShipment PartialShipmentDetail
	// ShortPickDisposal 为空是没办过拣不齐的处置。
	ShortPickDisposal *ShortPickDisposalDetail
	// Totes 是这张请求绑过的周转箱，按绑上的先后；还没放开的 ReleasedAt 为空。
	Totes         []OutboundToteDetail
	Request       OutboundRequestRow
	Lines         []OutboundRequestDetailLine
	Reservations  []OutboundReservationDetail
	Waves         []string
	PickTasks     []OutboundPickTaskDetail
	Checks        []OutboundCheckDetail
	Parcels       []OutboundParcelDetail
	Confirmations []OutboundConfirmationDetail
	ReturnTasks   []OutboundReturnTaskDetail
}

type OutboundRequestDetailLine struct {
	LineIndex   int
	GoodsCode   string
	Unit        string
	StockState  string
	LabelAlias  string
	BatchNumber string
	ExpiryDate  string
	Quantity    int64
}

type OutboundReservationDetail struct {
	LineIndex     int
	GoodsCode     string
	StockState    string
	LabelAlias    string
	BatchNumber   string
	ExpiryDate    string
	Quantity      int64
	FromUnlabeled int64
	Wanted        int64
}

type OutboundPickTaskDetail struct {
	TaskID          string
	WaveID          string
	LocationCode    string
	StagingLocation string
	GoodsCode       string
	Unit            string
	StockState      string
	InboundBatchID  string
	Quantity        int64
	Picked          int64
	Status          string
	FactID          string
}

type OutboundCheckDetail struct {
	FactID    string
	Observed  int64
	Matched   bool
	CheckedAt time.Time
}

type OutboundParcelLineDetail struct {
	GoodsCode string
	Unit      string
	Quantity  int64
}

type OutboundParcelDetail struct {
	ParcelSequence    int
	PackagingMaterial string
	WeightValue       string
	WeightUnit        string
	LengthValue       string
	WidthValue        string
	HeightValue       string
	DimensionUnit     string
	ShipmentRequestID string
	ConfirmationState string
	CutoffState       string
	OwnerLabel        string
	Lines             []OutboundParcelLineDetail
}

type OutboundConfirmationDetail struct {
	FactID           string
	ParcelSequence   int
	Kind             string
	Offline          bool
	Missing          string
	ExternalNumber   string
	PickupParty      string
	PickupCredential string
	OccurredAt       time.Time
	CorrectionKind   string
	CorrectionStatus string
}

type OutboundReturnTaskDetail struct {
	ReturnID       string
	ParcelSequence int
	LocationCode   string
	GoodsCode      string
	Unit           string
	StockState     string
	InboundBatchID string
	Quantity       int64
	Status         string
	FactID         string
}

// StockFreezePages 是运营分页查阅库存冻结单的查询声明。
var StockFreezePages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-freezes",
	Sorts: []cataloguepage.SortDimension{
		{Name: "frozenAt", Kind: cataloguepage.Instant},
		{Name: "freezeId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "frozenAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "ownerAccount"},
		{Name: "warehouseCode"},
		{Name: "target", Vocabulary: []string{"GOODS", "CARTON"}},
		{Name: "status", Vocabulary: []string{"FROZEN", "UNFROZEN"}},
	},
})

type StockFreezeListRow struct {
	FreezeID         string
	DecisionID       string
	OwnerAccount     string
	WarehouseCode    string
	Target           string
	LocationCode     string
	GoodsCode        string
	Unit             string
	StockState       string
	LabelAlias       string
	InboundBatchID   string
	BatchNumber      string
	ExpiryDate       string
	CartonNumber     string
	Quantity         int64
	Reason           string
	Operator         string
	FrozenAt         time.Time
	Status           string
	UnfreezeOperator string
	UnfrozenAt       *time.Time
	UnfreezeNote     string
}

// StockCountPages 是运营分页查阅盘点任务的查询声明。
var StockCountPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-counts",
	Sorts: []cataloguepage.SortDimension{
		{Name: "createdAt", Kind: cataloguepage.Instant},
		{Name: "countId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "createdAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "warehouseCode"},
		{Name: "ownerAccount"},
		{Name: "status", Vocabulary: []string{"PENDING", "COUNTING", "COUNTED", "VOID"}},
	},
})

type StockCountRow struct {
	CountID            string
	DecisionID         string
	WarehouseCode      string
	OwnerAccount       string
	Note               string
	Operator           string
	CreatedAt          time.Time
	Status             string
	StartedBy          string
	StartedAt          *time.Time
	CompletedBy        string
	CompletedAt        *time.Time
	VoidedBy           string
	VoidedAt           *time.Time
	VoidReason         string
	Locations          []string
	Goods              []string
	EntryCount         int
	DifferenceCount    int
	PendingDifferences int
}

type StockCountDetail struct {
	Count       StockCountRow
	Entries     []StockCountEntryDetail
	Differences []StockCountDifferenceDetail
}

type StockCountEntryLineDetail struct {
	OwnerAccount string
	GoodsCode    string
	Unit         string
	StockState   string
	LabelAlias   string
	BatchNumber  string
	ExpiryDate   string
	Quantity     int64
	Observed     string
}

type StockCountEntryDetail struct {
	FactID         string
	LocationCode   string
	OccurredAt     time.Time
	DeviceSequence int64
	DeviceID       string
	OperatorID     string
	ReceivedAt     time.Time
	Lines          []StockCountEntryLineDetail
}

type StockCountDifferenceDetail struct {
	DifferenceID    string
	CountID         string
	OwnerAccount    string
	WarehouseCode   string
	LocationCode    string
	GoodsCode       string
	Unit            string
	StockState      string
	LabelAlias      string
	BatchNumber     string
	ExpiryDate      string
	BookQuantity    int64
	CountedQuantity int64
	Status          string
	ApprovedBy      string
	ApprovedAt      *time.Time
	ApprovalNote    string
	RejectedBy      string
	RejectedAt      *time.Time
	RejectReason    string
}

// StockAdjustmentPages 是运营分页查阅库存调整单的查询声明。
var StockAdjustmentPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-adjustments",
	Sorts: []cataloguepage.SortDimension{
		{Name: "submittedAt", Kind: cataloguepage.Instant},
		{Name: "adjustmentId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "submittedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "ownerAccount"},
		{Name: "warehouseCode"},
		{Name: "status", Vocabulary: []string{"PENDING", "POSTED", "REJECTED"}},
	},
})

type StockAdjustmentLineDetail struct {
	LineIndex      int
	Kind           string
	LocationCode   string
	GoodsCode      string
	Unit           string
	StockState     string
	LabelAlias     string
	BatchNumber    string
	ExpiryDate     string
	InboundBatchID string
	Quantity       int64
	Basis          string
}

type StockAdjustmentRow struct {
	AdjustmentID  string
	OwnerAccount  string
	WarehouseCode string
	Reason        string
	Operator      string
	SubmittedAt   time.Time
	Status        string
	ApprovedBy    string
	ApprovedAt    *time.Time
	ApprovalNote  string
	RejectedBy    string
	RejectedAt    *time.Time
	RejectReason  string
	Lines         []StockAdjustmentLineDetail
}

// JobDiscrepancyPages 是运营分页查阅库内作业差异（stock_discrepancy）的查询声明。
var JobDiscrepancyPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-job-discrepancies",
	Sorts: []cataloguepage.SortDimension{
		{Name: "discrepancyId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "discrepancyId", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "ownerAccount"},
		{Name: "warehouseCode"},
		{Name: "status", Vocabulary: []string{"PENDING", "APPROVED", "REJECTED"}},
	},
})

type JobDiscrepancyRow struct {
	DiscrepancyID  string
	FactID         string
	LinePosition   int
	OwnerAccount   string
	WarehouseCode  string
	SourceLocation string
	LocationCode   string
	GoodsCode      string
	Unit           string
	StockState     string
	LabelAlias     string
	BatchNumber    string
	ExpiryDate     string
	InboundBatchID string
	CartonNumber   string
	Note           string
	Quantity       int64
	ShortOnBook    int64
	HeldQuantity   int64
	CartonMissing  bool
	Status         string
	ApprovedBy     string
	ApprovedAt     *time.Time
	ApprovalNote   string
	RejectedBy     string
	RejectedAt     *time.Time
	RejectReason   string
}

// InstructionPages 是运营分页查阅加工与换标指令的查询声明。
var InstructionPages = cataloguepage.MustCatalogue(cataloguepage.Spec{
	Name: "warehouse-fulfillment-instructions",
	Sorts: []cataloguepage.SortDimension{
		{Name: "receivedAt", Kind: cataloguepage.Instant},
		{Name: "instructionId", Kind: cataloguepage.Text},
	},
	DefaultSort: cataloguepage.Sort{Field: "receivedAt", Descending: true},
	Identity:    []cataloguepage.ValueKind{cataloguepage.Text},
	Filters: []cataloguepage.FilterDimension{
		{Name: "ownerAccount"},
		{Name: "warehouseCode"},
		{Name: "kind", Vocabulary: []string{"PROCESS", "DISASSEMBLE", "RELABEL"}},
		{Name: "status", Vocabulary: []string{"PENDING", "DONE", "VOID"}},
	},
})

type InstructionComponentDetail struct {
	GoodsCode string
	Quantity  int64
}

type InstructionExecutionDetail struct {
	FactID         string
	DeviceID       string
	OperatorID     string
	OccurredAt     time.Time
	DeviceSequence int64
	DoneQuantity   int64
	MovementID     string
}

type InstructionRow struct {
	InstructionID string
	OwnerAccount  string
	WarehouseCode string
	Kind          string
	Status        string
	Origin        string
	GoodsCode     string
	Unit          string
	Quantity      int64
	DoneQuantity  int64
	FromAlias     string
	ToAlias       string
	BatchNumber   string
	ExpiryDate    string
	Reason        string
	Basis         string
	Operator      string
	FixedRevision int
	VoidReason    string
	VoidedAt      *time.Time
	ReceivedAt    time.Time
	Components    []InstructionComponentDetail
	Executions    []InstructionExecutionDetail
}

// ReturnsOverview 是退货预报、退件到仓与回库处置的全量运营视图。
type ReturnsOverview struct {
	PreAdvices []ReturnPreAdviceRow
	Receipts   []ReturnReceiptRow
}

type ReturnPreAdviceLineRow struct {
	GoodsCode string
	Unit      string
	Quantity  int64
}

type ReturnPreAdviceRow struct {
	AdviceID         string
	DocumentNumber   string
	NumberingAbsence string
	OwnerAccount     string
	WarehouseCode    string
	Origin           string
	TrackingNumber   string
	Basis            string
	Operator         string
	ReceivedAt       time.Time
	Lines            []ReturnPreAdviceLineRow
}

type ReturnDispositionRow struct {
	DecisionID  string
	Destination string
	Basis       string
	Operator    string
	Origin      string
	DecidedAt   time.Time
}

type ReturnReceiptRow struct {
	FactID         string
	WarehouseCode  string
	LocationCode   string
	TrackingNumber string
	OwnerAccount   string
	AdviceID       string
	ArrivalID      string
	GoodsCode      string
	Unit           string
	StockState     string
	Quantity       int64
	Outcome        string
	OccurredAt     time.Time
	Disposition    *ReturnDispositionRow
}

// AgentOverview 是代理仓连接器、库存声明（带新鲜度）与代理仓回执记录的运营总览。
type AgentOverview struct {
	Connectors []AgentConnectorOverview
	Statements []AgentStatementRow
	Results    []AgentResultRow
}

type AgentStatementRow struct {
	SourceID      string
	StatedAt      time.Time
	ValidUntil    time.Time
	WarehouseCode string
	OwnerAccount  string
	GoodsCode     string
	Unit          string
	Quantity      int64
	Freshness     string
}

type AgentResultRow struct {
	ResultID      string
	RequestID     string
	WarehouseCode string
	Kind          string
	WeightValue   string
	WeightUnit    string
	LengthValue   string
	WidthValue    string
	HeightValue   string
	DimensionUnit string
	ReportedAt    time.Time
}
