import { useEffect, useState } from 'react';
import { AlertTriangle, Clock } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { moduleInfoById } from '../../navigation';
import { primaryActionClass } from '../../components/primary-action';
import { StatusPill } from '../../components/StatusPill';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import { WorkbenchPageTemplate, type WorkbenchChip, type WorkbenchColumn, type WorkbenchKpi } from '../../templates';
import { useTabViewState } from '../../templates/workspace-view-state';
import { catalogueViewState } from '../catalogue-view';
import { formatInstant } from '../moment';
import { useMayWriteRegistry } from '../use-operator-grants';
import { readDocumentNumbering, type ApiResult, type DocumentNumberingReadBody } from './api';
import {
  countRuleRows,
  filterRuleRows,
  numberingZoneMissingSentence,
  numberingZoneStateOf,
  ruleCountSummary,
  ruleRowsOf,
  zoneWords,
  type RuleRow,
  type SourceFilter,
} from './rows';
import { RuleDetailPanel } from './RuleDetailPanel';
import { TemplateParts } from './TemplateParts';
import { TimeZoneDialog } from './TimeZoneDialog';

// 主责与出处的唯一来源是 navigation 的 moduleInfoById，只读引用，不抄第二份。
const info = moduleInfoById['document-numbering'];

const SOURCE_SCOPE = 'document-numbering-source';

// 设置 → 单据编号（ADR-0355 决定五、六；票 document-numbering/03）：每类单据一行，此刻用的格式与样例号、来源、
// 重置方式与日期所用时区；右栏是这一类的历次规则版本与「改用新的编号格式」。页头是本公司的单据编号时区——没登时醒目地
// 说哪些单据此刻开立不带号。规则册是编号技术组件的配置，不属任何限界上下文；单据号本身归各单据所属的上下文。
//
// 读口一问答全册（各类的此刻规则、历次版本与单据编号时区），页面不分页、不另开读口。

function useRulebook() {
  const [answer, setAnswer] = useState<ApiResult<DocumentNumberingReadBody> | null>(null);
  const [reloadToken, setReloadToken] = useState(0);
  const [refreshing, setRefreshing] = useState(false);
  useEffect(() => {
    let cancelled = false;
    void readDocumentNumbering().then((result) => {
      if (cancelled) return;
      setAnswer(result);
      setRefreshing(false);
    });
    return () => {
      cancelled = true;
    };
  }, [reloadToken]);
  const reload = () => {
    setRefreshing(true);
    setReloadToken((token) => token + 1);
  };
  return { answer, reload, refreshing };
}

const columns: WorkbenchColumn<RuleRow>[] = [
  {
    key: 'name',
    header: '单据',
    width: '128px',
    render: (row) => (
      <span className="font-medium text-idpxyz-textBright" title={row.type.known ? undefined : '界面还没有这一类单据的叫法'}>
        {row.type.name}
      </span>
    ),
  },
  {
    key: 'sample',
    header: '样例号',
    render: (row) =>
      row.sample ? (
        <span className="font-mono text-[13px] text-idpxyz-textBright">{row.sample}</span>
      ) : (
        <span className="text-idpxyz-breadcrumb" title="登记单据编号时区后才有样例">
          —
        </span>
      ),
  },
  { key: 'format', header: '格式', minListWidth: 640, render: (row) => <TemplateParts template={row.template} /> },
  {
    key: 'source',
    header: '来源',
    width: '160px',
    render: (row) => (
      <span className="inline-flex flex-wrap items-center gap-1.5">
        <StatusPill code={row.source} scope={SOURCE_SCOPE} />
        {row.source === 'TENANT_RULE' && row.effectiveFrom ? (
          <span className="text-[11px] text-idpxyz-breadcrumb">{formatInstant(row.effectiveFrom).slice(0, 10)} 起</span>
        ) : null}
      </span>
    ),
  },
  { key: 'reset', header: '流水重排', width: '84px', hideWhenDetailOpen: true, render: (row) => <span className="text-idpxyz-text">{row.reset.short}</span> },
  {
    key: 'zone',
    header: '日期所用时区',
    hideWhenDetailOpen: true,
    render: (row) =>
      row.dateZone.withoutNumber ? (
        <span className="inline-flex items-center gap-1 text-idpxyz-text">
          <AlertTriangle className={`h-3.5 w-3.5 ${statusToneText.warning}`} aria-hidden="true" />
          {row.dateZone.short}
        </span>
      ) : (
        <span className="text-idpxyz-text" title={row.dateZone.sentence}>
          {row.dateZone.short}
        </span>
      ),
  },
];

/** 单据编号时区没登时的横幅：说哪些单据此刻不带号，登记入口按授予收起。 */
function MissingZoneBanner({ body, mayWrite, onOpen }: { body: DocumentNumberingReadBody; mayWrite: boolean; onOpen: () => void }) {
  const state = numberingZoneStateOf(body);
  return (
    <div role="alert" className={`flex shrink-0 flex-wrap items-center gap-x-3 gap-y-1.5 border-b px-6 py-2.5 text-[13px] ${statusToneBanner.warning}`}>
      <AlertTriangle className={`h-4 w-4 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
      <span className="min-w-0 flex-1">
        <span className="font-semibold text-idpxyz-textBright">{numberingZoneMissingSentence}</span>
        {state.upcoming ? (
          <span className="ml-1">
            已登记 {formatInstant(state.upcoming.effectiveFrom)} 起用{zoneWords(state.upcoming.zone)}，到时自动生效。
          </span>
        ) : (
          <span className="ml-1">此前开立的单据事后也不补号。</span>
        )}
      </span>
      {mayWrite ? (
        <Button size="sm" variant="default" className={`${primaryActionClass} h-7 text-[12px]`} onClick={onOpen}>
          登记单据编号时区
        </Button>
      ) : (
        <span className="text-[12px] text-idpxyz-breadcrumb">登记需要登记册配置写授予，请联系管理员。</span>
      )}
    </div>
  );
}

export function DocumentNumberingPage() {
  const { answer, reload, refreshing } = useRulebook();
  // 没有登记册配置写授予的操作者不摆两处登记入口（ADR-0315）；摆不摆只是呈现，登记口照旧在服务端判授予。
  const mayWrite = useMayWriteRegistry();
  const [search, setSearch] = useTabViewState('document-numbering.search', '');
  const [source, setSource] = useTabViewState<SourceFilter>('document-numbering.source', 'ALL');
  const [selectedKey, setSelectedKey] = useTabViewState<string | null>('document-numbering.selected', null);
  const [zoneDialogOpen, setZoneDialogOpen] = useState(false);

  const body = answer?.kind === 'outcome' ? answer.body : null;
  const rows = body ? ruleRowsOf(body) : [];
  const visible = filterRuleRows(rows, { search, source });
  const counts = countRuleRows(rows);
  const zone = body ? numberingZoneStateOf(body) : null;

  const chip = (id: SourceFilter, label: string, count: number): WorkbenchChip => ({
    id,
    label,
    count,
    active: source === id,
    onClick: () => setSource(source === id && id !== 'ALL' ? 'ALL' : id),
  });

  const kpis: WorkbenchKpi[] | undefined =
    body && zone
      ? [
          {
            label: '单据编号时区',
            value: zone.configured ? zone.current : '未配置',
            custom: zone.configured ? (
              <span className="text-[14px] font-semibold text-idpxyz-textBright" title={zone.currentZone}>
                {zone.current}
              </span>
            ) : (
              <span className="inline-flex items-center gap-1 text-[14px] font-semibold text-idpxyz-textBright">
                <AlertTriangle className={`h-3.5 w-3.5 ${statusToneText.warning}`} aria-hidden="true" />
                未配置
              </span>
            ),
          },
          ...(counts.withoutNumber > 0
            ? [
                {
                  label: '此刻开立不带号',
                  value: `${counts.withoutNumber} 类`,
                  icon: <AlertTriangle className={`h-3 w-3 ${statusToneText.warning}`} aria-hidden="true" />,
                },
              ]
            : []),
        ]
      : undefined;

  return (
    <div className="flex flex-1 flex-col overflow-hidden bg-idpxyz-editor">
      {body && zone && !zone.configured ? <MissingZoneBanner body={body} mayWrite={mayWrite} onOpen={() => setZoneDialogOpen(true)} /> : null}
      <WorkbenchPageTemplate<RuleRow>
        title={info.title}
        viewStateKey="document-numbering"
        subtitle="各类单据的号怎么出：此刻用的格式与样例号；改格式只管之后开立的单据，已开立的号不变。"
        kpis={kpis}
        onRefresh={reload}
        refreshing={refreshing}
        primaryAction={
          body ? (
            <Button variant="ghost" size="sm" className="gap-1.5 text-[12px] text-idpxyz-accent" onClick={() => setZoneDialogOpen(true)}>
              <Clock className="h-3.5 w-3.5" aria-hidden="true" />
              {mayWrite ? '登记单据编号时区' : '单据编号时区的历次登记'}
            </Button>
          ) : null
        }
        search={{ value: search, onChange: setSearch, placeholder: '搜单据名、样例号或格式…' }}
        statusChips={
          body
            ? [chip('ALL', '全部', counts.total), chip('PRODUCT_DEFAULT', '产品默认', counts.productDefault), chip('TENANT_RULE', '本公司规则', counts.tenantRule)]
            : undefined
        }
        toolbarRight={body ? ruleCountSummary(rows.length, visible.length) : undefined}
        columns={columns}
        rows={visible}
        rowKey={(row) => row.code}
        selectedKey={selectedKey}
        onSelect={setSelectedKey}
        rowAttention={(row) => row.dateZone.withoutNumber}
        noMatchText="没有匹配的单据。换个关键词，或清除筛选。"
        onClearFilters={() => {
          setSearch('');
          setSource('ALL');
        }}
        renderDetail={(key) => {
          const row = visible.find((candidate) => candidate.code === key);
          return row && body ? (
            <RuleDetailPanel row={row} at={body.at} mayWrite={mayWrite} onClose={() => setSelectedKey(null)} onRegistered={reload} />
          ) : null;
        }}
        viewState={catalogueViewState(answer, rows.length, reload, {
          module: info,
          endpoint: 'GET /document-numbering/rules',
          emptyTitle: '规则册没有答出任何单据类型',
          // 各类单据的默认格式随产品发布，空表只可能是读口出了毛病，不是「还没登记」。
          emptyDescription: '各类单据的默认编号格式随产品发布，不该为空；这不是“还没登记”，请联系产品支持。',
        })}
      />
      {body ? (
        <TimeZoneDialog open={zoneDialogOpen} onOpenChange={setZoneDialogOpen} body={body} mayWrite={mayWrite} onRegistered={reload} />
      ) : null}
    </div>
  );
}
