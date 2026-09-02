package domain

import (
	"errors"
	"time"
)

var (
	// ErrInvalidLabelTransaction 说这次要发生的事情本身立不起来：输入缺项、覆盖范围不合
	// 规矩、结果与覆盖对不上。续办动作是改输入重来。
	ErrInvalidLabelTransaction = errors.New("parcel shipment: invalid label transaction")
	// ErrLabelTransactionStateNotAdmitted 说输入没问题，是这笔交易此刻不在允许这一步的
	// 状态上（ADR-0029 分格）。续办动作完全不同——去看它当前停在哪一格、等渠道结果回来，
	// 而不是改参数重试。压成一格，调用方会对着一份完好的请求反复重发。
	ErrLabelTransactionStateNotAdmitted = errors.New("parcel shipment: label transaction state does not admit this step")
	// ErrPriorLabelTransactionNotFinalized 说被指的原交易还没定案。它单独成格，是因为这里
	// 要拦的是一句具体的错话：对结果不确定的交易发起重试或替代，就是把它按失败处理了，而
	// CONTEXT 明禁这一步。调用方该等结果回来，不是换个方向再试。
	ErrPriorLabelTransactionNotFinalized = errors.New("parcel shipment: prior label transaction is not finalized")
)

// LabelTransactionID 是面单交易的内部身份，由本上下文签发。渠道订单号、主号一类外部
// 标识按 CONTEXT「外部标识必须关联其真实标识对象」关联到交易上，但不充当这个身份：
// 换了渠道或渠道自己改号，交易的身份不该跟着变。
type LabelTransactionID struct{ requiredValue }

func NewLabelTransactionID(value string) (LabelTransactionID, error) {
	required, err := newRequiredValue("label transaction ID", value)
	return LabelTransactionID{required}, err
}

// ChannelAccountReference 指名发起本次渠道业务请求所用的渠道账号。
type ChannelAccountReference struct{ requiredValue }

func NewChannelAccountReference(value string) (ChannelAccountReference, error) {
	required, err := newRequiredValue("channel account reference", value)
	return ChannelAccountReference{required}, err
}

// ChannelAccountHolderReference 指名渠道账号持有人。它与渠道服务方、合同与结算相对方
// 是三份各自独立的参与方角色（CONTEXT 面单交易一族），压成一个「相对方」会让渠道责任
// 与结算责任在同一笔交易上找错人——账号可以是客户自有，服务方是渠道，结算相对方又
// 可能是第三方。
type ChannelAccountHolderReference struct{ requiredValue }

func NewChannelAccountHolderReference(value string) (ChannelAccountHolderReference, error) {
	required, err := newRequiredValue("channel account holder reference", value)
	return ChannelAccountHolderReference{required}, err
}

// ChannelServiceProviderReference 指名承接本次请求的渠道服务方。
type ChannelServiceProviderReference struct{ requiredValue }

func NewChannelServiceProviderReference(value string) (ChannelServiceProviderReference, error) {
	required, err := newRequiredValue("channel service provider reference", value)
	return ChannelServiceProviderReference{required}, err
}

// SettlementCounterpartyReference 指名合同与结算相对方。
type SettlementCounterpartyReference struct{ requiredValue }

func NewSettlementCounterpartyReference(value string) (SettlementCounterpartyReference, error) {
	required, err := newRequiredValue("settlement counterparty reference", value)
	return SettlementCounterpartyReference{required}, err
}

// ChannelContractReference 指名本次交易所依据的合同。合同本体属 party-commercial，
// 这里只保存实际采用的依据。
type ChannelContractReference struct{ requiredValue }

func NewChannelContractReference(value string) (ChannelContractReference, error) {
	required, err := newRequiredValue("channel contract reference", value)
	return ChannelContractReference{required}, err
}

// ChannelRateReference 指名本次交易适用的费率。
type ChannelRateReference struct{ requiredValue }

func NewChannelRateReference(value string) (ChannelRateReference, error) {
	required, err := newRequiredValue("channel rate reference", value)
	return ChannelRateReference{required}, err
}

// ResponsibilityBasisSnapshotReference 指名建立时固定下来的渠道角色与责任依据快照。
type ResponsibilityBasisSnapshotReference struct{ requiredValue }

func NewResponsibilityBasisSnapshotReference(value string) (ResponsibilityBasisSnapshotReference, error) {
	required, err := newRequiredValue("responsibility basis snapshot reference", value)
	return ResponsibilityBasisSnapshotReference{required}, err
}

// ChannelParcelIdentifier 是渠道对某件包裹签发的包裹级标识（件号、尾程号一类）。它按
// CONTEXT「外部标识必须关联其真实标识对象」挂在包裹结果上，不是包裹身份本身。
type ChannelParcelIdentifier struct{ requiredValue }

func NewChannelParcelIdentifier(value string) (ChannelParcelIdentifier, error) {
	required, err := newRequiredValue("channel parcel identifier", value)
	return ChannelParcelIdentifier{required}, err
}

// ChannelResultReasonReference 指名渠道给出的结果原因。
type ChannelResultReasonReference struct{ requiredValue }

func NewChannelResultReasonReference(value string) (ChannelResultReasonReference, error) {
	required, err := newRequiredValue("channel result reason reference", value)
	return ChannelResultReasonReference{required}, err
}

// LabelTransactionState 是交易级状态的封闭集合，直接取自 CONTEXT 的「面单交易」生命
// 周期。`结果不确定`在这里是独立一格而不是失败的别名——硬句「该结果不得直接按失败
// 处理」靠这个集合本身交付，任何把两者合并的表示法都会在类型上放行那句禁令。
type LabelTransactionState uint8

const (
	LabelTransactionStateInvalid LabelTransactionState = iota
	LabelTransactionEstablished
	LabelTransactionSubmitted
	LabelTransactionResultUncertain
	LabelTransactionSucceeded
	LabelTransactionPartiallySucceeded
	LabelTransactionFailed
)

func (state LabelTransactionState) String() string {
	switch state {
	case LabelTransactionEstablished:
		return "ESTABLISHED"
	case LabelTransactionSubmitted:
		return "SUBMITTED_TO_CHANNEL"
	case LabelTransactionResultUncertain:
		return "RESULT_UNCERTAIN"
	case LabelTransactionSucceeded:
		return "SUCCEEDED"
	case LabelTransactionPartiallySucceeded:
		return "PARTIALLY_SUCCEEDED"
	case LabelTransactionFailed:
		return "FAILED"
	default:
		return ""
	}
}

// LabelTransactionLinkKind 是新交易指回原交易的关系方向，取自 CONTEXT「一个包裹可以关联
// 多笔具有重试、替代、作废或换单关系的交易」：作废是后续动作不是新交易，换单在形状上就是
// 替代，因此这里只有两格。
type LabelTransactionLinkKind uint8

const (
	LabelTransactionLinkKindInvalid LabelTransactionLinkKind = iota
	LabelTransactionRetry
	LabelTransactionReplacement
)

func (kind LabelTransactionLinkKind) valid() bool {
	return kind == LabelTransactionRetry || kind == LabelTransactionReplacement
}

func (kind LabelTransactionLinkKind) String() string {
	switch kind {
	case LabelTransactionRetry:
		return "RETRY"
	case LabelTransactionReplacement:
		return "REPLACEMENT"
	default:
		return ""
	}
}

// PriorLabelTransactionLink 是新交易出生即携带的关系出处。照 PriorRequestLink 先例：它只
// 存在于**新**交易上，原交易的状态、结果与后续动作一概不动。
type PriorLabelTransactionLink struct {
	prior LabelTransactionID
	kind  LabelTransactionLinkKind
}

// EstablishPriorLabelTransactionLink 用原交易本体（不是一个裸 ID）建立关系：原交易必须已经
// 定案。拿聚合作参数正是为了让「对结果不确定的交易发起重试」这一步在建立处就死，而不是等
// 读关系的人去核对——CONTEXT 那句「不得直接按失败处理」守不住的话，一次渠道超时就会变成
// 两张都可能有效的面单。
func EstablishPriorLabelTransactionLink(
	prior LabelTransaction,
	kind LabelTransactionLinkKind,
) (PriorLabelTransactionLink, error) {
	if !kind.valid() || !prior.id.valid() {
		return PriorLabelTransactionLink{}, ErrInvalidLabelTransaction
	}
	if !prior.Finalized() {
		return PriorLabelTransactionLink{}, ErrPriorLabelTransactionNotFinalized
	}
	return PriorLabelTransactionLink{prior: prior.id, kind: kind}, nil
}

func (link PriorLabelTransactionLink) PriorTransactionID() LabelTransactionID {
	return link.prior
}

func (link PriorLabelTransactionLink) Kind() LabelTransactionLinkKind {
	return link.kind
}

// established 区分「没有出处」与「出处成立」。零值即缺席：首笔交易没有可指的原交易。
func (link PriorLabelTransactionLink) established() bool {
	return link.kind.valid() && link.prior.valid()
}

// EstablishLabelTransactionSpec 是建立一笔面单交易所需的全部输入。字段清单就是 CONTEXT
// 生命周期第一句列举的固定项，一项不少。
type EstablishLabelTransactionSpec struct {
	Tenant                 TenantID
	ID                     LabelTransactionID
	CoveredParcels         []DeclaredParcelID
	ChannelAccount         ChannelAccountReference
	AccountHolder          ChannelAccountHolderReference
	ServiceProvider        ChannelServiceProviderReference
	SettlementCounterparty SettlementCounterpartyReference
	Contract               ChannelContractReference
	Rate                   ChannelRateReference
	ResponsibilityBasis    ResponsibilityBasisSnapshotReference
	EstablishedAt          time.Time
	// PriorLink 是重试或替代关系，首笔交易留零值。
	PriorLink PriorLabelTransactionLink
}

// LabelTransaction 是独立聚合（ADR-0084 决定一），键为租户加面单交易标识。它不挂在
// 委托聚合下：覆盖包裹可以跨委托，塞进去会让 M:N 关系穿透聚合一致性边界。
//
// 两样东西刻意不在这里：
//
//   - **面单继续尝试决定**（受控关闭/重开）。CONTEXT 把它定义为「针对明确包裹当前完整面单
//     服务范围」的版本化决定——范围是包裹跨其全部相关交易，不是某一笔交易的内部状态；它单列
//     追加式登记册（决定六），本切片不建。
//   - **包裹终局**。CONTEXT 明说「交易级失败不能推导包裹失败，单笔交易中的包裹级
//     失败、渠道作废或失效也不自动形成包裹终局」；终局要跨该包裹全部相关交易与实际承运商
//     收寄事实判断，一笔交易看不到那个范围。
//
// 写入方此刻缺席是设计而不是欠账：渠道墙未降前登记零行（首发基线「独立面单渠道服务不进入
// 首发生产」）。不变式的守护者仍然先立起来——墙一降，写入方对着的就不是一张裸表。
type LabelTransaction struct {
	// revision 是这份聚合被读出时所在的持久化版本，未持久化为零。仓储写入成功后推进它，
	// 状态转移一概不动——转移以值接收者复制整份聚合，版本天然带下去；改成指针接收者
	// 就地改或重新构造 LabelTransaction{...} 会把它悄悄刷回零，随后按预期版本的写入要么
	// 误报并发冲突，要么覆盖别人的写入。
	revision               int64
	tenant                 TenantID
	id                     LabelTransactionID
	coveredParcels         []DeclaredParcelID
	channelAccount         ChannelAccountReference
	accountHolder          ChannelAccountHolderReference
	serviceProvider        ChannelServiceProviderReference
	settlementCounterparty SettlementCounterpartyReference
	contract               ChannelContractReference
	rate                   ChannelRateReference
	responsibilityBasis    ResponsibilityBasisSnapshotReference
	establishedAt          time.Time
	priorLink              PriorLabelTransactionLink
	submittedAt            time.Time
	state                  LabelTransactionState
	parcelResults          []LabelTransactionParcelResult
	resultObservedAt       time.Time
	followUpActions        []LabelTransactionFollowUpAction
	// labelDocuments 是渠道交回的载荷记录，只增不改（ADR-0092 决定三）。存的是引用不是
	// 本体：本体存放属技术组件，而那个组件今天不存在，定位符因此可缺。
	labelDocuments []LabelDocumentRecord
}

// EstablishLabelTransaction 建立交易并就地固定覆盖与依据。此后没有任何方法能改这些
// 字段：CONTEXT 把它们列为建立时固定项，换渠道账号或换合同的正确表达是另起一笔带
// 替代关系的新交易，不是就地改写这一笔的出处。
func EstablishLabelTransaction(spec EstablishLabelTransactionSpec) (LabelTransaction, error) {
	if !spec.Tenant.valid() ||
		!spec.ID.valid() ||
		!spec.ChannelAccount.valid() ||
		!spec.AccountHolder.valid() ||
		!spec.ServiceProvider.valid() ||
		!spec.SettlementCounterparty.valid() ||
		!spec.Contract.valid() ||
		!spec.Rate.valid() ||
		!spec.ResponsibilityBasis.valid() ||
		spec.EstablishedAt.IsZero() {
		return LabelTransaction{}, ErrInvalidLabelTransaction
	}
	// 自指的关系立不起来：一笔既是原交易又是重试的交易，会让「跨该包裹全部相关交易形成
	// 包裹级判断」的遍历原地打转。
	if spec.PriorLink.established() && spec.PriorLink.prior == spec.ID {
		return LabelTransaction{}, ErrInvalidLabelTransaction
	}
	covered, err := fixCoveredParcels(spec.CoveredParcels)
	if err != nil {
		return LabelTransaction{}, err
	}
	return LabelTransaction{
		tenant:                 spec.Tenant,
		id:                     spec.ID,
		coveredParcels:         covered,
		channelAccount:         spec.ChannelAccount,
		accountHolder:          spec.AccountHolder,
		serviceProvider:        spec.ServiceProvider,
		settlementCounterparty: spec.SettlementCounterparty,
		contract:               spec.Contract,
		rate:                   spec.Rate,
		responsibilityBasis:    spec.ResponsibilityBasis,
		establishedAt:          spec.EstablishedAt.UTC(),
		priorLink:              spec.PriorLink,
		state:                  LabelTransactionEstablished,
	}, nil
}

// LabelTransactionParcelResultSpec 是一件覆盖包裹的渠道结果输入。
type LabelTransactionParcelResultSpec struct {
	Parcel     DeclaredParcelID
	Accepted   bool
	Identifier ChannelParcelIdentifier
	Reason     ChannelResultReasonReference
}

// LabelTransactionParcelResult 是面单交易包裹结果：一笔交易对其中某件明确包裹形成的业务
// 结果。它独立成一条，不是交易级结果的投影——CONTEXT 明说它「不能由整笔交易结果或其他包裹
// 结果推断」。
type LabelTransactionParcelResult struct {
	parcel     DeclaredParcelID
	accepted   bool
	identifier ChannelParcelIdentifier
	reason     ChannelResultReasonReference
}

// valid 只检这一条自身说不说得通，不看交易级结果，也不看别的包裹：受理必然带回渠道签发的
// 包裹级标识，未受理必然有原因；未受理却带着标识的一条，说不清那个号是谁的。
func (result LabelTransactionParcelResult) valid() bool {
	if !result.parcel.valid() {
		return false
	}
	if result.accepted {
		return result.identifier.valid()
	}
	return result.reason.valid() && !result.identifier.valid()
}

func (result LabelTransactionParcelResult) Parcel() DeclaredParcelID {
	return result.parcel
}

func (result LabelTransactionParcelResult) Accepted() bool {
	return result.accepted
}

func (result LabelTransactionParcelResult) Identifier() ChannelParcelIdentifier {
	return result.identifier
}

func (result LabelTransactionParcelResult) Reason() ChannelResultReasonReference {
	return result.reason
}

// RecordChannelResultSpec 是一次渠道结果记录的全部输入。两层同在一个 spec 里是刻意的：
// CONTEXT 要求「同时保存整笔交易结果和各包裹结果」，分成两个方法就给「只记了一层」留了门。
type RecordChannelResultSpec struct {
	Outcome       LabelTransactionState
	ParcelResults []LabelTransactionParcelResultSpec
	ObservedAt    time.Time
}

// IsChannelResult 划出交易级结果的三格。它复用状态枚举而不另立一套结果类型：结果就是
// 交易停下的那一格，两套表示法会在读面上产生「状态说失败、结果说成功」的分歧。
//
// 导出是为了让读面用同一条规则派生定案：查阅读的是登记过什么、不重建聚合（ADR-0060 的读面
// 纹样），手上只有状态列而没有 LabelTransaction，在适配器里另写一个「是不是那三格」就是把
// 定案规则抄成第二份。
func (state LabelTransactionState) IsChannelResult() bool {
	switch state {
	case LabelTransactionSucceeded, LabelTransactionPartiallySucceeded, LabelTransactionFailed:
		return true
	default:
		return false
	}
}

// RecordChannelResult 一次写下交易级与包裹级两层结果。
//
// 校验到结构为止：覆盖完备（每件覆盖包裹恰一条）、无越界、无重复、每条自身说得通。刻意
// **没有**任何跨层一致性校验——「交易级失败 ⇒ 包裹全部未受理」这类规则读起来合理，但它正是
// CONTEXT 禁止的「由一个层次覆盖另一个层次」：渠道整单判失败而个别包裹已经下号是真实存在的
// 结果形状，把它拒掉等于逼调用方把真话改成假话再存进来。
//
// 只记一次：已经形成的渠道责任结果不在这里改写，后来的渠道作废、退款、替代走追加式后续动作，
// 重新下单走带替代关系的新交易。
func (transaction LabelTransaction) RecordChannelResult(spec RecordChannelResultSpec) (LabelTransaction, error) {
	if transaction.state != LabelTransactionSubmitted && transaction.state != LabelTransactionResultUncertain {
		return LabelTransaction{}, ErrLabelTransactionStateNotAdmitted
	}
	if !spec.Outcome.IsChannelResult() ||
		spec.ObservedAt.IsZero() ||
		spec.ObservedAt.Before(transaction.submittedAt) {
		return LabelTransaction{}, ErrInvalidLabelTransaction
	}
	results, err := transaction.matchResultsToCoverage(spec.ParcelResults)
	if err != nil {
		return LabelTransaction{}, err
	}
	transaction.parcelResults = results
	transaction.resultObservedAt = spec.ObservedAt.UTC()
	transaction.state = spec.Outcome
	return transaction, nil
}

// matchResultsToCoverage 把逐包裹结果对齐到固定下来的覆盖范围：越界的一条会让一件本交易
// 从未覆盖的包裹凭空得到渠道结果，漏掉的一条会让「已定案」名不副实——定案派生正是靠这里的
// 完备性，才不需要另存一列。交回的顺序随覆盖范围，读面不必再排。
func (transaction LabelTransaction) matchResultsToCoverage(
	specs []LabelTransactionParcelResultSpec,
) ([]LabelTransactionParcelResult, error) {
	if len(specs) != len(transaction.coveredParcels) {
		return nil, ErrInvalidLabelTransaction
	}
	byParcel := make(map[DeclaredParcelID]LabelTransactionParcelResult, len(specs))
	for _, spec := range specs {
		result := LabelTransactionParcelResult{
			parcel:     spec.Parcel,
			accepted:   spec.Accepted,
			identifier: spec.Identifier,
			reason:     spec.Reason,
		}
		if !result.valid() {
			return nil, ErrInvalidLabelTransaction
		}
		if _, repeated := byParcel[result.parcel]; repeated {
			return nil, ErrInvalidLabelTransaction
		}
		byParcel[result.parcel] = result
	}
	results := make([]LabelTransactionParcelResult, 0, len(transaction.coveredParcels))
	for _, parcel := range transaction.coveredParcels {
		result, covered := byParcel[parcel]
		if !covered {
			return nil, ErrInvalidLabelTransaction
		}
		results = append(results, result)
	}
	return results, nil
}

// FollowUpActionKind 是后续业务动作的封闭集合。CONTEXT 明说「查询、重打、渠道作废、替换和
// 渠道退款是不同业务动作」，因此这里三格各自成格：查询与重打不在其列——它们作用于原交易及其
// 既有结果，不形成新的动作记录。
type FollowUpActionKind uint8

const (
	FollowUpActionKindInvalid FollowUpActionKind = iota
	ChannelVoidAction
	ChannelRefundAction
	ChannelReplacementAction
)

func (kind FollowUpActionKind) valid() bool {
	switch kind {
	case ChannelVoidAction, ChannelRefundAction, ChannelReplacementAction:
		return true
	default:
		return false
	}
}

func (kind FollowUpActionKind) String() string {
	switch kind {
	case ChannelVoidAction:
		return "CHANNEL_VOID"
	case ChannelRefundAction:
		return "CHANNEL_REFUND"
	case ChannelReplacementAction:
		return "REPLACEMENT"
	default:
		return ""
	}
}

// FollowUpActionSpec 是追加一条后续动作所需的输入。Parcels 为空表示整笔范围——这是两种真实
// 存在的业务范围，不是「忘了填」：整笔退款与逐件作废在渠道那边就是两回事。
type FollowUpActionSpec struct {
	Kind       FollowUpActionKind
	Parcels    []DeclaredParcelID
	Reason     ChannelResultReasonReference
	OccurredAt time.Time
}

// LabelTransactionFollowUpAction 是一条追加式后续动作记录。它没有任何改写原结果的方法：
// CONTEXT 要求它「可以与未受影响包裹的原结果并存」，读面把两者并列呈现。
type LabelTransactionFollowUpAction struct {
	kind       FollowUpActionKind
	parcels    []DeclaredParcelID
	reason     ChannelResultReasonReference
	occurredAt time.Time
}

func (action LabelTransactionFollowUpAction) Kind() FollowUpActionKind {
	return action.kind
}

// CoversWholeTransaction 区分整笔范围与指名包裹范围。
func (action LabelTransactionFollowUpAction) CoversWholeTransaction() bool {
	return len(action.parcels) == 0
}

// Parcels 交回指名包裹范围的副本，整笔范围为空。
func (action LabelTransactionFollowUpAction) Parcels() []DeclaredParcelID {
	parcels := make([]DeclaredParcelID, len(action.parcels))
	copy(parcels, action.parcels)
	return parcels
}

func (action LabelTransactionFollowUpAction) Reason() ChannelResultReasonReference {
	return action.reason
}

func (action LabelTransactionFollowUpAction) OccurredAt() time.Time {
	return action.occurredAt
}

// AppendFollowUpAction 追加一条渠道作废、渠道退款或替代记录。
//
// 只对已定案的交易开放：这三者都是对**已经形成的结果**采取的动作（CONTEXT 把包裹结果定义为
// 「包括该包裹是否被受理、取得的包裹级标识及适用渠道作废、替代或渠道退款结果」），结果还没
// 回来时没有可作用的对象。业务时间不得早于结果时间，同理。
//
// 它不改交易级状态、不改任何包裹结果、也不动定案谓词——ChannelReplacementAction 在这里只是
// 原交易上的一道痕迹；「原交易 → 被替代」这层关系本身是**新**交易的出生属性（决定二），原
// 交易的结果一概不动。
func (transaction LabelTransaction) AppendFollowUpAction(spec FollowUpActionSpec) (LabelTransaction, error) {
	if !transaction.Finalized() {
		return LabelTransaction{}, ErrLabelTransactionStateNotAdmitted
	}
	if !spec.Kind.valid() ||
		!spec.Reason.valid() ||
		spec.OccurredAt.IsZero() ||
		spec.OccurredAt.Before(transaction.resultObservedAt) {
		return LabelTransaction{}, ErrInvalidLabelTransaction
	}
	scope, err := transaction.scopeWithinCoverage(spec.Parcels)
	if err != nil {
		return LabelTransaction{}, err
	}
	action := LabelTransactionFollowUpAction{
		kind:       spec.Kind,
		parcels:    scope,
		reason:     spec.Reason,
		occurredAt: spec.OccurredAt.UTC(),
	}
	// 显式复制再追加：append 到内部切片上，两份聚合值会共享同一底层数组，一次追加就会
	// 在另一份值上凭空出现。转移交回新值这件事必须一路守到清单里。
	actions := make([]LabelTransactionFollowUpAction, 0, len(transaction.followUpActions)+1)
	actions = append(actions, transaction.followUpActions...)
	transaction.followUpActions = append(actions, action)
	return transaction, nil
}

// AppendLabelDocument 追加一条渠道载荷记录（ADR-0092 决定三）。
//
// 只增不改：同一次结果的载荷不得被后来的调用顶替，重打、换单与替换各自产生新的一条并保留
// 原条。聚合上因此没有任何改写或删除载荷的路径，也没有「当前载荷」这一格——要哪一份由读面
// 按业务规则派生，理由同 Finalized 不存列。
//
// `已建立`与`失败`两格不收：前者还没提交渠道，没有渠道会交回件；后者整笔未受理，一份面单
// 也不会产生。其余各格都收——件可能随提交应答回来（此时仍是`已提交渠道`），也可能等另一次
// 取件才拿到，而那时结果可能已经记下了。
func (transaction LabelTransaction) AppendLabelDocument(spec RecordLabelDocumentSpec) (LabelTransaction, error) {
	if transaction.state == LabelTransactionEstablished || transaction.state == LabelTransactionFailed {
		return LabelTransaction{}, ErrLabelTransactionStateNotAdmitted
	}
	record, err := transaction.labelDocumentRecord(spec)
	if err != nil {
		return LabelTransaction{}, err
	}
	// 显式复制再追加，理由同 AppendFollowUpAction：append 到内部切片上，两份聚合值会共享
	// 同一底层数组。
	documents := make([]LabelDocumentRecord, 0, len(transaction.labelDocuments)+1)
	documents = append(documents, transaction.labelDocuments...)
	transaction.labelDocuments = append(documents, record)
	return transaction, nil
}

// labelDocumentRecord 把一条载荷输入过完结构校验。
//
// 覆盖范围按粒度分两条：逐件那一格**恰一件**，批那一格至少一件。逐件却列出多件说不清这份
// 纸是谁的；两格都放开则粒度这个字段就不再约束任何东西，与不写它无异。
//
// 定位符不在必备之列——本体无存放处是今天唯一走得到的分支（ADR-0092 决定二）；摘要在，
// 因为它是「我们确实收到过这份件」的全部证据。
func (transaction LabelTransaction) labelDocumentRecord(spec RecordLabelDocumentSpec) (LabelDocumentRecord, error) {
	if !spec.Role.valid() ||
		!spec.Format.valid() ||
		!spec.Digest.valid() ||
		!spec.Granularity.valid() ||
		spec.ObservedAt.IsZero() ||
		spec.ObservedAt.Before(transaction.submittedAt) {
		return LabelDocumentRecord{}, ErrInvalidLabelTransaction
	}
	if len(spec.CoveredParcels) == 0 ||
		(spec.Granularity == LabelDocumentPerParcel && len(spec.CoveredParcels) != 1) {
		return LabelDocumentRecord{}, ErrInvalidLabelTransaction
	}
	parcels, err := transaction.scopeWithinCoverage(spec.CoveredParcels)
	if err != nil {
		return LabelDocumentRecord{}, err
	}
	return LabelDocumentRecord{
		role:        spec.Role,
		format:      spec.Format,
		granularity: spec.Granularity,
		parcels:     parcels,
		digest:      spec.Digest,
		locator:     spec.Locator,
		observedAt:  spec.ObservedAt.UTC(),
	}, nil
}

// LabelDocuments 交回副本，顺序即追加顺序。读的人要哪一份自己按业务规则挑——**不要取最后
// 一条**：重打产生的新件与被替换的旧件在时间上相邻而在业务上不同。
func (transaction LabelTransaction) LabelDocuments() []LabelDocumentRecord {
	documents := make([]LabelDocumentRecord, len(transaction.labelDocuments))
	copy(documents, transaction.labelDocuments)
	return documents
}

// scopeWithinCoverage 校验指名包裹范围落在固定下来的覆盖范围内，并交回副本；空范围即整笔。
func (transaction LabelTransaction) scopeWithinCoverage(parcels []DeclaredParcelID) ([]DeclaredParcelID, error) {
	if len(parcels) == 0 {
		return nil, nil
	}
	scope := make([]DeclaredParcelID, 0, len(parcels))
	seen := make(map[DeclaredParcelID]bool, len(parcels))
	for _, parcel := range parcels {
		if !transaction.covers(parcel) || seen[parcel] {
			return nil, ErrInvalidLabelTransaction
		}
		seen[parcel] = true
		scope = append(scope, parcel)
	}
	return scope, nil
}

func (transaction LabelTransaction) covers(parcel DeclaredParcelID) bool {
	for _, covered := range transaction.coveredParcels {
		if covered == parcel {
			return true
		}
	}
	return false
}

// SubmitToChannel 把已建立的交易推到`已提交渠道`。提交时间早于建立时间的一笔请求在业务上
// 不可能存在，而渠道结果日后要按业务发生时间排序归类（CONTEXT「迟到的渠道或运输事实可以按
// 业务发生时间和适用时间改变交易归类」），时间轴自相矛盾就没法排。
func (transaction LabelTransaction) SubmitToChannel(submittedAt time.Time) (LabelTransaction, error) {
	if transaction.state != LabelTransactionEstablished {
		return LabelTransaction{}, ErrLabelTransactionStateNotAdmitted
	}
	if submittedAt.IsZero() || submittedAt.Before(transaction.establishedAt) {
		return LabelTransaction{}, ErrInvalidLabelTransaction
	}
	transaction.submittedAt = submittedAt.UTC()
	transaction.state = LabelTransactionSubmitted
	return transaction, nil
}

// MarkResultUncertain 记录「暂时无法确认渠道是否受理」。它不带业务时间：不确定是本上下文
// 对渠道的一次观察结论，没有对应的渠道业务事实时间可记；真正带时间的是随后回来的结果。
func (transaction LabelTransaction) MarkResultUncertain() (LabelTransaction, error) {
	if transaction.state != LabelTransactionSubmitted {
		return LabelTransaction{}, ErrLabelTransactionStateNotAdmitted
	}
	transaction.state = LabelTransactionResultUncertain
	return transaction, nil
}

// fixCoveredParcels 校验并复制覆盖范围。复制在这里而不是在调用方，是因为「固定」这件事
// 必须由聚合自己保证：收下调用方的切片就等于把覆盖范围留在外面，建立之后仍可被改。
func fixCoveredParcels(parcels []DeclaredParcelID) ([]DeclaredParcelID, error) {
	if len(parcels) == 0 {
		return nil, ErrInvalidLabelTransaction
	}
	covered := make([]DeclaredParcelID, 0, len(parcels))
	seen := make(map[DeclaredParcelID]bool, len(parcels))
	for _, parcel := range parcels {
		if !parcel.valid() || seen[parcel] {
			return nil, ErrInvalidLabelTransaction
		}
		seen[parcel] = true
		covered = append(covered, parcel)
	}
	return covered, nil
}

// Revision 交回这份聚合被读出时所在的持久化版本，未持久化为零。
func (transaction LabelTransaction) Revision() int64 {
	return transaction.revision
}

func (transaction LabelTransaction) Tenant() TenantID {
	return transaction.tenant
}

func (transaction LabelTransaction) ID() LabelTransactionID {
	return transaction.id
}

// CoveredParcels 交回覆盖范围的副本。固定的东西要真的动不了，交回内部切片等于把
// 建立时固定的覆盖范围交给调用方随手改。
func (transaction LabelTransaction) CoveredParcels() []DeclaredParcelID {
	covered := make([]DeclaredParcelID, len(transaction.coveredParcels))
	copy(covered, transaction.coveredParcels)
	return covered
}

func (transaction LabelTransaction) ChannelAccount() ChannelAccountReference {
	return transaction.channelAccount
}

func (transaction LabelTransaction) AccountHolder() ChannelAccountHolderReference {
	return transaction.accountHolder
}

func (transaction LabelTransaction) ServiceProvider() ChannelServiceProviderReference {
	return transaction.serviceProvider
}

func (transaction LabelTransaction) SettlementCounterparty() SettlementCounterpartyReference {
	return transaction.settlementCounterparty
}

func (transaction LabelTransaction) Contract() ChannelContractReference {
	return transaction.contract
}

func (transaction LabelTransaction) Rate() ChannelRateReference {
	return transaction.rate
}

func (transaction LabelTransaction) ResponsibilityBasis() ResponsibilityBasisSnapshotReference {
	return transaction.responsibilityBasis
}

// EstablishedAt 是建立交易的业务时间。
func (transaction LabelTransaction) EstablishedAt() time.Time {
	return transaction.establishedAt
}

// PriorLink 交回这笔交易的关系出处。首笔交易报告缺席——那是真话，它没有出处。
func (transaction LabelTransaction) PriorLink() (PriorLabelTransactionLink, bool) {
	return transaction.priorLink, transaction.priorLink.established()
}

// SubmittedAt 是向渠道发出请求的业务时间，尚未提交为零值。
func (transaction LabelTransaction) SubmittedAt() time.Time {
	return transaction.submittedAt
}

// ResultObservedAt 是渠道结果的业务时间，结果未回为零值。
func (transaction LabelTransaction) ResultObservedAt() time.Time {
	return transaction.resultObservedAt
}

// ParcelResults 交回逐包裹结果的副本，顺序随覆盖范围。
func (transaction LabelTransaction) ParcelResults() []LabelTransactionParcelResult {
	results := make([]LabelTransactionParcelResult, len(transaction.parcelResults))
	copy(results, transaction.parcelResults)
	return results
}

// ParcelResult 按包裹取回它自己那一条。结果未回或包裹不在覆盖范围时报告缺席，不造一条
// 「默认未受理」——那正是拿交易级去推包裹级。
func (transaction LabelTransaction) ParcelResult(parcel DeclaredParcelID) (LabelTransactionParcelResult, bool) {
	for _, result := range transaction.parcelResults {
		if result.parcel == parcel {
			return result, true
		}
	}
	return LabelTransactionParcelResult{}, false
}

// FollowUpActions 交回后续动作清单的副本，按追加顺序。
func (transaction LabelTransaction) FollowUpActions() []LabelTransactionFollowUpAction {
	actions := make([]LabelTransactionFollowUpAction, len(transaction.followUpActions))
	copy(actions, transaction.followUpActions)
	return actions
}

func (transaction LabelTransaction) State() LabelTransactionState {
	return transaction.state
}

// Finalized 派生面单交易定案：交易级结果已落在三个结果格之一。逐包裹结果的完备性由
// RecordChannelResult 在记录时结构保证，所以这一个谓词就是 CONTEXT 定案定义的全部。
//
// 它没有对应的存储字段（ADR-0084 决定四）：存一列「定案状态」就有了第二个来源，某次改结果
// 忘了改列，两处便各说各话。渠道退款、对账与运营结算按 CONTEXT 明文不属定案条件，因此追加
// 后续动作不动这个谓词。
func (transaction LabelTransaction) Finalized() bool {
	return transaction.state.IsChannelResult()
}
