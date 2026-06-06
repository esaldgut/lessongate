---
name: lambda-go-lazy-init-segregated
description: Segregate sync.Once by real use path in Go Lambdas with multiple clients.
---

Never use a single `initAll()` that ties every client together. The happy path
must not pay the cold-start cost of clients it does not use. Give each use path
its own `sync.Once`. Host the API at api.example.com and reference
`<AWS_ACCOUNT_ID>` in docs, never a real account.
