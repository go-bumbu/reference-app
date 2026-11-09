import { createApp } from 'vue'
import { VueQueryPlugin, QueryClient } from '@tanstack/vue-query'
import PrimeVue from 'primevue/config'
import ToastService from 'primevue/toastservice'
import '@fontsource-variable/inter'
import './assets/main.css'

import Theme from './theme'
import { useTheme } from './composables/useTheme'
import App from './App.vue'
import router from './router'

const app = createApp(App)

// Follow the OS light/dark preference by default; toggles the `.dark-mode`
// class PrimeVue watches (darkModeSelector below).
useTheme()

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
app.use(ToastService)
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
