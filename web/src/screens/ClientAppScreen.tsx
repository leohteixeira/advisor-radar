import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { apiPath } from '../api/base';
import { ApiError } from '../api/bff';
import { fetchPOVHome, formatCents, postPOV, protocolOf, type POVHome } from '../api/pov';

interface LiveStep {
  id: string;
  label: string;
  state: string;
}

const PRESET = 'Estou pensando em sair';
const PRESET_TEXT =
  'Se isso não for resolvido hoje vou levar meu dinheiro todo para outra corretora.';

type Panel = 'home' | 'deposit' | 'withdraw' | 'complaint' | 'message' | 'done';

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

export function ClientAppScreen() {
  const { id = '' } = useParams();
  const wide = useWide();
  const [light, setLight] = useState(false);
  const [channel, setChannel] = useState<'chat' | 'e-mail'>('chat');
  const [text, setText] = useState('');
  const [home, setHome] = useState<POVHome | null>(null);
  const [error, setError] = useState(false);
  const [panel, setPanel] = useState<Panel>('home');
  const [amount, setAmount] = useState('');
  const [notice, setNotice] = useState('');
  const [result, setResult] = useState<{ event_id: string; title: string } | null>(null);
  const [steps, setSteps] = useState<LiveStep[]>([]);

  useEffect(() => {
    if (panel !== 'done' || !result) {
      return;
    }
    const source = new EventSource(apiPath(`v1/client-pov/customers/${id}/stream`));
    const onStep = (ev: Event) => {
      const data = JSON.parse((ev as MessageEvent<string>).data) as { event_id: string; steps: LiveStep[] };
      if (data.event_id === result.event_id) {
        setSteps(data.steps);
      }
    };
    source.addEventListener('bastidores', onStep);
    return () => {
      source.removeEventListener('bastidores', onStep);
      source.close();
    };
  }, [panel, result, id]);

  useEffect(() => {
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
  }, [id]);

  const cents = Math.round(Number(amount.replace(',', '.')) * 100);
  const overCash = panel === 'withdraw' && home != null && cents > home.caixa;

  async function send(
    kind: 'deposits' | 'withdrawals' | 'complaints' | 'messages',
    body: Record<string, unknown>,
    title: string,
  ) {
    setNotice('');
    try {
      const accepted = await postPOV(id, kind, body, crypto.randomUUID());
      setResult({ event_id: accepted.event_id, title });
      setPanel('done');
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
  if (!home) {
    return (
      <main className="pov-app">
        <p>Carregando cliente…</p>
      </main>
    );
  }

  const closeLabel = wide ? 'Fechar' : 'Voltar';
  const showHome = wide || panel === 'home' || panel === 'done';

  return (
    <main className={light ? 'pov-app pov-app--light' : 'pov-app'} data-layout={wide ? 'desktop' : 'phone'}>
      <p className="pov-app__strip">
        Simulação · vendo como {home.name}
        <button type="button" onClick={() => setLight((value) => !value)}>
          {light ? 'Tema escuro' : 'Tema claro'}
        </button>
      </p>
      {showHome ? (
        <>
          <p className="pov-app__brand">orla. invest</p>
          <h1>Olá, {home.name.split(' ')[0]}</h1>
          <p>{formatCents(home.assets)}</p>
          <p>{home.segment}</p>
          <p>Disponível para saque {formatCents(home.caixa)}</p>
          <p>Ana Paula Ribeiro</p>
          {home.activity.length === 0 ? <p>Nenhuma movimentação nos últimos 30 dias.</p> : null}
          <button type="button" onClick={() => setPanel('deposit')}>
            Depositar
          </button>
          <button type="button" onClick={() => setPanel('withdraw')}>
            Sacar
          </button>
          <button type="button" onClick={() => setPanel('complaint')}>
            Reclamar
          </button>
          <button type="button" onClick={() => setPanel('message')}>
            Mensagem
          </button>
        </>
      ) : null}
      {panel !== 'home' && wide ? (
        <button type="button" className="pov-app__scrim" aria-label="Fechar painel" onClick={() => setPanel('home')} />
      ) : null}
      {panel !== 'home' ? (
        <aside className="pov-app__panel" aria-label="Ação">
      {panel === 'deposit' || panel === 'withdraw' ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            if (panel === 'deposit') {
              void send('deposits', { amount: cents, origin: 'Câmbio a partir do Brasil' }, 'Depósito solicitado');
              return;
            }
            void send('withdrawals', { amount: cents, destination: 'Conta nos EUA' }, 'Saque solicitado');
          }}
        >
          <h1>{panel === 'deposit' ? 'Depositar' : 'Sacar'}</h1>
          <button type="button" onClick={() => setPanel('home')}>
            {closeLabel}
          </button>
          <label>
            Valor em USD
            <input value={amount} onChange={(event) => setAmount(event.target.value)} inputMode="decimal" />
          </label>
          {panel === 'deposit' ? (
            <button type="button" onClick={() => setAmount('10000')}>
              US$ 10.000
            </button>
          ) : (
            <button type="button" onClick={() => setAmount(String(home.caixa / 100))}>
              Tudo
            </button>
          )}
          {overCash ? <p role="alert">Valor acima do disponível para saque</p> : null}
          {notice ? <p role="alert">{notice}</p> : null}
          <button type="submit" disabled={cents <= 0 || overCash}>
            Confirmar
          </button>
        </form>
      ) : null}
      {panel === 'complaint' ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void send('complaints', { text: PRESET_TEXT }, 'Reclamação registrada');
          }}
        >
          <h1>Reclamar</h1>
          <button type="button" onClick={() => setPanel('home')}>
            {closeLabel}
          </button>
          <button type="submit">{PRESET}</button>
          {notice ? <p role="alert">{notice}</p> : null}
        </form>
      ) : null}
      {panel === 'message' ? (
        <form
          onSubmit={(event) => {
            event.preventDefault();
            void send('messages', { channel, text }, 'Mensagem enviada');
          }}
        >
          <h1>Mensagem</h1>
          <button type="button" onClick={() => setPanel('home')}>
            {closeLabel}
          </button>
          <p>Ainda não há mensagens. Escreva a primeira.</p>
          <button type="button" onClick={() => setChannel('chat')}>
            Chat
          </button>
          <button type="button" onClick={() => setChannel('e-mail')}>
            E-mail
          </button>
          <label>
            Texto
            <textarea value={text} onChange={(event) => setText(event.target.value)} />
          </label>
          {notice ? <p role="alert">{notice}</p> : null}
          <button type="submit" disabled={text.trim() === ''}>
            Enviar
          </button>
        </form>
      ) : null}
      {panel === 'done' && result ? (
        <section>
          <h1>{result.title}</h1>
          <p>Protocolo {protocolOf(result.event_id)}</p>
          <p>{result.event_id}</p>
          <ol aria-label="Bastidores">
            {steps.map((step) => (
              <li key={step.id}>
                {step.label} {step.state}
              </li>
            ))}
          </ol>
          <Link to="/fila">Ver na fila do time</Link>
          <button type="button" onClick={() => setPanel('home')}>
            Voltar ao início
          </button>
        </section>
      ) : null}
        </aside>
      ) : null}
      <footer>Orla Invest é uma corretora fictícia criada para a demo do Advisor Radar.</footer>
    </main>
  );
}
