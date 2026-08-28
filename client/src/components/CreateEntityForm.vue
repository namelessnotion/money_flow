<script setup lang="ts">
import { ref } from 'vue'
import { useMutation } from '@vue/apollo-composable'
import {
  ENTITIES_QUERY,
  ONBOARD_ENTITY_MUTATION,
  PAGE_SIZE,
  type OnboardEntityMutationResult,
  type OnboardEntityMutationVariables,
} from '../graphql/entities'

const name = ref('')

const { mutate: onboardEntity, loading, error } = useMutation<
  OnboardEntityMutationResult,
  OnboardEntityMutationVariables
>(ONBOARD_ENTITY_MUTATION, {
  throws: 'never',
  refetchQueries: [{ query: ENTITIES_QUERY, variables: { first: PAGE_SIZE } }],
})

async function handleSubmit(): Promise<void> {
  const trimmedName = name.value.trim()
  if (!trimmedName) return

  const result = await onboardEntity({ variables: { name: trimmedName } })
  if (result?.data?.onboardEntity?.entity) {
    name.value = ''
  }
}
</script>

<template>
  <section class="mb-6 w-full max-w-3xl">
    <form class="flex items-start gap-3" @submit.prevent="handleSubmit">
      <div class="flex-1">
        <label for="entity-name" class="sr-only">Entity name</label>
        <input
          id="entity-name"
          v-model="name"
          type="text"
          placeholder="Entity name"
          class="w-full rounded-lg border border-slate-300 px-3 py-2 text-slate-900 focus:border-blue-700 focus:outline-none"
        />
      </div>
      <button
        type="submit"
        :disabled="loading || !name.trim()"
        class="shrink-0 rounded-lg bg-blue-700 px-4 py-2 font-bold text-white hover:bg-blue-800 disabled:cursor-not-allowed disabled:opacity-60"
      >
        {{ loading ? 'Adding…' : 'Add Entity' }}
      </button>
    </form>

    <p v-if="error" class="mt-2 text-sm text-rose-700">Unable to add entity: {{ error.message }}</p>
  </section>
</template>
