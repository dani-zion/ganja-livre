import { gql } from './generated'

export const ProductsDocument = gql(`
  query Products($first: Int) {
    products(first: $first) {
      totalCount
      edges {
        node {
          id
          name
          price
          category
        }
      }
    }
  }
`)
