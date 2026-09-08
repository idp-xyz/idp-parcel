# 17 `AUTHORIZATION_RULE` 版本的运营主路径：逐字段表单（取消授权按请求方逐格可加行）

Category: enhancement
Status: ready-for-agent——形状已裁清（逐字段表单 + 取消授权按请求方可加行；本票无待裁问题），Blocked by 08 未 resolved 前不在前沿；伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md) 拆出（MCP-6 2026-09-07，锚 `95182b9d`；接管会话 2026-09-07 于 `92579b0a` 逐句核过票面，见伞票 Comments）
Blocked by: 08（已进 main）；[20](./20-publication-vocabulary-read-face.md)（词表读口公共半边，2026-09-08 通道 1 代裁立票；落点广播前表单里的下拉先按票面写成占位、不内置枚举）

## 册与载荷

显示在**政策页·授权规则册**。版本壳之外归它的声明是 `declarations.cancellationAuthority[{party, rule}]`（0013 取消
授权目录：请求方是封闭二值，规则是引用）。授权授予册（`authority_grant.go` 的 `AuthorizedAction` 一族）不经这条
发布路，不在本票。

## 选形与理由（ADR-0101 决定八）

**逐字段表单，取消授权可加行。** 频次低、配置员操作、正文是一张两列几行的表。请求方下拉由服务端词表读口供（封闭
二值，表单不内置枚举——pc-gaps/08 若给 `AuthorizedAction` 加格，那是授予册的事，与这里的请求方二值无关，别混）；
同一请求方第二行由构造门拒。

## 硬句

伞票 [07](./07-commercial-publication-operator-main-paths-per-register.md)「硬句」一节逐字适用（不在此复述，那一节是唯一口径）；本册无特有硬句——「pc-gaps/08 加的授权动作格与这里的请求方二值无关」已写在上面「选形与理由」里，不重复。

## 完成判据

授权规则册旁多一签「发布授权规则版本」（表单 → 预览摘要 → 待批准 → 批准 → 发布），结果在同册立刻可见（取消授权
按请求方逐格上列）；tsc / run-tests 绿；Go 侧只加本册规范化一格。

## 边界

不动 0013；不动授权授予册。
