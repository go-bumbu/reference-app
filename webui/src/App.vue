<script setup lang="ts">
import { computed } from 'vue'
import Button from 'primevue/button'
import Message from 'primevue/message'
import Toast from 'primevue/toast'
import { useInfo, useMe } from '@/composables/useApi'
import { useTheme } from '@/composables/useTheme'
import { apiErrorMessage, apiErrorStatus } from '@/lib/apiError'

const { isDark, toggle } = useTheme()
const { data: me, error: meError } = useMe()
const { data: info } = useInfo()

// A 401 on /me means nothing authenticated this browser: in "proxy" mode the
// page was opened without the proxy in front of the server.
const unauthenticated = computed(() => apiErrorStatus(meError.value) === 401)
const version = computed(() => info.value?.version ?? '')
</script>

<template>
    <div class="app">
        <header class="topbar">
            <RouterLink to="/" class="brand">reference-app</RouterLink>
            <div class="topbar-end">
                <span v-if="me" class="identity" data-test="identity">
                    {{ me.subject }} <span class="muted">· {{ me.tenant }}</span>
                </span>
                <Button
                    :label="isDark ? 'Light' : 'Dark'"
                    size="small"
                    severity="secondary"
                    text
                    aria-label="Toggle the color scheme"
                    @click="toggle"
                />
            </div>
        </header>

        <main class="content">
            <Message v-if="unauthenticated" severity="warn" data-test="unauthenticated">
                You are not authenticated. The server expects an authenticating proxy in front of
                it.
            </Message>
            <Message v-else-if="meError" severity="error">{{ apiErrorMessage(meError) }}</Message>
            <RouterView v-else />
        </main>

        <footer class="footer muted">
            <span v-if="version">server {{ version }}</span>
        </footer>
        <Toast />
    </div>
</template>

<style scoped>
.app {
    min-height: 100vh;
    display: flex;
    flex-direction: column;
}

.topbar {
    display: flex;
    align-items: center;
    justify-content: space-between;
    padding: 0.5rem 1.25rem;
    background: var(--app-panel);
    border-bottom: 1px solid var(--app-border);
}

.brand {
    font-weight: 600;
    text-decoration: none;
    color: var(--app-text-primary);
}

.topbar-end {
    display: flex;
    align-items: center;
    gap: 0.75rem;
}

.content {
    flex: 1;
    width: 100%;
    max-width: 48rem;
    margin: 0 auto;
    padding: 1.5rem 1.25rem;
}

.footer {
    padding: 0.75rem 1.25rem;
    font-size: 0.8rem;
    text-align: center;
}
</style>
