import { describe, expect, it, vi } from 'vitest'
import { mount } from '@vue/test-utils'
import { ApolloClient, InMemoryCache } from '@apollo/client/core'
import { MockLink } from '@apollo/client/testing'
import { DefaultApolloClient } from '@vue/apollo-composable'
import CreateEntityForm from './CreateEntityForm.vue'
import { ENTITIES_QUERY, ONBOARD_ENTITY_MUTATION, PAGE_SIZE } from '../graphql/entities'

function mountForm(mocks: ConstructorParameters<typeof MockLink>[0]) {
  const client = new ApolloClient({
    link: new MockLink(mocks, { defaultOptions: { delay: 0 } }),
    cache: new InMemoryCache(),
  })

  return mount(CreateEntityForm, {
    global: {
      provide: {
        [DefaultApolloClient as unknown as string]: client,
      },
    },
  })
}

describe('CreateEntityForm', () => {
  it('disables submit until a name is entered', async () => {
    const wrapper = mountForm([])

    const button = wrapper.get('button')
    expect(button.attributes('disabled')).toBeDefined()

    await wrapper.get('input').setValue('Shining Knight Industries')
    expect(button.attributes('disabled')).toBeUndefined()
  })

  it('submits the entered name and clears the input on success', async () => {
    const wrapper = mountForm([
      {
        request: {
          query: ONBOARD_ENTITY_MUTATION,
          variables: { name: 'Shining Knight Industries' },
        },
        result: {
          data: {
            onboardEntity: {
              entity: {
                id: '1',
                name: 'Shining Knight Industries',
                holderUuid: '018f4d2e-0000-7000-8000-000000000000',
                createdAt: '2026-08-27T00:00:00Z',
              },
            },
          },
        },
      },
      {
        request: { query: ENTITIES_QUERY, variables: { first: PAGE_SIZE } },
        result: { data: { entities: { edges: [], pageInfo: { hasNextPage: false, endCursor: null } } } },
      },
    ])

    await wrapper.get('input').setValue('Shining Knight Industries')
    await wrapper.get('form').trigger('submit.prevent')

    await vi.waitFor(() => {
      expect((wrapper.get('input').element as HTMLInputElement).value).toBe('')
    })
    expect(wrapper.find('.text-rose-700').exists()).toBe(false)
  })

  it('shows an error message when the mutation fails', async () => {
    const wrapper = mountForm([
      {
        request: {
          query: ONBOARD_ENTITY_MUTATION,
          variables: { name: 'Shining Knight Industries' },
        },
        error: new Error('boom'),
      },
    ])

    await wrapper.get('input').setValue('Shining Knight Industries')
    await wrapper.get('form').trigger('submit.prevent')

    await vi.waitFor(() => {
      expect(wrapper.text()).toContain('boom')
    })
    expect((wrapper.get('input').element as HTMLInputElement).value).toBe('Shining Knight Industries')
  })
})
