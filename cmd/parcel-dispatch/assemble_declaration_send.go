package main

import (
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"

	ccchannel "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/declarationchannel"
	ccinbox "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/inbox"
	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccapplication "go.idp.xyz/idp-parcel/internal/customscompliance/application"
)

// declarationSendUndecidedSentinels 是 CC 提交版本已形成 → 申报发送这一路的未决哨兵（ADR-0342 决定一）：提交版本读不回、册读写
// 不动、确知没发出去，都是续办。通道没登、已发过与就绪或授权失效了是答案，入账。
var declarationSendUndecidedSentinels = []error{
	ccinbox.ErrDeclarationSendUndecided,
}

// declarationSendConsumer 装申报发送：提交版本册、通道册、发送事实册与就绪、授权两个读口，发送口是产品随附的 HTTP 连接器。
func declarationSendConsumer(db *bentopg.DB, inboxStore *inbox.Store, clock systemClock) (dispatch.Consumer, error) {
	submissions, err := ccpostgres.NewDeclarationSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: declaration submissions for sending: %w", err)
	}
	sends, err := ccpostgres.NewSubmissionSends(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: submission sends: %w", err)
	}
	readiness, err := ccpostgres.NewReadinessView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: readiness view for sending: %w", err)
	}
	authority, err := ccpostgres.NewSubmissionAuthorityView(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: submission authority view for sending: %w", err)
	}
	sender, err := ccapplication.NewSendDeclarationHandler(ccapplication.SendDeclarationDeps{
		Submissions: submissions, Channels: sends, SendFacts: sends, Readiness: readiness, Authority: authority,
		Sender: ccchannel.NewHTTPSender(nil, nil), Clock: clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: send declaration: %w", err)
	}
	consumer, err := ccinbox.NewDeclarationSendConsumer(db.Transactor(), inboxStore, sender)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: declaration send consumer: %w", err)
	}
	return consumer, nil
}
