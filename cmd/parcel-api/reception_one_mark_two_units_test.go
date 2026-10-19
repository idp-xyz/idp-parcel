package main

import (
	"slices"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	nopostgres "go.idp.xyz/idp-parcel/internal/nodeoperations/adapters/postgres"
	nodeopsapp "go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	nodomain "go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	noports "go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	"go.idp.xyz/idp-parcel/internal/platform/pgtest"
)

// Covers: `AT-NO-019`「同一外部条码出现在两件实物——两件实物保持独立，形成身份冲突并暂停方向性作业」的真库面（票
// no-reception-identity/01、ADR-0196）：委托经真编排带客户包裹件号建单并接受，号码在关联册里只标识那一件包裹；两件作业
// 实物先后贴着它经生产装配送站。先到那件形成收寄；后到那件答待识别、记身份冲突；先到那件的原判断原样读回，册上另多一版
// 身份冲突判断；方向依据的身份读口读这本册，两件的身份一格都含身份冲突。重放后到那件走已有结果。夹具全为合成（S 级）。
//
// 同时钉 `AT-NO-061` 的真库面（票 no-reception-identity/05、ADR-0210 决定五）：身份一格只读各链当前有效版本。先到那件的
// 冲突版指名了原判断，链头只剩冲突版，原因集是{待识别, 身份冲突}，不再只是身份冲突；后到那件只有自己那一版，原因集照旧
// 是{待识别, 身份冲突}。
func TestTheWiredReceptionTurnsOneMarkOnTwoUnitsIntoAnIdentityConflict(t *testing.T) {
	db, err := bentopg.NewDB(pgtest.Pool(t), bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		t.Fatalf("构造框架 DB：%v", err)
	}
	reception, err := buildReceptionOrchestration(db)
	if err != nil {
		t.Fatalf("装配收寄编排：%v", err)
	}
	acceptSubmitted(t, db, submitWithCustomerParcelNumber(t, db, "SYN-CUSTOMER-PARCEL-NRI01"))

	deliver := func(sourceID, unit string) nodeopsapp.ReceiveDeliveredUnitCommand {
		command := receptionCommand(t, sourceID, nodeopsapp.ExplicitReception)
		command.Unit = mustValue(t, nodomain.NewHandlingUnitID, unit)
		command.Mark = noports.ExternalMarkObservation{Mark: "SYN-CUSTOMER-PARCEL-NRI01"}
		return command
	}
	first := deliver("SYN-DELIVERY-NRI01-A", "SYN-UNIT-NRI01-A")
	formed, err := reception.Handle(t.Context(), first)
	if err != nil || formed.Outcome() != nodeopsapp.NodeIntakeFormed {
		t.Fatalf("先到那件 outcome = %v err = %v, want INTAKE_FORMED", formed.Outcome(), err)
	}
	original, _ := formed.Record()

	second := deliver("SYN-DELIVERY-NRI01-B", "SYN-UNIT-NRI01-B")
	conflicted, err := reception.Handle(t.Context(), second)
	if err != nil {
		t.Fatalf("后到那件：%v", err)
	}
	if got := conflicted.Outcome(); got != nodeopsapp.UnitPendingIdentification {
		t.Fatalf("后到那件 outcome = %v, want PENDING_IDENTIFICATION——同一个号已关联到另一件实物", got)
	}
	if late, _ := conflicted.Record(); !late.IdentityConflict || !late.Control.Active() {
		t.Fatalf("后到那件 = %+v, want 记身份冲突且控制成立", late)
	}

	receptions, err := nopostgres.NewReceptions(db)
	if err != nil {
		t.Fatalf("构造收寄库：%v", err)
	}
	kept, found, err := receptions.FindByKey(t.Context(), noports.ReceptionKey{TenantID: first.TenantID, SourceID: first.SourceID})
	if err != nil || !found {
		t.Fatalf("读回先到那件：found = %v err = %v", found, err)
	}
	if kept.Kind != noports.RecordIntakeFormed || kept.IdentityConflict || kept.Intake.Version() != original.Intake.Version() {
		t.Fatalf("先到那件的原判断被改了：%+v", kept)
	}

	identities, err := nopostgres.NewIntakeIdentities(db)
	if err != nil {
		t.Fatalf("构造身份读口：%v", err)
	}
	for _, unit := range []string{"SYN-UNIT-NRI01-A", "SYN-UNIT-NRI01-B"} {
		observations, err := identities.FindIntakeIdentities(t.Context(), first.TenantID, mustValue(t, nodomain.NewHandlingUnitID, unit))
		if err != nil {
			t.Fatalf("读 %s 的收寄身份：%v", unit, err)
		}
		basis, err := nodomain.JudgeIdentityBasis(observations)
		if err != nil {
			t.Fatalf("判 %s 的身份：%v", unit, err)
		}
		want := []nodomain.DirectionalBasisReason{nodomain.ReasonPendingIdentification, nodomain.ReasonIdentityConflict}
		if reasons := basis.Cell().Reasons(); basis.Cell().Answer() != nodomain.BasisMissing || !slices.Equal(reasons, want) {
			t.Fatalf("%s 的身份一格 = %s %v（%d 行）, want 缺少·%v", unit, basis.Cell().Answer(), reasons, len(observations), want)
		}
	}

	replayed, err := reception.Handle(t.Context(), second)
	if err != nil || replayed.Outcome() != nodeopsapp.ReceptionExistingResult {
		t.Fatalf("重放后到那件 outcome = %v err = %v, want EXISTING_RESULT", replayed.Outcome(), err)
	}
}
