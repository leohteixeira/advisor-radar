import { useEffect, useState } from 'react';
import { Link, useParams } from 'react-router-dom';
import { ApiError, fetchCustomer } from '../api/bff';
import { Timeline360 } from '../components/Timeline360';
import type { ClientInfo } from '../domain/types';

type LoadState =
  | { status: 'loading' }
  | { status: 'ok'; customer: ClientInfo }
  | { status: 'bad' }
  | { status: 'missing' }
  | { status: 'error' };

export function CustomerScreen() {
  const { id } = useParams<{ id: string }>();
  const [state, setState] = useState<LoadState>({ status: 'loading' });

  useEffect(() => {
    if (!id) {
      setState({ status: 'bad' });
      return;
    }
    let cancelled = false;
    setState({ status: 'loading' });
    void (async () => {
      try {
        const customer = await fetchCustomer(id);
        if (!cancelled) {
          setState({ status: 'ok', customer });
        }
      } catch (err) {
        if (cancelled) {
          return;
        }
        if (err instanceof ApiError && err.status === 400) {
          setState({ status: 'bad' });
          return;
        }
        if (err instanceof ApiError && err.status === 404) {
          setState({ status: 'missing' });
          return;
        }
        setState({ status: 'error' });
      }
    })();
    return () => {
      cancelled = true;
    };
  }, [id]);

  if (state.status === 'loading') {
    return <div className="queue-status">Carregando cliente…</div>;
  }

  if (state.status === 'bad') {
    return (
      <div className="customer-screen" data-testid="customer-bad-id">
        <Link to="/fila" className="customer-screen__back">
          ← Voltar à fila
        </Link>
        <p className="queue-status">Identificador de cliente inválido.</p>
      </div>
    );
  }

  if (state.status === 'missing') {
    return (
      <div className="customer-screen" data-testid="customer-not-found">
        <Link to="/fila" className="customer-screen__back">
          ← Voltar à fila
        </Link>
        <p className="queue-status">Cliente não encontrado.</p>
      </div>
    );
  }

  if (state.status === 'error') {
    return (
      <div className="customer-screen" data-testid="customer-error">
        <Link to="/fila" className="customer-screen__back">
          ← Voltar à fila
        </Link>
        <p className="queue-status">Não foi possível carregar o cliente.</p>
      </div>
    );
  }

  const { customer } = state;
  return (
    <div className="customer-screen" data-testid="customer-page">
      <Link to="/fila" className="customer-screen__back">
        ← Voltar à fila
      </Link>
      <Timeline360
        clientId={customer.id}
        name={customer.name}
        segment={customer.segment}
        aum={customer.aum}
        advisor={customer.advisor}
        since={customer.since}
      />
    </div>
  );
}
