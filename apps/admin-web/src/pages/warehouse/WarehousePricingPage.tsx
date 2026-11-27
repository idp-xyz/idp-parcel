import { useMemo, useState, type ReactNode } from 'react';
import { CalendarClock, Plus, Settings, X } from 'lucide-react';
import { Button, Tabs, TabsContent, TabsList, TabsTrigger } from '@idpxyz/ui-primitives';
import { WorkbenchPageTemplate, toggleWorkbenchSort, type WorkbenchColumn, type WorkbenchSort } from '../../templates';
import type { TemplateViewState } from '../../templates/state-slot';
import { useTabViewState } from '../../templates/workspace-view-state';
import { primaryActionClass } from '../../components/primary-action';
import { moduleInfoById } from '../../navigation';
import type { CatalogueConditions } from '../catalogue-query';
import { catalogueViewState, formatInstant } from '../catalogue-view';
import { heldCataloguePage } from '../party/held-catalogue-page';
import { CursorPagerBar, pastFirstPage } from '../shipment-request/cursor-pager';
import { useCatalogueCursor, useSettledKeyword } from '../use-catalogue-cursor';
import {
  listChargeOccurrences,
  listWarehouseAdoptions,
  listWarehouseChargeRequests,
  listWarehousePlans,
  type ChargeOccurrenceRecord,
  type WarehouseAdoptionRecord,
  type WarehouseChargeRequestRecord,
  type WarehousePlanRecord,
} from './api';
import { pricingActionSpecs } from './actions';
import { ActionDrawer, type ActiveDrawerAction } from './ActionDrawer';
import { useWarehouseTargetMap } from './ActionPanel';
import type { GuidedFormValues } from './guided-forms';
import { fitColumnWidth, nameOf, shortInstant } from './list-cells';
import { useWarehouseLookups, type WarehouseLookups } from './use-catalogues';
import { labelOf, WmsBadge, WmsTwoLine } from './wms-ui';

const info = moduleInfoById['warehouse-pricing'];
const moduleId = 'warehouse-pricing';

const nowrap = (content: ReactNode) => <span className="whitespace-nowrap">{content}</span>;

/** 缺省最新的在上面。人点过表头就照他的。 */
function newestSort(stored: string | null, field: string): WorkbenchSort {
  if (stored === field) return { key: field, dir: 1 };
  return { key: field, dir: -1 };
}

function namesOf(lookups: WarehouseLookups) {
  return {
    owners: new Map(lookups.owners.map((account) => [account.accountId, account.customerPartyName ?? ''])),
    warehouses: new Map(lookups.warehouses.map((warehouse) => [warehouse.warehouseCode, warehouse.name ?? ''])),
  };
}

function named(names: ReadonlyMap<string, string>, code: string) {
  const name = nameOf(names, code);
  return name ? <WmsTwoLine primary={name} secondary={code} code title={`${name} · ${code}`} /> : <span className="whitespace-nowrap font-mono">{code}</span>;
}

function SideDetail({
  kind,
  code,
  badge,
  subtitle,
  children,
  onClose,
}: {
  kind: string;
  code: string;
  badge?: ReactNode;
  subtitle?: string;
  children: ReactNode;
  onClose: () => void;
}) {
  return (
    <div className="flex h-full min-h-0 flex-col overflow-hidden bg-idpxyz-editor">
      <div className="flex shrink-0 items-start justify-between gap-2 border-b border-idpxyz-border px-4 py-3">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <span className="type-section font-semibold text-idpxyz-textBright">{kind}</span>
            <span className="font-mono">{code}</span>
            {badge}
          </div>
          {subtitle ? <p className="mt-1 type-caption text-idpxyz-textMuted">{subtitle}</p> : null}
        </div>
        <Button type="button" variant="ghost" size="icon" title="关闭详情" aria-label="关闭详情" onClick={onClose}>
          <X className="h-4 w-4" aria-hidden="true" />
        </Button>
      </div>
      <div className="min-h-0 flex-1 space-y-3 overflow-auto p-4 type-body">{children}</div>
    </div>
  );
}

function stepButton(label: string, onClick: () => void) {
  return (
    <Button
      type="button"
      size="sm"
      variant="outline"
      className="whitespace-nowrap text-[color:var(--ds-text-link-hover)] hover:text-[color:var(--ds-text-link-hover)]"
      onClick={(event) => {
        event.stopPropagation();
        onClick();
      }}
    >
      {label}
    </Button>
  );
}

function PlansTab({
  refreshKey,
  can,
  onOpen,
}: {
  refreshKey: number;
  can: (id: string) => boolean;
  onOpen: (id: string, prefill?: GuidedFormValues) => void;
}) {
  const field = 'effectiveFrom';
  const [keyword, setKeyword] = useTabViewState(`${moduleId}.plans.keyword`, '');
  const [storedSort, setSort] = useTabViewState<string | null>(`${moduleId}.plans.sort`, null);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.plans.open`, null);
  const settled = useSettledKeyword(keyword);
  const sort = newestSort(storedSort, field);
  const conditions: CatalogueConditions = { q: settled, sort: sort.dir === 1 ? field : `-${field}` };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listWarehousePlans);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const held = heldCataloguePage(answer, previous, (body) => body.plans.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.plans : (held?.body.plans ?? []);
  const now = new Date();
  const columns: WorkbenchColumn<WarehousePlanRecord>[] = [
    {
      key: 'id',
      header: '方案',
      width: fitColumnWidth,
      render: (row) => <WmsTwoLine primary={<span className="font-mono">{row.id}</span>} secondary={row.version} title={`${row.id}\n${row.version}`} />,
    },
    { key: 'scope', header: '定价口径', render: (row) => <span className="whitespace-nowrap font-mono">{row.scope}</span> },
    { key: 'currency', header: '币种', width: fitColumnWidth, minListWidth: 860, render: (row) => nowrap(row.currency) },
    {
      key: 'counts',
      header: nowrap('分组 / 规则'),
      right: true,
      width: fitColumnWidth,
      minListWidth: 1040,
      render: (row) => <span className="whitespace-nowrap tabular-nums">{`${row.groupCount} / ${row.ruleCount}`}</span>,
    },
    {
      key: 'from',
      header: nowrap('生效'),
      right: true,
      width: fitColumnWidth,
      sortKey: field,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.effectiveFrom)}>
          {shortInstant(row.effectiveFrom, now) || '—'}
        </span>
      ),
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => (can('charge-adoption') ? stepButton('登记采用', () => onOpen('charge-adoption', { pricingScope: row.scope })) : <span className="text-idpxyz-textMuted">—</span>),
    },
  ];
  const narrowed = settled.trim() !== '' || pastFirstPage(pagination);
  const clear = () => {
    setKeyword('');
    setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /pricing-warehouse-plan-list',
        emptyTitle: '还没有登记仓储计费方案',
        emptyDescription: '点右上角「登记计费方案」为操作费、仓租或增值服务建价卡。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;
  return (
    <WorkbenchPageTemplate<WarehousePlanRecord>
      title="仓储计费方案"
      viewStateKey={`${moduleId}.plans`}
      subtitle="点一行在右边看这版方案。生效时刻最新的在上面"
      onRefresh={retry}
      refreshing={answer === null}
      primaryAction={
        <>
          <Button type="button" variant="outline" onClick={() => { window.location.hash = '#/warehouse-settings'; }}>
            <Settings className="mr-1 h-3.5 w-3.5" />
            计费口径设置
          </Button>
          {can('warehouse-plan') ? (
            <Button type="button" className={primaryActionClass} onClick={() => onOpen('warehouse-plan')}>
              <Plus className="mr-1 h-3.5 w-3.5" />
              登记计费方案
            </Button>
          ) : null}
        </>
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜方案标识或口径' }}
      extraFilters={narrowed ? <Clear onClear={clear} /> : undefined}
      columns={columns}
      rows={rows}
      rowKey={(row) => `${row.id}/${row.version}`}
      selectedKey={openKey}
      onSelect={setOpenKey}
      rowAlign="baseline"
      sort={sort}
      onSort={(key) => {
        const next = toggleWorkbenchSort(sort, key);
        setSort(next.dir === 1 ? field : `-${field}`);
      }}
      noMatchText="当前条件下没有仓储计费方案"
      onClearFilters={clear}
      renderDetail={(key) => {
        const row = rows.find((item) => `${item.id}/${item.version}` === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这个方案。</p>;
        return (
          <SideDetail kind="计费方案" code={row.id} subtitle={`${row.version} · ${row.scope}`} onClose={() => setOpenKey(null)}>
            <p>币种 {row.currency} · 声明值 {row.declaredValue}</p>
            <p className="tabular-nums">分组 {row.groupCount} · 规则 {row.ruleCount}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">生效 {formatInstant(row.effectiveFrom)}</p>
            {can('charge-adoption') ? stepButton('为此方案登记货主采用', () => onOpen('charge-adoption', { pricingScope: row.scope })) : null}
          </SideDetail>
        );
      }}
      viewState={viewState}
      listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
    />
  );
}

function AdoptionsTab({
  refreshKey,
  lookups,
  can,
  onOpen,
}: {
  refreshKey: number;
  lookups: WarehouseLookups;
  can: (id: string) => boolean;
  onOpen: (id: string, prefill?: GuidedFormValues) => void;
}) {
  const field = 'effectiveAt';
  const [keyword, setKeyword] = useTabViewState(`${moduleId}.adoptions.keyword`, '');
  const [storedSort, setSort] = useTabViewState<string | null>(`${moduleId}.adoptions.sort`, null);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.adoptions.open`, null);
  const settled = useSettledKeyword(keyword);
  const sort = newestSort(storedSort, field);
  const conditions: CatalogueConditions = { q: settled, sort: sort.dir === 1 ? field : `-${field}` };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listWarehouseAdoptions);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const held = heldCataloguePage(answer, previous, (body) => body.adoptions.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.adoptions : (held?.body.adoptions ?? []);
  const maps = useMemo(() => namesOf(lookups), [lookups]);
  const now = new Date();
  const nextRevision = (row: WarehouseAdoptionRecord) =>
    onOpen('charge-adoption', {
      ownerAccount: row.ownerAccount,
      warehouse: row.warehouseCode,
      revision: String(row.revision + 1),
      account: row.account,
      pricingScope: row.pricingScope,
    });
  const columns: WorkbenchColumn<WarehouseAdoptionRecord>[] = [
    { key: 'owner', header: '货主', width: fitColumnWidth, render: (row) => named(maps.owners, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 860, render: (row) => named(maps.warehouses, row.warehouseCode) },
    { key: 'scope', header: '定价口径', render: (row) => <span className="whitespace-nowrap font-mono">{row.pricingScope}</span> },
    { key: 'account', header: '结算账户', width: fitColumnWidth, minListWidth: 1080, render: (row) => <span className="whitespace-nowrap font-mono">{row.account}</span> },
    { key: 'rev', header: '修订', width: fitColumnWidth, render: (row) => <span className="whitespace-nowrap tabular-nums">{`r${row.revision}`}</span> },
    {
      key: 'eff',
      header: nowrap('生效'),
      right: true,
      width: fitColumnWidth,
      sortKey: field,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.effectiveAt)}>
          {shortInstant(row.effectiveAt, now) || '—'}
        </span>
      ),
    },
    {
      key: 'next',
      header: nowrap('下一步'),
      width: fitColumnWidth,
      render: (row) => (can('charge-adoption') ? stepButton('下一修订', () => nextRevision(row)) : <span className="text-idpxyz-textMuted">—</span>),
    },
  ];
  const narrowed = settled.trim() !== '' || pastFirstPage(pagination);
  const clear = () => {
    setKeyword('');
    setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-adoption-list',
        emptyTitle: '还没有登记计费采用',
        emptyDescription: '点右上角「登记计费采用」把货主与仓绑定到仓储计费方案。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;
  return (
    <WorkbenchPageTemplate<WarehouseAdoptionRecord>
      title="货主计费采用"
      viewStateKey={`${moduleId}.adoptions`}
      subtitle="点一行在右边看。采用后新发生的收费项会自动发出销售计费请求"
      onRefresh={retry}
      refreshing={answer === null}
      primaryAction={
        can('charge-adoption') ? (
          <Button type="button" className={primaryActionClass} onClick={() => onOpen('charge-adoption')}>
            <Plus className="mr-1 h-3.5 w-3.5" />
            登记计费采用
          </Button>
        ) : undefined
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜货主、仓或定价口径' }}
      extraFilters={narrowed ? <Clear onClear={clear} /> : undefined}
      columns={columns}
      rows={rows}
      rowKey={(row) => `${row.ownerAccount}/${row.warehouseCode}/${row.revision}`}
      selectedKey={openKey}
      onSelect={setOpenKey}
      rowAlign="baseline"
      sort={sort}
      onSort={(key) => {
        const next = toggleWorkbenchSort(sort, key);
        setSort(next.dir === 1 ? field : `-${field}`);
      }}
      noMatchText="当前条件下没有计费采用记录"
      onClearFilters={clear}
      renderDetail={(key) => {
        const row = rows.find((item) => `${item.ownerAccount}/${item.warehouseCode}/${item.revision}` === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这条采用。</p>;
        return (
          <SideDetail kind="计费采用" code={`r${row.revision}`} subtitle={`${row.ownerAccount} · ${row.warehouseCode}`} onClose={() => setOpenKey(null)}>
            <p className="font-mono">结算账户 {row.account}</p>
            <p className="font-mono">定价口径 {row.pricingScope}</p>
            <p>依据 {row.basis || '—'}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">生效 {formatInstant(row.effectiveAt)}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">登记人 {row.registrant}</p>
            {can('charge-adoption') ? stepButton(`登记下一修订（r${row.revision + 1}）`, () => nextRevision(row)) : null}
          </SideDetail>
        );
      }}
      viewState={viewState}
      listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
    />
  );
}

function OccurrencesTab({
  refreshKey,
  lookups,
  can,
  onOpen,
}: {
  refreshKey: number;
  lookups: WarehouseLookups;
  can: (id: string) => boolean;
  onOpen: (id: string, prefill?: GuidedFormValues) => void;
}) {
  const field = 'businessTime';
  const [keyword, setKeyword] = useTabViewState(`${moduleId}.occurrences.keyword`, '');
  const [storedSort, setSort] = useTabViewState<string | null>(`${moduleId}.occurrences.sort`, null);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.occurrences.open`, null);
  const settled = useSettledKeyword(keyword);
  const sort = newestSort(storedSort, field);
  const conditions: CatalogueConditions = { q: settled, sort: sort.dir === 1 ? field : `-${field}` };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listChargeOccurrences);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const held = heldCataloguePage(answer, previous, (body) => body.occurrences.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.occurrences : (held?.body.occurrences ?? []);
  const maps = useMemo(() => namesOf(lookups), [lookups]);
  const now = new Date();
  const columns: WorkbenchColumn<ChargeOccurrenceRecord>[] = [
    {
      key: 'id',
      header: '发生项',
      width: fitColumnWidth,
      render: (row) => (
        <WmsTwoLine primary={<span className="font-mono">{row.occurrenceId}</span>} secondary={`v${row.version}`} title={`${row.occurrenceId}\nv${row.version}`} />
      ),
    },
    { key: 'kind', header: '收费类别', width: fitColumnWidth, render: (row) => nowrap(labelOf(row.kind)) },
    { key: 'owner', header: '货主', width: fitColumnWidth, minListWidth: 900, render: (row) => named(maps.owners, row.ownerAccount) },
    { key: 'warehouse', header: '仓', width: fitColumnWidth, minListWidth: 1080, render: (row) => named(maps.warehouses, row.warehouseCode) },
    { key: 'correction', header: '原项 / 更正', width: fitColumnWidth, minListWidth: 1220, render: (row) => nowrap(labelOf(row.correction)) },
    {
      key: 'time',
      header: nowrap('业务时刻'),
      right: true,
      width: fitColumnWidth,
      sortKey: field,
      render: (row) => (
        <span className="whitespace-nowrap tabular-nums" title={formatInstant(row.businessTime)}>
          {shortInstant(row.businessTime, now) || '—'}
        </span>
      ),
    },
  ];
  const narrowed = settled.trim() !== '' || pastFirstPage(pagination);
  const clear = () => {
    setKeyword('');
    setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-occurrences',
        emptyTitle: '还没有收费发生项',
        emptyDescription: '完成仓内收货/出库作业，或点右上角「触发仓租日结」后会生成收费发生项。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;
  return (
    <WorkbenchPageTemplate<ChargeOccurrenceRecord>
      title="仓储收费发生项"
      viewStateKey={`${moduleId}.occurrences`}
      subtitle="点一行在右边看。业务时刻最新的在上面"
      onRefresh={retry}
      refreshing={answer === null}
      primaryAction={
        can('storage-day-close') ? (
          <Button type="button" className={primaryActionClass} onClick={() => onOpen('storage-day-close')}>
            <CalendarClock className="mr-1 h-3.5 w-3.5" />
            触发仓租日结
          </Button>
        ) : undefined
      }
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜发生项号、货主或仓' }}
      extraFilters={narrowed ? <Clear onClear={clear} /> : undefined}
      columns={columns}
      rows={rows}
      rowKey={(row) => `${row.occurrenceId}/v${row.version}`}
      selectedKey={openKey}
      onSelect={setOpenKey}
      rowAlign="baseline"
      sort={sort}
      onSort={(key) => {
        const next = toggleWorkbenchSort(sort, key);
        setSort(next.dir === 1 ? field : `-${field}`);
      }}
      noMatchText="当前条件下没有收费发生项"
      onClearFilters={clear}
      renderDetail={(key) => {
        const row = rows.find((item) => `${item.occurrenceId}/v${item.version}` === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这条发生项。</p>;
        const measures = row.measures.map((measure) => `${measure.unit}:${measure.unmeasured ? '未量' : measure.quantity ?? ''}`).join('，');
        const attributes = row.attributes.map((attribute) => `${attribute.name}=${attribute.value}`).join('，');
        return (
          <SideDetail kind="收费发生项" code={row.occurrenceId} subtitle={`v${row.version} · ${labelOf(row.kind)}`} onClose={() => setOpenKey(null)}>
            <p>
              {row.ownerAccount} · {row.warehouseCode}
            </p>
            <p className="font-mono">来源事实 {row.sourceFact}</p>
            <p>{labelOf(row.correction)}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">业务时刻 {formatInstant(row.businessTime)}</p>
            <p className="font-mono">计费量 {measures || '—'}</p>
            <p className="font-mono">附加属性 {attributes || '—'}</p>
          </SideDetail>
        );
      }}
      viewState={viewState}
      listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
    />
  );
}

function RequestsTab({ refreshKey }: { refreshKey: number }) {
  const field = 'occurrenceId';
  const [keyword, setKeyword] = useTabViewState(`${moduleId}.requests.keyword`, '');
  const [storedSort, setSort] = useTabViewState<string | null>(`${moduleId}.requests.sort`, null);
  const [openKey, setOpenKey] = useTabViewState<string | null>(`${moduleId}.requests.open`, null);
  const settled = useSettledKeyword(keyword);
  const sort = newestSort(storedSort, field);
  const conditions: CatalogueConditions = { q: settled, sort: sort.dir === 1 ? field : `-${field}` };
  const { answer, previous, pagination, retry } = useCatalogueCursor(conditions, listWarehouseChargeRequests);
  const [last, setLast] = useState(refreshKey);
  if (refreshKey !== last) {
    setLast(refreshKey);
    retry();
  }
  const held = heldCataloguePage(answer, previous, (body) => body.requests.length > 0);
  const rows = answer?.kind === 'outcome' ? answer.body.requests : (held?.body.requests ?? []);
  const columns: WorkbenchColumn<WarehouseChargeRequestRecord>[] = [
    {
      key: 'req',
      header: '计费请求',
      width: fitColumnWidth,
      render: (row) => <span className="whitespace-nowrap font-mono">{row.requestId ?? '未发请求'}</span>,
    },
    {
      key: 'occ',
      header: '发生项',
      width: fitColumnWidth,
      sortKey: field,
      render: (row) => (
        <WmsTwoLine
          primary={<span className="font-mono">{row.occurrenceId}</span>}
          secondary={`v${row.version}`}
          title={`${row.occurrenceId}\nv${row.version}`}
        />
      ),
    },
    { key: 'status', header: '状态', width: fitColumnWidth, render: (row) => <WmsBadge code={row.status} label={labelOf(row.status)} /> },
    {
      key: 'account',
      header: '结算账户',
      render: (row) => (
        <WmsTwoLine
          primary={<span className="font-mono">{row.account ?? '—'}</span>}
          secondary={row.settlementCurrency}
          title={`${row.account ?? ''} ${row.settlementCurrency ?? ''}`}
        />
      ),
    },
    { key: 'charge', header: '计费结果', width: fitColumnWidth, minListWidth: 1100, render: (row) => <span className="whitespace-nowrap font-mono">{row.chargeId ?? '—'}</span> },
  ];
  const narrowed = settled.trim() !== '' || pastFirstPage(pagination);
  const clear = () => {
    setKeyword('');
    setSort(null);
  };
  const viewState: TemplateViewState = held
    ? { kind: 'ready' }
    : catalogueViewState(answer, rows.length, retry, {
        module: info,
        endpoint: 'GET /warehouse-fulfillment-charge-requests',
        emptyTitle: '还没有仓储计费请求',
        emptyDescription: '登记「货主计费采用」后，新产生的收费发生项会自动投递销售计费请求。',
        narrowed,
      });
  const shownPagination = pagination ?? held?.pagination;
  return (
    <WorkbenchPageTemplate<WarehouseChargeRequestRecord>
      title="销售计费请求"
      viewStateKey={`${moduleId}.requests`}
      subtitle="点一行在右边看。读口按发生项号排，缺省从新到旧"
      onRefresh={retry}
      refreshing={answer === null}
      search={{ value: keyword, onChange: setKeyword, placeholder: '搜请求号、发生项号或账户' }}
      extraFilters={narrowed ? <Clear onClear={clear} /> : undefined}
      columns={columns}
      rows={rows}
      rowKey={(row) => `${row.occurrenceId}/v${row.version}`}
      selectedKey={openKey}
      onSelect={setOpenKey}
      rowAlign="baseline"
      sort={sort}
      onSort={(key) => {
        const next = toggleWorkbenchSort(sort, key);
        setSort(next.dir === 1 ? field : `-${field}`);
      }}
      noMatchText="当前条件下没有销售计费请求"
      onClearFilters={clear}
      renderDetail={(key) => {
        const row = rows.find((item) => `${item.occurrenceId}/v${item.version}` === key);
        if (!row) return <p className="p-4 type-body text-idpxyz-textMuted">这一页没有这条请求。</p>;
        return (
          <SideDetail
            kind="销售计费请求"
            code={row.requestId ?? '未发请求'}
            badge={<WmsBadge code={row.status} label={labelOf(row.status)} />}
            subtitle={`${row.occurrenceId} v${row.version}`}
            onClose={() => setOpenKey(null)}
          >
            <p className="font-mono">结算账户 {row.account ?? '—'} · {row.settlementCurrency ?? '—'}</p>
            <p className="font-mono">定价口径 {row.pricingScope ?? '—'}</p>
            <p className="font-mono type-caption text-idpxyz-textMuted">定价基准 {row.pricingBasisAt ? formatInstant(row.pricingBasisAt) : '—'}</p>
            <p className="font-mono">计费结果 {row.chargeId ?? '—'} · 评估 {row.evaluationId ?? '—'}</p>
          </SideDetail>
        );
      }}
      viewState={viewState}
      listFooter={shownPagination && viewState.kind === 'ready' ? <CursorPagerBar pagination={shownPagination} /> : undefined}
    />
  );
}

function Clear({ onClear }: { onClear: () => void }) {
  return (
    <button
      type="button"
      className="whitespace-nowrap type-body text-idpxyz-textMuted hover:text-idpxyz-textBright focus-visible:underline focus-visible:outline-none"
      onClick={onClear}
    >
      清除
    </button>
  );
}

/** 仓储计费四签。都没有自己的单据页：点一行在右边看。 */
export function WarehousePricingPage() {
  const lookups = useWarehouseLookups();
  const targetMap = useWarehouseTargetMap(pricingActionSpecs);
  const [tab, setTab] = useTabViewState('warehouse-pricing.tab', 'plans');
  const [drawer, setDrawer] = useState<ActiveDrawerAction | null>(null);
  const [refreshKey, setRefreshKey] = useState(0);
  const can = (id: string) => targetMap.has(id);
  const openAction = (id: string, prefill?: GuidedFormValues) => {
    const target = targetMap.get(id);
    if (!target) return;
    setDrawer((current) => ({ target, prefill, nonce: (current?.nonce ?? 0) + 1 }));
  };

  return (
    <div className="flex-1 flex flex-col overflow-hidden bg-idpxyz-editor">
      <Tabs value={tab} onValueChange={setTab} className="flex-1 flex flex-col overflow-hidden gap-0">
        <TabsList className="px-4 shrink-0">
          <TabsTrigger value="plans">计费方案</TabsTrigger>
          <TabsTrigger value="adoptions">货主采用</TabsTrigger>
          <TabsTrigger value="occurrences">收费发生项</TabsTrigger>
          <TabsTrigger value="requests">销售计费请求</TabsTrigger>
        </TabsList>
        <TabsContent value="plans" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <PlansTab refreshKey={refreshKey} can={can} onOpen={openAction} />
        </TabsContent>
        <TabsContent value="adoptions" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <AdoptionsTab refreshKey={refreshKey} lookups={lookups} can={can} onOpen={openAction} />
        </TabsContent>
        <TabsContent value="occurrences" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <OccurrencesTab refreshKey={refreshKey} lookups={lookups} can={can} onOpen={openAction} />
        </TabsContent>
        <TabsContent value="requests" className="flex-1 flex flex-col overflow-hidden data-[state=inactive]:hidden">
          <RequestsTab refreshKey={refreshKey} />
        </TabsContent>
      </Tabs>
      <ActionDrawer
        moduleId={moduleId}
        active={drawer}
        lookups={lookups}
        onClose={() => setDrawer(null)}
        onSubmitted={() => setRefreshKey((count) => count + 1)}
      />
    </div>
  );
}
