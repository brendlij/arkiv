<script setup lang="ts">
defineProps<{ start: string; end: string }>();
const emit = defineEmits<{
  "update:start": [value: string];
  "update:end": [value: string];
}>();
function preset(days: number) {
  const d = new Date(Date.now() + days * 86400000);
  d.setMinutes(d.getMinutes() - d.getTimezoneOffset());
  emit("update:end", d.toISOString().slice(0, 16));
}
</script>
<template>
  <fieldset class="share-schedule">
    <legend>When can they access it?</legend>
    <label
      >Starts<input
        type="datetime-local"
        :value="start"
        @input="
          emit('update:start', ($event.target as HTMLInputElement).value)
        " /></label
    ><small
      >Leave empty to start immediately. Times use your local timezone.</small
    ><label
      >Expires<input
        type="datetime-local"
        :value="end"
        @input="emit('update:end', ($event.target as HTMLInputElement).value)"
    /></label>
    <div class="schedule-presets">
      <button type="button" class="button" @click="preset(1)">1 day</button
      ><button type="button" class="button" @click="preset(7)">7 days</button
      ><button type="button" class="button" @click="preset(30)">30 days</button
      ><button
        type="button"
        class="text-button"
        @click="emit('update:end', '')"
      >
        No expiry
      </button>
    </div>
    <small v-if="!end">Access lasts until you revoke it.</small>
  </fieldset>
</template>
