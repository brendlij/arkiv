<script setup lang="ts">
defineProps<{ full?: boolean; compact?: boolean }>();
</script>
<template>
  <svg
    v-if="compact"
    class="arkiv-logo arkiv-logo-compact"
    viewBox="0 0 48 48"
    fill="none"
    role="img"
    aria-label="arkiv"
    xmlns="http://www.w3.org/2000/svg"
  >
    <g
      stroke="currentColor"
      stroke-width="2.4"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <path d="M15 7h20a4 4 0 0 1 4 4v17M11 11h20a4 4 0 0 1 4 4v13" />
      <rect x="7" y="15" width="24" height="20" rx="4" />
      <circle cx="14" cy="22" r="1.8" fill="currentColor" stroke="none" />
      <path d="m10 31 6-6 5 4 4-5 4 5" />
    </g>
    <path
      d="M5 31h10l3 4h12l3-4h10v8a5 5 0 0 1-5 5H10a5 5 0 0 1-5-5z"
      fill="currentColor"
    />
    <path
      d="M20 39h8"
      stroke="var(--bg, #F4F1E8)"
      stroke-width="2.2"
      stroke-linecap="round"
    />
  </svg>
  <svg
    v-else-if="!full"
    class="arkiv-logo"
    role="img"
    aria-label="arkiv"
    width="256"
    height="256"
    viewBox="0 0 256 256"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
  >
    <g
      stroke="currentColor"
      stroke-width="10"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <rect x="75" y="42" width="118" height="92" rx="14" />
      <rect x="62" y="58" width="118" height="92" rx="14" />
      <rect x="49" y="75" width="118" height="92" rx="14" />

      <circle cx="82" cy="106" r="10" fill="currentColor" stroke="none" />

      <path d="M61 154L94 126L116 143L139 116L158 133" />

      <path
        d="M38 151L58 133V159H167V133L188 151V198C188 207.941 179.941 216 170 216H56C46.059 216 38 207.941 38 198V151Z"
        fill="currentColor"
        stroke="currentColor"
      />

      <path d="M91 184H136" stroke="#F4F1E8" stroke-width="9" />
    </g>
  </svg>

  <svg
    v-else
    class="arkiv-logo"
    role="img"
    aria-label="arkiv"
    width="420"
    height="420"
    viewBox="0 0 420 420"
    fill="none"
    xmlns="http://www.w3.org/2000/svg"
  >
    <g
      transform="translate(90 40)"
      stroke="currentColor"
      stroke-width="9"
      stroke-linecap="round"
      stroke-linejoin="round"
    >
      <rect x="65" y="22" width="120" height="94" rx="14" />
      <rect x="52" y="39" width="120" height="94" rx="14" />
      <rect x="39" y="56" width="120" height="94" rx="14" />

      <circle cx="74" cy="89" r="10" fill="currentColor" stroke="none" />

      <path d="M51 137L86 108L108 125L132 98L151 116" />

      <path
        d="M28 133L48 116V143H159V116L180 133V180C180 190 172 198 162 198H46C36 198 28 190 28 180V133Z"
        fill="currentColor"
        stroke="currentColor"
      />

      <path d="M81 166H128" stroke="#F4F1E8" stroke-width="9" />
    </g>

    <text
      x="210"
      y="330"
      text-anchor="middle"
      fill="currentColor"
      font-family="Geist, Inter, Arial, sans-serif"
      font-size="92"
      font-weight="700"
      letter-spacing="-5"
    >
      arkiv
    </text>
  </svg>
</template>
