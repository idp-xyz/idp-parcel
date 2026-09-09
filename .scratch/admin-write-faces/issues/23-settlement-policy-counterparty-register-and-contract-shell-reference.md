# 23 结算政策表单两条裁决落地：客户相对方从业务参与方册选、六维里的合同镜像成壳引用

Category: enhancement
Status: resolved——2026-09-09 22:3x 通道 4 交活，分支 `mcp4-awf23` tip 见下「完成记录」；进 main 的 SHA 由推送方重放后补记。此前：in-progress——22:19 通道 4 认领，分支 `mcp4-awf23` 基 `90c025ca`（= origin/main），单 task-23fabd10-e67d-4415-91fa-3847a309d9f9；ready-for-agent——2026-09-09 通道 1 代裁（用户授权自决）票 [15](./15-settlement-policy-form.md) 评审留下的两条判断题，裁决已写进
`docs/domain/party-commercial/CONTEXT.md` 结算方式那条规则末尾（客户相对方 = 业务参与方，不是货主客户账户；六维里的合同版本同时作壳引用交出）；
本票是它的落地。**Blocked by [22](./22-publication-form-private-helpers-lift-to-party-shared-layer.md)**：22 重构 `SettlementPolicyPublicationForm.tsx`
的私有件，本票改同一张表单的两格，等它抬完再改，不相撞。**2026-09-09 21:5x：22 已进 main `4209520b`，阻塞边解除**——本票现在改的是接了共享层之后的
`SettlementPolicyPublicationForm.tsx`（`Field` / `useLoaded` 从 `PublicationFormFields` 导入；`ContractPicker` 仍留在本册）
Blocked by: 22（已进 main）

## 两条裁决与理由

1. **客户相对方引用解析到业务参与方册，不是货主客户账户册。** GLOSSARY「货主客户账户」：「一个货主客户账户可以按责任法人、相对方、方向、币种和结算
   政策拥有多个结算账户」——相对方是账户之下细分结算账户的键，与账户不是同一对象；CONTEXT-MAP「结算账户是法人、相对方、方向、币种和结算政策共同确定的
   金额责任边界；两者不能合并」。相对方是承担结算责任的**法律主体**，那是业务参与方册的对象。今天表单从 `listCustomerAccounts` 取选单是票 15 作者按 seed
   的写法（seed 用账户标识），两读都通所以没锁死；现在锁死。`domain.CounterpartyReference` 仍是未绑定册的 `requiredValue`（存在性不由构造门查，
   与其它开放引用同），本票只改**表单选单的来源与读面的显法**，不给领域加存在性校验。
2. **六维里的合同版本同时镜像成版本壳的指名引用 `references.CUSTOMER_CONTRACT`。** 没有它，结算政策在被引合同未发布时照样发布，失去「被引合同
   未发布 → `发布未决`」那道排序门；seed 的结算政策壳本来就带着它，`CustomerContractPublicationForm` 也有 `alsoPaths=['references.ACCEPTANCE_RULE_PACKAGE',…]`
   的先例。表单不给操作者第二个输入格——**从六维里选出的合同自动写进壳引用**（同一个选择、两个落点），载荷层若两处都在场且不同答问题、点名
   `references.CUSTOMER_CONTRACT`。

## 完成判据

1. `SettlementPolicyPublicationForm.tsx`：相对方选单 `load` 换成 `listBusinessParties`（`GET /commercial-business-parties`），选项显示业务参与方的名称 +
   标识、值为参与方标识；目录 403 / 读不到仍退回手填格（既有行为）；不预选。
2. `settlement-policy-form.ts`：载荷生成把六维 `contract{objectId, version}` 的 `objectId` 同时写进壳 `references.CUSTOMER_CONTRACT`；node:test 钉「选了合同 →
   壳引用同值」「没选合同 → 壳无该键」（不送空串）。
3. Go 载荷层 `SettlementPolicyBodyPayload.body`（或壳解码处，作者定、写理由）：`references.CUSTOMER_CONTRACT` 在场且与六维 `contract.objectId` 不同 →
   问题路径 `references.CUSTOMER_CONTRACT`；缺席不补（旧载荷 / 受控批文照发）。传输面测试各一正一反。
4. 读面：第 N 册结算政策的「相对方」列显法不变（仍显引用串）；若 `presentation.ts` 有按册解析名称的机制则接业务参与方册，没有就不加（写进完成记录）。
5. demo seed `publish-batch.json` 的结算政策项：相对方若填的是客户账户标识，改成对应业务参与方标识（SYN 数据内自洽即可）；壳引用已带着，不动；
   `contentDigest` 若因正文改动而变，重算 PCC-1 并写明算法出处（同 15 / 13 的补笔做法）。
6. tsc / run-tests 绿；Go：PC 四包 + `cmd/parcel-api` + `internal/architecture`，**并带 DSN 跑 `./cmd/parcel-commercial/`**（同形票的教训，见
   parallel-sessions「验证」节 2026-09-09 补的那句）。

## 边界

不动 0011 与结算政策领域构造门；不给相对方加存在性校验；不动 settlement-accounting 的读路径；不改 `PublicationDraftFlow`。

## 完成记录（通道 4 · 2026-09-09 22:3x · 分支 `mcp4-awf23`，基 `90c025ca`）

### 逐笔（分支上的 SHA；进 main 后由推送方在「进 main 记录」写对照）

| 笔 | SHA | 内容 |
|---|---|---|
| 0 | `a9d7ef7b` | 票面 in-progress |
| 1 | `e906e012` | `settlement-policy-form.ts`：`settlementPolicyPayloadOf` 在 `contract.objectId` 非空时写 `references.CUSTOMER_CONTRACT` 同值，没选合同则壳无该键；认领表与渲染表各加 `references.CUSTOMER_CONTRACT`；`ContractPicker` 两种形态都显它（选单态 `PathProblems` 三条，手填态对象格 `alsoPaths`）；node:test 先 red 后 green 钉两态 |
| 2 | `7cd71479` | `SettlementPolicyPublicationForm.tsx`：相对方 `ReferencePicker` 的 `load` 换 `listBusinessParties`，选项 `partyId · partyName · status`、值为 `partyId`；标签 / 空册句 / 读面名三处文案改成业务参与方 |
| 3 | `08af0cf1` | Go 载荷层：`publication_draft_payload_settlement_policy.go` 新增 `SettlementPolicyBodyPayload.reconcileShellReference`；`publication_draft_payload.go` 的 `Publication` 在结算政策那一块**加一行**调用（占号 22:2x 广播、释号 22:3x 广播）；`TestSettlementPolicyShellReferenceMustAgreeWithTheSixDimensionContract` 一正一反加缺席不补 |
| 4 | —— | 读面无改动（见判据 4） |
| 5 | `9216dfff` | demo seed：`publish-batch.json` 结算政策项 `counterparty` → `SYN-PARTY-SHIPPER-01`，`contentDigest` 重算为 `PCC-1:c4cbd2ee81639a26ca49e8070ff27950ad04c5636ec29c20fd1887f00a2759aa`；**同笔** `resolution-key-syn-account-01.json` 的 `settlement.counterparty` 同改（理由见判据 5） |
| 6 | 本笔 | 票面 resolved + 本节 |

### 判据逐项

1. **相对方选单换业务参与方册** ✓ `7cd71479`。`load={listBusinessParties}`（既有读口 `GET /commercial-business-parties`，未新开）；选项显 `partyId · partyName · status`，值为 `partyId`。顺序取「标识 · 名称 · 状态」与同格旁责任法人选单一致——票面写的「名称 + 标识」列的是显什么，两样都在。403 / 读不到退手填、不预选：`ReferencePicker` 既有行为，共享层一行未动。
2. **载荷镜像壳引用** ✓ `e906e012`。`references.CUSTOMER_CONTRACT = contract.objectId`（去首尾空白后）；没选合同 → 载荷上没有 `references` 键（不送空串）。node:test「六维里选出的合同同时作壳引用 references.CUSTOMER_CONTRACT；没选合同则壳无该键」两态都钉；首条载荷形状测试的期望同笔带上 `references`。
3. **Go 载荷层一致性问题路径** ✓ `08af0cf1`。落点是 `reconcileShellReference`（新方法，本册文件），不并进 `body`：`body` 只看得见正文一格，壳引用在 `CommercialPublicationPayload` 上。两处都在场且不同 → `references.CUSTOMER_CONTRACT`（壳是派生的一侧）；壳上缺席不补；壳引用空串或六维那格空不比（各自已被 `Publication` / `body` 报过）。**因此 `publication_draft_payload.go` 动了一行**（`git diff --numstat` 1/0，`Publication` 里 `payload.SettlementPolicy != nil` 那一块末尾一句调用）——派单允许「非碰不可先占号」，占号 / 释号两条广播都发了；通道 3 wbr/10 改的 `CreditPolicyBodyPayload` 结构体与 `body()` 与此不相邻。传输面测试：同值 → 壳引用进 `PublicationDraftShell.References[CustomerContractObject]`；不同 → 问题字段恰是 `references.CUSTOMER_CONTRACT` 且 `settlementPolicy.contract*` 三条都不被连带；无壳引用的原载荷照翻、壳上不长出引用。
4. **读面** ✓ 无改动。`policy-rows.ts` 的 `SETTLEMENT_POLICY` 列 `counterparty: record.counterparty` 原样显引用串，显法不变。`presentation.ts` 只有码 → 中文的词表与提示句，**没有按册解析名称的机制**（`git grep -i "nameOf|resolveName|nameKnown|partyName"` 在 `presentation.ts` / `policy-rows.ts` / `CommercialPoliciesPage.tsx` 零命中，钉 `9216dfff`），按票面「没有就不加」。
5. **demo seed** ✓ `9216dfff`。`SYN-SETTLEMENT-PREPAID-01/v1` 正文 `counterparty`：`SYN-ACCOUNT-01`（账户）→ `SYN-PARTY-SHIPPER-01`（`register-parties.json` 里该账户的 `customerPartyId`）；壳引用 `CUSTOMER_CONTRACT: SYN-CONTRACT-01` 已带着未动。`contentDigest` 重算：取证同 awf/15（`7e2e2d8f`）——隔离树临时探针（未提交、已删）读本文件 → `publishCommandsFromJSON` → 按 `publicationContentOf` 同形折成 `domain.SettlementPolicyBody{Method, Applicability}` → `CanonicalizePublicationContent` → `ReconcileDeclaredDigest` 改前 mismatch、改后 nil。**地盘外一处同笔改**：`resolution-key-syn-account-01.json` 的 `settlement.counterparty` 同改——闭包按解析键选择器的精确六维选结算政策（`resolveSettlementPolicyBasis` → `NewSettlementQuery` 带 `key.Settlement.Counterparty`），只改发布批那一格，演示库闭包会从 `唯一解析` 退回 `无适用依据`（`seed.sh` 那段注释与 README「已知边界」都写着两份是配着的）；键上的 `customerAccountId` 仍是 `SYN-ACCOUNT-01`——账户与相对方是两个对象，这一改恰恰把它们分开。已用旧正文施加过的演示库重放本项会答 `CONTENT_CONFLICT`，要重建库再 seed。`cmd/parcel-commercial` 的夹具**不读**这份 seed（自带内联批文），派单那句「夹具用例会吃这份 seed」不成立，带 DSN 跑它照做了（见验证）。
6. **验证** ✓ 钉 `9216dfff`，隔离树 `D:/tops/idp-parcel-mcp4-awf23`（干净检出，`git status --untracked-files=all` 空）：
   - admin-web：`tsc -b --force` 0 错；`run-tests` **196/196**（基线 195 + 本票新钉 1）。
   - Go：`gofmt -l .` 空；`go build ./...` 0；`go vet ./...` 0；`go test -count=1 ./internal/partycommercial/... ./cmd/parcel-api/... ./internal/architecture/...` 全 ok（不带 DSN，PC postgres 一包 0.017s 即跳过态）。
   - **带 DSN**（占 55432 22:3x 广播、释 22:3x 广播）：`go test -p 1 -count=1 -v ./cmd/parcel-commercial/ ./cmd/parcel-api/...` → ok 3.7s / ok 2.1s，`--- PASS` 198 · `--- SKIP` 0 · `--- FAIL` 0。
   - 机制清点：本树 `tools/mechanism-inventory` 重生成，`docs/product/MECHANISM-INVENTORY.md` 零差（Go 只改既有文件、未新增文件或声明）。
   - 未跑：全量、`-race`（派单明写不跑）。

### 判断题（留给评审 / 推送方）

- **可见文案多改了一句**（`e906e012`）：版本壳说明里「壳上不带指名引用——政策约定的合同在正文六维里」在第 1 步之后为假，改成「壳上的指名引用不另填——正文六维里选出的合同同时作壳引用交出」。派单说文案只改相对方那一格；这一句是本票自己让它变假的，留着是给操作者看假话，改动只此一句。评审若裁「不该改」，撤回是一行。
- **地盘外两处**：`publication_draft_payload.go` 一行（判据 3，派单预留了口子并已占号 / 释号）；`resolution-key-syn-account-01.json` 一格（判据 5，不改则 seed 自相矛盾）。
- **选项顺序**：「标识 · 名称 · 状态」而非「名称 · 标识」，取与同格旁法人选单一致；若裁按票面字面顺序，改一行模板串。
- **`reconcileShellReference` 的空值口径**：壳引用为空串 / 六维合同对象为空时不比（各自已被点名）。另一种口径是空也算「在场」再报一次不一致；没取，理由是第二处说同一件事。

### 未落

- `seed.sh` 里「SYN-LE-01 与 SYN-ACCOUNT-01 在此获得身份册登记，与上面发布批里的同名引用同指一物」那句注释：发布批里现在不再出现 `SYN-ACCOUNT-01`（它只在上一行的解析键 `customerAccountId` 里），句子松了半格但不假，`seed.sh` 不在地盘，未动。
- 领域 `CounterpartyReference` 仍是未绑定册的 `requiredValue`，存在性不由构造门查——票面明写不加，未动。
- 工作树 `D:/tops/idp-parcel-mcp4-awf23` 与其 `apps/admin-web/node_modules` **目录联接**留给推送方：拆树前先 `cmd /c rmdir D:\tops\idp-parcel-mcp4-awf23\apps\admin-web\node_modules` 摘掉联接再 `git worktree remove`（不加 `--force`）。
