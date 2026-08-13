// Package ports 定义 transport-fulfillment 应用层与外界的边界。领域包不依赖 HTTP/pgx
// 的纪律与其余上下文一致。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

type Clock interface {
	Now() time.Time
}

// PickupAttemptKey 是一次尝试来源提交的幂等键：同一来源身份和内容返回已有处理结果
// （UC-TF-002「同一尝试来源身份和内容返回已有结果；同一身份不同内容形成冲突」）。
type PickupAttemptKey struct {
	TenantID domain.TenantID
	SourceID string
}

// PickupAttemptRecord 是一次尝试提交越过提交边界留下的东西：尝试本体、逐对象结果与
// 成功对象的场外揽收。失败与拒收对象只出现在 Results 里——Pickups 只收真正取得控制的
// 对象，「失败结果不制造实际履约段」在记录形状上就成立。
type PickupAttemptRecord struct {
	Key           PickupAttemptKey
	ContentDigest string
	Attempt       domain.FulfillmentAttempt
	Results       []domain.AttemptObjectResult
	Pickups       []domain.OffsitePickup
	RecordedAt    time.Time
}

type PickupSaveOutcome uint8

const (
	PickupSaveOutcomeInvalid PickupSaveOutcome = iota
	PickupSaved
	PickupAlreadyRecorded
)

// PickupAttemptStore 按幂等键找回并保存尝试提交（写入代数同 ADR-0031：并发落败读回
// 赢家，不覆盖）。
type PickupAttemptStore interface {
	FindByKey(ctx context.Context, key PickupAttemptKey) (PickupAttemptRecord, bool, error)
	Save(ctx context.Context, record PickupAttemptRecord) (PickupSaveOutcome, error)
}

// PickupIdentityFactory 签发揽收结果版本。逐成功对象签发：来源更正形成新版本不覆盖
// 本版，parcel-shipment 的采用判断按版本幂等。
type PickupIdentityFactory interface {
	NextPickupResultVersion(ctx context.Context) (domain.PickupResultVersion, error)
}

// OffsitePickupHandoffIntent 把已提交的对象级揽收交给 parcel-shipment 判断有效网络
// 收寄（UC-TF-002 步骤 6A）。意图由幂等键认领，重放重发同一份（ADR-0043 同款纪律）；
// 没有成功对象的记录没有可交的东西，不产生意图。
type OffsitePickupHandoffIntent struct {
	Record PickupAttemptRecord
}

// OffsitePickupHandoff 今天没有实现，唯一实现是测试替身；事务发布仍阻断于 ADR-0017
// 的 Bento/Outbox 闸门。
type OffsitePickupHandoff interface {
	HandOffOffsitePickup(ctx context.Context, intent OffsitePickupHandoffIntent) error
}

// DeliveryAttemptView 按（尝试、对象）取回派送尝试与该对象的结果供交付生效引用。
// found=false 表示指名的尝试/对象结果不存在——指错是提交矛盾，不是等谁。
type DeliveryAttemptView interface {
	LoadDeliveryResult(
		ctx context.Context,
		tenant domain.TenantID,
		attempt domain.AttemptReference,
		object domain.CarriedObjectReference,
	) (domain.FulfillmentAttempt, domain.DeliveryAttemptResult, bool, error)
}

// EffectiveDeliveryKey 是交付生效的幂等键：同一（租户+对象+尝试）的生效交付只登
// 一次，重放返回原版本；POD 更正走更正入口换版本，不在首登处顶替。
type EffectiveDeliveryKey struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	Attempt  domain.AttemptReference
}

// EffectiveDeliveryRecord 是一次交付生效越过提交边界留下的东西。更正后记录携带新
// 版本，版本链在 EffectiveDelivery 本体上（Corrects 回指前版）。
type EffectiveDeliveryRecord struct {
	Key           EffectiveDeliveryKey
	ContentDigest string
	Delivery      domain.EffectiveDelivery
	RecordedAt    time.Time
}

type DeliverySaveOutcome uint8

const (
	DeliverySaveOutcomeInvalid DeliverySaveOutcome = iota
	DeliverySaved
	DeliveryAlreadyRegistered
)

// EffectiveDeliveryStore 按幂等键找回并保存交付生效（写入代数同 ADR-0031）。
// Supersede 只在已有登记上落更正版本：found=false 表示没有可更正的登记。
type EffectiveDeliveryStore interface {
	FindByKey(ctx context.Context, key EffectiveDeliveryKey) (EffectiveDeliveryRecord, bool, error)
	Save(ctx context.Context, record EffectiveDeliveryRecord) (DeliverySaveOutcome, error)
	Supersede(ctx context.Context, record EffectiveDeliveryRecord) (bool, error)
}

// DeliveryIdentityFactory 签发交付结果版本。首登与更正各签新版：更正版本回指前版，
// 原版本不删。
type DeliveryIdentityFactory interface {
	NextDeliveryResultVersion(ctx context.Context) (domain.DeliveryResultVersion, error)
}

// EffectiveDeliveryHandoffIntent 把交付生效交给 parcel-shipment 终局判断消费（既有
// DeliveryOutcomeAdapter 的上游）。意图由幂等键认领，重放重发同一份（ADR-0043）。
type EffectiveDeliveryHandoffIntent struct {
	Record EffectiveDeliveryRecord
}

// EffectiveDeliveryHandoff 今天没有实现，唯一实现是测试替身。
type EffectiveDeliveryHandoff interface {
	HandOffEffectiveDelivery(ctx context.Context, intent EffectiveDeliveryHandoffIntent) error
}

// OffsitePickupKey 是揽收登记的幂等键：同一（租户+对象+尝试）的揽收只登一次，重放
// 返回原版本；来源更正走新版本，不在首登处顶替。
type OffsitePickupKey struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	Attempt  domain.AttemptReference
}

// OffsitePickupRecord 是一次揽收登记越过提交边界留下的东西。
type OffsitePickupRecord struct {
	Key           OffsitePickupKey
	ContentDigest string
	Pickup        domain.OffsitePickup
	RecordedAt    time.Time
}

type OffsitePickupSaveOutcome uint8

const (
	OffsitePickupSaveOutcomeInvalid OffsitePickupSaveOutcome = iota
	OffsitePickupSaved
	OffsitePickupAlreadyRegistered
)

// OffsitePickupRegistry 按幂等键找回并保存对象级揽收（写入代数同 ADR-0031）。
type OffsitePickupRegistry interface {
	FindByKey(ctx context.Context, key OffsitePickupKey) (OffsitePickupRecord, bool, error)
	Save(ctx context.Context, record OffsitePickupRecord) (OffsitePickupSaveOutcome, error)
}

// OffsitePickupRegistrationIntent 把对象级揽收交给 parcel-shipment 采认（既有
// OffsitePickupAdapter 的上游，UC-PS-003 揽收源链）。意图由幂等键认领，重放重发
// 同一份（ADR-0043）。
type OffsitePickupRegistrationIntent struct {
	Record OffsitePickupRecord
}

// OffsitePickupRegistrationHandoff 今天没有实现，唯一实现是测试替身。
type OffsitePickupRegistrationHandoff interface {
	HandOffOffsitePickupRegistration(ctx context.Context, intent OffsitePickupRegistrationIntent) error
}

// TransportHandoverKey 是交接判断登记的幂等键：判断版本由裁决过程指名，同一
// （租户+对象+范围+版本）只登一次；更正是新版本新登记，版本链在本体上回指。
type TransportHandoverKey struct {
	TenantID domain.TenantID
	Object   domain.CarriedObjectReference
	Scope    domain.HandoverScopeReference
	Version  domain.HandoverResultVersion
}

// TransportHandoverRecord 是一次交接判断登记越过提交边界留下的东西。
type TransportHandoverRecord struct {
	Key           TransportHandoverKey
	ContentDigest string
	Handover      domain.TransportHandover
	RecordedAt    time.Time
}

type HandoverSaveOutcome uint8

const (
	HandoverSaveOutcomeInvalid HandoverSaveOutcome = iota
	HandoverSaved
	HandoverAlreadyRegistered
)

// TransportHandoverRegistry 按幂等键找回并保存交接判断（写入代数同 ADR-0031）。
type TransportHandoverRegistry interface {
	FindByKey(ctx context.Context, key TransportHandoverKey) (TransportHandoverRecord, bool, error)
	Save(ctx context.Context, record TransportHandoverRecord) (HandoverSaveOutcome, error)
}

// TransportHandoverRegistrationIntent 把交接判断交给下游消费：一份意图，消费方自分
// ——node-operations 的控制转移只认得出 TransferOutBasis 的已交接，network-routing
// 以 TransportHandoverControl 证据种类触发重判；拒收与待确认同样是它们要看的事实。
type TransportHandoverRegistrationIntent struct {
	Record TransportHandoverRecord
}

// TransportHandoverRegistrationHandoff 今天没有实现，唯一实现是测试替身。
type TransportHandoverRegistrationHandoff interface {
	HandOffTransportHandover(ctx context.Context, intent TransportHandoverRegistrationIntent) error
}

// TransportCommissionKey 是运输委托的幂等键：同一委托标识只提交一次，重放返回原委托。
type TransportCommissionKey struct {
	TenantID   domain.TenantID
	Commission domain.TransportCommissionReference
}

// TransportCommissionRecord 是一次委托提交越过提交边界留下的东西。取消与开始改变
// 状态时以 Replace 换值——历史动作都在本体上（Cancelled/TransportStarted）。
type TransportCommissionRecord struct {
	Key           TransportCommissionKey
	ContentDigest string
	Commission    domain.TransportCommission
	RecordedAt    time.Time
}

type CommissionSaveOutcome uint8

const (
	CommissionSaveOutcomeInvalid CommissionSaveOutcome = iota
	CommissionSaved
	CommissionAlreadyRegistered
)

// TransportCommissionStore 按幂等键找回并保存运输委托（写入代数同 ADR-0031）。
// Replace 只在已有登记上落状态转换（取消/开始）：found=false 表示没有可转换的委托。
type TransportCommissionStore interface {
	FindByKey(ctx context.Context, key TransportCommissionKey) (TransportCommissionRecord, bool, error)
	Save(ctx context.Context, record TransportCommissionRecord) (CommissionSaveOutcome, error)
	Replace(ctx context.Context, record TransportCommissionRecord) (bool, error)
}

// BookingKey 是订舱申请的幂等键。
type BookingKey struct {
	TenantID domain.TenantID
	Booking  domain.BookingReference
}

// BookingRecord 是一次订舱申请越过提交边界留下的东西。
type BookingRecord struct {
	Key           BookingKey
	ContentDigest string
	Booking       domain.BookingRequest
	RecordedAt    time.Time
}

type BookingSaveOutcome uint8

const (
	BookingSaveOutcomeInvalid BookingSaveOutcome = iota
	BookingSaved
	BookingAlreadyRegistered
)

// BookingStore 按幂等键找回并保存订舱申请（写入代数同 ADR-0031）。
type BookingStore interface {
	FindByKey(ctx context.Context, key BookingKey) (BookingRecord, bool, error)
	Save(ctx context.Context, record BookingRecord) (BookingSaveOutcome, error)
}

// BookingAnswerRecord 是承运方对订舱的应答。一次订舱一个应答：应答不可覆盖——已接受
// 的订舱不能再被拒绝，改约走撤回与新订舱。
type BookingAnswerRecord struct {
	Key           BookingKey
	ContentDigest string
	Acceptance    domain.CarrierAcceptance
	RecordedAt    time.Time
}

type BookingAnswerSaveOutcome uint8

const (
	BookingAnswerSaveOutcomeInvalid BookingAnswerSaveOutcome = iota
	BookingAnswerSaved
	BookingAlreadyAnswered
)

// BookingAnswerStore 按订舱键找回并保存承运应答（写入代数同 ADR-0031）。
type BookingAnswerStore interface {
	FindByKey(ctx context.Context, key BookingKey) (BookingAnswerRecord, bool, error)
	Save(ctx context.Context, record BookingAnswerRecord) (BookingAnswerSaveOutcome, error)
}

// TransportCommissionIntent 把委托（含协议/条件/角色/责任快照）交给 settlement-
// accounting 作供应商成本预期的上游来源。意图由幂等键认领，重放重发同一份。
type TransportCommissionIntent struct {
	Record TransportCommissionRecord
}

// TransportCommissionHandoff 今天没有实现，唯一实现是测试替身。
type TransportCommissionHandoff interface {
	HandOffTransportCommission(ctx context.Context, intent TransportCommissionIntent) error
}

// AlternateJourneyKey 是替代/退运旅程的幂等键：同一（原旅程+目的+处置依据）只开一条
// 新旅程——同一处置决定不开两条替代旅程。
type AlternateJourneyKey struct {
	TenantID domain.TenantID
	Original domain.JourneyReference
	Purpose  domain.JourneyPurpose
	Basis    domain.DispositionBasisReference
}

// AlternateJourneyRecord 是一次旅程启动越过提交边界留下的东西。
type AlternateJourneyRecord struct {
	Key           AlternateJourneyKey
	ContentDigest string
	Journey       domain.AlternateJourney
	RecordedAt    time.Time
}

type AlternateJourneySaveOutcome uint8

const (
	AlternateJourneySaveOutcomeInvalid AlternateJourneySaveOutcome = iota
	AlternateJourneySaved
	AlternateJourneyAlreadyStarted
)

// AlternateJourneyStore 按幂等键找回并保存替代/退运旅程（写入代数同 ADR-0031）。
type AlternateJourneyStore interface {
	FindByKey(ctx context.Context, key AlternateJourneyKey) (AlternateJourneyRecord, bool, error)
	Save(ctx context.Context, record AlternateJourneyRecord) (AlternateJourneySaveOutcome, error)
}

// AlternateJourneyIntent 是旅程启动的发布意图。重放重发同一份（ADR-0043）。
type AlternateJourneyIntent struct {
	Record AlternateJourneyRecord
}

// DispositionExecutionHandoff 把监管来路的旅程交给 customs-compliance 作处置执行
// 事实源（CC 处置执行核对的上游）。只有 RegulatoryOrigin 的旅程走这条链。
type DispositionExecutionHandoff interface {
	HandOffDispositionExecution(ctx context.Context, intent AlternateJourneyIntent) error
}

// ExceptionJourneyHandoff 把旅程启动交给 visibility-exception 异常链——监管与非监管
// 来路都要让异常侧看见。
type ExceptionJourneyHandoff interface {
	HandOffExceptionJourney(ctx context.Context, intent AlternateJourneyIntent) error
}

// ScheduleKey 是班次的幂等键：同一班次标识只建一次。
type ScheduleKey struct {
	TenantID domain.TenantID
	Schedule domain.ScheduleReference
}

// ScheduleRecord 是一次班次建立越过提交边界留下的东西。
type ScheduleRecord struct {
	Key           ScheduleKey
	ContentDigest string
	Schedule      domain.TransportSchedule
	RecordedAt    time.Time
}

type ScheduleSaveOutcome uint8

const (
	ScheduleSaveOutcomeInvalid ScheduleSaveOutcome = iota
	ScheduleSaved
	ScheduleAlreadyEstablished
)

// TransportScheduleStore 按幂等键找回并保存班次（写入代数同 ADR-0031）。
type TransportScheduleStore interface {
	FindByKey(ctx context.Context, key ScheduleKey) (ScheduleRecord, bool, error)
	Save(ctx context.Context, record ScheduleRecord) (ScheduleSaveOutcome, error)
}

// CapacityPoolKey 是容量池的幂等键。
type CapacityPoolKey struct {
	TenantID domain.TenantID
	Pool     domain.CapacityPoolReference
}

// CapacityPoolRecord 是容量池及其全部预占的当前值。预占/释放/消耗以 Replace 换值
// ——三量守恒在 CapacityPool 本体上。
type CapacityPoolRecord struct {
	Key           CapacityPoolKey
	ContentDigest string
	Pool          domain.CapacityPool
	RecordedAt    time.Time
}

type PoolSaveOutcome uint8

const (
	PoolSaveOutcomeInvalid PoolSaveOutcome = iota
	PoolSaved
	PoolAlreadyEstablished
)

// CapacityPoolStore 按幂等键找回并保存容量池（写入代数同 ADR-0031）。Replace 只在
// 已有池上落转换：found=false 表示没有可转换的池。
type CapacityPoolStore interface {
	FindByKey(ctx context.Context, key CapacityPoolKey) (CapacityPoolRecord, bool, error)
	Save(ctx context.Context, record CapacityPoolRecord) (PoolSaveOutcome, error)
	Replace(ctx context.Context, record CapacityPoolRecord) (bool, error)
}

// CapacityConsumptionIntent 把容量消耗交给装载分配链——消耗以装载分配确认为依据，
// LoadAssignment 侧引用同一 LoadAssignmentReference。重放重发同一份（ADR-0043）。
type CapacityConsumptionIntent struct {
	Pool        CapacityPoolRecord
	Reservation domain.CapacityReservationReference
	Assignment  domain.LoadAssignmentReference
	Quantity    int64
}

// CapacityConsumptionHandoff 今天没有实现，唯一实现是测试替身。
type CapacityConsumptionHandoff interface {
	HandOffCapacityConsumption(ctx context.Context, intent CapacityConsumptionIntent) error
}
