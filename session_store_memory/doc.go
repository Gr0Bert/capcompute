// Package session_store_memory provides an in-memory capcompute.SessionStore.
//
// The store is intended for tests, local runtimes, and wrappers that want the
// standard session lookup behavior without durable persistence. It also exposes
// DeleteSession and ListSessions for application-owned lifecycle management.
package session_store_memory
