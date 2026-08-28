<script setup lang="ts">
import CreateEntityForm from './components/CreateEntityForm.vue'
import EntityList from './components/EntityList.vue'
</script>

<template>
  <main class="flex min-h-screen justify-center bg-slate-50 px-6 py-12">
    <div class="w-full max-w-3xl">
      <CreateEntityForm />
      <EntityList />
    </div>
  </main>
</template>
