<script setup lang="ts">
import SettingsRow from '@/components/settings/SettingsRow.vue'
import ModePicker from '@/components/settings/ModePicker.vue'
import ThemePicker from '@/components/settings/ThemePicker.vue'
import { useTheme } from '@/composables/useTheme'

// How the app looks: the accent theme and the light / dark mode. Both apply at
// once and are kept on this device (useTheme); there is no user settings API.
const { mode, theme, setMode, setTheme } = useTheme()
</script>

<template>
    <section aria-label="Appearance">
        <h2 class="set-section-title">Appearance</h2>
        <p class="set-note">Saved on this device.</p>
        <div class="set-card">
            <SettingsRow name="Theme" desc="The accent colour of the app." data-test="row-theme">
                <ThemePicker :model-value="theme" @update:model-value="setTheme" />
            </SettingsRow>
            <SettingsRow
                name="Mode"
                desc="Light, dark, or Auto to follow your system."
                data-test="row-mode"
            >
                <ModePicker :model-value="mode" @update:model-value="setMode" />
            </SettingsRow>
        </div>
    </section>
</template>
