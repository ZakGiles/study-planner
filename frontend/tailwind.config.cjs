/** @type {import('tailwindcss').Config} */
module.exports = {
  content: ['./index.html', './src/**/*.{svelte,js,ts}'],
  theme: {
    extend: {
      // Colours are backed by CSS variables so the dark/light themes (toggled
      // via [data-theme] on <html>) keep working with plain Tailwind classes.
      colors: {
        bg: 'var(--bg)',
        sidebar: 'var(--sidebar-bg)',
        inset: 'var(--inset)',
        surface: {
          DEFAULT: 'var(--surface)',
          2: 'var(--surface-2)',
          3: 'var(--surface-3)',
        },
        line: {
          DEFAULT: 'var(--border)',
          soft: 'var(--border-soft)',
          strong: 'var(--border-strong)',
        },
        fg: {
          DEFAULT: 'var(--text)',
          strong: 'var(--text-strong)',
          muted: 'var(--muted)',
          faint: 'var(--faint)',
        },
        accent: {
          DEFAULT: 'var(--accent)',
          bright: 'var(--accent-bright)',
          soft: 'var(--accent-soft)',
          line: 'var(--accent-line)',
        },
        green: 'var(--green)',
        amber: {
          DEFAULT: 'var(--amber)',
          soft: 'var(--amber-soft)',
          line: 'var(--amber-line)',
        },
        red: {
          DEFAULT: 'var(--red)',
          soft: 'var(--red-soft)',
          line: 'var(--red-line)',
        },
      },
      borderRadius: {
        xs: 'var(--r-xs)',
        sm: 'var(--r-sm)',
        md: 'var(--r-md)',
        lg: 'var(--r-lg)',
      },
      boxShadow: {
        1: 'var(--shadow-1)',
        2: 'var(--shadow-2)',
        pop: 'var(--shadow-pop)',
      },
      fontFamily: {
        display: ['"Bricolage Grotesque"', '"Hanken Grotesk"', 'ui-sans-serif', 'sans-serif'],
        body: ['"Hanken Grotesk"', 'ui-sans-serif', 'system-ui', '-apple-system', '"Segoe UI"', 'Roboto', 'sans-serif'],
      },
      maxWidth: {
        content: 'var(--content)',
      },
      transitionTimingFunction: {
        smooth: 'var(--ease)',
        'smooth-out': 'var(--ease-out)',
      },
    },
  },
  plugins: [],
};
