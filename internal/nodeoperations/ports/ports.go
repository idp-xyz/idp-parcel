// Package ports 定义 node-operations 应用层与外界的边界。领域包不依赖 HTTP/pgx 的
// 纪律与其余上下文一致。
package ports

import (
	"context"
	"errors"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
)

type Clock interface {
	Now() time.Time
}

// ExternalMarkObservation 是现场对实物外部标识的一次观察。外部条码只是线索，不自动
// 等于正式包裹（UC-NO-002 输入契约）。
type ExternalMarkObservation struct {
	Mark string
}

// ParcelIdentityView 用正式包裹与外部标识关联核对身份（PS 侧只读引用）。答复决定走向：唯一命中→关联；零个候选→
// 待识别；其余→身份冲突。依赖调不通作为错误返回。
type ParcelIdentityView interface {
	ResolveParcelIdentity(
		ctx context.Context,
		tenant domain.TenantID,
		observation ExternalMarkObservation,
	) (ParcelIdentityAnswer, error)
}

// ParcelIdentityAnswer 是一次身份核对的答复：PS 交回的包裹候选，加 PS 随候选答出的歧义信号。
type ParcelIdentityAnswer struct {
	Candidates []domain.ParcelAssociationReference
	// Ambiguous 说号码另被一份还在`已提交`的委托报过（ADR-0195），那份委托没接受，也没被拒绝或撤回。报过的那件不是
	// 正式包裹，不在候选里；这一格只让恰一个候选不再算唯一命中——客户给的号只在它自己的编号空间里算数，别人也报了
	// 这个号，命中的那一件可能不是这件实物。
	Ambiguous bool
}

// Unique 交回唯一命中的那件包裹：恰一个候选，且没有歧义信号。认唯一只经它，不数候选——收寄与重核走同一份身份判断
// （ADR-0199 决定三），数候选的那一处会漏掉信号。
func (answer ParcelIdentityAnswer) Unique() (domain.ParcelAssociationReference, bool) {
	if len(answer.Candidates) != 1 || answer.Ambiguous {
		return domain.ParcelAssociationReference{}, false
	}
	return answer.Candidates[0], true
}

// ReceptionKey 是收寄判断的幂等键：同一来源身份和内容返回已有处理结果。
type ReceptionKey struct {
	TenantID domain.TenantID
	SourceID string
}

// ReceptionRecordKind 是越过提交边界的记录种类。除 RecordMergedIntoExistingIntake 之外都是判断走向（结果语义契约的前四格；
// 已有结果与来源冲突是应答不是记录）；它不是判断，见它自己的注。
type ReceptionRecordKind uint8

const (
	ReceptionRecordKindInvalid ReceptionRecordKind = iota
	RecordIntakeFormed
	RecordPendingIdentification
	RecordIntakeNotFormed
	RecordReceptionUndecided
	// RecordMergedIntoExistingIntake 是「并入已有收寄」（ADR-0199 决定四）：同一作业实物在同一节点已有当前收寄时，这次送站
	// 不形成新判断，只把来源照样保全、指名并入的那一版。它不带收寄与控制、不签判断版本、不指名前版——并入不是后继，占了前版
	// 就把那条链的当前有效版本挪到一行不是判断的记录上。种类不进 outcome 词表：送站的一方收到的是已有结果。
	RecordMergedIntoExistingIntake
)

func (kind ReceptionRecordKind) String() string {
	switch kind {
	case RecordIntakeFormed:
		return "INTAKE_FORMED"
	case RecordPendingIdentification:
		return "PENDING_IDENTIFICATION"
	case RecordIntakeNotFormed:
		return "INTAKE_NOT_FORMED"
	case RecordReceptionUndecided:
		return "RECEPTION_UNDECIDED"
	case RecordMergedIntoExistingIntake:
		return "MERGED_INTO_EXISTING_INTAKE"
	default:
		return ""
	}
}

// ParcelServiceResults 是 PS 对一件正式包裹此刻服务结果的只读答复（UC-NO-002「服务结果引用」；票 operator-channel/25）：
// 已成立的取消决定与当前有效终局，各只在存在时有值。两样都不阻止接收，只限制后续方向性作业。
type ParcelServiceResults struct {
	// Cancellation 是已成立取消决定的标识；空即 PS 没有取消决定。
	Cancellation string
	// FinalKind / FinalVersion 是当前有效终局的种类与版本；两格同空即 PS 没有当前有效终局。
	FinalKind    string
	FinalVersion string
}

// 标记串的两种前缀只在这里出现：合成入口只有 Markers 一处，别处既不拼也不拆（ADR-0186）。
const (
	serviceMarkerCancelledPrefix = "CANCELLED/"
	serviceMarkerFinalPrefix     = "FINAL/"
)

// Markers 是收寄记录上服务结果标记的唯一合成入口：`CANCELLED/<取消决定>` 与 `FINAL/<终局种类>/<终局版本>`，带结果引用
// 与版本（UC-NO-002「服务结果引用」）。「无路由」「受限」不是 PS 的服务结果，不在这里。顺序固定先取消后终局，同一答复落成
// 同一组标记；没有取消也没有终局时交回空。
//
// 标记串只给人看与审计，不得解析：终局种类是租户配置的引用、版本也是引用，两者都可能自带 `/`，串拆不回种类与版本。
// 程序要知道一件包裹此刻有没有取消或终局，经 ParcelServiceResultView 现问（ADR-0186）。
func (results ParcelServiceResults) Markers() []string {
	var markers []string
	if results.Cancellation != "" {
		markers = append(markers, serviceMarkerCancelledPrefix+results.Cancellation)
	}
	if results.FinalKind != "" || results.FinalVersion != "" {
		markers = append(markers, serviceMarkerFinalPrefix+results.FinalKind+"/"+results.FinalVersion)
	}
	return markers
}

// ParcelServiceResultView 按（租户，正式包裹关联）读 PS 的当前服务结果（PS 侧只读引用，同 ParcelIdentityView）。
// 依赖读不动作为错误返回：「没有限制」与「没查到」是两件事，空答复只表示前者。
type ParcelServiceResultView interface {
	FindParcelServiceResults(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.ParcelAssociationReference,
	) (ParcelServiceResults, error)
}

// ServiceResultCheck 是一次关联收寄向 PS 取服务结果的核对态，只有 INTAKE_FORMED 记录带（ADR-0186）。它让「查过了，PS 没有
// 取消与终局」与「没取得 PS 的答复」在记录上分得开——两者的标记都是空的，而后者的空不代表没有限制。
//
// 零值即不带：其余三种记录没有可问的包裹，这一格缺席是真话，库里由迁移的 CHECK 拦住形成格以外带核对态的行；
// INTAKE_FORMED 记录上是零值就是写入方缺陷。
type ServiceResultCheck uint8

const (
	ServiceResultCheckAbsent ServiceResultCheck = iota
	// ServiceResultsChecked：读口答复了，标记照答复合成（可能为空）。
	ServiceResultsChecked
	// ServiceResultsUnreadable：读口交回错误、没取得 PS 的答复；标记必为空，且这份空不是「没有取消与终局」。
	ServiceResultsUnreadable
)

func (check ServiceResultCheck) String() string {
	switch check {
	case ServiceResultsChecked:
		return "CHECKED"
	case ServiceResultsUnreadable:
		return "UNREADABLE"
	default:
		return ""
	}
}

// ReceptionRecord 是一次收寄判断留下的东西。收寄与控制只在形成/待识别两格在场；
// 候选与身份冲突只在待识别格有意义；服务结果标记（已取消 / 终局，编排在身份唯一成立时向 PS 取，由
// ParcelServiceResults.Markers 合成）与核对态只在形成格在场——它们不阻止接收，只限制后续方向性作业。标记串原样保全，
// 只给人看与审计，不得解析（ADR-0186）。
//
// 判断版本、前版与来源文档见 ADR-0199 决定一、决定二：
//   - JudgmentVersion：每种判断都有。形成与待识别两格就是 Intake 上的收寄结果版本，不另签；未收寄与待确认两格由同一个
//     签发器另签。类型沿用 IntakeResultVersion，因为两者同一签发器、同一命名空间。
//   - Predecessor：只有追加的一版带，指名它接续的那条判断链此刻的当前有效版本；首判不带。一版至多一个后继，由收寄库守。
//   - Source：本次交付的来源文档，追加的冲突版带引起它的那次送站的。零值是来源未保全（来源保全之前落下的行）。
//   - MergedInto：只有「并入已有收寄」那一种带，指名并入的那一版判断（ADR-0199 决定四）；它不是前版，不占（租户，前版）唯一。
//
// 四格都不进内容指纹：判断版本与前版是服务端在判断时刻签下与认出的，来源文档里多出的证据、拒收原因与交付方按 ADR-0199
// 越权风险点不改幂等与冲突的分界。
type ReceptionRecord struct {
	Key                ReceptionKey
	ContentDigest      string
	Kind               ReceptionRecordKind
	JudgmentVersion    domain.IntakeResultVersion
	Predecessor        domain.IntakeResultVersion
	MergedInto         domain.IntakeResultVersion
	Source             domain.DeliverySource
	Intake             domain.NodeIntake
	Control            domain.PhysicalControl
	Candidates         []domain.ParcelAssociationReference
	IdentityConflict   bool
	RefusalReason      string
	ServiceMarkers     []string
	ServiceResultCheck ServiceResultCheck
	RecordedAt         time.Time
}

type ReceptionSaveOutcome uint8

const (
	ReceptionSaveOutcomeInvalid ReceptionSaveOutcome = iota
	ReceptionSaved
	ReceptionAlreadyRecorded
)

// ReceptionStore 按幂等键找回并保存收寄判断（写入代数同 ADR-0031）。
type ReceptionStore interface {
	FindByKey(ctx context.Context, key ReceptionKey) (ReceptionRecord, bool, error)
	Save(ctx context.Context, record ReceptionRecord) (ReceptionSaveOutcome, error)
	// FindOtherUnitIntakes 交回本租户收寄册上关联到 parcel、作业实物不是 unit 的各条记录，按来源标识定序；没有交回空切片。
	// 收寄编排据此认「同一外部条码出现在多个实物」（ADR-0196）：按关联到的包裹认，不按号码认——号码只是这次身份核对的
	// 输入，关联才是册上已经成立的判断。不看那件实物此刻是否仍在节点控制下。
	//
	// 这个读要在收寄落册所在的事务里调：从这次读到那笔事务结束，同一租户同一件包裹上的另一次读等着（票
	// no-reception-identity/03），否则两笔并发送站各自读不到对方未提交的关联，两件都关联成立。只在单线程里跑的替身不必
	// 模拟这一格。
	FindOtherUnitIntakes(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.ParcelAssociationReference,
		unit domain.HandlingUnitID,
	) ([]ReceptionRecord, error)
	// CurrentVersion 交回 version 所在判断链此刻的当前有效版本：沿「前版是它」的那一行一路走到没有后继的那一版；没有后继
	// 时就是 version 自己（ADR-0199 决定二）。只走本租户，不看 version 本身在不在册——没有判断版本的旧行无从问起，由调用方
	// 不问。读不动作为错误返回。
	//
	// 追加一版之前在同一笔事务里问：并发里另一方先追加的那一版要么已提交、在这里读得到，要么撞上（租户，前版）唯一。
	CurrentVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.IntakeResultVersion,
	) (domain.IntakeResultVersion, error)
	// FindCurrentJudgmentsAtNode 交回这件作业实物在这个节点上各条判断链此刻的当前有效版本，按来源标识定序；没有交回空切片。
	// 收寄编排据此认「同一作业实物在同一节点已有当前收寄」与「待确认由后来的观察续办」（ADR-0199 决定四）：同一实物按租户内
	// 的作业实物标识认，不按号码认。形成与待识别两种按收寄上的实物与节点认（追加的冲突版来源文档里是后到那件，收寄上才是
	// 它接续的那件），未收寄与待确认两种没有收寄，按来源文档认。并入行不是判断，不在其中；来源保全之前落下的未收寄与待确认行没有判断版本、
	// 也没存实物与节点，认不出是哪件，同样不在其中。
	//
	// 查之前先按（租户，作业实物）取锁，要在收寄落册所在的事务里调：从这次读到那笔事务结束，同一件实物的另一次收寄等着，
	// 否则两个来源并发送站各自读不到对方未提交的收寄，两条都形成控制起点。三种观察都取，唯一识别那一支随后才取包裹锁
	// （FindOtherUnitIntakes），次序固定先实物后包裹。只在单线程里跑的替身不必模拟这一格。
	FindCurrentJudgmentsAtNode(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.HandlingUnitID,
		node domain.NodeReference,
	) ([]ReceptionRecord, error)
	// FindByJudgmentVersion 交回本租户判断版本为 version 的那一行；没有交回 false。收寄编排答并入行的重投时按它读回并入的那一版，
	// 原判断种类取那一版的。读不动作为错误返回。
	FindByJudgmentVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.IntakeResultVersion,
	) (ReceptionRecord, bool, error)
}

// PendingIdentityLookup 找待重核的待识别实物（ADR-0199 决定三）。与 ReceptionStore 分立：收寄编排不按号码找记录，扩它的接口
// 就要连带改所有只为收寄而写的替身；按判断版本取一行两边都要，方法同名同形，生产上两者是同一只收寄库适配器。
type PendingIdentityLookup interface {
	// FindByJudgmentVersion 交回本租户判断版本为 version 的那一行；没有交回 false。人工重核与按判断版本自投的门铃都只指名
	// 这一件。读不动作为错误返回。
	FindByJudgmentVersion(
		ctx context.Context,
		tenant domain.TenantID,
		version domain.IntakeResultVersion,
	) (ReceptionRecord, bool, error)
	// FindRecheckableByMark 交回本租户里当前有效版本是待识别、不带关联、不记身份冲突，且来源文档的外部标识观察与 mark 逐字
	// 相等的各行，按来源标识定序；没有交回空切片。来源文档缺席的旧行不在其中：号码从未在册上留存，无从按号匹配。
	FindRecheckableByMark(
		ctx context.Context,
		tenant domain.TenantID,
		mark string,
	) ([]ReceptionRecord, error)
}

// ErrPredecessorAlreadySucceeded 说明 Save 写的一版所指名的前版在本租户已有后继：一版至多一个后继（ADR-0199 决定二）。它不译成
// ReceptionAlreadyRecorded——那一格说的是同一来源已在册，读回赢家作答；这里来源不同，读回什么都不是这一版的答案。
var ErrPredecessorAlreadySucceeded = errors.New("node operations: the predecessor judgment version already has a successor")

// IntakeIdentityFactory 签发收寄结果版本。
type IntakeIdentityFactory interface {
	NextIntakeResultVersion(ctx context.Context) (domain.IntakeResultVersion, error)
}

// NodeIntakeHandoffIntent 把已提交的收寄判断交给适用下游（parcel-shipment 的采用判断
// 正是消费者）。意图由收寄键认领，重放重发同一份（ADR-0043 同款纪律）。
type NodeIntakeHandoffIntent struct {
	Record ReceptionRecord
}

// NodeIntakeHandoff 把已提交的收寄判断写入 Outbox（`OutboxNodeIntakeHandoff`）。信封
// ID 由收寄幂等键认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份
// （ADR-0043）。
type NodeIntakeHandoff interface {
	HandOffNodeIntake(ctx context.Context, intent NodeIntakeHandoffIntent) error
}

// CollaborationAcceptanceKey 是承接决定的幂等键：同一协作事项只决定一次，重放返回
// 原决定；同事项异决定形成冲突，不按最后到达顶替。
type CollaborationAcceptanceKey struct {
	TenantID domain.TenantID
	Item     domain.CollaborationItemReference
}

// CollaborationAcceptanceRecord 是一次承接决定越过提交边界留下的东西。
type CollaborationAcceptanceRecord struct {
	Key           CollaborationAcceptanceKey
	ContentDigest string
	Acceptance    domain.CollaborationAcceptance
	RecordedAt    time.Time
}

type AcceptanceSaveOutcome uint8

const (
	AcceptanceSaveOutcomeInvalid AcceptanceSaveOutcome = iota
	AcceptanceSaved
	AcceptanceAlreadyDecided
)

// CollaborationAcceptanceStore 按幂等键找回并保存承接决定（写入代数同 ADR-0031）。
type CollaborationAcceptanceStore interface {
	FindByKey(ctx context.Context, key CollaborationAcceptanceKey) (CollaborationAcceptanceRecord, bool, error)
	Save(ctx context.Context, record CollaborationAcceptanceRecord) (AcceptanceSaveOutcome, error)
}

// CollaborationAcceptanceHandoffIntent 把承接决定交回 customs-compliance——承接结果
// 是协作链的回执信号。意图由幂等键认领，重放重发同一份（ADR-0043 同款纪律）。
type CollaborationAcceptanceHandoffIntent struct {
	Record CollaborationAcceptanceRecord
}

// CollaborationAcceptanceHandoff 把承接决定写入 Outbox（`OutboxCollaborationAcceptanceHandoff`）。
// 信封 ID 由承接幂等键认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份
// （ADR-0043）。
type CollaborationAcceptanceHandoff interface {
	HandOffCollaborationAcceptance(ctx context.Context, intent CollaborationAcceptanceHandoffIntent) error
}

// ExecutionFactKey 是执行事实登记的幂等键：同一（事项+实物+动作）只登一次，重放返回
// 原事实；同键异证据形成冲突。
type ExecutionFactKey struct {
	TenantID domain.TenantID
	Item     domain.CollaborationItemReference
	Unit     domain.HandlingUnitID
	Action   domain.CollaborationActionKind
}

// ExecutionFactRecord 是一次执行事实登记越过提交边界留下的东西。
type ExecutionFactRecord struct {
	Key           ExecutionFactKey
	ContentDigest string
	Fact          domain.NodeExecutionFact
	RecordedAt    time.Time
}

type ExecutionFactSaveOutcome uint8

const (
	ExecutionFactSaveOutcomeInvalid ExecutionFactSaveOutcome = iota
	ExecutionFactSaved
	ExecutionFactAlreadyRecorded
)

// ExecutionFactStore 按幂等键找回并保存执行事实（写入代数同 ADR-0031）。
type ExecutionFactStore interface {
	FindByKey(ctx context.Context, key ExecutionFactKey) (ExecutionFactRecord, bool, error)
	Save(ctx context.Context, record ExecutionFactRecord) (ExecutionFactSaveOutcome, error)
}

// ExecutionFactHandoffIntent 把执行事实交给 customs-compliance 的处置执行核对消费
// （CC 侧 ExecutionFactView 的上游源）。重放重发同一份。
type ExecutionFactHandoffIntent struct {
	Record ExecutionFactRecord
}

// ExecutionFactHandoff 把执行事实写入 Outbox（`OutboxExecutionFactHandoff`）。信封 ID
// 由执行事实幂等键认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份
// （ADR-0043）。
type ExecutionFactHandoff interface {
	HandOffExecutionFact(ctx context.Context, intent ExecutionFactHandoffIntent) error
}

// ConsolidationFactKey 是集运作业事实的幂等键：同一来源身份和内容返回已有结果，同一
// 身份不同内容形成冲突（AT-NO-043）。形状照 ReceptionKey——同一上下文里`来源身份`的
// 表达只有一种，集运再造一套会让两个口的重放语义各说各话。
type ConsolidationFactKey struct {
	TenantID domain.TenantID
	SourceID string
}

// ConsolidationFactRecord 是一次集运作业事实越过提交边界留下的东西，也就是集运口此前
// 整个缺席的那一层来源事实（ADR-0005）。它与单元行分开存：单元行是派生状态，同一个
// 单元会被许多次作业推进，把来源挤进单元行只留得下最后一次。
//
// Member 只在移入/移出两格在场，Seal 只在封装格在场——其余格上它们缺席是真话，不是漏填。
// OccurredAt 是现场自带的业务时间，RecordedAt 是服务端的记录时刻，两者分列（ADR-0023）。
//
// DirectionalBasis 只在移入格上有值：形成新的移入事实时同一笔记下的方向依据记录（票 no-directional-basis/01）。
// 它不进内容指纹——那是服务端在判断时刻所知，不是来源报来的内容；进了指纹，PS 状态一变，同一来源重投就成了
// 来源冲突。零值读作`未记录`（这一格出现之前落的移入事实），不读作依据齐全。
type ConsolidationFactRecord struct {
	Key              ConsolidationFactKey
	ContentDigest    string
	Action           domain.ConsolidationActionKind
	Unit             domain.ConsolidationUnitID
	Member           domain.HandlingUnitID
	Seal             domain.SealReference
	PerformedBy      domain.PerformingPartyReference
	Evidence         domain.ExecutionEvidenceReference
	OccurredAt       time.Time
	RecordedAt       time.Time
	DirectionalBasis domain.DirectionalBasisRecord
}

type ConsolidationFactSaveOutcome uint8

const (
	ConsolidationFactSaveOutcomeInvalid ConsolidationFactSaveOutcome = iota
	ConsolidationFactSaved
	ConsolidationFactAlreadyRecorded
)

// ConsolidationFactStore 按幂等键找回并保存集运作业来源事实（写入代数同 ADR-0031）。
type ConsolidationFactStore interface {
	FindByKey(ctx context.Context, key ConsolidationFactKey) (ConsolidationFactRecord, bool, error)
	Save(ctx context.Context, record ConsolidationFactRecord) (ConsolidationFactSaveOutcome, error)
}

type ConsolidationSaveOutcome uint8

const (
	ConsolidationSaveOutcomeInvalid ConsolidationSaveOutcome = iota
	ConsolidationSaved
	ConsolidationAlreadyRecorded
)

// ConsolidationStore 保存集运单元实例。Save 只管开启（同 ID 重复开启交回
// AlreadyRecorded）；加入/移出/封装/开封/关闭是同一实例的状态推进，走 Update。
type ConsolidationStore interface {
	FindByID(ctx context.Context, tenant domain.TenantID, id domain.ConsolidationUnitID) (*domain.ConsolidationUnit, bool, error)
	Save(ctx context.Context, tenant domain.TenantID, unit *domain.ConsolidationUnit) (ConsolidationSaveOutcome, error)
	Update(ctx context.Context, tenant domain.TenantID, unit *domain.ConsolidationUnit) error
}

// IntakeIdentityView 按作业实物读本上下文收寄册上带收寄的行（收寄已形成与待识别两格），供方向依据记录判身份
// （票 no-directional-basis/01）。今天正式包裹 → 作业实物的映射只有收寄判断里识别成功建立的版本化关联，身份因此
// 只问这本册。查无此物交回空切片；读不动作为错误返回——「没有收寄记录」与「没查到」是两件事。
type IntakeIdentityView interface {
	FindIntakeIdentities(
		ctx context.Context,
		tenant domain.TenantID,
		unit domain.HandlingUnitID,
	) ([]domain.IntakeIdentityObservation, error)
}

// RoutingBasisQuery 是一次路由依据询问：哪件作业实物（经已成立的正式包裹关联）要进哪个集运单元。路由指令按包裹
// 答，成员级集运兼容判断还要看拟组合的成员与目标单元，几样一并给出。
type RoutingBasisQuery struct {
	TenantID domain.TenantID
	Parcel   domain.ParcelAssociationReference
	Member   domain.HandlingUnitID
	Unit     domain.ConsolidationUnitID
}

// RoutingBasisView 答一件作业实物此刻的路由依据：当前有效路由指令与成员级集运兼容判断，两样都归 network-routing
// （UC-NO-003 启动条件）。NR 今天没有可消费的读口，生产装配显式未配置（票 no-directional-basis/02 换真）；读不动
// 作为错误返回。
type RoutingBasisView interface {
	FindRoutingBasis(ctx context.Context, query RoutingBasisQuery) (domain.RoutingBasis, error)
}

// ContainmentIndex 回答一件实物当前被哪个未关闭单元直接包含。跨单元的「同一时点
// 最多一个直接物理父级」需要仓储视野，领域对象只守住自己这一侧（重复加入拒）——
// 加入前的跨单元核对靠这里。
type ContainmentIndex interface {
	CurrentParent(
		ctx context.Context,
		tenant domain.TenantID,
		member domain.HandlingUnitID,
	) (domain.ConsolidationUnitID, bool, error)
}

// ParcelContainment 是按正式包裹的版本化关联答「此刻在不在某个未关闭集运单元里」的封闭
// 三值（读面为 parcel-shipment 的「资料修订阶段」判断而立，ADR-0118 决定四拆出的票
// ps-port-remainder/05）。ContainmentIndex 按作业实物答直接父级；正式包裹与作业实物之间是
// 识别成功后建立的版本化关联，本口经那条关联把问题从包裹搬到实物，再问容纳索引。
//
// 三值的分界是本上下文对自己事实的解释，消费方只翻译不判断：
//
//   - `在`：至少一件已关联该包裹的作业实物此刻被某个未关闭单元直接包含。
//   - `不可归属`：没有已关联实物在单元里，但有一件仍待识别的实物，其候选关联里列着该包裹，
//     且那件实物此刻在某个单元里。候选不是归属（CONTEXT「候选尚未确认时不得据此执行方向性
//     作业」），本上下文说不出那件在袋里的东西是不是它——既不能答`在`（那是拿候选冒充关联），
//     也不能答`不在`（那会让一个身份冲突尚未处置的包裹被读成「没装袋」）。
//   - `不在`：其余情形，含本上下文从未把任何作业实物关联到该包裹。没有关联就没有可归属于
//     它的装袋事实，这是对本上下文自己册子的如实回答，不是「不知道」——把它答成不知道，
//     会让每一个尚未到站的包裹都判不出阶段。
//
// 零值是坏值，与本包其余封闭集同形（ReceptionRecordKindInvalid）；消费方翻译时对零值上抛。
type ParcelContainment uint8

const (
	ParcelContainmentInvalid ParcelContainment = iota
	ParcelNotContained
	ParcelContained
	ParcelContainmentUnattributable
)

func (containment ParcelContainment) String() string {
	switch containment {
	case ParcelNotContained:
		return "NOT_CONTAINED"
	case ParcelContained:
		return "CONTAINED"
	case ParcelContainmentUnattributable:
		return "UNATTRIBUTABLE"
	default:
		return ""
	}
}

// ParcelContainmentView 按（租户 + 正式包裹的版本化关联引用）答 ParcelContainment。依赖调不通
// 作为错误返回。它不拓宽 ContainmentIndex：那一口按作业实物伺候加入前的跨单元核对，本口按
// 包裹伺候另一个上下文的一次同步询问，两个调用面各答各的问题。
type ParcelContainmentView interface {
	LoadParcelContainment(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.ParcelAssociationReference,
	) (ParcelContainment, error)
}

// SealedSnapshotHandoffIntent 把封装快照交给适用下游（装载与交接按封装快照对货）。
type SealedSnapshotHandoffIntent struct {
	TenantID domain.TenantID
	Unit     domain.ConsolidationUnitID
	Snapshot domain.SealedSnapshot
}

// SealedSnapshotHandoff 把封装快照写入 Outbox（`OutboxSealedSnapshotHandoff`）。信封
// ID 由租户加单元加封签认领（单元+快照认领键补租户维），入队由
// outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type SealedSnapshotHandoff interface {
	HandOffSnapshot(ctx context.Context, intent SealedSnapshotHandoffIntent) error
}
