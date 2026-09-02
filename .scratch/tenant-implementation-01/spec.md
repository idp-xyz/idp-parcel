# 首个租户实施材料（tenant-implementation-01）

Category: chore
Status: in-progress——材料四件已落（见下）；两处待裁项已于 2026-09-01 由用户裁定（均选 B，结果回写在简报与检查单 B 组），本 spec 仍不关：两项的落文分别归产品权威侧与 ADR-0021 地盘方，且检查单 A 组证据处置未完

## 背景

用户于 2026-08-31 经频道 4 告知：首个租户候选已确认按产品现有系统流程运营（改造其原有流程），期望下周启动实施；客户方证据五件（需求 xlsx、17track 对接三份、真实下单 API 抓包一份）暂存于仓库工作区一个**未被 git 跟踪**的受限目录，指纹索引见[登记册草案](./instance-register-draft.md)，敏感处置见[决策简报](./decision-brief-scope-and-frontline.md)。本 feature 只承载**实施材料**：对照、登记与待裁决项，不承载任何实现工。

材料取证基线：`2ab7f89`（2026-08-31，ADR-0085 首切片已落主线之后）。文中所有「当前状态」断言以该锚为准。

## 产物

| 文件 | 内容 | 性质 |
|---|---|---|
| [requirement-mapping.md](./requirement-mapping.md) | 客户四条业务流逐项 → 系统锚点（上下文/UC/端点/页面）与判定 | 取证对照，不拍板 |
| [instance-register-draft.md](./instance-register-draft.md) | 该租户实例登记册**增量草案**：只记本轮证据触达的行，模板与登记规则以 `docs/product/PILOT-PARAMETER-REGISTER.md` 为唯一权威，不复制表 | 草案，正式实例登记册的建立与落位归用户 |
| [decision-brief-scope-and-frontline.md](./decision-brief-scope-and-frontline.md) | 两处范围冲突（尾程面单渠道 vs PAR-COM-12；COD 字段确认）、一线作业过渡方案选项、gk.idp.xyz 待信息、证据文件安全处置建议 | 决策简报，裁决归用户；裁决项 1、2 已裁并回写，落文归各权威方 |
| [implementation-checklist.md](./implementation-checklist.md) | 简报末节五步节奏的可勾形态：A 证据处置 / B 裁决 / C accessidentity / D 建册 / E 价卡取证 / F 操作员登录门 / G 业务量基线，每项标半边、归属、前置与完成判据 | 检查单，勾选与执行归各归属方 |

## 本 feature 不做

- 不改代码、不改 `docs/**`、不碰装配四件与 page-registry、不动任何索引文件（占号广播于 2026-08-31 发往全部 11 个频道）。地盘 2026-09-01 经 MCP-1 分派扩到 `.scratch/syn-wall-door-audit/**`：本 feature 的 accessidentity 排期结论写在那边的票 01 票面上，不在本目录另存一份。
- 不裁决范围冲突（PILOT-SCOPE / 参数登记册的范围决定属产品权威，改动要走该文档自己的修订纪律）。
- 不替客户确认任何参数为「已确认」——按登记册登记规则，证据完整且决定生效前最高只到「待核验」。

## Comments

- 2026-08-31 MCP-4：三件材料初版落盘。前置对照工作（管理台完整性审查、后端能力盘点、客户文档解析）见频道 4 会话记录；客户 xlsx 十六张工作表已逐表解出（含 13,092 行商派邮编覆盖），解析产物在会话临时目录，未入仓。
- 2026-08-31 MCP-4：应频道 2 知会（原始抓包含明文 apikey/PII/真实单号，红线「敏感实例外置」），三件材料改**指纹索引制**：证据只记 SHA-256 短指纹与脱敏描述，不复录客户名称、域名、单号；`.gitignore` 守卫由 MCP-2 加（其工作树，待提交）。apikey 轮换升为硬性动作，见简报处置节。
- 2026-09-01 MCP-4：用户裁定两处待裁项（均选 B），经 MCP-2 裁决广播转达，本会话按地盘回写[简报](./decision-brief-scope-and-frontline.md)裁决项 1、2 与[检查单](./implementation-checklist.md) B 组。裁决 1 = 修订 `PAR-COM-12`、尾程流入首发（C「按专线退化形态建模」已明确否决：拿领域模型迁就范围决定会污染统一语言）；裁决 2 = 管理台代录/受控导入顶岗，须带拆除期限与 ADR-0021 偏离记录。**B1、B2 两项仍未勾**——落文分别归产品权威侧（`PILOT-SCOPE.md` + 登记册 `PAR-COM-12` 行）与 ADR-0021 地盘方，本 feature 明写不裁范围也不代改。仅 `.scratch/` 三份文件有改动，未开代码，未提交。
- 2026-09-01 MCP-4：第四件[实施检查单](./implementation-checklist.md)落盘，同批在票 [`syn-wall-door-audit/01`](../syn-wall-door-audit/issues/01-access-channel-registry-and-first-real-intake.md) 写实 accessidentity 重启范围（S0..S5）。取证重锚 `1665fdb`，三处口径随之改准：①「重启条件已满足」拆成两条门槛——ADR-0072 要的现行流程已到，`PAR-INT-01` 最低证据另有两件未到，后者决定登记册的列；②简报里 `PILOT-SCOPE-DECISIONS.md` 是断链，范围决定的权威处是 `PILOT-SCOPE.md`；③新增一条实施周硬事实：隔离读准入只收 `SYN-` 前缀租户，票 01 落地前真实租户在管理台上既下不了单也读不到数据（检查单 F2）。未开代码，未提交。
