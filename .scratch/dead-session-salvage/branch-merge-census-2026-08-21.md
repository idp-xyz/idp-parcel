# 本地分支并回清册(基线 main `b394adf`,2026-08-21 晚)

接一个死会话的交接办的:它报「25 已吸收 / 1 部分 / 6 真待合」但名单没落盘,本册重导一遍并把证据钉死,
**免得下一人再查一轮**。61 条本地分支(不含 main)逐条判定,结论:**真待合只有 1 条,已于本日合入**;
其余为已吸收、被取代平行稿、或已有裁定在案。

## 判据与方法(可重跑)

三条相互独立的探针,交叉使用:

1. **补丁等价**:`git cherry main <branch>`——全 `-` 即补丁已在上游。
2. **合并净增量**:`git merge-tree --write-tree main <branch>` 得树后 `git diff main <tree>`——
   干净且为空 = 吸收;非空 = 有货或有冲突,进第 3 步。
3. **blob 历史探针**:对分支侧差异文件取 `git rev-parse <branch>:<file>` 的 blob,
   `git log main --find-object=<blob>`——命中即「该文件的这个版本进过 main,后被演进」。
   两头都没有的残行再逐行眼验(方向 numstat:`git diff <branch> main -- <file>` 的 `-` 行是分支独有)。

冲突不等于没吸收——内容以改过的形态进 main 的分支照样冲突,只有第 3 步分得开这两种。

## 一、本日合入:1 条

- **`t1-09a-takeover`**(tip `12065d3`)→ 合并提交 **`b394adf`**(父 `a1f283c` + `12065d3`)。
  票 09-A VE 规则与政策登记册写口:catalog_registration 适配器、register_catalog 用例、
  ports 登记口、迁移 0019(号在 main 空缺,嵌入走目录级 `all:` 不动 migrations.go)。
  隔离树验证:build/vet 零信号;真库探针 9 个目录登记册用例真 `PASS` 非 `SKIP`
  (DSN 55432,容器健康);全仓 `go test -count=1 ./...` 全 `ok`。
  **未推送,已广播交 MCP-1 按确切 SHA 推。**

## 二、补丁全等已吸收:51 条(cherry 全 `-`,于 b394adf 复跑)

cons-final-a, cons-final-b, cons-intake-a, cons-intake-b, cons-pickup-a, cons-pickup-b,
cons-proj-delivery-a, cons-proj-handover-b, cons-proj-pickup-a, declaration-envelope-version-dedup,
dispatch-db-ready, dispatch-fanout, docs-abbreviations, integrate-b2, integrate-b3, integrate-b3-v2,
integrate-b5, integrate-b5-v2, integrate-b6, integrate-b7, integrate-closeout-docs,
integrate-cons-proj-a, integrate-cons-proj-b, integrate-cons-proj-delivery-a,
integrate-cons-proj-pickup-a, integrate-ps-index, integrate-syn-v0, mcp1-adr-0065-storage,
mcp2-decl-a, mcp3-pc-publication, mcp4-proj-b, nr-applicability-from-resolution,
product-version-closure-b2, product-version-closure-b3, product-version-closure-b4,
product-version-closure-docs, ps-adopted-owner, ps-intake-qual-evidence, ps-parcel-index,
syn-pc-product, syn-pc-seed, syn-vertical-closure, t-ratchet-gate, t1-02-ownership-authority,
t1-02-takeover, t1-06-cc-case-config, **t1-09a-takeover**(合入后转全等), **t1-09a-ve-registries**,
t1-10b-intake-qual-wire, ve008-accept-rederive, worker-cons-proj-b

其中 `t1-09a-ve-registries`(`6cb78e0`)在 b394adf 上另验了合并净增量为**零**——
其内容是 takeover 接手稿的真子集,不必再看。

## 三、内容已吸收(cherry 有 `+` 但逐文件有 main 历史锚点):5 条

| 分支 | 锚点证据 | 残行判定 |
|---|---|---|
| `syn-wall-door-audit`(`a958029`) | 采纳提交 `1b8534f`(08-20 17:03),11 文件全在 main 且票面已演进 | 无残行,main 为演进侧 |
| `pn07-b6-stage-content`(`22bb69f`) | blob 进 main 于 `7cb39b6`/`81c5183`/`c8abe94`;ADR-0058 在 main | ports.go 残行 = `CommercialResolutionStore.LoadResolution`,由 ADR-0062(`81c5183`)**有意拆除**;另一段是被演进的旧注释 |
| `product-version-closure-b7`(`9d8c09b`) | blob 进 main 于 `1fff679`;ADR-0059 在 main | ports.go 残行同上,ADR-0062 拆除件 |
| `ps-rehydrate-accepted`(`a22e24d`) | ADR-0061 经同题提交 `6228d8e` 落 main;fix 所动的四个 domain 文件与 main **tip 逐字节同** | shipment_request.go 残 5 行是旧 upsert 形态与旧注释,被 `6228d8e`/`0f40744` 演进取代;synthetic_v0_test blob 进 main 于 `6d4f5b3` |
| `cons-proj-tf-b`(`f3a3c2b`) | assemble.go/assemble_test.go blob 进 main 于 `7d7e138`;offsite 用例**同名**在 main(offsite_pickup_projection_test.go) | 分支的 delivery 投影用例被 main 拆成更细的族(StopsAtUnconfiguredFinalRule / KindClassifies / kind_mapping 等);`7d7e138`「交接登记只投 VE 不 FanOut PS」是后续有意演进,非丢失 |

## 四、被取代的平行稿,建议弃(⏸ 需人拍板,分支指针一律保留):3 条

1. **`mcp3-ve008-wire`**(`dca024a`,08-19):UC-VE-008 客户视图接线。main 侧同功能已由
   `49a2ab0` 一线重做(提交信同述「三维译码/按版本读回/PS 反查账户维」),现有
   `veconsume/customer_view_consumption.go` + `cmd/parcel-dispatch/customer_view_wiring_test.go` +
   `acceptance_rederive_test.go`;分支四文件的 blob 从未进过 main。**未逐行读分支稿**,
   若信不过提交信的功能对映,弃前读那四份 diff。
2. **`salvage-cons-proj-delivery-detached`**(`c66c97a`,08-19 12:09):与 main 的 `f388c50`
   (08-19 12:05,CONS-PROJ-DELIVERY-A)**同题双胞胎,差 4 分钟**,main 版先落地并已演进多笔;
   与集成稿逐文件差 217/145 行,**未逐行读**。dead-session-salvage 票 02 里「未判,保留」的就是它,
   本册补上时序与差量证据。
3. **`mcp1-pc-publication`**(`db81745`,封存稿,约七成完成、含一个死在编辑半途的测试文件):
   票 03(PC 申报发布写口)已在 main 由 MCP-3 转 resolved(`0ec62ea`);
   草稿的 resolution_key_registry.go 对应 main 现有 `commercial_resolution_keys.go`,
   application 写口对应 `publish_commercial_authority.go` 族。

## 五、已有裁定在案,本册不再动:2 条

- **`adr-0065-storage`**(`b72d96e`):08-20 已裁**弃**(被 `071eae0` 取代的平行旧稿,且违「已施加
  迁移不可改写」);**删分支指针一事仍等用户单独确认**。
- **`bento-gate-reeval`**(`2399ecd`):已裁**先蒸馏后定**,派 MCP-4 只读产出结论票;蒸馏完分支去留再定。

## 与死会话交接数的对账

它报 25/1/6(合计 32),本册于 `a1f283c` 量得 49 全等 + 12 有 `+`。它的 32 名单没落盘,
成员无从比对;两套数用的口径也不同(它先按「内容与 main 有差」筛过一轮)。**不必对齐**——
本册每一格都带可重跑命令与 SHA,以此为准。

## 未尽事项

- 三条弃候选的「未逐行读」如上标明;裁弃前要不要读,由拍板人定。
- 分支指针清理是另一件事:整类一起做、单独开票(与 dead-session-salvage 票 02 既有口径一致)。
