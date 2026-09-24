// Package registrationjson 是商业上下文服务形态、产品—渠道映射与注册号类型目录三类登记的 JSON 译装：受控批量口
// （cmd/parcel-commercial 的 register-products 与 register-registration-number-types）与操作者渠道在线口共用这一份
// 批文外壳与逐项翻译，两口对同一项译出同一条命令（票 operator-channel/04）。
//
// 分工：整批的回显与逐项事务留在受控批量口；在线口一次只收一项，租户由认证结果注入，外壳里的 tenantId 键在场即拒、
// 本口恰一项——那两道门在在线口一侧（commercialhttp），本包只给形状与翻译。
package registrationjson

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// DocumentTenant 取批文自带的整批租户，只供受控批量口：运维在库网内手跑时那是治理动作；在线口的租户格由接入渠道填，
// 不读这一格。
func DocumentTenant(raw json.RawMessage) (pcdomain.TenantID, error) {
	var tenant string
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &tenant); err != nil {
			return pcdomain.TenantID{}, fmt.Errorf("tenantId 不是字符串：%w", err)
		}
	}
	return pcdomain.NewTenantID(tenant)
}

// ProductBatchDocument 是 register-products 的批文外壳。TenantID 用 json.RawMessage：受控批量口经 DocumentTenant 取整批
// 租户，在线口见键在场即拒（`"tenantId": null` 也是自报）——同一份形状两种用法，形状只在这一处定义。
type ProductBatchDocument struct {
	TenantID json.RawMessage                 `json:"tenantId"`
	Scope    string                          `json:"scope"`
	Forms    []ServiceProductFormDocument    `json:"forms,omitempty"`
	Mappings []ProductChannelMappingDocument `json:"mappings,omitempty"`
}

type ServiceProductFormDocument struct {
	ProductID string `json:"productId"`
	Version   string `json:"version"`
	Form      string `json:"form"`
}

type ProductChannelMappingDocument struct {
	MappingID         string     `json:"mappingId"`
	Revision          int        `json:"revision"`
	ProductID         string     `json:"productId"`
	ProductVersion    string     `json:"productVersion"`
	Channels          []string   `json:"channels"`
	Basis             string     `json:"basis"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
}

// ServiceProductFormCommand 把批文里的一项服务形态译成登记命令。
func ServiceProductFormCommand(
	tenant pcdomain.TenantID,
	scope pcdomain.CommercialScopeReference,
	item ServiceProductFormDocument,
) (pcapplication.RegisterServiceProductFormCommand, error) {
	objectID, err := pcdomain.NewCommercialObjectID(item.ProductID)
	if err != nil {
		return pcapplication.RegisterServiceProductFormCommand{}, err
	}
	version, err := pcdomain.NewCommercialVersionLabel(item.Version)
	if err != nil {
		return pcapplication.RegisterServiceProductFormCommand{}, err
	}
	form, err := serviceProductFormFromName(item.Form)
	if err != nil {
		return pcapplication.RegisterServiceProductFormCommand{}, err
	}
	command := pcapplication.RegisterServiceProductFormCommand{
		Tenant:   tenant,
		Scope:    scope,
		ObjectID: objectID,
		Version:  version,
		Form:     form,
	}
	return command, nil
}

// ProductChannelMappingCommand 把批文里的一项产品—渠道映射修订译成登记命令。
func ProductChannelMappingCommand(
	tenant pcdomain.TenantID,
	scope pcdomain.CommercialScopeReference,
	item ProductChannelMappingDocument,
) (pcapplication.RegisterProductChannelMappingCommand, error) {
	id, err := pcdomain.NewProductChannelMappingID(item.MappingID)
	if err != nil {
		return pcapplication.RegisterProductChannelMappingCommand{}, err
	}
	product, err := pcdomain.NewCommercialObjectID(item.ProductID)
	if err != nil {
		return pcapplication.RegisterProductChannelMappingCommand{}, err
	}
	productVersion, err := pcdomain.NewCommercialVersionLabel(item.ProductVersion)
	if err != nil {
		return pcapplication.RegisterProductChannelMappingCommand{}, err
	}
	// channels 缺席（或 null）是输入缺件；`[]` 是登记者说出的“未配置”声明——该产品
	// 尚无可用渠道候选（CONTEXT 渠道绑定格）。两者必须可分辨，所以这里看 nil 而非长度。
	if item.Channels == nil {
		return pcapplication.RegisterProductChannelMappingCommand{}, fmt.Errorf("channels 缺席：要么给渠道引用，要么写 [] 显式声明未配置")
	}
	binding := pcdomain.UnconfiguredChannelBinding()
	if len(item.Channels) > 0 {
		references := make([]pcdomain.ChannelProductReference, 0, len(item.Channels))
		for _, raw := range item.Channels {
			reference, err := pcdomain.NewChannelProductReference(raw)
			if err != nil {
				return pcapplication.RegisterProductChannelMappingCommand{}, err
			}
			references = append(references, reference)
		}
		binding, err = pcdomain.NewConfiguredChannelBinding(references)
		if err != nil {
			return pcapplication.RegisterProductChannelMappingCommand{}, err
		}
	}
	basis, err := pcdomain.NewMappingBasisReference(item.Basis)
	if err != nil {
		return pcapplication.RegisterProductChannelMappingCommand{}, err
	}
	endsAt := time.Time{}
	if item.EffectiveEndsAt != nil {
		endsAt = *item.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(item.EffectiveStartsAt, endsAt)
	if err != nil {
		return pcapplication.RegisterProductChannelMappingCommand{}, err
	}
	command := pcapplication.RegisterProductChannelMappingCommand{
		Tenant:   tenant,
		Scope:    scope,
		ID:       id,
		Revision: item.Revision,
		Spec: pcdomain.ProductChannelMappingSpec{
			Product:        product,
			ProductVersion: productVersion,
			Binding:        binding,
			Effective:      interval,
			Basis:          basis,
		},
	}
	return command, nil
}

// serviceProductFormFromName 是 domain.ServiceProductForm 封闭集的名称镜像；集合外取值
// 拒收不吸收。两格自 ADR-0088 起并列（面单渠道服务由 PAR-COM-12 的范围裁剪改为纳入）。
func serviceProductFormFromName(raw string) (pcdomain.ServiceProductForm, error) {
	switch raw {
	case pcdomain.NetworkServiceForm.String():
		return pcdomain.NetworkServiceForm, nil
	case pcdomain.LabelChannelServiceForm.String():
		return pcdomain.LabelChannelServiceForm, nil
	}
	return pcdomain.ServiceProductFormInvalid, fmt.Errorf("未知服务形态 %q", raw)
}

// registrationNumberTypeReferenceDirectory 是注册号类型目录的参考配置标识前缀；标识末段（键）是注册
// 国家 / 地区，一份参考配置只收该国家 / 地区的类型。
const registrationNumberTypeReferenceDirectory = "party-commercial/registration-number-types/"

// RegistrationNumberTypeBatchDocument 是 register-registration-number-types 的批文外壳，TenantID 的两种用法同
// ProductBatchDocument。
type RegistrationNumberTypeBatchDocument struct {
	TenantID      json.RawMessage                              `json:"tenantId"`
	Types         []RegistrationNumberTypeDocument             `json:"types,omitempty"`
	Deactivations []RegistrationNumberTypeDeactivationDocument `json:"deactivations,omitempty"`
}

type RegistrationNumberTypeDocument struct {
	CountryCode   string    `json:"countryCode"`
	TypeCode      string    `json:"typeCode"`
	Revision      int       `json:"revision"`
	Name          string    `json:"name"`
	Layer         string    `json:"layer"`
	Format        string    `json:"format"`
	Basis         string    `json:"basis"`
	Adopt         string    `json:"adopt,omitempty"`
	EffectiveFrom time.Time `json:"effectiveFrom"`
}

type registrationNumberTypeReferenceDocument struct {
	Identifier string                                        `json:"identifier"`
	Version    int                                           `json:"version"`
	Title      string                                        `json:"title"`
	Source     string                                        `json:"source"`
	Note       string                                        `json:"note"`
	Types      []registrationNumberTypeReferenceItemDocument `json:"types"`
}

type registrationNumberTypeReferenceItemDocument struct {
	CountryCode string   `json:"countryCode"`
	TypeCode    string   `json:"typeCode"`
	Name        string   `json:"name"`
	Layer       string   `json:"layer"`
	Format      string   `json:"format"`
	Samples     []string `json:"samples"`
}

// referencedRegistrationNumberType 是一份参考配置里已过领域构造门的一个类型；依据格留空，由采用路径
// 写成引用串。样例只供发布前的测试拿来过判断方法，不进任何登记。
type referencedRegistrationNumberType struct {
	country pcdomain.RegistrationCountryCode
	code    pcdomain.RegistrationNumberTypeCode
	spec    pcdomain.RegistrationNumberTypeSpec
	samples []pcdomain.RegistrationNumber
}

func registrationNumberTypesFromReference(reference referenceconfig.Reference) ([]referencedRegistrationNumberType, error) {
	key, isRegistrationNumberTypes := strings.CutPrefix(reference.Identifier(), registrationNumberTypeReferenceDirectory)
	if !isRegistrationNumberTypes {
		return nil, fmt.Errorf("%s 不是注册号类型目录的参考配置", reference)
	}
	raw, err := referenceconfig.Open(reference)
	if err != nil {
		return nil, err
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registrationNumberTypeReferenceDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("参考配置 %s 不是注册号类型目录的形状：%w", reference, err)
	}
	if len(document.Types) == 0 {
		return nil, fmt.Errorf("参考配置 %s 没有任何类型", reference)
	}
	types := make([]referencedRegistrationNumberType, 0, len(document.Types))
	for _, item := range document.Types {
		if item.CountryCode != key {
			return nil, fmt.Errorf("参考配置 %s 的类型 %s 属 %q，不属本份的键 %q", reference, item.TypeCode, item.CountryCode, key)
		}
		country, err := pcdomain.NewRegistrationCountryCode(item.CountryCode)
		if err != nil {
			return nil, err
		}
		code, err := pcdomain.NewRegistrationNumberTypeCode(item.TypeCode)
		if err != nil {
			return nil, err
		}
		spec, err := registrationNumberTypeContentFrom(item.Name, item.Layer, item.Format)
		if err != nil {
			return nil, fmt.Errorf("参考配置 %s 的类型 %s：%w", reference, item.TypeCode, err)
		}
		samples := make([]pcdomain.RegistrationNumber, 0, len(item.Samples))
		for _, text := range item.Samples {
			sample, err := pcdomain.NewRegistrationNumber(text)
			if err != nil {
				return nil, err
			}
			samples = append(samples, sample)
		}
		types = append(types, referencedRegistrationNumberType{country: country, code: code, spec: spec, samples: samples})
	}
	return types, nil
}

// registrationNumberTypeContentFrom 翻译一个类型的名称、层与格式，批文与参考配置两条路共用这一段。
func registrationNumberTypeContentFrom(name, layer, format string) (pcdomain.RegistrationNumberTypeSpec, error) {
	typeName, err := pcdomain.NewRegistrationNumberTypeName(name)
	if err != nil {
		return pcdomain.RegistrationNumberTypeSpec{}, err
	}
	typeLayer, known := pcdomain.RegistrationNumberLayerNamed(layer)
	if !known {
		return pcdomain.RegistrationNumberTypeSpec{}, fmt.Errorf("未知层 %q：只收 %s 或 %s", layer,
			pcdomain.RegistrationNumberIdentityLayer, pcdomain.RegistrationNumberProfileLayer)
	}
	typeFormat, err := pcdomain.NewRegistrationNumberFormat(format)
	if err != nil {
		return pcdomain.RegistrationNumberTypeSpec{}, err
	}
	return pcdomain.RegistrationNumberTypeSpec{Name: typeName, Layer: typeLayer, Format: typeFormat}, nil
}

// adoptedRegistrationNumberTypeSpec 按批文点名的参考配置版本形成一项采用的正文，依据格写成引用串。
func adoptedRegistrationNumberTypeSpec(
	item RegistrationNumberTypeDocument,
	country pcdomain.RegistrationCountryCode,
	code pcdomain.RegistrationNumberTypeCode,
) (pcdomain.RegistrationNumberTypeSpec, referenceconfig.Reference, error) {
	if item.Name != "" || item.Layer != "" || item.Format != "" || item.Basis != "" {
		return pcdomain.RegistrationNumberTypeSpec{}, referenceconfig.Reference{},
			fmt.Errorf("采用项的名称、层、格式与依据取自参考配置，批文不另写")
	}
	reference, err := referenceconfig.ParseReference(item.Adopt)
	if err != nil {
		return pcdomain.RegistrationNumberTypeSpec{}, referenceconfig.Reference{}, err
	}
	types, err := registrationNumberTypesFromReference(reference)
	if err != nil {
		return pcdomain.RegistrationNumberTypeSpec{}, referenceconfig.Reference{}, err
	}
	for _, numberType := range types {
		if numberType.country != country || numberType.code != code {
			continue
		}
		basis, err := pcdomain.NewRegistrationNumberTypeBasisReference(reference.Citation())
		if err != nil {
			return pcdomain.RegistrationNumberTypeSpec{}, referenceconfig.Reference{}, err
		}
		spec := numberType.spec
		spec.Basis = basis
		return spec, reference, nil
	}
	return pcdomain.RegistrationNumberTypeSpec{}, referenceconfig.Reference{},
		fmt.Errorf("参考配置 %s 里没有类型 %s/%s", reference, item.CountryCode, item.TypeCode)
}

type RegistrationNumberTypeDeactivationDocument struct {
	CountryCode string    `json:"countryCode"`
	TypeCode    string    `json:"typeCode"`
	Revision    int       `json:"revision"`
	Basis       string    `json:"basis"`
	At          time.Time `json:"at"`
}

// RegistrationNumberTypeCommand 把批文里的一项注册号类型译成登记命令。带 adopt 的一项交回它采用的那一版参考配置
// （受控批量口拿去回显）；不带的交回零值。
func RegistrationNumberTypeCommand(
	tenant pcdomain.TenantID,
	item RegistrationNumberTypeDocument,
) (pcapplication.RegisterRegistrationNumberTypeCommand, referenceconfig.Reference, error) {
	none := pcapplication.RegisterRegistrationNumberTypeCommand{}
	country, err := pcdomain.NewRegistrationCountryCode(item.CountryCode)
	if err != nil {
		return none, referenceconfig.Reference{}, err
	}
	code, err := pcdomain.NewRegistrationNumberTypeCode(item.TypeCode)
	if err != nil {
		return none, referenceconfig.Reference{}, err
	}
	var adoptedFrom referenceconfig.Reference
	var spec pcdomain.RegistrationNumberTypeSpec
	if item.Adopt != "" {
		adopted, reference, err := adoptedRegistrationNumberTypeSpec(item, country, code)
		if err != nil {
			return none, referenceconfig.Reference{}, err
		}
		spec = adopted
		adoptedFrom = reference
	} else {
		content, err := registrationNumberTypeContentFrom(item.Name, item.Layer, item.Format)
		if err != nil {
			return none, referenceconfig.Reference{}, err
		}
		basis, err := pcdomain.NewRegistrationNumberTypeBasisReference(item.Basis)
		if err != nil {
			return none, referenceconfig.Reference{}, err
		}
		spec = content
		spec.Basis = basis
	}
	if item.EffectiveFrom.IsZero() {
		return none, referenceconfig.Reference{}, fmt.Errorf("effectiveFrom 缺席：生效时点不代填")
	}
	command := pcapplication.RegisterRegistrationNumberTypeCommand{
		Tenant:        tenant,
		Country:       country,
		Code:          code,
		Revision:      item.Revision,
		Spec:          spec,
		EffectiveFrom: item.EffectiveFrom,
	}
	return command, adoptedFrom, nil
}

// RegistrationNumberTypeDeactivationCommand 把批文里的一项停用译成停用命令。
func RegistrationNumberTypeDeactivationCommand(
	tenant pcdomain.TenantID,
	item RegistrationNumberTypeDeactivationDocument,
) (pcapplication.DeactivateRegistrationNumberTypeCommand, error) {
	country, err := pcdomain.NewRegistrationCountryCode(item.CountryCode)
	if err != nil {
		return pcapplication.DeactivateRegistrationNumberTypeCommand{}, err
	}
	code, err := pcdomain.NewRegistrationNumberTypeCode(item.TypeCode)
	if err != nil {
		return pcapplication.DeactivateRegistrationNumberTypeCommand{}, err
	}
	basis, err := pcdomain.NewRegistrationNumberTypeBasisReference(item.Basis)
	if err != nil {
		return pcapplication.DeactivateRegistrationNumberTypeCommand{}, err
	}
	if item.At.IsZero() {
		return pcapplication.DeactivateRegistrationNumberTypeCommand{}, fmt.Errorf("at 缺席：停用时点不代填")
	}
	command := pcapplication.DeactivateRegistrationNumberTypeCommand{
		Tenant:   tenant,
		Country:  country,
		Code:     code,
		Revision: item.Revision,
		Basis:    basis,
		At:       item.At,
	}
	return command, nil
}
