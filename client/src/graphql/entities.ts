import { gql } from '@apollo/client/core'

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
