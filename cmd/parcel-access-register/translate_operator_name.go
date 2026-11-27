package main

// 本文件把操作者姓名批译成领域类型：单项的形状与翻译取 registrationjson（与在线口 /operator-names 同一份），本文件只拆批、
// 取批头的租户与「谁登的」。姓名是 AGENTS.md 红线「敏感实例外置」对操作者开的例外，批文里的姓名由租户实施时给；
// 仓库里的批文（演示种子）只写合成名。

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/accessidentity"
	"go.idp.xyz/idp-parcel/internal/accessidentity/adapters/registrationjson"
)

type operatorNameBatchDocument struct {
	TenantID string `json:"tenantId"`
	// RecordedBy 是这一批由谁登的，落在姓名行上。在线口记调用者的标识；批文没有调用者，这一格要人写，不代填。
	RecordedBy string                              `json:"recordedBy"`
	Names      []registrationjson.OperatorNameItem `json:"names"`
}

func operatorNameBatchFromJSON(raw []byte) ([]batchItem, error) {
	var document operatorNameBatchDocument
	if err := decodeStrict(raw, &document, "姓名登记批"); err != nil {
		return nil, err
	}
	if err := requireBatch(document.TenantID, len(document.Names), "姓名登记批"); err != nil {
		return nil, err
	}
	recordedBy := strings.TrimSpace(document.RecordedBy)
	if recordedBy == "" {
		return nil, fmt.Errorf("姓名登记批缺 recordedBy：谁登的不代填")
	}
	items := make([]batchItem, 0, len(document.Names))
	for _, entry := range document.Names {
		subject, name, err := registrationjson.OperatorNameFrom(entry)
		if err != nil {
			return nil, fmt.Errorf("names/%s：%w", entry.Subject, err)
		}
		// 回显不带姓名本身：批的输出会进运维的日志，那不是该放人名的地方。
		action := "登姓名"
		if name == "" {
			action = "删姓名"
		}
		items = append(items, batchItem{
			label: fmt.Sprintf("操作者 %s %s %s", subject.Issuer(), subject.Subject(), action),
			apply: func(ctx context.Context, books registers) (outcome, error) {
				answer, err := books.names.SaveOperatorName(ctx, document.TenantID, subject, name, recordedBy)
				return operatorNameOutcome(answer), err
			},
		})
	}
	return items, nil
}

// operatorNameOutcome 只认登上与删掉为落定；本租户册上没有这个主体要请人看。
func operatorNameOutcome(answer accessidentity.OperatorNameOutcome) outcome {
	switch answer {
	case accessidentity.OperatorNameRecorded, accessidentity.OperatorNameCleared:
		return outcome{name: answer.String(), landed: true}
	}
	return outcome{name: answer.String()}
}
