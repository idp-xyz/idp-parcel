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

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/adapters/registrationjson"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

// registrationNumberTypeBatchCommand 是翻译产物里的一项：标签供回显，执行闭包对着处理器跑。
type registrationNumberTypeBatchCommand struct {
	label   string
	execute func(context.Context, *pcapplication.RegisterRegistrationNumberTypeHandler) (pcapplication.RegistrationNumberTypeResult, error)
}

func registrationNumberTypeBatchFromJSON(raw []byte) ([]registrationNumberTypeBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registrationjson.RegistrationNumberTypeBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("注册号类型登记批不是本入口的形状：%w", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
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
	item registrationjson.RegistrationNumberTypeDocument,
) (registrationNumberTypeBatchCommand, error) {
	command, adoptedFrom, err := registrationjson.RegistrationNumberTypeCommand(tenant, item)
	if err != nil {
		return registrationNumberTypeBatchCommand{}, err
	}
	label := fmt.Sprintf("注册号类型 %s/%s r%d", item.CountryCode, item.TypeCode, item.Revision)
	if item.Adopt != "" {
		label += fmt.Sprintf("（采用 %s）", adoptedFrom)
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
	item registrationjson.RegistrationNumberTypeDeactivationDocument,
) (registrationNumberTypeBatchCommand, error) {
	command, err := registrationjson.RegistrationNumberTypeDeactivationCommand(tenant, item)
	if err != nil {
		return registrationNumberTypeBatchCommand{}, err
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
