package main

// 本文件承载注册号类型目录的受控登记入口（票 legal-entity-profile/01）：register-registration-
// number-types 子命令。登记是操作者动作，走本 CLI 不占 parcel-api 端点面（判据同 publish 的文件
// 注释）。
//
// 批内各项独立成败、逐项各起事务（AT-PC-011 同款纪律）：文件内 types → deactivations 的次序只是
// 回显习惯，某项被拒不撤已落的前项，重跑同一批已落项以重放回答。批文里的国家 / 地区、层与格式
// 一格不代填：目录内容是实施时登记的配置，产品不带任何国家 / 地区的生产默认条目（ADR-0145 决定一）。
//
// 带 `adopt` 的一项是采用参考配置（ADR-0147 决定四）：名称、层与格式取自批文点名的那一版参考配置，
// 依据格由本入口写成引用串，批文再写这四格即拒收；修订号与生效时点照旧由批文给。这不是默认——
// 批文不点名，参考配置就不进任何租户的目录。两条路的名称、层与格式走同一段翻译，参考配置因此过的是
// 与批文同一套领域构造门。

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/referenceconfig"
)

// registrationNumberTypeReferenceDirectory 是注册号类型目录的参考配置标识前缀；标识末段（键）是注册
// 国家 / 地区，一份参考配置只收该国家 / 地区的类型。
const registrationNumberTypeReferenceDirectory = "party-commercial/registration-number-types/"

type registrationNumberTypeBatchDocument struct {
	TenantID      string                                       `json:"tenantId"`
	Types         []registrationNumberTypeItemDocument         `json:"types,omitempty"`
	Deactivations []registrationNumberTypeDeactivationDocument `json:"deactivations,omitempty"`
}

type registrationNumberTypeItemDocument struct {
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
	item registrationNumberTypeItemDocument,
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

type registrationNumberTypeDeactivationDocument struct {
	CountryCode string    `json:"countryCode"`
	TypeCode    string    `json:"typeCode"`
	Revision    int       `json:"revision"`
	Basis       string    `json:"basis"`
	At          time.Time `json:"at"`
}

// registrationNumberTypeBatchCommand 是翻译产物里的一项：标签供回显，执行闭包对着处理器跑。
type registrationNumberTypeBatchCommand struct {
	label   string
	execute func(context.Context, *pcapplication.RegisterRegistrationNumberTypeHandler) (pcapplication.RegistrationNumberTypeResult, error)
}

func registrationNumberTypeBatchFromJSON(raw []byte) ([]registrationNumberTypeBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registrationNumberTypeBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("注册号类型登记批不是本入口的形状：%w", err)
	}
	tenant, err := pcdomain.NewTenantID(document.TenantID)
	if err != nil {
		return nil, err
	}
	if len(document.Types)+len(document.Deactivations) == 0 {
		return nil, fmt.Errorf("注册号类型登记批没有任何项")
	}

	commands := make([]registrationNumberTypeBatchCommand, 0, len(document.Types)+len(document.Deactivations))
	for _, item := range document.Types {
		command, err := registrationNumberTypeCommandFrom(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("types/%s/%s：%w", item.CountryCode, item.TypeCode, err)
		}
		commands = append(commands, command)
	}
	for _, item := range document.Deactivations {
		command, err := registrationNumberTypeDeactivationFrom(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("deactivations/%s/%s：%w", item.CountryCode, item.TypeCode, err)
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func registrationNumberTypeCommandFrom(
	tenant pcdomain.TenantID,
	item registrationNumberTypeItemDocument,
) (registrationNumberTypeBatchCommand, error) {
	none := registrationNumberTypeBatchCommand{}
	country, err := pcdomain.NewRegistrationCountryCode(item.CountryCode)
	if err != nil {
		return none, err
	}
	code, err := pcdomain.NewRegistrationNumberTypeCode(item.TypeCode)
	if err != nil {
		return none, err
	}
	label := fmt.Sprintf("注册号类型 %s/%s r%d", item.CountryCode, item.TypeCode, item.Revision)
	var spec pcdomain.RegistrationNumberTypeSpec
	if item.Adopt != "" {
		adopted, reference, err := adoptedRegistrationNumberTypeSpec(item, country, code)
		if err != nil {
			return none, err
		}
		spec = adopted
		label += fmt.Sprintf("（采用 %s）", reference)
	} else {
		content, err := registrationNumberTypeContentFrom(item.Name, item.Layer, item.Format)
		if err != nil {
			return none, err
		}
		basis, err := pcdomain.NewRegistrationNumberTypeBasisReference(item.Basis)
		if err != nil {
			return none, err
		}
		spec = content
		spec.Basis = basis
	}
	if item.EffectiveFrom.IsZero() {
		return none, fmt.Errorf("effectiveFrom 缺席：生效时点不代填")
	}
	command := pcapplication.RegisterRegistrationNumberTypeCommand{
		Tenant:        tenant,
		Country:       country,
		Code:          code,
		Revision:      item.Revision,
		Spec:          spec,
		EffectiveFrom: item.EffectiveFrom,
	}
	return registrationNumberTypeBatchCommand{
		label: label,
		execute: func(ctx context.Context, handler *pcapplication.RegisterRegistrationNumberTypeHandler) (pcapplication.RegistrationNumberTypeResult, error) {
			return handler.Register(ctx, command)
		},
	}, nil
}

func registrationNumberTypeDeactivationFrom(
	tenant pcdomain.TenantID,
	item registrationNumberTypeDeactivationDocument,
) (registrationNumberTypeBatchCommand, error) {
	none := registrationNumberTypeBatchCommand{}
	country, err := pcdomain.NewRegistrationCountryCode(item.CountryCode)
	if err != nil {
		return none, err
	}
	code, err := pcdomain.NewRegistrationNumberTypeCode(item.TypeCode)
	if err != nil {
		return none, err
	}
	basis, err := pcdomain.NewRegistrationNumberTypeBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	if item.At.IsZero() {
		return none, fmt.Errorf("at 缺席：停用时点不代填")
	}
	command := pcapplication.DeactivateRegistrationNumberTypeCommand{
		Tenant:   tenant,
		Country:  country,
		Code:     code,
		Revision: item.Revision,
		Basis:    basis,
		At:       item.At,
	}
	return registrationNumberTypeBatchCommand{
		label: fmt.Sprintf("停用注册号类型 %s/%s r%d", item.CountryCode, item.TypeCode, item.Revision),
		execute: func(ctx context.Context, handler *pcapplication.RegisterRegistrationNumberTypeHandler) (pcapplication.RegistrationNumberTypeResult, error) {
			return handler.Deactivate(ctx, command)
		},
	}, nil
}

func runRegisterRegistrationNumberTypes(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-registration-number-types", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "注册号类型登记批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-registration-number-types 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读登记批：%v\n", err)
		return exitTechnical
	}
	commands, err := registrationNumberTypeBatchFromJSON(raw)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}

	db, cleanup, err := openDatabase(ctx, getenv)
	if err != nil {
		fmt.Fprintf(errOut, "%v\n", err)
		return exitTechnical
	}
	defer cleanup()

	registry, err := pcpostgres.NewRegistrationNumberTypes(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造注册号类型目录：%v\n", err)
		return exitTechnical
	}
	handler := pcapplication.NewRegisterRegistrationNumberTypeHandler(registry)

	attention := false
	for index, command := range commands {
		var result pcapplication.RegistrationNumberTypeResult
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = command.execute(txCtx, handler)
			return handleErr
		})
		label := fmt.Sprintf("第 %d/%d 项 %s", index+1, len(commands), command.label)
		if err != nil {
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), registrationNumberTypeCauseDetail(result))
		switch result.Outcome() {
		case pcapplication.RegistrationNumberTypeContentConflict,
			pcapplication.RegistrationNumberTypeNotAccepted,
			pcapplication.RegistrationNumberTypeNotFound:
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

func registrationNumberTypeCauseDetail(result pcapplication.RegistrationNumberTypeResult) string {
	if cause := result.Cause(); cause != nil {
		return fmt.Sprintf("（原因：%v）", cause)
	}
	return ""
}
