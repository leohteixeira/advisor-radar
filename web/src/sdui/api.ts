import { apiPath } from '../api/base';
import { ApiError } from '../api/bff';
import type { Component, Omitted, Screen, Section, Slug } from './types';

/** The screen route answered 200 with a body that is not a screen envelope. */
export class InvalidScreenError extends Error {
  constructor() {
    super('sdui: response is not a screen envelope');
    this.name = 'InvalidScreenError';
  }
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === 'object' && value !== null && !Array.isArray(value);
}

/** An envelope has a numeric `schema_version` and a `sections` array. */
export function isScreen(value: unknown): value is Record<string, unknown> & { schema_version: number; sections: unknown[] } {
  return isRecord(value) && typeof value.schema_version === 'number' && Array.isArray(value.sections);
}

function asComponent(value: unknown): Component | null {
  if (!isRecord(value) || typeof value.type !== 'string') {
    return null;
  }
  return {
    type: value.type,
    variant: typeof value.variant === 'string' ? value.variant : '',
    props: isRecord(value.props) ? value.props : {},
  };
}

function asSection(value: unknown): Section | null {
  if (!isRecord(value) || typeof value.id !== 'string' || !Array.isArray(value.components)) {
    return null;
  }
  const components = value.components.map(asComponent).filter((c): c is Component => c !== null);
  return { id: value.id, components };
}

function asOmitted(value: unknown): Omitted[] {
  if (!Array.isArray(value)) {
    return [];
  }
  return value
    .filter((row): row is Record<string, unknown> & { id: string; type: string } => isRecord(row) && typeof row.id === 'string' && typeof row.type === 'string')
    .map((row) => ({ id: row.id, type: row.type, reason: typeof row.reason === 'string' ? row.reason : '' }));
}

/** How long web waits for one screen before it treats the request as failed. */
export const SCREEN_TIMEOUT_MS = 3000;

/** A screen, the HTTP status, and the JSON body the BFF answered, as parsed before normalizing. */
export interface ScreenResponse {
  screen: Screen;
  status: number;
  body: unknown;
}

/**
 * Fetches one SDUI screen. A non-2xx answer throws ApiError; a 200 whose body
 * is not JSON, not an envelope, or an envelope of another slug throws
 * InvalidScreenError. The request aborts on `signal` or after
 * SCREEN_TIMEOUT_MS. Malformed sections, components, and omitted rows are
 * dropped.
 */
export async function fetchScreen(customerID: string, slug: Slug, signal: AbortSignal): Promise<Screen> {
  return (await fetchScreenResponse(customerID, slug, signal)).screen;
}

/**
 * Like fetchScreen, and also returns the body as the BFF sent it, for the
 * selection screen that shows the live response next to the rendered screen.
 */
export async function fetchScreenResponse(customerID: string, slug: Slug, signal: AbortSignal): Promise<ScreenResponse> {
  const res = await fetch(apiPath(`v1/client-pov/customers/${encodeURIComponent(customerID)}/screens/${slug}`), {
    signal: AbortSignal.any([signal, AbortSignal.timeout(SCREEN_TIMEOUT_MS)]),
  });
  if (!res.ok) {
    throw new ApiError(res.status);
  }
  let body: unknown;
  try {
    body = await res.json();
  } catch {
    throw new InvalidScreenError();
  }
  if (!isScreen(body) || body.slug !== slug) {
    throw new InvalidScreenError();
  }
  const screen: Screen = {
    schema_version: body.schema_version,
    slug,
    revision: typeof body.revision === 'string' ? body.revision : '',
    title: typeof body.title === 'string' ? body.title : '',
    subtitle: typeof body.subtitle === 'string' && body.subtitle !== '' ? body.subtitle : undefined,
    sections: body.sections.map(asSection).filter((s): s is Section => s !== null),
    omitted: asOmitted(body.omitted),
  };
  return { screen, status: res.status, body };
}
