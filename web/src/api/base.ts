/** API paths sit under the Vite base (`/advisor-radar/`). */
export function apiPath(path: string): string {
  const base = import.meta.env.BASE_URL;
  const cleaned = path.replace(/^\//, '');
  return `${base}${cleaned}`;
}
