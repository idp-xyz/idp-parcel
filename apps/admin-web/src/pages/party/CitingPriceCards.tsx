import { refHash } from './ref-links';
import { useAnswer } from '../../query/answers';
import { partyAnswersDomain } from './party-writes';
import { loadCitingPriceCards } from './citing-price-cards';

/** 价格政策与授权规则对象页上「引用它的价卡」。价卡目录按授权引用筛选；价格政策先找指名它的授权规则。 */
export function CitingPriceCards({
  register,
  objectId,
  version,
}: {
  register: 'PRICE_POLICY' | 'AUTHORIZATION_RULE';
  objectId: string;
  version: string;
}) {
  const answer = useAnswer(
    [partyAnswersDomain, 'citing-price-cards', register, objectId, version],
    () => loadCitingPriceCards(register, objectId, version),
    { fresh: true },
  );
  if (answer.answer === null) {
    return <p className="type-body text-idpxyz-textMuted">正在查找引用它的价卡…</p>;
  }
  if (answer.answer.kind !== 'outcome') {
    return (
      <p className="type-body text-[color:var(--ds-status-danger-fg)]">
        这次没读回引用它的价卡。
        <button type="button" className="ml-2 underline" onClick={answer.retry}>
          重试
        </button>
      </p>
    );
  }
  const { cards, truncated } = answer.answer.body;
  if (cards.length === 0) {
    return <p className="type-body text-idpxyz-textMuted">还没有价卡引用它。</p>;
  }
  return (
    <div>
      <ul className="space-y-1">
        {cards.map((card) => (
          <li key={`${card.planId}@${card.planVersion}`} className="type-body">
            <a className="font-mono text-idpxyz-accent hover:underline" href={refHash('price-card', card.planId)}>
              {card.planId}@{card.planVersion}
            </a>
            <span className="ml-2 text-idpxyz-textMuted">{card.direction}</span>
          </li>
        ))}
      </ul>
      {truncated ? <p className="mt-2 type-body text-idpxyz-textMuted">还有更多，到价卡目录里按方案检索。</p> : null}
    </div>
  );
}
