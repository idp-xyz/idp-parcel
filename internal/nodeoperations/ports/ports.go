// Package ports 定义 node-operations 应用层与外界的边界。领域包不依赖 HTTP/pgx 的
// 纪律与其余上下文一致。
package ports

import (
	"context"
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

// ParcelIdentityView 用正式包裹与外部标识关联核对身份（PS 侧只读引用）。候选数决定
// 走向：恰一个→关联；零个→待识别；多个→身份冲突。依赖调不通作为错误返回。
type ParcelIdentityView interface {
	ResolveParcelIdentity(
		ctx context.Context,
		tenant domain.TenantID,
		observation ExternalMarkObservation,
	) ([]domain.ParcelAssociationReference, error)
}

// ReceptionKey 是收寄判断的幂等键：同一来源身份和内容返回已有处理结果。
type ReceptionKey struct {
	TenantID domain.TenantID
	SourceID string
}

// ReceptionRecordKind 是越过提交边界的四种判断走向（结果语义契约的前四格；已有结果
// 与来源冲突是应答不是记录）。
type ReceptionRecordKind uint8

const (
	ReceptionRecordKindInvalid ReceptionRecordKind = iota
	RecordIntakeFormed
	RecordPendingIdentification
	RecordIntakeNotFormed
	RecordReceptionUndecided
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
	default:
		return ""
	}
}

// ReceptionRecord 是一次收寄判断留下的东西。收寄与控制只在形成/待识别两格在场；
// 候选与身份冲突只在待识别格有意义；服务结果标记（已取消/终局/无路由）原样保全——
// 它们不阻止接收，只限制后续方向性作业。
type ReceptionRecord struct {
	Key              ReceptionKey
	ContentDigest    string
	Kind             ReceptionRecordKind
	Intake           domain.NodeIntake
	Control          domain.PhysicalControl
	Candidates       []domain.ParcelAssociationReference
	IdentityConflict bool
	RefusalReason    string
	ServiceMarkers   []string
	RecordedAt       time.Time
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
}

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
type ConsolidationFactRecord struct {
	Key           ConsolidationFactKey
	ContentDigest string
	Action        domain.ConsolidationActionKind
	Unit          domain.ConsolidationUnitID
	Member        domain.HandlingUnitID
	Seal          domain.SealReference
	PerformedBy   domain.PerformingPartyReference
	Evidence      domain.ExecutionEvidenceReference
	OccurredAt    time.Time
	RecordedAt    time.Time
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
