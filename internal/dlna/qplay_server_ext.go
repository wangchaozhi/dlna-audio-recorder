package dlna

// This file intentionally keeps QPlay-specific state adjacent to the protocol adapter.
// Server embeds qplayState via fields declared in server.go; the separate type documents
// the queue model used by the adapter and keeps future QPlay-only state isolated.
type qplayState struct{}
