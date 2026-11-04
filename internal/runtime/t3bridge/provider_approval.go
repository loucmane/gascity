package t3bridge

import "github.com/gastownhall/gascity/internal/platforminstall"

// refuseDelegatedLaunch checks every supplied city identity. T3 selects its
// own provider executable over RPC and cannot consume the approved command.
// A missing or invalid surviving delegation must not restore that RPC path.
func (p *Provider) refuseDelegatedLaunch(cities ...string) error {
	return platforminstall.RefuseUnboundProviderTransport("t3bridge", append([]string{p.cityPath}, cities...)...)
}
