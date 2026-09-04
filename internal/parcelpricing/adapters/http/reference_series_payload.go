package pricinghttp

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是运营操作者面的序列登记载荷（票 pricing-reference-series-operations/08）：管理台逐字段
// 表单组出来的那份 JSON，预览口与登记口收的是**同一份形状、走同一段解码**。形状由产品定义、属
// 机制半边（ADR-0101 决定一）——它不是客户渠道载荷，那一半照旧等 `PAR-INT-01`。
//
// **载荷里只有内容，没有身份。** 租户与登记责任方从 ADR-0100 的 `OperatorEnvelope` 来，由 Intake
// 作为入参交进来；载荷里出现 tenant / registrant 之类的键一律按未知键拒——严格解码不是挑剔，是
// 不让自报身份有地方落。
//
// **三处 domain.VersionReference 的 digest 槽装的是声明令牌，不是摘要**（MCP-3 2026-09-03 裁决）。
// 领域要求引用的 digest 非空，却从不拿它比对任何内容——它只参与身份与指纹；而表单路径上没有任何
// 一处能给出真正的摘要：自身引用不能装 PRS 内容摘要，因为快照文档把引用（连 digest）折进了内容
// 摘要，装进去就自指循环；口径引用指向 party-commercial 的价格政策版本，其读口今天不透出
// content_digest；更正回指的那一版册上有登记时声明的引用 digest，逐版本读面透出后照实回指即可。
// 令牌用 `declared:` 前缀自报为声明——比一个长得像哈希而不是哈希的串诚实。**只在新铸引用时铸**：
// 载荷带来的 digest（回指与口径）照实用，不重铸。领域的 NewVersionReference 不为此改动。
// 这一格的领域改法（PRS-2 排除引用槽 / PC 读口透 digest / 把 digest 从引用身份里拿掉）立在票
// pricing-reference-series-operations/09，需 ADR，不在本票。

// ErrOperatorIdentityMissing 表示调用方没交来租户或登记责任方——那不是载荷的错（载荷本来就不该
// 带它们），是 Intake 没拿到操作者信封就来翻译。与 ErrMalformedRequest 分开：前者改载荷没用。
var ErrOperatorIdentityMissing = errors.New("parcel pricing http: operator identity (tenant, registrant) is required")

// declaredTokenPrefix 是声明令牌的前缀，自报「这是声明不是哈希」。
const declaredTokenPrefix = "declared:"

// DeclaredReferenceToken 铸一个版本引用的声明令牌：declared:<kind>/<id>@<version>。它是预览与登记
// 共用的唯一一条铸法——两口对同一份载荷铸出不同令牌，摘要就不同，ADR-0101 决定四那句硬句就破了。
func DeclaredReferenceToken(kind domain.ArtifactKind, id, version string) string {
	return declaredTokenPrefix + string(kind) + "/" + id + "@" + version
}

// ReferenceSeriesRegistrationPayload 是载荷的线格式。字段与 domain.ReferenceSeriesRegistrationSpec
// 一一对应，只少身份两格；不引入领域里没有的概念。
type ReferenceSeriesRegistrationPayload struct {
	SeriesID         string                            `json:"seriesId"`
	SeriesVersion    string                            `json:"seriesVersion"`
	Kind             string                            `json:"kind"`
	SourceIdentifier string                            `json:"sourceIdentifier"`
	QuoteBasis       *ReferenceSeriesQuoteBasisPayload `json:"quoteBasis,omitempty"`
	Periods          []ReferenceSeriesPeriodPayload    `json:"periods"`
	Correction       *ReferenceSeriesCorrectionPayload `json:"correction,omitempty"`
	// CompareWithVersion 只对预览有意义：指名对照版本。登记命令不带它。
	CompareWithVersion string `json:"compareWithVersion,omitempty"`
}

// ReferenceSeriesQuoteBasisPayload 指一版商业价格政策（汇率必备，燃油不得有——由领域构造门判）。
// Digest 可缺：缺则铸声明令牌；PC 读口透出 content_digest 之后表单带真值过来，这里照实用。
type ReferenceSeriesQuoteBasisPayload struct {
	PolicyID      string `json:"policyId"`
	PolicyVersion string `json:"policyVersion"`
	Digest        string `json:"digest,omitempty"`
}

// ReferenceSeriesPeriodPayload 是一期：时刻按 RFC 3339，EndsAt 缺即无上界（只许末期，领域判），
// Value 是十进制文本（不接受指数记法，ParseDecimal 判），EvidenceRef 缺即该期只有断言强度。
type ReferenceSeriesPeriodPayload struct {
	StartsAt    string `json:"startsAt"`
	EndsAt      string `json:"endsAt,omitempty"`
	Value       string `json:"value"`
	EvidenceRef string `json:"evidenceRef,omitempty"`
}

// ReferenceSeriesCorrectionPayload 声明更正关系：回指同序列的哪一版、凭什么。PriorReferenceDigest
// 是那一版登记时声明的引用 digest（逐版本读面的 referenceDigest），带来就照实回指，缺则铸令牌。
type ReferenceSeriesCorrectionPayload struct {
	PriorVersion         string `json:"priorVersion"`
	PriorReferenceDigest string `json:"priorReferenceDigest,omitempty"`
	Basis                string `json:"basis"`
}

// DecodeReferenceSeriesRegistrationPayload 只做结构解码：JSON 合法、键都认识。字段值对不对留给
// 下一步的领域构造器——这里不另造一套校验（ADR-0101 Context：导入器不需要、也不该另造校验规则）。
func DecodeReferenceSeriesRegistrationPayload(body io.Reader) (ReferenceSeriesRegistrationPayload, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var payload ReferenceSeriesRegistrationPayload
	if err := decoder.Decode(&payload); err != nil {
		return ReferenceSeriesRegistrationPayload{}, fmt.Errorf("%w: reference series payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// Registration 把载荷连同信封给的身份翻成领域登记对象。每一格都过领域构造器，拒了就是
// ErrMalformedRequest——同一份内容重发不会变好。
func (payload ReferenceSeriesRegistrationPayload) Registration(
	tenant domain.TenantID,
	registrant string,
) (domain.ReferenceSeriesRegistration, error) {
	if tenant.String() == "" || registrant == "" {
		return domain.ReferenceSeriesRegistration{}, ErrOperatorIdentityMissing
	}

	reference, err := domain.NewVersionReference(
		domain.ArtifactReferenceSeries, payload.SeriesID, payload.SeriesVersion,
		DeclaredReferenceToken(domain.ArtifactReferenceSeries, payload.SeriesID, payload.SeriesVersion))
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, fmt.Errorf("%w: series reference: %v", ErrMalformedRequest, err)
	}

	spec := domain.ReferenceSeriesRegistrationSpec{
		Tenant:           tenant,
		Kind:             domain.ReferenceSeriesKind(payload.Kind),
		Reference:        reference,
		SourceIdentifier: payload.SourceIdentifier,
		Registrant:       registrant,
	}

	if payload.QuoteBasis != nil {
		digest := payload.QuoteBasis.Digest
		if digest == "" {
			digest = DeclaredReferenceToken(domain.ArtifactCommercialPolicy, payload.QuoteBasis.PolicyID, payload.QuoteBasis.PolicyVersion)
		}
		basis, err := domain.NewVersionReference(domain.ArtifactCommercialPolicy, payload.QuoteBasis.PolicyID, payload.QuoteBasis.PolicyVersion, digest)
		if err != nil {
			return domain.ReferenceSeriesRegistration{}, fmt.Errorf("%w: quote basis: %v", ErrMalformedRequest, err)
		}
		spec.QuoteBasis = basis
	}

	spec.Periods = make([]domain.SeriesPeriodValue, 0, len(payload.Periods))
	for index, row := range payload.Periods {
		period, err := row.value()
		if err != nil {
			return domain.ReferenceSeriesRegistration{}, fmt.Errorf("%w: periods[%d]: %v", ErrMalformedRequest, index, err)
		}
		spec.Periods = append(spec.Periods, period)
	}

	if payload.Correction != nil {
		digest := payload.Correction.PriorReferenceDigest
		if digest == "" {
			digest = DeclaredReferenceToken(domain.ArtifactReferenceSeries, payload.SeriesID, payload.Correction.PriorVersion)
		}
		prior, err := domain.NewVersionReference(domain.ArtifactReferenceSeries, payload.SeriesID, payload.Correction.PriorVersion, digest)
		if err != nil {
			return domain.ReferenceSeriesRegistration{}, fmt.Errorf("%w: correction prior version: %v", ErrMalformedRequest, err)
		}
		spec.PriorVersion = prior
		spec.CorrectionBasis = payload.Correction.Basis
	}

	registration, err := domain.NewReferenceSeriesRegistration(spec)
	if err != nil {
		return domain.ReferenceSeriesRegistration{}, fmt.Errorf("%w: registration: %v", ErrMalformedRequest, err)
	}
	return registration, nil
}

// PreviewCommand 与 RegistrationCommand 从同一个 Registration 出发——两口拿到的登记对象是同一个
// 方法的返回值，摘要与三个引用逐字节相同不靠约定靠结构。
func (payload ReferenceSeriesRegistrationPayload) PreviewCommand(
	tenant domain.TenantID,
	registrant string,
) (application.PreviewReferenceSeriesCommand, error) {
	registration, err := payload.Registration(tenant, registrant)
	if err != nil {
		return application.PreviewReferenceSeriesCommand{}, err
	}
	return application.PreviewReferenceSeriesCommand{
		Registration:       registration,
		CompareWithVersion: payload.CompareWithVersion,
	}, nil
}

func (payload ReferenceSeriesRegistrationPayload) RegistrationCommand(
	tenant domain.TenantID,
	registrant string,
) (application.RegisterReferenceSeriesCommand, error) {
	registration, err := payload.Registration(tenant, registrant)
	if err != nil {
		return application.RegisterReferenceSeriesCommand{}, err
	}
	return application.RegisterReferenceSeriesCommand{Registration: registration}, nil
}

func (row ReferenceSeriesPeriodPayload) value() (domain.SeriesPeriodValue, error) {
	startsAt, err := time.Parse(time.RFC3339Nano, row.StartsAt)
	if err != nil {
		return domain.SeriesPeriodValue{}, fmt.Errorf("startsAt 须为 RFC 3339: %v", err)
	}
	var endsAt time.Time
	if row.EndsAt != "" {
		endsAt, err = time.Parse(time.RFC3339Nano, row.EndsAt)
		if err != nil {
			return domain.SeriesPeriodValue{}, fmt.Errorf("endsAt 须为 RFC 3339: %v", err)
		}
	}
	value, err := domain.ParseDecimal(row.Value)
	if err != nil {
		return domain.SeriesPeriodValue{}, fmt.Errorf("value: %v", err)
	}
	period, err := domain.NewSeriesPeriodValue(startsAt, endsAt, value, row.EvidenceRef)
	if err != nil {
		return domain.SeriesPeriodValue{}, err
	}
	return period, nil
}
