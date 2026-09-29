import { useEffect, useRef, useState, type ReactNode } from 'react';
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom';
import { apiPath } from '../api/base';
import { ApiError } from '../api/bff';
import { dollars, fetchPOVHome, fetchSimulation, formatCents, postAdvanceDay, postPOV, protocolOf, putPreferences, type POVHome } from '../api/pov';
import { fetchScreen } from '../sdui/api';
import { SduiContext, type SduiContextValue } from '../sdui/context';
import { findPurchase, type PurchaseTarget } from '../sdui/purchase';
import { SduiError, SduiLoading, SduiScreen } from '../sdui/SduiScreen';
import type { Preferences, Screen, Slug } from '../sdui/types';
import { purchaseSteps, PurchaseForm, PurchaseSent, type Bought, type LiveStep } from './PurchasePanel';

interface Sent {
  event_id: string;
  title: string;
  lead: string;
  routing: string;
  detail: string;
  backLabel: string;
}

interface ChatRow {
  text: string;
  meta: string;
}

/**
 * What is open over the screen. The coded side panels (`deposit` … `done`)
 * open as a drawer on desktop and full screen on the phone. The purchase form
 * and its confirmation open in the same drawer on desktop and take the screen
 * area on the phone, as in Compra-Fernanda-Aviso.
 */
type Panel = 'home' | 'deposit' | 'withdraw' | 'complaint' | 'message' | 'done' | 'purchase' | 'bought';

/**
 * The SDUI screen of the current tab. `slug` says which request the state
 * belongs to, so a tab change shows the skeleton instead of the previous
 * tab's screen. There is no hardcoded screen to fall back to: a failed,
 * timed out, or non-envelope response is the error state with a retry.
 */
type ScreenView = { slug: Slug; status: 'loading' } | { slug: Slug; status: 'ready'; screen: Screen } | { slug: Slug; status: 'error' };

/**
 * Client app tabs; each renders the SDUI screen of the same slug.
 */
const TABS = [
  { slug: 'home', label: 'Início', icon: 'home' },
  { slug: 'investir', label: 'Investir', icon: 'invest' },
  { slug: 'carteira', label: 'Carteira', icon: 'wallet' },
  { slug: 'perfil', label: 'Perfil', icon: 'user' },
] as const;

type TabSlug = (typeof TABS)[number]['slug'];

const PRESETS = [
  {
    title: 'Minha transferência está atrasada',
    text: 'Pedi uma transferência há dias e até agora não caiu. Preciso de uma posição hoje.',
  },
  {
    title: 'Uma cobrança que não reconheço',
    text: 'Apareceu uma cobrança no meu extrato que eu não reconheço. Quero entender o que é.',
  },
  {
    title: 'Quero falar com uma pessoa',
    text: 'Não quero mais resposta automática. Preciso que alguém da assessoria me ligue.',
  },
  {
    title: 'Estou pensando em sair',
    text: 'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.',
  },
];

const ICONS: Record<string, string> = {
  home: 'M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z',
  invest: 'M3 17l6-6 4 4 8-8M15 7h6v6',
  wallet: 'M21 12A9 9 0 1 1 12 3v9zM15 3.5A9 9 0 0 1 20.5 9H15z',
  user: 'M16 21v-1a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v1M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  sun: 'M12 12m-4 0a4 4 0 1 0 8 0a4 4 0 1 0 -8 0M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z',
};

const ACCOUNT_STEPS = [
  'Gravado na outbox do account-sim',
  'Publicado no RabbitMQ',
  'Avaliado pelas regras do advisory',
  'Na fila da assessoria, se uma regra disparar',
];

const MESSAGE_STEPS = [
  'Gravado na outbox do account-sim',
  'Publicado no RabbitMQ',
  'Classificado pela triagem',
  'Na fila da assessoria',
];

const TOO_MANY = 'Muitas ações em pouco tempo. Espere um minuto e tente de novo.';
const NOT_RECORDED = 'Não foi possível registrar a ação.';
// The advance-day budget is global: 20 days per 10 minutes for everyone.
const TOO_MANY_DAYS = 'Muitos dias avançados em pouco tempo. Espere até 10 minutos e tente de novo.';
// advisory consumes the reavaliacao events after the 202, so the screen is
// read once more this long after an advance to show the drop moment and row.
const ADVANCE_REFETCH_MS = 1500;

function Icon({ name, size = 20 }: { name: string; size?: number }) {
  return (
    <svg width={size} height={size} viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="1.8" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={ICONS[name] ?? ''} />
    </svg>
  );
}

function useWide(): boolean {
  const query = '(min-width: 900px)';
  const [wide, setWide] = useState(() => window.matchMedia?.(query).matches ?? false);
  useEffect(() => {
    if (!window.matchMedia) {
      return;
    }
    const media = window.matchMedia(query);
    const onChange = () => setWide(media.matches);
    onChange();
    media.addEventListener('change', onChange);
    return () => media.removeEventListener('change', onChange);
  }, []);
  return wide;
}

function money(amount: number): string {
  return amount.toLocaleString('pt-BR', { minimumFractionDigits: 2, maximumFractionDigits: 2 });
}

function initials(name: string): string {
  return name
    .split(' ')
    .slice(0, 2)
    .map((part) => part[0] ?? '')
    .join('')
    .toUpperCase();
}

/** The first step is done when the BFF answers 202; the stream moves the rest. */
function accepted(labels: string[]): LiveStep[] {
  return labels.map((label, index) => ({ id: label, label, state: index === 0 ? 'feito' : 'aguardando' }));
}

/**
 * Whether a purchase or advance refusal is final for its Idempotency-Key: any
 * 4xx but a 429 means that request will never be accepted, so a retry is a
 * new one.
 */
function definiteRefusal(err: unknown): boolean {
  return err instanceof ApiError && err.status >= 400 && err.status < 500 && err.status !== 429;
}

/** The notice of a purchase the BFF refused (story 9 contract). */
function purchaseNotice(err: unknown): string {
  if (err instanceof ApiError && err.status === 422) {
    return err.code === 'insufficient' ? 'Valor acima do caixa disponível.' : 'Confira o valor e tente de novo.';
  }
  if (err instanceof ApiError && err.status === 429) {
    return TOO_MANY;
  }
  return NOT_RECORDED;
}

const THEME_KEY = 'advisor-radar.pov-theme';

function readLightTheme(): boolean {
  try {
    return localStorage.getItem(THEME_KEY) === 'light';
  } catch {
    return false;
  }
}

function storeLightTheme(light: boolean) {
  try {
    localStorage.setItem(THEME_KEY, light ? 'light' : 'dark');
  } catch {
    // Keep the in-memory theme when storage is unavailable.
  }
}

/** Raio-X SDUI is a demo tool; remembering it is a per-browser convenience. */
const XRAY_KEY = 'advisor-radar.pov-xray';

function readXray(): boolean {
  try {
    return localStorage.getItem(XRAY_KEY) === 'on';
  } catch {
    return false;
  }
}

function storeXray(on: boolean) {
  try {
    localStorage.setItem(XRAY_KEY, on ? 'on' : 'off');
  } catch {
    // Keep the in-memory toggle when storage is unavailable.
  }
}

/** The canonical client app route of one tab; home has no tab segment. */
function clientPath(id: string, slug: TabSlug): string {
  const base = `/client-pov/${encodeURIComponent(id)}`;
  return slug === 'home' ? base : `${base}/${slug}`;
}

export function ClientAppScreen() {
  const { id = '', tab } = useParams();
  const current = TABS.find((item) => item.slug === (tab ?? 'home'));
  if (!current || tab === 'home') {
    return <Navigate to={clientPath(id, 'home')} replace />;
  }
  return <ClientApp key={id} id={id} tab={current.slug} />;
}

function ClientApp({ id, tab }: { id: string; tab: TabSlug }) {
  const navigate = useNavigate();
  const wide = useWide();
  const [light, setLight] = useState(readLightTheme);
  const [xray, setXray] = useState(readXray);
  const [hide, setHide] = useState(false);
  const [channel, setChannel] = useState<'chat' | 'e-mail'>('chat');
  const [text, setText] = useState('');
  const [preset, setPreset] = useState(-1);
  const [home, setHome] = useState<POVHome | null>(null);
  const [error, setError] = useState(false);
  const [panel, setPanel] = useState<Panel>('home');
  const [amount, setAmount] = useState('');
  const [origin, setOrigin] = useState('Câmbio a partir do Brasil');
  const [destination, setDestination] = useState('Conta nos EUA');
  const [notice, setNotice] = useState('');
  const [sent, setSent] = useState<Sent | null>(null);
  const [steps, setSteps] = useState<LiveStep[]>([]);
  const [extraChat, setExtraChat] = useState<ChatRow[]>([]);
  const slug: Slug = tab;
  const [view, setView] = useState<ScreenView>({ slug, status: 'loading' });
  const [reload, setReload] = useState(0);
  const [purchase, setPurchase] = useState<PurchaseTarget | null>(null);
  const [bought, setBought] = useState<Bought | null>(null);
  const [buying, setBuying] = useState(false);
  // The global simulated day; null until GET …/simulation answers (a failed
  // read hides the day and keeps the button).
  const [simDay, setSimDay] = useState<number | null>(null);
  const [advancing, setAdvancing] = useState(false);
  const [simNotice, setSimNotice] = useState('');
  // Bumped after a failed advance so the strip re-reads the day.
  const [simReads, setSimReads] = useState(0);
  // The screen an advance asked to read again after ADVANCE_REFETCH_MS.
  const [followUp, setFollowUp] = useState<{ id: string; slug: Slug } | null>(null);
  // A tab change, including browser back and forward, closes any panel.
  const [panelTab, setPanelTab] = useState(tab);
  if (panelTab !== tab) {
    setPanelTab(tab);
    setPanel('home');
  }
  // One Idempotency-Key per open form and amount, so a retry of the same
  // amount cannot buy twice; `form` counts panel changes so a purchase answer
  // that arrives after its form closed is dropped.
  const buyKey = useRef<{ cents: number; key: string } | null>(null);
  const form = useRef(0);
  const inFlight = useRef(false);
  // One Idempotency-Key per advance until a definite answer (202, or a 4xx
  // other than 429), so a retry after an uncertain failure cannot advance twice.
  const advanceKey = useRef<string | null>(null);
  const liveEvent = panel === 'done' ? sent?.event_id : panel === 'bought' ? bought?.event_id : undefined;

  useEffect(() => {
    form.current += 1;
  }, [panel, tab]);

  // The day is global: it is re-read with every screen read, so an advance by
  // another viewer shows, and after a failed advance.
  useEffect(() => {
    if (panel !== 'home') {
      return;
    }
    let gone = false;
    const abort = new AbortController();
    fetchSimulation(abort.signal)
      .then((day) => {
        if (!gone) {
          setSimDay(day);
        }
      })
      .catch(() => undefined);
    return () => {
      gone = true;
      abort.abort();
    };
  }, [id, panel, slug, reload, simReads]);

  // The second read of the screen after an advance; leaving the screen or the
  // app cancels it.
  useEffect(() => {
    if (!followUp || followUp.id !== id || followUp.slug !== slug) {
      return;
    }
    const timer = window.setTimeout(() => {
      setFollowUp(null);
      setReload((value) => value + 1);
    }, ADVANCE_REFETCH_MS);
    return () => window.clearTimeout(timer);
  }, [followUp, id, slug]);

  useEffect(() => {
    if (!liveEvent) {
      return;
    }
    const source = new EventSource(apiPath(`v1/client-pov/customers/${id}/stream`));
    const onStep = (ev: Event) => {
      const data = JSON.parse((ev as MessageEvent<string>).data) as { event_id: string; steps: LiveStep[] };
      if (data.event_id === liveEvent) {
        setSteps(data.steps);
      }
    };
    source.addEventListener('bastidores', onStep);
    return () => {
      source.removeEventListener('bastidores', onStep);
      source.close();
    };
  }, [liveEvent, id]);

  // The phase-2 customer read feeds the shell and the coded panels (name,
  // cash to withdraw, advisor, messages); it is re-read when a panel closes.
  useEffect(() => {
    if (panel !== 'home') {
      return;
    }
    let gone = false;
    fetchPOVHome(id)
      .then((row) => {
        if (!gone) {
          setHome(row);
        }
      })
      .catch(() => {
        if (!gone) {
          setError(true);
        }
      });
    return () => {
      gone = true;
    };
  }, [id, panel]);

  // One screen request per tab visit, panel close, or retry. A re-fetch of
  // the same screen keeps the current one on view until it settles.
  useEffect(() => {
    if (panel !== 'home') {
      return;
    }
    let gone = false;
    const abort = new AbortController();
    // A timeout (SCREEN_TIMEOUT_MS) rejects like any other failure.
    fetchScreen(id, slug, abort.signal)
      .then((screen) => {
        if (!gone) {
          setView({ slug, status: 'ready', screen });
        }
      })
      .catch(() => {
        if (!gone) {
          setView({ slug, status: 'error' });
        }
      });
    return () => {
      gone = true;
      abort.abort();
    };
  }, [id, panel, slug, reload]);

  const cents = Math.round(dollars(amount) * 100);
  const overCash = panel === 'withdraw' && home != null && cents > home.caixa;

  function open(next: Panel) {
    setAmount('');
    setNotice('');
    setText('');
    setPreset(-1);
    setPanel(next);
  }

  function toggleXray() {
    setXray((value) => {
      const next = !value;
      storeXray(next);
      return next;
    });
  }

  function toggleTheme() {
    setLight((value) => {
      const next = !value;
      storeLightTheme(next);
      return next;
    });
  }

  // Perfil stores the channel and beta flag, then re-reads the screen; the
  // current screen stays on view until the re-read settles. A failed write
  // rejects so the preference list keeps the previous value.
  async function savePreferences(next: Preferences) {
    await putPreferences(id, next);
    setReload((value) => value + 1);
  }

  function pickNav(next: TabSlug) {
    open('home');
    navigate(clientPath(id, next));
  }

  function retry() {
    setView({ slug, status: 'loading' });
    setReload((value) => value + 1);
  }

  // A press keeps its key until a definite answer. The screen on view is
  // re-fetched in place at once and again after ADVANCE_REFETCH_MS, when
  // advisory has applied the revaluation.
  async function advanceDay() {
    setAdvancing(true);
    setSimNotice('');
    advanceKey.current ??= crypto.randomUUID();
    try {
      const done = await postAdvanceDay(advanceKey.current);
      advanceKey.current = null;
      setSimDay(done.sim_day);
      setReload((value) => value + 1);
      setFollowUp({ id, slug });
    } catch (err) {
      if (definiteRefusal(err)) {
        advanceKey.current = null;
      }
      setSimNotice(err instanceof ApiError && err.status === 429 ? TOO_MANY_DAYS : NOT_RECORDED);
      setSimReads((value) => value + 1);
    } finally {
      setAdvancing(false);
    }
  }

  function openPurchase(productID: string) {
    const target = view.status === 'ready' ? findPurchase(view.screen, productID) : null;
    if (!target) {
      console.error(`sdui: purchase product ${productID} is not on the screen`);
      return;
    }
    buyKey.current = null;
    // A form opened over another one is a new form: a late answer for the
    // previous product must not land on it, even when `panel` stays 'purchase'.
    form.current += 1;
    setPurchase(target);
    open('purchase');
  }

  async function buy(target: PurchaseTarget, amountCents: number) {
    if (inFlight.current) {
      return;
    }
    if (buyKey.current?.cents !== amountCents) {
      buyKey.current = { cents: amountCents, key: crypto.randomUUID() };
    }
    const key = buyKey.current.key;
    const opened = form.current;
    inFlight.current = true;
    setNotice('');
    setBuying(true);
    try {
      const done = await postPOV(id, 'purchases', { product_id: target.product.product_id, amount_cents: amountCents }, key);
      buyKey.current = null;
      if (form.current !== opened) {
        return;
      }
      setBought({ event_id: done.event_id, cents: amountCents, product: target.product });
      setSteps(accepted(purchaseSteps(target.product)));
      setPanel('bought');
    } catch (err) {
      if (definiteRefusal(err)) {
        buyKey.current = null;
      }
      if (form.current === opened) {
        setNotice(purchaseNotice(err));
      }
    } finally {
      inFlight.current = false;
      setBuying(false);
    }
  }

  async function send(
    kind: 'deposits' | 'withdrawals' | 'complaints' | 'messages',
    body: Record<string, unknown>,
    next: Omit<Sent, 'event_id'>,
    labels: string[],
  ) {
    setNotice('');
    try {
      const done = await postPOV(id, kind, body, crypto.randomUUID());
      setSent({ ...next, event_id: done.event_id });
      setSteps(accepted(labels));
      setPanel('done');
      if (kind === 'messages') {
        setExtraChat((rows) => rows.concat([{ text: String(body.text), meta: `agora · ${String(body.channel)}` }]));
        setText('');
      }
    } catch (err) {
      if (err instanceof ApiError && err.status === 422) {
        setNotice('Valor acima do disponível para saque');
        return;
      }
      if (err instanceof ApiError && err.status === 429) {
        setNotice(TOO_MANY);
        return;
      }
      setNotice(NOT_RECORDED);
    }
  }

  // A failed re-read keeps the shell already on view; only a first read is terminal.
  if (error && !home) {
    return (
      <main className="pov-app">
        <p role="alert">Não foi possível abrir este cliente.</p>
        <Link to="/client-pov">Voltar à lista</Link>
      </main>
    );
  }
  const themeLabel = light ? 'Tema escuro' : 'Tema claro';
  if (!home) {
    // One skeleton until the shell data settles; the screen area then keeps
    // its own skeleton until the screen request settles.
    return (
      <main className={light ? 'pov-app pov-app--light' : 'pov-app'} data-layout={wide ? 'desktop' : 'phone'}>
        <Strip wide={wide} xray={xray} onXray={undefined} />
        <div className="pov-app__body">
          <div className="pov-app__main">
            {wide ? null : <PhoneBar light={light} themeLabel={themeLabel} onTheme={toggleTheme} initials="" />}
            <div className="sdui-main">
              <SduiLoading />
            </div>
          </div>
        </div>
        {wide ? null : <TabBar tab={tab} onPick={pickNav} />}
      </main>
    );
  }

  const chat = home.messages.concat(extraChat);
  const closeLabel = wide ? 'Fechar' : 'Voltar';
  const buyingOnView = (panel === 'purchase' && purchase !== null) || (panel === 'bought' && bought !== null);
  const buyInScreen = buyingOnView && !wide;
  const drawer = (panel !== 'home' && panel !== 'purchase' && panel !== 'bought') || (buyingOnView && wide);
  const showScreen = !buyInScreen && (wide || panel === 'home');
  const complaintText = preset >= 0 ? PRESETS[preset]?.text ?? '' : text.trim();
  const ready = view.slug === slug && view.status === 'ready' ? view.screen : null;
  // Raio-X only has something to outline while an SDUI screen of its own tab is on view.
  const sduiOnView = showScreen && ready !== null;
  const sdui: SduiContextValue = {
    onNavigate: pickNav,
    masked: hide,
    onToggleMask: () => setHide((value) => !value),
    onPanel: open,
    onPurchase: openPurchase,
    light,
    onToggleTheme: toggleTheme,
    onPreferences: savePreferences,
    wide,
  };
  // In the desktop drawer the purchase views carry the panel header and its close control.
  const buyHead = wide ? <PanelHead closeLabel={closeLabel} onClose={() => open('home')} /> : undefined;
  let buyView: ReactNode = null;
  if (panel === 'purchase' && purchase) {
    buyView = (
      <PurchaseForm
        key={purchase.product.product_id}
        target={purchase}
        notice={notice}
        busy={buying}
        masked={hide}
        head={buyHead}
        onBack={() => open('home')}
        onSubmit={(value) => void buy(purchase, value)}
      />
    );
  } else if (panel === 'bought' && bought) {
    buyView = <PurchaseSent bought={bought} steps={steps} masked={hide} head={buyHead} onHome={() => pickNav('home')} />;
  }
  let area = <SduiLoading />;
  if (ready) {
    area = (
      <SduiContext value={sdui}>
        <SduiScreen screen={ready} xray={xray && sduiOnView ? { customerID: id } : undefined} />
      </SduiContext>
    );
  } else if (view.slug === slug && view.status === 'error') {
    area = <SduiError onRetry={retry} />;
  }

  return (
    <main className={light ? 'pov-app pov-app--light' : 'pov-app'} data-layout={wide ? 'desktop' : 'phone'}>
      <Strip
        name={home.name}
        wide={wide}
        xray={xray}
        onXray={sduiOnView && !buyingOnView ? toggleXray : undefined}
        sim={{ day: simDay, busy: advancing, notice: simNotice, onAdvance: () => void advanceDay() }}
      />
      <div className="pov-app__body">
        {wide ? (
          <aside className="pov-app__side">
            <p className="pov-app__logo">
              orla<span>.</span>
              <em>invest</em>
            </p>
            <nav aria-label="Navegação principal">
              {TABS.map((item) => (
                <button
                  key={item.slug}
                  type="button"
                  className={item.slug === tab ? 'pov-app__nav-btn pov-app__nav-btn--on' : 'pov-app__nav-btn'}
                  aria-current={item.slug === tab ? 'page' : undefined}
                  onClick={() => pickNav(item.slug)}
                >
                  <Icon name={item.icon} /> {item.label}
                </button>
              ))}
            </nav>
            <span className="pov-app__grow" />
            <button type="button" className="pov-app__theme" aria-label={themeLabel} onClick={toggleTheme}>
              <Icon name={light ? 'moon' : 'sun'} size={18} />
              {themeLabel}
            </button>
            <div className="pov-app__who">
              <span aria-hidden="true">{initials(home.name)}</span>
              <div>
                <strong>{home.name}</strong>
                <em>Cliente {home.segment}</em>
              </div>
            </div>
          </aside>
        ) : null}
        {showScreen || buyInScreen ? (
          // Behind the desktop drawer the screen is visible but out of reach.
          <div className="pov-app__main" inert={drawer && wide}>
            {wide ? null : <PhoneBar light={light} themeLabel={themeLabel} onTheme={toggleTheme} initials={initials(home.name)} />}
            <div className="sdui-main">
              {buyInScreen ? buyView : null}
              {showScreen ? area : null}
            </div>
            <p className="pov-app__fine">Orla Invest é uma corretora fictícia criada para a demo do Advisor Radar.</p>
          </div>
        ) : null}
        {drawer && wide ? <button type="button" className="pov-app__scrim" aria-label="Fechar painel" onClick={() => open('home')} /> : null}
        {drawer ? (
          <aside className="pov-app__panel" aria-label="Ação">
            {buyInScreen ? null : buyView}
            {panel === 'deposit' || panel === 'withdraw' ? (
              <MoneyPanel
                kind={panel}
                amount={amount}
                cents={cents}
                overCash={overCash}
                cash={formatCents(home.caixa)}
                aum={formatCents(home.assets)}
                origin={origin}
                destination={destination}
                closeLabel={closeLabel}
                notice={notice}
                onAmount={setAmount}
                onOrigin={setOrigin}
                onDestination={setDestination}
                onClose={() => open('home')}
                onChip={(value) => setAmount(money(value))}
                onAll={() => setAmount(money(home.caixa / 100))}
                onSubmit={() => {
                  if (panel === 'deposit') {
                    void send(
                      'deposits',
                      { amount: cents, origin },
                      {
                        title: 'Depósito solicitado',
                        lead: `${formatCents(cents)} por ${origin === 'Câmbio a partir do Brasil' ? 'câmbio a partir do Brasil' : 'wire de outro banco'}.`,
                        routing: 'account.event.recorded',
                        detail: `kind: aporte · amount: ${dollars(amount)}`,
                        backLabel: 'Voltar ao início',
                      },
                      ACCOUNT_STEPS,
                    );
                    return;
                  }
                  void send(
                    'withdrawals',
                    { amount: cents, destination },
                    {
                      title: 'Saque solicitado',
                      lead: `${formatCents(cents)} para ${destination === 'Conta nos EUA' ? 'a conta nos EUA · final 4821' : 'a conta no Brasil · final 0937'}.`,
                      routing: 'account.event.recorded',
                      detail: `kind: saque · amount: ${dollars(amount)}`,
                      backLabel: 'Voltar ao início',
                    },
                    ACCOUNT_STEPS,
                  );
                }}
              />
            ) : null}
            {panel === 'complaint' ? (
              <form
                className="pov-app__form"
                onSubmit={(event) => {
                  event.preventDefault();
                  void send(
                    'complaints',
                    { text: complaintText },
                    {
                      title: 'Reclamação registrada',
                      lead: `“${complaintText}”`,
                      routing: 'message.received',
                      detail: 'canal: chat',
                      backLabel: 'Voltar ao início',
                    },
                    MESSAGE_STEPS,
                  );
                }}
              >
                <PanelHead title="Abrir reclamação" closeLabel={closeLabel} onClose={() => open('home')} />
                <p className="pov-app__lead">Conte o que aconteceu. Sua assessora recebe na hora.</p>
                <div role="group" aria-label="Motivos comuns" className="pov-app__options">
                  {PRESETS.map((item, index) => (
                    <button
                      key={item.title}
                      type="button"
                      className={preset === index ? 'pov-app__option pov-app__option--on' : 'pov-app__option'}
                      aria-pressed={preset === index}
                      onClick={() => {
                        setPreset(index);
                        setText('');
                      }}
                    >
                      <i />
                      <span>
                        <strong>{item.title}</strong>
                        <em>“{item.text}”</em>
                      </span>
                    </button>
                  ))}
                </div>
                <label className="pov-app__field">
                  Ou escreva com suas palavras
                  <textarea
                    rows={3}
                    placeholder="O que aconteceu?"
                    value={text}
                    onChange={(event) => {
                      setText(event.target.value);
                      setPreset(-1);
                    }}
                  />
                </label>
                <Note kicker="simulação → message.received · canal chat">
                  A triagem decide intenção, frustração, risco de saída e pedido de atendimento humano.
                </Note>
                {notice ? <p role="alert">{notice}</p> : null}
                <button type="submit" className="pov-app__primary" disabled={complaintText === ''}>
                  Enviar reclamação
                </button>
              </form>
            ) : null}
            {panel === 'message' ? (
              <form
                className="pov-app__chat"
                onSubmit={(event) => {
                  event.preventDefault();
                  void send(
                    'messages',
                    { channel, text },
                    {
                      title: 'Mensagem enviada',
                      lead: `“${text.trim()}”`,
                      routing: 'message.received',
                      detail: `canal: ${channel}`,
                      backLabel: 'Voltar à conversa',
                    },
                    MESSAGE_STEPS,
                  );
                }}
              >
                <div className="pov-app__chat-head">
                  <button type="button" aria-label={closeLabel} onClick={() => open('home')}>
                    <BackIcon wide={wide} />
                  </button>
                  <span aria-hidden="true">AP</span>
                  <div>
                    <h1>{home.advisor}</h1>
                    <em>Sua assessora · Orla Invest</em>
                  </div>
                </div>
                <div className="pov-app__thread">
                  <span>Suas mensagens chegam direto à assessoria</span>
                  {chat.length === 0 ? <p>Ainda não há mensagens. Escreva a primeira.</p> : null}
                  {chat.map((row) => (
                    <div key={`${row.meta}-${row.text}`}>
                      <p>{row.text}</p>
                      <em>{row.meta}</em>
                    </div>
                  ))}
                </div>
                <div className="pov-app__composer">
                  <div role="group" aria-label="Canal">
                    <button type="button" aria-pressed={channel === 'chat'} onClick={() => setChannel('chat')}>
                      Chat
                    </button>
                    <button type="button" aria-pressed={channel === 'e-mail'} onClick={() => setChannel('e-mail')}>
                      E-mail
                    </button>
                  </div>
                  <div>
                    <label htmlFor="chat-texto">Mensagem</label>
                    <textarea id="chat-texto" rows={2} placeholder="Escreva para sua assessora" value={text} onChange={(event) => setText(event.target.value)} />
                    <button type="submit" aria-label="Enviar mensagem" disabled={text.trim() === ''}>
                      <svg width="20" height="20" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                        <path d="M5 12h14M13 6l6 6-6 6" />
                      </svg>
                    </button>
                  </div>
                  <span>simulação → message.received · canal {channel}</span>
                </div>
              </form>
            ) : null}
            {panel === 'done' && sent ? (
              <section className="pov-app__sent">
                <span aria-hidden="true">
                  <svg width="30" height="30" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.2" strokeLinecap="round" strokeLinejoin="round">
                    <path d="M5 12.5l4.5 4.5L19 7.5" />
                  </svg>
                </span>
                <h1>{sent.title}</h1>
                <p>{sent.lead}</p>
                <code>Protocolo {protocolOf(sent.event_id)}</code>
                <section className="pov-app__bastidores" aria-label="Bastidores">
                  <header>
                    <span className="pov-app__mark" aria-hidden="true">
                      <i />
                      <i />
                      <i />
                    </span>
                    Bastidores · simulação
                  </header>
                  <dl>
                    <dt>Evento</dt>
                    <dd>{sent.routing}</dd>
                    <dt>Conteúdo</dt>
                    <dd>{sent.detail}</dd>
                    <dt>event_id</dt>
                    <dd>{sent.event_id}</dd>
                  </dl>
                  <ol>
                    {steps.map((step) => (
                      <li key={step.id} data-state={step.state}>
                        <i />
                        <span>
                          {step.label} {step.state}
                        </span>
                      </li>
                    ))}
                  </ol>
                  <p>Meta do produto: do evento à fila em menos de 2 s (p95).</p>
                </section>
                <button type="button" className="pov-app__primary" onClick={() => open(sent.backLabel === 'Voltar à conversa' ? 'message' : 'home')}>
                  {sent.backLabel}
                </button>
                <Link to="/fila">
                  Ver na fila do time
                  <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
                    <path d="M7 17L17 7M9 7h8v8" />
                  </svg>
                </Link>
              </section>
            ) : null}
          </aside>
        ) : null}
      </div>
      {!wide && !drawer ? <TabBar tab={tab} onPick={pickNav} /> : null}
    </main>
  );
}

/**
 * The simulation strip. "Raio-X SDUI" ("Raio-X" on the phone) toggles the
 * demo X-ray of the SDUI screen; without `onXray` (loading, the error state,
 * the purchase form, a tab with no SDUI screen) the toggle is not shown.
 * With `sim` it also shows the global simulated day and "Avançar um dia"
 * ("+1 dia" on the phone), with the error of the last advance under it.
 */
function Strip({
  name,
  wide,
  xray,
  onXray,
  sim,
}: {
  name?: string;
  wide: boolean;
  xray: boolean;
  onXray?: () => void;
  sim?: StripSimulation;
}) {
  return (
    <>
      <div className={sim ? 'pov-app__strip pov-app__strip--sim' : 'pov-app__strip'}>
        <span className="pov-app__mark" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span>
          <strong>Simulação</strong>
          {name ? ` · vendo como ${name}` : null}
        </span>
        {sim && sim.day !== null ? <span className="pov-app__day">Dia simulado {sim.day}</span> : null}
        {sim ? (
          <button
            type="button"
            className="pov-app__advance"
            title={wide ? undefined : 'Avançar um dia'}
            disabled={sim.busy}
            aria-busy={sim.busy}
            onClick={sim.onAdvance}
          >
            <span>{wide ? 'Avançar um dia' : '+1 dia'}</span>
          </button>
        ) : null}
        {onXray ? (
          <button type="button" className="pov-app__xray" aria-pressed={xray} onClick={onXray}>
            <span>{wide ? 'Raio-X SDUI' : 'Raio-X'}</span>
          </button>
        ) : null}
        <Link to="/client-pov">Trocar cliente</Link>
      </div>
      {sim?.notice ? (
        <p className="pov-app__strip-alert" role="alert">
          {sim.notice}
        </p>
      ) : null}
    </>
  );
}

interface StripSimulation {
  /** The global simulated day, or null while unknown. */
  day: number | null;
  busy: boolean;
  notice: string;
  onAdvance: () => void;
}

function TabBar({ tab, onPick }: { tab: TabSlug; onPick: (slug: TabSlug) => void }) {
  return (
    <nav className="pov-app__tabs" aria-label="Navegação principal">
      {TABS.map((item) => (
        <button key={item.slug} type="button" aria-current={item.slug === tab ? 'page' : undefined} onClick={() => onPick(item.slug)}>
          <Icon name={item.icon} size={22} /> {item.label}
        </button>
      ))}
    </nav>
  );
}

function PhoneBar({ light, themeLabel, onTheme, initials: mark }: { light: boolean; themeLabel: string; onTheme: () => void; initials: string }) {
  return (
    <div className="pov-app__phone-bar pov-app__phone-bar--sdui">
      <p className="pov-app__logo">
        orla<span>.</span>
      </p>
      <button type="button" aria-label={themeLabel} onClick={onTheme}>
        <Icon name={light ? 'moon' : 'sun'} size={20} />
      </button>
      <span aria-hidden="true">{mark}</span>
    </div>
  );
}

function BackIcon({ wide }: { wide: boolean }) {
  return (
    <svg width="22" height="22" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
      <path d={wide ? 'M6 6l12 12M18 6L6 18' : 'M15 6l-6 6 6 6'} />
    </svg>
  );
}

/** A panel's close control and title; the purchase views bring their own heading, so they pass no title. */
function PanelHead({ title, closeLabel, onClose }: { title?: string; closeLabel: string; onClose: () => void }) {
  return (
    <div className="pov-app__panel-head">
      <button type="button" aria-label={closeLabel} onClick={onClose}>
        <BackIcon wide={closeLabel === 'Fechar'} />
      </button>
      {title ? <h1>{title}</h1> : null}
    </div>
  );
}

function Note({ kicker, children }: { kicker: string; children: string }) {
  return (
    <div className="pov-app__note">
      <span>{kicker}</span>
      <p>{children}</p>
    </div>
  );
}

function MoneyPanel({
  kind,
  amount,
  cents,
  overCash,
  cash,
  aum,
  origin,
  destination,
  closeLabel,
  notice,
  onAmount,
  onOrigin,
  onDestination,
  onClose,
  onChip,
  onAll,
  onSubmit,
}: {
  kind: 'deposit' | 'withdraw';
  amount: string;
  cents: number;
  overCash: boolean;
  cash: string;
  aum: string;
  origin: string;
  destination: string;
  closeLabel: string;
  notice: string;
  onAmount: (value: string) => void;
  onOrigin: (value: string) => void;
  onDestination: (value: string) => void;
  onClose: () => void;
  onChip: (value: number) => void;
  onAll: () => void;
  onSubmit: () => void;
}) {
  const deposit = kind === 'deposit';
  const chips = deposit ? [1000, 10000, 50000] : [1000, 5000, 20000];
  const options: [string, string][] = deposit
    ? [
        ['Câmbio a partir do Brasil', 'Envie reais e receba em dólar'],
        ['Wire de outro banco', 'De uma conta sua nos EUA'],
      ]
    : [
        ['Conta nos EUA', 'Banco fictício · final 4821'],
        ['Conta no Brasil', 'Com câmbio para reais · final 0937'],
      ];
  const selected = deposit ? origin : destination;
  return (
    <form
      className="pov-app__form"
      onSubmit={(event) => {
        event.preventDefault();
        onSubmit();
      }}
    >
      <PanelHead title={deposit ? 'Depositar' : 'Sacar'} closeLabel={closeLabel} onClose={onClose} />
      <label className="pov-app__amount" htmlFor={deposit ? 'aporte-valor' : 'saque-valor'}>
        {deposit ? 'Quanto você quer depositar?' : 'Quanto você quer sacar?'}
        <span data-over={overCash ? 'true' : 'false'}>
          <em>US$</em>
          <input
            id={deposit ? 'aporte-valor' : 'saque-valor'}
            inputMode="decimal"
            autoComplete="off"
            placeholder="0,00"
            value={amount}
            onChange={(event) => onAmount(event.target.value)}
          />
        </span>
      </label>
      {deposit ? <p className="pov-app__hint">Patrimônio atual: {aum}</p> : null}
      {!deposit && overCash ? (
        <p role="alert">Valor acima do disponível para saque ({cash}).</p>
      ) : null}
      {!deposit && !overCash ? <p className="pov-app__hint">Disponível: {cash}</p> : null}
      <div className="pov-app__chips">
        {chips.map((value) => (
          <button key={value} type="button" onClick={() => onChip(value)}>
            US$ {money(value).replace(',00', '')}
          </button>
        ))}
        {deposit ? null : (
          <button type="button" onClick={onAll}>
            Tudo
          </button>
        )}
      </div>
      <p className="pov-app__hint">{deposit ? 'De onde vem o dinheiro' : 'Para onde'}</p>
      <div className="pov-app__options">
        {options.map(([title, sub]) => (
          <button
            key={title}
            type="button"
            className={selected === title ? 'pov-app__option pov-app__option--on' : 'pov-app__option'}
            aria-pressed={selected === title}
            onClick={() => (deposit ? onOrigin(title) : onDestination(title))}
          >
            <i />
            <span>
              <strong>{title}</strong>
              <em>{sub}</em>
            </span>
          </button>
        ))}
      </div>
      <Note kicker={deposit ? 'simulação → account.event.recorded · aporte' : 'simulação → account.event.recorded · saque'}>
        {deposit
          ? 'Pode gerar alerta de aporte grande e, se o patrimônio cruzar a faixa, de mudança de segmento.'
          : 'O advisory decide se vira alerta de saque relevante ou de queda de patrimônio. O app não calcula regra nenhuma.'}
      </Note>
      {notice ? <p role="alert">{notice}</p> : null}
      <button type="submit" className="pov-app__primary" disabled={cents <= 0 || overCash}>
        {deposit ? 'Confirmar depósito' : 'Confirmar saque'}
      </button>
    </form>
  );
}

