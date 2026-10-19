# 44 管理台节点作业复核页把收寄的「服务结果读不动」显示成单独一格，不再与「无标记」同形

Category: enhancement
Status: resolved——2026-10-19 进 main（`4bcde292`），见「完成记录」。此前 in-progress——2026-10-19 前端实施子代理认领（Blocked by 的 operator-channel/43 已进 main `4a09e100`），照前端切片在 main 上直接做。此前 ready-for-agent——2026-10-18 会话 `idp-parcel-9b` 立（票 26 序 2 评审遗留；用户授权代决）
Blocked by: operator-channel/43 进 main
父票：operator-channel/43。
出处：簇 C1 终裁 C1-Q2，可见性那一条。
地盘：
- apps/admin-web/src/pages/operations/NodeOperationsReviewPage.tsx
- apps/admin-web/src/pages/operations/records-api.ts
- apps/admin-web/src/pages/operations/（新的纯逻辑映射模块与它的 node:test 测试）

代决依据：用户 2026-10-16 授权会话 `idp-parcel-9b`「你作为业务和系统专家你来自决」；裁定经隔离子代理研判、对抗核验与合成（研判轮 review26-followups-decide）。

前端票：照 docs/agents/workflow.md「前端切片」在 main 上直接做。

## 为什么

operator-channel/29、C1-T2 落地后，节点作业查阅答复里，INTAKE_FORMED 行会带上服务结果核对态。

但复核页收寄区的「服务标记」一列，今天只做一件事：把 serviceMarkers 用顿号拼起来。结果是「读不动」「已查、无标记」「待识别，不适用」三种情况都显示成空白。今天没有方向性门禁，运营在这页上看到空白，就会当成「没有取消、没有终局」。

## 裁定

- 「服务标记」一列按三态出文字，文字各不相同：
  - 已查、有标记：照旧用顿号拼接；
  - 已查、无标记：写明「无取消与终局」；
  - 读不动：写明服务结果读不动，标记缺席不代表没有取消或终局。
- 待识别行写「不适用」。
- 「读不动」的文字不得与空白或「无」同形。具体措辞由作者定。

## 做什么

- records-api.ts：ReceptionRecord 类型加核对态字段，可选。
- 把 NodeOperationsReviewPage 里 rowsOf 收寄区的这一段映射，抽成纯函数，并加 node:test 测试。

## 不做

- 不改 Go 代码，不改其他区。

## 完成判据

- [x] 三道门退 0，用 Node 22：tsc -b --noEmit、node scripts/run-tests.mjs、vite build。
- [x] node:test 覆盖四格：已查有标记、已查无标记、读不动、待识别。
- [x] 票面的完成记录里，判据逐条标 ✅ / ◑ / ❌。

## 完成记录（2026-10-19，前端实施子代理，照前端切片在 main 上直接做）

**落点**：`4bcde292` 一笔，四份文件都在地盘内（另加本票面）。
- `apps/admin-web/src/pages/operations/records-api.ts`：`ReceptionRecord` 加可选的 `serviceResultCheck`。类型留 `string`，与 `kind` 同理：封闭集扩了而页面还没跟上时，照样露码。
- `apps/admin-web/src/pages/operations/reception-rows.ts`（新）：`serviceMarkersCellOf` 管「服务标记」一格出什么字，`receptionRowOf` 是收寄区整行转写，由原 `rowsOf` 的收寄分支原样搬来，只把服务标记一格换成前者。
- `apps/admin-web/src/pages/operations/reception-rows.test.ts`（新）：node:test 九例。
- `apps/admin-web/src/pages/operations/NodeOperationsReviewPage.tsx`：收寄分支改成 `body.receptions.map(receptionRowOf)`，删掉不再用的两个词表导入。

**「服务标记」一格的最终文字**：

| 格 | 条件 | 文字 |
|---|---|---|
| 已查、有标记 | `CHECKED`，标记非空 | 标记用顿号拼接（照旧），如 `SYN-CANCELLED、SYN-TERMINAL` |
| 已查、无标记 | `CHECKED`，标记为空 | `无取消与终局` |
| 读不动 | `UNREADABLE` | `服务结果读不动：没有标记不代表没有取消或终局` |
| 待识别 | `kind = PENDING_IDENTIFICATION` | `不适用` |
| 作者定的一格：形成行没带核对态 | `INTAKE_FORMED`（或页面未收录的 kind），整键缺席，标记为空 | `未带服务结果核对态：没有标记不代表没有取消或终局` |
| 同上，带了页面未收录的值 X | 标记为空 | `服务结果核对态 X（页面未收录）：没有标记不代表没有取消或终局` |

作者定的那一格，理由：核对态缺席时，查没查成都无从知道。写「无」等于替它下了一个没人下过的结论，写「读不动」又等于替它编了一次失败。所以两边都不用，只说出能确定的那半句，也就是空标记证明不了什么。缺席的来源可能是更早的服务端答复，也可能是库面 CHECK 没拦住的「形成行不带核对态」（operator-channel/43 偏离与说明第一条）。

判读顺序依次是：待识别、读不动、标记非空、已查、缺席、未收录值。另有两处取舍：
- 待识别按 `kind` 判，不按核对态缺席判。缺席另有一格要说，不能拿「不适用」把它盖过去。
- 读不动排在标记之前。ADR-0186 决定一规定读不动时标记必为空，库面 CHECK 守着这一条，所以不替那种不可能出现的行另编一套说法。标记非空而核对态缺席或未收录时，照实列出标记，因为标记只出自读成了的那一次。

**判据逐条**：
- ✅ 三道门（Node v22.23.3），在提交前的工作树上跑，提交内容与之相同。`node node_modules/typescript/bin/tsc -b --noEmit` 退 0；`node scripts/run-tests.mjs` 退 0，共 545 例：pass 545、fail 0；`pnpm exec vite build` 退 0，只有既有的 chunk 超 500 kB 提示。
- ✅ node:test 覆盖四格。用例在 `reception-rows.test.ts`，分别是「已查、有标记：标记照旧用顿号拼接」「已查、无标记：写明「无取消与终局」，不留空白」「读不动：写明服务结果读不动，且点明标记缺席不代表没有取消或终局」「待识别：写「不适用」——身份未定，本就不核对服务结果」。读不动那例另外断言：文字不是空白、不是「无」、不以「无」起头、与已查无标记那格不同。其余五例是「形成行没带核对态：既不冒充「已查无标记」，也不冒充「读不动」」「形成行带了页面未收录的核对态：如实露码，同样不下「没有」的结论」「标记在场即查到过：核对态缺席时照实列出标记」「四格加缺席一格两两不同，且没有一格是空白」「收寄行整行转写：服务标记一格经同一判读，其余各列照旧」。先红后绿：先只写用例，`run-tests.mjs` 在编译期就红，报 `serviceResultCheck` 不在 `ReceptionRecord` 上，模块也缺；补上类型与模块后转绿。
- ✅ 判据在本节逐条标了 ✅ / ◑ / ❌。
- ✅ CI：`4bcde292` 自己的那次 run（39219784084）被随后的推送取消了。之后头为 `19d9d486` 的 run 39219870998 已含本提交，结论 `success`，其中 `admin-web typecheck, tests, build` 一项 success。

**评审**：只碰了 `pages/**` 业务页，没碰 `templates/*`、`shell/*`、`components/*` 这些共享面。照 workflow「前端切片」第 5 条，自查判据即可。

**未验**：
- 没在开发服务器的真实页面上目视收寄区，也没用 dom-probe。本票改的是纯映射，没有事件路径；四格文字由 node:test 钉住，页面只是原样渲染 `values` 里的串。
- 读不动那句文字较长，在窄宽下的折行效果没看过。

**越出地盘的改动**：无。没碰 Go、SQL、`ci.yml` 与领域文档。
