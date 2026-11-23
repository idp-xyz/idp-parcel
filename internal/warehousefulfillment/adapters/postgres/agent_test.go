package postgres_test

import (
	"context"
	"testing"
	"time"

	adapter "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	"go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func TestAnAgentWarehouseKeepsStatementsOffTheLedgerAndForwardsOnlyWhenAConnectorIsRegistered(t *testing.T) {
	world := newReceiptWorld(t)
	world.seedReceiving(t)
	mustSave(t, world.transactor, world.warehouses, revisionFixture(t, "SYN-TENANT-01", "SYN-WH-AGENT", 1, func(spec *domain.WarehouseContentSpec) {
		spec.Kind = domain.WarehouseKindAgent
	}))
	outbound := world.outbound(t)
	command := outboundCommand(t, "REQ-AGENT", 2)
	command.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT")
	accepted := world.acceptOutbound(t, outbound, command)
	request, ok := accepted.Request()
	if !ok || accepted.Outcome() != application.OutboundHandedOff || reservedQuantity(t, world) != 0 {
		t.Fatalf("受理 %s 占用 %d", accepted.Outcome(), reservedQuantity(t, world))
	}
	handler := world.agents(t, &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-AGENT"})
	if world.forwardAgent(t, handler, request.ID()).Outcome() != application.AgentNotConfigured || requestStatus(t, world, request.ID()) != "HANDED_OFF" {
		t.Fatal("没登记连接器却转出了")
	}
	if world.registerAgent(t, handler).Outcome() != application.AgentRegistered {
		t.Fatal("连接器没有采用")
	}
	if world.forwardAgent(t, handler, request.ID()).Outcome() != application.AgentForwarded || requestStatus(t, world, request.ID()) != "FORWARDED" {
		t.Fatal("已登记的连接器没有转出")
	}
	if world.forwardAgent(t, handler, request.ID()).Outcome() != application.AgentExisting {
		t.Fatal("重复转出又推进了一次")
	}

	now := world.clock.Now()
	statement := agentStatement(t, "SRC-1", now.Add(-time.Hour), now.Add(time.Hour), 4)
	if world.recordStatement(t, handler, statement).Outcome() != application.AgentValid || world.agentMovements(t) != 0 || reservedQuantity(t, world) != 0 {
		t.Fatal("有效声明进了库存账")
	}
	later := world.agentsAt(t, now.Add(2*time.Hour), nil)
	if world.readStatement(t, later, statement).Outcome() != application.AgentUnknown {
		t.Fatal("过期声明没有答未知")
	}
	newer := agentStatement(t, "SRC-1", now.Add(3*time.Hour), now.Add(4*time.Hour), 9)
	if world.recordStatement(t, later, newer).Outcome() != application.AgentValid {
		t.Fatal("更新的声明没有收下")
	}
	if world.readStatement(t, later, statement).Outcome() != application.AgentSuperseded || world.agentMovements(t) != 0 {
		t.Fatal("旧声明没有被取代，或进了账")
	}

	refused := world.acceptAgent(t, outbound, "REQ-AGENT-NO")
	refusedID, _ := refused.Request()
	if world.forwardAgent(t, handler, refusedID.ID()).Outcome() != application.AgentForwarded {
		t.Fatal("第二份没有转出")
	}
	if world.agentResult(t, handler, refusedID.ID(), "RES-NO", "REJECTED").Outcome() != application.AgentRejected || requestStatus(t, world, refusedID.ID()) != "REJECTED" {
		t.Fatal("拒绝没有终结请求")
	}

	label := outboundCommand(t, "REQ-AGENT-LABEL", 1)
	label.Warehouse = command.Warehouse
	label.Dispatch = domain.DispatchOwnerLabel
	label.Service = ""
	labeled := world.acceptOutbound(t, outbound, label)
	labeledID, _ := labeled.Request()
	commissioner := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-LABEL"}
	labelHandler := world.agents(t, commissioner)
	if world.forwardAgent(t, labelHandler, labeledID.ID()).Outcome() != application.AgentForwarded {
		t.Fatal("自备面单没有转出")
	}
	if world.agentPack(t, labelHandler, labeledID.ID(), "RES-LABEL").Outcome() != application.AgentPacked || commissioner.called != 0 {
		t.Fatalf("自备面单提交了委托 %d", commissioner.called)
	}

	network := world.acceptAgent(t, outbound, "REQ-AGENT-NET")
	networkID, _ := network.Request()
	networkCommission := &scriptedCommissioner{outcome: ports.CommissionSubmittedWord, id: "SHR-NET"}
	networkHandler := world.agents(t, networkCommission)
	if world.forwardAgent(t, networkHandler, networkID.ID()).Outcome() != application.AgentForwarded {
		t.Fatal("本产品网络没有转出")
	}
	if world.agentPack(t, networkHandler, networkID.ID(), "RES-PACK").Outcome() != application.AgentPacked || networkCommission.called != 1 {
		t.Fatalf("装箱结果没有提交委托 %d", networkCommission.called)
	}
	if world.agentResult(t, networkHandler, networkID.ID(), "RES-HAND", "HANDED_OVER").Outcome() != application.AgentConfirmed || requestStatus(t, world, networkID.ID()) != "SHIPPED" || world.agentMovements(t) != 0 {
		t.Fatal("交接消耗了自营库存或没有确认")
	}
	missing := world.recordStatement(t, handler, agentStatementWarehouse(t, "SYN-WH-MISSING", now, now.Add(time.Hour)))
	if missing.Outcome() != application.AgentWarehouseNotFound {
		t.Fatalf("没登记的仓 %s", missing.Outcome())
	}
}

func (world receiptWorld) agents(t *testing.T, commissioner ports.OutboundCommissioner) *application.AgentHandler {
	t.Helper()
	return world.agentsAt(t, world.clock.Now(), commissioner)
}

func (world receiptWorld) agentsAt(t *testing.T, at time.Time, commissioner ports.OutboundCommissioner) *application.AgentHandler {
	t.Helper()
	agents, err := adapter.NewAgents(world.db)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := adapter.NewOutboundRequests(world.db)
	if err != nil {
		t.Fatal(err)
	}
	waves, err := adapter.NewWaves(world.db)
	if err != nil {
		t.Fatal(err)
	}
	return application.NewAgentHandler(agents, requests, waves, world.warehouses, commissioner, fixedClock{at: at})
}

func (world receiptWorld) registerAgent(t *testing.T, handler *application.AgentHandler) application.AgentResult {
	t.Helper()
	return world.agentCall(t, func(ctx context.Context) (application.AgentResult, error) {
		return handler.RegisterConnector(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT"))
	})
}

func (world receiptWorld) forwardAgent(t *testing.T, handler *application.AgentHandler, request domain.OutboundRequestID) application.AgentResult {
	t.Helper()
	return world.agentCall(t, func(ctx context.Context) (application.AgentResult, error) {
		return handler.Forward(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), request)
	})
}

func (world receiptWorld) recordStatement(t *testing.T, handler *application.AgentHandler, statement ports.AgentStatement) application.AgentResult {
	t.Helper()
	return world.agentCall(t, func(ctx context.Context) (application.AgentResult, error) {
		return handler.RecordStatement(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), statement)
	})
}

func (world receiptWorld) readStatement(t *testing.T, handler *application.AgentHandler, statement ports.AgentStatement) application.AgentResult {
	t.Helper()
	return world.agentCall(t, func(ctx context.Context) (application.AgentResult, error) {
		return handler.ReadStatement(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), statement)
	})
}

func (world receiptWorld) agentResult(t *testing.T, handler *application.AgentHandler, request domain.OutboundRequestID, id, kind string) application.AgentResult {
	t.Helper()
	return world.agentCall(t, func(ctx context.Context) (application.AgentResult, error) {
		return handler.RecordResult(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ports.AgentResult{
			ID: id, Request: request, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT"), Kind: kind, Reported: world.clock.Now(),
		})
	})
}

func (world receiptWorld) agentPack(t *testing.T, handler *application.AgentHandler, request domain.OutboundRequestID, id string) application.AgentResult {
	t.Helper()
	return world.agentCall(t, func(ctx context.Context) (application.AgentResult, error) {
		return handler.RecordResult(ctx, valueOf(t, domain.NewTenantID, "SYN-TENANT-01"), ports.AgentResult{
			ID: id, Request: request, Warehouse: valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT"), Kind: "PACKED",
			Weight: "1.2", WeightUnit: "KG", Length: "20", Width: "15", Height: "10", Dimension: "CM", Reported: world.clock.Now(),
		})
	})
}

func (world receiptWorld) agentCall(t *testing.T, run func(context.Context) (application.AgentResult, error)) application.AgentResult {
	t.Helper()
	var result application.AgentResult
	err := world.transactor.WithinTransaction(t.Context(), func(ctx context.Context) error {
		var runErr error
		result, runErr = run(ctx)
		return runErr
	})
	if err != nil {
		t.Fatalf("代理仓：%v", err)
	}
	return result
}

func (world receiptWorld) acceptAgent(t *testing.T, outbound *application.OutboundHandler, key string) application.OutboundResult {
	t.Helper()
	command := outboundCommand(t, key, 1)
	command.Warehouse = valueOf(t, domain.NewWarehouseCode, "SYN-WH-AGENT")
	return world.acceptOutbound(t, outbound, command)
}

func (world receiptWorld) agentMovements(t *testing.T) int {
	t.Helper()
	var count int
	if err := world.pool.QueryRow(t.Context(),
		`SELECT count(*) FROM warehouse_fulfillment.stock_movement WHERE tenant_id = 'SYN-TENANT-01'`).Scan(&count); err != nil {
		t.Fatal(err)
	}
	return count
}

func agentStatement(t *testing.T, source string, statedAt, validUntil time.Time, quantity int64) ports.AgentStatement {
	t.Helper()
	return agentStatementWarehouse(t, "SYN-WH-AGENT", statedAt, validUntil, source, quantity)
}

func agentStatementWarehouse(t *testing.T, warehouse string, statedAt, validUntil time.Time, extra ...any) ports.AgentStatement {
	t.Helper()
	source, quantity := "SRC-MISSING", int64(1)
	if len(extra) == 2 {
		source = extra[0].(string)
		quantity = extra[1].(int64)
	}
	return ports.AgentStatement{
		Source: source, StatedAt: statedAt, ValidUntil: validUntil,
		Warehouse: valueOf(t, domain.NewWarehouseCode, warehouse), Owner: valueOf(t, domain.NewOwnerAccountID, "SYN-OWNER-01"),
		Goods: valueOf(t, domain.NewGoodsCode, "SYN-GOODS-01"), Unit: valueOf(t, domain.NewGoodsUnitName, "件"), Quantity: quantity,
	}
}
