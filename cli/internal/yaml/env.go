// SPDX-License-Identifier: Apache-2.0

package yaml

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/goccy/go-yaml/token"
)

// EnvPath is a value in the raw config that references env vars.
type EnvPath struct {
	// Path in YAML path form, e.g. `$.chains[0].evm.rpc`.
	Path string

	// Template is the value as written in the file, without quotes, e.g. `https://a.example/${TOKEN}`.
	Template string

	// quote is the quote character the value was written with, or 0 if unquoted.
	quote byte
}

type envCollector struct {
	refs    []string
	anchors map[string]ast.Node
	paths   []EnvPath
}

// quotedTemplate writes a template with the quotes it was written with,
// so a value such as `"${DB}"` keeps protecting ` #` in the expanded value.
type quotedTemplate struct {
	value string
	quote byte
}

// pathSegment is a map key or, when key is empty, a list index.
type pathSegment struct {
	key   string
	index int
}

const envTokenPrefix = "__ibcenv_"

var (
	regexEnvToken  = regexp.MustCompile(envTokenPrefix + `(\d+)__`)
	regexPathIndex = regexp.MustCompile(`\[\d+\]`)
)

// PathMatches reports whether path matches pattern, where `[*]` in pattern matches any index:
// PathMatches("$.chains[2].evm.ics26Router", "$.chains[*].evm.ics26Router") == true.
func PathMatches(path, pattern string) bool {
	return path == pattern || regexPathIndex.ReplaceAllString(path, "[*]") == pattern
}

// FindEnvPaths returns every value in raw (unexpanded) YAML that references $VAR or ${VAR}.
func FindEnvPaths(raw []byte) ([]EnvPath, error) {
	tokenized, refs := tokenizeEnv(string(raw))
	if len(refs) == 0 {
		return []EnvPath{}, nil
	}

	file, err := parser.ParseBytes([]byte(tokenized), 0)
	switch {
	case err != nil:
		return nil, fmt.Errorf("unable to locate env references: %w", err)
	case len(file.Docs) != 1:
		return nil, fmt.Errorf("expected 1 document, got %d", len(file.Docs))

	}

	c := envCollector{
		refs:    refs,
		anchors: map[string]ast.Node{},
	}

	c.collect("$", file.Docs[0].Body)

	return c.paths, nil
}

// MaskEnvPaths renders v as an ordered YAML tree and puts each ENV template back at its path.
// ENV template is $var or ${var} or "a string with some ${var}", etc...
func MaskEnvPaths(v any, paths []EnvPath) (yaml.MapSlice, error) {
	bz, err := yaml.Marshal(v)
	if err != nil {
		return nil, err
	}

	var tree yaml.MapSlice
	if err := yaml.UnmarshalWithOptions(bz, &tree, yaml.UseOrderedMap()); err != nil {
		return nil, err
	}

	for _, p := range paths {
		segments, err := splitPath(p.Path)
		if err != nil {
			return nil, err
		}

		if node, ok := setPath(tree, segments, template(p)); ok {
			tree = node.(yaml.MapSlice)
		}
	}

	return tree, nil
}

// tokenizeEnv replaces each env reference with __ibcenv_N__ and returns the original texts by N.
// It follows os.Expand: a `$` not followed by a name is left as is,
// and invalid syntax such as `${}` counts as a reference because os.Expand removes it.
func tokenizeEnv(raw string) (string, []string) {
	var (
		out  strings.Builder
		refs []string
	)

	for i := 0; i < len(raw); i++ {
		if raw[i] != '$' || i+1 == len(raw) {
			out.WriteByte(raw[i])
			continue
		}

		name, width := shellName(raw[i+1:])
		if name == "" && width == 0 {
			out.WriteByte(raw[i])
			continue
		}

		fmt.Fprintf(&out, "%s%d__", envTokenPrefix, len(refs))
		refs = append(refs, raw[i:i+1+width])
		i += width
	}

	return out.String(), refs
}

// shellName mirrors the unexported os.getShellName: the variable name after a `$`
// and the number of bytes it spans.
func shellName(s string) (string, int) {
	if s[0] == '{' {
		if len(s) > 2 && isShellSpecialVar(s[1]) && s[2] == '}' {
			return s[1:2], 3
		}

		for i := 1; i < len(s); i++ {
			if s[i] == '}' {
				if i == 1 {
					return "", 2
				}

				return s[1:i], i + 1
			}
		}

		return "", 1
	}

	if isShellSpecialVar(s[0]) {
		return s[0:1], 1
	}

	var i int
	for i < len(s) && isAlphaNum(s[i]) {
		i++
	}

	return s[:i], i
}

func isShellSpecialVar(c byte) bool {
	return strings.IndexByte("*#$@!?-0123456789", c) >= 0
}

func isAlphaNum(c byte) bool {
	return c == '_' || '0' <= c && c <= '9' || 'a' <= c && c <= 'z' || 'A' <= c && c <= 'Z'
}

// collect walks node in document order, building paths as it goes,
// and records values holding a token with the references put back.
func (c *envCollector) collect(path string, node ast.Node) {
	switch n := node.(type) {
	case *ast.AnchorNode:
		c.collect(path, n.Value)
		c.anchors[n.Name.GetToken().Value] = n.Value
	case *ast.AliasNode:
		if value, ok := c.anchors[n.Value.GetToken().Value]; ok {
			c.collect(path, value)
		}
	case *ast.TagNode:
		c.collect(path, n.Value)
	case *ast.MappingNode:
		for _, entry := range n.Values {
			c.collect(path, entry)
		}
	case *ast.MappingValueNode:
		// merge keys (`<<: *base`) have no path of their own
		if !n.Key.IsMergeKey() {
			c.collect(path+"."+n.Key.GetToken().Value, n.Value)
		}
	case *ast.SequenceNode:
		for i, value := range n.Values {
			c.collect(fmt.Sprintf("%s[%d]", path, i), value)
		}
	case *ast.StringNode:
		var quote byte
		switch n.GetToken().Type {
		case token.DoubleQuoteType:
			quote = '"'
		case token.SingleQuoteType:
			quote = '\''
		}

		c.add(path, n.Value, quote)
	case *ast.LiteralNode:
		c.add(path, n.Value.Value, 0)
	}
}

func (c *envCollector) add(path, value string, quote byte) {
	if !strings.Contains(value, envTokenPrefix) {
		return
	}

	template := regexEnvToken.ReplaceAllStringFunc(value, func(tok string) string {
		n, _ := strconv.Atoi(regexEnvToken.FindStringSubmatch(tok)[1])
		return c.refs[n]
	})

	c.paths = append(c.paths, EnvPath{Path: path, Template: template, quote: quote})
}

func (q quotedTemplate) MarshalYAML() ([]byte, error) {
	if q.quote == '\'' {
		return []byte("'" + strings.ReplaceAll(q.value, "'", "''") + "'"), nil
	}

	return []byte(strconv.Quote(q.value)), nil
}

func template(p EnvPath) any {
	if p.quote == 0 {
		return p.Template
	}

	return quotedTemplate{value: p.Template, quote: p.quote}
}

// splitPath splits `$.a[0].b` into segments: key "a", index 0, key "b".
func splitPath(path string) ([]pathSegment, error) {
	rest, ok := strings.CutPrefix(path, "$")
	if !ok {
		return nil, fmt.Errorf("invalid path %q", path)
	}

	var segments []pathSegment
	for rest != "" {
		switch rest[0] {
		case '.':
			end := strings.IndexAny(rest[1:], ".[") + 1
			if end == 0 {
				end = len(rest)
			}

			segments = append(segments, pathSegment{key: rest[1:end]})
			rest = rest[end:]
		case '[':
			end := strings.IndexByte(rest, ']')
			if end < 0 {
				return nil, fmt.Errorf("invalid path %q", path)
			}

			index, err := strconv.Atoi(rest[1:end])
			if err != nil {
				return nil, fmt.Errorf("invalid path %q: %w", path, err)
			}

			segments = append(segments, pathSegment{index: index})
			rest = rest[end+1:]
		default:
			return nil, fmt.Errorf("invalid path %q", path)
		}
	}

	return segments, nil
}

// setPath sets value at segments under node and returns the updated node.
// Missing map keys are created; false if a list index is out of range or a scalar is in the way.
func setPath(node any, segments []pathSegment, value any) (any, bool) {
	if len(segments) == 0 {
		return value, true
	}

	seg := segments[0]

	if seg.key == "" {
		list, ok := node.([]any)
		if !ok || seg.index >= len(list) {
			return node, false
		}

		child, ok := setPath(list[seg.index], segments[1:], value)
		if ok {
			list[seg.index] = child
		}

		return list, ok
	}

	var mapping yaml.MapSlice
	switch n := node.(type) {
	case nil:
		// a dropped or empty block, e.g. `tls: {}` or a nil pointer
	case yaml.MapSlice:
		mapping = n
	default:
		return node, false
	}

	for i, item := range mapping {
		if item.Key != seg.key {
			continue
		}

		child, ok := setPath(item.Value, segments[1:], value)
		if ok {
			mapping[i].Value = child
		}

		return mapping, ok
	}

	child, ok := setPath(nil, segments[1:], value)
	if !ok {
		return node, false
	}

	return append(mapping, yaml.MapItem{Key: seg.key, Value: child}), true
}
