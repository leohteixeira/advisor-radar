import { useEffect, useRef } from 'react';
import { Boundary } from './Boundary';
import { lookup } from './registry';
import type { Component, Screen, Slug } from './types';
import { OmittedPlaceholder, sectionLabel, XrayBanner, XrayLabel, type XrayOptions } from './Xray';

/**
 * Web owns the grid: one column on the phone, two on desktop, and the span of
 * each section as drawn in OrlaApp.dc.html. A section not listed spans one.
 */
const WIDE_SECTIONS: Readonly<Record<Slug, readonly string[]>> = {
  home: ['moment', 'highlights'],
  investir: ['cash', 'highlights'],
  carteira: [],
  perfil: ['header', 'suitability', 'advisor'],
};

function spanOf(slug: Slug, section: string): 1 | 2 {
  return Object.hasOwn(WIDE_SECTIONS, slug) && WIDE_SECTIONS[slug].includes(section) ? 2 : 1;
}

function UnknownType({ type }: { type: string }) {
  useEffect(() => {
    console.error(`sdui: unknown component type ${type}`);
  }, [type]);
  return null;
}

function Slot({ component }: { component: Component }) {
  const Entry = lookup(component.type);
  if (!Entry) {
    return <UnknownType type={component.type} />;
  }
  return (
    <Boundary type={component.type} resetKey={component}>
      <Entry variant={component.variant} props={component.props} />
    </Boundary>
  );
}

/**
 * Renders a screen envelope: the heading, then each section in array order,
 * each component through the registry. Omitted sections render nothing unless
 * Raio-X is on (`xray`): then every section is outlined and labelled, a banner
 * shows the request and envelope metadata, and each omitted section is drawn
 * as a dashed placeholder after the rendered ones, in `omitted` order (web has
 * no catalog to place it). With `xray` absent nothing of Raio-X is rendered.
 */
export function SduiScreen({ screen, xray }: { screen: Screen; xray?: XrayOptions }) {
  const on = xray !== undefined;
  return (
    <div className="sdui-screen" data-slug={screen.slug} data-revision={screen.revision} data-xray={on ? 'on' : undefined}>
      <div className="sdui-screen__head">
        <h1>{screen.title}</h1>
        {screen.subtitle ? <span>{screen.subtitle}</span> : null}
      </div>
      {on ? <XrayBanner screen={screen} customerID={xray.customerID} /> : null}
      <div className="sdui-grid">
        {screen.sections.map((section, index) => (
          <div key={`${index}-${section.id}`} className="sdui-section" data-section={section.id} data-span={spanOf(screen.slug, section.id)}>
            {on ? <XrayLabel text={sectionLabel(section)} /> : null}
            {section.components.map((component, slot) => (
              <Slot key={`${slot}-${component.type}`} component={component} />
            ))}
          </div>
        ))}
        {on
          ? screen.omitted.map((row, index) => (
              <div key={`omitted-${index}-${row.id}`} className="sdui-section sdui-section--omitted" data-section={row.id} data-span={spanOf(screen.slug, row.id)}>
                <OmittedPlaceholder row={row} />
              </div>
            ))
          : null}
      </div>
    </div>
  );
}

/**
 * The screen area when the screen request failed, timed out, or answered
 * something that is not an envelope. There is no hardcoded screen to fall
 * back to; `onRetry` sends the request again.
 */
export function SduiError({ onRetry }: { onRetry: () => void }) {
  const heading = useRef<HTMLHeadingElement>(null);
  // The error replaces the screen, so focus moves to its heading.
  useEffect(() => heading.current?.focus(), []);
  return (
    <div className="sdui-error">
      <div role="alert">
        <h1 ref={heading} tabIndex={-1}>
          Não foi possível montar sua tela.
        </h1>
      </div>
      <button type="button" className="sdui-btn sdui-btn--secondary" onClick={onRetry}>
        Tentar de novo
      </button>
    </div>
  );
}

/**
 * The screen area when the BFF serves no `schema_version` this app renders
 * (a 406, or an envelope of another version). Only a newer app can show the
 * screen, so the button reloads the page to pick one up.
 */
export function SduiUpdate() {
  const heading = useRef<HTMLHeadingElement>(null);
  // The notice replaces the screen, so focus moves to its heading.
  useEffect(() => heading.current?.focus(), []);
  return (
    <div className="sdui-error">
      <div role="alert">
        <h1 ref={heading} tabIndex={-1}>
          Atualize o app para ver esta tela.
        </h1>
      </div>
      <button type="button" className="sdui-btn sdui-btn--secondary" onClick={() => window.location.reload()}>
        Recarregar
      </button>
    </div>
  );
}

/** The screen skeleton while the one screen request is in flight. */
export function SduiLoading() {
  return (
    <div className="sdui-loading">
      <span role="status">Montando sua tela…</span>
      <span className="sdui-skel sdui-skel--title" aria-hidden="true" />
      <span className="sdui-skel sdui-skel--moment" aria-hidden="true" />
      <span className="sdui-skel sdui-skel--wealth" aria-hidden="true" />
      <div className="sdui-skel__actions" aria-hidden="true">
        <span className="sdui-skel" />
        <span className="sdui-skel" />
        <span className="sdui-skel" />
        <span className="sdui-skel" />
      </div>
      <span className="sdui-skel sdui-skel--advisor" aria-hidden="true" />
      <p className="sdui-loading__note">Uma requisição só. Se passar do tempo, a tela mostra o que já chegou e cada seção atrasada cai na variante padrão.</p>
    </div>
  );
}
