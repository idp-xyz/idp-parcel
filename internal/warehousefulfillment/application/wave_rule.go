package application

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

const (
	// maxRequestsPerRuleWave 是一条规则能登记的「一个波次最多几张」的上限。规则登多少是租户的事；
	// 上限只挡住一个波次大到一趟拣不回来、一笔事务写不完的登记。
	maxRequestsPerRuleWave = 200
	// maxRequestsPerRuleRun 是一次按规则成波至多处理的请求数。圈到的比它多时，受理得早的先放，
	// 剩下的在答复里如实带出，再点一次接着放。
	maxRequestsPerRuleRun = 500
)

// RegisterWaveRuleCommand 登记一条成波规则的一版。Owner 与 Dispatch 为空是不限。
type RegisterWaveRuleCommand struct {
	Tenant      domain.TenantID
	Warehouse   domain.WarehouseCode
	Code        domain.WaveRuleCode
	Revision    int
	Name        string
	Owner       string
	Dispatch    string
	Profile     string
	MaxRequests int
	// MaxLines 与 MaxUnits 是一个波次最多几行、最多几件；0 是不限。
	MaxLines    int
	MaxUnits    int64
	BatchPick   bool
	Deactivated bool
	Operator    domain.Registrant
}

// RegisterWaveRule 成波规则是租户按仓定的：圈哪些单、一个波次多大、怎么拣，产品不替他定，也不带一条默认的规则。
// 修订从 1 起连续；与最新一版逐格相同的重放答已有结果，跳号或同号不同内容是内容冲突。只给登记过的自营仓登。
func (handler *WaveHandler) RegisterWaveRule(ctx context.Context, command RegisterWaveRuleCommand) (WaveResult, error) {
	if handler.toteWarehouses == nil {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	name := strings.TrimSpace(command.Name)
	if command.Operator.String() == "" || command.Warehouse.String() == "" || command.Code.String() == "" || command.Revision < 1 ||
		name == "" || command.MaxRequests < 1 || command.MaxRequests > maxRequestsPerRuleWave || command.MaxLines < 0 || command.MaxUnits < 0 ||
		!knownWaveRuleProfile(command.Profile) {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	warehouse, found, err := handler.toteWarehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return WaveResult{}, fmt.Errorf("register wave rule: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent {
		return WaveResult{outcome: WaveWarehouseNotAccepting}, nil
	}
	rule := ports.WaveRule{
		Warehouse: command.Warehouse, Code: command.Code, Revision: command.Revision, Name: name, Owner: command.Owner,
		Dispatch: command.Dispatch, Profile: command.Profile, MaxRequests: command.MaxRequests, MaxLines: command.MaxLines,
		MaxUnits: command.MaxUnits, BatchPick: command.BatchPick, Deactivated: command.Deactivated,
	}
	current, found, err := handler.waves.LoadWaveRule(ctx, command.Tenant, command.Warehouse, command.Code)
	if err != nil {
		return WaveResult{}, fmt.Errorf("register wave rule: %w", err)
	}
	if found && current.Revision == command.Revision {
		if sameWaveRule(current, rule) {
			return WaveResult{outcome: WaveExisting}, nil
		}
		return WaveResult{outcome: WaveConflict}, nil
	}
	if (found && command.Revision != current.Revision+1) || (!found && command.Revision != 1) {
		return WaveResult{outcome: WaveConflict}, nil
	}
	rule.Operator, rule.RegisteredAt = command.Operator.String(), handler.clock.Now()
	if err := handler.waves.SaveWaveRule(ctx, command.Tenant, rule); err != nil {
		return WaveResult{}, fmt.Errorf("register wave rule: %w", err)
	}
	return WaveResult{outcome: WaveStrategyRegistered}, nil
}

func knownWaveRuleProfile(profile string) bool {
	return profile == ports.WaveRuleProfileAny || profile == ports.WaveRuleProfileSingleUnit || profile == ports.WaveRuleProfileMultiUnit
}

// sameWaveRule 比的是规则说了什么，不比是谁在什么时候登的。
func sameWaveRule(left, right ports.WaveRule) bool {
	return left.Name == right.Name && left.Owner == right.Owner && left.Dispatch == right.Dispatch && left.Profile == right.Profile &&
		left.MaxRequests == right.MaxRequests && left.MaxLines == right.MaxLines && left.MaxUnits == right.MaxUnits &&
		left.BatchPick == right.BatchPick && left.Deactivated == right.Deactivated
}

// RunWaveRuleCommand 按一条成波规则把这个仓里圈得到的已预占请求放成波次。RunID 是这一次执行的号，
// 放出的波次按它依次取号。
type RunWaveRuleCommand struct {
	Tenant    domain.TenantID
	Warehouse domain.WarehouseCode
	Rule      domain.WaveRuleCode
	RunID     domain.WaveID
	Operator  domain.Registrant
}

// WaveRuleRunResult 是一次按规则成波。Released 是放进波次的请求数；Skipped 是圈到了而分配不出、留在已预占的；
// Remaining 是超过一次执行的上限、这一次没轮到的。
type WaveRuleRunResult struct {
	outcome   WaveOutcome
	waves     []ports.RuleWave
	revision  int
	released  int
	skipped   int
	remaining int
}

func (result WaveRuleRunResult) Outcome() WaveOutcome { return result.outcome }
func (result WaveRuleRunResult) Waves() []ports.RuleWave {
	return append([]ports.RuleWave(nil), result.waves...)
}
func (result WaveRuleRunResult) Revision() int  { return result.revision }
func (result WaveRuleRunResult) Released() int  { return result.released }
func (result WaveRuleRunResult) Skipped() int   { return result.skipped }
func (result WaveRuleRunResult) Remaining() int { return result.remaining }

// RunWaveRule 照规则圈单，受理得早的在前，每满规则登的张数切成一个波次放出；分配照放波次的同一套办。
// 圈到的请求里有分配不出的（库存不在可拣的库位、少拣的库位还没盘过），跳过它、留在已预占，不让一张单挡住整批。
// 一个波次也没放出时不记这一次执行，答没有可放的；跳过的请求照样记成放不出的例外。
// 同一个执行号重放答已有结果，带着当时放出的波次。
//
// 规则不带预取：估算委托要选包材，那是放某一个波次时人定的，不是规则圈单的条件。
func (handler *WaveHandler) RunWaveRule(ctx context.Context, command RunWaveRuleCommand) (WaveRuleRunResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	}
	if !configured {
		return WaveRuleRunResult{outcome: WaveNotConfigured}, nil
	}
	if command.Operator.String() == "" || command.RunID.String() == "" || command.Rule.String() == "" || command.Warehouse.String() == "" {
		return WaveRuleRunResult{outcome: WaveContentInvalid}, nil
	}
	ran, found, err := handler.waves.LoadWaveRuleRun(ctx, command.Tenant, command.RunID.String())
	if err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	}
	if found {
		if ran.Warehouse != command.Warehouse || ran.Rule != command.Rule {
			return WaveRuleRunResult{outcome: WaveConflict}, nil
		}
		waves, err := handler.waves.WavesOfRun(ctx, command.Tenant, command.RunID.String())
		if err != nil {
			return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
		}
		return WaveRuleRunResult{
			outcome: WaveExisting, waves: waves, revision: ran.Revision, released: ran.Released, skipped: ran.Skipped, remaining: ran.Remaining,
		}, nil
	}
	rule, found, err := handler.waves.LoadWaveRule(ctx, command.Tenant, command.Warehouse, command.Rule)
	if err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	}
	if !found {
		return WaveRuleRunResult{outcome: WaveRuleNotFound}, nil
	}
	if rule.Deactivated {
		return WaveRuleRunResult{outcome: WaveRuleDeactivated, revision: rule.Revision}, nil
	}
	if _, adopted, err := handler.waves.LoadStrategy(ctx, command.Tenant); err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	} else if !adopted {
		return WaveRuleRunResult{outcome: WaveNotConfigured}, nil
	}
	candidates, total, err := handler.waves.WaveRuleCandidates(ctx, command.Tenant, rule, maxRequestsPerRuleRun)
	if err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	}
	result := WaveRuleRunResult{revision: rule.Revision, remaining: total - len(candidates)}
	// 波次号在动任何东西之前先核一遍：放到一半才发现撞号，前面放出的波次已经落下，只能整笔回滚成没有答案。
	waveCommand := func(index int) (ReleaseWaveCommand, error) {
		id, err := domain.NewWaveID(fmt.Sprintf("%s-%02d", command.RunID.String(), index))
		return ReleaseWaveCommand{
			Tenant: command.Tenant, ID: id, Warehouse: command.Warehouse, Operator: command.Operator, BatchPick: rule.BatchPick,
		}, err
	}
	// 会放出几个波次要分配完才知道（行数、件数的上限会提前切波次），所以核的是「有没有以这个执行号开头的波次」。
	if clash, err := handler.waves.AnyWaveStartingWith(ctx, command.Tenant, command.RunID.String()+"-"); err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	} else if clash {
		return WaveRuleRunResult{outcome: WaveConflict, revision: rule.Revision}, nil
	}
	at := handler.clock.Now()
	current, err := waveCommand(1)
	if err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	}
	var members []domain.OutboundRequestID
	var tasks []ports.PickTask
	// 同一个波次里先分走的数量还没落成拣货任务，后一张请求读占用账看不见；波次存下之后就看得见了，从头再记。
	taken := map[string]int64{}
	// 当前这个波次已经有几行、几件：行数按预占行数，件数按预占的基本单位数量。
	lines, units := 0, int64(0)
	flush := func() error {
		if len(members) == 0 {
			return nil
		}
		if err := handler.waves.SaveWave(ctx, command.Tenant, current.ID, command.Warehouse, command.Operator, false, rule.BatchPick, at, members, tasks); err != nil {
			return err
		}
		if err := handler.waves.TagWaveRun(ctx, command.Tenant, current.ID, command.Rule, command.RunID.String()); err != nil {
			return err
		}
		result.waves = append(result.waves, ports.RuleWave{Wave: current.ID.String(), Requests: len(members)})
		result.released += len(members)
		members, tasks, taken = nil, nil, map[string]int64{}
		lines, units = 0, 0
		current, err = waveCommand(len(result.waves) + 1)
		return err
	}
	// 先把圈到的各位货主在这个仓的入账次序都排上，再逐张重读：圈单那一句没有锁，排队之前请求可能已被另一笔放波次或
	// 截单拿走。几位货主按账户号的先后去排，两次同时执行不会各拿一半互相等。
	owners := map[string]domain.OwnerAccountID{}
	for _, id := range candidates {
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
		}
		if found {
			owners[request.Owner().String()] = request.Owner()
		}
	}
	accounts := make([]string, 0, len(owners))
	for account := range owners {
		accounts = append(accounts, account)
	}
	sort.Strings(accounts)
	for _, account := range accounts {
		if err := handler.ledger.SerializePosting(ctx, command.Tenant, owners[account], command.Warehouse); err != nil {
			return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
		}
	}
	for _, id := range candidates {
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
		}
		if !found || request.Status() != domain.OutboundReserved || request.Warehouse() != command.Warehouse {
			continue
		}
		// 加上这一张会超过规则登的行数或件数时，先把手上这个波次放出去、另起一个。波次里还没有单时不切：
		// 一张请求自己就超了的单独成一个波次，一张请求拆不开。
		requestLines, requestUnits := len(request.Reservations()), int64(0)
		for _, reserved := range request.Reservations() {
			requestUnits += reserved.Quantity
		}
		if len(members) > 0 && ((rule.MaxLines > 0 && lines+requestLines > rule.MaxLines) || (rule.MaxUnits > 0 && units+requestUnits > rule.MaxUnits)) {
			if err := flush(); err != nil {
				return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
			}
		}
		// 分配不出时 allocate 已经往 taken 里记了一半，照分配之前的那一份还原。
		before := maps.Clone(taken)
		allocated, err := handler.allocate(ctx, current, request, taken)
		if err != nil {
			return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
		}
		if allocated == nil {
			taken = before
			result.skipped++
			// 跳过的这一张记成放不出的例外：人要在出库单列表里找得到它，不能只在这一次的答复里见一个数。
			if err := handler.waves.SaveAllocationException(ctx, command.Tenant, id, ports.AllocationExceptionFromRule, command.RunID.String(), at); err != nil {
				return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
			}
			continue
		}
		members = append(members, id)
		tasks = append(tasks, allocated...)
		lines, units = lines+requestLines, units+requestUnits
		if len(members) == rule.MaxRequests {
			if err := flush(); err != nil {
				return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
			}
		}
	}
	if err := flush(); err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	}
	if len(result.waves) == 0 {
		result.outcome = WaveNothingToRelease
		return result, nil
	}
	if err := handler.waves.SaveWaveRuleRun(ctx, command.Tenant, ports.WaveRuleRun{
		RunID: command.RunID.String(), Warehouse: command.Warehouse, Rule: command.Rule, Revision: rule.Revision,
		Operator: command.Operator.String(), RanAt: at, Released: result.released, Skipped: result.skipped, Remaining: result.remaining,
	}); err != nil {
		return WaveRuleRunResult{}, fmt.Errorf("run wave rule: %w", err)
	}
	result.outcome = WaveAllocated
	return result, nil
}
