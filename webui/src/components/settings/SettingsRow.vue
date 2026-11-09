<script setup lang="ts">
// One row of a settings card, after codex: the setting's name and a line that
// says what it does, with its control at the right (the default slot). The
// control goes under the text when there is no room for both.
defineProps<{
    name: string
    desc: string
}>()
</script>

<template>
    <div class="settings-row">
        <div class="settings-row-text">
            <span class="settings-row-name">{{ name }}</span>
            <span class="settings-row-desc">{{ desc }}</span>
        </div>
        <slot />
    </div>
</template>

<style scoped>
.settings-row {
    display: flex;
    flex-wrap: wrap;
    align-items: center;
    justify-content: space-between;
    gap: 0.5rem 1rem;
    padding: 0.75rem 0;
}
.settings-row + .settings-row {
    border-top: 1px solid var(--app-border);
}
.settings-row-text {
    display: flex;
    flex: 1 1 12rem;
    flex-direction: column;
    gap: 0.15rem;
}
.settings-row-name {
    font-size: 0.9rem;
    font-weight: 600;
    color: var(--app-text-primary);
}
.settings-row-desc {
    font-size: 0.8rem;
    color: var(--app-text-muted);
}
</style>
