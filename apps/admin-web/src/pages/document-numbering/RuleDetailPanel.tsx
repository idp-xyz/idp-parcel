import { useEffect, useState, type ReactNode } from 'react';
import { AlertTriangle, PenLine, X } from 'lucide-react';
import { Button } from '@idpxyz/ui-primitives';
import { primaryActionClass } from '../../components/primary-action';
import { StatusPill } from '../../components/StatusPill';
import { statusToneBanner, statusToneText } from '../../components/status-tone';
import { TechnicalDetails } from '../../components/TechnicalDetails';
import { formatInstant } from '../moment';
import { NewFormatForm } from './NewFormatForm';
import { registrantWords, ruleHistoryOf, type RuleRow } from './rows';
import { TemplateParts } from './TemplateParts';

// 右栏：一类单据此刻怎么出号、历次规则版本与生效时刻；类型代码与模板原文进技术详情（票 document-numbering/03）。
// 「改用新的编号格式」按登记册配置写授予收起（ADR-0315）：只在明确知道没有授予时不摆。

const SOURCE_SCOPE = 'document-numbering-source';

function Fact({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="min-w-0 rounded-md border border-idpxyz-border bg-idpxyz-sidebar px-3 py-2">
      <dt className="type-body text-idpxyz-breadcrumb">{label}</dt>
      <dd className="mt-0.5 break-words type-body font-semibold leading-snug text-idpxyz-textBright">{children}</dd>
    </div>
  );
}

export function RuleDetailPanel({
  row,
  at,
  mayWrite,
  onClose,
  onRegistered,
}: {
  row: RuleRow;
  /** 读口答这一问的时刻：历次版本的「生效中 / 未生效」按它判，与服务端同一刻。 */
  at: string;
  mayWrite: boolean;
  onClose: () => void;
  onRegistered: () => void;
}) {
  const [formOpen, setFormOpen] = useState(false);
  // 换了一类单据就收起表单：表单里填的是上一类的格式。
  useEffect(() => setFormOpen(false), [row.code]);
  const history = ruleHistoryOf(row.record, at);

  return (
    <div className="flex h-full w-full flex-col bg-idpxyz-bg" aria-label={`${row.type.name}的编号规则`}>
      <div className="flex items-start justify-between gap-3 border-b border-idpxyz-border px-5 pb-3 pt-4">
        <div className="min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h2 className="truncate type-title font-semibold text-idpxyz-textBright">{row.type.name}</h2>
            <StatusPill code={row.source} scope={SOURCE_SCOPE} />
          </div>
          <p className="mt-0.5 type-body text-idpxyz-breadcrumb">{row.type.numberName}怎么出</p>
        </div>
        <div className="flex shrink-0 items-center gap-1.5">
          {mayWrite && !formOpen ? (
            <Button size="sm" variant="default" className={`${primaryActionClass} type-body`} onClick={() => setFormOpen(true)}>
              <PenLine className="mr-1.5 h-3.5 w-3.5" aria-hidden="true" />
              改用新的编号格式
            </Button>
          ) : null}
          <Button className="text-idpxyz-breadcrumb" variant="ghost" size="icon" onClick={onClose} aria-label="关闭详情" title="关闭（Esc）">
            <X className="h-4 w-4" aria-hidden="true" />
          </Button>
        </div>
      </div>

      <div className="min-h-0 flex-1 space-y-4 overflow-y-auto px-5 py-4">
        {!row.type.known ? (
          <p className="type-body text-idpxyz-breadcrumb">界面还没有这一类单据的叫法，先显示它的类型代码。</p>
        ) : null}
        {row.dateZone.withoutNumber ? (
          <div className={`flex items-start gap-2 rounded border px-3 py-2 type-body leading-5 ${statusToneBanner.warning}`}>
            <AlertTriangle className={`mt-0.5 h-3.5 w-3.5 shrink-0 ${statusToneText.warning}`} aria-hidden="true" />
            <span>
              单据编号时区未配置——此刻开立的{row.type.name}照常开立，但不带{row.type.numberName}，事后也不补。登记单据编号时区在页头。
            </span>
          </div>
        ) : null}

        <dl className="grid grid-cols-2 gap-2.5">
          <Fact label="样例号">
            {row.sample ? (
              <span className="font-mono type-section">{row.sample}</span>
            ) : (
              <span className="font-normal text-idpxyz-breadcrumb">登记单据编号时区后才有样例</span>
            )}
          </Fact>
          <Fact label="格式">
            <TemplateParts template={row.template} />
          </Fact>
          <Fact label="流水重排">{row.reset.sentence || row.reset.short}</Fact>
          <Fact label="生效">
            {row.source === 'TENANT_RULE' && row.effectiveFrom ? (
              `${formatInstant(row.effectiveFrom)} 起`
            ) : (
              <span className="font-normal">产品默认：本公司没有登自己的格式时就用它</span>
            )}
          </Fact>
        </dl>
        <div className="type-body leading-5 text-idpxyz-text">
          <p className="text-idpxyz-breadcrumb">日期所用时区</p>
          <p className="mt-0.5">{row.dateZone.sentence}</p>
          {row.sample ? <p className="mt-1 text-idpxyz-breadcrumb">样例按单据编号时区与第一个流水出，只示号的形状，不占号。</p> : null}
        </div>

        {formOpen ? <NewFormatForm row={row} readAt={at} onRegistered={onRegistered} onClose={() => setFormOpen(false)} /> : null}

        <section>
          <h3 className="type-value font-semibold text-idpxyz-textBright">历次规则版本</h3>
          <p className="mt-0.5 type-body text-idpxyz-breadcrumb">
            版本只增不改；生效时刻到了的最新一版管此后开立的单据，本公司没有登过或登的都还没生效时用产品默认。
          </p>
          <ol className="mt-2 space-y-2">
            {history.map((entry) => (
              <li key={entry.key} className="rounded-md border border-idpxyz-border px-3 py-2">
                <div className="flex flex-wrap items-center gap-2">
                  <StatusPill code={entry.standing} />
                  <span className="type-body text-idpxyz-textBright">
                    {entry.productDefault ? '产品默认' : `${formatInstant(entry.effectiveFrom ?? '')} 起`}
                  </span>
                </div>
                <div className="mt-1">
                  <TemplateParts template={entry.template} />
                </div>
                {entry.registeredBy ? (
                  <p className="mt-1 type-body text-idpxyz-breadcrumb">
                    <span title={entry.registeredBy}>{registrantWords(entry.registeredBy)}</span> 登记于 {formatInstant(entry.registeredAt ?? '')}
                  </p>
                ) : null}
              </li>
            ))}
          </ol>
        </section>

        {!mayWrite ? (
          <p className="type-body text-idpxyz-breadcrumb">改格式要登记册配置写授予，需要时请联系管理员。</p>
        ) : null}

        <TechnicalDetails
          rows={[
            { label: '类型代码', value: row.code },
            { label: '此刻的模板', value: row.template },
            { label: '产品默认模板', value: row.defaultTemplate },
            { label: '日期时区来路', value: String(row.record.dateTimeZone.source) },
            { label: '重置周期码', value: row.record.resetPeriod },
            ...row.record.versions.map((version) => ({ label: `${formatInstant(version.effectiveFrom)} 起`, value: version.template })),
          ]}
        />
      </div>
    </div>
  );
}
