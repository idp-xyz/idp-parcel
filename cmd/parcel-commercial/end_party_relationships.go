package main

// 本文件承载参与方关系结束的受控入口（票 ps-external-mark-relations/17）：end-party-relationships 子命令，与 parcel-api 的
// `/commercial-party-relationship-revocations`、`-expirations`、`-supersessions` 共用一份批文形状与逐项翻译
// （registrationjson.PartyRelationshipEndingBatchDocument）。
//
// 批内各项独立成败、逐项各起事务（AT-PC-011 同款纪律）：文件内 revocations → expirations → supersessions 的次序只是回显
// 习惯，某项被拒不撤已落的前项，重跑同一批已落项以重放回答。替代一项的两笔（接替关系与原关系的终止修订）在同一事务里：
// 用例交回 error 即整笔回滚，批在此停下。结束哪段关系、凭什么依据、由谁接替，一格不代填——那是租户商业责任方的决定。

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

type relationshipEndingBatchCommand struct {
	label   string
	execute func(context.Context, *pcapplication.PartyRelationshipEndingHandler) (pcapplication.PartyRegistryResult, error)
}

func relationshipEndingBatchFromJSON(raw []byte) ([]relationshipEndingBatchCommand, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var document registrationjson.PartyRelationshipEndingBatchDocument
	if err := decoder.Decode(&document); err != nil {
		return nil, fmt.Errorf("关系结束批不是本入口的形状：%w", err)
	}
	tenant, err := registrationjson.DocumentTenant(document.TenantID)
	if err != nil {
		return nil, err
	}
	if document.ItemCount() == 0 {
		return nil, fmt.Errorf("关系结束批没有任何项")
	}
	commands := make([]relationshipEndingBatchCommand, 0, document.ItemCount())
	for _, item := range document.Revocations {
		command, err := registrationjson.PartyRelationshipRevocationCommand(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("revocations/%s：%w", item.RelationshipID, err)
		}
		commands = append(commands, relationshipEndingBatchCommand{
			label: fmt.Sprintf("撤销参与方关系 %s r%d", item.RelationshipID, item.Revision),
			execute: func(ctx context.Context, handler *pcapplication.PartyRelationshipEndingHandler) (pcapplication.PartyRegistryResult, error) {
				return handler.Revoke(ctx, command)
			},
		})
	}
	for _, item := range document.Expirations {
		command, err := registrationjson.PartyRelationshipExpirationCommand(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("expirations/%s：%w", item.RelationshipID, err)
		}
		commands = append(commands, relationshipEndingBatchCommand{
			label: fmt.Sprintf("到期参与方关系 %s r%d", item.RelationshipID, item.Revision),
			execute: func(ctx context.Context, handler *pcapplication.PartyRelationshipEndingHandler) (pcapplication.PartyRegistryResult, error) {
				return handler.Expire(ctx, command)
			},
		})
	}
	for _, item := range document.Supersessions {
		command, err := registrationjson.PartyRelationshipSupersessionCommand(tenant, item)
		if err != nil {
			return nil, fmt.Errorf("supersessions/%s：%w", item.RelationshipID, err)
		}
		commands = append(commands, relationshipEndingBatchCommand{
			label: fmt.Sprintf("替代参与方关系 %s r%d，由 %s 接替（%s）",
				item.RelationshipID, item.Revision, item.Successor, item.SuccessorRelationshipID),
			execute: func(ctx context.Context, handler *pcapplication.PartyRelationshipEndingHandler) (pcapplication.PartyRegistryResult, error) {
				return handler.Supersede(ctx, command)
			},
		})
	}
	return commands, nil
}

func runEndPartyRelationships(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	flags := flag.NewFlagSet("end-party-relationships", flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "关系结束批 JSON 文件路径")
	if err := flags.Parse(args); err != nil {
		return exitTechnical
	}
	if *input == "" {
		fmt.Fprintln(errOut, "end-party-relationships 需要 -input <file>")
		return exitTechnical
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读关系结束批：%v\n", err)
		return exitTechnical
	}
	commands, err := relationshipEndingBatchFromJSON(raw)
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

	registry, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		fmt.Fprintf(errOut, "构造身份登记册：%v\n", err)
		return exitTechnical
	}
	handler := pcapplication.NewPartyRelationshipEndingHandler(registry, systemClock{})

	attention := false
	for index, item := range commands {
		var result pcapplication.PartyRegistryResult
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
		fmt.Fprintf(out, "%s：%s%s\n", label, result.Outcome(), causeDetail(result))
		switch result.Outcome() {
		case pcapplication.PartyIdentityContentConflict,
			pcapplication.PartyIdentityNotAccepted,
			pcapplication.PartyIdentityNotFound:
			attention = true
		}
	}
	if attention {
		return exitAttention
	}
	return exitLanded
}
