import {defineConfig} from 'vite'
import {svelte} from '@sveltejs/vite-plugin-svelte'

// https://vitejs.dev/config/
export default defineConfig({
  plugins: [svelte()],
  // Relative asset URLs so the same build works embedded in the Wails desktop
  // shell (served at /) and on GitHub Pages (served under /study-planner/).
  base: './',
})
