<script setup lang="ts">
defineProps<{ full?: boolean; compact?: boolean }>();
</script>
<template>
  <img
    src="/brand/arkivlogotransparentbg.png"
    alt="Arkiv"
    class="arkiv-logo"
    :class="{
      'arkiv-logo-compact': compact,
      'arkiv-logo-full': full,
    }"
  />
</template>
