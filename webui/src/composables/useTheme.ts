import { computed, readonly, ref, watch } from 'vue'

/** `auto` follows the OS `prefers-color-scheme`; `light`/`dark` force a mode. */
export type ThemeMode = 'auto' | 'light' | 'dark'

// Module-level singleton: one theme for the whole app.
const mode = ref<ThemeMode>('auto')
const systemPrefersDark = ref(false)
const isDark = computed(
    () => mode.value === 'dark' || (mode.value === 'auto' && systemPrefersDark.value)
)

let initialized = false

function initTheme(): void {
    if (initialized || typeof window === 'undefined') return
    initialized = true

    const mql = window.matchMedia('(prefers-color-scheme: dark)')
    systemPrefersDark.value = mql.matches
    mql.addEventListener('change', (e) => {
        systemPrefersDark.value = e.matches
    })
    watch(isDark, (dark) => document.documentElement.classList.toggle('dark-mode', dark), {
        immediate: true
    })
}

/** Switch to the opposite of what is shown now. */
function toggle(): void {
    mode.value = isDark.value ? 'light' : 'dark'
}

export function useTheme() {
    initTheme()
    return { mode, isDark: readonly(isDark), toggle }
}
