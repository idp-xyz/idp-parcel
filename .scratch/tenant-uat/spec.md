# 空租户逐页验收（国际小包工作区）

Category: enhancement
Status: in-progress

出处：用户 2026-11-27「我想按照一个全新的租户进来，按照系统设置，实际的顺序，一个一个的来验收页面，我自己介入一个一个页面和功能点的确认，直到全部验收」。验收人是用户本人；本目录是验收台账，会话只备卡、记结论、修问题。

## 用户裁定（2026-11-27）

- **范围**：只验国际小包工作区；仓配系统另起一轮。
- **填什么**：按会话给的填写卡填，取值沿用演示数据故事里的对象与编号（`scripts/demo-seeds/README.md`「数据故事」），好让后面的站能真跑通一票货；个别页想试别的值随时可以。
- **节奏**：每个问题修完、复验通过，才进下一个功能点。
- **环境**：另起一套独立环境，不动 `:5173` 上别的会话在用的那一套。

## 结论记法

每个功能点一行，结论只有四种：

| 记号 | 含义 |
|---|---|
| ✅ | 通过 |
| ❌ | 不通过：立一张 `issues/NN-*.md`，修完复验通过后改记 ✅ 并留票号 |
| ◑ | 界面缺入口：页面上做不了，由会话用受控命令行代做以便往下走；要不要补入口由用户裁，裁定写在该行 |
| ⏸ | 用户暂不裁，写明等什么 |

一站的全部功能点都不是空白、且没有 ❌，这一站才算过；全部站都过，验收完成。

## 修复怎么落

- 修复在验收检出 `idp-parcel-uat`（分支 `cursor/tenant-uat`）上做，用户在 `:5174` 当场复验。
- diff 全在 `apps/admin-web/**` 与本目录的，照 `docs/agents/workflow.md`「前端切片」的门（三道门、碰共享面要评审）；碰到 Go、SQL 或领域文档的，照同一文件「进 main 的步骤」，这种票不挡验收往下走的判断由用户逐张裁。
- 每笔先重放到 `origin/main` 再推确切 SHA。

## 环境

仅限本机隔离环境，全部是 `SYN-` 合成值，证据层级只记 `S`。

| 件 | 取值 |
|---|---|
| 管理台 | `http://localhost:5174`（要按 `localhost` 打开，回调地址逐字匹配） |
| 后端 | `parcel-api` 在 `:19082`，`parcel-dispatch` 同库 |
| 库 | `55432` 上的 `uat_tenant_01` |
| 登录发行方 | 容器 `idp-parcel-uat-issuer`，`http://127.0.0.1:5557/dex`，只属本环境 |
| 验收检出 | `/home/tops/workspace/idp-parcel-uat`，搭建时钉 `origin/main` 的 `c33a10867` |
| 运行目录 | `/home/tops/workspace/idp-parcel-uat-run`（仓外）：搭建脚本、二进制、日志、PID、开通包输入 |

登录账号（口令都是合成值 `syn-demo-password`）：

| 邮箱 | 开通时的状态 |
|---|---|
| `syn-operator-config-01@syn-tenant-01.example.invalid` | 在册，只持「操作者接入管理」一格授予——租户的首个管理员 |
| `syn-operator-viewer-01@syn-tenant-01.example.invalid` | 不在册，留给验收人在「操作者与授予」页登记 |
| `syn-operator-finance-01@syn-tenant-01.example.invalid` | 不在册，同上；演示种子里没有这个人 |

**开通包里有什么。** 只有产品设计里归实施方的几样：迁移；第一个操作者与它的「操作者接入管理」授予（ADR-0363 决定四）；平台侧的治理权威区间与准入范围对照（命令逐字取自各种子步骤文件，只留这两种登记）。主数据一行不灌。搭好时实测：`access_identity.operator` 1 行，`party_commercial`、`parcel_pricing`、`network_routing`、`customs_compliance`、`visibility_exception`、`settlement_accounting`、`parcel_shipment`、`transport_fulfillment`、`node_operations`、`collection_remittance` 各 schema 全部表合计 0 行。

**与 main 的两行差异。** 验收检出里 `apps/admin-web/src/auth/oidc.ts` 的 `ISSUER` 与 `apps/admin-web/vite.config.ts` 的 `oidcTarget` 指到 `:5557`。只属本环境、不进 main，已用 `git update-index --skip-worktree` 藏起；修复若要动这两个文件，先 `--no-skip-worktree`、分块提交、再藏回去。

**不设隔离读写开关。** `parcel-api` 起时没有设 `IDP_PARCEL_ISOLATED_READ_TENANT` 与 `IDP_PARCEL_ISOLATED_WRITE_TENANT`，读写都只凭操作者册上的授予——与演示脚本 `demo-run/up.sh` 不同，那里两个开关都开着。验收里遇到「未接通」「未配置」时先分清是这个原因还是真缺口。

## 验收站

顺序按配置依赖排。站内各页的先后在各站的验收卡里定。

| 站 | 页面 | 卡 | 状态 |
|---|---|---|---|
| 1 进门 | 登录、外壳、工作台、最近打开、保存的视图、没有授予时各页怎么说 | [01](./stations/01-entry.md) | 首走未过：15 通过 / 5 不通过。五处已改，2026-11-27 复验对上，整站未再走一遍 |
| 2 账号与基础 | 操作者与授予、基础数据、单据编号；授予到位后各页空态巡一遍 | [02](./stations/02-accounts.md) | 暂停。先按 [授予对照](./grant-matrix.md) 把人授齐，再继续 |
| 3 公司主体 | 公司主体 | 待出 | — |
| 4 往来单位 | 往来单位 | 待出 | — |
| 5 产品与商业规则 | 服务产品、商业规则、审批与发布、客户与合同、供应商协议、渠道产品 | 待出 | — |
| 6 价格 | 汇率与燃油费率、价格表、报价发出、计价试算 | 待出 | — |
| 7 线路与运力 | 网络与网点、服务区域 | 待出 | — |
| 8 清关配置 | 口岸与申报路径、合规规则、合规限制与税费 | 待出 | — |
| 9 追踪与对客规则 | 追踪规则、通知与披露、索赔资格与授权 | 待出 | — |
| 10 账务与设备 | 扫描设备，以及账务设置 | 待出 | — |
| 11 第一票运单 | 新建与撤回、新建平台货件、运单、待受理复核、受限运单处置、渠道择优、面单、路线规划、网点作业、运输班次、外部轨迹时间确认、物流动态 | 待出 | — |
| 12 清关 | 清关单 | 待出 | — |
| 13 异常与售后 | 取消运单、货物处置、资金释放待处理、异常处理、异常案件、索赔与追偿 | 待出 | — |
| 14 账务结果 | 计费记录、客户费用、对账单、供应商账单与应付、收付款核销、代收货款、经营核算 | 待出 | — |
| 15 收尾 | 试点治理；用查阅员账号复走；深色主题与窄屏抽查 | 待出 | — |

相对开工前给用户看的那一版改了一处：「各页空态巡一遍」从第 1 站挪到第 2 站授予到位之后。首个管理员此刻只有账号管理一格授予，各页答的是「没有授予」而不是「没有数据」，空态要等查阅授予登上才看得到。

## 子票

验出问题时在 `issues/` 下立，编号从 `01` 起，这里登记。

| 票 | 题 | 出自 | 状态 |
|---|---|---|---|
| [01](./issues/01-command-palette-speaks-english.md) | 命令面板的提示是英文 | 第 1 站 1.7 | resolved |
| [02](./issues/02-shipment-views-still-on-the-isolated-read.md) | 运单三个读口仍挂隔离读 | 第 1 站 1.13、1.19 | resolved |
| [03](./issues/03-saved-views-has-an-entry-before-the-feature.md) | 保存的视图未开放已有入口 | 第 1 站 1.16 | resolved |
| [04](./issues/04-no-grant-copy-and-the-write-form-still-opens.md) | 没有授予的说明与仍可点开的写表单 | 第 1 站 1.18 | resolved |
| [05](./issues/05-workbench-does-not-say-what-a-new-admin-does-first.md) | 工作台不告诉新管理员先去授权 | 第 1 站 1.13 | resolved |

## 进 main

代码在 `281a5d33a`，已是 `origin/main`。分支出处是 `cursor/tenant-uat` 的 `a003a8293`。重放时台账留在分支上，本笔补进。

- 评审：标准轴无阻断。票面轴一处阻断，从「保存的视图」点回去时，还挂着的列表不把筛选写回地址。已在 `a003a8293` 修。探针「地址筛选的跳转带上查询串」通过。
- 全量真库测试：同一份 41 个文件的两笔重放 `09de199a4`、`7598d9ecc` 都是 `FULL-EXIT=0`。推上的 `281a5d33a` 与 `7598d9ecc` 的这 41 个文件相同，父提交只多了退货翻页。对 `281a5d33a` 本身未再跑。
- 种子：一次性库灌入退 0，用后删库。
- 本笔只补台账，全量测试未跑。

## Comments

- 2026-11-27：立项。环境搭好并实测：后端取证（首个管理员读 `GET /operators` 答 200，读 `GET /network-catalog?family=node` 答 403 `OPERATOR_NOT_GRANTED`，不带令牌答 401）；会话在 Cursor 自带浏览器里用首个管理员走通一次登录，落到工作台。搭建脚本 `10-backend.sh`、`20-frontend.sh` 在运行目录。
- 2026-11-27：第 1 站由本会话代走。15 通过、5 不通过，五张票见上表。02、03 等裁定，不改代码。
