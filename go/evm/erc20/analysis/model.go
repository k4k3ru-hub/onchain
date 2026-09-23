package analysis

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strconv"
)

const erc20File = "@openzeppelin/contracts/token/ERC20/ERC20.sol"
const permitFile = "@openzeppelin/contracts/token/ERC20/extensions/ERC20Permit.sol"
const eip712File = "@openzeppelin/contracts/utils/cryptography/EIP712.sol"

func recognizeModel(bundle SourceBundle, outputs map[string]outputSource) (string, map[string]bool, bool) {
	var input sourceInput
	if err := json.Unmarshal(bundle.Input, &input); err != nil {
		return "", nil, false
	}
	for name, source := range input.Sources {
		if name == bundle.ContractFile {
			continue
		}
		if source.Content == nil {
			return "", nil, false
		}
		digest := sha256.Sum256([]byte(*source.Content))
		if reviewedSources[name] != hex.EncodeToString(digest[:]) {
			return "", nil, false
		}
	}
	if _, ok := input.Sources[erc20File]; !ok {
		return "", nil, false
	}
	var contract *astNode
	for _, node := range outputs[bundle.ContractFile].AST.Nodes {
		switch node.NodeType {
		case "PragmaDirective", "ImportDirective":
		case "ContractDefinition":
			if contract != nil || node.Name != bundle.ContractName {
				return "", nil, false
			}
			value := node
			contract = &value
		default:
			return "", nil, false
		}
	}
	if contract != nil && len(contract.BaseContracts) == 2 {
		return recognizeOwnable(*contract, outputs)
	}
	if contract == nil || contract.Abstract || contract.ContractKind != "contract" || len(contract.BaseContracts) != 1 || len(contract.Nodes) != 1 {
		return "", nil, false
	}
	find := func(file, name string) (astNode, bool) {
		for _, node := range outputs[file].AST.Nodes {
			if node.NodeType == "ContractDefinition" && node.Name == name {
				return node, true
			}
		}
		return astNode{}, false
	}
	erc20, ok := find(erc20File, "ERC20")
	if !ok {
		return "", nil, false
	}
	base := contract.BaseContracts[0]
	if len(base.Arguments) != 0 {
		return "", nil, false
	}
	model := "openzeppelin-erc20-v5.5-v1"
	permit, permitOK := find(permitFile, "ERC20Permit")
	isPermit := permitOK && base.BaseName.ReferencedDeclaration == permit.ID
	if base.BaseName.ReferencedDeclaration != erc20.ID && !isPermit {
		return "", nil, false
	}
	constructor := contract.Nodes[0]
	if constructor.NodeType != "FunctionDefinition" || constructor.Kind != "constructor" || constructor.StateMutability != "nonpayable" || constructor.Body == nil || constructor.Body.NodeType != "Block" {
		return "", nil, false
	}
	var parameters struct {
		Parameters []astNode `json:"parameters"`
	}
	if err := json.Unmarshal(constructor.Parameters, &parameters); err != nil || len(parameters.Parameters) != 0 {
		return "", nil, false
	}
	seenERC20, seenPermit := false, false
	for _, modifier := range constructor.Modifiers {
		if modifier.Kind != "baseConstructorSpecifier" || modifier.ModifierName == nil {
			return "", nil, false
		}
		switch modifier.ModifierName.ReferencedDeclaration {
		case erc20.ID:
			if seenERC20 || len(modifier.Arguments) != 2 {
				return "", nil, false
			}
			seenERC20 = true
		case permit.ID:
			if !isPermit || seenPermit || len(modifier.Arguments) != 1 {
				return "", nil, false
			}
			seenPermit = true
		default:
			return "", nil, false
		}
		for _, arg := range modifier.Arguments {
			if arg.NodeType != "Literal" || arg.Kind != "string" {
				return "", nil, false
			}
		}
	}
	if !seenERC20 || seenPermit != isPermit {
		return "", nil, false
	}
	if len(constructor.Body.Statements) > 1 {
		return "", nil, false
	}
	for _, statement := range constructor.Body.Statements {
		call := statement.Expression
		if statement.NodeType != "ExpressionStatement" || call == nil || call.NodeType != "FunctionCall" || call.Kind != "functionCall" || call.Expression == nil || len(call.Arguments) != 2 {
			return "", nil, false
		}
		mintID := 0
		for _, node := range erc20.Nodes {
			if node.NodeType == "FunctionDefinition" && node.Name == "_mint" {
				mintID = node.ID
			}
		}
		if mintID == 0 || call.Expression.NodeType != "Identifier" || call.Expression.ReferencedDeclaration != mintID {
			return "", nil, false
		}
		receiver, amount := call.Arguments[0], call.Arguments[1]
		if receiver.NodeType != "MemberAccess" || receiver.MemberName != "sender" || receiver.Expression == nil || receiver.Expression.NodeType != "Identifier" || receiver.Expression.Name != "msg" {
			return "", nil, false
		}
		if amount.NodeType != "Literal" || amount.Kind != "number" {
			return "", nil, false
		}
	}
	immutables := make(map[string]bool)
	if isPermit {
		model = "openzeppelin-erc20-permit-v5.5-v1"
		// Only the reviewed EIP712 domain cache/name/version immutables are tax
		// independent. Never accept arbitrary compiler-reported substitutions.
		eip712, ok := find(eip712File, "EIP712")
		if !ok {
			return "", nil, false
		}
		allowed := map[string]bool{"_cachedDomainSeparator": true, "_cachedChainId": true, "_cachedThis": true, "_hashedName": true, "_hashedVersion": true, "_name": true, "_version": true}
		for _, node := range eip712.Nodes {
			if node.NodeType == "VariableDeclaration" && node.Mutability == "immutable" {
				if !allowed[node.Name] {
					return "", nil, false
				}
				immutables[strconv.Itoa(node.ID)] = true
			}
		}
	}
	return model, immutables, true
}
