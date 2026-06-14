# TODO

Minimum remaining work for the library:

1. Reconstruct yielded sessions from the session store.

   `Replay(ctx, sessionID)` should load the session record when the session is
   not already in memory, recreate the Extism plugin instance, rebuild the
   dispatcher chain, and replay from the original request.

2. Keep session persistence interface-only.

   `SessionStore` is root-owned and stores only data needed to reconstruct a
   yielded session:

   - session key;
   - guest data;
   - original `PlayRequest`;
   - yielded call.

   Do not add concrete persistent store implementations to this library yet.

Out of scope for this library:

- concrete persistent store implementations;
- dispatching calls to other guests;
- schedulers, queues, engines, or product-specific workflow code.
