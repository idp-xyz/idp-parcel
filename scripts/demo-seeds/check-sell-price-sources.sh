#!/usr/bin/env bash
# 灌完演示库之后核对票 commercial-four-module-requirements/21 的种子形状。
# 不进 seed.sh：种子是主数据，这一步是核对。要 IDP_PARCEL_POSTGRES_DSN 指向刚灌过的那只库。
set -euo pipefail

: "${IDP_PARCEL_POSTGRES_DSN:?IDP_PARCEL_POSTGRES_DSN 未设置}"

db="$(python3 -c 'import os; from urllib.parse import urlparse; print(urlparse(os.environ["IDP_PARCEL_POSTGRES_DSN"]).path.strip("/"))')"

docker exec -i idp-parcel-postgres-gate psql -U parcel -d "$db" -v ON_ERROR_STOP=1 <<'SQL'
DO $$
DECLARE
    sources text;
    old_sources int;
    account01 text;
    blank21 int;
    blank22 int;
    excl_zones text;
BEGIN
    SELECT string_agg(pricing_scope_ref, '>' ORDER BY ordinal)
      INTO sources
      FROM party_commercial.customer_contract_sell_price_source
     WHERE tenant_id = 'SYN-TENANT-01' AND object_id = 'SYN-CONTRACT-21';
    IF sources IS DISTINCT FROM 'SYN-SCOPE-01' THEN
        RAISE EXCEPTION '合同 21 的来源是 %，应只有公共范围 SYN-SCOPE-01', sources;
    END IF;

    SELECT string_agg(pricing_scope_ref, '>' ORDER BY ordinal)
      INTO sources
      FROM party_commercial.customer_contract_sell_price_source
     WHERE tenant_id = 'SYN-TENANT-01' AND object_id = 'SYN-CONTRACT-22';
    IF sources IS DISTINCT FROM 'SYN-SCOPE-EXCL-01>SYN-SCOPE-01' THEN
        RAISE EXCEPTION '合同 22 的来源是 %，应是专属范围在前、公共范围在后', sources;
    END IF;

    SELECT count(*) INTO old_sources
      FROM party_commercial.customer_contract_sell_price_source
     WHERE tenant_id = 'SYN-TENANT-01'
       AND object_id IN ('SYN-CONTRACT-01', 'SYN-CONTRACT-02');
    IF old_sources <> 0 THEN
        RAISE EXCEPTION '旧合同不该有销售价格约定，查到 % 行', old_sources;
    END IF;

    SELECT pricing_scope_id INTO account01
      FROM settlement_accounting.pre_acceptance_estimate_adoption
     WHERE tenant_id = 'SYN-TENANT-01' AND account_id = 'SYN-SETTLEMENT-ACCOUNT-01';
    IF account01 IS DISTINCT FROM 'SYN-SCOPE-01' THEN
        RAISE EXCEPTION '旧户估价采用的范围是 %，应仍是 SYN-SCOPE-01', account01;
    END IF;

    SELECT count(*) INTO blank21
      FROM settlement_accounting.pre_acceptance_estimate_adoption
     WHERE tenant_id = 'SYN-TENANT-01'
       AND account_id = 'SYN-SETTLEMENT-ACCOUNT-21'
       AND pricing_scope_id IS NULL;
    SELECT count(*) INTO blank22
      FROM settlement_accounting.sell_billing_moment_adoption
     WHERE tenant_id = 'SYN-TENANT-01'
       AND account_id = 'SYN-SETTLEMENT-ACCOUNT-22'
       AND pricing_scope_id IS NULL;
    IF blank21 <> 1 OR blank22 <> 1 THEN
        RAISE EXCEPTION '新户采用册应不带计价范围（估价 21=%，计费时点 22=%）', blank21, blank22;
    END IF;

    SELECT scope INTO excl_zones
      FROM parcel_pricing.price_card_version
     WHERE tenant_id = 'SYN-TENANT-01' AND plan_id = 'SYN-PLAN-CN-SG-EXCL-01';
    IF excl_zones IS DISTINCT FROM 'SYN-SCOPE-EXCL-01' THEN
        RAISE EXCEPTION '专属卡的范围是 %', excl_zones;
    END IF;

    IF EXISTS (
        SELECT 1 FROM parcel_pricing.price_card_version
         WHERE tenant_id = 'SYN-TENANT-01'
           AND plan_id = 'SYN-PLAN-CN-SG-EXCL-01'
           AND snapshot::text LIKE '%Z2%'
    ) THEN
        RAISE EXCEPTION '专属卡的运费表不该有 Z2';
    END IF;

    IF NOT EXISTS (
        SELECT 1 FROM parcel_pricing.price_card_version
         WHERE tenant_id = 'SYN-TENANT-01'
           AND plan_id = 'SYN-PLAN-CN-SG-EXCL-01'
           AND snapshot #>> '{plan,rateTable,firstContinue,0,zone}' = 'Z1'
           AND jsonb_array_length(snapshot #> '{plan,rateTable,firstContinue}') = 1
           AND snapshot::text LIKE '%SYN-CAT-ZONE-CN-SG%'
    ) THEN
        RAISE EXCEPTION '专属卡应只写 Z1，并绑分区目录 SYN-CAT-ZONE-CN-SG';
    END IF;
END $$;
SQL

echo "销售价格来源种子核对通过：合同 21 只有公共范围，合同 22 专属在前，旧合同没有约定，新户采用册不带范围，专属卡只有 Z1。"

if [[ -x /usr/local/go/bin/go ]]; then
  export PATH="/usr/local/go/bin:${PATH}"
fi
export IDP_PARCEL_DEMO_CHECK=1
repo="$(cd "$(dirname "$0")/../.." && pwd)"
(cd "$repo" && go test -count=1 -v -run TestDemoSellPriceWalk ./scripts/demo-seeds/sellpricewalk/)

