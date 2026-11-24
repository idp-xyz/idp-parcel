// 演示租户完整仓配履约（PN-09）与仓储收费合成种子。
// 全部名称、编码、单价、账户与方案均为 SYN- 演示值（证据层级 S），不是生产默认。
// 重复执行对同一库幂等：登记册重放答 ALREADY_REGISTERED，业务单据与作业事实答 EXISTING_RESULT，不重复写账。
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	bentopg "go.idp.xyz/idp-bento-go/postgres"
	"go.idp.xyz/idp-bento-go/postgres/outbox"

	pricingstore "go.idp.xyz/idp-parcel/internal/parcelpricing/adapters/postgres"
	pricingapp "go.idp.xyz/idp-parcel/internal/parcelpricing/application"
	pricingdomain "go.idp.xyz/idp-parcel/internal/parcelpricing/domain"
	pcpostgres "go.idp.xyz/idp-parcel/internal/partycommercial/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber"
	"go.idp.xyz/idp-parcel/internal/platform/docnumber/docnumberpg"
	"go.idp.xyz/idp-parcel/internal/platform/migrate"
	adapter "go.idp.xyz/idp-parcel/internal/settlementaccounting/adapters/postgres"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/application"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/domain"
	"go.idp.xyz/idp-parcel/internal/settlementaccounting/ports"
	whidentity "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/identity"
	whparty "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/partycommercial"
	whpostgres "go.idp.xyz/idp-parcel/internal/warehousefulfillment/adapters/postgres"
	whapp "go.idp.xyz/idp-parcel/internal/warehousefulfillment/application"
	whdomain "go.idp.xyz/idp-parcel/internal/warehousefulfillment/domain"
	whports "go.idp.xyz/idp-parcel/internal/warehousefulfillment/ports"
)

func main() {
	dsn := os.Getenv("IDP_PARCEL_POSTGRES_DSN")
	if dsn == "" {
		fmt.Fprintln(os.Stderr, "IDP_PARCEL_POSTGRES_DSN 未设置——仓配演示种子不猜连接串")
		os.Exit(1)
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		fmt.Fprintf(os.Stderr, "连接数据库：%v\n", err)
		os.Exit(1)
	}
	defer pool.Close()
	db, err := bentopg.NewDB(pool, bentopg.WithSchema(migrate.SchemaBento))
	if err != nil {
		fmt.Fprintf(os.Stderr, "构造框架 DB：%v\n", err)
		os.Exit(1)
	}
	if err := walkOperations(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "仓配业务种子：%v\n", err)
		os.Exit(1)
	}
	if err := walkBilling(ctx, db); err != nil {
		fmt.Fprintf(os.Stderr, "仓储收费种子：%v\n", err)
		os.Exit(1)
	}
	fmt.Println("== 仓配全链路演示数据：五册主数据、八项规则、商品、库存、入库、待认领、退货、盘点与调整、换标、出库波次与仓储计费均已灌入 ==")
}

// walkOperations 灌入仓配履约全部核心业务对象，使管理台 16 页与模拟 PDA 作业台开箱即有真实可操作的待办数据。
func walkOperations(ctx context.Context, db *bentopg.DB) error {
	tenant := mustRef(whdomain.NewTenantID, "SYN-TENANT-01")
	owner1 := mustRef(whdomain.NewOwnerAccountID, "SYN-ACCOUNT-01")
	owner2 := mustRef(whdomain.NewOwnerAccountID, "SYN-ACCOUNT-02")
	whSelf := mustRef(whdomain.NewWarehouseCode, "SYN-WH-01")
	whAgent := mustRef(whdomain.NewWarehouseCode, "SYN-WH-AGENT-01")
	regOp := mustRef(whdomain.NewRegistrant, "SEED-WF/config-operator")
	approverOp := mustRef(whdomain.NewRegistrant, "SEED-WF/supervisor-approver")
	pdaDevice := mustRef(whdomain.NewReceiptDeviceID, "SYN-PDA-WH01-01")
	pdaOp := mustRef(whdomain.NewReceiptOperatorID, "SEED-WF/pda-operator")
	measDevice := mustRef(whdomain.NewMeasurementDeviceID, "SYN-PDA-WH01-01")
	measOp := mustRef(whdomain.NewMeasurementOperatorID, "SEED-WF/pda-operator")

	baseTime := time.Date(2026, 11, 18, 2, 0, 0, 0, time.UTC)
	opTime := time.Date(2026, 11, 20, 6, 0, 0, 0, time.UTC)
	clock := seedClock{at: baseTime}
	opClock := seedClock{at: opTime}

	docCatalogue, err := docnumber.NewCatalogue(whdomain.DocumentNumberTypes())
	if err != nil {
		return fmt.Errorf("装配单据编号目录：%w", err)
	}
	docBook, err := docnumberpg.NewStore(db)
	if err != nil {
		return fmt.Errorf("装配单据编号册：%w", err)
	}
	docSeqs, err := docnumberpg.NewSequences(db)
	if err != nil {
		return fmt.Errorf("装配单据编号序列：%w", err)
	}
	assigner, err := docnumber.NewAssigner(docnumber.AssignerDeps{
		Catalogue: docCatalogue, Book: docBook, Sequences: docSeqs, Transactions: docBook, Secrets: docnumber.SecretTable{}, Clock: opClock,
	})
	if err != nil {
		return fmt.Errorf("装配单据编号分配器：%w", err)
	}
	warehouses, err := whpostgres.NewWarehouses(db)
	if err != nil {
		return err
	}
	zones, err := whpostgres.NewZones(db)
	if err != nil {
		return err
	}
	locTypes, err := whpostgres.NewLocationTypes(db)
	if err != nil {
		return err
	}
	locations, err := whpostgres.NewLocations(db)
	if err != nil {
		return err
	}
	packaging, err := whpostgres.NewPackagingMaterials(db)
	if err != nil {
		return err
	}
	goods, err := whpostgres.NewGoods(db)
	if err != nil {
		return err
	}
	goodsCat, err := whpostgres.NewGoodsCatalogue(db)
	if err != nil {
		return err
	}
	partyAccounts, err := pcpostgres.NewPartyIdentityRegistrations(db)
	if err != nil {
		return err
	}
	ownerLookup, err := whparty.NewOwnerAccountLookup(partyAccounts)
	if err != nil {
		return err
	}
	stock, err := whpostgres.NewStockLedger(db)
	if err != nil {
		return err
	}
	stockIDs, err := whidentity.NewStockIdentities()
	if err != nil {
		return err
	}
	cartons, err := whpostgres.NewCartonLedger(db)
	if err != nil {
		return err
	}
	cartonIDs, err := whidentity.NewCartonIdentities()
	if err != nil {
		return err
	}
	preAdviceStore, err := whpostgres.NewPreAdvices(db)
	if err != nil {
		return err
	}
	preAdviceNums, err := whpostgres.NewPreAdviceNumbers(db, assigner)
	if err != nil {
		return err
	}
	preAdviceIDs, err := whidentity.NewPreAdviceIdentities()
	if err != nil {
		return err
	}
	receipts, err := whpostgres.NewReceipts(db)
	if err != nil {
		return err
	}
	discrepancyIDs, err := whidentity.NewReceiptDiscrepancyIdentities()
	if err != nil {
		return err
	}
	arrivals, err := whpostgres.NewUnclaimedArrivals(db)
	if err != nil {
		return err
	}
	arrivalIDs, err := whidentity.NewUnclaimedArrivalIdentities()
	if err != nil {
		return err
	}
	disposalRules, err := whpostgres.NewUnclaimedDisposalRules(db)
	if err != nil {
		return err
	}
	measurements, err := whpostgres.NewWarehouseMeasurements(db)
	if err != nil {
		return err
	}
	stockImports, err := whpostgres.NewStockImports(db)
	if err != nil {
		return err
	}
	internalJobStore, err := whpostgres.NewInternalJobs(db)
	if err != nil {
		return err
	}
	stockDiscIDs, err := whidentity.NewStockDiscrepancyIdentities()
	if err != nil {
		return err
	}
	freezeStore, err := whpostgres.NewFreezes(db)
	if err != nil {
		return err
	}
	freezeIDs, err := whidentity.NewFreezeIdentities()
	if err != nil {
		return err
	}
	countStore, err := whpostgres.NewCounts(db)
	if err != nil {
		return err
	}
	outboundStore, err := whpostgres.NewOutboundRequests(db)
	if err != nil {
		return err
	}
	outboundNums, err := whpostgres.NewOutboundNumbers(db, assigner)
	if err != nil {
		return err
	}
	outboundIDs, err := whidentity.NewOutboundIdentities()
	if err != nil {
		return err
	}
	waveStore, err := whpostgres.NewWaves(db)
	if err != nil {
		return err
	}
	pickTaskIDs, err := whidentity.NewPickTaskIdentities()
	if err != nil {
		return err
	}
	agentStore, err := whpostgres.NewAgents(db)
	if err != nil {
		return err
	}
	chargeStore, err := whpostgres.NewCharges(db)
	if err != nil {
		return err
	}
	chargeIDs, err := whidentity.NewChargeIdentities()
	if err != nil {
		return err
	}
	outboxStore, err := outbox.NewStore(db)
	if err != nil {
		return err
	}
	chargeHandoff, err := whpostgres.NewOutboxChargeHandoff(db, outboxStore, opClock)
	if err != nil {
		return err
	}

	holds := whpostgres.LedgerHolds{
		Freezes:      whpostgres.NewFreezeHolds(freezeStore),
		Counts:       countStore,
		Reservations: outboundStore,
		Allocations:  waveStore,
	}

	// 1. 仓库、库区、库位类型、库位、包材五册主数据
	whHandler := whapp.NewRegisterWarehouseHandler(warehouses, clock)
	zoneHandler := whapp.NewRegisterZoneHandler(zones, warehouses, clock)
	locTypeHandler := whapp.NewRegisterLocationTypeHandler(locTypes, warehouses, clock)
	locHandler := whapp.NewRegisterLocationHandler(locations, warehouses, zones, locTypes, clock)
	pkgHandler := whapp.NewRegisterPackagingMaterialHandler(packaging, warehouses, clock)

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		whSpecs := []struct {
			code whdomain.WarehouseCode
			spec whdomain.WarehouseContentSpec
		}{
			{
				code: whSelf,
				spec: whdomain.WarehouseContentSpec{
					Name:             mustRef(whdomain.NewWarehouseName, "深圳宝安一号自营仓（演示）"),
					Kind:             whdomain.WarehouseKindSelfOperated,
					Address:          whdomain.NewWarehouseAddress(mustRef(whdomain.NewCountryCode, "CN"), mustRef(whdomain.NewAddressText, "深圳市宝安区福永物流园区 1 号库（模拟地址）")),
					BusinessTimeZone: mustRef(whdomain.NewBusinessTimeZone, "Asia/Shanghai"),
					ColocatedNode:    mustRef(whdomain.NewColocatedNode, "SYN-NODE-SZX-GATE"),
				},
			},
			{
				code: whAgent,
				spec: whdomain.WarehouseContentSpec{
					Name:             mustRef(whdomain.NewWarehouseName, "新加坡裕廊代理仓（演示）"),
					Kind:             whdomain.WarehouseKindAgent,
					Address:          whdomain.NewWarehouseAddress(mustRef(whdomain.NewCountryCode, "SG"), mustRef(whdomain.NewAddressText, "新加坡裕廊保税物流区 8 号仓（模拟地址）")),
					BusinessTimeZone: mustRef(whdomain.NewBusinessTimeZone, "Asia/Singapore"),
				},
			},
		}
		for _, item := range whSpecs {
			content, err := whdomain.NewWarehouseContent(item.spec)
			if err != nil {
				return err
			}
			res, err := whHandler.Register(txCtx, whapp.RegisterWarehouseCommand{
				Tenant: tenant, Code: item.code, Revision: 1, Content: content, Registrant: regOp,
			})
			if err != nil {
				return err
			}
			if res.Outcome() != whapp.RegistrationRegistered && res.Outcome() != whapp.RegistrationAlreadyRegistered {
				return fmt.Errorf("登记仓 %s：%s", item.code, res.Outcome())
			}
		}

		zoneSpecs := []struct {
			code  string
			name  string
			usage whdomain.ZoneUsage
		}{
			{"SYN-ZONE-RCV", "收货暂存区", whdomain.ZoneUsageReceivingStaging},
			{"SYN-ZONE-STORE", "标准存储区", whdomain.ZoneUsageStorage},
			{"SYN-ZONE-PICK", "快件拣货区", whdomain.ZoneUsagePicking},
			{"SYN-ZONE-OUT", "出库复核暂存区", whdomain.ZoneUsageOutboundStaging},
			{"SYN-ZONE-RET", "退货暂存区", whdomain.ZoneUsageReturnStaging},
			{"SYN-ZONE-DEF", "不良品隔离区", whdomain.ZoneUsageDefective},
		}
		for _, z := range zoneSpecs {
			content, err := whdomain.NewZoneContent(whdomain.ZoneContentSpec{
				Name: mustRef(whdomain.NewZoneName, z.name), Usage: z.usage,
			})
			if err != nil {
				return err
			}
			res, err := zoneHandler.Register(txCtx, whapp.RegisterZoneCommand{
				Tenant: tenant, Warehouse: whSelf, Code: mustRef(whdomain.NewZoneCode, z.code), Revision: 1, Content: content, Registrant: regOp,
			})
			if err != nil {
				return err
			}
			if res.Outcome() != whapp.RegistrationRegistered && res.Outcome() != whapp.RegistrationAlreadyRegistered {
				return fmt.Errorf("登记库区 %s：%s", z.code, res.Outcome())
			}
		}

		for _, lt := range []struct{ code, name string }{
			{"SYN-LT-SHELF", "轻型货架位"},
			{"SYN-LT-PALLET", "重型托盘位"},
			{"SYN-LT-FLOOR", "地面暂存位"},
		} {
			content, err := whdomain.NewLocationTypeContent(whdomain.LocationTypeContentSpec{
				Name: mustRef(whdomain.NewLocationTypeName, lt.name),
			})
			if err != nil {
				return err
			}
			res, err := locTypeHandler.Register(txCtx, whapp.RegisterLocationTypeCommand{
				Tenant: tenant, Code: mustRef(whdomain.NewLocationTypeCode, lt.code), Revision: 1, Content: content, Registrant: regOp,
			})
			if err != nil {
				return err
			}
			if res.Outcome() != whapp.RegistrationRegistered && res.Outcome() != whapp.RegistrationAlreadyRegistered {
				return fmt.Errorf("登记库位类型 %s：%s", lt.code, res.Outcome())
			}
		}

		locSpecs := []struct {
			code     string
			zone     string
			locType  string
			priority int32
		}{
			{"SYN-LOC-RCV-01", "SYN-ZONE-RCV", "SYN-LT-FLOOR", 10},
			{"SYN-LOC-STORE-A01", "SYN-ZONE-STORE", "SYN-LT-PALLET", 50},
			{"SYN-LOC-STORE-A02", "SYN-ZONE-STORE", "SYN-LT-PALLET", 40},
			{"SYN-LOC-PICK-B01", "SYN-ZONE-PICK", "SYN-LT-SHELF", 100},
			{"SYN-LOC-PICK-B02", "SYN-ZONE-PICK", "SYN-LT-SHELF", 90},
			{"SYN-LOC-OUT-01", "SYN-ZONE-OUT", "SYN-LT-FLOOR", 10},
			{"SYN-LOC-RET-01", "SYN-ZONE-RET", "SYN-LT-FLOOR", 10},
			{"SYN-LOC-DEF-01", "SYN-ZONE-DEF", "SYN-LT-FLOOR", 10},
		}
		for _, loc := range locSpecs {
			content, err := whdomain.NewLocationContent(whdomain.LocationContentSpec{
				ZoneCode:     mustRef(whdomain.NewZoneCode, loc.zone),
				LocationType: mustRef(whdomain.NewLocationTypeCode, loc.locType),
				Priority:     whdomain.NewLocationPriority(loc.priority),
			})
			if err != nil {
				return err
			}
			res, err := locHandler.Register(txCtx, whapp.RegisterLocationCommand{
				Tenant: tenant, Warehouse: whSelf, Code: mustRef(whdomain.NewLocationCode, loc.code), Revision: 1, Content: content, Registrant: regOp,
			})
			if err != nil {
				return err
			}
			if res.Outcome() != whapp.RegistrationRegistered && res.Outcome() != whapp.RegistrationAlreadyRegistered {
				return fmt.Errorf("登记库位 %s：%s", loc.code, res.Outcome())
			}
		}

		pkgSpecs := []struct {
			code, name, l, w, h, wt string
		}{
			{"SYN-BOX-S", "标准小号纸箱（演示）", "25", "20", "12", "0.18"},
			{"SYN-BOX-M", "标准中号纸箱（演示）", "35", "25", "20", "0.32"},
			{"SYN-BOX-L", "标准大号纸箱（演示）", "50", "40", "30", "0.55"},
		}
		for _, p := range pkgSpecs {
			content, err := whdomain.NewPackagingMaterialContent(whdomain.PackagingMaterialContentSpec{
				Name:           mustRef(whdomain.NewPackagingMaterialName, p.name),
				Length:         mustRef(whdomain.NewMeasure, p.l),
				Width:          mustRef(whdomain.NewMeasure, p.w),
				Height:         mustRef(whdomain.NewMeasure, p.h),
				DimensionsUnit: mustRef(whdomain.NewUnitCode, "CM"),
				Weight:         mustRef(whdomain.NewMeasure, p.wt),
				WeightUnit:     mustRef(whdomain.NewUnitCode, "KG"),
			})
			if err != nil {
				return err
			}
			res, err := pkgHandler.Register(txCtx, whapp.RegisterPackagingMaterialCommand{
				Tenant: tenant, Code: mustRef(whdomain.NewPackagingMaterialCode, p.code), Revision: 1, Content: content, Registrant: regOp,
			})
			if err != nil {
				return err
			}
			if res.Outcome() != whapp.RegistrationRegistered && res.Outcome() != whapp.RegistrationAlreadyRegistered {
				return fmt.Errorf("登记包材 %s：%s", p.code, res.Outcome())
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// 2. 八项仓配租户规则（全部配置为演示租户 SYN-TENANT-01 取值）
	chargeEarlyHandler := whapp.NewChargeHandler(chargeStore, chargeIDs, chargeHandoff, warehouses, clock).WithReadings(goods, measurements)
	chargeHandler := whapp.NewChargeHandler(chargeStore, chargeIDs, chargeHandoff, warehouses, opClock).WithReadings(goods, measurements)
	disposalRuleHandler := whapp.NewRegisterDisposalRuleHandler(disposalRules, warehouses, clock)
	approvalHandler := whapp.NewCountApprovalHandler(countStore, countStore, nil, holds, clock, warehouses)
	outboundHandler := whapp.NewOutboundHandler(outboundStore, outboundIDs, outboundNums, stock, warehouses, locations, zones, goods, holds, ownerLookup, opClock, warehouses)
	poster := whapp.NewPostStockMovementHandler(stock, warehouses, locations, zones, goods, stockIDs, opClock)
	waveHandler := whapp.NewWaveHandler(waveStore, outboundStore, pickTaskIDs, stockDiscIDs, poster, stock, locations, zones, goods, holds, opClock, warehouses).WithApprovalDuties(countStore).WithCharges(chargeHandler)
	agentHandler := whapp.NewAgentHandler(agentStore, outboundStore, waveStore, warehouses, nil, opClock).WithCharges(chargeHandler)

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		// 2.1 参考分配策略
		if res, err := waveHandler.Adopt(txCtx, whapp.AdoptStrategyCommand{
			Tenant: tenant, Revision: 1, Operator: regOp,
		}); err != nil {
			return err
		} else if res.Outcome() != whapp.WaveStrategyRegistered && res.Outcome() != whapp.WaveExisting {
			return fmt.Errorf("采用分配策略：%s", res.Outcome())
		}
		// 2.2 货主缺货处置规则
		for _, ow := range []whdomain.OwnerAccountID{owner1, owner2} {
			if res, err := outboundHandler.RegisterPolicy(txCtx, whapp.RegisterShortagePolicyCommand{
				Tenant: tenant, Owner: ow, Revision: 1, Kind: whdomain.ShortagePolicyPartial, Operator: regOp,
			}); err != nil {
				return err
			} else if res.Outcome() != whapp.OutboundPolicyRegistered && res.Outcome() != whapp.OutboundExisting {
				return fmt.Errorf("缺货规则 %s：%s", ow, res.Outcome())
			}
		}
		// 2.3 库存调整批准职责规则（要求提交人与批准人不同主体）
		if res, err := approvalHandler.RegisterDuty(txCtx, whapp.RegisterApprovalDutyCommand{
			Tenant: tenant, Revision: 1, Distinct: true, Operator: regOp,
		}); err != nil {
			return err
		} else if res.Outcome() != whapp.ApprovalDutyRegistered && res.Outcome() != whapp.ApprovalExisting {
			return fmt.Errorf("批准职责规则：%s", res.Outcome())
		}
		// 2.4 待认领到货处置规则
		ruleContent := whdomain.NewDisposalRuleContent(30*86400, []whdomain.UnclaimedDisposalMethod{
			mustRef(whdomain.NewUnclaimedDisposalMethod, "DESTROY"),
			mustRef(whdomain.NewUnclaimedDisposalMethod, "RETURN_TO_SENDER"),
			mustRef(whdomain.NewUnclaimedDisposalMethod, "DONATE"),
		})
		if res, err := disposalRuleHandler.Register(txCtx, whapp.RegisterDisposalRuleCommand{
			Tenant: tenant, Warehouse: whSelf, Revision: 1, Content: ruleContent, Registrant: regOp,
		}); err != nil {
			return err
		} else if res.Outcome() != whapp.RegistrationRegistered && res.Outcome() != whapp.RegistrationAlreadyRegistered {
			return fmt.Errorf("待认领处置规则：%s", res.Outcome())
		}
		// 2.5 仓重量体积单位、库龄口径、增值服务项目
		if res, err := chargeEarlyHandler.RegisterUnits(txCtx, tenant, whSelf, "KG", "M3"); err != nil {
			return err
		} else if res.Outcome() != whapp.ChargeFormed && res.Outcome() != whapp.ChargeExisting {
			return fmt.Errorf("计费单位：%s", res.Outcome())
		}
		if res, err := chargeEarlyHandler.RegisterAging(txCtx, tenant, whSelf, "ACTUAL_BATCH"); err != nil {
			return err
		} else if res.Outcome() != whapp.ChargeFormed && res.Outcome() != whapp.ChargeExisting {
			return fmt.Errorf("库龄口径：%s", res.Outcome())
		}
		for _, proj := range []string{"贴平台标", "外观质检", "礼盒加固"} {
			if res, err := chargeEarlyHandler.RegisterProject(txCtx, tenant, whSelf, proj); err != nil {
				return err
			} else if res.Outcome() != whapp.ChargeFormed && res.Outcome() != whapp.ChargeExisting {
				return fmt.Errorf("增值服务项目 %s：%s", proj, res.Outcome())
			}
		}
		// 2.6 代理仓连接器
		if res, err := agentHandler.RegisterConnector(txCtx, tenant, whAgent); err != nil {
			return err
		} else if res.Outcome() != whapp.AgentRegistered && res.Outcome() != whapp.AgentExisting {
			return fmt.Errorf("代理仓连接器：%s", res.Outcome())
		}
		return nil
	}); err != nil {
		return err
	}

	// 3. 货主商品档案与仓测尺寸重量
	goodsHandler := whapp.NewRegisterGoodsHandler(goods, warehouses, stock, ownerLookup, clock)
	measHandler := whapp.NewRecordWarehouseMeasurementHandler(measurements, warehouses, goods, cartons, opClock)

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		type goodsSeed struct {
			owner      whdomain.OwnerAccountID
			code       string
			name       string
			key        string
			barcode    string
			hsCode     string
			price      string
			l, w, h    string
			wt         string
			boxFactor  int64
			trackBatch bool
			trackExp   bool
		}
		items := []goodsSeed{
			{owner1, "SYN-SKU-EARBUD-01", "无线降噪蓝牙耳机（演示）", "DEC-SEED-G1", "6900001000011", "8518300000", "28.50", "12", "9", "5", "0.22", 20, false, false},
			{owner1, "SYN-SKU-CHARGER-02", "氮化镓 65W 快充头（演示）", "DEC-SEED-G2", "6900001000028", "8504409999", "16.00", "10", "7", "4", "0.18", 24, false, false},
			{owner1, "SYN-SKU-MASK-03", "草本舒缓面膜礼盒（演示）", "DEC-SEED-G3", "6900001000035", "3304990099", "19.90", "18", "12", "6", "0.35", 12, true, true},
			{owner2, "SYN-SKU-BOTTLE-01", "不锈钢真空保温杯（演示）", "DEC-SEED-G4", "6900002000018", "9617001900", "14.50", "8", "8", "24", "0.42", 16, false, false},
		}
		for _, g := range items {
			decl, err := whdomain.NewGoodsDeclaration(whdomain.GoodsDeclarationSpec{
				Name:          mustRef(whdomain.NewDeclaredGoodsName, g.name),
				HSCode:        mustRef(whdomain.NewHSCode, g.hsCode),
				OriginCountry: mustRef(whdomain.NewCountryCode, "CN"),
				UnitPrice:     mustRef(whdomain.NewMeasure, g.price),
				Currency:      mustRef(whdomain.NewCurrencyCode, "USD"),
			})
			if err != nil {
				return err
			}
			dims, err := whdomain.NewDeclaredDimensions(
				mustRef(whdomain.NewMeasure, g.l),
				mustRef(whdomain.NewMeasure, g.w),
				mustRef(whdomain.NewMeasure, g.h),
				mustRef(whdomain.NewUnitCode, "CM"),
			)
			if err != nil {
				return err
			}
			wt, err := whdomain.NewDeclaredWeight(mustRef(whdomain.NewMeasure, g.wt), mustRef(whdomain.NewUnitCode, "KG"))
			if err != nil {
				return err
			}
			dm, err := whdomain.NewDeclaredMeasurement(whdomain.DeclaredMeasurementSpec{
				Level:      mustRef(whdomain.NewGoodsUnitName, "件"),
				Dimensions: &dims,
				Weight:     &wt,
			})
			if err != nil {
				return err
			}
			alias := whdomain.NewGoodsAlias(
				mustRef(whdomain.NewGoodsAliasCode, g.barcode),
				whdomain.GoodsAliasBarcode,
				whdomain.PlatformName{},
				whdomain.AliasValidity{},
			)
			aliases := []whdomain.GoodsAlias{alias}
			if g.code == "SYN-SKU-EARBUD-01" {
				aliases = append(aliases, whdomain.NewGoodsAlias(
					mustRef(whdomain.NewGoodsAliasCode, "AMZ-FBA-EARBUD-01"),
					whdomain.GoodsAliasPlatformCode,
					mustRef(whdomain.NewPlatformName, "AMAZON"),
					whdomain.AliasValidity{},
				))
			}
			content, err := whdomain.NewGoodsContent(whdomain.GoodsContentSpec{
				Name:        mustRef(whdomain.NewGoodsName, g.name),
				BaseUnit:    mustRef(whdomain.NewGoodsUnitName, "件"),
				TrackBatch:  g.trackBatch,
				TrackExpiry: g.trackExp,
				PackagingLevels: []whdomain.PackagingLevel{
					whdomain.NewPackagingLevel(mustRef(whdomain.NewGoodsUnitName, "件"), 1),
					whdomain.NewPackagingLevel(mustRef(whdomain.NewGoodsUnitName, "箱"), g.boxFactor),
				},
				Aliases:              aliases,
				Declaration:          decl,
				DeclaredMeasurements: []whdomain.DeclaredMeasurement{dm},
			})
			if err != nil {
				return err
			}
			sub, err := whdomain.NewOnBehalfSubmission(mustRef(whdomain.NewRequestKey, g.key), mustRef(whdomain.NewEvidenceReference, "SYN-EVID-GOODS"))
			if err != nil {
				return err
			}
			res, err := goodsHandler.Register(txCtx, whapp.RegisterGoodsCommand{
				Tenant: tenant, Owner: g.owner, Code: mustRef(whdomain.NewGoodsCode, g.code),
				Content: content, Submission: sub, Registrant: regOp,
			})
			if err != nil {
				return err
			}
			if res.Outcome() != whapp.GoodsOutcomeRegistered && res.Outcome() != whapp.GoodsOutcomeExistingResult {
				return fmt.Errorf("商品 %s：%s", g.code, res.Outcome())
			}

			// 补一条仓测尺寸重量，供后续预取面单与计费读取
			target, err := whdomain.NewGoodsLevelMeasurementTarget(g.owner, mustRef(whdomain.NewGoodsCode, g.code), mustRef(whdomain.NewGoodsUnitName, "件"))
			if err != nil {
				return err
			}
			mRes, err := measHandler.Record(txCtx, whapp.RecordWarehouseMeasurementCommand{
				Tenant: tenant, FactID: mustRef(whdomain.NewMeasurementFactID, "SYN-FACT-MEAS-"+g.code),
				Device: measDevice, Operator: measOp, OccurredAt: opTime, DeviceSequence: 1,
				Warehouse: whSelf, Target: target, Dimensions: dims, Weight: wt,
			})
			if err != nil {
				return err
			}
			if mRes.Outcome() != whapp.RecordMeasurementRecorded && mRes.Outcome() != whapp.RecordMeasurementExistingResult {
				return fmt.Errorf("仓测 %s：%s", g.code, mRes.Outcome())
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// 4. 建账导入：一笔已批准入账（形成丰厚商品库存格与整箱库存），一笔待审批（演示待批队列）
	importHandler := whapp.NewStockImportHandler(stockImports, ownerLookup, warehouses, locations, zones, goods, cartons, cartonIDs, poster, opClock).WithApproval(countStore, warehouses)
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		g1, err := whdomain.NewStockImportGoodsLine(
			mustRef(whdomain.NewLocationCode, "SYN-LOC-PICK-B01"),
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"),
			mustRef(whdomain.NewGoodsUnitName, "件"),
			whdomain.StockStateGood, 120, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.GoodsAliasCode{}, baseTime,
		)
		if err != nil {
			return err
		}
		g2, err := whdomain.NewStockImportGoodsLine(
			mustRef(whdomain.NewLocationCode, "SYN-LOC-PICK-B02"),
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-CHARGER-02"),
			mustRef(whdomain.NewGoodsUnitName, "件"),
			whdomain.StockStateGood, 90, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.GoodsAliasCode{}, baseTime,
		)
		if err != nil {
			return err
		}
		g3, err := whdomain.NewStockImportGoodsLine(
			mustRef(whdomain.NewLocationCode, "SYN-LOC-STORE-A01"),
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-MASK-03"),
			mustRef(whdomain.NewGoodsUnitName, "件"),
			whdomain.StockStateGood, 60,
			mustRef(whdomain.NewBatchNumber, "LOT-202611A"),
			mustRef(whdomain.NewExpiryDate, "2027-11-30"),
			whdomain.GoodsAliasCode{}, baseTime,
		)
		if err != nil {
			return err
		}
		gDef, err := whdomain.NewStockImportGoodsLine(
			mustRef(whdomain.NewLocationCode, "SYN-LOC-DEF-01"),
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-CHARGER-02"),
			mustRef(whdomain.NewGoodsUnitName, "件"),
			whdomain.StockStateDefective, 4, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.GoodsAliasCode{}, baseTime,
		)
		if err != nil {
			return err
		}
		cItem, err := whdomain.NewStockImportCartonContent(mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"), 20)
		if err != nil {
			return err
		}
		c1, err := whdomain.NewStockImportCartonLine(
			mustRef(whdomain.NewLocationCode, "SYN-LOC-STORE-A02"),
			mustRef(whdomain.NewCartonNumber, "SYN-CTN-0001"),
			[]whdomain.StockImportCartonContent{cItem},
			baseTime,
		)
		if err != nil {
			return err
		}
		impPostedID := mustRef(whdomain.NewStockImportID, "SYN-IMP-INIT-01")
		subRes, err := importHandler.Submit(txCtx, whapp.SubmitStockImportCommand{
			Tenant: tenant, ID: impPostedID, Owner: owner1, Warehouse: whSelf,
			Reason:  mustRef(whdomain.NewStockImportReason, "开仓期初库存建账（演示）"),
			Goods:   []whdomain.StockImportGoodsLine{g1, g2, g3, gDef},
			Cartons: []whdomain.StockImportCartonLine{c1}, Submitter: regOp,
		})
		if err != nil {
			return err
		}
		if subRes.Outcome() != whapp.StockImportSubmitted && subRes.Outcome() != whapp.StockImportExistingResult {
			return fmt.Errorf("提交建账导入 01：%s", subRes.Outcome())
		}
		postRes, err := importHandler.Approve(txCtx, whapp.PostStockImportCommand{
			Tenant: tenant, ID: impPostedID, Poster: approverOp,
		})
		if err != nil {
			return err
		}
		if postRes.Outcome() != whapp.StockImportPosted && postRes.Outcome() != whapp.StockImportExistingResult && postRes.Outcome() != whapp.StockImportStatusMismatch {
			return fmt.Errorf("批准建账导入 01：%s", postRes.Outcome())
		}

		// 第二笔保持待批，方便管理台直接演示「批准并入账 / 驳回」
		gPending, err := whdomain.NewStockImportGoodsLine(
			mustRef(whdomain.NewLocationCode, "SYN-LOC-STORE-A01"),
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-BOTTLE-01"),
			mustRef(whdomain.NewGoodsUnitName, "件"),
			whdomain.StockStateGood, 50, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.GoodsAliasCode{}, opTime,
		)
		if err != nil {
			return err
		}
		pendRes, err := importHandler.Submit(txCtx, whapp.SubmitStockImportCommand{
			Tenant: tenant, ID: mustRef(whdomain.NewStockImportID, "SYN-IMP-PENDING-02"), Owner: owner2, Warehouse: whSelf,
			Reason: mustRef(whdomain.NewStockImportReason, "货主二号首批建账待审批（演示）"),
			Goods:  []whdomain.StockImportGoodsLine{gPending}, Submitter: approverOp,
		})
		if err != nil {
			return err
		}
		if pendRes.Outcome() != whapp.StockImportSubmitted && pendRes.Outcome() != whapp.StockImportExistingResult {
			return fmt.Errorf("提交待批建账 02：%s", pendRes.Outcome())
		}
		return nil
	}); err != nil {
		return err
	}

	// 5. 入库预报：1 笔已预报（供作业台收货台点选扫码收货），1 笔收货中并带收货确认与上架作业事实
	preAdviceHandler := whapp.NewAcceptPreAdviceHandler(preAdviceStore, warehouses, warehouses, goods, preAdviceNums, preAdviceIDs, ownerLookup, opClock)
	receiptHandler := whapp.NewConfirmReceiptHandler(
		receipts, preAdviceStore, preAdviceHandler, poster, cartons, cartonIDs,
		warehouses, locations, zones, goodsCat, goods, discrepancyIDs, arrivals, arrivalIDs, opClock,
	).WithCharges(chargeHandler)
	internalJobHandler := whapp.NewRegisterInternalJobHandler(
		internalJobStore, internalJobStore, poster, stock, stock, cartons, cartonIDs,
		warehouses, locations, zones, goods, holds, stockDiscIDs, opClock,
	).WithCharges(chargeHandler)
	unclaimedHandler := whapp.NewRegisterUnclaimedArrivalHandler(arrivals, arrivalIDs, warehouses, opClock)

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		// 5.1 待收货入库预报（ADVISED）
		line1, err := whdomain.NewGoodsInboundLine(
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"), mustRef(whdomain.NewGoodsUnitName, "件"),
			40, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.CartonCount{},
		)
		if err != nil {
			return err
		}
		line2, err := whdomain.NewGoodsInboundLine(
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-CHARGER-02"), mustRef(whdomain.NewGoodsUnitName, "件"),
			30, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.CartonCount{},
		)
		if err != nil {
			return err
		}
		sub1, err := whdomain.NewOnBehalfPreAdviceSubmission(mustRef(whdomain.NewRequestKey, "DEC-SEED-PA-01"), mustRef(whdomain.NewEvidenceReference, "SYN-EVID-PA-01"))
		if err != nil {
			return err
		}
		pa1, err := preAdviceHandler.Accept(txCtx, whapp.AcceptPreAdviceCommand{
			Tenant: tenant, Owner: owner1, Warehouse: whSelf,
			Arrival: whdomain.ArrivalLooseCarton, Inbound: whdomain.InboundByGoods,
			GoodsLines: []whdomain.GoodsInboundLine{line1, line2},
			ExpectedAt: opTime.Add(24 * time.Hour),
			Clue:       mustRef(whdomain.NewArrivalClue, "粤B·SYN01 预计明日上午到仓"),
			Reference:  mustRef(whdomain.NewOwnerReference, "PO-202611-001"),
			Submission: sub1, Registrant: regOp,
		})
		if err != nil {
			return err
		}
		if pa1.Outcome() != whapp.PreAdviceOutcomeAdvised && pa1.Outcome() != whapp.PreAdviceOutcomeExistingResult {
			return fmt.Errorf("入库预报 01：%s", pa1.Outcome())
		}

		// 5.2 收货中的入库预报（已部分收货入账并上架 10 件，留 10 件在收货暂存位）
		line3, err := whdomain.NewGoodsInboundLine(
			mustRef(whdomain.NewGoodsCode, "SYN-SKU-CHARGER-02"), mustRef(whdomain.NewGoodsUnitName, "件"),
			50, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.CartonCount{},
		)
		if err != nil {
			return err
		}
		sub2, err := whdomain.NewOnBehalfPreAdviceSubmission(mustRef(whdomain.NewRequestKey, "DEC-SEED-PA-02"), mustRef(whdomain.NewEvidenceReference, "SYN-EVID-PA-02"))
		if err != nil {
			return err
		}
		pa2, err := preAdviceHandler.Accept(txCtx, whapp.AcceptPreAdviceCommand{
			Tenant: tenant, Owner: owner1, Warehouse: whSelf,
			Arrival: whdomain.ArrivalPallet, Inbound: whdomain.InboundByGoods,
			GoodsLines: []whdomain.GoodsInboundLine{line3},
			ExpectedAt: opTime,
			Clue:       mustRef(whdomain.NewArrivalClue, "首批补货托盘已抵月台"),
			Reference:  mustRef(whdomain.NewOwnerReference, "PO-202611-002"),
			Submission: sub2, Registrant: regOp,
		})
		if err != nil {
			return err
		}
		advice2, ok := pa2.PreAdvice()
		if !ok {
			return fmt.Errorf("入库预报 02 未返回对象：%s", pa2.Outcome())
		}
		rcptLine, err := whdomain.NewReceiptLine(
			"SYN-SKU-CHARGER-02", mustRef(whdomain.NewGoodsUnitName, "件"),
			whdomain.StockStateGood, 20, whdomain.BatchNumber{}, whdomain.ExpiryDate{}, whdomain.GoodsAliasCode{},
			mustRef(whdomain.NewLocationCode, "SYN-LOC-RCV-01"),
		)
		if err != nil {
			return err
		}
		rcptRes, err := receiptHandler.Confirm(txCtx, whapp.ConfirmReceiptCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewReceiptFactID, "SYN-FACT-SEED-RCV-01"),
			Device: pdaDevice, Operator: pdaOp, OccurredAt: opTime,
			Warehouse: whSelf, PreAdvice: advice2.ID(), Shape: whdomain.InboundByGoods,
			Lines: []whdomain.ReceiptLine{rcptLine},
		})
		if err != nil {
			return err
		}
		if rcptRes.Outcome() != whapp.ConfirmReceiptConfirmed && rcptRes.Outcome() != whapp.ConfirmReceiptExistingResult {
			return fmt.Errorf("收货确认 01：%s", rcptRes.Outcome())
		}

		// 5.3 库内上架移库事实（把 10 件从收货暂存移至拣货位 B02）
		moveLine, err := whdomain.NewInternalJobLine(whdomain.InternalJobLineSpec{
			Owner:    owner1,
			Goods:    mustRef(whdomain.NewGoodsCode, "SYN-SKU-CHARGER-02"),
			Unit:     mustRef(whdomain.NewGoodsUnitName, "件"),
			State:    whdomain.StockStateGood,
			Quantity: 10,
			Source:   mustRef(whdomain.NewLocationCode, "SYN-LOC-RCV-01"),
			Target:   mustRef(whdomain.NewLocationCode, "SYN-LOC-PICK-B02"),
		})
		if err != nil {
			return err
		}
		moveRes, err := internalJobHandler.Register(txCtx, whapp.RegisterInternalJobCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewInternalJobFactID, "SYN-FACT-SEED-MOVE-01"),
			Kind: whdomain.InternalJobMove, Device: pdaDevice, Operator: pdaOp,
			OccurredAt: opTime.Add(15 * time.Minute), DeviceSequence: 2, Warehouse: whSelf,
			Lines: []whdomain.InternalJobLine{moveLine},
		})
		if err != nil {
			return err
		}
		if moveRes.Outcome() != whapp.RegisterInternalJobRegistered && moveRes.Outcome() != whapp.RegisterInternalJobExisting {
			return fmt.Errorf("上架移库 01：%s", moveRes.Outcome())
		}

		// 5.4 一件无主待认领到货（UNCLAIMED，供「待认领到货」直接演示认领与处置）
		uncRes, err := unclaimedHandler.Register(txCtx, whapp.RegisterUnclaimedArrivalCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewUnclaimedArrivalFactID, "SYN-FACT-SEED-UNC-01"),
			Device: pdaDevice, Operator: pdaOp, OccurredAt: opTime.Add(20 * time.Minute), DeviceSequence: 3,
			Warehouse: whSelf, Kind: whdomain.UnclaimedArrivalKindInbound,
			Observation:     mustRef(whdomain.NewUnclaimedArrivalObservation, "无唛头棕色瓦楞箱 2 件，快递单尾号 9912，外箱略潮"),
			StorageLocation: mustRef(whdomain.NewUnclaimedArrivalStorageLocation, "收货月台西侧异常暂存笼 #03"),
		})
		if err != nil {
			return err
		}
		if uncRes.Outcome() != whapp.RegisterUnclaimedRegistered && uncRes.Outcome() != whapp.RegisterUnclaimedExistingResult {
			return fmt.Errorf("待认领到货 01：%s", uncRes.Outcome())
		}
		return nil
	}); err != nil {
		return err
	}

	// 6. 一笔库存冻结（在「实时库存 → 占用与冻结」可见）
	freezeHandler := whapp.NewFreezeHandler(freezeStore, freezeIDs, stock, cartons, warehouses, locations, zones, goods, holds, opClock, warehouses)
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		cells, err := stock.ListOnHandCells(txCtx, tenant, owner1, whSelf)
		if err != nil {
			return err
		}
		for _, row := range cells {
			if row.GoodsCode == "SYN-SKU-MASK-03" && row.LocationCode == "SYN-LOC-STORE-A01" {
				spec := whdomain.StockCellSpec{
					Location:     mustRef(whdomain.NewLocationCode, row.LocationCode),
					Goods:        mustRef(whdomain.NewGoodsCode, row.GoodsCode),
					Unit:         mustRef(whdomain.NewGoodsUnitName, row.Unit),
					State:        mustRef(whdomain.NewStockState, row.State),
					InboundBatch: mustRef(whdomain.NewInboundBatchID, row.InboundBatchID),
				}
				if row.BatchNumber != nil && *row.BatchNumber != "" {
					spec.BatchNumber = mustRef(whdomain.NewBatchNumber, *row.BatchNumber)
				}
				if row.Expiry != nil && *row.Expiry != "" {
					spec.Expiry = mustRef(whdomain.NewExpiryDate, *row.Expiry)
				}
				cell, err := whdomain.NewStockCell(spec)
				if err != nil {
					return err
				}
				fRes, err := freezeHandler.Freeze(txCtx, whapp.FreezeCommand{
					Tenant: tenant, DecisionID: mustRef(whdomain.NewRequestKey, "DEC-SEED-FRZ-01"),
					Owner: owner1, Warehouse: whSelf, Target: whdomain.FreezeGoods,
					Cell: cell, Quantity: 5, Reason: "质检抽样留存（演示冻结）", Operator: regOp,
				})
				if err != nil {
					return err
				}
				if fRes.Outcome() != whapp.FreezeFrozen && fRes.Outcome() != whapp.FreezeExisting {
					return fmt.Errorf("冻结库存：%s", fRes.Outcome())
				}
				break
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// 7. 覆盖全出库生命周期的 6 笔出库订单 + 2 个波次（让出库订单、波次与拣货、代理仓履约、作业台拣货车/复核打包台/发货台全部有实时待办）
	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		acceptOB := func(key string, wh whdomain.WarehouseCode, dispatch whdomain.DispatchMethod, service, recipient, addr, goodsCode string, qty int64) (whdomain.OutboundRequest, error) {
			line, err := whdomain.NewOutboundLine(whdomain.OutboundLineSpec{
				Goods: mustRef(whdomain.NewGoodsCode, goodsCode), Unit: mustRef(whdomain.NewGoodsUnitName, "件"),
				State: whdomain.StockStateGood, Quantity: qty,
			})
			if err != nil {
				return whdomain.OutboundRequest{}, err
			}
			res, err := outboundHandler.Accept(txCtx, whapp.AcceptOutboundCommand{
				Tenant: tenant, Owner: owner1, Warehouse: wh, Origin: whdomain.OutboundOriginOperator,
				Key: mustRef(whdomain.NewRequestKey, key), Dispatch: dispatch, Service: service,
				Recipient: recipient, Address: addr, Basis: "SYN-OB-BASIS-" + key, Operator: regOp,
				Lines: []whdomain.OutboundLine{line},
			})
			if err != nil {
				return whdomain.OutboundRequest{}, err
			}
			req, ok := res.Request()
			if !ok {
				return whdomain.OutboundRequest{}, fmt.Errorf("出库单 %s 未落册：%s", key, res.Outcome())
			}
			return req, nil
		}

		// 7.1 两张 RESERVED 订单：留给用户在「出库订单」勾选合并放波次
		if _, err := acceptOB("DEC-SEED-OB-RES-01", whSelf, whdomain.DispatchProductNetwork, "SYN-PROD-CN-SG-EXPRESS", "林小满", "新加坡乌节路 88 号 #12-03（模拟）", "SYN-SKU-EARBUD-01", 3); err != nil {
			return err
		}
		if _, err := acceptOB("DEC-SEED-OB-RES-02", whSelf, whdomain.DispatchOwnerLabel, "", "陈志明", "新加坡淡滨尼中路 18 号（模拟）", "SYN-SKU-CHARGER-02", 4); err != nil {
			return err
		}

		// 7.2 一张 WAVED 订单（波次 WAVE-SEED-OPEN）：有 OPEN 拣货任务，直接在作业台「拣货车」可点击扫码拣货
		obWaveOpen, err := acceptOB("DEC-SEED-OB-WAVED-03", whSelf, whdomain.DispatchOwnerLabel, "", "王嘉禾", "新加坡牛车水新桥路 50 号（模拟）", "SYN-SKU-EARBUD-01", 5)
		if err != nil {
			return err
		}
		w1, err := waveHandler.Release(txCtx, whapp.ReleaseWaveCommand{
			Tenant: tenant, ID: mustRef(whdomain.NewWaveID, "WAVE-SEED-OPEN-01"), Warehouse: whSelf,
			Operator: regOp, Requests: []whdomain.OutboundRequestID{obWaveOpen.ID()},
		})
		if err != nil {
			return err
		}
		if w1.Outcome() != whapp.WaveAllocated && w1.Outcome() != whapp.WaveExisting {
			return fmt.Errorf("释放波次 01：%s", w1.Outcome())
		}

		// 7.3 三张订单推进到 PICKED（待复核）、CHECKED（待打包）、PACKED（待出库发货），让作业台每个工位都有待办
		obPicked, err := acceptOB("DEC-SEED-OB-PICKED-04", whSelf, whdomain.DispatchOwnerLabel, "", "张安然", "新加坡武吉士维多利亚街 20 号（模拟）", "SYN-SKU-CHARGER-02", 2)
		if err != nil {
			return err
		}
		obChecked, err := acceptOB("DEC-SEED-OB-CHECKED-05", whSelf, whdomain.DispatchOwnerLabel, "", "周明宇", "新加坡红山中路 168 号（模拟）", "SYN-SKU-EARBUD-01", 2)
		if err != nil {
			return err
		}
		obPacked, err := acceptOB("DEC-SEED-OB-PACKED-06", whSelf, whdomain.DispatchOwnerLabel, "", "赵思远", "新加坡实龙岗中心 23 号（模拟）", "SYN-SKU-EARBUD-01", 1)
		if err != nil {
			return err
		}

		w2, err := waveHandler.Release(txCtx, whapp.ReleaseWaveCommand{
			Tenant: tenant, ID: mustRef(whdomain.NewWaveID, "WAVE-SEED-FLOW-02"), Warehouse: whSelf,
			Operator: regOp, Requests: []whdomain.OutboundRequestID{obPicked.ID(), obChecked.ID(), obPacked.ID()},
		})
		if err != nil {
			return err
		}
		if w2.Outcome() != whapp.WaveAllocated && w2.Outcome() != whapp.WaveExisting {
			return fmt.Errorf("释放波次 02：%s", w2.Outcome())
		}
		outStage := mustRef(whdomain.NewLocationCode, "SYN-LOC-OUT-01")
		for idx, task := range w2.Tasks() {
			if task.Status != "OPEN" {
				continue
			}
			pRes, err := waveHandler.Pick(txCtx, whapp.PickCommand{
				Tenant: tenant, FactID: mustRef(whdomain.NewPickFactID, fmt.Sprintf("SYN-FACT-SEED-PICK-%02d", idx+1)),
				TaskID: task.ID, Quantity: task.Quantity, Target: outStage, Warehouse: whSelf,
			})
			if err != nil {
				return err
			}
			if pRes.Outcome() != whapp.WavePicked && pRes.Outcome() != whapp.WaveExisting {
				return fmt.Errorf("拣货 %s：%s", task.ID, pRes.Outcome())
			}
		}

		// obChecked 推进到 CHECKED
		if cRes, err := waveHandler.Check(txCtx, whapp.CheckCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewPickFactID, "SYN-FACT-SEED-CHK-05"),
			Request: obChecked.ID(), Observed: 2, Warehouse: whSelf,
		}); err != nil {
			return err
		} else if cRes.Outcome() != whapp.WaveChecked && cRes.Outcome() != whapp.WaveExisting && cRes.Outcome() != whapp.WaveStatusMismatch {
			return fmt.Errorf("复核 05：%s", cRes.Outcome())
		}

		// obPacked 推进到 CHECKED -> PACKED + 登记货主面单（供发货台一键出库确认）
		if cRes, err := waveHandler.Check(txCtx, whapp.CheckCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewPickFactID, "SYN-FACT-SEED-CHK-06"),
			Request: obPacked.ID(), Observed: 1, Warehouse: whSelf,
		}); err != nil {
			return err
		} else if cRes.Outcome() != whapp.WaveChecked && cRes.Outcome() != whapp.WaveExisting && cRes.Outcome() != whapp.WaveStatusMismatch {
			return fmt.Errorf("复核 06：%s", cRes.Outcome())
		}
		if pkRes, err := waveHandler.Pack(txCtx, whapp.PackCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewPickFactID, "SYN-FACT-SEED-PACK-06"),
			Request: obPacked.ID(), Warehouse: whSelf,
			Parcels: []whapp.PackParcelCommand{{
				Sequence: 1, Packaging: "SYN-BOX-S", Weight: "0.40", WeightUnit: "KG",
				Length: "25", Width: "20", Height: "12", Dimension: "CM",
				Goods: mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"), Unit: mustRef(whdomain.NewGoodsUnitName, "件"), Quantity: 1,
			}},
		}); err != nil {
			return err
		} else if pkRes.Outcome() != whapp.WavePacked && pkRes.Outcome() != whapp.WaveExisting && pkRes.Outcome() != whapp.WaveStatusMismatch {
			return fmt.Errorf("打包 06：%s", pkRes.Outcome())
		}
		if lbRes, err := waveHandler.RegisterOwnerLabel(txCtx, whapp.RegisterOwnerLabelCommand{
			Tenant: tenant, Request: obPacked.ID(), Sequence: 1, ExternalNumber: "SYN-EXT-LBL-20261120-01",
		}); err != nil {
			return err
		} else if lbRes.Outcome() != whapp.WaveStrategyRegistered && lbRes.Outcome() != whapp.WaveRecorded && lbRes.Outcome() != whapp.WaveExisting {
			return fmt.Errorf("登记货主面单 06：%s", lbRes.Outcome())
		}

		// 7.4 一笔代理仓出库单（HANDED_OFF）+ 代理仓库存声明，供「代理仓履约」演示转出与回执
		if _, err := acceptOB("DEC-SEED-OB-AGENT-07", whAgent, whdomain.DispatchOwnerLabel, "", "新加坡本地买家（演示）", "新加坡滨海湾大道 10 号（模拟）", "SYN-SKU-EARBUD-01", 2); err != nil {
			return err
		}
		if stRes, err := agentHandler.RecordStatement(txCtx, tenant, whports.AgentStatement{
			Source: "SYN-AGENT-FEED-01", StatedAt: opTime, ValidUntil: opTime.Add(7 * 24 * time.Hour),
			Warehouse: whAgent, Owner: owner1, Goods: mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"),
			Unit: mustRef(whdomain.NewGoodsUnitName, "件"), Quantity: 35,
		}); err != nil {
			return err
		} else if stRes.Outcome() != whapp.AgentValid && stRes.Outcome() != whapp.AgentExisting {
			return fmt.Errorf("代理仓库存声明：%s", stRes.Outcome())
		}

		// 7.5 触发 2026-11-19 一笔仓租日结发生项，在「仓储计费 → 收费发生项」可直接查阅
		if dayRes, err := chargeHandler.CloseStorageDay(txCtx, tenant, owner1, whSelf, time.Date(2026, 11, 19, 0, 0, 0, 0, time.UTC)); err != nil {
			return err
		} else if dayRes.Outcome() != whapp.ChargeFormed && dayRes.Outcome() != whapp.ChargeExisting {
			return fmt.Errorf("仓租日结：%s", dayRes.Outcome())
		}
		return nil
	}); err != nil {
		return err
	}

	// 8. 补齐退货预报与到仓、盘点任务、待批库存调整、库内短少差异与待执行换标指令，使全部 16 个 WMS 页面均有真实可操作的台账和明细
	countIDs, err := whidentity.NewCountIdentities()
	if err != nil {
		return err
	}
	countHandler := whapp.NewCountHandler(countStore, countIDs, stockDiscIDs, warehouses, locations, zones, goods, holds, stock, opClock, warehouses).WithCharges(chargeHandler)
	adjustmentStore, err := whpostgres.NewAdjustments(db)
	if err != nil {
		return err
	}
	adjustmentHandler := whapp.NewAdjustmentHandler(adjustmentStore, countStore, poster, stock, warehouses, locations, zones, goods, ownerLookup, holds, opClock, warehouses).WithCharges(chargeHandler)
	instructionStore, err := whpostgres.NewInstructions(db)
	if err != nil {
		return err
	}
	instructionIDs, err := whidentity.NewInstructionIdentities()
	if err != nil {
		return err
	}
	instructionHandler := whapp.NewInstructionHandler(
		instructionStore, instructionIDs, poster, stock, stock, warehouses, locations, zones, goods, goods,
		ownerLookup, holds, opClock, warehouses, instructionStore, cartons, cartonIDs, stockIDs,
	).WithCharges(chargeHandler)
	returnStore, err := whpostgres.NewReturns(db)
	if err != nil {
		return err
	}
	returnIDs, err := whidentity.NewReturnIdentities()
	if err != nil {
		return err
	}
	returnNums, err := whpostgres.NewReturnNumbers(db, assigner)
	if err != nil {
		return err
	}
	returnHandler := whapp.NewReturnHandler(
		returnStore, returnIDs, returnNums, warehouses, locations, zones, warehouses, ownerLookup, poster, stock,
		unclaimedHandler, arrivals, opClock,
	).WithCharges(chargeHandler)

	if err := db.Transactor().WithinTransaction(ctx, func(txCtx context.Context) error {
		// 8.1 退货预报 + 一件已到仓待回库处置的退件（供「退货管理」直接查看预报明细与一键回库处置）
		retAdv, err := returnHandler.Accept(txCtx, whapp.AcceptReturnCommand{
			Tenant: tenant, Owner: owner1, Warehouse: whSelf, Origin: whdomain.ReturnOriginOperator,
			Key: "DEC-SEED-RET-01", Tracking: "SYN-TRK-RET-9001", Basis: "SYN-RET-BASIS-01", Operator: regOp,
			Lines: []whports.ReturnLine{{Goods: mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"), Unit: mustRef(whdomain.NewGoodsUnitName, "件"), Quantity: 2}},
		})
		if err != nil {
			return err
		}
		if retAdv.Outcome() != whapp.ReturnAccepted && retAdv.Outcome() != whapp.ReturnExisting {
			return fmt.Errorf("退货预报 01：%s", retAdv.Outcome())
		}
		retRec, err := returnHandler.Receive(txCtx, whapp.ReceiveReturnCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewReturnFactID, "SYN-FACT-SEED-RET-01"),
			Warehouse: whSelf, Location: mustRef(whdomain.NewLocationCode, "SYN-LOC-RET-01"),
			Tracking: "SYN-TRK-RET-9001", Goods: mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"),
			Unit: mustRef(whdomain.NewGoodsUnitName, "件"), State: whdomain.StockStatePendingInspection,
			Quantity: 2, OccurredAt: opTime.Add(30 * time.Minute), Operator: pdaOp, Device: pdaDevice,
		})
		if err != nil {
			return err
		}
		if retRec.Outcome() != whapp.ReturnCounted && retRec.Outcome() != whapp.ReturnExisting {
			return fmt.Errorf("退件到仓 01：%s", retRec.Outcome())
		}

		// 8.2 一张已开始进行中的盘点任务（供「盘点与调整」与作业台录清点）
		cnt1, err := countHandler.Create(txCtx, whapp.CreateCountCommand{
			Tenant: tenant, DecisionID: mustRef(whdomain.NewRequestKey, "DEC-SEED-CNT-01"),
			Warehouse: whSelf, Locations: []whdomain.LocationCode{mustRef(whdomain.NewLocationCode, "SYN-LOC-STORE-A02")},
			Owner: owner1, Note: "标准存储区 A02 周度抽盘（演示）", Operator: regOp,
		})
		if err != nil {
			return err
		}
		if c1, ok := cnt1.Count(); ok && c1.Status() == whdomain.CountPending {
			if st, err := countHandler.Start(txCtx, whapp.CountCommand{Tenant: tenant, ID: c1.ID(), Operator: regOp}); err != nil {
				return err
			} else if st.Outcome() != whapp.CountStarted && st.Outcome() != whapp.CountExisting {
				return fmt.Errorf("开始盘点 01：%s", st.Outcome())
			}
		}

		// 8.3 一笔待批库存调整申请（由 approverOp 提交，演示配置员可直接点批准过账或驳回）
		var defBatch whdomain.InboundBatchID
		if defCells, err := stock.ListOnHandCells(txCtx, tenant, owner1, whSelf); err == nil {
			for _, row := range defCells {
				if row.LocationCode == "SYN-LOC-DEF-01" && row.GoodsCode == "SYN-SKU-CHARGER-02" {
					defBatch = mustRef(whdomain.NewInboundBatchID, row.InboundBatchID)
					break
				}
			}
		}
		adjRes, err := adjustmentHandler.Submit(txCtx, whapp.SubmitAdjustmentCommand{
			Tenant: tenant, ID: mustRef(whdomain.NewRequestKey, "SYN-ADJ-SEED-01"),
			Owner: owner1, Warehouse: whSelf, Reason: "库位复核发现外包装破损调减 1 件（演示待批）", Operator: approverOp,
			Lines: []whports.AdjustmentLine{{
				Kind: "DECREASE", Location: mustRef(whdomain.NewLocationCode, "SYN-LOC-DEF-01"),
				Goods: mustRef(whdomain.NewGoodsCode, "SYN-SKU-CHARGER-02"), Unit: mustRef(whdomain.NewGoodsUnitName, "件"),
				State: whdomain.StockStateDefective, Inbound: defBatch, Quantity: 1,
			}},
		})
		if err != nil {
			return err
		}
		if adjRes.Outcome() != whapp.ApprovalSubmitted && adjRes.Outcome() != whapp.ApprovalExisting {
			return fmt.Errorf("提交库存调整 01：%s", adjRes.Outcome())
		}

		// 8.4 一条库内短少作业事实（自动生成 1 条待审批的 stock_discrepancy 差异记录）
		shortLine, err := whdomain.NewInternalJobLine(whdomain.InternalJobLineSpec{
			Owner: owner1, Goods: mustRef(whdomain.NewGoodsCode, "SYN-SKU-CHARGER-02"),
			Unit: mustRef(whdomain.NewGoodsUnitName, "件"), State: whdomain.StockStateGood,
			Quantity: 1, Source: mustRef(whdomain.NewLocationCode, "SYN-LOC-PICK-B02"),
			Note: "巡检发现拣货位实物少 1 件（演示作业差异）",
		})
		if err != nil {
			return err
		}
		shortRes, err := internalJobHandler.Register(txCtx, whapp.RegisterInternalJobCommand{
			Tenant: tenant, FactID: mustRef(whdomain.NewInternalJobFactID, "SYN-FACT-SEED-SHORT-01"),
			Kind: whdomain.InternalJobShortage, Device: pdaDevice, Operator: pdaOp,
			OccurredAt: opTime.Add(35 * time.Minute), DeviceSequence: 4, Warehouse: whSelf,
			Lines: []whdomain.InternalJobLine{shortLine},
		})
		if err != nil {
			return err
		}
		if shortRes.Outcome() != whapp.RegisterInternalJobShortageRegistered && shortRes.Outcome() != whapp.RegisterInternalJobExisting {
			return fmt.Errorf("库内短少 01：%s", shortRes.Outcome())
		}

		// 8.5 一条待执行的换标指令（供「加工与换标」列表查阅与作业台执行）
		relRes, err := instructionHandler.Accept(txCtx, whapp.AcceptInstructionCommand{
			Tenant: tenant, Owner: owner1, Warehouse: whSelf, Kind: whdomain.InstructionRelabel,
			Origin: whdomain.InstructionOriginOperator, Goods: mustRef(whdomain.NewGoodsCode, "SYN-SKU-EARBUD-01"),
			Unit: mustRef(whdomain.NewGoodsUnitName, "件"), Quantity: 10,
			ToAlias:    mustRef(whdomain.NewGoodsAliasCode, "AMZ-FBA-EARBUD-01"),
			DecisionID: mustRef(whdomain.NewRequestKey, "DEC-SEED-REL-01"),
			Operator:   regOp, Reason: "备货转贴平台 FNSKU 标签（演示）", Basis: "SYN-REL-BASIS-01",
		})
		if err != nil {
			return err
		}
		if relRes.Outcome() != whapp.InstructionAccepted && relRes.Outcome() != whapp.InstructionExisting {
			return fmt.Errorf("换标指令 01：%s", relRes.Outcome())
		}
		return nil
	}); err != nil {
		return err
	}

	return nil
}

// walkBilling 灌入演示租户的仓储计费方案、货主采用与已确认收费样本。
func walkBilling(ctx context.Context, db *bentopg.DB) error {
	plans, err := pricingstore.NewWarehousePlans(db)
	if err != nil {
		return err
	}
	store, err := adapter.NewWarehouseCharges(db)
	if err != nil {
		return err
	}
	accounts, err := adapter.NewSettlementAccounts(db)
	if err != nil {
		return err
	}
	mappings, err := adapter.NewFeeItemMappings(db)
	if err != nil {
		return err
	}
	charges, err := adapter.NewCustomerCharges(db)
	if err != nil {
		return err
	}
	conditions, err := adapter.NewChargeConfirmationConditions(db)
	if err != nil {
		return err
	}
	moments, err := adapter.NewSettlementMoments(db)
	if err != nil {
		return err
	}
	supersessions, err := adapter.NewChargeSupersessions(db)
	if err != nil {
		return err
	}
	momentHandler, err := application.NewAdmitSettlementMomentHandler(moments)
	if err != nil {
		return err
	}
	when := time.Date(2026, 11, 20, 8, 0, 0, 0, time.UTC)
	pricing := seedPricing{handler: pricingapp.NewWarehousePlanHandler(plans, true, &countingIDs{prefix: "WEV-SEED"})}
	handler := application.NewWarehouseChargeHandler(
		store, pricing, accounts, mappings, charges, conditions, conditions, momentHandler, supersessions,
		&countingIDs{prefix: "CHG-SEED"}, seedClock{at: when},
	)
	tenant, err := domain.NewTenantID("SYN-TENANT-01")
	if err != nil {
		return err
	}
	account, err := seedAccount()
	if err != nil {
		return err
	}
	return db.Transactor().WithinTransaction(ctx, func(ctx context.Context) error {
		if effect, err := accounts.Save(ctx, tenant, account, when); err != nil {
			return err
		} else if effect != ports.SettlementAccountRegistered && effect != ports.SettlementAccountReplay {
			return fmt.Errorf("结算账户 %v", effect)
		}
		if outcome, err := pricing.handler.Register(ctx, tenant.String(), seedPlan()); err != nil {
			return err
		} else if outcome != pricingapp.WarehousePlanRecorded && outcome != pricingapp.WarehousePlanExisting {
			return fmt.Errorf("方案 %s", outcome)
		}
		for _, ow := range []string{"SYN-OWNER-01", "SYN-ACCOUNT-01"} {
			adopted, err := handler.Adopt(ctx, tenant, application.WarehouseAdoption{
				Owner: ow, Warehouse: "SYN-WH-01", Revision: 1,
				Effective: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
				Account:   account.ID().String(), Scope: "SYN-SCOPE-WH", Basis: "SYN-BASIS", Registrant: "operator-1",
			})
			if err != nil {
				return err
			}
			if adopted != application.WarehouseChargeFormed && adopted != application.WarehouseChargeExisting {
				return fmt.Errorf("采用 %s：%s", ow, adopted)
			}
		}
		if err := saveMapping(ctx, mappings, tenant, "INBOUND_PIECE", when); err != nil {
			return err
		}
		if err := saveMapping(ctx, mappings, tenant, "STORAGE_PIECE", when); err != nil {
			return err
		}
		feeItem, err := domain.NewFeeItemReference("SYN-FEE-WH")
		if err != nil {
			return err
		}
		if _, err := conditions.RegisterCondition(ctx, tenant, feeItem, "WAREHOUSE_WORK_OCCURRED", when); err != nil {
			return err
		}
		if effect, err := moments.SaveSettlementMoment(ctx, tenant, domain.SettlementMomentConfirm, when); err != nil {
			return err
		} else if effect != ports.CatalogueRegistered && effect != ports.CatalogueReplay {
			return fmt.Errorf("确认时点 %v", effect)
		}
		if err := settleConfirmed(ctx, handler, charges, tenant, seedInbound(), 400); err != nil {
			return err
		}
		return settleConfirmed(ctx, handler, charges, tenant, seedStorage(), 100)
	})
}

func settleConfirmed(
	ctx context.Context,
	handler *application.WarehouseChargeHandler,
	charges interface {
		FindByID(context.Context, domain.TenantID, domain.CustomerChargeID) (domain.CustomerCharge, bool, error)
	},
	tenant domain.TenantID,
	fact application.WarehouseOccurrenceFact,
	wantMinor int64,
) error {
	outcome, chargeID, err := handler.Settle(ctx, tenant, fact)
	if err != nil {
		return err
	}
	if outcome != application.WarehouseChargeConfirmed && outcome != application.WarehouseChargeExisting {
		return fmt.Errorf("%s %s", fact.ID, outcome)
	}
	id, err := domain.NewCustomerChargeID(chargeID)
	if err != nil {
		return err
	}
	stored, found, err := charges.FindByID(ctx, tenant, id)
	if err != nil {
		return err
	}
	if !found || stored.Stage() != domain.ChargeConfirmed {
		return fmt.Errorf("%s 未确认", fact.ID)
	}
	if _, minor := stored.OriginalAmount(); minor != wantMinor {
		return fmt.Errorf("%s 金额 %d", fact.ID, minor)
	}
	return nil
}

func saveMapping(ctx context.Context, mappings *adapter.FeeItemMappings, tenant domain.TenantID, code string, at time.Time) error {
	mapping, err := domain.NewFeeItemMapping(
		mustRef(domain.NewPricingChargeCode, code),
		mustRef(domain.NewFeeItemReference, "SYN-FEE-WH"),
		mustRef(domain.NewFeeItemMappingBasisReference, "SYN-BASIS"),
	)
	if err != nil {
		return err
	}
	effect, err := mappings.SaveFeeItemMapping(ctx, tenant, mapping, at)
	if err != nil {
		return err
	}
	if effect != ports.CatalogueRegistered && effect != ports.CatalogueReplay {
		return fmt.Errorf("映射 %s %v", code, effect)
	}
	return nil
}

func seedAccount() (domain.SettlementAccount, error) {
	key, err := domain.NewSettlementAccountKey(
		mustRef(domain.NewLegalEntityReference, "SYN-LE-WH"),
		mustRef(domain.NewSettlementCounterpartyReference, "SYN-OWNER-01"),
		domain.ChargeReceivable,
		mustRef(domain.NewCurrencyCode, "CNY"),
		mustRef(domain.NewSettlementPolicyReference, "SYN-POL-WH"),
	)
	if err != nil {
		return domain.SettlementAccount{}, err
	}
	return domain.NewSettlementAccount(
		mustRef(domain.NewSettlementAccountID, "SYN-ACCT-WH"),
		key, domain.SettlementCounterpartyReference{}, false,
		mustRef(domain.NewResponsibilityBasis, "SYN-CONTRACT-WH"),
	)
}

func seedPlan() pricingdomain.WarehousePlan {
	return pricingdomain.WarehousePlan{
		ID: "SYN-PLAN-WH-01", Version: "1", Scope: "SYN-SCOPE-WH", Currency: "CNY", DeclaredValue: "REFUSE",
		EffectiveFrom: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC),
		Groups: []pricingdomain.WarehouseChargeGroup{
			{Name: "INBOUND", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", OnMiss: "PENDING"},
			{Name: "STORAGE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", OnMiss: "PENDING"},
		},
		Rules: []pricingdomain.WarehouseChargeRule{
			{Code: "INBOUND_PIECE", Kind: "INBOUND_HANDLING", Level: "OCCURRENCE", Group: "INBOUND", Priority: 1, Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 100, Rounding: "EXACT"},
			{Code: "STORAGE_FREE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 1, Method: "WAIVE", When: []pricingdomain.WarehouseCondition{
				{MeasureUnit: "DAY", Operator: "LT", Threshold: "30"},
			}},
			{Code: "STORAGE_PIECE", Kind: "STORAGE", Level: "LINE", Line: "INBOUND_BATCH", Group: "STORAGE", Priority: 1, Method: "CHARGE", Unit: "PIECE", UnitQuantity: "1", UnitPrice: 50, Rounding: "EXACT", When: []pricingdomain.WarehouseCondition{
				{MeasureUnit: "DAY", Operator: "GE", Threshold: "30"},
				{MeasureClass: "VOLUME", MeasureUnit: "M3", Operator: "GT", Threshold: "0"},
			}},
		},
	}
}

func seedInbound() application.WarehouseOccurrenceFact {
	return application.WarehouseOccurrenceFact{
		ID: "CHO-SEED-IN", Version: 1, Kind: "INBOUND_HANDLING", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01",
		Occurred: time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC), Correction: "ORIGINAL",
		Measures: []application.WarehouseFactMeasure{{Unit: "PIECE", Quantity: "4"}},
	}
}

func seedStorage() application.WarehouseOccurrenceFact {
	return application.WarehouseOccurrenceFact{
		ID: "CHO-SEED-STORE", Version: 1, Kind: "STORAGE", Owner: "SYN-OWNER-01", Warehouse: "SYN-WH-01",
		Occurred: time.Date(2026, 11, 20, 1, 0, 0, 0, time.UTC), Correction: "ORIGINAL",
		Lines: []application.WarehouseFactLine{
			{Kind: "INBOUND_BATCH", Measures: []application.WarehouseFactMeasure{
				{Unit: "PIECE", Quantity: "9"},
				{Class: "VOLUME", Unit: "M3", Quantity: "1.2"},
				{Unit: "DAY", Quantity: "10"},
			}},
			{Kind: "INBOUND_BATCH", Measures: []application.WarehouseFactMeasure{
				{Unit: "PIECE", Quantity: "2"},
				{Class: "VOLUME", Unit: "M3", Quantity: "0.4"},
				{Unit: "DAY", Quantity: "30"},
			}},
		},
	}
}

type seedPricing struct {
	handler *pricingapp.WarehousePlanHandler
}

func (bridge seedPricing) Price(ctx context.Context, tenant, scope string, at time.Time, fact application.WarehouseOccurrenceFact) (application.WarehouseChargePrice, error) {
	snapshot := pricingdomain.WarehouseOccurrenceSnapshot{
		ID: fact.ID, Version: fact.Version, Kind: fact.Kind, Owner: fact.Owner, Warehouse: fact.Warehouse, Occurred: fact.Occurred,
		Correction: fact.Correction,
	}
	for _, measure := range fact.Measures {
		snapshot.Measures = append(snapshot.Measures, seedMeasure(measure))
	}
	for _, line := range fact.Lines {
		mapped := pricingdomain.WarehouseOccurrenceLine{Kind: line.Kind}
		for _, measure := range line.Measures {
			mapped.Measures = append(mapped.Measures, seedMeasure(measure))
		}
		snapshot.Lines = append(snapshot.Lines, mapped)
	}
	outcome, evaluation, id, err := bridge.handler.Price(ctx, tenant, scope, at, snapshot)
	if err != nil {
		return application.WarehouseChargePrice{}, err
	}
	status := string(outcome)
	switch outcome {
	case pricingapp.WarehousePlanNotConfigured:
		status = "NOT_CONFIGURED"
	case pricingapp.WarehousePlanPending:
		status = "PENDING"
	case pricingapp.WarehousePlanConflict:
		status = "CONFLICT"
	case pricingapp.WarehousePlanZero:
		status = "ZERO"
	case pricingapp.WarehousePlanPriced:
		status = "COMPLETED"
	}
	return application.WarehouseChargePrice{
		EvaluationID: id, Status: status, TotalMinor: evaluation.TotalMinor, Currency: evaluation.Currency, Codes: evaluation.Codes,
	}, nil
}

func seedMeasure(measure application.WarehouseFactMeasure) pricingdomain.WarehouseMeasure {
	return pricingdomain.WarehouseMeasure{
		Unit: measure.Unit, Quantity: measure.Quantity, Unmeasured: measure.Unmeasured,
		Declared: measure.Declared, DeclaredUnit: measure.DeclaredUnit, Class: measure.Class,
	}
}

type countingIDs struct {
	prefix string
	n      int
}

func (ids *countingIDs) Next() (string, error) {
	ids.n++
	return fmt.Sprintf("%s-%d", ids.prefix, ids.n), nil
}

type seedClock struct{ at time.Time }

func (clock seedClock) Now() time.Time { return clock.at }

func mustRef[T any](parse func(string) (T, error), value string) T {
	parsed, err := parse(value)
	if err != nil {
		panic(err)
	}
	return parsed
}
