-- 逐表行数：report.md 分批依据的取证脚本。
--
-- 用 query_to_xml 而不是拼一串 UNION ALL：表清单会随迁移变，写死的 UNION 会在新表
-- 加进来那天静默漏掉它，而漏掉的恰好是「这一格有没有燃料」这个判断要的输入。
--
--   Get-Content rowcount.sql -Raw | docker exec -i -e PGPASSWORD=parcel \
--     idp-parcel-postgres-gate psql -U parcel -d postgres -tA -F"|"

SELECT table_schema || '.' || table_name AS t,
       (xpath('/row/cnt/text()',
              query_to_xml(format('SELECT count(*) AS cnt FROM %I.%I', table_schema, table_name),
                           false, true, '')))[1]::text::bigint AS c
FROM information_schema.tables
WHERE table_schema NOT IN ('pg_catalog', 'information_schema', 'public')
  AND table_type = 'BASE TABLE'
ORDER BY 1;
