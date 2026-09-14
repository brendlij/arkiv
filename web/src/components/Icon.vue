<script setup lang="ts">
defineProps<{ name: string }>();
const paths: Record<string, string> = {
  map: "M3 5l6-2 6 2 6-2v16l-6 2-6-2-6 2z M9 3v16 M15 5v16",
  photos: "M4 4h16v16H4z M4 15l5-5 4 4 3-3 4 4 M15 8h.01",
  albums: "M3 7h18v13H3z M6 4h12",
  heart:
    "M20.8 4.6a5.5 5.5 0 0 0-7.8 0L12 5.7l-1.1-1.1a5.5 5.5 0 0 0-7.8 7.8L12 21l8.8-8.6a5.5 5.5 0 0 0 0-7.8z",
  folder: "M3 5h7l2 3h9v12H3z",
  inbox: "M4 4h16l2 12v4H2v-4z M2 16h6l2 2h4l2-2h6",
  search: "M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0",
  sun: "M12 2v2 M12 20v2 M2 12h2 M20 12h2 M5 5l1.5 1.5 M17.5 17.5L19 19 M5 19l1.5-1.5 M17.5 6.5L19 5 M16 12a4 4 0 1 1-8 0 4 4 0 0 1 8 0",
  moon: "M20.8 13A9 9 0 0 1 11 3.2 9 9 0 1 0 20.8 13",
  close: "M6 6l12 12 M18 6L6 18",
  left: "M15 5l-7 7 7 7",
  right: "M9 5l7 7-7 7",
  plus: "M12 5v14 M5 12h14",
  minus: "M5 12h14",
  albumAdd: "M3 7h14v13H3z M6 4h8 M20 3v8 M16 7h8",
  info: "M12 11v6 M12 7h.01 M22 12a10 10 0 1 1-20 0 10 10 0 0 1 20 0",
  expand: "M8 3H3v5 M16 3h5v5 M3 16v5h5 M21 16v5h-5",
  zoom: "M21 21l-5-5 M18 10a8 8 0 1 1-16 0 8 8 0 0 1 16 0 M10 6v8 M6 10h8",
  download: "M12 3v12 M7 10l5 5 5-5 M4 16v5h16v-5",
  refresh: "M20 7V3l-3 3 M4 17v4l3-3 M20 7a9 9 0 0 0-15-2 M4 17a9 9 0 0 0 15 2",
  logout: "M9 4H4v16h5 M10 12h11 M17 8l4 4-4 4",
  play: "M8 4l12 8-12 8z",
  pause: "M8 5v14 M16 5v14",
};
</script>
<template>
  <svg
    class="gallery-icon"
    viewBox="0 0 24 24"
    fill="none"
    stroke="currentColor"
    stroke-width="1.6"
    stroke-linecap="round"
    stroke-linejoin="round"
    aria-hidden="true"
  >
    <path :d="paths[name] || paths.photos" />
  </svg>
</template>
