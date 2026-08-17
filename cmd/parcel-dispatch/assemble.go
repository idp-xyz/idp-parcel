package main

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpartycommercial "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/partycommercial"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 本文件是派发进程的组合根。它是全仓依赖面最宽的包，而这是 ADR-0049 认下的代价：
// 进程内直投要求派发方与被投递的消费者同进程，组合根因此必须同时知道消费侧适配器
// 与它们的编排。约束是「只在这一个包里知道」——`internal/platform/` 一行上下文类型
// 都不进，平台层对事件类型一无所知这条不破。

// systemClock 是各处 Clock 端口的生产实现。一拍要的是真实时钟，`time.Now()` 就是它，
// 不是替身。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// dispatchSettings 是本进程的部署形态参数，全部必填、一个默认值都不给。
//
// ADR-0049 第四条要求投递超时显式给出，`dispatch.Config` 的注释也写明批量与租约没有
// 合理默认。给默认值的坏处很具体：一个没人调过的节奏会在生产上表现为「事件好像卡住
// 了」，而现场看不出那个数是谁定的。
type dispatchSettings struct {
	databaseDSN     string
	purpose         nrdomain.ServicePurpose
	deliveryTimeout time.Duration
	config          dispatch.Config
}

const (
	envDatabaseDSN     = "IDP_PARCEL_POSTGRES_DSN"
	envServicePurpose  = "IDP_PARCEL_ROUTE_SERVICE_PURPOSE"
	envDeliveryTimeout = "IDP_PARCEL_DISPATCH_DELIVERY_TIMEOUT"
	envBatchLimit      = "IDP_PARCEL_DISPATCH_LIMIT"
	envLeaseFor        = "IDP_PARCEL_DISPATCH_LEASE"
	envMaxAttempts     = "IDP_PARCEL_DISPATCH_MAX_ATTEMPTS"
	envRetryAfter      = "IDP_PARCEL_DISPATCH_RETRY_AFTER"
)

func settingsFromEnv(getenv func(string) string) (dispatchSettings, error) {
	var settings dispatchSettings
	var err error

	if settings.databaseDSN = getenv(envDatabaseDSN); settings.databaseDSN == "" {
		return dispatchSettings{}, fmt.Errorf("parcel-dispatch: %s is required", envDatabaseDSN)
	}
	// 服务目的属实例半边：判断键含它，而目的由服务产品定义。未配置时停下不猜——
	// 猜一个等于替租户宣布这批包裹按哪种服务判断。
	rawPurpose := getenv(envServicePurpose)
	if rawPurpose == "" {
		return dispatchSettings{}, fmt.Errorf("parcel-dispatch: %s is required", envServicePurpose)
	}
	if settings.purpose, err = nrdomain.NewServicePurpose(rawPurpose); err != nil {
		return dispatchSettings{}, fmt.Errorf("parcel-dispatch: %s: %w", envServicePurpose, err)
	}
	if settings.deliveryTimeout, err = durationFromEnv(getenv, envDeliveryTimeout); err != nil {
		return dispatchSettings{}, err
	}
	if settings.config.LeaseFor, err = durationFromEnv(getenv, envLeaseFor); err != nil {
		return dispatchSettings{}, err
	}
	if settings.config.RetryAfter, err = durationFromEnv(getenv, envRetryAfter); err != nil {
		return dispatchSettings{}, err
	}
	limit, err := positiveIntFromEnv(getenv, envBatchLimit)
	if err != nil {
		return dispatchSettings{}, err
	}
	settings.config.Limit = limit
	attempts, err := positiveIntFromEnv(getenv, envMaxAttempts)
	if err != nil {
		return dispatchSettings{}, err
	}
	settings.config.MaxAttempts = uint32(attempts)
	return settings, nil
}

func durationFromEnv(getenv func(string) string, name string) (time.Duration, error) {
	raw := getenv(name)
	if raw == "" {
		return 0, fmt.Errorf("parcel-dispatch: %s is required", name)
	}
	value, err := time.ParseDuration(raw)
	if err != nil {
		return 0, fmt.Errorf("parcel-dispatch: %s: %w", name, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("parcel-dispatch: %s must be positive", name)
	}
	return value, nil
}

func positiveIntFromEnv(getenv func(string) string, name string) (int, error) {
	raw := getenv(name)
	if raw == "" {
		return 0, fmt.Errorf("parcel-dispatch: %s is required", name)
	}
	value, err := strconv.Atoi(raw)
	if err != nil {
		return 0, fmt.Errorf("parcel-dispatch: %s: %w", name, err)
	}
	if value <= 0 {
		return 0, fmt.Errorf("parcel-dispatch: %s must be positive", name)
	}
	return value, nil
}

// assembleDispatcher 是派发一拍的装配点：读部署形态、开连接池、接完整依赖图。
//
// 交回的第二个值是收尾函数，进程停机时调。装配中途失败时连接池就地关掉——半开的池
// 会在下一次装配尝试时耗掉连接数，而那种耗尽看起来像数据库出了问题。
func assembleDispatcher(ctx context.Context, getenv func(string) string) (Beat, func(), error) {
	settings, err := settingsFromEnv(getenv)
	if err != nil {
		return nil, nil, err
	}
	pool, err := pgxpool.New(ctx, settings.databaseDSN)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-dispatch: connect: %w", err)
	}
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-dispatch: framework db: %w", err)
	}
	beat, err := wireDispatcher(db, settings)
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return beat, pool.Close, nil
}

// wireDispatcher 接依赖图。它与读环境分开，是为了让组合根能对着真库整体验一遍——
// 一个只能靠进程起停验证的装配点，等于没有验证。
//
// 路由表今天只有一条。这不是省事：`AcceptanceConsumer` 是本仓唯一的消费者，而按
// ADR-0049 第三条，登记一个本进程接不住的类型比不登记更糟——它会让无订阅者的失败
// 变成「订阅了但处理不了」。
func wireDispatcher(db *bentopg.DB, settings dispatchSettings) (Beat, error) {
	if db == nil {
		return nil, errors.New("parcel-dispatch: framework db is required")
	}
	clock := systemClock{}

	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: outbox store: %w", err)
	}
	inboxStore, err := inbox.NewStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: inbox store: %w", err)
	}

	consumer, err := acceptanceConsumer(db, outboxStore, inboxStore, settings, clock)
	if err != nil {
		return nil, err
	}
	// 未决哨兵在路由条目处翻译（不在 NR 侧，也不在派发器里）：NR 的
	// ErrRouteHandoffUndecided 不翻，失败码会塌成 dispatch.publish_failed，运维据此
	// 去查传输，而实际要查的是消费方等的那个依赖。
	routed, err := dispatch.WithUndecidedSentinels(consumer, nrparcelshipment.ErrRouteHandoffUndecided)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: undecided translation: %w", err)
	}

	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{nrinbox.AcceptedDecisionEventType: routed},
		settings.deliveryTimeout,
		settings.config,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: direct publisher: %w", err)
	}

	dispatcher, err := dispatch.NewDispatcher(outboxStore, outboxStore, publisher, clock, settings.config)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: dispatcher: %w", err)
	}
	return dispatcher, nil
}

// acceptanceConsumer 接 UC-NR-001 那条线：PS 接受决定信封 → 消费门 → 初始路由编排。
//
// 商业适用性视图的解析标识来源是实例半边，今天没有租户登记过，因此它按「依赖不可用」
// 答复，编排形成`未决`。网络定义登记册同理答`未配置`。两者都不是接线错误——首发就该
// 停在这里，而不是靠一份编出来的默认值往下走。
func acceptanceConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	settings dispatchSettings,
	clock systemClock,
) (dispatch.Consumer, error) {
	definitions, err := nrpostgres.NewNetworkDefinitions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: network definitions: %w", err)
	}
	routeStore, err := nrpostgres.NewInitialRoutes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: initial route store: %w", err)
	}
	handoffLog, err := nrpostgres.NewRouteHandoffLogs(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route handoff log: %w", err)
	}
	identities, err := nrpostgres.NewRouteIdentities(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route identities: %w", err)
	}
	downstream, err := nrpostgres.NewOutboxInitialRouteHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: initial route handoff: %w", err)
	}
	closures, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: commercial resolutions: %w", err)
	}
	// 第二个入参是把判断键折成解析标识的实例半边映射，今天没有任何租户登记过它。
	// nil 是「显式未配置」的诚实表达：视图据此答依赖不可用，编排停在未决。
	applicability, err := nrpartycommercial.NewRoutingApplicability(closures, nil)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: routing applicability: %w", err)
	}

	routeHandler := nrapplication.NewCreateInitialRouteHandler(nrapplication.CreateInitialRouteDeps{
		Applicability: applicability,
		Evidence:      definitions,
		Store:         routeStore,
		Log:           handoffLog,
		Downstream:    downstream,
		Identities:    identities,
		Clock:         clock,
	})

	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: shipment requests: %w", err)
	}
	decisions, err := nrparcelshipment.NewRouteOnAcceptanceAdapter(requests, routeHandler, settings.purpose)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route on acceptance: %w", err)
	}
	consumer, err := nrinbox.NewAcceptanceConsumer(db.Transactor(), inboxStore, decisions)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance consumer: %w", err)
	}
	return consumer, nil
}
