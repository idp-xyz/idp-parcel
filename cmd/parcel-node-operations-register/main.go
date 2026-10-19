// parcel-node-operations-register 是节点作业登记册的受控登记口（ADR-0085 决定一）：运营方操作员在数据库网络内手跑，不监听任何端口。
// 它与 parcel-api 上对应的登记写面调同一个编排：在线口的租户取操作者信封，本口的租户取输入文件里的 tenantId——授权依据是「能运行
// 本 CLI」这道运维边界（先例：parcel-network-register、parcel-transport-register）。
//
// 登记族今天只有一个：
//   - pre-advice-window 预报有效时窗册的一行（ADR-0212 决定二末项；参数登记册 PAR-NET-18）：提前量、宽限量（分钟）、生效区间与
//     登记依据。同租户生效区间与在册行重叠在登记时拒；原样再登答已有结果。
//
// 输入是一份登记行 JSON（-kind 指族，-file 指路径），字段与在线口的载荷一一对应、另加 tenantId；未知字段一律拒绝——打错的键静默丢弃
// 会让操作员以为登进去的比实际多。本工具不携带任何默认取值：时窗的两个时长是租户取值，缺了就拒，不当零。
//
// 本工具假设业务 schema 已由迁移作业施加，不自行迁移。环境事务由本入口给出（写口无环境事务即拒），一次调用一笔事务。
//
// 退出码按恢复动作分四格（ADR-0029 判据）：0 = 已登记 / 已有结果（重灌同一份种子不断）；1 = 用法或输入不合法（含形成门拒绝——改输入，
// 重跑同一份没有意义）；2 = 区间重叠（册上那一行不被顶替，要人看过再定登哪一段）；3 = 未决（依赖故障，登记与否未知，重跑同一命令
// 续办）。
package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nodeopshttp "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/http"
	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const (
	exitRegistered = 0
	exitUsage      = 1
	exitOverlap    = 2
	exitUndecided  = 3
)

const kindPreAdviceWindow = "pre-advice-window"

// supportedKinds 只为 `-kind` 的用法文本与未知族的错误文本服务，列成一处免得两段文本各抄一遍族名。
var supportedKinds = []string{kindPreAdviceWindow}

// systemClock 是 Clock 端口的生产实现：登记时刻由编排取时钟，不由登记行携带。
type systemClock struct{}

func (systemClock) Now() time.Time { return time.Now().UTC() }

// registrar 收本口要用到的编排与事务入口。
type registrar struct {
	handler    *application.RegisterPreAdviceWindowHandler
	transactor bentoapp.Transactor
}

func main() {
	kind := flag.String("kind", "", "登记族："+strings.Join(supportedKinds, " / "))
	file := flag.String("file", "", "登记行 JSON 路径")
	flag.Parse()

	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IDP_PARCEL_POSTGRES_DSN 未设置——登记口不猜连接串")
		os.Exit(exitUsage)
	}
	if *file == "" {
		fmt.Fprintln(os.Stderr, "-file 未指定")
		os.Exit(exitUsage)
	}
	raw, err := os.ReadFile(*file)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取登记行：%v\n", err)
		os.Exit(exitUsage)
	}

	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库：%v\n", err)
		os.Exit(exitUndecided)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造框架 DB：%v\n", err)
		os.Exit(exitUndecided)
	}
	built, err := buildRegistrar(db)
	if err != nil {
		fmt.Fprintf(os.Stderr, "装配登记口：%v\n", err)
		os.Exit(exitUndecided)
	}

	message, code := execute(ctx, *kind, raw, built)
	fmt.Println(message)
	os.Exit(code)
}

// buildRegistrar 装本口的编排，与 parcel-api 的 buildPreAdviceWindowRegistration 装的是同一个：同一本册、同一只时钟实现。
func buildRegistrar(db *bentopg.DB) (registrar, error) {
	windows, err := nopostgres.NewPreAdviceWindows(db)
	if err != nil {
		return registrar{}, fmt.Errorf("构造时窗册：%w", err)
	}
	handler, err := application.NewRegisterPreAdviceWindowHandler(application.RegisterPreAdviceWindowDeps{
		Windows: windows,
		Clock:   systemClock{},
	})
	if err != nil {
		return registrar{}, fmt.Errorf("构造时窗登记编排：%w", err)
	}
	return registrar{handler: handler, transactor: db.Transactor()}, nil
}

// windowInput 是本口 pre-advice-window 族的输入：在线口的载荷类型原样嵌入，另加 tenantId。载荷翻成命令只走在线口那一份（Command），
// 两个入口的严格性不会各自漂移。
type windowInput struct {
	TenantID string `json:"tenantId"`
	nodeopshttp.PreAdviceWindowRegistrationPayload
}

// execute 把一份登记输入推进到登记册答案：译装 → 在环境事务内交编排 → 答案译成退出码。译装失败当场拒，不进事务——用法错误与
// 「登记与否未知」是两个退出码，让它进了事务就分不开了。
func execute(ctx context.Context, kind string, raw []byte, built registrar) (string, int) {
	if kind != kindPreAdviceWindow {
		return fmt.Sprintf("未知登记族 %q（支持 %s）", kind, strings.Join(supportedKinds, " / ")), exitUsage
	}
	var input windowInput
	if err := decodeStrict(raw, &input); err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
	}
	tenant, err := domain.NewTenantID(input.TenantID)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：tenantId：%v", kind, err), exitUsage
	}
	command, err := input.PreAdviceWindowRegistrationPayload.Command(tenant)
	if err != nil {
		return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
	}

	var result application.PreAdviceWindowRegistrationResult
	err = built.transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		handled, err := built.handler.Handle(txCtx, command)
		result = handled
		return err
	})
	switch {
	case err == nil:
	case errors.Is(err, application.ErrPreAdviceWindowRejected):
		return fmt.Sprintf("%s: 被拒：%v", kind, err), exitUsage
	default:
		return fmt.Sprintf("%s: 未决：%v", kind, err), exitUndecided
	}

	window := result.Window()
	message := fmt.Sprintf("%s: %s（%s 起，提前 %d 分、宽限 %d 分）", kind, result.Outcome(),
		window.EffectiveFrom().Format(time.RFC3339), int64(window.Lead()/time.Minute), int64(window.Grace()/time.Minute))
	switch result.Outcome() {
	case application.PreAdviceWindowRegistered, application.PreAdviceWindowExisting:
		return message, exitRegistered
	case application.PreAdviceWindowOverlapping:
		return message + "——册上这一行与输入区间重叠，未登", exitOverlap
	default:
		// 编排交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %d", kind, result.Outcome()), exitUndecided
	}
}

// decodeStrict 拒未知字段与尾随内容：打错的键静默丢弃，会让操作员以为登进去的比实际多。
func decodeStrict(raw []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("一份文件只收一条登记")
	}
	return nil
}
