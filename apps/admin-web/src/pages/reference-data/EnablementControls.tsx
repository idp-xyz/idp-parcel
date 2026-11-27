import { useState, type ReactNode } from 'react';
import { Button, ConfirmDialog, Input } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import type { TechnicalRow } from '../../components/TechnicalDetails';
import type { WorkbenchColumn } from '../../templates';
import { formatInstant } from '../moment';
import { operatorLabel, type OperatorNames } from '../operator-access/operator-names';
import { useMayWriteRegistry } from '../use-operator-grants';
import { useCatalogue } from './use-catalogue';
import {
  listEnablements,
  registerEnablement,
  type EnablementKind,
  type EnablementListResponseBody,
  type EnablementRecord,
  type EnablementRevisionRecord,
} from './api';
import {
  actionFor,
  actionLabel,
  confirmMessage,
  currentEnablement,
  enablementByCode,
  enablementOutcomeNote,
  enablementStandingCounts,
  type EnablementStandingChoice,
  nextRegistration,
  sourceProblem,
  standingCell,
  type EnablementAction,
} from './enablement';

// 国家和地区、币种两个签共用的“启用状态”列与“启用 / 停用”动作（ADR-0345）。登记册由本公司自己的登记人员维护，
// 没有登记写授予的人看得到状态、看不到动作（只收起入口，真正的授予判断在服务端）。

/** 一行正在登记的动作；null 就是没有对话框。 */
interface PendingAction {
  code: string;
  name: string;
  action: EnablementAction;
}

export function useEnablement(kind: EnablementKind) {
  const { answer, retry, refreshing } = useCatalogue<EnablementListResponseBody>(() => listEnablements(kind));
  const byCode = enablementByCode(answer?.kind === 'outcome' ? answer.body.enablements : []);
  const asOf = answer?.kind === 'outcome' ? answer.body.asOf : undefined;
  return { answer, byCode, asOf, reload: retry, refreshing };
}

type Enablement = ReturnType<typeof useEnablement>;

/**
 * 启用状态、登记人（人人可见）与动作（有登记写授予者可见）。nameOf 取这一行的中文名，进确认语。
 * 登记人是此刻生效那一笔的人，经姓名册换成姓名；没登记的行空着。动作键不触发整行选中。
 */
export function enablementColumns<Row extends { code: string }>(
  enablement: Enablement,
  mayWrite: boolean,
  nameOf: (row: Row) => string,
  ask: (pending: PendingAction) => void,
  names: OperatorNames,
): WorkbenchColumn<Row>[] {
  const status: WorkbenchColumn<Row> = {
    key: 'enablement',
    header: '启用状态',
    width: '104px',
    render: (row) => {
      const cell = standingCell(enablement.answer, enablement.byCode, row.code);
      if (cell.kind === 'loading') return <span className="text-idpxyz-breadcrumb">…</span>;
      if (cell.kind === 'unreadable') return <StatusPill label="读不到" tone="muted" />;
      // 未配置占绝大多数行，且不是警告（未登记的码照常可用），所以灰色；词仍取词典。
      return <StatusPill code={cell.standing} tone={cell.standing === 'NOT_CONFIGURED' ? 'muted' : undefined} />;
    },
  };
  const registrant: WorkbenchColumn<Row> = {
    key: 'enablement-registrant',
    header: '登记人',
    width: '7rem',
    render: (row) => {
      const cell = standingCell(enablement.answer, enablement.byCode, row.code);
      if (cell.kind === 'loading') return <span className="text-idpxyz-breadcrumb">…</span>;
      if (cell.kind !== 'standing' || cell.standing === 'NOT_CONFIGURED') return null;
      const current = currentEnablement(enablement.byCode.get(row.code), enablement.asOf ?? '');
      const who = current === null ? null : operatorLabel(current.registeredBy, names);
      if (who === null || current === null) return null;
      return (
        <span className="block truncate text-idpxyz-text" title={registrantHint(current)}>
          {who}
        </span>
      );
    },
  };
  if (!mayWrite) return [status, registrant];
  const actions: WorkbenchColumn<Row> = {
    key: 'enablement-action',
    header: '',
    // 这一列是表的最后一列，格子自带右侧留白，宽度又按边框盒把留白算进去。给窄了，两个字的按钮会被挤成竖排。
    width: '7.5rem',
    right: true,
    render: (row) => {
      const cell = standingCell(enablement.answer, enablement.byCode, row.code);
      // 没读到册面时不摆动作：不知道现状就不知道该启用还是该停用。
      if (cell.kind !== 'standing') return null;
      const action = actionFor(cell.standing);
      return (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 shrink-0 whitespace-nowrap px-2 type-body text-idpxyz-link"
          onClick={(event) => {
            event.stopPropagation();
            ask({ code: row.code, name: nameOf(row), action });
          }}
          aria-label={`${actionLabel(action)}${nameOf(row)}`}
        >
          {actionLabel(action)}
        </Button>
      );
    },
  };
  return [status, registrant, actions];
}

/** 悬停：标识、来源单据、登记时间。姓名在格子里，这三样留给要核对的人。 */
function registrantHint(row: EnablementRevisionRecord): string {
  const bits = [row.registeredBy];
  if (row.source) bits.push(`来源单据 ${row.source}`);
  bits.push(formatInstant(row.recordedAt, { precision: 'minute' }));
  return bits.join(' · ');
}

/**
 * 详情里此刻这一笔的登记事实。没有生效的行就什么都不加：未配置不是“没有人”，是还没这一笔。
 * 姓名给人看；发行方#sub 进技术详情，两个人可以同名，对账用标识。
 */
export function enablementAudit(
  record: EnablementRecord | undefined,
  asOf: string | undefined,
  names: OperatorNames,
): { facts: { label: string; value: string }[]; technical: TechnicalRow[] } {
  const current = currentEnablement(record, asOf ?? '');
  if (current === null) return { facts: [], technical: [] };
  const who = operatorLabel(current.registeredBy, names) ?? current.registeredBy;
  return {
    facts: [
      { label: '登记人', value: who },
      ...(current.source ? [{ label: '来源单据', value: current.source }] : []),
      { label: '登记时间', value: formatInstant(current.recordedAt, { precision: 'minute' }) },
    ],
    technical: [{ label: '登记责任方', value: current.registeredBy }],
  };
}

/**
 * 启用状态这一维的胶囊，跟大洲、小数位分开。计数是调用方已经按检索和分组筛过的那些码，
 * 再点一种状态时另外两种的数还在。再点已按下的那一格就回到全部。
 */
export function EnablementStandingChips({
  codes,
  byCode,
  active,
  onChange,
}: {
  codes: readonly string[];
  byCode: ReadonlyMap<string, EnablementRecord>;
  active: EnablementStandingChoice;
  onChange: (next: EnablementStandingChoice) => void;
}): ReactNode {
  return (
    <div className="flex flex-wrap items-center gap-1 border-l border-idpxyz-border pl-3" role="group" aria-label="启用状态">
      {enablementStandingCounts(codes, byCode).map((entry) => {
        const pressed = active === entry.standing;
        return (
          <button
            key={entry.standing}
            type="button"
            aria-pressed={pressed}
            onClick={() => onChange(pressed ? 'ALL' : entry.standing)}
            className={`rounded px-2 py-1 type-body transition-colors ${
              pressed ? 'bg-idpxyz-accent text-white' : 'text-idpxyz-breadcrumb hover:bg-idpxyz-hover hover:text-idpxyz-text'
            }`}
          >
            {entry.label}
            <span className="opacity-70"> {entry.count}</span>
          </button>
        );
      })}
    </div>
  );
}

/** 摆不摆动作列：答出之前与答不出时摆，只有明确知道没有登记写授予才收起（ADR-0315）。 */
export function useMayConfigureEnablement(): boolean {
  return useMayWriteRegistry();
}

/**
 * 启用 / 停用的确认窗。一格“来源单据”（可不填），确认后登记；登记上了就关窗并重取册面，没登记上把原因摆在窗里不关。
 */
export function EnablementDialog({
  kind,
  pending,
  enablement,
  onClose,
}: {
  kind: EnablementKind;
  pending: PendingAction | null;
  enablement: Enablement;
  onClose: () => void;
}): ReactNode {
  const [source, setSource] = useState('');
  const [note, setNote] = useState<string | null>(null);
  if (pending === null) return null;
  const problem = sourceProblem(source);
  const verb = actionLabel(pending.action);

  const close = () => {
    setSource('');
    setNote(null);
    onClose();
  };

  return (
    <ConfirmDialog
      open
      renderFooter={confirmDialogFooter}
      title={`${verb}${pending.name}`}
      message={confirmMessage(pending.action, pending.name)}
      extraContent={
        <div className="space-y-2 type-value">
          <label className="block">
            <span className="text-idpxyz-breadcrumb">来源单据（可不填）</span>
            <Input
              className="mt-1"
              value={source}
              onChange={(event) => {
                setSource(event.target.value);
                setNote(null);
              }}
              placeholder="例如会议纪要或审批单编号"
              aria-invalid={problem !== null}
            />
          </label>
          {problem ? <p className="text-idpxyz-breadcrumb">{problem}</p> : null}
          {note ? (
            <p role="alert" className="text-idpxyz-text">
              {note}
            </p>
          ) : null}
        </div>
      }
      confirmLabel={verb}
      cancelLabel="先不改"
      onConfirm={async () => {
        if (problem !== null) return;
        const payload = nextRegistration(kind, pending.code, enablement.byCode.get(pending.code), pending.action, source, new Date());
        const answer = await registerEnablement(payload);
        const outcome = enablementOutcomeNote(answer, pending.action);
        if (outcome.landed) {
          close();
          enablement.reload();
          return;
        }
        setNote(outcome.text);
      }}
      onCancel={close}
    />
  );
}
