<script setup lang="ts" generic="T">
// v-autocomplete with the look used across the application: outlined, and with several chosen values
// shown on one line, collapsed into a count with a list on hover when they do not fit.
// Other props, events and slots are passed through to v-autocomplete.
import { computed, ref, useSlots } from 'vue'
import SelectionChips, { type SelectionEntry } from './SelectionChips.vue'

const props = withDefaults(
  defineProps<{
    modelValue?: T
    items?: readonly unknown[]
    itemTitle?: string
    itemValue?: string
    multiple?: boolean
    variant?: 'outlined' | 'filled' | 'underlined' | 'plain' | 'solo' | 'solo-filled' | 'solo-inverted'
  }>(),
  { modelValue: undefined, items: () => [], itemTitle: 'title', itemValue: 'value', variant: 'outlined' },
)
const emit = defineEmits<{ 'update:modelValue': [value: T] }>()

const focused = ref(false)
const slots = useSlots()
const forwarded = computed(() => Object.keys(slots).filter((name) => !(props.multiple && name === 'chip')))

function field(item: unknown, key: string) {
  return item !== null && typeof item === 'object' ? (item as Record<string, unknown>)[key] : item
}

const entries = computed<SelectionEntry[]>(() => {
  const chosen = Array.isArray(props.modelValue) ? (props.modelValue as unknown[]) : []
  return chosen.map((value) => {
    const item = props.items.find((candidate) => field(candidate, props.itemValue) === value)
    return { value, title: String(item === undefined ? value : field(item, props.itemTitle)) }
  })
})

function remove(value: unknown) {
  const chosen = Array.isArray(props.modelValue) ? (props.modelValue as unknown[]) : []
  emit('update:modelValue', chosen.filter((other) => other !== value) as T)
}
</script>

<template>
  <v-autocomplete
    :model-value="modelValue"
    :items="items"
    :item-title="itemTitle"
    :item-value="itemValue"
    :multiple="multiple"
    :chips="multiple"
    :variant="variant"
    :class="{ 'one-line': multiple }"
    @update:focused="focused = $event"
    @update:model-value="emit('update:modelValue', $event)"
  >
    <template v-for="name in forwarded" #[name]="slotProps">
      <slot :name="name" v-bind="slotProps ?? {}" />
    </template>
    <template v-if="multiple" #chip="{ index }">
      <SelectionChips v-if="index === 0" :entries="entries" :focused="focused" @remove="remove" />
    </template>
  </v-autocomplete>
</template>

<style scoped>
/* The chips and the search input share one line, so the field keeps its height. */
.one-line :deep(.v-field__input) {
  flex-wrap: nowrap;
}

.one-line :deep(.v-field input) {
  min-width: 0;
}
</style>
