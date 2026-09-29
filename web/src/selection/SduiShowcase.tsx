import { useEffect, useId, useState } from 'react';
import { fetchPOVClients, type POVClient } from '../api/pov';
import { fetchScreenResponse } from '../sdui/api';
import { SduiContext, type SduiContextValue } from '../sdui/context';
import { SduiScreen } from '../sdui/SduiScreen';
import type { Screen } from '../sdui/types';
import {
  MOMENT_RULE_COUNT,
  SDUI_CLIENTS,
  SDUI_DEFAULT_CLIENT,
  SDUI_PRINCIPLES,
  SDUI_STEPS,
  momentRule,
  type SduiClientName,
} from './sdui';

/** How long the section waits for the POV client list. */
export const ROSTER_TIMEOUT_MS = 3000;

/** Lines of the response shown before the disclosure. */
export const JSON_PREVIEW_LINES = 24;

type Roster = { status: 'loading' } | { status: 'ready'; clients: POVClient[] } | { status: 'down' };

/** A seed client as the POV list reports it; fields the list got wrong are left out. */
interface RosterClient {
  segment?: string;
  name?: string;
}

type Live =
  | { status: 'loading'; who: SduiClientName }
  | { status: 'ready'; who: SduiClientName; client: RosterClient; screen: Screen; httpStatus: number; json: string }
  | { status: 'down'; who: SduiClientName }
  | { status: 'missing'; who: SduiClientName };

/** The rendered home is a picture of the app: its controls do nothing here. */
const PREVIEW: SduiContextValue = {
  onNavigate: () => undefined,
  masked: false,
  onToggleMask: () => undefined,
  onPanel: () => undefined,
  onPurchase: () => undefined,
  light: false,
  onToggleTheme: () => undefined,
  onPreferences: async () => undefined,
};

function initials(name: string): string {
  return name
    .split(' ')
    .slice(0, 2)
    .map((part) => part[0] ?? '')
    .join('')
    .toUpperCase();
}

function seedID(who: SduiClientName): string {
  return SDUI_CLIENTS.find((item) => item.first === who)?.id ?? '';
}

/** Finds a seed client in the POV list by its fixed id, keeping only well-typed fields. */
function findClient(clients: readonly unknown[], id: string): RosterClient | undefined {
  const row = clients.find(
    (item): item is Record<string, unknown> => typeof item === 'object' && item !== null && (item as Record<string, unknown>).customer_id === id,
  );
  if (!row) {
    return undefined;
  }
  return {
    segment: typeof row.segment === 'string' && row.segment !== '' ? row.segment : undefined,
    name: typeof row.name === 'string' && row.name !== '' ? row.name : undefined,
  };
}

function momentVariant(screen: Screen): string | undefined {
  return screen.sections.find((section) => section.id === 'moment')?.components[0]?.variant;
}

/**
 * The selection screen's #sdui section: client tabs, the live home envelope of
 * the selected seed client, the moment variant and its documented rule, the
 * rendered home in a phone frame, then the composition steps and principles.
 * When the BFF is unreachable it shows a neutral note in place of the demo.
 */
export function SduiShowcase({ wide }: { wide: boolean }) {
  const [roster, setRoster] = useState<Roster>({ status: 'loading' });
  const [rosterTry, setRosterTry] = useState(0);
  const [who, setWho] = useState<SduiClientName>(SDUI_DEFAULT_CLIENT);
  const [live, setLive] = useState<Live>({ status: 'loading', who: SDUI_DEFAULT_CLIENT });

  useEffect(() => {
    let gone = false;
    const abort = new AbortController();
    fetchPOVClients(AbortSignal.any([abort.signal, AbortSignal.timeout(ROSTER_TIMEOUT_MS)]))
      .then((clients) => {
        if (!gone) {
          setRoster({ status: 'ready', clients });
        }
      })
      .catch(() => {
        if (!gone) {
          setRoster({ status: 'down' });
        }
      });
    return () => {
      gone = true;
      abort.abort();
    };
  }, [rosterTry]);

  useEffect(() => {
    if (roster.status === 'loading') {
      return;
    }
    if (roster.status === 'down') {
      setLive({ status: 'down', who });
      return;
    }
    const id = seedID(who);
    const client = findClient(roster.clients, id);
    if (!client) {
      setLive({ status: 'missing', who });
      return;
    }
    let gone = false;
    const abort = new AbortController();
    setLive({ status: 'loading', who });
    fetchScreenResponse(id, 'home', abort.signal)
      .then(({ screen, status, body }) => {
        if (!gone) {
          setLive({ status: 'ready', who, client, screen, httpStatus: status, json: JSON.stringify(body, null, 2) });
        }
      })
      .catch(() => {
        if (!gone) {
          setLive({ status: 'down', who });
        }
      });
    return () => {
      gone = true;
      abort.abort();
    };
  }, [roster, who]);

  const clients = roster.status === 'ready' ? roster.clients : [];
  const current = live.who === who ? live : ({ status: 'loading', who } as const);

  function pick(next: SduiClientName) {
    setWho(next);
    // A failed or timed-out POV list is asked again on the next tab click.
    if (roster.status === 'down') {
      setRoster({ status: 'loading' });
      setLive({ status: 'loading', who: next });
      setRosterTry((value) => value + 1);
    }
  }

  return (
    <section id="sdui" className="selection__sdui" aria-labelledby="selection-sdui">
      <div className="selection__sdui-head">
        <div>
          <p className="selection__eyebrow selection__eyebrow--brand">Server-Driven UI</p>
          <h2 id="selection-sdui">O backend monta a tela de cada cliente</h2>
          <p>
            {wide
              ? 'Cada tela do app chega numa única resposta do bff: página, seções e componentes, já preenchidos. O backend escolhe a variante de cada seção pelo momento do cliente. O front conhece um catálogo fixo de componentes e só desenha o que recebeu.'
              : 'Cada tela chega numa única resposta do bff, já preenchida para o momento do cliente. O app só desenha um catálogo fixo de componentes.'}
          </p>
        </div>
        <div role="group" aria-label="Escolha um cliente" className="selection__sdui-tabs">
          {SDUI_CLIENTS.map((item) => {
            const segment = findClient(clients, item.id)?.segment ?? item.segment;
            return (
              <button
                key={item.first}
                type="button"
                aria-label={`${item.first} · ${segment}`}
                aria-pressed={item.first === who}
                onClick={() => pick(item.first)}
              >
                {item.first}
                <span className="selection__sdui-tab-sep"> · </span>
                <span className="selection__sdui-tab-seg">{segment}</span>
              </button>
            );
          })}
        </div>
      </div>

      <LiveDemo live={current} wide={wide} />

      <ol className="selection__sdui-steps">
        {SDUI_STEPS.map((step, index) => (
          <li key={step.title}>
            <span className="selection__sdui-num">{String(index + 1).padStart(2, '0')}</span>
            <div>
              <h3>{step.title}</h3>
              <p>{step.text}</p>
              {wide ? <code>{step.tag}</code> : null}
            </div>
          </li>
        ))}
      </ol>

      <div className="selection__sdui-principles">
        {SDUI_PRINCIPLES.map((item) => (
          <article key={item.title}>
            <h3>{item.title}</h3>
            <p>{item.text}</p>
          </article>
        ))}
      </div>
    </section>
  );
}

function LiveDemo({ live, wide }: { live: Live; wide: boolean }) {
  if (live.status === 'loading') {
    return (
      <p role="status" className="selection__sdui-note">
        Buscando no bff a tela de {live.who}…
      </p>
    );
  }
  if (live.status === 'missing') {
    return (
      <p role="status" className="selection__sdui-note">
        Esse cliente não está na lista do bff agora.
      </p>
    );
  }
  if (live.status === 'down') {
    return (
      <p role="status" className="selection__sdui-note">
        O bff não respondeu agora, então a resposta ao vivo não aparece aqui. Com a demo no ar, cada cliente mostra a tela que o
        backend montou para ele.
      </p>
    );
  }
  const variant = momentVariant(live.screen);
  const omittedMoment = live.screen.omitted.find((row) => row.id === 'moment');
  const response = (
    <div className="selection__sdui-response">
      <div className="selection__sdui-request">
        <span className="selection__eyebrow">Resposta do bff</span>
        {wide ? <code>GET /v1/client-pov/customers/{'{id}'}/screens/home</code> : null}
        <span className="selection__sdui-ok">{live.httpStatus} · 1 requisição</span>
      </div>
      <ResponseJSON key={live.who} json={live.json} />
    </div>
  );
  const facts = (
    <div className="selection__sdui-facts">
      <div>
        <span className="selection__eyebrow">Momento (advisory, gRPC)</span>
        <code>{omittedMoment ? 'omitido' : variant || '—'}</code>
      </div>
      <RulePanel variant={variant} omittedReason={omittedMoment?.reason} />
    </div>
  );
  const phone = <PhoneFrame who={live.who} client={live.client} screen={live.screen} />;
  if (!wide) {
    return (
      <div className="selection__sdui-demo">
        {phone}
        {response}
        {facts}
      </div>
    );
  }
  return (
    <div className="selection__sdui-demo">
      <div className="selection__sdui-left">
        {response}
        {facts}
      </div>
      <div className="selection__sdui-arrow" aria-hidden="true">
        <svg width="28" height="28" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round">
          <path d="M5 12h14M13 6l6 6-6 6" />
        </svg>
        <span>por type</span>
      </div>
      <div className="selection__sdui-render">
        <span className="selection__eyebrow">O que o app desenha</span>
        {phone}
      </div>
    </div>
  );
}

function RulePanel({ variant, omittedReason }: { variant: string | undefined; omittedReason: string | undefined }) {
  const rule = variant === undefined ? undefined : momentRule(variant);
  return (
    <div>
      <span className="selection__eyebrow">A regra que escolheu a variante</span>
      {omittedReason !== undefined ? (
        <p>A seção moment ficou fora da resposta (motivo: {omittedReason || 'não informado'}), então nenhuma variante foi escolhida.</p>
      ) : rule ? (
        <>
          <code>
            {rule.rank} de {MOMENT_RULE_COUNT} · {variant} · {rule.source}
          </code>
          <p>{rule.fact}</p>
        </>
      ) : (
        <p>Sem linha documentada na tabela de prioridade para esta variante.</p>
      )}
    </div>
  );
}

function ResponseJSON({ json }: { json: string }) {
  const id = useId();
  const [open, setOpen] = useState(false);
  const lines = json.split('\n');
  const long = lines.length > JSON_PREVIEW_LINES;
  const shown = long && !open ? `${lines.slice(0, JSON_PREVIEW_LINES).join('\n')}\n…` : json;
  return (
    <>
      <pre id={id} className="selection__sdui-json">
        {shown}
      </pre>
      {long ? (
        <button type="button" className="selection__sdui-more" aria-expanded={open} aria-controls={id} onClick={() => setOpen((value) => !value)}>
          {open ? 'Mostrar menos' : `Mostrar a resposta inteira (${lines.length} linhas)`}
        </button>
      ) : null}
    </>
  );
}

function PhoneFrame({ who, client, screen }: { who: SduiClientName; client: RosterClient; screen: Screen }) {
  return (
    <div className="selection__sdui-phone" role="region" aria-label={`Início de ${who} no app`} tabIndex={0}>
      <div className="pov-app selection__sdui-app" inert>
        <div className="selection__sdui-appbar">
          <span className="pov-app__logo">
            orla<span>.</span>
          </span>
          <span aria-hidden="true">{initials(client.name ?? who)}</span>
        </div>
        <SduiContext value={PREVIEW}>
          <SduiScreen screen={screen} />
        </SduiContext>
      </div>
    </div>
  );
}
