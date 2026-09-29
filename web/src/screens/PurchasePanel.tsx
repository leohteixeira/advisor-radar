import { useEffect, useRef, useState, type FormEvent, type ReactNode } from 'react';
import { Link } from 'react-router-dom';
import { dollars, formatCents, protocolOf } from '../api/pov';
import { RiskBars } from '../sdui/components/RiskBars';
import { MASKED_MONEY } from '../sdui/context';
import type { PurchaseTarget } from '../sdui/purchase';
import type { ProductItem } from '../sdui/types';

/** One Bastidores line as the customer stream reports it. */
export interface LiveStep {
  id: string;
  label: string;
  state: string;
}

/** A purchase the BFF accepted (202), for the confirmation. */
export interface Bought {
  event_id: string;
  cents: number;
  product: ProductItem;
}

/** The suggested amounts of the form, in cents; "Tudo" is the cash. */
const CHIPS = [
  { key: '250', label: 'US$ 250', cents: 25_000 },
  { key: '1000', label: 'US$ 1.000', cents: 100_000 },
] as const;

const DEFAULT_CENTS = 100_000;

type Chip = '250' | '1000' | 'all' | null;

/**
 * The Bastidores steps of a purchase. Above the profile the suitability rule
 * fires and the alert reaches the queue; otherwise a rule may or may not fire.
 */
export function purchaseSteps(product: ProductItem): string[] {
  return [
    'Gravado na outbox do account-sim',
    'Publicado no RabbitMQ',
    product.above_profile ? 'Regra: compra acima do perfil' : 'Avaliado pelas regras',
    product.above_profile ? 'Na fila da assessoria' : 'Na fila da assessoria, se uma regra disparar',
  ];
}

/** Focuses the returned heading once, when the view that holds it appears. */
function useFocusedHeading() {
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => heading.current?.focus(), []);
  return heading;
}

function centsOf(text: string): number {
  return Math.round(dollars(text) * 100);
}

function BackChevron() {
  return (
    <svg width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d="M15 6l-6 6 6 6" />
    </svg>
  );
}

/**
 * The coded purchase form (Compra-Fernanda-Aviso). Every product value and the
 * cash come from the Investir envelope; the only thing web works out is the
 * typed amount, capped at the cash as a convenience. The BFF decides the rest.
 */
export function PurchaseForm({
  target,
  notice,
  busy,
  masked,
  onBack,
  onSubmit,
  head,
}: {
  target: PurchaseTarget;
  notice: string;
  busy: boolean;
  masked: boolean;
  onBack: () => void;
  onSubmit: (cents: number) => void;
  /** The side panel header with its close control; it replaces the back link in the desktop drawer. */
  head?: ReactNode;
}) {
  const { product, cash } = target;
  const cap = cash?.cents;
  const capped = (cents: number) => (cap === undefined ? cents : Math.min(cents, cap));
  const [text, setText] = useState(() => formatCents(capped(DEFAULT_CENTS)));
  // With less than US$ 1.000 in cash the opening amount is the whole cash.
  const [chip, setChip] = useState<Chip>(() => (cap !== undefined && cap < DEFAULT_CENTS ? 'all' : '1000'));
  const cents = centsOf(text);
  const heading = useFocusedHeading();
  const described = [cash ? 'valor-compra-caixa' : '', 'valor-compra-minimo', product.warning ? 'valor-compra-aviso' : '']
    .filter(Boolean)
    .join(' ');

  function pick(key: Exclude<Chip, null>, value: number) {
    setChip(key);
    setText(formatCents(capped(value)));
  }

  function typed(value: string) {
    setChip(null);
    setText(cap !== undefined && centsOf(value) > cap ? formatCents(cap) : value);
  }

  function submit(event: FormEvent) {
    event.preventDefault();
    onSubmit(cents);
  }

  return (
    <form className="pov-buy" aria-label={`Investir em ${product.name}`} onSubmit={submit}>
      {head ?? (
        <button type="button" className="pov-buy__back" aria-label="Voltar para Investir" onClick={onBack}>
          <BackChevron />
          Investir
        </button>
      )}
      <h1 ref={heading} tabIndex={-1}>
        Investir em {product.name}
      </h1>
      <div className="pov-buy__card">
        <span className="sdui-kicker">{product.class_label}</span>
        <span className="sdui-product__risk">
          <RiskBars risk={product.risk} above={product.above_profile} />
          {product.risk_label}
        </span>
        <span className="pov-buy__return">{product.return_label}</span>
        <span className="pov-buy__fine">Preço fixo da simulação, sem cotação. Rentabilidade fictícia.</span>
      </div>
      <div className="pov-buy__amount">
        <label htmlFor="valor-compra">Quanto investir</label>
        <input
          id="valor-compra"
          inputMode="decimal"
          autoComplete="off"
          aria-describedby={described}
          value={text}
          onChange={(event) => typed(event.target.value)}
        />
        <div className="pov-buy__chips" role="group" aria-label="Valores sugeridos">
          {CHIPS.map((item) => (
            <button key={item.key} type="button" aria-pressed={chip === item.key} onClick={() => pick(item.key, item.cents)}>
              {item.label}
            </button>
          ))}
          {cash ? (
            <button type="button" aria-pressed={chip === 'all'} onClick={() => pick('all', cash.cents)}>
              Tudo
            </button>
          ) : null}
        </div>
        {cash ? (
          <span id="valor-compra-caixa" className="pov-buy__hint">
            Disponível no caixa: {masked ? MASKED_MONEY : cash.display}
          </span>
        ) : null}
        <span id="valor-compra-minimo" className="pov-buy__hint">
          {product.minimum}
        </span>
      </div>
      {product.warning ? (
        <div id="valor-compra-aviso" role="note" className="pov-buy__warn">
          <strong>
            <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.9" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
              <path d="M12 3l10 18H2zM12 10v5M12 18v.5" />
            </svg>
            {product.badge}
          </strong>
          <p>{product.warning}</p>
        </div>
      ) : null}
      {notice ? <p role="alert">{notice}</p> : null}
      <button type="submit" className="pov-buy__primary" disabled={busy || cents <= 0 || cents < product.minimum_cents}>
        Confirmar compra de {masked ? MASKED_MONEY : formatCents(cents)}
      </button>
      <button type="button" className="pov-buy__ghost" onClick={onBack}>
        Cancelar
      </button>
    </form>
  );
}

/**
 * The "Compra enviada" confirmation (Compra-Enviada): the protocol, the event
 * that account-sim recorded, and the Bastidores steps with the states the
 * customer stream reports for this event.
 */
export function PurchaseSent({
  bought,
  steps,
  masked,
  onHome,
  head,
}: {
  bought: Bought;
  steps: LiveStep[];
  masked: boolean;
  onHome: () => void;
  /** The side panel header with its close control, in the desktop drawer. */
  head?: ReactNode;
}) {
  const labels = purchaseSteps(bought.product);
  const heading = useFocusedHeading();
  return (
    <section className="pov-buy pov-buy--sent" aria-label="Compra enviada">
      {head}
      <span className="pov-buy__ok" aria-hidden="true">
        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 12l5 5 9-10" />
        </svg>
      </span>
      <h1 ref={heading} tabIndex={-1}>
        Compra enviada
      </h1>
      <p className="pov-buy__lead">
        {masked ? MASKED_MONEY : formatCents(bought.cents)} em {bought.product.name}. O valor saiu do caixa e já aparece na sua carteira.
      </p>
      <div className="pov-buy__card">
        <div className="pov-buy__protocol">
          <span>Protocolo</span>
          <code>{protocolOf(bought.event_id)}</code>
        </div>
        <div className="pov-buy__bastidores">
          <strong>Bastidores</strong>
          <code>account.event.recorded · kind aplicacao · schema 3</code>
          <code>event_id {bought.event_id}</code>
        </div>
        <ol className="pov-buy__steps">
          {labels.map((label, index) => {
            const state = steps[index]?.state ?? 'aguardando';
            return (
              <li key={label} data-state={state}>
                <i aria-hidden="true" />
                <span>{label}</span>
                <em>{state}</em>
              </li>
            );
          })}
        </ol>
      </div>
      <Link to="/fila" className="pov-buy__secondary">
        Ver na fila do time
      </Link>
      <button type="button" className="pov-buy__primary" onClick={onHome}>
        Voltar ao início
      </button>
    </section>
  );
}
