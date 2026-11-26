package application

import (
	"context"
	"fmt"

	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

// maxTotesPerRegistration 是一次登记最多收的箱号数。一个仓的箱分几次登就是了；
// 不设上限的话，一份报文可以让一笔事务写上不知多少行。
const maxTotesPerRegistration = 500

// RegisterTotesCommand 给一个仓登记一批周转箱。拣货车上的格口也按箱登记。
type RegisterTotesCommand struct {
	Tenant    domain.TenantID
	Warehouse domain.WarehouseCode
	Codes     []domain.ToteCode
	Operator  domain.Registrant
}

// RetireToteCommand 停用一个周转箱。
type RetireToteCommand struct {
	Tenant    domain.TenantID
	Warehouse domain.WarehouseCode
	Code      domain.ToteCode
	Operator  domain.Registrant
}

// WithToteWarehouses 接上仓名册。没接时周转箱登记答未配置。
func (handler *WaveHandler) WithToteWarehouses(warehouses ports.WarehouseLookup) *WaveHandler {
	handler.toteWarehouses = warehouses
	return handler
}

// RegisterTotes 整批成立：已经在用的箱号照旧，停用的恢复在用，没有的新登。这一批里一个新的都没有时答已有结果。
// 只给登记过的自营仓登：代理仓的拣货不在这套系统里做，箱登在那里用不上。
func (handler *WaveHandler) RegisterTotes(ctx context.Context, command RegisterTotesCommand) (WaveResult, error) {
	if handler.toteWarehouses == nil {
		return WaveResult{outcome: WaveNotConfigured}, nil
	}
	if command.Operator.String() == "" || command.Warehouse.String() == "" ||
		len(command.Codes) == 0 || len(command.Codes) > maxTotesPerRegistration {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	warehouse, found, err := handler.toteWarehouses.LoadLatestWarehouse(ctx, command.Tenant, command.Warehouse)
	if err != nil {
		return WaveResult{}, fmt.Errorf("register totes: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveWarehouseNotFound}, nil
	}
	if warehouse.Content().Kind() == domain.WarehouseKindAgent || warehouse.Content().Deactivated() {
		return WaveResult{outcome: WaveWarehouseNotAccepting}, nil
	}
	at := handler.clock.Now()
	seen := map[string]struct{}{}
	changed := false
	for _, code := range command.Codes {
		if code.String() == "" {
			return WaveResult{outcome: WaveContentInvalid}, nil
		}
		if _, repeated := seen[code.String()]; repeated {
			continue
		}
		seen[code.String()] = struct{}{}
		registered, err := handler.waves.RegisterTote(ctx, command.Tenant, command.Warehouse, code, command.Operator, at)
		if err != nil {
			return WaveResult{}, fmt.Errorf("register totes: %w", err)
		}
		changed = changed || registered
	}
	if !changed {
		return WaveResult{outcome: WaveExisting}, nil
	}
	return WaveResult{outcome: WaveTotesRegistered}, nil
}

// RetireTote 停用一个箱：之后拣货不再认它。箱上还绑着出库请求时不停——货还在里面。已经停用的答已有结果。
func (handler *WaveHandler) RetireTote(ctx context.Context, command RetireToteCommand) (WaveResult, error) {
	if command.Operator.String() == "" || command.Warehouse.String() == "" || command.Code.String() == "" {
		return WaveResult{outcome: WaveContentInvalid}, nil
	}
	tote, found, err := handler.waves.LoadTote(ctx, command.Tenant, command.Warehouse, command.Code)
	if err != nil {
		return WaveResult{}, fmt.Errorf("retire tote: %w", err)
	}
	if !found {
		return WaveResult{outcome: WaveToteNotFound}, nil
	}
	if tote.Retired {
		return WaveResult{outcome: WaveExisting}, nil
	}
	if _, held, err := handler.waves.ToteHolder(ctx, command.Tenant, command.Warehouse, command.Code); err != nil {
		return WaveResult{}, fmt.Errorf("retire tote: %w", err)
	} else if held {
		return WaveResult{outcome: WaveToteInUse}, nil
	}
	if err := handler.waves.RetireTote(ctx, command.Tenant, command.Warehouse, command.Code, command.Operator, handler.clock.Now()); err != nil {
		return WaveResult{}, fmt.Errorf("retire tote: %w", err)
	}
	return WaveResult{outcome: WaveToteRetired}, nil
}
