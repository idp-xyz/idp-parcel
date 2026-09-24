package main

import (
	"context"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
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

// Covers: ADR-0126 Decision 三 — 运营操作者面发布路径的第二参是真编排：录入 → 批准 → 发布三笔各自提交，发布那一笔
// 把载体推进与受控发布同事务落库——发布后按同一版本重录答`内容已固定`（读得到载体行）、重发布答`已发布`、受控
// 批文口对同一版本答`重复`（读得到版本行）。审批职责规则由写口直接登一条（今天没有治理写面，ADR-0126 Decision 五）。
// 测试输入是隔离合成，只记 `S`。
func TestTheWiredPublicationDraftPathLandsAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	ctx := t.Context()
	tenant := pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-API-PC08")

	rules, err := pcpostgres.NewApprovalDutyRules(db)
	if err != nil {
		t.Fatalf("规则册：%v", err)
	}
	rule, err := commercialdomain.NewApprovalDutyRule(tenant, true, commercialdomain.AuthorityLevel{})
	if err != nil {
		t.Fatalf("规则：%v", err)
	}
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		_, err := rules.SaveApprovalDutyRule(txCtx, rule)
		return err
	}); err != nil {
		t.Fatalf("登记审批职责规则：%v", err)
	}

	interval, err := commercialdomain.NewEffectiveInterval(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), time.Time{})
	if err != nil {
		t.Fatalf("区间：%v", err)
	}
	limit, err := commercialdomain.NewCreditAmountLimit(500_000)
	if err != nil {
		t.Fatalf("额度：%v", err)
	}
	shell := commercialdomain.PublicationDraftShell{
		TenantID:  tenant,
		Kind:      commercialdomain.CreditPolicyObject,
		ObjectID:  pcSynthetic(t, commercialdomain.NewCommercialObjectID, "SYN-API-PC08-CREDIT-1"),
		Version:   pcSynthetic(t, commercialdomain.NewCommercialVersionLabel, "v1"),
		Scope:     pcSynthetic(t, commercialdomain.NewCommercialScopeReference, "SYN-API-PC08-SCOPE"),
		Effective: interval,
	}
	content := commercialdomain.PublicationContent{Kind: commercialdomain.CreditPolicyObject, CreditPolicy: &commercialdomain.CreditPolicyBody{
		LegalEntity: pcSynthetic(t, commercialdomain.NewLegalEntityReference, "SYN-API-PC08-LEGAL"),
		Level:       pcSynthetic(t, commercialdomain.NewAuthorityLevel, "SYN-LEVEL-COMMERCIAL"),
		ChargeType:  pcSynthetic(t, commercialdomain.NewChargeTypeReference, "SYN-CHARGE-FREIGHT"),
		Limit:       limit,
		Effective:   interval,
	}}

	submitted, err := registration.publicationDrafts.Submit(ctx, commercialapp.SubmitPublicationDraftCommand{
		Shell: shell, Content: content, Submitter: pcSynthetic(t, commercialdomain.NewOperatorSubjectReference, "SYN-OP-SUBMITTER"),
	})
	if err != nil || submitted.Outcome() != commercialapp.PublicationDraftSubmitted {
		t.Fatalf("录入：outcome = %s, err = %v", submitted.Outcome(), err)
	}

	approver, err := commercialdomain.NewOperatorSubject(pcSynthetic(t, commercialdomain.NewOperatorSubjectReference, "SYN-OP-APPROVER"), nil)
	if err != nil {
		t.Fatalf("批准者：%v", err)
	}
	approved, err := registration.publicationDrafts.Approve(ctx, commercialapp.ApprovePublicationDraftCommand{
		Tenant: tenant, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version, Approver: approver,
	})
	if err != nil || approved.Outcome() != commercialapp.PublicationDraftApproved {
		t.Fatalf("批准：outcome = %s, err = %v——读不到载体行说明录入事务没提交", approved.Outcome(), err)
	}

	published, err := registration.publicationDrafts.Publish(ctx, commercialapp.PublishPublicationDraftCommand{
		Tenant: tenant, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
	})
	if err != nil || published.Outcome() != commercialapp.PublicationDraftPublished {
		t.Fatalf("发布：outcome = %s, err = %v", published.Outcome(), err)
	}
	publication, _ := published.Publication()
	if publication.Outcome() != commercialapp.CommercialVersionPublishedEffective {
		t.Fatalf("受控发布 outcome = %s, want PUBLISHED_EFFECTIVE", publication.Outcome())
	}

	fixed, err := registration.publicationDrafts.Submit(ctx, commercialapp.SubmitPublicationDraftCommand{
		Shell: shell, Content: content, Submitter: pcSynthetic(t, commercialdomain.NewOperatorSubjectReference, "SYN-OP-SUBMITTER"),
	})
	if err != nil || fixed.Outcome() != commercialapp.PublicationDraftReplayed {
		t.Fatalf("发布后同一次录入：outcome = %s, err = %v", fixed.Outcome(), err)
	}
	again, err := registration.publicationDrafts.Publish(ctx, commercialapp.PublishPublicationDraftCommand{
		Tenant: tenant, Kind: shell.Kind, ObjectID: shell.ObjectID, Version: shell.Version,
	})
	if err != nil || again.Outcome() != commercialapp.PublicationDraftAlreadyPublished {
		t.Fatalf("重发布：outcome = %s, err = %v——读不到已发布状态说明发布事务没提交", again.Outcome(), err)
	}
	version, _ := publication.Version()
	basis, _ := version.ApprovalBasis()
	replayed, err := registration.publication.Handle(ctx, commercialapp.PublishCommercialAuthorityCommand{
		Spec: submittedSpec(submitted), Approval: basis, RoleStanding: commercialdomain.ApprovalRoleConfirmed,
	})
	if err != nil || replayed.Outcome() != commercialapp.CommercialPublicationReplayed {
		t.Fatalf("受控批文口同版本：outcome = %s, err = %v——版本行不在说明发布事务没提交", replayed.Outcome(), err)
	}
}

// Covers: 票 legal-entity-profile/03 资料登记口的第二参是真编排——资料登记用例接到了法人身份读口与注册号类型目录。
// 两者都是构造参数，传错编译器看不见：带税务登记号的资料修订答`已登记`，要地址国家对着法人身份判过、税号对着
// 目录判过；同修订同内容重放答`已在册`，即首登事务已提交。法人与目录先经同一编排登好。测试输入是隔离合成，只记 `S`。
func TestTheWiredLegalEntityProfileRegistrationLandsAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	registration, err := buildCommercialRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配商业登记编排：%v", err)
	}
	ctx := t.Context()
	tenant := pcSynthetic(t, commercialdomain.NewTenantID, "SYN-TENANT-API-LEP03")
	country := pcSynthetic(t, commercialdomain.NewRegistrationCountryCode, "XA")
	typesFrom := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	entityFrom := time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC)

	for _, numberType := range []struct {
		code, format string
		layer        commercialdomain.RegistrationNumberLayer
	}{
		{"SYN-XA-LIFETIME", `SYN-XA-[0-9]{6}`, commercialdomain.RegistrationNumberIdentityLayer},
		{"SYN-XA-TAX", `SYN-XA-TAX-[0-9]{4}`, commercialdomain.RegistrationNumberProfileLayer},
	} {
		result, err := registration.registrationNumberType.Register(ctx, commercialapp.RegisterRegistrationNumberTypeCommand{
			Tenant:   tenant,
			Country:  country,
			Code:     pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeCode, numberType.code),
			Revision: 1,
			Spec: commercialdomain.RegistrationNumberTypeSpec{
				Name:   pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeName, "SYN 合成类型 "+numberType.code),
				Layer:  numberType.layer,
				Format: pcSynthetic(t, commercialdomain.NewRegistrationNumberFormat, numberType.format),
				Basis:  pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeBasisReference, "SYN-BASIS/"+numberType.code),
			},
			EffectiveFrom: typesFrom,
		})
		if err != nil || result.Outcome() != commercialapp.RegistrationNumberTypeRegistered {
			t.Fatalf("注册号类型 %s 首登：outcome = %s，err = %v", numberType.code, result.Outcome(), err)
		}
	}

	party, err := commercialdomain.NewBusinessParty(tenant,
		pcSynthetic(t, commercialdomain.NewPartyID, "SYN-API-LEP03-PARTY"),
		pcSynthetic(t, commercialdomain.NewPartyName, "SYN 合成参与方（票 legal-entity-profile/03）"))
	if err != nil {
		t.Fatalf("构造业务参与方：%v", err)
	}
	if result, err := registration.partyIdentity.RegisterBusinessParty(ctx, commercialapp.RegisterBusinessPartyCommand{
		Party:         party,
		Revision:      1,
		Basis:         pcSynthetic(t, commercialdomain.NewIdentityBasisReference, "SYN-BASIS/party-lep03"),
		EffectiveFrom: typesFrom,
	}); err != nil || result.Outcome() != commercialapp.PartyIdentityRegistered {
		t.Fatalf("参与方首登：outcome = %s（原因 %v），err = %v", result.Outcome(), result.Cause(), err)
	}
	lifetime, err := commercialdomain.NewLifetimeRegistrationNumber(
		pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeCode, "SYN-XA-LIFETIME"),
		pcSynthetic(t, commercialdomain.NewRegistrationNumber, "SYN-XA-000003"))
	if err != nil {
		t.Fatalf("构造终身注册号：%v", err)
	}
	entity := pcSynthetic(t, commercialdomain.NewLegalEntityReference, "SYN-API-LEP03-LE")
	if result, err := registration.partyIdentity.RegisterLegalEntity(ctx, commercialapp.RegisterLegalEntityCommand{
		Tenant:              tenant,
		Entity:              entity,
		Party:               party.ID(),
		Revision:            1,
		Basis:               pcSynthetic(t, commercialdomain.NewIdentityBasisReference, "SYN-BASIS/le-lep03"),
		EffectiveFrom:       entityFrom,
		RegistrationCountry: &country,
		LifetimeNumbers:     []commercialdomain.LifetimeRegistrationNumber{lifetime},
	}); err != nil || result.Outcome() != commercialapp.PartyIdentityRegistered {
		t.Fatalf("法人首登：outcome = %s（原因 %v），err = %v", result.Outcome(), result.Cause(), err)
	}

	address, err := commercialdomain.NewRegisteredAddress(country, []string{"SYN 合成路 3 号"})
	if err != nil {
		t.Fatalf("构造注册地址：%v", err)
	}
	tax, err := commercialdomain.NewTaxRegistrationNumber(
		pcSynthetic(t, commercialdomain.NewRegistrationNumberTypeCode, "SYN-XA-TAX"),
		pcSynthetic(t, commercialdomain.NewRegistrationNumber, "SYN-XA-TAX-0003"))
	if err != nil {
		t.Fatalf("构造税务登记号：%v", err)
	}
	invoicing, err := commercialdomain.NewInvoicingDetails(pcSynthetic(t, commercialdomain.NewInvoiceTitle, "SYN 合成抬头"))
	if err != nil {
		t.Fatalf("构造开票资料：%v", err)
	}
	command := commercialapp.RegisterLegalEntityProfileCommand{
		Tenant:        tenant,
		Entity:        entity,
		Revision:      1,
		Basis:         pcSynthetic(t, commercialdomain.NewLegalEntityProfileBasisReference, "SYN-BASIS/profile-lep03"),
		EffectiveFrom: entityFrom,
		Address:       address,
		TaxNumbers:    []commercialdomain.TaxRegistrationNumber{tax},
		Invoicing:     &invoicing,
	}
	landed, err := registration.legalEntityProfile.Register(ctx, command)
	if err != nil || landed.Outcome() != commercialapp.LegalEntityProfileRegistered {
		t.Fatalf("资料首登：outcome = %s（原因 %v），err = %v", landed.Outcome(), landed.Cause(), err)
	}
	replayed, err := registration.legalEntityProfile.Register(ctx, command)
	if err != nil || replayed.Outcome() != commercialapp.LegalEntityProfileAlreadyRegistered {
		t.Fatalf("资料重放：outcome = %s（原因 %v），err = %v——读不到首行说明首登事务没提交",
			replayed.Outcome(), replayed.Cause(), err)
	}
}

func submittedSpec(result commercialapp.SubmitPublicationDraftResult) commercialdomain.CommercialVersionSpec {
	draft, _ := result.Draft()
	return draft.PublicationSpec()
}

func pcSynthetic[T any](t *testing.T, construct func(string) (T, error), raw string) T {
	t.Helper()
	value, err := construct(raw)
	if err != nil {
		t.Fatalf("构造 %q：%v", raw, err)
	}
	return value
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
