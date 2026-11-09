// SPDX-License-Identifier: Apache-2.0

package yaml

import (
	"fmt"
	"os"

	"github.com/goccy/go-yaml"
	"github.com/goccy/go-yaml/ast"
	"github.com/goccy/go-yaml/parser"
	"github.com/pkg/errors"
)

// ValueToFile encodes v into a single-document YAML AST.
func ValueToFile(v any) (*ast.File, error) {
	node, err := yaml.ValueToNode(v)
	if err != nil {
		return nil, err
	}

	return &ast.File{
		Docs: []*ast.DocumentNode{
			{Body: node},
		},
	}, nil
}

// NodeHasEnvValue reports whether scaler node contains ENV var reference like ${var} or $var
func NodeHasEnvValue(node ast.Node) bool {
	var raw string

	switch n := unwrap(node).(type) {
	case *ast.StringNode:
		raw = n.Value
	case *ast.LiteralNode:
		raw = n.Value.Value
	default:
		return false
	}

	// no real env vars is read, we're just reusing the helper
	var found bool
	os.Expand(raw, func(string) string {
		found = true
		return ""
	})

	return found
}

// Merge walks source and overlay side by side (mapping entries by key, sequence items by index)
// and puts the overlay node in place of the source node wherever condition(source, overlay) holds.
// An overlay mapping entry missing from source (e.g. dropped by omitempty) is appended when
// condition(nil, overlay) holds. Overlay entries without a source counterpart otherwise are skipped,
// so list items must keep their order; appended items are fine.
// Aliases are passed to condition resolved to their anchored value.
// Source is modified in place; overlay is left untouched.
func Merge(source, overlay *ast.File, condition func(source, overlay ast.Node) bool) (*ast.File, error) {
	if overlay == nil {
		return source, nil
	}

	// Overlay nodes are moved into source and re-indented (token columns drive the printer),
	// and go-yaml has no node clone, so work on a private copy.
	overlay, err := parser.ParseBytes([]byte(overlay.String()), parser.ParseComments)
	if err != nil {
		return nil, errors.Wrap(err, "unable to copy overlay")
	}

	if len(source.Docs) != 1 || len(overlay.Docs) != 1 {
		return nil, fmt.Errorf("expected 1 document, got %d and %d", len(source.Docs), len(overlay.Docs))
	}

	m := merger{
		condition: condition,
		anchors:   map[string]ast.Node{},
		emitted:   map[string]bool{},
	}

	for _, node := range ast.FilterFile(ast.AnchorType, overlay) {
		anchor := node.(*ast.AnchorNode)
		m.anchors[anchor.Name.GetToken().Value] = anchor.Value
	}

	doc := source.Docs[0]
	doc.Body = m.merge(doc.Body, overlay.Docs[0].Body)

	if m.err != nil {
		return nil, m.err
	}

	return source, nil
}

type merger struct {
	condition func(source, overlay ast.Node) bool

	// anchor name -> anchored value, from the overlay
	anchors map[string]ast.Node

	// anchors written into the source; an alias can only be written after its anchor
	emitted map[string]bool

	err error
}

// merge returns the node that should take src's place.
func (m *merger) merge(source, overlay ast.Node) ast.Node {
	if overlay == nil {
		return source
	}

	if m.condition(source, m.resolve(overlay)) {
		node := m.take(overlay)
		node.AddColumn(source.GetToken().Position.Column - node.GetToken().Position.Column)

		return node
	}

	switch s := unwrap(source).(type) {
	case *ast.MappingNode:
		o, ok := m.resolve(overlay).(*ast.MappingNode)
		if !ok {
			return source
		}

		// walk in source order: it is the output order, so anchors are emitted before their aliases
		for _, sv := range s.Values {
			ov := findMappingValue(o, sv.Key.GetToken().Value)
			switch {
			case ov == nil:
				continue
			case isEmptyMapping(sv.Value):
				// e.g. `tls: {}` rendered from a block whose values expanded to empty
				if node := m.prune(ov.Value); node != nil {
					node.AddColumn(sv.Key.GetToken().Position.Column - ov.Key.GetToken().Position.Column)
					sv.Value = node
				}
			default:
				sv.Value = m.merge(sv.Value, ov.Value)
			}
		}

		for _, ov := range o.Values {
			if len(s.Values) == 0 || findMappingValue(s, ov.Key.GetToken().Value) != nil {
				continue
			}

			// e.g. dropped by omitempty
			if node := m.prune(ov.Value); node != nil {
				entry := ast.MappingValue(ov.Start, ov.Key, node)
				entry.AddColumn(s.Values[0].Key.GetToken().Position.Column - ov.Key.GetToken().Position.Column)
				s.Values = append(s.Values, entry)
			}
		}
	case *ast.SequenceNode:
		o, ok := m.resolve(overlay).(*ast.SequenceNode)
		if !ok {
			return source
		}

		for i := range min(len(s.Values), len(o.Values)) {
			s.Values[i] = m.merge(s.Values[i], o.Values[i])
		}
	}

	return source
}

// prune returns the part of an overlay node that has no source counterpart but satisfies
// condition(nil, overlay): the node itself, or a mapping reduced to such entries. Nil if nothing is left.
func (m *merger) prune(overlay ast.Node) ast.Node {
	if m.condition(nil, m.resolve(overlay)) {
		return m.take(overlay)
	}

	// unwrap, not resolve: an aliased mapping is shared and must not be pruned in place
	o, ok := unwrap(overlay).(*ast.MappingNode)
	if !ok {
		return nil
	}

	kept := o.Values[:0]
	for _, ov := range o.Values {
		node := m.prune(ov.Value)
		if node == nil {
			continue
		}

		node.AddColumn(ov.Value.GetToken().Position.Column - node.GetToken().Position.Column)
		ov.Value = node
		kept = append(kept, ov)
	}

	if len(kept) == 0 {
		return nil
	}

	o.Values = kept

	return o
}

// take returns the overlay node to write into the source.
// An alias whose anchor was not written is replaced by a copy of the anchored value.
func (m *merger) take(ovl ast.Node) ast.Node {
	switch n := ovl.(type) {
	case *ast.AnchorNode:
		m.emitted[n.Name.GetToken().Value] = true
	case *ast.AliasNode:
		name := n.Value.GetToken().Value
		if m.emitted[name] {
			break
		}

		// copy: the anchored value can be aliased more than once
		f, err := parser.ParseBytes([]byte(m.anchors[name].String()), 0)
		if err != nil || len(f.Docs) != 1 {
			m.err = errors.Errorf("unable to inline alias %q: %v", name, err)
			break
		}

		return f.Docs[0].Body
	}

	return ovl
}

// resolve follows an alias to its anchored value and strips anchors and tags.
func (m *merger) resolve(node ast.Node) ast.Node {
	if alias, ok := node.(*ast.AliasNode); ok {
		node = m.anchors[alias.Value.GetToken().Value]
	}

	return unwrap(node)
}

// unwrap strips anchors and tags: `&name !!tag value` -> value
func unwrap(node ast.Node) ast.Node {
	for {
		switch n := node.(type) {
		case *ast.AnchorNode:
			node = n.Value
		case *ast.TagNode:
			node = n.Value
		default:
			return node
		}
	}
}

func isEmptyMapping(node ast.Node) bool {
	mapping, ok := unwrap(node).(*ast.MappingNode)

	return ok && len(mapping.Values) == 0
}

func findMappingValue(node *ast.MappingNode, key string) *ast.MappingValueNode {
	for _, v := range node.Values {
		if v.Key.GetToken().Value == key {
			return v
		}
	}

	return nil
}
