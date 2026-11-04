package platforminstall

import (
	"fmt"
	"path/filepath"
)

// RefuseUnboundProviderTransport rejects delegated launches on transports that
// cannot enforce the approved executable in their execution domain. Every
// supplied filesystem city identity is checked; display names are ignored.
// Missing or malformed surviving approval inputs remain errors.
func RefuseUnboundProviderTransport(transport string, cities ...string) error {
	for _, city := range cities {
		if !filepath.IsAbs(city) {
			continue
		}
		delegated, err := CityDelegatesProviderApproval(city)
		if err != nil {
			return fmt.Errorf("%s: provider allowlist: %w", transport, err)
		}
		if delegated {
			return fmt.Errorf("%s: provider allowlist delegation refuses launches: transport cannot enforce the approved executable", transport)
		}
	}
	return nil
}
