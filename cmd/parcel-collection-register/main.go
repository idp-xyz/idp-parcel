// parcel-collection-register 是代收与清分登记面的受控进程入口（票
// admin-remainder-mechanism-batch/04）：运营方操作员在数据库网络内手跑，不监听任何端口
// ——受托代收资金的登记与记账是运营动作，不是在线请求面，走独立进程而不进 parcel-api
// 的端点表（先例：parcel-customs-register、parcel-governance-register）。
//
// 七个命令覆盖 CONTEXT 的五个词形加分户账两面：instruction（代收指令）、subledger
// （分户账开立）、fact（代收事实）、discrepancy（差异事项）、batch（回汇批次形成）、
// hand-over（交出汇付主张）、posting（分户账记账）。
//
// **记账输入里没有分户账键，也没有币种**：键由依据推出，币种取自那本账。这不是省字段
// ——传键的写法在库里看不出记错账，传币种的写法在库里看不出记错币种。
//
// 输入全部来自 -input 指定的 JSON 文件，未知字段一律拒绝；进程不内置任何生产默认——
// 客户、责任法人、代收渠道、币种与金额全属实例半边（PA-CR-01 形态已准入，实例值留空），
// 汇率、回汇周期、手续费与支付通道更不进本口。验证用脱敏合成值（SYN- 前缀，ADR-0078，
// S 级只记 S）。环境事务由本入口给出（写口 RequireExecutor 无环境事务即拒），一次调用
// 一笔事务——余额守卫要的「读回账面与落账看同一份快照」就靠这一点。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：
//
//	0 = 已登记/幂等重放/已交出汇付主张/已在已交出态（意图达成）；
//	1 = 用法或输入不合法（含受理门拒——改请求，重跑同一份没有意义）；
//	2 = 要人看过再走：内容冲突（同键异内容绝不覆盖）、依据不在册（先把上游那一笔登进来）、
//	    批次已交出（成员只增不改，要继续归集就形成新批次）；
//	3 = 未决（依赖故障，登记与否未知，重跑同一命令即可续办）；
//	4 = 账上钱不够（来源位置余额不足）。它单独一格：请求本身没错，等实收或先清分之后
//	    重跑同一命令就会成——与 3 的区别是这一格**确定没落账**，而 3 连落没落都不知道。
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

	adapter "go.idp.xyz/idp-parcel/internal/collectionremittance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/collectionremittance/application"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitRegistered  = 0
	exitUsage       = 1
	exitNeedsReview = 2
	exitUndecided   = 3
	exitUnderfunded = 4
)

const (
	commandInstruction = "instruction"
	commandSubledger   = "subledger"
	commandFact        = "fact"
	commandDiscrepancy = "discrepancy"
	commandBatch       = "batch"
	commandHandOver    = "hand-over"
	commandPosting     = "posting"
)

// allCommands 的次序就是登记的依赖次序：指令与分户账先行，事实/批次/差异事项挂在
// 指令或分户账上，记账最后——外键链与依据门都按这个方向立。
var allCommands = []string{
	commandInstruction, commandSubledger, commandFact,
	commandDiscrepancy, commandBatch, commandHandOver, commandPosting,
}

// registrar 是本口的全部依赖：一个用例 handler 加环境事务的来源。
type registrar struct {
	handler    *application.Handler
	transactor bentoapp.Transactor
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintf(errOut, "用法：parcel-collection-register <%s> -input <file>\n",
			strings.Join(allCommands, "|"))
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

// buildRegistrar 装配真实登记链：三对写口读口。读口不是可选的便利，冲突判定与余额守卫
// 都靠它——只有写口时「已在册」永远说不出是重放还是改内容，账上够不够扣也无从判起。
func buildRegistrar(db *bentopg.DB) (registrar, error) {
	none := registrar{}
	collections, err := adapter.NewCollectionRegistrations(db)
	if err != nil {
		return none, fmt.Errorf("构造代收面写口：%w", err)
	}
	collectionView, err := adapter.NewCollectionPointView(db)
	if err != nil {
		return none, fmt.Errorf("构造代收面读口：%w", err)
	}
	subledgers, err := adapter.NewSubledgerStore(db)
	if err != nil {
		return none, fmt.Errorf("构造分户账写口：%w", err)
	}
	subledgerView, err := adapter.NewSubledgerBalanceView(db)
	if err != nil {
		return none, fmt.Errorf("构造分户账读口：%w", err)
	}
	remittances, err := adapter.NewRemittanceStore(db)
	if err != nil {
		return none, fmt.Errorf("构造回汇写口：%w", err)
	}
	remittanceView, err := adapter.NewRemittanceBatchView(db)
	if err != nil {
		return none, fmt.Errorf("构造回汇读口：%w", err)
	}

	handler := application.NewHandler(application.Deps{
		Collections: collections,
		Collection:  collectionView,
		Subledgers:  subledgers,
		Subledger:   subledgerView,
		Remittances: remittances,
		Remittance:  remittanceView,
	})
	return registrar{handler: handler, transactor: db.Transactor()}, nil
}

// execute 把一份登记输入推进到答案：译装 → 在环境事务内交用例 → 答案译成退出码。
// 一次调用一个命令一笔事务，记账的账面读回与落账因此看同一份快照。
func execute(ctx context.Context, command string, raw []byte, registrar registrar) (string, int) {
	dispatch, err := dispatcherFor(command, raw)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", command, err), exitUsage
	}

	var outcome application.Outcome
	err = registrar.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := dispatch(txCtx, registrar.handler)
		outcome = handled
		return err
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
	}
	return answer(command, outcome)
}

type dispatcher func(context.Context, *application.Handler) (application.Outcome, error)

func dispatcherFor(command string, raw []byte) (dispatcher, error) {
	switch command {
	case commandInstruction:
		parsed, err := instructionCommandFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, handler *application.Handler) (application.Outcome, error) {
			return handler.RegisterInstruction(ctx, parsed)
		}, nil
	case commandSubledger:
		parsed, err := subledgerCommandFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, handler *application.Handler) (application.Outcome, error) {
			return handler.OpenSubledger(ctx, parsed)
		}, nil
	case commandFact:
		parsed, err := factCommandFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, handler *application.Handler) (application.Outcome, error) {
			return handler.AcceptFact(ctx, parsed)
		}, nil
	case commandDiscrepancy:
		parsed, err := discrepancyCommandFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, handler *application.Handler) (application.Outcome, error) {
			return handler.RegisterDiscrepancy(ctx, parsed)
		}, nil
	case commandBatch:
		parsed, err := batchCommandFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, handler *application.Handler) (application.Outcome, error) {
			return handler.FormBatch(ctx, parsed)
		}, nil
	case commandHandOver:
		parsed, err := handOverCommandFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, handler *application.Handler) (application.Outcome, error) {
			return handler.HandOverBatch(ctx, parsed)
		}, nil
	case commandPosting:
		parsed, err := postingCommandFromJSON(raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, handler *application.Handler) (application.Outcome, error) {
			return handler.PostSubledger(ctx, parsed)
		}, nil
	default:
		return nil, fmt.Errorf("未知登记命令 %q", command)
	}
}

// answer 把用例的封闭十格译成退出码，不增不减。分组的判据是**处置动作**：能直接接着
// 干的归 0，要改请求的归 1，要人看一眼再决定的归 2，不知道落没落的归 3，确定没落账但
// 等一等就能成的归 4。
func answer(command string, outcome application.Outcome) (string, int) {
	message := command + ": " + outcome.String()
	switch outcome {
	case application.OutcomeRegistered, application.OutcomeExisting,
		application.OutcomeHandedOver, application.OutcomeAlreadyHandedOver:
		return message, exitRegistered
	case application.OutcomeNotAccepted:
		return message, exitUsage
	case application.OutcomeContentConflict:
		return message + "（同键已在册且内容不同——绝不覆盖，先核对既有登记）", exitNeedsReview
	case application.OutcomeBasisMissing:
		return message + "（依据不在册：先登代收指令/分户账/回汇批次/差异事项/原记账）", exitNeedsReview
	case application.OutcomeBatchClosed:
		return message + "（批次已交出汇付主张，成员只增不改——要继续归集请形成新批次）", exitNeedsReview
	case application.OutcomeUndecided:
		return message, exitUndecided
	case application.OutcomeUnderfunded:
		return message + "（来源位置余额不足——等实收或先清分后重跑同一命令）", exitUnderfunded
	default:
		// 用例交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", command, outcome), exitUndecided
	}
}
