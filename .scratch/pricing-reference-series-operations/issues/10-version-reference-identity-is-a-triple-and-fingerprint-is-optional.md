# 10 版本引用改为三元身份 + 可选指纹，过渡令牌退役：ADR-0108 的实施票

Category: enhancement
Status: resolved——MCP-6（2026-09-04，隔离分支 `mcp6-pp-renumber` 代码笔 `b8dfc9a8`，基线 main `4cc1bc34`，task-2b9edfe4 换号批五票之一；收缩步与机制清点见文末完成记录）；裁决已落 [ADR-0108](../../../docs/adr/0108-version-reference-identity-is-kind-id-version-and-digest-becomes-an-optional-declared-fingerprint.md)；本票只做机制半边（通道 6 2026-09-04 立票，只写票面未动代码）
Blocked by: 无

## 缺口

见 [`09`](./09-version-reference-digest-has-no-source-on-the-operator-path.md) 的取证与裁决：`VersionReference` 的 `digest` 槽装着真摘要 / 占位 /
`builtin:` 常量 / `declared:` 令牌四种东西而领域一视同仁，且领域自己的去重键早已不含它。ADR-0108 裁定身份三元、指纹可选、不进规范化文档、
换号一次、令牌退役。本票把它做成代码。

## 做什么

1. **领域**（`internal/parcelpricing/domain`）：
   - `VersionReference` 改为三元身份 + 可选指纹；相等、`NewVersionManifest` 去重、`compareVersionReferences` 排序只看三元（把 `referenceIdentity`
     提成类型定义，去掉与它相悖的第四对比较）。
   - `NewVersionReference`：三元非空且去空白，指纹可空；**走三步法**——先加新构造（三元 + 可选指纹）与访问器，迁全部调用点，再删旧四参签名。
     调用点分布在价卡、评价清单、参考序列登记、seedgen、测试夹具，开工时 `git grep -n 'NewVersionReference('` 重取清单，别引任何旧数。
   - 规范化文档（`fingerprint.go`）：`canonicalVersionReference` 去掉 `Digest` 字段，`compareCanonicalReferences` 去掉第四对；`canonicalization`
     按 ADR-0014 换号。**若与 `pricing-amount-precision/02`（ADR-0107）同期实施，合并为一次换号**，照 PPC-2 先例在 `canonicalizationVersion`
     的注释里记下这一次合了哪几处。
   - 快照文档（`plan_snapshot.go` / `evaluation_snapshot.go` / `reference_series_register.go`）：指纹字段可缺；**读回旧形状的快照时把旧 `digest`
     放进可选指纹**，钉测试；旧规范化版本按 `CANONICALIZATION_VERSION_UNSUPPORTED` 既有语义处置，既有评价不追溯改写。
   - `builtin:decimal-bigint-v1` 那条内置引用改为三元。
2. **传输层**（`adapters/http/reference_series_payload.go`）：删 `DeclaredReferenceToken` 与 `declared:` 前缀；自身版本引用与口径 `QuoteBasis`
   只带三元、指纹留空；更正回指 `PriorVersion` 带三元 + 前版登记的内容摘要作指纹（有来源）。预览与登记共用同一条构造（ADR-0101 决定四）——
   钉它的测试 `TestSeriesPayloadMintsDeclaredTokensOnlyForNewReferences` 改写为「两条路产出同一份引用且指纹按来源在场 / 缺席」。
3. **seedgen**：`sha256:syn-<id>-<version>` 占位一律去掉，SYN 夹具引用只带三元（它们本来就记 `S`）。
4. **不做**：不给 PC 读口透出 `content_digest`（可选增强，归 PC owner，另立时落进可选指纹即可）；不新造「按指纹校验被引对象没变」的读法。

## 红线

- 三元一格不许空；不在身份里保留一个可空的 digest（票 `09` 红线的本义）。
- 指纹缺席不影响任何摘要、相等与排序——钉测试：同一引用带 / 不带指纹，内容摘要与语义摘要逐字相同。
- 不写任何真实政策或序列的摘要进仓。
- 领域包不依赖 HTTP / `pgx`；在共享树上做会让全仓编不过，走单独 worktree 或三步法并在频道占号。

## 完成判据

- 三元相等 / 去重 / 排序三处判据一致的用例；带与不带指纹摘要相同的用例；旧快照读回指纹落位的用例；令牌铸法删除后预览与登记同构造的用例。
- 规范化版本换号，旧版本快照的既有 `CANONICALIZATION_VERSION_UNSUPPORTED` 用例仍绿。
- `gofmt -l` 空、`go build` / `go vet` 退 0、`go test -count=1 ./...` 绿并注明含不含真库；机制清点在干净检出上重生成随笔提。

## 参照

ADR-0108；ADR-0014；ADR-0101 决定四；票 `08`、`09`；`pricing-amount-precision/02`（同期换号合并）。

## 完成记录（2026-09-04，通道 6，task-2b9edfe4 换号批）

分支 `mcp6-pp-renumber`，代码笔 `b8dfc9a8`（基线 main `4cc1bc34`；分支上还有同批其余票的笔，最终已验 tip 由完工报给）。

- **领域**：`VersionReference` 身份三元 + 可选 `fingerprint`；`SameIdentity` / 清单去重 / 排序 / `VersionManifest.Equal` / 方案必含引用检查一律三元。新构造 `NewVersionReferenceIdentity`、`NewVersionReferenceWithFingerprint`；`NumericProfileV1Reference` 只带三元。
- **换号**：`canonicalizationVersion` `PPC-4`→`PPC-5`（合并换号，注释记下同批 ADR-0107/0109/0110/0111 落同一号，以及为何换号在第一份改规范化的笔里：不换号的中间态会让同一版本号下存在两套字节）；序列登记 `seriesCanonicalization` `PRS-1`→`PRS-2`（三处引用只以三元入内容摘要，旧快照按版本门拒而不是按摘要不符拒）。
- **快照**：`fingerprint,omitempty` 写、旧 `digest` 读回落进可选指纹（包内用例钉）。
- **传输层**：`DeclaredReferenceToken` 与 `declared:` 前缀删除；自身引用只带三元，口径带载荷 digest 则进指纹否则留空，更正回指载荷键 `priorReferenceDigest`→`priorFingerprint`（= 前版 `contentDigest`）；admin-web 序列表单同步（`priorFingerprint: record.contentDigest`）。
- **seedgen**：SYN 引用只带三元；六份 pricing 种子重生成为 PPC-5 / PRS-2。
- **三步法未收缩的那一步**（按 MCP-1 派单约束，等 pricing/06 入 main 后另笔）：`NewVersionReference(kind,id,version,digest)` 与 `Digest()` 仍在，为薄包装；其余 SYN 测试夹具（`git grep -n 'NewVersionReference('` 可列）仍经它带 `sha256:syn-` 占位串——只进指纹，不进任何摘要、相等与排序。收缩时一并改用三元构造并删旧签名。
- **验证**（本树，DSN 已设）：gofmt 空；go build / vet 0；go test -count=1 parcelpricing/... parcel-pricing-register parcel-api settlementaccounting/... PS adapters/parcelpricing 全 ok；admin-web tsc --noEmit 0、run-tests 61/61。既有 `CANONICALIZATION_VERSION_UNSUPPORTED` 用例仍绿。机制清点随本批最后一笔在干净检出上重生成。

## Comments

- 2026-09-04 · 通道 6：立票。票 `09` 自定的 resolved 判据是「ADR 编号落进某一条并被引用」，故裁决与实施分票；本票承接实施。**只写票面，未动代码。**
- 2026-09-04 · 通道 6：实施落地 `b8dfc9a8`，转 resolved；收缩步另笔（见完成记录）。
