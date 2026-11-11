# 11 客户合同不必指名接单规则包

Category: enhancement
Status: in-progress
Blocked by: 无（已提交，未进 main；分支 `feat/goods-details-admin`，与 09、10、12 同一条线）

依据：[ADR-0376](../../../docs/adr/0376-fba-submission-conditions-are-judged-as-required-documents-platform-identifiers-may-be-supplied-before-acceptance-and-rule-packages-follow-the-service-product.md) 决定三（PC CONTEXT 合同不变量已改）；ADR-0153 决定四（规则包按申报的服务产品选）；ADR-0032（合同指名只作确认）；ADR-0267 决定三（同号改形的前提）。

## 要做的

- 客户合同正文的「接单规则包」（`contractContent.rulePackage`）改为可缺：领域构造不再拒缺；规范化 `PCC-1` 同号改形，没指名即不编码，指名了的旧文档逐字节不变（钉一条改形前取证的摘要常量）。
- 外壳与正文都指名时仍要相等（`ConsistentAcceptanceRulePackage`）；外壳不指名、正文也不指名，商业解析照旧按申报的服务产品选规则包。
- 落库：合同正文表的规则包列改为可空（新迁移）；读回、目录行与查阅口照实答缺。
- 受控批文与在线登记两口（`registrationjson`、HTTP 草稿载荷）收不带规则包的合同正文。
- 管理台客户合同表单的规则包一格改为可选，说明「多产品合同不指名，规则包随服务产品选」。

## 完成判据

- 一份不指名规则包的合同版本登记成立；同一货主账户下两个服务产品各配一份规则包，申报哪个产品就解出哪一份，不答指名引用未确认。
- 指名了规则包的旧合同摘要常量不变；PC 与管理台各自的测试与三道门通过。

## 不做的

合同按产品各指名规则包（ADR-0376 Alternatives 否决）；接受前财务控制策略仍须合同指名，不动。

## 完成记录（作者一侧，2026-11-11）

- 提交：分支 `feat/goods-details-admin` 上的 `2f0963ca7`（在分支 `feat/contract-rulepkg-optional` 上写成 `86cf517ca`，基线 `22a710c40`，拣入时无冲突）与评审修订 `724e2d65a`，未推送、未进 main。
- 落点：
  - 领域：`NewCustomerContract` 不再拒缺规则包；`AcceptanceRulePackage` 交回（引用，指名了没有）。`ConsistentAcceptanceRulePackage` 壳上指名了规则包时正文要写同一个——缺或不等都拒，两处都不写放行（可缺不等于可以与壳分歧）。
  - 规范化：`PCC-1` 同号改形，`contractContent.rulePackage` 成可缺键，没指名即不编码，折回为「没指名」。前提照 ADR-0376 决定三（ADR-0267 决定三的上线前阶段）。
  - 落库：迁移 `0048` 让 `customer_contract_content.rule_package_id` 可空，在场仍不许空白；写入与重放比对按可缺比（一边指名一边没指名是内容不同）；读回 NULL 即没指名；目录行「正文已登记」改看登记时刻列，不再借规则包列兼作。
  - 受控批文（`registrationjson`）与在线草稿载荷收不带规则包的合同正文；管理台合同表单规则包一格改为可选，注明「多产品合同不指名，规则包随服务产品选」，载荷里留空即壳与正文都不带这一键，发布摘要显「不指名（规则包随服务产品选）」。
  - 陈旧注释随改：目录行端口注释、发布路径的 F-3 注释、合同正文类型注释。迁移 `0012` 头注「rule_package_id 必存」按字节校验不改，由 `0048` 头注说明它随 ADR-0376 不再成立。
- 判据：
  - 一份不指名规则包的合同版本登记成立，真库往返读回没指名、重放答已登记、同版本改成指名答内容冲突、目录行答正文已登记而规则包空 ✅。
  - 同一货主账户下两个服务产品各配一份规则包、合同不指名，申报哪个产品解出哪一份，不答指名引用未确认；合同外壳指名其一时，另一个产品答 `NamedReferenceNotConfirmed`（钉住多产品合同不该指名的理由）✅。
  - 指名了规则包的旧合同摘要常量不变：同一份正文在基线 `22a710c40` 与改后各算一次，都是 `PCC-1:dd4783f31cd8af4fec84fe52945a0ccef795232779f2f20ff16430e0bd5cc226`，钉进测试 ✅。
  - PC 与管理台各自的测试与三道门通过 ✅。
- 未做：「不做的」两项照旧不做；演示种子没动（种子里的合同都指名规则包，字节不变），演示合同重发一版不指名归票 12。
- 验证（含 PG）：`go build ./...`、`gofmt`、`go vet ./internal/partycommercial/...` 通过；`./internal/partycommercial/...`、`./internal/parcelshipment/adapters/partycommercial/...`、`./internal/architecture/...`、`./cmd/parcel-commercial/...`（含受控发布回显逐字比对）、`./cmd/parcel-api/...`、`./migrations/...`、`./internal/platform/migrate/...` 全部 ok。管理台三道门：`tsc` 0、`run-tests` 1344 过 0 败、`vite build` 0。全量 `scripts/test-full.sh` 未跑。
- 评审（非作者，2026-11-11）：无阻断；两处已改（`724e2d65a`）：
  - 管理台合同列表把「正文登记了、不指名规则包」显示成「—」，两处技术详情显示空白，读起来像漏登；改为显示「不指名（规则包随服务产品选）」，删掉「库上 NOT NULL」那句过时注释。
  - 冲突判断只测了先不指名、后指名一个方向；补了先指名、后不指名答内容冲突。
  - 顺带：壳指名了规则包而正文漏写，单独答 `ErrRulePackageMissingFromContent`（包住 `ErrRulePackageReferenceMismatch`，按原错误认的调用方不受影响），与两边都写了却不一样分开。
- 评审另提一条，未采纳：登记时拒收「壳不指名、正文指名」。这是 `ErrRulePackageReferenceMismatch` 注释里早已写明的语义（壳上没指名时正文那一格只是说明，确认只读壳上，ADR-0032），本票不改；照改会让 PC 一批既有登记用例失败，要改须另立 ADR。
- 全量门禁（拣入 `feat/goods-details-admin` 后、含 09 与 10）：带 PG 的 `go test ./internal/... ./cmd/... ./referenceconfig/... ./migrations/...` 退 0；管理台 `tsc`、`run-tests` 1345 过 0 败、`vite build` 退 0。`scripts/test-full.sh` 未跑。
