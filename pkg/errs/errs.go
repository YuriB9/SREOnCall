// Package errs holds sentinel errors shared across all services so that
// errors.Is works both inside a service and across the network boundary.
//
// Each service's store package previously declared its own ErrNotFound, which
// produced four distinct values: errors.Is between services could never match.
// Aliasing the store sentinels to these shared values gives the monorepo a
// single canonical "not found"/"conflict" that HTTP clients can return and
// callers can test for with errors.Is.
package errs

import "errors"

// ErrNotFound is the canonical "record does not exist" sentinel. HTTP clients
// map a 404 response to this value so callers can errors.Is across services.
var ErrNotFound = errors.New("not found")

// ErrConflict is the canonical "conflicting state" sentinel. HTTP clients map a
// 409 response to this value.
var ErrConflict = errors.New("conflict")
