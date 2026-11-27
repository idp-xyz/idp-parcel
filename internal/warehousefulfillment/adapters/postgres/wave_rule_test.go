package postgres_test

import (
	"context"
	"errors"
	"testing"

	bentopg "go.idp.xyz/idp-bento-go/postgres"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func waveRuleCommand(t *testing.T, code string, revision int, change func(*application.RegisterWaveRuleCommand)) application.RegisterWaveRuleCommand {
	t.Helper()
	command := application.RegisterWaveRuleCommand{
		Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
		Code: valueOf(t, domain.NewWaveRuleCode, code), Revision: revision, Name: "合成规则 " + code,
		Profile: ports.WaveRuleProfileAny, MaxRequests: 2, Operator: valueOf(t, domain.NewRegistrant, "operator-2"),
	}
	if change != nil {
		change(&command)
	}
	return command
}

func (world receiptWorld) registerWaveRule(t *testing.T, handler *application.WaveHandler, command application.RegisterWaveRuleCommand) application.WaveOutcome {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var registerErr error
		result, registerErr = handler.RegisterWaveRule(ctx, command)
		return registerErr
	})
	if err != nil {
		t.Fatalf("登记成波规则 %s：%v", command.Code, err)
	}
	return result.Outcome()
}

func (world receiptWorld) runWaveRule(t *testing.T, handler *application.WaveHandler, rule, run string) application.WaveRuleRunResult {
	t.Helper()
	var result application.WaveRuleRunResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var runErr error
		result, runErr = handler.RunWaveRule(ctx, application.RunWaveRuleCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"),
			Rule: valueOf(t, domain.NewWaveRuleCode, rule), RunID: valueOf(t, domain.NewWaveID, run),
			Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
		})
		return runErr
	})
	if err != nil {
		t.Fatalf("按规则 %s 成波 %s：%v", rule, run, err)
	}
	return result
}

// Covers: 成波规则按仓登记，修订从 1 起连续：同一版同样内容重放答已有结果，同号不同内容与跳号是内容冲突。
// 按规则成波把圈到的已预占请求照受理先后、每满规则登的张数切成一个波次放出，波次上记下规则与合并拣货；
// 同一个执行号重放答已有结果并带着当时的波次，换一条规则重放是冲突；没有可放的什么都不记；
// 没登记的规则答查无此规则，停用的规则不能再用。设置总览列出各条规则最新的一版。
func TestAWaveRuleReleasesWhatItCirclesInWavesOfItsSize(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	batch := func(command *application.RegisterWaveRuleCommand) { command.BatchPick = true }
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-A", 1, batch)); got != application.WaveStrategyRegistered {
		t.Fatalf("登记第一版 = %s", got)
	}
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-A", 1, batch)); got != application.WaveExisting {
		t.Fatalf("同一版重放 = %s", got)
	}
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-A", 1, nil)); got != application.WaveConflict {
		t.Fatalf("同号不同内容 = %s", got)
	}
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-A", 3, batch)); got != application.WaveConflict {
		t.Fatalf("跳号 = %s", got)
	}
	for name, change := range map[string]func(*application.RegisterWaveRuleCommand){
		"一个波次零张":  func(command *application.RegisterWaveRuleCommand) { command.MaxRequests = 0 },
		"一个波次太大":  func(command *application.RegisterWaveRuleCommand) { command.MaxRequests = 100000 },
		"不认识的件型":  func(command *application.RegisterWaveRuleCommand) { command.Profile = "LARGE" },
		"名字是空白":   func(command *application.RegisterWaveRuleCommand) { command.Name = "  " },
		"修订不是正整数": func(command *application.RegisterWaveRuleCommand) { command.Revision = 0 },
	} {
		if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-BAD", 1, change)); got != application.WaveContentInvalid {
			t.Fatalf("%s = %s", name, got)
		}
	}
	elsewhere := waveRuleCommand(t, "SYN-RULE-ELSEWHERE", 1, func(command *application.RegisterWaveRuleCommand) {
		command.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-NOT-THERE")
	})
	if got := world.registerWaveRule(t, handler, elsewhere); got != application.WaveWarehouseNotFound {
		t.Fatalf("没登记的仓 = %s", got)
	}

	var requests []domain.OutboundRequestID
	for _, key := range []string{"REQ-RULE-1", "REQ-RULE-2", "REQ-RULE-3", "REQ-RULE-4", "REQ-RULE-5"} {
		requests = append(requests, reserve(key, 2))
	}
	if got := world.runWaveRule(t, handler, "SYN-RULE-NOT-THERE", "SYN-RUN-0"); got.Outcome() != application.WaveRuleNotFound {
		t.Fatalf("没登记的规则 = %s", got.Outcome())
	}
	ran := world.runWaveRule(t, handler, "SYN-RULE-A", "SYN-RUN-1")
	if ran.Outcome() != application.WaveAllocated || ran.Released() != 5 || ran.Skipped() != 0 || ran.Remaining() != 0 || ran.Revision() != 1 {
		t.Fatalf("按规则成波 %s：放 %d 跳 %d 余 %d", ran.Outcome(), ran.Released(), ran.Skipped(), ran.Remaining())
	}
	waves := ran.Waves()
	if len(waves) != 3 || waves[0] != (ports.RuleWave{Wave: "SYN-RUN-1-01", Requests: 2}) ||
		waves[1] != (ports.RuleWave{Wave: "SYN-RUN-1-02", Requests: 2}) || waves[2] != (ports.RuleWave{Wave: "SYN-RUN-1-03", Requests: 1}) {
		t.Fatalf("放出的波次 %+v", waves)
	}
	for _, wave := range waves {
		row := world.waveRow(t, wave.Wave)
		if row.RuleCode != "SYN-RULE-A" || !row.BatchPick || row.Status != ports.WaveReleased || len(row.Requests) != wave.Requests {
			t.Fatalf("波次 %s：%+v", wave.Wave, row)
		}
	}
	for _, request := range requests {
		if world.requestStatus(t, request) != "ALLOCATED" {
			t.Fatalf("请求 %s 是 %s", request, world.requestStatus(t, request))
		}
	}
	again := world.runWaveRule(t, handler, "SYN-RULE-A", "SYN-RUN-1")
	if again.Outcome() != application.WaveExisting || again.Released() != 5 || len(again.Waves()) != 3 || again.Waves()[2].Requests != 1 {
		t.Fatalf("同一个执行号重放 %s：%+v", again.Outcome(), again.Waves())
	}
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-B", 1, nil)); got != application.WaveStrategyRegistered {
		t.Fatalf("登记另一条规则 = %s", got)
	}
	if got := world.runWaveRule(t, handler, "SYN-RULE-B", "SYN-RUN-1"); got.Outcome() != application.WaveConflict {
		t.Fatalf("同一个执行号换一条规则 = %s", got.Outcome())
	}
	nothing := world.runWaveRule(t, handler, "SYN-RULE-A", "SYN-RUN-2")
	if nothing.Outcome() != application.WaveNothingToRelease || nothing.Released() != 0 || len(nothing.Waves()) != 0 {
		t.Fatalf("没有可放的 %s：%+v", nothing.Outcome(), nothing.Waves())
	}
	// 没放出波次的那一次什么都没记：同一个执行号还能用。
	waveStore, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	if _, found, err := waveStore.LoadWaveRuleRun(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), "SYN-RUN-2"); err != nil || found {
		t.Fatalf("没放出波次的执行记下了：found %v err %v", found, err)
	}
	// 由人点单放的波次与规则的执行号撞了：整次不成立，一个波次都不放。
	// 从规则放出的波次里撤出两张：一张由人放进一个号正好撞上的波次，另一张留在已预占、等规则来圈。
	manual := valueOf(t, domain.NewOutboundRequestID, world.waveRow(t, "SYN-RUN-1-03").Requests[0])
	waiting := valueOf(t, domain.NewOutboundRequestID, world.waveRow(t, "SYN-RUN-1-02").Requests[0])
	if world.withdraw(t, handler, "SYN-RUN-1-03", manual).Outcome() != application.WaveWithdrawn ||
		world.withdraw(t, handler, "SYN-RUN-1-02", waiting).Outcome() != application.WaveWithdrawn ||
		world.releaseRequests(t, handler, "SYN-RUN-3-01", manual).Outcome() != application.WaveAllocated {
		t.Fatal("没能备出一个撞号的波次")
	}
	if got := world.runWaveRule(t, handler, "SYN-RULE-A", "SYN-RUN-3"); got.Outcome() != application.WaveConflict {
		t.Fatalf("波次号撞了 = %s", got.Outcome())
	}
	if world.requestStatus(t, waiting) != "RESERVED" {
		t.Fatalf("撞号之后等着的那张是 %s", world.requestStatus(t, waiting))
	}
	retired := waveRuleCommand(t, "SYN-RULE-A", 2, func(command *application.RegisterWaveRuleCommand) {
		command.BatchPick, command.Deactivated = true, true
	})
	if got := world.registerWaveRule(t, handler, retired); got != application.WaveStrategyRegistered {
		t.Fatalf("停用 = %s", got)
	}
	if got := world.runWaveRule(t, handler, "SYN-RULE-A", "SYN-RUN-4"); got.Outcome() != application.WaveRuleDeactivated {
		t.Fatalf("停用的规则 = %s", got.Outcome())
	}
	reads, err := adapter.NewWorkspaceReads(world.db)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := reads.SettingsOverview(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"))
	if err != nil || len(overview.WaveRules) != 2 || overview.WaveRules[0].RuleCode != "SYN-RULE-A" || overview.WaveRules[0].Revision != 2 ||
		!overview.WaveRules[0].Deactivated || !overview.WaveRules[0].BatchPick || overview.WaveRules[1].RuleCode != "SYN-RULE-B" ||
		overview.WaveRules[1].MaxRequests != 2 || overview.WaveRules[1].Profile != ports.WaveRuleProfileAny {
		t.Fatalf("设置总览里的成波规则 %+v：%v", overview.WaveRules, err)
	}
}

// Covers: 规则按件型、货主、发运方式圈单，不带的条件不限：只圈单品单件的规则不碰两件的请求，反过来也一样；
// 点了别的货主或别的发运方式的规则圈不到。
func TestAWaveRuleOnlyCirclesWhatItsConditionsName(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	single := func(command *application.RegisterWaveRuleCommand) {
		command.Profile, command.MaxRequests = ports.WaveRuleProfileSingleUnit, 10
	}
	multi := func(command *application.RegisterWaveRuleCommand) { command.Profile = ports.WaveRuleProfileMultiUnit }
	other := func(command *application.RegisterWaveRuleCommand) { command.Owner = "SYN-OWNER-NOT-THERE" }
	pickup := func(command *application.RegisterWaveRuleCommand) { command.Dispatch = domain.DispatchPickup.String() }
	for code, change := range map[string]func(*application.RegisterWaveRuleCommand){
		"SYN-RULE-SINGLE": single, "SYN-RULE-MULTI": multi, "SYN-RULE-OTHER-OWNER": other, "SYN-RULE-PICKUP": pickup,
	} {
		if got := world.registerWaveRule(t, handler, waveRuleCommand(t, code, 1, change)); got != application.WaveStrategyRegistered {
			t.Fatalf("登记 %s = %s", code, got)
		}
	}
	one, two, three := reserve("REQ-CIRCLE-1", 1), reserve("REQ-CIRCLE-2", 2), reserve("REQ-CIRCLE-3", 1)
	for _, rule := range []string{"SYN-RULE-OTHER-OWNER", "SYN-RULE-PICKUP"} {
		if got := world.runWaveRule(t, handler, rule, "SYN-RUN-"+rule); got.Outcome() != application.WaveNothingToRelease {
			t.Fatalf("%s = %s", rule, got.Outcome())
		}
	}
	singles := world.runWaveRule(t, handler, "SYN-RULE-SINGLE", "SYN-RUN-SINGLE")
	if singles.Outcome() != application.WaveAllocated || singles.Released() != 2 || len(singles.Waves()) != 1 {
		t.Fatalf("只圈单品单件 %s：%+v", singles.Outcome(), singles.Waves())
	}
	if row := world.waveRow(t, "SYN-RUN-SINGLE-01"); !row.SingleUnit || row.RuleCode != "SYN-RULE-SINGLE" {
		t.Fatalf("单品单件规则放出的波次 %+v", row)
	}
	if world.requestStatus(t, one) != "ALLOCATED" || world.requestStatus(t, three) != "ALLOCATED" || world.requestStatus(t, two) != "RESERVED" {
		t.Fatalf("圈单之后 %s / %s / %s", world.requestStatus(t, one), world.requestStatus(t, two), world.requestStatus(t, three))
	}
	multis := world.runWaveRule(t, handler, "SYN-RULE-MULTI", "SYN-RUN-MULTI")
	if multis.Outcome() != application.WaveAllocated || multis.Released() != 1 || world.requestStatus(t, two) != "ALLOCATED" {
		t.Fatalf("只圈不是单品单件的 %s：放 %d", multis.Outcome(), multis.Released())
	}
}

// Covers: 圈到的请求里有分配不出的，跳过它、留在已预占，后面分配得出的照放；跳过的那一张试分配时记了一半的占用不算数。
// 圈到的全都分配不出时一个波次也不放，答没有可放的并带出跳过了几张。
func TestAWaveRuleSkipsARequestThatCannotBeAllocated(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-SKIP", 1, nil)); got != application.WaveStrategyRegistered {
		t.Fatalf("登记 = %s", got)
	}
	// 六件那一格少拣之后在盘点盘过之前不再分配；差的三件换到四件那一格补，那一格只剩一件可分。
	blocker := reserve("REQ-SKIP-BLOCKER", 4)
	released := world.releaseRequests(t, handler, "WAVE-SKIP-BLOCKER", blocker)
	if released.Outcome() != application.WaveAllocated || len(released.Tasks()) != 1 {
		t.Fatalf("放出 %s，任务 %+v", released.Outcome(), released.Tasks())
	}
	if world.pick(t, handler, released.Tasks()[0].ID, 1).Outcome() != application.WavePicked {
		t.Fatal("少拣没有记成")
	}
	stuck, fits := reserve("REQ-SKIP-A-STUCK", 2), reserve("REQ-SKIP-B-FITS", 1)
	ran := world.runWaveRule(t, handler, "SYN-RULE-SKIP", "SYN-RUN-SKIP")
	if ran.Outcome() != application.WaveAllocated || ran.Released() != 1 || ran.Skipped() != 1 || len(ran.Waves()) != 1 {
		t.Fatalf("跳过分配不出的 %s：放 %d 跳 %d，波次 %+v", ran.Outcome(), ran.Released(), ran.Skipped(), ran.Waves())
	}
	if world.requestStatus(t, stuck) != "RESERVED" || world.requestStatus(t, fits) != "ALLOCATED" {
		t.Fatalf("跳过之后 %s / %s", world.requestStatus(t, stuck), world.requestStatus(t, fits))
	}
	if row := world.waveRow(t, "SYN-RUN-SKIP-01"); len(row.Requests) != 1 || row.Requests[0] != fits.String() || len(row.Tasks) != 1 || row.Tasks[0].Quantity != 1 {
		t.Fatalf("放出的波次 %+v", row)
	}
	// 跳过的那一张记成放不出的例外，出库请求列表按它筛得出来，写明是哪一次执行跳过的。
	listed := world.listedOutbound(t, "allocationException=true")
	if exception := listed[stuck.String()].AllocationException; len(listed) != 1 || exception == nil ||
		exception.Source != ports.AllocationExceptionFromRule || exception.Reference != "SYN-RUN-SKIP" {
		t.Fatalf("放不出的例外 %+v", listed)
	}
	none := world.runWaveRule(t, handler, "SYN-RULE-SKIP", "SYN-RUN-SKIP-2")
	if none.Outcome() != application.WaveNothingToRelease || none.Skipped() != 1 || none.Released() != 0 {
		t.Fatalf("全都分配不出 %s：跳 %d", none.Outcome(), none.Skipped())
	}
	if again := world.listedOutbound(t, "allocationException=true")[stuck.String()].AllocationException; again == nil || again.Reference != "SYN-RUN-SKIP-2" {
		t.Fatalf("再跳过一次之后的例外 %+v", again)
	}
}

// Covers: 规则登了一个波次最多几件、最多几行时，加上下一张会超就先把手上的波次放出去、另起一个；
// 一张请求自己就超了的单独成一个波次。负数的上限不收。
func TestAWaveRuleCutsWavesByUnitsAndLines(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	byUnits := func(command *application.RegisterWaveRuleCommand) { command.MaxRequests, command.MaxUnits = 10, 3 }
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-UNITS", 1, byUnits)); got != application.WaveStrategyRegistered {
		t.Fatalf("登记按件数切的规则 = %s", got)
	}
	negative := func(command *application.RegisterWaveRuleCommand) { command.MaxLines = -1 }
	if got := world.registerWaveRule(t, handler, waveRuleCommand(t, "SYN-RULE-NEGATIVE", 1, negative)); got != application.WaveContentInvalid {
		t.Fatalf("负数的行数上限 = %s", got)
	}
	reserve("REQ-CAP-1", 2)
	reserve("REQ-CAP-2", 2)
	reserve("REQ-CAP-3", 1)
	reserve("REQ-CAP-4", 4)
	ran := world.runWaveRule(t, handler, "SYN-RULE-UNITS", "SYN-RUN-UNITS")
	// 四张同一刻受理，先后按请求号定，这里不认次序，只认：九件、上限三件、有一张四件的，至少要三个波次；
	// 不止一张请求的波次不超过三件；超过三件的波次只有那一张自己就超了的请求。
	if ran.Outcome() != application.WaveAllocated || ran.Released() != 4 || len(ran.Waves()) < 3 {
		t.Fatalf("按件数切 %s：放 %d，波次 %+v", ran.Outcome(), ran.Released(), ran.Waves())
	}
	alone := 0
	for _, wave := range ran.Waves() {
		row := world.waveRow(t, wave.Wave)
		var units int64
		for _, task := range row.Tasks {
			units += task.Quantity
		}
		if units > 3 {
			alone++
			if len(row.Requests) != 1 || units != 4 {
				t.Fatalf("波次 %s 有 %d 张请求、%d 件，超了上限", wave.Wave, len(row.Requests), units)
			}
		}
	}
	if alone != 1 {
		t.Fatalf("自己就超了的那一张该单独成一个波次，实际有 %d 个超上限的波次", alone)
	}

	other, otherHandler, otherReserve := twoCellWorld(t)
	byLines := func(command *application.RegisterWaveRuleCommand) { command.MaxRequests, command.MaxLines = 10, 2 }
	if got := other.registerWaveRule(t, otherHandler, waveRuleCommand(t, "SYN-RULE-LINES", 1, byLines)); got != application.WaveStrategyRegistered {
		t.Fatalf("登记按行数切的规则 = %s", got)
	}
	for _, key := range []string{"REQ-LINE-1", "REQ-LINE-2", "REQ-LINE-3"} {
		otherReserve(key, 1)
	}
	lined := other.runWaveRule(t, otherHandler, "SYN-RULE-LINES", "SYN-RUN-LINES")
	if lined.Outcome() != application.WaveAllocated || len(lined.Waves()) != 2 || lined.Waves()[0].Requests != 2 || lined.Waves()[1].Requests != 1 {
		t.Fatalf("按行数切 %s：%+v", lined.Outcome(), lined.Waves())
	}
	reads, err := adapter.NewWorkspaceReads(other.db)
	if err != nil {
		t.Fatal(err)
	}
	overview, err := reads.SettingsOverview(t.Context(), valueOf(t, domain.NewTenantID, "SYN-TENANT-01"))
	if err != nil || len(overview.WaveRules) != 1 || overview.WaveRules[0].MaxLines != 2 || overview.WaveRules[0].MaxUnits != 0 {
		t.Fatalf("设置总览里的容量 %+v：%v", overview.WaveRules, err)
	}
}

func (world receiptWorld) releasePartial(t *testing.T, handler *application.WaveHandler, id string, requests ...domain.OutboundRequestID) application.WaveResult {
	t.Helper()
	var result application.WaveResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var releaseErr error
		result, releaseErr = handler.Release(ctx, application.ReleaseWaveCommand{
			Tenant: valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ID: valueOf(t, domain.NewWaveID, id),
			Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-01"), Operator: valueOf(t, domain.NewRegistrant, "operator-1"),
			Requests: requests, Partial: true,
		})
		return releaseErr
	})
	if err != nil {
		t.Fatalf("放行能放的 %s：%v", id, err)
	}
	return result
}

// Covers: 放波次不选「放行能放的」时有一张不行整个波次不放，什么都不记。选了，分配不出、不是已预占、找不到的各自
// 带着缘由跳过，其余的放进波次；分配不出的那一张记成放不出的例外，列表筛得出来。同一份重放答已有结果并带着同样的例外，
// 换一批单用同一个波次号是冲突。点名的全都不行时一个波次也不放。
func TestAPartialReleaseLetsThroughWhatCanBeAllocatedAndNamesTheRest(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	blocker := reserve("REQ-PART-BLOCKER", 4)
	released := world.releaseRequests(t, handler, "WAVE-PART-BLOCKER", blocker)
	if released.Outcome() != application.WaveAllocated || world.pick(t, handler, released.Tasks()[0].ID, 1).Outcome() != application.WavePicked {
		t.Fatal("没能备出一格少拣的库位")
	}
	stuck, fits := reserve("REQ-PART-A-STUCK", 2), reserve("REQ-PART-B-FITS", 1)
	ghost := valueOf(t, domain.NewOutboundRequestID, "OBR-NOT-THERE")
	if got := world.releaseRequests(t, handler, "WAVE-PART-ALL", stuck, fits).Outcome(); got != application.WaveContentInvalid {
		t.Fatalf("不选放行能放的 = %s", got)
	}
	if len(world.listedOutbound(t, "allocationException=true")) != 0 || world.requestStatus(t, fits) != "RESERVED" {
		t.Fatal("整个不放的那一次记下了东西")
	}
	reasons := func(result application.WaveResult) map[string]string {
		named := map[string]string{}
		for _, exception := range result.Exceptions() {
			named[exception.Request.String()] = exception.Reason
		}
		return named
	}
	want := map[string]string{
		stuck.String(): application.ReleaseNotAllocatable, blocker.String(): application.ReleaseNotReserved, ghost.String(): application.ReleaseNotFound,
	}
	partial := world.releasePartial(t, handler, "WAVE-PART", stuck, fits, blocker, ghost)
	if got := reasons(partial); partial.Outcome() != application.WaveAllocated || len(partial.Tasks()) != 1 || len(got) != 3 ||
		got[stuck.String()] != want[stuck.String()] || got[blocker.String()] != want[blocker.String()] || got[ghost.String()] != want[ghost.String()] {
		t.Fatalf("放行能放的 %s：任务 %d 条，例外 %+v", partial.Outcome(), len(partial.Tasks()), got)
	}
	if row := world.waveRow(t, "WAVE-PART"); len(row.Requests) != 1 || row.Requests[0] != fits.String() {
		t.Fatalf("放出的波次 %+v", row.Requests)
	}
	if world.requestStatus(t, stuck) != "RESERVED" || world.requestStatus(t, fits) != "ALLOCATED" {
		t.Fatalf("放行之后 %s / %s", world.requestStatus(t, stuck), world.requestStatus(t, fits))
	}
	listed := world.listedOutbound(t, "allocationException=true")
	if exception := listed[stuck.String()].AllocationException; len(listed) != 1 || exception == nil ||
		exception.Source != ports.AllocationExceptionFromWave || exception.Reference != "WAVE-PART" {
		t.Fatalf("放不出的例外 %+v", listed)
	}
	if others := world.listedOutbound(t, "allocationException=false"); others[stuck.String()].RequestID != "" || others[fits.String()].RequestID == "" {
		t.Fatalf("不是例外的那些 %+v", others)
	}
	again := world.releasePartial(t, handler, "WAVE-PART", stuck, fits, blocker, ghost)
	if got := reasons(again); again.Outcome() != application.WaveExisting || len(got) != 3 || got[stuck.String()] != want[stuck.String()] ||
		got[blocker.String()] != want[blocker.String()] || got[ghost.String()] != want[ghost.String()] {
		t.Fatalf("同一份重放 %s：%+v", again.Outcome(), got)
	}
	if got := world.releasePartial(t, handler, "WAVE-PART", stuck).Outcome(); got != application.WaveConflict {
		t.Fatalf("同一个波次号换一批单 = %s", got)
	}
	nothing := world.releasePartial(t, handler, "WAVE-PART-NONE", stuck, ghost)
	if nothing.Outcome() != application.WaveNothingToRelease || len(nothing.Exceptions()) != 2 {
		t.Fatalf("点名的全都不行 %s：%+v", nothing.Outcome(), nothing.Exceptions())
	}
}

// Covers: 记着放不出的例外的请求之后放进了波次，那一笔例外就了结了，列表不再把它筛成放不出。
func TestAnAllocationExceptionEndsWhenTheRequestIsReleased(t *testing.T) {
	world, handler, reserve := twoCellWorld(t)
	request := reserve("REQ-EXCEPTION-CLEARED", 1)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant := valueOf(t, domain.NewTenantID, "SYN-TENANT-01")
	if err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		return waves.SaveAllocationException(ctx, tenant, request, ports.AllocationExceptionFromWave, "WAVE-EARLIER", preAdviceNumberingInstant)
	}); err != nil {
		t.Fatal(err)
	}
	if listed := world.listedOutbound(t, "allocationException=true"); len(listed) != 1 || listed[request.String()].AllocationException == nil {
		t.Fatalf("记下的例外 %+v", listed)
	}
	if world.releaseRequests(t, handler, "WAVE-EXCEPTION-CLEARED", request).Outcome() != application.WaveAllocated {
		t.Fatal("没有放出")
	}
	has, err := waves.HasAllocationException(t.Context(), tenant, request)
	if err != nil || has || len(world.listedOutbound(t, "allocationException=true")) != 0 {
		t.Fatalf("放进波次之后例外还在：%v %v", has, err)
	}
}

func TestWritingWaveRulesRefusesToRunOutsideATransaction(t *testing.T) {
	world := newReceiptWorld(t)
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	tenant, warehouse := valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-01")
	rule := valueOf(t, domain.NewWaveRuleCode, "SYN-RULE-A")
	err = waves.SaveWaveRule(context.Background(), tenant, ports.WaveRule{
		Warehouse: warehouse, Code: rule, Revision: 1, Name: "合成规则", Profile: ports.WaveRuleProfileAny, MaxRequests: 2,
		Operator: "operator-2", RegisteredAt: preAdviceNumberingInstant,
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveWaveRule：%v", err)
	}
	err = waves.SaveWaveRuleRun(context.Background(), tenant, ports.WaveRuleRun{
		RunID: "SYN-RUN-1", Warehouse: warehouse, Rule: rule, Revision: 1, Operator: "operator-1", RanAt: preAdviceNumberingInstant, Released: 1,
	})
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveWaveRuleRun：%v", err)
	}
	if err := waves.TagWaveRun(context.Background(), tenant, valueOf(t, domain.NewWaveID, "WAVE-1"), rule, "SYN-RUN-1"); !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("TagWaveRun：%v", err)
	}
	err = waves.SaveAllocationException(context.Background(), tenant, valueOf(t, domain.NewOutboundRequestID, "OBR-1"),
		ports.AllocationExceptionFromWave, "WAVE-1", preAdviceNumberingInstant)
	if !errors.Is(err, bentopg.ErrTransactionRequired) {
		t.Fatalf("SaveAllocationException：%v", err)
	}
}
