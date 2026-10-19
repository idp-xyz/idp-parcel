// parcel-transport-register 是运输履约周期安排、具体班次与容量池三本册子的受控登记口（ADR-0201 决定一）：运营方
// 操作员在数据库网络内手跑，不监听任何端口。它与 parcel-api 上对应的登记口调同一个编排（ADR-0085）：在线口的
// 租户取操作者信封，本口的租户取输入文件里的 tenantId——授权依据是「能运行本 CLI」这道运维边界（先例：
// parcel-customs-register、parcel-network-register）。
//
// 命令三种：
//   - schedule-template 登一份周期安排。同代码同内容答已存在，异内容成下一版。
//   - schedule 登一个具体班次：只给 template 与 runningDate 即按那份周期安排的当前版物化，那一版声明了逐维容量就
//     随班次建池；给 schedule 与正文各格即登临时班次。同一班次同内容答已存在，异内容成下一版。
//   - capacity-pool 在一个已登记的班次上登一个维度的池。同一（班次、维度）同内容答已存在，异内容答池冲突。
//
// 输入是一份 JSON（-input 指路径），字段与在线口的载荷一一对应、另加 tenantId；未知字段一律拒绝——打错的键静默
// 丢弃会让操作员以为登进去的比实际多。本工具不携带任何默认取值：周期安排、班次与容量的内容是租户取值
// （PAR-NET-08、PAR-NET-12），文法与物化规则才是产品（ADR-0146、ADR-0201）。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。环境事务由本入口给出（写口 RequireExecutor 无环境事务即
// 拒），一次调用一笔事务。
//
// 退出码按恢复动作分四格（ADR-0029 判据）：0 = 已登记 / 下一版 / 已建立 / 已存在；1 = 用法或输入不合法（含受理门
// 拒绝、周期安排不在册、那一天不开、池所在班次不在册——改输入或先登前置，重跑同一份没有意义）；2 = 池冲突（库里
// 那一池与这次的内容不同，登记口不改池，要人看过再定）；3 = 未决（依赖故障，登记与否未知，重跑同一命令续办）。周期
// 安排与班次没有冲突格：同键异内容在那两本册子上就是下一版；物化登记一班时任一个池冲突，班次照登，退出码取 2。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"slices"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	tfhttp "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/http"
	tfpostgres "go.idp.xyz/idp-parcel/internal/transportfulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitRegistered = 0
	exitUsage      = 1
	exitConflict   = 2
	exitUndecided  = 3
)

const (
	commandScheduleTemplate = "schedule-template"
	commandSchedule         = "schedule"
	commandCapacityPool     = "capacity-pool"
)

// allCommands 只为用法文本与未知命令的错误文本服务，列成一处免得两段文本各抄一遍。
var allCommands = []string{commandScheduleTemplate, commandSchedule, commandCapacityPool}

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// registrar 是本口的全部依赖：两个登记编排加环境事务的来源。
type registrar struct {
	handler    *application.RegisterTransportScheduleHandler
	pools      *application.PrepareTransportOpportunityHandler
	transactor bentoapp.Transactor
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintf(errOut, "用法：parcel-transport-register <%s> -input <file>\n", strings.Join(allCommands, "|"))
		return exitUsage
	}
	command := args[0]
	if !slices.Contains(allCommands, command) {
		fmt.Fprintf(errOut, "未知登记命令 %q（支持 %s）\n", command, strings.Join(allCommands, " / "))
		return exitUsage
	}
	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(errOut)
	input := flags.String("input", "", "登记输入 JSON 路径")
	if err := flags.Parse(args[1:]); err != nil {
		return exitUsage
	}
	if *input == "" {
		fmt.Fprintf(errOut, "%s 需要 -input <file>\n", command)
		return exitUsage
	}
	raw, err := os.ReadFile(*input)
	if err != nil {
		fmt.Fprintf(errOut, "读登记输入：%v\n", err)
		return exitUsage
	}
	dsn := getenv(envDatabaseDSN)
	if dsn == "" {
		fmt.Fprintf(errOut, "%s 未设置——登记口不猜连接串\n", envDatabaseDSN)
		return exitUsage
	}

	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(errOut, "连接数据库：%v\n", err)
		return exitUndecided
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		fmt.Fprintf(errOut, "构造框架 DB：%v\n", err)
		return exitUndecided
	}
	registrar, err := buildRegistrar(db)
	if err != nil {
		fmt.Fprintf(errOut, "装配登记口：%v\n", err)
		return exitUndecided
	}

	message, code := execute(ctx, command, raw, registrar)
	fmt.Fprintln(out, message)
	return code
}

// buildRegistrar 装配真实登记链：三本册子的适配器加编排，与 parcel-api 的 buildTransportScheduleRegistration、
// buildCapacityPoolRegistration 同形——建池编排同样不接消耗交接，本口交不出建池以外的入口。
func buildRegistrar(db *bentopg.DB) (registrar, error) {
	templates, err := tfpostgres.NewScheduleTemplates(db)
	if err != nil {
		return registrar{}, fmt.Errorf("构造周期安排册：%w", err)
	}
	schedules, err := tfpostgres.NewTransportSchedules(db)
	if err != nil {
		return registrar{}, fmt.Errorf("构造班次册：%w", err)
	}
	pools, err := tfpostgres.NewCapacityPools(db)
	if err != nil {
		return registrar{}, fmt.Errorf("构造容量池册：%w", err)
	}
	return registrar{
		handler: application.NewRegisterTransportScheduleHandler(application.RegisterTransportScheduleDeps{
			Templates: templates,
			Schedules: schedules,
			Pools:     pools,
			Clock:     systemClock{},
		}),
		pools: application.NewPrepareTransportOpportunityHandler(application.PrepareTransportOpportunityDeps{
			Pools:     pools,
			Schedules: schedules,
			Clock:     systemClock{},
		}),
		transactor: db.Transactor(),
	}, nil
}

// templateInput、scheduleInput 与 poolInput 是本口的输入：在线口的载荷类型原样嵌入，另加 tenantId。载荷翻成命令
// 只走在线口那一份（Command），两个入口的严格性不会各自漂移。
type templateInput struct {
	TenantID string `json:"tenantId"`
	tfhttp.ScheduleTemplateRegistrationPayload
}

type scheduleInput struct {
	TenantID string `json:"tenantId"`
	tfhttp.TransportScheduleRegistrationPayload
}

type poolInput struct {
	TenantID string `json:"tenantId"`
	tfhttp.CapacityPoolRegistrationPayload
}

// dispatchFunc 在事务内调一次用例，交回把答案译成退出码的那一步：三种命令的用例结果类型不同，译法各随各的命令。
type dispatchFunc func(ctx context.Context, registrar registrar) (func() (string, int), error)

// commandFor 按命令译装输入，交回一个在事务内执行的调用。译装失败当场拒，不进事务——用法错误与「登记与否未知」
// 是两个退出码，让它进了事务就分不开了。
func commandFor(command string, raw []byte) (dispatchFunc, error) {
	switch command {
	case commandScheduleTemplate:
		var input templateInput
		if err := decodeStrict(raw, &input); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(input.TenantID)
		if err != nil {
			return nil, fmt.Errorf("tenantId：%w", err)
		}
		translated, err := input.ScheduleTemplateRegistrationPayload.Command(tenant)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (func() (string, int), error) {
			result, err := registrar.handler.RegisterTemplate(ctx, translated)
			return func() (string, int) { return answer(command, result) }, err
		}, nil
	case commandSchedule:
		var input scheduleInput
		if err := decodeStrict(raw, &input); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(input.TenantID)
		if err != nil {
			return nil, fmt.Errorf("tenantId：%w", err)
		}
		translated, err := input.TransportScheduleRegistrationPayload.Command(tenant)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (func() (string, int), error) {
			result, err := registrar.handler.RegisterSchedule(ctx, translated)
			return func() (string, int) { return answer(command, result) }, err
		}, nil
	case commandCapacityPool:
		var input poolInput
		if err := decodeStrict(raw, &input); err != nil {
			return nil, err
		}
		tenant, err := domain.NewTenantID(input.TenantID)
		if err != nil {
			return nil, fmt.Errorf("tenantId：%w", err)
		}
		translated, err := input.CapacityPoolRegistrationPayload.Command(tenant)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, registrar registrar) (func() (string, int), error) {
			result, err := registrar.pools.EstablishPool(ctx, translated)
			return func() (string, int) { return answerPool(command, result) }, err
		}, nil
	default:
		return nil, fmt.Errorf("未知登记命令 %q", command)
	}
}

func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("登记输入：%w", err)
	}
	if decoder.More() {
		return fmt.Errorf("登记输入：一份文件只收一条登记")
	}
	return nil
}

// execute 把一份登记输入推进到登记册答案：译装 → 在环境事务内交用例 → 答案译成退出码。
func execute(ctx context.Context, command string, raw []byte, registrar registrar) (string, int) {
	dispatch, err := commandFor(command, raw)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", command, err), exitUsage
	}
	var translate func() (string, int)
	err = registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		answered, err := dispatch(txCtx, registrar)
		translate = answered
		return err
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
	}
	return translate()
}

// answer 把用例的封闭各格译成退出码，不增不减。带记录的答案附上那一版的身份与版本，操作员凭它核对册面；物化登记
// 的一班随后逐池列出池身份与各自的答案。
func answer(command string, result application.ScheduleRegistrationResult) (string, int) {
	message := command + ": " + result.Outcome().String()
	if record, present := result.Template(); present {
		message += fmt.Sprintf("（%s v%d）", record.Template.Code(), record.Template.Version())
	}
	if record, present := result.Schedule(); present {
		message += fmt.Sprintf("（%s v%d）", record.Schedule.Schedule(), record.Schedule.Version())
	}
	conflicted := false
	for _, pool := range result.Pools() {
		message += fmt.Sprintf("；%s %s", pool.Pool.Pool.Pool(), pool.Outcome)
		conflicted = conflicted || pool.Outcome == application.PoolConflict
	}
	switch result.Outcome() {
	case application.ScheduleTemplateRegistered, application.ScheduleTemplateRevised, application.ScheduleTemplateExisting,
		application.TransportScheduleRegistered, application.TransportScheduleRevised, application.TransportScheduleExisting:
		if conflicted {
			return message, exitConflict
		}
		return message, exitRegistered
	case application.ScheduleRegistrationNotAccepted, application.ScheduleTemplateNotFound, application.ScheduleTemplateNotRunning:
		return message, exitUsage
	case application.ScheduleRegistrationUndecided:
		return message + "（" + result.UndecidedReason().String() + "）", exitUndecided
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", command, result.Outcome()), exitUndecided
	}
}

// answerPool 把建池用例的封闭各格译成退出码，不增不减。带池的答案附上池身份，冲突时那是库里已建的那一个。
func answerPool(command string, result application.OpportunityResult) (string, int) {
	message := command + ": " + result.Outcome().String()
	if record, present := result.Pool(); present {
		message += fmt.Sprintf("（%s %s %d）", record.Pool.Pool(), record.Pool.Unit(), record.Pool.Capacity())
	}
	switch result.Outcome() {
	case application.PoolEstablished, application.PoolExistingResult:
		return message, exitRegistered
	case application.PoolConflict:
		return message, exitConflict
	case application.OpportunityNotAccepted, application.PoolScheduleNotFound:
		return message, exitUsage
	case application.OpportunityUndecided:
		return message + "（" + result.UndecidedReason().String() + "）", exitUndecided
	default:
		// 建池口只该交回上面几格；交回别的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", command, result.Outcome()), exitUndecided
	}
}
