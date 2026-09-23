package controls

func unknown(status, reason string) *Observation {
	v := &Observation{}
	v.Ownership.OwnerAddress = StringFinding{Status: status, Reason: reason}
	v.Ownership.Renounced = BoolFinding{Status: status, Reason: reason}
	v.TransferRestrictions.BlacklistPresent = BoolFinding{Status: status, Reason: reason}
	v.TransferRestrictions.AllowlistPresent = BoolFinding{Status: status, Reason: reason}
	v.TransferRestrictions.AutomaticBuyerRestrictionPresent = BoolFinding{Status: status, Reason: reason}
	v.TransferRestrictions.TransferLimitsPresent = BoolFinding{Status: status, Reason: reason}
	v.TransferRestrictions.PausePresent = BoolFinding{Status: status, Reason: reason}
	v.TransferRestrictions.Paused = BoolFinding{Status: status, Reason: reason}
	v.TransferRestrictions.CanChange = BoolFinding{Status: status, Reason: reason}
	v.Minting.Present = BoolFinding{Status: status, Reason: reason}
	v.Minting.CanMint = BoolFinding{Status: status, Reason: reason}
	v.Minting.Authorization = StringFinding{Status: status, Reason: reason}
	v.Minting.RequiresBacking = BoolFinding{Status: status, Reason: reason}
	v.Minting.HasSupplyCap = BoolFinding{Status: status, Reason: reason}
	v.Minting.SupplyCapRaw = StringFinding{Status: status, Reason: reason}
	v.Upgrade.CanUpgrade = BoolFinding{Status: status, Reason: reason}
	v.BalanceControl.CanForceTransfer = BoolFinding{Status: status, Reason: reason}
	v.BalanceControl.CanForceBurn = BoolFinding{Status: status, Reason: reason}
	return v
}
