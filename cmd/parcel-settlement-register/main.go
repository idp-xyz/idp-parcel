// parcel-settlement-register 是 settlement-accounting「外部资金事实采用」这一口的受控进程入口（票 sa-cc/27
// 裁决 2 第一步）：结算运营或财务集成责任方在数据库网络内手跑，不监听任何端口。它给 ADR-0137 决定四保留的
// 那唯一一口装上门——外部资金事实进产品只经 SA 采用，`customs-compliance` 不开第二个铸造或补录入口，人工核实与
// 更正也先登在这里、再经采用信封到 CC。在此之前采用编排在生产依赖图上零装配，首版与更正都只有测试直写能到。
//
// 命令表在 translate.go 的 allCommands，一处列全：`external-funds-fact` 采用一条事实的首版，
// `external-funds-fact-correction` 采用一次外部更正（同一事实回指当前链头的新版本，只有金额变——UC-SA-001
// 「更正必须形成新来源版本」）。两条是两个命令类型、两条用例方法，不共享入口：更正无回指时不能退化成首版。
//
// 本口只开人工 / 受控批量的门，不自动采用：任何回调 / 文件到达都不在这里（UC-SA-005 输入表「不因接收回调
// 自动采用」；真实财务系统来源仍 `No-Go / 待参数化`，PAR-INT-05）。输入全部来自 -input 指定的 JSON 文件，未知
// 字段一律拒绝；进程不内置任何生产默认——来源、账户、币种属实例半边，机制先行，验证用脱敏合成值（S 级只记 S）。
//
// 与在线面的关系（ADR-0085 决定一）：CLI 与端点消费同一登记用例、答案代数一致；载荷译装因此放在
// `internal/settlementaccounting/adapters/registrationjson` 而不在本包——端点那一侧（第二步，后继票）收同源
// 载荷，`internal` 导不进 `cmd`。本口不因端点长出而退场，它是同一能力的受控批量口。
//
// 环境事务由本入口给出，一次调用一笔事务：采用 / 更正的版本行与向 CC 交的采用信封落在同一只 db 的同一笔里
// （Outbox 意图与登记同生同灭），写口无环境事务即拒。
//
// 退出码按恢复动作分四格（ADR-0029 判据；归格见 fundsAnswer）：0 = 已采用 / 幂等重放（重放不是错误，无恢复
// 动作）；1 = 用法或输入不合法（含用例的`未受理`——回指非链头、更正未采用的事实、构造门拒——改请求，不是重试）；
// 2 = 内容冲突（同版本字面异内容、同事实第二个首版：绝不覆盖，人工核对既有登记再续办）；3 = 未决（依赖故障，
// 登记与否未知；也含「行已落、信封未出」那一格——它需要人重跑同一命令补发同一封，不能与已登记同格）。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	sapostgres "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitRegistered = 0
	exitUsage      = 1
	exitConflict   = 2
	exitUndecided  = 3
)

// registrar 是本口的全部依赖：采用编排加环境事务的来源。
type registrar struct {
	funds      *application.MapExternalFundsHandler
	transactor bentoapp.Transactor
}

// systemClock 给版本行的采用时刻与信封的记录时刻：那两个时刻不是登记输入，是「本口此刻形成」——与载荷里的
// occurredAt / correctedAt 由操作员交进来是两回事，后者是被采用事实的一部分。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintf(errOut, "用法：parcel-settlement-register <%s> -input <file>\n", strings.Join(allCommands, "|"))
		return exitUsage
	}
	command := args[0]
	if !knownCommand(command) {
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

func knownCommand(command string) bool {
	for _, known := range allCommands {
		if command == known {
			return true
		}
	}
	return false
}

// buildRegistrar 装配真实采用链（裁决 3）：六口全接真，一只都不留 nil、不放替身。映射 / 核销 / 已结视图交接三口
// 不在采用 / 更正路径上也接真——「生产装配里不放任何替身」是 SA 装配处的既有纪律，且构造门（裁决 4）拒 nil，半装
// 根本装不进去。资金事实交接口与登记册同一只 db、同一只 Outbox Store：事务壳给的环境事务因此同时罩住版本行与
// 向 CC 交的采用信封——两者同生共死，形照 parcel-customs-register 的 buildRegistrar。
func buildRegistrar(db *bentopg.DB) (registrar, error) {
	none := registrar{}
	facts, err := sapostgres.NewExternalFundsFacts(db)
	if err != nil {
		return none, fmt.Errorf("构造外部资金事实写口：%w", err)
	}
	mappings, err := sapostgres.NewFundsMappings(db)
	if err != nil {
		return none, fmt.Errorf("构造资金映射写口：%w", err)
	}
	applications, err := sapostgres.NewSettlementApplications(db)
	if err != nil {
		return none, fmt.Errorf("构造核销写口：%w", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("构造 Outbox Store：%w", err)
	}
	factHandoff, err := sapostgres.NewOutboxExternalFundsFactHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造资金事实采用交接口：%w", err)
	}
	downstream, err := sapostgres.NewOutboxSettlementApplicationHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造核销交接口：%w", err)
	}
	funds, err := application.NewMapExternalFundsHandler(application.MapExternalFundsDeps{
		Facts:        facts,
		Mappings:     mappings,
		Applications: applications,
		Downstream:   downstream,
		FactHandoff:  factHandoff,
		Clock:        systemClock{},
	})
	if err != nil {
		return none, fmt.Errorf("构造外部资金采用编排：%w", err)
	}
	return registrar{funds: funds, transactor: db.Transactor()}, nil
}

// execute 把一份登记输入推进到采用答案：译装 → 在环境事务内交用例 → 答案译成退出码。译装失败当场拒、不进事务
// ——用法错误与「登记与否未知」是两个退出码，让它进了事务就分不开了。一次调用一个命令一笔事务，采用与它的
// 重放 / 冲突判定读回、以及向 CC 交的信封因此看同一份快照。
func execute(ctx context.Context, command string, raw []byte, registrar registrar) (string, int) {
	dispatch, err := commandFor(command, raw)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", command, err), exitUsage
	}

	var handled answer
	err = registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		result, err := dispatch(txCtx, registrar)
		handled = result
		return err
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
	}
	if handled == nil {
		// 事务成功却没有答案是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 用例未交回任何结果", command), exitUndecided
	}
	return handled.exit(command)
}

// fundsResult 是采用编排交回的封闭结果在本入口的译法。
type fundsResult struct {
	result application.FundsResult
}

func (result fundsResult) exit(command string) (string, int) {
	return fundsAnswer(command, result.result.Outcome(), result.result.UndecidedReason(),
		result.result.FundsHandoffReference(), subjectOf(result.result))
}

// subjectOf 取已采用 / 已存在那一版的事实与版本字面，打进答复：操作员看的是「哪条事实的哪一版落了」，不是一个词。
func subjectOf(result application.FundsResult) string {
	record, found := result.Fact()
	if !found {
		return ""
	}
	return record.Key.Fact.String() + " 版本 " + record.Fact.Version().String()
}

// fundsAnswer 把 `application.FundsOutcome` 的封闭各格译成退出码，不增不减。一族一张表：映射与核销那几格本口没有
// 命令能交回，仍在表上——同一格不因来自哪条命令而换退出码（parcel-customs-register 的 dutyReconciliationAnswer
// 同一条纪律）。归格只看恢复动作（裁决 5）：
//   - 已采用 / 已存在 / 已映射 / 已核销 / 已撤销 / 重放各格 → 0。重放不是错误，无恢复动作。
//   - `未受理` 与三个业务负向格（不可映射的失败付款、分配失衡、跨币种无换算）→ 1：内容改对了才该重来，重跑同一份
//     没有意义。回指非链头、更正一条未采用的事实、构造门拒都落在`未受理`。
//   - 内容冲突三格 → 2：同键异内容绝不覆盖，要人核对既有登记再决定续办。
//   - 未决 → 3，带编排指名的原因（今天与采用相关的只有资金事实库不可用）。
//   - 行已落、信封未出（续办引用非空）→ 3 并把续办引用打出：事实已采用是真的，但 CC 等的那封没出去，重跑同一命令
//     会重发同一份（Outbox 按认领键吞重）；它需要人动手，不能与已登记同格。
func fundsAnswer(
	command string,
	outcome application.FundsOutcome,
	reason application.FundsUndecidedReason,
	handoff string,
	subject string,
) (string, int) {
	message := command + ": " + outcome.String()
	if subject != "" {
		message += "（" + subject + "）"
	}
	switch outcome {
	case application.FundsFactAdopted, application.FundsFactExisting,
		application.FundsMapped, application.FundsMappingExisting,
		application.SettlementApplied, application.ApplicationExisting,
		application.ApplicationReversedOutcome, application.ApplicationAlreadyReversed:
		if handoff != "" {
			return message + "（记录已落、信封未出——续办引用 " + handoff + "，重跑同一命令补发同一封）", exitUndecided
		}
		return message, exitRegistered
	case application.FundsNotAccepted,
		application.UnfundableFactOutcome, application.ApplicationImbalanceOutcome, application.CrossCurrencyOutcome:
		return message, exitUsage
	case application.FundsFactConflict, application.FundsMappingConflict, application.ApplicationConflict:
		return message + "（同键已在册且内容不同——绝不覆盖，先核对既有登记）", exitConflict
	case application.FundsUndecided:
		return message + "（" + reason.String() + "）", exitUndecided
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", command, outcome), exitUndecided
	}
}
