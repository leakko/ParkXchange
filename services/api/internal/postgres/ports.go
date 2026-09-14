package postgres

import (
	"github.com/marco/parkxchange/services/api/internal/accounts"
	"github.com/marco/parkxchange/services/api/internal/reservations"
	"github.com/marco/parkxchange/services/api/internal/spots"
)

// Compile-time proof that this adapter still satisfies every port it is wired
// into.
//
// These assertions are the reason the dependency direction is safe to rely on:
// the services never mention this package, so nothing else would notice if a
// port gained a method or changed a signature. Without them the mismatch
// surfaces in cmd/api, where the compiler reports it against the wiring rather
// than against the method that is actually wrong.
var (
	_ accounts.Store     = (*DB)(nil)
	_ spots.Store        = (*DB)(nil)
	_ reservations.Store = (*DB)(nil)
)
