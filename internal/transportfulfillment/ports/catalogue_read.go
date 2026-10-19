package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// 本文件是运输履约查阅页的伴生列表读端口（ADR-0077，票 admin-skeleton-closure-batch/05）：
// 管理台 transport-fulfillment-review 页的供数面。读口不与班次/容量/交接/交付写入方
// 共接口——扩写侧接口会拆全部写侧测试替身，伴生读端口另立（判据同 collectionremittance
// CodSubledgerCatalogueRow 那句）。
//
// 上列的是**检索列面**的照实转写：班次、周期安排、容量池、权威交接判断、有效交付、
// 总单与运输准备结果各照登记行转写，容量四量按预占子表求和派生、总单的关联数按关联子表计数派生——
// 派生是求和与计数不是判断（判据同代收分户账六位置余额）。总单一格在 ADR-0113 立册
// 之前刻意没有——没有表就没有读法，造一个恒空方法会把「无处可登」演成「登记册为空」
// （票 05 Comments 记明）；立册之后空册从此是「登记册为空」，周期安排一格随 ADR-0201
// 立册同理。运输舱单仍无册，本端口仍然没有它的方法，理由同上。
//
// 租户在方法签名上（ADR-0077 Decision 五）；Limit 必须为正，每页多大由接入面按渠道
// 契约裁决，读口只拒绝无意义的取值；空登记册如实交回空列表（Decision 四：空册本身
// 就是内容，续办是交付两个命令端点的真渠道去产生事实，不折成未配置）。

// TransportScheduleCatalogueRow 是班次登记册上列的一行：只列每个班次的当前版，方向三格、出发与到达、服务范围与
// 来历照行转写。物化来的班次带周期安排代码、物化时那一版与运行日期；临时班次这三格缺席（空串与零）。
//
// 没有执行准备键、也没有实际执行键——班次的开放/关闭订舱、暂停、延误与出发、移动、到达在存储上还没有登记格，
// 本行不代填「未出发」一类派生状态词（票 05 Comments 记明）。
type TransportScheduleCatalogueRow struct {
	ScheduleID      string
	Version         int
	Connection      string
	FromNode        string
	ToNode          string
	DepartsAt       time.Time
	ArrivesAt       time.Time
	ServiceScope    string
	Origin          string
	Template        string
	TemplateVersion int
	RunningDate     string
	RecordedAt      time.Time
}

// ScheduleTemplateCatalogueRow 是周期安排登记册上列的一行：只列每份周期安排的当前版，正文照行转写（ADR-0201）。
// 出发时刻是当地当日分钟数，时区随行给出；周期安排不是班次，本行没有任何绝对出发时刻——那要按日期物化才有。
// EffectiveUntil 空串即不设终点；Capacities 按维度词排列逐维容量声明，空即这份周期安排物化出的班次不带池。
type ScheduleTemplateCatalogueRow struct {
	Template             string
	Version              int
	Connection           string
	FromNode             string
	ToNode               string
	Weekdays             []string
	DepartureLocalMinute int
	DurationMinutes      int64
	TimeZone             string
	ServiceScope         string
	EffectiveFrom        string
	EffectiveUntil       string
	Capacities           []ScheduleTemplateCapacityRow
	RecordedAt           time.Time
}

// ScheduleTemplateCapacityRow 是周期安排上一条逐维容量声明：维度词、单位与硬上限照行转写。
type ScheduleTemplateCapacityRow struct {
	Dimension string
	Unit      string
	Capacity  int64
}

// CapacityPoolCatalogueRow 是容量池登记册上列的一行：池身份、所属班次、维度、单位与
// 有效容量照行转写；已预占、已释放、实际使用三量按预占子表逐维求和——
// CONTEXT 要求四量分别维护，三个和各自照实转写，不互相抵扣（可用量是领域按时点算
// 的判断，列面不代算）。
//
// 没有适用期间键——期间在存储上只在预占行的有效期上，池级没有登记格，本行不把某个
// 预占的有效期冒充池的适用期间（票 05 Comments 记明）。
type CapacityPoolCatalogueRow struct {
	PoolID     string
	Schedule   string
	Dimension  string
	Unit       string
	Capacity   int64
	Reserved   int64
	Released   int64
	Consumed   int64
	RecordedAt time.Time
}

// TransportHandoverCatalogueRow 是权威交接判断登记册上列的一行：逐载运对象、一行
// 一个判断版本——更正是新版本新行，原行不删，版本链因此在册面上完整可见，
// CorrectsVersion 两件成对缺席表示首登版本。
//
// Verdict 是三值封闭词（HANDED_OVER / REFUSED / PENDING_CONFIRMATION，
// domain.HandoverVerdict）；Basis 只在拒收与待确认上在场（已交接不带依据，迁移
// 0005 的逐格完备性），缺席即空串。交接边界由交出方与接收方两个参与方引用表达，
// 列面不把两方折成一个「边界」串——组合是呈现的事。
//
// ReleasingNode、ReceivingNode 与 OnwardNode 是登记方声明的交出、接收与去向节点（ADR-0213），各自缺席即空串；
// 与两个参与方引用并列透出，不互相折算。GateReading 是这一版随记的交出门禁读数的整格（ADR-0213 决定四，五格封闭词
// 同出发读数），只在`已交接`上在场；逐对象依据与关务判断标识不上列面，按键读回交接看。
type TransportHandoverCatalogueRow struct {
	Object          string
	Scope           string
	Version         string
	ReleasedBy      string
	ReceivedBy      string
	Verdict         string
	Basis           string
	CorrectsVersion string
	CorrectedAt     *time.Time
	JudgedAt        time.Time
	RecordedAt      time.Time
	ReleasingNode   string
	ReceivingNode   string
	OnwardNode      string
	GateReading     string
}

// EffectiveDeliveryCatalogueRow 是有效交付结果册上列的一行：只列当前版
// （is_current）——「已签收」不是可直接修改的状态，更正翻旧行插新行，册面回答
// 「此刻有效的交付判断是什么」，历史版本按键与版本走详情读口，不在列面展开。
//
// Proof 是交付证明引用不是证据内容：证据构成（签名、照片、验证码…）属交付证明
// 本体，列面只指名。CorrectsVersion 两件在场即这行是更正版本，指回被更正的那一版。
type EffectiveDeliveryCatalogueRow struct {
	Object          string
	Attempt         string
	Version         string
	Place           string
	Method          string
	Recipient       string
	Proof           string
	CorrectsVersion string
	CorrectedAt     *time.Time
	OccurredAt      time.Time
	RecordedAt      time.Time
}

// CarrierMasterDocumentCatalogueRow 是总单登记册上列的一行：一行一版本——撤销、替代、
// 关联重述都是新版本新行，原行不删，版本链因此在册面上完整可见；Supersedes 与
// ChangedAt 成对缺席表示首版。
//
// Standing 是三值封闭词（IN_FORCE / REVOKED / SUPERSEDED，domain.MasterDocumentStanding）；
// ReplacedBy 只在已替代上在场，缺席即空串。Commission 与 Booking 是可缺引用，缺席即
// 空串。AssociationCount 是这一版关联子表的行数——计数是派生不是判断，关联本体（哪些
// 集运单元、包裹、段）按键与版本走登记册的读口，不在列面展开。
type CarrierMasterDocumentCatalogueRow struct {
	Document         string
	Version          string
	Issuer           string
	Scope            string
	Commission       string
	Booking          string
	Standing         string
	Supersedes       string
	ReplacedBy       string
	AssociationCount int64
	ChangedAt        *time.Time
	RecordedAt       time.Time
}

// TransportPreparationCatalogueRow 是运输准备结果册上列的一行（ADR-0201 决定七）：一段计划履约段对一个载运对象按一个
// 规则版本的那一份结果，照行转写——段的方向与窗口、需求量快照、结论、选中的班次与逐候选依据都是判断当时写下的，
// 本行不按此刻的班次与池重算。只插不改，所以没有版本链要展开。
type TransportPreparationCatalogueRow struct {
	PlannedLeg   string
	Object       string
	Rule         string
	Outcome      string
	Connection   string
	FromNode     string
	ToNode       string
	WindowStart  time.Time
	WindowEnd    time.Time
	DemandSource string
	DemandAnchor string
	Demand       []PreparationDemandRow
	Selected     string
	Candidates   []PreparationCandidateRow
	JudgedAt     time.Time
	RecordedAt   time.Time
}

// PreparationDemandRow 是需求量快照的一维。
type PreparationDemandRow struct {
	Dimension string
	Amount    string
	Unit      string
}

// PreparationCandidateRow 是一个候选班次的依据行。Version 为 0 即这一班只在评估时物化、没有登记。
type PreparationCandidateRow struct {
	Schedule  string
	Version   int
	DepartsAt time.Time
	ArrivesAt time.Time
	Verdict   string
	Pools     []PreparationPoolRow
}

// PreparationPoolRow 是一个池在一个候选上的依据：判断时点的可用量与换成池单位的需求量，换不成时 Requested 为空串。
type PreparationPoolRow struct {
	Pool      string
	Dimension string
	Unit      string
	Available int64
	Requested string
	Verdict   string
}

// ReviewCatalogueRead 是运输履约查阅页的伴生列表读端口。
type ReviewCatalogueRead interface {
	ListTransportSchedules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]TransportScheduleCatalogueRow, error)
	ListScheduleTemplates(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ScheduleTemplateCatalogueRow, error)
	ListCapacityPools(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CapacityPoolCatalogueRow, error)
	ListTransportHandovers(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]TransportHandoverCatalogueRow, error)
	ListEffectiveDeliveries(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]EffectiveDeliveryCatalogueRow, error)
	ListCarrierMasterDocuments(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]CarrierMasterDocumentCatalogueRow, error)
	ListTransportPreparations(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]TransportPreparationCatalogueRow, error)
}
