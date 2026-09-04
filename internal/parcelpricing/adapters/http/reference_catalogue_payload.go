package pricinghttp

import (
	"encoding/json"
	"fmt"
	"io"
	"time"

	"go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	"go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
)

// 本文件是运营操作者面的目录登记载荷（ADR-0109 Decision 二；写面按 ADR-0101 决定八自裁）：分区表与偏远
// 档位表是万行级映射，写面是**模板导入**——一份 JSON 带整张表，不是逐字段表单。与序列载荷同一条纪律：
// 载荷里只有内容没有身份，租户与登记责任方从操作者信封来；未知键拒；每一格过领域构造器，这里不另造校验。

// ReferenceCataloguePayload 是载荷的线格式，字段与 domain.ReferenceCatalogueRegistrationSpec 一一对应，
// 只少身份两格。
type ReferenceCataloguePayload struct {
	CatalogueID      string                               `json:"catalogueId"`
	CatalogueVersion string                               `json:"catalogueVersion"`
	Kind             string                               `json:"kind"`
	SourceIdentifier string                               `json:"sourceIdentifier"`
	Origin           ReferenceCatalogueOriginPayload      `json:"origin"`
	PrefixLength     int                                  `json:"prefixLength"`
	Entries          []ReferenceCatalogueEntryPayload     `json:"entries"`
	EffectiveFrom    string                               `json:"effectiveFrom"`
	EffectiveTo      string                               `json:"effectiveTo,omitempty"`
	Correction       *ReferenceCatalogueCorrectionPayload `json:"correction,omitempty"`
}

// ReferenceCatalogueOriginPayload 声明始发维度：INDEPENDENT 不带前缀集，POSTAL_PREFIXES 带非空前缀集
// （由领域构造门判）。
type ReferenceCatalogueOriginPayload struct {
	Scope    string   `json:"scope"`
	Prefixes []string `json:"prefixes,omitempty"`
}

// ReferenceCatalogueEntryPayload 是映射的一行：目的邮编前缀 → 类别值。
type ReferenceCatalogueEntryPayload struct {
	Prefix string `json:"prefix"`
	Value  string `json:"value"`
}

// ReferenceCatalogueCorrectionPayload 声明更正关系，形照序列载荷。
type ReferenceCatalogueCorrectionPayload struct {
	PriorVersion     string `json:"priorVersion"`
	PriorFingerprint string `json:"priorFingerprint,omitempty"`
	Basis            string `json:"basis"`
}

// DecodeReferenceCataloguePayload 只做结构解码：JSON 合法、键都认识。
func DecodeReferenceCataloguePayload(body io.Reader) (ReferenceCataloguePayload, error) {
	decoder := json.NewDecoder(body)
	decoder.DisallowUnknownFields()
	var payload ReferenceCataloguePayload
	if err := decoder.Decode(&payload); err != nil {
		return ReferenceCataloguePayload{}, fmt.Errorf("%w: reference catalogue payload: %v", ErrMalformedRequest, err)
	}
	return payload, nil
}

// Registration 把载荷连同信封给的身份翻成领域登记对象。拒了就是 ErrMalformedRequest——同一份内容重发不会变好。
func (payload ReferenceCataloguePayload) Registration(
	tenant domain.TenantID,
	registrant string,
) (domain.ReferenceCatalogueRegistration, error) {
	if tenant.String() == "" || registrant == "" {
		return domain.ReferenceCatalogueRegistration{}, ErrOperatorIdentityMissing
	}
	reference, err := domain.NewVersionReferenceIdentity(domain.ArtifactReferenceCatalogue, payload.CatalogueID, payload.CatalogueVersion)
	if err != nil {
		return domain.ReferenceCatalogueRegistration{}, fmt.Errorf("%w: catalogue reference: %v", ErrMalformedRequest, err)
	}
	origin, err := payload.Origin.origin()
	if err != nil {
		return domain.ReferenceCatalogueRegistration{}, fmt.Errorf("%w: origin: %v", ErrMalformedRequest, err)
	}
	period, err := payload.period()
	if err != nil {
		return domain.ReferenceCatalogueRegistration{}, fmt.Errorf("%w: %v", ErrMalformedRequest, err)
	}
	spec := domain.ReferenceCatalogueRegistrationSpec{
		Tenant:           tenant,
		Kind:             domain.CatalogueKind(payload.Kind),
		Reference:        reference,
		SourceIdentifier: payload.SourceIdentifier,
		Registrant:       registrant,
		Origin:           origin,
		PrefixLength:     payload.PrefixLength,
		Period:           period,
		Entries:          make([]domain.CatalogueEntry, 0, len(payload.Entries)),
	}
	for index, row := range payload.Entries {
		entry, err := domain.NewCatalogueEntry(row.Prefix, domain.CategoryValue(row.Value))
		if err != nil {
			return domain.ReferenceCatalogueRegistration{}, fmt.Errorf("%w: entries[%d]: %v", ErrMalformedRequest, index, err)
		}
		spec.Entries = append(spec.Entries, entry)
	}
	if payload.Correction != nil {
		prior, err := domain.NewVersionReferenceWithFingerprint(
			domain.ArtifactReferenceCatalogue, payload.CatalogueID, payload.Correction.PriorVersion, payload.Correction.PriorFingerprint)
		if err != nil {
			return domain.ReferenceCatalogueRegistration{}, fmt.Errorf("%w: correction prior version: %v", ErrMalformedRequest, err)
		}
		spec.PriorVersion = prior
		spec.CorrectionBasis = payload.Correction.Basis
	}
	registration, err := domain.NewReferenceCatalogueRegistration(spec)
	if err != nil {
		return domain.ReferenceCatalogueRegistration{}, fmt.Errorf("%w: registration: %v", ErrMalformedRequest, err)
	}
	return registration, nil
}

// RegistrationCommand 把载荷翻成登记命令。
func (payload ReferenceCataloguePayload) RegistrationCommand(
	tenant domain.TenantID,
	registrant string,
) (application.RegisterReferenceCatalogueCommand, error) {
	registration, err := payload.Registration(tenant, registrant)
	if err != nil {
		return application.RegisterReferenceCatalogueCommand{}, err
	}
	return application.RegisterReferenceCatalogueCommand{Registration: registration}, nil
}

func (payload ReferenceCataloguePayload) period() (domain.EffectivePeriod, error) {
	from, err := time.Parse(time.RFC3339Nano, payload.EffectiveFrom)
	if err != nil {
		return domain.EffectivePeriod{}, fmt.Errorf("effectiveFrom 须为 RFC 3339: %v", err)
	}
	var to time.Time
	if payload.EffectiveTo != "" {
		to, err = time.Parse(time.RFC3339Nano, payload.EffectiveTo)
		if err != nil {
			return domain.EffectivePeriod{}, fmt.Errorf("effectiveTo 须为 RFC 3339: %v", err)
		}
	}
	period, err := domain.NewEffectivePeriod(from, to)
	if err != nil {
		return domain.EffectivePeriod{}, fmt.Errorf("effective period: %v", err)
	}
	return period, nil
}

func (origin ReferenceCatalogueOriginPayload) origin() (domain.CatalogueOrigin, error) {
	switch domain.CatalogueOriginScope(origin.Scope) {
	case domain.CatalogueOriginIndependent:
		if len(origin.Prefixes) != 0 {
			return domain.CatalogueOrigin{}, fmt.Errorf("INDEPENDENT origin carries prefixes")
		}
		return domain.NewIndependentCatalogueOrigin(), nil
	case domain.CatalogueOriginPostalPrefixes:
		return domain.NewPostalPrefixCatalogueOrigin(origin.Prefixes)
	default:
		return domain.CatalogueOrigin{}, fmt.Errorf("origin scope %q is outside the closed set", origin.Scope)
	}
}
