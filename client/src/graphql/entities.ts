import { gql } from '@apollo/client/core'

export const PAGE_SIZE = 20

export const ENTITIES_QUERY = gql`
  query Entities($first: Int, $after: String) {
    entities(first: $first, after: $after) {
      edges {
        cursor
        node {
          id
          name
          holderUuid
          createdAt
        }
      }
      pageInfo {
        hasNextPage
        endCursor
      }
    }
  }
`

export interface EntityNode {
  id: string
  name: string
  holderUuid: string
  createdAt: string
}

interface EntityEdge {
  cursor: string
  node: EntityNode | null
}

export interface EntitiesQueryResult {
  entities: {
    edges: EntityEdge[]
    pageInfo: {
      hasNextPage: boolean
      endCursor: string | null
    }
  }
}

export interface EntitiesQueryVariables {
  first?: number
  after?: string | null
}

export const ONBOARD_ENTITY_MUTATION = gql`
  mutation OnboardEntity($name: String!) {
    onboardEntity(name: $name) {
      entity {
        id
        name
        holderUuid
        createdAt
      }
    }
  }
`

export interface OnboardEntityMutationResult {
  onboardEntity: {
    entity: EntityNode | null
  } | null
}

export interface OnboardEntityMutationVariables {
  name: string
}
