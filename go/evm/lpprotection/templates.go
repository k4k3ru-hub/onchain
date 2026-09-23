package lpprotection

// Reviewed bytecode fingerprints; provenance and immutable meaning are in README.md.
func template(name string) codeTemplate {
	switch name {
	case "v4LaunchLocker":
		return codeTemplate{length: 5757, digest: "32562f683e5e70e56c01b78668929412d5fb9d7d9dc644f3fc4821bccab978f0", fields: map[string][]int{
			"manager": {797, 1892, 2656, 3433, 3529},
			"factory": {1208, 1782},
		}}
	case "v4LaunchFactory":
		return codeTemplate{length: 13681, digest: "f194573d2926cf2826ccdada4eb8b6a136844c3dcca58802862b4cfeb6cde8be", fields: map[string][]int{
			"poolManager": {205, 1332},
			"manager":     {1648, 1789, 2290, 2386, 4793},
			"permit2":     {1482, 1571, 1721, 2464, 5518},
			"locker":      {277, 2047, 2682, 2801, 2955, 3663, 3800},
		}}
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
