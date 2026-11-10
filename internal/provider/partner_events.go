package provider

// Event names remain compatible with existing GUI, TUI and provider call sites.
const (
	PartnerShown  = "shown"
	PartnerOpened = "opened"
	PartnerKeys   = "keys"
	PartnerAdded  = "added"
)

// CountPartner does nothing: this fork collects no sponsor interaction telemetry.
func CountPartner(what string, ids ...string) {}
