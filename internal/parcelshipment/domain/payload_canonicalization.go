package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

// 本文件是 PS CONTEXT「规范化业务内容摘要（PayloadDigest）」的机制半边：把规范化后的
// 客户请求内容变成稳定摘要，供同一来源身份下的重放与冲突分类使用。真渠道 Intake 与
// 测试共用这一个函数；把渠道原始载荷翻译成 SubmissionPayloadSpec 的词表属 PAR-INT-01
// 实例半边，这里一个渠道字段名都不认。
//
// 摘要输入刻意没有 occurredAt / receivedAt 的位置：CONTEXT 把两者定为来源信封元数据，
// 不进摘要、不能单独造成冲突。结构上放不进去，比任何「记得别放」都硬。

var ErrInvalidSubmissionPayload = errors.New("parcel shipment: invalid submission payload")

// payloadCanonicalizationVersion 标识规范化形状（ADR-0014）：版本进摘要输入，形状变更
// 走新版本、不改旧形状的产出。PSC-1 的强类型段只覆盖领域已建模的内容（成员及其测量
// 画像、基础版本、requestEffectiveAt）；寄收件范围与服务要求尚无领域模型（地址边界
// 属 T3 待裁），以规范化条目承载。等这些内容拿到自己的模型，按 ADR-0014 开 PSC-2，
// 不在 PSC-1 上就地扩列。
const payloadCanonicalizationVersion = "PSC-1"

// CurrentPayloadCanonicalizationVersion 报出本构建按哪套形状规范化。按其他取值记录的
// 摘要，本构建无法重算。
func CurrentPayloadCanonicalizationVersion() string { return payloadCanonicalizationVersion }

// CanonicalContentEntry 是尚无领域模型的规范化内容条目。名做键：去空白后必须非空；
// 值保真原样进摘要——「显式清空」与「条目缺席」是两种内容，前者是一条值为空的条目，
// 后者根本没有这条。
type CanonicalContentEntry struct {
	name  requiredValue
	value string
}

func NewCanonicalContentEntry(name, value string) (CanonicalContentEntry, error) {
	required, err := newRequiredValue("canonical content entry name", strings.TrimSpace(name))
	if err != nil {
		return CanonicalContentEntry{}, err
	}
	return CanonicalContentEntry{name: required, value: value}, nil
}

func (entry CanonicalContentEntry) Name() string {
	return entry.name.String()
}

func (entry CanonicalContentEntry) Value() string {
	return entry.value
}

func (entry CanonicalContentEntry) valid() bool {
	return entry.name.valid()
}

// SubmissionPayloadSpec 是一次客户提交的规范化业务内容，段位对应 CONTEXT 摘要定义的
// 五类：成员（DeclaredParcelIDs + Profiles）、范围（Scope）、基础版本（BasisVersion）、
// 服务要求（Service，含客户委托参考之外的服务请求内容）、requestEffectiveAt。
type SubmissionPayloadSpec struct {
	// RequestReference 是客户声明的委托参考（UC-PS-001 服务请求组），属请求内容：同一
	// 来源请求键换一个委托参考就是另一份内容。
	RequestReference ShipmentRequestID
	// BasisVersion 是受控补充/更正所声明的基础版本；首次提交保持零值。基准不同即
	// 内容不同（CONTEXT：同一身份携带不同基础版本形成冲突）。
	BasisVersion SubmissionVersionID
	// EffectiveAt 是客户请求生效时间（requestEffectiveAt）。缺失保持零值——值与
	// 缺失/显式存在状态都进摘要，绝不用 occurredAt 或 receivedAt 顶替（CONTEXT 硬句，
	// 家规先例见 customer_source_data.go 的同名字段）。
	EffectiveAt time.Time
	// DeclaredParcelIDs 与 Profiles 沿用提交管线的成员语义：成员至少一名、不重复；
	// 画像允许缺席或部分覆盖，但指着集合外成员与一员两张即拒（与 SubmitShipmentRequest
	// 同款裁决——摘要收下一份委托管线会拒的输入，重放分类就会先于受理答「已有结果」）。
	DeclaredParcelIDs []DeclaredParcelID
	Profiles          []DeclaredParcelProfile
	// Scope 承载寄收件范围（寄件关系、收件关系、地址、目的服务范围），Service 承载
	// 服务产品/服务要求与客户明确约束。两段的条目词表由渠道翻译规则给出，这里只定
	// 排序、去重与编码纪律。
	Scope   []CanonicalContentEntry
	Service []CanonicalContentEntry
}

// CanonicalizeSubmissionPayload 按当前规范化版本把规范化业务内容变成 PayloadDigest。
//
// 摘要串自带版本前缀（`PSC-1:<sha256>`）：ADR-0014 要求已保存摘要必须携带产生它的
// 规范化版本，而来源指纹只存摘要串一格——前缀让版本随既有存储同行，零迁移。跨版本
// 的比较纪律（版本不同不是冲突、回放按原版本重新规范化）在只有一个版本的今天没有
// 分支可走；引入 PSC-2 的那笔工作按前缀取版本再分支。
func CanonicalizeSubmissionPayload(spec SubmissionPayloadSpec) (PayloadDigest, error) {
	if !spec.RequestReference.valid() {
		return PayloadDigest{}, ErrInvalidSubmissionPayload
	}
	members, err := canonicalMemberDocuments(spec.DeclaredParcelIDs, spec.Profiles)
	if err != nil {
		return PayloadDigest{}, err
	}
	scope, err := canonicalEntryDocuments(spec.Scope)
	if err != nil {
		return PayloadDigest{}, err
	}
	service, err := canonicalEntryDocuments(spec.Service)
	if err != nil {
		return PayloadDigest{}, err
	}

	document := canonicalSubmissionDocument{
		Canonicalization: payloadCanonicalizationVersion,
		RequestReference: spec.RequestReference.String(),
		BasisVersion:     spec.BasisVersion.String(),
		EffectiveAt:      canonicalEffectiveAtValue(spec.EffectiveAt),
		Members:          members,
		Scope:            scope,
		Service:          service,
	}
	encoded, err := json.Marshal(document)
	if err != nil {
		return PayloadDigest{}, fmt.Errorf("canonicalize submission payload: %w", err)
	}
	sum := sha256.Sum256(encoded)
	return NewPayloadDigest(payloadCanonicalizationVersion + ":" + hex.EncodeToString(sum[:]))
}

// canonicalSubmissionDocument 是 PSC-1 的封闭编码形状。所有段常在场、缺席以显式空值
// 表达，不用 omitempty——那是给「老摘要要保持可比」的形状演进用的（见计价侧先例），
// 全新版本用不上，省掉它换取「读形状即读全集」。
type canonicalSubmissionDocument struct {
	Canonicalization string                       `json:"canonicalization"`
	RequestReference string                       `json:"request_reference"`
	BasisVersion     string                       `json:"basis_version"`
	EffectiveAt      canonicalEffectiveAtDocument `json:"request_effective_at"`
	Members          []canonicalMemberDocument    `json:"members"`
	Scope            []canonicalEntryDocument     `json:"scope"`
	Service          []canonicalEntryDocument     `json:"service"`
}

// canonicalEffectiveAtDocument 把值与存在状态各占一格：缺失不是一种特殊的值，是另一种
// 状态，两格分开才谈得上「缺失与显式存在的差异形成冲突」。
type canonicalEffectiveAtDocument struct {
	Declared bool   `json:"declared"`
	At       string `json:"at"`
}

func canonicalEffectiveAtValue(at time.Time) canonicalEffectiveAtDocument {
	if at.IsZero() {
		return canonicalEffectiveAtDocument{Declared: false, At: ""}
	}
	// 统一到 UTC 再编码：同一时刻带不同时区表示是同一内容，不是两份。
	return canonicalEffectiveAtDocument{Declared: true, At: at.UTC().Format(time.RFC3339Nano)}
}

type canonicalMemberDocument struct {
	Reference  string                       `json:"reference"`
	Weight     *canonicalWeightDocument     `json:"weight"`
	Dimensions *canonicalDimensionsDocument `json:"dimensions"`
}

type canonicalWeightDocument struct {
	Value string `json:"value"`
	Unit  string `json:"unit"`
}

type canonicalDimensionsDocument struct {
	Length string `json:"length"`
	Width  string `json:"width"`
	Height string `json:"height"`
	Unit   string `json:"unit"`
}

type canonicalEntryDocument struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

func canonicalMemberDocuments(
	parcels []DeclaredParcelID,
	profiles []DeclaredParcelProfile,
) ([]canonicalMemberDocument, error) {
	if len(parcels) == 0 {
		return nil, ErrNoDeclaredParcels
	}
	declared := make(map[DeclaredParcelID]struct{}, len(parcels))
	for _, parcelID := range parcels {
		if !parcelID.valid() {
			return nil, ErrInvalidSubmissionPayload
		}
		if _, duplicated := declared[parcelID]; duplicated {
			return nil, ErrDuplicateDeclaredParcel
		}
		declared[parcelID] = struct{}{}
	}

	measurements := make(map[DeclaredParcelID]DeclaredMeasurement, len(profiles))
	for _, profile := range profiles {
		if _, member := declared[profile.parcel]; !member || !profile.measurement.declared() {
			return nil, ErrInvalidDeclaredMeasurement
		}
		if _, duplicated := measurements[profile.parcel]; duplicated {
			return nil, ErrInvalidDeclaredMeasurement
		}
		measurements[profile.parcel] = profile.measurement
	}

	documents := make([]canonicalMemberDocument, 0, len(parcels))
	for _, parcelID := range parcels {
		document := canonicalMemberDocument{Reference: parcelID.String()}
		if measurement, present := measurements[parcelID]; present {
			weight := canonicalWeightDocument{
				Value: measurement.weight.value.String(),
				Unit:  measurement.weight.unit.String(),
			}
			document.Weight = &weight
			if dimensions, present := measurement.Dimensions(); present {
				document.Dimensions = &canonicalDimensionsDocument{
					Length: dimensions.length.String(),
					Width:  dimensions.width.String(),
					Height: dimensions.height.String(),
					Unit:   dimensions.unit.String(),
				}
			}
		}
		documents = append(documents, document)
	}
	// 成员按引用排序：成员是集合（CONTEXT 称「成员集合」），给入顺序不是内容。
	sort.Slice(documents, func(left, right int) bool {
		return documents[left].Reference < documents[right].Reference
	})
	return documents, nil
}

func canonicalEntryDocuments(entries []CanonicalContentEntry) ([]canonicalEntryDocument, error) {
	documents := make([]canonicalEntryDocument, 0, len(entries))
	for _, entry := range entries {
		// 零值条目只能是绕过构造函数硬造的，收下它等于收一条没有名字的内容。
		if !entry.valid() {
			return nil, ErrInvalidSubmissionPayload
		}
		documents = append(documents, canonicalEntryDocument{Name: entry.Name(), Value: entry.Value()})
	}
	sort.Slice(documents, func(left, right int) bool {
		if documents[left].Name != documents[right].Name {
			return documents[left].Name < documents[right].Name
		}
		return documents[left].Value < documents[right].Value
	})
	// 逐字相同的条目去重：条目是集合语义，同一句话说两遍不是第二种内容。渠道翻译若
	// 不稳定地重复条目，去重让重放仍被认出是重放。
	compacted := make([]canonicalEntryDocument, 0, len(documents))
	for _, document := range documents {
		if len(compacted) > 0 && compacted[len(compacted)-1] == document {
			continue
		}
		compacted = append(compacted, document)
	}
	return compacted, nil
}
