package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 首方客户命令四口的规范化形状版本（ADR-0139 决定八「未定形状的口不开首方渠道」；ADR-0310；票 first-party-api/06）。
// 写法照 payloadCanonicalizationVersion（PSC-1，ADR-0014）：版本进摘要输入，形状变更走新版本、不改旧形状的产出。
//
// 本票只定名与版本，不定字段：形状的字段、编码与摘要函数归各口换上首方 Intake 的那一票填，填之前这四口的摘要没有产出者，
// 对应的首方口也就不开。名彼此不同、也不同于 PSC-1，是为了让已存的摘要串凭前缀就认得出它是哪一口、哪一版的形状。
const (
	// CustomerWithdrawalShapeVersion 是委托撤回的形状（PWD-1，票 first-party-api/07 填字段，见 CanonicalizeCustomerWithdrawal）。
	CustomerWithdrawalShapeVersion = "PWD-1"
	// CustomerParcelCancellationShapeVersion 是取消包裹的形状（PCN-1，票 first-party-api/11 填字段，见 CanonicalizeCustomerParcelCancellation）。
	CustomerParcelCancellationShapeVersion = "PCN-1"
	// CustomerSourceDataAmendmentShapeVersion 是客户资料修订的形状，字段由票 first-party-api/10 填；要素内容复用 PSC-1 的要素构造，
	// 不重算（pp-seams/05）。
	CustomerSourceDataAmendmentShapeVersion = "PAM-1"
	// CustomerSupplementShapeVersion 是委托补充的形状，字段由票 first-party-api/08 填；它另立名而不沿用 PSC-1，是因为补充载荷
	// 多带基准版本一格，且重放与冲突要在补充自己的判重空间里判。
	CustomerSupplementShapeVersion = "PSU-1"
)

// CanonicalizeCustomerSupplement 是首方补充口的规范化入口（形状 PSU-1，票 first-party-api/08）。内容段与 PSC-1 同构——补充重报
// 整份内容、不从旧版本继承（FormNewSubmissionVersionCommand 的同名纪律），规格里的 BasisVersion 必须带上客户带回的基准：基准
// 不同即内容不同，同一补充请求标识换基准重发是来源冲突、不是重放。版本名 PSU-1 进摘要前缀，补充的判重空间因此不与提交的
// PSC-1 摘要相混。
func CanonicalizeCustomerSupplement(spec SubmissionPayloadSpec) (CanonicalSubmission, error) {
	if !spec.BasisVersion.valid() {
		return CanonicalSubmission{}, ErrInvalidSubmissionPayload
	}
	digest, err := canonicalizeSubmissionPayloadUnder(CustomerSupplementShapeVersion, spec)
	if err != nil {
		return CanonicalSubmission{}, err
	}
	return canonicalSubmissionOf(digest, spec), nil
}

// CustomerWithdrawalSpec 是首方撤回载荷里进摘要的指名与原因（形状 PWD-1）。撤回者、租户与账户不在其中：它们取自铸造的信封，
// 不由载荷自报，也就不该参与「同一请求是否同一内容」的判定。
type CustomerWithdrawalSpec struct {
	// OriginalAccessRequestID 是原提交的接入请求标识，指名要撤的委托。
	OriginalAccessRequestID string
	// ShipmentRequestID 是客户带回的回执委托号；载荷没带时为空，摘要里写成显式空值，带与不带是两份内容。
	ShipmentRequestID string
	// Reason 是结构化原因。
	Reason WithdrawalReasonReference
}

// canonicalWithdrawalDocument 是 PWD-1 的封闭编码形状：段常在场、缺席以空串显式写出，字段顺序固定，版本名同进文档与摘要前缀，
// 所以撤回的判重空间不会与 PSC-1、PSU-1 的摘要相混。
type canonicalWithdrawalDocument struct {
	Canonicalization        string `json:"canonicalization"`
	OriginalAccessRequestID string `json:"originalAccessRequestId"`
	ShipmentRequestID       string `json:"shipmentRequestId"`
	Reason                  string `json:"reason"`
}

// CanonicalizeCustomerWithdrawal 是首方撤回口的规范化入口。同一撤回接入请求标识改指另一份原提交、另一个委托号或另一个原因，
// 摘要就变，答来源冲突而不是当重放。
func CanonicalizeCustomerWithdrawal(spec CustomerWithdrawalSpec) (PayloadDigest, error) {
	original := strings.TrimSpace(spec.OriginalAccessRequestID)
	if original == "" || !spec.Reason.valid() {
		return PayloadDigest{}, ErrInvalidSubmissionPayload
	}
	encoded, err := json.Marshal(canonicalWithdrawalDocument{
		Canonicalization:        CustomerWithdrawalShapeVersion,
		OriginalAccessRequestID: original,
		ShipmentRequestID:       strings.TrimSpace(spec.ShipmentRequestID),
		Reason:                  spec.Reason.String(),
	})
	if err != nil {
		return PayloadDigest{}, fmt.Errorf("canonicalize customer withdrawal: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return NewPayloadDigest(CustomerWithdrawalShapeVersion + ":" + hex.EncodeToString(sum[:]))
}

// CustomerParcelCancellationSpec 是首方取消载荷里进摘要的指名与内容（形状 PCN-1）。取消者、租户与账户不在其中：它们取自铸造的
// 信封，不由载荷自报。
type CustomerParcelCancellationSpec struct {
	// OriginalAccessRequestID 是原提交的接入请求标识，指名要取消其包裹的委托。
	OriginalAccessRequestID string
	// ShipmentRequestID 是客户带回的回执委托号，必填，与找到的委托核对。
	ShipmentRequestID ShipmentRequestID
	Parcel            DeclaredParcelID
	Reason            CancellationReasonReference
	// RequestedAt 是客户系统声明的取消请求时刻。它进取消决定库的内容摘要，所以重放必须带同一个值。
	RequestedAt time.Time
}

type canonicalCancellationDocument struct {
	Canonicalization        string `json:"canonicalization"`
	OriginalAccessRequestID string `json:"originalAccessRequestId"`
	ShipmentRequestID       string `json:"shipmentRequestId"`
	Parcel                  string `json:"parcelId"`
	Reason                  string `json:"reason"`
	RequestedAt             string `json:"requestedAt"`
}

// CanonicalizeCustomerParcelCancellation 是首方取消口的规范化入口（形状 PCN-1）。摘要目前只用来校验载荷立得住，不入库：取消命令没有摘要槽，判重靠取消库自己的内容摘要。指名（原提交、委托号、包裹）、原因与请求时刻
// 全部进摘要，所以同一取消请求改指另一份原提交、另一个包裹、另一个原因或另一个时刻，摘要就变。时刻按 UTC 的 RFC3339Nano 写，
// 同一时刻的不同写法同摘要。
func CanonicalizeCustomerParcelCancellation(spec CustomerParcelCancellationSpec) (PayloadDigest, error) {
	original := strings.TrimSpace(spec.OriginalAccessRequestID)
	if original == "" || !spec.ShipmentRequestID.valid() || !spec.Parcel.valid() || !spec.Reason.valid() || spec.RequestedAt.IsZero() {
		return PayloadDigest{}, ErrInvalidSubmissionPayload
	}
	encoded, err := json.Marshal(canonicalCancellationDocument{
		Canonicalization:        CustomerParcelCancellationShapeVersion,
		OriginalAccessRequestID: original,
		ShipmentRequestID:       spec.ShipmentRequestID.String(),
		Parcel:                  spec.Parcel.String(),
		Reason:                  spec.Reason.String(),
		RequestedAt:             spec.RequestedAt.UTC().Format(time.RFC3339Nano),
	})
	if err != nil {
		return PayloadDigest{}, fmt.Errorf("canonicalize customer parcel cancellation: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return NewPayloadDigest(CustomerParcelCancellationShapeVersion + ":" + hex.EncodeToString(sum[:]))
}

// SourceDataAmendmentPayloadSpec 是首方资料修订口（形状 PAM-1，票 first-party-api/10）的规范化输入：指名（原提交的接入请求
// 标识、资料范围）、基准、意图、原因、适用时间与内容。内容有两种形——地址要素条目（沿提交的条目命名，由 AddressElementsOf 挑）
// 与申报测量；两者最多一种在场，显式清空时都缺席。
type SourceDataAmendmentPayloadSpec struct {
	OriginalAccessRequestID string
	Scope                   SourceDataScope
	Basis                   SourceDataBasis
	Intent                  AmendmentIntent
	Reason                  AmendmentReasonReference
	EffectiveAt             time.Time
	// Entries 是落在寄 / 收件范围上的要素条目，名字由 AddressElementEntryName 拼出。
	Entries     []CanonicalContentEntry
	Measurement DeclaredMeasurement
}

// CanonicalSourceDataAmendment 是一次修订规范化的成对产出：摘要与同一份输入挑出的内容，二者出自同一道门。
type CanonicalSourceDataAmendment struct {
	digest  PayloadDigest
	content SourceDataVersionContent
}

func (canonical CanonicalSourceDataAmendment) Digest() PayloadDigest { return canonical.digest }

func (canonical CanonicalSourceDataAmendment) Content() SourceDataVersionContent {
	return canonical.content
}

type canonicalAmendmentDocument struct {
	Canonicalization string                       `json:"canonicalization"`
	Original         string                       `json:"original_access_request_id"`
	ShipmentRequest  string                       `json:"shipment_request_id"`
	Parcel           string                       `json:"parcel_id"`
	DataGroup        string                       `json:"data_group"`
	BasisVersion     string                       `json:"basis_version"`
	Intent           string                       `json:"intent"`
	Reason           string                       `json:"reason"`
	EffectiveAt      canonicalEffectiveAtDocument `json:"effective_at"`
	Entries          []canonicalEntryDocument     `json:"entries"`
	Weight           *canonicalWeightDocument     `json:"weight"`
	Dimensions       *canonicalDimensionsDocument `json:"dimensions"`
}

// CanonicalizeCustomerSourceDataAmendment 是首方资料修订口的规范化入口（形状 PAM-1）。要素内容复用提交的
// AddressElementsOf，不另写一套挑法；摘要把指名、范围、基准、意图、原因、适用时间与内容全部纳入，所以同一修订请求标识改指
// 另一份原提交、另一处范围或改写内容，摘要就变，答来源冲突而不是当重放。内容与范围、意图不配的组合在这里不判（编排的 admit 判），
// 摘要照常算出。
func CanonicalizeCustomerSourceDataAmendment(spec SourceDataAmendmentPayloadSpec) (CanonicalSourceDataAmendment, error) {
	if strings.TrimSpace(spec.OriginalAccessRequestID) == "" || !spec.Scope.valid() ||
		!spec.Basis.valid() || !spec.Intent.valid() || !spec.Reason.valid() {
		return CanonicalSourceDataAmendment{}, ErrInvalidSubmissionPayload
	}
	entries := append([]CanonicalContentEntry(nil), spec.Entries...)
	for _, entry := range entries {
		if !entry.valid() {
			return CanonicalSourceDataAmendment{}, ErrInvalidSubmissionPayload
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if entries[i].Name() != entries[j].Name() {
			return entries[i].Name() < entries[j].Name()
		}
		return entries[i].Value() < entries[j].Value()
	})
	parcel, _ := spec.Scope.DeclaredParcelID()
	prior, _ := spec.Basis.PriorVersion()
	document := canonicalAmendmentDocument{
		Canonicalization: CustomerSourceDataAmendmentShapeVersion,
		Original:         strings.TrimSpace(spec.OriginalAccessRequestID),
		ShipmentRequest:  spec.Scope.ShipmentRequestID().String(),
		Parcel:           parcel.String(),
		DataGroup:        spec.Scope.DataGroup().String(),
		BasisVersion:     prior.String(),
		Intent:           spec.Intent.String(),
		Reason:           spec.Reason.String(),
		EffectiveAt:      canonicalEffectiveAtValue(spec.EffectiveAt),
		Entries:          make([]canonicalEntryDocument, 0, len(entries)),
	}
	for _, entry := range entries {
		document.Entries = append(document.Entries, canonicalEntryDocument{Name: entry.Name(), Value: entry.Value()})
	}
	var content SourceDataVersionContent
	if spec.Measurement.declared() {
		weight := spec.Measurement.Weight()
		document.Weight = &canonicalWeightDocument{Value: weight.Value().String(), Unit: weight.Unit().String()}
		if dimensions, present := spec.Measurement.Dimensions(); present {
			document.Dimensions = &canonicalDimensionsDocument{
				Length: dimensions.Length().String(), Width: dimensions.Width().String(),
				Height: dimensions.Height().String(), Unit: dimensions.Unit().String(),
			}
		}
		measured, err := NewMeasurementContent(spec.Measurement)
		if err != nil {
			return CanonicalSourceDataAmendment{}, err
		}
		content = measured
	} else if group := spec.Scope.DataGroup(); group == SenderPlaceDataGroup() || group == DeliveryPlaceDataGroup() {
		content = NewAddressElementsContent(AddressElementsOf(group, entries))
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return CanonicalSourceDataAmendment{}, fmt.Errorf("canonicalize source data amendment: %w", err)
	}
	sum := sha256.Sum256(encoded)
	digest, err := NewPayloadDigest(CustomerSourceDataAmendmentShapeVersion + ":" + hex.EncodeToString(sum[:]))
	if err != nil {
		return CanonicalSourceDataAmendment{}, err
	}
	return CanonicalSourceDataAmendment{digest: digest, content: content}, nil
}
