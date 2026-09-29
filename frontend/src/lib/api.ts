import { GraphQLClient } from 'graphql-request'

const endpoint =
  import.meta.env.VITE_GRAPHQL_ENDPOINT ??
  new URL('/query', window.location.origin).href

export const api = new GraphQLClient(endpoint)
