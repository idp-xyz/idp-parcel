// parcel-frontline-import 是一线作业过渡期的受控批量导入口（ADR-0089 机制半边，票
// frontline-transition-import/01）。ADR-0021 裁定的一线作业客户端尚未开工，过渡期由现场
// 按汇总模板记录作业事实、内勤汇总、开发方受控运维在数据库网络内手跑本口灌入**既有**
// 命令用例。它不监听端口、不进 parcel-api 的端点表、不进任何准入放行表，形照
// cmd/parcel-*-register 家族；管理台零改动——「管理台无作业动词」是 ADR-0021 的有意设计，
// 过渡不废它。
//
// 三条不可商量的形状：
//
//   - **结构性拆除期限在 run 的第一行**（见 guardStructuralSunset）。过期即启动拒绝、退出
//     非零，没有环境变量或参数可绕过；延长只能经 supersede ADR-0089 改源码发版。时钟经
//     run 注入，测试把钟拨到期限后证明拒绝。
//   - **导入事实与设备事实可区分**：来源身份与证据引用都带 `FTI/<模板版本>/…` 标记（收寄
//     见 intakeSourceID / intakeEvidence，集运见 consolidationSourceID / consolidationEvidence），
//     录入操作者在证据引用里。ADR-0023 把作业事实的身份与发生时间的签发权判给设备；本口
//     是那条规则之外带期限的例外，标记是例外的印记，让派生与取证任何时候都能把这批事实
//     单独挑出来。
//   - **只灌既有用例、不造用例、不改 domain**。四类现场事实里收寄与装箱封签两类对得上且
//     有落点（集运六口收 domain.WorkFactSource 之后，来源标记才有了落点），换单与称重仍无
//     用例；判定与缺口写在 .scratch/frontline-transition-import/fact-to-usecase-mapping.md，
//     不在这里复述第二份。
//
// 收寄子命令今天接的身份核对缝是显式未配置替身（同 parcel-api 的处置，见
// unconfiguredParcelIdentityView）：`RECEIVED` 行必然答未决且不落库，本口开工前把这一点
// 打印出来。不是缺参数，是 PS 侧机制未建，恢复动作在那一侧。集运子命令没有这条缝：成员是
// 作业实物号，不经 PS 侧解析，模板行按顺序推进六口即可。
//
// 输入是一份 CSV 模板（-file），租户由 -tenant 给出（ADR-0003：租户是最高隔离边界，由受控
// 运维给出，不让文件自报）。模板整体不合格整批不开工；合格后一行一笔事务，部分成功是
// 常态，逐行结果打到标准输出。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 全部行已落地或重放；1 = 用法或模板不合格（整批未开工）；2 = 有行被拒
// （来源冲突，集运口还有单元不在册、成员在别处、三相不允许——都要改文件，重跑同一份不会
// 变）；3 = 有行未决或装配失败（重跑同一文件续办，已落地的行答重放）；4 = 已过结构性拆除
// 期限（启动拒绝）。
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
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitLanded    = 0
	exitUsage     = 1
	exitRejected  = 2
	exitUndecided = 3
	exitSunset    = 4
)

// 子命令按事实类型分：收寄与装箱封签两类有既有用例可接；换单与称重无用例，按 ADR-0089
// 细则⑤ 如实记缺口不造，判定见映射表。
const (
	commandIntake        = "intake"
	commandConsolidation = "consolidation"
)

var allCommands = []string{commandIntake, commandConsolidation}

// systemClock 是 Clock 端口的生产实现：要的是真实时钟，`time.Now()` 就是它。期限守卫
// 与收寄记录的系统接收时间都从这一个钟读。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now() }

// errParcelIdentityViewNotConfigured 见 unconfiguredParcelIdentityView 的注释。
var errParcelIdentityViewNotConfigured = errors.New("parcel-frontline-import: parcel identity view is not configured")

// unconfiguredParcelIdentityView 是身份核对缝的「显式未配置」，与 parcel-api 装配点的同名
// 替身同形同理：PS 侧的外部标识关联模型还不存在（.scratch/ps-external-mark-relations），
// 这条缝今天没有可接的提供方机制。端口合同写着「依赖调不通作为错误返回」，据此它对
// 每次核对如实报错，编排停在收寄待确认并携续办引用、不落库。
//
// 绝不交回空候选顶替：零候选是「查过了，查无此标识」的业务答案（走待识别、真实建立
// 收寄与控制），拿它顶「没查」会把一件没核对过身份的实物记成已核对。也绝不让内勤在
// 模板里自报正式包裹身份——「现场人员不得……决定正式包裹身份」是 CONTEXT 硬句。PS 侧
// 提供方落地后这里换真适配器，本口其余一格不动（buildIntakeImporter 的 identity 参数
// 就是那个换点）。
type unconfiguredParcelIdentityView struct{}

var _ ports.ParcelIdentityView = unconfiguredParcelIdentityView{}

func (unconfiguredParcelIdentityView) ResolveParcelIdentity(
	context.Context,
	domain.TenantID,
	ports.ExternalMarkObservation,
) ([]domain.ParcelAssociationReference, error) {
	return nil, errParcelIdentityViewNotConfigured
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Getenv, systemClock{}, os.Stdout, os.Stderr))
}

// run 是进程口的全部编排，时钟与环境经参数注入以便测试拨钟、断环境。期限守卫是第一
// 条语句：在读任何参数、环境或文件之前——过期的导入口连用法都不该解释。
func run(
	ctx context.Context,
	args []string,
	getenv func(string) string,
	clock ports.Clock,
	out, errOut io.Writer,
) int {
	if err := guardStructuralSunset(clock.Now()); err != nil {
		fmt.Fprintln(errOut, err)
		return exitSunset
	}

	if len(args) < 1 {
		fmt.Fprintf(errOut, "用法：parcel-frontline-import <%s> -tenant <租户> -file <模板 CSV>\n", strings.Join(allCommands, "|"))
		return exitUsage
	}
	command := args[0]
	if !knownCommand(command) {
		fmt.Fprintf(errOut, "未知导入命令 %q（支持 %s）\n", command, strings.Join(allCommands, " / "))
		return exitUsage
	}

	flags := flag.NewFlagSet(command, flag.ContinueOnError)
	flags.SetOutput(errOut)
	tenantFlag := flags.String("tenant", "", "租户标识（受控运维给出，不由模板自报）")
	file := flags.String("file", "", "汇总模板 CSV 路径")
	if err := flags.Parse(args[1:]); err != nil {
		return exitUsage
	}
	tenant, err := domain.NewTenantID(*tenantFlag)
	if err != nil {
		fmt.Fprintf(errOut, "%s 需要 -tenant <租户>\n", command)
		return exitUsage
	}
	if *file == "" {
		fmt.Fprintf(errOut, "%s 需要 -file <模板 CSV>\n", command)
		return exitUsage
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(errOut, "读模板：%v\n", err)
		return exitUsage
	}
	// 模板整体先核，再碰数据库：不合格的文件不该消耗一次连接，也不该让内勤等到连库
	// 之后才知道表头错了。
	job, err := planImport(command, tenant, raw, clock, out)
	if err != nil {
		fmt.Fprintln(errOut, err)
		return exitUsage
	}

	dsn := getenv(envDatabaseDSN)
	if dsn == "" {
		fmt.Fprintf(errOut, "%s 未设置——导入口不猜连接串\n", envDatabaseDSN)
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
	return job(ctx, db, errOut)
}

func knownCommand(command string) bool {
	for _, known := range allCommands {
		if command == known {
			return true
		}
	}
	return false
}

// importJob 是一份译装合格的模板等着连库之后做的那一步：装配该口的导入链并逐行推进。
// 译装与连库分成两步，正是为了让不合格的文件不消耗一次连接；装配失败按未决退出——
// 那是运维环境的事，改文件解决不了。
type importJob func(ctx context.Context, db *bentopg.DB, errOut io.Writer) int

// planImport 按子命令译装模板并交回连库后要做的那一步。每个子命令各自的模板与导入链
// 都在各自的文件里，这里只分派。
func planImport(command string, tenant domain.TenantID, raw []byte, clock ports.Clock, out io.Writer) (importJob, error) {
	switch command {
	case commandIntake:
		batch, err := decodeIntakeTemplate(tenant, raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, db *bentopg.DB, errOut io.Writer) int {
			importer, err := buildIntakeImporter(db, clock, unconfiguredParcelIdentityView{})
			if err != nil {
				fmt.Fprintf(errOut, "装配导入口：%v\n", err)
				return exitUndecided
			}
			return executeIntake(ctx, batch, importer, out)
		}, nil
	case commandConsolidation:
		batch, err := decodeConsolidationTemplate(tenant, raw)
		if err != nil {
			return nil, err
		}
		return func(ctx context.Context, db *bentopg.DB, errOut io.Writer) int {
			importer, err := buildConsolidationImporter(db, clock)
			if err != nil {
				fmt.Fprintf(errOut, "装配导入口：%v\n", err)
				return exitUndecided
			}
			return executeConsolidation(ctx, batch, importer, out)
		}, nil
	default:
		return nil, fmt.Errorf("未知导入命令 %q（支持 %s）", command, strings.Join(allCommands, " / "))
	}
}
