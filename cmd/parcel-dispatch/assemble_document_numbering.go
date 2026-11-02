package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
)

// buildDocumentNumbers 在派发进程里装编号组件（ADR-0355 决定四、六），与 parcel-api 是同一个装配（docnumbering.Build）、
// 同一份类型汇总、同一项加扰密钥配置。装配点调它只为两件事：启动即查（类型声明不合规、序列存储构造失败就带原因退出），
// 与把取号编排交到 dispatchSources 上——本进程开立的单据由各自的实现票去接，这里不接任何一类的开立。
//
// 必须在 wireDispatcher 用的那个 db 上装：开立事务由同一个 DB 的事务器开，框架按 DB 认事务归属。
func buildDocumentNumbers(db *bentopg.DB, settings dispatchSettings) (docnumbering.Components, error) {
	numbers, err := docnumbering.Build(db, settings.scrambleSecrets)
	if err != nil {
		return docnumbering.Components{}, fmt.Errorf("parcel-dispatch: document numbering: %w", err)
	}
	return numbers, nil
}
