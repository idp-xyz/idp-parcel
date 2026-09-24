package main

// 本文件承载服务形态与产品—渠道映射的受控登记入口（票 admin-remainder-mechanism-
// batch/02）：register-products 子命令。登记是操作者动作，走本 CLI 不占 parcel-api
// 端点面（判据同 publish 的文件注释）。
//
// 批内各项独立成败、逐项各起事务（AT-PC-011 同款纪律）：文件内 forms → mappings 的
// 次序只是回显习惯——形态不是映射的门（ADR-0050：形态缺席不使解析退化），两组各自
// 对着版本册做引用检查。某项被拒不撤已落的前项，重跑同一批已落项以重放回答。

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

// productBatchCommand 是翻译产物里的一项：标签供回显，执行闭包对着处理器跑。
type productBatchCommand struct {
	label   string
	execute func(context.Context, *pcapplication.RegisterProductChannelHandler) (pcapplication.ProductChannelResult, error)
}

func productBatchFromJSON(raw []byte) ([]productBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registrationjson.ProductBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("产品登记批不是本入口的形状：%w", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
	if err != nil {
		return nil, err
	}
	scope, err := pcdomain.NewCommercialScopeReference(document.Scope)
	if err != nil {
		return nil, err
	}
	if len(document.Forms)+len(document.Mappings) == 0 {
		return nil, fmt.Errorf("产品登记批没有任何项")
	}

	commands := make([]productBatchCommand, 0, len(document.Forms)+len(document.Mappings))
	for _, item := range document.Forms {
		command, err := serviceProductFormCommandFrom(tenant, scope, item)
		if err != nil {
			return nil, fmt.Errorf("forms/%s：%w", item.ProductID, err)
		}
		commands = append(commands, command)
	}
	for _, item := range document.Mappings {
		command, err := productChannelMappingCommandFrom(tenant, scope, item)
		if err != nil {
			return nil, fmt.Errorf("mappings/%s：%w", item.MappingID, err)
		}
		commands = append(commands, command)
	}
	return commands, nil
}

func serviceProductFormCommandFrom(
	tenant pcdomain.TenantID,
	scope pcdomain.CommercialScopeReference,
	item registrationjson.ServiceProductFormDocument,
) (productBatchCommand, error) {
	command, err := registrationjson.ServiceProductFormCommand(tenant, scope, item)
	if err != nil {
		return productBatchCommand{}, err
	}
	return productBatchCommand{
		label: fmt.Sprintf("服务形态 %s/%s %s", item.ProductID, item.Version, item.Form),
		execute: func(ctx context.Context, handler *pcapplication.RegisterProductChannelHandler) (pcapplication.ProductChannelResult, error) {
			return handler.RegisterServiceProductForm(ctx, command)
		},
	}, nil
}

func productChannelMappingCommandFrom(
	tenant pcdomain.TenantID,
	scope pcdomain.CommercialScopeReference,
	item registrationjson.ProductChannelMappingDocument,
) (productBatchCommand, error) {
	command, err := registrationjson.ProductChannelMappingCommand(tenant, scope, item)
	if err != nil {
		return productBatchCommand{}, err
	}
	return productBatchCommand{
		label: fmt.Sprintf("产品—渠道映射 %s r%d", item.MappingID, item.Revision),
		execute: func(ctx context.Context, handler *pcapplication.RegisterProductChannelHandler) (pcapplication.ProductChannelResult, error) {
			return handler.RegisterMapping(ctx, command)
		},
	}, nil
}

func runRegisterProducts(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-products", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "产品登记批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-products 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读登记批：%v\n", err)
		return exitTechnical
	}
	commands, err := productBatchFromJSON(raw)
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

	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造发布登记册：%v\n", err)
		return exitTechnical
	}
	mappings, err := pcpostgres.NewProductChannelMappings(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造映射登记册：%v\n", err)
		return exitTechnical
	}
	handler := pcapplication.NewRegisterProductChannelHandler(publications, mappings)

	attention := false
	for index, command := range commands {
		var result pcapplication.ProductChannelResult
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
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), productCauseDetail(result))
		switch result.Outcome() {
		case pcapplication.ProductChannelContentConflict,
			pcapplication.ProductChannelNotAccepted:
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

func productCauseDetail(result pcapplication.ProductChannelResult) string {
	if cause := result.Cause(); cause != nil {
		return fmt.Sprintf("（原因：%v）", cause)
	}
	return ""
}
