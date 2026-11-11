import { TechnicalDetails } from '../../components/TechnicalDetails';
import type { SubmitResponseBody } from './api';
import { admissionControlLabels, authorityLabels, submitOutcomeViews, type OutcomeView } from './presentation';
import { DetailRow, NoticeCard } from './controls';
import { ShipmentNo } from './refs';
import { documentNumberAbsenceNoteOf } from './ref-no';
import { documentTypeWords } from '../../vocabulary/document-numbering';

// 提交答复的卡片:小包下单页与 FBA 货件页提交的是同一个端点,答复同一套词。

// 未收录的 outcome 如实报「不认识」,不译不并:词汇表是封闭的,页面落后于服务端时
// 比错并进某个认识的格要安全;原词在技术详情里。
const unknownOutcome: OutcomeView = {
  label: '结果未收录',
  note: '系统交回了本页还不认识的结果,请按技术详情里的结果码核对记录。',
};

export function SubmitOutcomeCard({ body, status }: { body: SubmitResponseBody; status: number }) {
  const view = submitOutcomeViews[body.outcome] ?? unknownOutcome;
  const ownership = body.productionOwnership;
  return (
    <NoticeCard title={view.label} titleClass={view.affirmative ? 'text-idpxyz-accent' : 'text-idpxyz-textBright'}>
      <p className="type-body leading-5 text-idpxyz-textMuted mt-2">{view.note}</p>

      <dl className="mt-3 space-y-1">
        {body.shipmentRequestId ? (
          <div className="flex gap-2 type-body leading-5">
            <dt className="text-idpxyz-textMuted shrink-0 w-[104px]">{documentTypeWords.SHIPMENT.numberName}</dt>
            <dd>
              <ShipmentNo shipmentRequestId={body.shipmentRequestId} documentNumber={body.documentNumber} />
            </dd>
          </div>
        ) : null}
        {ownership ? <DetailRow label="由谁承运" value={authorityLabels[ownership.authority] ?? ownership.authority} /> : null}
        {ownership && ownership.admissionControl !== 'OPEN' ? (
          <DetailRow label="接单状态" value={admissionControlLabels[ownership.admissionControl] ?? ownership.admissionControl} />
        ) : null}
      </dl>

      {body.gateBlockReasons && body.gateBlockReasons.length > 0 ? (
        <p className="type-caption leading-4 text-idpxyz-textMuted mt-2">之后再提交会被拦住,原因见技术详情。</p>
      ) : null}

      <TechnicalDetails
        rows={[
          { label: 'HTTP 状态', value: String(status) },
          { label: '结果码', value: body.outcome },
          // 指名了委托却没有单据号时注明是哪一种情形;没指名委托(没建单)就没有这一行。
          ...(body.shipmentRequestId
            ? [{ label: documentTypeWords.SHIPMENT.numberName, value: documentNumberAbsenceNoteOf(body) ?? body.documentNumber ?? '' }]
            : []),
          { label: '委托标识', value: body.shipmentRequestId ?? '' },
          { label: '生产归属判断', value: ownership?.decisionId ?? '' },
          { label: '权威身份', value: ownership?.authority ?? '' },
          { label: '准入控制', value: ownership?.admissionControl ?? '' },
          { label: '规则版本', value: ownership?.ruleVersion ?? '' },
          { label: '修订', value: ownership?.revision ?? '' },
          { label: '当前权威方', value: ownership?.otherAuthority ?? '' },
          { label: '交接引用', value: ownership?.handoffReference ?? '' },
          { label: '未决原因', value: ownership?.unresolvedReason ?? '' },
          { label: '续办引用', value: ownership?.continuationReference ?? '' },
          { label: '交接确认引用', value: ownership?.handoffConfirmationReference ?? '' },
          { label: '暂停引用', value: ownership?.suspensionReference ?? '' },
          { label: '闸门阻断原因', value: (body.gateBlockReasons ?? []).join(' · ') },
        ]}
      />
    </NoticeCard>
  );
}
