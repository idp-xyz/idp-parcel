package main

// 本文件承载注册号类型目录的受控登记入口（票 legal-entity-profile/01）：register-registration-
// number-types 子命令。登记是操作者动作，走本 CLI 不占 parcel-api 端点面（判据同 publish 的文件
// 注释）。
//
// 批内各项独立成败、逐项各起事务（AT-PC-011 同款纪律）：文件内 types → deactivations 的次序只是
// 回显习惯，某项被拒不撤已落的前项，重跑同一批已落项以重放回答。批文里的国家 / 地区、层与格式
// 一格不代填：目录内容是实施时登记的配置，产品不带任何国家 / 地区的生产默认条目（ADR-0145 决定一）。

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"time"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

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
	EffectiveFrom time.Time `json:"effectiveFrom"`
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
	name, err := pcdomain.NewRegistrationNumberTypeName(item.Name)
	if err != nil {
		return none, err
	}
	layer, known := pcdomain.RegistrationNumberLayerNamed(item.Layer)
	if !known {
		return none, fmt.Errorf("未知层 %q：只收 %s 或 %s", item.Layer,
			pcdomain.RegistrationNumberIdentityLayer, pcdomain.RegistrationNumberProfileLayer)
	}
	format, err := pcdomain.NewRegistrationNumberFormat(item.Format)
	if err != nil {
		return none, err
	}
	basis, err := pcdomain.NewRegistrationNumberTypeBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	if item.EffectiveFrom.IsZero() {
		return none, fmt.Errorf("effectiveFrom 缺席：生效时点不代填")
	}
	command := pcapplication.RegisterRegistrationNumberTypeCommand{
		Tenant:   tenant,
		Country:  country,
		Code:     code,
		Revision: item.Revision,
		Spec: pcdomain.RegistrationNumberTypeSpec{
			Name:   name,
			Layer:  layer,
			Format: format,
			Basis:  basis,
		},
		EffectiveFrom: item.EffectiveFrom,
	}
	return registrationNumberTypeBatchCommand{
		label: fmt.Sprintf("注册号类型 %s/%s r%d", item.CountryCode, item.TypeCode, item.Revision),
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
