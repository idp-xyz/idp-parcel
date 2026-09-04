package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
	tfapp "go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	tfdomain "go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
)

// Covers: 规则登记口的第二参是真编排（票 label-channel/19）——规则目录在真实 PostgreSQL 上装得起来，
// 事务边界成立（重放走已有版本，证首笔真的提交了）；换版经真装配落新版本并回指前版。测试输入是隔离
// 合成，只记 `S`，不进生产装配；两版取值都是显式登记的结果，不是默认值。
func TestTheWiredEffectiveTimeRuleRegistrarAnswersHonestlyAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	registrar, err := buildEffectiveTimeRuleRegistration(db)
	if err != nil {
		t.Fatalf("装配规则登记编排：%v", err)
	}

	first := tfapp.RegisterEffectiveTimeRuleCommand{
		TenantID:          mustValue(t, tfdomain.NewTenantID, "SYN-TENANT-1"),
		Source:            "SYN-CARRIER-X",
		Version:           "SYN-ETR-000000000001",
		SourceTimeMeaning: "EVENT_OCCURRENCE",
		Anchor:            "OCCURRED_AT",
		Offset:            0,
	}
	registered, err := registrar.Register(t.Context(), first)
	if err != nil {
		t.Fatalf("首登：%v", err)
	}
	if got := registered.Outcome(); got != tfapp.EffectiveTimeRuleRegistered {
		t.Fatalf("outcome = %v, want RULE_REGISTERED", got)
	}

	replay, err := registrar.Register(t.Context(), first)
	if err != nil {
		t.Fatalf("重放：%v", err)
	}
	if got := replay.Outcome(); got != tfapp.EffectiveTimeRuleExistingVersion {
		t.Fatalf("outcome = %v, want EXISTING_VERSION——重放没走已有版本，首笔事务没有提交", got)
	}

	revised, err := registrar.Register(t.Context(), tfapp.RegisterEffectiveTimeRuleCommand{
		TenantID:          first.TenantID,
		Source:            first.Source,
		Version:           "SYN-ETR-000000000002",
		SourceTimeMeaning: "SOURCE_PROCESSING",
		Anchor:            "RECEIVED_AT",
		Offset:            -2 * time.Hour,
	})
	if err != nil {
		t.Fatalf("换版：%v", err)
	}
	if got := revised.Outcome(); got != tfapp.EffectiveTimeRuleRevised {
		t.Fatalf("outcome = %v, want RULE_REVISED", got)
	}
	record, has := revised.Record()
	if !has {
		t.Fatal("换版成功却没带回记录")
	}
	if prior, present := record.Rule.Supersedes(); !present || prior.String() != first.Version {
		t.Fatalf("supersedes = (%q, %v)，新版本没有回指首版", prior.String(), present)
	}
}
