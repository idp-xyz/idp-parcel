# 版本引用的 digest 在操作者路径上没有来源：声明令牌是过渡，领域改法要 ADR

Category: enhancement
Status: resolved——已裁改法 3，落文 [ADR-0108](../../../docs/adr/0108-version-reference-identity-is-kind-id-version-and-digest-becomes-an-optional-declared-fingerprint.md)（2026-09-04，通道 6，owner 授权）；本票自定的 resolved 判据「ADR 编号落进某一条并被引用」已满足；实施另立 [`10`](./10-version-reference-identity-is-a-triple-and-fingerprint-is-optional.md)（ready-for-agent）
Blocked by: 无（`08` 已 resolved `0a67406`）

## 为什么立

票 [08](./08-series-write-face-needs-a-draft-and-a-version-read-face.md) 做逐字段表单时撞出、
MCP-3 于 2026-09-03 裁了过渡做法、本票记的是**那条裁决没解决的那半**。

`domain.ReferenceSeriesRegistrationSpec` 要填三处 `domain.VersionReference`（自身版本引用、
口径 `QuoteBasis`、更正回指 `PriorVersion`），领域要求三者 `digest` 非空；而运营操作者路径上
**三处都没有能给出真正摘要的来源**（取证见票 08 那条 2026-09-03 · MCP-5 Comment）：

| 引用 | 为什么给不出摘要 |
|---|---|
| 自身版本引用 | 不能装 PRS 内容摘要——快照文档把引用（连 digest）折进了内容摘要，装进去就自指循环 |
| 口径 `QuoteBasis` | `GET /commercial-policies?kind=PRICE_POLICY` 不透出政策版本的 `content_digest`（库上有列，读口没透） |
| 更正回指 `PriorVersion` | 册上有登记时声明的引用 digest，逐版本读面透出后可照实回指——**这一处有来源**，另两处没有 |

领域里这个 digest **只参与身份与指纹、从不被比对任何内容**；seedgen 一律写
`sha256:syn-<id>-<version>` 占位。也就是说它在今天的模型里承担的是「引用身份的一部分」，
而不是「可验证的内容指纹」——一个叫 digest 的槽装着不是 digest 的东西。

## 已裁的过渡（MCP-3 2026-09-03，落在票 08）

三处槽装**声明令牌**：`declared:<kind>/<id>@<version>`，前缀自报「这是声明不是哈希」；只在新铸引用
时铸，载荷带来的（回指与口径）照实用不重铸；预览与登记共用同一条铸法，否则决定四那句硬句就破。
领域的 `NewVersionReference` 不为此改动。实现与钉它的测试见传输层 `reference_series_payload.go` /
`reference_series_payload_test.go`（`TestSeriesPayloadMintsDeclaredTokensOnlyForNewReferences`）。

**过渡的代价**：册上会同时存在两种形状的引用 digest（受控批量口带真摘要或 `sha256:syn-…` 占位、
表单路径带 `declared:` 令牌），而领域对两者一视同仁。今天没有任何读法依赖它们可比，所以不出事；
但任何日后想「按引用 digest 校验被引对象没变」的读法，都会在这一格上遇到两种语义。

## 三条领域改法（互斥，要 ADR 裁一条）

1. **PRS-2 排除引用槽**：内容摘要的规范化文档不折入引用的 digest（只折 kind/id/version），自身引用
   便可装真摘要——自指循环拆掉。代价：改规范化版本（`canonicalization` 换号），旧版本按
   `CANONICALIZATION_DIFFERS` 不可比；另两处仍缺来源。
2. **PC 读口透出 `content_digest`**：口径引用有了真来源。代价：跨上下文（party-commercial 是
   MCP-5 地盘但读口形状属 PC 语言），且只解决口径一处。
3. **把 digest 从引用身份里拿掉**：`VersionReference` 只剩 kind/id/version，digest 成可选的
   「声明时附带的指纹」，不参与身份、不参与指纹。代价：改领域值对象与快照文档形状，触及所有
   引用它的册（价卡、评价清单）。

倾向（不是裁决）：3 最诚实——它让类型名与它装的东西一致；1 与 2 各修一处、留两处。但 3 的
波及面最大，要看评价清单那边怎么用 `VersionReference.Digest`（票 01 绑的序列标识就在那里）。

## 红线

- 不改 `NewVersionReference` 的非空校验来「放行」空 digest——那是把问题从命名挪到数据里。
- 预览与登记的铸法始终同一条（ADR-0101 决定四），无论过渡还是改法落地。
- 不写任何真实政策或序列的 digest 进仓；SYN 夹具只记 `S`。

## 裁决（2026-09-04，通道 6，task-f530ad56 裁决批口径：owner 授权自决，写明能力边界）

**取改法 3，落文 [ADR-0108](../../../docs/adr/0108-version-reference-identity-is-kind-id-version-and-digest-becomes-an-optional-declared-fingerprint.md)。** 决定性的证据是领域自己的身份判断早就不含 digest：`NewVersionManifest` 的去重键 `referenceIdentity{kind, id, version}`（`value_objects.go`）——同一清单里两条只差 digest 的引用被当成重复而拒，也就是领域已认定 digest 不是身份，只是类型没跟上；而 `fingerprint.go` 的 `canonicalReference` 与 `compareCanonicalReferences` 却把它折进摘要、参与排序，与去重键相悖。加上「没有任何读法拿 `Digest()` 比对过内容」（重放比的是 `planContentDigest`，另一个字段）与域内本来就装着 `builtin:decimal-bigint-v1` 常量串这两条，改法 3 是把类型改成领域已经在用的定义，不是新发明。

ADR 六条：身份三元；digest 改为可选「声明时附带的指纹」，进快照不进规范化文档、不参与相等；`NewVersionReference` 三元非空、指纹可空（红线「不改非空校验放行空 digest」的前提是 digest 仍在身份里，先拿出来非空校验就失去对象）；规范化换号一次（与 ADR-0107 同期实施合并为一次）；过渡令牌 `declared:` 退役，三处引用各归其位（自身引用与口径指纹留空，回指带前版内容摘要）；改法 1 只解一处不取，改法 2 是可选增强归 PC owner。

**能力边界**：读了 `value_objects.go` 的 `VersionReference` / `NewVersionReference` / `NewVersionManifest` / `compareVersionReferences`、`fingerprint.go` 的 `canonicalReference` / `compareCanonicalReferences` / 语义摘要文档、`evaluation.go` 的重放校验字段、`reference_series_payload.go` 的令牌铸法，以及 ADR-0014 / 0099 / 0101；**未读** `reference_series_register.go` 快照文档的逐字段形状与 `plan_snapshot.go` / `evaluation_snapshot.go` 读回旧快照的路径——「旧形状里的 digest 读回后放进可选指纹」那格兼容读法由实施票钉测试；未数 `NewVersionReference` 的调用点总数（实施开工时重取，别引本票）。

## 验证

ADR 接受后按所选改法另拆实施票；本票 `resolved` 的判据是 ADR 编号落进上面某一条并被引用。

## Comments

- 2026-09-04 · MCP-5：立票。起因是票 08 解码器文件头写着「领域改法立在票 10」而票 10 不存在
  （一览表止于 07，08 也未入表）；本票补上那个引用并把编号改为 09，解码器注释同步改指本票。
  **只写票面，未动领域代码。**
