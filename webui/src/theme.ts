import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

// Selectable accent palettes (PrimeVue primary ramps). The preset starts on
// teal; useTheme swaps the live primary with `updatePrimaryPalette` when the
// theme changes. The shell's --app-accent tokens read --c-primary-color, so
// they follow without a palette of their own.
export const primaryPalettes = {
    // aether's teal, the default
    teal: {
        50: '#ecfbff',
        100: '#cff4fb',
        200: '#a5e9f5',
        300: '#6ddbec',
        400: '#2fd3ef',
        500: '#12b3d1',
        600: '#0e9bb5',
        700: '#0b8299',
        800: '#0c6a7d',
        900: '#0f5766',
        950: '#073843'
    },
    // codex's Amber
    amber: {
        50: '#fdf7ea',
        100: '#f9ecc9',
        200: '#f2d792',
        300: '#ebc25b',
        400: '#e0a94e',
        500: '#d1961f',
        600: '#b57c12',
        700: '#90600f',
        800: '#744d10',
        900: '#614011',
        950: '#372105'
    },
    violet: {
        50: '#f5f3ff',
        100: '#ede9fe',
        200: '#ddd6fe',
        300: '#c4b5fd',
        400: '#a78bfa',
        500: '#8b5cf6',
        600: '#7c3aed',
        700: '#6d28d9',
        800: '#5b21b6',
        900: '#4c1d95',
        950: '#2e1065'
    }
} as const

// Two orthogonal axes the user controls (see composables/useTheme.ts):
//   mode  = brightness (light | dark | auto — auto follows the OS)
//   theme = accent identity (the colour of the shell and of PrimeVue's primary)
// They compose: any theme works in either mode.
export type ThemeMode = 'light' | 'dark' | 'auto'
export type ThemeName = keyof typeof primaryPalettes

export interface ThemeModeDef {
    name: ThemeMode
    label: string
    /** PrimeIcons glyph for the segmented control. */
    icon: string
}

export const MODES: ThemeModeDef[] = [
    { name: 'light', label: 'Light', icon: 'pi pi-sun' },
    { name: 'dark', label: 'Dark', icon: 'pi pi-moon' },
    { name: 'auto', label: 'Auto', icon: 'pi pi-desktop' }
]

export const DEFAULT_MODE: ThemeMode = 'auto'

export interface ThemeDef {
    name: ThemeName
    label: string
    /** Representative accent colour for the settings swatch. */
    swatch: string
}

export const THEMES: ThemeDef[] = [
    { name: 'teal', label: 'Teal', swatch: primaryPalettes.teal[500] },
    { name: 'amber', label: 'Amber', swatch: primaryPalettes.amber[500] },
    { name: 'violet', label: 'Violet', swatch: primaryPalettes.violet[500] }
]

export const DEFAULT_THEME: ThemeName = 'teal'

const Theme = definePreset(Aura, {
    semantic: {
        primary: { ...primaryPalettes[DEFAULT_THEME] }
    }
})

export default Theme
