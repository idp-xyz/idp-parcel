# 11 SA 对账单纳入不核「结算归属日」，归属日判定也没接进任何进程

Category: bug
Status: in-progress——完工待评审与进 main，分支 claude-code11。原分级 R2（上线后尽快）：截单发布已在生产装配（`cmd/parcel-api/assemble_settlement_orchestrations.go` 的 `CutOffPublishStatementHandler`）。2026-10-21 文档自洽 02 立（用户「你作为业务和系统专家自决」授权），分支 claude-dc02。
Blocked by: 无——2026-10-21 文档自洽 09 已裁（[ADR-0269](../../../docs/adr/0269-business-time-zone-and-cutoff-belong-to-the-statement-arrangement-of-the-settlement-account-and-the-cut-admits-by-attribution-date.md)），裁向见文末「裁向（文档自洽 09，2026-10-21）」。此前：docs-consistency/09（SA+CR 修订）先裁 [audit/sa-cr.md](../audit/sa-cr.md) M11：业务时区与截单时刻的唯一权威是「对账安排」还是费用项目的归属日登记。纳入判据读哪一处，随这一裁。
出处：[audit/sa-cr.md](../audit/sa-cr.md) M7（审计钉 `6b0074d0`；2026-10-21 按 main `67438a0f` 复核，SA 代码在此期间未改）。

## 现象

CONTEXT 与 UC-SA-003 写费用要「形成结算归属日……且落入截单快照」才能入对账单；代码截单只核费用阶段是不是已确认，没有任何归属日判断。归属日判定编排存在，却没有任何进程调用。

## 证据

- 文档：
  - SA CONTEXT Rules：「费用只有在达到自身确认条件、形成结算归属日、尚未出账且落入截单快照时才能进入对账单」。
  - UC-SA-003 步 2：「读取费用和调整的确认状态、结算归属日……归属日按已登记形态判定」。
  - ADR-0168 越权风险点 3：「截单编排不调用这次判定」。
- 代码：
  - `internal/settlementaccounting/domain/customer_statement.go` 的 `CutStatementDraft`：只核 `charge.Stage() != ChargeConfirmed`（→ `ErrChargeNotConfirmed`）、币种与去重。
  - `application/cut_off_publish_statement.go`：费用清单来自命令的 `ChargeIDs`，`CutOffAt` 由调用方给。
  - `application.JudgeChargeAttributionHandler` 在 `cmd/` 下无构造；只有登记一侧接在 `cmd/parcel-settlement-register`。

## 三类

机制：截单按归属日与截单快照判纳入。产品策略：归属日形态（ADR-0168 已定为登记形态）。租户取值：业务时区、截单时刻、各费用项目的形态——未登记答未配置，不默认。

## 规模

M：截单编排读（或当场判）每件费用的归属日，归属日晚于本期截单的拒收并指名；归属日未形成的答未决、指名缺哪一格；补 UC-SA-003 的用例。

## 最小的诚实修法

截单时对命令列出的每件费用取归属日：已形成的按它判是否落入本期，未形成的当场按已登记形态判；判不出（形态、时区或截单时刻未登记）就答未配置并指名，不按调用方的列表放行。在费用确认时就形成归属日是另一条路，属同一裁定的选项。

## 要裁的

- 见 Blocked by：时区与截单时刻的权威在哪一处（M11）。
- 归属日在费用确认那一刻形成并落库，还是截单时当场判。推荐前者（与「不可变的归属」一致），随 09 的裁定写定。

## 裁向（文档自洽 09，2026-10-21）

[ADR-0269](../../../docs/adr/0269-business-time-zone-and-cutoff-belong-to-the-statement-arrangement-of-the-settlement-account-and-the-cut-admits-by-attribution-date.md) 裁定，本票照它实现，「最小的诚实修法」与「要裁的」以本节为准：

1. **对账安排登记册**：按（租户、结算账户）登记业务时区（IANA 名）与截单时刻（当天的分钟数 0–1439，0 是一份登记），可追加新一版、读最新一版；没登记答未配置。迁移另取号；`parcel-settlement-register` 另立登记命令。
2. **费用项目一册只登形态**：`domain.ChargeAttributionRegistration` 的 `timeZone`、`cutoffMinute` 退场，判定改读该费用所属结算账户的对账安排；`charge-attribution` 命令随之收窄。今天这一册没有任何种子行，迁移不必搬数。
3. **归属日随费用确认形成**：确认同笔判并落库，此后不变；判不出不挡确认，归属日留空。`domain.CustomerCharge` 今天不带来源发生时点（`CustomerChargeSpec` 没有这一格），形态为来源发生时判不出；形成费用时从它的来源事实钉上这一时点（包裹计费时点那一路可取 SELL 请求上的计价基准时点——ADR-0190 决定二定它就是这件包裹的责任起点或终局的生效时点），这一格在本票内补。
4. **截单按归属日纳入**：截单命令加「本期最后一个归属日」一格；命令列出的费用里归属日晚于它的不入本期并逐件指名，已出账的不再纳入；归属日留空的当场判（`JudgeChargeAttributionHandler` 接进截单编排）、判出即落库，判不出答未配置（`CHARGE_ATTRIBUTION_UNCONFIGURED`）并指名缺哪一册。`CutOffAt` 照旧记在草稿与快照上，不参与纳入。命令列出的费用一件都入不了本期、且其中有判不出归属日的，不形成草稿，整笔答未配置并指名缺哪一册；一件都入不了本期而其中没有判不出归属日的，也不形成草稿，逐件指名晚于本期或已出账，不答未配置——截单编排要新立这一格整笔未配置的答复（今天没有）；归属日判定的结果要补「缺哪一册」的原因格（今天 `CHARGE_ATTRIBUTION_UNCONFIGURED` 不带原因）。
5. 用例：UC-SA-003 `AT-SA-062`、`AT-SA-212`、`AT-SA-213`，UC-SA-002 主流程里确认费用的那一步（「按费用类型确认条件形成确认费用，或保持暂估」）。

建议分级不变（R2）。


## 完成记录（2026-10-21，作者会话，分支 claude-code11，基 `e0d0909cf`）

照「裁向（文档自洽 09）」五条与 ADR-0269 做完；证据只记 `S`（隔离合成）。

### 落了什么

1. **对账安排登记册**：迁移 `settlement_accounting/0045_statement_arrangement_and_charge_attribution_date.sql` 立 `statement_arrangement`（租户、结算账户、版次，外键到账户册）；`domain.StatementArrangement`、`ports.StatementArrangementRegister`/`StatementArrangementView`、`postgres.StatementArrangements`（追加一版、读最新一版）、`application.RegisterStatementArrangementHandler`；`parcel-settlement-register statement-arrangement` 命令（`registrationjson.StatementArrangementFromJSON`）。没登记答未配置，不预填。
2. **费用项目一册只登形态**：`domain.ChargeAttributionRegistration` 去掉时区与截单时刻，`Judge` 改收这件费用所属结算账户的对账安排；迁移删 `charge_attribution` 两列；`charge-attribution` 命令收窄，带 `timeZone`/`cutoffMinute` 的旧写法按未知字段拒。
3. **归属日随确认形成**：`customer_charge` 加 `source_occurred_at`、`attribution_date`（只在已确认行上，库约束挡）。`CustomerChargeSpec.SourceOccurredAt` 补上；包裹计费时点那一路（`FormEstimatedCustomerChargeHandler`）取 SELL 请求的计价基准时点；迁移按请求回指回填存量费用。`ConfirmChargeHandler` 经 `JudgeChargeAttributionHandler.JudgeConfirmedCharge` 判，与确认同笔落库；判不出不挡确认。
4. **截单按归属日纳入**：`PublishStatementCommand.LastAttributionDate`（YYYY-MM-DD）；`CutStatementDraft` 只收已形成归属日且不晚于它的费用（`ErrChargeNotAttributed`、`ErrChargeAfterPeriod`）；截单编排逐件定去向：已出账（`ports.ChargeBillingLock`，`CustomerStatements.LockChargeBilling` 锁费用行后查未作废的已发布单）→ `ALREADY_BILLED`；没有归属日的当场判（`JudgeChargeAttributionHandler` 接进截单），判出即落库（`ports.ChargeAttributionDateRecorder`，`CustomerCharges.RecordAttributionDate`），判不出 → `CHARGE_ATTRIBUTION_UNCONFIGURED` 并带缺哪一册；晚于本期 → `ATTRIBUTED_AFTER_PERIOD` 带归属日。一件都入不了本期：有判不出的整笔 `STATEMENT_CUT_UNCONFIGURED`，没有的 `NO_CHARGE_IN_PERIOD`，都逐件指名、不形成草稿。`CutOffAt` 照旧记在草稿上，不参与纳入。归属日判定结果补「缺哪一册」原因格（`ChargeAttributionResult.Missing()`：`STATEMENT_ARRANGEMENT`、`CHARGE_ATTRIBUTION_FORM`）。
5. **装配**：`cmd/parcel-api/assemble_settlement_orchestrations.go` 构造两本册与判定，交给确认与截单；`cmd/parcel-settlement-register` 装登记命令。未动 `cmd/parcel-api` 的 endpoints.go、main.go、endpoints_test.go、routes.go 与 assemble.go。
6. **文档**：UC-SA-003 状态行与「未配置结束」一行补截单整笔未配置的码，交接表删本票一行（code/13 一行留着）；UC-SA-002 交接表删本票一行；`.scratch/at-coverage-inventory.md` 把 `AT-SA-212`、`AT-SA-213` 从②、`AT-SA-062` 从③移进①（只移行，总览计数留给推送方重算）。

### 代决

1. **申报总额与入本期的行勾稽**，不与命令列出的清单勾稽：草稿只含入本期的费用，按清单申报会答 `STATEMENT_IMBALANCE`，那时不入本期的照样逐件指名。
2. **排除清单只在本次截单走到判纳入时交回**（发布、勾稽不平、本期无费用、截单未配置四格）；重放答 `EXISTING_STATEMENT`，不重判也不重给——此刻再判可能已是另一个答案。排除清单与本期最后一个归属日不另落在对账单行上，后者进发布摘要；与 `CutOffAt` 今天不落在 `customer_statement` 上同一处理。
3. **「已出账」= 在一张未作废的已发布对账单上**；整单作废后替代单可重收那几件费用。出账判断以 `FOR UPDATE` 锁费用行再查对账单行（jsonb 包含，加 GIN 索引），锁与对账单的写在同一事务里，锁按费用标识的次序取以免互等。读到的已出账恰在本命令的同一单号上时，按并发重放答已有对账单。
4. **未确认费用仍整笔答 `CHARGE_NOT_CONFIRMED`**，先于判归属日，不改成逐件排除（AT-SA-060 的既有口径）。
5. **两册都登了仍判不出**（形态为来源发生、费用上没有来源发生时点）：确认时不挡确认、留空；截单时整笔 `SOURCE_NOT_ACCEPTED`，不当缺册指名——补登哪一册都补不上。这一情形只出现在迁移回填不到的存量行。
6. **确认时读不出两本册**：确认答未决（`ATTRIBUTION_VIEW_UNAVAILABLE`）、不落确认，而不是留空归属日放行——留空会让截单改用那时在册的一版（ADR-0269 越权风险点 3 的那一差别），本是判得出的。截单时读不出同样答未决。
7. **对账安排换版**：与最新一版同内容答重放；否则追加新一版，换回更早一版的内容也落新一版；不答冲突。`Local`（服务器时区别名）不收。
8. **发布摘要只这一口换号 `SAC-2`**（多本期最后一个归属日一格），已存 `SAC-1` 发布摘要按 `SAC-1` 那一版重算比对（`domain.CompareStoredPublishDigest`），其余各口仍是 `SAC-1`。这是照 ADR-0014「改规范化形状照本记录换号」办，不另立 ADR。
9. **来源发生时点**：`FormSellCustomerChargeHandler` 要求必带（缺席答 `SOURCE_NOT_ACCEPTED`，形成之后再没有一处能补）；领域形成门不强求，存量行要能重建。迁移按 SELL 请求的 `customer_charge_id` 回指回填 `pricing_basis_at`，对不上请求的留空，不另找时点顶上。
10. **演示种子不登对账安排与归属日形态**：ADR-0269 Context 记「没有演示种子登过归属日形态」，本票不改；演示租户的确认照常确认、归属日留空，截单对没有归属日的费用答未配置。

### 越权风险点（待 SA owner 复核）

1. 存量 `SAC-1` 发布单的重放不看本期最后一个归属日：同单号换哪一天都答已有对账单（那时的截单本不按归属日纳入）。
2. 排除清单不落库、重放不重给；要复核某次截单排除了哪几件，只能看当次答复。
3. 缺来源发生时点的存量费用让整笔截单未受理，而不是只排除那一件。
4. 「已出账」以未作废为界：作废单上的费用对任何一张新单都可重收，不限于替代单（替代关系今天没有，见 code/13）。
5. 确认时读册失败让确认未决，确认编排多了一种未决原因。
6. 迁移删 `charge_attribution` 的时区与截单时刻两列不搬数：ADR-0269 只核了种子，生产库这一册有没有行本票没核；有行就丢那两格，要在对账安排上补登。
7. 存量费用的来源发生时点按 SELL 请求的计价基准时点回填；回填只在迁移里跑一次，对不上请求的行留空。
8. 申报总额对入本期的行勾稽（代决 1）：调用方若按列出的清单申报，晚于本期的费用会让整单勾稽不平而不发布。

### 新票

- [code/86](./86-statement-cut-does-not-check-that-a-listed-charge-belongs-to-the-statement-account.md)：截单与迟到纳入不核费用的结算账户是否等于这张单的账户（读码时发现，本票不改）。

### 测试（含 PG，`TMPDIR=/home/tops/.claude/jobs/1be2e64f/tmp scripts/test-full.sh`）

- 改动包的反向依赖（`go list -test` 算出 66 个包，含 `./migrations` 依赖链上的各 postgres 适配器与 `cmd/*`）加 `./internal/architecture/...`、`./migrations/...`：退 0（含 PBC-08 门禁：新写口 `SaveStatementArrangement`、`RecordAttributionDate`、`LockChargeBilling` 都有事务外被拒的同包负向证据）。
- 新用例 `-count=3`：退 0。
- 种子灌一次性库 `c11seed`：退 0，用后删库。迁移回填语句另在一次性库 `c11mig` 上对造的两行验过（对得上请求的补上、对不上的留空），用后删库；`pgtest` 的库都从空表施加迁移，迁移里的回填在用例里没有数据可跑。
- 变异（每处改完都跑红、复原后跑绿）：截单不看本期最后一个归属日；整笔未配置并进本期无费用；已出账照收；判定只报先查到的一本缺；确认不钉归属日；截单判出不落库；`LockChargeBilling` 去掉 `voided_at IS NULL`；`RecordAttributionDate` 去掉 `attribution_date IS NULL`；`LockChargeBilling` 去掉 `FOR UPDATE`（并发用例 `TestTwoCutsListingTheSameChargeAreSerializedByTheBillingLock` 报红）。
- 顺序如实记：实现先于新用例写成，「先红」是靠上面的变异把各行为逐一拿掉看到的，不是先写用例再实现。作者会话中途用过一次 `git checkout <file>` 撤回自己未提交的一处改动（摘要形状），违了派单「不用 `git checkout -- <file>`」一条；只动本 worktree 本人改动，无他人内容受影响。
