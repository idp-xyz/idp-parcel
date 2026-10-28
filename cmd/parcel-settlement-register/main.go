// parcel-settlement-register 是 settlement-accounting「外部资金事实采用」这一口的受控进程入口（票 sa-cc/27
// 裁决 2 第一步）：结算运营或财务集成责任方在数据库网络内手跑，不监听任何端口。它给 ADR-0137 决定四保留的
// 那唯一一口装上门——外部资金事实进产品只经 SA 采用，`customs-compliance` 不开第二个铸造或补录入口，人工核实与
// 更正也先登在这里、再经采用信封到 CC。在此之前采用编排在生产依赖图上零装配，首版与更正都只有测试直写能到。
//
// 命令表在 translate.go 的 allCommands，一处列全：`external-funds-fact` 采用一条事实的首版，
// `external-funds-fact-correction` 采用一次外部更正，`settlement-account` 登记一笔结算账户。
// 资金两条是两个命令类型、两条用例方法，不共享入口：更正无回指时不能退化成首版。
// `funds-mapping`、`funds-application`、`funds-application-reversal` 把一笔已采用的资金映射到目标、按写全的分配形成核销、
// 撤销一次核销；映射、核销与核销撤销只有这一路进程入口，不与采用授予相通（ADR-0236 决定七不给它们开在线口）。
// 结算账户是另一族答案：固定属性冲突绝不覆盖，册上没有的账户也不在这里代拟。
// `allocation-form` 只登记一条分摊规则版本选用的分法，不登权重，也不在这里把来源金额展开成份额。
// `accounting-connector` 只登记某个对方采用规范文书。没登记的外部交换不在这里放行，本口也不解析账单文件或财务系统报文。
// `sell-billing-moment-adoption` 与 `fee-item-mapping` 登 ADR-0190 的两本租户取值册：一户采用哪种包裹计费时点、计价费用代码
// 映到哪个费用项目。本口不预选时点、不代拟映射，没登记的由读口答未找到。这两本册与 `buy-evaluation-trigger` 落行时同笔交
// 「评价参数已登记」，停着的评价请求由它续办（ADR-0229 决定五）。
// `evaluation-resume` 是运营续办口（ADR-0229 决定六）：点名一件包裹或一版发生项、给一个续办引用，续办在它上面停着的
// 每一件；同一个引用重跑是重放。它不登任何租户取值，缺的取值补登之前续办只会照实再停一次。
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
// （Outbox 意图与登记同生同灭），结算账户行与向 parcel-shipment 交的「结算账户已登记」信封同理，核销行、对被指向
// 金额的锁与向已结视图交的核销信封也同理；写口无环境事务即拒。
//
// 退出码按恢复动作分四格（ADR-0029 判据；归格见 fundsAnswer）：0 = 已采用 / 幂等重放（重放不是错误，无恢复
// 动作）；1 = 用法或输入不合法（含用例的`未受理`——回指非链头、更正未采用的事实、构造门拒——改请求，不是重试；
// 也含采用信封、核销信封或结算账户已登记信封被框架确定性校验拒收那一格——什么都没登记、要改的是引用的长度或形，
// 见 execute）；
// 2 = 内容冲突（同版本字面异内容、同事实第二个首版：绝不覆盖，人工核对既有登记再续办；也含作用域账不平那一格，
// 见 execute）；3 = 未决（依赖故障，
// 登记与否未知；也含「行已落、信封未出」那一格——它需要人重跑同一命令补发同一封，不能与已登记同格）。
package main

import (
	"context"
	"errors"
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
	sadomain "go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitRegistered = 0
	exitUsage      = 1
	exitConflict   = 2
	exitUndecided  = 3
)

// registrar 是本口的全部依赖：采用编排、结算账户登记，加环境事务的来源。
type registrar struct {
	funds        *application.MapExternalFundsHandler
	accounts     *application.RegisterSettlementAccountHandler
	catalogues   *application.RegisterSettlementCatalogueHandler
	grammars     *application.RegisterAmountGrammarHandler
	forms        *application.RegisterAllocationFormHandler
	ceilings     *application.RegisterAuditEscalationCeilingHandler
	triggers     *application.RegisterBuyEvaluationTriggerHandler
	connectors   *application.RegisterAccountingConnectorHandler
	fees         *application.RegisterPeriodicFeeHandler
	attributions *application.RegisterChargeAttributionHandler
	arrangements *application.RegisterStatementArrangementHandler
	sellTriggers *application.RegisterSellEvaluationTriggerHandler
	moments      *application.RegisterSettlementMomentHandler
	conditions   *application.RegisterChargeConfirmationConditionHandler
	cycles       *application.RegisterStatementCycleHandler
	adoptions    *application.RegisterPreAcceptanceEstimateAdoptionHandler
	sellMoments  *application.RegisterSellBillingMomentAdoptionHandler
	feeMappings  *application.RegisterFeeItemMappingHandler
	resumes      *application.OperatorResumeEvaluationHandler
	transactor   bentoapp.Transactor
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

// buildRegistrar 装配真实采用链（裁决 3）：编排的每一口都接真，一只都不留 nil、不放替身。映射写口、核销写口与
// 已结视图交接口不在采用 / 更正路径上也接真——「生产装配里不放任何替身」是 SA 装配处的既有纪律，且构造门
// （裁决 4）拒 nil，半装根本装不进去。资金事实交接口与登记册同一只 db、同一只 Outbox Store：事务壳给的环境事务
// 因此同时罩住版本行与向 CC 交的采用信封——两者同生共死，形照 parcel-customs-register 的 buildRegistrar。
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
	postings, err := sapostgres.NewCustomerReceiptPostings(db)
	if err != nil {
		return none, fmt.Errorf("构造客户收款余额重算：%w", err)
	}
	claimTargets, err := sapostgres.NewClaimSettlementTargets(db)
	if err != nil {
		return none, fmt.Errorf("构造赔付与追偿认可目标口：%w", err)
	}
	funds, err := application.NewMapExternalFundsHandler(application.MapExternalFundsDeps{
		Facts:        facts,
		Mappings:     mappings,
		Applications: applications,
		Downstream:   downstream,
		FactHandoff:  factHandoff,
		Clock:        systemClock{},
		Books:        postings,
		Balances:     postings,
		Standings:    postings,
		ClaimTargets: claimTargets,
		FactLocks:    applications,
	})
	if err != nil {
		return none, fmt.Errorf("构造外部资金采用编排：%w", err)
	}
	accounts, err := sapostgres.NewSettlementAccounts(db)
	if err != nil {
		return none, fmt.Errorf("构造结算账户登记册：%w", err)
	}
	// 「结算账户已登记」交接口与登记册同一只 db、同一只 Outbox Store，账户行与信封因此落在同一笔环境事务里：
	// 停在作用域那一格的委托等的就是这封信（ADR-0094 决定四，票 routing-first-cut/19）。
	accountHandoff, err := sapostgres.NewOutboxSettlementAccountRegisteredHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造结算账户已登记交接口：%w", err)
	}
	accountHandler, err := application.NewRegisterSettlementAccountHandler(accounts, accountHandoff, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造结算账户登记：%w", err)
	}
	catalogues, err := sapostgres.NewSettlementCatalogues(db)
	if err != nil {
		return none, fmt.Errorf("构造结算读口登记册：%w", err)
	}
	catalogueHandler, err := application.NewRegisterSettlementCatalogueHandler(
		catalogues, catalogues, catalogues, catalogues, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造结算读口登记：%w", err)
	}
	grammars, err := sapostgres.NewAmountGrammars(db)
	if err != nil {
		return none, fmt.Errorf("构造金额文法登记册：%w", err)
	}
	grammarHandler, err := application.NewRegisterAmountGrammarHandler(grammars, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造金额文法登记：%w", err)
	}
	formStore, err := sapostgres.NewAllocationForms(db)
	if err != nil {
		return none, fmt.Errorf("构造分摊分法登记册：%w", err)
	}
	formHandler, err := application.NewRegisterAllocationFormHandler(formStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造分摊分法登记：%w", err)
	}
	ceilings, err := sapostgres.NewAuditEscalationCeilings(db)
	if err != nil {
		return none, fmt.Errorf("构造越权升级上限登记册：%w", err)
	}
	ceilingHandler, err := application.NewRegisterAuditEscalationCeilingHandler(ceilings, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造越权升级上限登记：%w", err)
	}
	// 「评价参数已登记」交接口与 accountHandoff 同一只 db、同一只 Outbox Store：BUY 触发册、SELL 计费时点采用册与费用项目
	// 映射册的行与信封因此落在同一笔环境事务里，停在发起之前或映射不全的评价请求等的就是这封信（ADR-0229 决定五）。
	parameterHandoff, err := sapostgres.NewOutboxEvaluationParameterRegisteredHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造评价参数已登记交接口：%w", err)
	}
	triggerStore, err := sapostgres.NewBuyEvaluationTriggers(db)
	if err != nil {
		return none, fmt.Errorf("构造评价请求触发登记册：%w", err)
	}
	triggerHandler, err := application.NewRegisterBuyEvaluationTriggerHandler(triggerStore, parameterHandoff, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造评价请求触发登记：%w", err)
	}
	connectorStore, err := sapostgres.NewAccountingConnectors(db)
	if err != nil {
		return none, fmt.Errorf("构造账务连接器登记册：%w", err)
	}
	connectorHandler, err := application.NewRegisterAccountingConnectorHandler(connectorStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造账务连接器登记：%w", err)
	}
	feeStore, err := sapostgres.NewPeriodicFees(db)
	if err != nil {
		return none, fmt.Errorf("构造周期费用登记册：%w", err)
	}
	feeHandler, err := application.NewRegisterPeriodicFeeHandler(feeStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造周期费用登记：%w", err)
	}
	attributionStore, err := sapostgres.NewChargeAttributions(db)
	if err != nil {
		return none, fmt.Errorf("构造归属日判定登记册：%w", err)
	}
	attributionHandler, err := application.NewRegisterChargeAttributionHandler(attributionStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造归属日判定登记：%w", err)
	}
	arrangementStore, err := sapostgres.NewStatementArrangements(db)
	if err != nil {
		return none, fmt.Errorf("构造对账安排登记册：%w", err)
	}
	arrangementHandler, err := application.NewRegisterStatementArrangementHandler(arrangementStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造对账安排登记：%w", err)
	}
	sellTriggerStore, err := sapostgres.NewSellEvaluationTriggers(db)
	if err != nil {
		return none, fmt.Errorf("构造 SELL 评价触发登记册：%w", err)
	}
	sellTriggerHandler, err := application.NewRegisterSellEvaluationTriggerHandler(sellTriggerStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造 SELL 评价触发登记：%w", err)
	}
	momentStore, err := sapostgres.NewSettlementMoments(db)
	if err != nil {
		return none, fmt.Errorf("构造结算触发登记册：%w", err)
	}
	momentHandler, err := application.NewRegisterSettlementMomentHandler(momentStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造结算触发登记：%w", err)
	}
	conditionStore, err := sapostgres.NewChargeConfirmationConditions(db)
	if err != nil {
		return none, fmt.Errorf("构造确认条件目录：%w", err)
	}
	conditionHandler, err := application.NewRegisterChargeConfirmationConditionHandler(conditionStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造确认条件登记：%w", err)
	}
	cycleStore, err := sapostgres.NewStatementCycles(db)
	if err != nil {
		return none, fmt.Errorf("构造对账周期册：%w", err)
	}
	cycleHandler, err := application.NewRegisterStatementCycleHandler(cycleStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造对账周期登记：%w", err)
	}
	adoptionStore, err := sapostgres.NewPreAcceptanceEstimateAdoptions(db)
	if err != nil {
		return none, fmt.Errorf("构造受理前估价采用册：%w", err)
	}
	// 「受理前估价采用已登记」交接口同上：采用行与信封落在同一笔环境事务里，停在金额那一格的委托等的就是这封信
	// （ADR-0179 决定五、ADR-0094 决定四）。
	adoptionHandoff, err := sapostgres.NewOutboxPreAcceptanceEstimateAdoptionRegisteredHandoff(db, outboxStore, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造受理前估价采用已登记交接口：%w", err)
	}
	adoptionHandler, err := application.NewRegisterPreAcceptanceEstimateAdoptionHandler(adoptionStore, adoptionHandoff, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造受理前估价采用登记：%w", err)
	}
	sellMomentStore, err := sapostgres.NewSellBillingMomentAdoptions(db)
	if err != nil {
		return none, fmt.Errorf("构造 SELL 计费时点采用册：%w", err)
	}
	sellMomentHandler, err := application.NewRegisterSellBillingMomentAdoptionHandler(sellMomentStore, parameterHandoff, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造 SELL 计费时点采用登记：%w", err)
	}
	feeItemMappingStore, err := sapostgres.NewFeeItemMappings(db)
	if err != nil {
		return none, fmt.Errorf("构造费用项目映射册：%w", err)
	}
	feeItemMappingHandler, err := application.NewRegisterFeeItemMappingHandler(feeItemMappingStore, parameterHandoff, systemClock{})
	if err != nil {
		return none, fmt.Errorf("构造费用项目映射登记：%w", err)
	}
	resumes, err := buildEvaluationResume(db, outboxStore)
	if err != nil {
		return none, err
	}
	return registrar{
		resumes:      resumes,
		funds:        funds,
		accounts:     accountHandler,
		catalogues:   catalogueHandler,
		grammars:     grammarHandler,
		forms:        formHandler,
		ceilings:     ceilingHandler,
		triggers:     triggerHandler,
		connectors:   connectorHandler,
		fees:         feeHandler,
		attributions: attributionHandler,
		arrangements: arrangementHandler,
		sellTriggers: sellTriggerHandler,
		moments:      momentHandler,
		conditions:   conditionHandler,
		cycles:       cycleHandler,
		adoptions:    adoptionHandler,
		sellMoments:  sellMomentHandler,
		feeMappings:  feeItemMappingHandler,
		transactor:   db.Transactor(),
	}, nil
}

// execute 把一份登记输入推进到采用答案：译装 → 在环境事务内交用例 → 答案译成退出码。译装失败当场拒、不进事务
// ——用法错误与「登记与否未知」是两个退出码，让它进了事务就分不开了。一次调用一个命令一笔事务，采用与它的
// 重放 / 冲突判定读回、以及向 CC 交的信封因此看同一份快照。
func execute(ctx context.Context, command string, raw []byte, registrar registrar) (string, int) {
	dispatch, err := commandFor(command, raw)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", command, err), exitUsage
	}

	var message string
	var code int
	err = registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		answered, answeredCode, err := dispatch(txCtx, registrar)
		message, code = answered, answeredCode
		return err
	})
	if errors.Is(err, ports.ErrFundsFactHandoffRejected) {
		// 采用信封被框架确定性校验拒收（票 sa-cc/32 裁决 2）：事务已回滚，版本行与信封一样都没落。归用法格而不是
		// 未决，按 ADR-0029 的恢复动作判：同一份输入重投永远同一个结果，要改的是引用的长度或形，不是重试——归 3
		// 会让人按未决那句「重跑补发」重跑到预算耗尽。答复点名框架给的原因，操作者据此改输入。
		return fmt.Sprintf("%s: 未登记——采用信封被框架确定性拒收，要改的是输入引用的长度或形，同一份再交仍会被拒：%v", command, err), exitUsage
	}
	if errors.Is(err, ports.ErrSettlementApplicationHandoffRejected) {
		// 归格理由同采用信封那一格：核销行随信封一起回滚，同一份输入再交仍会被拒，要改的是核销引用的长度或形。
		return fmt.Sprintf("%s: 未登记——核销信封被框架确定性拒收，要改的是核销引用的长度或形，同一份再交仍会被拒：%v", command, err), exitUsage
	}
	if errors.Is(err, ports.ErrSettlementAccountHandoffRejected) {
		// 归格理由同上一格：账户行随信封一起回滚，同一份输入再交仍会被拒，要改的是账户标识的长度。
		return fmt.Sprintf("%s: 未登记——结算账户已登记信封被框架确定性拒收，要改的是账户标识的长度或形，同一份再交仍会被拒：%v", command, err), exitUsage
	}
	if errors.Is(err, ports.ErrPreAcceptanceEstimateAdoptionHandoffRejected) {
		return fmt.Sprintf("%s: 未登记——受理前估价采用已登记信封被框架确定性拒收，要改的是账户标识的长度或形，同一份再交仍会被拒：%v", command, err), exitUsage
	}
	if errors.Is(err, sadomain.ErrNotReceivableAccount) {
		// 映射目标写成了一个不是应收的结算账户：改目标才行，重交同一份永远同一个结果（ADR-0029），不是未决。
		return fmt.Sprintf("%s: 未登记——映射目标不是本租户的应收结算账户，改目标再交：%v", command, err), exitUsage
	}
	if errors.Is(err, sadomain.ErrNegativeConfirmedUnsettled) || errors.Is(err, sadomain.ErrInvalidClaimTargetStanding) {
		// 账已经不平：作用域按账求出的已确认未结为负（多半是一笔超额的对账单核销，或确认之后的贷记调整），或一条赔付、
		// 追偿认可上的已核销越过了它的金额本体。这一笔连同重算一起回滚；重交同一份永远同一个结果，续办两格相同——
		// 要人核对指向它的核销与调整、撤掉多出的那一笔——归冲突格，不是未决。
		return fmt.Sprintf("%s: 未登记——账不平（作用域已确认未结为负，或金额本体上的已核销越过了它的金额），先核对指向它的核销与调整再续办：%v", command, err), exitConflict
	}
	if errors.Is(err, sadomain.ErrCatalogueTargetMissing) {
		return fmt.Sprintf("%s: 未登记——指向的结算账户还不在账户册上，先登记账户再交这一行：%v", command, err), exitUsage
	}
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
	}
	return message, code
}

// subjectOf 取已采用 / 已存在那一版的事实与版本字面，打进答复：操作员看的是「哪条事实的哪一版落了」，不是一个词。
func subjectOf(result application.FundsResult) string {
	record, found := result.Fact()
	if !found {
		return ""
	}
	return record.Key.Fact.String() + " 版本 " + record.Fact.Version().String()
}

// correctionSubjectOf 在 subjectOf 之后补两类话。更正把事实金额改到了未撤销核销的合计以下时说「累计超核」——与映射、核销
// 两类答复同一句话，不截成 0；撤哪一笔是人的判断，本上下文不代撤。事实落到的作用域账已不平、推进不了时点名那个作用域：
// 更正照采、入账余额留在原值，要人核对指向它的核销与调整。两种情形更正都照采（外部系统拥有资金事实），答复仍是已采用、
// 退出码不变，这些话与读面上的负数是操作员知道还有事要人做的仅有的信号（ADR-0285）。
func correctionSubjectOf(result application.FundsResult) string {
	subject := subjectOf(result)
	record, found := result.Fact()
	if over := result.OverAppliedMinor(); over > 0 && found {
		subject += fmt.Sprintf("；事实 %s 累计超核 %d", record.Key.Fact.String(), over)
	}
	for _, scope := range result.UnreconciledScopes() {
		subject += fmt.Sprintf("；作用域 法人 %s、账户 %s、币种 %s 账已不平，入账余额没有跟着新金额推进——先核对指向它的核销与调整，账回平后下一次映射、核销、撤销或更正会推进它",
			scope.LegalEntity().String(), scope.Account().String(), scope.Currency().String())
	}
	return subject
}

// applicationSubjectOf 取核销或撤销落在哪一笔核销上，与这一步之后该事实的累计未分配：同一笔钱上的别的核销也算在里头，
// 部分核销留下的、撤销还回来的未分配都让操作员看得见。撤销那两格不报原核销的金额——撤销之后它已不在核。
func applicationSubjectOf(result application.FundsResult) string {
	record, found := result.Application()
	if !found {
		return ""
	}
	subject := "核销 " + record.Key.Application.String()
	switch result.Outcome() {
	case application.ApplicationReversedOutcome, application.ApplicationAlreadyReversed:
		subject += " 已撤销"
	default:
		subject += fmt.Sprintf("：本笔 %d", record.Application.AppliedMinor())
	}
	return subject + factBalanceOf(record.Application.Fact().String(), result)
}

// mappingSubjectOf 只在映射让这条事实落到几个客户应收作用域上时开口：一个变两个的那一刻原来那个作用域不再记没核销的
// 部分，入账余额会降，操作员要知道余额为什么动了、钱停在哪（ADR-0283）。落到一个或零个作用域的映射只报结果格。
func mappingSubjectOf(result application.FundsResult) string {
	record, found := result.Mapping()
	posting, posted := result.FactScopePosting()
	if !found || !posted || !posting.ByApplication() {
		return ""
	}
	return "映射 " + record.Key.Mapping.String() + factBalanceOf(record.Mapping.Fact().String(), result)
}

// factBalanceOf 报这一步之后该事实的累计未分配；外部更正把事实金额改到了已核销合计以下时报累计超核——负数就是超核，
// 照实说出来，不截成 0，映射与核销两类答复说同一句话（更正自己的答复也说这一句，见 correctionSubjectOf）。
func factBalanceOf(fact string, result application.FundsResult) string {
	unapplied, known := result.FactUnappliedMinor()
	switch {
	case !known:
		return ""
	case unapplied < 0:
		return fmt.Sprintf("；事实 %s 累计超核 %d", fact, -unapplied)
	default:
		return fmt.Sprintf("；事实 %s 累计未分配 %d", fact, unapplied) + scopeNoteOf(result)
	}
}

// scopeNoteOf 说累计未分配在入账余额上怎么算、怎么续办：事实落到几个客户应收作用域时它不在任何一个作用域的入账余额里
// 当成钱——收款的这一截哪个作用域都不记，退回的这一截每个作用域都先扣；把它核到作用域上余额才各归各位（ADR-0283）。
// 只落到一个作用域时不开口，那一截在那个作用域的入账余额里。
func scopeNoteOf(result application.FundsResult) string {
	posting, posted := result.FactScopePosting()
	if !posted || !posting.ByApplication() {
		return ""
	}
	if posting.Kind == sadomain.FundsReturned {
		return fmt.Sprintf("（落在 %d 个客户应收作用域上，按核销记入：未分配的退回在每个作用域都先扣，把它核到钱离开的那个账户上就只扣那一边）",
			posting.Scopes)
	}
	return fmt.Sprintf("（落在 %d 个客户应收作用域上，按核销记入：未分配不计入任何作用域，核到哪个作用域那里才记）", posting.Scopes)
}

// fundsAnswer 把 `application.FundsOutcome` 的封闭各格译成退出码，不增不减。一族一张表：采用、映射、核销与撤销几条
// 命令共用——同一格不因来自哪条命令而换退出码（parcel-customs-register 的 dutyReconciliationAnswer 同一条纪律）。
// 归格只看恢复动作（裁决 5）：
//   - 已采用 / 已存在 / 已映射 / 已核销 / 已撤销 / 重放各格 → 0。重放不是错误，无恢复动作。
//   - `未受理` 与业务负向各格（不可映射的失败付款、分配失衡、跨币种无换算）→ 1：内容改对了才该重来，重跑同一份
//     没有意义。回指非链头、更正一条未采用的事实、构造门拒、映射到不存在或不属本租户的赔付与追偿认可、撤销一笔
//     不存在的核销、分给对账单超出该作用域已确认应收的分配都落在`未受理`；分到一条赔付或认可上超出它未核销的部分、
//     同一事实上的核销合计超出事实金额都落在分配失衡。
//   - 内容冲突各格 → 2：同键异内容绝不覆盖，要人核对既有登记再决定续办。
//   - 未决 → 3，带编排指名的原因（哪一本库读不回）。
//   - 行已落、信封未出（续办引用非空）→ 3 并把续办引用打出：事实已采用、核销已落是真的，但下游等的那封没出去，
//     重跑同一命令会重发同一份（Outbox 按认领键吞重）；它需要人动手，不能与已登记同格。自票 sa-cc/32 起这一格只对依赖故障
//     成立——信封被确定性拒收的那一格编排返 error、在 execute 归用法格，到不了这里；「重跑补发」因此只对还能补发的
//     因说话。
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
	case application.FundsMappingConflict:
		return message + "（同键已在册且内容不同，或这条事实已整条认作代收本金、已有别的映射——绝不覆盖，先核对既有登记）", exitConflict
	case application.FundsFactConflict, application.ApplicationConflict:
		return message + "（同键已在册且内容不同——绝不覆盖，先核对既有登记）", exitConflict
	case application.FundsUndecided:
		return message + "（" + reason.String() + "）", exitUndecided
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", command, outcome), exitUndecided
	}
}

func accountAnswer(command string, effect ports.SettlementAccountRegistrationEffect, accountID string) (string, int) {
	switch effect {
	case ports.SettlementAccountRegistered:
		return command + ": 已登记（" + accountID + "）", exitRegistered
	case ports.SettlementAccountReplay:
		return command + ": 已存在（" + accountID + "）", exitRegistered
	case ports.SettlementAccountConflict:
		return command + ": 冲突（" + accountID + "，同键已在册且内容不同——绝不覆盖，先核对既有登记）", exitConflict
	default:
		return fmt.Sprintf("%s: 未知应用结果 %d", command, effect), exitUndecided
	}
}

func adoptionAnswer(command string, effect ports.PreAcceptanceEstimateAdoptionEffect, accountID string) (string, int) {
	switch effect {
	case ports.PreAcceptanceEstimateAdoptionRegistered:
		return command + ": 已登记（" + accountID + "）", exitRegistered
	case ports.PreAcceptanceEstimateAdoptionReplay:
		return command + ": 已存在（" + accountID + "）", exitRegistered
	case ports.PreAcceptanceEstimateAdoptionConflict:
		return command + ": 冲突（" + accountID + "，这一户已有采用且内容不同——采用册不设修订，绝不覆盖）", exitConflict
	default:
		return fmt.Sprintf("%s: 未知应用结果 %d", command, effect), exitUndecided
	}
}

func catalogueAnswer(command string, effect ports.CatalogueRegistrationEffect, subject string) (string, int) {
	switch effect {
	case ports.CatalogueRegistered:
		return command + ": 已登记（" + subject + "）", exitRegistered
	case ports.CatalogueReplay:
		return command + ": 已存在（" + subject + "）", exitRegistered
	case ports.CatalogueConflict:
		return command + ": 冲突（" + subject + "，同键已在册且内容不同——绝不覆盖，先核对既有登记）", exitConflict
	default:
		return fmt.Sprintf("%s: 未知应用结果 %d", command, effect), exitUndecided
	}
}
