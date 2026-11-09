import { createApp } from 'vue'
import { VueQueryPlugin, QueryClient } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import ConfirmationService from 'primevue/confirmationservice'
import '@fontsource-variable/inter'
import 'primeicons/primeicons.css'
import './assets/main.css'

import Theme from './theme'
import { useTheme } from './composables/useTheme'
import App from './App.vue'
import router from './router'

const app = createApp(App)

app.use(PrimeVue, {
    theme: {
        preset: Theme,
        options: {
            prefix: 'c',
            darkModeSelector: '.dark-mode',
            cssLayer: false
        }
    }
})

// Apply the saved mode and theme (auto follows the OS): toggles the
// `.dark-mode` class PrimeVue watches (darkModeSelector above) and swaps the
// primary palette. After PrimeVue, whose install would reset the palette.
useTheme()
app.use(ToastService)
app.use(ConfirmationService)
app.use(router)

// Server state lives in TanStack Query; there is no client-side store.
const queryClient = new QueryClient({
    defaultOptions: {
        queries: {
            refetchOnWindowFocus: false,
            retry: 1,
            staleTime: 1000 * 30
        },
        mutations: {
            retry: false
        }
    }
})
app.use(VueQueryPlugin, { queryClient })

app.mount('#app')
