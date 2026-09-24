# 04 审批职责规则、批准与发布

Category: enhancement
Status: ready-for-agent——2026-09-25 通道 3 立票并激活（用户授权自决）
Blocked by: 03
地盘：
- `migrations/parcel_pricing/`；
- `internal/parcelpricing/`：审批职责规则、操作者主体消费面、批准与发布编排及 HTTP；
- `cmd/parcel-api/`；
- `docs/product/PILOT-PARAMETER-REGISTER.md` 一行；
- `docs/product/MECHANISM-INVENTORY.md` 重生成。
出处：[spec](../spec.md) 自决第 2 格；ADR-0101 决定五、六；ADR-0126 决定三。

## 要做的

1. **审批职责规则**（PP 自有，按租户）：录入者与批准者须否为不同主体、批准者须持哪一格授予。
   - 立读口与表；写口只给测试用。
   - 两格都不要求也是一条合法的租户声明。
2. **操作者主体消费面**：主体引用加授予集，由 Intake 把信封译成它。形状同 PC 的 `OperatorSubject`，各上下文各立一个。
3. **批准**：只接 `已校验`。
   - 先读审批职责规则：未登记即答`未配置`、不放行；主体相同或授予不足各答一格。
   - 通过后记批准者与批准时刻，转 `已批准`。
4. **发布**：只接 `已批准`。
   - 用草稿上的方案快照、源文件身份、方向授权引用，加上作为 `publicationApprover` 的批准者主体，立登记，交既有 `RegisterPriceCard`。
   - 不重解析、不重算（ADR-0101 决定四）。
   - 答案代数一格不改；落定后草稿转 `已发布`。
5. **端点**：`POST /pricing-price-card-draft-approvals` 与 `POST /pricing-price-card-draft-publications`，挂 `UnconfiguredIntake{}`。载荷只收草稿引用。
6. **参数登记册**增一行「价卡发布审批职责规则」：实例半边，租户取值留空，答`未配置`。

## 验收

- 批准门每一格有测试：未配置、主体相同、授予不足、通过、状态不对。
- 发布路径有测试：交给登记用例的摘要与草稿上的逐字节相等；登记用例的每格答复原样交回。
- Postgres 适配器带 DSN 测试；端点表测试含新行；机制清点重生成。

## 形态

碰 Go、SQL 与端点表，走并行会话那条路。
