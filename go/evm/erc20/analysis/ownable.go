package analysis

import "encoding/json"

const ownableFile = "@openzeppelin/contracts/access/Ownable.sol"

func recognizeOwnable(c astNode, sources map[string]outputSource) (string, map[string]bool, bool) {
	reject := func() (string, map[string]bool, bool) { return "", nil, false }
	if c.Abstract || c.ContractKind != "contract" || len(c.Nodes) < 1 || len(c.Nodes) > 2 {
		return reject()
	}
	find := func(file, name string) astNode {
		for _, n := range sources[file].AST.Nodes {
			if n.NodeType == "ContractDefinition" && n.Name == name {
				return n
			}
		}
		return astNode{}
	}
	erc, own := find(erc20File, "ERC20"), find(ownableFile, "Ownable")
	if erc.ID == 0 || own.ID == 0 {
		return reject()
	}
	seen := map[int]bool{}
	for _, b := range c.BaseContracts {
		id := b.BaseName.ReferencedDeclaration
		if len(b.Arguments) != 0 || seen[id] || (id != erc.ID && id != own.ID) {
			return reject()
		}
		seen[id] = true
	}
	var ctor, mint *astNode
	for i := range c.Nodes {
		n := &c.Nodes[i]
		if n.NodeType != "FunctionDefinition" || n.Virtual || len(n.Overrides) != 0 || n.StateMutability != "nonpayable" || n.Body == nil || n.Body.NodeType != "Block" {
			return reject()
		}
		switch {
		case n.Kind == "constructor" && ctor == nil:
			ctor = n
		case n.Kind == "function" && n.Name == "mint" && mint == nil:
			mint = n
		default:
			return reject()
		}
	}
	if ctor == nil || !emptyParameters(ctor.Parameters) || len(ctor.Modifiers) != 2 || len(ctor.Body.Statements) > 1 {
		return reject()
	}
	seen = map[int]bool{}
	for _, m := range ctor.Modifiers {
		if m.Kind != "baseConstructorSpecifier" || m.ModifierName == nil {
			return reject()
		}
		id := m.ModifierName.ReferencedDeclaration
		if seen[id] {
			return reject()
		}
		seen[id] = true
		switch id {
		case erc.ID:
			if len(m.Arguments) != 2 {
				return reject()
			}
			for _, a := range m.Arguments {
				if a.NodeType != "Literal" || a.Kind != "string" {
					return reject()
				}
			}
		case own.ID:
			if len(m.Arguments) != 1 || !msgSender(m.Arguments[0]) {
				return reject()
			}
		default:
			return reject()
		}
	}
	mintID, onlyOwnerID := 0, 0
	for _, n := range erc.Nodes {
		if n.NodeType == "FunctionDefinition" && n.Name == "_mint" {
			mintID = n.ID
		}
	}
	for _, n := range own.Nodes {
		if n.NodeType == "ModifierDefinition" && n.Name == "onlyOwner" {
			onlyOwnerID = n.ID
		}
	}
	if mintID == 0 || onlyOwnerID == 0 {
		return reject()
	}
	for _, s := range ctor.Body.Statements {
		args, ok := mintArguments(s, mintID)
		if !ok || !msgSender(args[0]) || args[1].NodeType != "Literal" || args[1].Kind != "number" {
			return reject()
		}
	}
	if mint == nil {
		return "openzeppelin-erc20-ownable-v5.5-v1", nil, true
	}
	if (mint.Visibility != "public" && mint.Visibility != "external") || !emptyParameters(mint.ReturnParameters) || len(mint.Modifiers) != 1 || len(mint.Body.Statements) != 1 {
		return reject()
	}
	mod := mint.Modifiers[0]
	if mod.Kind != "modifierInvocation" || mod.ModifierName == nil || mod.ModifierName.ReferencedDeclaration != onlyOwnerID || len(mod.Arguments) != 0 {
		return reject()
	}
	var params struct {
		Parameters []astNode `json:"parameters"`
	}
	if json.Unmarshal(mint.Parameters, &params) != nil || len(params.Parameters) != 2 {
		return reject()
	}
	for i, want := range []string{"address", "uint256"} {
		p := params.Parameters[i]
		if p.NodeType != "VariableDeclaration" || p.TypeName == nil || p.TypeName.NodeType != "ElementaryTypeName" || p.TypeName.Name != want {
			return reject()
		}
	}
	args, ok := mintArguments(mint.Body.Statements[0], mintID)
	if !ok {
		return reject()
	}
	for i := range args {
		if args[i].NodeType != "Identifier" || args[i].ReferencedDeclaration != params.Parameters[i].ID {
			return reject()
		}
	}
	return "openzeppelin-erc20-ownable-mint-v5.5-v1", nil, true
}

func emptyParameters(raw json.RawMessage) bool {
	var p struct {
		Parameters []astNode `json:"parameters"`
	}
	return len(raw) > 0 && json.Unmarshal(raw, &p) == nil && p.Parameters != nil && len(p.Parameters) == 0
}

func msgSender(n astNode) bool {
	return n.NodeType == "MemberAccess" && n.MemberName == "sender" && n.Expression != nil && n.Expression.NodeType == "Identifier" && n.Expression.Name == "msg"
}

func mintArguments(s astNode, id int) ([]astNode, bool) {
	c := s.Expression
	if s.NodeType != "ExpressionStatement" || c == nil || c.NodeType != "FunctionCall" || c.Kind != "functionCall" || c.Expression == nil || c.Expression.NodeType != "Identifier" || c.Expression.ReferencedDeclaration != id || len(c.Arguments) != 2 {
		return nil, false
	}
	return c.Arguments, true
}
