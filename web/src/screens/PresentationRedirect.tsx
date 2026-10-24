import { useEffect } from 'react';
export function presentationUrl(hash = ''): string {
  return `https://leohteixeira.com/pt/projects/advisor-radar${hash}`;
}
export function PresentationRedirect() {
  const destination = presentationUrl(window.location.hash);
  useEffect(() => { window.location.replace(destination); }, [destination]);
  return <main className="selection"><h1>Advisor Radar</h1><a href={destination}>Conhecer o projeto no portfólio</a></main>;
}
