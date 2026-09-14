// Package contract holds the request and response types generated from
// packages/api-contract/openapi.yaml.
//
// Handlers stay handwritten on net/http and ServeMux. These types exist so
// Go and TypeScript cannot each grow a private copy of the same payload and
// drift. Do not edit types.gen.go; change the OpenAPI document and run
// `task contract:generate`.
package contract

