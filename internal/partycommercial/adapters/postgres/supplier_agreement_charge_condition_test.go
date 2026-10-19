package postgres_test

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
	bentoapp "go.idp.xyz/idp-bento-go/application"
	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/partycommercial/domain"
	"go.idp.xyz/idp-parcel/internal/partycommercial/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// 本文件对真实 PostgreSQL 16 证供应商协议的收费发生条件一节（票 demo-chain-gaps/29，ADR-0209；迁移 0038）：条件随正文
// 同笔落、LoadSupplierAgreement 与 LoadSupplierAgreementVersion 都原样读回；没声明读成没声明；同内容重放答已登记、条件不同答内容冲突；库面守住原因与数量形的
// 封闭集与「计费必带数量形」。夹具全是合成串，证据只记 S。

// newChargeConditionFixture 与 newSupplierAgreementContents 同形，多交出底层连接池：库面 CHECK 要绕过适配器直写才证得到。
func newChargeConditionFixture(t *testing.T) (*adapter.CommercialPublications, bentoapp.Transactor, *pgxpool.Pool) {
	t.Helper()
	pool := pgtest.Pool(t)
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	repository, err := adapter.NewCommercialPublications(db)
	if err != nil {
		t.Fatalf("构造发布登记册：%v", err)
	}
	return repository, db.Transactor(), pool
}

func chargeConditionsOf(t *testing.T, fulfillment domain.ChargeQuantityForm) domain.ChargeOccurrenceConditions {
	t.Helper()
	charged, err := domain.NewChargedOccurrenceCondition(domain.ActualFulfillmentChargeReason, fulfillment)
	if err != nil {
		t.Fatal(err)
	}
	uncharged, err := domain.NewUnchargedOccurrenceCondition(domain.FailedAttemptChargeReason)
	if err != nil {
		t.Fatal(err)
	}
	conditions, err := domain.NewChargeOccurrenceConditions(charged, uncharged)
	if err != nil {
		t.Fatal(err)
	}
	return conditions
}

func withConditions(t *testing.T, agreement domain.SupplierAgreement, conditions domain.ChargeOccurrenceConditions) domain.SupplierAgreement {
	t.Helper()
	declared, err := agreement.WithChargeConditions(conditions)
	if err != nil {
		t.Fatalf("带条件：%v", err)
	}
	return declared
}

// Covers: AT-PC-053——条件随正文同笔落，按版本点读与按已选版本读都原样读回；同一对象另一版不带条件，读成没声明。
func TestChargeConditionsRoundTripWithTheAgreementBody(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	v1 := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v1", "digest-a1")
	v2 := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v2", "digest-a2")
	mustSaveVersion(t, transactor, ctx, repository, v1)
	mustSaveVersion(t, transactor, ctx, repository, v2)
	mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, v1, "supplier-1", "plan-buy-1"))
	declared := chargeConditionsOf(t, domain.PerSegmentQuantity)
	mustSaveSupplierAgreement(t, transactor, ctx, repository,
		withConditions(t, supplierAgreementOn(t, v2, "supplier-1", "plan-buy-1"), declared))

	read, err := loadVersion(t, contents, "tenant-1", "agreement-1", "v2")
	if err != nil || !read.HasAgreement {
		t.Fatalf("点读 v2：%v / 正文 %v", err, read.HasAgreement)
	}
	conditions, present := read.Agreement.ChargeConditions()
	if !present || !conditions.Equal(declared) {
		t.Fatalf("点读读回的条件 = %+v（在场 %v），want %+v", conditions.All(), present, declared.All())
	}
	selected, found, err := contents.LoadSupplierAgreement(ctx, pcTenant(t, "tenant-1"), v2)
	if err != nil || !found {
		t.Fatalf("按已选版本读 v2：found=%v err=%v", found, err)
	}
	if conditions, present := selected.ChargeConditions(); !present || !conditions.Equal(declared) {
		t.Fatal("按已选版本读丢了条件")
	}

	plain, err := loadVersion(t, contents, "tenant-1", "agreement-1", "v1")
	if err != nil || !plain.HasAgreement {
		t.Fatalf("点读 v1：%v", err)
	}
	if _, present := plain.Agreement.ChargeConditions(); present {
		t.Fatal("没声明条件的 v1 读出了条件")
	}
}

// Covers: AT-PC-053——同内容（含条件）重放答已登记；条件换了数量形、或同一正文少了条件，都是内容冲突，原行不动。
func TestSavingChargeConditionsTwiceIsAReplayAndChangedConditionsConflict(t *testing.T) {
	repository, contents, _, transactor := newSupplierAgreementContents(t)
	ctx := t.Context()

	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v2", "digest-a2")
	mustSaveVersion(t, transactor, ctx, repository, version)
	body := supplierAgreementOn(t, version, "supplier-1", "plan-buy-1")
	original := withConditions(t, body, chargeConditionsOf(t, domain.PerPieceQuantity))
	mustSaveSupplierAgreement(t, transactor, ctx, repository, original)

	for name, candidate := range map[string]struct {
		agreement domain.SupplierAgreement
		want      ports.SupplierAgreementSaveOutcome
	}{
		"same content":      {original, ports.SupplierAgreementAlreadyRegistered},
		"another form":      {withConditions(t, body, chargeConditionsOf(t, domain.PerSegmentQuantity)), ports.SupplierAgreementContentConflict},
		"conditions absent": {body, ports.SupplierAgreementContentConflict},
	} {
		mustWithinPublicationTransaction(t, transactor, ctx, func(txCtx context.Context) error {
			outcome, err := repository.SaveSupplierAgreement(txCtx, candidate.agreement)
			if err != nil {
				return err
			}
			if outcome != candidate.want {
				t.Fatalf("%s: outcome = %q, want %q", name, outcome, candidate.want)
			}
			return nil
		})
	}

	read, err := loadVersion(t, contents, "tenant-1", "agreement-1", "v2")
	if err != nil {
		t.Fatal(err)
	}
	conditions, _ := read.Agreement.ChargeConditions()
	fulfillment, _ := conditions.For(domain.ActualFulfillmentChargeReason)
	if form, _ := fulfillment.QuantityForm(); form != domain.PerPieceQuantity {
		t.Fatalf("冲突写入改动了原行：数量形 = %s", form)
	}
}

// Covers: 迁移 0038——库面守原因与数量形的封闭集，计费必带数量形、不计费必不带；条件行必须挂在正文行上。
func TestTheChargeConditionTableRefusesWhatTheGrammarCannotSay(t *testing.T) {
	repository, transactor, pool := newChargeConditionFixture(t)
	ctx := t.Context()
	version := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-1", "v2", "digest-a2")
	mustSaveVersion(t, transactor, ctx, repository, version)
	mustSaveSupplierAgreement(t, transactor, ctx, repository, supplierAgreementOn(t, version, "supplier-1", "plan-buy-1"))
	bare := effectiveVersionOfKind(t, domain.SupplierAgreementObject, "agreement-bare", "v1", "digest-b1")
	mustSaveVersion(t, transactor, ctx, repository, bare)

	for name, row := range map[string]struct {
		object, reason string
		charged        bool
		form           any
	}{
		"outside reason":        {"agreement-1", "DETENTION", false, nil},
		"outside form":          {"agreement-1", "ACTUAL_FULFILLMENT", true, "PER_KG"},
		"charged without form":  {"agreement-1", "ACTUAL_FULFILLMENT", true, nil},
		"uncharged with a form": {"agreement-1", "FAILED_ATTEMPT", false, "PER_ATTEMPT"},
		"no body row":           {"agreement-bare", "BOOKING", false, nil},
	} {
		version := "v2"
		if row.object == "agreement-bare" {
			version = "v1"
		}
		if _, err := pool.Exec(ctx,
			`INSERT INTO party_commercial.supplier_agreement_charge_condition
				(tenant_id, object_kind, object_id, version_label, reason, charged, quantity_form)
			 VALUES ('tenant-1', 3, $1, $2, $3, $4, $5)`,
			row.object, version, row.reason, row.charged, row.form,
		); err == nil {
			t.Errorf("%s：库面收下了文法说不出的一行", name)
		}
	}
}
