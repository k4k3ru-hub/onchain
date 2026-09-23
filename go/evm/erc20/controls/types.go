// Package controls reports reviewed token capabilities at one pinned block.
package controls

type BoolFinding struct {
	Status string `json:"status"`
	Value  *bool  `json:"value"`
	Reason string `json:"reason,omitempty"`
}
type StringFinding struct {
	Status string  `json:"status"`
	Value  *string `json:"value"`
	Reason string  `json:"reason,omitempty"`
}
type Ownership struct {
	OwnerAddress StringFinding `json:"ownerAddress"`
	Renounced    BoolFinding   `json:"renounced"`
}
type TransferRestrictions struct {
	BlacklistPresent                 BoolFinding `json:"blacklistPresent"`
	AllowlistPresent                 BoolFinding `json:"allowlistPresent"`
	AutomaticBuyerRestrictionPresent BoolFinding `json:"automaticBuyerRestrictionPresent"`
	TransferLimitsPresent            BoolFinding `json:"transferLimitsPresent"`
	PausePresent                     BoolFinding `json:"pausePresent"`
	Paused                           BoolFinding `json:"paused"`
	CanChange                        BoolFinding `json:"canChange"`
}
type Minting struct {
	Present         BoolFinding   `json:"present"`
	CanMint         BoolFinding   `json:"canMint"`
	Authorization   StringFinding `json:"authorization"`
	RequiresBacking BoolFinding   `json:"requiresBacking"`
	HasSupplyCap    BoolFinding   `json:"hasSupplyCap"`
	SupplyCapRaw    StringFinding `json:"supplyCapRaw"`
}
type Upgrade struct {
	CanUpgrade BoolFinding `json:"canUpgrade"`
}
type BalanceControl struct {
	CanForceTransfer BoolFinding `json:"canForceTransfer"`
	CanForceBurn     BoolFinding `json:"canForceBurn"`
}
type Observation struct {
	Ownership            Ownership            `json:"ownership"`
	TransferRestrictions TransferRestrictions `json:"transferRestrictions"`
	Minting              Minting              `json:"minting"`
	Upgrade              Upgrade              `json:"upgrade"`
	BalanceControl       BalanceControl       `json:"balanceControl"`
}
