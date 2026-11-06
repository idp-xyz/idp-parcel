import { useMemo } from 'react';
import { listRegistrationNumberTypes, type BusinessPartyIdentificationRecord } from './api';
import { partyIdentificationCellsOf } from './business-party-identification';
import { registrationNumberTypeNamesOf } from './legal-entity-identity';
import { countryLabel } from '../refdata-names';
import { DetailRow } from './detail-primitives';
import { useLoaded } from './PublicationFormFields';

/**
 * 往来单位识别信息的三行读出（ADR-0367），对象页概要与右栏共用。没登的格显「未登记」：识别信息本来可缺，空着不是错。
 * 注册号类型目录读一次，把类型码译成目录译名；读不到就按码显。
 */
export function BusinessPartyIdentificationRows({ record }: { record: BusinessPartyIdentificationRecord }) {
  const typeCatalogue = useLoaded(listRegistrationNumberTypes);
  const typeNameOf = useMemo(
    () => registrationNumberTypeNamesOf(typeCatalogue?.kind === 'outcome' ? typeCatalogue.body.registrationNumberTypes : []),
    [typeCatalogue],
  );
  const cells = partyIdentificationCellsOf(record, typeNameOf);
  const unregistered = <span className="text-idpxyz-breadcrumb">未登记</span>;
  return (
    <>
      <DetailRow label="注册地">{cells.country === null ? unregistered : countryLabel(cells.country)}</DetailRow>
      <DetailRow label="注册号">
        {cells.numbers.length === 0 ? (
          unregistered
        ) : (
          <span className="flex flex-col gap-0.5">
            {cells.numbers.map((cell) => (
              <span key={`${cell.typeName}-${cell.number}`}>
                <span className="text-idpxyz-breadcrumb">{cell.typeName}</span> <span className="font-mono">{cell.number}</span>
              </span>
            ))}
          </span>
        )}
      </DetailRow>
      <DetailRow label="外部系统编码">
        {cells.references.length === 0 ? (
          unregistered
        ) : (
          <span className="flex flex-col gap-0.5">
            {cells.references.map((cell) => (
              <span key={`${cell.system}-${cell.purpose}`}>
                <span className="font-mono">{cell.system}</span> <span className="text-idpxyz-breadcrumb">{cell.purpose}</span>{' '}
                <span className="font-mono">{cell.code}</span>
              </span>
            ))}
          </span>
        )}
      </DetailRow>
    </>
  );
}
