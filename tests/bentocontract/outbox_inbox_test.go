package bentocontract

import (
	"testing"
	"time"

	"go.idp.xyz/idp-bento-go/eventing"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/inbox"
	"go.idp.xyz/idp-bento-go/postgres/outbox"
	"go.idp.xyz/idp-bento-go/testkit"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件取 `PBC-06` 第三项：Outbox 与 Inbox 合同在真实 PostgreSQL 16 上通过。
//
// 运行器由框架提供，本仓只负责用**真实**的 Store 与 Transactor 把 scenario 装起来。
// 框架合同明禁用内存 Store 或假 Transactor 的通过结果顶替——那样测的是替身而不是
// 适配器，而 Outbox 要证的恰好是「业务写入与入队同事务提交或回滚」这类只有真实
// 引擎才会暴露的性质。
//
// testkit 与 pgtest 都只出现在 `_test.go` 里：前者受 `PBC-08` 的生产依赖图约束，
// 后者会把 `testing` 拉进任何引它的二进制，两条都由 `internal/architecture` 的
// 门禁看着。需要它们的代码放进测试文件，而不是为它们开豁免。

func TestOutboxContract(t *testing.T) {
	testkit.RunOutboxContract(t, func(t *testing.T) testkit.OutboxContractScenario {
		db := frameworkDB(t)

		store, err := outbox.NewStore(db)
		if err != nil {
			t.Fatalf("构造 Outbox Store：%v", err)
		}

		first := testkit.ValidEnvelope()
		samePartition := testkit.ValidEnvelope()
		samePartition.ID = "event-0002"
		otherPartition := testkit.ValidEnvelope()
		otherPartition.ID = "event-0003"
		otherPartition.Subject = "other-subject"
		otherPartition.PartitionKey = "test-scope/other-subject"

		return testkit.OutboxContractScenario{
			Transactor:     db.Transactor(),
			Writer:         store,
			Claimer:        store,
			Finalizer:      store,
			First:          first,
			SamePartition:  samePartition,
			OtherPartition: otherPartition,
			Now:            time.Now().UTC(),
			LeaseFor:       time.Minute,
		}
	})
}

func TestInboxContract(t *testing.T) {
	testkit.RunInboxContract(t, func(t *testing.T) testkit.InboxContractScenario {
		db := frameworkDB(t)

		store, err := inbox.NewStore(db)
		if err != nil {
			t.Fatalf("构造 Inbox Store：%v", err)
		}

		return testkit.InboxContractScenario{
			Transactor: db.Transactor(),
			Starter:    store,
			Completer:  store,
			Key: eventing.InboxKey{
				Consumer: "parcel-contract",
				Source:   "testkit",
				EventID:  "event-0001",
			},
			Now: time.Now().UTC(),
		}
	})
}

// frameworkDB 为一个子测试准备独立数据库，并把框架技术表所在的 schema 交给 DB。
// 每个子测试各拿一份，是框架合同的要求：共用一个库会让先跑的用例留下的行影响
// 后跑的用例，而 Outbox 合同大量依赖「队首是哪一条」。
func frameworkDB(t *testing.T) *bentopg.DB {
	t.Helper()

	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	return db
}
