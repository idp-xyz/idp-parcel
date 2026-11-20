package postgres_test

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

func TestPreAdviceAcceptsGoodsInboundOnARealWarehouse(t *testing.T) {
	world := newPreAdviceWorld(t, preAdviceNumberingInstant)
	world.seedWarehouse(t, nil)
	world.seedGoods(t)
	before := stockLineCount(t, world, "SYN-TENANT-01")
	result := world.accept(t, preAdviceAcceptCommand(t, "REQ-1", nil))
	after := stockLineCount(t, world, "SYN-TENANT-01")
	advice, has := result.PreAdvice()
	number, numbered := advice.DocumentNumber()
	notice, hasNotice := result.Notice()
	if result.Outcome() != application.PreAdviceOutcomeAdvised || !has || !numbered || !hasNotice || notice.Version() != 1 {
		t.Fatalf("受理 = %s 有预报 %v 有号 %v 告知 %d", result.Outcome(), has, numbered, notice.Version())
	}
	if !strings.Contains(number.String(), "20261120") || strings.Contains(number.String(), "20261119") {
		t.Fatalf("单据号 %s，日期应按仓的业务时区是 2026-11-20", number)
	}
	if before != after {
		t.Fatalf("库存账行数 %d → %d", before, after)
	}
	reloaded, found, err := world.preAdvices.Find(t.Context(), advice.Tenant(), advice.ID())
	if err != nil || !found || reloaded.CurrentNotice().Version() != 1 || len(reloaded.GoodsLines()) != 1 {
		t.Fatalf("读回 %v %v 行 %d", found, err, len(reloaded.GoodsLines()))
	}
	reloadedNumber, ok := reloaded.DocumentNumber()
	if !ok || reloadedNumber != number || reloaded.NumberingAbsence().String() != "" {
		t.Fatalf("读回的号 %s 原因 %s", reloadedNumber, reloaded.NumberingAbsence())
	}
}

func TestPreAdviceUsesTheTenantTimeZoneWhenTheWarehouseHasNone(t *testing.T) {
	world := newPreAdviceWorld(t, preAdviceNumberingInstant)
	world.seedWarehouse(t, func(spec *domain.WarehouseContentSpec) {
		spec.BusinessTimeZone = domain.BusinessTimeZone{}
	})
	world.seedGoods(t)
	world.registerNumberingTimeZone(t)
	result := world.accept(t, preAdviceAcceptCommand(t, "REQ-1", nil))
	advice, _ := result.PreAdvice()
	number, numbered := advice.DocumentNumber()
	if result.Outcome() != application.PreAdviceOutcomeAdvised || !numbered ||
		!strings.Contains(number.String(), "20261120") || strings.Contains(number.String(), "20261119") {
		t.Fatalf("租户时区的号 = %s（%s）", number, result.Outcome())
	}
}

func TestPreAdviceAcceptsWithoutANumberWhenNoTimeZoneIsConfigured(t *testing.T) {
	world := newPreAdviceWorld(t, preAdviceNumberingInstant)
	world.seedWarehouse(t, func(spec *domain.WarehouseContentSpec) {
		spec.BusinessTimeZone = domain.BusinessTimeZone{}
	})
	world.seedGoods(t)
	result := world.accept(t, preAdviceAcceptCommand(t, "REQ-1", nil))
	advice, has := result.PreAdvice()
	_, numbered := advice.DocumentNumber()
	if result.Outcome() != application.PreAdviceOutcomeAdvised || !has || numbered ||
		advice.NumberingAbsence().String() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("无时区 = %s 有号 %v 原因 %s", result.Outcome(), numbered, advice.NumberingAbsence())
	}
	reloaded, found, err := world.preAdvices.Find(t.Context(), advice.Tenant(), advice.ID())
	if err != nil || !found {
		t.Fatalf("读回 %v %v", found, err)
	}
	if _, hasNumber := reloaded.DocumentNumber(); hasNumber || reloaded.NumberingAbsence().String() != "NUMBERING_TIME_ZONE_NOT_CONFIGURED" {
		t.Fatalf("读回原因 %s", reloaded.NumberingAbsence())
	}
}

func TestPreAdviceNoticeUpdatesKeepPriorVersions(t *testing.T) {
	world := newPreAdviceWorld(t, preAdviceNumberingInstant)
	world.seedWarehouse(t, nil)
	world.seedGoods(t)
	accepted := world.accept(t, preAdviceAcceptCommand(t, "REQ-1", func(command *application.AcceptPreAdviceCommand) {
		command.Clue = valueOf(t, domain.NewArrivalClue, "CONT-1")
	}))
	advice, _ := accepted.PreAdvice()
	original, _ := advice.DocumentNumber()
	first := world.update(t, preAdviceNoticeCommand(t, advice.ID(), "REQ-N1", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.ExpectedAt = preAdviceNumberingInstant.Add(48 * time.Hour)
		command.Reference = valueOf(t, domain.NewOwnerReference, "REF-2")
	}))
	second := world.update(t, preAdviceNoticeCommand(t, advice.ID(), "REQ-N2", func(command *application.UpdatePreAdviceNoticeCommand) {
		command.ExpectedAt = preAdviceNumberingInstant.Add(72 * time.Hour)
	}))
	notice, _ := second.Notice()
	if first.Outcome() != application.PreAdviceOutcomeNoticeUpdated || second.Outcome() != application.PreAdviceOutcomeNoticeUpdated || notice.Version() != 3 {
		t.Fatalf("两版更新 = %s %s 现行 %d", first.Outcome(), second.Outcome(), notice.Version())
	}
	reloaded, found, err := world.preAdvices.Find(t.Context(), advice.Tenant(), advice.ID())
	if err != nil || !found {
		t.Fatalf("读回 %v %v", found, err)
	}
	number, _ := reloaded.DocumentNumber()
	notices := reloaded.Notices()
	if len(notices) != 3 || reloaded.CurrentNotice().Version() != 3 || number != original || len(reloaded.GoodsLines()) != 1 ||
		reloaded.GoodsLines()[0].Quantity() != 2 || notices[0].Clue().String() != "CONT-1" || notices[2].Clue().Given() ||
		notices[1].Reference().String() != "REF-2" || notices[2].Reference().Given() {
		t.Fatalf("现行 %d 号 %s 行 %d 线索 %q/%v", reloaded.CurrentNotice().Version(), number, len(reloaded.GoodsLines()), notices[0].Clue(), notices[2].Clue().Given())
	}
}

func TestPreAdviceWritesRefuseToRunOutsideATransaction(t *testing.T) {
	world := newPreAdviceWorld(t, preAdviceNumberingInstant)
	ctx := t.Context()
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	id := valueOf(t, domain.NewPreAdviceID, "IPA-1")
	submission := preAdviceOwnerSubmission(t, "REQ-1")
	if err := world.preAdvices.HoldSubmission(ctx, tenant, submission); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldSubmission：%v", err)
	}
	if err := world.preAdvices.HoldPreAdvice(ctx, tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("HoldPreAdvice：%v", err)
	}
	if _, err := world.preAdvices.Insert(ctx, domain.PreAdvice{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Insert：%v", err)
	}
	if _, err := world.preAdvices.AppendNotice(ctx, tenant, id, domain.PreAdviceNotice{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("AppendNotice：%v", err)
	}
	if _, err := world.preAdvices.Cancel(ctx, domain.PreAdvice{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("Cancel：%v", err)
	}
	if err := world.preAdvices.MarkReceiving(ctx, tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkReceiving：%v", err)
	}
	if err := world.preAdvices.MarkClosed(ctx, tenant, id); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("MarkClosed：%v", err)
	}
	if _, err := world.numbers.IssuePreAdviceNumber(ctx, tenant, domain.BusinessTimeZone{}); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("IssuePreAdviceNumber：%v", err)
	}
}

func (world preAdviceWorld) accept(t *testing.T, command application.AcceptPreAdviceCommand) application.AcceptPreAdviceResult {
	t.Helper()
	var result application.AcceptPreAdviceResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var acceptErr error
		result, acceptErr = world.handler().Accept(ctx, command)
		return acceptErr
	})
	if err != nil {
		t.Fatalf("受理：%v", err)
	}
	return result
}

func (world preAdviceWorld) update(t *testing.T, command application.UpdatePreAdviceNoticeCommand) application.AcceptPreAdviceResult {
	t.Helper()
	var result application.AcceptPreAdviceResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var updateErr error
		result, updateErr = world.handler().UpdateNotice(ctx, command)
		return updateErr
	})
	if err != nil {
		t.Fatalf("告知：%v", err)
	}
	return result
}

func (world preAdviceWorld) registerNumberingTimeZone(t *testing.T) {
	t.Helper()
	var registration docnumber.TimeZoneRegistration
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		registration, registerErr = world.registrar.RegisterTimeZone(ctx, docnumber.RegisterTimeZoneCommand{
			Tenant: "SYN-TENANT-01", TimeZone: "Asia/Shanghai",
			EffectiveFrom: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), RegisteredBy: "SYN-TEST",
		})
		return registerErr
	})
	if err != nil || registration.Outcome != docnumber.VersionRegistered {
		t.Fatalf("登单据编号时区：%+v %v", registration, err)
	}
}

func stockLineCount(t *testing.T, world preAdviceWorld, tenant string) int {
	t.Helper()
	var count int
	err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.stock_movement_line WHERE tenant_id = $1`, tenant).Scan(&count)
	if err != nil {
		t.Fatalf("数库存账：%v", err)
	}
	return count
}

func preAdviceAcceptCommand(t *testing.T, key string, edit func(*application.AcceptPreAdviceCommand)) application.AcceptPreAdviceCommand {
	t.Helper()
	line, err := domain.NewGoodsInboundLine(
		valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), valueOf(t, domain.NewGoodsUnitName, "件"),
		2, domain.BatchNumber{}, domain.ExpiryDate{}, domain.CartonCount{},
	)
	if err != nil {
		t.Fatal(err)
	}
	command := application.AcceptPreAdviceCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Arrival: domain.ArrivalFullContainer, Inbound: domain.InboundByGoods,
		GoodsLines: []domain.GoodsInboundLine{line}, ExpectedAt: preAdviceNumberingInstant.Add(24 * time.Hour),
		Submission: preAdviceOwnerSubmission(t, key), Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
	if edit != nil {
		edit(&command)
	}
	return command
}

func preAdviceNoticeCommand(t *testing.T, id domain.PreAdviceID, key string, edit func(*application.UpdatePreAdviceNoticeCommand)) application.UpdatePreAdviceNoticeCommand {
	t.Helper()
	command := application.UpdatePreAdviceNoticeCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		ID: id, ExpectedAt: preAdviceNumberingInstant.Add(36 * time.Hour),
		Submission: preAdviceOwnerSubmission(t, key), Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
	if edit != nil {
		edit(&command)
	}
	return command
}

func preAdviceWithdrawCommand(t *testing.T, id domain.PreAdviceID, key string) application.WithdrawPreAdviceCommand {
	t.Helper()
	return application.WithdrawPreAdviceCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		ID: id, Submission: preAdviceOwnerSubmission(t, key), Registrant: valueOf(t, domain.NewRegistrant, "operator-1"),
	}
}

func preAdviceOwnerSubmission(t *testing.T, key string) domain.PreAdviceSubmission {
	t.Helper()
	submission, err := domain.NewOwnerPreAdviceSubmission(
		valueOf(t, domain.NewSubmissionSource, "SYN-CHANNEL"), valueOf(t, domain.NewRequestKey, key))
	if err != nil {
		t.Fatal(err)
	}
	return submission
}
