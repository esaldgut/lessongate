## Lesson: lazy-init segregated by use

When wiring multiple clients, segregate sync.Once by real use path. In our
deployment under account 123456789012 the federated signup flow only needs the
auth client, not Mongo or EventBridge.
