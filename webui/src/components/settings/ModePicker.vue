<script setup lang="ts">
import { MODES, type ThemeMode } from '@/theme'

// Light, dark or auto as a segmented control, one of them on (`modelValue`).
// A pick is said as `update:modelValue`; the page saves it.
defineProps<{ modelValue: ThemeMode }>()
const emit = defineEmits<{ 'update:modelValue': [mode: ThemeMode] }>()
</script>

<template>
    <div class="mode-switch" role="radiogroup" aria-label="Colour mode">
        <button
            v-for="m in MODES"
            :key="m.name"
            type="button"
            class="mode-option"
            role="radio"
            :data-test="`mode-${m.name}`"
            :aria-checked="modelValue === m.name"
            :class="{ active: modelValue === m.name }"
            @click="modelValue !== m.name && emit('update:modelValue', m.name)"
        >
            <i :class="m.icon" aria-hidden="true" />
            <span>{{ m.label }}</span>
        </button>
    </div>
</template>

<style scoped>
/* A connected segmented control: one choice of three. */
.mode-switch {
    display: inline-flex;
    gap: 0.2rem;
    padding: 0.2rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius-lg);
    background: var(--app-background);
}

.mode-option {
    display: inline-flex;
    align-items: center;
    gap: 0.4rem;
    padding: 0.4rem 0.85rem;
    border: none;
    border-radius: var(--app-radius);
    background: transparent;
    color: var(--app-text-muted);
    font-family: inherit;
    font-size: 0.82rem;
    font-weight: 500;
    cursor: pointer;
    transition:
        background 0.15s,
        color 0.15s;
}
.mode-option i {
    font-size: 0.82rem;
}
.mode-option:hover {
    color: var(--app-accent);
}
.mode-option.active {
    background: var(--app-panel);
    color: var(--app-accent);
    font-weight: 600;
    box-shadow: var(--app-shadow);
}
.mode-option:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}
</style>
