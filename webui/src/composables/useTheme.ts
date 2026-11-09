import { computed, readonly, ref, watch } from 'vue'
import { updatePrimaryPalette } from '@primeuix/themes'
import {
    DEFAULT_MODE,
    DEFAULT_THEME,
    MODES,
    THEMES,
    primaryPalettes,
    type ThemeMode,
    type ThemeName
} from '@/theme'

export type { ThemeMode, ThemeName }

const MODE_KEY = 'refapp-mode'
const THEME_KEY = 'refapp-theme'

// The user's picks are kept per device: there is no user settings API. What
// storage holds is not trusted: an unknown value is the default.
function load<T extends string>(key: string, known: readonly { name: T }[], fallback: T): T {
    try {
        const raw = localStorage.getItem(key)
        return known.find((k) => k.name === raw)?.name ?? fallback
    } catch {
        return fallback
    }
}

function save(key: string, value: string): void {
    try {
        localStorage.setItem(key, value)
    } catch {
        // storage unavailable (private mode): keep the in-memory value
    }
}

// Module-level singleton: one theme for the whole app.
const mode = ref<ThemeMode>(load(MODE_KEY, MODES, DEFAULT_MODE))
const theme = ref<ThemeName>(load(THEME_KEY, THEMES, DEFAULT_THEME))
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
    // The preset starts on the default palette: swap only for another one.
    watch(
        theme,
        (t, previous) => {
            if (previous !== undefined || t !== DEFAULT_THEME) updatePrimaryPalette(primaryPalettes[t])
        },
        { immediate: true }
    )
}

function setMode(m: ThemeMode): void {
    mode.value = m
    save(MODE_KEY, m)
}

function setTheme(t: ThemeName): void {
    theme.value = t
    save(THEME_KEY, t)
}

/** Switch to the opposite of what is shown now (the top bar's quick toggle). */
function toggle(): void {
    setMode(isDark.value ? 'light' : 'dark')
}

export function useTheme() {
    initTheme()
    return {
        mode: readonly(mode),
        theme: readonly(theme),
        isDark: readonly(isDark),
        setMode,
        setTheme,
        toggle
    }
}
