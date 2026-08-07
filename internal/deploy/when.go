package deploy

import (
	"fmt"
	"reflect"
	"strconv"
	"strings"
	"unicode"
)

// EvalWhen evaluates a small condition language:
// comparisons, &&, ||, !, parentheses, dotted identifiers, literals,
// `is defined`, `in` and `not in`. Referencing an undefined variable is an
// error, matching Ansible's behavior; use `is defined` for optional values.
func EvalWhen(expression string, env map[string]any) (bool, error) {
	node, err := parseWhen(expression)
	if err != nil {
		return false, err
	}
	value, err := node.eval(env)
	if err != nil {
		return false, fmt.Errorf("when 表达式 %q: %w", expression, err)
	}
	return truthy(value), nil
}

// ParseWhen checks the syntax of a when expression without evaluating it.
// Plan-time validation uses this so register/facts variables do not need to
// exist yet; undefined-variable errors are only reported at execution time.
func ParseWhen(expression string) error {
	_, err := parseWhen(expression)
	return err
}

func parseWhen(expression string) (whenNode, error) {
	tokens, err := lexWhen(expression)
	if err != nil {
		return nil, err
	}
	parser := whenParser{tokens: tokens}
	node, err := parser.parseExpr()
	if err != nil {
		return nil, err
	}
	if !parser.atEnd() {
		return nil, fmt.Errorf("when 表达式 %q 存在多余内容 %q", expression, parser.peek().text)
	}
	return node, nil
}

type whenTokenKind int

const (
	tokenIdent whenTokenKind = iota
	tokenNumber
	tokenString
	tokenOp
	tokenEOF
)

type whenToken struct {
	kind whenTokenKind
	text string
}

func lexWhen(input string) ([]whenToken, error) {
	var tokens []whenToken
	runes := []rune(input)
	for index := 0; index < len(runes); {
		r := runes[index]
		if unicode.IsSpace(r) {
			index++
			continue
		}
		if isIdentStart(r) {
			start := index
			for index < len(runes) && isIdentPart(runes[index]) {
				index++
			}
			text := string(runes[start:index])
			switch text {
			case "true", "false", "null", "nil":
				tokens = append(tokens, whenToken{kind: tokenNumber, text: text})
			case "and":
				tokens = append(tokens, whenToken{kind: tokenOp, text: "&&"})
			case "or":
				tokens = append(tokens, whenToken{kind: tokenOp, text: "||"})
			default:
				tokens = append(tokens, whenToken{kind: tokenIdent, text: text})
			}
			continue
		}
		if unicode.IsDigit(r) || (r == '-' && index+1 < len(runes) && unicode.IsDigit(runes[index+1])) {
			start := index
			if runes[index] == '-' {
				index++
			}
			for index < len(runes) && (unicode.IsDigit(runes[index]) || runes[index] == '.') {
				index++
			}
			tokens = append(tokens, whenToken{kind: tokenNumber, text: string(runes[start:index])})
			continue
		}
		if r == '\'' || r == '"' {
			quote := r
			index++
			start := index
			for index < len(runes) && runes[index] != quote {
				index++
			}
			if index >= len(runes) {
				return nil, fmt.Errorf("when 表达式字符串未闭合")
			}
			tokens = append(tokens, whenToken{kind: tokenString, text: string(runes[start:index])})
			index++
			continue
		}
		two := ""
		if index+1 < len(runes) {
			two = string(runes[index]) + string(runes[index+1])
		}
		switch two {
		case "==", "!=", "<=", ">=", "&&", "||":
			tokens = append(tokens, whenToken{kind: tokenOp, text: two})
			index += 2
			continue
		}
		switch string(r) {
		case "<", ">", "!", "(", ")", ".":
			tokens = append(tokens, whenToken{kind: tokenOp, text: string(r)})
			index++
			continue
		}
		return nil, fmt.Errorf("when 表达式包含非法字符 %q", string(r))
	}
	tokens = append(tokens, whenToken{kind: tokenEOF})
	return tokens, nil
}

func isIdentStart(r rune) bool {
	return unicode.IsLetter(r) || r == '_'
}

func isIdentPart(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_'
}

type whenParser struct {
	tokens []whenToken
	index  int
}

func (p *whenParser) peek() whenToken {
	return p.tokens[p.index]
}

func (p *whenParser) next() whenToken {
	token := p.tokens[p.index]
	if token.kind != tokenEOF {
		p.index++
	}
	return token
}

func (p *whenParser) atEnd() bool {
	return p.peek().kind == tokenEOF
}

func (p *whenParser) matchOp(ops ...string) bool {
	token := p.peek()
	if token.kind == tokenOp {
		for _, op := range ops {
			if token.text == op {
				p.next()
				return true
			}
		}
	}
	return false
}

func (p *whenParser) parseExpr() (whenNode, error) {
	return p.parseOr()
}

func (p *whenParser) parseOr() (whenNode, error) {
	left, err := p.parseAnd()
	if err != nil {
		return nil, err
	}
	for p.matchOp("||") {
		right, err := p.parseAnd()
		if err != nil {
			return nil, err
		}
		left = whenBinary{op: "||", left: left, right: right}
	}
	return left, nil
}

func (p *whenParser) parseAnd() (whenNode, error) {
	left, err := p.parseNot()
	if err != nil {
		return nil, err
	}
	for p.matchOp("&&") {
		right, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		left = whenBinary{op: "&&", left: left, right: right}
	}
	return left, nil
}

func (p *whenParser) parseNot() (whenNode, error) {
	if p.matchOp("!") {
		child, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return whenUnary{child: child}, nil
	}
	if p.peek().kind == tokenIdent && p.peek().text == "not" {
		p.next()
		child, err := p.parseNot()
		if err != nil {
			return nil, err
		}
		return whenUnary{child: child}, nil
	}
	return p.parseComparison()
}

func (p *whenParser) parseComparison() (whenNode, error) {
	left, err := p.parsePrimary()
	if err != nil {
		return nil, err
	}
	if p.matchOp("==", "!=", "<", "<=", ">", ">=") {
		op := p.tokens[p.index-1].text
		right, err := p.parsePrimary()
		if err != nil {
			return nil, err
		}
		return whenBinary{op: op, left: left, right: right}, nil
	}
	if p.peek().kind == tokenIdent {
		switch p.peek().text {
		case "is":
			p.next()
			defined := true
			if p.peek().kind == tokenIdent && p.peek().text == "not" {
				p.next()
				defined = false
			}
			if p.peek().kind != tokenIdent || p.peek().text != "defined" {
				return nil, fmt.Errorf("is 后应为 defined 或 not defined")
			}
			p.next()
			return whenIsDefined{node: left, defined: defined}, nil
		case "in":
			p.next()
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			return whenMembership{left: left, right: right, negate: false}, nil
		case "not":
			p.next()
			if p.peek().kind != tokenIdent || p.peek().text != "in" {
				return nil, fmt.Errorf("not 后应为 in")
			}
			p.next()
			right, err := p.parsePrimary()
			if err != nil {
				return nil, err
			}
			return whenMembership{left: left, right: right, negate: true}, nil
		}
	}
	return left, nil
}

func (p *whenParser) parsePrimary() (whenNode, error) {
	token := p.next()
	switch token.kind {
	case tokenNumber:
		switch token.text {
		case "true":
			return whenLiteral{value: true}, nil
		case "false":
			return whenLiteral{value: false}, nil
		case "null", "nil":
			return whenLiteral{value: nil}, nil
		}
		number, err := strconv.ParseFloat(token.text, 64)
		if err != nil {
			return nil, fmt.Errorf("非法数字 %q", token.text)
		}
		return whenLiteral{value: number}, nil
	case tokenString:
		return whenLiteral{value: token.text}, nil
	case tokenIdent:
		parts := []string{token.text}
		for p.matchOp(".") {
			next := p.next()
			if next.kind != tokenIdent {
				return nil, fmt.Errorf("字段访问后缺少名称")
			}
			parts = append(parts, next.text)
		}
		return whenPath{parts: parts}, nil
	case tokenOp:
		if token.text == "(" {
			node, err := p.parseExpr()
			if err != nil {
				return nil, err
			}
			if !p.matchOp(")") {
				return nil, fmt.Errorf("缺少右括号")
			}
			return node, nil
		}
	}
	return nil, fmt.Errorf("意外 token %q", token.text)
}

type whenNode interface {
	eval(env map[string]any) (any, error)
}

type whenLiteral struct {
	value any
}

func (n whenLiteral) eval(map[string]any) (any, error) {
	return n.value, nil
}

type whenPath struct {
	parts []string
}

func (n whenPath) eval(env map[string]any) (any, error) {
	var current any = any(env)
	for index, part := range n.parts {
		if current == nil {
			return nil, undefinedVarError(strings.Join(n.parts[:index], "."))
		}
		value, ok := mapLookup(current, part)
		if !ok {
			return nil, undefinedVarError(strings.Join(n.parts[:index+1], "."))
		}
		current = value
	}
	return current, nil
}

func undefinedVarError(name string) error {
	return fmt.Errorf("未定义变量 %q", name)
}

type whenUnary struct {
	child whenNode
}

func (n whenUnary) eval(env map[string]any) (any, error) {
	value, err := n.child.eval(env)
	if err != nil {
		return nil, err
	}
	return !truthy(value), nil
}

type whenBinary struct {
	op          string
	left, right whenNode
}

func (n whenBinary) eval(env map[string]any) (any, error) {
	left, err := n.left.eval(env)
	if err != nil {
		return nil, err
	}
	switch n.op {
	case "&&":
		if !truthy(left) {
			return false, nil
		}
		right, err := n.right.eval(env)
		if err != nil {
			return nil, err
		}
		return truthy(right), nil
	case "||":
		if truthy(left) {
			return true, nil
		}
		right, err := n.right.eval(env)
		if err != nil {
			return nil, err
		}
		return truthy(right), nil
	}
	right, err := n.right.eval(env)
	if err != nil {
		return nil, err
	}
	return compare(n.op, left, right)
}

// whenIsDefined reports whether the wrapped node resolves without an
// undefined-variable error. Only paths can be undefined; literals are always
// defined.
type whenIsDefined struct {
	node    whenNode
	defined bool
}

func (n whenIsDefined) eval(env map[string]any) (any, error) {
	path, ok := n.node.(whenPath)
	if !ok {
		return n.defined, nil
	}
	_, err := path.eval(env)
	return err == nil == n.defined, nil
}

// whenMembership implements `item in container` and `item not in container`.
type whenMembership struct {
	left, right whenNode
	negate      bool
}

func (n whenMembership) eval(env map[string]any) (any, error) {
	left, err := n.left.eval(env)
	if err != nil {
		return nil, err
	}
	right, err := n.right.eval(env)
	if err != nil {
		return nil, err
	}
	matches, err := membership(right, left)
	if err != nil {
		return nil, err
	}
	if n.negate {
		matches = !matches
	}
	return matches, nil
}

func membership(container, item any) (bool, error) {
	switch typed := container.(type) {
	case []any:
		for _, element := range typed {
			equal, _, _, err := compareValues(item, element)
			if err != nil {
				return false, err
			}
			if equal {
				return true, nil
			}
		}
		return false, nil
	case []string:
		text, ok := item.(string)
		if !ok {
			return false, nil
		}
		for _, element := range typed {
			if element == text {
				return true, nil
			}
		}
		return false, nil
	case string:
		text, ok := item.(string)
		if !ok {
			return false, nil
		}
		return strings.Contains(typed, text), nil
	default:
		return false, fmt.Errorf("in 右侧必须是列表或字符串（得到 %T）", container)
	}
}

func compare(op string, left, right any) (bool, error) {
	equal, ordered, orderable, err := compareValues(left, right)
	if err != nil {
		return false, err
	}
	switch op {
	case "==":
		return equal, nil
	case "!=":
		return !equal, nil
	}
	if !orderable {
		return false, fmt.Errorf("运算符 %s 不能用于比较 %T 与 %T", op, left, right)
	}
	switch op {
	case "<":
		return ordered < 0, nil
	case "<=":
		return ordered <= 0, nil
	case ">":
		return ordered > 0, nil
	case ">=":
		return ordered >= 0, nil
	}
	return false, fmt.Errorf("未知比较符 %q", op)
}

// compareValues returns equality, ordering and whether ordering is supported
// for the pair. Structured values (lists, maps) compare by deep equality and
// never panic; ordering is only defined for numbers and strings.
func compareValues(left, right any) (equal bool, ordered int, orderable bool, err error) {
	leftNum, leftIsNum := asNumber(left)
	rightNum, rightIsNum := asNumber(right)
	if leftIsNum && rightIsNum {
		switch {
		case leftNum < rightNum:
			return false, -1, true, nil
		case leftNum > rightNum:
			return false, 1, true, nil
		default:
			return true, 0, true, nil
		}
	}
	leftStr, leftIsStr := asString(left)
	rightStr, rightIsStr := asString(right)
	if leftIsStr && rightIsStr {
		return leftStr == rightStr, strings.Compare(leftStr, rightStr), true, nil
	}
	if left == nil && right == nil {
		return true, 0, false, nil
	}
	if left == nil || right == nil {
		return false, 0, false, nil
	}
	return reflect.DeepEqual(left, right), 0, false, nil
}

func asNumber(value any) (float64, bool) {
	switch typed := value.(type) {
	case int:
		return float64(typed), true
	case int64:
		return float64(typed), true
	case float64:
		return typed, true
	case float32:
		return float64(typed), true
	}
	return 0, false
}

func asString(value any) (string, bool) {
	text, ok := value.(string)
	return text, ok
}

func truthy(value any) bool {
	if value == nil {
		return false
	}
	switch typed := value.(type) {
	case bool:
		return typed
	case string:
		return typed != ""
	case int:
		return typed != 0
	case int64:
		return typed != 0
	case float64:
		return typed != 0
	case []any:
		return len(typed) > 0
	case []string:
		return len(typed) > 0
	case map[string]any:
		return len(typed) > 0
	case Vars:
		return len(typed) > 0
	}
	return true
}
