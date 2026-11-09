<script setup lang="ts">
import { THEMES, type ThemeName } from '@/theme'

// The accent themes as a row of swatch buttons, one of them on (`modelValue`).
// A pick is said as `update:modelValue`; the page saves it.
defineProps<{ modelValue: ThemeName }>()
const emit = defineEmits<{ 'update:modelValue': [theme: ThemeName] }>()
</script>

<template>
    <div class="theme-options" role="radiogroup" aria-label="Theme">
        <button
            v-for="t in THEMES"
            :key="t.name"
            type="button"
            class="theme-option"
            role="radio"
            :data-test="`theme-${t.name}`"
            :aria-checked="modelValue === t.name"
            :class="{ active: modelValue === t.name }"
            @click="modelValue !== t.name && emit('update:modelValue', t.name)"
        >
            <span class="theme-swatch" :style="{ background: t.swatch }" />
            <span>{{ t.label }}</span>
            <i v-if="modelValue === t.name" class="pi pi-check theme-check" aria-hidden="true" />
        </button>
    </div>
</template>

<style scoped>
.theme-options {
    display: flex;
    flex-wrap: wrap;
    gap: 0.5rem;
}

.theme-option {
    display: flex;
    align-items: center;
    gap: 0.55rem;
    padding: 0.5rem 0.8rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius);
    background: var(--app-panel);
    color: var(--app-text-muted);
    font-family: inherit;
    font-size: 0.85rem;
    font-weight: 500;
    cursor: pointer;
    transition:
        background 0.15s,
        color 0.15s,
        border-color 0.15s;
}
.theme-option:hover {
    background: var(--app-hover);
    color: var(--app-accent);
}
.theme-option.active {
    border-color: var(--app-accent);
    background: var(--app-accent-soft);
    color: var(--app-accent);
    font-weight: 600;
}
.theme-option:focus-visible {
    outline: none;
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}

.theme-swatch {
    flex-shrink: 0;
    width: 0.85rem;
    height: 0.85rem;
    border-radius: 999px;
    box-shadow: inset 0 0 0 1px rgb(255 255 255 / 0.25);
}

.theme-check {
    font-size: 0.75rem;
}
</style>
