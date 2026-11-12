import { useState } from 'react';
import { Button, Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from '@idpxyz/ui-primitives';
import { StatusPill } from '../../components/StatusPill';
import { OperatorRefusalNote, SectionError, TechnicalDetails } from '../../components/states';
import { objectNouns } from '../../vocabulary/objects';
import { formatInstant } from '../moment';
import { RefLink } from '../party/RefLink';
import type { ShipmentRequestDetail } from './api';
import { FieldRow } from './field-row';
import { financialControlConclusionOf } from './financial-control';
import { foldedRowsOf } from './goods-view';
import { lastAttemptOf } from './last-attempt';
import { estimateBlockStateOf, type EstimateBlockState, type EstimateSectionView, type ParcelEstimateView } from './pre-acceptance-estimate';
import { ParcelNo } from './refs';
import { ResumeNoteText } from './resume-note';
import { usePreAcceptanceEstimates } from './use-pre-acceptance-estimates';

// 运单详情的「受理前财务控制」一块（ADR-0379 决定四，票 declared-goods-lines/21）。结论来自委托详情（小包托运，不带金额），
// 估价与占用来自结算的读口——两侧各交各的、在这里拼，小包托运不转交金额。只对运营，不对客户。判读都在 financial-control.ts
// 与 pre-acceptance-estimate.ts，这里只摆。

export function PreAcceptanceFinancialControl({ view }: { view: ShipmentRequestDetail }) {
  const { answer, retry } = usePreAcceptanceEstimates(view.shipmentRequestId);
  const estimate = estimateBlockStateOf(
    answer,
    { currentVersion: view.submissionVersionId, controlOutcome: view.financialControl?.outcome },
    lastAttemptOf(view.acceptanceTask),
  );
  return (
    <div className="flex flex-col gap-4 type-body">
      {/* 预估与实际分开记，这一句放在最前，免得把估价读成账。 */}
      <p className="text-idpxyz-textMuted">
        受理前估价，不是实际费用；实际费用见
        <a href="#/charges-billing" className="text-idpxyz-accent hover:underline focus-visible:underline">
          费用页
        </a>
        。合计、小计与占用金额按最小币单位写（例如 12.34 元写 1234），与费用页同一写法；费用行是试算当时算出的金额，照原样显示。
      </p>
      <ConclusionPart view={view} />
      <div>
        <span className="text-idpxyz-textMuted">估价与占用</span>
        <div className="mt-1">
          <EstimatePart state={estimate} retry={retry} />
        </div>
      </div>
    </div>
  );
}

function ConclusionPart({ view }: { view: ShipmentRequestDetail }) {
  const control = view.financialControl;
  const conclusion = financialControlConclusionOf(control);
  return (
    <div>
      <span className="text-idpxyz-textMuted">结论</span>
      {conclusion === null ? (
        // 没判过是整格缺席；不写「无」，那会和「判过了、没有控制」分不开。
        <p className="mt-1">还没判。</p>
      ) : (
        <div className="mt-1 flex flex-col gap-1">
          <p className="flex flex-wrap items-center gap-2">
            <StatusPill label={conclusion.verdict} tone={conclusion.tone} />
            {conclusion.detail ? <span>{conclusion.detail}</span> : null}
          </p>
          {conclusion.items.length > 0 && (
            <ul className="flex flex-col gap-0.5">
              {conclusion.items.map((item) => (
                <li key={item.key}>{item.text}</li>
              ))}
            </ul>
          )}
          {control?.asOfAt ? (
            <p className="text-idpxyz-textMuted">
              判断时点 <span className="font-mono">{formatInstant(control.asOfAt)}</span>
            </p>
          ) : null}
        </div>
      )}
    </div>
  );
}

function EstimatePart({ state, retry }: { state: EstimateBlockState; retry: () => void }) {
  switch (state.kind) {
    case 'loading':
      return (
        <p role="status" className="text-idpxyz-textMuted">
          正在读受理前估价…
        </p>
      );
    case 'unconfigured':
      // 重试不会改变结果，不给重试。
      return <p className="text-idpxyz-textMuted">{state.text}</p>;
    case 'refused':
      return <OperatorRefusalNote kind={state.refusal} what={state.what} />;
    case 'problem':
      return (
        <div className="text-idpxyz-textMuted">
          <p>{state.text}</p>
          <TechnicalDetails rows={state.technical} />
        </div>
      );
    case 'unreachable':
      return <SectionError title={state.title} description={state.description} technical={state.technical} onRetry={retry} />;
    case 'none':
      return (
        <div className="flex flex-col gap-1">
          <p>{state.note}</p>
          {state.resume ? (
            <p>
              <ResumeNoteText note={state.resume} />
            </p>
          ) : null}
        </div>
      );
    case 'formed':
      return (
        <div className="flex flex-col gap-3">
          {state.gapNote ? <p>{state.gapNote}</p> : null}
          {state.resume ? (
            <p>
              <ResumeNoteText note={state.resume} />
            </p>
          ) : null}
          {state.sections.map((section) => (
            <EstimateSection key={`${section.submissionVersion}:${section.trialAt}`} section={section} />
          ))}
        </div>
      );
  }
}

function EstimateSection({ section }: { section: EstimateSectionView }) {
  return (
    <div className="flex flex-col gap-2 rounded border border-idpxyz-border p-3">
      <p className="flex flex-wrap items-baseline gap-2">
        <span className="font-medium text-idpxyz-text">{section.versionLabel}的估价</span>
        <span className="text-idpxyz-textMuted">
          试算于 <span className="font-mono">{formatInstant(section.trialAt)}</span>
        </span>
      </p>
      <dl className="space-y-1">
        <FieldRow label="估价合计" value={section.total} mono />
        <FieldRow label="价卡" value={<RefLink kind="price-card" id={section.priceCard.plan} text={section.priceCard.text} />} />
        <FieldRow
          label="服务产品"
          value={section.serviceProduct ? <RefLink kind="service-product" id={section.serviceProduct} /> : '没申报或未记'}
        />
        <FieldRow label="估价方式" value={section.form} />
        <FieldRow label="记下时间" value={formatInstant(section.recordedAt)} mono />
      </dl>
      {section.detailNote ? <p className="text-idpxyz-textMuted">{section.detailNote}</p> : null}
      <ParcelEstimatesTable parcels={section.parcels} detailRecorded={section.detailRecorded} />
      <dl className="space-y-1">
        {section.occupationNote ? <FieldRow label="占用" value={section.occupationNote} /> : null}
        {section.occupations.map((occupation) => (
          <FieldRow
            key={occupation.id}
            label="占用"
            value={
              <>
                <div>{occupation.summary}</div>
                <div className="type-caption text-idpxyz-textMuted">
                  {occupation.times.map((time) => `${time.label} ${formatInstant(time.at)}`).join('；')}
                </div>
              </>
            }
          />
        ))}
      </dl>
      <TechnicalDetails rows={section.technical} title="估价的技术详情" />
    </div>
  );
}

/** 逐件的计价重量与费用行。补记之前的旧行只有小计，计价重量与费用两列不出，不显一整列「未记」。件多时同包裹表一样折起。 */
function ParcelEstimatesTable({ parcels, detailRecorded }: { parcels: ParcelEstimateView[]; detailRecorded: boolean }) {
  const [expanded, setExpanded] = useState(false);
  const { shown, hidden } = foldedRowsOf(parcels, expanded);
  return (
    <div className="space-y-2">
      <Table>
        <TableHeader>
          <TableRow>
            <TableHead>{objectNouns.parcelNumber}</TableHead>
            {detailRecorded && <TableHead>计价重量</TableHead>}
            {detailRecorded && <TableHead>费用行</TableHead>}
            <TableHead>小计</TableHead>
          </TableRow>
        </TableHeader>
        <TableBody>
          {shown.map((parcel) => (
            <TableRow key={parcel.parcel}>
              <TableCell>
                <ParcelNo parcelId={parcel.parcel} />
              </TableCell>
              {detailRecorded && <TableCell>{parcel.weight}</TableCell>}
              {detailRecorded && (
                <TableCell>
                  {parcel.charges === null ? (
                    '未记'
                  ) : parcel.charges.length === 0 ? (
                    '—'
                  ) : (
                    <ul className="flex flex-col gap-0.5">
                      {parcel.charges.map((line, index) => (
                        <li key={`${line.code}-${index}`} className="flex flex-wrap items-baseline gap-2">
                          <span>{line.label}</span>
                          <span className="font-mono type-caption text-idpxyz-textMuted">{line.code}</span>
                          <span>{line.effect}</span>
                          <span className="font-mono">{line.amount}</span>
                        </li>
                      ))}
                    </ul>
                  )}
                </TableCell>
              )}
              <TableCell>
                <span className="font-mono">{parcel.subtotal}</span>
              </TableCell>
            </TableRow>
          ))}
        </TableBody>
      </Table>
      {hidden > 0 && (
        <Button variant="ghost" size="sm" onClick={() => setExpanded(true)}>
          展开其余 {hidden} 件
        </Button>
      )}
      {expanded && foldedRowsOf(parcels, false).hidden > 0 && (
        <Button variant="ghost" size="sm" onClick={() => setExpanded(false)}>
          收起
        </Button>
      )}
    </div>
  );
}
