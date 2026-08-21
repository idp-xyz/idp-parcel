# 弃定与已吸收分支的指针清理（整类一次，不逐个删）

Category: enhancement
Status: resolved

[清册](../branch-merge-census-2026-08-21.md)裁决追记的执行件。**删引用是破坏性动作，执行前
须用户点头**——本票只把「删什么、怎么删、何时能删」一次写全，防下一个清点的人对剩下的
重新论证（票 02 的既有教训）。

## 何时能删

前置一个：`b394adf`（及后续裁决落地提交）已推远端——指针的补验用途在推送前不可替代。
（`bento-gate-reeval` 的蒸馏已完，结论票 resolved、行动票已立，不再是前置。）

## 删什么（按类，名单锚于清册基线 `b394adf`）

- **弃定四支**：`adr-0065-storage`（b72d96e）、`mcp3-ve008-wire`（dca024a）、
  `salvage-cons-proj-delivery-detached`（c66c97a）、`mcp1-pc-publication`（db81745）。
  裁弃记录与证据在清册第四、五节与「裁定」节（MCP-3/MCP-4 两路独立同裁）。
- **补丁全等已吸收 51 支**：名单见清册第二节。全部 cherry 全 `-`，零独有补丁。
- **`bento-gate-reeval`（2399ecd）**：清册裁定并入整类清理，**但带一道额外核查**——其蒸馏
  结论票写明「证据以分支 SHA 作索引，大输出不进 main」。删指针前先核结论票的证据索引是否
  仍需该 SHA 可达；仍需就把证据输出另行归档（或在结论票里改锚），核完才删。指针删掉后
  未被引用的对象最终会被 GC，这一步不是过度谨慎。

## 怎么删（逐支配方，照 worktree 拆除纪律移植）

```powershell
# 每支删前重验一次，不信清册的旧数——清册锚的是 b394adf，删时 main 已前进:
git cherry main <branch>          # 必须全 `-` 或空（弃定四支除外，它们按裁决删）
git log -1 <branch>               # 记 SHA 进删除记录，删后仍可从 reflog/记录找回
git branch -D <branch>
```

- 删除记录（支名 → tip SHA → 类别）**回写进本票 Comments**，一支一行——分支指针是事后
  补验的唯一凭据，删掉指针后这份记录就是替代凭据。
- 弃定四支的 tip SHA 已在清册，删时照抄并复核。

## 不删什么

- `bento-gate-reeval`：随蒸馏票定案，不在本票。
- `main` 与任何有未上游补丁的分支（删前 cherry 重验就是为了兜住「清册后又有人往上提交」）。

## Comments

- 2026-08-21 MCP-2：随清册裁决追记开出。执行等两个前置，勿抢跑。

- 2026-08-21 MCP-3（用户批复「请你直接干吧」后执行，本票转 resolved）：

  **前置核验**：用户点头已得（对本票与推送积压两项的直接批复）；推送已完成——
  `git push origin main` 后 `ls-remote` 实测远端 = `327a517`，含 `b394adf` 与全部裁决落地
  提交（`dc167e6`/`3756eeb`/`ca03b76`/`409bcf4`/`327a517`）。

  **随删先拆树**（分支被 worktree 占用时删不掉；拆树本属票 02 余量，一并执行）：33 棵
  非主树全部拆除。31 棵双查（`status --short --untracked-files=all`）为零行后不带
  `--force` 拆除；`idp-verify-t109a` 暂存着 09-A 实现，先证后拆——`diff main` 显示其暂存
  内容与 main 逐字节同（09-A 各文件根本不出现在 diff 里），所谓树侧独有行全是 `a1f283c`
  时代旧基线（main 演进后的旧影），阳性对照（`diff HEAD~1 HEAD`）有输出证工具在匹；
  证据在手后按票 02 修正口径以 `git worktree remove --force` 拆除（不走文件系统删）。

  **删支记录（一支一行，支名 → tip → 类别；51 支删前逐支重验 `git cherry main` 全 `-`）**：

  ```
  cons-final-a→21ab21c、cons-final-b→6af88ce、cons-intake-a→9215919、cons-intake-b→b29cf03、
  cons-pickup-a→43f26c9、cons-pickup-b→d3f44a5、cons-proj-delivery-a→64d8ca3、
  cons-proj-handover-b→7d7e138、cons-proj-pickup-a→47afd83、declaration-envelope-version-dedup→3bde80e、
  dispatch-db-ready→093d53c、dispatch-fanout→d5e6b94、docs-abbreviations→b82c026、
  integrate-b2→50aed5e、integrate-b3→6b18a2e、integrate-b3-v2→81707fd、integrate-b5→54bc17b、
  integrate-b5-v2→5c3d03a、integrate-b6→6e4ccda、integrate-b7→1fff679、integrate-closeout-docs→0f1ce20、
  integrate-cons-proj-a→d0de019、integrate-cons-proj-b→138d361、integrate-cons-proj-delivery-a→f388c50、
  integrate-cons-proj-pickup-a→dc38ec8、integrate-ps-index→0f40744、integrate-syn-v0→6ab9f0c、
  mcp1-adr-0065-storage→071eae0、mcp2-decl-a→7d3d35c、mcp3-pc-publication→0ec62ea、
  mcp4-proj-b→b98568b、nr-applicability-from-resolution→ce92bdd、product-version-closure-b2→1b66fa0、
  product-version-closure-b3→800fb80、product-version-closure-b4→26864d9、product-version-closure-docs→5274aad、
  ps-adopted-owner→510adb5、ps-intake-qual-evidence→aac1747、ps-parcel-index→d358ab8、
  syn-pc-product→ae49eeb、syn-pc-seed→3bb6061、syn-vertical-closure→80543a8、t-ratchet-gate→ce494ca、
  t1-02-ownership-authority→92c8e92、t1-02-takeover→8d701e4、t1-06-cc-case-config→06aaf41、
  t1-09a-takeover→12065d3、t1-09a-ve-registries→6cb78e0、t1-10b-intake-qual-wire→381c344、
  ve008-accept-rederive→a771bc3、worker-cons-proj-b→e0efdcb    ……以上 51 支类别=已吸收
  adr-0065-storage→b72d96e、mcp3-ve008-wire→dca024a、salvage-cons-proj-delivery-detached→c66c97a、
  mcp1-pc-publication→db81745    ……4 支类别=弃定（按两路同裁裁决删，独有内容随裁弃放弃）
  ```

  **保留的（与「删什么」核对）**：`bento-gate-reeval`（`2399ecd`）——票面额外核查**不过**：
  蒸馏结论票的证据索引与 bento 票 02 的工作输入（`evidence-blocks.txt` 的 53 行归属定案）
  都以该 SHA 为唯一调取路径，删指针即令证据可 GC；**bento 票 02 收口时归档或改锚后再删**。
  另五支 `syn-wall-door-audit`/`pn07-b6-stage-content`/`product-version-closure-b7`/
  `ps-rehydrate-accepted`/`cons-proj-tf-b` 属清册第三节（cherry 有 `+`、内容经锚点证已吸收），
  不在本票「删什么」范围，留待后续轮按同法处置。执行后 `git branch --list` = main + 以上六支，
  `git worktree list` 仅剩主树。
