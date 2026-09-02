package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	commercialapp "go.idp.xyz/idp-parcel/internal/partycommercial/application"
	commercialdomain "go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: 商业八类写面的第二参是真编排——`buildCommercialRegistrationOrchestration`
// 在真实 PostgreSQL 上装得起来，且身份族的事务壳**确实提交**。
//
// 只走身份族一条链：三族共用同一个泛型事务壳 `commercialInTransaction`，它提交与否
// 与结果类型无关；而三族各自接到哪个用例由编译期锁住（各 Registrar 契约的方法签名与
// application 的用例逐一对上，见 register_*.go 里那三行 `var _`）。这里要证的是编译器
// 看不见的那一半——写入有没有留在库里。
//
// 取「首登 → 同修订同内容重放」这一对：重放答`已在册`要先读回首行才答得出来，因此
// 重放这一格本身就是首登事务已提交的证据。测试输入是隔离合成，只记 `S`。
func TestTheWiredCommercialRegistrationsRecordAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}

	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}

	command := syntheticBusinessPartyRegistration(t)
	registered, err := registration.partyIdentity.RegisterBusinessParty(t.Context(), command)
	if err != nil {
		t.Fatalf("参与方首登：%v", err)
	}
	if registered.Outcome() != commercialapp.PartyIdentityRegistered {
		t.Fatalf("参与方首登 outcome = %s（原因 %v），想要 REGISTERED",
			registered.Outcome(), registered.Cause())
	}

	replayed, err := registration.partyIdentity.RegisterBusinessParty(t.Context(), command)
	if err != nil {
		t.Fatalf("参与方重放：%v", err)
	}
	if replayed.Outcome() != commercialapp.PartyIdentityAlreadyRegistered {
		t.Fatalf("参与方重放 outcome = %s（原因 %v），想要 ALREADY_REGISTERED"+
			"——读不到首行说明首登事务没提交", replayed.Outcome(), replayed.Cause())
	}
}

// syntheticBusinessPartyRegistration 造一份立得住的最小合成参与方身份登记：首笔修订，
// 生效起点取一个绝对时刻。身份全取 SYN-API-PC 前缀避免与其它测试相撞。
func syntheticBusinessPartyRegistration(t *testing.T) commercialapp.RegisterBusinessPartyCommand {
	t.Helper()
	tenant, err := commercialdomain.NewTenantID("SYN-TENANT-API-PC")
	if err != nil {
		t.Fatalf("构造租户：%v", err)
	}
	partyID, err := commercialdomain.NewPartyID("SYN-API-PC-PARTY-1")
	if err != nil {
		t.Fatalf("构造参与方标识：%v", err)
	}
	name, err := commercialdomain.NewPartyName("SYN 合成参与方一号")
	if err != nil {
		t.Fatalf("构造参与方名称：%v", err)
	}
	party, err := commercialdomain.NewBusinessParty(tenant, partyID, name)
	if err != nil {
		t.Fatalf("构造业务参与方：%v", err)
	}
	basis, err := commercialdomain.NewIdentityBasisReference("SYN-API-PC-BASIS-1")
	if err != nil {
		t.Fatalf("构造身份依据引用：%v", err)
	}
	return commercialapp.RegisterBusinessPartyCommand{
		Party:         party,
		Revision:      1,
		Basis:         basis,
		EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}
