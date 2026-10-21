echo "== 外部资金事实与监管凭证手工采用各口的前提（pilot-governance 权威区间；ADR-0236） =="
# 配置员另持外部资金事实采用、监管凭证登记两种运营决定（data/access/02 的授予、03 的对照随 450-access 一起登）；管理台收付款核销页的
# 「采用资金事实」签与关务案件页的凭证登记签经手工口提交，铸信封时判准入范围，这里登对应的两份运营决定权威区间——少了哪一份，那一口
# 授予齐备也答 OUTSIDE_ADMISSION_SCOPE。集成客户端族的外部资金事实、监管凭证两类区间早在 400-governance 登过，两族并存，各登各的。
# 种子不经这几口采用任何事实：资金事实与凭证照旧由 500-settlement、250-customs 经受控 CLI 登入，那几行不记提交方。证据只记 S。
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/46-authority-interval-operation-decision-external-funds-fact-adoption.json"
"$BIN/parcel-governance-register" authority-interval -input "$SEEDS/governance/47-authority-interval-operation-decision-regulatory-credential-registration.json"
