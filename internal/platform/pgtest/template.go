package pgtest

import (
	"context"
	"fmt"
	"hash/fnv"
	"io"
	"log/slog"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
)

// templatePrefix 是模板库名的固定前缀。回收器按它扫 pg_database，它因此也是「哪些库
// 归本包的模板机制管」的唯一判据，不能与测试库的 parcel_test_ 前缀混用。
const templatePrefix = "parcel_tpl_"

// templateLockClass 是模板库会话锁的类别号（pg_advisory_lock 双 int 形式的第一个键），
// 第二个键由模板库名散列而来。取值任意但必须稳定：换一个值等于换一把锁，回收器就再也
// 探不到旧进程握着的那把。
const templateLockClass int32 = 820_251_102

// templateBuildTimeout 只约束建模板那一次（建库 + 全套迁移）。单跑量得 300 ms 上下，
// 多包并行时多等一阵也远够；真等到这么久，说明实例本身出了问题，早报比晚报好。
const templateBuildTimeout = 2 * time.Minute

var (
	templateMu sync.Mutex
	// templateName 是本进程的模板库名，空表示尚未建成。建失败不缓存，下一个用例重试。
	templateName string
	// templateOwner 握着本进程模板库的会话锁：建模板前打开，只由进程退出关闭。它是
	// 模板库的心跳——连接在、锁在、模板库有主；进程没了连接断、锁随之释放，别的进程
	// 才可以回收。除了拿锁它什么都不做，以免任何一次失败误把心跳掐断。
	templateOwner *pgx.Conn
)

// templateDatabase 返回本进程的模板库名，第一次调用时建它。
//
// 模板库属于测试进程而不属于某个用例：每个包的测试进程只付一次迁移，之后每个用例
// 用 CREATE DATABASE … TEMPLATE 拷一份。隔离语义不变——每个用例仍是自己的物理库；
// 「证的是随产品发出的那份 SQL」也不变——模板库就是用那份迁移计划建的。
//
// 进程退出时没有钩子可删模板库：Go 的测试二进制在 m.Run 返回后直接 os.Exit，而本包
// 又不能要求每个调用方补 TestMain。于是采取「主人以锁示活，后来者回收」：建模板前先
// 拿住以模板库名为键的会话级咨询锁，连接随进程结束而断、锁随之释放；下一个进程建
// 自己的模板前扫一遍 pg_database，锁拿得到的模板库主人已死，删之。锁比 pid 可靠：
// 实例在容器里，看不见宿主机的进程表。
func templateDatabase(t *testing.T, adminDSN string) string {
	t.Helper()

	templateMu.Lock()
	defer templateMu.Unlock()

	if templateName != "" {
		return templateName
	}
	name, err := buildTemplate(t, adminDSN)
	if err != nil {
		t.Fatalf("建模板库失败：%v", err)
	}
	templateName = name
	return name
}

func buildTemplate(t *testing.T, adminDSN string) (string, error) {
	t.Helper()

	suffix, err := randomSuffix()
	if err != nil {
		return "", fmt.Errorf("生成模板库名随机后缀：%w", err)
	}
	name := fmt.Sprintf("%s%d_%s", templatePrefix, os.Getpid(), suffix)

	if err := reapOrphanTemplates(adminDSN); err != nil {
		// 回收别人的遗留是顺手的事，失败不该把本进程的用例拖红；但要出声，
		// 不然孤儿库会无声积累到没人记得来历。
		t.Logf("回收孤儿模板库失败，本进程照常建模板：%v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), templateBuildTimeout)
	defer cancel()

	// 先锁再建：任何人在 pg_database 里看到这个名字时，锁必定已在本进程手里；反过来
	// 就会在「建成」与「上锁」的空当被回收器当成孤儿删掉。用 Background 而不是用例的
	// 上下文：这条连接要活过触发建模板的那个用例。
	owner, err := pgx.Connect(ctx, adminDSN)
	if err != nil {
		return "", fmt.Errorf("为模板库 %s 打开主人连接：%w", name, err)
	}
	var locked bool
	if err := owner.QueryRow(ctx, `SELECT pg_try_advisory_lock($1, $2)`,
		templateLockClass, templateLockKey(name)).Scan(&locked); err != nil {
		_ = owner.Close(ctx)
		return "", fmt.Errorf("锁定模板库 %s：%w", name, err)
	}
	if !locked {
		_ = owner.Close(ctx)
		return "", fmt.Errorf("模板库 %s 的锁已被别的会话握着；名字撞了，重跑即换名", name)
	}

	if err := createDatabase(adminDSN, name); err != nil {
		_ = owner.Close(ctx)
		return "", fmt.Errorf("创建模板库 %s：%w", name, err)
	}
	// 建库之后、建成之前任何一步失败的库名字对、内容错（迁移半途）或谁都连得上（许可没关上），留下就是陷阱。
	// 自己收拾，不等下一个进程的回收器；主人连接一关锁就释放，即便这里删不掉，也还有回收器兜底。
	discard := func(cause error) (string, error) {
		_ = runAsAdmin(adminDSN, func(ctx context.Context, conn *pgx.Conn) error {
			return dropDatabaseOn(ctx, conn, name)
		})
		_ = owner.Close(ctx)
		return "", cause
	}
	if err := migrateTemplate(ctx, adminDSN, name); err != nil {
		return discard(err)
	}
	if err := forbidConnections(adminDSN, name); err != nil {
		return discard(fmt.Errorf("关闭模板库 %s 的连接许可：%w", name, err))
	}

	templateOwner = owner
	return name, nil
}

// forbidConnections 关掉模板库的连接许可（ALTER DATABASE … ALLOW_CONNECTIONS false，template0 同法）。
//
// 迁完断开之后模板库不该再有任何会话：CREATE DATABASE … TEMPLATE 拒绝拷贝仍被连着的源库，「被其他用户访问」
// 是克隆失败的唯一来源；而拿着 AdminDSN 的用例只要改个库名就能连上模板库写脏，此前只靠约定挡着。关掉许可后
// 连都连不上，「模板库不被任何用例写」从约定变成结构。克隆与回收都不需要连接源库（DROP 亦然），不受影响；
// 建模板期间本包自己那条迁移连接在此之前已经断开。
func forbidConnections(adminDSN, name string) error {
	return runAsAdmin(adminDSN, func(ctx context.Context, conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, `ALTER DATABASE `+quoteIdentifier(name)+` ALLOW_CONNECTIONS false`)
		return err
	})
}

// migrateTemplate 对模板库施加完整迁移计划，然后断开：CREATE DATABASE … TEMPLATE 拒绝
// 拷贝仍有会话连着的库，这条连接不关，之后每一次克隆都会失败。
func migrateTemplate(ctx context.Context, adminDSN, name string) error {
	dsn, err := withDatabase(adminDSN, name)
	if err != nil {
		return fmt.Errorf("构造模板库连接串：%w", err)
	}
	conn, err := pgx.Connect(ctx, dsn)
	if err != nil {
		return fmt.Errorf("连接模板库：%w", err)
	}
	quiet := slog.New(slog.NewTextHandler(io.Discard, nil))
	if err := migrate.Run(ctx, conn, quiet); err != nil {
		_ = conn.Close(ctx)
		return fmt.Errorf("对模板库施加迁移计划：%w", err)
	}
	if err := conn.Close(ctx); err != nil {
		return fmt.Errorf("关闭模板库迁移连接：%w", err)
	}
	return nil
}

// reapOrphanTemplates 删掉主人已不在的模板库。
//
// 主人在不在，看它的锁还握不握得住：pg_try_advisory_lock 拿得到，说明持锁的会话已经
// 断了。本进程自己的模板库也走这条判断——锁在 templateOwner 那条会话上，从管理连接
// 试锁必然拿不到——所以不必按名字排除。两个进程同时回收同一个孤儿也安全：一个拿到
// 锁去删，另一个试锁失败跳过；或者前者删完解锁，后者再拿到锁时 IF EXISTS 让删除成空操作。
func reapOrphanTemplates(adminDSN string) error {
	return runAsAdmin(adminDSN, func(ctx context.Context, conn *pgx.Conn) error {
		rows, err := conn.Query(ctx,
			`SELECT datname FROM pg_database WHERE starts_with(datname, $1)`, templatePrefix)
		if err != nil {
			return fmt.Errorf("列模板库：%w", err)
		}
		names, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			return fmt.Errorf("列模板库：%w", err)
		}

		for _, name := range names {
			key := templateLockKey(name)
			var free bool
			if err := conn.QueryRow(ctx, `SELECT pg_try_advisory_lock($1, $2)`,
				templateLockClass, key).Scan(&free); err != nil {
				return fmt.Errorf("探测模板库 %s 的主人：%w", name, err)
			}
			if !free {
				continue
			}
			dropErr := dropDatabaseOn(ctx, conn, name)
			// 试锁拿到的锁得还回去：管理连接活到进程结束，不还，这个名字在本进程眼里
			// 会一直显得「有主」。
			if _, err := conn.Exec(ctx, `SELECT pg_advisory_unlock($1, $2)`,
				templateLockClass, key); err != nil && dropErr == nil {
				dropErr = err
			}
			if dropErr != nil {
				return fmt.Errorf("回收孤儿模板库 %s：%w", name, dropErr)
			}
		}
		return nil
	})
}

// templateLockKey 把模板库名散列成咨询锁的第二个键。主人与回收器都经这一个函数取键，
// 两边才锁的是同一把。
func templateLockKey(name string) int32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(name))
	return int32(h.Sum32())
}
