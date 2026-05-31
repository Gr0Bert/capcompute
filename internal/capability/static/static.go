package static

import "capcompute/capability"

// StaticBroker authorizes commands from an in-memory grant list.
type StaticBroker struct {
	grants []capability.Grant
}

var _ capability.Broker = (*StaticBroker)(nil)

// NewStaticBroker creates a deny-by-default broker backed by static grants.
func NewStaticBroker(grants []capability.Grant) *StaticBroker {
	copied := append([]capability.Grant(nil), grants...)
	return &StaticBroker{grants: copied}
}

func (b *StaticBroker) Authorize(req capability.Request) capability.Decision {
	for _, grant := range b.grants {
		if grant.PrincipalType == req.Principal.Type &&
			grant.PrincipalID == req.Principal.ID &&
			grant.ModuleDigest == req.Module.Digest &&
			grant.CommandName == req.Command.Name &&
			matches(grant.SourceType, req.Source.Type) &&
			matches(grant.SourceID, req.Source.ID) &&
			matches(grant.CommandID, req.Command.ID) &&
			matches(string(grant.CommandMode), string(req.Command.Mode)) &&
			matches(grant.CommandArgsSHA, req.CommandArgsSHA) {
			return capability.Decision{Allowed: true}
		}
	}
	return capability.Decision{Allowed: false, Reason: "no matching grant"}
}

func matches(grantValue string, requestValue string) bool {
	return grantValue == "" || grantValue == requestValue
}
