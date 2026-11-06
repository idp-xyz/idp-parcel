# 01 共用件补件：落列声明、取页 SQL 片段、`Finish` 与统一结果类型

Category: enhancement
Status: in-progress——作者完成，与 02 一并评审（首轮钉 `3a20b2bcc`，修订轮复核中；之后补的 NUL 缺口见文末完成记录，须一并复核）。2026-11-06 通道 1，分支 `cursor/catalogue-pagination-rollout`，worktree `idp-parcel-pagination`，基 main `a1274a6b8`
地盘：`internal/platform/cataloguepage`（新文件与其测试；包头注一句）。
出处：[ADR-0144](../../../docs/adr/0144-catalogue-reads-share-one-cursor-pagination-sort-and-filter-contract.md) 决定一、三、四、五、七；spec「共用件的补件」。

## 做什么

1. **落列声明**：一册在自己的 `Catalogue` 上绑定「可排维 → 列、行标识各段 → 列、筛选维 → 列、`q` → 文本列」，绑定时校验一次——声明的每一维都落了列、没有多出的列、
   标识段数与声明一致、`q` 至少一列；不合即编程错误，包级声明写法 panic。
2. **取页 SQL 片段**：按一次查询拼出总数与取页共用的条件（读面给的固定条件，如 `tenant_id = $1`，与上筛选维与 `q`）、键集比较（第一页没有）、排序维加各段标识的同向次序、
   页大小占位（取 `limit+1` 行）；新参数接在读面已占用的参数之后编号。筛选维内为或、维间为与；`q` 只做字面包含（`%`、`_`、`\` 转义）、不分大小写。
   列里存的是代码而不是词表上的词时，允许按维给「词 → 存值」的换算，不必为筛选另导一列。
3. **`Finish` 与统一结果类型**：把读面取回的至多 `limit+1` 行切成本页，还有下一页时用本页末行的位置编游标；结果是「本页行 + 下一游标 + 总数」，并能直接拼出答复的 `page`。
4. 查询对象不是本册 `Decode` 解出来的、`limit` 非正，都答错误而不是拼出一句错 SQL。

## 不做

- 不碰数据库连接、不引 `pgx`：片段与参数交回读面，由读面执行（共用件不含任何上下文的词，也不认任何表）。
- 不改首批两册（归 15）；不改游标编码与查询解码的既有行为。

## 完成判据

- 单元用例钉住：绑定校验的各种不合（漏维、多列、段数不符、`q` 无列）；第一页与带游标的片段与参数逐字；倒序时比较号与次序同向翻转；筛选维内或、维间与且按维名排序稳定；
  `q` 的字面转义；换算维的存值进参数；读面已占用参数时编号接续；他册解出的查询对象与非正 `limit` 被拒；`Finish` 在恰好一页、多一行、空页三种情形下的切页与游标。
- 现有用例全绿；`gofmt -l` 空，`go vet` 退 0。

## 完成记录（2026-11-06，通道 1，分支 `cursor/catalogue-pagination-rollout`，基 main `a1274a6b8`）

**落点**

| 笔 | 做了什么 |
|---|---|
| `7b1166838` | 落列声明 `Columns` / `Bind` / `MustBind`，取页片段 `Plan`（`CountSQL` / `PageSQL`），`Finish` 与统一结果类型 `Result` |
| `9726b2f35` | 领域枚举的词表与代码换算 `Words` / `Code`；`Finish` 头注写明游标位置取自 ORDER BY 那几列的扫描值 |
| 补 NUL 缺口（本笔） | 游标里的文本段、`q`、自由取值的筛选值按 `storable.Text` 判，库存不下的答 400；判据从 `httpapi` 挪到新包 `internal/platform/storable`，`httpapi.StorableText` 改为转调 |

**完成判据**

- ✅ 单元用例覆盖本票判据各条（`binding_test.go`、`result_test.go`、`enum_test.go`），另证伪造游标的文本段带 NUL 答「游标与本次的排序或筛选不符」、读面交来带 NUL 的位置编不出游标、`q` 与筛选值带 NUL 或不合法 UTF-8 答 400 且理由点名那一格、不回显坏值。
- ✅ 门（钉本笔，WSL go1.26.8）：`gofmt -l` 空；全仓 `go build`、`go vet` 退 0；`internal/platform/cataloguepage` 138 PASS、`internal/platform/storable` 7、`internal/platform/httpapi` 12；依赖本包的 62 个包与 `internal/architecture/...` 无 DSN 全过；`cmd/parcel-api` 带 DSN 654 PASS / 0 SKIP（含存不下的读值对真库答 400 那条）。

**判断项**

1. **补 NUL 缺口，偏离本票「不改游标编码与查询解码的既有行为」**：13 的作者交回时指出，游标的文本段解开后不判就进 SQL 参数。游标不防篡改——摘要只用来认出「条件变了」，不是凭证，作用域来自 Intake——改写一个真游标就能把 NUL 带进去，库拒收、读口答 500。`cmd/parcel-api` 登记处的请求目标检查判的是 base64 编过的整条游标，看不到解开后的值。合规请求的答法不变，变的只是原本答 500 的那几类请求；`q` 与筛选值在 `cmd/parcel-api` 上本由那道检查先拦，这里再判是为了不靠装配处：豁免表里的口与别的二进制同样受护。
2. **判据挪包，偏离本票地盘**：判据原在 `httpapi`。共用件导入它，各上下文的 `ports` 与 `application` 就会经共用件依赖 `net/http` 与 chi（实测今天没有一个业务 `ports` / `application` 包依赖它们）；在共用件里另写一份又违「单一权威」。所以挪到不属传输层的 `internal/platform/storable`，`httpapi.StorableText` 保留为转调，既有调用方一个不动。
