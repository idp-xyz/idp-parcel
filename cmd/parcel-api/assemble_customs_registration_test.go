package main

import (
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	ccpostgres "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/postgres"
	ccregistrationjson "go.idp.xyz/idp-parcel/internal/customscompliance/adapters/registrationjson"
	customsapp "go.idp.xyz/idp-parcel/internal/customscompliance/application"
	customsdomain "go.idp.xyz/idp-parcel/internal/customscompliance/domain"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: `/customs-regulatory-credential-registrations`、`/customs-duty-collaboration-registrations`
// 与 `/customs-duty-payment-verification-registrations` 的第二参是真编排——buildCustomsRegistrationOrchestration
// 在真实 PostgreSQL 上装得起来（含 sa-cc/05 的 Handoff 口：构造门对每一口一视同仁，装配少一口在这里
// 就红，不等第一份核对到达），且新加的两个事务壳**确实提交**（票 sa-cc/07 步二，完成判据 2）。
//
// 接没接对不在本文件证：三个 Registrar 契约的命令类型互不相同、协作与核对又按用例方法名分成两个
// 契约，接错编译期就红。这里证的是编译器看不见的那一半，一正一反：
//   - 正：凭证首登 REGISTERED、同内容重放 EXISTING；协作事项形成 COLLABORATION_FORMED、重放
//     COLLABORATION_EXISTING——重放要读回首行才答得出，重放即提交证据；凭证册列面上也读得到那一版。
//   - 反：同身份换有效期答 CONTENT_CONFLICT 且册面仍是首版（同键异内容绝不覆盖）；核对在资金事实
//     未接收时答 FUNDS_FACT_NOT_RECEIVED、核对册无行——前置门在装配后的编排上照样把门，事务壳没有
//     替它落半行。
//
// 核对的形成格不在本文件：它要先有一条经 SA 采用信封进来的资金事实（ADR-0137 Decision 四），那条链
// 归 sa-cc/03 的消费者与 05 的 handoff 真库用例。测试输入是隔离合成，只记 `S`，不进生产装配。
func TestTheWiredCredentialAndDutyRegistrationsRecordAgainstARealDatabase(t *testing.T) {
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	ctx := t.Context()
	registration, err := buildCustomsRegistrationOrchestration(db)
	if err != nil {
		t.Fatalf("装配关务登记编排：%v", err)
	}
	tenant := mustValue(t, customsdomain.NewTenantID, "SYN-TENANT-API-CC07")

	t.Run("凭证：首登提交、重放已在册、换有效期是冲突且册面不动", func(t *testing.T) {
		first, err := ccregistrationjson.RegulatoryCredentialFromJSON([]byte(
			`{"tenantId":"SYN-TENANT-API-CC07","credentialId":"SYN-API-CRED-1",` +
				`"issuerRef":"SYN-API-AUTHORITY-1","holderRef":"SYN-API-HOLDER-1","procedureRef":"SYN-API-PROC-1",` +
				`"validFrom":"2026-01-01T00:00:00Z","validTo":"2027-01-01T00:00:00Z","uses":3}`))
		if err != nil {
			t.Fatalf("译装凭证登记：%v", err)
		}
		registered, err := registration.regulatoryCredential.Handle(ctx, first)
		if err != nil {
			t.Fatalf("凭证首登：%v", err)
		}
		if registered != customsapp.ConfigurationRegistered {
			t.Fatalf("凭证首登 outcome = %s，想要 REGISTERED", registered)
		}
		replayed, err := registration.regulatoryCredential.Handle(ctx, first)
		if err != nil {
			t.Fatalf("凭证重放：%v", err)
		}
		if replayed != customsapp.ConfigurationExisting {
			t.Fatalf("凭证重放 outcome = %s，想要 EXISTING——读不到首行说明首登事务没提交", replayed)
		}

		changed := first
		changed.ValidTo = time.Date(2028, 1, 1, 0, 0, 0, 0, time.UTC)
		conflicted, err := registration.regulatoryCredential.Handle(ctx, changed)
		if err != nil {
			t.Fatalf("凭证换有效期：%v", err)
		}
		if conflicted != customsapp.ConfigurationContentConflict {
			t.Fatalf("凭证换有效期 outcome = %s，想要 CONTENT_CONFLICT", conflicted)
		}

		catalogue, err := ccpostgres.NewCredentialCatalogue(db)
		if err != nil {
			t.Fatalf("构造凭证册读面：%v", err)
		}
		rows, err := catalogue.ListCredentials(ctx, tenant, 50)
		if err != nil {
			t.Fatalf("读回凭证册：%v", err)
		}
		if len(rows) != 1 {
			t.Fatalf("凭证册上 %d 行，想要 1——首登要在、冲突那版不得落册", len(rows))
		}
		if rows[0].Credential.ID() != first.ID || !rows[0].Credential.ValidTo().Equal(first.ValidTo) {
			t.Fatalf("凭证册上的是 %s / 止于 %s，想要首版 %s / 止于 %s——冲突覆盖了首版",
				rows[0].Credential.ID(), rows[0].Credential.ValidTo().Format(time.RFC3339),
				first.ID, first.ValidTo.Format(time.RFC3339))
		}
	})

	t.Run("协作事项：形成提交、重放已存在", func(t *testing.T) {
		command, err := ccregistrationjson.DutyCollaborationFromJSON([]byte(
			`{"tenantId":"SYN-TENANT-API-CC07","kind":"ASSESSED_DUTY","dutyRef":"SYN-API-DUTY-1/v1",` +
				`"scopeRef":"SYN-API-UNIT-1","obligorRef":"SYN-API-OBLIGOR-1",` +
				`"requirementRef":"SYN-API-ASSESSMENT-1","targetRef":"SYN-API-DUTY-DESK"}`))
		if err != nil {
			t.Fatalf("译装协作事项形成：%v", err)
		}
		formed, err := registration.dutyCollaboration.FormCollaboration(ctx, command)
		if err != nil {
			t.Fatalf("协作事项形成：%v", err)
		}
		if formed.Outcome() != customsapp.CollaborationFormed {
			t.Fatalf("协作事项形成 outcome = %s（reason %s），想要 COLLABORATION_FORMED",
				formed.Outcome(), formed.UndecidedReason())
		}
		replayed, err := registration.dutyCollaboration.FormCollaboration(ctx, command)
		if err != nil {
			t.Fatalf("协作事项重放：%v", err)
		}
		if replayed.Outcome() != customsapp.CollaborationExisting {
			t.Fatalf("协作事项重放 outcome = %s，想要 COLLABORATION_EXISTING——读不到首行说明形成事务没提交",
				replayed.Outcome())
		}
	})

	t.Run("核对：资金事实未接收答前置未齐、核对册无行", func(t *testing.T) {
		command, err := ccregistrationjson.DutyPaymentVerificationFromJSON([]byte(
			`{"tenantId":"SYN-TENANT-API-CC07","dutyRef":"SYN-API-DUTY-1/v1","fundsRef":"SYN-API-FUNDS-NEVER","fundsVersion":"SYN-API-FUNDS-NEVER/v1",` +
				`"scopeRef":"SYN-API-UNIT-1","procedureRef":"SYN-API-PROC-1","coverage":"COVERED","delta":"NO_DELTA","validity":"VALID",` +
				`"basis":"SYN-API-RULE-1: remittance quotes assessment"}`))
		if err != nil {
			t.Fatalf("译装付款核对：%v", err)
		}
		result, err := registration.dutyPaymentVerification.VerifyPayment(ctx, command)
		if err != nil {
			t.Fatalf("付款核对：%v", err)
		}
		if result.Outcome() != customsapp.FundsFactNotReceived {
			t.Fatalf("付款核对 outcome = %s（reason %s），想要 FUNDS_FACT_NOT_RECEIVED",
				result.Outcome(), result.UndecidedReason())
		}

		catalogue, err := ccpostgres.NewDutyReconciliationCatalogue(db)
		if err != nil {
			t.Fatalf("构造协作与核对读面：%v", err)
		}
		rows, err := catalogue.ListDutyVerifications(ctx, tenant, 50)
		if err != nil {
			t.Fatalf("读回核对册：%v", err)
		}
		if len(rows) != 0 {
			t.Fatalf("核对册上 %d 行，想要 0——前置未齐不得落核对", len(rows))
		}
	})
}
