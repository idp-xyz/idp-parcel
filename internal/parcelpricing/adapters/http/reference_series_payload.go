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
// **三处 domain.VersionReference 各归其位，本包不铸任何摘要或令牌**（ADR-0108 Decision 五）：
// 版本引用的身份是三元，指纹是「声明时手上有什么」的可选痕迹。自身引用只带三元——登记的内容摘要
// 是登记自己的字段，不再塞进自身引用；口径引用只带三元——party-commercial 的读口今天不透出
// content_digest，日后透出可补进指纹，那是可选增强不是前提；更正回指带三元加前版登记的内容摘要
// 作指纹（逐版本读面的 contentDigest，操作者手上有的就是它）。此前这里以 `declared:` 令牌填满
// 领域要求非空的 digest 槽，ADR-0108 把 digest 从身份里拿掉后令牌失去对象，随之退役；预览与登记
// 共用同一条构造（ADR-0101 决定四）因为不再铸任何东西而更简单。

// ErrOperatorIdentityMissing 表示调用方没交来租户或登记责任方——那不是载荷的错（载荷本来就不该
// 带它们），是 Intake 没拿到操作者信封就来翻译。与 ErrMalformedRequest 分开：前者改载荷没用。
var ErrOperatorIdentityMissing = errors.New("parcel pricing http: operator identity (tenant, registrant) is required")

// ReferenceSeriesRegistrationPayload 是载荷的线格式。字段与 domain.ReferenceSeriesRegistrationSpec
// 一一对应，只少身份两格；不引入领域里没有的概念。
type ReferenceSeriesRegistrationPayload struct {
	SeriesID         string                            `json:"seriesId"`
	SeriesVersion    string                            `json:"seriesVersion"`
	Kind             string                            `json:"kind"`
	SourceIdentifier string                            `json:"sourceIdentifier"`
	QuoteBasis       *ReferenceSeriesQuoteBasisPayload `json:"quoteBasis,omitempty"`
	// Currency 只对金额序列（PUBLISHED_AMOUNT，ADR-0110）声明；费率序列带它由领域构造门拒。
	Currency   string                            `json:"currency,omitempty"`
	Periods    []ReferenceSeriesPeriodPayload    `json:"periods"`
	Correction *ReferenceSeriesCorrectionPayload `json:"correction,omitempty"`
	// CompareWithVersion 只对预览有意义：指名对照版本。登记命令不带它。
	CompareWithVersion string `json:"compareWithVersion,omitempty"`
}

// ReferenceSeriesQuoteBasisPayload 指一版商业价格政策（汇率必备，燃油不得有——由领域构造门判）。
// Digest 可缺：缺则指纹留空；PC 读口透出 content_digest 之后表单带真值过来，这里照实放进指纹。
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

// ReferenceSeriesCorrectionPayload 声明更正关系：回指同序列的哪一版、凭什么。PriorFingerprint 是
// 前版登记的内容摘要（逐版本读面的 contentDigest），作回指的指纹；操作者手上没有就留空，回指只带
// 三元——不铸任何东西顶替（ADR-0108 Decision 五）。
type ReferenceSeriesCorrectionPayload struct {
	PriorVersion     string `json:"priorVersion"`
	PriorFingerprint string `json:"priorFingerprint,omitempty"`
	Basis            string `json:"basis"`
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

	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactReferenceSeries, payload.SeriesID, payload.SeriesVersion)
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
		basis, err := domain.NewVersionReferenceWithFingerprint(
			domain.ArtifactCommercialPolicy, payload.QuoteBasis.PolicyID, payload.QuoteBasis.PolicyVersion, payload.QuoteBasis.Digest)
		if err != nil {
			return domain.ReferenceSeriesRegistration{}, fmt.Errorf("%w: quote basis: %v", ErrMalformedRequest, err)
		}
		spec.QuoteBasis = basis
	}
	if payload.Currency != "" {
		currency, err := domain.NewCurrency(payload.Currency)
		if err != nil {
			return domain.ReferenceSeriesRegistration{}, fmt.Errorf("%w: currency: %v", ErrMalformedRequest, err)
		}
		spec.Currency = currency
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
		prior, err := domain.NewVersionReferenceWithFingerprint(
			domain.ArtifactReferenceSeries, payload.SeriesID, payload.Correction.PriorVersion, payload.Correction.PriorFingerprint)
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
