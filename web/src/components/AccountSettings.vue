<script setup lang="ts">
import { ref } from "vue";
import { api } from "../api";
import type { User } from "../types";
defineProps<{ user: User }>();
const emit = defineEmits<{ changed: [] }>();
const current = ref(""),
  password = ref(""),
  confirmation = ref(""),
  busy = ref(false),
  error = ref("");
async function save() {
  error.value = "";
  if (password.value !== confirmation.value) {
    error.value = "The new passwords do not match.";
    return;
  }
  busy.value = true;
  try {
    await api("/auth/password", "POST", {
      currentPassword: current.value,
      newPassword: password.value,
    });
    current.value = password.value = confirmation.value = "";
    emit("changed");
  } catch (e) {
    error.value = (e as Error).message;
  } finally {
    busy.value = false;
  }
}
</script>
<template>
  <section class="settings-card">
    <p class="eyebrow">YOUR ACCOUNT</p>
    <h2>{{ user.displayName }}</h2>
    <p>
      @{{ user.username }} ·
      {{ user.role === "admin" ? "Administrator" : "Member" }}
    </p>
    <p v-if="user.proxy">
      Your sign-in and password are managed by your identity provider.
    </p>
    <form v-else class="settings-form" @submit.prevent="save">
      <h3>
        {{
          user.mustChangePassword
            ? "Choose your own password"
            : "Change password"
        }}
      </h3>
      <p v-if="user.mustChangePassword">
        Replace your temporary password before opening your library.
      </p>
      <label
        >Current password<input
          v-model="current"
          type="password"
          autocomplete="current-password"
          required
          maxlength="1024" /></label
      ><label
        >New password<input
          v-model="password"
          type="password"
          autocomplete="new-password"
          required
          minlength="12"
          maxlength="1024" /></label
      ><label
        >Confirm new password<input
          v-model="confirmation"
          type="password"
          autocomplete="new-password"
          required
          minlength="12"
          maxlength="1024"
      /></label>
      <p>Use at least 12 characters. This signs out all your sessions.</p>
      <p v-if="error" class="error" role="alert">{{ error }}</p>
      <button class="primary" :disabled="busy">
        {{ busy ? "Saving…" : "Save password and sign out" }}
      </button>
    </form>
  </section>
</template>
