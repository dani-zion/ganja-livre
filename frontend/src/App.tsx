import { useQuery } from '@tanstack/react-query'
import { ProductsDocument } from './graphql/documents'
import { api } from './lib/api'

export default function App() {
  const { data, isPending, error } = useQuery({
    queryKey: ['products'],
    queryFn: () => api.request(ProductsDocument, { first: 10 }),
  })

  return (
    <main>
      <h1>Ganja Livre</h1>
      <section>
        <h2>Produtos ({data?.products.totalCount ?? 0})</h2>
        {isPending && <p className="muted">Carregando…</p>}
        {error && <p className="error">API indisponível: {String(error)}</p>}
        {data && (
          <ul>
            {data.products.edges.map(({ node }) => (
              <li key={node.id}>
                <span>{node.name}</span>
                <span className="muted">
                  {node.category} · R$ {node.price.toFixed(2)}
                </span>
              </li>
            ))}
          </ul>
        )}
      </section>
    </main>
  )
}
