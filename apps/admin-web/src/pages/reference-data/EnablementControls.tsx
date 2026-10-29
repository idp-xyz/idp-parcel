import { useState, type ReactNode } from 'react';
import { Button, ConfirmDialog, Input } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { confirmDialogFooter } from '../../components/confirm-dialog-footer';
import type { WorkbenchColumn } from '../../templates';
import { useMayWriteRegistry } from '../use-operator-grants';
import { useCatalogue } from './use-catalogue';
import {
  listEnablements,
  registerEnablement,
  type EnablementKind,
  type EnablementListResponseBody,
  type EnablementRecord,
} from './api';
import {
  actionFor,
  actionLabel,
  confirmMessage,
  enablementByCode,
  enablementOutcomeNote,
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
  return { answer, byCode, reload: retry, refreshing };
}

type Enablement = ReturnType<typeof useEnablement>;

/**
 * 两列：启用状态（人人可见）与动作（有登记写授予者可见）。nameOf 取这一行的中文名，进确认语。
 * 动作键不触发整行选中（点它不该同时打开详情栏）。
 */
export function enablementColumns<Row extends { code: string }>(
  enablement: Enablement,
  mayWrite: boolean,
  nameOf: (row: Row) => string,
  ask: (pending: PendingAction) => void,
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
  if (!mayWrite) return [status];
  const actions: WorkbenchColumn<Row> = {
    key: 'enablement-action',
    header: '',
    width: '72px',
    render: (row) => {
      const cell = standingCell(enablement.answer, enablement.byCode, row.code);
      // 没读到册面时不摆动作：不知道现状就不知道该启用还是该停用。
      if (cell.kind !== 'standing') return null;
      const action = actionFor(cell.standing);
      return (
        <Button
          variant="ghost"
          size="sm"
          className="h-6 px-2 text-[12px] text-idpxyz-link"
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
  return [status, actions];
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
        <div className="space-y-2 text-[13px]">
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
