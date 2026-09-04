// parcel-governance-register 是治理登记的受控进程入口（syn-wall-door-audit 票 12）：
// 运营方操作员在数据库网络内手跑，不监听任何端口。治理登记的主体是租户运营方自己，
// 信任边界是运维边界而不是商业渠道，故走受控 CLI 而不进 parcel-api 的端点面
// （ADR-0055 的业务端点章程不辖此面；接入渠道那半在票 01 另行推进）。
//
// 首批只开三类（票 12 裁决）：权威区间（authority-interval——`OWNERSHIP_UNRESOLVED`
// 的恢复动作落点）、暂停（suspend）与恢复（resume，四件由领域把门，无简化路径）。
// 阶段评审与接管第二批，不在本入口。
//
// 执行者身份走双轨（票 12 裁决）：①通道技术身份（OS 进程属主、主机名）由本入口
// 自取，没有任何参数能传入或覆盖它，与登记同笔事务落 channel_execution 留痕；
// ②决定人/执行人（executedBy/decidedBy）是登记内容，由输入显式必填提供，册面语义
// 是「登记者声明了谁」。授权依据是「能运行本 CLI」这道运维边界加①的留痕，②只是
// 被记录的声明——这与 ADR-0022→ADR-0003 禁采信自报身份作授权那条链不冲突。
//
// 声明内容全部来自输入文件，进程不内置任何生产默认；真实角色名与「谁被允许运行
// 本 CLI」的授权方案属实例半边（PAR-GOV-03..07 待提供），留待租户运营方案。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 已登记/幂等重放；1 = 用法或输入不合法（含领域门拒绝与悬空引用）；
// 2 = 治理答案（权威冲突阻断、已入册但下游意图待续办——人工看过再走）；3 = 未决
// （依赖故障，登记与否未知，重跑同一命令即可续办）。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"os/user"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pgadapter "go.idp.xyz/idp-parcel/internal/pilotgovernance/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/application"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/domain"
	"go.idp.xyz/idp-parcel/internal/pilotgovernance/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitRegistered = 0
	exitUsage      = 1
	exitGovernance = 2
	exitUndecided  = 3
)

// 子命令词就是领域封闭集 domain.ChannelCommand 的字面拼法（票 pilot-governance-context-gaps/01）：
// 入口在这里按词认命令，留痕按枚举落——两边同一个来源，集合不再靠本文件的常量单独封闭。
var (
	commandAuthorityInterval = domain.ChannelCommandAuthorityInterval.String()
	commandSuspend           = domain.ChannelCommandSuspend.String()
	commandResume            = domain.ChannelCommandResume.String()
)

type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// channelIdentity 是身份双轨的第①轨。只由 currentChannelIdentity 从运行环境取得，
// 结构上不存在从参数或输入文件写入它的路径。
type channelIdentity struct {
	osUser   string
	hostname string
}

func currentChannelIdentity() (channelIdentity, error) {
	current, err := user.Current()
	if err != nil {
		return channelIdentity{}, fmt.Errorf("取 OS 进程属主：%w", err)
	}
	hostname, err := os.Hostname()
	if err != nil {
		return channelIdentity{}, fmt.Errorf("取主机名：%w", err)
	}
	return channelIdentity{osUser: current.Username, hostname: hostname}, nil
}

// executionTracer 把留痕库收窄成本工具消费的形状，测试用替身顶上。
type executionTracer interface {
	Append(ctx context.Context, execution pgadapter.ChannelExecution) error
}

var _ executionTracer = (*pgadapter.ChannelExecutions)(nil)

type registrars struct {
	incidents  *application.GovernIncidentHandler
	intervals  *application.RegisterAuthorityIntervalHandler
	tracer     executionTracer
	transactor bentoapp.Transactor
	clock      ports.Clock
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, getenv func(string) string, out, errOut io.Writer) int {
	if len(args) < 1 {
		fmt.Fprintf(errOut, "用法：parcel-governance-register <%s|%s|%s> -input <file>\n",
			commandAuthorityInterval, commandSuspend, commandResume)
		return exitUsage
	}
	command := args[0]
	if _, known := domain.ParseChannelCommand(command); !known {
		fmt.Fprintf(errOut, "未知登记种类 %q；首批只开 %s、%s、%s（阶段评审与接管第二批）\n",
			command, commandAuthorityInterval, commandSuspend, commandResume)
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

	identity, err := currentChannelIdentity()
	if err != nil {
		fmt.Fprintf(errOut, "取通道技术身份：%v\n", err)
		return exitUndecided
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
	regs, err := buildRegistrars(db)
	if err != nil {
		fmt.Fprintf(errOut, "装配登记口：%v\n", err)
		return exitUndecided
	}

	message, code := execute(ctx, command, raw, identity, regs)
	fmt.Fprintln(out, message)
	return code
}

// buildRegistrars 装配真实登记链：治理三库 + 权威区间册 + Outbox 交接 + 留痕库。
// 接管库照样接上——治理用例的依赖不留 nil 口，但本入口没有开它的命令。
func buildRegistrars(db *bentopg.DB) (registrars, error) {
	none := registrars{}
	suspensions, err := pgadapter.NewSuspensions(db)
	if err != nil {
		return none, fmt.Errorf("构造暂停库：%w", err)
	}
	resumptions, err := pgadapter.NewResumptions(db)
	if err != nil {
		return none, fmt.Errorf("构造恢复库：%w", err)
	}
	takeovers, err := pgadapter.NewTakeovers(db)
	if err != nil {
		return none, fmt.Errorf("构造接管库：%w", err)
	}
	intervals, err := pgadapter.NewAuthorityIntervals(db)
	if err != nil {
		return none, fmt.Errorf("构造权威区间册：%w", err)
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return none, fmt.Errorf("构造 Outbox Store：%w", err)
	}
	clock := systemClock{}
	downstream, err := pgadapter.NewOutboxGovernanceHandoff(db, outboxStore, clock)
	if err != nil {
		return none, fmt.Errorf("构造治理交接：%w", err)
	}
	tracer, err := pgadapter.NewChannelExecutions(db)
	if err != nil {
		return none, fmt.Errorf("构造留痕库：%w", err)
	}

	incidents := application.NewGovernIncidentHandler(application.GovernIncidentDeps{
		Suspensions: suspensions,
		Resumptions: resumptions,
		Takeovers:   takeovers,
		Intervals:   intervals,
		Downstream:  downstream,
		Clock:       clock,
	})
	registration := application.NewRegisterAuthorityIntervalHandler(application.RegisterAuthorityIntervalDeps{
		Intervals: intervals,
	})
	return registrars{
		incidents:  incidents,
		intervals:  registration,
		tracer:     tracer,
		transactor: db.Transactor(),
		clock:      clock,
	}, nil
}

// execute 把一份登记输入推进到登记册答案：翻译 → 同一笔事务内交用例并给落册/已在册
// 的执行留痕 → 答案译成退出码。留痕失败随事务翻成未决——登记不许在无痕状态下落地，
// 痕也不声称一笔没落库的登记。
func execute(
	ctx context.Context,
	command string,
	raw []byte,
	identity channelIdentity,
	regs registrars,
) (string, int) {
	channelCommand, known := domain.ParseChannelCommand(command)
	if !known {
		return fmt.Sprintf("未知登记种类 %q", command), exitUsage
	}
	switch channelCommand {
	case domain.ChannelCommandAuthorityInterval:
		interval, err := authorityIntervalFromJSON(raw)
		if err != nil {
			return fmt.Sprintf("%s: 输入被拒：%v", command, err), exitUsage
		}
		var result application.RegisterAuthorityIntervalResult
		err = regs.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			handled, err := regs.intervals.Handle(txCtx, interval)
			if err != nil {
				return err
			}
			result = handled
			return traceExecution(txCtx, regs, channelCommand, intervalReference(interval), identity,
				result.Outcome() == application.IntervalRegistered ||
					result.Outcome() == application.IntervalAlreadyRegistered,
				result.Outcome().String())
		})
		if err != nil {
			return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
		}
		return intervalAnswer(result.Outcome(), result.Conflicts())
	case domain.ChannelCommandSuspend:
		spec, err := suspensionSpecFromJSON(raw)
		if err != nil {
			return fmt.Sprintf("%s: 输入被拒：%v", command, err), exitUsage
		}
		var result application.GovernIncidentResult
		err = regs.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			handled, err := regs.incidents.Suspend(txCtx, spec)
			if err != nil {
				return err
			}
			result = handled
			return traceExecution(txCtx, regs, channelCommand, spec.ID.String(), identity,
				tracedIncidentOutcome(result.Outcome()), result.Outcome().String())
		})
		if err != nil {
			return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
		}
		return incidentAnswer(command, result.Outcome(), result.HandoffReference())
	case domain.ChannelCommandResume:
		spec, err := resumptionSpecFromJSON(raw)
		if err != nil {
			return fmt.Sprintf("%s: 输入被拒：%v", command, err), exitUsage
		}
		var result application.GovernIncidentResult
		err = regs.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
			handled, err := regs.incidents.Resume(txCtx, spec)
			if err != nil {
				return err
			}
			result = handled
			return traceExecution(txCtx, regs, channelCommand, spec.Suspension.String(), identity,
				tracedIncidentOutcome(result.Outcome()), result.Outcome().String())
		})
		if err != nil {
			return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
		}
		return incidentAnswer(command, result.Outcome(), result.HandoffReference())
	default:
		// ParseChannelCommand 只交回集合内的取值；这一支留给编译器之外的那种「集合加了格、这里没跟」。
		return fmt.Sprintf("登记种类 %q 尚无执行路径", command), exitUsage
	}
}

// tracedIncidentOutcome 圈定要留痕的治理答案：落册与已在册。拒绝与悬空引用没碰过
// 登记册，未决则连落没落都不知道——痕只陪真实到达登记册的执行。
func tracedIncidentOutcome(outcome application.GovernIncidentOutcome) bool {
	switch outcome {
	case application.SuspensionRecorded, application.SuspensionExisting,
		application.ResumptionRecorded, application.ResumptionExisting:
		return true
	default:
		return false
	}
}

func traceExecution(
	ctx context.Context,
	regs registrars,
	command domain.ChannelCommand,
	reference string,
	identity channelIdentity,
	landed bool,
	outcome string,
) error {
	if !landed {
		return nil
	}
	return regs.tracer.Append(ctx, pgadapter.ChannelExecution{
		Command:         command,
		RecordReference: reference,
		OSUser:          identity.osUser,
		Hostname:        identity.hostname,
		Outcome:         outcome,
		ExecutedAt:      regs.clock.Now(),
	})
}

// intervalReference 是留痕指名权威区间那一行的引用：四维身份加生效起止。
func intervalReference(interval domain.AuthorityInterval) string {
	reference := interval.ObjectScope + "/" + interval.Capability + "/" + interval.FactKind +
		"/" + interval.Authority + "/" + interval.From.UTC().Format(time.RFC3339)
	if !interval.To.IsZero() {
		reference += "/" + interval.To.UTC().Format(time.RFC3339)
	}
	return reference
}

// incidentAnswer 把治理用例答案译成退出码。续办引用非空优先成治理退出码：记录已
// 入册但下游意图没交出去，这一格要人看过再走，不能混进 0 里被忽略。
func incidentAnswer(command string, outcome application.GovernIncidentOutcome, handoffRef string) (string, int) {
	message := command + ": " + outcome.String()
	if handoffRef != "" {
		return fmt.Sprintf("%s（已入册，下游意图未交出去，续办引用 %s）", message, handoffRef), exitGovernance
	}
	switch outcome {
	case application.SuspensionRecorded, application.SuspensionExisting,
		application.ResumptionRecorded, application.ResumptionExisting:
		return message, exitRegistered
	case application.SuspensionNotFound, application.GovernIncidentNotAccepted:
		return message, exitUsage
	default:
		return message, exitUndecided
	}
}

// intervalAnswer 把权威区间登记答案译成退出码。冲突阻断带全部冲突对——处置者要
// 知道撞上了哪些区间。
func intervalAnswer(
	outcome application.RegisterAuthorityIntervalOutcome,
	conflicts []domain.AuthorityConflict,
) (string, int) {
	message := commandAuthorityInterval + ": " + outcome.String()
	switch outcome {
	case application.IntervalRegistered, application.IntervalAlreadyRegistered:
		return message, exitRegistered
	case application.IntervalConflictBlocked:
		detail := ""
		for _, conflict := range conflicts {
			detail += "\n  " + formatInterval(conflict.First) + " 撞 " + formatInterval(conflict.Second)
		}
		return message + detail, exitGovernance
	case application.IntervalNotAccepted:
		return message, exitUsage
	default:
		return message, exitUndecided
	}
}

func formatInterval(interval domain.AuthorityInterval) string {
	to := "开放"
	if !interval.To.IsZero() {
		to = interval.To.UTC().Format(time.RFC3339)
	}
	return fmt.Sprintf("[%s/%s/%s] %s（%s ~ %s）",
		interval.ObjectScope, interval.Capability, interval.FactKind,
		interval.Authority, interval.From.UTC().Format(time.RFC3339), to)
}
