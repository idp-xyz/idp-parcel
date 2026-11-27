package application

import (
	"context"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
)

// maxReleaseTimesPerRule 是一条规则一天最多登几个时点。半小时一个也只有四十八个。
const maxReleaseTimesPerRule = 48

var releaseTimePattern = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// normalizedReleaseTimes 把登记的时点理成从早到晚、不重复的一列。有一个不是 HH:MM，或太多，整份不收。
func normalizedReleaseTimes(times []string) ([]string, bool) {
	if len(times) > maxReleaseTimesPerRule {
		return nil, false
	}
	normalized := make([]string, 0, len(times))
	for _, raw := range times {
		slot := strings.TrimSpace(raw)
		if !releaseTimePattern.MatchString(slot) {
			return nil, false
		}
		if !slices.Contains(normalized, slot) {
			normalized = append(normalized, slot)
		}
	}
	slices.Sort(normalized)
	return normalized, true
}

// RunDueWaveRulesCommand 放此刻已经到点的成波规则。Warehouse 为空是本租户各仓的。Operator 是来调的那个身份：
// 产品里没有定时器，到点由部署方的定时任务带着一个操作者凭证来调，放出的波次记在他名下。
type RunDueWaveRulesCommand struct {
	Tenant    domain.TenantID
	Warehouse string
	Operator  domain.Registrant
}

// DueWaveRuleRun 是一个到点的时点这一次办的结果。
type DueWaveRuleRun struct {
	Warehouse string
	Rule      string
	// Slot 是时点 HH:MM，Date 是它所在的业务日 YYYY-MM-DD，都按仓的业务时区。
	Slot   string
	Date   string
	RunID  string
	Result WaveRuleRunResult
}

// DueWaveRulesResult 是一次「放到点的规则」。Unscheduled 是登了时点、但仓的业务时区没登记所以算不出到没到点的规则——
// 那是未配置，不拿部署机器的时区顶替。
type DueWaveRulesResult struct {
	outcome     WaveOutcome
	runs        []DueWaveRuleRun
	unscheduled []DueWaveRuleRun
}

func (result DueWaveRulesResult) Outcome() WaveOutcome { return result.outcome }
func (result DueWaveRulesResult) Runs() []DueWaveRuleRun {
	return append([]DueWaveRuleRun(nil), result.runs...)
}
func (result DueWaveRulesResult) Unscheduled() []DueWaveRuleRun {
	return append([]DueWaveRuleRun(nil), result.unscheduled...)
}

// RunDueWaveRules 对每条登了时点的规则，把今天（仓的业务日）已经过了、还没办过的时点从早到晚各办一次：
// 照 RunWaveRule 成一次波，执行号由规则码、业务日与时点定，所以同一个时点重复调不会放两遍。
// 到点时没有可放的也算办过——不然之后才受理的单会在下一次调用时被这个早已过去的时点放走。
// 没办成的（未采用分配策略、波次号撞了）不算办过，下一次调用再办。昨天漏掉的时点不补。
func (handler *WaveHandler) RunDueWaveRules(ctx context.Context, command RunDueWaveRulesCommand) (DueWaveRulesResult, error) {
	if handler.toteWarehouses == nil {
		return DueWaveRulesResult{outcome: WaveNotConfigured}, nil
	}
	if command.Operator.String() == "" {
		return DueWaveRulesResult{outcome: WaveContentInvalid}, nil
	}
	rules, err := handler.waves.ScheduledWaveRules(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return DueWaveRulesResult{}, fmt.Errorf("run due wave rules: %w", err)
	}
	now := handler.clock.Now()
	result := DueWaveRulesResult{outcome: WaveNothingDue}
	zones := map[string]*time.Location{}
	for _, rule := range rules {
		location, known := zones[rule.Warehouse.String()]
		if !known {
			location, err = handler.businessLocation(ctx, command.Tenant, rule.Warehouse)
			if err != nil {
				return DueWaveRulesResult{}, fmt.Errorf("run due wave rules: %w", err)
			}
			zones[rule.Warehouse.String()] = location
		}
		if location == nil {
			result.unscheduled = append(result.unscheduled, DueWaveRuleRun{Warehouse: rule.Warehouse.String(), Rule: rule.Code.String()})
			continue
		}
		local := now.In(location)
		date, clock := local.Format("2006-01-02"), local.Format("15:04")
		for _, slot := range rule.ReleaseTimes {
			if slot > clock {
				break
			}
			runID, err := domain.NewWaveID(rule.Code.String() + "-" + local.Format("20060102") + "-" + strings.ReplaceAll(slot, ":", ""))
			if err != nil {
				return DueWaveRulesResult{}, fmt.Errorf("run due wave rules: %w", err)
			}
			claimed, err := handler.waves.ClaimWaveRuleSlot(ctx, command.Tenant, rule, date, slot, runID.String(), command.Operator.String(), now)
			if err != nil {
				return DueWaveRulesResult{}, fmt.Errorf("run due wave rules: %w", err)
			}
			if !claimed {
				continue
			}
			ran, err := handler.RunWaveRule(ctx, RunWaveRuleCommand{
				Tenant: command.Tenant, Warehouse: rule.Warehouse, Rule: rule.Code, RunID: runID, Operator: command.Operator,
			})
			if err != nil {
				return DueWaveRulesResult{}, fmt.Errorf("run due wave rules: %w", err)
			}
			switch ran.Outcome() {
			case WaveAllocated, WaveExisting, WaveNothingToRelease:
				err = handler.waves.SettleWaveRuleSlot(ctx, command.Tenant, rule, date, slot, ran.Outcome().String())
			default:
				err = handler.waves.ReleaseWaveRuleSlot(ctx, command.Tenant, rule, date, slot)
			}
			if err != nil {
				return DueWaveRulesResult{}, fmt.Errorf("run due wave rules: %w", err)
			}
			result.runs = append(result.runs, DueWaveRuleRun{
				Warehouse: rule.Warehouse.String(), Rule: rule.Code.String(), Slot: slot, Date: date, RunID: runID.String(), Result: ran,
			})
		}
	}
	if len(result.runs) > 0 {
		result.outcome = WaveDueRulesRun
	}
	return result, nil
}

// businessLocation 取这个仓登记的业务时区；没登记或时区库里读不出答 nil。
func (handler *WaveHandler) businessLocation(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode) (*time.Location, error) {
	revision, found, err := handler.toteWarehouses.LoadLatestWarehouse(ctx, tenant, warehouse)
	if err != nil {
		return nil, err
	}
	if !found || !revision.Content().BusinessTimeZone().Registered() {
		return nil, nil
	}
	location, err := time.LoadLocation(revision.Content().BusinessTimeZone().String())
	if err != nil {
		return nil, nil
	}
	return location, nil
}
