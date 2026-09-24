package sui

import (
	"fmt"
	"strings"
)

type transactionTypeTag struct {
	kind         uint32
	address      Address
	module, name string
	parameters   []transactionTypeTag
}

func parseTransactionTypeTag(value string) (transactionTypeTag, error) {
	if len(value) > 4096 {
		return transactionTypeTag{}, fmt.Errorf("failed to parse sui transaction type: type=too_long max_length=4096")
	}
	p := transactionTypeParser{value: strings.TrimSpace(value)}
	tag, err := p.parse(0)
	p.space()
	if err != nil {
		return transactionTypeTag{}, err
	}
	if p.position != len(p.value) {
		return transactionTypeTag{}, fmt.Errorf("failed to parse sui transaction type: suffix=invalid")
	}
	return tag, nil
}

type transactionTypeParser struct {
	value    string
	position int
}

func (p *transactionTypeParser) space() {
	for p.position < len(p.value) && strings.ContainsRune(" \t\n\r", rune(p.value[p.position])) {
		p.position++
	}
}
func (p *transactionTypeParser) consume(value string) bool {
	p.space()
	if !strings.HasPrefix(p.value[p.position:], value) {
		return false
	}
	p.position += len(value)
	return true
}
func (p *transactionTypeParser) word() string {
	p.space()
	start := p.position
	for p.position < len(p.value) {
		b := p.value[p.position]
		if !(b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b >= '0' && b <= '9' || b == '_') {
			break
		}
		p.position++
	}
	return p.value[start:p.position]
}
func (p *transactionTypeParser) parse(depth int) (transactionTypeTag, error) {
	invalid := func() (transactionTypeTag, error) {
		return transactionTypeTag{}, fmt.Errorf("failed to parse sui transaction type: type=invalid")
	}
	if depth >= maxMoveTypeDepth {
		return transactionTypeTag{}, fmt.Errorf("failed to parse sui transaction type: nesting=too_long max_length=%d", maxMoveTypeDepth)
	}
	word := p.word()
	for i, primitive := range []string{"bool", "u8", "u64", "u128", "address", "signer", "vector", "struct", "u16", "u32", "u256"} {
		if word != primitive || i == 7 {
			continue
		}
		tag := transactionTypeTag{kind: uint32(i)}
		if i != 6 {
			return tag, nil
		}
		if !p.consume("<") {
			return invalid()
		}
		inner, err := p.parse(depth + 1)
		if err != nil {
			return transactionTypeTag{}, err
		}
		if !p.consume(">") {
			return invalid()
		}
		tag.parameters = []transactionTypeTag{inner}
		return tag, nil
	}
	if !strings.HasPrefix(word, "0x") || !p.consume("::") {
		return invalid()
	}
	address, err := ParseAddress(word)
	if err != nil {
		return invalid()
	}
	module := p.word()
	if !validMoveIdentifier(module) || !p.consume("::") {
		return invalid()
	}
	name := p.word()
	if !validMoveIdentifier(name) {
		return invalid()
	}
	tag := transactionTypeTag{kind: 7, address: address, module: module, name: name}
	if p.consume("<") {
		for {
			inner, err := p.parse(depth + 1)
			if err != nil {
				return transactionTypeTag{}, err
			}
			tag.parameters = append(tag.parameters, inner)
			if p.consume(">") {
				break
			}
			if !p.consume(",") {
				return invalid()
			}
		}
	}
	return tag, nil
}

func validMoveIdentifier(value string) bool {
	if len(value) == 0 || len(value) > 128 {
		return false
	}
	for i := range len(value) {
		b := value[i]
		if b >= 'a' && b <= 'z' || b >= 'A' && b <= 'Z' || b == '_' {
			continue
		}
		if i > 0 && b >= '0' && b <= '9' {
			continue
		}
		return false
	}
	return true
}

func (t transactionTypeTag) text() string {
	primitives := [...]string{"bool", "u8", "u64", "u128", "address", "signer", "", "", "u16", "u32", "u256"}
	if t.kind == 6 {
		return "vector<" + t.parameters[0].text() + ">"
	}
	if t.kind != 7 {
		return primitives[t.kind]
	}
	value := t.address.String() + "::" + t.module + "::" + t.name
	if len(t.parameters) > 0 {
		parameters := make([]string, len(t.parameters))
		for i, parameter := range t.parameters {
			parameters[i] = parameter.text()
		}
		value += "<" + strings.Join(parameters, ",") + ">"
	}
	return value
}

func (w *transactionBCSWriter) typeTag(tag transactionTypeTag) {
	w.uleb(tag.kind)
	if tag.kind == 6 {
		w.typeTag(tag.parameters[0])
	}
	if tag.kind == 7 {
		w.address(tag.address)
		w.text(tag.module)
		w.text(tag.name)
		w.uleb(uint32(len(tag.parameters)))
		for _, parameter := range tag.parameters {
			w.typeTag(parameter)
		}
	}
}

func (r *transactionBCSReader) typeTag(depth int) transactionTypeTag {
	if r.err != nil {
		return transactionTypeTag{}
	}
	if depth >= maxMoveTypeDepth {
		r.fail("type_nesting=too_long")
		return transactionTypeTag{}
	}
	tag := transactionTypeTag{kind: r.uleb()}
	switch tag.kind {
	case 0, 1, 2, 3, 4, 5, 8, 9, 10:
	case 6:
		tag.parameters = []transactionTypeTag{r.typeTag(depth + 1)}
	case 7:
		tag.address, tag.module, tag.name = r.address(), r.text(), r.text()
		if !validMoveIdentifier(tag.module) || !validMoveIdentifier(tag.name) {
			r.fail("type_identifier=invalid")
		}
		tag.parameters = make([]transactionTypeTag, r.length(maxTransactionElements))
		for i := range tag.parameters {
			tag.parameters[i] = r.typeTag(depth + 1)
		}
	default:
		r.fail("type_variant=unsupported")
		return transactionTypeTag{}
	}
	return tag
}
