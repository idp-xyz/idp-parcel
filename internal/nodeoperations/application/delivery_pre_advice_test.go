package application_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"go.idp.xyz/idp-parcel/internal/nodeoperations/application"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/domain"
	"go.idp.xyz/idp-parcel/internal/nodeoperations/ports"
)

// 本文件证送站预报的提交与撤回编排（UC-NO-006 应用流程步骤 2；ADR-0212 决定二；票 ps-external-mark-relations/09 做什么「编排」）：
// 按来源请求键认重放与来源冲突，首次签发预报标识并落册；撤回只撤信封里那个客户账户的预报，别的账户的与查无此预报同答。夹具全为
// 合成（S 级）。

// preAdviceRegisterDouble 是预报册替身，照真库的代数答（ports.DeliveryPreAdviceRegister 各方法的注释）：同一（租户，来源，键）
// 已在册回显那一份；撤回只在预报属撤回方的客户账户时落，同键回显那一份撤回，否则这份预报已撤回时回显它的撤回。
type preAdviceRegisterDouble struct {
	preAdvices  []domain.DeliveryPreAdvice
	withdrawals []domain.DeliveryPreAdviceWithdrawal
	err         error
}

func (register *preAdviceRegisterDouble) RecordDeliveryPreAdvice(_ context.Context, preAdvice domain.DeliveryPreAdvice) (domain.DeliveryPreAdvice, ports.DeliveryScopeAppendOutcome, error) {
	if register.err != nil {
		return domain.DeliveryPreAdvice{}, ports.DeliveryScopeAppendInvalid, register.err
	}
	for _, recorded := range register.preAdvices {
		if recorded.TenantID() == preAdvice.TenantID() && recorded.Source().Channel == preAdvice.Source().Channel &&
			recorded.Source().RequestKey == preAdvice.Source().RequestKey {
			return recorded, ports.DeliveryScopeAlreadyRecorded, nil
		}
	}
	register.preAdvices = append(register.preAdvices, preAdvice)
	return preAdvice, ports.DeliveryScopeAppended, nil
}

func (register *preAdviceRegisterDouble) RecordDeliveryPreAdviceWithdrawal(_ context.Context, withdrawal domain.DeliveryPreAdviceWithdrawal) (domain.DeliveryPreAdviceWithdrawal, ports.DeliveryScopeAppendOutcome, error) {
	if register.err != nil {
		return domain.DeliveryPreAdviceWithdrawal{}, ports.DeliveryScopeAppendInvalid, register.err
	}
	for _, recorded := range register.withdrawals {
		if recorded.TenantID() == withdrawal.TenantID() && recorded.Source().Channel == withdrawal.Source().Channel &&
			recorded.Source().RequestKey == withdrawal.Source().RequestKey {
			return recorded, ports.DeliveryScopeAlreadyRecorded, nil
		}
	}
	owned := false
	for _, preAdvice := range register.preAdvices {
		if preAdvice.TenantID() == withdrawal.TenantID() && preAdvice.ID() == withdrawal.PreAdvice() && preAdvice.CustomerAccount() == withdrawal.CustomerAccount() {
			owned = true
		}
	}
	if !owned {
		return domain.DeliveryPreAdviceWithdrawal{}, ports.DeliveryScopeTargetNotFound, nil
	}
	for _, recorded := range register.withdrawals {
		if recorded.TenantID() == withdrawal.TenantID() && recorded.PreAdvice() == withdrawal.PreAdvice() {
			return recorded, ports.DeliveryScopeAlreadyRecorded, nil
		}
	}
	register.withdrawals = append(register.withdrawals, withdrawal)
	return withdrawal, ports.DeliveryScopeAppended, nil
}

func (register *preAdviceRegisterDouble) FindDeliveryPreAdvice(context.Context, domain.TenantID, domain.DeliveryPreAdviceID) (domain.DeliveryPreAdviceState, bool, error) {
	panic("the pre-advice commands do not read a pre-advice's state")
}

// preAdviceIDs 依次签出给定的标识，并记下签过几个。
type preAdviceIDs struct {
	values []string
	issued int
	err    error
}

func (ids *preAdviceIDs) NextDeliveryPreAdviceID(context.Context) (domain.DeliveryPreAdviceID, error) {
	if ids.err != nil {
		return domain.DeliveryPreAdviceID{}, ids.err
	}
	value := ids.values[ids.issued]
	ids.issued++
	return domain.NewDeliveryPreAdviceID(value)
}

var preAdviceNow = time.Date(2026, 10, 19, 6, 0, 0, 0, time.UTC)

const (
	preAdviceChannel = "https://auth.syn.example/customers#SYN-CUSTOMER-CLIENT-01"
	accountOne       = "SYN-ACCOUNT-01"
	accountTwo       = "SYN-ACCOUNT-02"
)

func mustNO[T any](t *testing.T, build func(string) (T, error), value string) T {
	t.Helper()
	built, err := build(value)
	if err != nil {
		t.Fatal(err)
	}
	return built
}

func submitCommand(t *testing.T, account, requestKey string, numbers ...string) application.SubmitDeliveryPreAdviceCommand {
	t.Helper()
	command := application.SubmitDeliveryPreAdviceCommand{
		TenantID:          mustNO(t, domain.NewTenantID, "SYN-TENANT-01"),
		CustomerAccount:   mustNO(t, domain.NewCustomerAccountReference, account),
		Channel:           preAdviceChannel,
		RequestKey:        requestKey,
		Node:              mustNO(t, domain.NewNodeReference, "SYN-NODE-SHA-HUB"),
		ExpectedArrivalAt: time.Date(2026, 10, 20, 2, 0, 0, 0, time.UTC),
	}
	for _, number := range numbers {
		command.Numbers = append(command.Numbers, mustNO(t, domain.NewPreAdvisedParcelNumber, number))
	}
	return command
}

func withdrawCommand(t *testing.T, account, requestKey, preAdvice string) application.WithdrawDeliveryPreAdviceCommand {
	t.Helper()
	return application.WithdrawDeliveryPreAdviceCommand{
		TenantID:        mustNO(t, domain.NewTenantID, "SYN-TENANT-01"),
		CustomerAccount: mustNO(t, domain.NewCustomerAccountReference, account),
		Channel:         preAdviceChannel,
		RequestKey:      requestKey,
		PreAdvice:       mustNO(t, domain.NewDeliveryPreAdviceID, preAdvice),
	}
}

func preAdviceHandler(t *testing.T, register ports.DeliveryPreAdviceRegister, ids ports.DeliveryPreAdviceIdentityFactory) *application.DeliveryPreAdviceHandler {
	t.Helper()
	handler, err := application.NewDeliveryPreAdviceHandler(application.DeliveryPreAdviceDeps{
		Register: register,
		IDs:      ids,
		Clock:    fixedClock{at: preAdviceNow},
	})
	if err != nil {
		t.Fatal(err)
	}
	return handler
}

// Covers: AT-NO-069（编排半边）——首次交来答送站预报已登记：标识由本上下文签发，形成时刻取时钟，来源是（客户端，来源请求键，NOC-1
// 内容摘要）；同一接入请求原样重投答已有结果、回显第一次那一份与它的标识，册上不多一行；同一接入请求换了内容答来源冲突，回显在册
// 那一份。
func TestSubmittingAPreAdviceRegistersReplaysAndConflicts(t *testing.T) {
	register := &preAdviceRegisterDouble{}
	ids := &preAdviceIDs{values: []string{"PREADV-SYN-0001", "PREADV-SYN-0002", "PREADV-SYN-0003", "PREADV-SYN-0004"}}
	handler := preAdviceHandler(t, register, ids)
	ctx := context.Background()

	first, err := handler.Submit(ctx, submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0002", "SYN-PCL-PA-0001"))
	if err != nil || first.Outcome() != application.DeliveryPreAdviceRegistered {
		t.Fatalf("首次 = %s, %v", first.Outcome(), err)
	}
	registered := first.PreAdvice()
	if registered.ID().String() != "PREADV-SYN-0001" || !registered.FormedAt().Equal(preAdviceNow) {
		t.Fatalf("签发 %q 于 %v", registered.ID(), registered.FormedAt())
	}
	source := registered.Source()
	_, digest, err := domain.CanonicalizeDeliveryPreAdviceSubmissionPayload(registered.Node(), registered.ExpectedArrivalAt(), registered.Numbers(), domain.DeliveringPartyReference{})
	if err != nil {
		t.Fatal(err)
	}
	if source.Channel != preAdviceChannel || source.RequestKey != "SYN-ACCESS-REQUEST-PA-0001" || source.ContentDigest != digest {
		t.Fatalf("来源 = %+v, want 客户端、来源请求键与内容摘要 %s", source, digest)
	}

	replay, err := handler.Submit(ctx, submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001", "SYN-PCL-PA-0002"))
	if err != nil || replay.Outcome() != application.DeliveryPreAdviceExistingResult || replay.PreAdvice().ID() != registered.ID() {
		t.Fatalf("原样重投（号码换序）= %s %q, %v", replay.Outcome(), replay.PreAdvice().ID(), err)
	}

	conflict, err := handler.Submit(ctx, submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001"))
	if err != nil || conflict.Outcome() != application.DeliveryPreAdviceSourceConflict || conflict.PreAdvice().ID() != registered.ID() {
		t.Fatalf("换内容 = %s %q, %v", conflict.Outcome(), conflict.PreAdvice().ID(), err)
	}
	if len(register.preAdvices) != 1 {
		t.Fatalf("册上 %d 份，want 只有第一份", len(register.preAdvices))
	}

	another, err := handler.Submit(ctx, submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0002", "SYN-PCL-PA-0001"))
	if err != nil || another.Outcome() != application.DeliveryPreAdviceRegistered || another.PreAdvice().ID() == registered.ID() {
		t.Fatalf("另一接入请求 = %s %q, %v", another.Outcome(), another.PreAdvice().ID(), err)
	}
	for _, outcome := range []application.DeliveryPreAdviceOutcome{
		application.DeliveryPreAdviceRegistered, application.DeliveryPreAdviceWithdrawn,
		application.DeliveryPreAdviceExistingResult, application.DeliveryPreAdviceSourceConflict,
	} {
		if outcome.String() == "" {
			t.Fatalf("outcome %d 没有线上名", outcome)
		}
	}
}

// Covers: AT-NO-069「号码为空或重复……答坏载荷」的编排半边——立不起来的预报答被拒，册一次也不被问到；册读写不动、签不出标识原样
// 交回，不形成答案。
func TestAPreAdviceThatCannotFormNeverReachesTheRegister(t *testing.T) {
	register := &preAdviceRegisterDouble{err: errors.New("the register must not be asked")}
	handler := preAdviceHandler(t, register, &preAdviceIDs{values: []string{"PREADV-SYN-0001", "PREADV-SYN-0002"}})
	for name, command := range map[string]application.SubmitDeliveryPreAdviceCommand{
		"no numbers":        submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001"),
		"repeated number":   submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001", "SYN-PCL-PA-0001"),
		"blank request key": submitCommand(t, accountOne, " ", "SYN-PCL-PA-0001"),
	} {
		if _, err := handler.Submit(context.Background(), command); !errors.Is(err, application.ErrDeliveryPreAdviceRejected) {
			t.Fatalf("%s：err = %v, want ErrDeliveryPreAdviceRejected", name, err)
		}
	}

	broken := errors.New("connection refused")
	if _, err := preAdviceHandler(t, &preAdviceRegisterDouble{err: broken}, &preAdviceIDs{values: []string{"PREADV-SYN-0001"}}).
		Submit(context.Background(), submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001")); !errors.Is(err, broken) ||
		errors.Is(err, application.ErrDeliveryPreAdviceRejected) {
		t.Fatalf("册读写不动：err = %v", err)
	}
	unissued := errors.New("entropy exhausted")
	if _, err := preAdviceHandler(t, &preAdviceRegisterDouble{}, &preAdviceIDs{err: unissued}).
		Submit(context.Background(), submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001")); !errors.Is(err, unissued) {
		t.Fatalf("签不出标识：err = %v", err)
	}
}

// Covers: AT-NO-070（撤回的命令半边）——撤自己的预报落撤回、撤回时刻取时钟；同一接入请求原样重投答已有结果，同一接入请求改指别的
// 预报答来源冲突；这份预报已被另一接入请求撤回时答已有结果、回显在册那一份撤回（撤回是终态）；撤别的客户账户的预报与查无此预报
// 同答，册上不多一行。
func TestWithdrawingAPreAdviceTouchesOnlyTheEnvelopesOwnAccount(t *testing.T) {
	register := &preAdviceRegisterDouble{}
	handler := preAdviceHandler(t, register, &preAdviceIDs{values: []string{"PREADV-SYN-0001", "PREADV-SYN-0002"}})
	ctx := context.Background()
	if _, err := handler.Submit(ctx, submitCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0001", "SYN-PCL-PA-0001")); err != nil {
		t.Fatal(err)
	}
	if _, err := handler.Submit(ctx, submitCommand(t, accountTwo, "SYN-ACCESS-REQUEST-PA-0002", "SYN-PCL-PA-0002")); err != nil {
		t.Fatal(err)
	}

	for name, command := range map[string]application.WithdrawDeliveryPreAdviceCommand{
		"another account's pre-advice": withdrawCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0010", "PREADV-SYN-0002"),
		"no such pre-advice":           withdrawCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0011", "PREADV-SYN-9999"),
	} {
		_, err := handler.Withdraw(ctx, command)
		if !errors.Is(err, application.ErrDeliveryPreAdviceNotFound) {
			t.Fatalf("%s：err = %v, want ErrDeliveryPreAdviceNotFound", name, err)
		}
	}
	if len(register.withdrawals) != 0 {
		t.Fatalf("册上落了 %d 份撤回", len(register.withdrawals))
	}

	withdrawn, err := handler.Withdraw(ctx, withdrawCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0003", "PREADV-SYN-0001"))
	if err != nil || withdrawn.Outcome() != application.DeliveryPreAdviceWithdrawn {
		t.Fatalf("撤自己的 = %s, %v", withdrawn.Outcome(), err)
	}
	recorded := withdrawn.Withdrawal()
	if recorded.PreAdvice().String() != "PREADV-SYN-0001" || !recorded.WithdrawnAt().Equal(preAdviceNow) ||
		recorded.Source().RequestKey != "SYN-ACCESS-REQUEST-PA-0003" || recorded.Source().ContentDigest == "" {
		t.Fatalf("撤回 = %+v", recorded)
	}

	for name, testCase := range map[string]struct {
		command application.WithdrawDeliveryPreAdviceCommand
		want    application.DeliveryPreAdviceOutcome
	}{
		"same request replayed":              {withdrawCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0003", "PREADV-SYN-0001"), application.DeliveryPreAdviceExistingResult},
		"same request names another":         {withdrawCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0003", "PREADV-SYN-0009"), application.DeliveryPreAdviceSourceConflict},
		"another request, already withdrawn": {withdrawCommand(t, accountOne, "SYN-ACCESS-REQUEST-PA-0004", "PREADV-SYN-0001"), application.DeliveryPreAdviceExistingResult},
	} {
		result, err := handler.Withdraw(ctx, testCase.command)
		if err != nil || result.Outcome() != testCase.want {
			t.Fatalf("%s = %s, %v; want %s", name, result.Outcome(), err, testCase.want)
		}
		if result.Withdrawal().Source().RequestKey != "SYN-ACCESS-REQUEST-PA-0003" {
			t.Fatalf("%s：回显的不是在册那一份撤回：%+v", name, result.Withdrawal())
		}
	}
	if len(register.withdrawals) != 1 {
		t.Fatalf("册上 %d 份撤回，want 一份", len(register.withdrawals))
	}

	if _, err := handler.Withdraw(ctx, withdrawCommand(t, accountOne, " ", "PREADV-SYN-0001")); !errors.Is(err, application.ErrDeliveryPreAdviceRejected) {
		t.Fatalf("空白来源请求键：err = %v", err)
	}
}

func TestDeliveryPreAdviceHandlerRefusesMissingDependencies(t *testing.T) {
	for name, deps := range map[string]application.DeliveryPreAdviceDeps{
		"register": {IDs: &preAdviceIDs{}, Clock: fixedClock{at: preAdviceNow}},
		"ids":      {Register: &preAdviceRegisterDouble{}, Clock: fixedClock{at: preAdviceNow}},
		"clock":    {Register: &preAdviceRegisterDouble{}, IDs: &preAdviceIDs{}},
	} {
		if _, err := application.NewDeliveryPreAdviceHandler(deps); err == nil {
			t.Fatalf("缺 %s 也建起来了", name)
		}
	}
}
