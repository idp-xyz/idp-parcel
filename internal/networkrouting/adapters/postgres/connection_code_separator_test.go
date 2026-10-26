package postgres_test

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"

	"go.idp.xyz/idp-parcel/internal/networkrouting/domain"
	"go.idp.xyz/idp-parcel/internal/networkrouting/ports"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	"go.idp.xyz/idp-parcel/migrations"
)

const connectionCodeSeparatorConstraint = "network_connection_version_code_without_basis_separator"

// Covers: AT-NR-094 的库面半边（票 demo-chain-gaps/176）——迁移 0028 给连接代码加 CHECK，挡窗口依据的分隔符 `+`。
// 库里已有带 `+` 的行就拒绝施加、在报文里列出那几行、行原样不动；演示种子登的连接代码与带 `@`、`/` 的代码不挡，
// 移走带 `+` 的行之后照常施加。施加之后，带 `+` 的行撞的是这条约束。
// 从空库按序施加本上下文的迁移，停在 0028 之前落存量行，再施加 0028。
func TestMigration0028RefusesToApplyOverConnectionCodesCarryingTheSeparator(t *testing.T) {
	ctx := t.Context()
	conn, err := pgx.Connect(ctx, pgtest.FreshDatabase(t))
	if err != nil {
		t.Fatalf("连空库：%v", err)
	}
	defer conn.Close(context.WithoutCancel(ctx))
	if _, err := conn.Exec(ctx, `CREATE SCHEMA network_routing`); err != nil {
		t.Fatalf("建 schema：%v", err)
	}
	assets, err := migrations.NetworkRouting()
	if err != nil {
		t.Fatalf("读迁移：%v", err)
	}
	const target = "0028_connection_code_without_window_basis_separator"
	var guard *migrations.Asset
	for index := range assets {
		if assets[index].Name == target {
			guard = &assets[index]
			break
		}
		if _, err := conn.Exec(ctx, assets[index].SQL); err != nil {
			t.Fatalf("施加 %s：%v", assets[index].Name, err)
		}
	}
	if guard == nil {
		t.Fatalf("没有找到迁移 %s", target)
	}

	insert := func(tenant, code string) error {
		_, err := conn.Exec(ctx, `INSERT INTO network_routing.network_connection_version
			(tenant_id, connection_code, version, from_node_code, to_node_code, business_timezone, effective_from)
			VALUES ($1, $2, 1, 'SYN-NODE-A', 'SYN-NODE-B', 'Asia/Shanghai', '2026-01-01T00:00:00Z')`, tenant, code)
		return err
	}
	kept := append(demoSeedConnectionCodes(t), "SYN@CONN/B-C", "CONNECTION/SYN-B-C")
	for _, code := range kept {
		if err := insert("SYN-TENANT-NR28", code); err != nil {
			t.Fatalf("落 0028 之前的连接 %q：%v", code, err)
		}
	}
	if err := insert("SYN-TENANT-NR28", "SYN-CONN-A+B"); err != nil {
		t.Fatalf("落 0028 之前带 + 的连接：%v", err)
	}

	_, err = conn.Exec(ctx, guard.SQL)
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || !strings.Contains(pgErr.Message, "SYN-TENANT-NR28/SYN-CONN-A+B@1") {
		t.Fatalf("库里有带 + 的连接代码，0028 应拒绝施加并列出那一行，实得：%v", err)
	}
	for _, code := range kept {
		if strings.Contains(pgErr.Message, code+"@") {
			t.Fatalf("报文列出了不带 + 的连接 %q：%s", code, pgErr.Message)
		}
	}
	var rows int
	if err := conn.QueryRow(ctx, `SELECT count(*) FROM network_routing.network_connection_version`).Scan(&rows); err != nil ||
		rows != len(kept)+1 {
		t.Fatalf("拒绝施加之后行数 = %d（%v），want %d 原样不动", rows, err, len(kept)+1)
	}

	if _, err := conn.Exec(ctx, `DELETE FROM network_routing.network_connection_version
		WHERE connection_code = 'SYN-CONN-A+B'`); err != nil {
		t.Fatalf("移走带 + 的行：%v", err)
	}
	if _, err := conn.Exec(ctx, guard.SQL); err != nil {
		t.Fatalf("演示种子的连接代码与带 @、/ 的代码之上施加 %s：%v", target, err)
	}

	err = insert("SYN-TENANT-NR28", "SYN-CONN-B+C")
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != connectionCodeSeparatorConstraint {
		t.Fatalf("0028 之后库面放行了带 + 的连接代码：%v", err)
	}
}

// Covers: AT-NR-094 的库面半边——绕过受理门直接交给目录写入口，带 `+` 的连接代码撞 0028 的 CHECK，一行不落；写入口
// 的错误原样上抛，登记用例那一侧把它当未决（第二道网，不是答案）。
func TestTheCatalogTableRefusesAConnectionCodeCarryingTheSeparator(t *testing.T) {
	catalog, transactor, pool := newNetworkCatalog(t)
	ctx := t.Context()
	err := transactor.WithinTransaction(ctx, func(txCtx context.Context) error {
		return catalog.RegisterConnectionVersion(txCtx, scalar(t, domain.NewTenantID, "tenant-separator"),
			ports.ConnectionDefinitionVersion{
				Code: "conn-a+b", Version: 1, FromNode: "node-a", ToNode: "node-b",
				BusinessTimezone: "Asia/Shanghai", EffectiveFrom: catalogAsOf,
			})
	})
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.ConstraintName != connectionCodeSeparatorConstraint {
		t.Fatalf("带 + 的连接代码没有撞上 %s：%v", connectionCodeSeparatorConstraint, err)
	}
	var rows int
	if err := pool.QueryRow(ctx, `SELECT count(*) FROM network_routing.network_connection_version
		WHERE tenant_id = 'tenant-separator'`).Scan(&rows); err != nil || rows != 0 {
		t.Fatalf("撞了约束还落了 %d 行（%v）", rows, err)
	}
}

// demoSeedConnectionCodes 读演示种子登的连接代码：从各步脚本里认出 `-kind connection` 登的那几份文件，再读各自的代码。
// 读的是 scripts/demo-seeds 里的文件本身：种子改了、加了连接，这里跟着读到，「0028 在演示种子上施加不失败」才不是一句
// 写死了的话。
func demoSeedConnectionCodes(t *testing.T) []string {
	t.Helper()
	seeds := filepath.Join("..", "..", "..", "..", "scripts", "demo-seeds")
	steps, err := filepath.Glob(filepath.Join(seeds, "steps", "*.sh"))
	if err != nil || len(steps) == 0 {
		t.Fatalf("没读到演示种子的步脚本：%v（%d 份）", err, len(steps))
	}
	registered := regexp.MustCompile(`-kind connection -file "\$SEEDS/([^"]+)"`)
	var codes []string
	for _, step := range steps {
		script, err := os.ReadFile(step)
		if err != nil {
			t.Fatalf("读 %s：%v", step, err)
		}
		for _, match := range registered.FindAllStringSubmatch(string(script), -1) {
			raw, err := os.ReadFile(filepath.Join(seeds, "data", match[1]))
			if err != nil {
				t.Fatalf("读 %s：%v", match[1], err)
			}
			var row struct {
				Code string `json:"code"`
			}
			if err := json.Unmarshal(raw, &row); err != nil || row.Code == "" {
				t.Fatalf("%s 没有连接代码：%v", match[1], err)
			}
			codes = append(codes, row.Code)
		}
	}
	if len(codes) == 0 {
		t.Fatal("演示种子的步脚本里一份连接登记都没认出来")
	}
	return codes
}
