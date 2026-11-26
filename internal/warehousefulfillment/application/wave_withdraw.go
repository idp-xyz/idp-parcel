package application

import (
	"context"
	"fmt"
	"sort"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// WithdrawWaveCommand 把几张出库请求从已放出的波次里撤出。只撤还没拣过的：拣出来的货要回库，那是截单的事。
type WithdrawWaveCommand struct {
	Tenant   domain.TenantID
	ID       domain.WaveID
	Operator domain.Registrant
	Requests []domain.OutboundRequestID
}

// Withdraw 整份成立或整份不成立，与放波次同一口径：点名的请求里有一张撤不了，就一张都不撤。
// 撤出的请求回到已预占，预占原样留着，可以再放进别的波次；这个波次不再收它，也不往里加别的单。
func (handler *WaveHandler) Withdraw(ctx context.Context, command WithdrawWaveCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("withdraw from wave: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if len(command.Requests) == 0 || command.Operator.String() == "" || command.ID.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	members, found, err := handler.waves.LoadWaveMembers(ctx, command.Tenant, command.ID)
	if err != nil {
		return WaveResult{}, fmt.Errorf("withdraw from wave: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	withdrawn := map[string]bool{}
	for _, member := range members {
		withdrawn[member.Request.String()] = member.Withdrawn
	}
	var pending []domain.OutboundRequestID
	named := map[string]struct{}{}
	for _, id := range command.Requests {
		already, member := withdrawn[id.String()]
		if !member {
			return WaveResult{outcome: WaveContentInvalid}, nil
		}
		if _, repeated := named[id.String()]; repeated {
			continue
		}
		named[id.String()] = struct{}{}
		if !already {
			pending = append(pending, id)
		}
	}
	if len(pending) == 0 {
		return WaveResult{outcome: WaveExisting}, nil
	}
	// 撤单放开的是待执行任务占着的格。先拿到各货主在这个仓的入账次序，再读请求与任务：
	// 拣货入账与放波次的分配都排在同一道次序里，拿到之后读到的任务状态不会再被它们改。
	// 各货主按固定的先后去拿：两笔撤单点名的货主相同、先后相反时，不会各拿一半互相等。
	type posting struct {
		owner     domain.OwnerAccountID
		warehouse domain.WarehouseCode
	}
	postings := map[string]posting{}
	for _, id := range pending {
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return WaveResult{}, fmt.Errorf("withdraw from wave: %w", err)
		}
		if !found {
			return WaveResult{outcome: WaveNotFound}, nil
		}
		postings[request.Owner().String()+"\x1f"+request.Warehouse().String()] = posting{owner: request.Owner(), warehouse: request.Warehouse()}
	}
	keys := make([]string, 0, len(postings))
	for key := range postings {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	for _, key := range keys {
		if err := handler.ledger.SerializePosting(ctx, command.Tenant, postings[key].owner, postings[key].warehouse); err != nil {
			return WaveResult{}, fmt.Errorf("withdraw from wave: %w", err)
		}
	}
	for _, id := range pending {
		request, found, err := handler.requests.LoadByID(ctx, command.Tenant, id)
		if err != nil {
			return WaveResult{}, fmt.Errorf("withdraw from wave: %w", err)
		}
		if !found {
			return WaveResult{outcome: WaveNotFound}, nil
		}
		if request.Status() != domain.OutboundAllocated {
			return WaveResult{outcome: WaveStatusMismatch}, nil
		}
		tasks, err := handler.waves.TasksOf(ctx, command.Tenant, id)
		if err != nil {
			return WaveResult{}, fmt.Errorf("withdraw from wave: %w", err)
		}
		if !CanWithdrawFromWave(tasks, command.ID) {
			return WaveResult{outcome: WaveStatusMismatch}, nil
		}
	}
	at := handler.clock.Now()
	for _, id := range pending {
		if err := handler.waves.WithdrawFromWave(ctx, command.Tenant, command.ID, id, command.Operator, at); err != nil {
			return WaveResult{}, fmt.Errorf("withdraw from wave: %w", err)
		}
	}
	return WaveResult{outcome: WaveWithdrawn}, nil
}

// CanWithdrawFromWave 看一张已放波次的请求能不能从这个波次撤出：它在这个波次里得有任务等着拣，
// 并且没有一条拣过（拣满或少拣）。拣过的货已经在待出库，撤出不会把它送回去，得走截单。
// tasks 是这张请求的全部任务，别的波次里的不看。
func CanWithdrawFromWave(tasks []ports.PickTask, wave domain.WaveID) bool {
	open := false
	for _, task := range tasks {
		if task.WaveID != wave {
			continue
		}
		switch task.Status {
		case "OPEN":
			open = true
		case "DONE", "SHORT":
			return false
		}
	}
	return open
}
