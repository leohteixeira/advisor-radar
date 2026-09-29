import type { Omitted, Screen, Section } from './types';

/**
 * Raio-X SDUI: a demo tool of the client app, not part of a real client app.
 * It labels each rendered section, adds a banner with the request and the
 * envelope metadata, and draws omitted sections as dashed placeholders. Every
 * value it shows is read from the envelope; web decides nothing here.
 */

/** Raio-X is on for this screen; the banner names the customer's request. */
export interface XrayOptions {
  customerID: string;
}

/** The `omitted` reason for a section the BFF failed to build. */
const BUILD_ERROR = 'build_error';

/** Service names behind the source reasons, for the placeholder copy. */
const SOURCE_SERVICES: Readonly<Record<string, string>> = {
  'account-sim': 'account-sim',
  advisory: 'advisory',
  cases: 'cases',
  moments: 'advisory',
  profile: 'advisory',
  timeline: 'timeline-indexer',
};

/** `id · type · variant` of a rendered section, from its first component; empty parts are left out. */
export function sectionLabel(section: Section): string {
  const first = section.components[0];
  return [section.id, first?.type, first?.variant].filter((part) => part !== undefined && part !== '').join(' · ');
}

/** `id · type · omitido` of an omitted section. */
export function omittedLabel(row: Omitted): string {
  return `${row.id} · ${row.type} · omitido`;
}

/**
 * The failures the banner names: "{reason} fora do ar" once per source reason,
 * and "erro ao montar {id}" per section the BFF failed to build.
 */
export function failureNotes(omitted: readonly Omitted[]): string[] {
  const notes: string[] = [];
  for (const row of omitted) {
    const note = row.reason === BUILD_ERROR ? `erro ao montar ${row.id}` : row.reason === '' ? '' : `${row.reason} fora do ar`;
    if (note !== '' && !notes.includes(note)) {
      notes.push(note);
    }
  }
  return notes;
}

/** "slug … · revision … · schema … · N seções", plus the failure notes. */
export function xrayMeta(screen: Screen): string {
  const count = screen.sections.length;
  const parts = [
    `slug ${screen.slug}`,
    `revision ${screen.revision}`,
    `schema ${screen.schema_version}`,
    `${count} ${count === 1 ? 'seção' : 'seções'}`,
    ...failureNotes(screen.omitted),
  ];
  return parts.join(' · ');
}

function omittedBody(reason: string): string {
  const tail = 'O resto da tela seguiu. No app real, nada aparece aqui.';
  if (reason === BUILD_ERROR) {
    return `O bff não conseguiu montar a seção e a tirou da resposta. ${tail}`;
  }
  if (reason === '') {
    return `O bff tirou a seção da resposta. ${tail}`;
  }
  const service = Object.hasOwn(SOURCE_SERVICES, reason) ? SOURCE_SERVICES[reason] : reason;
  return `O ${service} não respondeu a tempo e o bff tirou a seção. ${tail}`;
}

/** The request and envelope metadata above the sections. */
export function XrayBanner({ screen, customerID }: { screen: Screen; customerID: string }) {
  return (
    <section className="sdui-xray-banner" aria-label="Raio-X SDUI">
      <code>
        GET /v1/client-pov/customers/{encodeURIComponent(customerID)}/screens/{screen.slug}
      </code>
      <span>{xrayMeta(screen)}</span>
    </section>
  );
}

/** The chip on top of each outlined section. */
export function XrayLabel({ text }: { text: string }) {
  return <span className="sdui-xray-label">{text}</span>;
}

/** The dashed stand-in for a section the BFF left out. Raio-X only. */
export function OmittedPlaceholder({ row }: { row: Omitted }) {
  const label = omittedLabel(row);
  return (
    <>
      <XrayLabel text={label} />
      <div className="sdui-omitted">
        <strong>{label}</strong>
        <span>{omittedBody(row.reason)}</span>
      </div>
    </>
  );
}
