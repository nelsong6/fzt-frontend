package frontend

// identityClaims is the baked-in claim set for each known identity.
//
// Source-of-truth for "who can mint a JWT for api.romaine.life." The
// identities are Nelson's personal / Engineered Arts / R1 accounts; they
// change never. Baking them in means:
//   - No identities.json file is required at runtime
//   - Adding/editing an identity is a source change + CI cascade, visible
//     in git history
//   - romaine-api.py (the Python equivalent for ad-hoc calls) is a separate
//     consumer and keeps its own copy
//
// The load-* command names are already baked in similarly (see the
// FrontendCommands list in fzt-automate/main.go).
var identityClaims = map[string]IdentityClaims{
	"nelson":    {Sub: "nelson", Email: "nelson-devops-project@outlook.com", Name: "personal", Role: "member"},
	"nelson-ea": {Sub: "nelson-ea", Email: "n.romaine@engineeredarts.com", Name: "ea", Role: "member"},
	"nelson-r1": {Sub: "nelson-r1", Email: "gromaine@r1rcm.com", Name: "r1", Role: "member"},
}
