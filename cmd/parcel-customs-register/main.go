// parcel-customs-register 是关务案件配置面五本登记册的受控进程入口（syn-wall-door-audit
// 票 06 B 半边）：运营方操作员在数据库网络内手跑，不监听任何端口——配置登记是治理动作，
// 不是在线请求面，走独立进程而不进 parcel-api 的端点表（先例：parcel-pricing-register、
// parcel-network-register、parcel-governance-register）。
//
// 命令表在 translate.go 的 allCommands，一处列全。册子分两族：案件配置族——就绪判断与
// 提交授权各带撤销半边（撤销是状态推进不是删除，原判断原样留在行内）；解释规则按（辖区，
// 法定生效起点）登记不可覆盖的版本（ADR-0070——换版即登记更晚起点的新版，开放前版终点
// 随之落定，历史区间不接受追改）；关闭义务与门禁前置条件各分目录与明细两个命令——
// 「目录登记了但清单空」是必须登得出来的一格，与「未登记」含义相反；建案要求规则
// （case-requirement）挡的是建案链第一步那堵 EstablishCaseUndecided 墙，「不要求建案」
// 也必须带依据登记，未登记是未决不是「不要求」；口岸目录与申报路径目录（candidate-port /
// declaration-path，票 admin-remainder-mechanism-batch/03）按（键，生效起点）登记版本，
// 代数同解释规则；监管凭证（regulatory-credential，票 sa-cc/07）是不可变版本，同身份换
// 期限 / 持有人 / 额度全是冲突——那是另一张凭证。税费付款协作与核对族（duty-collaboration /
// duty-payment-verification，票 sa-cc/07）走 UC-CC-009 步 4–7 的编排：协作事项按（范围，
// 税费引用）一格一行，核对按三维加内容指纹逐版追加、迟到事实按新版本进不覆盖；三轴与
// 关联依据由登记方交进来，本口不从金额相等推任何一轴。
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
// 退出码按恢复动作分四格（ADR-0029 判据），两族用例的答案各自原词进答复、各自归格：
// 0 = 已登记/幂等重放/撤销落地/已撤销（撤销的意图是使失效，已失效即达成——册面原因
// 归首撤者，未决重跑落在这一格时不该再劳人工）；1 = 用法或输入不合法（含受理门拒绝、
// 撤销无对象，以及核对无关联依据的`待关联`——改请求，不是重试）；2 = 内容冲突（同键异
// 内容绝不覆盖，人工核对既有登记再续办）；3 = 未决（依赖故障或撞上库上防线，登记与否
// 未知；也含协作事项等税费结果、核对等资金事实或协作事项这类前置未齐——续办动作同款：
// 等它到了重跑同一命令，答复里的原词分得开在等谁）。
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

// registrar 是本口的全部依赖：各登记用例 handler 加环境事务的来源。
type registrar struct {
	configurations     *application.RegisterCaseConfigurationHandler
	requirements       *application.RegisterCaseRequirementRuleHandler
	portsPaths         *application.RegisterPortsPathsHandler
	credentials        *application.RegisterCredentialHandler
	dutyReconciliation *application.DutyPaymentReconciliationHandler
	transactor         bentoapp.Transactor
}

// systemClock 给协作事项与核对的形成时间：那两个时刻不是登记输入，是「本口此刻形成」
// ——与目录登记的 registeredAt 由操作员交进来是两回事，后者是被登记事实的一部分。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

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

// buildRegistrar 装配真实登记链：八本册子写口加对应只读视图。读口不是可选的便利，
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
	portsPaths, err := adapter.NewPortsPathsRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造口岸路径写口：%w", err)
	}
	portsPathsView, err := adapter.NewPortsPathsPointView(db)
	if err != nil {
		return none, fmt.Errorf("构造口岸路径读口：%w", err)
	}
	credentialRegistry, err := adapter.NewCredentialRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造凭证写口：%w", err)
	}
	credentialView, err := adapter.NewCredentialView(db)
	if err != nil {
		return none, fmt.Errorf("构造凭证读口：%w", err)
	}
	// 协作事项、资金事实引用、付款核对三口在同一个适配器上（同一迁移的三张表）；编排
	// 的三个依赖都指它，资金事实那一口只被核对读前置，本口没有登它的命令。
	dutyReconciliation, err := adapter.NewDutyPaymentReconciliation(db)
	if err != nil {
		return none, fmt.Errorf("构造税费付款协作与核对写口：%w", err)
	}
	// 核对形成那一格同事务向 settlement-accounting 交信封（票 sa-cc/05）；本口一次调用一笔事务，
	// 意图与核对行因此同生共死。
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("构造 Outbox Store：%w", err)
	}
	verificationHandoff, err := adapter.NewOutboxDutyPaymentVerificationHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造税费付款核对交接口：%w", err)
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
		// 「税费付款」规则行的写读两半与目录 / 认定同一对适配器（票 sa-cc/06）；本 CLI 今天没有登这一格的
		// 子命令，接上只为让组合根不留 nil、后继登记面来接时一处可核。
		DutyRules:    gates,
		DutyRuleView: gateView,
		// 「要不要求付款人」那一格规则的写读两半与核对同一对适配器（票 sa-cc/12）；同上一格，本 CLI 今天
		// 没有登它的子命令。
		PayerRules:    dutyReconciliation,
		PayerRuleView: dutyReconciliation,
	})
	requirementHandler := application.NewRegisterCaseRequirementRuleHandler(
		application.RegisterCaseRequirementRuleDeps{Rules: requirements, View: requirementView})
	portsPathsHandler := application.NewRegisterPortsPathsHandler(
		application.RegisterPortsPathsDeps{Registry: portsPaths, View: portsPathsView})
	credentialHandler := application.NewRegisterCredentialHandler(
		application.RegisterCredentialDeps{Registry: credentialRegistry, View: credentialView})
	dutyReconciliationHandler, err := application.NewDutyPaymentReconciliationHandler(
		application.DutyPaymentReconciliationDeps{
			Collaborations: dutyReconciliation,
			Funds:          dutyReconciliation,
			Verifications:  dutyReconciliation,
			PayerRules:     dutyReconciliation,
			Handoff:        verificationHandoff,
			Clock:          systemClock{},
		})
	if err != nil {
		return none, fmt.Errorf("构造税费付款协作与核对编排：%w", err)
	}
	return registrar{
		configurations:     configurations,
		requirements:       requirementHandler,
		portsPaths:         portsPathsHandler,
		credentials:        credentialHandler,
		dutyReconciliation: dutyReconciliationHandler,
		transactor:         db.Transactor(),
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

// configurationResult 是案件配置族的答案（含凭证册——RegisterCredentialHandler 交回同一
// 套案件配置族的格）。
type configurationResult struct {
	outcome application.CaseConfigurationOutcome
}

func (result configurationResult) exit(command string) (string, int) {
	return configurationAnswer(command, result.outcome)
}

// dutyReconciliationResult 是税费付款协作与核对族的答案。
type dutyReconciliationResult struct {
	result application.DutyReconciliationResult
}

func (result dutyReconciliationResult) exit(command string) (string, int) {
	return dutyReconciliationAnswer(command, result.result.Outcome(), result.result.UndecidedReason())
}

// configurationAnswer 把用例（案件配置族）的封闭各格译成退出码，不增不减：
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

// dutyReconciliationAnswer 把税费付款协作与核对族的封闭各格译成退出码，不增不减。
// 一族一张表：资金事实那族格本口没有命令能交回，仍在表上——同一格不因来自哪条命令而换
// 退出码。归格只看恢复动作：
//   - 形成/重放（协作事项、资金事实、核对各自的两格）→ 0。核对没有「内容冲突」格：同三维
//     换内容是新版本追加（迟到事实按新版本进、不按到达顺序覆盖），答的仍是形成。
//   - 未受理与`待关联`→ 1。待关联是「无权威依据不关联」——金额相等、同范围、同付款人都不
//     单独构成依据，补上依据再登，重跑同一份没有意义；它不是失败，答复里原词可见。
//   - 协作事项 / 资金事实的内容冲突 → 2，同案件配置族那格的理由。
//   - 未决与两道前置未齐（资金事实未接收、协作事项未形成）→ 3。未决带编排指名的原因：
//     义务依据缺席（等 UC-CC-006 的核定税费或真实程序的无需付款依据）、程序要求付款人而
//     来源未提供（等来源补事实）、付款人规则未配置（等登记方补规则）是业务未决；存储或
//     读口不可用是依赖故障——续办动作不同，折成一个词就得让操作员猜。前置未齐的续办与
//     未决同款——等前置落册后重跑同一命令——所以同格，原词分得开在等谁。
func dutyReconciliationAnswer(
	command string,
	outcome application.DutyReconciliationOutcome,
	reason application.DutyReconciliationReason,
) (string, int) {
	message := command + ": " + outcome.String()
	switch outcome {
	case application.CollaborationFormed, application.CollaborationExisting,
		application.FundsFactReceived, application.FundsFactExisting,
		application.DutyVerificationFormed, application.DutyVerificationExisting:
		return message, exitRegistered
	case application.DutyReconciliationNotAccepted:
		return message, exitUsage
	case application.FundsFactPendingAssociation:
		return message + "（无权威关联依据不关联——补上依据再登，重跑同一份没有意义）", exitUsage
	case application.CollaborationContentConflict, application.FundsFactContentConflict:
		return message + "（同键已在册且内容不同——绝不覆盖，先核对既有登记）", exitConflict
	case application.FundsFactNotReceived, application.CollaborationNotFormed:
		return message + "（前置未齐——等它落册后重跑同一命令续办）", exitUndecided
	case application.DutyReconciliationUndecided:
		return message + "（" + reason.String() + "）", exitUndecided
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", command, outcome), exitUndecided
	}
}
