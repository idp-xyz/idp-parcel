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
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psnodeops "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
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

// nodeIntakeUndecidedSentinels 是采用那条链登记的未决哨兵。做成包级量是为了让测试
// 用生产的同一份名单：名单在测试里重抄一遍，抄漏的那个哨兵会在测试里绿、在生产里
// 塌成 dispatch.publish_failed。
//
// ErrAdoptionHandoffPending 不在名单里，它要运维查的是 outbox 下游而非商业资格目录；
// ErrAmbiguousParcelTarget 同样不在——反查歧义要人去看为什么两份已接受委托声明了同一
// 个包裹，不是等某个依赖到位。
var nodeIntakeUndecidedSentinels = []error{
	psnodeops.ErrReceptionNotVisible,
	psnodeops.ErrParcelTargetNotFound,
	psnodeops.ErrAdoptionUndecided,
	psnodeops.ErrUnidentifiedHandlingUnit,
}

// wireDispatcher 接依赖图。它与读环境分开，是为了让组合根能对着真库整体验一遍——
// 一个只能靠进程起停验证的装配点，等于没有验证。
//
// 路由表今天有三条：PS 接受决定 → 初始路由（UC-NR-001）、PS 有效网络收寄采用结果 →
// 路由复核（UC-PS-003 步骤 8 → UC-NR-003）、NO 节点收寄形成 → PS 来源采用
// （UC-PS-003）。前两条投向 network-routing，第三条投回 parcel-shipment 自己。
// 登记的仍然只有
// 本进程真接得住的类型——按 ADR-0049 第三条，登记一个接不住的比不登记更糟，它会让
// 无订阅者的失败变成「订阅了但处理不了」。其余已发布但无消费者的类型照旧撞
// `dispatch.no_subscriber`，那是记录里认下的代价。
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

	intakes, err := networkIntakeConsumer(db, inboxStore, settings, clock)
	if err != nil {
		return nil, err
	}
	// 两条链各带各的未决哨兵：合用一个失败码，运维就分不出该去查初始路由那条还是
	// 复核这条等的依赖。
	routedIntakes, err := dispatch.WithUndecidedSentinels(intakes, nrparcelshipment.ErrReassessmentUndecided)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reassessment undecided translation: %w", err)
	}

	adoptions, err := adoptNodeIntakeConsumer(db, outboxStore, inboxStore, clock)
	if err != nil {
		return nil, err
	}
	// 采用这条链的未决面比前两条宽：收寄可见性滞后、目标委托还没落到已接受、资格
	// 目录未配置、实物还没识别，四种都是「等一个依赖」而不是发布失败。
	routedAdoptions, err := dispatch.WithUndecidedSentinels(adoptions, nodeIntakeUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node intake undecided translation: %w", err)
	}

	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{
			nrinbox.AcceptedDecisionEventType:     routed,
			nrinbox.AdoptedNetworkIntakeEventType: routedIntakes,
			psinbox.NodeIntakeFormedEventType:     routedAdoptions,
		},
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

// networkIntakeConsumer 接 UC-NR-003 那条线：PS 有效网络收寄采用结果信封 → 消费门 →
// 按采用键取回记录 → 复核编排。它是纵向闭环里收寄那一段回到路由的一拍。
//
// 与接受决定那条线各建各的仓储包装：它们都是 db 上的无状态包装，共享一份反而让两条
// 链的依赖图看不出各自要什么。真正共享的只有 db、inbox 与时钟。
func networkIntakeConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	settings dispatchSettings,
	clock systemClock,
) (dispatch.Consumer, error) {
	routeStore, err := nrpostgres.NewInitialRoutes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: initial route store: %w", err)
	}
	definitions, err := nrpostgres.NewNetworkDefinitions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: network definitions: %w", err)
	}
	applicabilities, err := nrpostgres.NewPlanApplicabilities(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: plan applicabilities: %w", err)
	}
	reassessments, err := nrpostgres.NewRouteReassessments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route reassessments: %w", err)
	}
	handoffLog, err := nrpostgres.NewRouteHandoffLogs(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route handoff log: %w", err)
	}
	identities, err := nrpostgres.NewRouteIdentities(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: route identities: %w", err)
	}

	reassessHandler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        routeStore,
		Evidence:      definitions,
		Applicability: applicabilities,
		Store:         reassessments,
		Log:           handoffLog,
		Identities:    identities,
		Clock:         clock,
		// 自动改路四条件的事实目录没有生产实现。nil 是「显式未配置」的诚实表达，
		// 与端口注释同义：失效照常落库，改路评估整段不做——连建议都不形成，因为
		// 说不出「为什么没自动」。
		AutoReroute: nil,
	})

	adoptions, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: intake adoptions: %w", err)
	}
	intakes, err := nrparcelshipment.NewReassessOnNetworkIntakeAdapter(
		adoptions,
		nrparcelshipment.NewReassessOnIntakeAdapter(reassessHandler, settings.purpose),
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reassess on network intake: %w", err)
	}
	consumer, err := nrinbox.NewNetworkIntakeConsumer(db.Transactor(), inboxStore, intakes)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: network intake consumer: %w", err)
	}
	return consumer, nil
}

// adoptNodeIntakeConsumer 接 UC-PS-003 那条线：NO 节点收寄形成信封 → 消费门 → 按收寄
// 幂等键重读记录 → 按版本化包裹关联反查当前已接受委托（ADR-0060）→ 来源采用编排。
//
// 与前两条的方向相反：信封由 node-operations 发出，消费者在 parcel-shipment 侧
// （ADR-0025 适配器在消费方）。信封只带收寄键，收寄本体由处理方按键重取——载荷里带
// 一份收寄快照会让「权威事实在 NO」这条变成两处定义。
//
// 阶段内容的采用规则版本属实例半边：SourceIdentity 上取不到接受时固定的规则包
// （ADR-0058），因此这里装 UnconfiguredAdoptedStageOwner，资格视图答未配置，编排停在
// `资格判断未决`。默认一个规则包等于替租户宣布这批收寄按哪套资格判断。
func adoptNodeIntakeConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: receptions: %w", err)
	}
	// 同一个仓储对象同时满足聚合仓储与包裹反查两个口：反查读的是 ADR-0060 的当前投影
	// 列，与聚合写在同一张表上，拆两个对象等于让两处各自决定读哪些列。
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: shipment requests: %w", err)
	}
	adoptionStore, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: intake adoptions: %w", err)
	}
	// 取消决定库是真实的：取消边界核验按业务时间裁决（AT-PS-044/080/081），装 nil 会
	// 让「收寄前已取消」的包裹被照常采用，而那条判断的机制半边已经做完了。
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: parcel cancellations: %w", err)
	}
	identities, err := psidentity.NewCommitmentVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: commitment versions: %w", err)
	}
	downstream, err := pspostgres.NewOutboxNetworkIntakeHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: network intake handoff: %w", err)
	}

	stageContent, err := pcpostgres.NewStageContentDeclarations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: stage content declarations: %w", err)
	}
	declared := pspartycommercial.NewDeclaredStageContent(
		stageContent, stageContent, stageContent,
		pspartycommercial.UnconfiguredAdoptedStageOwner{},
	)
	// 第四个入参是取消请求方的格映射，取消编排才用得到；采用这条路径只走 intake 一口。
	// 给它一个能答的替身会假装映射已配置，而没有租户时谁也说不出某个引用是客户还是运营。
	eligibility := pspartycommercial.NewServiceStageRulesAdapter(declared, declared, declared, nil)

	handler := psapplication.NewAdoptNetworkIntakeHandler(psapplication.AdoptNetworkIntakeDeps{
		Requests:      requests,
		Eligibility:   eligibility,
		Adoptions:     adoptionStore,
		Identities:    identities,
		Downstream:    downstream,
		Clock:         clock,
		Cancellations: cancellations,
	})

	processing, err := psnodeops.NewAdoptOnNodeIntakeAdapter(
		receptions, requests, psnodeops.NewNodeIntakeAdapter(handler))
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: adopt on node intake: %w", err)
	}
	consumer, err := psinbox.NewNodeIntakeConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node intake consumer: %w", err)
	}
	return consumer, nil
}
