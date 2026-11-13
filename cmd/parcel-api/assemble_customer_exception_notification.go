package main

import (
	"context"
	"fmt"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	vechannel "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/notificationchannel"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// customerExceptionNotificationFace 把一次客户异常通知包进一笔事务：通知册的写口按框架合同无事务即拒，同一份披露上的
// 发送靠事务级的锁排成一列。未决整笔回滚——渠道没登记时连「已生成」也不留。
//
// 推送发生在事务之内：推送成了而提交没成的那一笔会整笔回滚，重发时渠道凭幂等键（披露身份）认出是同一份。代价是推送
// 等答复期间占着一条连接，上限是连接器的超时。
type customerExceptionNotificationFace struct {
	notification customsDecisionTransaction[visibilityapp.NotifySignalCommand, visibilityapp.NotifyCustomerResult]
}

func (face customerExceptionNotificationFace) HandleSignal(ctx context.Context, command visibilityapp.NotifySignalCommand) (visibilityapp.NotifyCustomerResult, error) {
	return face.notification.Handle(ctx, command)
}

// noNotificationSubscribers 顶在通知交接那一格上：通知决定的下游（义务台账、升级判断）今天没有任何订阅者，往 Outbox
// 里写一种没人消费的事件只会堆在那里。有了订阅者，换成 vepostgres.NewOutboxNotificationHandoff。
type noNotificationSubscribers struct{}

func (noNotificationSubscribers) HandOffNotification(context.Context, veports.NotificationHandoffIntent) error {
	return nil
}

// buildCustomerExceptionNotificationFace 装配本口背后的真编排。通知策略的读面与渠道连接器在构造时钉住租户（连接器
// 还钉住这份通知说的是哪件包裹哪一类信号），而它们随每次请求到达，所以编排逐请求构造；各只只持有连接池的引用，构造
// 不触库。getenv 读渠道凭据变量的值。
func buildCustomerExceptionNotificationFace(db *bentopg.DB, getenv func(string) string) (customerExceptionNotificationFace, error) {
	none := customerExceptionNotificationFace{}
	episodes, err := vepostgres.NewSignalEpisodes(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: notification signal episodes: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: notification shipment requests: %w", err)
	}
	customers, err := veps.NewParcelCustomerAccountLookup(requests)
	if err != nil {
		return none, fmt.Errorf("parcel-api: notification customer accounts: %w", err)
	}
	decisions, err := vepostgres.NewDisclosureDecisions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: notification disclosure decisions: %w", err)
	}
	notifications, err := vepostgres.NewCustomerNotifications(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: customer notifications: %w", err)
	}
	channels, err := vepostgres.NewNotificationChannels(db)
	if err != nil {
		return none, fmt.Errorf("parcel-api: notification channels: %w", err)
	}
	identities, err := veidentity.NewNotifications()
	if err != nil {
		return none, fmt.Errorf("parcel-api: notification identities: %w", err)
	}
	handle := func(ctx context.Context, command visibilityapp.NotifySignalCommand) (visibilityapp.NotifyCustomerResult, error) {
		policy, err := vepostgres.NewNotificationPolicies(db, command.TenantID)
		if err != nil {
			return visibilityapp.NotifyCustomerResult{}, fmt.Errorf("parcel-api: notification policy: %w", err)
		}
		gateway, err := vechannel.NewHTTPGateway(channels, command.TenantID, vechannel.Subject{Parcel: command.Parcel, Kind: command.Kind}, nil, getenv)
		if err != nil {
			return visibilityapp.NotifyCustomerResult{}, fmt.Errorf("parcel-api: notification channel gateway: %w", err)
		}
		return visibilityapp.NewNotifyCustomerHandler(visibilityapp.NotifyCustomerDeps{
			Decisions: decisions, Notifications: notifications, Policy: policy, Identities: identities,
			Channel: gateway, Downstream: noNotificationSubscribers{}, Clock: systemClock{},
			EpisodeByID: episodes, Customers: customers, SendLock: notifications,
		}).HandleSignal(ctx, command)
	}
	return customerExceptionNotificationFace{
		notification: customsDecisionTransaction[visibilityapp.NotifySignalCommand, visibilityapp.NotifyCustomerResult]{
			transactor: db.Transactor(), handle: handle,
			rollBack: func(result visibilityapp.NotifyCustomerResult) bool {
				return result.Outcome() == visibilityapp.NotifyUndecided
			},
		},
	}, nil
}
