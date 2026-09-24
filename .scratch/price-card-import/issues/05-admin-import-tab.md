# 05 管理台「导入价卡」签

Category: enhancement
Status: ready-for-agent——2026-09-25 通道 3 立票并激活（用户授权自决）
Blocked by: 03
地盘：`apps/admin-web/src/pages/pricing/`（价卡页、纯逻辑、`api.ts`），以及 01 定下的模板文件存放处（若在管理台内）。
出处：[spec](../spec.md)；ADR-0101 Consequences 第二条与末条。

## 要做的

1. 价卡页的「登记价卡」签改为「导入价卡」。流程：下载模板 → 上传 → 校验结果与摘要预览 → 存为草稿。
   - 预览走 02 的口，存草稿走 03 的口，两次上传同一份字节。
   - 逐格问题按表、行、列排给人看，能对回模板里的那一格。
2. JSON 快照签退为高级口，不出现在运营配置员的主路径上。
3. `api.ts` 里「请求体形状此刻没有契约」那一段按 ADR-0101 改写。
4. 四态如实：端点今天答 403，页面照实呈现未配置，不拿演示数据顶替。

## 验收

- 三道门：`tsc -b --noEmit`、`run-tests`、`vite build`。
- 浏览器验证：真后端的 403 一态；预览与存草稿两态用真适配器序列化的响应注入，注入方式照 [operator-workspace-gaps/06](../../operator-workspace-gaps/issues/06-estimate-admin-page.md) 完成记录。

## 形态

diff 全在 `apps/admin-web/**` 与票面，按 workflow「前端切片」在 `main` 上做。
