package analysis

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

var compilerVersion = regexp.MustCompile(`^0\.[0-9]{1,3}\.[0-9]{1,3}\+commit\.[0-9a-f]{8}$`)

type sourceInput struct {
	Language string `json:"language"`
	Sources  map[string]struct {
		Content   *string         `json:"content"`
		URLs      json.RawMessage `json:"urls,omitempty"`
		Keccak256 string          `json:"keccak256,omitempty"`
	} `json:"sources"`
	Settings map[string]json.RawMessage `json:"settings"`
}

type runtimeArtifact struct {
	Object              string                     `json:"object"`
	LinkReferences      map[string]json.RawMessage `json:"linkReferences"`
	ImmutableReferences map[string][]struct {
		Start  int `json:"start"`
		Length int `json:"length"`
	} `json:"immutableReferences"`
}

type astNode struct {
	Visibility            string          `json:"visibility"`
	Virtual               bool            `json:"virtual"`
	Overrides             json.RawMessage `json:"overrides"`
	ReturnParameters      json.RawMessage `json:"returnParameters"`
	TypeName              *astNode        `json:"typeName"`
	ID                    int             `json:"id"`
	NodeType              string          `json:"nodeType"`
	Name                  string          `json:"name"`
	Kind                  string          `json:"kind"`
	Abstract              bool            `json:"abstract"`
	ContractKind          string          `json:"contractKind"`
	StateMutability       string          `json:"stateMutability"`
	Mutability            string          `json:"mutability"`
	ReferencedDeclaration int             `json:"referencedDeclaration"`
	MemberName            string          `json:"memberName"`
	Nodes                 []astNode       `json:"nodes"`
	BaseContracts         []struct {
		BaseName  astNode   `json:"baseName"`
		Arguments []astNode `json:"arguments"`
	} `json:"baseContracts"`
	Parameters   json.RawMessage `json:"parameters"`
	Body         *astNode        `json:"body"`
	Statements   []astNode       `json:"statements"`
	Modifiers    []astNode       `json:"modifiers"`
	ModifierName *astNode        `json:"modifierName"`
	Expression   *astNode        `json:"expression"`
	Arguments    []astNode       `json:"arguments"`
}

type outputSource struct {
	AST astNode `json:"ast"`
}

func validVersion(value string) bool { return compilerVersion.MatchString(value) }

func compareVersion(left, right string) int {
	parse := func(v string) []int {
		parts := strings.Split(strings.SplitN(v, "+", 2)[0], ".")
		values := make([]int, 3)
		for i, part := range parts {
			value, err := strconv.Atoi(part)
			if err != nil {
				return nil
			}
			values[i] = value
		}
		return values
	}
	l, r := parse(left), parse(right)
	for i := range l {
		if l[i] > r[i] {
			return 1
		}
		if l[i] < r[i] {
			return -1
		}
	}
	return strings.Compare(left, right)
}

func prepareInput(bundle SourceBundle) (json.RawMessage, error) {
	if !validVersion(bundle.CompilerVersion) || bundle.ContractFile == "" || bundle.ContractName == "" {
		return nil, fmt.Errorf("failed to prepare token source: %w: identity=invalid", ErrUnsupported)
	}
	if len(bundle.Input) > 2*1024*1024 {
		return nil, fmt.Errorf("failed to prepare token source: %w: input=too_long", ErrUnsupported)
	}
	var input sourceInput
	if err := json.Unmarshal(bundle.Input, &input); err != nil {
		return nil, fmt.Errorf("failed to prepare token source: %w", err)
	}
	if input.Language != "Solidity" || len(input.Sources) == 0 || len(input.Sources) > 128 {
		return nil, fmt.Errorf("failed to prepare token source: %w: sources=invalid", ErrUnsupported)
	}
	if _, ok := input.Sources[bundle.ContractFile]; !ok {
		return nil, fmt.Errorf("failed to prepare token source: %w: contract_file=invalid", ErrUnsupported)
	}
	for name, source := range input.Sources {
		if name == "" || len(name) > 2048 || source.Content == nil || len(source.URLs) != 0 {
			return nil, fmt.Errorf("failed to prepare token source: %w: source_content=invalid", ErrUnsupported)
		}
	}
	if input.Settings == nil {
		input.Settings = make(map[string]json.RawMessage)
	}
	if _, ok := input.Settings["modelChecker"]; ok {
		return nil, fmt.Errorf("failed to prepare token source: %w: model_checker=invalid", ErrUnsupported)
	}
	input.Settings["outputSelection"] = json.RawMessage(`{"*":{"*":["evm.deployedBytecode","evm.methodIdentifiers"],"":["ast"]}}`)
	result, err := json.Marshal(input)
	if err != nil {
		return nil, fmt.Errorf("failed to prepare token source: %w", err)
	}
	if len(result) > 2*1024*1024 {
		return nil, fmt.Errorf("failed to prepare token source: %w: input=too_long", ErrUnsupported)
	}
	return result, nil
}

func parseOutput(raw json.RawMessage, bundle SourceBundle) (runtimeArtifact, map[string]outputSource, error) {
	var output struct {
		Errors []struct {
			Severity string `json:"severity"`
		} `json:"errors"`
		Contracts map[string]map[string]struct {
			EVM struct {
				Runtime runtimeArtifact `json:"deployedBytecode"`
			} `json:"evm"`
		} `json:"contracts"`
		Sources map[string]outputSource `json:"sources"`
	}
	if len(raw) > 8*1024*1024 {
		return runtimeArtifact{}, nil, fmt.Errorf("failed to parse compiler output: output=too_long")
	}
	if err := json.Unmarshal(raw, &output); err != nil {
		return runtimeArtifact{}, nil, fmt.Errorf("failed to parse compiler output: %w", err)
	}
	for _, diagnostic := range output.Errors {
		if diagnostic.Severity == "error" {
			return runtimeArtifact{}, nil, fmt.Errorf("failed to compile token source: %w", ErrCompilation)
		}
	}
	artifact := output.Contracts[bundle.ContractFile][bundle.ContractName].EVM.Runtime
	if artifact.Object == "" || output.Sources[bundle.ContractFile].AST.NodeType != "SourceUnit" {
		return runtimeArtifact{}, nil, fmt.Errorf("failed to parse compiler output: artifact=empty")
	}
	if _, err := hex.DecodeString(artifact.Object); err != nil {
		return runtimeArtifact{}, nil, fmt.Errorf("failed to parse compiler runtime: %w", err)
	}
	return artifact, output.Sources, nil
}
