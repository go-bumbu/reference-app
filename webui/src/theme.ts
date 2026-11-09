import { definePreset } from '@primeuix/themes'
import Aura from '@primeuix/themes/aura'

// Aura with aether's teal primary. Components read the semantic tokens, so the
// primary scale is the one place the brand color lives.
const Theme = definePreset(Aura, {
    semantic: {
        primary: {
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
        }
    }
})

export default Theme
