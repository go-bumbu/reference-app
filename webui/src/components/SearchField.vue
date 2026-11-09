<script setup lang="ts">
// The top bar's search field. The query text is the caller's (v-model), so the
// shell can bind it to the route and every view reads it from there.
withDefaults(
    defineProps<{
        modelValue: string
        placeholder?: string
        ariaLabel?: string
    }>(),
    { placeholder: 'Search...', ariaLabel: 'Search' }
)
const emit = defineEmits<{ 'update:modelValue': [value: string] }>()

function onInput(e: Event) {
    emit('update:modelValue', (e.target as HTMLInputElement).value)
}
</script>

<template>
    <div class="search-box">
        <i class="pi pi-search search-icon" aria-hidden="true"></i>
        <input
            :value="modelValue"
            type="search"
            class="search-input"
            data-test="search-input"
            :placeholder="placeholder"
            :aria-label="ariaLabel"
            @input="onInput"
        />
        <button
            v-if="modelValue"
            class="search-clear"
            type="button"
            data-test="search-clear"
            aria-label="Clear search"
            @click="emit('update:modelValue', '')"
        >
            <i class="pi pi-times" aria-hidden="true"></i>
        </button>
    </div>
</template>

<style scoped>
.search-box {
    position: relative;
    width: 100%;
}

.search-icon {
    position: absolute;
    left: 0.75rem;
    top: 50%;
    transform: translateY(-50%);
    font-size: 0.85rem;
    color: var(--app-text-muted);
    pointer-events: none;
}

.search-input {
    width: 100%;
    height: 2.25rem;
    padding: 0 2.25rem;
    border: 1px solid var(--app-border);
    border-radius: var(--app-radius);
    font-size: 0.85rem;
    font-family: inherit;
    color: var(--app-text-primary);
    background: var(--app-panel);
    outline: none;
    transition:
        border-color 0.15s,
        box-shadow 0.15s;
}
/* The clear button is ours; drop the browser's own. */
.search-input::-webkit-search-cancel-button {
    display: none;
}
.search-input::placeholder {
    color: var(--app-text-muted);
}
.search-input:focus {
    border-color: var(--app-accent);
    box-shadow: 0 0 0 3px var(--app-focus-ring);
}

.search-clear {
    position: absolute;
    right: 0.5rem;
    top: 50%;
    transform: translateY(-50%);
    display: flex;
    align-items: center;
    justify-content: center;
    width: 1.5rem;
    height: 1.5rem;
    border: none;
    background: transparent;
    border-radius: var(--app-radius);
    color: var(--app-text-muted);
    cursor: pointer;
    font-size: 0.75rem;
}
.search-clear:hover {
    color: var(--app-accent);
}
</style>
