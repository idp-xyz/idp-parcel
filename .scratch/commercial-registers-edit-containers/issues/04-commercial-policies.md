# 04 商业规则：行详情进右栏、发布签改登记视图（九本册换册保活照旧）、收尾用对话框

Category: enhancement
Status: resolved——2026-11-04 进 main `fc956259f`（记录见文末）
父票：[spec](../spec.md)
Blocked by: 01

## 要做的

照 spec 的决定 1–4，用票 01 抽出的 `VersionDetailPanel`、`VersionClosureDialog`、`RegisterView` 接线；本页特有的东西（表单、选择器、地址键）保持。

## 完成判据

- 三道门退 0。
- 浏览器核：右栏详情、登记视图进出与离开守卫、收尾对话框、查看者无写入口。

## 完成记录（2026-11-04，直接在 `main` 上做，自 `5aae1ad23`，提交 `fc956259f`，CI run 41798618995 全绿）

- 落点：`CommercialPoliciesPage.tsx` 的九本册目录改在 `WorkbenchPageTemplate` 上（列按 id 搬成 key，右栏开时只留前三列），行详情用 `VersionDetailPanel`，收尾用 `VersionClosureDialog`；「发布新版本」与「JSON 发布（高级）」是 `?tab=publish` / `?tab=json` 的 `RegisterView`，键沿用此前的签名（`usePoliciesPlace` / `policiesHash` 不动），进视图压历史、出视图后退或替换；视图里选册照旧只替换地址、各册表单在视图里保活；任一册表单有未提交输入时「返回列表」先问。查看者拿视图深链进来、授予答出后清键落回册。
- ✅ 三道门退 0：tsc、node:test、vite build；CI run 41798618995 全部 job success。
- ✅ 浏览器在演示租户核（playwright 只读探针，零写请求、零页面错误）：九个册 chip 在页头；接单规则包的行点开右栏（详情 / 版本历史、技术详情），右栏开时列收到三列；「发新版本」进发布视图带预填说明、选中的册对、工作台不在；视图里换到信用政策只换 `register` 键；改一格后「返回列表」弹「离开这一页？」，「继续填写」留在视图，「离开」回册；页头「发布新版本」进视图、未改动返回不问；「JSON 发布（高级）」进 `?tab=json` 视图；深链 `?tab=publish&register=CREDIT_POLICY` 直落视图且 chip 选中信用政策，返回用替换留下 `register` 键。查看者深链落回册、页头无两个写入口、右栏无动作。
- 查看者深链时控制台有一条 403（发布视图在授予答出前短暂挂起，去取 `commercial-publication-vocabularies`）：授予加载期按 ADR-0315 乐观摆入口的既有行为，服务端照旧拒绝；非本票引入，未改。
- 演示数据各册只有一版，收尾对话框没有可收尾的行可核（与票 01 同）。
- 评审：diff 全在 `pages/**`，按 workflow「前端切片」第 5 条自审。证据 S。
