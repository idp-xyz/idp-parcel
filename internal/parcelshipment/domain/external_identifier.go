package domain

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

// ErrInvalidCustomerParcelNumber 说明委托上的（声明包裹，客户包裹件号）配对立不起来：指着成员集合外的包裹，
// 或一件包裹给了两个号。它与 ErrInvalidDeclaredMeasurement 同格——客户输入的问题，编排答`输入未受理`。
var ErrInvalidCustomerParcelNumber = errors.New("parcel shipment: invalid customer parcel number")

// ErrExternalIdentifierNumberNotStorable 说明号码存不进外部标识关联册。它是确定的：同一个号再交多少次都一样，所以写入方
// 答坏输入、不答依赖故障。核对的一侧只在号带 NUL 时遇到它（NewExternalIdentifierLookupNumber），那样的号不可能在册，据此答
// 查无此号。
var ErrExternalIdentifierNumberNotStorable = errors.New("parcel shipment: external identifier number cannot enter the register")

// externalIdentifierNumberMaxBytes 是号码「存得下、索引得了」的机制界（票 ps-external-mark-relations/06 裁定一），不是业务
// 口径：关联册按号码建 btree 索引，索引行有长度上限，号码不设界就会在写入那一刻整笔回滚。界远低于那条上限，远高于现行
// 条码与运单号的长度；租户真实号码多长不在这里配。按 UTF-8 字节计，因为库里的上限按字节算。
const externalIdentifierNumberMaxBytes = 512

// ExternalIdentifierNumber 是外部参与方分配的号码本身。逐字保存、逐字比对，不做大小写、空白或校验位折算：
// 条码规则属租户取值（UC-NO-002 明禁写默认条码规则；ADR-0184 决定五）。
type ExternalIdentifierNumber struct{ requiredValue }

// NewExternalIdentifierNumber 是号码进册前唯一的一道门：全空白、带 NUL（Postgres 的 text 存不下）、超过字节界的立不起来。
// 其余字符一个不拒，回车换行、Tab、GS 分隔符都是现场或客户给出的号码的一部分，拒掉或去掉就是在折算号码。
func NewExternalIdentifierNumber(value string) (ExternalIdentifierNumber, error) {
	required, err := newRequiredValue("external identifier number", value)
	if err != nil {
		return ExternalIdentifierNumber{}, err
	}
	if err := refuseNUL(value); err != nil {
		return ExternalIdentifierNumber{}, err
	}
	if len(value) > externalIdentifierNumberMaxBytes {
		return ExternalIdentifierNumber{}, fmt.Errorf("%w: it is %d bytes, over the %d-byte limit",
			ErrExternalIdentifierNumberNotStorable, len(value), externalIdentifierNumberMaxBytes)
	}
	return ExternalIdentifierNumber{required}, nil
}

// RehydrateExternalIdentifierNumber 还原库里已经存下的号码，只拒全空白。「存得下」那道门守的是进册，不守读回：门立起来之前，
// 提交口只拒全空白，超过字节界的号已经能进委托快照；还原时再过一遍那道门，这样的委托就整份读不回来——与面单交易的门不放进
// matchResultsToCoverage 同一个理由，重建要读得回。只给仓储还原用，新号一律走 NewExternalIdentifierNumber。
func RehydrateExternalIdentifierNumber(value string) (ExternalIdentifierNumber, error) {
	required, err := newRequiredValue("external identifier number", value)
	return ExternalIdentifierNumber{required}, err
}

// ExternalIdentifierLookupNumber 是拿去按号核对的号码（ports.ParcelExternalIdentifierView）。它与 ExternalIdentifierNumber 分成
// 两个类型：入册门只管写，读的一侧不拿它挡已在册的号——门立起来之前进册的号可能超过字节界，现场扫到同一个号时照样要比得上
// （票 ps-external-mark-relations/06 裁定补）。分成两个类型，核对用的号就交不进任何写入方，也就绕不过入册门。
type ExternalIdentifierLookupNumber struct{ requiredValue }

// NewExternalIdentifierLookupNumber 只拒全空白与带 NUL 的号，长度不设界：读口按号比较、不构造索引行，长号不报错，查不到照常
// 答空。带 NUL 的号 Postgres 的 text 存不下，确定不在册，拿去查反倒在库面报错，所以在这里拒。号码逐字，不折算（ADR-0184
// 决定五）。
func NewExternalIdentifierLookupNumber(value string) (ExternalIdentifierLookupNumber, error) {
	required, err := newRequiredValue("external identifier lookup number", value)
	if err != nil {
		return ExternalIdentifierLookupNumber{}, err
	}
	if err := refuseNUL(value); err != nil {
		return ExternalIdentifierLookupNumber{}, err
	}
	return ExternalIdentifierLookupNumber{required}, nil
}

// refuseNUL 是入册门与核对用号共用的那一格：Postgres 的 text 存不下 NUL，带它的号写不进册，也不可能已在册。
func refuseNUL(value string) error {
	if strings.ContainsRune(value, 0) {
		return fmt.Errorf("%w: it carries a NUL character", ErrExternalIdentifierNumberNotStorable)
	}
	return nil
}

// ExternalIdentifierKind 是标识类型的封闭集合。类型决定被标识对象的种类与分配方的种类（ADR-0184 决定二）：
// 「号码挂错对象」在形状上立不起来，CONTEXT「外部标识必须关联其真实标识对象」因此不靠调用方自觉。
//
// 今天只有有来源的类型进来。客户参考号（委托）与渠道订单号或主号（面单交易）等各自的来源与消费方。
type ExternalIdentifierKind uint8

const (
	ExternalIdentifierKindInvalid ExternalIdentifierKind = iota
	// CustomerParcelNumber 是客户在委托里为声明包裹给出的包裹件号：分配方是该委托的客户账户，标识包裹。
	CustomerParcelNumber
	// ChannelParcelNumber 是渠道在面单交易包裹结果里签发的包裹级标识（件号、尾程号一类）：分配方是该交易的渠道
	// 服务方，标识包裹。
	ChannelParcelNumber
)

func (kind ExternalIdentifierKind) String() string {
	switch kind {
	case CustomerParcelNumber:
		return "CUSTOMER_PARCEL_NUMBER"
	case ChannelParcelNumber:
		return "CHANNEL_PARCEL_NUMBER"
	default:
		return ""
	}
}

// CountsInCustomerAccountSpace 报告这一类号码的适用范围是不是某个客户账户的编号空间（PS CONTEXT「范围相容」）。是的，号码只在
// 带了那个客户账户的核对里范围相容；不是的（渠道包裹件号在渠道服务方内算数），与一次送站属于哪个客户账户无关，恒范围相容
// （ADR-0212 决定四）。按类型分而不按分配方分：适用范围单列一格（ADR-0184 决定六），日后窄于客户账户的范围仍落在该账户的空间里。
func (kind ExternalIdentifierKind) CountsInCustomerAccountSpace() bool {
	switch kind {
	case CustomerParcelNumber:
		return true
	default:
		return false
	}
}

// CustomerAccountSpaceKinds 交回在客户账户编号空间里算数的各类型。身份核对读口按它比范围相容，分类只在这里一处。
func CustomerAccountSpaceKinds() []ExternalIdentifierKind {
	var kinds []ExternalIdentifierKind
	for _, kind := range []ExternalIdentifierKind{CustomerParcelNumber, ChannelParcelNumber} {
		if kind.CountsInCustomerAccountSpace() {
			kinds = append(kinds, kind)
		}
	}
	return kinds
}

// ExternalIdentifierAllocator 是号码的分配方。它的种类由标识类型定：客户包裹件号的分配方是客户账户。
type ExternalIdentifierAllocator struct{ requiredValue }

// ExternalIdentifierScope 是号码在其中算数的分配方编号空间。首切与分配方同值，仍单列一格：委托内箱号、渠道
// 产品内号段这类更窄的范围日后是加列，不是改键（ADR-0184 决定六）。
type ExternalIdentifierScope struct{ requiredValue }

// ExternalIdentifierBasis 是关联的形成依据——客户包裹件号的那一条是接受决定。
type ExternalIdentifierBasis struct{ requiredValue }

// CustomerParcelNumberDeclaration 是委托上的一组（声明包裹，客户包裹件号）。它挂在委托上、首次提交时固定：
// 号码标识的是成员，新提交版本的成员集合又必须与当前版本相等，所以它不随版本重报（ADR-0184 决定四）。
type CustomerParcelNumberDeclaration struct {
	parcel DeclaredParcelID
	number ExternalIdentifierNumber
}

func DeclareCustomerParcelNumber(parcel DeclaredParcelID, number ExternalIdentifierNumber) (CustomerParcelNumberDeclaration, error) {
	if !parcel.valid() || !number.valid() {
		return CustomerParcelNumberDeclaration{}, ErrInvalidCustomerParcelNumber
	}
	return CustomerParcelNumberDeclaration{parcel: parcel, number: number}, nil
}

func (declaration CustomerParcelNumberDeclaration) Parcel() DeclaredParcelID {
	return declaration.parcel
}

func (declaration CustomerParcelNumberDeclaration) Number() ExternalIdentifierNumber {
	return declaration.number
}

// customerParcelNumbersFor 校验配对贴合成员集合并交回拷贝：每件至多一个号，指着成员集合外即拒。两件同号
// 不拒——节点会答身份冲突；在提交时就拒是一条没有 CONTEXT 依据的新门（ADR-0184 越权风险点 4）。
func customerParcelNumbersFor(
	declarations []CustomerParcelNumberDeclaration,
	members []DeclaredParcelID,
) ([]CustomerParcelNumberDeclaration, error) {
	if len(declarations) == 0 {
		return nil, nil
	}
	declared := make(map[DeclaredParcelID]struct{}, len(members))
	for _, member := range members {
		declared[member] = struct{}{}
	}
	seen := make(map[DeclaredParcelID]struct{}, len(declarations))
	copied := make([]CustomerParcelNumberDeclaration, 0, len(declarations))
	for _, declaration := range declarations {
		if !declaration.parcel.valid() || !declaration.number.valid() {
			return nil, ErrInvalidCustomerParcelNumber
		}
		if _, member := declared[declaration.parcel]; !member {
			return nil, ErrInvalidCustomerParcelNumber
		}
		if _, twice := seen[declaration.parcel]; twice {
			return nil, ErrInvalidCustomerParcelNumber
		}
		seen[declaration.parcel] = struct{}{}
		copied = append(copied, declaration)
	}
	return copied, nil
}

// ExternalIdentifierRelation 是一条外部标识关联（PS CONTEXT「外部标识关联」）。它是产物：只由形成它的
// 来源事实交出，没有公开构造器；登记册只插不改，适用状态的变化另起一行挂在它上面。
//
// 被标识对象今天只有包裹一种——有来源的两类号码都标识包裹。委托与面单交易两种对象随各自的类型进来。
type ExternalIdentifierRelation struct {
	tenant    TenantID
	kind      ExternalIdentifierKind
	allocator ExternalIdentifierAllocator
	scope     ExternalIdentifierScope
	number    ExternalIdentifierNumber
	parcel    DeclaredParcelID
	basis     ExternalIdentifierBasis
	formedAt  time.Time
}

func (relation ExternalIdentifierRelation) Tenant() TenantID {
	return relation.tenant
}

func (relation ExternalIdentifierRelation) Kind() ExternalIdentifierKind {
	return relation.kind
}

func (relation ExternalIdentifierRelation) Allocator() ExternalIdentifierAllocator {
	return relation.allocator
}

func (relation ExternalIdentifierRelation) Scope() ExternalIdentifierScope {
	return relation.scope
}

func (relation ExternalIdentifierRelation) Number() ExternalIdentifierNumber {
	return relation.number
}

func (relation ExternalIdentifierRelation) Parcel() DeclaredParcelID {
	return relation.parcel
}

func (relation ExternalIdentifierRelation) Basis() ExternalIdentifierBasis {
	return relation.basis
}

func (relation ExternalIdentifierRelation) FormedAt() time.Time {
	return relation.formedAt
}

// CustomerParcelNumbers 交回委托上的客户包裹件号配对（拷贝）。
func (request ShipmentRequest) CustomerParcelNumbers() []CustomerParcelNumberDeclaration {
	return append([]CustomerParcelNumberDeclaration(nil), request.customerParcelNumbers...)
}

// ExternalIdentifierRelationsFormedByAcceptance 交回这份委托随接受形成的外部标识关联：已接受时，接受基线里
// 每件给了号的包裹一条，分配方与适用范围都是来源身份上的客户账户，依据是接受决定，形成时刻是基线固定时刻；
// 未接受（已提交、已拒绝、已撤回）交空——接受前的声明包裹不是正式包裹（PS CONTEXT Rules）。
//
// identity 是这份委托的来源身份，客户账户取自它：委托聚合本身不带租户与客户账户，那两维在来源身份上。
func (request ShipmentRequest) ExternalIdentifierRelationsFormedByAcceptance(identity SourceIdentity) []ExternalIdentifierRelation {
	if request.state != ShipmentRequestAccepted || !identity.valid() {
		return nil
	}
	baseline, accepted := request.AcceptanceBaseline()
	decision, decided := request.AcceptanceDecision()
	if !accepted || !decided {
		return nil
	}
	customer := identity.CustomerAccountID().String()
	relations := make([]ExternalIdentifierRelation, 0, len(request.customerParcelNumbers))
	for _, declaration := range request.customerParcelNumbers {
		if !baseline.covers(declaration.parcel) {
			continue
		}
		relations = append(relations, ExternalIdentifierRelation{
			tenant:    identity.TenantID(),
			kind:      CustomerParcelNumber,
			allocator: ExternalIdentifierAllocator{requiredValue{value: customer}},
			scope:     ExternalIdentifierScope{requiredValue{value: customer}},
			number:    declaration.number,
			parcel:    declaration.parcel,
			basis:     ExternalIdentifierBasis{requiredValue{value: decision.DecisionID().String()}},
			formedAt:  baseline.FixedAt(),
		})
	}
	return relations
}

// ExternalIdentifierRelationsFormedByChannelResults 交回这笔面单交易的包裹结果形成的外部标识关联：受理的每件
// 包裹一条渠道包裹件号，分配方与适用范围都是交易的渠道服务方，依据是这笔交易，形成时刻是结果观察时刻；未受理的
// 不形成，结果回来之前交空（ADR-0184 决定三第二项）。
func (transaction LabelTransaction) ExternalIdentifierRelationsFormedByChannelResults() []ExternalIdentifierRelation {
	var relations []ExternalIdentifierRelation
	for _, result := range transaction.parcelResults {
		if !result.accepted {
			continue
		}
		relations = append(relations, transaction.channelRelationOf(result))
	}
	return relations
}

func (transaction LabelTransaction) channelRelationOf(result LabelTransactionParcelResult) ExternalIdentifierRelation {
	provider := transaction.serviceProvider.String()
	return ExternalIdentifierRelation{
		tenant:    transaction.tenant,
		kind:      ChannelParcelNumber,
		allocator: ExternalIdentifierAllocator{requiredValue{value: provider}},
		scope:     ExternalIdentifierScope{requiredValue{value: provider}},
		number:    ExternalIdentifierNumber{requiredValue{value: result.identifier.String()}},
		parcel:    result.parcel,
		basis:     ExternalIdentifierBasis{requiredValue{value: transaction.id.String()}},
		formedAt:  transaction.resultObservedAt,
	}
}

// ExternalIdentifierChangeKind 是外部标识关联适用状态的终态。已失效不在其中：有效期规则今天没有登记处，没有
// 执行器会形成它（ADR-0184 决定七）。
type ExternalIdentifierChangeKind uint8

const (
	ExternalIdentifierChangeKindInvalid ExternalIdentifierChangeKind = iota
	ExternalIdentifierVoided
	ExternalIdentifierReplaced
)

func (kind ExternalIdentifierChangeKind) String() string {
	switch kind {
	case ExternalIdentifierVoided:
		return "VOIDED"
	case ExternalIdentifierReplaced:
		return "REPLACED"
	default:
		return ""
	}
}

// ExternalIdentifierChangeCause 是带来一次适用状态变化的来源事实：作废是被作废的交易本身，替代是新交易。它与形成依据
// 分成两个类型——一个说关联从哪来，一个说它为何终结，混用一个类型就读不出这一格说的是哪件事。
type ExternalIdentifierChangeCause struct{ requiredValue }

// ExternalIdentifierChange 是一次适用状态变化：作用于（租户，类型，形成依据，包裹）选出的那条关联——渠道包裹件号
// 由形成它的交易与包裹唯一确定，替代那一笔手里没有原交易的号，所以按选择而不是按号码指名。终态一次：已有终态的
// 关联不再变（PS CONTEXT「已作废、已被替代、已失效都是终态，不回到适用」）。
type ExternalIdentifierChange struct {
	tenant         TenantID
	identifierKind ExternalIdentifierKind
	formedBy       ExternalIdentifierBasis
	parcel         DeclaredParcelID
	terminal       ExternalIdentifierChangeKind
	successor      ExternalIdentifierRelation
	cause          ExternalIdentifierChangeCause
	changedAt      time.Time
}

func (change ExternalIdentifierChange) Tenant() TenantID {
	return change.tenant
}

// IdentifierKind 是被改那条关联的标识类型；它变成的终态见 ChangeKind。
func (change ExternalIdentifierChange) IdentifierKind() ExternalIdentifierKind {
	return change.identifierKind
}

// FormedBy 是被改那条关联的形成依据。
func (change ExternalIdentifierChange) FormedBy() ExternalIdentifierBasis {
	return change.formedBy
}

func (change ExternalIdentifierChange) Parcel() DeclaredParcelID {
	return change.parcel
}

// ChangeKind 是这次变化把关联带到的终态。
func (change ExternalIdentifierChange) ChangeKind() ExternalIdentifierChangeKind {
	return change.terminal
}

// Successor 交回已被替代那一格的后继关联；作废没有后继。
func (change ExternalIdentifierChange) Successor() (ExternalIdentifierRelation, bool) {
	return change.successor, change.terminal == ExternalIdentifierReplaced
}

func (change ExternalIdentifierChange) Cause() ExternalIdentifierChangeCause {
	return change.cause
}

func (change ExternalIdentifierChange) ChangedAt() time.Time {
	return change.changedAt
}

// ExternalIdentifierChanges 交回这笔交易带来的适用状态变化（ADR-0184 决定七）。每件受理包裹在作废与替代上各至多
// 一条，两条作用于不同的关联（作废作用于本交易形成的那条，替代作用于原交易形成的那条）：
//   - 有渠道作废作用到它（整笔或点名，与终局判断同一读法，见 voidSpan），本交易给它形成的那条关联转已作废，时刻取
//     最早那条作废——号从第一次作废起就不再算数。库里落下的是先写入的那一行（终态一次、只插不改），之后补进一条
//     更早的作废不回头改写它，ADR-0185 后果节记为已知局限；
//   - 前序关系为替代时，原交易给它形成的那条关联转已被替代，后继是本交易的新关联，时刻是结果观察时刻。原交易必然
//     已定案（EstablishPriorLabelTransactionLink 只认已定案的原交易），它的关联此时已在册里。沿用原号也是新的一条
//     关联（形成依据不同，ADR-0185），后继因此不会指回被替代者自己。
//
// 渠道退款不改适用状态——退款不等于号作废；原交易上的替代痕迹不在这里起作用，替代由新交易出生时的前序关系表达。
func (transaction LabelTransaction) ExternalIdentifierChanges() []ExternalIdentifierChange {
	var changes []ExternalIdentifierChange
	cause := ExternalIdentifierChangeCause{requiredValue{value: transaction.id.String()}}
	for _, result := range transaction.parcelResults {
		if !result.accepted {
			continue
		}
		if voidedAt, _, voided := transaction.voidSpan(result.parcel); voided {
			changes = append(changes, ExternalIdentifierChange{
				tenant:         transaction.tenant,
				identifierKind: ChannelParcelNumber,
				formedBy:       ExternalIdentifierBasis{requiredValue{value: transaction.id.String()}},
				parcel:         result.parcel,
				terminal:       ExternalIdentifierVoided,
				cause:          cause,
				changedAt:      voidedAt,
			})
		}
	}
	if link, linked := transaction.PriorLink(); linked && link.Kind() == LabelTransactionReplacement {
		for _, result := range transaction.parcelResults {
			if !result.accepted {
				continue
			}
			changes = append(changes, ExternalIdentifierChange{
				tenant:         transaction.tenant,
				identifierKind: ChannelParcelNumber,
				formedBy:       ExternalIdentifierBasis{requiredValue{value: link.PriorTransactionID().String()}},
				parcel:         result.parcel,
				terminal:       ExternalIdentifierReplaced,
				successor:      transaction.channelRelationOf(result),
				cause:          cause,
				changedAt:      transaction.resultObservedAt,
			})
		}
	}
	return changes
}
