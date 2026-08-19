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
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	venodeops "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/nodeoperations"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	veapplication "go.idp.xyz/idp-parcel/internal/visibilityexception/application"
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

// veProjectionUndecidedSentinels 只给 VE 投影这一路。与采用那份分开列：两路停的依赖
// 不同，合用一份会把资格未证明也宣布成「等映射」，或把收寄还看不见宣布成「等资格」。
//
// 不在名单里的几格，恢复动作各不相同，保持 publish_failed：
//   - ErrReceptionRecordInconsistent——仓储不变量已破，重投不自愈。
//   - ErrUntranslatableAnswer——词汇表外，编程错误。
//   - ErrProjectionHandoffPending——投影已提交、意图还没交出去，要查 outbox 下游。
//   - veconsume.ErrUnexpectedProjectionOutcome——封闭集合外，静默入账等于替编排作判断。
//
// 毒丸 veinbox.ErrPoisonEnvelope 由消费门入账后交 nil，不要当未决哨兵。
var veProjectionUndecidedSentinels = []error{
	venodeops.ErrReceptionNotVisible,
	venodeops.ErrUnidentifiedHandlingUnit,
	venodeops.ErrProjectionUndecided,
}

// offsitePickupUndecidedSentinels 是揽收采用那条链登记的未决哨兵。与上面那份分开列：
// 两条链的未决面不同，合用一份会把某条链接不住的格子也宣布成「等依赖」。
//
// 三个不在名单里，恢复动作各不相同：
//   - ErrPickupRecordInconsistent——按键取回的登记指着另一个键或另一个对象，是仓储/数据
//     不变量已破（ADR-0029）。重投同一内容不自愈，登记成未决只会一路重试到失败预算耗尽，
//     而现场要做的是去查那一行为什么长成这样。保持 dispatch.publish_failed 让它响亮。
//   - ErrAdoptionHandoffPending——采用行已提交、意图还没交出去。要查的是 outbox 下游收不
//     下那份意图的原因，不是商业资格目录。
//   - ErrAmbiguousParcelTarget——两份当前已接受委托声明了同一个包裹。要人去看为什么，
//     不是等某个依赖到位；机制上也不允许按 latest 挑一份。
var offsitePickupUndecidedSentinels = []error{
	pstf.ErrPickupNotVisible,
	pstf.ErrParcelTargetNotFound,
	pstf.ErrAdoptionUndecided,
}

// effectiveDeliveryUndecidedSentinels 是终局这条链登记的未决哨兵。与揽收那份分开列：
// 未决面是交付可见性、目标委托与终局规则三样，合用揽收名单会把资格未决也宣布成「等
// 依赖」。
//
// 不在名单里的几格，恢复动作各不相同：
//   - ErrDeliveryRecordInconsistent——按键取回的登记指着另一个键或另一个对象，是仓储
//     不变量已破（ADR-0029）。重投同一内容不自愈，登记成未决只会一路重试到失败预算
//     耗尽，而现场要查的是那一行为什么长成这样。保持 dispatch.publish_failed 让它响亮。
//   - ErrFinalHandoffPending——终局行已提交、意图还没交出去。要查的是 outbox 下游收
//     不下那份意图的原因，不是终局规则目录。
//   - ErrAmbiguousParcelTarget——两份当前已接受委托声明了同一个包裹。要人去看为什么，
//     不是等某个依赖到位；机制上也不允许按 latest 挑一份。
//   - ErrUntranslatableAnswer——词汇表外，编程错误。
//   - ErrUnexpectedFinalOutcome——封闭集合外，静默入账等于替编排作判断。
var effectiveDeliveryUndecidedSentinels = []error{
	pstf.ErrDeliveryNotVisible,
	pstf.ErrParcelTargetNotFound,
	pstf.ErrFinalUndecided,
}

// wireDispatcher 接依赖图。它与读环境分开，是为了让组合根能对着真库整体验一遍——
// 一个只能靠进程起停验证的装配点，等于没有验证。
//
// 路由表今天有五类事件：PS 接受决定 → 初始路由（UC-NR-001）、PS 有效网络收寄采用
// 结果 → 路由复核（UC-PS-003 步骤 8 → UC-NR-003）、NO 节点收寄形成 → FanOut（先 VE
// 投影 UC-VE-002，再 PS 来源采用）、TF 对象级场外揽收登记 → PS 来源采用（与节点收寄
// 同属 UC-PS-003 的两个合格物理来源）、TF 有效交付登记 → PS 终局采用（UC-PS-004）。
// 前两条投向 network-routing；节点收寄这一类同一 EventType 投给两个独立消费者，顺序
// 先 VE 后 PS，避免把投影堵在资格墙上。第五条只接 `effective-delivery.registered`，
// 不接 `transport-handover.registered` 或 `offsite-pickup.formed`。
// 不登记 `visibility-exception.tracking-projection.derived`（UC-VE-008）：派生交接会
// 入队，未登记是 ADR-0049 认下的 no_subscriber，不是漏接。
// 登记的仍然只有本进程真接得住的类型——按 ADR-0049 第三条，登记一个接不住的比不登记
// 更糟。其余已发布但无消费者的类型照旧撞 `dispatch.no_subscriber`。
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

	veConsumer, err := deriveProjectionConsumer(db, outboxStore, inboxStore, clock)
	if err != nil {
		return nil, err
	}
	// 各路先包自己的哨兵再 FanOut：禁止把两路哨兵合成一份再包 FanOut。FanOut 不翻译。
	veRouted, err := dispatch.WithUndecidedSentinels(veConsumer, veProjectionUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection undecided translation: %w", err)
	}
	nodeIntakeFan, err := dispatch.FanOut(veRouted, routedAdoptions)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node intake fan-out: %w", err)
	}

	pickups, err := adoptOffsitePickupConsumer(db, outboxStore, inboxStore, clock)
	if err != nil {
		return nil, err
	}
	// 揽收这条链自带一份哨兵名单：它的未决面是揽收登记可见性、目标委托与资格目录三样，
	// 没有节点侧那个「实物还没识别」，而多出一格不变量破坏必须响亮而不是等。
	routedPickups, err := dispatch.WithUndecidedSentinels(pickups, offsitePickupUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: offsite pickup undecided translation: %w", err)
	}

	finals, err := adoptEffectiveDeliveryConsumer(db, outboxStore, inboxStore, clock)
	if err != nil {
		return nil, err
	}
	// 终局这条链的未决面是交付可见性、目标委托与终局规则三样——都是「等一个依赖」
	// 而不是发布失败。键/本体不符、交接待发、反查歧义不在这份名单里。
	routedFinals, err := dispatch.WithUndecidedSentinels(finals, effectiveDeliveryUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: effective delivery undecided translation: %w", err)
	}

	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{
			nrinbox.AcceptedDecisionEventType:            routed,
			nrinbox.AdoptedNetworkIntakeEventType:        routedIntakes,
			psinbox.NodeIntakeFormedEventType:            nodeIntakeFan,
			psinbox.OffsitePickupRegisteredEventType:     routedPickups,
			psinbox.EffectiveDeliveryRegisteredEventType: routedFinals,
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
	// 适用性闭包标识从已接受解析回指（ADR-0064），不再要一份判断键到 RES 的实例映射。
	// 解析库没有 SYN-RES-01 那一行时 found=false，视图答依赖不可用——不要默认适用，
	// 也不要为纵向变绿去种服务产品。
	applicability, err := nrpartycommercial.NewRoutingApplicability(closures)
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
// 编排与委托仓储取自 networkIntakeAdoption，实例半边的停点写在那里。
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
	adoption, err := networkIntakeAdoption(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}

	processing, err := psnodeops.NewAdoptOnNodeIntakeAdapter(
		receptions, adoption.requests, psnodeops.NewNodeIntakeAdapter(adoption.handler))
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: adopt on node intake: %w", err)
	}
	consumer, err := psinbox.NewNodeIntakeConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node intake consumer: %w", err)
	}
	return consumer, nil
}

// deriveProjectionConsumer 接 UC-VE-002 那条线：同一封 NO 节点收寄形成信封 → VE 消费
// 门 → 按收寄幂等键重读记录 → 译成已接受源事实 → 派生追踪投影。
//
// 与 adoptNodeIntakeConsumer 分函数、分 inbox 名：Inbox 键只由（消费者名 + 来源 +
// 事件 ID）认领，塞进采用那路会让投影把采用的投递当重复跳过。
func deriveProjectionConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection receptions: %w", err)
	}
	facts, err := vepostgres.NewAcceptedFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: accepted facts: %w", err)
	}
	projections, err := vepostgres.NewProjections(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projections: %w", err)
	}
	identities, err := veidentity.NewProjectionVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection versions: %w", err)
	}
	downstream, err := vepostgres.NewOutboxProjectionHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection handoff: %w", err)
	}

	processing, err := venodeops.NewDeriveOnNodeIntakeAdapter(receptions, &tenantBoundProjectionDerive{
		db:          db,
		facts:       facts,
		projections: projections,
		identities:  identities,
		downstream:  downstream,
		clock:       clock,
	})
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on node intake: %w", err)
	}
	consumer, err := veinbox.NewNodeIntakeConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive projection consumer: %w", err)
	}
	return consumer, nil
}

// tenantBoundProjectionDerive 在每次 Handle 用命令上的租户构造映射视图。
// MilestoneMappings 在构造期绑租户，ClassifyFact 签名没有租户；派发进程是多租户。
// 禁止 NewMilestoneMappings(db, 空租户)——看起来接了库、永远读不到行。该租户目录空
// → found=false → 编排 PROJECTION_DERIVED + 未归类（MAPPING_NOT_CONFIGURED）。
type tenantBoundProjectionDerive struct {
	db          *bentopg.DB
	facts       *vepostgres.AcceptedFacts
	projections *vepostgres.Projections
	identities  *veidentity.ProjectionVersions
	downstream  *vepostgres.OutboxProjectionHandoff
	clock       systemClock
}

var _ venodeops.ProjectionHandler = (*tenantBoundProjectionDerive)(nil)

func (derive *tenantBoundProjectionDerive) Handle(
	ctx context.Context,
	command veapplication.DeriveProjectionCommand,
) (veapplication.DeriveProjectionResult, error) {
	if command.TenantID.String() == "" {
		return veapplication.DeriveProjectionResult{}, errors.New("parcel-dispatch: projection tenant is empty")
	}
	mapping, err := vepostgres.NewMilestoneMappings(derive.db, command.TenantID)
	if err != nil {
		return veapplication.DeriveProjectionResult{}, err
	}
	return veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:       derive.facts,
		Mapping:     mapping,
		Projections: derive.projections,
		Identities:  derive.identities,
		Downstream:  derive.downstream,
		Clock:       derive.clock,
	}).Handle(ctx, command)
}

// adoptOffsitePickupConsumer 接 UC-PS-003 的另一个合格物理来源：TF 对象级场外揽收登记
// 信封 → 消费门 → 按（租户+对象+尝试）重读登记 → 按包裹反查当前已接受委托（ADR-0060）
// → 同一个来源采用编排。
//
// 只接对象级的 `offsite-pickup.registered`，不接尝试级的 `offsite-pickup.formed`：后者
// 一封信带一批成功对象，而采用判断逐对象成立，两条都登记会让同一份揽收结果被采用两次
// （第二次落已有结果），把一次事实变成两处消费。
//
// 信封只带揽收登记键，揽收本体由处理适配器按键重取——权威事实留在 transport-fulfillment，
// 载荷里带一份快照会让它变成两处定义。
func adoptOffsitePickupConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	registrations, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: offsite pickup registrations: %w", err)
	}
	adoption, err := networkIntakeAdoption(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}

	processing, err := pstf.NewAdoptOnOffsitePickupAdapter(
		registrations, adoption.requests, pstf.NewOffsitePickupAdapter(adoption.handler))
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: adopt on offsite pickup: %w", err)
	}
	consumer, err := psinbox.NewOffsitePickupConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: offsite pickup consumer: %w", err)
	}
	return consumer, nil
}

// adoptEffectiveDeliveryConsumer 接 UC-PS-004 那条线：TF 有效交付登记信封 → 消费门 →
// 按（租户+对象+尝试）重读当前版 → 按包裹反查当前已接受委托（ADR-0060）→ 终局编排。
//
// 只接 `effective-delivery.registered`。信封只带键，交付本体由处理适配器按键重取当前
// 版——版本进事件 ID 是为了两代入队，不从 ID 回解析去查已翻旧的行。
//
// 终局规则从已接受委托的解析标识回指提供方闭包，与收寄采用同一条 ADR-0062 纪律，但
// 编排与仓储刻意自建：不要从 networkIntakeAdoption 掏 Eligibility，两条链的停点（收寄
// 资格 vs 终局规则）必须各自诚实，合用一份 handler 会把终局未配置讲成资格未成立。
func adoptEffectiveDeliveryConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: effective deliveries: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: shipment requests: %w", err)
	}
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: final outcomes: %w", err)
	}
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: parcel cancellations: %w", err)
	}
	identities, err := psidentity.NewFinalOutcomeVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: final outcome versions: %w", err)
	}
	downstream, err := pspostgres.NewOutboxFinalOutcomeHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: final outcome handoff: %w", err)
	}

	stageContent, err := pcpostgres.NewStageContentDeclarations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: stage content declarations: %w", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: commercial resolutions: %w", err)
	}
	owners, err := pspartycommercial.NewResolvedAdoptedStageOwner(requests, resolutions)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: adopted stage owner: %w", err)
	}
	declared := pspartycommercial.NewDeclaredStageContent(
		stageContent, stageContent, stageContent, owners,
	)
	// 第四个入参是取消请求方的格映射，取消编排才用得到；终局这条路径只走 final 一口。
	// 第五个入参是硬资格证据口：终局判断不走它，但仍要显式未配置——nil 会在非空清单上
	// 变成依赖错误。不得为变绿去种 PAR-COM-17 终局声明行。
	rules := pspartycommercial.NewServiceStageRulesAdapter(
		declared, declared, declared, nil,
		pspartycommercial.UnconfiguredIntakeQualificationEvidence{},
	)

	handler := psapplication.NewFormParcelFinalHandler(psapplication.FormParcelFinalDeps{
		Requests:      requests,
		Rules:         rules,
		Finals:        finals,
		Cancellations: cancellations,
		Identities:    identities,
		Downstream:    downstream,
		Clock:         clock,
	})

	processing, err := pstf.NewAdoptOnEffectiveDeliveryAdapter(
		deliveries, requests, pstf.NewDeliveryOutcomeAdapter(handler))
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: adopt on effective delivery: %w", err)
	}
	consumer, err := psinbox.NewEffectiveDeliveryConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: effective delivery consumer: %w", err)
	}
	return consumer, nil
}

// networkIntakeAdoption 是两条采用消费链共用的编排与委托仓储。
//
// 与 acceptanceConsumer / networkIntakeConsumer 各建各的仓储不同，这一份刻意共用：里面
// 每个 nil 与「未配置」都是判断，而两条链必须给出同一个答案。采用规则版本从已接受委托
// 的解析标识回指提供方闭包（ADR-0062），因此装 ResolvedAdoptedStageOwner：身份上仍取不
// 到规则对象（ADR-0058 第三条），但已接受快照上的 ResolutionID 可以回指。PC 解析库没有
// 那一行时 found=false，资格视图答未配置——不要默认一个规则包，也不要为纵向变绿去种。
// 各写一份的坏处很具体：日后有人只给一条链换上真实规则包，另一条会静默停在未决，
// 而两条本该同时越过同一道闸。
type networkIntakeAdoptionGraph struct {
	// requests 同时满足聚合仓储与包裹反查两个口：反查读的是 ADR-0060 的当前投影列，与
	// 聚合写在同一张表上，拆两个对象等于让两处各自决定读哪些列。
	requests *pspostgres.ShipmentRequests
	handler  *psapplication.AdoptNetworkIntakeHandler
}

func networkIntakeAdoption(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
) (networkIntakeAdoptionGraph, error) {
	none := networkIntakeAdoptionGraph{}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: shipment requests: %w", err)
	}
	adoptionStore, err := pspostgres.NewIntakeAdoptions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: intake adoptions: %w", err)
	}
	// 取消决定库是真实的：取消边界核验按业务时间裁决（AT-PS-044/080/081），装 nil 会
	// 让「收寄前已取消」的包裹被照常采用，而那条判断的机制半边已经做完了。
	cancellations, err := pspostgres.NewParcelCancellations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: parcel cancellations: %w", err)
	}
	identities, err := psidentity.NewCommitmentVersions()
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: commitment versions: %w", err)
	}
	downstream, err := pspostgres.NewOutboxNetworkIntakeHandoff(db, outboxStore, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: network intake handoff: %w", err)
	}

	stageContent, err := pcpostgres.NewStageContentDeclarations(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: stage content declarations: %w", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: commercial resolutions: %w", err)
	}
	owners, err := pspartycommercial.NewResolvedAdoptedStageOwner(requests, resolutions)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: adopted stage owner: %w", err)
	}
	declared := pspartycommercial.NewDeclaredStageContent(
		stageContent, stageContent, stageContent, owners,
	)
	// 第四个入参是取消请求方的格映射，取消编排才用得到；采用这条路径只走 intake 一口。
	// 给它一个能答的替身会假装映射已配置，而没有租户时谁也说不出某个引用是客户还是运营。
	// 第五个入参是硬资格证据口（ADR-0063）：显式未配置答未证明，nil 会在非空清单上变成
	// 依赖错误，两者都不得默认 ESTABLISHED。
	eligibility := pspartycommercial.NewServiceStageRulesAdapter(
		declared, declared, declared, nil,
		pspartycommercial.UnconfiguredIntakeQualificationEvidence{},
	)

	return networkIntakeAdoptionGraph{
		requests: requests,
		handler: psapplication.NewAdoptNetworkIntakeHandler(psapplication.AdoptNetworkIntakeDeps{
			Requests:      requests,
			Eligibility:   eligibility,
			Adoptions:     adoptionStore,
			Identities:    identities,
			Downstream:    downstream,
			Clock:         clock,
			Cancellations: cancellations,
		}),
	}, nil
}
