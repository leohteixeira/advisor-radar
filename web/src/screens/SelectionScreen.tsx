import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { SduiShowcase } from '../selection/SduiShowcase';
import { WALK_INTERVAL_MS, WALK_STEPS, walkStep } from '../selection/steps';

const EDGES: Record<string, string> = {
  webbff: 'M188 226 L250 226',
  bffweb: 'M250 258 L188 258',
  bffacct: 'M438 226 L470 226 L470 80 L500 80',
  acctbus: 'M688 80 L730 80',
  bustriage: 'M842 60 L880 60',
  triagejev: 'M1068 60 L1084 60',
  busadv: 'M842 180 L880 180',
  buscases: 'M842 300 L880 300',
  busidx: 'M842 420 L880 420',
  idxes: 'M1068 420 L1084 420',
  busbff: 'M730 400 L344 400 L344 284',
};

const WIDE_QUERY = '(min-width: 900px)';
const REDUCE_QUERY = '(prefers-reduced-motion: reduce)';

function useMedia(query: string): boolean {
  const [matches, setMatches] = useState(() => window.matchMedia(query).matches);

  useEffect(() => {
    const media = window.matchMedia(query);
    const onChange = () => setMatches(media.matches);
    onChange();
    media.addEventListener('change', onChange);
    return () => media.removeEventListener('change', onChange);
  }, [query]);

  return matches;
}

export function SelectionScreen() {
  const wide = useMedia(WIDE_QUERY);
  const reduced = useMedia(REDUCE_QUERY);
  const [step, setStep] = useState(0);
  const [paused, setPaused] = useState(reduced);
  const current = walkStep(step);

  useEffect(() => {
    if (reduced) {
      setPaused(true);
    }
  }, [reduced]);

  useEffect(() => {
    if (paused || reduced) {
      return;
    }
    const timer = window.setInterval(() => {
      setStep((index) => (index + 1) % WALK_STEPS.length);
    }, WALK_INTERVAL_MS);
    return () => window.clearInterval(timer);
  }, [paused, reduced]);

  function togglePlayback() {
    if (reduced) {
      return;
    }
    setPaused((value) => !value);
  }

  const pauseLabel = paused ? 'Reproduzir' : 'Pausar';

  return (
    <main className={wide ? 'selection selection--wide' : 'selection'}>
      <header className="selection__bar">
        <span className="selection__brand">Advisor Radar</span>
        <nav aria-label="Nesta página" className="selection__anchors">
          <a href="#arquitetura">Arquitetura</a>
          <a href="#sdui" className="selection__anchor--new">
            {wide ? 'Server-Driven UI' : 'SDUI'}
            <span className="selection__new">Novo</span>
          </a>
        </nav>
        {wide ? <span className="selection__demo">Demo técnica · dados fictícios</span> : null}
      </header>

      <section className="selection__hero" aria-labelledby="selection-headline">
        <p className="selection__eyebrow">Escolha uma visão</p>
        <h1 id="selection-headline">O mesmo sinal, visto dos dois lados da conversa.</h1>
        {wide ? (
          <p className="selection__lede">
            Entre como assessoria para trabalhar a fila, ou como cliente para provocar os eventos que
            chegam nela.
          </p>
        ) : null}
      </section>

      <section className="selection__cards" aria-label="Visões">
        <Link className="selection-card" to="/fila">
          <p className="selection-card__eyebrow">Assessor · Analista · Gestor</p>
          <h2>Visão do time</h2>
          <p>
            {wide
              ? 'A fila do assessor em tempo real, com alertas de conta e mensagens triadas em ordem de prioridade. Inclui a revisão das classificações incertas e o painel da gestão com os SLAs em risco.'
              : 'Fila em tempo real com alertas e mensagens priorizadas, revisão da triagem e o painel com os SLAs em risco.'}
          </p>
          {wide ? (
            <ul className="selection-card__chips">
              <li>Fila do assessor</li>
              <li>Revisão de triagem</li>
              <li>Painel da assessoria</li>
              <li>Visão 360 do cliente</li>
            </ul>
          ) : null}
          {wide ? <span className="selection-card__cta selection-card__cta--ink">Entrar como time</span> : null}
        </Link>

        <Link className="selection-card" to="/client-pov">
          <p className="selection-card__eyebrow">
            {wide ? 'Cliente fictício · um por segmento' : 'Um cliente fictício por segmento'}
          </p>
          <h2>Visão do cliente</h2>
          <p>
            {wide
              ? 'Entre no app de uma corretora fictícia como um cliente Essencial, Advance ou Singular. Saques, depósitos, mensagens e reclamações viram eventos reais no RabbitMQ e aparecem na fila do time.'
              : 'Use o app de uma corretora fictícia. Saques, depósitos, mensagens e reclamações viram eventos e chegam à fila do time.'}
          </p>
          {wide ? (
            <ul className="selection-card__chips">
              <li>Saque e wire-out</li>
              <li>Depósito</li>
              <li>Mensagem livre</li>
              <li>Reclamações prontas</li>
            </ul>
          ) : null}
          {wide ? (
            <span className="selection-card__cta selection-card__cta--green">Escolher cliente</span>
          ) : null}
        </Link>
      </section>

      <section className="selection__band" aria-labelledby="selection-band">
        <span className="selection__band-mark" aria-hidden="true">
          <i />
          <i />
          <i />
        </span>
        <div>
          <p className="selection__band-kicker">Novo nesta versão · Server-Driven UI</p>
          <h2 id="selection-band">Três clientes, três inícios diferentes, nenhuma linha de front escrita para cada um.</h2>
          <p>O bff devolve a tela como página, seções e componentes, já preenchida para o momento do cliente. O app só renderiza.</p>
        </div>
        <a href="#sdui" className="selection__band-cta">
          Ver como funciona
          <svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round" aria-hidden="true">
            <path d="M12 5v14M6 13l6 6 6-6" />
          </svg>
        </a>
      </section>

      <section className="selection__product" aria-labelledby="selection-product">
        <div className="selection__product-intro">
          <p className="selection__eyebrow">O produto</p>
          <h2 id="selection-product">Quem precisa de atenção agora, e por quê.</h2>
          <p>
            O Advisor Radar junta o que acontece na conta do cliente e o que ele escreve, e transforma
            isso em ações priorizadas para a assessoria.
          </p>
        </div>
        <div className="selection__pillars">
          <article>
            <h3>Alertas proativos</h3>
            <p>
              {wide
                ? 'Regras determinísticas sobre eventos de conta: saque relevante, queda de patrimônio, aporte grande, mudança de segmento, cliente sem contato e compra acima do perfil.'
                : 'Saque relevante, queda de patrimônio, aporte grande, mudança de segmento, cliente sem contato e compra acima do perfil.'}
            </p>
          </article>
          <article>
            <h3>Triagem de mensagens</h3>
            <p>
              {wide
                ? 'Intenção, frustração, risco de saída e pedido de atendimento humano. Abaixo de 0,85 de certeza, uma pessoa revisa. Se o modelo cair, a heurística assume e o resultado sai marcado.'
                : 'Intenção, frustração, risco de saída e pedido humano. Abaixo de 0,85 de certeza, uma pessoa revisa.'}
            </p>
          </article>
          <article>
            <h3>Casos com SLA</h3>
            <p>
              {wide
                ? 'Aberto, em atendimento, aguardando cliente, resolvido. O SLA depende do segmento e escala sozinho no vencimento, por fila com TTL, sem cron.'
                : 'SLA por segmento, com escalonamento por fila com TTL, sem cron.'}
            </p>
          </article>
          <article className="selection__pillar--new">
            <h3>Telas pelo servidor</h3>
            <p>
              O app do cliente é server-driven. O backend escolhe a variante de cada seção pelo momento
              do cliente, e o front só desenha um catálogo fixo de componentes.
            </p>
          </article>
        </div>
      </section>

      <section id="arquitetura" className="selection__walk" aria-labelledby="selection-walk">
        <div className="selection__walk-head">
          <div>
            <p className="selection__eyebrow">Arquitetura</p>
            <h2 id="selection-walk">Como uma reclamação chega à fila do assessor</h2>
          </div>
          <div className="selection__dots" role="group" aria-label="Passos">
            {WALK_STEPS.map((item, index) => (
              <button
                key={item.title}
                type="button"
                aria-label={`Passo ${index + 1}: ${item.title}`}
                aria-current={index === step ? 'step' : undefined}
                onClick={() => {
                  setStep(index);
                  setPaused(true);
                }}
              />
            ))}
          </div>
          <button
            type="button"
            className="selection__pause"
            aria-label={pauseLabel}
            aria-pressed={paused}
            onClick={togglePlayback}
          >
            {paused ? (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d="M7 4l13 8-13 8z" />
              </svg>
            ) : (
              <svg width="14" height="14" viewBox="0 0 24 24" fill="currentColor" aria-hidden="true">
                <path d="M6 4h4v16H6zM14 4h4v16h-4z" />
              </svg>
            )}
            {pauseLabel}
          </button>
        </div>

        <div className="selection__board">
            <div className="selection__fit">
            <div className="selection__sheet">
            <div className="selection__stage">
              <svg viewBox="0 0 1180 500" aria-hidden="true">
                {Object.entries(EDGES).map(([id, d]) => (
                  <path key={id} className={current.edges.includes(id) ? 'edge on' : 'edge'} d={d} />
                ))}
              </svg>
              <span className="selection__elabel" style={{ left: 196, top: 206 }}>
                HTTP
              </span>
              <span className="selection__elabel" style={{ left: 200, top: 264 }}>
                SSE
              </span>
              <span className="selection__elabel" style={{ left: 420, top: 140 }}>
                gRPC
              </span>
              <span className="selection__elabel" style={{ left: 470, top: 380 }}>
                alert.raised · case.*
              </span>
              <article className={current.graph.includes('web') ? 'selection__node on' : 'selection__node'} style={{ left: 0, top: 200 }}>
                <span>web</span>
                <strong>App e fila</strong>
                <small>React · TypeScript · Vite</small>
              </article>
              <article className={current.graph.includes('bff') ? 'selection__node on' : 'selection__node'} style={{ left: 250, top: 200 }}>
                <span>bff</span>
                <strong>Go · HTTP · SSE</strong>
                <small>Agrega via gRPC · sem banco</small>
              </article>
              <article className={current.graph.includes('acct') ? 'selection__node on' : 'selection__node'} style={{ left: 500, top: 40 }}>
                <span>account-sim</span>
                <strong>Go · conta e mensagens</strong>
                <small>PostgreSQL · outbox</small>
              </article>
              <article className={current.graph.includes('bus') ? 'selection__bus on' : 'selection__bus'}>
                <strong>RabbitMQ</strong>
                <small>Eventos com trace nos headers</small>
                <span>message.received</span>
                <span>message.triaged</span>
                <span>account.event.recorded</span>
                <span>alert.raised</span>
                <span>case.opened</span>
                <span>case.sla.breached</span>
                <em>Inbox por event_id · DLQ por fila · TTL + DLX</em>
              </article>
              <article className={current.graph.includes('triage') ? 'selection__node on' : 'selection__node'} style={{ left: 880, top: 20 }}>
                <span>triage</span>
                <strong>Go · classificação</strong>
                <small>PostgreSQL · fallback</small>
              </article>
              <article className={current.graph.includes('jev') ? 'selection__node selection__node--ext on' : 'selection__node selection__node--ext'} style={{ left: 1084, top: 20, width: 96 }}>
                <span>externo</span>
                <strong>Jev</strong>
                <small>AI Gateway</small>
              </article>
              <article className={current.graph.includes('advisory') ? 'selection__node on' : 'selection__node'} style={{ left: 880, top: 140 }}>
                <span>advisory</span>
                <strong>Go · regras de alerta</strong>
                <small>PostgreSQL · gRPC</small>
              </article>
              <article className={current.graph.includes('cases') ? 'selection__node on' : 'selection__node'} style={{ left: 880, top: 260 }}>
                <span>cases</span>
                <strong>Go · casos e SLA</strong>
                <small>PostgreSQL · gRPC</small>
              </article>
              <article className={current.graph.includes('indexer') ? 'selection__node on' : 'selection__node'} style={{ left: 880, top: 380 }}>
                <span>timeline-indexer</span>
                <strong>Go · visão 360</strong>
                <small>Indexa a timeline</small>
              </article>
              <article className={current.graph.includes('es') ? 'selection__node selection__node--ext on' : 'selection__node selection__node--ext'} style={{ left: 1084, top: 380, width: 96 }}>
                <span>busca</span>
                <strong>Elastic{"\u00AD"}search</strong>
              </article>
            </div>
            <div className="selection__readout">
              <span>{String(step + 1).padStart(2, '0')}</span>
              <div>
                <strong>{current.title}</strong>
                <p>{current.detail}</p>
              </div>
              <div>
                <small>Neste passo</small>
                <code>{current.tag}</code>
                <em>trace 4bf92f35…0e4736 · mesmo trace do clique à fila</em>
              </div>
            </div>
            </div>
          </div>
        </div>
      </section>

      <SduiShowcase wide={wide} />

      <footer className="selection__foot">
        <ul>
          {(wide
            ? [
                'Go 1.26',
                'React + TypeScript + Vite',
                'PostgreSQL · um por serviço',
                'RabbitMQ',
                'Elasticsearch',
                'gRPC',
                'Jev · decisão tipada',
                'OpenTelemetry',
                'Docker Compose',
              ]
            : [
                'Go 1.26',
                'React + Vite',
                'PostgreSQL',
                'RabbitMQ',
                'Elasticsearch',
                'gRPC',
                'Jev',
                'OpenTelemetry',
              ]
          ).map((chip) => (
            <li key={chip}>{chip}</li>
          ))}
        </ul>
        <p>
          Demo técnica com dados, nomes e marcas fictícios, sem vínculo com nenhuma instituição real.
          {wide ? ' Nenhuma mensagem é enviada a clientes de verdade.' : ''}
        </p>
      </footer>
    </main>
  );
}
