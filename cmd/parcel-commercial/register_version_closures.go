package main

// 本文件承载按后继收尾旧版的受控入口（ADR-0233，票 demo-chain-gaps/50）：register-version-closures 子命令，与 parcel-api 的
// `/commercial-version-successor-cuts`、`/commercial-version-retirements` 共用一份批文形状与逐项翻译（registrationjson）。
//
// 批内各项独立成败、逐项各起事务（AT-PC-011 同款纪律）：文件内 cuts → retirements 的次序只是回显习惯，某项被拒不撤已落
// 的前项，重跑同一批已落项以重放回答。收尾哪一版、让位给哪个后继、凭哪份依据，一格不代填——那是租户商业责任方的决定。

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
)

type versionClosureBatchCommand struct {
	label   string
	execute func(context.Context, *pcapplication.CloseCommercialVersionHandler) (pcapplication.VersionClosureResult, error)
}

func versionClosureBatchFromJSON(raw []byte) ([]versionClosureBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registrationjson.VersionClosureBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("收尾批不是本入口的形状：%w", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
	if err != nil {
		return nil, err
	}
	if len(document.Cuts)+len(document.Retirements) == 0 {
		return nil, fmt.Errorf("收尾批没有任何项")
	}
	commands := make([]versionClosureBatchCommand, 0, len(document.Cuts)+len(document.Retirements))
	for _, item := range document.Cuts {
		command, err := registrationjson.SuccessorCutCommand(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("cuts/%s/%s：%w", item.ObjectID, item.Version, err)
		}
		commands = append(commands, versionClosureBatchCommand{
			label: fmt.Sprintf("截止 %s %s/%s 于 %s 的生效时点", item.Kind, item.ObjectID, item.Version, item.SuccessorVersion),
			execute: func(ctx context.Context, handler *pcapplication.CloseCommercialVersionHandler) (pcapplication.VersionClosureResult, error) {
				return handler.Cut(ctx, command)
			},
		})
	}
	for _, item := range document.Retirements {
		command, err := registrationjson.SameStartRetirementCommand(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("retirements/%s/%s：%w", item.ObjectID, item.Version, err)
		}
		commands = append(commands, versionClosureBatchCommand{
			label: fmt.Sprintf("退役 %s %s/%s，让位给 %s", item.Kind, item.ObjectID, item.Version, item.SuccessorVersion),
			execute: func(ctx context.Context, handler *pcapplication.CloseCommercialVersionHandler) (pcapplication.VersionClosureResult, error) {
				return handler.RetireAtSameStart(ctx, command)
			},
		})
	}
	return commands, nil
}

func runRegisterVersionClosures(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("register-version-closures", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "收尾批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "register-version-closures 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读收尾批：%v\n", err)
		return exitTechnical
	}
	commands, err := versionClosureBatchFromJSON(raw)
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
	handler := pcapplication.NewCloseCommercialVersionHandler(publications, systemClock{})

	attention := false
	for index, item := range commands {
		var result pcapplication.VersionClosureResult
		err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
			var handleErr error
			result, handleErr = item.execute(txCtx, handler)
			return handleErr
		})
		label := fmt.Sprintf("第 %d/%d 项 %s", index+1, len(commands), item.label)
		if err != nil {
			fmt.Fprintf(errOut, "%s：技术失败，批在此停下：%v\n", label, err)
			return exitTechnical
		}
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), versionClosureDetail(result))
		if result.Outcome() == pcapplication.VersionClosureNotAccepted {
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}

func versionClosureDetail(result pcapplication.VersionClosureResult) string {
	if cause := result.Cause(); cause != nil {
		return fmt.Sprintf("（原因：%v）", cause)
	}
	const layout = "2006-01-02T15:04:05Z07:00"
	if correction, held := result.Correction(); held {
		interval := correction.CorrectedInterval()
		if end, bounded := interval.EndsAt(); bounded {
			return fmt.Sprintf("（旧版选用区间 %s 至 %s）", interval.StartsAt().Format(layout), end.Format(layout))
		}
	}
	if retirement, held := result.Retirement(); held {
		return fmt.Sprintf("（自 %s 起不再参与新的选择，让位给 %s）", retirement.RetiredAt().Format(layout), retirement.Successor())
	}
	return ""
}
