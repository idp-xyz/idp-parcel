echo "== 预报有效时窗登记（node-operations：演示租户一行；票 ps-external-mark-relations/08） =="
# 预报有效时窗是租户取值（ADR-0212 决定二末项，PAR-NET-18）：一份送站预报只对业务时间落在〔预计送达时刻减提前量，预计送达时刻加
# 宽限量〕里的送站算数。这一行是演示租户的合成取值（提前 1440 分、宽限 720 分，2026-01-01 起不设终点），不是任何租户的预报惯例，
# 证据只记 S。没有这一行，演示租户凭预报的交付范围核对态一律答未配置。重灌同一份答已有结果。
"$BIN/parcel-node-operations-register" -kind pre-advice-window \
  -file "$SEEDS/node-operations/01-pre-advice-window-syn-tenant-01.json"
