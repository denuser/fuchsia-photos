<script setup lang="ts">
import { onBeforeUnmount, onMounted, ref, watch } from 'vue'

export interface SelectionEntry {
  value: unknown
  title: string
}

const props = defineProps<{ entries: SelectionEntry[]; focused: boolean }>()
const emit = defineEmits<{ remove: [value: unknown] }>()

// Room kept for the search input next to the chips while the field is focused, so that it stays on the same line.
const INPUT_ROOM = 64

const root = ref<HTMLElement>()
const measure = ref<HTMLElement>()
const fits = ref(true)

function check() {
  const input = root.value?.closest('.v-field__input')
  if (!input || !measure.value) return
  const style = getComputedStyle(input)
  const available =
    input.clientWidth - parseFloat(style.paddingInlineStart) - parseFloat(style.paddingInlineEnd) - (props.focused ? INPUT_ROOM : 0)
  fits.value = measure.value.offsetWidth <= available
}

let observer: ResizeObserver | undefined

onMounted(() => {
  observer = new ResizeObserver(check)
  const input = root.value?.closest('.v-field__input')
  if (input) observer.observe(input)
  watch(measure, (element, previous) => {
    if (previous) observer?.unobserve(previous)
    if (element) observer?.observe(element)
  }, { immediate: true })
  watch(() => props.focused, check)
})

onBeforeUnmount(() => observer?.disconnect())
</script>

<template>
  <!-- Clicks on the chips must not open the list of the field around them. -->
  <div ref="root" class="selection-chips" @mousedown.stop.prevent @click.stop>
    <div ref="measure" class="measure" aria-hidden="true">
      <v-chip v-for="entry in entries" :key="String(entry.value)" :text="entry.title" size="small" closable />
    </div>
    <template v-if="fits">
      <v-chip
        v-for="entry in entries"
        :key="String(entry.value)"
        :text="entry.title"
        size="small"
        closable
        @click:close="emit('remove', entry.value)"
      />
    </template>
    <v-menu v-else open-on-hover open-on-click :close-on-content-click="false" location="bottom start">
      <template #activator="{ props }">
        <v-chip
          v-bind="props"
          :text="`${entries.length} selected`"
          color="primary"
          variant="flat"
          size="small"
          class="font-weight-semibold"
        />
      </template>
      <v-card class="selection-list">
        <v-chip
          v-for="entry in entries"
          :key="String(entry.value)"
          :text="entry.title"
          size="small"
          closable
          @click:close="emit('remove', entry.value)"
        />
      </v-card>
    </v-menu>
  </div>
</template>

<style scoped>
.selection-chips {
  position: relative;
  display: flex;
  flex-wrap: nowrap;
  gap: 4px;
  min-width: 0;
}

.measure {
  position: absolute;
  top: 0;
  left: 0;
  display: flex;
  gap: 4px;
  width: max-content;
  visibility: hidden;
  pointer-events: none;
}

.selection-list {
  display: flex;
  flex-wrap: wrap;
  gap: 4px;
  max-width: min(360px, calc(100vw - 24px));
  padding: 8px;
}
</style>
