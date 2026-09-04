package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	nrinbox "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/inbox"
	nrparcelshipment "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/parcelshipment"
	nrpartycommercial "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/partycommercial"
	nrpostgres "go.idp.xyz/idp-parcel/internal/networkrouting/adapters/postgres"
	nrapplication "go.idp.xyz/idp-parcel/internal/networkrouting/application"
	nrdomain "go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	psidentity "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/identity"
	psinbox "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/inbox"
	psnetworkrouting "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/networkrouting"
	psnodeops "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/nodeoperations"
	pspartycommercial "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/partycommercial"
	pspostgres "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/postgres"
	pssettlement "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/settlementaccounting"
	pstf "go.idp.xyz/idp-parcel/internal/parcelshipment/adapters/transportfulfillment"
	psapplication "go.idp.xyz/idp-parcel/internal/parcelshipment/application"
	psports "go.idp.xyz/idp-parcel/internal/parcelshipment/ports"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	pcapplication "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	"go.idp.xyz/idp-parcel/internal/platform/dispatch"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sapartycommercial "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/partycommercial"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	saapplication "go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	vecc "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/customscompliance"
	veidentity "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/identity"
	veinbox "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/inbox"
	venr "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/networkrouting"
	venodeops "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/nodeoperations"
	veps "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/parcelshipment"
	vepostgres "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	vetf "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/transportfulfillment"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/veconsume"
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

// assembleDispatcher 是派发一拍的装配点：读部署形态、开连接池、验启动就绪、接完整
// 依赖图。
//
// 交回的第二个值是收尾函数，进程停机时调。装配中途失败时连接池就地关掉——半开的池
// 会在下一次装配尝试时耗掉连接数，而那种耗尽看起来像数据库出了问题。
//
// 就绪检查放在这里而不是交给 Loop，是因为两类失败要运维做的事相反。`Loop.Run` 对
// 一拍失败只记不停是对的，运行中的依赖抖动不该拖死进程；但错的 DSN、库不可达、缺
// 框架 schema 都是部署本身坏了，让它经同一条路径表现，就与抖动在日志里长成同一种
// 东西——进程照常常驻，每拍报一次错，没人看得出该去改部署还是去等上游。装配点失败
// 让进程带着原因退出，这两格才分得开。
//
// 业务迁移是否施加齐全没有第三道检查：`migrate` 今天只导出施加计划的 `Run`，它要一
// 条独占连接、会建表，而那个包明写自己从不在应用启动时运行；没有可用的只读状态读口。
// 因此业务表缺失这一格仍会起得来并按每拍报错表现——要补得先给 `migrate` 定出只读
// 状态口，那是另一件事。
// logger 只用来构造失败观察口（ADR-0095）。收 logger 而不是收一个现成的观察口，是因为
// 「怎么出声」本就该在这一层定：平台层交出原始错误，装配点决定级别与维度。传 nil 即不观察，
// 行为与加这道缝之前逐字相同。
func assembleDispatcher(
	ctx context.Context,
	getenv func(string) string,
	logger *slog.Logger,
) (Beat, func(), error) {
	settings, err := settingsFromEnv(getenv)
	if err != nil {
		return nil, nil, err
	}
	pool, err := pgxpool.New(ctx, settings.databaseDSN)
	if err != nil {
		return nil, nil, fmt.Errorf("parcel-dispatch: connect: %w", err)
	}
	// 建池是惰性的：`pgxpool.New` 只解析连接串，不实际连库，因此错的 DSN 到这里
	// 一声不响。库可达性由这一次 Ping 认定。
	//
	// 不在这里加拨号超时常量：那是部署形态，写死一个数就是替租户定了启动等多久。
	// 要限时的部署把 `connect_timeout` 写进 DSN，pgx 认它；ctx 则由停机信号驱动，
	// 启动途中收到 SIGTERM 就当场停下。
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-dispatch: ping: %w", err)
	}
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-dispatch: framework db: %w", err)
	}
	// `CheckSchema` 是 bento 自带的只读形状检查，排在 Ping 之后：库连不上时它一样
	// 会失败，但失败发生在它自己开只读事务那一步，成因被记在「schema check」名下，
	// 运维会去查迁移而不是查连通。
	if err := db.CheckSchema(ctx); err != nil {
		pool.Close()
		return nil, nil, fmt.Errorf("parcel-dispatch: schema check: %w", err)
	}
	beat, err := wireDispatcher(db, settings, dispatch.WithDeliveryFailureObserver(logDeliveryFailure(logger)))
	if err != nil {
		pool.Close()
		return nil, nil, err
	}
	return beat, pool.Close, nil
}

// acceptanceChainUndecidedSentinels 是接受判断链这条路登记的未决哨兵。名单只有一格：
// 三步各自的未决（商业依据、时点策略、可达性权威、财务控制、人工复核……）在编排里已经
// 收成同一个「本轮没形成决定」，消费门这一侧只认得那一格，停在哪一步与原因都在错误正文里。
//
// 两个不在名单里，恢复动作与「等一个依赖」相反：
//   - psinbox.ErrUnexpectedAcceptanceChainOutcome——编排交回了封闭集合以外的结果，是
//     编程错误。登记成未决只会一路重投到失败预算耗尽，而重投改不了它。
//   - psapplication.ErrAcceptanceChainNotAssembled / ErrAcceptanceChainHasNoMembers——
//     前者是本文件漏接了一步，后者是发布侧发了一份没有声明成员的委托。两者都不会因为
//     等下去而长出来，保持 dispatch.publish_failed 让它们响亮。
var acceptanceChainUndecidedSentinels = []error{
	psinbox.ErrAcceptanceChainUndecided,
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

// vePickupUndecidedSentinels 只给 VE 揽收投影这一路。与 PS 采用那份分开列，也不与
// 节点收寄投影合用——揽收没有「实物还没识别」，多出来的 inconsistent 不得宣布成等依赖。
//
// 不在名单里：ErrPickupRecordInconsistent、ErrPickupUntranslatableAnswer、
// ErrPickupProjectionHandoffPending、veconsume.ErrUnexpectedProjectionOutcome。
var vePickupUndecidedSentinels = []error{
	vetf.ErrPickupNotVisible,
	vetf.ErrPickupProjectionUndecided,
}

// veDeliveryUndecidedSentinels 只给 VE 交付投影这一路。与 PS 终局那份分开列。
//
// 不在名单里：ErrDeliveryRecordInconsistent、ErrUntranslatableAnswer、
// ErrProjectionHandoffPending、veconsume.ErrUnexpectedProjectionOutcome。
var veDeliveryUndecidedSentinels = []error{
	vetf.ErrDeliveryNotVisible,
	vetf.ErrProjectionUndecided,
}

// veHandoverUndecidedSentinels 只给 VE 交接投影这一路。本路不 FanOut 给 PS：终局只认
// 有效交付；NO/NR 控制转移不在本票。
//
// 不在名单里：ErrHandoverRecordInconsistent、ErrHandoverUntranslatableAnswer、
// ErrHandoverProjectionHandoffPending、veconsume.ErrUnexpectedProjectionOutcome。
var veHandoverUndecidedSentinels = []error{
	vetf.ErrHandoverNotVisible,
	vetf.ErrHandoverProjectionUndecided,
}

// veExternalTrackingUndecidedSentinels 只给 VE 外部承运轨迹投影这一路（label-channel/16）。
//
// 不在名单里：ErrExternalTrackingRecordInconsistent——它含「待判断版本到了这里」那一格，那是
// TF 侧交接口的缺陷不是等谁（ADR-0102 决定三：待判断的不提供给 VE）；以及
// ErrExternalTrackingUntranslatableAnswer、ErrExternalTrackingProjectionHandoffPending、
// veconsume.ErrUnexpectedProjectionOutcome。
var veExternalTrackingUndecidedSentinels = []error{
	vetf.ErrExternalTrackingNotVisible,
	vetf.ErrExternalTrackingProjectionUndecided,
}

// veFinalOutcomeUndecidedSentinels 只给 VE 终局投影这一路。与上面四份分开列：本路
// 的未决面只有终局登记可见性与派生未决两样，合用别路名单会把不存在的格子也宣布成
// 「等依赖」。
//
// 不在名单里：ErrFinalRecordInconsistent（仓储不变量已破，含采认时刻为零）、
// ErrFinalUntranslatableAnswer（词汇表外，编程错误）、ErrFinalProjectionHandoffPending
// （要查 outbox 下游）、veconsume.ErrUnexpectedProjectionOutcome。
var veFinalOutcomeUndecidedSentinels = []error{
	veps.ErrFinalNotVisible,
	veps.ErrFinalProjectionUndecided,
}

// veInitialRouteUndecidedSentinels 只给 VE 初始路由投影这一路。本路不 FanOut：按消费
// 清点该信封的应消费方还有 NO/TF，但两侧消费者今天不存在——登记接不住的比不登记更糟
// （ADR-0049 第三条），照交接路的先例只投 VE。
//
// 不在名单里：ErrInitialRouteRecordInconsistent、ErrInitialRouteUntranslatableAnswer、
// ErrInitialRouteProjectionHandoffPending、veconsume.ErrUnexpectedProjectionOutcome。
var veInitialRouteUndecidedSentinels = []error{
	venr.ErrInitialRouteNotVisible,
	venr.ErrInitialRouteProjectionUndecided,
}

// veExceptionJourneyUndecidedSentinels 只给 VE 异常旅程投影这一路。本路不 FanOut：
// 旅程启动是 TF 自家过程事实，PS 侧今天没有它的消费者——登记接不住的比不登记更糟
// （ADR-0049 第三条），照交接路先例只投 VE。
//
// 不在名单里：ErrExceptionJourneyRecordInconsistent（仓储不变量已破，含成员为空）、
// ErrExceptionJourneyUntranslatableAnswer（词汇表外，编程错误）、
// ErrJourneyProjectionHandoffPending（要查 outbox 下游）、
// veconsume.ErrUnexpectedProjectionOutcome。
var veExceptionJourneyUndecidedSentinels = []error{
	vetf.ErrExceptionJourneyNotVisible,
	vetf.ErrJourneyProjectionUndecided,
}

// veCustomsCaseUndecidedSentinels 只给 VE 关务案件投影这一路。本路不 FanOut：案件
// 建立是 CC 自家责任容器事实，PS 侧今天没有它的消费者——登记接不住的比不登记更糟
// （ADR-0049 第三条），照旅程路先例只投 VE。
//
// 不在名单里：ErrCustomsCaseRecordInconsistent（仓储不变量已破，含成员关联为空）、
// ErrCustomsCaseUntranslatableAnswer（词汇表外，编程错误）、
// ErrProjectionHandoffPending（要查 outbox 下游）、veconsume.ErrUnexpectedProjectionOutcome。
var veCustomsCaseUndecidedSentinels = []error{
	vecc.ErrCustomsCaseNotVisible,
	vecc.ErrProjectionUndecided,
}

// veDeclarationSubmissionUndecidedSentinels 只给 VE 申报提交投影这一路。本路不
// FanOut：提交版本是 CC 自家申报链事实，PS 侧今天没有它的消费者——登记接不住的比
// 不登记更糟（ADR-0049 第三条），照案件路先例只投 VE。
//
// 不在名单里：ErrDeclarationSubmissionRecordInconsistent（仓储不变量已破，含版本
// 身份与载荷宣告不符、成员快照为空）、ErrDeclarationSubmissionUntranslatableAnswer
// （词汇表外，编程错误）、ErrProjectionHandoffPending（要查 outbox 下游）、
// veconsume.ErrUnexpectedProjectionOutcome。
var veDeclarationSubmissionUndecidedSentinels = []error{
	vecc.ErrDeclarationSubmissionNotVisible,
	vecc.ErrProjectionUndecided,
}

// veCustomerViewUndecidedSentinels 只给「投影派生 → 客户视图」这一路。
//
// 歧义账户也在名单里：同包裹被多份已接受委托同时声明是机制拒绝自动采认（ADR-0060、
// AT-VE-152），运维要去 PS 侧解开歧义，解开前这封信如实卡着——不任选，也不折成
// 「无视图」。不在名单里：ErrDerivedProjectionUntranslatable 与
// ErrDerivedProjectionInconsistent（引用坏了 / 仓储不变量已破，编程错误）、
// ErrCustomerAccountUntranslatable（同前）、veconsume.ErrCustomerViewHandoffPending
// （要查 outbox 下游）、veconsume.ErrUnexpectedCustomerViewOutcome。
var veCustomerViewUndecidedSentinels = []error{
	veps.ErrDerivedProjectionUnreadable,
	veps.ErrCustomerAccountUnavailable,
	veps.ErrAmbiguousCustomerAccount,
	veconsume.ErrCustomerViewUndecided,
}

// veAcceptanceRederiveUndecidedSentinels 只给「接受决定 → 客户归属确立补派生」这一路
// （UC-VE-008 AT-VE-169）。与投影派生那路分开列：本路多一格「声明清单读口调不通」；
// 歧义、投影库、反查口与派生编排四格与那路同义——歧义仍是机制拒绝自动采认（ADR-0060、
// AT-VE-152），运维去 PS 侧解开歧义，解开前这封信如实卡着，不任选也不折成「无视图」。
//
// 不在名单里的几格，恢复动作各不相同，保持 publish_failed：
//   - veps.ErrAcceptanceDecisionUntranslatable——集合外状态字或引用坏了，编程错误。
//   - veps.ErrAcceptanceRecordInconsistent——信封在而委托行不在 / 成员在清单里而反查
//     零行，两次读自相矛盾，仓储不变量已破，重投不自愈。
//   - veps.ErrCustomerAccountMismatch——信封账户与权威反查不符。基线上结构不可达
//     （已接受撤不了换不了代、修订不动成员不动账户），到达即绕过领域直写库，硬失败。
//   - veconsume.ErrCustomerViewHandoffPending——要查 outbox 下游。
//   - veconsume.ErrUnexpectedCustomerViewOutcome——封闭集合外。
var veAcceptanceRederiveUndecidedSentinels = []error{
	veps.ErrDeclaredParcelsUnavailable,
	veps.ErrDerivedProjectionUnreadable,
	veps.ErrCustomerAccountUnavailable,
	veps.ErrAmbiguousCustomerAccount,
	veconsume.ErrCustomerViewUndecided,
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
// 路由表登记这些事件：PS 委托已提交 → 接受判断链、PS 复核已完成 → 同一条接受判断
// 链的续办门（ADR-0086；两扇门同一编排、各记各的 inbox 账）、
// PS 接受决定 → FanOut（先 VE 客户归属确立补派生 UC-VE-008
// AT-VE-169，再 NR 初始路由 UC-NR-001）、PS 有效网络收寄采用结果 → 路由复核
// （UC-PS-003 步骤 8 → UC-NR-003）、NO 节点收寄形成 → FanOut（先 VE 投影 UC-VE-002，
// 再 PS 来源采用）、TF 对象级场外揽收登记 → FanOut（先 VE 投影，再 PS 来源采用）、
// TF 有效交付登记 → FanOut（先 VE 投影，再 PS 终局 UC-PS-004）、
// TF 权威交接登记 → 只投 VE 投影（不 FanOut 给 PS：终局只认有效交付）、
// PS 包裹服务终局形成 → 只投 VE 投影（不 FanOut：终局是 PS 自家事实，让它经调度器
// 消费自己等于把一份事实记两遍）、NR 包裹级初始路由判断 → 只投 VE 投影（不 FanOut：
// 按消费清点该信封的应消费方还有 NO/TF，但两侧消费者今天不存在，登记接不住的比不
// 登记更糟）、TF 替代/退运旅程启动 → 只投 VE 投影（不 FanOut：一封信带全体成员，
// 消费侧按成员循环拆分派生，ADR-0066）、CC 关务案件建立 → 只投 VE 投影（不 FanOut：
// 同为多成员信封，成员维与案件维一并进事实引用，ADR-0066）、CC 申报提交版本形成 →
// 只投 VE 投影（不 FanOut：同为多成员信封，成员维进引用、提交版本走版本维，
// ADR-0066）、VE 投影派生 → 客户视图
// （UC-VE-008 内部半边：账户维经 PS 按包裹反查填上，ADR-0060 三格）。
// 「PS 有效网络收寄采用结果」那一路单投 network-routing；各类 FanOut 同一 EventType
// 各投两个独立消费者，顺序一律先 VE 后 PS/NR，避免把投影或补派生堵在资格墙、终局
// 规则墙或路由证据墙上。
// 「TF 有效交付登记」只接 `effective-delivery.registered`，不接 `offsite-pickup.formed`。
// 「TF 权威交接登记」只接 `transport-handover.registered`，不接 PS。
//
// 上面几处一律按名字指，不按「第几条」——本注释的枚举刚被 ADR-0086 的两扇门从头部
// 顶偏过一次（原三处序数在 1665fdb 上还都是对的，加了「委托已提交」与「复核已完成」
// 之后同时错位两位），而 build、vet 与全仓 test 对此零信号。
// `visibility-exception.tracking-projection.derived` 已登记（UC-VE-008）：早先不登记
// 的理由是 Customer 那一维填不上；ADR-0060 的按包裹反查把账户随来源身份一并交回之后
// 本进程真接得住它了——接得住才登记，正是 ADR-0049 第三条的判据。
// 登记的仍然只有本进程真接得住的类型——按 ADR-0049 第三条，登记一个接不住的比不登记
// 更糟。其余已发布但无消费者的类型照旧撞 `dispatch.no_subscriber`。
// options 用变参收：多数调用点（含各真库装配用例）不需要观察口，变参让它们一个都不必改，
// 与 dispatch.NewDispatcher 那一处同一手法（ADR-0095 Decision 四）。
func wireDispatcher(db *bentopg.DB, settings dispatchSettings, options ...dispatch.Option) (Beat, error) {
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

	chainGates, err := acceptanceChainConsumers(db, outboxStore, inboxStore, settings, clock)
	if err != nil {
		return nil, err
	}
	routedChain, err := dispatch.WithUndecidedSentinels(chainGates.submitted, acceptanceChainUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance chain undecided translation: %w", err)
	}
	// 续办门与提交门同一份哨兵：两扇门转交同一个编排，未决面完全相同（ADR-0086 第四条
	// ——重跑停在其它未决即回滚重投）。各列一份就会有漂开的那一天。
	routedResume, err := dispatch.WithUndecidedSentinels(chainGates.reviewCompleted, acceptanceChainUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: review resume undecided translation: %w", err)
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

	veAcceptance, err := deriveCustomerViewOnAcceptanceConsumer(db, outboxStore, inboxStore, clock)
	if err != nil {
		return nil, err
	}
	veAcceptanceRouted, err := dispatch.WithUndecidedSentinels(
		veAcceptance, veAcceptanceRederiveUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive undecided translation: %w", err)
	}
	// 先 VE 后 NR：NR 腿今天必撞路由证据实例墙（ROUTE_EVIDENCE_NOT_CONFIGURED），
	// 反序会把补派生一直堵在墙外。VE 腿空转或成功不改变 NR 腿的未决记账；VE 腿硬失败
	// 把整格升成 publish_failed，那是 failureCodeFor 分格的正确行为，不在这里遮。
	acceptanceFan, err := dispatch.FanOut(veAcceptanceRouted, routed)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance fan-out: %w", err)
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

	projectionDerive, err := newTenantBoundProjectionDerive(db, outboxStore, clock)
	if err != nil {
		return nil, err
	}

	veConsumer, err := deriveProjectionConsumer(db, inboxStore, projectionDerive)
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
	vePickup, err := derivePickupConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	vePickupRouted, err := dispatch.WithUndecidedSentinels(vePickup, vePickupUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: pickup projection undecided translation: %w", err)
	}
	pickupFan, err := dispatch.FanOut(vePickupRouted, routedPickups)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: offsite pickup fan-out: %w", err)
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
	veDelivery, err := deriveDeliveryConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veDeliveryRouted, err := dispatch.WithUndecidedSentinels(veDelivery, veDeliveryUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: delivery projection undecided translation: %w", err)
	}
	deliveryFan, err := dispatch.FanOut(veDeliveryRouted, routedFinals)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: effective delivery fan-out: %w", err)
	}

	veHandover, err := deriveHandoverConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veHandoverRouted, err := dispatch.WithUndecidedSentinels(veHandover, veHandoverUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: handover projection undecided translation: %w", err)
	}

	veExternalTracking, err := deriveExternalTrackingConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veExternalTrackingRouted, err := dispatch.WithUndecidedSentinels(
		veExternalTracking, veExternalTrackingUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: external tracking projection undecided translation: %w", err)
	}

	veFinalOutcome, err := deriveFinalOutcomeConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veFinalOutcomeRouted, err := dispatch.WithUndecidedSentinels(veFinalOutcome, veFinalOutcomeUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: final outcome projection undecided translation: %w", err)
	}

	veInitialRoute, err := deriveInitialRouteConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veInitialRouteRouted, err := dispatch.WithUndecidedSentinels(veInitialRoute, veInitialRouteUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: initial route projection undecided translation: %w", err)
	}

	veExceptionJourney, err := deriveExceptionJourneyConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veExceptionJourneyRouted, err := dispatch.WithUndecidedSentinels(
		veExceptionJourney, veExceptionJourneyUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: exception journey projection undecided translation: %w", err)
	}

	veCustomsCase, err := deriveCustomsCaseConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veCustomsCaseRouted, err := dispatch.WithUndecidedSentinels(
		veCustomsCase, veCustomsCaseUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customs case projection undecided translation: %w", err)
	}

	veDeclarationSubmission, err := deriveDeclarationSubmissionConsumer(db, inboxStore, projectionDerive)
	if err != nil {
		return nil, err
	}
	veDeclarationSubmissionRouted, err := dispatch.WithUndecidedSentinels(
		veDeclarationSubmission, veDeclarationSubmissionUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: declaration submission projection undecided translation: %w", err)
	}

	veCustomerView, err := deriveCustomerViewConsumer(db, outboxStore, inboxStore, clock)
	if err != nil {
		return nil, err
	}
	veCustomerViewRouted, err := dispatch.WithUndecidedSentinels(
		veCustomerView, veCustomerViewUndecidedSentinels...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer view undecided translation: %w", err)
	}

	publisher, err := dispatch.NewDirectPublisher(
		map[eventing.EventType]dispatch.Consumer{
			psinbox.ShipmentRequestSubmittedEventType:      routedChain,
			psinbox.ManualReviewCompletedEventType:         routedResume,
			nrinbox.AcceptedDecisionEventType:              acceptanceFan,
			nrinbox.AdoptedNetworkIntakeEventType:          routedIntakes,
			psinbox.NodeIntakeFormedEventType:              nodeIntakeFan,
			psinbox.OffsitePickupRegisteredEventType:       pickupFan,
			psinbox.EffectiveDeliveryRegisteredEventType:   deliveryFan,
			veinbox.TransportHandoverRegisteredEventType:   veHandoverRouted,
			veinbox.ExternalCarrierTrackingJudgedEventType: veExternalTrackingRouted,
			veinbox.FinalOutcomeFormedEventType:            veFinalOutcomeRouted,
			veinbox.InitialRouteFormedEventType:            veInitialRouteRouted,
			veinbox.ExceptionJourneyRecordedEventType:      veExceptionJourneyRouted,
			veinbox.CustomsCaseEstablishedEventType:        veCustomsCaseRouted,
			veinbox.DeclarationSubmissionFormedEventType:   veDeclarationSubmissionRouted,
			veinbox.TrackingProjectionDerivedEventType:     veCustomerViewRouted,
		},
		settings.deliveryTimeout,
		settings.config,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: direct publisher: %w", err)
	}

	dispatcher, err := dispatch.NewDispatcher(outboxStore, outboxStore, publisher, clock, settings.config, options...)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: dispatcher: %w", err)
	}
	return dispatcher, nil
}

// logDeliveryFailure 是 ADR-0095 Decision 二那个观察口在本装配点的实现。
//
// 出声这件事归装配方：平台层对事件类型一无所知、十个上下文共用同一拍，它只把手上那个原始
// 错误交出来。这里决定用哪个 logger、什么级别、带哪几个维度。
//
// 带上 `error` 是要害。库里那一行只留得下失败码，而失败码按运维要做的动作取值，答不出「停在
// 哪一站」；消费方恰恰把停站与未决原因写在错误正文里（见 psinbox 的
// `advanceAcceptanceChainThrough`）。此前它被派发器就地丢弃，于是那些字没有任何人读得到——
// 实测 dispatch 连跑 11 分钟、库里累计 24 次失败、进程输出零行。
//
// 用 Warn 不用 Error：这一条失败已经入账且会按重投节奏再来，进程本身没坏。整拍失败才是
// Error，那一格由 `Loop.report` 管。
//
// 会按重投节奏重复出现，这是真实的，不在这里压噪——要压由部署形态决定，而只有装配方知道
// 这个部署的节奏。
func logDeliveryFailure(logger *slog.Logger) dispatch.DeliveryFailureObserver {
	if logger == nil {
		return nil
	}
	return func(delivery eventing.Delivery, code eventing.FailureCode, err error) {
		logger.Warn("Delivery failed and was recorded for retry",
			"eventType", delivery.Envelope.Type,
			"subject", delivery.Envelope.Subject,
			"eventId", delivery.Envelope.ID,
			"attempt", delivery.Attempt,
			"failures", delivery.Failures,
			"failureCode", code,
			"error", err)
	}
}

// acceptanceChainGates 是接受判断链的两扇消费门：同一个编排，两个触发时机（ADR-0086
// ——「委托已提交」开局，「复核已完成」续办）。两扇门各占各的 inbox 名与事件类型，
// 但依赖图必须同一份，所以由同一个装配函数一次建成。
type acceptanceChainGates struct {
	submitted       dispatch.Consumer
	reviewCompleted dispatch.Consumer
}

// acceptanceChainConsumers 接 UC-PS-001 步骤 8 那条线：PS「委托已提交」信封 → 消费门 →
// 接受判断链（逐成员可达性 → 整份委托财务控制 → 形成决定）；外加 ADR-0086 的续办门：
// PS「复核已完成」信封 → 同一条链再驱一拍（前两步读回已记录判断即过，形成决定读到已
// 完成的复核即成决定）。
//
// 它是本进程里**信封驱动本上下文自己**的链：发布侧是 parcel-shipment 的提交事务（续办
// 那扇是 cmd/parcel-api 的复核完成事务），消费侧也是 parcel-shipment。之所以不做成 HTTP
// 端点，是因为「提交之后自动推进接受判断」在用例里是提交的后继，不是外部再发一条命令——
// 由调用方显式推进会把「谁来调它」变成新的实例半边问题，而 outbox/inbox 的续办语义正好
// 是这条链需要的（未决整笔回滚等重投；停等复核那一格的例外见 ADR-0086）。
//
// 三步共用一个商业依据适配器实例，不是省事：形成决定那一步要按**判断当初采用的那份解析**
// 重校验（UC-PC-002 步骤 8），三步各建一个适配器不改变行为，但会让「三条腿问的是同一个
// 权威」这件事在装配上看不出来，下一个改这里的人很容易给某一条腿换上另一份配置。
//
// 实例半边全部留空，各自的「显式未配置」形状各归各口——今天没有租户，这些参数一个也说
// 不出，而每一个空位都是首发该停下的地方，不是要绕过的地方：
//
//   - 时点取值源（AsOfValueSource）nil——时点停在`未配置`，判断不发起；
//   - 可达性闭包标识（ReachabilityClosureIdentity）nil——资格视图答未配置，判断`未形成`；
//   - 结算账户目录（SettlementAccountDirectory）与控制金额源（ControlAmountSource）nil
//     ——控制停在 `CONTROL_SCOPE_NOT_CONFIGURED` / `CONTROL_AMOUNT_NOT_CONFIGURED`，绝不
//     代拟一个账户或拿零去占客户资金。
//
// 解析键登记面（Keys）反而接真：它是本上下文自己的登记表，`parcel-commercial
// register-resolution-key` 已经能往里写，空册时按「显式未配置」答`解析未决`。接上它与留
// nil 的区别不在结果在来源——恢复动作从「写代码」变成「登记参数」（ADR-0063）。
func acceptanceChainConsumers(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	settings dispatchSettings,
	clock systemClock,
) (acceptanceChainGates, error) {
	none := acceptanceChainGates{}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: acceptance chain shipment requests: %w", err)
	}
	// 判断库一物两用：AcceptanceJudgmentRecorder（前两步写）与 RecordedJudgmentReader
	// （形成决定读）读写的是同一批判断行。拆两个对象等于让写的那半与读的那半各自决定
	// 认哪些列，而形成决定要的正是「前两步刚记下的那几条」。
	judgments, err := pspostgres.NewAcceptanceJudgments(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: acceptance judgments: %w", err)
	}

	commercial, err := acceptanceCommercialBasis(db, clock)
	if err != nil {
		return none, err
	}
	// 解析库在下面三处各要一次（商业依据、可达性资格、控制策略）。它在 acceptanceCommercialBasis
	// 里已经建过一个，这里再建一个：都是 db 上的无状态包装，共享反而让三条依赖看不出各自要什么
	// （与 acceptanceConsumer / networkIntakeConsumer 各建各的仓储同一条理由）。
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: acceptance chain commercial resolutions: %w", err)
	}

	reachability, err := acceptanceReachability(db, outboxStore, resolutions, settings, clock)
	if err != nil {
		return none, err
	}
	control, err := acceptanceFinancialControl(db, commercial, resolutions, clock)
	if err != nil {
		return none, err
	}

	identities, err := psidentity.NewAcceptanceDecisions()
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: acceptance decision identities: %w", err)
	}
	downstream, err := pspostgres.NewOutboxAcceptanceDecisionHandoff(db, outboxStore, clock)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: acceptance decision handoff: %w", err)
	}

	chain := psapplication.NewAdvanceAcceptanceChainHandler(psapplication.AdvanceAcceptanceChainDeps{
		Reachability: psapplication.NewAdvanceAcceptanceJudgmentHandler(
			commercial, reachability, judgments, requests, clock),
		FinancialControl: psapplication.NewAdvanceFinancialControlJudgmentHandler(
			commercial, control, judgments, requests, clock),
		Decision: psapplication.NewFormAcceptanceDecisionHandler(psapplication.FormAcceptanceDecisionDeps{
			Requests:     requests,
			Commercial:   commercial,
			Reachability: reachability,
			Judgments:    judgments,
			Recorder:     judgments,
			Release:      control,
			Downstream:   downstream,
			Identities:   identities,
			Clock:        clock,
		}),
	})

	submitted, err := psinbox.NewShipmentRequestSubmittedConsumer(db.Transactor(), inboxStore, chain)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: acceptance chain consumer: %w", err)
	}
	reviewCompleted, err := psinbox.NewManualReviewCompletedConsumer(db.Transactor(), inboxStore, chain)
	if err != nil {
		return none, fmt.Errorf("parcel-dispatch: review resume consumer: %w", err)
	}
	return acceptanceChainGates{submitted: submitted, reviewCompleted: reviewCompleted}, nil
}

// acceptanceCommercialBasis 接商业依据三阶段（UC-PC-002）：解析闭包、按规则包声明形成
// 判断时点、提交决定前按原解析重校验。
//
// Values 留 nil 的代价要说清：时点语义的取值（「按哪个时刻算」）属 `PAR-COM-14` 实例半边，
// 没有租户就说不出。留 nil 时第二阶段答`未配置`，两条判断腿都在发起权威调用之前停下——
// 那是对的，一次在无人授权的时点上作出的判断，既解释不了自己按哪一版策略执行，也没法
// 在事后核对。
func acceptanceCommercialBasis(
	db *bentopg.DB,
	clock systemClock,
) (*pspartycommercial.CommercialBasisAdapter, error) {
	publications, err := pcpostgres.NewCommercialPublications(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: commercial publications: %w", err)
	}
	authority, err := pcpostgres.NewCommercialAuthority(publications)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: commercial authority: %w", err)
	}
	resolutions, err := pcpostgres.NewCommercialResolutions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: commercial basis resolutions: %w", err)
	}
	asOfPolicies, err := pcpostgres.NewAsOfPolicyDeclarations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: as-of policy declarations: %w", err)
	}
	contents, err := pcpostgres.NewAcceptanceContentDeclarations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance content declarations: %w", err)
	}
	keyStore, err := pspostgres.NewCommercialResolutionKeyStore(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: resolution key store: %w", err)
	}
	keys, err := pspartycommercial.NewCommercialResolutionKeys(keyStore)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: resolution keys: %w", err)
	}
	return pspartycommercial.NewCommercialBasisAdapter(pspartycommercial.CommercialBasisAdapterDeps{
		Resolve:      pcapplication.NewResolveCommercialBasisHandler(authority, resolutions, clock),
		Revalidate:   pcapplication.NewValidateCommercialBasisHandler(resolutions, authority, clock),
		Judgments:    pcapplication.NewFormJudgmentAsOfHandler(resolutions, asOfPolicies),
		AsOfPolicies: asOfPolicies,
		Contents:     contents,
		Keys:         keys,
		// Values 留空：实例半边，见函数注释。
	}), nil
}

// acceptanceReachability 接可达性两个端口（UC-NR-002）：形成三值判断，与提交决定前重校
// 那一份是否仍基于当前网络证据视图。
//
// 服务目的取本进程的部署形态参数——它属实例半边但已由 settingsFromEnv 强制必填，因此这
// 里拿到的一定是配置过的值，不是零值兜底。
//
// 资格视图的闭包标识留 nil：可达性判断键上没有解析标识，而范围到解析的映射属试点参数
// （ADR-0064 明写本记录只改初始路由那条链，可达性这条不变）。留 nil 时资格视图答未配置，
// 编排形成`未形成判断`——不代拟一个解析标识去问另一个产品的网络资格。
func acceptanceReachability(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	resolutions *pcpostgres.CommercialResolutions,
	settings dispatchSettings,
	clock systemClock,
) (*psnetworkrouting.ReachabilityAdapter, error) {
	definitions, err := nrpostgres.NewNetworkDefinitions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reachability network definitions: %w", err)
	}
	store, err := nrpostgres.NewReachabilityJudgments(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reachability judgments: %w", err)
	}
	handoff, err := nrpostgres.NewOutboxReachabilityHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reachability handoff: %w", err)
	}
	eligibility, err := nrpartycommercial.NewCommercialEligibility(
		resolutions,
		// 闭包标识留空：实例半边，见函数注释。
		nil,
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: reachability commercial eligibility: %w", err)
	}
	return psnetworkrouting.NewReachabilityAdapter(psnetworkrouting.ReachabilityAdapterDeps{
		Assess: nrapplication.NewAssessParcelReachabilityHandler(
			eligibility, definitions, store, handoff, clock),
		Revalidate: nrapplication.NewValidateReachabilityJudgmentHandler(definitions, store),
		Purpose:    settings.purpose,
	}), nil
}

// acceptanceFinancialControl 接接受前财务控制两个端口：施加与释放。
//
// 两侧都接真而不是只接施加半边：形成决定那一步在拒绝时要按原关联释放冻结（AT-PS-035），
// 只接施加会让一次拒绝把货主的钱留在原处占着。撤回那条链（cmd/parcel-api）只接释放半边，
// 两处的取舍相反而各自都对——那里根本不发起控制。
//
// 控制策略视图接 PC 的声明册：SA 不得自行从资金作用域反查合同（sa-preacceptance-policy-view
// 那笔裁决），回指由本装配显式接上。空册时它答未配置，编排形成`待判断`而不是「不要求控制」
// ——后者正是 CONTEXT 禁止的默认信用通过。
//
// 作用域源接 PolicyBackedControlScopeSource 而不是留 nil，两者今天的答复同为
// `CONTROL_SCOPE_NOT_CONFIGURED`，差别在缺的是哪一半：留 nil 是机制半边也没接，接上它
// 之后缺的只剩账户目录这一个租户参数（`SettlementAccountDirectory` 留 nil，不得为验它
// 造一份映射）。恢复动作因此从「写代码」变成「登记参数」，与解析键登记面同一条理由。
//
// 它与三条判断腿共用同一个商业依据适配器：作用域从**那一次**解析的结算政策回显派生
// （ADR-0044/0047 接通的缝），另建一个解析入口会给出第二次解析的机会，而施加与释放两径
// 同引用正靠同源。
func acceptanceFinancialControl(
	db *bentopg.DB,
	commercial *pspartycommercial.CommercialBasisAdapter,
	resolutions *pcpostgres.CommercialResolutions,
	clock systemClock,
) (*pssettlement.PreAcceptanceControlAdapter, error) {
	freezes, err := sapostgres.NewFreezeLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: freeze ledgers: %w", err)
	}
	exposures, err := sapostgres.NewCreditExposureLedgers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: credit exposure ledgers: %w", err)
	}
	balances, err := sapostgres.NewOperationalBalances(db, freezes)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: operational balances: %w", err)
	}
	standings, err := sapostgres.NewCreditStandings(db, exposures)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: credit standings: %w", err)
	}
	declarations, err := pcpostgres.NewPreAcceptanceControlDeclarations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: pre-acceptance control declarations: %w", err)
	}
	policy, err := sapartycommercial.NewPreAcceptanceControlPolicy(resolutions, declarations)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: pre-acceptance control policy: %w", err)
	}
	return pssettlement.NewPreAcceptanceControlAdapter(pssettlement.PreAcceptanceControlAdapterDeps{
		Apply: saapplication.NewApplyPreAcceptanceControlHandler(saapplication.ApplyPreAcceptanceControlDeps{
			Policy:    policy,
			Balance:   balances,
			Freezes:   freezes,
			Credit:    standings,
			Exposures: exposures,
			Clock:     clock,
		}),
		Release: saapplication.NewReleasePreAcceptanceControlHandler(freezes, exposures, clock),
		Scopes: pssettlement.NewPolicyBackedControlScopeSource(
			commercial,
			// 账户目录留空：实例半边，见函数注释。
			nil,
		),
		// Amounts 留空：估价缝属实例半边，见 acceptanceChainConsumer 的注释。
	}), nil
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

	rerouteFacts, err := nrpostgres.NewAutoRerouteFactsCatalog(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: auto reroute facts: %w", err)
	}
	reassessHandler := nrapplication.NewReassessRouteHandler(nrapplication.ReassessRouteDeps{
		Routes:        routeStore,
		Evidence:      definitions,
		Applicability: applicabilities,
		Store:         reassessments,
		Log:           handoffLog,
		Identities:    identities,
		Clock:         clock,
		// 自动改路四条件的事实目录已就位（审计票 05）：按判断键取当前陈述，从未
		// 登记的键照端口第二格答未配置——失效照常落库、改路评估整段不做，与先前
		// 显式 nil 在空册上的行为等价；登记过的键才走 7B/7C。改善阈值、改路条件
		// 与权限、冻结边界的取值属 PAR-NET-14 实例半边，目录只存登记方折算完的
		// 陈述与出处，不种任何默认行。
		AutoReroute: rerouteFacts,
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
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection receptions: %w", err)
	}
	processing, err := venodeops.NewDeriveOnNodeIntakeAdapter(receptions, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on node intake: %w", err)
	}
	consumer, err := veinbox.NewNodeIntakeConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive projection consumer: %w", err)
	}
	return consumer, nil
}

// derivePickupConsumer 接对象级揽收登记 → VE 投影。与 PS 采用分 inbox 名。
func derivePickupConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	registrations, err := tfpostgres.NewOffsitePickupRegistrations(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection pickup registrations: %w", err)
	}
	processing, err := vetf.NewDeriveOnOffsitePickupAdapter(registrations, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on offsite pickup: %w", err)
	}
	consumer, err := veinbox.NewOffsitePickupConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive pickup consumer: %w", err)
	}
	return consumer, nil
}

// deriveDeliveryConsumer 接有效交付登记 → VE 投影。与 PS 终局分 inbox 名。
func deriveDeliveryConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	deliveries, err := tfpostgres.NewEffectiveDeliveries(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection deliveries: %w", err)
	}
	processing, err := vetf.NewDeriveOnEffectiveDeliveryAdapter(deliveries, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on effective delivery: %w", err)
	}
	consumer, err := veinbox.NewEffectiveDeliveryConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive delivery consumer: %w", err)
	}
	return consumer, nil
}

// deriveExternalTrackingConsumer 接 TF 外部承运轨迹事实（有效时间已判断的版本）→ VE 投影。
// 本路不接 PS：外部轨迹是来源事实，不是有效交付，也不构成终局（TF CONTEXT「外部承运轨迹事实」）。
func deriveExternalTrackingConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	facts, err := tfpostgres.NewExternalTrackingFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection external tracking facts: %w", err)
	}
	processing, err := vetf.NewDeriveOnExternalCarrierTrackingAdapter(facts, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on external carrier tracking: %w", err)
	}
	consumer, err := veinbox.NewExternalTrackingConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive external tracking consumer: %w", err)
	}
	return consumer, nil
}

// deriveHandoverConsumer 接 TF 权威交接登记 → VE 投影。本路不接 PS：终局只认有效交付。
func deriveHandoverConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	handovers, err := tfpostgres.NewTransportHandovers(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection transport handovers: %w", err)
	}
	processing, err := vetf.NewDeriveOnTransportHandoverAdapter(handovers, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on transport handover: %w", err)
	}
	consumer, err := veinbox.NewTransportHandoverConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive handover consumer: %w", err)
	}
	return consumer, nil
}

// deriveFinalOutcomeConsumer 接 PS 包裹服务终局 → VE 投影。本路不接 PS：终局就是 PS
// 自己形成的事实，让 PS 经调度器消费自己等于把一份事实记两遍。
//
// 与交付投影分 inbox 名也分事实引用前缀：有效交付是终局的**上游来源**，同一包裹上两者
// 都会到，共用会让追踪把「交付发生」与「服务终局成立」叠成一条。
func deriveFinalOutcomeConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	finals, err := pspostgres.NewFinalOutcomes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection final outcomes: %w", err)
	}
	processing, err := veps.NewDeriveOnFinalOutcomeAdapter(finals, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on final outcome: %w", err)
	}
	consumer, err := veinbox.NewFinalOutcomeConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive final outcome consumer: %w", err)
	}
	return consumer, nil
}

// deriveInitialRouteConsumer 接 NR 包裹级初始路由判断 → VE 投影。本路只投 VE 不
// FanOut：按消费清点该信封的应消费方还有 NO/TF，但两侧消费者今天不存在——登记一个
// 接不住的比不登记更糟（ADR-0049 第三条），照交接路先例办。
//
// 信封只带完整判断键（含 customerAccountId），判断本体由处理适配器按键重取——权威
// 事实留在 network-routing，计划段链与候选依据都不进载荷。
func deriveInitialRouteConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	routes, err := nrpostgres.NewInitialRoutes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection initial routes: %w", err)
	}
	processing, err := venr.NewDeriveOnInitialRouteAdapter(routes, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on initial route: %w", err)
	}
	consumer, err := veinbox.NewInitialRouteConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive initial route consumer: %w", err)
	}
	return consumer, nil
}

// deriveExceptionJourneyConsumer 接 TF 替代/退运旅程启动 → VE 投影。本路只投 VE 不
// FanOut：旅程启动是 TF 自家过程事实，PS 侧今天没有它的消费者——登记接不住的比不
// 登记更糟（ADR-0049 第三条），照交接路先例办。
//
// 信封只带旅程幂等键四维，本体（含成员清单）由处理适配器按键重取——权威事实留在
// transport-fulfillment。一个信封型对两个事实类型（目的分格 alternate/return 进
// 类型，ADR-0066），登记仍按信封型一行，分岔在适配器里译。
func deriveExceptionJourneyConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	journeys, err := tfpostgres.NewAlternateJourneys(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection alternate journeys: %w", err)
	}
	processing, err := vetf.NewDeriveOnExceptionJourneyAdapter(journeys, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on exception journey: %w", err)
	}
	consumer, err := veinbox.NewExceptionJourneyConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive exception journey consumer: %w", err)
	}
	return consumer, nil
}

// deriveCustomsCaseConsumer 接 CC 关务案件建立 → VE 投影。本路只投 VE 不 FanOut：
// 案件建立是 CC 自家责任容器事实，PS 侧今天没有它的消费者——登记接不住的比不登记
// 更糟（ADR-0049 第三条），照旅程路先例办。
//
// 信封只带案件身份键五维，本体（含成员关联与客户归属）由处理适配器按键重取——权威
// 事实留在 customs-compliance。成员维与案件维一并进事实引用（ADR-0066）；建立是一件
// 事、无语义分支，单一事实类型，不存在旅程路那种目的分岔。
func deriveCustomsCaseConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	cases, err := ccpostgres.NewCustomsCases(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection customs cases: %w", err)
	}
	processing, err := vecc.NewDeriveOnCustomsCaseAdapter(cases, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on customs case: %w", err)
	}
	consumer, err := veinbox.NewCustomsCaseConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive customs case consumer: %w", err)
	}
	return consumer, nil
}

// deriveDeclarationSubmissionConsumer 接 CC 申报提交版本形成 → VE 投影。本路只投
// VE 不 FanOut：提交版本是 CC 自家申报链事实，PS 侧今天没有它的消费者——登记接不
// 住的比不登记更糟（ADR-0049 第三条），照案件路先例办。
//
// 信封只带提交幂等键三维加提交版本维，本体（含成员快照）由处理适配器按键重取——
// 权威事实留在 customs-compliance。成员维进事实引用、提交版本进版本维（ADR-0066；
// 同一逻辑申报目标的引用跨版本稳定，版本演进走版本维不走引用）；形成是一件事、
// 无语义分支，单一事实类型，不存在旅程路那种目的分岔。
func deriveDeclarationSubmissionConsumer(
	db *bentopg.DB,
	inboxStore *inbox.Store,
	derive *tenantBoundProjectionDerive,
) (dispatch.Consumer, error) {
	submissions, err := ccpostgres.NewDeclarationSubmissions(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: projection declaration submissions: %w", err)
	}
	processing, err := vecc.NewDeriveOnDeclarationSubmissionAdapter(submissions, derive)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive on declaration submission: %w", err)
	}
	consumer, err := veinbox.NewDeclarationSubmissionConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive declaration submission consumer: %w", err)
	}
	return consumer, nil
}

// deriveCustomerViewConsumer 接 VE 投影派生 → 客户视图（UC-VE-008 的内部半边）。
// 账户维经 veps.ParcelCustomerAccountLookup 从 PS 的按包裹反查取回（ADR-0060 零/一/
// 多三格：零行入账不派生、恰一行取账户、多行落未决不任选）；披露策略在租户现绑包装
// 里按命令租户构造，空册即四维全部待确认——实例半边如实说等，不虚构可见性。
func deriveCustomerViewConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	projections, err := vepostgres.NewProjections(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer view projections: %w", err)
	}
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer view parcel targets: %w", err)
	}
	accounts, err := veps.NewParcelCustomerAccountLookup(requests)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer account lookup: %w", err)
	}
	views, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer views: %w", err)
	}
	identities, err := veidentity.NewCustomerViewVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer view versions: %w", err)
	}
	downstream, err := vepostgres.NewOutboxCustomerViewHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: customer view handoff: %w", err)
	}
	processing, err := veps.NewDeriveCustomerViewOnProjectionAdapter(
		projections,
		accounts,
		&tenantBoundCustomerViewDerive{
			db:         db,
			views:      views,
			identities: identities,
			downstream: downstream,
			clock:      clock,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive customer view on projection: %w", err)
	}
	consumer, err := veinbox.NewTrackingProjectionConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive customer view consumer: %w", err)
	}
	return consumer, nil
}

// deriveCustomerViewOnAcceptanceConsumer 接 PS 接受决定 → VE 客户归属确立补派生
// （UC-VE-008 AT-VE-169：归属迟于包裹源事实时，视图在归属确立时按当前投影形成，
// 不等下一份源事实）。与初始路由消费者收同一封信、各记各的 inbox 账（FanOut 前提）。
//
// 声明清单读口与账户反查口装同一个 ShipmentRequests：两口读的本就是同一投影列的两个
// 方向（ADR-0060），拆两个对象等于让两处各自决定读哪些列。账户维必走反查口，信封的
// CustomerAccountID 只作一致性校验——绕开反查就丢了多行歧义闸（AT-VE-152），跨账户
// 泄露正是从那里进来。视图/标识/交接与投影派生那路各建各的包装（无状态，共享只会让
// 依赖图看不出各自要什么）；披露策略仍按命令租户现绑。
func deriveCustomerViewOnAcceptanceConsumer(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	inboxStore *inbox.Store,
	clock systemClock,
) (dispatch.Consumer, error) {
	requests, err := pspostgres.NewShipmentRequests(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive parcel views: %w", err)
	}
	projections, err := vepostgres.NewProjections(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive projections: %w", err)
	}
	accounts, err := veps.NewParcelCustomerAccountLookup(requests)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive account lookup: %w", err)
	}
	views, err := vepostgres.NewCustomerViews(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive customer views: %w", err)
	}
	identities, err := veidentity.NewCustomerViewVersions()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive view versions: %w", err)
	}
	downstream, err := vepostgres.NewOutboxCustomerViewHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive view handoff: %w", err)
	}
	processing, err := veps.NewDeriveCustomerViewOnAcceptanceAdapter(
		requests,
		projections,
		accounts,
		&tenantBoundCustomerViewDerive{
			db:         db,
			views:      views,
			identities: identities,
			downstream: downstream,
			clock:      clock,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: derive customer view on acceptance: %w", err)
	}
	consumer, err := veinbox.NewAcceptanceDecisionConsumer(db.Transactor(), inboxStore, processing)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: acceptance rederive consumer: %w", err)
	}
	return consumer, nil
}

// tenantBoundCustomerViewDerive 在每次 Handle 用命令上的租户构造披露策略视图。
// DisclosurePolicies 在构造期绑租户，AssessDisclosure 签名没有租户；派发进程是多租户。
// 禁止 NewDisclosurePolicies(db, 空租户)——看起来接了库、永远读不到行。该租户策略
// 空册 → configured=false → 编排四维全部待确认（实例半边如实说等）。
type tenantBoundCustomerViewDerive struct {
	db         *bentopg.DB
	views      *vepostgres.CustomerViews
	identities *veidentity.CustomerViewVersions
	downstream *vepostgres.OutboxCustomerViewHandoff
	clock      systemClock
}

var _ veps.CustomerViewDeriveHandler = (*tenantBoundCustomerViewDerive)(nil)

func (derive *tenantBoundCustomerViewDerive) Handle(
	ctx context.Context,
	command veapplication.DeriveCustomerViewCommand,
) (veapplication.DeriveCustomerViewResult, error) {
	if command.TenantID.String() == "" {
		return veapplication.DeriveCustomerViewResult{}, errors.New("parcel-dispatch: customer view tenant is empty")
	}
	policy, err := vepostgres.NewDisclosurePolicies(derive.db, command.TenantID)
	if err != nil {
		return veapplication.DeriveCustomerViewResult{}, err
	}
	return veapplication.NewDeriveCustomerViewHandler(veapplication.DeriveCustomerViewDeps{
		Policy:     policy,
		Views:      derive.views,
		Identities: derive.identities,
		Downstream: derive.downstream,
		Clock:      derive.clock,
	}).Handle(ctx, command)
}

// newTenantBoundProjectionDerive 九路投影共用一份事实/投影/交接仓储。映射仍按每次
// Handle 的租户现绑，不要为揽收、交付、交接、终局、初始路由、异常旅程、关务案件、
// 申报提交再复制八份包装。
func newTenantBoundProjectionDerive(
	db *bentopg.DB,
	outboxStore *outbox.Store,
	clock systemClock,
) (*tenantBoundProjectionDerive, error) {
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
	// 冲突信号进分诊（UC-VE-002 → UC-VE-004）那一支的租户无关半边：发作期库、发作期与
	// 案件标识、分诊结论意图。分诊规则与冲突信号规则两份目录在构造期绑租户，随 Handle 现绑。
	episodes, err := vepostgres.NewSignalEpisodes(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: signal episodes: %w", err)
	}
	episodeIdentities, err := veidentity.NewSignalEpisodes()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: signal episode identities: %w", err)
	}
	caseIdentities, err := veidentity.NewExceptionCases()
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: exception case identities: %w", err)
	}
	triageHandoff, err := vepostgres.NewOutboxTriageHandoff(db, outboxStore, clock)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: triage handoff: %w", err)
	}
	return &tenantBoundProjectionDerive{
		db:                db,
		facts:             facts,
		projections:       projections,
		identities:        identities,
		episodes:          episodes,
		episodeIdentities: episodeIdentities,
		caseIdentities:    caseIdentities,
		triageHandoff:     triageHandoff,
		downstream:        downstream,
		clock:             clock,
	}, nil
}

// tenantBoundProjectionDerive 在每次 Handle 用命令上的租户构造映射视图。
// MilestoneMappings 在构造期绑租户，ClassifyFact 签名没有租户；派发进程是多租户。
// 禁止 NewMilestoneMappings(db, 空租户)——看起来接了库、永远读不到行。该租户目录空
// → found=false → 编排 PROJECTION_DERIVED + 未归类（MAPPING_NOT_CONFIGURED）。
//
// 分诊规则（TriageRules）与冲突信号规则（ConflictSignalRules）同一纪律：都在构造期绑租户，
// 都随 Handle 现绑；该租户目录空 → 冲突记为无适用信号规则 / 信号进人工复核格，不虚构。
type tenantBoundProjectionDerive struct {
	db                *bentopg.DB
	facts             *vepostgres.AcceptedFacts
	projections       *vepostgres.Projections
	identities        *veidentity.ProjectionVersions
	episodes          *vepostgres.SignalEpisodes
	episodeIdentities *veidentity.SignalEpisodes
	caseIdentities    *veidentity.ExceptionCases
	triageHandoff     *vepostgres.OutboxTriageHandoff
	downstream        *vepostgres.OutboxProjectionHandoff
	clock             systemClock
}

var (
	_ venodeops.ProjectionHandler        = (*tenantBoundProjectionDerive)(nil)
	_ vetf.PickupProjectionHandler       = (*tenantBoundProjectionDerive)(nil)
	_ vetf.ProjectionHandler             = (*tenantBoundProjectionDerive)(nil)
	_ vetf.HandoverProjectionHandler     = (*tenantBoundProjectionDerive)(nil)
	_ vetf.JourneyProjectionHandler      = (*tenantBoundProjectionDerive)(nil)
	_ vecc.CaseProjectionHandler         = (*tenantBoundProjectionDerive)(nil)
	_ vecc.SubmissionProjectionHandler   = (*tenantBoundProjectionDerive)(nil)
	_ veps.FinalProjectionHandler        = (*tenantBoundProjectionDerive)(nil)
	_ venr.InitialRouteProjectionHandler = (*tenantBoundProjectionDerive)(nil)
)

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
	conflictRules, err := vepostgres.NewConflictSignalRules(derive.db, command.TenantID)
	if err != nil {
		return veapplication.DeriveProjectionResult{}, err
	}
	triage, err := vepostgres.NewTriageRules(derive.db, command.TenantID)
	if err != nil {
		return veapplication.DeriveProjectionResult{}, err
	}
	// 冲突信号走真的信号编排进分诊：与派生同一事务（两边都按 RequireExecutor 落库），
	// 分叉双方留在投影里的那一笔与它们的信号发作期同生共死。
	signals := veapplication.NewRaiseSignalHandler(veapplication.RaiseSignalDeps{
		Episodes:   derive.episodes,
		Triage:     triage,
		Identities: derive.episodeIdentities,
		Cases:      derive.caseIdentities,
		Downstream: derive.triageHandoff,
		Clock:      derive.clock,
	})
	return veapplication.NewDeriveProjectionHandler(veapplication.DeriveProjectionDeps{
		Facts:         derive.facts,
		Mapping:       mapping,
		Projections:   derive.projections,
		Identities:    derive.identities,
		ConflictRules: conflictRules,
		Signals:       signals,
		Downstream:    derive.downstream,
		Clock:         derive.clock,
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
	//
	// 这一格与采用链那一格不一样，且必须不一样，不是漏改。终局链把这个适配器当
	// FinalRuleView 用，该接口只有 JudgeFinalOutcome；读证据口的是 JudgeIntakeEligibility，
	// 属 IntakeEligibilityView。终局链在类型上就到不了这一格——所以即使采用链那边已经接上
	// 真的节点权威口，这里仍然留白。在这里配上权威段与节点执行事实，等于替终局链写下一条
	// 它并不需要的依赖，读的人会以为终局也在判收寄资格。
	//
	// 两道实例墙分开登也是这个道理：收寄停在 INTAKE_QUALIFICATION_UNPROVEN（PAR-COM-16），
	// 终局停在 FINAL_RULE_UNCONFIGURED（PAR-COM-17）。接进来就是拿前者去顶后者的停点，
	// 两条链的停点不再各自诚实。
	//
	// 「反正行为一样」不构成留白的理由：一样只在 FinalRuleView 单方法这个静态事实成立时
	// 成立。哪天它不成立——接口长出第二个方法，或有人在本链上把这个适配器当
	// IntakeEligibilityView 用——显式未配置答未证明是安全的那一边，真适配器会默默给出一个
	// 没人要的判断。
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

// nodeQualificationAuthority 是「本部署把哪一段收寄硬资格交给 node-operations 作证」的
// 实例配置：认领的权威段前缀，加上该段下每条引用由哪一件节点执行事实来证。
//
// 两个字段同属实例半边——段怎么起名、哪条引用算数，都由租户的资格声明决定，没有租户就
// 一个也说不出，因此生产装配交进来的是零值。零值读作「本部署没认领任何段」，不是「忘了
// 填」：后者要有人去补，前者是今天的真话。
type nodeQualificationAuthority struct {
	prefix     string
	qualifying map[string]psnodeops.QualifyingNodeExecution
}

// intakeQualificationEvidence 按实例配置交回收寄硬资格证据口（ADR-0063）。
//
// 未认领时交回诚实未配置口，恒答未证明——这是首发唯一走得到的分支。这里不替它猜一个
// 前缀：认领哪一段取决于租户声明怎么起名，猜错的表现是真段永远路由不到，而那与「证据
// 还没到」在判断结果上完全一样，现场分不出该去补配置还是该去等证据。
//
// 认领了才建节点口，登记键取 view.AuthorityPrefix() 而不在这里另写一份——两处各写一份
// 就会有写岔的那一天，写岔同样只表现为恒答未证明。
//
// 选这个形状而不是干脆留着 UnconfiguredIntakeQualificationEvidence{}，图的是恢复动作看
// 得见：两种取值行为完全相同（都恒答未证明），但租户出现那天，要补的东西就在参数表
// 上——一个前缀与一张表，而不必先读一遍适配器包才认出「原来还有个节点权威口可以接」。
//
// 还剩一道机制拦不住的事：prefix 若声明成 node-operations 说不了的那种段（正式关务判断
// 归 customs-compliance，ADR-0063 决定五），再把该段的引用登进 qualifying，构造期两道
// 校验都会放行——它们只比两者是否同段，不知道那一段该归谁。那正是 ADR-0063 Consequences
// 点名的「为了变绿拆出一个假关务身份」，填这个值的人自己守住。
func intakeQualificationEvidence(
	db *bentopg.DB,
	authority nodeQualificationAuthority,
) (psports.IntakeQualificationEvidenceView, error) {
	if authority.prefix == "" {
		// 填了表却没认领段：两个字段要一起才立得住。放过去就是一张永远问不到的表，
		// 判断结果与真的没配置一模一样，而要人做的事相反。
		if len(authority.qualifying) > 0 {
			return nil, fmt.Errorf(
				"parcel-dispatch: intake qualification registry has %d entries but claims no authority prefix",
				len(authority.qualifying))
		}
		return pspartycommercial.UnconfiguredIntakeQualificationEvidence{}, nil
	}
	facts, err := nopostgres.NewExecutionFacts(db)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node execution facts: %w", err)
	}
	view, err := psnodeops.NewNodeExecutionQualificationEvidence(facts, authority.prefix, authority.qualifying)
	if err != nil {
		return nil, fmt.Errorf("parcel-dispatch: node intake qualification evidence: %w", err)
	}
	return pspartycommercial.NewKnownPrefixIntakeQualificationEvidence(
		map[string]psports.IntakeQualificationEvidenceView{view.AuthorityPrefix(): view},
	), nil
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
	// 第五个入参是硬资格证据口（ADR-0063）：交零值即本部署未认领任何权威段，答未证明；
	// nil 会在非空清单上变成依赖错误，两者都不得默认 ESTABLISHED。
	evidence, err := intakeQualificationEvidence(db, nodeQualificationAuthority{})
	if err != nil {
		return none, err
	}
	eligibility := pspartycommercial.NewServiceStageRulesAdapter(
		declared, declared, declared, nil, evidence,
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
