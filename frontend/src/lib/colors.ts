// Curated palette for tasks and subjects. Tokens mirror the backend's TaskColors
// list; an empty or unknown token falls back to the default accent. Each colour
// is supplied as a single hex and exposed to CSS via a --task custom property,
// with soft fills and borders derived through color-mix().

export type ColorToken =
  | 'blue'
  | 'violet'
  | 'emerald'
  | 'amber'
  | 'rose'
  | 'cyan'
  | 'orange'
  | 'slate';

export const TASK_COLORS: { token: ColorToken; label: string; hex: string }[] = [
  { token: 'blue', label: 'Blue', hex: '#36a6f2' },
  { token: 'violet', label: 'Violet', hex: '#a78bfa' },
  { token: 'emerald', label: 'Emerald', hex: '#34d399' },
  { token: 'amber', label: 'Amber', hex: '#fbbf24' },
  { token: 'rose', label: 'Rose', hex: '#fb7185' },
  { token: 'cyan', label: 'Cyan', hex: '#22d3ee' },
  { token: 'orange', label: 'Orange', hex: '#fb923c' },
  { token: 'slate', label: 'Slate', hex: '#94a3b8' },
];

const DEFAULT_HEX = '#36a6f2'; // matches --accent

const HEX_BY_TOKEN = new Map<string, string>(TASK_COLORS.map((c) => [c.token, c.hex]));

// taskHex resolves a stored token to a hex colour, falling back to the default.
// Used for both task and subject colours (they share one palette).
export function taskHex(token: string | undefined | null): string {
  return (token && HEX_BY_TOKEN.get(token)) || DEFAULT_HEX;
}
