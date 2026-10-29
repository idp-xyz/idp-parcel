import { useState, type ReactNode } from 'react';
import { X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, type WorkbenchChip, type WorkbenchColumn } from '../../templates';
import { moduleInfoById } from '../../navigation';
import { TechnicalDetails, type TechnicalRow } from '../../components/TechnicalDetails';
import { EnablementDialog, enablementColumns, useEnablement, useMayConfigureEnablement } from './EnablementControls';
import { enablementScopeNote } from './enablement';
import { useCatalogue } from './use-catalogue';
import {
  listCountries,
  listCurrencies,
  listUnits,
  type CountryListResponseBody,
  type CountryRecord,
  type CurrencyListResponseBody,
  type CurrencyRecord,
  type UnitListResponseBody,
  type UnitRecord,
} from './api';
import {
  countByMinorUnitScale,
  countByRegion,
  countByUnitCategory,
  countSummary,
  filterCountries,
  filterCurrencies,
  filterUnits,
  minorUnitScaleText,
  noMatchNote,
  referenceViewState,
  regionLabel,
  unitCategoryLabel,
  unitConversionText,
  type FilterCount,
} from './rows';

// 主责与出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['reference-data'];

// 基础数据：国家 / 地区、币种、计量单位三本产品内置目录的只读查阅（ADR-0335 决定七）。三签各接一个读口，整表取回、前端检索。
// 目录本身没有写入口——随产品发布，租户与操作者都改不了；要的码不在里面是发版的事，不是登记的事。国家和币种两签另有一列
// “启用状态”与“启用 / 停用”动作：那是本公司自己的登记（ADR-0345），登记的码只能从目录里选，登记不改目录也不拦任何业务。

function filterChips(counts: readonly FilterCount[], total: number, active: string, select: (value: string) => void): WorkbenchChip[] {
  return [
    { id: 'ALL', label: '全部', count: total, active: active === 'ALL', onClick: () => select('ALL') },
    ...counts.map((entry) => ({
      id: entry.value,
      label: entry.label,
      count: entry.count,
      active: active === entry.value,
      onClick: () => select(active === entry.value ? 'ALL' : entry.value),
    })),
  ];
}

const codeCell = (code: string) => <span className="truncate font-mono font-medium text-[color:var(--ds-text-link-hover)]">{code}</span>;

/** 详情栏：头部码加名称，几格事实，底部技术详情。目录没有可编辑的东西，所以只有关闭钮。 */
function DetailPanel({
  code,
  name,
  facts,
  note,
  technical,
  onClose,
}: {
  code: string;
  name: string;
  facts: readonly { label: string; value: ReactNode }[];
  note?: string;
  technical: readonly TechnicalRow[];
  onClose: () => void;
}) {
  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`${name} ${code}`}>
      <div className="flex items-start justify-between gap-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="min-w-0">
          <h2 className="truncate text-[16px] font-semibold text-idpxyz-textBright" title={name}>
            {name}
          </h2>
          <p className="mt-0.5 font-mono text-[12px] text-idpxyz-breadcrumb">{code}</p>
        </div>
        <Button className="shrink-0 text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
          <X className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 overflow-y-auto px-5 py-4">
        <dl className="grid grid-cols-2 gap-2.5">
          {facts.map((fact) => (
            <div key={fact.label} className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
              <dt className="text-[12px] text-idpxyz-breadcrumb">{fact.label}</dt>
              <dd className="mt-0.5 break-words text-[12px] font-semibold leading-snug text-idpxyz-textBright">{fact.value}</dd>
            </div>
          ))}
        </dl>
        {note ? <p className="mt-3 text-[12px] leading-5 text-idpxyz-breadcrumb">{note}</p> : null}
        <TechnicalDetails className="mt-4" rows={technical} />
      </div>
    </div>
  );
}

// —— 国家 / 地区 ——

const countryColumns: WorkbenchColumn<CountryRecord>[] = [
  { key: 'code', header: '代码', width: '88px', render: (row) => codeCell(row.code) },
  { key: 'name-zh', header: '中文名', render: (row) => <span className="text-idpxyz-textBright">{row.nameZh}</span> },
  { key: 'name-en', header: '英文名', hideWhenDetailOpen: true, render: (row) => <span className="text-idpxyz-text">{row.nameEn}</span> },
  { key: 'region', header: '区域', hideWhenDetailOpen: true, render: (row) => <span className="text-idpxyz-breadcrumb">{regionLabel(row.region)}</span> },
];

function CountriesTab() {
  const { answer, retry, refreshing } = useCatalogue<CountryListResponseBody>(listCountries);
  const enablement = useEnablement('COUNTRY');
  const mayConfigure = useMayConfigureEnablement();
  const [asking, setAsking] = useState<Parameters<typeof EnablementDialog>[0]['pending']>(null);
  const [search, setSearch] = useState('');
  const [region, setRegion] = useState('ALL');
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const answered = answer?.kind === 'outcome';
  const rows = answered ? answer.body.countries : [];
  const visible = filterCountries(rows, { search, region });

  return (
    <>
      <WorkbenchPageTemplate<CountryRecord>
        title="国家和地区"
        subtitle={`标准两位代码对应的国家和地区名称。${enablementScopeNote}`}
        onRefresh={() => {
          retry();
          enablement.reload();
        }}
        refreshing={refreshing || enablement.refreshing}
        search={{ value: search, onChange: setSearch, placeholder: '搜代码、中文名、英文名…' }}
        statusChips={answered ? filterChips(countByRegion(rows), rows.length, region, setRegion) : undefined}
        toolbarRight={answered ? countSummary(rows.length, visible.length, '个国家和地区') : undefined}
        columns={[...countryColumns, ...enablementColumns<CountryRecord>(enablement, mayConfigure, (row) => row.nameZh, setAsking)]}
        rows={visible}
        rowKey={(row) => row.code}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        noMatchText={noMatchNote('国家和地区')}
        onClearFilters={() => {
          setSearch('');
          setRegion('ALL');
        }}
        renderDetail={(key) => {
          const row = rows.find((item) => item.code === key);
          return row ? (
            <DetailPanel
              code={row.code}
              name={row.nameZh}
              facts={[
                { label: '英文名', value: row.nameEn },
                { label: '区域', value: regionLabel(row.region) },
              ]}
              note="区域只用来分组查找，不是服务区域或覆盖范围；那些在“服务区域”里登记。"
              technical={[
                { label: '两位代码', value: row.code },
                { label: '区域码', value: row.region },
              ]}
              onClose={() => setSelectedKey(null)}
            />
          ) : null;
        }}
        viewState={referenceViewState(answer, rows.length, retry, info, 'GET /reference-data/countries', '国家和地区')}
      />
      <EnablementDialog kind="COUNTRY" pending={asking} enablement={enablement} onClose={() => setAsking(null)} />
    </>
  );
}

// —— 币种 ——

const currencyColumns: WorkbenchColumn<CurrencyRecord>[] = [
  { key: 'code', header: '代码', width: '88px', render: (row) => codeCell(row.code) },
  { key: 'name-zh', header: '中文名', render: (row) => <span className="text-idpxyz-textBright">{row.nameZh}</span> },
  { key: 'name-en', header: '英文名', hideWhenDetailOpen: true, render: (row) => <span className="text-idpxyz-text">{row.nameEn}</span> },
  { key: 'symbol', header: '符号', width: '72px', render: (row) => <span className="font-mono text-idpxyz-text">{row.symbol}</span> },
  { key: 'scale', header: '小数位', hideWhenDetailOpen: true, render: (row) => <span className="text-idpxyz-breadcrumb">{minorUnitScaleText(row.minorUnitScale)}</span> },
];

function CurrenciesTab() {
  const { answer, retry, refreshing } = useCatalogue<CurrencyListResponseBody>(listCurrencies);
  const enablement = useEnablement('CURRENCY');
  const mayConfigure = useMayConfigureEnablement();
  const [asking, setAsking] = useState<Parameters<typeof EnablementDialog>[0]['pending']>(null);
  const [search, setSearch] = useState('');
  const [scale, setScale] = useState('ALL');
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const answered = answer?.kind === 'outcome';
  const rows = answered ? answer.body.currencies : [];
  const visible = filterCurrencies(rows, { search, scale });

  return (
    <>
      <WorkbenchPageTemplate<CurrencyRecord>
        title="币种"
        subtitle={`标准三位币种代码，以及各币种金额最小到几位小数。${enablementScopeNote}`}
        onRefresh={() => {
          retry();
          enablement.reload();
        }}
        refreshing={refreshing || enablement.refreshing}
        search={{ value: search, onChange: setSearch, placeholder: '搜代码、中文名、英文名、符号…' }}
        statusChips={answered ? filterChips(countByMinorUnitScale(rows), rows.length, scale, setScale) : undefined}
        toolbarRight={answered ? countSummary(rows.length, visible.length, '种币种') : undefined}
        columns={[...currencyColumns, ...enablementColumns<CurrencyRecord>(enablement, mayConfigure, (row) => row.nameZh, setAsking)]}
        rows={visible}
        rowKey={(row) => row.code}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        noMatchText={noMatchNote('币种')}
        onClearFilters={() => {
          setSearch('');
          setScale('ALL');
        }}
        renderDetail={(key) => {
          const row = rows.find((item) => item.code === key);
          return row ? (
            <DetailPanel
              code={row.code}
              name={row.nameZh}
              facts={[
                { label: '英文名', value: row.nameEn },
                { label: '符号', value: row.symbol },
                { label: '小数位', value: minorUnitScaleText(row.minorUnitScale) },
              ]}
              note="小数位是这个币种金额最小到哪一位：计价与结算把金额合计取整时按它定位数。目录之外的三位代码仍可使用，只是这里查不到名称。"
              technical={[
                { label: '三位代码', value: row.code },
                { label: '最小币单位小数位', value: String(row.minorUnitScale) },
              ]}
              onClose={() => setSelectedKey(null)}
            />
          ) : null;
        }}
        viewState={referenceViewState(answer, rows.length, retry, info, 'GET /reference-data/currencies', '币种')}
      />
      <EnablementDialog kind="CURRENCY" pending={asking} enablement={enablement} onClose={() => setAsking(null)} />
    </>
  );
}

// —— 计量单位 ——

function unitColumns(all: readonly UnitRecord[]): WorkbenchColumn<UnitRecord>[] {
  return [
    { key: 'code', header: '代码', width: '88px', render: (row) => codeCell(row.code) },
    { key: 'name-zh', header: '中文名', render: (row) => <span className="text-idpxyz-textBright">{row.nameZh}</span> },
    { key: 'category', header: '类别', hideWhenDetailOpen: true, render: (row) => <span className="text-idpxyz-breadcrumb">{unitCategoryLabel(row.category)}</span> },
    { key: 'conversion', header: '换算', render: (row) => <span className="text-idpxyz-text">{unitConversionText(row, all)}</span> },
  ];
}

function UnitsTab() {
  const { answer, retry, refreshing } = useCatalogue<UnitListResponseBody>(listUnits);
  const [search, setSearch] = useState('');
  const [category, setCategory] = useState('ALL');
  const [selectedKey, setSelectedKey] = useState<string | null>(null);
  const answered = answer?.kind === 'outcome';
  const rows = answered ? answer.body.units : [];
  const visible = filterUnits(rows, { search, category });

  return (
    <WorkbenchPageTemplate<UnitRecord>
      title="计量单位"
      subtitle="重量、长度、体积和件数的单位，以及它们之间精确的换算关系，只读。"
      onRefresh={retry}
      refreshing={refreshing}
      search={{ value: search, onChange: setSearch, placeholder: '搜代码、中文名…' }}
      statusChips={answered ? filterChips(countByUnitCategory(rows), rows.length, category, setCategory) : undefined}
      toolbarRight={answered ? countSummary(rows.length, visible.length, '个单位') : undefined}
      columns={unitColumns(rows)}
      rows={visible}
      rowKey={(row) => row.code}
      selectedKey={selectedKey}
      onSelect={setSelectedKey}
      noMatchText={noMatchNote('单位')}
      onClearFilters={() => {
        setSearch('');
        setCategory('ALL');
      }}
      renderDetail={(key) => {
        const row = rows.find((item) => item.code === key);
        return row ? (
          <DetailPanel
            code={row.code}
            name={row.nameZh}
            facts={[
              { label: '类别', value: unitCategoryLabel(row.category) },
              { label: '换算', value: unitConversionText(row, rows) },
            ]}
            note="换算系数是精确的定义值，不做四舍五入。同一类别的单位之间才能换算，不同类别之间没有换算；当前计价仍要求重量单位一致，不会自动换算。"
            technical={[
              { label: '单位代码', value: row.code },
              { label: '类别码', value: row.category },
              { label: '基准单位', value: row.baseUnit },
              { label: '对基准单位的系数', value: row.factorToBase },
            ]}
            onClose={() => setSelectedKey(null)}
          />
        ) : null;
      }}
      viewState={referenceViewState(answer, rows.length, retry, info, 'GET /reference-data/units', '计量单位')}
    />
  );
}

/**
 * 基础数据（产品内置目录，不属任何限界上下文）。三签各一个读口；只渲染当前签，免得三个工作台同时挂着、
 * 各自抢 `/` 与方向键。
 */
export function ReferenceDataPage() {
  const [tab, setTab] = useState('countries');
  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="countries">国家和地区</TabsTrigger>
          <TabsTrigger value="currencies">币种</TabsTrigger>
          <TabsTrigger value="units">计量单位</TabsTrigger>
        </TabsList>
        <TabsContent value="countries" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {tab === 'countries' ? <CountriesTab /> : null}
        </TabsContent>
        <TabsContent value="currencies" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {tab === 'currencies' ? <CurrenciesTab /> : null}
        </TabsContent>
        <TabsContent value="units" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          {tab === 'units' ? <UnitsTab /> : null}
        </TabsContent>
      </Tabs>
    </div>
  );
}
