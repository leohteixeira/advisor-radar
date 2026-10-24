import { expect, it } from 'vitest';
import { presentationUrl } from './PresentationRedirect';
it('preserves public anchors in the client-side landing fallback', () => {
  expect(presentationUrl('#arquitetura')).toBe('https://leohteixeira.com/pt/projects/advisor-radar#arquitetura');
  expect(presentationUrl('#sdui')).toBe('https://leohteixeira.com/pt/projects/advisor-radar#sdui');
});
