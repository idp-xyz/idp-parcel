package application_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// unversionedDigestCase 是一口在 NOC-1 之前入库的一条记录。first 首次提交并交回替身里那条记录的
// 摘要与改写它的办法；随后把摘要改写成 unversioned，再走 replay（同内容，必须答已有结果）与
// conflict（同身份异内容，必须答内容冲突）。三步共用一个替身。
//
// unversioned 是基 1121ba61 上的旧代码对 first 那条命令落下的摘要，写成定值而不在这里现算：
// 拿恢复后的函数现算，恢复得不一致也照样绿。
type unversionedDigestCase struct {
	name        string
	unversioned string
	first       func(t *testing.T) (string, func(string))
	replay      func(t *testing.T)
	conflict    func(t *testing.T)
}

// storedDigest 交回替身里 key 那条记录的摘要，与把它改写成别的串的办法。
func storedDigest[K comparable, V any](t *testing.T, records map[K]V, key K, digest func(*V) *string) (string, func(string)) {
	t.Helper()
	record, found := records[key]
	if !found {
		t.Fatalf("替身里没有 %v 那条记录", key)
	}
	return *digest(&record), func(rewritten string) {
		*digest(&record) = rewritten
		records[key] = record
	}
}

func wantOutcome[O comparable](t *testing.T, got O, err error, want O) {
	t.Helper()
	if err != nil {
		t.Fatalf("handle: %v", err)
	}
	if got != want {
		t.Fatalf("outcome = %v, want %v", got, want)
	}
}

func unversionedDigestCases(t *testing.T) []unversionedDigestCase {
	t.Helper()
	return []unversionedDigestCase{
		acceptanceUnversionedCase(t),
		executionUnversionedCase(t),
		openUnitUnversionedCase(t),
		addMemberUnversionedCase(t),
		removeMemberUnversionedCase(t),
		sealUnitUnversionedCase(t),
		unsealUnitUnversionedCase(t),
		closeUnitUnversionedCase(t),
		receptionUnversionedCase(t),
	}
}

// Covers: ADR-0014「规范化版本不同不是冲突」——NOC-1 之前入库的记录存的是无版本摘要，同一条命令
// 重放按无版本那一版重算再比，答已有结果；同身份换内容仍是内容冲突。定形口逐口走一遍。
func TestRecordsStoredBeforeNOC1AreComparedUnderTheirOwnShape(t *testing.T) {
	for _, testCase := range unversionedDigestCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			_, rewrite := testCase.first(t)
			rewrite(testCase.unversioned)
			testCase.replay(t)
			testCase.conflict(t)
		})
	}
}

// Covers: 新记录写的是 NOC-1 形——无版本那一版只用来比旧记录，误写进新记录时上一例照样绿，
// 要在这里断。
func TestNewRecordsAreStoredUnderNOC1(t *testing.T) {
	for _, testCase := range unversionedDigestCases(t) {
		t.Run(testCase.name, func(t *testing.T) {
			if stored, _ := testCase.first(t); !strings.HasPrefix(stored, "NOC-1:") {
				t.Fatalf("新记录落下的摘要是 %q，want NOC-1 形", stored)
			}
		})
	}
}

// Covers: 认不出的形状版本不答冲突——按该处读失败的既有答复作答（承接口是存储未决）。
func TestAnUnknownStoredShapeIsUndecidedNotAConflict(t *testing.T) {
	fixture := newCollabFixture(t)
	command := acceptCommand(t)
	wantOutcome(t, mustAccept(fixture, command).Outcome(), nil, application.CollaborationDecided)
	_, rewrite := storedDigest(t, fixture.acceptances.records, acceptanceKey(ports.CollaborationAcceptanceKey{
		TenantID: command.TenantID,
		Item:     mustValue(t, domain.NewCollaborationItemReference, command.Item),
	}), func(record *ports.CollaborationAcceptanceRecord) *string { return &record.ContentDigest })
	rewrite("NOC-9:9938edb9fffaab6f67866d4ed949ee826323c990a1a5ffc322c9b7f3ad19b430")

	result, err := fixture.handler.Accept(context.Background(), command)
	wantOutcome(t, result.Outcome(), err, application.CollaborationUndecided)
	if result.UndecidedReason() != application.AcceptanceStoreUnavailable {
		t.Fatalf("reason = %q", result.UndecidedReason())
	}
}

func mustAccept(fixture *collabFixture, command application.AcceptCollaborationCommand) application.CollaborationResult {
	result, _ := fixture.handler.Accept(context.Background(), command)
	return result
}

func acceptanceUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newCollabFixture(t)
	command := acceptCommand(t)
	key := acceptanceKey(ports.CollaborationAcceptanceKey{
		TenantID: command.TenantID,
		Item:     mustValue(t, domain.NewCollaborationItemReference, command.Item),
	})
	accept := func(t *testing.T, command application.AcceptCollaborationCommand, want application.CollaborationOutcome) {
		t.Helper()
		result, err := fixture.handler.Accept(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "ACCEPT_COLLABORATION",
		unversioned: "871c56d459556488be4362668b01cbe55158407bbbb95b5cfc14c0b797ded1e3",
		first: func(t *testing.T) (string, func(string)) {
			accept(t, command, application.CollaborationDecided)
			return storedDigest(t, fixture.acceptances.records, key,
				func(record *ports.CollaborationAcceptanceRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { accept(t, command, application.CollaborationExistingDecision) },
		conflict: func(t *testing.T) {
			changed := acceptCommand(t)
			changed.Authority = "node-authority-2"
			accept(t, changed, application.CollaborationDecisionConflict)
		},
	}
}

func executionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newCollabFixture(t)
	command := executeCommand(t)
	key := factKey(ports.ExecutionFactKey{
		TenantID: command.TenantID,
		Item:     mustValue(t, domain.NewCollaborationItemReference, command.Item),
		Unit:     mustValue(t, domain.NewHandlingUnitID, command.Unit),
		Action:   command.Action,
	})
	record := func(t *testing.T, command application.RecordExecutionCommand, want application.CollaborationOutcome) {
		t.Helper()
		result, err := fixture.handler.RecordExecution(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "RECORD_EXECUTION",
		unversioned: "90b68a7f12095c1a9ebfe754792f089742e205fb0683dc94c1390de34ff466ae",
		first: func(t *testing.T) (string, func(string)) {
			wantOutcome(t, mustAccept(fixture, acceptCommand(t)).Outcome(), nil, application.CollaborationDecided)
			record(t, command, application.ExecutionRecorded)
			return storedDigest(t, fixture.facts.records, key,
				func(record *ports.ExecutionFactRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { record(t, command, application.ExecutionExistingFact) },
		conflict: func(t *testing.T) {
			changed := executeCommand(t)
			changed.Evidence = "execution-evidence-2"
			record(t, changed, application.ExecutionFactConflict)
		},
	}
}

// consolidationUnversionedCase 是集运各口共用的形：prepare 把单元推到这一口能做的状态，submit
// 以来源身份 sourceID 交这一口的命令。replay 与 conflict 在受理闸上就分出去，不再碰单元。
func consolidationUnversionedCase(
	t *testing.T,
	name string,
	sourceID string,
	unversioned string,
	prepare func(t *testing.T, fixture *consolidateFixture),
	submit func(fixture *consolidateFixture, source domain.WorkFactSource, changed bool) (application.ConsolidationResult, error),
	formed application.ConsolidationOutcome,
) unversionedDigestCase {
	fixture := newConsolidateFixture(t)
	source := workSource(t, sourceID, consolidationAt.Add(time.Hour))
	key := ports.ConsolidationFactKey{TenantID: consolidationTenant(t), SourceID: sourceID}
	return unversionedDigestCase{
		name:        name,
		unversioned: unversioned,
		first: func(t *testing.T) (string, func(string)) {
			prepare(t, fixture)
			result, err := submit(fixture, source, false)
			wantOutcome(t, result.Outcome(), err, formed)
			return storedDigest(t, fixture.facts.byKey, key,
				func(record *ports.ConsolidationFactRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) {
			result, err := submit(fixture, source, false)
			wantOutcome(t, result.Outcome(), err, application.ConsolidationExistingResult)
		},
		conflict: func(t *testing.T) {
			result, err := submit(fixture, source, true)
			wantOutcome(t, result.Outcome(), err, application.ConsolidationSourceConflict)
		},
	}
}

func addMemberTo(t *testing.T, fixture *consolidateFixture, unit, member, sourceID string) {
	t.Helper()
	result, err := fixture.handler.AddMember(context.Background(), application.AddMemberCommand{
		TenantID: consolidationTenant(t),
		Unit:     unitID(t, unit),
		Member:   handlingUnit(t, member),
		Source:   workSource(t, sourceID, consolidationAt),
	})
	wantOutcome(t, result.Outcome(), err, application.MemberAdded)
}

func sealUnit(t *testing.T, fixture *consolidateFixture, unit, sourceID string) {
	t.Helper()
	result, err := fixture.handler.Seal(context.Background(), application.SealUnitCommand{
		TenantID: consolidationTenant(t),
		Unit:     unitID(t, unit),
		Seal:     mustValue(t, domain.NewSealReference, "seal-prepared"),
		Basis:    mustValue(t, domain.NewWorkBasisReference, "WORK-ORDER/prepared"),
		Source:   workSource(t, sourceID, consolidationAt),
	})
	wantOutcome(t, result.Outcome(), err, application.UnitSealedRecorded)
}

func openUnitUnversionedCase(t *testing.T) unversionedDigestCase {
	tenant := consolidationTenant(t)
	unit := unitID(t, "unit-legacy-open")
	asset := mustValue(t, domain.NewCarrierAssetReference, "cage-legacy-1")
	changedAsset := mustValue(t, domain.NewCarrierAssetReference, "cage-legacy-2")
	return consolidationUnversionedCase(t, "OPEN_UNIT", "src-legacy-open",
		"a18161c8378a138b361140f8a7edb6ca97e6f28238f831c1d92a3e2e5818904d",
		func(*testing.T, *consolidateFixture) {},
		func(fixture *consolidateFixture, source domain.WorkFactSource, changed bool) (application.ConsolidationResult, error) {
			command := application.OpenUnitCommand{TenantID: tenant, Unit: unit, Asset: asset, Source: source}
			if changed {
				command.Asset = changedAsset
			}
			return fixture.handler.Open(context.Background(), command)
		},
		application.UnitOpened)
}

func addMemberUnversionedCase(t *testing.T) unversionedDigestCase {
	tenant := consolidationTenant(t)
	unit := unitID(t, "unit-legacy-add")
	member := handlingUnit(t, "hu-legacy-1")
	changedMember := handlingUnit(t, "hu-legacy-2")
	return consolidationUnversionedCase(t, "ADD_MEMBER", "src-legacy-add",
		"72cb96a33422754e59a8d11d08f80f02d81acc5ef10e9af49b9698605dea9d94",
		func(t *testing.T, fixture *consolidateFixture) { openUnit(t, fixture, "unit-legacy-add") },
		func(fixture *consolidateFixture, source domain.WorkFactSource, changed bool) (application.ConsolidationResult, error) {
			command := application.AddMemberCommand{TenantID: tenant, Unit: unit, Member: member, Source: source}
			if changed {
				command.Member = changedMember
			}
			return fixture.handler.AddMember(context.Background(), command)
		},
		application.MemberAdded)
}

func removeMemberUnversionedCase(t *testing.T) unversionedDigestCase {
	tenant := consolidationTenant(t)
	unit := unitID(t, "unit-legacy-remove")
	member := handlingUnit(t, "hu-legacy-1")
	changedMember := handlingUnit(t, "hu-legacy-2")
	return consolidationUnversionedCase(t, "REMOVE_MEMBER", "src-legacy-remove",
		"1bdf60db8d459e66e88a7c2a3706fb784558b32410a1cbe644625bbb8ef75ad8",
		func(t *testing.T, fixture *consolidateFixture) {
			openUnit(t, fixture, "unit-legacy-remove")
			addMemberTo(t, fixture, "unit-legacy-remove", "hu-legacy-1", "src-legacy-remove-prepare")
		},
		func(fixture *consolidateFixture, source domain.WorkFactSource, changed bool) (application.ConsolidationResult, error) {
			command := application.RemoveMemberCommand{TenantID: tenant, Unit: unit, Member: member, Source: source}
			if changed {
				command.Member = changedMember
			}
			return fixture.handler.RemoveMember(context.Background(), command)
		},
		application.MemberRemoved)
}

func sealUnitUnversionedCase(t *testing.T) unversionedDigestCase {
	tenant := consolidationTenant(t)
	unit := unitID(t, "unit-legacy-seal")
	seal := mustValue(t, domain.NewSealReference, "seal-legacy-1")
	changedSeal := mustValue(t, domain.NewSealReference, "seal-legacy-2")
	basis := mustValue(t, domain.NewWorkBasisReference, "WORK-ORDER/legacy-seal")
	return consolidationUnversionedCase(t, "SEAL_UNIT", "src-legacy-seal",
		"104255f9f90ef14f6dc5f61e3fb5e0f22d23838424855d3ecbceb7b93321b3fa",
		func(t *testing.T, fixture *consolidateFixture) {
			openUnit(t, fixture, "unit-legacy-seal")
			addMemberTo(t, fixture, "unit-legacy-seal", "hu-legacy-1", "src-legacy-seal-prepare")
		},
		func(fixture *consolidateFixture, source domain.WorkFactSource, changed bool) (application.ConsolidationResult, error) {
			command := application.SealUnitCommand{TenantID: tenant, Unit: unit, Seal: seal, Basis: basis, Source: source}
			if changed {
				command.Seal = changedSeal
			}
			return fixture.handler.Seal(context.Background(), command)
		},
		application.UnitSealedRecorded)
}

func unsealUnitUnversionedCase(t *testing.T) unversionedDigestCase {
	tenant := consolidationTenant(t)
	unit := unitID(t, "unit-legacy-unseal")
	basis := mustValue(t, domain.NewWorkBasisReference, "WORK-ORDER/legacy-unseal-1")
	changedBasis := mustValue(t, domain.NewWorkBasisReference, "WORK-ORDER/legacy-unseal-2")
	return consolidationUnversionedCase(t, "UNSEAL_UNIT", "src-legacy-unseal",
		"826e16896757736e2ca30fba13203c51f16bf2f40b0bf1dff72a950951037e65",
		func(t *testing.T, fixture *consolidateFixture) {
			openUnit(t, fixture, "unit-legacy-unseal")
			addMemberTo(t, fixture, "unit-legacy-unseal", "hu-legacy-1", "src-legacy-unseal-add")
			sealUnit(t, fixture, "unit-legacy-unseal", "src-legacy-unseal-seal")
		},
		func(fixture *consolidateFixture, source domain.WorkFactSource, changed bool) (application.ConsolidationResult, error) {
			command := application.UnsealUnitCommand{TenantID: tenant, Unit: unit, Basis: basis, Source: source}
			if changed {
				command.Basis = changedBasis
			}
			return fixture.handler.Unseal(context.Background(), command)
		},
		application.UnitUnsealed)
}

func closeUnitUnversionedCase(t *testing.T) unversionedDigestCase {
	tenant := consolidationTenant(t)
	unit := unitID(t, "unit-legacy-close")
	changedDisposition := mustValue(t, domain.NewWorkBasisReference, "DISPOSITION/legacy-close")
	return consolidationUnversionedCase(t, "CLOSE_UNIT", "src-legacy-close",
		"d6e5e4cc34bbe52b50b9e92b3db134e8deba4bc3de92b52015eecfa4fa28d81d",
		func(t *testing.T, fixture *consolidateFixture) { openUnit(t, fixture, "unit-legacy-close") },
		func(fixture *consolidateFixture, source domain.WorkFactSource, changed bool) (application.ConsolidationResult, error) {
			command := application.CloseUnitCommand{TenantID: tenant, Unit: unit, Source: source}
			if changed {
				command.Disposition = changedDisposition
			}
			return fixture.handler.Close(context.Background(), command)
		},
		application.UnitClosedRecorded)
}

func receptionUnversionedCase(t *testing.T) unversionedDigestCase {
	fixture := newReceptionFixture(t)
	command := receiveCommand(t)
	key := ports.ReceptionKey{TenantID: command.TenantID, SourceID: command.SourceID}
	receive := func(t *testing.T, command application.ReceiveDeliveredUnitCommand, want application.ReceptionOutcome) {
		t.Helper()
		result, err := fixture.handler.Handle(context.Background(), command)
		wantOutcome(t, result.Outcome(), err, want)
	}
	return unversionedDigestCase{
		name:        "RECEIVE_DELIVERED_UNIT",
		unversioned: "9a5afc40ad43a53d750e5f339a36b88541d3ec8316ed28cc39e80bf3cba50238",
		first: func(t *testing.T) (string, func(string)) {
			receive(t, command, application.NodeIntakeFormed)
			return storedDigest(t, fixture.receptions.byKey, key,
				func(record *ports.ReceptionRecord) *string { return &record.ContentDigest })
		},
		replay: func(t *testing.T) { receive(t, command, application.ReceptionExistingResult) },
		conflict: func(t *testing.T) {
			changed := receiveCommand(t)
			changed.Mark = ports.ExternalMarkObservation{Mark: "BARCODE-2"}
			receive(t, changed, application.ReceptionSourceConflict)
		},
	}
}
