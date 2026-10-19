package domain

import (
	"errors"
	"slices"
	"strings"
	"time"
)

// 送站预报（NO CONTEXT「送站预报」；ADR-0212 决定二）：客户账户在送站之前经客户渠道交来的声明——送到哪个节点、预计何时送达、
// 送来哪些客户包裹件号。它是节点的到货计划，不形成也不改变委托、包裹或客户承诺；提交即固定，改动走撤回再提交。

// ErrInvalidDeliveryPreAdvice 说明一份送站预报或它的撤回立不起来：缺了必备的格、号码集合为空或重复。重发同样的内容不会变好。
var ErrInvalidDeliveryPreAdvice = errors.New("node operations: invalid delivery pre-advice")

// CustomerAccountReference 指名一个客户账户（party-commercial 的客户账户标识）。本上下文只引用它：预报落到哪个客户账户、交接单
// 声称了哪些，都按这个引用逐字比。
type CustomerAccountReference struct{ requiredValue }

func NewCustomerAccountReference(value string) (CustomerAccountReference, error) {
	required, err := newRequiredValue("customer account reference", value)
	return CustomerAccountReference{required}, err
}

// DeliveryPreAdviceID 是本上下文在预报形成时签发的标识（ADR-0212 决定二「标识由 NO 签发」）：答回客户，送站时随交付出示。不收
// 客户自带的号——自带的号要加上客户账户才唯一，设备只扫到号，会跨客户撞号。
type DeliveryPreAdviceID struct{ requiredValue }

func NewDeliveryPreAdviceID(value string) (DeliveryPreAdviceID, error) {
	required, err := newRequiredValue("delivery pre-advice ID", value)
	return DeliveryPreAdviceID{required}, err
}

// PreAdvisedParcelNumber 是预报里列的一个客户包裹件号，逐字保存、逐字比对，与 parcel-shipment 的身份核对同一口径（NO CONTEXT
// 生命周期「送站预报」）。
//
// 本构造只拒全空白。号码能不能进预报还有一道形状门，与 PS 入册门同一界（ADR-0212 决定二；票 ps-external-mark-relations/06 裁定一）：
// 那一界归 PS 的领域，本上下文不另立一份，经 ports.PreAdvisedNumberGate 去问（防腐适配器在 adapters/parcelshipment）。所以新号
// 一律从那道门取；本构造只给门的实现与仓储还原用——还原不再过门，理由同 PS 的 RehydrateExternalIdentifierNumber：门立起来之后
// 界若收紧，已在册的预报要读得回。
type PreAdvisedParcelNumber struct{ requiredValue }

func NewPreAdvisedParcelNumber(value string) (PreAdvisedParcelNumber, error) {
	required, err := newRequiredValue("pre-advised parcel number", value)
	return PreAdvisedParcelNumber{required}, err
}

// Matches 报告现场观察到的号码是否逐字就是这个号。带空白、大小写不同、全角半角不同都是另一个号：条码规则属租户取值，这里不折算
// （ADR-0184 决定五）。
func (number PreAdvisedParcelNumber) Matches(mark string) bool {
	return number.valid() && number.value == mark
}

// CustomerRequestSource 是一份经客户渠道交来的事实的来源：交来它的客户端、来源请求键与内容摘要。同一来源同一键是重放，内容摘要
// 分得开「原样重投」与「换了内容」（ADR-0139 决定五；UC-NO-006「一致性、幂等与并发」）。
//
// 客户端单列一格、不并进键：请求键只在一个客户端之内唯一，两个客户端碰巧用了同一个键是两次来源请求（首方族把发行方与主体拼成
// 来源格，理由同那一格的注释）。
type CustomerRequestSource struct {
	Channel       string
	RequestKey    string
	ContentDigest string
}

func (source CustomerRequestSource) valid() bool {
	return strings.TrimSpace(source.Channel) != "" &&
		strings.TrimSpace(source.RequestKey) != "" &&
		strings.TrimSpace(source.ContentDigest) != ""
}

// DeliveryPreAdviceSpec 是形成一份送站预报所需的全部输入（ADR-0212 决定二「内容」）。DeliveringParty 是可选的交付方线索，零值即
// 没给；它只是线索，不参与范围判断（决定一）。
type DeliveryPreAdviceSpec struct {
	TenantID          TenantID
	ID                DeliveryPreAdviceID
	CustomerAccount   CustomerAccountReference
	Node              NodeReference
	ExpectedArrivalAt time.Time
	Numbers           []PreAdvisedParcelNumber
	DeliveringParty   DeliveringPartyReference
	Source            CustomerRequestSource
	FormedAt          time.Time
}

// DeliveryPreAdvice 是一份已登记的送站预报。它没有改写路径：撤回是另一份事实（DeliveryPreAdviceWithdrawal）。
type DeliveryPreAdvice struct {
	tenantID          TenantID
	id                DeliveryPreAdviceID
	customerAccount   CustomerAccountReference
	node              NodeReference
	expectedArrivalAt time.Time
	numbers           []PreAdvisedParcelNumber
	deliveringParty   DeliveringPartyReference
	source            CustomerRequestSource
	formedAt          time.Time
}

// FormDeliveryPreAdvice 立一份送站预报。号码集合非空、逐字不重复，照交来的顺序保存：顺序本身不承载意义，保存它只为原样回显。
// 时刻按 UTC、截到微秒——册的精度就是库的精度，截在这里，读回的预报与形成时的那一份逐格相等。
func FormDeliveryPreAdvice(spec DeliveryPreAdviceSpec) (DeliveryPreAdvice, error) {
	if !spec.TenantID.valid() ||
		!spec.ID.valid() ||
		!spec.CustomerAccount.valid() ||
		!spec.Node.valid() ||
		spec.ExpectedArrivalAt.IsZero() ||
		!spec.Source.valid() ||
		spec.FormedAt.IsZero() {
		return DeliveryPreAdvice{}, ErrInvalidDeliveryPreAdvice
	}
	if len(spec.Numbers) == 0 {
		return DeliveryPreAdvice{}, ErrInvalidDeliveryPreAdvice
	}
	seen := make(map[string]bool, len(spec.Numbers))
	for _, number := range spec.Numbers {
		if !number.valid() || seen[number.value] {
			return DeliveryPreAdvice{}, ErrInvalidDeliveryPreAdvice
		}
		seen[number.value] = true
	}
	return DeliveryPreAdvice{
		tenantID:          spec.TenantID,
		id:                spec.ID,
		customerAccount:   spec.CustomerAccount,
		node:              spec.Node,
		expectedArrivalAt: registerInstant(spec.ExpectedArrivalAt),
		numbers:           slices.Clone(spec.Numbers),
		deliveringParty:   spec.DeliveringParty,
		source:            spec.Source,
		formedAt:          registerInstant(spec.FormedAt),
	}, nil
}

func (preAdvice DeliveryPreAdvice) TenantID() TenantID      { return preAdvice.tenantID }
func (preAdvice DeliveryPreAdvice) ID() DeliveryPreAdviceID { return preAdvice.id }
func (preAdvice DeliveryPreAdvice) CustomerAccount() CustomerAccountReference {
	return preAdvice.customerAccount
}
func (preAdvice DeliveryPreAdvice) Node() NodeReference           { return preAdvice.node }
func (preAdvice DeliveryPreAdvice) ExpectedArrivalAt() time.Time  { return preAdvice.expectedArrivalAt }
func (preAdvice DeliveryPreAdvice) Source() CustomerRequestSource { return preAdvice.source }
func (preAdvice DeliveryPreAdvice) FormedAt() time.Time           { return preAdvice.formedAt }

// Numbers 交回所列号码的副本，照交来的顺序。
func (preAdvice DeliveryPreAdvice) Numbers() []PreAdvisedParcelNumber {
	return slices.Clone(preAdvice.numbers)
}

// DeliveringParty 报告交付方线索及其是否在场。
func (preAdvice DeliveryPreAdvice) DeliveringParty() (DeliveringPartyReference, bool) {
	return preAdvice.deliveringParty, preAdvice.deliveringParty.valid()
}

// Lists 报告现场观察到的号码是否逐字是所列号码之一（ADR-0212 决定二「外部标识观察逐字等于所列号码之一」）。
func (preAdvice DeliveryPreAdvice) Lists(mark string) bool {
	return slices.ContainsFunc(preAdvice.numbers, func(number PreAdvisedParcelNumber) bool { return number.Matches(mark) })
}

func (preAdvice DeliveryPreAdvice) valid() bool {
	return preAdvice.tenantID.valid() && preAdvice.id.valid()
}

// DeliveryPreAdviceWithdrawalSpec 是撤回一份预报所需的输入。CustomerAccount 是撤回它的那个客户账户：只能撤自己的预报，别的账户的
// 与查无此预报同答（UC-NO-006 输入契约「预报撤回」）。
type DeliveryPreAdviceWithdrawalSpec struct {
	TenantID        TenantID
	PreAdvice       DeliveryPreAdviceID
	CustomerAccount CustomerAccountReference
	Source          CustomerRequestSource
	WithdrawnAt     time.Time
}

// DeliveryPreAdviceWithdrawal 是一份预报的撤回。撤回是终态（NO CONTEXT 生命周期「送站预报」）：一份预报至多一份撤回，此后的收寄与
// 重核不再据它成立范围，此前据它形成的关联不回滚。
type DeliveryPreAdviceWithdrawal struct {
	tenantID        TenantID
	preAdvice       DeliveryPreAdviceID
	customerAccount CustomerAccountReference
	source          CustomerRequestSource
	withdrawnAt     time.Time
}

func FormDeliveryPreAdviceWithdrawal(spec DeliveryPreAdviceWithdrawalSpec) (DeliveryPreAdviceWithdrawal, error) {
	if !spec.TenantID.valid() ||
		!spec.PreAdvice.valid() ||
		!spec.CustomerAccount.valid() ||
		!spec.Source.valid() ||
		spec.WithdrawnAt.IsZero() {
		return DeliveryPreAdviceWithdrawal{}, ErrInvalidDeliveryPreAdvice
	}
	return DeliveryPreAdviceWithdrawal{
		tenantID:        spec.TenantID,
		preAdvice:       spec.PreAdvice,
		customerAccount: spec.CustomerAccount,
		source:          spec.Source,
		withdrawnAt:     registerInstant(spec.WithdrawnAt),
	}, nil
}

func (withdrawal DeliveryPreAdviceWithdrawal) TenantID() TenantID { return withdrawal.tenantID }
func (withdrawal DeliveryPreAdviceWithdrawal) PreAdvice() DeliveryPreAdviceID {
	return withdrawal.preAdvice
}
func (withdrawal DeliveryPreAdviceWithdrawal) CustomerAccount() CustomerAccountReference {
	return withdrawal.customerAccount
}
func (withdrawal DeliveryPreAdviceWithdrawal) Source() CustomerRequestSource {
	return withdrawal.source
}
func (withdrawal DeliveryPreAdviceWithdrawal) WithdrawnAt() time.Time { return withdrawal.withdrawnAt }

func (withdrawal DeliveryPreAdviceWithdrawal) valid() bool {
	return withdrawal.tenantID.valid() && withdrawal.preAdvice.valid()
}

// DeliveryPreAdviceState 是一份预报此刻的证据状态（ADR-0212 决定四、六「依据此刻的证据状态」）：预报本身，加它有没有被撤回。零值
// 即「不在册」——推导据此答依据不在册，调用方不必另带一个布尔。
type DeliveryPreAdviceState struct {
	PreAdvice  DeliveryPreAdvice
	Withdrawal DeliveryPreAdviceWithdrawal
}

// Registered 报告这份预报在册。
func (state DeliveryPreAdviceState) Registered() bool {
	return state.PreAdvice.valid()
}

// Withdrawn 报告这份预报已被撤回。
func (state DeliveryPreAdviceState) Withdrawn() bool {
	return state.Withdrawal.valid()
}

// registerInstant 把一个时刻折成册上的样子：UTC、微秒。库的 timestamptz 只到微秒，不截在形成处，同一份事实落册前后就差几个纳秒，
// 重放比对与读回比对都会把它当成另一份。
func registerInstant(at time.Time) time.Time {
	return at.UTC().Truncate(time.Microsecond)
}
