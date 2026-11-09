<script setup lang="ts">
import SettingsSidebar from '@/components/settings/SettingsSidebar.vue'

// The settings chrome, after codex's (GitHub-style), the parent of every
// settings page: the title, then the sidebar and the open page beside it. It
// is drawn once for the whole area, so the sidebar stays put while its links
// change the page.
</script>

<template>
    <div class="settings-page">
        <header class="settings-head">
            <h1>Settings</h1>
        </header>
        <div class="settings-layout">
            <SettingsSidebar />
            <div class="settings-panel">
                <RouterView />
            </div>
        </div>
    </div>
</template>

<style scoped>
.settings-page {
    max-width: 60rem;
    margin: 0 auto;
}

.settings-head {
    margin-bottom: 1.5rem;
    padding-bottom: 1rem;
    border-bottom: 1px solid var(--app-border);
}
.settings-head h1 {
    margin: 0;
    font-size: 1.5rem;
}

.settings-layout {
    display: grid;
    grid-template-columns: 13rem minmax(0, 1fr);
    gap: 2.5rem;
    align-items: start;
}

.settings-panel {
    min-width: 0;
}

/* Shared by every settings page: a titled section of rows on a card. */
.settings-panel :deep(.set-section-title) {
    margin: 0 0 1rem;
    padding-bottom: 0.6rem;
    border-bottom: 1px solid var(--app-border);
    font-size: 1.15rem;
}
.settings-panel :deep(.set-card) {
    padding: 0.25rem 1rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-lg);
    background: var(--app-panel);
}
.settings-panel :deep(.set-note) {
    margin: 0 0 1rem;
    font-size: 0.85rem;
    color: var(--app-text-muted);
}

@media (max-width: 640px) {
    .settings-layout {
        grid-template-columns: minmax(0, 1fr);
        gap: 1.25rem;
    }
}
</style>
