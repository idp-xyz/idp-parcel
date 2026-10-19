package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

// 本文件是节点作业查阅页的伴生列表读端口（ADR-0077，票 admin-skeleton-closure-batch/05）：
// 管理台 node-operations-review 页的供数面。读口不与收寄/集运写入方共接口——扩写侧
// 接口会拆全部写侧测试替身，伴生读端口另立（判据同 collectionremittance
// CodSubledgerCatalogueRow 那句）。
//
// 上列的是**检索列面**的照实转写：收寄判断照登记行转写（intake/control 两 jsonb 的
// 字段逐个透出，不重建领域对象、不形成判断），集运单元照行转写。例外是集运成员关系册上的方向依据记录：它经领域
// 重建门读回成 domain.DirectionalBasisRecord——NULL 必须读作`未记录`、坏行必须响亮，照实转写那几格做不到「绝不显示
// 成依据齐全」。页面五区里实际测量
// 与节点侧交接证据两区在存储上还没有登记册，本端口刻意没有那两个方法——没有表就
// 没有读法，造一个恒空方法会把「无处可登」演成「登记册为空」，两者是不同的答案
// （票 05 Comments 记明）。
//
// 租户在方法签名上（ADR-0077 Decision 五）；Limit 必须为正，每页多大由接入面按渠道
// 契约裁决，读口只拒绝无意义的取值；空登记册如实交回空列表（Decision 四：空册本身
// 就是内容，续办是收寄命令端点的真渠道去产生事实，不折成未配置）。

// ReceptionCatalogueRow 是收寄登记册上列的一行。
//
// Kind 是收寄判断的封闭词，本册只列带收寄与控制在场的两格（INTAKE_FORMED /
// PENDING_IDENTIFICATION，迁移 0001 的在场规则）：另两格（INTAKE_NOT_FORMED /
// RECEPTION_UNDECIDED）是提交处理结果，没有收寄事实可列，也不是 CONTEXT 词条的
// 「节点收寄」——到站扫描、卸载或发现实物本身都不等于节点收寄。
//
// Control* 系列照 control jsonb 转写：ControlKind 是控制建立来源的封闭词
// （NODE_INTAKE / HANDOVER_IN，domain.ControlEstablishmentKind）；Released 两件成对
// 缺席表示实物仍在节点控制中——零值时刻是合法时间，用指针分「未转出」与「转出」
// 两态，不用零时刻兼表没发生。
//
// ServiceResultCheck 照行转写收寄登记的服务结果核对态（ADR-0186）：只有 INTAKE_FORMED 行带，待识别行恒为零值
// ServiceResultCheckAbsent（库里由迁移的 CHECK 守）。「已查、无标记」与「读不动」两行的 ServiceMarkers 同为空，
// 少了这一格两者在册面上逐字相同，而后者的空不代表没有取消与终局，审阅者照空放行就可能放走一件已取消的包裹。
// 它不像 Kind、ControlKind 那样以串转写，而用写侧同一个枚举：取值词表只在 ServiceResultCheck 一处定义，读面不另立
// 一份串。这仍是照行转写——核对态是登记时就落下的一格，不是重建出的领域对象，目录也不据它形成判断。
//
// IdentityCheck 照行转写身份核对态（ADR-0199 决定五），只有 PENDING_IDENTIFICATION 行带：未核对那一行与零候选的已核对行在本册
// 上逐字相同，少了这一格，审阅者会把「没问到」读成「查无此号」。取写侧同一个枚举，理由同 ServiceResultCheck。
type ReceptionCatalogueRow struct {
	SourceID             string
	Kind                 string
	Unit                 string
	Node                 string
	DeliveredBy          string
	ReceivedAt           time.Time
	ControlKind          string
	ControlEstablishedAt time.Time
	ControlReleasedBy    string
	ControlReleasedAt    *time.Time
	ServiceMarkers       []string
	ServiceResultCheck   ServiceResultCheck
	IdentityCheck        IdentityCheck
	RecordedAt           time.Time
}

// UnidentifiedItemCatalogueRow 是待识别实物册上列的一行：收寄登记里
// PENDING_IDENTIFICATION 那一格的身份视角。整行是永久作业记录——识别成功不删除原
// 实物记录，身份确认由 parcel-shipment 以版本化关联形成，本读口只转写登记时已有的
// 候选、冲突标记与正式包裹关联引用（Association 缺席即空串）。
//
// Unit 是节点签发的作业实物标识：待识别实物没有正式包裹身份，该标识即页面「内部
// 作业标签」列的内容（CONTEXT：节点可以自行生成和更换内部作业标签）。发现位置与
// 状况观察在收寄登记上没有登记格，本行刻意没有那两个字段（票 05 Comments 记明）。
//
// 后四格为复核页人工重核而立（ADR-0199 决定三）：JudgmentVersion 是这一行的判断版本，人工重核按它指名；SourcePreserved 说
// 来源文档在不在，Mark 是在时那一格外部标识观察原样（在线口带的空白照留）——来源保全之前落下的行号码从未留存，读面如实说
// 「来源未保全」，不拿空串冒充「设备没报号」；SucceededBy 是直接接续这一版的那一版判断版本，空即这一版仍是它那条链的当前有效
// 版本。同一件实物的待识别行不止一版时（重核、冲突各追加一版），审阅者靠它分得开哪一行已被接续。
//
// IdentityCheck 是这一行的身份核对态（ADR-0199 决定五）：未核对的候选为空不代表查无此号，同 ReceptionCatalogueRow 那一格。
type UnidentifiedItemCatalogueRow struct {
	SourceID         string
	Unit             string
	Node             string
	Candidates       []string
	IdentityConflict bool
	ReceivedAt       time.Time
	Association      string
	RecordedAt       time.Time
	JudgmentVersion  string
	SourcePreserved  bool
	Mark             string
	SucceededBy      string
	IdentityCheck    IdentityCheck
}

// IdentityReviewUnitRow 是待识别实物页按实物列的一行（ADR-0210 决定八末项与越权风险点「逐链追加会写到操作者没看过的链上」）：
// 一件有过待识别那一格的作业实物，连同它各条判断链此刻的当前有效版本。身份冲突处置逐链追加、可能写到操作者没看过的链上，复核页
// 因此要把一件实物的各链列在一起；处置命令要指名的「所处置的版本」与处置候选也都从这几版来。
type IdentityReviewUnitRow struct {
	Unit  string
	Heads []IdentityReviewHead
}

// IdentityReviewHead 是一条判断链的当前有效版本：Kind 是收寄判断的封闭词（INTAKE_FORMED / PENDING_IDENTIFICATION），候选、冲突
// 标记与关联照行转写。Disposition 只在这一版是身份冲突处置版时在场，经领域读回门重建——一份坏文档要让整页答不出，而不是悄悄
// 显示成「没处置过」。
type IdentityReviewHead struct {
	SourceID         string
	Kind             string
	Node             string
	JudgmentVersion  string
	Association      string
	Candidates       []string
	IdentityConflict bool
	ReceivedAt       time.Time
	RecordedAt       time.Time
	Disposition      domain.IdentityDisposition
}

// ConsolidationUnitCatalogueRow 是集运单元登记册上列的一行：实例阶段是三相封闭词
// （OPEN / SEALED / CLOSED，domain.ConsolidationPhase*）。
//
// MemberCount 照 members 列计当前成员数——关闭后成员可能仍留在实例里（处置转移），
// 计数不另判。LatestSeal 两件取最近一次封装快照的封签引用与封装时刻，尚未封装过则
// 成对缺席；SealCount 把「从未封装」与「重新封装过几次」分开——快照历史本体属写
// 模型，列面只给计数不展开。单元的形成时间没有登记格（行上只有库面簿记时刻，不是
// 业务事实），本行刻意不带形成时间（票 05 Comments 记明）。
//
// Opened* 与 LatestSeal* 的来源两件是 UC-NO-003 结果契约要求保存的`执行方`与`来源`：
// 开启那一组取自单元行的开启来源，恒在场；封装那一组取自最近一份快照，与 LatestSeal
// 同缺同在。**页面据此分得开导入进来的封签事实与设备扫描的封签事实**——两者此前在
// 列面上长得一模一样。
//
// 移入、移出、开封与关闭各自的执行方不在本行：它们是逐次的作业事实，一个单元会有
// 许多次，列面每格只放得下一个值。要看那些得读来源事实登记，而本册是单元册。
type ConsolidationUnitCatalogueRow struct {
	UnitID                string
	Asset                 string
	Phase                 string
	MemberCount           int64
	SealCount             int64
	OpenedSourceID        string
	OpenedBy              string
	LatestSeal            string
	LatestSealSourceID    string
	LatestSealPerformedBy string
	LatestSealedAt        *time.Time
	ClosedAt              *time.Time
}

// ConsolidationMemberCatalogueRow 是「集运成员关系」册上列的一行：一次移入或移出的来源事实（CONTEXT「集运成员
// 关系」要求记录形成、变更和解除事实）。Action 是封闭词 ADD_MEMBER / REMOVE_MEMBER；SourceID 原样带 FTI/ 标记，
// 页面据此分得开导入事实与设备事实（ADR-0089 决定四）。
//
// DirectionalBasis 只在移入行上可能有值：形成那条事实时同一笔记下的方向依据记录（票 no-directional-basis/01）。
// 移入行上的零值读作`未记录`——那条事实落册时还不记方向依据——不读作依据齐全；移出行上恒为零值。
type ConsolidationMemberCatalogueRow struct {
	SourceID         string
	Action           string
	UnitID           string
	Member           string
	PerformedBy      string
	Evidence         string
	OccurredAt       time.Time
	RecordedAt       time.Time
	DirectionalBasis domain.DirectionalBasisRecord
}

// ReviewCatalogueRead 是节点作业查阅页的伴生列表读端口。
type ReviewCatalogueRead interface {
	ListReceptions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ReceptionCatalogueRow, error)
	ListUnidentifiedItems(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]UnidentifiedItemCatalogueRow, error)
	ListConsolidationUnits(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ConsolidationUnitCatalogueRow, error)
	ListConsolidationMembers(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]ConsolidationMemberCatalogueRow, error)
	// ListIdentityReviewUnits 按实物上列待识别实物页（IdentityReviewUnitRow）：limit 数的是实物，不是版本。
	ListIdentityReviewUnits(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]IdentityReviewUnitRow, error)
}
