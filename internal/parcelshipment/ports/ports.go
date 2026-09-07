// Package ports 声明 parcel-shipment 自己拥有的语义边界。这里只有接口：它们的
// PostgreSQL 适配器仍阻断在 Bento 持久化闸门之后（ADR-0017），所以当前唯一的实现是
// 测试用的确定性替身。
package ports

import (
	"context"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelshipment/domain"
)

// SourceSubmissionRepository 在明确的来源作用域下保全不可变来源事实。查询以完整的
// SourceIdentity 为键，因此否定结果不区分「不存在」与「属于另一个租户或客户账户」。
type SourceSubmissionRepository interface {
	FindPreserved(ctx context.Context, identity domain.SourceIdentity) (domain.SourceSubmissionFingerprint, bool, error)
	Preserve(ctx context.Context, submission domain.SourceSubmissionFingerprint) error
	// AppendObservation 记录同一逻辑请求再次被观察到。它追加在已保全事实旁边，绝不
	// 替换它——这正是不同的 occurredAt/receivedAt 无法改写历史的原因。
	AppendObservation(ctx context.Context, observed domain.SourceSubmissionFingerprint) error
}

// ShipmentRequestSaveOutcome 是一次委托聚合写入在本上下文的落点。
//
// `版本冲突`不译成 error。写入这条路走通了，只是有人先落了一步——那是业务答案而非技术故障，
// 调用方要做的是重读再重放，不是把同一份过期聚合当作故障重试。译成 error 之后调用方只剩
// 「没落库」一格，而那一格既可能是库坏了也可能是正常竞争，两者的运维动作相反（ADR-0031）。
type ShipmentRequestSaveOutcome uint8

const (
	ShipmentRequestSaveOutcomeInvalid ShipmentRequestSaveOutcome = iota
	ShipmentRequestSaved
	ShipmentRequestRevisionConflict
)

func (outcome ShipmentRequestSaveOutcome) String() string {
	switch outcome {
	case ShipmentRequestSaved:
		return "SAVED"
	case ShipmentRequestRevisionConflict:
		return "REVISION_CONFLICT"
	default:
		return ""
	}
}

// ShipmentRequestInsertOutcome 是一次建单写入在本上下文的落点。
//
// `已存在`不译成 error，理由与 Save 那一格相同：写入这条路走通了，只是另一方先把同一个来源
// 身份建了单——那是业务答案而非技术故障。译成 error，调用方只剩「没落库」一格，而那一格既
// 可能是库坏了也可能是一次正常的并发重复，两者的运维动作相反（ADR-0031）。
//
// 它与 Save 的`版本冲突`**不共用一个代数**，判据是恢复动作不同：`版本冲突`要重读再重放，
// `已存在`要回到重放判定去重答。这一格由 `PBC-04`「并发重复不能创建第二份委托或第二个
// EventID」驱动。
//
// **`已存在`不是一个终局取值。** ADR-0031 的入口条件问「已经建过了」是不是就是 `SubmitOutcome`
// 里既有的`已有结果`，答案是否定的：建单前的重放判定先用 `ClassifySourceSubmission` 比内容，
// 同内容才答`已有结果`，不同内容答`接入冲突`；而既有那一份的内容同样可能与本次不同。直接译成
// `已有结果`会跳过那次内容比对，对一份其实是另一个载荷的输入答「这次请求已经办过了」。所以
// 编排拿到它要做的是**回去按重放规则重答**，而不是照抄某一格。
type ShipmentRequestInsertOutcome uint8

const (
	ShipmentRequestInsertOutcomeInvalid ShipmentRequestInsertOutcome = iota
	ShipmentRequestInserted
	ShipmentRequestAlreadyExists
)

func (outcome ShipmentRequestInsertOutcome) String() string {
	switch outcome {
	case ShipmentRequestInserted:
		return "INSERTED"
	case ShipmentRequestAlreadyExists:
		return "ALREADY_EXISTS"
	default:
		return ""
	}
}

// ShipmentRequestRepository 以产生委托的来源身份为键存储委托聚合，这样重放才能返回
// 原委托而不是再建一份。
//
// Insert 与 Save 分开：建单只能发生一次，而决定是在既有委托上推进。合成一个方法会让
// 「这是第一份还是第二份」失去表达。
//
// Save 的预期版本由聚合自己携带（`request.Revision()`），不作独立参数：ADR-0028 已把
// 「聚合只记自己是从哪一版读出来的」定为版本字段的含义，那就是预期版本；再开一个参数是
// 造第二个来源，而两者相等由「转移一律不动版本」保证、不由本签名保证（ADR-0031）。
//
// Insert 与 Save 各有各的写入结果代数，理由见 ShipmentRequestInsertOutcome：两者的失败答案
// 不是同一件事，恢复动作也不同。ADR-0031 把 Insert 那一格登记为已知缺口，本签名关闭它。
type ShipmentRequestRepository interface {
	FindBySourceIdentity(ctx context.Context, identity domain.SourceIdentity) (domain.ShipmentRequest, bool, error)
	Insert(
		ctx context.Context,
		identity domain.SourceIdentity,
		request domain.ShipmentRequest,
	) (ShipmentRequestInsertOutcome, error)
	Save(
		ctx context.Context,
		identity domain.SourceIdentity,
		request domain.ShipmentRequest,
	) (ShipmentRequestSaveOutcome, error)
}

// ShipmentRequestSubmittedHandoffIntent 把一份刚进入`已提交`的委托交给适用下游。
// 意图由来源身份认领：同一来源身份至多建立一份委托（同键同摘要重放返原、不同摘要
// 冲突拒绝），因此同一身份无论交几次都是同一份（ADR-0043）。
type ShipmentRequestSubmittedHandoffIntent struct {
	Identity domain.SourceIdentity
	Request  domain.ShipmentRequest
}

// ShipmentRequestSubmittedHandoff 把「委托已提交」写入 Outbox
// （`OutboxShipmentRequestSubmittedHandoff`）。信封 ID 由来源身份（含租户）再加类型段
// 认领，入队由 outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。它必须与
// 建单写入同一事务交出（简报「事务边界」的委托提交事务），任一步失败全部回滚。
type ShipmentRequestSubmittedHandoff interface {
	HandOffShipmentRequestSubmitted(ctx context.Context, intent ShipmentRequestSubmittedHandoffIntent) error
}

// ManualReviewCompletedHandoffIntent 把一次刚记下的复核完成交给适用下游（ADR-0086
// Decision 二）。携带完成后的聚合而不是逐字段抄写：信封要的成员清单、提交版本与完成
// 时刻都在聚合上，抄成平铺字段就得在两处约定哪一份是准的。
type ManualReviewCompletedHandoffIntent struct {
	Identity domain.SourceIdentity
	Request  domain.ShipmentRequest
}

// ManualReviewCompletedHandoff 把「复核已完成」写入 Outbox
// （`OutboxManualReviewCompletedHandoff`）。信封 ID 由来源身份加提交版本再加类型段认领：
// 同一提交版本至多一次`已记录`完成（领域对重复完成答`已有完成`，不再走到交接），重放
// 因此重发同一份（ADR-0043）。它必须与完成落库同一事务交出（ADR-0086：完成与信封一起
// 成立或一起消失），任一步失败全部回滚——完成落了库而信封没入队，停等复核的委托就再也
// 没有投递来续办。
type ManualReviewCompletedHandoff interface {
	HandOffManualReviewCompleted(ctx context.Context, intent ManualReviewCompletedHandoffIntent) error
}

// SubmissionVersionFormedHandoffIntent 把一份刚形成的新提交版本交给适用下游（ADR-0106
// Decision 三）。携带形成之后的聚合而不是逐字段抄写，理由同复核完成那份意图：信封要的成员
// 清单、新版本与形成时刻都在聚合的当前版本上。
type SubmissionVersionFormedHandoffIntent struct {
	Identity domain.SourceIdentity
	Request  domain.ShipmentRequest
}

// SubmissionVersionFormedHandoff 把「新提交版本已形成」写入 Outbox
// （`OutboxSubmissionVersionFormedHandoff`）。信封 ID 由来源身份加**新**提交版本再加类型段认领：
// 同一版本至多形成一次（领域对同一补充身份的重放答`已处理`，走不到交接），重放因此重发同一份
// （ADR-0043）。它必须与新版本落库同一事务交出（ADR-0106 Decision 三：翻转与信封同笔落地，
// 否则不许落地），任一步失败全部回滚——版本落了库而信封没入队，停等补充的委托就再也没有投递
// 来续办，等待态入账反而成了更安静的永久停滞。
type SubmissionVersionFormedHandoff interface {
	HandOffSubmissionVersionFormed(ctx context.Context, intent SubmissionVersionFormedHandoffIntent) error
}

// CurrentAcceptedParcelTargetView 按（租户+声明包裹）反查**当前已接受**委托目标。
//
// 它与 ShipmentRequestRepository 分开：建单与推进的调用方不该持有反查；收寄/交付
// 下一票只依赖本口，不必打开整份聚合仓储。查询只认当前投影列上的已接受行
// （ADR-0060），不扫 snapshot 里的 priorVersions。
//
// 零行 = found=false；恰一行 = 交回来源身份、委托号与当前提交版本号；多于一行 =
// ErrAmbiguousParcelTarget，不按时间或行序任选。
type CurrentAcceptedParcelTargetView interface {
	FindCurrentAcceptedByParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.CurrentAcceptedParcelTarget, bool, error)
}

// CurrentDeclaredParcelsView 按（租户+委托）取回该委托当前提交版本的声明包裹清单。
//
// 它与 CurrentAcceptedParcelTargetView 读的是同一投影列（ADR-0060），方向相反：那口
// 按包裹反查委托，本口按委托取成员。两者分立而不合成一个：按包裹反查的调用方手上
// 没有委托标识，按委托取成员的调用方也不需要反查——合成会让各自的适配器实现一个它
// 根本答不了的方法。
//
// 委托标识与租户两样都收，理由同 RecordedJudgmentReader：`shipment_request_id_unique`
// 是（租户 + 委托标识）而不是单列唯一，只凭委托标识定不到一份委托。
//
// 本口刻意不设状态门。反查那口只认`已接受`是因为多份委托可能声明同一包裹，得靠状态
// 消歧义；而（租户 + 委托标识）本身唯一，再加一道状态门只会让一份确实存在的委托读起来
// 像不存在，那条规则没有任何用例要求过。
//
// 零行 = found=false。清单不造默认也不发明成员；读得到的那一行必有至少一件成员，库上
// `shipment_request_declared_parcels_present` 镜像的正是领域那条门。
type CurrentDeclaredParcelsView interface {
	FindCurrentDeclaredParcels(
		ctx context.Context,
		tenant domain.TenantID,
		requestID domain.ShipmentRequestID,
	) ([]domain.DeclaredParcelID, bool, error)
}

// ProductionOwnershipAuthority 是试点准入控制，回答完整拟受理范围当前由谁承接。
// parcel-shipment 只消费该决定，绝不自行推导一个。
type ProductionOwnershipAuthority interface {
	DecideProductionOwnership(ctx context.Context, scope domain.AdmissionScope) (domain.ProductionOwnershipDecision, error)
}

// SubmissionIdentityFactory 签发 parcel-shipment 自己拥有的内部身份。刻意不从调用方
// 接收：客户参考号不得变成内部标识。
type SubmissionIdentityFactory interface {
	NextSubmissionVersionID(ctx context.Context) (domain.SubmissionVersionID, error)
	NextAcceptanceDecisionTaskID(ctx context.Context) (domain.AcceptanceDecisionTaskID, error)
}

// AcceptanceDecisionIdentity 与 SubmissionIdentityFactory 分开：建单期签发的身份与决定期
// 签发的身份由不同用例触发，合并会让形成决定的编排依赖它根本不签发的那两个身份。
type AcceptanceDecisionIdentity interface {
	NextAcceptanceDecisionID(ctx context.Context) (domain.AcceptanceDecisionID, error)
}

// RecordedJudgments 是接受判断任务上已经采用的全部权威判断。财务控制用零值表示尚未形成，
// 而不是配一个布尔：翻译函数据零值形成`无法判定`，因此「没有控制」无法被悄悄读成通过。
//
// AdoptedCommercialResolution 是这些判断形成时所采用的那次商业解析。它必须留住，否则提交
// 决定前无从做 `UC-PC-002` 步骤 8 的重解——拿决定时刻的新解析自比永远相容，`AT-PC-026` 的
// 提交前失效就永远抓不到，而判断是在旧依据的时点策略下形成的。零值表示还没有任何一轮采用
// 过依据。
type RecordedJudgments struct {
	Reachability                []domain.ReachabilityJudgment
	FinancialControl            domain.FinancialControlResult
	AdoptedCommercialResolution domain.CommercialResolutionID
}

// RecordedJudgmentReader 取回接受判断任务上已记录的判断，供形成决定那一步装配校验结果。
//
// 租户与委托标识两样都收，不是冗余：委托标识的唯一性本身就按租户圈定——`shipment_request`
// 上的 `shipment_request_id_unique` 是（租户 + 委托标识）而不是单列唯一，所以两个租户各有
// 一份同号委托不违反任何约束。只凭委托标识定不到一份委托，跨租户同号的两份就会读到彼此的
// 判断。按 ADR-0003 租户是最高数据隔离边界，跨越它必须在签名上看得见。
//
// 提交版本同样在签名上：可达性与财务控制只交回**这一版**的判断（ADR-0045 Consequences 预告的
// 版本维）。旧版本的判断是对旧内容作出的，受控补充正因内容变了才形成新版本——读跨版本最新会让
// 新版本尚未重判时拿旧版的`可达`去接受，不读版本会让同时点的新判断被旧判断压住；两个方向都是
// 把一版的判断用在另一版上。采用解析不分版本：每轮重解、后写覆盖，读回的恒是本轮那次。
type RecordedJudgmentReader interface {
	LoadRecordedJudgments(
		ctx context.Context,
		tenant domain.TenantID,
		requestID domain.ShipmentRequestID,
		version domain.SubmissionVersionID,
	) (RecordedJudgments, error)
}

type Clock interface {
	Now() time.Time
}

// IntakeEligibilityOutcome 是收寄阶段资格判断的封闭三值（UC-PS-003 资格核对第 5 条）。
// `不适用`单独一格：服务形态不承担网络责任与资格没过是两回事，用不适用代替资格失败被
// 用例明禁。
type IntakeEligibilityOutcome uint8

const (
	IntakeEligibilityOutcomeInvalid IntakeEligibilityOutcome = iota
	IntakeEligibilityEstablished
	IntakeEligibilityNotEstablished
	IntakeServiceNotApplicable
)

func (outcome IntakeEligibilityOutcome) String() string {
	switch outcome {
	case IntakeEligibilityEstablished:
		return "ESTABLISHED"
	case IntakeEligibilityNotEstablished:
		return "NOT_ESTABLISHED"
	case IntakeServiceNotApplicable:
		return "NOT_APPLICABLE"
	default:
		return ""
	}
}

// IntakeEligibility 是资格答复：非`成立`必带依据——没有依据的不适用与来源缺失分不开，
// 没有依据的不成立说不出缺哪条硬资格。
type IntakeEligibility struct {
	Outcome IntakeEligibilityOutcome
	Basis   domain.CheckReason
}

// IntakeEligibilityView 按接受时固定的产品与合同规则判断收寄阶段资格（PAR-COM-16）。
// 第二个返回值为 false 即「资格目录未配置」——保持未决，不默认通过（`AT-PS-047`）；
// 依赖调不通作为错误返回。
type IntakeEligibilityView interface {
	JudgeIntakeEligibility(
		ctx context.Context,
		identity domain.SourceIdentity,
		shipmentRequestID domain.ShipmentRequestID,
		source domain.IntakeSource,
	) (IntakeEligibility, bool, error)
}

// IntakeQualificationProof 是硬资格取证窄口的封闭两值（ADR-0063）。依赖不可用走
// error，不并进未证明，也不折成资格目录未配置——恢复动作不同（ADR-0029）。
type IntakeQualificationProof uint8

const (
	IntakeQualificationProofInvalid IntakeQualificationProof = iota
	IntakeQualificationProven
	IntakeQualificationUnproven
)

func (proof IntakeQualificationProof) String() string {
	switch proof {
	case IntakeQualificationProven:
		return "PROVEN"
	case IntakeQualificationUnproven:
		return "UNPROVEN"
	default:
		return ""
	}
}

// IntakeQualificationEvidenceView 问一条已声明的硬资格在收寄业务时点是否已证明。
// 端口是消费方的话语：身份、来源（含包裹）、引用、时点。未知前缀必须答未证明，
// 不得已证明。正式关务判断不在本口形成。
type IntakeQualificationEvidenceView interface {
	ProveIntakeQualification(
		ctx context.Context,
		identity domain.SourceIdentity,
		source domain.IntakeSource,
		rule domain.QualificationRuleReference,
		asOf time.Time,
	) (IntakeQualificationProof, error)
}

// IntakeAdoptionKey 是采用结果的幂等键：「同一包裹、同一来源类型和同一来源结果版本
// 只能形成一个有效网络收寄采用结果」——三维加租户隔离，全在键上。
type IntakeAdoptionKey struct {
	TenantID domain.TenantID
	Parcel   domain.DeclaredParcelID
	Kind     domain.IntakeSourceKind
	Version  domain.SourceResultVersion
}

// IntakeAdoptionRecord 是一次采用判断越过提交边界后留下的东西：采用（带收寄与承诺）或
// 不采用（带原因）二居其一。ContentDigest 是同一采用身份的内容比对锚——同键异内容是
// 来源冲突，不是重放。
//
// 客户与委托维度随记录带出：审计要求采用结果关联到接受决定与委托，下游（路由复核）的
// 触发键也要这两维——只有幂等键上的包裹身份，触发挂不回明确委托。
//
// SupersedesVersion 只在「同来源更正形成的新采用判断版本」上给出（ADR-0117 决定二）：它
// 指回本记录取代的那一版同种类来源，采用行由此成一条链——根（缺席）是先合法形成的责任
// 起点，此后每一版更正回指前一版；「当前责任起点」是链尾，按回指派生，不存列。带它的
// 记录其承诺必带前版与原因（RestateOnCorrectedIntake 的产物），不带它的承诺必是首版。
type IntakeAdoptionRecord struct {
	Key               IntakeAdoptionKey
	CustomerAccountID domain.CustomerAccountID
	ShipmentRequestID domain.ShipmentRequestID
	ContentDigest     string
	Adopted           bool
	Intake            domain.EffectiveNetworkIntake
	Commitment        domain.FormalCommitment
	SupersedesVersion domain.SourceResultVersion
	RefusalBasis      domain.CheckReason
	AdoptedAt         time.Time
}

// Supersedes 只在更正形成的采用记录上给出：被取代的那一版来源版本。
func (record IntakeAdoptionRecord) Supersedes() (domain.SourceResultVersion, bool) {
	return record.SupersedesVersion, record.SupersedesVersion.String() != ""
}

// IntakeAdoptionSaveOutcome 与其余判断库同一套写入代数（ADR-0031）。
type IntakeAdoptionSaveOutcome uint8

const (
	IntakeAdoptionSaveOutcomeInvalid IntakeAdoptionSaveOutcome = iota
	IntakeAdoptionSaved
	IntakeAdoptionAlreadyRecorded
)

// IntakeAdoptionStore 按幂等键找回并保存采用结果；FindResponsibilityStart 按包裹找回
// 责任起点**当前所在的那一版**——采用链的链尾（没有任何行回指它的那一行 adopted）。
// 链只有一个根（「客户送站与场外揽收都指向同一包裹时不能形成两个责任起点，先合法形成者
// 保留」，`AT-PS-049`），同来源更正在根之后逐版回指（`AT-PS-050`，ADR-0117 决定二）；
// 采用编排拿链尾核对更正关系、写拒绝依据。
type IntakeAdoptionStore interface {
	FindByKey(ctx context.Context, key IntakeAdoptionKey) (IntakeAdoptionRecord, bool, error)
	FindResponsibilityStart(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (IntakeAdoptionRecord, bool, error)
	Save(ctx context.Context, record IntakeAdoptionRecord) (IntakeAdoptionSaveOutcome, error)
}

// CommitmentIdentityFactory 签发正式承诺版本标识。与其余身份工厂分开的理由同
// AcceptanceDecisionIdentity：不同用例触发的签发合并会造出用不上的依赖。
type CommitmentIdentityFactory interface {
	NextCommitmentVersionID(ctx context.Context) (domain.CommitmentVersionID, error)
}

// ParcelCancellationView 按包裹读回已成立的取消决定。采用编排用它核对取消边界：
// 取消早于收寄发生即不采用，收寄发生早于取消决定即顺序冲突保持未决——按业务时间
// 裁决，不按消息到达顺序。
type ParcelCancellationView interface {
	FindCancellation(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.ParcelCancellation, bool, error)
}

// CancellationAuthorityJudgment 是取消授权规则的答复：允许、或不允许带依据。
type CancellationAuthorityJudgment struct {
	Granted bool
	Basis   domain.CheckReason
}

// CancellationAuthorityView 按请求方与目标包裹判断取消授权（PAR-COM-17 实例缝）。
// 第二个返回值为 false 即「授权目录未配置」——保持未决，不写默认授权（红线）；
// 依赖调不通作为错误返回。
type CancellationAuthorityView interface {
	JudgeCancellationAuthority(
		ctx context.Context,
		identity domain.SourceIdentity,
		requester domain.CancellationRequesterReference,
		parcel domain.DeclaredParcelID,
	) (CancellationAuthorityJudgment, bool, error)
}

// CancellationRequestKey 是取消请求的幂等键：同一请求身份返回原逐包裹结果。
type CancellationRequestKey struct {
	TenantID   domain.TenantID
	RequestKey domain.SourceRequestKey
	Parcel     domain.DeclaredParcelID
}

// CancellationRecordKind 是一次取消请求越过提交边界的三种走向：取消成立、边界后转
// 待处置（收寄已先行成立，明确不能回退取消）、规则明确拒绝。
type CancellationRecordKind uint8

const (
	CancellationRecordKindInvalid CancellationRecordKind = iota
	RecordParcelCancelled
	RecordDispositionPending
	RecordCancellationRefused
)

func (kind CancellationRecordKind) String() string {
	switch kind {
	case RecordParcelCancelled:
		return "PARCEL_CANCELLED"
	case RecordDispositionPending:
		return "DISPOSITION_PENDING"
	case RecordCancellationRefused:
		return "CANCELLATION_REFUSED"
	default:
		return ""
	}
}

// CancellationRecord 是一次取消判断留下的东西。`待处置`记录携带越过的收寄引用——
// 处置决定属后续独立判断，这里只登记「取消不能回退、在等明确处置」。
type CancellationRecord struct {
	Key           CancellationRequestKey
	ContentDigest string
	Kind          CancellationRecordKind
	Cancellation  domain.ParcelCancellation
	IntakeVersion domain.SourceResultVersion
	RefusalBasis  domain.CheckReason
	DecidedAt     time.Time
}

type CancellationSaveOutcome uint8

const (
	CancellationSaveOutcomeInvalid CancellationSaveOutcome = iota
	CancellationSaved
	CancellationAlreadyRecorded
)

// ParcelCancellationStore 按请求键找回并保存取消判断（写入代数同 ADR-0031）。
// 采用编排的 ParcelCancellationView 读的就是这里成立的取消决定。
type ParcelCancellationStore interface {
	FindByKey(ctx context.Context, key CancellationRequestKey) (CancellationRecord, bool, error)
	Save(ctx context.Context, record CancellationRecord) (CancellationSaveOutcome, error)
}

// ParcelCancellationHandoffIntent 把已提交的取消决定交给适用下游（路由释放、财务控制
// 释放等各自独立承接）。意图由请求键认领，重放重发同一份（ADR-0043）。
type ParcelCancellationHandoffIntent struct {
	Record CancellationRecord
}

// ParcelCancellationHandoff 把已提交的取消决定写入 Outbox
// （`OutboxParcelCancellationHandoff`）。信封 ID 由取消键（含租户）认领，入队由
// outboxintent.EnqueueOnce 承担；重放重发同一份（ADR-0043）。
type ParcelCancellationHandoff interface {
	HandOffParcelCancellation(ctx context.Context, intent ParcelCancellationHandoffIntent) error
}

// FinalRuleJudgment 是合同终局规则对一份责任结果的答复：满足时带终局类型与规则版本，
// 不满足时带缺口依据。类型由规则给出——网络服务不统一规定跨产品终局集合。
type FinalRuleJudgment struct {
	Satisfied   bool
	Kind        domain.FinalKindReference
	RuleVersion domain.FinalRuleVersionReference
	Basis       domain.CheckReason
}

// FinalRuleView 按接受时固定的产品与合同解析终局规则（PAR-COM-17 实例缝）。第二个
// 返回值为 false 即「终局规则未配置」——保持未决，不默认「有效交付即所有产品终局」
// （红线）；依赖调不通作为错误返回。
type FinalRuleView interface {
	JudgeFinalOutcome(
		ctx context.Context,
		identity domain.SourceIdentity,
		outcome domain.ResponsibilityOutcome,
	) (FinalRuleJudgment, bool, error)
}

// FinalAdoptionKey 是终局采用判断的幂等键：「同一包裹、来源身份和来源版本只能形成
// 一个终局采用判断」。
type FinalAdoptionKey struct {
	TenantID domain.TenantID
	Parcel   domain.DeclaredParcelID
	Kind     domain.ResponsibilityOutcomeKind
	Version  domain.ResponsibilityOutcomeVersion
}

// FinalOutcomeRecord 是一次终局采用判断留下的东西：终局（首派生或重派生）或不采用
// 二居其一。
type FinalOutcomeRecord struct {
	Key           FinalAdoptionKey
	ContentDigest string
	Finalized     bool
	Final         domain.ParcelFinalOutcome
	RefusalBasis  domain.CheckReason
	AdoptedAt     time.Time
}

type FinalOutcomeSaveOutcome uint8

const (
	FinalOutcomeSaveOutcomeInvalid FinalOutcomeSaveOutcome = iota
	FinalOutcomeSaved
	FinalOutcomeAlreadyRecorded
)

// FinalOutcomeStore 按幂等键找回并保存终局采用判断；FindCurrentFinal 按包裹找回当前
// 有效终局（重派生的锚与委托完成派生的读口）。
type FinalOutcomeStore interface {
	FindByKey(ctx context.Context, key FinalAdoptionKey) (FinalOutcomeRecord, bool, error)
	FindCurrentFinal(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (FinalOutcomeRecord, bool, error)
	Save(ctx context.Context, record FinalOutcomeRecord) (FinalOutcomeSaveOutcome, error)
}

// FinalIdentityFactory 签发终局判断版本标识。
type FinalIdentityFactory interface {
	NextFinalOutcomeVersionID(ctx context.Context) (domain.FinalOutcomeVersionID, error)
}

// FinalOutcomeHandoffIntent 把已提交的终局判断交给适用下游（追踪、异常、结算消费稳定
// 引用，不回写终局）。意图由采用键认领，重放重发同一份（ADR-0043）。
type FinalOutcomeHandoffIntent struct {
	Record FinalOutcomeRecord
}

// FinalOutcomeHandoff 把已提交的终局判断写入 Outbox（`OutboxFinalOutcomeHandoff`）。
// 信封 ID 由采用键（含租户）再加类型段认领，入队由 outboxintent.EnqueueOnce 承担；
// 重放重发同一份（ADR-0043）。
type FinalOutcomeHandoff interface {
	HandOffFinalOutcome(ctx context.Context, intent FinalOutcomeHandoffIntent) error
}

// NetworkIntakeHandoffIntent 把一份已提交的采用结果交给适用下游（network-routing 的
// 复核触发正是它的消费者）。意图由采用键认领：同一结果无论交几次都是同一份（ADR-0043）。
type NetworkIntakeHandoffIntent struct {
	Record IntakeAdoptionRecord
}

// NetworkIntakeHandoff 把已提交的采用结果写入 Outbox（`OutboxNetworkIntakeHandoff`）。
// 信封 ID 由采用键（含租户）再加类型段认领，入队由 outboxintent.EnqueueOnce 承担；
// 重放重发同一份（ADR-0043）。
type NetworkIntakeHandoff interface {
	HandOffNetworkIntake(ctx context.Context, intent NetworkIntakeHandoffIntent) error
}

// CommercialBasisQuery 是 parcel-shipment 请 party-commercial 据以解析的范围。它只
// 携带引用：本上下文说明需要哪种依据，绝不指定应当选中哪个商业版本。
type CommercialBasisQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
}

// CommercialBasisResolution 是 parcel-shipment 视角下的一次商业解析结果。
//
// 它不只交回快照。`UC-PC-002` 的消费者端口契约明写「必须返回结构化的唯一成功、无适用依据、
// 适用冲突、解析未决……不能只返回对象或通用错误」，原因就在这里：`无适用依据`要由本上下文
// 形成拒绝，`解析未决`只能保持未决，而两者都表现为「没有快照」，光看快照分不出来。
//
// Reason 是 party-commercial 给的稳定原因引用，本上下文原样记到校验结果上，不重新解释。
type CommercialBasisResolution struct {
	Snapshot      domain.CommercialBasisSnapshot
	Applicability domain.CommercialApplicability
	Reason        domain.CheckReason
}

// JudgmentAsOfQuery 请 party-commercial 为某一类判断校验并回显一个时点值。
//
// 它只回指第一阶段的解析标识，不回传第一阶段的结果对象：跨上下文多步协议的中间状态由提供方
// 按标识保留（ADR-0027）。
//
// 它刻意不带时点值。`UC-PC-002` 步骤 6 要消费方形成值，而 ADR-0025 把「消费方」定为适配器：
// 值按声明的语义在适配器里形成，编排给不出它——编排手上只有本地时钟，拿它顶就是用例明禁的
// 「用一个全局时间代替」。没有租户时适配器形不出值，交回`未配置`，那正是首发要停下的地方。
type JudgmentAsOfQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Resolution        domain.CommercialResolutionID
	Declared          domain.DeclaredAsOf
}

// JudgmentAsOfOutcome 是 party-commercial 第二阶段答复在本上下文的落点。
//
// 五种未成形分开而不压成一个「不可用」：`未配置`等租户把 `PAR-COM-14` 登记上，`未决`等依赖
// 恢复，`值被拒`要本方改这次请求，`依据未解析`要回第一阶段重解，`输入未受理`说的是提供方在
// 任何查询发生之前就短路拒绝了——身份或解析标识立不起来——五者里只有一种靠重试能解决。压平
// 之后调用方只能靠猜，而 ADR-0025 要求提供方封闭集合里每个取值都有明确落点。
//
// `输入未受理`**不**再指「本方指名了一份不属于自己的解析」。越权那一支按 ADR-0029 改落
// `依据未解析`：它与「查无此解析」的恢复动作相同（都得回第一阶段重解），而让两者可区分等于
// 回答一个调用方无权知道的问题——标识存不存在。取值本身保留，短路支仍是它的来源。
type JudgmentAsOfOutcome uint8

const (
	JudgmentAsOfOutcomeInvalid JudgmentAsOfOutcome = iota
	JudgmentAsOfFormed
	JudgmentAsOfBasisNotResolved
	JudgmentAsOfNotConfigured
	JudgmentAsOfPending
	JudgmentAsOfValueRejected
	JudgmentAsOfInputNotAccepted
)

func (outcome JudgmentAsOfOutcome) String() string {
	switch outcome {
	case JudgmentAsOfFormed:
		return "FORMED"
	case JudgmentAsOfBasisNotResolved:
		return "BASIS_NOT_RESOLVED"
	case JudgmentAsOfNotConfigured:
		return "NOT_CONFIGURED"
	case JudgmentAsOfPending:
		return "PENDING"
	case JudgmentAsOfValueRejected:
		return "VALUE_REJECTED"
	case JudgmentAsOfInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// JudgmentAsOfFormation 只在`已形成`时携带时点。其余取值一律不带：交回一个零值时点，调用方
// 会拿一个没人授权过的时刻去推进权威判断。
//
// 不写「其余几种」：那个数字每加一个未成形取值就过期一次，而它过期时没有任何东西会变红。
// 上一次它就是这么错的——`输入未受理`加进来之后这里仍写着四种。
type JudgmentAsOfFormation struct {
	Outcome JudgmentAsOfOutcome
	AsOf    domain.JudgmentAsOf
}

// CommercialRevalidationQuery 请 party-commercial 在本方提交决定前按原查询重解一次。
//
// 与第二阶段同样只回指解析标识：让调用方另给一份解析键，一次「校验」就能拿另一个范围的视图
// 去证明这份解析仍然成立（ADR-0027）。
type CommercialRevalidationQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Resolution        domain.CommercialResolutionID
}

// CommercialRevalidationOutcome 是第三阶段独有的结果代数。
//
// `已失效`必须与`解析未决`分开，不能一起落进 CommercialApplicability 的`无法判定`。两者要
// 采取的动作相反：未决是等权威恢复，本方重试同一次重校验就行；`已失效`重试一万次也还是失效，
// `UC-PC-002` 要的是**重新解析**——回第一阶段拿当前有效的那一份。混成一格，一份被新修订
// 推翻的依据会拿着同一个失效标识把第三阶段重试到底。
//
// 后两个取值属**取回那一步**的拒绝，与前两个「取回成功之后作出的判断」分属不同步骤。
// ADR-0029 只管取回失败那一步，`已失效`不在它管辖内——因此不能援引它把`依据未解析`并进
// `已失效`：那不是应用它的合并规则，是把它扩到它明确划出去的地方。`UC-PC-002` 的结果表也
// 把两者列为两行，交接一节更把它们并列为消费者端口必须分别返回的结构化结果。
//
// 两个一起补而不是先补一个：ADR-0029 要求消费侧同轮改完，只补一个会让中间态既不全函数也
// 不隐蔽。**补落点**本身不需要新记录——ADR-0027 已把「端口结果代数缺口」判为 ADR-0025「翻译
// 必须是全函数」下的端口契约缺陷，改 ports 即可；而`依据未解析`**落成什么形状**（自占一格
// 还是并进`已失效`）是另一问，由 ADR-0033 裁定，权威口径以它为准，此处只作就近说明。
//
// **`依据未解析`刻意不可再分。** 提供方已经把「该标识从未签发」与「它属于另一个客户账户」
// 合并进这一格，为的是不让一串标识挨个问就能枚举同租户下别人的解析。在消费侧拆回两格，就是
// 把刚拆掉的预言机在这边重建一遍——ADR-0029 点名警告过这条下场。
type CommercialRevalidationOutcome uint8

const (
	CommercialRevalidationOutcomeInvalid CommercialRevalidationOutcome = iota
	CommercialBasisStillValid
	CommercialBasisSuperseded
	CommercialRevalidationUndetermined
	CommercialRevalidationBasisNotResolved
	CommercialRevalidationInputNotAccepted
)

func (outcome CommercialRevalidationOutcome) String() string {
	switch outcome {
	case CommercialBasisStillValid:
		return "STILL_VALID"
	case CommercialBasisSuperseded:
		return "SUPERSEDED"
	case CommercialRevalidationUndetermined:
		return "UNDETERMINED"
	case CommercialRevalidationBasisNotResolved:
		return "BASIS_NOT_RESOLVED"
	case CommercialRevalidationInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// CommercialRevalidation 只在`仍然成立`时携带解析。`已失效`不带：交回一份，调用方会以为
// 可以继续用它，而用例明写「不能继续使用或覆盖原历史」。
type CommercialRevalidation struct {
	Outcome    CommercialRevalidationOutcome
	Resolution CommercialBasisResolution
	Reason     domain.CheckReason
}

// ReachabilityRevalidationQuery 请 network-routing 在本方提交决定前核对一次可达性判断
// 是否仍基于当前网络视图（`AT-PS-037` 提交前窗口）。它携带原判断形成时的标识组成——
// 提供方按请求关联持有中间状态（ADR-0027），适配器据这些标识重建同一个关联，本上下文
// 不持有提供方的关联或视图修订。
type ReachabilityRevalidationQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelID  domain.DeclaredParcelID
	AsOf              domain.JudgmentAsOf
}

// ReachabilityRevalidationOutcome 是可达性重校在本上下文的封闭落点。
//
// `已换代`与`无法判定`分开：前者重试一万次也还是换代了，要以新时点重新请求判断；后者等
// 权威恢复重试同一次重校即可。`判断未找回`说的是提供方找不到本方回指的那次判断——恢复
// 动作同样是重新请求判断，但可观测性上它是关联丢失而不是正常换代，压成一格运维会把丢失
// 当成日常。
type ReachabilityRevalidationOutcome uint8

const (
	ReachabilityRevalidationOutcomeInvalid ReachabilityRevalidationOutcome = iota
	ReachabilityJudgmentStillCurrent
	ReachabilityJudgmentSuperseded
	ReachabilityRevalidationJudgmentNotFound
	ReachabilityRevalidationUndetermined
	ReachabilityRevalidationInputNotAccepted
)

func (outcome ReachabilityRevalidationOutcome) String() string {
	switch outcome {
	case ReachabilityJudgmentStillCurrent:
		return "STILL_CURRENT"
	case ReachabilityJudgmentSuperseded:
		return "SUPERSEDED"
	case ReachabilityRevalidationJudgmentNotFound:
		return "JUDGMENT_NOT_FOUND"
	case ReachabilityRevalidationUndetermined:
		return "UNDETERMINED"
	case ReachabilityRevalidationInputNotAccepted:
		return "INPUT_NOT_ACCEPTED"
	default:
		return ""
	}
}

// ReachabilityRevalidation 不在任何取值下携带新判断：重校只回答「失效没失效」，重判是
// 本方以新时点发起的新判断（提供方 CONTEXT 的分工句），混在一起会让一次「核对」悄悄换掉
// 判断本体。
type ReachabilityRevalidation struct {
	Outcome ReachabilityRevalidationOutcome
	Reason  domain.CheckReason
}

// ReachabilityRevalidator 与 CommercialBasisResolver 的第三阶段同属提交前重校窗口，分成
// 两个端口：两类判断由不同权威拥有，合成一个会让某一方的适配器实现它根本答不了的方法。
type ReachabilityRevalidator interface {
	RevalidateReachabilityJudgment(ctx context.Context, query ReachabilityRevalidationQuery) (ReachabilityRevalidation, error)
}

// CommercialBasisResolver 是 parcel-shipment 视角下的 party-commercial 三步协议。
//
// 三个方法对应 `UC-PC-002` 的三个阶段，用例「给开发的交接」要求「第一阶段解析与第二阶段逐项
// `asOf` 必须在类型和测试中可见」，「首发试点叠加条件」要求实现的是「两阶段端口」。它们不合成
// 一个带开关的方法：
// 三步由三个不同时刻的事件触发——开始判断、逐项形成时点、即将提交决定——合成会让编排依赖
// 它当轮根本不会走的分支，也会把提交前重解提前到判断开始时做（ADR-0027）。
type CommercialBasisResolver interface {
	ResolveCommercialBasis(ctx context.Context, query CommercialBasisQuery) (CommercialBasisResolution, error)
	FormJudgmentAsOf(ctx context.Context, query JudgmentAsOfQuery) (JudgmentAsOfFormation, error)
	RevalidateCommercialBasis(ctx context.Context, query CommercialRevalidationQuery) (CommercialRevalidation, error)
}

// ReachabilityRequest 携带按所采用规则包声明的策略形成的判断时点。权威提供方必须校验
// 并回显它——所以它显式随请求传递，而不是留给提供方自己的时钟。
type ReachabilityRequest struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelID  domain.DeclaredParcelID
	AsOf              domain.JudgmentAsOf
}

// ReachabilityOutcome 是 network-routing 一次可达性答复在本上下文的落点。
//
// `请求冲突`与`未受理`不译成 error。它们是业务答案而非技术故障——调用方必须能据以纠正自己
// 这次请求，而不是当作故障重试。译成 error 之后消费侧只剩「依赖答不出」一格，续办路径是
// 内部重试，正是那句话禁止的事：对着一个拼错的请求重试到底，冲突也不会消失。
type ReachabilityOutcome uint8

const (
	ReachabilityOutcomeInvalid ReachabilityOutcome = iota
	ReachabilityAssessed
	ReachabilityNotFormed
	ReachabilityRequestConflict
	ReachabilityRequestNotAccepted
)

func (outcome ReachabilityOutcome) String() string {
	switch outcome {
	case ReachabilityAssessed:
		return "ASSESSED"
	case ReachabilityNotFormed:
		return "NOT_FORMED"
	case ReachabilityRequestConflict:
		return "REQUEST_CONFLICT"
	case ReachabilityRequestNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	default:
		return ""
	}
}

// ReachabilityAssessment 只在`已判断`时携带判断，其余一律不带：交回一份零值判断，编排会把
// 一次没作出的判断记到接受判断任务上。
//
// 提供方的`首次形成`、`重放既有`与`不适用`都落在`已判断`。前两者交回的是同一份判断，重放与
// 首次对本上下文没有分别；`不适用`的分别由判断自身的取值与所携依据带过来，不必在这一层再
// 声明一次——`ReachabilityValue` 已经有那一格。
//
// Reason 是提供方给的稳定原因引用，非`已判断`时给出，本上下文原样记录，不重新解释。
type ReachabilityAssessment struct {
	Outcome  ReachabilityOutcome
	Judgment domain.ReachabilityJudgment
	Reason   domain.CheckReason
}

// ReachabilityAssessor 是 parcel-shipment 视角下的 network-routing 可达性答复。没有一个
// 取值是接受决定，本上下文也不得在这里据其推导出一个。
type ReachabilityAssessor interface {
	AssessParcelReachability(ctx context.Context, request ReachabilityRequest) (ReachabilityAssessment, error)
}

// FinancialControlRequest 是 parcel-shipment 请求一次接受前财务控制的范围。它按当前提交
// 版本取，不像可达性那样按声明包裹取：控制作用在整份委托上，逐成员发起会把一份委托的资金
// 占用重复成成员份数。
//
// 它同样不携带金额、账户或阈值。价格、余额与冻结属 settlement-accounting，策略属
// party-commercial；本上下文说明要为哪份提交版本、按哪个时点控制，仅此而已。
type FinancialControlRequest struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	AsOf              domain.JudgmentAsOf
}

// PreAcceptanceControlOutcome 是 settlement-accounting 一次接受前控制答复在本上下文的落点。
//
// 与可达性同一条分界：`请求冲突`与`未受理`是业务答案，提供方明写「调用方必须能据以纠正，而
// 不是当作故障重试」。译成 error 会让它们并进「依赖答不出」，续办路径变成内部重试。
type PreAcceptanceControlOutcome uint8

const (
	PreAcceptanceControlOutcomeInvalid PreAcceptanceControlOutcome = iota
	PreAcceptanceControlFormed
	PreAcceptanceControlNotFormed
	PreAcceptanceControlRequestConflict
	PreAcceptanceControlRequestNotAccepted
)

func (outcome PreAcceptanceControlOutcome) String() string {
	switch outcome {
	case PreAcceptanceControlFormed:
		return "FORMED"
	case PreAcceptanceControlNotFormed:
		return "NOT_FORMED"
	case PreAcceptanceControlRequestConflict:
		return "REQUEST_CONFLICT"
	case PreAcceptanceControlRequestNotAccepted:
		return "REQUEST_NOT_ACCEPTED"
	default:
		return ""
	}
}

// PreAcceptanceControlAssessment 只在`已形成`时携带结果。提供方的`已执行`与`明确无控制`
// 都落在`已形成`：两者的分别由 FinancialControlOutcome 与所携依据带过来。
//
// 其余取值一律不带结果。交回一个零值结果正是用例禁止的默认放行——一次没能执行的控制会因此
// 看起来像通过了。数字不写进来，理由同 JudgmentAsOfFormation。
type PreAcceptanceControlAssessment struct {
	Outcome PreAcceptanceControlOutcome
	Result  domain.FinancialControlResult
	Reason  domain.CheckReason
}

// PreAcceptanceFinancialController 是 parcel-shipment 视角下的接受前财务控制。没有一个
// 取值是接受决定，本上下文也不得在这里据其推导出一个。
//
// 依赖调不通仍作为错误返回。把它读成`明确无控制`正是用例禁止的默认放行：一次故障会因此
// 变成一个看起来通过了的接受前控制。
type PreAcceptanceFinancialController interface {
	ApplyPreAcceptanceFinancialControl(
		ctx context.Context,
		request FinancialControlRequest,
	) (PreAcceptanceControlAssessment, error)
}

// ControlReleaseRequest 指名要释放哪一次接受前资金控制。它只携带原控制的业务关联，不带
// 金额、账户或币种：释放哪一笔由 settlement-accounting 按原关联认领，冻结不属本上下文。
type ControlReleaseRequest struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	ControlResultID   domain.FinancialControlResultID
}

// PreAcceptanceControlRelease 在接受确定未成立后按原关联请求解除资金控制。
//
// 它与 PreAcceptanceFinancialController 分开：施加控制由推进判断那一步发起，解除由形成决定
// 那一步发起，合并成一个端口会让形成决定的编排依赖一个它根本不会调用的方法。
type PreAcceptanceControlRelease interface {
	ReleasePreAcceptanceControl(ctx context.Context, request ControlReleaseRequest) error
}

// AuthorizationOutcome 是三个授权端口共用的封闭答复集合。端口分开而答案集合共用：授权来源、
// 有效期间与原因目录三者各不相同（那是端口分开的理由），但「答得出什么」只有这三种，且分格
// 维度相同——按消费方的恢复动作分（ADR-0029）。共用一个集合还有一层强制力：日后多一种答复，
// 三处译函数会一起报错，而不是各自静默归入某一格。
//
// `未配置`与`不允许`必须分开，这是本类型存在的全部理由：前者要租户先把 `PAR-COM-14` 的授权
// 规则登记上，后者是权威已经答过的业务拒绝，再登记也不会变。压成一格，**没有租户的首发期
// 每一次请求都会被答以「你无权这么做」**——而真相是还没有人给这个产品配过授权规则。那是红线
// 「实例半边留空并拒绝默认值」在授权这一维上的同一个错。
//
// 这不是新裁断：`JudgmentAsOfOutcome` 早把`未配置`单列，理由一字不差，连待提供的实例参数都
// 是同一个 `PAR-COM-14`。那次只做在时点那一维，这里补上授权这一维。
//
// 零值取`未设`而不取`未配置`：适配器必须说出它看到的是哪一种，靠漏填落进`未配置`会让「问过、
// 确实没规则」与「压根没实现这一支」长得一模一样。漏填因此是一次端口坏了（译函数上抛），
// 不是一次安静的停顿。
type AuthorizationOutcome uint8

const (
	AuthorizationOutcomeInvalid AuthorizationOutcome = iota
	AuthorizationGranted
	AuthorizationRefused
	AuthorizationRulesNotConfigured
)

func (outcome AuthorizationOutcome) String() string {
	switch outcome {
	case AuthorizationGranted:
		return "GRANTED"
	case AuthorizationRefused:
		return "REFUSED"
	case AuthorizationRulesNotConfigured:
		return "RULES_NOT_CONFIGURED"
	default:
		return ""
	}
}

// ActiveRejectionAuthorizationQuery 说明谁要以什么原因主动拒绝哪一份提交版本。它刻意不带
// 授权引用：调用方自带一个，就等于自己给自己签字。
type ActiveRejectionAuthorizationQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Decider           domain.DeciderReference
	Reason            domain.RejectionReasonReference
}

// ActiveRejectionAuthorization 只在`已授权`时携带授权引用，其余取值一律不带：交回一个零值
// 引用，编排会把一次没拿到的授权记进决定。
type ActiveRejectionAuthorization struct {
	Outcome   AuthorizationOutcome
	Authority domain.RejectionAuthorityReference
}

// ActiveRejectionAuthorizer 回答 party-commercial 是否授权这次主动拒绝。授权引用由那边
// 签发，parcel-shipment 只保存所采用的引用——角色等级与原因目录都不属本上下文。
//
// 答不出与答得出分属两回事：前者是错误，后者一律经 AuthorizationOutcome 交回，包括拒绝。
type ActiveRejectionAuthorizer interface {
	AuthorizeActiveRejection(ctx context.Context, query ActiveRejectionAuthorizationQuery) (ActiveRejectionAuthorization, error)
}

// WithdrawalAuthorizationQuery 说明谁要以什么原因撤回哪一份待决委托。与主动拒绝那一支同样
// 不带授权引用：调用方自带一个，就等于自己给自己签字。
type WithdrawalAuthorizationQuery struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	Requester         domain.WithdrawalRequesterReference
	Reason            domain.WithdrawalReasonReference
}

// WithdrawalAuthorizer 回答请求方当前是否有权撤回这份委托。它与 ActiveRejectionAuthorizer
// 分开：一个问的是货主客户或其授权代表，另一个问的是运营侧授权角色，两者的授权来源、有效
// 期间与原因目录都不同，合并会让「客户能不能取消」和「我们能不能不接」共用一套规则。
//
// 答不出与答得出分属两回事：前者是错误，后者一律经 AuthorizationOutcome 交回。真实撤回授权
// 角色与原因语义仍是 `PAR-COM-14` 待提供的实例参数，本上下文不内置任何默认——`UC-PS-005`
// 明禁默认任何角色有撤回权，而把没有规则答成`不允许`同样是一次默认，只是方向朝紧。
type WithdrawalAuthorization struct {
	Outcome   AuthorizationOutcome
	Authority domain.WithdrawalAuthorityReference
}

type WithdrawalAuthorizer interface {
	AuthorizeWithdrawal(ctx context.Context, query WithdrawalAuthorizationQuery) (WithdrawalAuthorization, error)
}

// SourceDataAmendmentAuthorizationQuery 说明谁要以什么原因修订哪一处资料范围。它同样不带
// 授权引用：调用方自带一个，就等于自己给自己签字。
type SourceDataAmendmentAuthorizationQuery struct {
	Identity  domain.SourceIdentity
	Scope     domain.SourceDataScope
	Requester domain.RequesterReference
	Reason    domain.AmendmentReasonReference
}

// SourceDataAmendmentAuthorization 交回本次修订所采用的授权依据与实际决定方。
//
// 两项一起由 party-commercial 给出，不由调用方声明：`UC-PS-002` 要求「登录操作人不能替代实际
// 决定方」，而让请求方自报决定方正是那句话禁止的事。本上下文只保存所采用的那一份，不判断它
// 够不够格——授权规则属 party-commercial。
// 两项只在`已授权`时携带，其余取值一律不带。
type SourceDataAmendmentAuthorization struct {
	Outcome   AuthorizationOutcome
	Authority domain.AmendmentAuthoritySnapshot
	Decider   domain.DeciderReference
}

// SourceDataAmendmentAuthorizer 回答请求方当前是否有权修订这处资料范围。
//
// 答不出与答得出分属两回事：前者是错误，后者一律经 AuthorizationOutcome 交回。真实请求方、
// 实际决定方与授权入口仍是 `BD-PS-009` 待确认的实例参数，本上下文不内置任何默认。
type SourceDataAmendmentAuthorizer interface {
	AuthorizeSourceDataAmendment(
		ctx context.Context,
		query SourceDataAmendmentAuthorizationQuery,
	) (SourceDataAmendmentAuthorization, error)
}

// SourceDataAmendmentAllowance 是某处资料范围在当前阶段上「能不能这样改」的登记结论。
//
// `NotDeclared` 是零值且刻意如此：矩阵没登记就是没登记，而零值必须落在最保守的那一格。让它
// 落在`允许`上，一个尚未登记规则的租户会因为系统便利而放行任意修订，那正是 `UC-PS-002`
// 「未登记时只能形成未决或业务拒绝，不能以系统便利推断允许」明禁的事。
type SourceDataAmendmentAllowance uint8

const (
	SourceDataAmendmentNotDeclared SourceDataAmendmentAllowance = iota
	SourceDataAmendmentAllowed
	SourceDataAmendmentDisallowed
)

// SourceDataAmendmentQuery 说明要判断哪一处资料范围在当前阶段的允许动作。
//
// Intent 随查询进矩阵：允许性按「哪一处、哪个动作」登记，`AT-PS-020` 的显式清空与改成
// 新值在同一阶段的允许性可以相反，矩阵收不到意图就登记不了那种规则。
//
// Stage 同样随查询进矩阵，且由本上下文判出后才发问：矩阵按（资料组 × 阶段 × 意图）登记
// （`UC-PS-002` 阶段表六行、`BD-PS-010`「版本化阶段矩阵」），「此刻在哪个阶段」是 PS 对自己
// 对象生命周期位置的判断，规则包只声明「在某阶段允许什么」。判不出阶段的查询不该发出——
// 编排在那之前就停在未决；实现方收到零值 Stage 应当拒答而不是当最早阶段查。
type SourceDataAmendmentQuery struct {
	Identity domain.SourceIdentity
	Scope    domain.SourceDataScope
	Intent   domain.AmendmentIntent
	Reason   domain.AmendmentReasonReference
	Stage    domain.AmendmentStage
}

// SourceDataRuleDeclaration 回答已登记规则是否允许这次修订。
//
// 字段、字段组、阶段与允许动作由 `PAR-COM-13` 与真实合同、产品、线路和关务规则登记，属实例
// 半边；本上下文只消费登记结论，绝不自带一份矩阵。没有租户时它必然交回 `NotDeclared`，编排
// 据以停在`待复核`——那是「还没人说这能不能改」，不是「客户违规」。
type SourceDataRuleDeclaration interface {
	DeclareSourceDataAmendment(
		ctx context.Context,
		query SourceDataAmendmentQuery,
	) (SourceDataAmendmentAllowance, error)
}

// SourceDataVersionIdentity 签发客户原始资料版本的内部身份。与另外两个身份工厂分开，理由
// 相同：建单期、决定期与修订期由不同用例触发，合并会让一个编排依赖它根本不签发的身份。
type SourceDataVersionIdentity interface {
	NextSourceDataVersionID(ctx context.Context) (domain.SourceDataVersionID, error)
}

// SourceDataVersionHandoffIntent 是步骤 9 交给下游的那份引用。
//
// 它只携带引用与范围，不携带资料内容：跨上下文只传自己拥有的事实与引用，`UC-CC-002` 等下游按
// 各自的业务时间与门禁重新判断，本上下文既不替它们判断，也不把客户声明复制过去。
//
// Adoption 与 Version 一起交出，两者在分叉时并不是同一个。下游要消费的是「此刻该用哪一份」，
// 只发刚形成的那个版本号，会让它把一份尚未合并的支线当成当前资料。
type SourceDataVersionHandoffIntent struct {
	Identity domain.SourceIdentity
	Version  domain.SourceDataVersionID
	Scope    domain.SourceDataScope
	Adoption domain.SourceDataAdoptionJudgment
}

// SourceDataVersionAppendOutcome 是一次版本追加在本上下文的落点。`已记录`不译成
// error，理由与建单的`已存在`相同（ADR-0031）：写入这条路走通了，只是同一个版本号
// 已经在库里——版本只形成一次，重复追加是重放不是故障。
type SourceDataVersionAppendOutcome uint8

const (
	SourceDataVersionAppendOutcomeInvalid SourceDataVersionAppendOutcome = iota
	SourceDataVersionAppended
	SourceDataVersionAlreadyRecorded
)

func (outcome SourceDataVersionAppendOutcome) String() string {
	switch outcome {
	case SourceDataVersionAppended:
		return "APPENDED"
	case SourceDataVersionAlreadyRecorded:
		return "ALREADY_RECORDED"
	default:
		return ""
	}
}

// SourceDataVersionRecords 以委托的来源身份加版本标识为键登记客户原始资料版本。
//
// 它与聚合仓储分立：版本不可覆盖、只追加，而聚合重建门今天只开到`已提交`（ADR-0030），
// 资料版本却只在`已接受`之后形成——把版本塞进聚合快照，重建门一开就得整层重排。独立
// 登记册按版本链平铺（基准/前版引用可查），聚合侧持有的仍是同一批版本的运行时视图。
type SourceDataVersionRecords interface {
	FindVersion(
		ctx context.Context,
		identity domain.SourceIdentity,
		versionID domain.SourceDataVersionID,
	) (domain.CustomerSourceDataVersion, bool, error)
	Append(
		ctx context.Context,
		identity domain.SourceIdentity,
		version domain.CustomerSourceDataVersion,
	) (SourceDataVersionAppendOutcome, error)
}

// SourceDataVersionHandoff 把一份已形成的客户原始资料版本引用交给适用下游。
//
// 一份版本发一份意图，而不是逐下游各设一个端口：哪些下游该重新判断，取决于范围、阶段与各自的
// 门禁，那是下游自己的判断。按下游拆端口会把那份判断搬进本上下文，而 `UC-PS-002` 明写本用例
// 「不形成关务、节点、运输、路由或财务决定」。
//
// 意图由版本标识认领，因此重发的是同一份而不是第二份——`AT-PS-031` 要的「版本只形成一次；仅
// 重试同一发布意图」正落在这里。首次交接失败时编排停在`技术未形成`，版本不因此重形成一遍。
//
// 交接写入 Outbox（`OutboxSourceDataHandoff`）。
type SourceDataVersionHandoff interface {
	HandOffSourceDataVersion(ctx context.Context, intent SourceDataVersionHandoffIntent) error
}

// AcceptanceDecisionHandoffIntent 是一次已越过提交边界的接受决定交给适用下游的那份引用。
//
// 它携带决定标识、委托与提交版本的引用及决定后的生命周期状态，不携带校验明细或基线内容：
// 跨上下文只传引用，下游按各自的门禁重新读取与判断。State 一起交出，因为接受与拒绝都是
// 已形成的决定而下游要办的事不同——只发决定标识会逼每个下游先回读一次才能分流。
type AcceptanceDecisionHandoffIntent struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DecisionID        domain.AcceptanceDecisionID
	State             domain.ShipmentRequestState
}

// AcceptanceDecisionHandoff 把一份已提交的接受决定引用交给适用下游。
//
// 形状与 SourceDataVersionHandoff 是同一条缝：一份结果发一份意图、意图由结果标识认领、
// 重放重发同一份。`AT-PS-013`「重试同一发布意图，不重复接受或回退决定」正落在这里——
// 首次投递失败不改写业务结果，决定已越过提交边界，编排交回决定本身外加一条发布续办引用。
//
// 本上下文不记意图完没完成：那份状态要与决定同一事务落库才算数，而事务与 outbox 仍阻断于
// ADR-0017 的 Bento 闸门。在那之前重放一律重发同一意图，由下游按决定标识认领。它今天没有
// 实现，唯一的实现是测试用的确定性替身。
type AcceptanceDecisionHandoff interface {
	HandOffAcceptanceDecision(ctx context.Context, intent AcceptanceDecisionHandoffIntent) error
}

// ShipmentRequestSummaryRecord 是委托查阅列表的一行。它是读模型而不是聚合：查阅要跨
// 生命周期覆盖`已拒绝`/`已撤回`，而聚合重建门只开到`已提交`与`已接受`（ADR-0061），
// 走聚合仓储的查阅会让恰恰最需要交代的两种终局读不出来。
type ShipmentRequestSummaryRecord struct {
	CustomerAccountID   domain.CustomerAccountID
	Source              domain.Source
	SourceRequestKey    domain.SourceRequestKey
	ShipmentRequestID   domain.ShipmentRequestID
	State               domain.ShipmentRequestState
	SubmissionVersionID domain.SubmissionVersionID
	DeclaredParcelCount int
	SubmittedAt         time.Time
}

// DeclaredParcelViewRecord 是详情里的一件声明包裹。测量值保持客户引用原样的字符串
// （"2.50" 不规范化），与快照存的形状一致；缺画像时测量字段为空——画像本就可缺。
type DeclaredParcelViewRecord struct {
	Parcel         domain.DeclaredParcelID
	WeightValue    string
	WeightUnit     string
	HasDimensions  bool
	Length         string
	Width          string
	Height         string
	DimensionsUnit string
}

// AcceptanceDecisionViewRecord 是详情里已形成的接受决定摘要。只带查阅要用的三样，
// 校验明细与商业依据不进读面——那是复核与争议入口的事。
type AcceptanceDecisionViewRecord struct {
	DecisionID domain.AcceptanceDecisionID
	Accepted   bool
	DecidedAt  time.Time
}

// AcceptanceTaskViewRecord 是详情里当前接受判断任务的查阅面：任务阶段、最近一次没能
// 推进的处理记录、当前停在哪个等待态、复核完成留痕。前两样是用例对任务的要求——只留
// 成功判断，一份卡了十轮的委托看起来会和刚建单的一模一样；后两样是复核队列查阅面
// （票 acceptance-review-read-face/01）要如实呈现的「停在哪、复核录了没」——WaitingOn
// 零值即缺席（决定已形成或任务已完成），复核留痕四字段只在 ReviewCompleted 时在场。
type AcceptanceTaskViewRecord struct {
	State                   domain.AcceptanceTaskState
	HasAttempt              bool
	LastAttemptReason       string
	LastAttemptContinuation string
	LastAttemptedAt         time.Time
	WaitingOn               domain.ResumePath
	ReviewCompleted         bool
	ReviewAuthority         string
	ReviewReviewer          string
	ReviewEvidence          string
	ReviewCompletedAt       time.Time
}

// ShipmentRequestDetailRecord 是单份委托的查阅详情。
type ShipmentRequestDetailRecord struct {
	ShipmentRequestSummaryRecord
	BatchID           domain.SubmissionBatchID
	OccurredAt        time.Time
	ReceivedAt        time.Time
	DeclaredParcels   []DeclaredParcelViewRecord
	PriorVersionCount int
	Task              AcceptanceTaskViewRecord
	HasDecision       bool
	Decision          AcceptanceDecisionViewRecord
}

// ShipmentRequestViews 是委托查阅的读口（CONTEXT「授权查询作用域」）。
//
// 两个方法都以完整作用域为键：过滤在键上而不在结果后处理上，读口因此答不出作用域外
// 的行。FindVisibleByID 的否定结果不区分「不存在」「属其他租户或客户账户」——那正是
// CONTEXT`统一不可见结果`在读口上的形状，拆开就是造跨作用域存在性预言机（ADR-0029）。
//
// Limit 必须为正；每页多大由接入面按渠道契约裁决，读口只拒绝无意义的取值。
type ShipmentRequestViews interface {
	ListVisible(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		limit int,
	) ([]ShipmentRequestSummaryRecord, error)
	FindVisibleByID(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		requestID domain.ShipmentRequestID,
	) (ShipmentRequestDetailRecord, bool, error)
}

// AcceptanceReviewQueueRecord 是复核队列上的一行：一份当前停在「等待人工复核」的
// 委托（票 acceptance-review-read-face/01）。概要之外带两样队列要看的东西——最近
// 一次没能推进的处理记录（这份委托此前卡在哪）与复核完成留痕（复核已录完成但还没
// 续办的行照列并标示，出队靠下一轮判断清等待态，不靠读侧折叠）。「停等起点」刻意
// 缺席：未决的一轮不落决定时刻，读面没有它就不上列。
type AcceptanceReviewQueueRecord struct {
	ShipmentRequestSummaryRecord
	HasAttempt              bool
	LastAttemptReason       string
	LastAttemptContinuation string
	LastAttemptedAt         time.Time
	ReviewCompleted         bool
	ReviewAuthority         string
	ReviewReviewer          string
	ReviewEvidence          string
	ReviewCompletedAt       time.Time
}

// AcceptanceReviewQueue 是复核队列查阅的读口。队列的定义就是任务文档上的
// `waitingOn = MANUAL_REVIEW`——那由 Decide 看过全部校验后写下（三个终态转移各自
// 清零），读口照登记过滤，不在读侧重推域判断。
//
// FindVisibleByID 与 ShipmentRequestViews 同签名不是巧合：复核详情复用委托查阅详情
// （审的就是这份委托登记过什么），不另造第二种详情。同一真库适配器同时满足两口。
//
// 排序与列表读口相反：老的在前（先来先审），同刻按委托标识正序保证分页可重复。
type AcceptanceReviewQueue interface {
	ListAwaitingManualReview(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		limit int,
	) ([]AcceptanceReviewQueueRecord, error)
	FindVisibleByID(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		requestID domain.ShipmentRequestID,
	) (ShipmentRequestDetailRecord, bool, error)
}

// CustomerSupplementQueueRecord 是「等待受控补充」队列上的一行：一份当前停在`等待受控补充`
// 的`已提交`委托（ADR-0106 Consequences「等客户补件的都有谁」）。概要之外只带最近一次没能
// 推进的处理记录——这份委托此前卡在哪、续办引用是什么，是运营催客户补件时要看的东西。
// 没有复核完成那组字段：这一格的续办方是客户，续办动作是新提交版本，形成之后任务换代、
// 等待态随之清零，出队靠下一轮判断不靠读侧折叠（与复核队列同一条纪律）。
type CustomerSupplementQueueRecord struct {
	ShipmentRequestSummaryRecord
	HasAttempt              bool
	LastAttemptReason       string
	LastAttemptContinuation string
	LastAttemptedAt         time.Time
}

// CustomerSupplementQueue 是受控补充队列查阅的读口。队列的定义就是投影列上的
// `waitingOn = CUSTOMER_SUPPLEMENT` 且 `state = SUBMITTED`——由 Decide 看过全部校验后写下、
// 由形成新版本与各终态转移改写或清零，读口照登记过滤，不在读侧重推域判断。
//
// 以授权查询作用域为键而不是租户：这一口是查阅读面（客户或运营看「谁在等补件」），与复核
// 队列同属 CONTEXT「授权查询作用域」管辖，作用域外的行答不出——登记续办门那种按租户整批
// 重驱的口不在这里，受控补充的续办由每份委托自己的「新提交版本已形成」信封驱动，不需要
// 扫队列。只有列表一口：详情本就是委托查阅详情（ShipmentRequestViews.FindVisibleByID），
// 接查阅端点的那笔工作要哪几口由它自己拼，这里不预先替它并进来。
//
// 排序与列表读口相反：老的在前（先停的先催），同刻按委托标识正序保证分页可重复。
type CustomerSupplementQueue interface {
	ListWaitingOnCustomerSupplement(
		ctx context.Context,
		scope domain.AuthorizedQueryScope,
		limit int,
	) ([]CustomerSupplementQueueRecord, error)
}

// OperatorRegistrationQueueRecord 是「等待运营登记」队列上的一行：一份在决定形成之前停在
// `等待运营登记`的`已提交`委托，带的正是重驱接受判断链要的那几样（AdvanceAcceptanceChainCommand
// 的输入）。它不是查阅读面：没有概要、没有处理记录——那些归 ShipmentRequestViews；这里只给续办
// 消费门一份「该重驱谁」的名单（ADR-0094 Decision 四）。
type OperatorRegistrationQueueRecord struct {
	Identity          domain.SourceIdentity
	ShipmentRequestID domain.ShipmentRequestID
	SubmissionVersion domain.SubmissionVersionID
	DeclaredParcelIDs []domain.DeclaredParcelID
	SubmittedAt       time.Time
}

// OperatorRegistrationQueue 按租户列出当前停在`等待运营登记`的`已提交`委托，老的在前，同刻按
// 委托标识正序保证分页可重复（ADR-0094 Decision 五）。
//
// 以租户为键而不是授权查询作用域：调用方是「参数已登记」信封的消费门，它代表登记那一侧的运营
// 企业，一次登记解开的是该租户下所有等这份参数的委托，不是某几个客户账户的。作用域读口
// （AcceptanceReviewQueue）那套统一不可见纪律守的是跨作用域存在性泄露，而这一口不出进程。
//
// 队列的定义就是投影列上的 `waitingOn = OPERATOR_REGISTRATION` 且 `state = SUBMITTED`（迁移 0013
// 的部分索引逐字吻合）。等待态由 AwaitOperatorRegistration 在决定之前写下，由 Decide 与各终态
// 转移改写或清零；读口照登记过滤，不在读侧重推。Limit 必须为正，判据同 ShipmentRequestViews。
type OperatorRegistrationQueue interface {
	ListWaitingOnOperatorRegistration(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]OperatorRegistrationQueueRecord, error)
}

// LabelTransactionInsertOutcome 与 LabelTransactionSaveOutcome 是面单交易写入的两套代数，
// 与委托那两套同形同理（ADR-0031）：`已存在`要回去按重放规则重答，`版本冲突`要重读再重放，
// 两者的恢复动作不同，因此不共用一个集合，也都不译成 error。
//
// 它们与 ShipmentRequest 那两套**不复用同一个类型**：面单交易是独立聚合（ADR-0084 决定一），
// 共用类型会让某天其中一侧多一格取值时另一侧被迫接受一个它答不出的答案。
type LabelTransactionInsertOutcome uint8

const (
	LabelTransactionInsertOutcomeInvalid LabelTransactionInsertOutcome = iota
	LabelTransactionInserted
	LabelTransactionAlreadyExists
)

func (outcome LabelTransactionInsertOutcome) String() string {
	switch outcome {
	case LabelTransactionInserted:
		return "INSERTED"
	case LabelTransactionAlreadyExists:
		return "ALREADY_EXISTS"
	default:
		return ""
	}
}

type LabelTransactionSaveOutcome uint8

const (
	LabelTransactionSaveOutcomeInvalid LabelTransactionSaveOutcome = iota
	LabelTransactionSaved
	LabelTransactionRevisionConflict
)

func (outcome LabelTransactionSaveOutcome) String() string {
	switch outcome {
	case LabelTransactionSaved:
		return "SAVED"
	case LabelTransactionRevisionConflict:
		return "REVISION_CONFLICT"
	default:
		return ""
	}
}

// LabelTransactionRepository 以（租户 + 面单交易标识）为键存储面单交易聚合。
//
// 键不含来源身份，也不含委托：覆盖包裹可以跨委托（ADR-0084 决定一），拿委托的来源身份作键
// 会让一笔跨委托交易无处安放。Insert 与 Save 分开的理由同委托仓储：建立只发生一次，结果与
// 后续动作是在既有交易上推进。Save 的预期版本由聚合自己携带（`transaction.Revision()`）。
//
// **本口今天没有生产写入方，这是设计而不是欠账**：写入方是渠道适配器，而首发基线明写「独立
// 面单渠道服务不进入首发生产」。机制先立起来，墙降那天写编排对着的不是一张裸表。
type LabelTransactionRepository interface {
	FindByID(
		ctx context.Context,
		tenant domain.TenantID,
		transactionID domain.LabelTransactionID,
	) (domain.LabelTransaction, bool, error)
	Insert(ctx context.Context, transaction domain.LabelTransaction) (LabelTransactionInsertOutcome, error)
	Save(ctx context.Context, transaction domain.LabelTransaction) (LabelTransactionSaveOutcome, error)
}

// LabelTransactionsByParcelView 按包裹取回其**全部**相关面单交易——首笔、重试、替代、换单，以及
// 违反截断边界的边界后交易，一笔不筛（CONTEXT：「任何已经实际形成且归属该包裹的面单结果都必须
// 参与终局判断」）。它是包裹终局跨交易判断（JudgeLabelServiceFinal）的读口：按交易标识取一笔的
// FindByID 答不出「这个包裹还关联着哪些交易」。空切片是诚实答案——没有过任何交易。
//
// 与 LabelTransactionRepository 分名：读方拿到的是聚合本体（判断要读结果、定案与后续动作），但不
// 该拿到 Insert/Save。
type LabelTransactionsByParcelView interface {
	ListByCoveredParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) ([]domain.LabelTransaction, error)
}

// LabelValidityRuleView 按「接受时固定的有效期规则」判一笔交易上该包裹的成功面单结果是否已
// 不可逆失效（CONTEXT：「自然失效必须来自渠道确认或接受时固定的有效期规则」；关闭路径的
// 终局服务结果要求「已有成功结果均已成功作废或依据接受时固定的规则不可逆失效」）。
//
// 第二个返回值为 false 即规则未配置——那是实例半边：没有规则就没有失效，那笔成功照常阻止终局，
// 不按墙钟推算过期。依赖调不通作为错误返回。
type LabelValidityRuleView interface {
	JudgeLabelLapsed(
		ctx context.Context,
		tenant domain.TenantID,
		transaction domain.LabelTransaction,
		parcel domain.DeclaredParcelID,
		asOf time.Time,
	) (bool, bool, error)
}

// LabelTransactionParcelRow 是读面上「交易 × 包裹」那一行（ADR-0084 决定七：页面行粒度在
// 读侧由快照摊开）。每件覆盖包裹恒有一行，结果未回时也在——覆盖范围是建立即固定的事实，
// 结果回来与否不改变「这笔交易覆盖了它」。
//
// HasResult 因此必须与 Accepted 分开：结果未回的一行 Accepted 为零值，而零值读作「未受理」
// 就是把「还没有结果」当成失败处理，与 CONTEXT 禁止的「结果不确定按失败处理」是同一个错，
// 只是错在读面这一侧。两层结果照搬不折算，也不互推——CONTEXT 明写「部分成功、部分失败或
// 不同作废范围不得压缩成一个无法解释的通用状态」。
type LabelTransactionParcelRow struct {
	Parcel     domain.DeclaredParcelID
	HasResult  bool
	Accepted   bool
	Identifier string
	Reason     string
	// FollowUpKinds 是作用到本件包裹的后续动作种类，按追加顺序；整笔范围的动作作用于每一件，
	// 因此也出现在这里。它与 Accepted 并列而不改写它：渠道作废与「当初有没有被受理」是两件事。
	FollowUpKinds []domain.FollowUpActionKind
	// ContinuedAttemptOpen 是**包裹级**继续尝试判断，按 CONTEXT「只由有效的关闭、重开决定
	// 及当前有效终局结果派生」由登记册聚合的 Judge 现算，不是存下来的状态。读面把它与
	// ContinuedAttemptDecided 并列交出：「开放」既可能是有人作过决定后重开，也可能是根本没有人
	// 作过决定——两者在这一个布尔上长得一样，读的人要分得开。
	ContinuedAttemptOpen bool
	// ContinuedAttemptDecided 说这件包裹的继续尝试决定登记册里**有没有任何决定**（CONTEXT
	// 「没有人作过决定」那一句的载体）。为假时 ContinuedAttemptOpen 的开放只来自「无生效关闭且无
	// 当前有效终局」，不来自任何人的判断。
	ContinuedAttemptDecided bool
}

// LabelTransactionRecord 是面单交易查阅的一笔。
type LabelTransactionRecord struct {
	TransactionID          domain.LabelTransactionID
	State                  domain.LabelTransactionState
	Finalized              bool
	ChannelAccount         string
	AccountHolder          string
	ServiceProvider        string
	SettlementCounterparty string
	Contract               string
	Rate                   string
	ResponsibilityBasis    string
	EstablishedAt          time.Time
	SubmittedAt            time.Time
	ResultObservedAt       time.Time
	PriorTransactionID     string
	PriorLinkKind          domain.LabelTransactionLinkKind
	Parcels                []LabelTransactionParcelRow
	// Documents 挂在交易级而不是包裹行上（ADR-0092 决定一）：一份批粒度件覆盖多件包裹，
	// 挂到包裹行就等于把它复制成 N 行，而那正是端口形状当初拒绝的「拆成假的逐件」——复制
	// 之后没有任何东西说得出这 N 行其实是同一张纸。要按包裹看，从每一行自带的覆盖范围过滤。
	Documents []LabelTransactionDocumentRow
}

// LabelTransactionDocumentRow 是读面上的一条载荷记录。
//
// 顺序即追加顺序，**不要取最后一条**：重打产生的新件与被替换的旧件在时间上相邻而在业务上
// 不同，要哪一份得按业务规则挑（ADR-0092 决定三）。
//
// 这里没有定位符。读面回答的是「有没有这份件、它是什么、本体拿不拿得到」；本体在哪是存放
// 端口的事，把存放地址摊到查阅面上，等于让查阅面替存放方作证。
type LabelTransactionDocumentRow struct {
	Role           string
	Format         string
	Granularity    domain.LabelDocumentGranularity
	CoveredParcels []domain.DeclaredParcelID
	Digest         string
	// BodyStored 派生自定位符在不在，不是存下来的一格。
	//
	// **它今天恒为假，而这是真话不是默认值**：本体存放是一条未配置的出向缝（ADR-0092
	// 决定二），文件组件尚不存在，所以每一条载荷都只有摘要没有本体。读面要在页头把这条
	// 依据讲明白，免得读成「存过但取不回来」——那是两件事。
	BodyStored bool
	ObservedAt time.Time
}

// LabelTransactionViews 是面单交易查阅的读口。
//
// 签名收（租户, limit）而不是完整授权作用域：客户账户不是交易的维度——覆盖包裹可以跨委托，
// 按客户过滤会把一笔跨客户的交易归给其中一个客户（ADR-0084 决定七）。租户维照 ADR-0003
// 的隔离边界照常在。
//
// 命令面不在这里：本上下文的面单交易写入等渠道墙，本切片根本不开写行。
type LabelTransactionViews interface {
	ListLabelTransactions(
		ctx context.Context,
		tenant domain.TenantID,
		limit int,
	) ([]LabelTransactionRecord, error)
}

// AcceptanceJudgmentRecorder 把一个已采用的判断记到它所推进的那份委托的接受判断任务上。
//
// RecordProcessingAttempt 记的是没能推进的那一轮。用例要求任务「追加判断与处理尝试」两样
// 都留：只留成功的判断，一份卡了十轮的委托看起来会和刚建单的一模一样。
// 四个方法都收租户，理由与 RecordedJudgmentReader 同一条：委托标识的唯一性按租户圈定。
//
// 两类判断还收提交版本：判断是对某一版内容作出的，任务随版本重立（ADR-0045），记下它属于哪一版，
// 读口才能只把当前版本的判断交给形成决定那一步。处理尝试与采用解析不收——尝试不参与决定，续办
// 引用的派生已含版本；采用解析每轮按版本重解、后写覆盖。
type AcceptanceJudgmentRecorder interface {
	RecordReachabilityJudgment(
		ctx context.Context,
		tenant domain.TenantID,
		requestID domain.ShipmentRequestID,
		version domain.SubmissionVersionID,
		judgment domain.ReachabilityJudgment,
	) error
	RecordFinancialControlResult(
		ctx context.Context,
		tenant domain.TenantID,
		requestID domain.ShipmentRequestID,
		version domain.SubmissionVersionID,
		result domain.FinancialControlResult,
	) error
	RecordProcessingAttempt(
		ctx context.Context,
		tenant domain.TenantID,
		requestID domain.ShipmentRequestID,
		attempt domain.ProcessingAttempt,
	) error
	// RecordAdoptedCommercialResolution 记下本轮采用的那次商业解析，供提交决定前按它重解。
	//
	// 它与三个判断记录方法分开：解析在任何一项判断之前就被采用，两类判断也共用同一次解析，
	// 挂到某一个判断的记录上会让另一类判断的轮次看起来没有采用过依据。重复记录同一标识是
	// 幂等的——一份委托的多轮判断本就该采用同一次解析。
	//
	// 记入不同标识时以后写的为准。决定期的提交前重解会采用新的一次解析并再记一次，若不覆盖，
	// 下一轮读回的就是已被取代的那次，重校会对着陈旧依据做。
	RecordAdoptedCommercialResolution(
		ctx context.Context,
		tenant domain.TenantID,
		requestID domain.ShipmentRequestID,
		resolution domain.CommercialResolutionID,
	) error
}

// ContinuedAttemptRegisterInsertOutcome 与 ContinuedAttemptRegisterSaveOutcome 是`面单继续尝试
// 决定`登记册写入的两套代数，形照面单交易那一对（ADR-0031）：`已存在`要回去读那一册再重放，
// `版本冲突`要重读再重放，恢复动作不同，因此不共用一个集合，也都不译成 error。
//
// 不复用 LabelTransaction 那两个类型，理由与那一对不复用委托的相同：登记册是另一个聚合（键为
// 租户 + 包裹），共用类型会让某天其中一侧多一格取值时另一侧被迫接受一个它答不出的答案。
type ContinuedAttemptRegisterInsertOutcome uint8

const (
	ContinuedAttemptRegisterInsertOutcomeInvalid ContinuedAttemptRegisterInsertOutcome = iota
	ContinuedAttemptRegisterInserted
	ContinuedAttemptRegisterAlreadyExists
)

func (outcome ContinuedAttemptRegisterInsertOutcome) String() string {
	switch outcome {
	case ContinuedAttemptRegisterInserted:
		return "INSERTED"
	case ContinuedAttemptRegisterAlreadyExists:
		return "ALREADY_EXISTS"
	default:
		return ""
	}
}

type ContinuedAttemptRegisterSaveOutcome uint8

const (
	ContinuedAttemptRegisterSaveOutcomeInvalid ContinuedAttemptRegisterSaveOutcome = iota
	ContinuedAttemptRegisterSaved
	ContinuedAttemptRegisterRevisionConflict
)

func (outcome ContinuedAttemptRegisterSaveOutcome) String() string {
	switch outcome {
	case ContinuedAttemptRegisterSaved:
		return "SAVED"
	case ContinuedAttemptRegisterRevisionConflict:
		return "REVISION_CONFLICT"
	default:
		return ""
	}
}

// ContinuedAttemptRegisterRepository 以（租户 + 包裹）为键存储`面单继续尝试决定`登记册。
//
// 键是包裹而不是面单交易：CONTEXT 把决定定义为「针对明确包裹当前完整面单服务范围形成」，一个
// 包裹可关联多笔重试、替代、作废或换单交易，挂在交易上会让同一个包裹在不同交易下各有一套关闭
// 状态。键也不含委托：覆盖包裹可以跨委托（ADR-0084 决定一）。
//
// Insert 与 Save 分开，理由同面单交易仓储：开册只发生一次，决定是在既有册上追加。Save 的预期
// 版本由聚合自己携带（`register.Revision()`）。否定的 FindByParcel 只回 false，不区分「没开过册」
// 与「属于另一个租户」——区分它们等于泄露其他租户下是否存在该包裹。
//
// **本口今天没有生产写入方，这是设计而不是欠账**：形成关闭或重开决定的命令口要先过 party-commercial
// 的授权规则校验（CONTEXT「形成关闭或重开决定时仍须重新校验当前角色与客户授权」），那是另一张票；
// 本票立的是册、端口、持久化与读面派生四层，让那张票落地那天对着的不是一张裸表。
type ContinuedAttemptRegisterRepository interface {
	FindByParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.ContinuedAttemptRegister, bool, error)
	Insert(ctx context.Context, register domain.ContinuedAttemptRegister) (ContinuedAttemptRegisterInsertOutcome, error)
	Save(ctx context.Context, register domain.ContinuedAttemptRegister) (ContinuedAttemptRegisterSaveOutcome, error)
}

// ContinuedAttemptRegisterView 是登记册的只读半边：包裹终局的跨交易判断要读它派生`受控关闭`
// 与作为证据的那份生效关闭，但不该拿到开册与追加的写口。同一个适配器两个接口都满足。
type ContinuedAttemptRegisterView interface {
	FindByParcel(
		ctx context.Context,
		tenant domain.TenantID,
		parcel domain.DeclaredParcelID,
	) (domain.ContinuedAttemptRegister, bool, error)
}
