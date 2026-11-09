<script setup lang="ts">
// The filters in use, laid out as PatternFly shows them under a toolbar: a group of chips for each
// category, with the category's name and a button that removes the whole group, and "Clear all
// filters" at the end. Groups without chips are left out, and nothing shows when no filter is set.
import { computed } from 'vue'

export interface FilterGroup {
  key: string
  label: string
  chips: { value: string; title: string }[]
}

const props = defineProps<{ groups: FilterGroup[] }>()
const emit = defineEmits<{ remove: [key: string, value: string]; clear: [key: string]; clearAll: [] }>()

const shown = computed(() => props.groups.filter((group) => group.chips.length > 0))
</script>

<template>
  <div v-if="shown.length > 0" class="filter-chips" data-testid="applied-filters">
    <div v-for="group in shown" :key="group.key" class="group" role="group" :aria-label="group.label">
      <span class="label">{{ group.label }}</span>
      <v-chip
        v-for="chip in group.chips"
        :key="chip.value"
        size="small"
        closable
        :close-label="`Remove ${chip.title}`"
        @click:close="emit('remove', group.key, chip.value)"
      >
        {{ chip.title }}
      </v-chip>
      <v-btn
        icon="fa-xmark"
        size="x-small"
        variant="text"
        density="comfortable"
        :aria-label="`Clear ${group.label}`"
        @click="emit('clear', group.key)"
      />
    </div>
    <v-btn text="Clear all filters" variant="text" color="primary" size="small" @click="emit('clearAll')" />
  </div>
</template>

<style scoped>
.filter-chips {
  display: flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 8px;
  padding: 0 8px 8px;
}

.group {
  display: inline-flex;
  flex-wrap: wrap;
  align-items: center;
  gap: 4px;
  padding: 2px 2px 2px 8px;
  border: 1px solid rgb(var(--v-border-color));
  border-radius: var(--radius-small);
}

.label {
  margin-right: 4px;
  font-size: 0.875rem;
}
</style>
