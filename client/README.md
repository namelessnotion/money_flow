# Money Flow client

Vue 3 and TypeScript client for the Ruby GraphQL API. Apollo Client sends
requests to `/graphql`.

Start the Ruby server first:

```sh
cd ../ruby
bin/server
```

Then run the client:

```sh
npm install
npm run dev
```

Vite proxies `/graphql` to `http://localhost:9292` by default. Set
`VITE_RUBY_SERVER_URL` to change the development proxy target, or
`VITE_GRAPHQL_URL` to make Apollo Client use an explicit GraphQL URL.
