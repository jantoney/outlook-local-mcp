package auth

import "sync"

// accountsMutationMu serializes read-modify-write account-file mutations in
// this process so concurrent policy updates cannot overwrite one another.
var accountsMutationMu sync.Mutex
