package application

import (
	"context"
	"fmt"
	"strings"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// AssignWaveCommand 由管波次的人把一个波次派给一个人拣。Assignee 是拣货人的操作者标识；为空是取消指派。
// Zone 给了就只派这个波次里这一个库区的格：没单独派的库区照整个波次的指派走。
type AssignWaveCommand struct {
	Tenant   domain.TenantID
	Wave     domain.WaveID
	Zone     domain.ZoneCode
	Assignee string
	Operator domain.Registrant
}

// ClaimWaveCommand 是拣货的人在工位上自己领一个波次。Operator 是他的操作者标识，取自作业事实的认证。
type ClaimWaveCommand struct {
	Tenant    domain.TenantID
	Wave      domain.WaveID
	Warehouse domain.WarehouseCode
	Operator  string
}

// waveStillPickable 回答这个波次还有没有任务等着拣：只有已放出与拣货中的波次可以派、可以领。
func waveStillPickable(status string) bool {
	return status == ports.WaveReleased || status == ports.WavePicking
}

// AssignWave 派工到波次为止：一个波次派给一个人，之后别人拣它待执行的任务不成立。已经派给这个人的答已有结果；
// 派给另一个人就是改派，取消指派之后谁都可以拣、可以领。拣完或取消的波次没有可派的任务。
func (handler *WaveHandler) AssignWave(ctx context.Context, command AssignWaveCommand) (WaveResult, error) {
	assignee := strings.TrimSpace(command.Assignee)
	if command.Operator.String() == "" || command.Wave.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	current, found, err := handler.waves.LoadWaveAssignment(ctx, command.Tenant, command.Wave)
	if err != nil {
		return WaveResult{}, fmt.Errorf("assign wave: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if command.Zone.String() != "" {
		return handler.assignWaveZone(ctx, command, current, assignee)
	}
	if current.Assignee == assignee {
		return WaveResult{outcome: WaveExisting, assignee: assignee}, nil
	}
	if !waveStillPickable(current.Status) {
		return WaveResult{outcome: WaveStatusMismatch, assignee: current.Assignee}, nil
	}
	if err := handler.waves.SaveWaveAssignee(ctx, command.Tenant, command.Wave, assignee, command.Operator.String(), handler.clock.Now()); err != nil {
		return WaveResult{}, fmt.Errorf("assign wave: %w", err)
	}
	if assignee == "" {
		return WaveResult{outcome: WaveUnassigned}, nil
	}
	return WaveResult{outcome: WaveAssigned, assignee: assignee}, nil
}

// assignWaveZone 按库区派：这个波次里这一个库区的格归一个人，别的库区照整个波次的指派走。波次那一行已经由
// LoadWaveAssignment 锁住，两个人同时改同一个库区排在它上面。库区得是这个仓登记过的，认不得答内容不合——
// 派给一个不存在的库区不会拦住任何一格，只会让人以为派出去了。答复里的 assignee 是这个库区此刻派给了谁。
func (handler *WaveHandler) assignWaveZone(ctx context.Context, command AssignWaveCommand, current ports.WaveAssignment, assignee string) (WaveResult, error) {
	if _, found, err := handler.zones.LoadLatestZone(ctx, command.Tenant, current.Warehouse, command.Zone); err != nil {
		return WaveResult{}, fmt.Errorf("assign wave zone: %w", err)
	} else if !found {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	had, err := handler.waves.WaveZoneAssignee(ctx, command.Tenant, command.Wave, command.Zone)
	if err != nil {
		return WaveResult{}, fmt.Errorf("assign wave zone: %w", err)
	}
	if had == assignee {
		return WaveResult{outcome: WaveExisting, assignee: assignee}, nil
	}
	if !waveStillPickable(current.Status) {
		return WaveResult{outcome: WaveStatusMismatch, assignee: had}, nil
	}
	if err := handler.waves.SaveWaveZoneAssignee(ctx, command.Tenant, command.Wave, command.Zone, assignee, command.Operator.String(), handler.clock.Now()); err != nil {
		return WaveResult{}, fmt.Errorf("assign wave zone: %w", err)
	}
	if assignee == "" {
		return WaveResult{outcome: WaveUnassigned}, nil
	}
	return WaveResult{outcome: WaveAssigned, assignee: assignee}, nil
}

// pickAssignee 交回一条任务此刻归谁拣：它所在库位的库区单独派了人就是那个人，没有就是整个波次派给的人；
// 都没派答空，谁都可以拣。库位在册上认不出库区时照整个波次的指派走。
func (handler *WaveHandler) pickAssignee(ctx context.Context, tenant domain.TenantID, warehouse domain.WarehouseCode, task ports.PickTask) (string, error) {
	place, found, err := handler.locations.LoadLatestLocation(ctx, tenant, warehouse, task.Location)
	if err != nil {
		return "", err
	}
	if found {
		assignee, err := handler.waves.WaveZoneAssignee(ctx, tenant, task.WaveID, place.Content().ZoneCode())
		if err != nil {
			return "", err
		}
		if assignee != "" {
			return assignee, nil
		}
	}
	return handler.waves.WaveAssignee(ctx, tenant, task.WaveID)
}

// ClaimWave 没人领的波次归领的这个人；本来就是他的答已有结果；已经派给了别人不成立，答复带着派给了谁——
// 要改派是管波次的人的事，拣货的人不能从别人手里拿。
func (handler *WaveHandler) ClaimWave(ctx context.Context, command ClaimWaveCommand) (WaveResult, error) {
	configured, err := handler.anyWarehouse.HasAnyWarehouse(ctx, command.Tenant)
	if err != nil {
		return WaveResult{}, fmt.Errorf("claim wave: %w", err)
	}
	if !configured {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.Operator == "" || command.Wave.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	current, found, err := handler.waves.LoadWaveAssignment(ctx, command.Tenant, command.Wave)
	if err != nil {
		return WaveResult{}, fmt.Errorf("claim wave: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveNotFound}, nil
	}
	if command.Warehouse.String() != "" && command.Warehouse != current.Warehouse {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	if current.Assignee == command.Operator {
		return WaveResult{outcome: WaveExisting, assignee: current.Assignee}, nil
	}
	if current.Assignee != "" {
		return WaveResult{outcome: WaveAssignedToOther, assignee: current.Assignee}, nil
	}
	if !waveStillPickable(current.Status) {
		return WaveResult{outcome: WaveStatusMismatch}, nil
	}
	if err := handler.waves.SaveWaveAssignee(ctx, command.Tenant, command.Wave, command.Operator, command.Operator, handler.clock.Now()); err != nil {
		return WaveResult{}, fmt.Errorf("claim wave: %w", err)
	}
	return WaveResult{outcome: WaveAssigned, assignee: command.Operator}, nil
}
