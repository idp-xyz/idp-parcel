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
	"time"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	pcdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
)

type productBatchDocument struct {
	TenantID string                          `json:"tenantId"`
	Scope    string                          `json:"scope"`
	Forms    []serviceProductFormDocument    `json:"forms,omitempty"`
	Mappings []productChannelMappingDocument `json:"mappings,omitempty"`
}

type serviceProductFormDocument struct {
	ProductID string `json:"productId"`
	Version   string `json:"version"`
	Form      string `json:"form"`
}

type productChannelMappingDocument struct {
	MappingID         string     `json:"mappingId"`
	Revision          int        `json:"revision"`
	ProductID         string     `json:"productId"`
	ProductVersion    string     `json:"productVersion"`
	Channels          []string   `json:"channels"`
	Basis             string     `json:"basis"`
	EffectiveStartsAt time.Time  `json:"effectiveStartsAt"`
	EffectiveEndsAt   *time.Time `json:"effectiveEndsAt,omitempty"`
}

// productBatchCommand 是翻译产物里的一项：标签供回显，执行闭包对着处理器跑。
type productBatchCommand struct {
	label   string
	execute func(context.Context, *pcapplication.RegisterProductChannelHandler) (pcapplication.ProductChannelResult, error)
}

func productBatchFromJSON(raw []byte) ([]productBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document productBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("产品登记批不是本入口的形状：%w", err)
	}
	tenant, err := pcdomain.NewTenantID(document.TenantID)
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
	item serviceProductFormDocument,
) (productBatchCommand, error) {
	none := productBatchCommand{}
	objectID, err := pcdomain.NewCommercialObjectID(item.ProductID)
	if err != nil {
		return none, err
	}
	version, err := pcdomain.NewCommercialVersionLabel(item.Version)
	if err != nil {
		return none, err
	}
	form, err := serviceProductFormFromName(item.Form)
	if err != nil {
		return none, err
	}
	command := pcapplication.RegisterServiceProductFormCommand{
		Tenant:   tenant,
		Scope:    scope,
		ObjectID: objectID,
		Version:  version,
		Form:     form,
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
	item productChannelMappingDocument,
) (productBatchCommand, error) {
	none := productBatchCommand{}
	id, err := pcdomain.NewProductChannelMappingID(item.MappingID)
	if err != nil {
		return none, err
	}
	product, err := pcdomain.NewCommercialObjectID(item.ProductID)
	if err != nil {
		return none, err
	}
	productVersion, err := pcdomain.NewCommercialVersionLabel(item.ProductVersion)
	if err != nil {
		return none, err
	}
	// channels 缺席（或 null）是输入缺件；`[]` 是登记者说出的“未配置”声明——该产品
	// 尚无可用渠道候选（CONTEXT 渠道绑定格）。两者必须可分辨，所以这里看 nil 而非长度。
	if item.Channels == nil {
		return none, fmt.Errorf("channels 缺席：要么给渠道引用，要么写 [] 显式声明未配置")
	}
	binding := pcdomain.UnconfiguredChannelBinding()
	if len(item.Channels) > 0 {
		references := make([]pcdomain.ChannelProductReference, 0, len(item.Channels))
		for _, raw := range item.Channels {
			reference, err := pcdomain.NewChannelProductReference(raw)
			if err != nil {
				return none, err
			}
			references = append(references, reference)
		}
		binding, err = pcdomain.NewConfiguredChannelBinding(references)
		if err != nil {
			return none, err
		}
	}
	basis, err := pcdomain.NewMappingBasisReference(item.Basis)
	if err != nil {
		return none, err
	}
	endsAt := time.Time{}
	if item.EffectiveEndsAt != nil {
		endsAt = *item.EffectiveEndsAt
	}
	interval, err := pcdomain.NewEffectiveInterval(item.EffectiveStartsAt, endsAt)
	if err != nil {
		return none, err
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
	return productBatchCommand{
		label: fmt.Sprintf("产品—渠道映射 %s r%d", item.MappingID, item.Revision),
		execute: func(ctx context.Context, handler *pcapplication.RegisterProductChannelHandler) (pcapplication.ProductChannelResult, error) {
			return handler.RegisterMapping(ctx, command)
		},
	}, nil
}

// serviceProductFormFromName 是 domain.ServiceProductForm 封闭集的名称镜像；集合外
// 取值拒收不吸收（独立面单渠道形态对首发不适用，PAR-COM-12，故封闭集只有一格）。
func serviceProductFormFromName(raw string) (pcdomain.ServiceProductForm, error) {
	if raw == pcdomain.NetworkServiceForm.String() {
		return pcdomain.NetworkServiceForm, nil
	}
	return pcdomain.ServiceProductFormInvalid, fmt.Errorf("未知服务形态 %q", raw)
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
