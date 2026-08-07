package deployv3

import (
	"fmt"
	"strconv"
	"strings"
	"unicode"
)

// EvalWhen evaluates a small condition language:
// comparisons, &&, ||, !, parentheses, dotted identifiers, literals.
func EvalWhen(expression string, env map[string]any) (bool, error) {
	tokens, err := lexWhen(expression)
	if err != nil {
		return false, err
	}
	parser := whenParser{tokens: tokens}
	node, err := parser.parseExpr()
	if err != nil {
		return false, err
	}
	if !parser.atEnd() {
		return false, fmt.Errorf("when 表达式 %q 存在多余内容 %q", expression, parser.peek().text)
	}
	value, err := node.eval(env)
	if err != nil {
		return false, fmt.Errorf("when 表达式 %q: %w", expression, err)
	}
	return truthy(value), nil
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
			default:
				tokens = append(tokens, whenToken{kind: tokenIdent, text: text})
			}
			continue
		}
		if unicode.IsDigit(r) || (r == '-' && index+1 < len(runes) && unicode.IsDigit(runes[index+1])) {
			start := index
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
	for _, part := range n.parts {
		if current == nil {
			return nil, nil
		}
		value, ok := mapLookup(current, part)
		if !ok {
			return nil, nil
		}
		current = value
	}
	return current, nil
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

func compare(op string, left, right any) (bool, error) {
	equal, ordered, err := compareValues(left, right)
	if err != nil {
		return false, err
	}
	switch op {
	case "==":
		return equal, nil
	case "!=":
		return !equal, nil
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

func compareValues(left, right any) (bool, int, error) {
	leftNum, leftIsNum := asNumber(left)
	rightNum, rightIsNum := asNumber(right)
	if leftIsNum && rightIsNum {
		if leftNum == rightNum {
			return true, 0, nil
		}
		if leftNum < rightNum {
			return false, -1, nil
		}
		return false, 1, nil
	}
	leftStr, leftIsStr := asString(left)
	rightStr, rightIsStr := asString(right)
	if leftIsStr && rightIsStr {
		return leftStr == rightStr, strings.Compare(leftStr, rightStr), nil
	}
	if left == nil && right == nil {
		return true, 0, nil
	}
	if left == nil || right == nil {
		return left == right, 0, nil
	}
	return left == right, 0, nil
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
	}
	return true
}
