package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/transportfulfillment/application"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/transportfulfillment/ports"
)

// 本文件证总单登记编排（ADR-0113）的结果代数：首登、重放、冲突、已有版本链、三种新版本（撤销 / 替代 /
// 关联重述）、未登记、已不适用、未受理、未决，以及并发撞键读回赢家与链被别人推进后的冲突。登记册用内存
// 替身，真库那一半在 adapters/postgres 的用例里。

var (
	masterDocumentNowAt     = time.Date(2026, 9, 6, 12, 0, 0, 0, time.UTC)
	masterDocumentChangedAt = time.Date(2026, 9, 6, 8, 0, 0, 0, time.UTC)
)

type masterDocumentClock struct{ at time.Time }

func (clock masterDocumentClock) Now() time.Time { return clock.at }

// masterDocumentRegistryDouble 是只插不改的内存登记册；`当前版`同真实现一样按回指派生，第二个首版与同一
// 前版被回指两次也同真实现一样只答已登记（0017 两道部分唯一索引的替身）。raceWinner 非空时模拟并发：本方
// Save 那一刻另一方先落了它。
type masterDocumentRegistryDouble struct {
	records    []ports.MasterDocumentRecord
	failFind   error
	failSave   error
	raceWinner *ports.MasterDocumentRecord
}

func (double *masterDocumentRegistryDouble) FindByKey(
	_ context.Context,
	key ports.MasterDocumentKey,
) (ports.MasterDocumentRecord, bool, error) {
	if double.failFind != nil {
		return ports.MasterDocumentRecord{}, false, double.failFind
	}
	for _, record := range double.records {
		if record.Key == key {
			return record, true, nil
		}
	}
	return ports.MasterDocumentRecord{}, false, nil
}

func (double *masterDocumentRegistryDouble) FindCurrent(
	_ context.Context,
	tenant domain.TenantID,
	document domain.MasterDocumentReference,
) (ports.MasterDocumentRecord, bool, error) {
	if double.failFind != nil {
		return ports.MasterDocumentRecord{}, false, double.failFind
	}
	superseded := map[domain.MasterDocumentVersion]bool{}
	for _, record := range double.records {
		if record.Key.TenantID != tenant || record.Key.Document != document {
			continue
		}
		if prior, has := record.Document.Supersedes(); has {
			superseded[prior] = true
		}
	}
	var current ports.MasterDocumentRecord
	found := false
	for _, record := range double.records {
		if record.Key.TenantID != tenant || record.Key.Document != document || superseded[record.Key.Version] {
			continue
		}
		if !found || record.RecordedAt.After(current.RecordedAt) {
			current, found = record, true
		}
	}
	return current, found, nil
}

func (double *masterDocumentRegistryDouble) ListVersions(
	_ context.Context,
	tenant domain.TenantID,
	document domain.MasterDocumentReference,
) ([]ports.MasterDocumentRecord, error) {
	var versions []ports.MasterDocumentRecord
	for _, record := range double.records {
		if record.Key.TenantID == tenant && record.Key.Document == document {
			versions = append(versions, record)
		}
	}
	return versions, nil
}

func (double *masterDocumentRegistryDouble) Save(
	_ context.Context,
	record ports.MasterDocumentRecord,
) (ports.MasterDocumentSaveOutcome, error) {
	if double.failSave != nil {
		return ports.MasterDocumentSaveOutcomeInvalid, double.failSave
	}
	if double.raceWinner != nil {
		double.records = append(double.records, *double.raceWinner)
		double.raceWinner = nil
		return ports.MasterDocumentAlreadyRegistered, nil
	}
	_, incomingSupersedes := record.Document.Supersedes()
	for _, existing := range double.records {
		if existing.Key == record.Key {
			return ports.MasterDocumentAlreadyRegistered, nil
		}
		if existing.Key.TenantID != record.Key.TenantID || existing.Key.Document != record.Key.Document {
			continue
		}
		_, existingSupersedes := existing.Document.Supersedes()
		if !incomingSupersedes && !existingSupersedes {
			return ports.MasterDocumentAlreadyRegistered, nil
		}
		if a, ok := existing.Document.Supersedes(); ok {
			if b, ok := record.Document.Supersedes(); ok && a == b {
				return ports.MasterDocumentAlreadyRegistered, nil
			}
		}
	}
	double.records = append(double.records, record)
	return ports.MasterDocumentSaved, nil
}

func newMasterDocumentHandler(registry *masterDocumentRegistryDouble) *application.RegisterMasterDocumentHandler {
	return application.NewRegisterMasterDocumentHandler(application.RegisterMasterDocumentDeps{
		Documents: registry,
		Clock:     masterDocumentClock{at: masterDocumentNowAt},
	})
}

func registerMasterDocumentCommand(t *testing.T, document, version string) application.RegisterMasterDocumentCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	return application.RegisterMasterDocumentCommand{
		TenantID: tenant,
		Document: document,
		Version:  version,
		Issuer:   "party/carrier-x",
		Scope:    "SYN-LANE-1",
		Associations: []application.MasterDocumentAssociationInput{
			{Kind: "CONSOLIDATION_UNIT", Reference: "CU-1"},
			{Kind: "PARCEL", Reference: "PCL-1"},
		},
	}
}

func reviseMasterDocumentCommand(t *testing.T, document string, revision domain.MasterDocumentRevision, version string) application.ReviseMasterDocumentCommand {
	t.Helper()
	tenant, err := domain.NewTenantID("tenant-1")
	if err != nil {
		t.Fatalf("租户：%v", err)
	}
	command := application.ReviseMasterDocumentCommand{
		TenantID:   tenant,
		Document:   document,
		Revision:   revision,
		At:         masterDocumentChangedAt,
		NewVersion: version,
	}
	switch revision {
	case domain.MasterDocumentSupersession:
		command.Replacement = document + "-B"
	case domain.MasterDocumentAssociationRestatement:
		command.Associations = []application.MasterDocumentAssociationInput{{Kind: "CONSOLIDATION_UNIT", Reference: "CU-2"}}
	}
	return command
}

func mustRegisterMasterDocument(t *testing.T, handler *application.RegisterMasterDocumentHandler, command application.RegisterMasterDocumentCommand) ports.MasterDocumentRecord {
	t.Helper()
	result, err := handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.MasterDocumentRegistered {
		t.Fatalf("首登：%v %s", err, result.Outcome())
	}
	record, has := result.Record()
	if !has {
		t.Fatal("首登没有带回记录")
	}
	return record
}

func TestRegisteringAMasterDocumentPersistsTheFirstVersionWithItsAssociations(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	command := registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1")
	command.Commission = "COMM-1"

	record := mustRegisterMasterDocument(t, handler, command)

	if record.Document.Issuer().String() != "party/carrier-x" || record.Document.Scope().String() != "SYN-LANE-1" || !record.Document.InForce() {
		t.Fatalf("首版没有原样落库：%+v", record.Document)
	}
	if commission, has := record.Document.Commission(); !has || commission.String() != "COMM-1" {
		t.Fatalf("运输委托引用走样：%v %q", has, commission)
	}
	if _, has := record.Document.Booking(); has {
		t.Fatal("没给订舱引用却落了一个")
	}
	if len(record.Document.Associations()) != 2 || !record.RecordedAt.Equal(masterDocumentNowAt) {
		t.Fatalf("关联集或登记时刻走样：%d %s", len(record.Document.Associations()), record.RecordedAt)
	}
	if len(registry.records) != 1 {
		t.Fatalf("登记册里应有一版：%d", len(registry.records))
	}
}

func TestRegisteringTheSameVersionAgainIsAReplayAndADifferentBodyIsAConflict(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	command := registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1")
	mustRegisterMasterDocument(t, handler, command)

	reordered := command
	reordered.Associations = []application.MasterDocumentAssociationInput{command.Associations[1], command.Associations[0]}
	replay, _ := handler.Register(t.Context(), reordered)
	if replay.Outcome() != application.MasterDocumentExistingVersion {
		t.Fatalf("同内容重放（关联换序）应答已有版本：%s", replay.Outcome())
	}

	changed := command
	changed.Scope = "SYN-LANE-2"
	conflict, _ := handler.Register(t.Context(), changed)
	if conflict.Outcome() != application.MasterDocumentContentConflict {
		t.Fatalf("同键异内容应答冲突：%s", conflict.Outcome())
	}
	if record, has := conflict.Record(); !has || record.Document.Scope().String() != "SYN-LANE-1" {
		t.Fatal("冲突应带回既有版本而不是顶替它")
	}
	if len(registry.records) != 1 {
		t.Fatal("冲突落了第二版")
	}
}

func TestASecondFirstVersionForARegisteredDocumentAnswersAlreadyChained(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))

	result, _ := handler.Register(t.Context(), registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1B"))
	if result.Outcome() != application.MasterDocumentAlreadyChained {
		t.Fatalf("第二个首版应答已有版本链：%s", result.Outcome())
	}
	if record, has := result.Record(); !has || record.Key.Version.String() != "MDV-1" {
		t.Fatal("已有版本链应带回当前版")
	}
}

func TestEachRevisionFormsANewVersionChainedToTheCurrentOne(t *testing.T) {
	cases := map[string]struct {
		revision domain.MasterDocumentRevision
		standing domain.MasterDocumentStanding
		assert   func(*testing.T, domain.MasterDocument)
	}{
		"撤销": {revision: domain.MasterDocumentRevocation, standing: domain.MasterDocumentRevoked, assert: func(t *testing.T, document domain.MasterDocument) {
			if len(document.Associations()) != 2 {
				t.Fatal("撤销应原样带过关联集")
			}
		}},
		"替代": {revision: domain.MasterDocumentSupersession, standing: domain.MasterDocumentSuperseded, assert: func(t *testing.T, document domain.MasterDocument) {
			if replacedBy, has := document.ReplacedBy(); !has || replacedBy.String() != "SYN-MAWB-1-B" {
				t.Fatalf("替代者走样：%v %q", has, replacedBy)
			}
		}},
		"关联重述": {revision: domain.MasterDocumentAssociationRestatement, standing: domain.MasterDocumentInForce, assert: func(t *testing.T, document domain.MasterDocument) {
			if associations := document.Associations(); len(associations) != 1 || associations[0].Reference() != "CU-2" {
				t.Fatalf("重述后的关联集走样：%+v", associations)
			}
		}},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			registry := &masterDocumentRegistryDouble{}
			handler := newMasterDocumentHandler(registry)
			mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))

			result, err := handler.Revise(t.Context(), reviseMasterDocumentCommand(t, "SYN-MAWB-1", testCase.revision, "MDV-2"))
			if err != nil || result.Outcome() != application.MasterDocumentRevised {
				t.Fatalf("形成新版本：%v %s", err, result.Outcome())
			}
			record, _ := result.Record()
			if record.Key.Version.String() != "MDV-2" || record.Document.Standing() != testCase.standing {
				t.Fatalf("新版本走样：%q %s", record.Key.Version, record.Document.Standing())
			}
			if prior, has := record.Document.Supersedes(); !has || prior.String() != "MDV-1" {
				t.Fatalf("新版本应回指首版：%v %q", has, prior)
			}
			if changedAt, has := record.Document.ChangedAt(); !has || !changedAt.Equal(masterDocumentChangedAt) {
				t.Fatalf("改变时间走样：%v %s", has, changedAt)
			}
			testCase.assert(t, record.Document)

			current, found, _ := registry.FindCurrent(t.Context(), record.Key.TenantID, record.Key.Document)
			if !found || current.Key.Version.String() != "MDV-2" {
				t.Fatalf("当前版应是新版本：%v %q", found, current.Key.Version)
			}
			first, _, _ := registry.FindByKey(t.Context(), ports.MasterDocumentKey{TenantID: record.Key.TenantID, Document: record.Key.Document, Version: domainVersion(t, "MDV-1")})
			if !first.Document.InForce() || len(first.Document.Associations()) != 2 {
				t.Fatal("首版被改动")
			}
		})
	}
}

func domainVersion(t *testing.T, raw string) domain.MasterDocumentVersion {
	t.Helper()
	version, err := domain.NewMasterDocumentVersion(raw)
	if err != nil {
		t.Fatalf("版本：%v", err)
	}
	return version
}

func TestReplayingARevisionAnswersExistingVersionAndADifferentRevisionUnderThatVersionIsAConflict(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
	revoke := reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentRevocation, "MDV-2")
	if result, _ := handler.Revise(t.Context(), revoke); result.Outcome() != application.MasterDocumentRevised {
		t.Fatalf("首次撤销：%s", result.Outcome())
	}

	replay, _ := handler.Revise(t.Context(), revoke)
	if replay.Outcome() != application.MasterDocumentExistingVersion {
		t.Fatalf("同一次撤销重放应答已有版本：%s", replay.Outcome())
	}

	different := revoke
	different.At = masterDocumentChangedAt.Add(time.Hour)
	conflict, _ := handler.Revise(t.Context(), different)
	if conflict.Outcome() != application.MasterDocumentContentConflict {
		t.Fatalf("同版本号不同改变应答冲突：%s", conflict.Outcome())
	}
	if len(registry.records) != 2 {
		t.Fatal("冲突落了第三版")
	}
}

func TestRevisingWithTheCurrentVersionNumberOfAFirstVersionIsNotAccepted(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))

	result, _ := handler.Revise(t.Context(), reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentRevocation, "MDV-1"))
	if result.Outcome() != application.MasterDocumentRegistrationNotAccepted {
		t.Fatalf("拿首版的版本号改它就是覆盖，应未受理：%s", result.Outcome())
	}
}

func TestRevisionAnswersThatRegisterNothing(t *testing.T) {
	cases := map[string]struct {
		arrange func(*testing.T, *masterDocumentRegistryDouble, *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand
		want    application.MasterDocumentRegistrationOutcome
	}{
		"从没登记过的总单": {
			arrange: func(t *testing.T, _ *masterDocumentRegistryDouble, _ *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand {
				return reviseMasterDocumentCommand(t, "SYN-MAWB-never", domain.MasterDocumentRevocation, "MDV-2")
			},
			want: application.MasterDocumentNotRegistered,
		},
		"已撤销的总单再替代": {
			arrange: func(t *testing.T, _ *masterDocumentRegistryDouble, handler *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand {
				mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
				handler.Revise(t.Context(), reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentRevocation, "MDV-2"))
				return reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentSupersession, "MDV-3")
			},
			want: application.MasterDocumentNoLongerInForce,
		},
		"撤销却带了替代者": {
			arrange: func(t *testing.T, _ *masterDocumentRegistryDouble, handler *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand {
				mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
				command := reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentRevocation, "MDV-2")
				command.Replacement = "SYN-MAWB-9"
				return command
			},
			want: application.MasterDocumentRegistrationNotAccepted,
		},
		"替代却没有替代者": {
			arrange: func(t *testing.T, _ *masterDocumentRegistryDouble, handler *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand {
				mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
				command := reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentSupersession, "MDV-2")
				command.Replacement = ""
				return command
			},
			want: application.MasterDocumentRegistrationNotAccepted,
		},
		"关联重述给了同一组关联": {
			arrange: func(t *testing.T, _ *masterDocumentRegistryDouble, handler *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand {
				mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
				command := reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentAssociationRestatement, "MDV-2")
				command.Associations = []application.MasterDocumentAssociationInput{
					{Kind: "PARCEL", Reference: "PCL-1"},
					{Kind: "CONSOLIDATION_UNIT", Reference: "CU-1"},
				}
				return command
			},
			want: application.MasterDocumentRegistrationNotAccepted,
		},
		"关联类别不在封闭集合内": {
			arrange: func(t *testing.T, _ *masterDocumentRegistryDouble, handler *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand {
				mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
				command := reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentAssociationRestatement, "MDV-2")
				command.Associations = []application.MasterDocumentAssociationInput{{Kind: "BAG", Reference: "B-1"}}
				return command
			},
			want: application.MasterDocumentRegistrationNotAccepted,
		},
		"改变词不在封闭集合内": {
			arrange: func(t *testing.T, _ *masterDocumentRegistryDouble, handler *application.RegisterMasterDocumentHandler) application.ReviseMasterDocumentCommand {
				mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
				return reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentRevisionInvalid, "MDV-2")
			},
			want: application.MasterDocumentRegistrationNotAccepted,
		},
	}
	for name, testCase := range cases {
		t.Run(name, func(t *testing.T) {
			registry := &masterDocumentRegistryDouble{}
			handler := newMasterDocumentHandler(registry)
			command := testCase.arrange(t, registry, handler)
			before := len(registry.records)
			result, err := handler.Revise(t.Context(), command)
			if err != nil || result.Outcome() != testCase.want {
				t.Fatalf("outcome = %s err = %v, want %s", result.Outcome(), err, testCase.want)
			}
			if len(registry.records) != before {
				t.Fatal("没登记任何东西的答案却落了一版")
			}
		})
	}
}

func TestRegistrationInputThatCannotFormADocumentIsNotAccepted(t *testing.T) {
	handler := newMasterDocumentHandler(&masterDocumentRegistryDouble{})
	for name, mutate := range map[string]func(*application.RegisterMasterDocumentCommand){
		"缺签发方": func(command *application.RegisterMasterDocumentCommand) { command.Issuer = "  " },
		"缺范围":  func(command *application.RegisterMasterDocumentCommand) { command.Scope = "" },
		"关联重复": func(command *application.RegisterMasterDocumentCommand) {
			command.Associations = append(command.Associations, application.MasterDocumentAssociationInput{Kind: "PARCEL", Reference: "PCL-1"})
		},
		"关联类别未知": func(command *application.RegisterMasterDocumentCommand) {
			command.Associations = []application.MasterDocumentAssociationInput{{Kind: "TRACKING_NUMBER", Reference: "X"}}
		},
	} {
		t.Run(name, func(t *testing.T) {
			command := registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1")
			mutate(&command)
			result, err := handler.Register(t.Context(), command)
			if err != nil || result.Outcome() != application.MasterDocumentRegistrationNotAccepted {
				t.Fatalf("outcome = %s err = %v", result.Outcome(), err)
			}
		})
	}
}

func TestRegistryFailuresLeaveTheRegistrationUndecidedWithAContinuation(t *testing.T) {
	t.Run("find fails", func(t *testing.T) {
		registry := &masterDocumentRegistryDouble{failFind: errors.New("registry down")}
		result, err := newMasterDocumentHandler(registry).Register(t.Context(), registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
		if err != nil || result.Outcome() != application.MasterDocumentRegistrationUndecided || result.UndecidedReason() != application.MasterDocumentRegistryUnavailable {
			t.Fatalf("outcome = %s reason = %s err = %v", result.Outcome(), result.UndecidedReason(), err)
		}
		if result.ContinuationReference() == "" {
			t.Fatal("未决没有带续办引用")
		}
	})
	t.Run("save fails", func(t *testing.T) {
		registry := &masterDocumentRegistryDouble{failSave: errors.New("registry down")}
		result, _ := newMasterDocumentHandler(registry).Register(t.Context(), registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
		if result.Outcome() != application.MasterDocumentRegistrationUndecided {
			t.Fatalf("outcome = %s", result.Outcome())
		}
	})
	t.Run("the same key yields the same continuation", func(t *testing.T) {
		registry := &masterDocumentRegistryDouble{failFind: errors.New("registry down")}
		handler := newMasterDocumentHandler(registry)
		first, _ := handler.Register(t.Context(), registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
		second, _ := handler.Register(t.Context(), registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))
		if first.ContinuationReference() != second.ContinuationReference() {
			t.Fatal("同键的续办引用应稳定")
		}
	})
}

func TestARaceOnTheSameKeyReadsBackTheWinner(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	command := registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1")
	winnerCommand := command
	winnerCommand.Scope = "SYN-LANE-OTHER"
	winnerHandler := newMasterDocumentHandler(&masterDocumentRegistryDouble{})
	winner := mustRegisterMasterDocument(t, winnerHandler, winnerCommand)
	registry.raceWinner = &winner

	result, err := handler.Register(t.Context(), command)
	if err != nil || result.Outcome() != application.MasterDocumentContentConflict {
		t.Fatalf("并发下另一方先落了不同内容，应读回赢家答冲突：%v %s", err, result.Outcome())
	}
	if record, has := result.Record(); !has || record.Document.Scope().String() != "SYN-LANE-OTHER" {
		t.Fatal("冲突应带回赢家那一版")
	}
}

func TestARaceThatMovedTheChainAnswersConflictWithTheCurrentVersion(t *testing.T) {
	registry := &masterDocumentRegistryDouble{}
	handler := newMasterDocumentHandler(registry)
	first := mustRegisterMasterDocument(t, handler, registerMasterDocumentCommand(t, "SYN-MAWB-1", "MDV-1"))

	// 另一方在本方读到当前版之后、写入之前，先把首版撤销成 MDV-2；本方基于首版形成的重述 MDV-2B 撞链索引。
	otherRevoked, err := first.Document.Revoke(masterDocumentChangedAt, domainVersion(t, "MDV-2"))
	if err != nil {
		t.Fatalf("另一方撤销：%v", err)
	}
	otherRecord := ports.MasterDocumentRecord{
		Key:        ports.MasterDocumentKey{TenantID: first.Key.TenantID, Document: first.Key.Document, Version: otherRevoked.Version()},
		Document:   otherRevoked,
		RecordedAt: masterDocumentNowAt.Add(time.Minute),
	}
	registry.raceWinner = &otherRecord

	result, err := handler.Revise(t.Context(), reviseMasterDocumentCommand(t, "SYN-MAWB-1", domain.MasterDocumentAssociationRestatement, "MDV-2B"))
	if err != nil || result.Outcome() != application.MasterDocumentContentConflict {
		t.Fatalf("链被别人推进应答冲突并带回当前版：%v %s", err, result.Outcome())
	}
	if record, has := result.Record(); !has || record.Key.Version.String() != "MDV-2" {
		t.Fatalf("应带回链尾 MDV-2：%v %q", has, record.Key.Version)
	}
}
