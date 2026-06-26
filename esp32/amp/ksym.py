#!/usr/bin/env python3
"""Minimal KiCad .kicad_sym S-expression reader for our generator.

Parses the symbol library just enough to:
  - pull a whole `(symbol "NAME" ...)` top-level block as text (for embedding in
    a schematic's lib_symbols), and
  - enumerate a symbol's pins with their (number, name, x, y, rot, length) so we
    can drop net labels exactly on each pin's connection point.
"""
import sys, re

def tokenize(s):
    # split into parens / atoms / quoted strings
    toks = re.findall(r'"(?:[^"\\]|\\.)*"|\(|\)|[^\s()]+', s)
    return toks

def parse(toks, i=0):
    # returns (node, next_index); node is list or atom
    assert toks[i] == '('
    node = []
    i += 1
    while toks[i] != ')':
        if toks[i] == '(':
            child, i = parse(toks, i)
            node.append(child)
        else:
            t = toks[i]
            if t.startswith('"'):
                t = t[1:-1].replace('\\"', '"')
            node.append(t)
            i += 1
    return node, i + 1

def load(path):
    with open(path) as f:
        txt = f.read()
    toks = tokenize(txt)
    root, _ = parse(toks, 0)
    return root  # ('kicad_symbol_lib' ... (symbol ...) ...)

def find_symbol(root, name):
    for node in root:
        if isinstance(node, list) and node and node[0] == 'symbol' and node[1] == name:
            return node
    return None

def symbol_block_text(path, name):
    """Return the raw text of the top-level (symbol "name" ...) block."""
    with open(path) as f:
        txt = f.read()
    needle = '(symbol "%s"' % name
    start = txt.index(needle)
    depth = 0
    i = start
    while i < len(txt):
        c = txt[i]
        if c == '(':
            depth += 1
        elif c == ')':
            depth -= 1
            if depth == 0:
                return txt[start:i+1]
        i += 1
    raise ValueError("unbalanced")

def get(node, key):
    for c in node:
        if isinstance(c, list) and c and c[0] == key:
            return c
    return None

def getall(node, key):
    return [c for c in node if isinstance(c, list) and c and c[0] == key]

def pins_of(root, name):
    """All pins across all units of symbol `name`, following (extends ...)."""
    sym = find_symbol(root, name)
    ext = get(sym, 'extends')
    if ext:
        name = ext[1]
        sym = find_symbol(root, name)
    out = []
    def walk(node):
        for c in node:
            if isinstance(c, list):
                if c and c[0] == 'pin':
                    at = get(c, 'at')
                    nm = get(c, 'name')
                    num = get(c, 'number')
                    out.append(dict(
                        number=num[1] if num else '?',
                        name=nm[1] if nm else '',
                        x=float(at[1]), y=float(at[2]),
                        rot=float(at[3]) if len(at) > 3 else 0.0,
                    ))
                else:
                    walk(c)
    walk(sym)
    return out

if __name__ == '__main__':
    path, name = sys.argv[1], sys.argv[2]
    root = load(path)
    ps = pins_of(root, name)
    print(f"# {name}: {len(ps)} pins")
    for p in sorted(ps, key=lambda d: (len(d['number']), d['number'])):
        print(f"  {p['number']:>4}  {p['name']:<10}  at ({p['x']:.2f},{p['y']:.2f}) rot {p['rot']:.0f}")
