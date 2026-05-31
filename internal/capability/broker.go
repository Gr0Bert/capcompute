package capability

type Request struct {
	PrincipalType string
	PrincipalID   string
	ModuleDigest  string
	CommandName   string
}

type Decision struct {
	Allowed bool
	Reason  string
}

type Grant struct {
	PrincipalType string
	PrincipalID   string
	ModuleDigest  string
	CommandName   string
}

type StaticBroker struct {
	grants []Grant
}

func NewStaticBroker(grants []Grant) StaticBroker {
	copied := append([]Grant(nil), grants...)
	return StaticBroker{grants: copied}
}

func (b StaticBroker) Authorize(req Request) Decision {
	for _, grant := range b.grants {
		if grant.PrincipalType == req.PrincipalType &&
			grant.PrincipalID == req.PrincipalID &&
			grant.ModuleDigest == req.ModuleDigest &&
			grant.CommandName == req.CommandName {
			return Decision{Allowed: true}
		}
	}
	return Decision{Allowed: false, Reason: "no matching grant"}
}
