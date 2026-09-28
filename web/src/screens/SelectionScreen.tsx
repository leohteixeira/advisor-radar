import { useEffect, useState } from 'react';
import { Link } from 'react-router-dom';
import { GRAPH_NODES, WALK_INTERVAL_MS, WALK_STEPS, walkStep } from '../selection/steps';

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
  const pauseAria = paused ? 'Reproduzir animação' : 'Pausar animação';

  return (
    <main className={wide ? 'selection selection--wide' : 'selection'}>
      <header className="selection__bar">
        <span className="selection__brand">Advisor Radar</span>
        <span className="selection__demo">Demo técnica · dados fictícios</span>
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

      <section className="selection__product" aria-labelledby="selection-product">
        <p className="selection__eyebrow">O produto</p>
        <h2 id="selection-product">Quem precisa de atenção agora, e por quê.</h2>
        <p>
          O Advisor Radar junta o que acontece na conta do cliente e o que ele escreve, e transforma
          isso em ações priorizadas para a assessoria.
        </p>
        <div className="selection__pillars">
          <article>
            <h3>Alertas proativos</h3>
            <p>
              {wide
                ? 'Regras determinísticas sobre eventos de conta: saque relevante, queda de patrimônio, aporte grande, mudança de segmento e cliente sem contato.'
                : 'Saque relevante, queda de patrimônio, aporte grande, mudança de segmento e cliente sem contato.'}
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
        </div>
      </section>

      <section className="selection__walk" aria-labelledby="selection-walk">
        <div className="selection__walk-head">
          <div>
            <p className="selection__eyebrow">Arquitetura</p>
            <h2 id="selection-walk">
              {wide
                ? 'Como uma reclamação chega à fila do assessor'
                : 'Como uma reclamação chega à fila'}
            </h2>
          </div>
          {wide ? (
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
          ) : null}
          <button
            type="button"
            className="selection__pause"
            aria-label={pauseAria}
            aria-pressed={paused}
            onClick={togglePlayback}
          >
            {pauseLabel}
          </button>
        </div>

        {wide ? (
          <div className="selection__graph" aria-hidden="true">
            {GRAPH_NODES.map((node) => (
              <span key={node} className={current.nodes.includes(node) ? 'is-on' : undefined}>
                {node}
              </span>
            ))}
          </div>
        ) : null}

        <ol className="selection__steps" aria-label="Passos da reclamação">
          {WALK_STEPS.map((item, index) => {
            const active = index === step;
            return (
              <li key={item.title} aria-current={active ? 'step' : undefined}>
                <span className="selection__num">{index + 1}</span>
                <div>
                  <span className="selection__where">{item.where}</span>
                  <strong>{item.title}</strong>
                  {active ? (
                    <>
                      <p>{item.detail}</p>
                      <code>{item.tag}</code>
                    </>
                  ) : null}
                </div>
              </li>
            );
          })}
        </ol>

        {wide ? (
          <p className="selection__step-readout">
            <span>{String(step + 1).padStart(2, '0')}</span>
            <span>{current.title}</span>
            <code>{current.tag}</code>
          </p>
        ) : null}
      </section>

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
