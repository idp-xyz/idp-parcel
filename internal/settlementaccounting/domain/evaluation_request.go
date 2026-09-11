package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

var (
	ErrInvalidEvaluationRequest  = errors.New("settlement accounting: invalid evaluation request")
	ErrUnknownCalculationPurpose = errors.New("settlement accounting: unknown calculation purpose")
)

// EvaluationRequestID 是评价请求的铸造身份（票 sa-cc/08 裁决 2）。后续信封与预期成本形成的消费者
// 引的都是它，不引自然键——自然键的成分一变引用就断，铸造出来的 ID 不随成分变。
type EvaluationRequestID struct{ requiredValue }

func NewEvaluationRequestID(value string) (EvaluationRequestID, error) {
	required, err := newRequiredValue("evaluation request ID", value)
	return EvaluationRequestID{required}, err
}

// PrimaryScopeReference 指名结算提交主要范围（UC-SA-002 步 2「结算提交主要范围、计算目的和合格来源
// 引用」）。它由调用方交进来，本上下文不从任何登记册推。与 SettlementScope 不是一回事：那是资金维
// （责任法人 / 结算账户 / 币种），这里是一次结算提交所针对的业务范围。
type PrimaryScopeReference struct{ requiredValue }

func NewPrimaryScopeReference(value string) (PrimaryScopeReference, error) {
	required, err := newRequiredValue("primary scope reference", value)
	return PrimaryScopeReference{required}, err
}

// RequesterReference 指名发起请求的一方——结算作业或上游编排。
type RequesterReference struct{ requiredValue }

func NewRequesterReference(value string) (RequesterReference, error) {
	required, err := newRequiredValue("requester reference", value)
	return RequesterReference{required}, err
}

// CalculationPurpose 是评价请求的计算目的，封闭词表。首发只有一格：BUY 方向的供应商成本，与
// parcel-pricing 的 SUPPLIER_COST 目的成对（票面写作 BUY·SUPPLIER_COST）。词表封闭是有意的：
// 目的与价格方向在提供方那边成对声明，本上下文能请求的目的只能是提供方认下的那几对。
type CalculationPurpose uint8

const (
	CalculationPurposeInvalid CalculationPurpose = iota
	BuySupplierCost
)

func (purpose CalculationPurpose) String() string {
	switch purpose {
	case BuySupplierCost:
		return "BUY_SUPPLIER_COST"
	default:
		return ""
	}
}

func (purpose CalculationPurpose) valid() bool {
	return purpose.String() != ""
}

// ParseCalculationPurpose 把登记册里的文本折回词表。词表外的值拒：库里出现一个本上下文不认识的
// 目的，只可能是适配器或迁移的 bug，读成某一格会让一份请求换了目的还看不出来。
func ParseCalculationPurpose(value string) (CalculationPurpose, error) {
	for _, candidate := range []CalculationPurpose{BuySupplierCost} {
		if candidate.String() == value {
			return candidate, nil
		}
	}
	return CalculationPurposeInvalid, fmt.Errorf("%w: %q", ErrUnknownCalculationPurpose, value)
}

// EligibleSourceReferences 是一次 BUY 评价请求携带的合格来源引用三件：TF 运输收费发生项、费用项目、
// 供应商协议版本。三件由调用方交进来——本上下文不读 TF / PC 的内容，只登记引用；从 TF 登记册推出来的
// 匹配不是「合格来源引用」（票 sa-cc/08 红线）。
type EligibleSourceReferences struct {
	Occurrence TransportChargeOccurrence
	FeeItem    FeeItemReference
	Agreement  SupplierAgreementReference
}

func (sources EligibleSourceReferences) valid() bool {
	return sources.Occurrence.ID().valid() &&
		sources.Occurrence.Version().valid() &&
		sources.FeeItem.valid() &&
		sources.Agreement.valid()
}

// sourceReferenceCanonicalization 标识合格来源引用摘要所依据的规范化形状。已保存的摘要必须携带产生它的
// 规范化版本（ADR-0014，先例 `PCC-1:<hex>`）：改了摘要成分就换号，旧串与新串在库上不可比、不会被
// 误当成同一自然键。
const sourceReferenceCanonicalization = "ESRC-1"

// EvaluationRequestNaturalKey 是评价请求的自然键（裁决 2）：主要范围 + 计算目的 + 合格来源引用集合的
// 稳定摘要；租户在登记册键上另带。同自然键的第二次提交是重放，答`已存在`并交回原 ID；成分不同就是
// 另一份请求、另一个 ID。
type EvaluationRequestNaturalKey struct {
	Scope        PrimaryScopeReference
	Purpose      CalculationPurpose
	SourceDigest string
}

// EvaluationRequestNaturalKeyOf 由成分算自然键。
//
// 摘要**排序后**再算（照 PCC-1 集合稳定序的先例）：每件引用写成「种类=值」再按字典序排，同一组引用
// 不论以什么顺序交进来都只有一种字节；种类标签让排序不会把两种引用的同一个值当成一件。发生项以
// 「身份@版本」入摘要——发生项有效性更正换版本，指向另一版本的就是另一份请求；原因与业务时间是
// TF 在该版本上钉死的属性，随引用带出供形成用，不构成第二个身份维。
func EvaluationRequestNaturalKeyOf(
	scope PrimaryScopeReference,
	purpose CalculationPurpose,
	sources EligibleSourceReferences,
) (EvaluationRequestNaturalKey, error) {
	if !scope.valid() || !purpose.valid() || !sources.valid() {
		return EvaluationRequestNaturalKey{}, ErrInvalidEvaluationRequest
	}
	entries := []string{
		"occurrence=" + sources.Occurrence.ID().String() + "@" + sources.Occurrence.Version().String(),
		"fee-item=" + sources.FeeItem.String(),
		"supplier-agreement=" + sources.Agreement.String(),
	}
	sort.Strings(entries)
	sum := sha256.Sum256([]byte(strings.Join(entries, "\x00")))
	return EvaluationRequestNaturalKey{
		Scope:        scope,
		Purpose:      purpose,
		SourceDigest: sourceReferenceCanonicalization + ":" + hex.EncodeToString(sum[:]),
	}, nil
}

// EvaluationRequestSpec 是提交一份评价请求所需的全部输入。
type EvaluationRequestSpec struct {
	ID          EvaluationRequestID
	Scope       PrimaryScopeReference
	Purpose     CalculationPurpose
	Sources     EligibleSourceReferences
	RequestedAt time.Time
	RequestedBy RequesterReference
}

// EvaluationRequest 是 UC-SA-002 步 2「请求评价」留在本上下文的登记：本上下文向 parcel-pricing 提交了
// 什么主要范围、什么计算目的、哪三件合格来源引用。类型上没有金额、币种、换算或评价结果——它们整组
// 归评价（ADR-0107 / ADR-0013），请求只记「问了什么」，不记「答了什么」。
type EvaluationRequest struct {
	id          EvaluationRequestID
	naturalKey  EvaluationRequestNaturalKey
	sources     EligibleSourceReferences
	requestedAt time.Time
	requestedBy RequesterReference
}

// SubmitEvaluationRequest 形成一份评价请求。每一件都必备：缺任何一件 parcel-pricing 都形成不了计价
// 输入快照，而一份缺件的请求登进册，只会让按 ID 回查三件引用的消费者落空。
func SubmitEvaluationRequest(spec EvaluationRequestSpec) (EvaluationRequest, error) {
	if !spec.ID.valid() || !spec.RequestedBy.valid() || spec.RequestedAt.IsZero() {
		return EvaluationRequest{}, ErrInvalidEvaluationRequest
	}
	naturalKey, err := EvaluationRequestNaturalKeyOf(spec.Scope, spec.Purpose, spec.Sources)
	if err != nil {
		return EvaluationRequest{}, err
	}
	return EvaluationRequest{
		id:          spec.ID,
		naturalKey:  naturalKey,
		sources:     spec.Sources,
		requestedAt: spec.RequestedAt.UTC(),
		requestedBy: spec.RequestedBy,
	}, nil
}

func (request EvaluationRequest) ID() EvaluationRequestID {
	return request.id
}

func (request EvaluationRequest) Scope() PrimaryScopeReference {
	return request.naturalKey.Scope
}

func (request EvaluationRequest) Purpose() CalculationPurpose {
	return request.naturalKey.Purpose
}

func (request EvaluationRequest) Sources() EligibleSourceReferences {
	return request.sources
}

func (request EvaluationRequest) NaturalKey() EvaluationRequestNaturalKey {
	return request.naturalKey
}

func (request EvaluationRequest) RequestedAt() time.Time {
	return request.requestedAt
}

func (request EvaluationRequest) RequestedBy() RequesterReference {
	return request.requestedBy
}
