// parcel-document-numbering-register 是单据编号规则册的受控登记口（ADR-0085 决定一；ADR-0355 决定六）：运营方操作员在
// 数据库网络内手跑，不监听任何端口。它与 parcel-api 上对应的登记口调同一个编排：在线口的租户与登记人取操作者信封，本口
// 的租户取输入文件里的 tenantId、登记人记为本口——授权依据是「能运行本 CLI」这道运维边界（先例：
// parcel-node-operations-register）。
//
// 登记族：
//   - time-zone 租户的单据编号时区一版（ADR-0355 决定七）：IANA 时区名与生效时刻。租户取值、无默认。
//   - rule 一类单据的一版租户规则（ADR-0355 决定五）：类型代码、模板与生效时刻。用加扰流水的模板要该租户在实例配置里
//     配了加扰密钥（docnumber.ScrambleSecretsFileEnv），本口与 parcel-api 读同一项。
//
// 输入是一份登记行 JSON（-kind 指族，-file 指路径），字段与在线口的载荷一一对应、另加 tenantId；未知字段一律拒绝。原样
// 再登答已有结果。
//
// 本工具假设 schema 已由迁移作业施加，不自行迁移。环境事务由本入口给出（写口无环境事务即拒），一次调用一笔事务。
//
// 退出码按恢复动作分四格（ADR-0029 判据）：0 = 已登记 / 已有结果（重灌同一份种子不断）；1 = 用法或输入不合法（含登记
// 被拒——改输入，重跑同一份没有意义）；2 = 同一生效时刻已登了另一版（版本只增不改，要人另选生效时刻）；3 = 未决（依赖
// 故障，登记与否未知，重跑同一命令续办）。
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
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/cmd/internal/docnumbering"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberhttp"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

const (
	exitRegistered = 0
	exitUsage      = 1
	exitTaken      = 2
	exitUndecided  = 3
)

const (
	kindTimeZone = "time-zone"
	kindRule     = "rule"
)

// registeredBy 是本口记在册上的登记人：受控登记口不认识操作者，记下的是授权所凭的那道运维边界。
const registeredBy = "CONTROLLED-CLI/parcel-document-numbering-register"

var supportedKinds = []string{kindTimeZone, kindRule}

// registrar 是本口转交的登记：与 parcel-api 的登记口是同一个包进事务的编排。
type registrar = docnumbering.Registrations

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
	secrets, err := docnumbering.LoadScrambleSecrets(os.Getenv, os.ReadFile, nil)
	if err != nil {
		fmt.Fprintf(os.Stderr, "读取加扰密钥：%v\n", err)
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
	built, err := buildRegistrar(db, secrets)
	if err != nil {
		fmt.Fprintf(os.Stderr, "装配登记口：%v\n", err)
		os.Exit(exitUndecided)
	}

	message, code := execute(ctx, *kind, raw, built)
	fmt.Println(message)
	os.Exit(code)
}

// buildRegistrar 装本口的编排，与 parcel-api 装的是同一个（docnumbering.Build）：同一份类型汇总、同一本册、同一项
// 加扰密钥配置。
func buildRegistrar(db *bentopg.DB, secrets docnumber.ScrambleSecrets) (registrar, error) {
	numbers, err := docnumbering.Build(db, secrets)
	if err != nil {
		return registrar{}, fmt.Errorf("装配编号组件：%w", err)
	}
	return numbers.Registrations, nil
}

// timeZoneInput 与 ruleInput 是本口两族的输入：在线口的载荷类型原样嵌入，另加 tenantId。载荷翻成命令只走在线口那一份
// （Command），两个入口的严格性不会各自漂移。
type timeZoneInput struct {
	TenantID string `json:"tenantId"`
	docnumberhttp.TimeZoneRegistrationPayload
}

type ruleInput struct {
	TenantID string `json:"tenantId"`
	docnumberhttp.RuleRegistrationPayload
}

// execute 把一份登记输入推进到登记册答案：译装 → 在环境事务内交编排 → 答案译成退出码。译装失败当场拒，不进事务。
func execute(ctx context.Context, kind string, raw []byte, built registrar) (string, int) {
	switch kind {
	case kindTimeZone:
		var input timeZoneInput
		if err := decodeStrict(raw, &input); err != nil {
			return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
		}
		command, err := input.TimeZoneRegistrationPayload.Command(input.TenantID, registeredBy)
		if err != nil {
			return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
		}
		result, err := built.RegisterTimeZone(ctx, command)
		if message, code, done := failure(kind, err); done {
			return message, code
		}
		return answer(kind, result.Outcome, fmt.Sprintf("%s，%s 起", result.Version.TimeZone, result.Version.EffectiveFrom.Format(time.RFC3339)))
	case kindRule:
		var input ruleInput
		if err := decodeStrict(raw, &input); err != nil {
			return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
		}
		command, err := input.RuleRegistrationPayload.Command(input.TenantID, registeredBy)
		if err != nil {
			return fmt.Sprintf("%s: 译装被拒：%v", kind, err), exitUsage
		}
		result, err := built.RegisterRule(ctx, command)
		if message, code, done := failure(kind, err); done {
			return message, code
		}
		return answer(kind, result.Outcome, fmt.Sprintf("%s %s，%s 起", result.Version.Type, result.Version.Template, result.Version.EffectiveFrom.Format(time.RFC3339)))
	default:
		return fmt.Sprintf("未知登记族 %q（支持 %s）", kind, strings.Join(supportedKinds, " / ")), exitUsage
	}
}

func failure(kind string, err error) (string, int, bool) {
	switch {
	case err == nil:
		return "", 0, false
	case errors.Is(err, docnumber.ErrRegistrationRejected):
		return fmt.Sprintf("%s: 被拒：%v", kind, err), exitUsage, true
	default:
		return fmt.Sprintf("%s: 未决：%v", kind, err), exitUndecided, true
	}
}

func answer(kind string, outcome docnumber.RegistrationOutcome, version string) (string, int) {
	message := fmt.Sprintf("%s: %s（%s）", kind, outcome, version)
	switch outcome {
	case docnumber.VersionRegistered, docnumber.VersionExisting:
		return message, exitRegistered
	case docnumber.EffectiveFromTaken:
		return message + "——同一生效时刻已登了另一版，未登", exitTaken
	default:
		// 编排交回一个它自己都不认识的格是实现坏了，不是业务答案。
		return fmt.Sprintf("%s: 未知应用结果 %q", kind, outcome), exitUndecided
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
