// parcel-customs-register 是关务案件配置面五本登记册的受控进程入口（syn-wall-door-audit
// 票 06 B 半边）：运营方操作员在数据库网络内手跑，不监听任何端口——配置登记是治理动作，
// 不是在线请求面，走独立进程而不进 parcel-api 的端点表（先例：parcel-pricing-register、
// parcel-network-register、parcel-governance-register）。
//
// 六本册子十个命令：就绪判断与提交授权各带撤销半边（撤销是状态推进不是删除，原判断
// 原样留在行内）；解释规则按（辖区，法定生效起点）登记不可覆盖的版本（ADR-0070——
// 换版即登记更晚起点的新版，开放前版终点随之落定，历史区间不接受追改）；关闭义务与
// 门禁前置条件各分目录与明细两个命令——「目录登记了但清单空」是必须登得出来的一格，
// 与「未登记」含义相反；建案要求规则（case-requirement）挡的是建案链第一步那堵
// EstablishCaseUndecided 墙，「不要求建案」也必须带依据登记，未登记是未决不是「不要求」。
//
// 输入全部来自 -input 指定的 JSON 文件，未知字段一律拒绝；进程不内置任何生产默认——
// 配置内容属实例半边（PAR-CUS-01..07 待提供），机制先行，验证用脱敏合成值（S 级只记 S）。
// 环境事务由本入口给出（写口 RequireExecutor 无环境事务即拒），一次调用一笔事务。
//
// 不留 channel_execution 痕：那是票 12 对治理登记面的裁决，其留痕册属 pilot-governance
// 所有权；本口的登记内容也不含执行人轨，授权依据就是「能运行本 CLI」这道运维边界。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 已登记/幂等重放/撤销落地/已撤销（撤销的意图是使失效，已失效即达成——
// 册面原因归首撤者，未决重跑落在这一格时不该再劳人工）；1 = 用法或输入不合法（含受理
// 门拒绝与撤销无对象——改请求，不是重试）；2 = 内容冲突（同键异内容绝不覆盖，人工核
// 对既有登记再续办）；3 = 未决（依赖故障或撞上库上防线，登记与否未知，重跑同一命令
// 即可续办）。
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

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/customscompliance/application"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitRegistered = 0
	exitUsage      = 1
	exitConflict   = 2
	exitUndecided  = 3
)

// registrar 是本口的全部依赖：两个登记用例 handler 加环境事务的来源。
type registrar struct {
	configurations *application.RegisterCaseConfigurationHandler
	requirements   *application.RegisterCaseRequirementRuleHandler
	transactor     bentoapp.Transactor
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintf(errOut, "用法：parcel-customs-register <%s> -input <file>\n", strings.Join(allCommands, "|"))
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

// buildRegistrar 装配真实登记链：五本册子写口加五个只读视图。读口不是可选的便利，
// 冲突判定就靠它——只有写口时「已在册」永远说不出是重放还是改内容。
func buildRegistrar(db *bentopg.DB) (registrar, error) {
	none := registrar{}
	readiness, err := adapter.NewReadinessRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造就绪写口：%w", err)
	}
	readinessView, err := adapter.NewReadinessView(db)
	if err != nil {
		return none, fmt.Errorf("构造就绪读口：%w", err)
	}
	authorities, err := adapter.NewSubmissionAuthorityRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造授权写口：%w", err)
	}
	authorityView, err := adapter.NewSubmissionAuthorityView(db)
	if err != nil {
		return none, fmt.Errorf("构造授权读口：%w", err)
	}
	rules, err := adapter.NewInterpretationRuleRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造解释规则写口：%w", err)
	}
	ruleView, err := adapter.NewInterpretationRuleView(db)
	if err != nil {
		return none, fmt.Errorf("构造解释规则读口：%w", err)
	}
	obligations, err := adapter.NewObligationInventoryRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造义务写口：%w", err)
	}
	obligationView, err := adapter.NewObligationInventoryView(db)
	if err != nil {
		return none, fmt.Errorf("构造义务读口：%w", err)
	}
	gates, err := adapter.NewGateConditionRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造门禁写口：%w", err)
	}
	gateView, err := adapter.NewGateConditionView(db)
	if err != nil {
		return none, fmt.Errorf("构造门禁读口：%w", err)
	}
	requirements, err := adapter.NewCaseRequirementRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造建案规则写口：%w", err)
	}
	requirementView, err := adapter.NewCaseRequirementView(db)
	if err != nil {
		return none, fmt.Errorf("构造建案规则读口：%w", err)
	}

	configurations := application.NewRegisterCaseConfigurationHandler(application.RegisterCaseConfigurationDeps{
		Readiness:      readiness,
		ReadinessView:  readinessView,
		Authorities:    authorities,
		AuthorityView:  authorityView,
		Rules:          rules,
		RuleView:       ruleView,
		Obligations:    obligations,
		ObligationView: obligationView,
		Gates:          gates,
		GateView:       gateView,
	})
	requirementHandler := application.NewRegisterCaseRequirementRuleHandler(
		application.RegisterCaseRequirementRuleDeps{Rules: requirements, View: requirementView})
	return registrar{
		configurations: configurations,
		requirements:   requirementHandler,
		transactor:     db.Transactor(),
	}, nil
}

// execute 把一份登记输入推进到登记册答案：译装 → 在环境事务内交用例 → 答案译成
// 退出码。事务由本层给出正是写口 RequireExecutor 等的那一半；一次调用一个命令一笔
// 事务，登记与它的冲突判定读回因此看同一份快照。
func execute(ctx context.Context, command string, raw []byte, registrar registrar) (string, int) {
	dispatch, err := commandFor(command, raw)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", command, err), exitUsage
	}

	var outcome application.CaseConfigurationOutcome
	err = registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := dispatch(txCtx, registrar)
		outcome = handled
		return err
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
	}
	return configurationAnswer(command, outcome)
}

// configurationAnswer 把用例的封闭八格译成退出码，不增不减：
//   - 登记/重放/撤销落地/已撤销 → 0。已撤销归 0 而不归 2：撤销的意图是使失效，已失效
//     即达成；写口纪律「谁先撤销成功谁算」，册面原因归首撤者，未决重跑落在这一格时
//     不该再劳人工。
//   - 受理拒与撤销无对象 → 1：内容改对了才该重来，重跑同一份没有意义。
//   - 内容冲突 → 2：同键异内容绝不覆盖（先例：parcel-governance-register 的治理格），
//     要人核对既有登记再决定续办。
//   - 未决 → 3：用例把依赖故障折成 UNDECIDED，登记与否未知，重跑同一命令续办。
func configurationAnswer(command string, outcome application.CaseConfigurationOutcome) (string, int) {
	message := command + ": " + outcome.String()
	switch outcome {
	case application.ConfigurationRegistered, application.ConfigurationExisting,
		application.ConfigurationRevoked, application.ConfigurationAlreadyRevoked:
		return message, exitRegistered
	case application.ConfigurationNotAccepted, application.ConfigurationNotRegistered:
		return message, exitUsage
	case application.ConfigurationContentConflict:
		return message + "（同键已在册且内容不同——绝不覆盖，先核对既有登记）", exitConflict
	case application.ConfigurationUndecided:
		return message, exitUndecided
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", command, outcome), exitUndecided
	}
}
