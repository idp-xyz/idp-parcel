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
// 上列的是**检索列面**的照实转写：班次、容量池、权威交接判断、有效交付与总单各照
// 登记行转写，容量四量按预占子表求和派生、总单的关联数按关联子表计数派生——派生是
// 求和与计数不是判断（判据同代收分户账六位置余额）。总单一格在 ADR-0113 立册之前
// 刻意没有——没有表就没有读法，造一个恒空方法会把「无处可登」演成「登记册为空」
// （票 05 Comments 记明）；立册之后它是第五本册，空册从此是「登记册为空」。运输舱单
// 仍无册，本端口仍然没有它的方法，理由同上。
//
// 租户在方法签名上（ADR-0077 Decision 五）；Limit 必须为正，每页多大由接入面按渠道
// 契约裁决，读口只拒绝无意义的取值；空登记册如实交回空列表（Decision 四：空册本身
// 就是内容，续办是交付两个命令端点的真渠道去产生事实，不折成未配置）。

// TransportScheduleCatalogueRow 是班次登记册上列的一行：班次身份、运行方向与出发
// 时刻照行转写。
//
// 没有执行准备键、也没有实际执行键——班次的开放/关闭订舱、暂停、延误与出发、移动、
// 到达在存储上还没有登记格（迁移 0004 的班次行只登身份、方向与出发时刻），本行不
// 代填「未出发」一类派生状态词（票 05 Comments 记明）。时间范围同理：行上只有出发
// 时刻，没有范围终点。
type TransportScheduleCatalogueRow struct {
	ScheduleID string
	Direction  string
	DepartsAt  time.Time
	RecordedAt time.Time
}

// CapacityPoolCatalogueRow 是容量池登记册上列的一行：池身份、所属班次、容量维度
// 单位与有效容量照行转写；已预占、已释放、实际使用三量按预占子表逐维求和——
// CONTEXT 要求四量分别维护，三个和各自照实转写，不互相抵扣（可用量是领域按时点算
// 的判断，列面不代算）。
//
// 没有适用期间键——期间在存储上只在预占行的有效期上，池级没有登记格，本行不把某个
// 预占的有效期冒充池的适用期间（票 05 Comments 记明）。
type CapacityPoolCatalogueRow struct {
	PoolID     string
	Schedule   string
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

// ReviewCatalogueRead 是运输履约查阅页的伴生列表读端口。
type ReviewCatalogueRead interface {
	ListTransportSchedules(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]TransportScheduleCatalogueRow, error)
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
}
