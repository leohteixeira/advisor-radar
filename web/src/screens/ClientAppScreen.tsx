import { useEffect, useState } from 'react';
import { Link, Navigate, useNavigate, useParams } from 'react-router-dom';
import { apiPath } from '../api/base';
import { ApiError } from '../api/bff';
import { fetchPOVHome, formatCents, postPOV, protocolOf, type POVHome } from '../api/pov';
import { fetchScreen } from '../sdui/api';
import { SduiContext, type SduiContextValue } from '../sdui/context';
import { SduiLoading, SduiScreen } from '../sdui/SduiScreen';
import type { Screen } from '../sdui/types';

interface LiveStep {
  id: string;
  label: string;
  state: string;
}

interface Sent {
  event_id: string;
  title: string;
  lead: string;
  routing: string;
  detail: string;
  backLabel: string;
}

interface ActivityRow {
  title: string;
  meta: string;
  value: string;
  tone: '' | 'pos' | 'neg';
  icon: string;
}

interface ChatRow {
  text: string;
  meta: string;
}

type Panel = 'home' | 'deposit' | 'withdraw' | 'complaint' | 'message' | 'done';

/**
 * The SDUI home, or the phase-2 home when the screen request fails or answers
 * something that is not an envelope. The fallback is removed in story 10.
 */
type HomeScreen = { status: 'loading' } | { status: 'ready'; screen: Screen } | { status: 'fallback' };

/** Client app tabs. Only Início has a screen yet; the others show a note. */
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
  in: 'M21 12a9 9 0 1 1-18 0 9 9 0 0 1 18 0zM12 8v8M8 12h8',
  out: 'M3 21h18M4 10h16M12 3l8 5H4zM6 10v8M10 10v8M14 10v8M18 10v8',
  msg: 'M21 12a8 8 0 0 1-11.6 7.1L4 20l1-4.6A8 8 0 1 1 21 12z',
  seg: 'M12 3l2.6 5.6 6.1.7-4.5 4.2 1.2 6L12 16.6 6.6 19.5l1.2-6-4.5-4.2 6.1-.7z',
  flag: 'M5 21V4h11l-1.5 4L16 12H5',
  home: 'M3 11l9-7 9 7v9a1 1 0 0 1-1 1h-5v-6H9v6H4a1 1 0 0 1-1-1z',
  invest: 'M3 17l6-6 4 4 8-8M15 7h6v6',
  wallet: 'M21 12A9 9 0 1 1 12 3v9zM15 3.5A9 9 0 0 1 20.5 9H15z',
  user: 'M16 21v-1a4 4 0 0 0-4-4H8a4 4 0 0 0-4 4v1M12 11a4 4 0 1 0 0-8 4 4 0 0 0 0 8z',
  bell: 'M6 8a6 6 0 0 1 12 0c0 7 3 9 3 9H3s3-2 3-9M10.3 21a1.94 1.94 0 0 0 3.4 0',
  sun: 'M12 12m-4 0a4 4 0 1 0 8 0a4 4 0 1 0 -8 0M12 2v2M12 20v2M4.9 4.9l1.4 1.4M17.7 17.7l1.4 1.4M2 12h2M20 12h2M4.9 19.1l1.4-1.4M17.7 6.3l1.4-1.4',
  moon: 'M21 12.8A9 9 0 1 1 11.2 3a7 7 0 0 0 9.8 9.8z',
  eye: 'M2 12s3.5-7 10-7 10 7 10 7-3.5 7-10 7S2 12 2 12zM12 12m-3 0a3 3 0 1 0 6 0a3 3 0 1 0 -6 0',
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

function dollars(value: string): number {
  const parsed = Number(value.replace(/[^0-9.,]/g, '').replace(/\./g, '').replace(',', '.'));
  return Number.isFinite(parsed) ? parsed : 0;
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

function waiting(labels: string[]): LiveStep[] {
  return labels.map((label) => ({ id: label, label, state: 'aguardando' }));
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
  const [homeScreen, setHomeScreen] = useState<HomeScreen>({ status: 'loading' });
  const navNote = tab === 'home' ? null : (TABS.find((item) => item.slug === tab)?.label ?? null);

  useEffect(() => {
    if (panel !== 'done' || !sent) {
      return;
    }
    const source = new EventSource(apiPath(`v1/client-pov/customers/${id}/stream`));
    const onStep = (ev: Event) => {
      const data = JSON.parse((ev as MessageEvent<string>).data) as { event_id: string; steps: LiveStep[] };
      if (data.event_id === sent.event_id) {
        setSteps(data.steps);
      }
    };
    source.addEventListener('bastidores', onStep);
    return () => {
      source.removeEventListener('bastidores', onStep);
      source.close();
    };
  }, [panel, sent, id]);

  useEffect(() => {
    if (panel !== 'home') {
      return;
    }
    let gone = false;
    const abort = new AbortController();
    // A timeout (SCREEN_TIMEOUT_MS) rejects like any other failure: fallback.
    fetchScreen(id, 'home', abort.signal)
      .then((screen) => {
        if (!gone) {
          setHomeScreen({ status: 'ready', screen });
        }
      })
      .catch(() => {
        // Phase-2 fallback, removed in story 10: a failed or non-envelope
        // screen response renders the home from GET /customers/{id}.
        if (!gone) {
          setHomeScreen({ status: 'fallback' });
        }
      });
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
      abort.abort();
    };
  }, [id, panel]);

  const cents = Math.round(dollars(amount) * 100);
  const overCash = panel === 'withdraw' && home != null && cents > home.caixa;

  function open(next: Panel) {
    setAmount('');
    setNotice('');
    setText('');
    setPreset(-1);
    setPanel(next);
  }

  function toggleTheme() {
    setLight((value) => {
      const next = !value;
      storeLightTheme(next);
      return next;
    });
  }

  function pickNav(slug: TabSlug) {
    open('home');
    navigate(clientPath(id, slug));
  }

  async function send(
    kind: 'deposits' | 'withdrawals' | 'complaints' | 'messages',
    body: Record<string, unknown>,
    next: Omit<Sent, 'event_id'>,
    labels: string[],
  ) {
    setNotice('');
    try {
      const accepted = await postPOV(id, kind, body, crypto.randomUUID());
      setSent({ ...next, event_id: accepted.event_id });
      const initial = waiting(labels);
      const first = initial[0];
      if (first) {
        initial[0] = { id: first.id, label: first.label, state: 'feito' };
      }
      setSteps(initial);
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
        setNotice('Muitas ações em pouco tempo. Espere um minuto e tente de novo.');
        return;
      }
      setNotice('Não foi possível registrar a ação.');
    }
  }

  if (error) {
    return (
      <main className="pov-app">
        <p role="alert">Não foi possível abrir este cliente.</p>
        <Link to="/client-pov">Voltar à lista</Link>
      </main>
    );
  }
  const themeLabel = light ? 'Tema escuro' : 'Tema claro';
  if (!home || homeScreen.status === 'loading') {
    // One skeleton until both the first screen request and the phase-2 shell
    // data settle, so the first paint is already SDUI or the fallback. Each
    // re-fetch on returning to Início keeps the current home until it settles,
    // then shows the new screen, or switches to the fallback if it failed.
    return (
      <main className={light ? 'pov-app pov-app--light' : 'pov-app'} data-layout={wide ? 'desktop' : 'phone'}>
        <div className="pov-app__strip">
          <span className="pov-app__mark" aria-hidden="true">
            <i />
            <i />
            <i />
          </span>
          <span>
            <strong>Simulação</strong>
            {home ? ` · vendo como ${home.name}` : null}
          </span>
          <Link to="/client-pov">Trocar cliente</Link>
        </div>
        <div className="pov-app__body">
          <div className="pov-app__main">
            {wide ? null : <PhoneBar light={light} themeLabel={themeLabel} onTheme={toggleTheme} initials={home ? initials(home.name) : ''} />}
            <div className="sdui-main">
              <SduiLoading />
            </div>
          </div>
        </div>
        {wide ? null : <TabBar tab={tab} onPick={pickNav} />}
      </main>
    );
  }

  const activity = home.activity;
  const chat = home.messages.concat(extraChat);
  const first = home.name.split(' ')[0] ?? home.name;
  const showHome = wide || panel === 'home';
  const closeLabel = wide ? 'Fechar' : 'Voltar';
  const total = home.assets || 1;
  const alloc = [
    ['Ações', home.allocation.acoes, 0],
    ['ETFs', home.allocation.etfs, 1],
    ['Renda fixa', home.allocation.renda_fixa, 2],
    ['Caixa', home.allocation.caixa, 3],
  ]
    .map(([label, value, slot]) => ({
      label: String(label),
      slot: Number(slot),
      pct: Math.round((Number(value) / total) * 100),
    }))
    .filter((row) => row.pct > 0);
  const complaintText = preset >= 0 ? PRESETS[preset]?.text ?? '' : text.trim();
  const aumText = hide ? 'US$ ••••••' : formatCents(home.assets);
  const sdui: SduiContextValue = {
    onNavigate: pickNav,
    masked: hide,
    onToggleMask: () => setHide((value) => !value),
    onPanel: open,
  };

  return (
    <main className={light ? 'pov-app pov-app--light' : 'pov-app'} data-layout={wide ? 'desktop' : 'phone'}>
      <div className="pov-app__strip">
        <span className="pov-app__mark" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <span>
          <strong>Simulação</strong> · vendo como {home.name}
        </span>
        <Link to="/client-pov">Trocar cliente</Link>
      </div>
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
        {showHome && homeScreen.status === 'ready' ? (
          <div className="pov-app__main">
            {wide ? null : <PhoneBar light={light} themeLabel={themeLabel} onTheme={toggleTheme} initials={initials(home.name)} />}
            <div className="sdui-main">
              <NavNote name={navNote} />
              <SduiContext value={sdui}>
                <SduiScreen screen={homeScreen.screen} />
              </SduiContext>
            </div>
            <p className="pov-app__fine">Orla Invest é uma corretora fictícia criada para a demo do Advisor Radar.</p>
          </div>
        ) : null}
        {showHome && homeScreen.status === 'fallback' ? (
          <div className="pov-app__main">
            {wide ? (
              <HomeDesktop
                first={first}
                since={home.since}
                segment={home.segment}
                aumText={aumText}
                cash={formatCents(home.caixa)}
                hide={hide}
                onHide={() => setHide((value) => !value)}
                alloc={alloc}
                advisor={home.advisor}
                initials={initials(home.name)}
                activity={activity}
                last={chat.at(-1)}
                note={navNote}
                onOpen={open}
              />
            ) : (
              <HomePhone
                first={first}
                since={home.since}
                segment={home.segment}
                aumText={aumText}
                cash={formatCents(home.caixa)}
                hide={hide}
                themeLabel={themeLabel}
                onTheme={toggleTheme}
                onHide={() => setHide((value) => !value)}
                alloc={alloc}
                advisor={home.advisor}
                initials={initials(home.name)}
                activity={activity}
                note={navNote}
                onOpen={open}
              />
            )}
          </div>
        ) : null}
        {panel !== 'home' && wide ? (
          <button type="button" className="pov-app__scrim" aria-label="Fechar painel" onClick={() => open('home')} />
        ) : null}
        {panel !== 'home' ? (
          <aside className="pov-app__panel" aria-label="Ação">
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
      {!wide && panel === 'home' ? <TabBar tab={tab} onPick={pickNav} /> : null}
    </main>
  );
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

function PanelHead({ title, closeLabel, onClose }: { title: string; closeLabel: string; onClose: () => void }) {
  return (
    <div className="pov-app__panel-head">
      <button type="button" aria-label={closeLabel} onClick={onClose}>
        <BackIcon wide={closeLabel === 'Fechar'} />
      </button>
      <h1>{title}</h1>
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

function HomePhone({
  first,
  since,
  segment,
  aumText,
  cash,
  hide,
  themeLabel,
  onTheme,
  onHide,
  alloc,
  advisor,
  initials: mark,
  activity,
  note,
  onOpen,
}: {
  first: string;
  since: string;
  segment: string;
  aumText: string;
  cash: string;
  hide: boolean;
  themeLabel: string;
  onTheme: () => void;
  onHide: () => void;
  alloc: { label: string; slot: number; pct: number }[];
  advisor: string;
  initials: string;
  activity: ActivityRow[];
  note: string | null;
  onOpen: (panel: Panel) => void;
}) {
  return (
    <>
      <NavNote name={note} />
      <div className="pov-app__phone-bar">
        <p className="pov-app__logo">
          orla<span>.</span>
          <em>invest</em>
        </p>
        <button type="button" aria-label={themeLabel} onClick={onTheme}>
          <Icon name={themeLabel === 'Tema claro' ? 'sun' : 'moon'} size={19} />
        </button>
        <button type="button" aria-label="Notificações">
          <Icon name="bell" />
        </button>
        <span aria-hidden="true">{mark}</span>
      </div>
      <div className="pov-app__hello">
        <h1>Olá, {first}</h1>
        <span>Conta em dólar nos EUA{since ? ` · cliente desde ${since}` : ''}</span>
      </div>
      <Wealth aumText={aumText} segment={segment} cash={cash} hide={hide} onHide={onHide} alloc={alloc} wide={false} />
      <Actions onOpen={onOpen} wide={false} />
      <section className="pov-app__advisor" aria-label="Sua assessoria">
        <span aria-hidden="true">AP</span>
        <div>
          <em>Sua assessora</em>
          <strong>{advisor}</strong>
        </div>
        <button type="button" onClick={() => onOpen('message')}>
          Conversar
        </button>
      </section>
      <Activity activity={activity} wide={false} />
      <p className="pov-app__fine">Orla Invest é uma corretora fictícia criada para a demo do Advisor Radar.</p>
    </>
  );
}

function HomeDesktop({
  first,
  since,
  segment,
  aumText,
  cash,
  hide,
  onHide,
  alloc,
  advisor,
  activity,
  last,
  note,
  onOpen,
}: {
  first: string;
  since: string;
  segment: string;
  aumText: string;
  cash: string;
  hide: boolean;
  onHide: () => void;
  alloc: { label: string; slot: number; pct: number }[];
  advisor: string;
  initials: string;
  activity: ActivityRow[];
  last?: ChatRow;
  note: string | null;
  onOpen: (panel: Panel) => void;
}) {
  return (
    <div className="pov-app__desk">
      <NavNote name={note} />
      <div className="pov-app__desk-top">
        <div className="pov-app__hello">
          <h1>Olá, {first}</h1>
          <span>Conta em dólar nos EUA{since ? ` · cliente desde ${since}` : ''}</span>
        </div>
        <Actions onOpen={onOpen} wide />
        <button type="button" aria-label="Notificações">
          <Icon name="bell" />
        </button>
      </div>
      <div className="pov-app__desk-grid">
        <Wealth aumText={aumText} segment={segment} cash={cash} hide={hide} onHide={onHide} alloc={alloc} wide />
        <div>
          <section className="pov-app__advisor" aria-label="Sua assessoria">
            <div>
              <span aria-hidden="true">AP</span>
              <div>
                <em>Sua assessora</em>
                <strong>{advisor}</strong>
              </div>
            </div>
            {last ? (
              <blockquote>
                <em>Sua última mensagem · {last.meta}</em>
                {last.text}
              </blockquote>
            ) : null}
            <div>
              <button type="button" onClick={() => onOpen('message')}>
                Conversar
              </button>
              <button type="button" onClick={() => onOpen('complaint')}>
                Abrir reclamação
              </button>
            </div>
          </section>
          <p className="pov-app__fine">Orla Invest é uma corretora fictícia criada para a demo do Advisor Radar.</p>
        </div>
      </div>
      <Activity activity={activity} wide />
    </div>
  );
}

function NavNote({ name }: { name: string | null }) {
  if (!name) {
    return null;
  }
  return (
    <p role="status" className="pov-app__nav-note">
      {name} não entra nesta simulação. O caminho é Depositar, Sacar, Mensagem ou Reclamar.
    </p>
  );
}

function Wealth({
  aumText,
  segment,
  cash,
  hide,
  onHide,
  alloc,
  wide,
}: {
  aumText: string;
  segment: string;
  cash: string;
  hide: boolean;
  onHide: () => void;
  alloc: { label: string; slot: number; pct: number }[];
  wide: boolean;
}) {
  return (
    <section className="pov-app__card" aria-label="Patrimônio" data-wide={wide ? 'true' : 'false'}>
      <div>
        <span>Patrimônio total</span>
        <em>Cliente {segment}</em>
        <button type="button" aria-label={hide ? 'Mostrar valores' : 'Esconder valores'} onClick={onHide}>
          <Icon name="eye" />
        </button>
      </div>
      <strong>{aumText}</strong>
      <div className="pov-app__bar" aria-hidden="true">
        {alloc.map((row) => (
          <i key={row.label} data-slot={row.slot} style={{ flex: row.pct }} />
        ))}
      </div>
      <ul>
        {alloc.map((row) => (
          <li key={row.label}>
            <i data-slot={row.slot} />
            <span>{row.label}</span>
            <b>{row.pct}%</b>
          </li>
        ))}
      </ul>
      <p>
        <span>Disponível para saque</span>
        <b>{cash}</b>
      </p>
    </section>
  );
}

function Actions({ onOpen, wide }: { onOpen: (panel: Panel) => void; wide: boolean }) {
  const items: { label: string; icon: string; panel: Panel }[] = [
    { label: 'Depositar', icon: 'in', panel: 'deposit' },
    { label: 'Sacar', icon: 'out', panel: 'withdraw' },
    { label: 'Mensagem', icon: 'msg', panel: 'message' },
    { label: 'Reclamar', icon: 'flag', panel: 'complaint' },
  ];
  return (
    <nav className={wide ? 'pov-app__pills' : 'pov-app__actions'} aria-label="Ações">
      {items.map((item, index) => (
        <button key={item.label} type="button" data-main={index === 0 ? 'true' : 'false'} onClick={() => onOpen(item.panel)}>
          <Icon name={item.icon} size={wide ? 18 : 22} />
          {item.label}
        </button>
      ))}
    </nav>
  );
}

function Activity({ activity, wide }: { activity: ActivityRow[]; wide: boolean }) {
  return (
    <section className="pov-app__activity" aria-label="Atividade recente" data-wide={wide ? 'true' : 'false'}>
      <h2>Atividade recente</h2>
      {activity.length === 0 ? <p>Nenhuma movimentação nos últimos 30 dias.</p> : null}
      {activity.map((row) => (
        <div key={`${row.title}-${row.meta}`}>
          <span aria-hidden="true">
            <Icon name={row.icon} size={18} />
          </span>
          <div>
            <strong>{row.title}</strong>
            <em>{row.meta}</em>
          </div>
          <b data-tone={row.tone}>{row.value}</b>
        </div>
      ))}
    </section>
  );
}
