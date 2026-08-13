// Package query provides read-only query services for the application layer.
// These services do NOT mutate state — they are called by HTTP handlers for
// list/detail endpoints that return data without side effects.
//
// TODO: implement as usecases are wired (Phase 4 HTTP handler migration).
package query
