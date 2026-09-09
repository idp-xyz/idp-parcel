# 18 `CUSTOMER_SERVICE_RULE` 版本的运营主路径：逐字段表单（适用对象恰一 + 期限表 + 材料表）——先等它的读面

Category: enhancement
Status: draft——形状已裁清（逐字段表单 + 两张子表可加行），但**等一件裁决**：管理台要不要先立并落客户服务规则册（读面），票 [06](./06-pre-acceptance-financial-control-policy-versions-have-no-read-face.md) 完成记录末尾已交 MCP-1 定；那张读面票立了且 resolved、本票补上它的编号进 Blocked by 之后，才转 ready-for-agent。伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 复核：`apps/admin-web/src` 里 `CUSTOMER_SERVICE_RULE` 仍零命中）。伞票原写九类，`CommercialObjectKind` 在 `95182b9d` 上已是十类（pc-gaps/04 把客户服务规则版本纳入封闭集），本票补第十册
Blocked by: 08（已进 main）；[21](./21-customer-service-rule-register-read-face.md)（管理台客户服务规则册读面，2026-09-09 通道 1 代裁立票——用户授权自决；21 resolved 后本票转 ready-for-agent，由 21 的作者同笔改）；[20](./20-publication-vocabulary-read-face.md)（词表读口，已进 main）

## 册与载荷

后端读面是第八册 `?kind=CUSTOMER_SERVICE_RULE`（0023，ADR-0104），**管理台今天没有这本册**。`declarations.
customerServiceRuleBody{serviceProduct | customerContract, responsible, scope, claimDeadlines[{kind, startEvent, days,
calendar}], minimumMaterials[{claimKind, materials[]}]}`：适用对象恰一（产品或合同）、两张子表至少一项有内容。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，两张子表可加行。** 频次低、配置员操作、正文是两个引用 + 两张几行的表，不是矩阵。适用对象用二选一
控件呈现、恰一由服务端裁；期限种类与起算事件的封闭集由服务端词表读口供。

**为什么多一条 Blocked by**：伞票落点判据是「写签跟着读签走」（票 03）。读面不在，写签摆不到任何一页——先立并落
管理台的客户服务规则册（与票 06 的做法同形：`CommercialPolicyKind` 加格、Record、列向、标签、来源提示句），再谈本票。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；另一条本册特有：VE 那侧已有的两本册（通知义务、索赔类型覆盖）与本册正文归谁，pc-gaps/05 记着要
走 ADR——本票只发布 PC 这一侧的正文，不替那个所有权裁决开口。

## 完成判据

管理台客户服务规则册旁多一签「发布客户服务规则版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻
可见；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0023；不碰 VE。
