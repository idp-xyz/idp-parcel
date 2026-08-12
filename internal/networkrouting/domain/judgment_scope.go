package domain

import (
	"errors"
	"time"
)

var (
	ErrInvalidJudgmentAsOf = errors.New("network routing: invalid judgment asOf")
)

type TenantID struct{ requiredValue }

func NewTenantID(value string) (TenantID, error) {
	required, err := newRequiredValue("tenant ID", value)
	return TenantID{required}, err
}

type CustomerAccountID struct{ requiredValue }

func NewCustomerAccountID(value string) (CustomerAccountID, error) {
	required, err := newRequiredValue("customer account ID", value)
	return CustomerAccountID{required}, err
}

type ShipmentRequestID struct{ requiredValue }

func NewShipmentRequestID(value string) (ShipmentRequestID, error) {
	required, err := newRequiredValue("shipment request ID", value)
	return ShipmentRequestID{required}, err
}

type SubmissionVersionID struct{ requiredValue }

func NewSubmissionVersionID(value string) (SubmissionVersionID, error) {
	required, err := newRequiredValue("submission version ID", value)
	return SubmissionVersionID{required}, err
}

type DeclaredParcelID struct{ requiredValue }

func NewDeclaredParcelID(value string) (DeclaredParcelID, error) {
	required, err := newRequiredValue("declared parcel ID", value)
	return DeclaredParcelID{required}, err
}

// RequestCorrelationID 是发起方给一次判断请求的稳定关联。它不参与判断身份：判断身份由
// 范围各维度构成，关联只用来找回原次请求——响应或事件丢失时按它查询原判断，而不是靠一次
// 无关联的新请求去猜结果。
type RequestCorrelationID struct{ requiredValue }

func NewRequestCorrelationID(value string) (RequestCorrelationID, error) {
	required, err := newRequiredValue("request correlation ID", value)
	return RequestCorrelationID{required}, err
}

// Valid 供应用层在读取任何权威之前判断请求是否可关联。关联不成立时既找不回原判断也接不
// 回续办，因此它与判断身份一样属于受理前提。
func (id RequestCorrelationID) Valid() bool {
	return id.valid()
}

// NetworkViewRevision 标识一次判断所依据的网络证据视图版本。CONTEXT 要求每次判断保留
// 「关键输入的有效区间和当前修订标识」——修订标识就是消费方日后发现「关键依据已经失效、
// 被替代或修订标识变化」的比对锚（`AT-PS-037` 提交前重判的触发载体）。它与 PC 的
// AuthorityViewRevision 同型不同主：网络视图属本上下文。
type NetworkViewRevision struct{ requiredValue }

func NewNetworkViewRevision(value string) (NetworkViewRevision, error) {
	required, err := newRequiredValue("network view revision", value)
	return NetworkViewRevision{required}, err
}

// Valid 供应用层核对证据答复的完整性：一份不带修订标识的证据答复，判断留不下比对锚。
func (revision NetworkViewRevision) Valid() bool {
	return revision.valid()
}

// ServicePurpose 是本次要判断的服务目的，取自服务产品而不是本上下文的枚举。它刻意是
// 引用而非封闭取值集：目的由产品定义，在这里列一份就成了第二处定义，而本上下文并不拥有
// 服务产品。
type ServicePurpose struct{ requiredValue }

func NewServicePurpose(value string) (ServicePurpose, error) {
	required, err := newRequiredValue("service purpose", value)
	return ServicePurpose{required}, err
}

type AsOfSemantic struct{ requiredValue }

func NewAsOfSemantic(value string) (AsOfSemantic, error) {
	required, err := newRequiredValue("asOf semantic", value)
	return AsOfSemantic{required}, err
}

type AsOfStrategyVersion struct{ requiredValue }

func NewAsOfStrategyVersion(value string) (AsOfStrategyVersion, error) {
	required, err := newRequiredValue("asOf strategy version", value)
	return AsOfStrategyVersion{required}, err
}

// JudgmentAsOf 是发起方按所采用接单规则包的时点策略为本次可达性判断形成的适用时点。
//
// 语义与策略版本和取值一同携带，因为用例要求本上下文校验并回显实际采用值：只回显一个
// 时刻证明不了它来自哪条策略，而「不得用一个全局时间代替各类判断的时点语义」正是要靠
// 这两项才验得出来。三项缺一即构造不出来——这是拦住本上下文拿自己的时钟顶替它的办法。
type JudgmentAsOf struct {
	semantic        AsOfSemantic
	at              time.Time
	strategyVersion AsOfStrategyVersion
}

func NewJudgmentAsOf(semantic AsOfSemantic, at time.Time, strategyVersion AsOfStrategyVersion) (JudgmentAsOf, error) {
	if !semantic.valid() || at.IsZero() || !strategyVersion.valid() {
		return JudgmentAsOf{}, ErrInvalidJudgmentAsOf
	}
	return JudgmentAsOf{semantic: semantic, at: at.UTC(), strategyVersion: strategyVersion}, nil
}

func (asOf JudgmentAsOf) Semantic() AsOfSemantic {
	return asOf.semantic
}

func (asOf JudgmentAsOf) At() time.Time {
	return asOf.at
}

func (asOf JudgmentAsOf) StrategyVersion() AsOfStrategyVersion {
	return asOf.strategyVersion
}

func (asOf JudgmentAsOf) valid() bool {
	return asOf.semantic.valid() && !asOf.at.IsZero() && asOf.strategyVersion.valid()
}

func (asOf JudgmentAsOf) sameAs(other JudgmentAsOf) bool {
	return asOf.semantic == other.semantic &&
		asOf.strategyVersion == other.strategyVersion &&
		asOf.at.Equal(other.at)
}

// ReachabilityJudgmentKey 是一次包裹级可达性判断的完整范围。少任何一维，一个包裹的判断
// 就可能回答另一个包裹、另一个提交版本或另一个客户——用例把「不得跨包裹、跨客户或跨提交
// 版本复用结果」放在候选评估的第一层，正是因为后面每一层都建立在这个范围成立之上。
type ReachabilityJudgmentKey struct {
	TenantID          TenantID
	CustomerAccountID CustomerAccountID
	ShipmentRequestID ShipmentRequestID
	SubmissionVersion SubmissionVersionID
	DeclaredParcelID  DeclaredParcelID
	ServicePurpose    ServicePurpose
	AsOf              JudgmentAsOf
}

// MinimumIdentityEstablished 让应用层在读取任何权威之前就能判断该不该查。用例要求最小
// 判断身份不成立时请求未受理，而一次已经发出的查询收不回来，它本身就回答了这个客户、这个
// 委托存不存在。
func (key ReachabilityJudgmentKey) MinimumIdentityEstablished() bool {
	return key.TenantID.valid() &&
		key.CustomerAccountID.valid() &&
		key.ShipmentRequestID.valid() &&
		key.SubmissionVersion.valid() &&
		key.DeclaredParcelID.valid() &&
		key.ServicePurpose.valid() &&
		key.AsOf.valid()
}

// SameJudgmentScope 回答两次请求是不是同一次判断。它是幂等与冲突共用的那条界线：相同
// 范围的重复请求返回原判断，范围不同却共用一个请求关联则是冲突，两者都不得覆盖原判断。
// 界线定义在这里而不在应用层，因为判断身份由哪些维度构成是本上下文的规则。
func (key ReachabilityJudgmentKey) SameJudgmentScope(other ReachabilityJudgmentKey) bool {
	return key.TenantID == other.TenantID &&
		key.CustomerAccountID == other.CustomerAccountID &&
		key.ShipmentRequestID == other.ShipmentRequestID &&
		key.SubmissionVersion == other.SubmissionVersion &&
		key.DeclaredParcelID == other.DeclaredParcelID &&
		key.ServicePurpose == other.ServicePurpose &&
		key.AsOf.sameAs(other.AsOf)
}
