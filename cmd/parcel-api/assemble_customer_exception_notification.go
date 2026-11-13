package main

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	bentoapp "go.idp.xyz/idp-bento-go/application"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	vechannel "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/notificationchannel"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	visibilityapp "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	vedomain "go.idp.xyz/idp-parcel/internal/visibilityexception/domain"
	veports "go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

// sendTransactionTimeout 是推送那一笔事务的上限：连接器等答复至多十五秒，再留出落库的时间。
const sendTransactionTimeout = 30 * time.Second

// customerExceptionNotificationFace 把一次客户异常通知分成两笔事务。第一笔受理并把通知记录落下（策略或渠道未配置时
// 答未决、整笔回滚，什么也不留）；第二笔推送并回填过程节点。通知记录先于推送提交：推送那一笔无论怎么断，记录都在、
// 通知标识不变，重发时客户系统凭幂等键认出是同一份。
//
// 第二笔不随调用方的取消而取消：操作员关掉页面、反向代理超时都不该让一次已经发出去的推送记不下来。同一份披露上的
// 发送靠事务级的锁排成一列，两笔各取一次；两个人同时发，后到的在第二笔里读到已提交、不再推。
type customerExceptionNotificationFace struct {
	transactor bentoapp.Transactor
	handlerFor func(visibilityapp.NotifySignalCommand) (*visibilityapp.NotifyCustomerHandler, error)
}

func (face customerExceptionNotificationFace) HandleSignal(ctx context.Context, command visibilityapp.NotifySignalCommand) (visibilityapp.NotifyCustomerResult, error) {
	handler, err := face.handlerFor(command)
	if err != nil {
		return visibilityapp.NotifyCustomerResult{}, err
	}
	var (
		result   visibilityapp.NotifyCustomerResult
		prepared bool
	)
	if err := face.withinTransaction(ctx, &result, func(txCtx context.Context) (visibilityapp.NotifyCustomerResult, error) {
		answer, ready, err := handler.PrepareSignal(txCtx, command)
		prepared = ready
		return answer, err
	}); err != nil || !prepared {
		return result, err
	}
	sendCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), sendTransactionTimeout)
	defer cancel()
	err = face.withinTransaction(sendCtx, &result, func(txCtx context.Context) (visibilityapp.NotifyCustomerResult, error) {
		return handler.HandleSignal(txCtx, command)
	})
	return result, err
}

// errNotificationUndecided 只用来让事务回滚：未决不留下任何写入，答复照旧交回编排那一份。
var errNotificationUndecided = errors.New("parcel-api: notification undecided")

func (face customerExceptionNotificationFace) withinTransaction(
	ctx context.Context,
	result *visibilityapp.NotifyCustomerResult,
	work func(context.Context) (visibilityapp.NotifyCustomerResult, error),
) error {
	err := face.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		answer, err := work(txCtx)
		if err != nil {
			return err
		}
		*result = answer
		if answer.Outcome() == visibilityapp.NotifyUndecided {
			return errNotificationUndecided
		}
		return nil
	})
	if errors.Is(err, errNotificationUndecided) {
		return nil
	}
	return err
}

// notifiedOffline 顶在渠道那一格上，给手工标记用：通知已经由人在线下送达，这里没有东西要发，也没有端点要预检。
type notifiedOffline struct{}

func (notifiedOffline) SubmitToChannel(context.Context, *vedomain.CustomerNotification) error {
	return nil
}

func (notifiedOffline) CheckChannel(context.Context, vedomain.NotificationChannelReference) error {
	return nil
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
	handlerFor := func(command visibilityapp.NotifySignalCommand) (*visibilityapp.NotifyCustomerHandler, error) {
		policy, err := vepostgres.NewNotificationPolicies(db, command.TenantID)
		if err != nil {
			return nil, fmt.Errorf("parcel-api: notification policy: %w", err)
		}
		if strings.TrimSpace(command.Method) != "" || strings.TrimSpace(command.Original) != "" {
			// 手工标记：操作员已经在线下通知了客户，这里不推送任何东西，也不要求登记过推送端点。其余——披露决定、通知
			// 策略给的时限与渠道引用、过程节点、发送锁——与推送那一路是同一套。
			return visibilityapp.NewNotifyCustomerHandler(visibilityapp.NotifyCustomerDeps{
				Decisions: decisions, Notifications: notifications, Policy: policy, Identities: identities,
				Channel: notifiedOffline{}, Downstream: noNotificationSubscribers{}, Clock: systemClock{},
				EpisodeByID: episodes, Customers: customers, SendLock: notifications, Preflight: notifiedOffline{},
				ManualRecords: notifications,
			}), nil
		}
		gateway, err := vechannel.NewHTTPGateway(channels, command.TenantID, vechannel.Subject{Parcel: command.Parcel, Kind: command.Kind}, nil, getenv)
		if err != nil {
			return nil, fmt.Errorf("parcel-api: notification channel gateway: %w", err)
		}
		return visibilityapp.NewNotifyCustomerHandler(visibilityapp.NotifyCustomerDeps{
			Decisions: decisions, Notifications: notifications, Policy: policy, Identities: identities,
			Channel: gateway, Downstream: noNotificationSubscribers{}, Clock: systemClock{},
			EpisodeByID: episodes, Customers: customers, SendLock: notifications, Preflight: gateway,
		}), nil
	}
	return customerExceptionNotificationFace{transactor: db.Transactor(), handlerFor: handlerFor}, nil
}
