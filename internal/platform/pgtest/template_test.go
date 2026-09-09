package pgtest

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// 判据来自票面「完成判据」：模板库只建一次 / 每用例库独立 / 退出进程的模板库由后来者回收 / 模板库不接受连接而克隆仍成。
// 全部要真库，无 DSN 时经 AdminDSN 走与 Pool 同一条跳过／失败分界。
//
// 这里查 pg_database 一律另开一条管理连接，不借 runAsAdmin：被测的管理面与测它的
// 观察面共用一把锁、一条连接，坏在管理面上的东西就会同时坏掉观察它的那只眼。

func TestTemplateIsBuiltOncePerProcessAndClonesCarryTheShippedPlan(t *testing.T) {
	adminDSN := AdminDSN(t)
	admin := connectAdmin(t, adminDSN)

	first := Pool(t)
	second := Pool(t)

	if a, b := currentDatabase(t, first), currentDatabase(t, second); a == b {
		t.Fatalf("两次 Pool 落在同一个库 %s，隔离语义被模板库改掉了", a)
	}

	// 名字带进程号：并行跑的两个包各自有模板，谁也不会拿到别人那份。
	ownPrefix := fmt.Sprintf("%s%d_", templatePrefix, os.Getpid())
	if !strings.HasPrefix(templateName, ownPrefix) {
		t.Fatalf("模板库名 %q 不以 %q 开头", templateName, ownPrefix)
	}
	owned := databasesWithPrefix(t, admin, ownPrefix)
	if len(owned) != 1 || owned[0] != templateName {
		t.Fatalf("本进程的模板库应恰好一个且为 %s，实际 %v", templateName, owned)
	}

	// 克隆出来的库携带完整迁移计划——ID 与规范校验和逐条对得上工件，才算「证的是随产品
	// 发出的那份 SQL」没有因为多了一层模板而变味。
	plan, err := migrate.Plan()
	if err != nil {
		t.Fatalf("读迁移计划：%v", err)
	}
	applied := appliedMigrations(t, second)
	if len(applied) != len(plan) {
		t.Fatalf("克隆库记录了 %d 条已施加迁移，计划有 %d 步", len(applied), len(plan))
	}
	for _, step := range plan {
		if checksum, ok := applied[step.ID]; !ok {
			t.Errorf("克隆库缺迁移 %s", step.ID)
		} else if checksum != step.Checksum {
			t.Errorf("克隆库里 %s 的校验和 %s 与工件 %s 不一致", step.ID, checksum, step.Checksum)
		}
	}
}

func TestEachPoolIsAnIndependentCopyOfTheTemplate(t *testing.T) {
	ctx := t.Context()
	first := Pool(t)
	if _, err := first.Exec(ctx,
		`CREATE TABLE probe_written_after_clone (id integer PRIMARY KEY)`); err != nil {
		t.Fatalf("在第一个用例库建探针表：%v", err)
	}
	if _, err := first.Exec(ctx, `INSERT INTO probe_written_after_clone VALUES (1)`); err != nil {
		t.Fatalf("向探针表写入：%v", err)
	}

	// 第二个用例库在第一个写过之后才克隆：它看不见那张表，既证两库彼此独立，也证
	// 用例对自己库的写入没有漏回模板。
	second := Pool(t)
	var visible bool
	if err := second.QueryRow(ctx,
		`SELECT to_regclass('public.probe_written_after_clone') IS NOT NULL`).Scan(&visible); err != nil {
		t.Fatalf("在第二个用例库查探针表：%v", err)
	}
	if visible {
		t.Fatalf("第一个用例库写下的表在第二个用例库可见：两库不独立，或模板被用例写脏")
	}
}

func TestTemplateRefusesConnectionsWhileClonesStillSucceed(t *testing.T) {
	adminDSN := AdminDSN(t)
	first := Pool(t)

	// 模板库建成即关连接许可：拿着 AdminDSN 的用例就算改个库名也连不上，写脏模板从「别这么做」变成「做不到」。
	// PostgreSQL 对不接受连接的库答 55000（object_not_in_prerequisite_state），钉状态码不钉文案。
	templateDSN, err := withDatabase(adminDSN, templateName)
	if err != nil {
		t.Fatalf("构造模板库连接串：%v", err)
	}
	conn, err := pgx.Connect(t.Context(), templateDSN)
	if err == nil {
		_ = conn.Close(context.Background())
		t.Fatalf("模板库 %s 接受了连接，连接许可没有关上", templateName)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != "55000" {
		t.Fatalf("连模板库应被 PostgreSQL 以 55000 拒绝，实际：%v", err)
	}

	// 关掉许可不妨碍克隆：CREATE DATABASE … TEMPLATE 不需要连接源库（template0 同法）。
	second := Pool(t)
	if a, b := currentDatabase(t, first), currentDatabase(t, second); a == b {
		t.Fatalf("两次 Pool 落在同一个库 %s", a)
	}
}

// helperOwnerEnv 设了才让 TestHelperTemplateOwnerProcess 真跑；它只作为子进程被驱动。
const helperOwnerEnv = "IDP_PARCEL_PGTEST_HELPER_TEMPLATE_OWNER"

func TestTemplateOfAnExitedProcessIsReapedByTheNext(t *testing.T) {
	adminDSN := AdminDSN(t)
	admin := connectAdmin(t, adminDSN)

	// 本进程先有一个活着的模板库，好证回收器只删主人已退出的，不删主人还在的。
	_ = Pool(t)
	live := templateName

	orphan := runTemplateOwnerHelper(t)
	t.Logf("子进程退出后其模板库 %s 仍在：%v（本包没有退出钩，靠后来者回收）",
		orphan, databaseExists(t, admin, orphan))

	// 子进程退出到 PostgreSQL 察觉其会话断开之间有毫秒级空当，锁未释放时回收器会
	// 正确地把它当成有主而跳过——所以轮询，不断言第一次就删掉。
	deadline := time.Now().Add(10 * time.Second)
	for {
		if err := reapOrphanTemplates(adminDSN); err != nil {
			t.Fatalf("回收孤儿模板库：%v", err)
		}
		if !databaseExists(t, admin, orphan) {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("子进程退出 10 秒后其模板库 %s 仍未被回收", orphan)
		}
		time.Sleep(200 * time.Millisecond)
	}
	if !databaseExists(t, admin, live) {
		t.Fatalf("回收器删掉了主人仍在的模板库 %s", live)
	}
}

// TestHelperTemplateOwnerProcess 是被 TestTemplateOfAnExitedProcessIsReapedByTheNext 以
// 子进程方式驱动的主人：建好本进程的模板库、报出名字、正常退出，留下一个主人已死的
// 模板库给父进程回收。直接跑本包时它跳过，且跳过理由写明它不是一条独立判据。
func TestHelperTemplateOwnerProcess(t *testing.T) {
	if os.Getenv(helperOwnerEnv) == "" {
		t.Skip("只由 TestTemplateOfAnExitedProcessIsReapedByTheNext 以子进程驱动")
	}
	_ = Pool(t)
	fmt.Fprintf(os.Stdout, "template=%s\n", templateName)
}

func runTemplateOwnerHelper(t *testing.T) string {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 2*time.Minute)
	defer cancel()
	cmd := exec.CommandContext(ctx, os.Args[0],
		"-test.run=^TestHelperTemplateOwnerProcess$", "-test.count=1")
	// 子进程不打点：量的是被测包的夹具费，子进程那一行会混进同一份 TSV。
	cmd.Env = append(os.Environ(), helperOwnerEnv+"=1", TimingVariable+"=")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("子进程建模板库失败：%v\n%s", err, out)
	}
	for _, line := range strings.Split(string(out), "\n") {
		if name, ok := strings.CutPrefix(strings.TrimSpace(line), "template="); ok && name != "" {
			return name
		}
	}
	t.Fatalf("子进程输出里没有模板库名：\n%s", out)
	return ""
}

func connectAdmin(t *testing.T, adminDSN string) *pgx.Conn {
	t.Helper()

	conn, err := pgx.Connect(t.Context(), adminDSN)
	if err != nil {
		t.Fatalf("打开观察用管理连接：%v", err)
	}
	t.Cleanup(func() { _ = conn.Close(context.Background()) })
	return conn
}

func currentDatabase(t *testing.T, pool *pgxpool.Pool) string {
	t.Helper()

	var name string
	if err := pool.QueryRow(t.Context(), `SELECT current_database()`).Scan(&name); err != nil {
		t.Fatalf("读当前库名：%v", err)
	}
	return name
}

func databaseExists(t *testing.T, admin *pgx.Conn, name string) bool {
	t.Helper()

	var exists bool
	if err := admin.QueryRow(t.Context(),
		`SELECT EXISTS (SELECT 1 FROM pg_database WHERE datname = $1)`, name).Scan(&exists); err != nil {
		t.Fatalf("查库 %s 是否存在：%v", name, err)
	}
	return exists
}

func databasesWithPrefix(t *testing.T, admin *pgx.Conn, prefix string) []string {
	t.Helper()

	rows, err := admin.Query(t.Context(),
		`SELECT datname FROM pg_database WHERE starts_with(datname, $1) ORDER BY datname`, prefix)
	if err != nil {
		t.Fatalf("按前缀 %s 列库：%v", prefix, err)
	}
	names, err := pgx.CollectRows(rows, pgx.RowTo[string])
	if err != nil {
		t.Fatalf("按前缀 %s 列库：%v", prefix, err)
	}
	return names
}

func appliedMigrations(t *testing.T, pool *pgxpool.Pool) map[string]string {
	t.Helper()

	rows, err := pool.Query(t.Context(),
		`SELECT migration_id, canonical_checksum FROM `+migrate.SchemaHistory+`.applied_migration`)
	if err != nil {
		t.Fatalf("读克隆库的迁移历史：%v", err)
	}
	defer rows.Close()

	applied := make(map[string]string)
	for rows.Next() {
		var id, checksum string
		if err := rows.Scan(&id, &checksum); err != nil {
			t.Fatalf("扫描迁移历史：%v", err)
		}
		applied[id] = checksum
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("读克隆库的迁移历史：%v", err)
	}
	return applied
}
