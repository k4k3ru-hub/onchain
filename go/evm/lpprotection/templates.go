package lpprotection

// Reviewed bytecode fingerprints; provenance and immutable meaning are in README.md.
func template(name string) codeTemplate {
	switch name {
	case "vault":
		return codeTemplate{length: 10973, digest: "bc1e63cbeb10aebceb42eb55442da017e4dc698951a4f1471af89d931666113d", fields: map[string][]int{
			"factory":     {758, 1931, 2465, 3797, 4507, 5401},
			"key":         {797, 3929},
			"beneficiary": {617, 4067},
			"deployer":    {514, 4707},
			"createdAt":   {1037},
		}}
	case "clLocker":
		return codeTemplate{length: 10584, digest: "3e837d26af2a2069d0cb675d34030aac097e4ef1db05eff173fd1fbc5461e89c", fields: map[string][]int{
			"voter":       {795, 1884, 3439, 3763, 3910},
			"factory":     {1025, 4402, 6637},
			"rewardToken": {1291, 4066, 4127, 6508, 7041, 7261, 7409},
			"poolType":    {954},
			"root":        {1205, 3679},
			"manager":     {896, 2219, 3074, 5154, 5206, 5361, 5558, 5749, 5801, 7972, 8293},
		}}
	case "clFactory":
		return codeTemplate{length: 12223, digest: "1cb81f8b24806d4f55e69cdac3918814e41f945ad65e23af9ae5de693b2481b4", fields: map[string][]int{
			"voter":          {732},
			"poolLauncher":   {669, 2185, 3055, 6027, 6084, 6797},
			"implementation": {1210, 2854},
			"poolType":       {1081},
			"manager":        {1042, 3136, 3272, 5489, 5868, 6460, 6673, 6955, 7114, 8067},
			"poolFactory":    {1171, 7315},
		}}
	default:
		return codeTemplate{}
	}
}
