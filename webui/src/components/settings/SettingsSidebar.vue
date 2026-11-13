<script setup lang="ts">
import { computed } from 'vue'
import { useRoute } from 'vue-router'
import { useApiKeysEnabled } from '@/composables/useApi'

// The settings sidebar, after codex's (GitHub-style): every page of the
// settings as a link, the open one marked. It works its state out from the
// route, so it takes no props. The API keys page is listed only when the
// server offers keys.
const route = useRoute()
const apiKeysEnabled = useApiKeysEnabled()

const ALL_PAGES = [
    { name: 'settings-appearance', label: 'Appearance', icon: 'pi pi-palette' },
    { name: 'settings-jobs', label: 'Jobs', icon: 'pi pi-history' },
    { name: 'settings-api-keys', label: 'API keys', icon: 'pi pi-key' },
    { name: 'settings-about', label: 'About', icon: 'pi pi-info-circle' }
] as const

const PAGES = computed(() =>
    ALL_PAGES.filter((p) => p.name !== 'settings-api-keys' || apiKeysEnabled.value === true)
)

const open = computed(() => route.name)
</script>

<template>
    <nav class="settings-sidebar" aria-label="Settings">
        <RouterLink
            v-for="p in PAGES"
            :key="p.name"
            class="nav-item"
            :class="{ active: open === p.name }"
            :to="{ name: p.name }"
            :aria-current="open === p.name ? 'page' : undefined"
            :data-test="`nav-${p.name}`"
        >
            <i :class="p.icon" aria-hidden="true" /><span>{{ p.label }}</span>
        </RouterLink>
    </nav>
</template>

<style scoped>
.settings-sidebar {
    display: flex;
    flex-direction: column;
    gap: 0.15rem;
}

.nav-item {
    display: flex;
    align-items: center;
    gap: 0.6rem;
    padding: 0.5rem 0.75rem;
    border-radius: var(--app-radius);
    color: var(--app-text-muted);
    font-size: 0.88rem;
    font-weight: 500;
    text-decoration: none;
    transition:
        background 0.15s,
        color 0.15s;
}
.nav-item i {
    width: 1rem;
    font-size: 0.85rem;
    text-align: center;
}
.nav-item:hover {
    background: var(--app-hover);
    color: var(--app-text-primary);
}
.nav-item.active {
    background: var(--app-accent-soft);
    color: var(--app-accent);
    font-weight: 600;
}
.nav-item:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}

/* A phone has no room beside the page: the links run in a row above it. */
@media (max-width: 640px) {
    .settings-sidebar {
        flex-direction: row;
        flex-wrap: wrap;
    }
}
</style>
