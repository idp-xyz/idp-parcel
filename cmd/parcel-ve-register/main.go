// parcel-ve-register 是 VE 五类规则与策略目录（syn-wall-door-audit 票 15，即票 09
// 的 B 半边）与索赔材料归集面（票 ve-claims-read-seams/02）的受控登记口：运营方操作
// 员在数据库网络内手跑，不监听任何端口。目录登记是租户运营方的治理动作，收讫登记是
// 租户作业面的操作动作，信任边界都是运维边界而不是商业渠道，故走受控 CLI 而不进
// parcel-api 的端点面（形状循 parcel-pricing-register / parcel-governance-register；
// 票 12 已裁治理类登记不受 ADR-0055 业务端点章程管）。它也不碰任何装配接线——
// `tenantBoundCustomerViewDerive` 那格哨兵等的是目录内容（实例半边），不是代码接线。
//
// 目录册的登记种类（`PAR-VIS-08` 跨索赔资格与申请人授权两组表）：
// milestone-mapping（`PAR-VIS-01`）、triage-rules（`PAR-VIS-05`）、
// notification-policy（`PAR-VIS-07` 的渠道与时限半边）、claim-eligibility /
// claim-authorization（`PAR-VIS-08`）、disclosure-policy（`PAR-VIS-09`）、
// exception-disclosure-rules（`PAR-VIS-07` 的披露与自动发布范围半边，0023）、
// conflict-signal-rule（`PAR-VIS-04`，0025；票 ve-disclosure-policy-view/02 接入后两册）。
// 后两册的命令名对齐读口 /visibility-catalogues?kind= 的原词 EXCEPTION_DISCLOSURE_RULE /
// CONFLICT_SIGNAL_RULE，按 DISCLOSURE_POLICY ↔ disclosure-policy 的既有变形；异常披露规则
// 与披露策略是相邻的两本册，命令名里「规则」「策略」两词就是分册的记号。此外材料归集面
// 两命令 claim-material-receipt / claim-material-receipt-revocation（票
// ve-claims-read-seams/02）登的是收讫事实不是规则册——材料实物经租户的客服/作业面
// 收到后由操作者在此登记收讫，客户自助提交材料属 PAR-INT-01 之后的渠道工作，不在
// 本入口。
//
// 执行者身份走双轨（票 12 裁决，经票 15 沿用）：①通道技术身份（OS 进程属主、主机名）
// 由本入口自取，没有任何参数能传入或覆盖它，与登记同笔事务落
// visibility_exception.channel_execution 留痕；②发布批准责任（approvedBy）是登记
// 内容，由输入显式必填提供，册面语义是「登记者声明了谁批准」。授权依据是「能运行
// 本 CLI」这道运维边界加①的留痕，②只是被记录的声明。
//
// 目录内容全部来自输入文件，进程不内置任何生产默认；真实映射、规则、策略与授权名单
// 属实例半边（`PAR-VIS-01`/`05`/`07`/`08`/`09` 待提供），留待租户登记。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。
//
// 退出码：0 = 已登记；1 = 用法或输入不合法（含用例指名的缺件拒绝）；2 = 登记册治理
// 答案（目录册：版本不可覆盖 / 同一时点已有另一适用版本——原行未被顶替，人工核对后
// 换版本号或改区间续办；归集面：无从撤销——五件指名的收讫行不在册）；3 = 未决（依赖
// 故障，登记与否未知，重跑同一命令续办）。幂等重放格只归集面有（同五件重登答
// ALREADY_* 且走 0——行身份就是事实本身，没有内容可被顶替）；目录册没有：登记册不
// 比对内容，同版本号再登一律答版本不可覆盖。
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

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	vepg "go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/adapters/registrationjson"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/application"
	"go.idp.xyz/idp-parcel/internal/visibilityexception/ports"
)

const envDatabaseDSN = "IDP_PARCEL_POSTGRES_DSN"

const (
	exitRegistered = 0
	exitUsage      = 1
	exitGovernance = 2
	exitUndecided  = 3
)

const (
	commandMilestoneMapping          = "milestone-mapping"
	commandTriageRules               = "triage-rules"
	commandNotificationPolicy        = "notification-policy"
	commandClaimEligibility          = "claim-eligibility"
	commandClaimAuthorization        = "claim-authorization"
	commandDisclosurePolicy          = "disclosure-policy"
	commandExceptionDisclosureRules  = "exception-disclosure-rules"
	commandConflictSignalRule        = "conflict-signal-rule"
	commandMaterialReceipt           = "claim-material-receipt"
	commandMaterialReceiptRevocation = "claim-material-receipt-revocation"
)

// registerCommands 是本入口开的全部登记种类，用法提示与路由共用一份，不各列一遍。
// 目录册各命令在前，材料归集面的两条事实登记（票 ve-claims-read-seams/02）在末——两族
// 的答案代数不同（见 executeMaterialReceipt），路由在 execute 分岔。
var registerCommands = []string{
	commandMilestoneMapping,
	commandTriageRules,
	commandNotificationPolicy,
	commandClaimEligibility,
	commandClaimAuthorization,
	commandDisclosurePolicy,
	commandExceptionDisclosureRules,
	commandConflictSignalRule,
	commandMaterialReceipt,
	commandMaterialReceiptRevocation,
}

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
	Append(ctx context.Context, execution vepg.ChannelExecution) error
}

var _ executionTracer = (*vepg.ChannelExecutions)(nil)

type registrars struct {
	catalogs   *application.CatalogRegistration
	receipts   *application.MaterialReceiptRegistration
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
	usage := func() {
		fmt.Fprintf(errOut, "用法：parcel-ve-register <%s> -input <file>\n",
			joinCommands("|"))
	}
	if len(args) < 1 {
		usage()
		return exitUsage
	}
	command := args[0]
	if !knownCommand(command) {
		fmt.Fprintf(errOut, "未知登记种类 %q（本入口开 %s）\n", command, joinCommands("、"))
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

func knownCommand(command string) bool {
	for _, candidate := range registerCommands {
		if candidate == command {
			return true
		}
	}
	return false
}

func joinCommands(separator string) string {
	joined := ""
	for index, command := range registerCommands {
		if index > 0 {
			joined += separator
		}
		joined += command
	}
	return joined
}

// buildRegistrars 装配真实登记链：目录写入方与归集面写入方 + 各自用例 + 留痕库。
func buildRegistrars(db *bentopg.DB) (registrars, error) {
	none := registrars{}
	registrar, err := vepg.NewCatalogRegistrar(db)
	if err != nil {
		return none, fmt.Errorf("构造目录写入方：%w", err)
	}
	catalogs, err := application.NewCatalogRegistration(registrar)
	if err != nil {
		return none, fmt.Errorf("构造登记用例：%w", err)
	}
	receiptRegistrar, err := vepg.NewMaterialReceiptRegistrar(db)
	if err != nil {
		return none, fmt.Errorf("构造归集面写入方：%w", err)
	}
	receipts, err := application.NewMaterialReceiptRegistration(receiptRegistrar)
	if err != nil {
		return none, fmt.Errorf("构造收讫登记用例：%w", err)
	}
	tracer, err := vepg.NewChannelExecutions(db)
	if err != nil {
		return none, fmt.Errorf("构造留痕库：%w", err)
	}
	return registrars{
		catalogs:   catalogs,
		receipts:   receipts,
		tracer:     tracer,
		transactor: db.Transactor(),
		clock:      systemClock{},
	}, nil
}

// registration 是翻译产物：一次登记的执行闭包加留痕引用。各目录命令的类型差异收在
// 翻译处，事务与留痕编排只写一遍。
type registration struct {
	// reference 指名这次执行送达的登记：区间型目录用租户+版本，键型目录用租户+键身份。
	reference string
	perform   func(context.Context, *application.CatalogRegistration) (application.RegisterCatalogResult, error)
}

func translateCommand(command string, raw []byte) (registration, error) {
	none := registration{}
	switch command {
	case commandMilestoneMapping:
		cmd, err := registrationjson.MilestoneMappingFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			reference: cmd.TenantID.String() + "/" + cmd.Header.Version,
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterMilestoneMapping(ctx, cmd)
			},
		}, nil
	case commandTriageRules:
		cmd, err := registrationjson.TriageRulesFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			reference: cmd.TenantID.String() + "/" + cmd.Header.Version,
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterTriageRules(ctx, cmd)
			},
		}, nil
	case commandNotificationPolicy:
		cmd, err := registrationjson.NotificationPolicyFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			reference: cmd.TenantID.String() + "/" + cmd.Policy.String(),
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterNotificationPolicy(ctx, cmd)
			},
		}, nil
	case commandClaimEligibility:
		cmd, err := registrationjson.ClaimEligibilityFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			reference: cmd.TenantID.String() + "/" + cmd.Contract.String(),
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterClaimEligibility(ctx, cmd)
			},
		}, nil
	case commandClaimAuthorization:
		cmd, err := registrationjson.ClaimAuthorizationFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			reference: cmd.TenantID.String() + "/" + cmd.Customer.String(),
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterClaimAuthorization(ctx, cmd)
			},
		}, nil
	case commandDisclosurePolicy:
		cmd, err := registrationjson.DisclosurePolicyFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			reference: cmd.TenantID.String() + "/" + cmd.Header.Version,
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterDisclosurePolicy(ctx, cmd)
			},
		}, nil
	case commandExceptionDisclosureRules:
		cmd, err := registrationjson.ExceptionDisclosureRulesFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			reference: cmd.TenantID.String() + "/" + cmd.Header.Version,
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterExceptionDisclosureRules(ctx, cmd)
			},
		}, nil
	case commandConflictSignalRule:
		cmd, err := registrationjson.ConflictSignalRuleFromJSON(raw)
		if err != nil {
			return none, err
		}
		return registration{
			// 一租户一条（0025），键只有租户；痕上再带识别规则版本，是因为换版是一次治理
			// 动作、旧行不被顶替——痕要答得出这一次登的是哪一版，光记租户答不出。
			reference: cmd.TenantID.String() + "/" + cmd.Rule.String(),
			perform: func(ctx context.Context, catalogs *application.CatalogRegistration) (application.RegisterCatalogResult, error) {
				return catalogs.RegisterConflictSignalRule(ctx, cmd)
			},
		}, nil
	default:
		return none, fmt.Errorf("未知登记种类 %q", command)
	}
}

// execute 把一份登记输入推进到登记册答案：翻译 → 同一笔事务内交用例并给已登记的
// 执行留痕 → 答案译成退出码。留痕失败随事务翻成未决——登记不许在无痕状态下落地，
// 痕也不声称一笔没落库的登记。拒绝不留痕：无论缺件拒绝还是登记册治理答案，都没有
// 任何行到达登记册。
//
// 材料归集两命令在此分岔（executeMaterialReceipt）：骨架同款，答案代数不同——那一族
// 有幂等重放格，目录册没有。
func execute(
	ctx context.Context,
	command string,
	raw []byte,
	identity channelIdentity,
	regs registrars,
) (string, int) {
	if command == commandMaterialReceipt || command == commandMaterialReceiptRevocation {
		return executeMaterialReceipt(ctx, command, raw, identity, regs)
	}
	reg, err := translateCommand(command, raw)
	if err != nil {
		return fmt.Sprintf("%s: 输入被拒：%v", command, err), exitUsage
	}

	var result application.RegisterCatalogResult
	err = regs.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := reg.perform(txCtx, regs.catalogs)
		if err != nil {
			return err
		}
		result = handled
		if result.Outcome() != application.CatalogRegistered {
			return nil
		}
		return regs.tracer.Append(txCtx, vepg.ChannelExecution{
			Command:         command,
			RecordReference: reg.reference,
			OSUser:          identity.osUser,
			Hostname:        identity.hostname,
			Outcome:         result.Outcome().String(),
			ExecutedAt:      regs.clock.Now(),
		})
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
	}
	return catalogAnswer(command, result)
}

// receiptExecution 是材料归集两命令的翻译产物，形状随 registration：执行闭包加留痕
// 引用（五件行身份连成一串——归集面没有版本号，行身份就是这笔登记的名字）。
type receiptExecution struct {
	reference string
	perform   func(context.Context, *application.MaterialReceiptRegistration) (application.MaterialReceiptResult, error)
}

func translateReceiptCommand(command string, raw []byte) (receiptExecution, error) {
	none := receiptExecution{}
	switch command {
	case commandMaterialReceipt:
		cmd, err := registrationjson.MaterialReceiptFromJSON(raw)
		if err != nil {
			return none, err
		}
		return receiptExecution{
			reference: receiptReference(cmd.TenantID, cmd.Batch, cmd.Item, cmd.Material, cmd.ReceivedAt),
			perform: func(ctx context.Context, receipts *application.MaterialReceiptRegistration) (application.MaterialReceiptResult, error) {
				return receipts.RegisterReceipt(ctx, cmd)
			},
		}, nil
	case commandMaterialReceiptRevocation:
		cmd, err := registrationjson.MaterialReceiptRevocationFromJSON(raw)
		if err != nil {
			return none, err
		}
		return receiptExecution{
			reference: receiptReference(cmd.TenantID, cmd.Batch, cmd.Item, cmd.Material, cmd.ReceivedAt),
			perform: func(ctx context.Context, receipts *application.MaterialReceiptRegistration) (application.MaterialReceiptResult, error) {
				return receipts.RevokeReceipt(ctx, cmd)
			},
		}, nil
	default:
		return none, fmt.Errorf("未知登记种类 %q", command)
	}
}

// receiptReference 把五件行身份连成留痕引用。收讫时刻带满精度（RFC3339Nano）：它是
// 行身份的一件，截掉精度会让两笔不同收讫在痕里同名。
func receiptReference(
	tenant fmt.Stringer,
	batch fmt.Stringer,
	item fmt.Stringer,
	material fmt.Stringer,
	receivedAt time.Time,
) string {
	return tenant.String() + "/" + batch.String() + "/" + item.String() + "/" +
		material.String() + "@" + receivedAt.UTC().Format(time.RFC3339Nano)
}

// executeMaterialReceipt 与 execute 同骨架：翻译 → 同一笔事务内交用例并给真正落库的
// 执行留痕 → 答案译成退出码。留痕只随 REGISTERED 与 REVOKED 两格——幂等重放没有行
// 到达登记册，与拒绝同样不留痕（留痕表证「这笔登记经受控通道执行且落了库」，不证
// 「有人跑过命令」）。
func executeMaterialReceipt(
	ctx context.Context,
	command string,
	raw []byte,
	identity channelIdentity,
	regs registrars,
) (string, int) {
	reg, err := translateReceiptCommand(command, raw)
	if err != nil {
		return fmt.Sprintf("%s: 输入被拒：%v", command, err), exitUsage
	}

	var result application.MaterialReceiptResult
	err = regs.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := reg.perform(txCtx, regs.receipts)
		if err != nil {
			return err
		}
		result = handled
		outcome := result.Outcome()
		if outcome != application.MaterialReceiptRegistered && outcome != application.MaterialReceiptRevoked {
			return nil
		}
		return regs.tracer.Append(txCtx, vepg.ChannelExecution{
			Command:         command,
			RecordReference: reg.reference,
			OSUser:          identity.osUser,
			Hostname:        identity.hostname,
			Outcome:         outcome.String(),
			ExecutedAt:      regs.clock.Now(),
		})
	})
	if err != nil {
		return fmt.Sprintf("%s: 未决：%v", command, err), exitUndecided
	}
	return receiptAnswer(command, result)
}

// receiptAnswer 把归集面用例答案译成退出码。幂等重放（ALREADY_*）走 0：重跑同一命令
// 是未决路的续办动作，答案已指名本次没有写入，不能让续办被读成失败。「无从撤销」是
// 登记册的治理答案——五件指名的收讫行不在册，人工核对引用后再来（2）；其余拒绝要
// 登记方改输入（1）。
func receiptAnswer(command string, result application.MaterialReceiptResult) (string, int) {
	switch result.Outcome() {
	case application.MaterialReceiptRegistered,
		application.MaterialReceiptReplayed,
		application.MaterialReceiptRevoked,
		application.MaterialReceiptRevocationReplayed:
		return command + ": " + result.Outcome().String(), exitRegistered
	case application.MaterialReceiptRefused:
		message := command + ": " + result.Outcome().String() + " " + result.RefusalReason().String()
		if result.RefusalReason() == application.ReceiptNotFound {
			return message, exitGovernance
		}
		return message, exitUsage
	default:
		return fmt.Sprintf("%s: 未知用例结果 %d", command, result.Outcome()), exitUndecided
	}
}

// catalogAnswer 把用例答案译成退出码。拒绝按恢复动作分两路：缺件与矛盾要登记方改
// 输入（1）；版本不可覆盖与区间重叠是登记册的治理答案——原行未被顶替，人工核对后
// 换版本号或改区间再来（2），不能混进 1 里被当成打错字。
func catalogAnswer(command string, result application.RegisterCatalogResult) (string, int) {
	switch result.Outcome() {
	case application.CatalogRegistered:
		return command + ": " + result.Outcome().String(), exitRegistered
	case application.CatalogRegistrationRefused:
		message := command + ": " + result.Outcome().String() + " " + result.RefusalReason().String()
		switch result.RefusalReason() {
		case application.CatalogVersionNotOverwritable, application.CatalogVersionOverlaps:
			return message, exitGovernance
		default:
			return message, exitUsage
		}
	default:
		return fmt.Sprintf("%s: 未知用例结果 %d", command, result.Outcome()), exitUndecided
	}
}
